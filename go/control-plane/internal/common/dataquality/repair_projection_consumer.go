package dataquality

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	FlowProjectionReplayTopic   = "flow.projection-replay.v1"
	FlowReplayProjectionVersion = "flow-replay-pg-v1"
)

var ErrReplayProjectionConflict = errors.New("replay projection identity conflict")

type FlowReplayProjectionInput struct {
	TenantID       string
	RepairID       string
	EventID        string
	IdempotencyKey string
	TraceID        string
	Payload        []byte
	SourceEventTS  int64
	SourceIngestTS int64
	KafkaTopic     string
	KafkaPartition int
	KafkaOffset    int64
}

// PostgresFlowReplayProjection is the authoritative repair target for the
// bounded flows_raw replay slice. The immutable target object and its receipt
// commit in one PostgreSQL transaction; Kafka is acknowledged only afterwards.
type PostgresFlowReplayProjection struct {
	db *sql.DB
}

func NewPostgresFlowReplayProjection(db *sql.DB) (*PostgresFlowReplayProjection, error) {
	if db == nil {
		return nil, fmt.Errorf("flow replay projection PostgreSQL is required")
	}
	return &PostgresFlowReplayProjection{db: db}, nil
}

func (p *PostgresFlowReplayProjection) Ready(ctx context.Context) error {
	if p == nil || p.db == nil {
		return fmt.Errorf("flow replay projection is unavailable")
	}
	var projectionTable, receiptTable sql.NullString
	if err := p.db.QueryRowContext(ctx, `
		SELECT to_regclass('data_quality_flow_replay_projection')::text,
		       to_regclass('data_quality_replay_projection_receipts')::text
	`).Scan(&projectionTable, &receiptTable); err != nil {
		return fmt.Errorf("verify flow replay projection schema: %w", err)
	}
	if !projectionTable.Valid || !receiptTable.Valid {
		return fmt.Errorf("flow replay projection schema is not applied")
	}
	return nil
}

