package grpc

import (
	"context"
	"log"
	"time"

	anomalypb "github.com/PickStranger/reverse-proxy-agent/pkg/gen/anomaly"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCAIClient는 실제 AI 이상탐지 gRPC 서버(anomaly.v1.AnomalyAnalyzeService)와
// 통신하는 AIClient 구현체다. Mock 대신 이걸 붙이면 실제 gRPC로 판정한다.
type GRPCAIClient struct {
	conn    *grpc.ClientConn
	client  anomalypb.AnomalyAnalyzeServiceClient
	timeout time.Duration
}

// NewGRPCAIClient는 addr(host:port)로 gRPC 클라이언트를 만든다.
//   - useTLS=false: 평문 연결 (docker compose 내부망 기본값)
//   - useTLS=true : 시스템 루트 CA 기반 TLS 연결
//
// grpc.NewClient는 lazy 연결이라 실제 커넥션은 첫 RPC 때 맺어진다.
func NewGRPCAIClient(addr string, useTLS bool, timeout time.Duration) (*GRPCAIClient, error) {
	var creds credentials.TransportCredentials
	if useTLS {
		creds = credentials.NewTLS(nil) // nil → 시스템 루트 CA 사용
	} else {
		creds = insecure.NewCredentials()
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}

	log.Printf("AI gRPC 클라이언트 준비: addr=%s tls=%v timeout=%v", addr, useTLS, timeout)
	return &GRPCAIClient{
		conn:    conn,
		client:  anomalypb.NewAnomalyAnalyzeServiceClient(conn),
		timeout: timeout,
	}, nil
}

func (g *GRPCAIClient) Analyze(ctx context.Context, req *AnalyzeRequest) (*RiskResponse, error) {
	if g.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.timeout)
		defer cancel()
	}

	resp, err := g.client.Analyze(ctx, &anomalypb.AnalyzeRequest{
		SessionToken:    req.SessionToken,
		UserId:          req.UserID,
		Ip:              req.IP,
		UserAgent:       req.UserAgent,
		FingerprintHash: req.FingerprintHash,
		Timestamp:       req.Timestamp,
		Country:         req.Country,
	})
	if err != nil {
		return nil, err
	}

	return &RiskResponse{
		RiskScore: resp.GetRiskScore(),
		Block:     resp.GetBlock(),
		Reason:    resp.GetReason(),
	}, nil
}

// Close는 gRPC 연결을 닫는다. main에서 defer로 호출.
func (g *GRPCAIClient) Close() error {
	return g.conn.Close()
}
