package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/PickStranger/reverse-proxy-agent/internal/middleware"
)

func main() {
	target, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fp := middleware.ExtractFingerprint(r)
		log.Printf("Fingerprint: %s | IP: %s | UA: %s", fp.Hash, fp.IP, fp.UserAgent)
		proxy.ServeHTTP(w, r)
	})

	log.Println("리버스 프록시 시작 :9000")
	if err := http.ListenAndServe(":9000", nil); err != nil {
		log.Fatal(err)
	}
}
