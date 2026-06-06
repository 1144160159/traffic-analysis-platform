package server

import (
	"context"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/auth"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/dedup"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/metrics"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var sharedHandler *IngestHandler

func getHandler(t *testing.T) *IngestHandler {
	t.Helper()
	if sharedHandler != nil { return sharedHandler }
	logger := zap.NewNop()
	cfg := HandlerConfig{MaxBatchSize: 1000, MaxEventSize: 65536, EnableDLQ: false, EnableDedup: true, ProbeStatusTimeout: config.DefaultProbeStatusTimeout}
	m := metrics.NewMetrics(logger)
	h := NewIngestHandlerWithConfig(logger, nil, nil, m, cfg)
	dedupCfg := dedup.DefaultDedupConfig()
	dedupCfg.RedisEnabled = false
	d, _ := dedup.NewDeduplicator(dedupCfg, nil, logger)
	h.SetDeduplicator(d)
	auditLogger, _ := audit.NewLogger(audit.Config{}, logger)
	h.SetAuditLogger(auditLogger)
	sharedHandler = h
	return h
}

func ctxWithTenant(tenantID string) context.Context {
	return auth.WithTestTenant(context.Background(), tenantID)
}

func TestHandlerEmptyRequest(t *testing.T) {
	h := getHandler(t)
	resp, err := h.UploadFlows(ctxWithTenant("t1"), &pb.UploadFlowsRequest{})
	if err != nil { t.Fatalf("error: %v", err) }
	if resp.Accepted != 0 { t.Errorf("accepted=%d want 0", resp.Accepted) }
}

func TestHandlerBatchTooLarge(t *testing.T) {
	h := getHandler(t)
	events := make([]*pb.FlowEvent, 2000)
	for i := range events { events[i] = &pb.FlowEvent{Header: &pb.EventHeader{EventId: "e"}} }
	_, err := h.UploadFlows(ctxWithTenant("t1"), &pb.UploadFlowsRequest{Events: events})
	if err == nil { t.Fatal("expected error") }
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument { t.Errorf("code=%v", st.Code()) }
}

func TestHandlerMissingTenant(t *testing.T) {
	h := getHandler(t)
	_, err := h.UploadFlows(context.Background(), &pb.UploadFlowsRequest{Events: []*pb.FlowEvent{{}}})
	if err == nil { t.Fatal("expected error") }
	st, _ := status.FromError(err)
	if st.Code() != codes.Unauthenticated { t.Errorf("code=%v", st.Code()) }
}
