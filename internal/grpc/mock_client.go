package grpc

import (
	"context"
	"log"
)

type RiskResponse struct {
	RiskScore float32
	Block     bool
	Reason    string // 추가: FINGERPRINT_MISMATCH 등
}

// 인터페이스 정의 (나중에 실제 gRPC로 교체할 때 여기만 바꾸면 됨)
type AIClient interface {
	Analyze(ctx context.Context, req *AnalyzeRequest) (*RiskResponse, error)
}

// 요청 구조체 (proto의 AnalyzeRequest랑 맞춤)
type AnalyzeRequest struct {
	SessionToken    string
	UserID          string
	IP              string
	UserAgent       string
	FingerprintHash string
	Timestamp       int64
	Country         string
}

// Mock 구현체
type MockAIClient struct {
	BlockThreshold float64
}

func NewMockAIClient(threshold float64) *MockAIClient {
	return &MockAIClient{BlockThreshold: threshold}
}

func (m *MockAIClient) Analyze(ctx context.Context, req *AnalyzeRequest) (*RiskResponse, error) {
	var riskScore float32 = 0.1

	// 테스트용: 특정 IP면 위험하다고 판단
	if req.IP == "192.168.1.100" {
		riskScore = 0.95
	}

	block := float64(riskScore) >= m.BlockThreshold

	log.Printf("AI 판정: user=%s score=%.2f block=%v", req.UserID, riskScore, block)

	return &RiskResponse{
		RiskScore: riskScore,
		Block:     block,
		Reason:    "MOCK",
	}, nil
}
