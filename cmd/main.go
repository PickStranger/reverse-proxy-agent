package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PickStranger/reverse-proxy-agent/internal/middleware"
	redisclient "github.com/PickStranger/reverse-proxy-agent/internal/redis"
	"github.com/joho/godotenv"
)

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func main() {
	godotenv.Load()

	targetURL := getEnv("TARGET_URL", "https://localhost:8080")
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	port := getEnv("PORT", "9000")
	blockThreshold, _ := strconv.ParseFloat(getEnv("BLOCK_THRESHOLD", "0.8"), 64)
	blacklistTTL, _ := strconv.Atoi(getEnv("BLACKLIST_TIL", "3600"))
	whitelistIPs := strings.Split(getEnv("WHITELIST_IPS", ""), ",")
	aiTimeout, _ := strconv.Atoi(getEnv("AI_TIMEOUT", "5"))

	log.Printf("설정: 임계값=%.1f, TTL=%d초, AI타임아웃=%d초", blockThreshold, blacklistTTL, aiTimeout)
	log.Printf("화이트리스트: %v", whitelistIPs)

	target, err := url.Parse(targetURL)
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	rc := redisclient.NewClient(redisAddr)
	ctx := context.Background()

	whitelist := make(map[string]bool)
	for _, ip := range whitelistIPs {
		whitelist[strings.TrimSpace(ip)] = true
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fp := middleware.ExtractFingerprint(r)

		if whitelist[fp.IP] {
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
			log.Printf("차단됨: %s", fp.Hash)
			return
		}

		_ = time.Duration(aiTimeout) * time.Second

		log.Printf("Fingerprint: %s | IP: %s | UA: %s", fp.Hash, fp.IP, fp.UserAgent)
		proxy.ServeHTTP(w, r)
	})

	log.Printf("리버스 프록시 시작 :%s", port)
	if err := http.ListenAndServe(":9000", nil); err != nil {
		log.Fatal(err)
	}
}
