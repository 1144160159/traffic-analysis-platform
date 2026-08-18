package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/api"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type alertEvidenceLinkProjection interface {
	VerifySchema(context.Context) error
	Apply(context.Context, api.AlertEvidenceLinkProjectionInput) error
}

type AlertEvidenceLinkConsumer struct {
	consumer        *commonkafka.Consumer
	projection      alertEvidenceLinkProjection
	expectedTopic   string
	consumerGroup   string
	candidateSHA256 string
	logger          *zap.Logger
	ready           atomic.Bool
}

type alertEvidenceLinkWireEnvelope struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	SchemaVersion    int    `json:"schema_version"`
	TenantID         string `json:"tenant_id"`
	AggregateType    string `json:"aggregate_type"`
	AggregateID      string `json:"aggregate_id"`
	AggregateVersion int64  `json:"aggregate_version"`
	PartitionKey     string `json:"partition_key"`
	AlertID          string `json:"alert_id"`
	EvidenceID       string `json:"evidence_id"`
	EvidenceType     string `json:"evidence_type"`
	Status           string `json:"status"`
	SourceStore      string `json:"source_store"`
	ObjectBucket     string `json:"object_bucket"`
	ObjectKey        string `json:"object_key"`
	ObjectVersion    string `json:"object_version"`
	ObjectSHA256     string `json:"object_sha256"`
	SizeBytes        int64  `json:"size_bytes"`
	ContentType      string `json:"content_type"`
	ManifestRevision int64  `json:"manifest_revision"`
	Reason           string `json:"reason"`
	TraceID          string `json:"trace_id"`
	OccurredAt       string `json:"occurred_at"`
}

func NewAlertEvidenceLinkConsumer(
	consumer *commonkafka.Consumer,
	projection alertEvidenceLinkProjection,
	expectedTopic, consumerGroup, candidateSHA256 string,
	logger *zap.Logger,
) (*AlertEvidenceLinkConsumer, error) {
	expectedTopic = strings.TrimSpace(expectedTopic)
	consumerGroup = strings.TrimSpace(consumerGroup)
	candidateSHA256 = strings.ToLower(strings.TrimSpace(candidateSHA256))
	if consumer == nil || projection == nil || expectedTopic == "" || consumerGroup == "" ||
		len(candidateSHA256) != 64 {
		return nil, fmt.Errorf("alert evidence link consumer configuration is incomplete")
	}
	for _, value := range candidateSHA256 {
		if !strings.ContainsRune("0123456789abcdef", value) {
			return nil, fmt.Errorf("alert evidence link consumer candidate SHA256 is invalid")
		}
	}
	return &AlertEvidenceLinkConsumer{
		consumer: consumer, projection: projection, expectedTopic: expectedTopic,
		consumerGroup: consumerGroup, candidateSHA256: candidateSHA256, logger: logger,
	}, nil
}

func (c *AlertEvidenceLinkConsumer) Start(ctx context.Context) error {
	if err := c.projection.VerifySchema(ctx); err != nil {
		return fmt.Errorf("verify alert evidence link projection schema: %w", err)
	}
	c.ready.Store(true)
	defer c.ready.Store(false)
	return c.consumer.Consume(ctx, c.Handle)
}

func (c *AlertEvidenceLinkConsumer) Ready(context.Context) error {
	if c == nil || !c.ready.Load() {
		return fmt.Errorf("alert evidence link projection consumer is not assigned and ready")
	}
	return nil
}

func (c *AlertEvidenceLinkConsumer) Close() error {
	if c == nil || c.consumer == nil {
		return nil
	}
	c.ready.Store(false)
	return c.consumer.Close()
}

