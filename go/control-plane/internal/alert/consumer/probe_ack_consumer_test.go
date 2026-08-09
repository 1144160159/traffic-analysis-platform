package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
)

type probeAckApplyCall struct {
	tenantID, probeID, operationID, eventID string
	input                                   api.ProbeOperationAckInput
}

type fakeProbeAckApplier struct {
	calls []probeAckApplyCall
	err   error
}

func (applier *fakeProbeAckApplier) ApplyProbeOperationAck(
	_ context.Context,
	tenantID, probeID, operationID, eventID string,
	input api.ProbeOperationAckInput,
) error {
	applier.calls = append(applier.calls, probeAckApplyCall{
		tenantID: tenantID, probeID: probeID, operationID: operationID,
		eventID: eventID, input: input,
	})
	return applier.err
}

func probeAckKafkaMessage(t *testing.T) *commonkafka.ReceivedMessage {
	t.Helper()
	event := map[string]interface{}{
		"event_id":           "11111111-1111-4111-8111-111111111111",
		"event_type":         probeAgentAckEventType,
		"schema_version":     2,
		"tenant_id":          "tenant-a",
		"probe_id":           "probe-a",
		"operation_id":       "22222222-2222-4222-8222-222222222222",
		"command_revision":   3,
		"reported_version":   "cfg-3",
		"reported_hash":      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"agent_version":      "0.1.0",
		"applied":            true,
		"error":              "",
		"acknowledged_at_ms": time.Now().UnixMilli(),
		"detail":             map[string]interface{}{"applied": true},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	headers := []segmentkafka.Header{}
	for key, value := range map[string]string{
		"event_id": event["event_id"].(string), "event_type": event["event_type"].(string),
		"tenant_id": event["tenant_id"].(string), "probe_id": event["probe_id"].(string),
		"operation_id": event["operation_id"].(string),
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "probe.acks.v2", Partition: 1, Offset: 7, Value: payload, Headers: headers,
	}}
}

func TestProbeAckConsumerAppliesValidatedMessage(t *testing.T) {
	applier := &fakeProbeAckApplier{}
	consumer := &ProbeAckConsumer{applier: applier}

	if err := consumer.handle(context.Background(), probeAckKafkaMessage(t)); err != nil {
		t.Fatal(err)
	}
	if len(applier.calls) != 1 {
		t.Fatalf("apply calls=%d want 1", len(applier.calls))
	}
	call := applier.calls[0]
	if call.tenantID != "tenant-a" || call.probeID != "probe-a" ||
		call.input.CommandRevision != 3 || !call.input.Applied {
		t.Fatalf("unexpected apply call: %#v", call)
	}
}

func TestProbeAckConsumerRejectsHeaderBodyIdentityMismatch(t *testing.T) {
	applier := &fakeProbeAckApplier{}
	consumer := &ProbeAckConsumer{applier: applier}
	message := probeAckKafkaMessage(t)
	for index := range message.Headers {
		if message.Headers[index].Key == "probe_id" {
			message.Headers[index].Value = []byte("probe-b")
		}
	}

	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected identity mismatch")
	}
	if len(applier.calls) != 0 {
		t.Fatal("mismatched ACK reached transaction applier")
	}
}

func TestProbeAckConsumerPropagatesTransactionFailure(t *testing.T) {
	applier := &fakeProbeAckApplier{err: errors.New("database unavailable")}
	consumer := &ProbeAckConsumer{applier: applier}

	if err := consumer.handle(context.Background(), probeAckKafkaMessage(t)); err == nil {
		t.Fatal("expected transaction failure")
	}
}

func TestProbeAckConsumerRejectsTrailingJSONValue(t *testing.T) {
	applier := &fakeProbeAckApplier{}
	consumer := &ProbeAckConsumer{applier: applier}
	message := probeAckKafkaMessage(t)
	message.Value = append(message.Value, []byte(`{"unexpected":true}`)...)

	if err := consumer.handle(context.Background(), message); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
	if len(applier.calls) != 0 {
		t.Fatal("invalid ACK reached transaction applier")
	}
}
