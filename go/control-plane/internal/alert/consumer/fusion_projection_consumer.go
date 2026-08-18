package consumer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/fusion"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	segmentkafka "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

type FusionSourceSyncProjector interface {
	ApplySourceSync(context.Context, fusion.SourceSyncCommand, string, fusion.KafkaPosition) (fusion.ProjectionReceipt, error)
}

type FusionProjectionConsumer struct {
	consumer  *commonkafka.Consumer
	projector FusionSourceSyncProjector
	logger    *zap.Logger
}

func NewFusionProjectionConsumer(
	consumer *commonkafka.Consumer,
	projector FusionSourceSyncProjector,
	logger *zap.Logger,
) (*FusionProjectionConsumer, error) {
	if consumer == nil || projector == nil {
		return nil, fmt.Errorf("fusion Kafka consumer and projector are required")
	}
	return &FusionProjectionConsumer{consumer: consumer, projector: projector, logger: logger}, nil
}

func NewFusionProjectionGenerationAdapter(
	projector FusionSourceSyncProjector,
	logger *zap.Logger,
) (*FusionProjectionConsumer, error) {
	if projector == nil {
		return nil, fmt.Errorf("fusion source-sync projector is required")
	}
	return &FusionProjectionConsumer{projector: projector, logger: logger}, nil
}

func (consumer *FusionProjectionConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.Handle)
}

func (consumer *FusionProjectionConsumer) Close() error {
	if consumer == nil || consumer.consumer == nil {
		return nil
	}
	return consumer.consumer.Close()
}

func (consumer *FusionProjectionConsumer) StartGeneration(
	ctx context.Context,
	runner *commonkafka.GenerationConsumer,
	processor *commonkafka.GenerationMessageProcessor,
) error {
	if consumer == nil || consumer.projector == nil || runner == nil || processor == nil {
		return fmt.Errorf("fusion generation runner processor and projector are required")
	}
	return runner.Run(ctx, func(
		generationContext context.Context,
		generation *segmentkafka.Generation,
		topic string,
		assignment segmentkafka.PartitionAssignment,
	) error {
		return processor.ProcessPartition(generationContext, generation, topic, assignment, consumer.Handle)
	})
}

func (consumer *FusionProjectionConsumer) Handle(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	if message == nil {
		return commonkafka.Permanent(fmt.Errorf("fusion source-sync Kafka message is nil"))
	}
	if message.Topic != fusion.SourceSyncTopic || len(message.DuplicateHeaderNames()) > 0 {
		return commonkafka.Permanent(fmt.Errorf("fusion source-sync topic or duplicate headers are invalid"))
	}
	decoder := json.NewDecoder(bytes.NewReader(message.Value))
	decoder.DisallowUnknownFields()
	var command fusion.SourceSyncCommand
	if err := decoder.Decode(&command); err != nil {
		return commonkafka.Permanent(fmt.Errorf("decode fusion source-sync event: %w", err))
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return commonkafka.Permanent(fmt.Errorf("decode fusion source-sync event: multiple JSON values"))
		}
		return commonkafka.Permanent(fmt.Errorf("decode fusion source-sync trailing data: %w", err))
	}
	if err := command.Validate(); err != nil {
		return commonkafka.Permanent(err)
	}
	expectedHeaders := map[string]string{
		"event_id": command.EventID, "event_type": command.EventType,
		"schema_version": strconv.FormatInt(command.SchemaVersion, 10),
		"aggregate_type": command.AggregateType, "aggregate_id": command.AggregateID,
		"aggregate_version": strconv.FormatInt(command.AggregateVersion, 10),
		"tenant_id":         command.TenantID, "job_id": command.JobID, "source_id": command.SourceID,
		"trace_id": command.TraceID, "target_topic": fusion.SourceSyncTopic,
	}
	for header, expected := range expectedHeaders {
		if message.GetHeader(header) != expected {
			return commonkafka.Permanent(fmt.Errorf("fusion source-sync %s header/body mismatch", header))
		}
	}
	if string(message.Key) != command.PartitionKey {
		return commonkafka.Permanent(fmt.Errorf("fusion source-sync Kafka key/body mismatch"))
	}
	canonical, err := json.Marshal(command)
	if err != nil {
		return commonkafka.Permanent(fmt.Errorf("normalize fusion source-sync event: %w", err))
	}
	eventSHA := sha256.Sum256(canonical)
	receipt, err := consumer.projector.ApplySourceSync(ctx, command, hex.EncodeToString(eventSHA[:]), fusion.KafkaPosition{
		Topic: message.Topic, Partition: message.Partition, Offset: message.Offset,
	})
	if err != nil {
		return fmt.Errorf("apply fusion source-sync event %s: %w", command.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info("Fusion source-sync projection committed",
			zap.String("event_id", command.EventID), zap.String("job_id", command.JobID),
			zap.String("tenant_id", command.TenantID), zap.String("source_id", command.SourceID),
			zap.String("disposition", receipt.Disposition), zap.String("quality_status", receipt.QualityStatus),
			zap.String("source_snapshot_id", receipt.SourceSnapshotID), zap.String("data_snapshot_id", receipt.DataSnapshotID),
			zap.Int64("kafka_offset", message.Offset), zap.Bool("replayed", receipt.Replayed))
	}
	return nil
}
