package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/PickStranger/reverse-proxy-agent/internal/middleware"
	"github.com/PickStranger/reverse-proxy-agent/internal/redis"
)

type Injector struct {
	proxy       *httputil.ReverseProxy
	redisClient *redis.Client
}

func NewInjector(targetURL string, redisClient *redis.Client) (*Injector, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	return &Injector{
		proxy:       proxy,
		redisClient: redisClient,
	}, nil
}

func (i *Injector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. fingerprint 수집 + Redis 저장 + session_token 발급
	_, sessionToken, err := middleware.ExtractAndStoreFingerprint(r, i.redisClient)
	if err != nil {
		log.Printf("fingerprint 저장 실패: %v", err)
		// Fail-Open: 실패해도 서비스는 통과시킴
		i.proxy.ServeHTTP(w, r)
		return
	}

	// 2. session_token 헤더에 추가
	r.Header.Set("X-Session-Token", sessionToken)

	// 3. 서비스 서버로 포워딩
	i.proxy.ServeHTTP(w, r)
}
