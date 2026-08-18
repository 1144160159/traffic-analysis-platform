package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/baseline"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type BaselineActivationAckProjector interface {
	ApplyActivationAck(context.Context, baseline.ActivationAck) (baseline.ActivationReceipt, error)
}

type baselineActivationAckEvent struct {
	EventID         string    `json:"event_id"`
	EventType       string    `json:"event_type"`
	SchemaVersion   int64     `json:"schema_version"`
	PartitionKey    string    `json:"partition_key"`
	TenantID        string    `json:"tenant_id"`
	BaselineID      string    `json:"baseline_id"`
	BaselineVersion int64     `json:"baseline_version"`
	ConsumerID      string    `json:"consumer_id"`
	CandidateSHA256 string    `json:"candidate_sha256"`
	SnapshotSHA256  string    `json:"snapshot_sha256"`
	AckSHA256       string    `json:"ack_sha256"`
	AppliedAt       time.Time `json:"applied_at"`
	TraceID         string    `json:"trace_id"`
}

type BaselineActivationAckConsumer struct {
	projector       BaselineActivationAckProjector
	candidateSHA256 string
	logger          *zap.Logger
}

func NewBaselineActivationAckConsumer(
	projector BaselineActivationAckProjector,
	candidateSHA256 string,
	logger *zap.Logger,
) (*BaselineActivationAckConsumer, error) {
	if projector == nil || len(strings.TrimSpace(candidateSHA256)) != 64 {
		return nil, fmt.Errorf("baseline ACK projector and candidate SHA are required")
	}
	return &BaselineActivationAckConsumer{projector: projector,
		candidateSHA256: strings.TrimSpace(candidateSHA256), logger: logger}, nil
}

func (consumer *BaselineActivationAckConsumer) StartGeneration(
	ctx context.Context,
	runner *commonkafka.GenerationConsumer,
	processor *commonkafka.GenerationMessageProcessor,
) error {
	if consumer == nil || consumer.projector == nil || runner == nil || processor == nil {
		return fmt.Errorf("baseline activation ACK generation dependencies are required")
	}
	return runner.Run(ctx, func(generationContext context.Context, generation *segmentkafka.Generation,
		topic string, assignment segmentkafka.PartitionAssignment) error {
		return processor.ProcessPartition(generationContext, generation, topic, assignment, consumer.Handle)
	})
}

func (consumer *BaselineActivationAckConsumer) Handle(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	if message == nil {
		return commonkafka.Permanent(fmt.Errorf("behavior baseline activation ACK message is nil"))
	}
	if message.Topic != baseline.ActivationAckTopic || len(message.DuplicateHeaderNames()) > 0 {
		return commonkafka.Permanent(fmt.Errorf("behavior baseline ACK topic or duplicate headers are invalid"))
	}
	decoder := json.NewDecoder(bytes.NewReader(message.Value))
	decoder.DisallowUnknownFields()
	var event baselineActivationAckEvent
	if err := decoder.Decode(&event); err != nil {
		return commonkafka.Permanent(fmt.Errorf("decode behavior baseline activation ACK: %w", err))
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return commonkafka.Permanent(fmt.Errorf("behavior baseline activation ACK has trailing JSON"))
	}
	if event.EventType != baseline.ActivationAckEventType || event.SchemaVersion != 1 ||
		event.PartitionKey != event.TenantID+":"+event.BaselineID || event.CandidateSHA256 != consumer.candidateSHA256 {
		return commonkafka.Permanent(fmt.Errorf("behavior baseline activation ACK envelope or candidate is invalid"))
	}
	ack := baseline.ActivationAck{EventID: event.EventID, TenantID: event.TenantID, BaselineID: event.BaselineID,
		BaselineVersion: event.BaselineVersion, ConsumerID: event.ConsumerID,
		CandidateSHA256: event.CandidateSHA256, SnapshotSHA256: event.SnapshotSHA256,
		AckSHA256: event.AckSHA256, AppliedAt: event.AppliedAt, TraceID: event.TraceID}
	if err := ack.Validate(); err != nil {
		return commonkafka.Permanent(err)
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"schema_version": strconv.FormatInt(event.SchemaVersion, 10), "tenant_id": event.TenantID,
		"baseline_id": event.BaselineID, "baseline_version": strconv.FormatInt(event.BaselineVersion, 10),
		"consumer_id": event.ConsumerID, "candidate_sha256": event.CandidateSHA256,
		"snapshot_sha256": event.SnapshotSHA256, "trace_id": event.TraceID, "target_topic": baseline.ActivationAckTopic,
	}
	for name, expected := range expectedHeaders {
		if message.GetHeader(name) != expected {
			return commonkafka.Permanent(fmt.Errorf("behavior baseline activation ACK %s header/body mismatch", name))
		}
	}
	if string(message.Key) != event.PartitionKey {
		return commonkafka.Permanent(fmt.Errorf("behavior baseline activation ACK key/body mismatch"))
	}
	receipt, err := consumer.projector.ApplyActivationAck(ctx, ack)
	if err != nil {
		if errors.Is(err, baseline.ErrInvalidRequest) || errors.Is(err, baseline.ErrIdentityConflict) ||
			errors.Is(err, baseline.ErrRevisionConflict) || errors.Is(err, baseline.ErrStateConflict) {
			return commonkafka.Permanent(err)
		}
		return fmt.Errorf("apply behavior baseline activation ACK %s: %w", event.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info("Behavior baseline activation ACK committed",
			zap.String("event_id", event.EventID), zap.String("tenant_id", event.TenantID),
			zap.String("baseline_id", event.BaselineID), zap.Int64("baseline_version", event.BaselineVersion),
			zap.String("consumer_id", event.ConsumerID), zap.String("lifecycle_state", receipt.LifecycleState),
			zap.Int64("kafka_offset", message.Offset), zap.Bool("replayed", receipt.Replayed))
	}
	return nil
}
