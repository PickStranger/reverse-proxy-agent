package grpc

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/PickStranger/reverse-proxy-agent/internal/metrics"
)

// CircuitBreaker는 AIClient를 감싸서, AI 서버 연속 장애 시
// 일정 시간 동안 AI 호출을 생략하고 기본값(ALLOW)으로 즉시 응답한다(Fail-Open).
// 목적: AI 분석 서버 장애가 리버스 프록시 및 원 서비스 전체 장애로 전파되는 것을 방지.
type CircuitBreaker struct {
	client    AIClient
	threshold int           // 연속 실패 임계값
	cooldown  time.Duration // open 상태 유지 시간

	mu                  sync.Mutex
	consecutiveFailures int
	openUntil           time.Time
}

func NewCircuitBreaker(client AIClient, threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		client:    client,
		threshold: threshold,
		cooldown:  cooldown,
	}
}

func (cb *CircuitBreaker) Predict(ctx context.Context, req *PredictInput) (*PredictResult, error) {
	cb.mu.Lock()
	open := time.Now().Before(cb.openUntil)
	cb.mu.Unlock()

	// ─── 회로가 열려있으면: AI 호출 생략, 즉시 기본값(ALLOW)으로 응답 ───
	if open {
		metrics.FailOpenTotal.Inc()
		log.Printf("회로차단기 OPEN: AI 호출 생략, 기본 ALLOW 처리 user=%s", req.UserID)
		return &PredictResult{
			IsAnomaly: false,
			RiskScore: 0,
			EventID:   0,
		}, nil
	}

	// ─── 정상 호출 시도 ───
	resp, err := cb.client.Predict(ctx, req)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.consecutiveFailures++
		if cb.consecutiveFailures >= cb.threshold {
			cb.openUntil = time.Now().Add(cb.cooldown)
			metrics.CircuitBreakerOpen.Set(1)
			log.Printf("회로차단기 OPEN 전환: 연속 실패 %d회 도달, %v 동안 AI 호출 생략", cb.consecutiveFailures, cb.cooldown)
		}
		return nil, err
	}

	// 성공 시 상태 초기화 (회로 닫힘)
	if cb.consecutiveFailures > 0 {
		log.Printf("회로차단기 CLOSED 복귀: AI 서버 정상 응답 확인")
	}
	cb.consecutiveFailures = 0
	metrics.CircuitBreakerOpen.Set(0)

	return resp, nil
}

// Close는 내부 클라이언트가 Closer를 구현하면 그대로 위임한다.
func (cb *CircuitBreaker) Close() error {
	if closer, ok := cb.client.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
