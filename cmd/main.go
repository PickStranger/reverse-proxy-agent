package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	// 백엔드 서비스 주소 (나중에 env로 분리)
	target, err := url.Parse("http://localhost:8080")
	if err != nil {
		log.Fatal(err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	log.Println("리버스 프록시 시작 :9000")
	if err := http.ListenAndServe(":9000", proxy); err != nil {
		log.Fatal(err)
	}
}