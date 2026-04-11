package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/PickStranger/reverse-proxy-agent/internal/middleware"
	redisclient "github.com/PickStranger/reverse-proxy-agent/internal/redis"
)

func main() {
	target, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	rc := redisclient.NewClient("localhost:6379")
	ctx := context.Background()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fp := middleware.ExtractFingerprint(r)

		blocked, err := rc.IsBlacklisted(ctx, fp.Hash)
		if err != nil {
			log.Printf("Redis 오류: %v", err)
		}

		if blocked {
			http.Error(w, "403 Forbidden", http.StatusForbidden)
			log.Printf("차단됨: %s", fp.Hash)
			return
		}

		log.Printf("Fingerprint: %s | IP: %s | UA: %s", fp.Hash, fp.IP, fp.UserAgent)
		proxy.ServeHTTP(w, r)
	})

	log.Println("리버스 프록시 시작 :9000")
	if err := http.ListenAndServe(":9000", nil); err != nil {
		log.Fatal(err)
	}
}
