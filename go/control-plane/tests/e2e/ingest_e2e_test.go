//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

const (
	serverAddr = "localhost:9090"
	certDir    = "../../certs"
)

func loadClientTLSConfig(t *testing.T) *tls.Config {
	// 加载客户端证书
	cert, err := tls.LoadX509KeyPair(
		certDir+"/client-probe-01.crt",
		certDir+"/client-probe-01.key",
	)
	if err != nil {
		t.Fatalf("Failed to load client cert: %v", err)
	}

	// 加载 CA
	caCert, err := os.ReadFile(certDir + "/ca.crt")
	if err != nil {
		t.Fatalf("Failed to read CA: %v", err)
	}

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caCert)

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
		ServerName:   "ingest-gateway",
	}
}

func TestE2E_UploadFlows(t *testing.T) {
	// 创建 gRPC 连接
	tlsConfig := loadClientTLSConfig(t)
	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewIngestServiceClient(conn)

	// 添加认证 metadata
	md := metadata.New(map[string]string{
		"x-tenant-token": "test-token-12345",
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 创建测试请求
	req := &pb.BatchUploadRequest{
		Events: []*pb.FlowEvent{
			{
				Header: &pb.EventHeader{
					EventId:  "e2e-test-001",
					TenantId: "tenant-e2e",
					ProbeId:  "probe-01",
					EventTs:  time.Now().UnixMilli(),
				},
				FlowId:      "flow-e2e-001",
				CommunityId: "1:e2e:test",
				Tuple: &pb.FiveTuple{
					SrcIp:    "192.168.1.100",
					DstIp:    "10.0.0.1",
					SrcPort:  54321,
					DstPort:  443,
					Protocol: 6,
				},
				Direction:  "c2s",
				TsStart:    time.Now().Add(-time.Minute).UnixMilli(),
				TsEnd:      time.Now().UnixMilli(),
				DurationMs: 60000,
				PacketsFwd: 150,
				PacketsBwd: 120,
				BytesFwd:   75000,
				BytesBwd:   60000,
			},
		},
		Compression: "none",
	}

	// 发送请求
	resp, err := client.UploadFlows(ctx, req)
	if err != nil {
		t.Fatalf("UploadFlows failed: %v", err)
	}

	// 验证响应
	if resp.Accepted != 1 {
		t.Errorf("Expected 1 accepted, got %d", resp.Accepted)
	}
	if resp.Rejected != 0 {
		t.Errorf("Expected 0 rejected, got %d", resp.Rejected)
	}

	t.Logf("E2E test passed: %s", resp.Message)
}

func TestE2E_Heartbeat(t *testing.T) {
	tlsConfig := loadClientTLSConfig(t)
	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewIngestServiceClient(conn)

	md := metadata.New(map[string]string{
		"x-tenant-token": "test-token-12345",
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req := &pb.HeartbeatRequest{
		ProbeId:   "probe-01",
		TenantId:  "tenant-e2e",
		Timestamp: time.Now().UnixMilli(),
		Status: &pb.ProbeStatus{
			CpuUsage:        45.5,
			MemoryUsage:     62.3,
			PacketsCaptured: 1000000,
			PacketsDropped:  100,
			CapturePps:      50000,
			UploadBps:       10000000,
		},
	}

	resp, err := client.Heartbeat(ctx, req)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if !resp.Ok {
		t.Error("Expected heartbeat OK")
	}
}

func TestE2E_StreamFlows(t *testing.T) {
	tlsConfig := loadClientTLSConfig(t)
	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewIngestServiceClient(conn)

	md := metadata.New(map[string]string{
		"x-tenant-token": "test-token-12345",
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	stream, err := client.StreamFlows(ctx)
	if err != nil {
		t.Fatalf("StreamFlows failed: %v", err)
	}

	// 发送多个事件
	for i := 0; i < 10; i++ {
		event := &pb.FlowEvent{
			Header: &pb.EventHeader{
				EventId:  "stream-" + string(rune('0'+i)),
				TenantId: "tenant-e2e",
				ProbeId:  "probe-01",
				EventTs:  time.Now().UnixMilli(),
			},
			FlowId:      "flow-stream-" + string(rune('0'+i)),
			CommunityId: "1:stream:test",
			TsStart:     time.Now().UnixMilli(),
			TsEnd:       time.Now().UnixMilli(),
		}

		if err := stream.Send(event); err != nil {
			t.Fatalf("Stream send failed: %v", err)
		}

		// 接收确认
		ack, err := stream.Recv()
		if err != nil {
			t.Fatalf("Stream recv failed: %v", err)
		}

		if !ack.Accepted {
			t.Errorf("Event %s not accepted: %s", ack.EventId, ack.Error)
		}
	}

	// 关闭发送端
	stream.CloseSend()

	t.Log("Stream test passed")
}

func TestE2E_HighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high throughput test in short mode")
	}

	tlsConfig := loadClientTLSConfig(t)
	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.Dial(serverAddr, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := pb.NewIngestServiceClient(conn)

	md := metadata.New(map[string]string{
		"x-tenant-token": "test-token-12345",
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	// 发送 10000 个事件，分 10 批
	totalEvents := 10000
	batchSize := 1000
	batches := totalEvents / batchSize

	start := time.Now()

	for b := 0; b < batches; b++ {
		events := make([]*pb.FlowEvent, batchSize)
		for i := 0; i < batchSize; i++ {
			events[i] = &pb.FlowEvent{
				Header: &pb.EventHeader{
					EventId:  "perf-" + string(rune(b*batchSize+i)),
					TenantId: "tenant-perf",
					ProbeId:  "probe-01",
					EventTs:  time.Now().UnixMilli(),
				},
				FlowId:      "flow-perf",
				CommunityId: "1:perf:test",
				TsStart:     time.Now().UnixMilli(),
				TsEnd:       time.Now().UnixMilli(),
			}
		}

		req := &pb.BatchUploadRequest{Events: events}

		batchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err := client.UploadFlows(batchCtx, req)
		cancel()

		if err != nil {
			t.Fatalf("Batch %d failed: %v", b, err)
		}
	}

	elapsed := time.Since(start)
	eps := float64(totalEvents) / elapsed.Seconds()

	t.Logf("Throughput: %.2f events/sec (%d events in %v)", eps, totalEvents, elapsed)

	// 验证至少能达到 1000 EPS（保守估计，网络往返）
	if eps < 1000 {
		t.Errorf("Throughput too low: %.2f EPS", eps)
	}
}