func (p *PostgresFlowReplayProjection) Commit(ctx context.Context, input FlowReplayProjectionInput) error {
	if err := validateFlowReplayProjectionInput(input); err != nil {
		return err
	}
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin flow replay projection transaction: %w", err)
	}
	defer tx.Rollback()

	var operationID, status string
	if err := tx.QueryRowContext(ctx, `
		SELECT operation_id,status FROM data_quality_repairs
		WHERE tenant_id=$1 AND repair_id=$2
		FOR SHARE
	`, input.TenantID, input.RepairID).Scan(&operationID, &status); errors.Is(err, sql.ErrNoRows) {
		return ErrRepairNotFound
	} else if err != nil {
		return fmt.Errorf("load flow replay repair authority: %w", err)
	}
	if operationID != "flow_replay_window_v1" || (status != "executing" && status != "executed") {
		return ErrRepairConflict
	}

	payloadHash := fmt.Sprintf("%x", sha256.Sum256(input.Payload))
	result, err := tx.ExecContext(ctx, `
		INSERT INTO data_quality_flow_replay_projection(
			tenant_id,repair_id,event_id,idempotency_key,source_event_sha256,flow_payload,
			source_event_ts,source_ingest_ts,projection_version,trace_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT DO NOTHING
	`, input.TenantID, input.RepairID, input.EventID, input.IdempotencyKey, payloadHash,
		input.Payload, input.SourceEventTS, input.SourceIngestTS, FlowReplayProjectionVersion, input.TraceID)
	if err != nil {
		return fmt.Errorf("commit flow replay projection target: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read flow replay projection insert outcome: %w", err)
	}
	if inserted == 0 {
		var existingHash, existingKey, existingVersion string
		if err := tx.QueryRowContext(ctx, `
			SELECT source_event_sha256,idempotency_key,projection_version
			FROM data_quality_flow_replay_projection
			WHERE tenant_id=$1 AND repair_id=$2 AND event_id=$3
		`, input.TenantID, input.RepairID, input.EventID).Scan(&existingHash, &existingKey, &existingVersion); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrReplayProjectionConflict
			}
			return fmt.Errorf("verify replayed flow projection target: %w", err)
		}
		if existingHash != payloadHash || existingKey != input.IdempotencyKey || existingVersion != FlowReplayProjectionVersion {
			return ErrReplayProjectionConflict
		}
	}

	targetObjectID := input.RepairID + ":" + input.EventID
	result, err = tx.ExecContext(ctx, `
		INSERT INTO data_quality_replay_projection_receipts(
			tenant_id,repair_id,event_id,projection_id,target_store,target_object_id,target_version,
			source_event_sha256,target_payload_sha256,kafka_topic,kafka_partition,kafka_offset,trace_id
		) VALUES ($1,$2,$3,$4,'postgresql',$5,$4,$6,$6,$7,$8,$9,$10)
		ON CONFLICT DO NOTHING
	`, input.TenantID, input.RepairID, input.EventID, FlowReplayProjectionVersion, targetObjectID,
		payloadHash, input.KafkaTopic, input.KafkaPartition, input.KafkaOffset, input.TraceID)
	if err != nil {
		return fmt.Errorf("commit flow replay projection receipt: %w", err)
	}
	inserted, err = result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read flow replay receipt insert outcome: %w", err)
	}
	if inserted == 0 {
		var existingSourceHash, existingTargetHash, existingObject, existingTopic string
		var existingPartition int
		var existingOffset int64
		if err := tx.QueryRowContext(ctx, `
			SELECT source_event_sha256,target_payload_sha256,target_object_id,kafka_topic,kafka_partition,kafka_offset
			FROM data_quality_replay_projection_receipts
			WHERE tenant_id=$1 AND repair_id=$2 AND event_id=$3 AND projection_id=$4
		`, input.TenantID, input.RepairID, input.EventID, FlowReplayProjectionVersion).Scan(
			&existingSourceHash, &existingTargetHash, &existingObject, &existingTopic, &existingPartition, &existingOffset,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrReplayProjectionConflict
			}
			return fmt.Errorf("verify flow replay projection receipt: %w", err)
		}
		if existingSourceHash != payloadHash || existingTargetHash != payloadHash || existingObject != targetObjectID ||
			existingTopic != input.KafkaTopic || existingPartition != input.KafkaPartition || existingOffset != input.KafkaOffset {
			return ErrReplayProjectionConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit flow replay target and receipt transaction: %w", err)
	}
	return nil
}

func validateFlowReplayProjectionInput(input FlowReplayProjectionInput) error {
	if strings.TrimSpace(input.TenantID) == "" || strings.TrimSpace(input.EventID) == "" ||
		strings.TrimSpace(input.IdempotencyKey) == "" || strings.TrimSpace(input.TraceID) == "" || len(input.Payload) == 0 {
		return fmt.Errorf("flow replay projection identity is incomplete")
	}
	if _, err := uuid.Parse(input.RepairID); err != nil {
		return fmt.Errorf("flow replay projection repair_id is invalid")
	}
	if input.KafkaTopic != FlowProjectionReplayTopic || input.KafkaPartition < 0 || input.KafkaOffset < 0 {
		return fmt.Errorf("flow replay projection Kafka position is invalid")
	}
	return nil
}

type FlowReplayProjectionConsumer struct {
	consumer  *commonkafka.Consumer
	projector *PostgresFlowReplayProjection
	topic     string
	logger    *zap.Logger
}

func NewFlowReplayProjectionConsumer(consumer *commonkafka.Consumer, projector *PostgresFlowReplayProjection, topic string, logger *zap.Logger) (*FlowReplayProjectionConsumer, error) {
	if consumer == nil || projector == nil || strings.TrimSpace(topic) == "" {
		return nil, fmt.Errorf("flow replay consumer, projector and topic are required")
	}
	if topic != FlowProjectionReplayTopic {
		return nil, fmt.Errorf("unsupported flow replay projection topic %q", topic)
	}
	return &FlowReplayProjectionConsumer{consumer: consumer, projector: projector, topic: topic, logger: logger}, nil
}

