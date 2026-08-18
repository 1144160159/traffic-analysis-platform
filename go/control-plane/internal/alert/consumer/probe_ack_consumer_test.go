package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		"operation_id":     event["operation_id"].(string),
		"command_revision": "3", "schema_version": "2", "target_topic": "probe.acks.v2",
	} {
		headers = append(headers, segmentkafka.Header{Key: key, Value: []byte(value)})
	}
	return &commonkafka.ReceivedMessage{Message: segmentkafka.Message{
		Topic: "probe.acks.v2", Partition: 1, Offset: 7, Key: []byte("tenant-a:probe-a"),
		Value: payload, Headers: headers,
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

	if err := consumer.handle(context.Background(), message); !commonkafka.IsPermanent(err) {
		t.Fatalf("error = %v, want permanent identity mismatch", err)
	}
	if len(applier.calls) != 0 {
		t.Fatal("mismatched ACK reached transaction applier")
	}
}

func TestProbeAckConsumerPropagatesTransactionFailure(t *testing.T) {
	applier := &fakeProbeAckApplier{err: errors.New("database unavailable")}
	consumer := &ProbeAckConsumer{applier: applier}

	if err := consumer.handle(context.Background(), probeAckKafkaMessage(t)); err == nil || commonkafka.IsPermanent(err) {
		t.Fatalf("error = %v, want retryable transaction failure", err)
	}
}

func TestProbeAckConsumerRejectsTrailingJSONValue(t *testing.T) {
	applier := &fakeProbeAckApplier{}
	consumer := &ProbeAckConsumer{applier: applier}
	message := probeAckKafkaMessage(t)
	message.Value = append(message.Value, []byte(`{"unexpected":true}`)...)

	if err := consumer.handle(context.Background(), message); !commonkafka.IsPermanent(err) {
		t.Fatalf("error = %v, want permanent trailing JSON rejection", err)
	}
	if len(applier.calls) != 0 {
		t.Fatal("invalid ACK reached transaction applier")
	}
}

func TestProbeAckErrorClassificationMatrix(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		permanent bool
	}{
		{name: "operation missing", err: api.ErrProbeOperationNotFound, permanent: true},
		{name: "revision mismatch", err: api.ErrProbeAckRevisionMismatch, permanent: true},
		{name: "persistence unavailable", err: api.ErrProbeAckPersistenceUnavailable, permanent: false},
		{name: "deadline", err: context.DeadlineExceeded, permanent: false},
		{name: "canceled", err: context.Canceled, permanent: false},
		{name: "unknown database", err: errors.New("serialization failure"), permanent: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classified := classifyProbeAckError(fmt.Errorf("wrapped: %w", test.err))
			if commonkafka.IsPermanent(classified) != test.permanent {
				t.Fatalf("classifyProbeAckError(%v) permanent=%v, want %v", test.err, commonkafka.IsPermanent(classified), test.permanent)
			}
			if !errors.Is(classified, test.err) {
				t.Fatalf("classified error lost cause: %v", classified)
			}
		})
	}
}
