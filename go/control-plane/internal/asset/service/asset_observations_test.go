package service

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
)

func TestRecordMacIpBindingRejectsMissingStableObservationIdentity(t *testing.T) {
	svc := New(nil, nil, zap.NewNop())
	accepted, rejected, err := svc.RecordMacIpBinding(context.Background(), []*config.MacIpBinding{{
		TenantID: "tenant-a", MACAddress: "00:11:22:33:44:55", IPAddress: "10.1.2.3", Source: "arp",
	}}, BindingProvenance{
		Channel: BindingChannelGRPC, Actor: "probe-a", TraceID: "trace-a",
	})
	if err != nil {
		t.Fatalf("semantic rejection must not become a transport error: %v", err)
	}
	if accepted != 0 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d", accepted, rejected)
	}
}

func TestRecordMacIpBindingRejectsImplicitDefaultTenant(t *testing.T) {
	svc := New(nil, nil, zap.NewNop())
	accepted, rejected, err := svc.RecordMacIpBinding(context.Background(), []*config.MacIpBinding{{
		MACAddress: "00:11:22:33:44:55", IPAddress: "10.1.2.3", ObservedAt: time.Now().UnixMilli(),
	}}, BindingProvenance{
		Channel: BindingChannelGRPC, Actor: "probe-a", TraceID: "trace-a",
	})
	if err != nil {
		t.Fatalf("semantic rejection must not become a transport error: %v", err)
	}
	if accepted != 0 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d", accepted, rejected)
	}
}

func TestStableAssetCommandKeyIsDeterministicAndOpaque(t *testing.T) {
	first := stableAssetCommandKey("asset-binding", "topic:3:42:0")
	second := stableAssetCommandKey("asset-binding", "topic:3:42:0")
	different := stableAssetCommandKey("asset-binding", "topic:3:43:0")
	if first != second || first == different {
		t.Fatalf("unstable command keys first=%q second=%q different=%q", first, second, different)
	}
	if len(first) < 16 || len(first) > 200 || first == "topic:3:42:0" {
		t.Fatalf("unexpected opaque command key %q", first)
	}
}

func TestUpsertAssetAtomicRejectsHumanCommandWithoutReason(t *testing.T) {
	svc := New(nil, nil, zap.NewNop())
	_, err := svc.UpsertAssetAtomic(context.Background(), &config.AssetRecord{
		TenantID: "tenant-a", MACAddress: "00:11:22:33:44:55", Source: "manual",
	}, config.AssetUpsertCommand{
		ActionID: config.AssetUpsertAction, ExpectedRevision: 0,
		IdempotencyKey: "asset-human-command-0001", Actor: "operator-a", TraceID: "trace-a",
	})
	if err == nil {
		t.Fatal("expected missing reason to fail before repository access")
	}
}
