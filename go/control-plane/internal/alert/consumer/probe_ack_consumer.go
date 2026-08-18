package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

const probeAgentAckEventType = "traffic.probe.v2.OperationAgentAcknowledged"

type ProbeAckApplier interface {
	ApplyProbeOperationAck(
		context.Context,
		string,
		string,
		string,
		string,
		api.ProbeOperationAckInput,
	) error
}

type ProbeAckConsumer struct {
	consumer *commonkafka.Consumer
	applier  ProbeAckApplier
	logger   *zap.Logger
}

func NewProbeAckConsumer(
	consumer *commonkafka.Consumer,
	applier ProbeAckApplier,
	logger *zap.Logger,
) (*ProbeAckConsumer, error) {
	if consumer == nil || applier == nil {
		return nil, fmt.Errorf("probe ACK Kafka consumer and applier are required")
	}
	return &ProbeAckConsumer{consumer: consumer, applier: applier, logger: logger}, nil
}

func NewProbeAckGenerationAdapter(
	applier ProbeAckApplier,
	logger *zap.Logger,
) (*ProbeAckConsumer, error) {
	if applier == nil {
		return nil, fmt.Errorf("probe ACK generation applier is required")
	}
	return &ProbeAckConsumer{applier: applier, logger: logger}, nil
}

func (consumer *ProbeAckConsumer) Start(ctx context.Context) error {
	if consumer == nil || consumer.consumer == nil {
		return fmt.Errorf("probe ACK legacy consumer is unavailable")
	}
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *ProbeAckConsumer) StartGeneration(
	ctx context.Context,
	runner *commonkafka.GenerationConsumer,
	processor *commonkafka.GenerationMessageProcessor,
) error {
	if consumer == nil || consumer.applier == nil || runner == nil || processor == nil {
		return fmt.Errorf("probe ACK generation runner processor and applier are required")
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

func (consumer *ProbeAckConsumer) Close() error {
	if consumer == nil || consumer.consumer == nil {
		return nil
	}
	return consumer.consumer.Close()
}

type probeAgentAckEvent struct {
	EventID          string                 `json:"event_id"`
	EventType        string                 `json:"event_type"`
	SchemaVersion    int                    `json:"schema_version"`
	TenantID         string                 `json:"tenant_id"`
	ProbeID          string                 `json:"probe_id"`
	OperationID      string                 `json:"operation_id"`
	CommandRevision  int64                  `json:"command_revision"`
	ReportedVersion  string                 `json:"reported_version"`
	ReportedHash     string                 `json:"reported_hash"`
	AgentVersion     string                 `json:"agent_version"`
	Applied          bool                   `json:"applied"`
	Error            string                 `json:"error"`
	AcknowledgedAtMS int64                  `json:"acknowledged_at_ms"`
	Detail           map[string]interface{} `json:"detail"`
}

func (consumer *ProbeAckConsumer) handle(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	if message == nil {
		return commonkafka.Permanent(fmt.Errorf("probe ACK Kafka message is nil"))
	}
	var event probeAgentAckEvent
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return commonkafka.Permanent(fmt.Errorf("decode probe Agent ACK: %w", err))
	}
	if err := rejectTrailingProbeAckJSON(decoder); err != nil {
		return commonkafka.Permanent(err)
	}
	if event.EventType != probeAgentAckEventType || event.SchemaVersion != 2 {
		return commonkafka.Permanent(fmt.Errorf("unsupported probe Agent ACK contract"))
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return commonkafka.Permanent(fmt.Errorf("invalid probe Agent ACK event_id"))
	}
	if _, err := uuid.Parse(event.OperationID); err != nil {
		return commonkafka.Permanent(fmt.Errorf("invalid probe Agent ACK operation_id"))
	}
	if event.TenantID == "" || event.ProbeID == "" || event.CommandRevision <= 0 ||
		len(event.ReportedHash) != 64 || event.AgentVersion == "" || event.AcknowledgedAtMS <= 0 {
		return commonkafka.Permanent(fmt.Errorf("incomplete probe Agent ACK contract"))
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"tenant_id": event.TenantID, "probe_id": event.ProbeID,
		"operation_id":     event.OperationID,
		"command_revision": strconv.FormatInt(event.CommandRevision, 10),
		"schema_version":   strconv.Itoa(event.SchemaVersion),
		"target_topic":     "probe.acks.v2",
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return commonkafka.Permanent(fmt.Errorf("probe Agent ACK %s header/body mismatch", key))
		}
	}
	if message.Topic != "probe.acks.v2" || string(message.Key) != event.TenantID+":"+event.ProbeID ||
		message.Partition < 0 || message.Offset < 0 {
		return commonkafka.Permanent(fmt.Errorf("probe Agent ACK Kafka source or key mismatch"))
	}
	input := api.ProbeOperationAckInput{
		CommandRevision: event.CommandRevision,
		ReportedVersion: event.ReportedVersion,
		ReportedHash:    event.ReportedHash,
		AgentVersion:    event.AgentVersion,
		Applied:         event.Applied,
		Error:           event.Error,
		AcknowledgedAt:  time.UnixMilli(event.AcknowledgedAtMS).UTC(),
		Detail:          event.Detail,
	}
	if err := consumer.applier.ApplyProbeOperationAck(
		ctx, event.TenantID, event.ProbeID, event.OperationID, event.EventID, input,
	); err != nil {
		return classifyProbeAckError(fmt.Errorf("apply probe Agent ACK %s: %w", event.EventID, err))
	}
	if consumer.logger != nil {
		consumer.logger.Info(
			"Probe Agent ACK transaction committed",
			zap.String("event_id", event.EventID),
			zap.String("operation_id", event.OperationID),
			zap.String("tenant_id", event.TenantID),
			zap.String("probe_id", event.ProbeID),
		)
	}
	return nil
}

func classifyProbeAckError(err error) error {
	if err == nil || commonkafka.IsPermanent(err) {
		return err
	}
	if errors.Is(err, api.ErrProbeOperationNotFound) ||
		errors.Is(err, api.ErrProbeAckRevisionMismatch) {
		return commonkafka.Permanent(err)
	}
	// Persistence unavailability, context cancellation/deadline, SQL/network
	// failures and all unknown errors remain retryable by default.
	return err
}

func rejectTrailingProbeAckJSON(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode probe Agent ACK: multiple JSON values")
		}
		return fmt.Errorf("decode probe Agent ACK trailing data: %w", err)
	}
	return nil
}
