package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/PickStranger/reverse-proxy-agent/internal/config"
	aigrpc "github.com/PickStranger/reverse-proxy-agent/internal/grpc"
	"github.com/PickStranger/reverse-proxy-agent/internal/middleware"
	redisclient "github.com/PickStranger/reverse-proxy-agent/internal/redis"
)

func main() {
	cfg := config.Load()
	aiClient := aigrpc.NewMockAIClient(cfg.BlockThreshold)

	log.Printf("설정: 임계값=%.1f, TTL=%d초, AI타임아웃=%v", cfg.BlockThreshold, cfg.BlacklistTTL, cfg.AITimeout)
	log.Printf("화이트리스트: %v", cfg.WhitelistIPs)

	target, err := url.Parse(cfg.TargetURL)
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	rc := redisclient.NewClient(cfg.RedisAddr)
	ctx := context.Background()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fp := middleware.ExtractFingerprint(r)

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
			log.Printf("차단됨(Redis): %s", fp.Hash)
			return
		}

		resp, err := aiClient.Analyze(ctx, fp.Hash, fp.IP, fp.UserAgent)
		if err != nil {
			log.Printf("AI 오류: %v", err)
		} else if resp.Block {
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			log.Printf("차단됨(AI): score=%.2f hash=%s", resp.RiskScore, fp.Hash)
			return
		}

		r.Header.Set("X-Device-Id", fp.Hash)
		r.Header.Set("X-Risk-Score", fmt.Sprintf("%.2f", resp.RiskScore))

		log.Printf("통과: %s | score=%.2f", fp.IP, resp.RiskScore)
		proxy.ServeHTTP(w, r)
	})

	log.Printf("리버스 프록시 시작 :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatal(err)
	}
}
