package main

import (
	"context"
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
	aiClient := aigrpc.NewMockAIClient(cfg.BlockThreshold)
	rc := redisclient.NewClient(cfg.RedisAddr)
	ctx := context.Background()

	log.Printf("설정: 임계값=%.1f, TTL=%v, AI타임아웃=%v", cfg.BlockThreshold, cfg.BlacklistTTL, cfg.AITimeout)
	log.Printf("화이트리스트: %v", cfg.WhitelistIPs)

	target, err := url.Parse(cfg.TargetURL)
	if err != nil {
		log.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

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

// riskscore 기반 액션 결정
func actionFromScore(score float32) string {
	if score >= 0.7 {
		return "BLOCK"
	} else if score >= 0.4 {
		return "CHALLENGE"
	}
	return "ALLOW"
}
