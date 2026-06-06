// Mock Ingest — gRPC mock server + load generator.
//
//	server mode:    go run ./cmd/mock-ingest
//	generator mode: go run ./cmd/mock-ingest --mode=generator --target=localhost:50051 --rate=100
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

var (
	mode            = flag.String("mode", "server", "server | generator")
	target          = flag.String("target", "localhost:50051", "Ingest Gateway addr")
	rate            = flag.Int("rate", 100, "events/sec")
	count           = flag.Int("count", 0, "total events (0=infinite)")
	tenant          = flag.String("tenant", "test-tenant", "tenant ID")
	probe           = flag.String("probe", "mock-probe-001", "probe ID")
	listen          = flag.String("listen", ":50051", "gRPC listen addr")
	communityPrefix = flag.String("community-prefix", "1:mock", "Community ID prefix")
)

type server struct{ trafficv1.UnimplementedIngestServiceServer }

func (s *server) RegisterProbe(ctx context.Context, req *trafficv1.RegisterProbeRequest) (*trafficv1.RegisterProbeResponse, error) {
	log.Printf("Mock RegisterProbe: tenant=%s probe=%s", req.GetTenantId(), req.GetProbeId())
	return &trafficv1.RegisterProbeResponse{Success: true, Message: "mock-registered",
		InitialConfig: &trafficv1.ProbeConfig{ConfigVersion: "mock-v1"}}, nil
}

func (s *server) UploadFlows(ctx context.Context, req *trafficv1.UploadFlowsRequest) (*trafficv1.UploadFlowsResponse, error) {
	n := len(req.GetEvents())
	probeID := "unknown"
	if n > 0 && req.GetEvents()[0].GetHeader() != nil {
		probeID = req.GetEvents()[0].GetHeader().GetProbeId()
	}
	log.Printf("Mock UploadFlows: %d events from probe=%s", n, probeID)
	return &trafficv1.UploadFlowsResponse{Accepted: int32(n), Rejected: 0}, nil
}

func runServer() {
	lis, _ := net.Listen("tcp", *listen)
	s := grpc.NewServer()
	trafficv1.RegisterIngestServiceServer(s, &server{})
	log.Printf("Mock Ingest Gateway listening on %s", *listen)
	log.Fatal(s.Serve(lis))
}

var (
	srcIPs    = []string{"192.168.1.100", "192.168.1.101", "192.168.2.50", "10.0.0.10", "10.0.0.20"}
	dstIPs    = []string{"10.0.1.80", "10.0.1.443", "8.8.8.8", "1.1.1.1", "93.184.216.34"}
	protocols = []uint32{6, 6, 6, 6, 17}
	srcPorts  = []uint32{54321, 54322, 60000, 49152, 34567}
	dstPorts  = []uint32{80, 443, 53, 8080, 443}
)

func generateFlow() *trafficv1.FlowEvent {
	now := time.Now().UnixMilli()
	i := rand.Intn(5)
	return &trafficv1.FlowEvent{
		Header: &trafficv1.EventHeader{
			EventId: uuid.New().String(), TenantId: *tenant, RunId: "realtime",
			EventTs: now, IngestTs: now, ProbeId: *probe, FeatureSetId: "v1-default",
		},
		FlowId:      uuid.New().String(),
		CommunityId: fmt.Sprintf("%s%s", *communityPrefix, uuid.New().String()[:8]),
		Tuple:       &trafficv1.FiveTuple{SrcIp: srcIPs[i], DstIp: dstIPs[i], SrcPort: srcPorts[i], DstPort: dstPorts[i], Protocol: protocols[i]},
		TsStart:     now, TsEnd: now + int64(rand.Intn(5000)), DurationMs: uint32(rand.Intn(5000)),
		PacketsFwd: uint32(10 + rand.Intn(100)), PacketsBwd: uint32(5 + rand.Intn(50)),
		BytesFwd: uint64(1000 + rand.Intn(50000)), BytesBwd: uint64(500 + rand.Intn(20000)),
	}
}

func runGenerator() {
	conn, _ := grpc.NewClient(*target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer conn.Close()
	client := trafficv1.NewIngestServiceClient(conn)
	log.Printf("FlowEvent Generator: target=%s rate=%d/s count=%d", *target, *rate, *count)

	total := 0
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for *count == 0 || total < *count {
		<-ticker.C
		batch := make([]*trafficv1.FlowEvent, 0, *rate)
		for i := 0; i < *rate && (*count == 0 || total < *count); i++ {
			batch = append(batch, generateFlow())
			total++
		}
		if len(batch) > 0 {
			resp, err := client.UploadFlows(context.Background(), &trafficv1.UploadFlowsRequest{Events: batch})
			if err != nil {
				log.Printf("ERROR: %v", err)
			} else {
				log.Printf("Sent %d events (total=%d) accepted=%d", len(batch), total, resp.GetAccepted())
			}
		}
	}
	log.Printf("Generator finished: %d total events sent", total)
}

func main() {
	flag.Parse()
	if *mode == "generator" {
		runGenerator()
	} else {
		runServer()
	}
}
