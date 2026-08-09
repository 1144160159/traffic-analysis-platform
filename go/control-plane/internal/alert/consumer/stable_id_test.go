package consumer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/dedup"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func TestStableAlertIDIsReplayDeterministicAndTenantScoped(t *testing.T) {
	first := stableAlertID("tenant-a", "event-42", "fingerprint-a")
	replay := stableAlertID("tenant-a", "event-42", "fingerprint-changed")
	otherTenant := stableAlertID("tenant-b", "event-42", "fingerprint-a")
	if first != replay {
		t.Fatalf("replay changed alert identity: %s != %s", first, replay)
	}
	if first == otherTenant {
		t.Fatal("alert identity must be tenant scoped")
	}
}

func validDetectionBehavior() *pb.DetectionBehavior {
	return &pb.DetectionBehavior{
		Header: &pb.EventHeader{
			EventId: "event-42", TenantId: "tenant-a",
			EventType: "traffic.detection.behavior.v1", SchemaVersion: "1",
			AggregateType: "detection", AggregateId: "session-42", AggregateVersion: 1,
			OccurredAt: 1_700_000_000_000, ProducedAt: 1_700_000_000_100,
			TraceId: "trace-42", CausationId: "feature-42", CorrelationId: "community-42",
			IdempotencyKey: "event-42", Producer: "flink-behavior-job",
		},
		ModelVersion: "model-v1", CommunityId: "community-42", ObjectType: "session", ObjectId: "session-42",
		Ts: 1_700_000_000_000, Labels: []string{"scan"}, TopLabel: "scan", TopScore: 0.91,
		Tuple:       &pb.FiveTuple{SrcIp: "192.0.2.10", DstIp: "198.51.100.20", SrcPort: 34567, DstPort: 443, Protocol: 6},
		EvidenceIds: []string{"flow-1", "flow-2"},
	}
}

func TestDetectionEnvelopeRejectsMissingTupleAndIdentity(t *testing.T) {
	missingTuple := validDetectionBehavior()
	missingTuple.Tuple = nil
	if err := validateDetectionBehavior(missingTuple); err == nil || !strings.Contains(err.Error(), "tuple") {
		t.Fatalf("error=%v want tuple rejection", err)
	}

	missingIdentity := validDetectionBehavior()
	missingIdentity.Header.TraceId = ""
	if err := validateDetectionBehavior(missingIdentity); err == nil || !strings.Contains(err.Error(), "trace_id") {
		t.Fatalf("error=%v want trace_id rejection", err)
	}
}

func TestBuildAlertPreservesSourceTupleEvidenceAndReplayIdentity(t *testing.T) {
	behavior := validDetectionBehavior()
	if err := validateDetectionBehavior(behavior); err != nil {
		t.Fatal(err)
	}
	batch := &pb.DetectionBatch{TenantId: "tenant-a", Behaviors: []*pb.DetectionBehavior{behavior}}
	result := &dedup.DedupResult{IsNew: true, Count: 1, FirstSeen: behavior.Ts, LastSeen: behavior.Ts}
	consumer := &Consumer{logger: zap.NewNop()}

	first := consumer.buildAlert(batch, "fingerprint-a", result)
	replay := consumer.buildAlert(batch, "fingerprint-changed", result)

	if first.AlertID != replay.AlertID {
		t.Fatalf("replay changed alert ID: %s != %s", first.AlertID, replay.AlertID)
	}
	if first.SrcIP != "192.0.2.10" || first.DstIP != "198.51.100.20" || first.SrcPort != 34567 || first.DstPort != 443 || first.Protocol != 6 {
		t.Fatalf("source tuple lost: %+v", first)
	}
	if first.SessionID != "session-42" || strings.Join(first.EvidenceIDs, ",") != "flow-1,flow-2" || first.EventID != "event-42" {
		t.Fatalf("source identity/evidence lost: session=%s evidence=%v event=%s", first.SessionID, first.EvidenceIDs, first.EventID)
	}
	if first.UpdatedTs.Nanosecond()%int(time.Millisecond) != 0 {
		t.Fatalf("updated timestamp exceeds canonical millisecond precision: %s", first.UpdatedTs.Format(time.RFC3339Nano))
	}
	if first.UpdatedTs.UnixMilli() != behavior.Ts {
		t.Fatalf("legacy behavior.ts was not used as source version: got=%d want=%d", first.UpdatedTs.UnixMilli(), behavior.Ts)
	}
}

func TestCanonicalDetectionEventMillisPrefersHeaderThenLegacyBehaviorThenBatch(t *testing.T) {
	behavior := validDetectionBehavior()
	batch := &pb.DetectionBatch{CreatedAt: 1_600_000_000_000, Behaviors: []*pb.DetectionBehavior{behavior}}
	if got := canonicalDetectionEventMillis(batch); got != behavior.Ts {
		t.Fatalf("legacy behavior timestamp fallback=%d want=%d", got, behavior.Ts)
	}
	behavior.Header.EventTs = 1_800_000_000_000
	if got := canonicalDetectionEventMillis(batch); got != behavior.Header.EventTs {
		t.Fatalf("header timestamp=%d want=%d", got, behavior.Header.EventTs)
	}
	behavior.Header.EventTs = 0
	behavior.Ts = 0
	if got := canonicalDetectionEventMillis(batch); got != batch.CreatedAt {
		t.Fatalf("batch timestamp fallback=%d want=%d", got, batch.CreatedAt)
	}
}

func TestProcessMessageRejectsEmptyBehaviorsWithoutPanic(t *testing.T) {
	payload, err := proto.Marshal(&pb.DetectionBatch{TenantId: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{Value: payload}}
	consumer := &Consumer{logger: zap.NewNop()}

	_, _, err = consumer.processMessage(context.Background(), message)

	if err == nil || !strings.Contains(err.Error(), "behaviors") {
		t.Fatalf("error=%v want explicit behaviors validation", err)
	}
}
