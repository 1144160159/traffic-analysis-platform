package consumer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/baseline"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
)

type baselineAckProjectorStub struct {
	called int
	ack    baseline.ActivationAck
	err    error
}

func (stub *baselineAckProjectorStub) ApplyActivationAck(_ context.Context, ack baseline.ActivationAck) (baseline.ActivationReceipt, error) {
	stub.called++
	stub.ack = ack
	return baseline.ActivationReceipt{LifecycleState: "frozen"}, stub.err
}

func TestBaselineActivationAckConsumerValidatesExactEnvelope(t *testing.T) {
	event := testBaselineAckEvent()
	payload, _ := json.Marshal(event)
	headers := []segmentkafka.Header{
		{Key: "event_id", Value: []byte(event.EventID)}, {Key: "event_type", Value: []byte(event.EventType)},
		{Key: "schema_version", Value: []byte("1")}, {Key: "tenant_id", Value: []byte(event.TenantID)},
		{Key: "baseline_id", Value: []byte(event.BaselineID)}, {Key: "baseline_version", Value: []byte("2")},
		{Key: "consumer_id", Value: []byte(event.ConsumerID)}, {Key: "candidate_sha256", Value: []byte(event.CandidateSHA256)},
		{Key: "snapshot_sha256", Value: []byte(event.SnapshotSHA256)}, {Key: "trace_id", Value: []byte(event.TraceID)},
		{Key: "target_topic", Value: []byte(baseline.ActivationAckTopic)},
	}
	stub := &baselineAckProjectorStub{}
	consumer, err := NewBaselineActivationAckConsumer(stub, event.CandidateSHA256, nil)
	if err != nil {
		t.Fatal(err)
	}
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{Topic: baseline.ActivationAckTopic,
		Partition: 1, Offset: 7, Key: []byte(event.PartitionKey), Value: payload, Headers: headers}}
	if err := consumer.Handle(context.Background(), message); err != nil {
		t.Fatalf("valid baseline ACK rejected: %v", err)
	}
	if stub.called != 1 || stub.ack.EventID != event.EventID || stub.ack.ConsumerID != event.ConsumerID {
		t.Fatalf("ACK projector did not receive exact event: %#v", stub)
	}
}

func TestBaselineActivationAckConsumerRejectsCandidateOrHeaderMismatchPermanently(t *testing.T) {
	event := testBaselineAckEvent()
	payload, _ := json.Marshal(event)
	stub := &baselineAckProjectorStub{}
	consumer, _ := NewBaselineActivationAckConsumer(stub, strings.Repeat("f", 64), nil)
	message := &commonkafka.ReceivedMessage{Message: segmentkafka.Message{Topic: baseline.ActivationAckTopic,
		Key: []byte(event.PartitionKey), Value: payload}}
	err := consumer.Handle(context.Background(), message)
	if err == nil || !commonkafka.IsPermanent(err) || stub.called != 0 {
		t.Fatalf("candidate mismatch was not poison: %v calls=%d", err, stub.called)
	}
}

func testBaselineAckEvent() baselineActivationAckEvent {
	return baselineActivationAckEvent{
		EventID: "00000000-0000-0000-0000-000000000601", EventType: baseline.ActivationAckEventType,
		SchemaVersion: 1, PartitionKey: "tenant-a:asset:asset-a", TenantID: "tenant-a",
		BaselineID: "asset:asset-a", BaselineVersion: 2, ConsumerID: "flink-user-behavior-job",
		CandidateSHA256: strings.Repeat("a", 64), SnapshotSHA256: strings.Repeat("b", 64),
		AckSHA256: strings.Repeat("c", 64), AppliedAt: time.Unix(200, 0).UTC(), TraceID: "trace-a",
	}
}
