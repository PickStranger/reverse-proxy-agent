package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// 요청 총 개수 (경로, 메서드, 상태코드별)
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rev_http_requests_total",
			Help: "Total number of HTTP requests processed by rev",
		},
		[]string{"path", "method", "status"},
	)

	// 요청 처리 지연시간 (히스토그램 → p50/p95/p99 계산 가능)
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rev_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets, // 0.005 ~ 10s 기본 버킷
		},
		[]string{"path", "method"},
	)

	// AI 서버 gRPC 호출 지연시간
	AIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rev_ai_grpc_duration_seconds",
			Help:    "gRPC call latency to AI risk analysis server",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"status"}, // success / error / timeout
	)

	// ALLOW/BLOCK 판정 카운터
	RiskDecisionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rev_risk_decision_total",
			Help: "Total number of ALLOW/BLOCK decisions made",
		},
		[]string{"decision"}, // allow / block
	)

	// 현재 처리 중인 요청 수 (동시성 확인용)
	InFlightRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "rev_in_flight_requests",
			Help: "Number of requests currently being processed",
		},
	)
)