func (c *FlowReplayProjectionConsumer) Ready(ctx context.Context) error {
	return c.projector.Ready(ctx)
}
func (c *FlowReplayProjectionConsumer) Start(ctx context.Context) error {
	return c.consumer.Consume(ctx, c.handle)
}
func (c *FlowReplayProjectionConsumer) Close() error { return c.consumer.Close() }

func (c *FlowReplayProjectionConsumer) handle(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	input, err := decodeFlowReplayProjectionMessage(message, c.topic)
	if err != nil {
		return commonkafka.Permanent(err)
	}
	if err := c.projector.Commit(ctx, input); err != nil {
		if errors.Is(err, ErrRepairNotFound) || errors.Is(err, ErrRepairConflict) || errors.Is(err, ErrReplayProjectionConflict) {
			return commonkafka.Permanent(err)
		}
		return err
	}
	if c.logger != nil {
		c.logger.Info("Flow replay target and receipt committed",
			zap.String("tenant_id", input.TenantID), zap.String("repair_id", input.RepairID),
			zap.String("event_id", input.EventID), zap.Int64("kafka_offset", input.KafkaOffset))
	}
	return nil
}

func decodeFlowReplayProjectionMessage(message *commonkafka.ReceivedMessage, expectedTopic string) (FlowReplayProjectionInput, error) {
	input := FlowReplayProjectionInput{}
	if message == nil {
		return input, fmt.Errorf("flow replay Kafka message is nil")
	}
	if message.Topic != expectedTopic || message.ContentType() != "application/x-protobuf" ||
		message.ProtoMessageType() != "traffic.v1.FlowEvent" || message.GetHeader("proto_schema_version") != "v1" ||
		message.GetHeader("replay") != "true" {
		return input, fmt.Errorf("flow replay Kafka envelope is invalid")
	}
	var event pb.FlowEvent
	if err := message.UnmarshalProto(&event); err != nil {
		return input, fmt.Errorf("decode flow replay Protobuf: %w", err)
	}
	if event.Header == nil || event.Tuple == nil || strings.TrimSpace(event.Header.EventId) == "" ||
		strings.TrimSpace(event.Header.TenantId) == "" || strings.TrimSpace(event.Header.TraceId) == "" ||
		strings.TrimSpace(event.Header.IdempotencyKey) == "" || strings.TrimSpace(event.Header.CausationId) == "" ||
		event.Header.EventType != "flow.replay.v1" || event.Header.Producer != "data-quality-repair-executor" {
		return input, fmt.Errorf("flow replay Protobuf identity is incomplete")
	}
	repairID := message.GetHeader("repair_id")
	expectedHeaders := map[string]string{
		"tenant_id":       event.Header.TenantId,
		"event_id":        event.Header.EventId,
		"idempotency_key": event.Header.IdempotencyKey,
		"repair_id":       event.Header.CausationId,
	}
	for key, expected := range expectedHeaders {
		if message.GetHeader(key) != expected {
			return input, fmt.Errorf("flow replay %s header/body mismatch", key)
		}
	}
	if event.Header.CorrelationId != repairID || event.Header.IdempotencyKey != repairID+":"+event.Header.EventId ||
		string(message.Key) != event.Header.TenantId+":"+event.CommunityId {
		return input, fmt.Errorf("flow replay causation or partition key mismatch")
	}
	canonical, err := proto.MarshalOptions{Deterministic: true}.Marshal(&event)
	if err != nil {
		return input, fmt.Errorf("normalize flow replay Protobuf: %w", err)
	}
	if partition := message.GetHeader("kafka_partition"); partition != "" && partition != strconv.Itoa(message.Partition) {
		return input, fmt.Errorf("flow replay kafka_partition header mismatch")
	}
	return FlowReplayProjectionInput{
		TenantID: event.Header.TenantId, RepairID: repairID, EventID: event.Header.EventId,
		IdempotencyKey: event.Header.IdempotencyKey, TraceID: event.Header.TraceId, Payload: canonical,
		SourceEventTS: event.Header.EventTs, SourceIngestTS: event.Header.IngestTs,
		KafkaTopic: message.Topic, KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}, nil
}
