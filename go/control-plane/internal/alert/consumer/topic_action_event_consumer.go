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

const (
	topicActionRequestedEvent = "traffic.topic.v2.ActionRequested"
	topicActionResultEvent    = "traffic.topic.v2.ActionResult"
)

type TopicActionProjectionApplier interface {
	ApplyTopicActionProjection(context.Context, api.TopicActionProjectionInput) error
}

type TopicActionEventConsumer struct {
	consumer *commonkafka.Consumer
	applier  TopicActionProjectionApplier
	logger   *zap.Logger
}

func NewTopicActionEventConsumer(
	consumer *commonkafka.Consumer,
	applier TopicActionProjectionApplier,
	logger *zap.Logger,
) (*TopicActionEventConsumer, error) {
	if consumer == nil || applier == nil {
		return nil, fmt.Errorf("topic action Kafka consumer and projection applier are required")
	}
	return &TopicActionEventConsumer{consumer: consumer, applier: applier, logger: logger}, nil
}

func (consumer *TopicActionEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *TopicActionEventConsumer) Close() error {
	return consumer.consumer.Close()
}

type topicActionLifecycleEvent struct {
	EventID   string                 `json:"event_id"`
	EventType string                 `json:"event_type"`
	TenantID  string                 `json:"tenant_id"`
	Topic     string                 `json:"topic"`
	JobID     string                 `json:"job_id"`
	ActionID  string                 `json:"action_id"`
	Revision  int64                  `json:"revision"`
	TraceID   string                 `json:"trace_id"`
	Status    string                 `json:"status"`
	Extra     map[string]interface{} `json:"-"`
}

func (consumer *TopicActionEventConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return fmt.Errorf("topic action Kafka message is nil")
	}
	var payload map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("decode topic action event: %w", err)
	}
	if err := rejectTrailingTopicActionJSON(decoder); err != nil {
		return err
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("normalize topic action event: %w", err)
	}
	var event topicActionLifecycleEvent
	if err := json.Unmarshal(canonical, &event); err != nil {
		return fmt.Errorf("bind topic action event: %w", err)
	}
	if event.EventType != topicActionRequestedEvent && event.EventType != topicActionResultEvent {
		return fmt.Errorf("unsupported topic action event_type")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid topic action event_id")
	}
	if _, err := uuid.Parse(event.JobID); err != nil {
		return fmt.Errorf("invalid topic action job_id")
	}
	if strings.TrimSpace(event.TenantID) == "" || strings.TrimSpace(event.Topic) == "" ||
		event.Revision <= 0 || strings.TrimSpace(event.TraceID) == "" {
		return fmt.Errorf("incomplete topic action event contract")
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"tenant_id": event.TenantID, "job_id": event.JobID,
		"aggregate_version": strconv.FormatInt(event.Revision, 10),
		"schema_version":    "2",
		"target_topic":      "traffic.topic.action.v2",
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return fmt.Errorf("topic action %s header/body mismatch", key)
		}
	}
	input := api.TopicActionProjectionInput{
		EventID: event.EventID, EventType: event.EventType,
		TenantID: event.TenantID, Topic: event.Topic, JobID: event.JobID,
		ActionID: event.ActionID, Revision: event.Revision, Status: event.Status,
		TraceID: event.TraceID, Payload: payload,
		KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyTopicActionProjection(ctx, input); err != nil {
		return fmt.Errorf("apply topic action projection %s: %w", event.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info(
			"Topic action lifecycle projection committed",
			zap.String("event_id", event.EventID),
			zap.String("job_id", event.JobID),
			zap.String("tenant_id", event.TenantID),
			zap.Int64("revision", event.Revision),
			zap.Int64("kafka_offset", message.Offset),
		)
	}
	return nil
}

func rejectTrailingTopicActionJSON(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode topic action event: multiple JSON values")
		}
		return fmt.Errorf("decode topic action event trailing data: %w", err)
	}
	return nil
}
