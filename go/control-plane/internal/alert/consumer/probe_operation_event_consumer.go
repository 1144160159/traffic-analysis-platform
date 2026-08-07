package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const probeOperationLifecycleEvent = "traffic.probe.v2.OperationAcknowledged"

type ProbeOperationProjectionApplier interface {
	ApplyProbeOperationProjection(context.Context, api.ProbeOperationProjectionInput) error
}

type ProbeOperationEventConsumer struct {
	consumer *commonkafka.Consumer
	applier  ProbeOperationProjectionApplier
	logger   *zap.Logger
}

func NewProbeOperationEventConsumer(
	consumer *commonkafka.Consumer,
	applier ProbeOperationProjectionApplier,
	logger *zap.Logger,
) (*ProbeOperationEventConsumer, error) {
	if consumer == nil || applier == nil {
		return nil, fmt.Errorf("probe operation event consumer and projection applier are required")
	}
	return &ProbeOperationEventConsumer{consumer: consumer, applier: applier, logger: logger}, nil
}

func (consumer *ProbeOperationEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *ProbeOperationEventConsumer) Close() error {
	return consumer.consumer.Close()
}

type probeOperationLifecycleEnvelope struct {
	EventID     string `json:"event_id"`
	EventType   string `json:"event_type"`
	TenantID    string `json:"tenant_id"`
	ProbeID     string `json:"probe_id"`
	OperationID string `json:"operation_id"`
	Revision    int64  `json:"revision"`
	Status      string `json:"status"`
	TraceID     string `json:"trace_id"`
}

func (consumer *ProbeOperationEventConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return fmt.Errorf("probe operation Kafka message is nil")
	}
	var payload map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("decode probe operation event: %w", err)
	}
	if err := rejectTrailingProbeOperationJSON(decoder); err != nil {
		return err
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("normalize probe operation event: %w", err)
	}
	var event probeOperationLifecycleEnvelope
	if err := json.Unmarshal(canonical, &event); err != nil {
		return fmt.Errorf("bind probe operation event: %w", err)
	}
	if event.EventType != probeOperationLifecycleEvent {
		return fmt.Errorf("unsupported probe operation event_type")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid probe operation event_id")
	}
	if _, err := uuid.Parse(event.OperationID); err != nil {
		return fmt.Errorf("invalid probe operation operation_id")
	}
	if strings.TrimSpace(event.TenantID) == "" || strings.TrimSpace(event.ProbeID) == "" ||
		event.Revision <= 0 || strings.TrimSpace(event.Status) == "" ||
		strings.TrimSpace(event.TraceID) == "" {
		return fmt.Errorf("incomplete probe operation event contract")
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"tenant_id": event.TenantID, "operation_id": event.OperationID,
		"aggregate_version": strconv.FormatInt(event.Revision, 10),
		"schema_version":    "2", "target_topic": "probe.events.v2",
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return fmt.Errorf("probe operation %s header/body mismatch", key)
		}
	}
	if actualKey := string(message.Key); actualKey != event.TenantID+":"+event.ProbeID {
		return fmt.Errorf("probe operation partition key/body mismatch")
	}
	input := api.ProbeOperationProjectionInput{
		EventID: event.EventID, EventType: event.EventType,
		TenantID: event.TenantID, ProbeID: event.ProbeID,
		OperationID: event.OperationID, Revision: event.Revision,
		Status: event.Status, TraceID: event.TraceID, Payload: payload,
		KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyProbeOperationProjection(ctx, input); err != nil {
		return fmt.Errorf("apply probe operation projection %s: %w", event.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info(
			"Probe operation lifecycle projection committed",
			zap.String("event_id", event.EventID),
			zap.String("operation_id", event.OperationID),
			zap.String("tenant_id", event.TenantID),
			zap.String("probe_id", event.ProbeID),
			zap.Int64("revision", event.Revision),
			zap.Int64("kafka_offset", message.Offset),
		)
	}
	return nil
}

func rejectTrailingProbeOperationJSON(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode probe operation event: multiple JSON values")
		}
		return fmt.Errorf("decode probe operation event trailing data: %w", err)
	}
	return nil
}
