package persistence

import (
	"testing"
	"time"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

func TestAlertTraceIDPropagatesFromDetectionHeaderToProtoProjection(t *testing.T) {
	const traceID = "0123456789abcdef0123456789abcdef"
	detection := &pb.DetectionBatch{
		TenantId: "tenant-a",
		BatchId:  "batch-a",
		Behaviors: []*pb.DetectionBehavior{{
			Header: &pb.EventHeader{
				TenantId:     "tenant-a",
				EventId:      "event-a",
				FeatureSetId: "features-v1",
				TraceId:      traceID,
			},
			CommunityId: "community-a",
		}},
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	alert := NewAlertFromProto(detection, "alert-a", "fingerprint-a", 1, now, now)
	if alert == nil {
		t.Fatal("alert must be constructed")
	}
	if alert.TraceID != traceID {
		t.Fatalf("trace ID was not inherited: got %q", alert.TraceID)
	}
	if got := alert.ToProto().GetTraceId(); got != traceID {
		t.Fatalf("trace ID was not projected to Proto: got %q", got)
	}
}
