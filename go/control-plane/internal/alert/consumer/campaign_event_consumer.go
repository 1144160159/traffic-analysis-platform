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

type CampaignEventProjectionApplier interface {
	ApplyCampaignEventProjection(context.Context, api.CampaignEventProjectionInput) error
}

type CampaignEventConsumer struct {
	consumer       *commonkafka.Consumer
	applier        CampaignEventProjectionApplier
	expectedStream string
	expectedTopic  string
	logger         *zap.Logger
}

func NewCampaignEventConsumer(consumer *commonkafka.Consumer, applier CampaignEventProjectionApplier,
	expectedStream, expectedTopic string, logger *zap.Logger) (*CampaignEventConsumer, error) {
	if consumer == nil || applier == nil {
		return nil, fmt.Errorf("campaign Kafka consumer and projection applier are required")
	}
	if (expectedStream != "aggregate" && expectedStream != "membership") || strings.TrimSpace(expectedTopic) == "" {
		return nil, fmt.Errorf("campaign stream and topic are invalid")
	}
	return &CampaignEventConsumer{consumer: consumer, applier: applier, expectedStream: expectedStream,
		expectedTopic: expectedTopic, logger: logger}, nil
}

func (consumer *CampaignEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *CampaignEventConsumer) Close() error { return consumer.consumer.Close() }

