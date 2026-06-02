package grpc

import (
	"context"
	"log"
)

// AI 모듈 응답 구조체 (pb.go 대신 임시로 정의)
type RiskResponse struct {
	RiskScore float32
	Block     bool
}

// Mock AI 클라이언트
type MockAIClient struct {
	BlockThreshold float64
}

func NewMockAIClient(threshold float64) *MockAIClient {
	return &MockAIClient{BlockThreshold: threshold}
}

func (m *MockAIClient) Analyze(ctx context.Context, hash, ip, userAgent string) (*RiskResponse, error) {
	// 테스트용: 특정 IP면 위험하다고 판단
	var riskScore float32 = 0.1

	if ip == "192.168.1.100" {
		riskScore = 0.95 // 위험!
	}

	block := float64(riskScore) >= m.BlockThreshold

	log.Printf("AI 판정: hash=%s score=%.2f block=%v", hash, riskScore, block)

	return &RiskResponse{
		RiskScore: riskScore,
		Block:     block,
	}, nil
}
