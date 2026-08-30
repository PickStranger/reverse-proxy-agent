package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/PickStranger/reverse-proxy-agent/internal/metrics"
)

// statusRecorder: 응답 상태코드를 가로채기 위한 wrapper
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metrics.InFlightRequests.Inc()
		defer metrics.InFlightRequests.Dec()

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}

		next.ServeHTTP(rec, r)

		duration := time.Since(start).Seconds()
		metrics.HTTPRequestDuration.WithLabelValues(r.URL.Path, r.Method).Observe(duration)
		metrics.HTTPRequestsTotal.WithLabelValues(r.URL.Path, r.Method, strconv.Itoa(rec.status)).Inc()
	})
}
