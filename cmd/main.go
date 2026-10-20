package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/PickStranger/reverse-proxy-agent/internal/config"
	aigrpc "github.com/PickStranger/reverse-proxy-agent/internal/grpc"
	"github.com/PickStranger/reverse-proxy-agent/internal/middleware"
	redisclient "github.com/PickStranger/reverse-proxy-agent/internal/redis"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg := config.Load()
	rc := redisclient.NewClient(cfg.RedisAddr)
	ctx := context.Background()

	// ─── AI 클라이언트 선택: Mock vs 실제 gRPC ─────────────
	aiClient := newAIClient(cfg)
	aiClient = aigrpc.NewCircuitBreaker(aiClient, cfg.CircuitBreakerThreshold, cfg.CircuitBreakerCooldown)
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
	if cfg.TargetInsecureSkipVerify {
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		log.Printf("주의: 타깃 TLS 인증서 검증 비활성화됨 (self-signed 용)")
	}

	if !cfg.PreserveHostHeader {
		baseDirector := proxy.Director
		proxy.Director = func(r *http.Request) {
			baseDirector(r)
			r.Host = target.Host
		}
	}

	// ─── mux 생성 ─────────────────────────────────────────
	mux := http.NewServeMux()

	// ─── 1단계: 모든 요청 처리 ───────────────────────────────
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fp, sessionToken, err := middleware.ExtractAndStoreFingerprint(r, rc)
		if err != nil {
			log.Printf("fingerprint 저장 실패: %v", err)
			proxy.ServeHTTP(w, r)
			return
		}

		if cfg.WhitelistIPs[fp.IP] {
			log.Printf("화이트리스트 통과: %s", fp.IP)
			proxy.ServeHTTP(w, r)
			return
		}

		blocked, err := rc.IsBlacklisted(ctx, fp.Hash)
		if err != nil {
			log.Printf("Redis 오류: %v", err)
		}
		if blocked {
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			log.Printf("차단됨(블랙리스트): %s", fp.Hash)
			return
		}

		r.Header.Set("X-Session-Token", sessionToken)
		log.Printf("포워딩: %s | session=%s", fp.IP, sessionToken)
		proxy.ServeHTTP(w, r)
	})

	// ─── 2단계: SDK가 user_id + session_token 보내오는 엔드포인트 ───
	mux.HandleFunc("/internal/analyze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionToken string `json:"session_token"`
			UserID       string `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		var fp middleware.Fingerprint
		if err := rc.GetFingerprint(ctx, req.SessionToken, &fp); err != nil {
			log.Printf("fingerprint 조회 실패: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		resp, err := aiClient.Predict(ctx, &aigrpc.PredictInput{
			UserID:         req.UserID,
			IP:             fp.IP,
			LoginTimestamp: time.Now().Format(time.RFC3339),
			IsSuccess:      true,
			StoreEvent:     true,
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				log.Printf("AI 타임아웃: user=%s", req.UserID)
				http.Error(w, "AI Server Timeout", http.StatusGatewayTimeout) // 504
			} else {
				log.Printf("AI 오류: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError) // 500
			}
			return
		}

		_ = rc.DeleteFingerprint(ctx, req.SessionToken)

		// risk_score가 정상 범위(0.0~1.0)를 벗어나면 로그로 남겨 AI 팀과 공유할 근거를 확보
		if resp.RiskScore < 0 || resp.RiskScore > 1 {
			log.Printf("경고: risk_score 비정상 범위 감지 score=%.2f user=%s (0.0~1.0 예상)", resp.RiskScore, req.UserID)
		}

		if resp.IsAnomaly {
			_ = rc.AddBlacklist(ctx, fp.Hash, cfg.BlacklistTTL)
			log.Printf("차단됨(AI): score=%.2f user=%s", resp.RiskScore, req.UserID)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"risk_score": resp.RiskScore,
			"is_anomaly": resp.IsAnomaly,
			"action":     actionFromResult(resp.IsAnomaly, resp.RiskScore),
		})
	})

	// ─── Prometheus 메트릭 엔드포인트 ───────────────────────
	mux.Handle("/metrics", promhttp.Handler())

	// ─── 메트릭 미들웨어 적용 후 서버 시작 ───────────────────
	handler := middleware.MetricsMiddleware(mux)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("리버스 프록시 시작 :%s", cfg.Port)
	if err := srv.ListenAndServe(); err != nil {
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

// / actionFromResult는 is_anomaly를 1차 판단 기준으로 삼는다.
// AI 서버의 risk_score가 아직 0.0~1.0 스케일을 보장하지 않는 상태라
// (예: 37.75 같은 비정상 값 관측됨) score 단독으로는 액션을 결정하지 않는다.
// TODO: AI 팀과 risk_score 스케일 정합성 확인 후 score 기반 세분화 로직 복원
func actionFromResult(isAnomaly bool, score float32) string {
	if isAnomaly {
		return "BLOCK"
	}
	// score가 정상 범위(0.0~1.0)일 때만 보조적으로 CHALLENGE 판단
	if score >= 0 && score <= 1 && score >= 0.4 {
		return "CHALLENGE"
	}
	return "ALLOW"
}