func (c *AlertEvidenceLinkConsumer) Handle(
	ctx context.Context, message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return commonkafka.Permanent(fmt.Errorf("alert evidence link message is nil"))
	}
	if message.Topic != c.expectedTopic || message.GetHeader("target_topic") != c.expectedTopic {
		return commonkafka.Permanent(fmt.Errorf("alert evidence link topic mismatch"))
	}
	if message.GetHeader("content_type") != "application/json" ||
		message.GetHeader("proto_message_type") != "" {
		return commonkafka.Permanent(fmt.Errorf("alert evidence link consumer requires the JSON v1 envelope"))
	}
	decoder := json.NewDecoder(bytes.NewReader(message.Value))
	decoder.DisallowUnknownFields()
	var envelope alertEvidenceLinkWireEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return commonkafka.Permanent(fmt.Errorf("decode alert evidence link event: %w", err))
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return commonkafka.Permanent(fmt.Errorf("decode alert evidence link event: trailing JSON value"))
	}
	if err := validateAlertEvidenceLinkEnvelope(envelope); err != nil {
		return commonkafka.Permanent(err)
	}
	expectedHeaders := map[string]string{
		"event_id": envelope.EventID, "event_type": envelope.EventType,
		"tenant_id": envelope.TenantID, "stream": "alert_evidence_link",
		"aggregate_id":      envelope.AggregateID,
		"aggregate_version": strconv.FormatInt(envelope.AggregateVersion, 10),
		"schema_version":    "1", "trace_id": envelope.TraceID,
	}
	for key, expected := range expectedHeaders {
		if message.GetHeader(key) != expected {
			return commonkafka.Permanent(fmt.Errorf("alert evidence link %s header/body mismatch", key))
		}
	}
	if string(message.Key) != envelope.PartitionKey {
		return commonkafka.Permanent(fmt.Errorf("alert evidence link Kafka key/body mismatch"))
	}
	var payload map[string]interface{}
	canonicalDecoder := json.NewDecoder(bytes.NewReader(message.Value))
	canonicalDecoder.UseNumber()
	if err := canonicalDecoder.Decode(&payload); err != nil {
		return commonkafka.Permanent(fmt.Errorf("normalize alert evidence link event: %w", err))
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt)
	if err != nil {
		return commonkafka.Permanent(fmt.Errorf("alert evidence link occurred_at is invalid"))
	}
	input := api.AlertEvidenceLinkProjectionInput{
		EventID: envelope.EventID, EventType: envelope.EventType, TenantID: envelope.TenantID,
		AggregateID: envelope.AggregateID, AggregateVersion: envelope.AggregateVersion,
		PartitionKey: envelope.PartitionKey, AlertID: envelope.AlertID, EvidenceID: envelope.EvidenceID,
		EvidenceType: envelope.EvidenceType, Status: envelope.Status, SourceStore: envelope.SourceStore,
		ObjectBucket: envelope.ObjectBucket, ObjectKey: envelope.ObjectKey,
		ObjectVersion: envelope.ObjectVersion, ObjectSHA256: envelope.ObjectSHA256,
		SizeBytes: envelope.SizeBytes, ContentType: envelope.ContentType,
		ManifestRevision: envelope.ManifestRevision, Reason: envelope.Reason,
		TraceID: envelope.TraceID, OccurredAt: occurredAt.UTC(), Payload: payload,
		KafkaTopic: message.Topic, KafkaPartition: message.Partition, KafkaOffset: message.Offset,
		ReceivedAt: message.Time.UTC(),
	}
	if err := c.projection.Apply(ctx, input); err != nil {
		c.ready.Store(false)
		return fmt.Errorf("apply alert evidence link projection %s: %w", envelope.EventID, err)
	}
	c.ready.Store(true)
	if c.logger != nil {
		c.logger.Info("Alert evidence link projection committed",
			zap.String("event_id", envelope.EventID), zap.Int64("kafka_offset", message.Offset),
			zap.String("candidate_sha256", c.candidateSHA256))
	}
	return nil
}

func validateAlertEvidenceLinkEnvelope(envelope alertEvidenceLinkWireEnvelope) error {
	if _, err := uuid.Parse(envelope.EventID); err != nil {
		return fmt.Errorf("alert evidence link event_id is invalid")
	}
	if _, err := uuid.Parse(envelope.AggregateID); err != nil {
		return fmt.Errorf("alert evidence link aggregate_id is invalid")
	}
	if envelope.SchemaVersion != 1 || envelope.AggregateType != "alert_evidence_link" ||
		envelope.AggregateVersion < 1 || strings.TrimSpace(envelope.TenantID) == "" ||
		strings.EqualFold(strings.TrimSpace(envelope.TenantID), "unknown") ||
		strings.TrimSpace(envelope.PartitionKey) == "" || strings.TrimSpace(envelope.AlertID) == "" ||
		strings.TrimSpace(envelope.EvidenceID) == "" || strings.TrimSpace(envelope.EvidenceType) == "" ||
		envelope.ManifestRevision < 1 || envelope.SizeBytes < 0 || strings.TrimSpace(envelope.TraceID) == "" {
		return fmt.Errorf("incomplete alert evidence link event contract")
	}
	if envelope.EventType == "traffic.alert-evidence.v1.Linked" && envelope.Status != "linked" {
		return fmt.Errorf("linked alert evidence event has mismatched status")
	}
	if envelope.EventType == "traffic.alert-evidence.v1.Unlinked" && envelope.Status != "unlinked" {
		return fmt.Errorf("unlinked alert evidence event has mismatched status")
	}
	if envelope.EventType != "traffic.alert-evidence.v1.Linked" && envelope.EventType != "traffic.alert-evidence.v1.Unlinked" {
		return fmt.Errorf("unsupported alert evidence link event type")
	}
	if envelope.SourceStore == "minio" {
		if envelope.ObjectBucket == "" || envelope.ObjectKey == "" || envelope.ObjectVersion == "" ||
			len(envelope.ObjectSHA256) != 64 || envelope.ObjectSHA256 != strings.ToLower(envelope.ObjectSHA256) {
			return fmt.Errorf("incomplete alert evidence object identity")
		}
		for _, value := range envelope.ObjectSHA256 {
			if !strings.ContainsRune("0123456789abcdef", value) {
				return fmt.Errorf("invalid alert evidence object digest")
			}
		}
	} else if envelope.ObjectBucket != "" || envelope.ObjectKey != "" || envelope.ObjectVersion != "" || envelope.ObjectSHA256 != "" {
		return fmt.Errorf("non-object alert evidence cannot carry object coordinates")
	}
	return nil
}
