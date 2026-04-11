package middleware

import (
	"crypto/sha256"
	"fmt"
	"net/http"
)

type Fingerprint struct {
	IP        string
	UserAgent string
	Language  string
	Hash      string
}

func ExtractFingerprint(r *http.Request) Fingerprint {
	ip := r.RemoteAddr
	ua := r.Header.Get("User-Agent")
	lang := r.Header.Get("Accept-Language")

	raw := ip + ua + lang
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))

	return Fingerprint{
		IP:        ip,
		UserAgent: ua,
		Language:  lang,
		Hash:      hash,
	}
}
