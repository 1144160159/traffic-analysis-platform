package consumer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/fusion"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
)

type fusionProjectorStub struct {
	called  int
	command fusion.SourceSyncCommand
	sha     string
}

func (stub *fusionProjectorStub) ApplySourceSync(
	_ context.Context, command fusion.SourceSyncCommand, sha string, _ fusion.KafkaPosition,
) (fusion.ProjectionReceipt, error) {
	stub.called++
	stub.command, stub.sha = command, sha
	return fusion.ProjectionReceipt{Disposition: "applied", QualityStatus: "partial"}, nil
}

func TestFusionProjectionConsumerValidatesEnvelopeBeforeProjecting(t *testing.T) {
	command := fusion.SourceSyncCommand{
		EventID: "00000000-0000-0000-0000-000000000101", EventType: fusion.SourceSyncEventType,
		SchemaVersion: 1, AggregateType: "source_sync_job", AggregateID: "00000000-0000-0000-0000-000000000102",
		AggregateVersion: 1, PartitionKey: "tenant-a:00000000-0000-0000-0000-000000000102",
		TenantID: "tenant-a", JobID: "00000000-0000-0000-0000-000000000102", SourceID: "traffic", SourceKind: "flow",
		WindowStart: time.Unix(100, 0).UTC(), WindowEnd: time.Unix(200, 0).UTC(), RequestedBy: "analyst-a",
		Reason: "manual source refresh", TraceID: "trace-a", OccurredAt: time.Unix(201, 0).UTC(),
	}
	payload, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	headers := []segmentkafka.Header{
		{Key: "event_id", Value: []byte(command.EventID)}, {Key: "event_type", Value: []byte(command.EventType)},
		{Key: "schema_version", Value: []byte("1")}, {Key: "aggregate_type", Value: []byte(command.AggregateType)},
		{Key: "aggregate_id", Value: []byte(command.AggregateID)}, {Key: "aggregate_version", Value: []byte("1")},
		{Key: "tenant_id", Value: []byte(command.TenantID)}, {Key: "job_id", Value: []byte(command.JobID)},
		{Key: "source_id", Value: []byte(command.SourceID)}, {Key: "trace_id", Value: []byte(command.TraceID)},
		{Key: "target_topic", Value: []byte(fusion.SourceSyncTopic)},
	}
	stub := &fusionProjectorStub{}
	consumer := &FusionProjectionConsumer{projector: stub}
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: fusion.SourceSyncTopic, Partition: 1, Offset: 2, Key: []byte(command.PartitionKey), Value: payload, Headers: headers,
	}}
	if err := consumer.Handle(context.Background(), message); err != nil {
		t.Fatalf("handle valid fusion command: %v", err)
	}
	if stub.called != 1 || len(stub.sha) != 64 || stub.command.JobID != command.JobID {
		t.Fatalf("projector did not receive canonical command: %#v", stub)
	}
}

func TestFusionProjectionConsumerRejectsHeaderMismatchAsPermanent(t *testing.T) {
	command := fusion.SourceSyncCommand{
		EventID: "00000000-0000-0000-0000-000000000101", EventType: fusion.SourceSyncEventType,
		SchemaVersion: 1, AggregateType: "source_sync_job", AggregateID: "00000000-0000-0000-0000-000000000102",
		AggregateVersion: 1, PartitionKey: "tenant-a:00000000-0000-0000-0000-000000000102",
		TenantID: "tenant-a", JobID: "00000000-0000-0000-0000-000000000102", SourceID: "traffic", SourceKind: "flow",
		WindowStart: time.Unix(100, 0).UTC(), WindowEnd: time.Unix(200, 0).UTC(), RequestedBy: "analyst-a",
		Reason: "manual source refresh", TraceID: "trace-a", OccurredAt: time.Unix(201, 0).UTC(),
	}
	payload, _ := json.Marshal(command)
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: fusion.SourceSyncTopic, Partition: 1, Offset: 2, Key: []byte(command.PartitionKey), Value: payload,
		Headers: []segmentkafka.Header{{Key: "event_id", Value: []byte("wrong")}},
	}}
	consumer := &FusionProjectionConsumer{projector: &fusionProjectorStub{}}
	err := consumer.Handle(context.Background(), message)
	if err == nil || !commonkafka.IsPermanent(err) || !strings.Contains(err.Error(), "header/body mismatch") {
		t.Fatalf("expected permanent header mismatch, got %v", err)
	}
}
