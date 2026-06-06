package main

import (
	"context"
	"log"
	"net"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"google.golang.org/grpc"
)

type server struct {
	trafficv1.UnimplementedIngestServiceServer
}

func (s *server) RegisterProbe(ctx context.Context, req *trafficv1.RegisterProbeRequest) (*trafficv1.RegisterProbeResponse, error) {
	log.Printf("Mock RegisterProbe called: tenant=%s probe=%s", req.GetTenantId(), req.GetProbeId())
	return &trafficv1.RegisterProbeResponse{
		Success:       true,
		Message:       "mock-registered",
		InitialConfig: &trafficv1.ProbeConfig{ConfigVersion: "mock-v1"},
	}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	trafficv1.RegisterIngestServiceServer(s, &server{})
	log.Printf("mock ingest-gateway listening on :50051")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
