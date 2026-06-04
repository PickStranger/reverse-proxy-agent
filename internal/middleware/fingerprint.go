package middleware

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"time"

	redisclient "github.com/PickStranger/reverse-proxy-agent/internal/redis"
)

type Fingerprint struct {
	IP        string
	UserAgent string
	Language  string
	Hash      string
}

func ExtractAndStoreFingerprint(r *http.Request, rc *redisclient.Client) (Fingerprint, string, error) {
	// 1. fingerprint 추출
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	ua := r.Header.Get("User-Agent")
	lang := r.Header.Get("Accept-Language")

	raw := ip + ua + lang
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))

	fp := Fingerprint{
		IP:        ip,
		UserAgent: ua,
		Language:  lang,
		Hash:      hash,
	}

	// 2. session_token 발급 (session_token.go에서 호출)
	sessionToken, err := generateSessionToken()
	if err != nil {
		return fp, "", err
	}

	// 3. Redis에 임시 저장 (TTL 60초)
	ctx := context.Background()
	err = rc.StoreFingerprint(ctx, sessionToken, fp, 60*time.Second)
	if err != nil {
		return fp, "", err
	}

	return fp, sessionToken, nil
}
