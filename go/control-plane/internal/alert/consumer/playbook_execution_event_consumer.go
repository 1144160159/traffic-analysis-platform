package consumer

import (
	"bytes"
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

type PlaybookExecutionEventProjectionApplier interface {
	ApplyPlaybookExecutionEventProjection(context.Context, api.PlaybookExecutionEventProjectionInput) error
}

type PlaybookExecutionEventConsumer struct {
	consumer      *commonkafka.Consumer
	applier       PlaybookExecutionEventProjectionApplier
	expectedTopic string
	logger        *zap.Logger
}

func NewPlaybookExecutionEventConsumer(
	consumer *commonkafka.Consumer,
	applier PlaybookExecutionEventProjectionApplier,
	expectedTopic string,
	logger *zap.Logger,
) (*PlaybookExecutionEventConsumer, error) {
	if consumer == nil || applier == nil || strings.TrimSpace(expectedTopic) == "" {
		return nil, fmt.Errorf("playbook execution consumer, projection applier and topic are required")
	}
	return &PlaybookExecutionEventConsumer{
		consumer: consumer, applier: applier, expectedTopic: expectedTopic, logger: logger,
	}, nil
}

func (consumer *PlaybookExecutionEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *PlaybookExecutionEventConsumer) Close() error { return consumer.consumer.Close() }

func (consumer *PlaybookExecutionEventConsumer) handle(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	if message == nil {
		return fmt.Errorf("playbook execution Kafka message is nil")
	}
	if message.Topic != consumer.expectedTopic {
		return fmt.Errorf("playbook execution Kafka topic mismatch")
	}
	var payload map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(message.Value))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("decode playbook execution event: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode playbook execution event: multiple JSON values")
		}
		return fmt.Errorf("decode playbook execution event trailing data: %w", err)
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("normalize playbook execution event: %w", err)
	}
	var envelope struct {
		EventID          string `json:"event_id"`
		EventType        string `json:"event_type"`
		TenantID         string `json:"tenant_id"`
		AggregateType    string `json:"aggregate_type"`
		AggregateID      string `json:"aggregate_id"`
		AggregateVersion int64  `json:"aggregate_version"`
		PartitionKey     string `json:"partition_key"`
		SchemaVersion    int    `json:"schema_version"`
		ExecutionID      string `json:"execution_id"`
		PlaybookName     string `json:"playbook_name"`
		PlaybookVersion  int    `json:"playbook_version"`
		AlertID          string `json:"alert_id"`
		Status           string `json:"status"`
		ApprovalStatus   string `json:"approval_status"`
		ExecutorStatus   string `json:"executor_status"`
		TraceID          string `json:"trace_id"`
	}
	if err := json.Unmarshal(canonical, &envelope); err != nil {
		return fmt.Errorf("bind playbook execution event: %w", err)
	}
	if _, err := uuid.Parse(envelope.EventID); err != nil || envelope.SchemaVersion != 2 ||
		envelope.AggregateType != "playbook_execution" || envelope.AggregateID != envelope.ExecutionID ||
		envelope.AggregateVersion <= 0 || strings.TrimSpace(envelope.TenantID) == "" ||
		strings.TrimSpace(envelope.ExecutionID) == "" || strings.TrimSpace(envelope.PlaybookName) == "" ||
		strings.TrimSpace(envelope.PartitionKey) == "" || strings.TrimSpace(envelope.Status) == "" ||
		strings.TrimSpace(envelope.TraceID) == "" || !validPlaybookExecutionConsumerEvent(envelope.EventType) {
		return fmt.Errorf("incomplete playbook execution event contract")
	}
	expectedHeaders := map[string]string{
		"event_id": envelope.EventID, "event_type": envelope.EventType, "tenant_id": envelope.TenantID,
		"aggregate_type": "playbook_execution", "aggregate_id": envelope.ExecutionID,
		"aggregate_version": strconv.FormatInt(envelope.AggregateVersion, 10),
		"schema_version":    "2", "trace_id": envelope.TraceID, "target_topic": consumer.expectedTopic,
	}
	for key, expected := range expectedHeaders {
		if message.GetHeader(key) != expected {
			return fmt.Errorf("playbook execution %s header/body mismatch", key)
		}
	}
	if string(message.Key) != envelope.PartitionKey {
		return fmt.Errorf("playbook execution Kafka key/body mismatch")
	}
	input := api.PlaybookExecutionEventProjectionInput{
		EventID: envelope.EventID, TenantID: envelope.TenantID, ExecutionID: envelope.ExecutionID,
		PlaybookName: envelope.PlaybookName, PlaybookVersion: envelope.PlaybookVersion,
		AlertID: envelope.AlertID, EventType: envelope.EventType, Status: envelope.Status,
		ApprovalStatus: envelope.ApprovalStatus, ExecutorStatus: envelope.ExecutorStatus,
		SchemaVersion: 2, AggregateVersion: envelope.AggregateVersion, PartitionKey: envelope.PartitionKey,
		TraceID: envelope.TraceID, Payload: payload, KafkaTopic: message.Topic,
		KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyPlaybookExecutionEventProjection(ctx, input); err != nil {
		return fmt.Errorf("apply playbook execution event %s: %w", envelope.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info("Playbook execution event projection committed", zap.String("event_id", envelope.EventID), zap.Int64("kafka_offset", message.Offset))
	}
	return nil
}

func validPlaybookExecutionConsumerEvent(value string) bool {
	switch value {
	case "traffic.playbook.v2.ExecutionRequested", "traffic.playbook.v2.ExecutionApproved",
		"traffic.playbook.v2.ExecutionRejected", "traffic.playbook.v2.ExecutionCancelled",
		"traffic.playbook.v2.ExecutionCompleted", "traffic.playbook.v2.ExecutionPartial",
		"traffic.playbook.v2.ExecutionFailed", "traffic.playbook.v2.CompensationRequested",
		"traffic.playbook.v2.Compensated", "traffic.playbook.v2.CompensationFailed":
		return true
	default:
		return false
	}
}
