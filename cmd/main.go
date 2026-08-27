package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/PickStranger/reverse-proxy-agent/internal/config"
	aigrpc "github.com/PickStranger/reverse-proxy-agent/internal/grpc"
	"github.com/PickStranger/reverse-proxy-agent/internal/middleware"
	redisclient "github.com/PickStranger/reverse-proxy-agent/internal/redis"
)

func main() {
	cfg := config.Load()
	rc := redisclient.NewClient(cfg.RedisAddr)
	ctx := context.Background()

	// ─── AI 클라이언트 선택: Mock vs 실제 gRPC ─────────────
	aiClient := newAIClient(cfg)
	if closer, ok := aiClient.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	log.Printf("설정: 임계값=%.1f, TTL=%v, AI타임아웃=%v", cfg.BlockThreshold, cfg.BlacklistTTL, cfg.AITimeout)
	log.Printf("화이트리스트: %v", cfg.WhitelistIPs)
	log.Printf("타깃(서비스앱): %s | AI모드: mock=%v grpc=%s", cfg.TargetURL, cfg.UseMockAI, cfg.AIGrpcAddr)

	target, err := url.Parse(cfg.TargetURL)
	if err != nil {
		log.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	// ─── 서비스앱 HTTPS 대응 ────────────────────────────────
	// self-signed 인증서면 TLS 검증 skip (env: TARGET_INSECURE_SKIP_VERIFY)
	if cfg.TargetInsecureSkipVerify {
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		log.Printf("주의: 타깃 TLS 인증서 검증 비활성화됨 (self-signed 용)")
	}

	// HTTPS vhost/SNI 정합을 위해 기본적으로 Host 헤더를 타깃으로 재작성.
	// 원본 Host를 유지하려면 PRESERVE_HOST_HEADER=true.
	if !cfg.PreserveHostHeader {
		baseDirector := proxy.Director
		proxy.Director = func(r *http.Request) {
			baseDirector(r)
			r.Host = target.Host
		}
	}

	// ─── 1단계: 모든 요청 처리 ───────────────────────────────
	// fingerprint 수집 → Redis 저장 → session_token 헤더 추가 → 포워딩
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fp, sessionToken, err := middleware.ExtractAndStoreFingerprint(r, rc)
		if err != nil {
			log.Printf("fingerprint 저장 실패: %v", err)
			// Fail-Open: 실패해도 서비스는 통과
			proxy.ServeHTTP(w, r)
			return
		}

		// 화이트리스트 체크
		if cfg.WhitelistIPs[fp.IP] {
			log.Printf("화이트리스트 통과: %s", fp.IP)
			proxy.ServeHTTP(w, r)
			return
		}

		// 블랙리스트 체크
		blocked, err := rc.IsBlacklisted(ctx, fp.Hash)
		if err != nil {
			log.Printf("Redis 오류: %v", err)
		}
		if blocked {
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			log.Printf("차단됨(블랙리스트): %s", fp.Hash)
			return
		}

		// session_token 헤더에 얹어서 포워딩
		r.Header.Set("X-Session-Token", sessionToken)
		log.Printf("포워딩: %s | session=%s", fp.IP, sessionToken)
		proxy.ServeHTTP(w, r)
	})

	// ─── 2단계: SDK가 user_id + session_token 보내오는 엔드포인트 ───
	// OAuth 완료 후 SDK가 호출 → AI 판정 → riskscore 반환
	http.HandleFunc("/internal/analyze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		// 요청 파싱
		var req struct {
			SessionToken string `json:"session_token"`
			UserID       string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Redis에서 fingerprint 조회
		var fp middleware.Fingerprint
		if err := rc.GetFingerprint(ctx, req.SessionToken, &fp); err != nil {
			log.Printf("fingerprint 조회 실패: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// AI 판정 요청
		resp, err := aiClient.Analyze(ctx, &aigrpc.AnalyzeRequest{
			SessionToken:    req.SessionToken,
			UserID:          req.UserID,
			IP:              fp.IP,
			UserAgent:       fp.UserAgent,
			FingerprintHash: fp.Hash,
			Timestamp:       time.Now().Unix(),
		})
		if err != nil {
			log.Printf("AI 오류: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// 사용 끝난 session_token 삭제
		_ = rc.DeleteFingerprint(ctx, req.SessionToken)

		// 블랙리스트 등록 (고위험)
		if resp.Block {
			_ = rc.AddBlacklist(ctx, fp.Hash, cfg.BlacklistTTL)
			log.Printf("차단됨(AI): score=%.2f user=%s", resp.RiskScore, req.UserID)
		}

		// SDK에 결과 반환
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"risk_score": resp.RiskScore,
			"block":      resp.Block,
			"action":     actionFromScore(resp.RiskScore),
		})
	})

	log.Printf("리버스 프록시 시작 :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatal(err)
	}
}

// newAIClient는 설정에 따라 Mock 또는 실제 gRPC AI 클라이언트를 만든다.
func newAIClient(cfg *config.Config) aigrpc.AIClient {
	if cfg.UseMockAI {
		log.Printf("AI 모드: Mock (USE_MOCK_AI=true)")
		return aigrpc.NewMockAIClient(cfg.BlockThreshold)
	}

	client, err := aigrpc.NewGRPCAIClient(cfg.AIGrpcAddr, cfg.AIGrpcTLS, cfg.AITimeout)
	if err != nil {
		log.Fatalf("AI gRPC 연결 실패: %v", err)
	}
	return client
}

// riskscore 기반 액션 결정
func actionFromScore(score float32) string {
	if score >= 0.7 {
		return "BLOCK"
	} else if score >= 0.4 {
		return "CHALLENGE"
	}
	return "ALLOW"
}
