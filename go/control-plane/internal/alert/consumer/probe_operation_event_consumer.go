package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const (
	probeOperationAcknowledgedLifecycleEvent = "traffic.probe.v2.OperationAcknowledged"
	probeOperationExpiredLifecycleEvent      = "traffic.probe.v2.OperationExpired"
)

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

func NewProbeOperationEventGenerationAdapter(
	applier ProbeOperationProjectionApplier,
	logger *zap.Logger,
) (*ProbeOperationEventConsumer, error) {
	if applier == nil {
		return nil, fmt.Errorf("probe lifecycle generation projection applier is required")
	}
	return &ProbeOperationEventConsumer{applier: applier, logger: logger}, nil
}

func (consumer *ProbeOperationEventConsumer) Start(ctx context.Context) error {
	if consumer == nil || consumer.consumer == nil {
		return fmt.Errorf("probe operation event legacy consumer is unavailable")
	}
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *ProbeOperationEventConsumer) StartGeneration(
	ctx context.Context,
	runner *commonkafka.GenerationConsumer,
	processor *commonkafka.GenerationMessageProcessor,
) error {
	if consumer == nil || consumer.applier == nil || runner == nil || processor == nil {
		return fmt.Errorf("probe lifecycle generation runner processor and applier are required")
	}
	return runner.Run(ctx, func(
		generationContext context.Context,
		generation *kafka.Generation,
		topic string,
		assignment kafka.PartitionAssignment,
	) error {
		return processor.ProcessPartition(
			generationContext, generation, topic, assignment, consumer.handle,
		)
	})
}

func (consumer *ProbeOperationEventConsumer) Close() error {
	if consumer == nil || consumer.consumer == nil {
		return nil
	}
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
		return commonkafka.Permanent(fmt.Errorf("probe operation Kafka message is nil"))
	}
	var payload map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return commonkafka.Permanent(fmt.Errorf("decode probe operation event: %w", err))
	}
	if err := rejectTrailingProbeOperationJSON(decoder); err != nil {
		return commonkafka.Permanent(err)
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("normalize probe operation event: %w", err)
	}
	var event probeOperationLifecycleEnvelope
	if err := json.Unmarshal(canonical, &event); err != nil {
		return commonkafka.Permanent(fmt.Errorf("bind probe operation event: %w", err))
	}
	switch event.EventType {
	case probeOperationAcknowledgedLifecycleEvent, probeOperationExpiredLifecycleEvent:
	default:
		return commonkafka.Permanent(fmt.Errorf("unsupported probe operation event_type"))
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return commonkafka.Permanent(fmt.Errorf("invalid probe operation event_id"))
	}
	if _, err := uuid.Parse(event.OperationID); err != nil {
		return commonkafka.Permanent(fmt.Errorf("invalid probe operation operation_id"))
	}
	if strings.TrimSpace(event.TenantID) == "" || strings.TrimSpace(event.ProbeID) == "" ||
		event.Revision <= 0 || strings.TrimSpace(event.Status) == "" ||
		strings.TrimSpace(event.TraceID) == "" {
		return commonkafka.Permanent(fmt.Errorf("incomplete probe operation event contract"))
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"tenant_id": event.TenantID, "probe_id": event.ProbeID,
		"operation_id":      event.OperationID,
		"aggregate_version": strconv.FormatInt(event.Revision, 10),
		"schema_version":    "2", "target_topic": "probe.events.v2",
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return commonkafka.Permanent(fmt.Errorf("probe operation %s header/body mismatch", key))
		}
	}
	if actualKey := string(message.Key); actualKey != event.TenantID+":"+event.ProbeID {
		return commonkafka.Permanent(fmt.Errorf("probe operation partition key/body mismatch"))
	}
	if message.Topic != "probe.events.v2" || message.Partition < 0 || message.Offset < 0 {
		return commonkafka.Permanent(fmt.Errorf("probe operation Kafka source mismatch"))
	}
	input := api.ProbeOperationProjectionInput{
		EventID: event.EventID, EventType: event.EventType,
		TenantID: event.TenantID, ProbeID: event.ProbeID,
		OperationID: event.OperationID, Revision: event.Revision,
		Status: event.Status, TraceID: event.TraceID, Payload: payload,
		KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyProbeOperationProjection(ctx, input); err != nil {
		projectionErr := fmt.Errorf("apply probe operation projection %s: %w", event.EventID, err)
		if errors.Is(err, api.ErrProbeOperationProjectionConflict) {
			return commonkafka.Permanent(projectionErr)
		}
		return projectionErr
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
