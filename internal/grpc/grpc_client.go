package grpc

import (
	"context"
	"log"
	"time"

	rbapb "github.com/PickStranger/reverse-proxy-agent/pkg/gen/rba"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCAIClient는 실제 AI 이상탐지 gRPC 서버(rba.RBAService)와
// 통신하는 AIClient 구현체다. Mock 대신 이걸 붙이면 실제 gRPC로 판정한다.
type GRPCAIClient struct {
	conn    *grpc.ClientConn
	client  rbapb.RBAServiceClient
	timeout time.Duration
}

// NewGRPCAIClient는 addr(host:port)로 gRPC 클라이언트를 만든다.
func NewGRPCAIClient(addr string, useTLS bool, timeout time.Duration) (*GRPCAIClient, error) {
	var creds credentials.TransportCredentials
	if useTLS {
		creds = credentials.NewTLS(nil)
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
		client:  rbapb.NewRBAServiceClient(conn),
		timeout: timeout,
	}, nil
}

func (g *GRPCAIClient) Predict(ctx context.Context, req *PredictInput) (*PredictResult, error) {
	if g.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.timeout)
		defer cancel()
	}
	resp, err := g.client.Predict(ctx, &rbapb.PredictRequest{
		UserId:         req.UserID,
		Ip:             req.IP,
		Country:        req.Country,
		OsName:         req.OSName,
		OsVersion:      req.OSVersion,
		BrowserName:    req.BrowserName,
		BrowserVersion: req.BrowserVersion,
		DeviceType:     req.DeviceType,
		LoginTimestamp: req.LoginTimestamp,
		IsSuccess:      req.IsSuccess,
		StoreEvent:     req.StoreEvent,
	})
	if err != nil {
		return nil, err
	}
	return &PredictResult{
		IsAnomaly: resp.GetIsAnomaly(),
		RiskScore: resp.GetRiskScore(),
		EventID:   resp.GetEventId(),
	}, nil
}

// Close는 gRPC 연결을 닫는다. main에서 defer로 호출.
func (g *GRPCAIClient) Close() error {
	return g.conn.Close()
}
