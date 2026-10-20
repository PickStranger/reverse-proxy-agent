package grpc

import (
	"context"
	"log"
)

// PredictResult는 AI 판정 결과를 담는다 (proto의 PredictResponse에 대응)
type PredictResult struct {
	IsAnomaly bool
	RiskScore float32
	EventID   int32
}

// 인터페이스 정의 (나중에 실제 gRPC로 교체할 때 여기만 바꾸면 됨)
type AIClient interface {
	Predict(ctx context.Context, req *PredictInput) (*PredictResult, error)
}

// PredictInput은 rev가 수집 가능한 데이터를 담는 요청 구조체
// (proto의 PredictRequest 중 서버 사이드에서 채울 수 있는 필드만 사용)
type PredictInput struct {
	UserID         string
	IP             string
	Country        string
	OSName         string
	OSVersion      string
	BrowserName    string
	BrowserVersion string
	DeviceType     string
	LoginTimestamp string
	IsSuccess      bool
	StoreEvent     bool
}

// Mock 구현체
type MockAIClient struct {
	BlockThreshold float64
}

func NewMockAIClient(threshold float64) *MockAIClient {
	return &MockAIClient{BlockThreshold: threshold}
}

func (m *MockAIClient) Predict(ctx context.Context, req *PredictInput) (*PredictResult, error) {
	var riskScore float32 = 0.1
	// 테스트용: 특정 IP면 위험하다고 판단
	if req.IP == "192.168.1.100" {
		riskScore = 0.95
	}
	isAnomaly := float64(riskScore) >= m.BlockThreshold
	log.Printf("AI 판정: user=%s score=%.2f anomaly=%v", req.UserID, riskScore, isAnomaly)
	return &PredictResult{
		IsAnomaly: isAnomaly,
		RiskScore: riskScore,
		EventID:   0,
	}, nil
}