func (consumer *CampaignEventConsumer) handle(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	if message == nil {
		return fmt.Errorf("campaign Kafka message is nil")
	}
	if message.Topic != consumer.expectedTopic {
		return fmt.Errorf("campaign Kafka topic mismatch")
	}
	var payload map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(message.Value))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("decode campaign event: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode campaign event: multiple JSON values")
		}
		return fmt.Errorf("decode campaign event trailing data: %w", err)
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("normalize campaign event: %w", err)
	}
	var envelope struct {
		EventID, EventType, TenantID, AggregateType, AggregateID            string
		PartitionKey, CampaignID, RelationID, AlertID, TraceID              string
		SchemaVersion, AggregateVersion, RelationRevision, CampaignRevision int64
	}
	type wireEnvelope struct {
		EventID          string `json:"event_id"`
		EventType        string `json:"event_type"`
		TenantID         string `json:"tenant_id"`
		AggregateType    string `json:"aggregate_type"`
		AggregateID      string `json:"aggregate_id"`
		PartitionKey     string `json:"partition_key"`
		CampaignID       string `json:"campaign_id"`
		RelationID       string `json:"relation_id"`
		AlertID          string `json:"alert_id"`
		TraceID          string `json:"trace_id"`
		SchemaVersion    int64  `json:"schema_version"`
		AggregateVersion int64  `json:"aggregate_version"`
		RelationRevision int64  `json:"relation_revision"`
		CampaignRevision int64  `json:"campaign_revision"`
	}
	var wire wireEnvelope
	if err := json.Unmarshal(canonical, &wire); err != nil {
		return fmt.Errorf("bind campaign event: %w", err)
	}
	envelope.EventID, envelope.EventType, envelope.TenantID = wire.EventID, wire.EventType, wire.TenantID
	envelope.AggregateType, envelope.AggregateID, envelope.PartitionKey = wire.AggregateType, wire.AggregateID, wire.PartitionKey
	envelope.CampaignID, envelope.RelationID, envelope.AlertID, envelope.TraceID = wire.CampaignID, wire.RelationID, wire.AlertID, wire.TraceID
	envelope.SchemaVersion, envelope.AggregateVersion = wire.SchemaVersion, wire.AggregateVersion
	envelope.RelationRevision, envelope.CampaignRevision = wire.RelationRevision, wire.CampaignRevision
	if _, err := uuid.Parse(envelope.EventID); err != nil || envelope.SchemaVersion != 2 ||
		strings.TrimSpace(envelope.TenantID) == "" || strings.TrimSpace(envelope.TraceID) == "" ||
		strings.TrimSpace(envelope.PartitionKey) == "" || !validCampaignConsumerEvent(envelope.EventType) {
		return fmt.Errorf("incomplete campaign event contract")
	}
	campaignID := envelope.AggregateID
	aggregateID := envelope.AggregateID
	aggregateRevision := envelope.AggregateVersion
	relationID, alertID := "", ""
	relationRevision := int64(0)
	if consumer.expectedStream == "aggregate" {
		if envelope.AggregateType != "campaign" || envelope.AggregateVersion <= 0 {
			return fmt.Errorf("invalid campaign aggregate identity")
		}
	} else {
		if envelope.EventType != "traffic.campaign.v2.AlertLinked" && envelope.EventType != "traffic.campaign.v2.AlertUnlinked" {
			return fmt.Errorf("invalid campaign membership event type")
		}
		if _, err := uuid.Parse(envelope.RelationID); err != nil || envelope.RelationRevision <= 0 ||
			envelope.CampaignRevision <= 0 || envelope.CampaignID == "" || envelope.AlertID == "" {
			return fmt.Errorf("invalid campaign membership identity")
		}
		aggregateID, relationID, alertID, campaignID = envelope.RelationID, envelope.RelationID, envelope.AlertID, envelope.CampaignID
		aggregateRevision, relationRevision = envelope.CampaignRevision, envelope.RelationRevision
	}
	expectedHeaders := map[string]string{
		"event_id": envelope.EventID, "event_type": envelope.EventType, "tenant_id": envelope.TenantID,
		"stream": consumer.expectedStream, "aggregate_id": aggregateID, "campaign_id": campaignID,
		"aggregate_version": strconv.FormatInt(aggregateRevision, 10),
		"relation_revision": strconv.FormatInt(relationRevision, 10),
		"schema_version":    "2", "trace_id": envelope.TraceID, "target_topic": consumer.expectedTopic,
	}
	for key, expected := range expectedHeaders {
		if message.GetHeader(key) != expected {
			return fmt.Errorf("campaign %s header/body mismatch", key)
		}
	}
	if string(message.Key) != envelope.PartitionKey {
		return fmt.Errorf("campaign Kafka key/body mismatch")
	}
	input := api.CampaignEventProjectionInput{Stream: consumer.expectedStream, EventID: envelope.EventID,
		TenantID: envelope.TenantID, AggregateID: aggregateID, CampaignID: campaignID, RelationID: relationID,
		AlertID: alertID, EventType: envelope.EventType, SchemaVersion: 2, AggregateRevision: aggregateRevision,
		RelationRevision: relationRevision, PartitionKey: envelope.PartitionKey, TraceID: envelope.TraceID,
		Payload: payload, KafkaTopic: message.Topic, KafkaPartition: message.Partition, KafkaOffset: message.Offset,
		ReceivedAt: message.Time}
	if err := consumer.applier.ApplyCampaignEventProjection(ctx, input); err != nil {
		return fmt.Errorf("apply campaign event projection %s/%s: %w", consumer.expectedStream, envelope.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info("Campaign event inbox committed", zap.String("stream", consumer.expectedStream),
			zap.String("event_id", envelope.EventID), zap.Int64("kafka_offset", message.Offset))
	}
	return nil
}

func validCampaignConsumerEvent(value string) bool {
	switch value {
	case "traffic.campaign.v2.OwnerAssigned", "traffic.campaign.v2.StatusChanged",
		"traffic.campaign.v2.ReportRequested", "traffic.campaign.v2.ReportCompleted", "traffic.campaign.v2.ReportFailed",
		"traffic.campaign.v2.SoarRequested", "traffic.campaign.v2.SoarCompleted",
		"traffic.campaign.v2.SoarPartial", "traffic.campaign.v2.SoarFailed",
		"traffic.campaign.v2.SoarCompensated", "traffic.campaign.v2.SoarCompensationFailed",
		"traffic.campaign.v2.AlertLinked", "traffic.campaign.v2.AlertUnlinked":
		return true
	default:
		return false
	}
}
