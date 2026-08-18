package projection

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	trafficv1 "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

const (
	Topic                  = "graph.projections.v1"
	ConsumerGroup          = "graph-service-projection-v1"
	ProjectionProtoType    = "traffic.v1.GraphProjectionEvent"
	ProjectionKindEntity   = "entity"
	ProjectionKindRelation = "relation"
)

var (
	ErrProjectionEnvelope = errors.New("graph projection envelope mismatch")
	ErrProjectionConflict = errors.New("graph projection version conflict")
	ErrProjectionLate     = errors.New("late graph projection version")
)

type InboxDisposition string

const (
	InboxAccepted  InboxDisposition = "accepted"
	InboxDuplicate InboxDisposition = "duplicate"
)

type Inbox struct {
	db *sql.DB
}

func NewInbox(db *sql.DB) (*Inbox, error) {
	if db == nil {
		return nil, fmt.Errorf("graph projection inbox database is required")
	}
	return &Inbox{db: db}, nil
}

func (inbox *Inbox) VerifySchema(ctx context.Context) error {
	var tableCount int
	if err := inbox.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND table_name IN (
		  'graph_projection_inbox_v1','graph_projection_current_v1',
		  'graph_projection_watermarks_v1','graph_projection_dead_letters_v1'
		)`).Scan(&tableCount); err != nil {
		return fmt.Errorf("verify graph projection inbox schema: %w", err)
	}
	if tableCount != 4 {
		return fmt.Errorf("graph projection inbox schema is incomplete: got %d of 4 tables", tableCount)
	}
	return nil
}

// Handle validates the protobuf envelope and commits the durable inbox row
// before the common Kafka consumer may commit the source offset.
func (inbox *Inbox) Handle(ctx context.Context, message *commonkafka.ReceivedMessage) error {
	_, err := inbox.Accept(ctx, message)
	return err
}

func (inbox *Inbox) Accept(ctx context.Context, message *commonkafka.ReceivedMessage) (InboxDisposition, error) {
	if message == nil || message.Topic != Topic || message.Partition < 0 ||
		message.Offset < 0 || message.Time.UnixMilli() <= 0 ||
		len(message.Value) == 0 || len(message.DuplicateHeaderNames()) > 0 {
		return "", commonkafka.Permanent(fmt.Errorf("%w: invalid source coordinates or duplicate headers", ErrProjectionEnvelope))
	}
	if !message.IsProtobuf() || message.ProtoMessageType() != ProjectionProtoType {
		return "", commonkafka.Permanent(fmt.Errorf("%w: protobuf content type or message type differs", ErrProjectionEnvelope))
	}
	var event trafficv1.GraphProjectionEvent
	if err := message.UnmarshalProto(&event); err != nil {
		return "", commonkafka.Permanent(fmt.Errorf("%w: decode protobuf: %v", ErrInvalidProjection, err))
	}
	if err := ValidateEvent(&event); err != nil {
		return "", commonkafka.Permanent(err)
	}
	metadata, err := metadataOf(&event)
	if err != nil {
		return "", commonkafka.Permanent(err)
	}
	if err := validateKafkaEnvelope(message, &event, metadata); err != nil {
		return "", commonkafka.Permanent(err)
	}
	payloadSum := sha256.Sum256(message.Value)
	payloadSHA := hex.EncodeToString(payloadSum[:])

	tx, err := inbox.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", fmt.Errorf("begin graph projection inbox transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		metadata.tenantID+"\x00"+metadata.kind+"\x00"+metadata.projectionID,
	); err != nil {
		return "", fmt.Errorf("lock graph projection identity: %w", err)
	}

	disposition, decided, err := classifyExisting(
		ctx, tx, event.GetHeader().GetEventId(), metadata, payloadSHA)
	if err != nil {
		return "", err
	}
	if decided {
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("commit duplicate graph projection: %w", err)
		}
		return disposition, nil
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO graph_projection_inbox_v1 (
		  event_id,tenant_id,event_type,schema_version,projection_kind,projection_id,
		  partition_key,aggregate_type,aggregate_id,aggregate_version,
		  source_event_id,source_system,source_sha256,projection_sha256,revoked,
		  source_topic,source_partition,source_offset,source_timestamp_ms,
		  raw_payload,payload_sha256,trace_id,occurred_at_ms
		) VALUES (
		  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
		  $16,$17,$18,$19,$20,$21,$22,$23
		)`,
		event.GetHeader().GetEventId(), metadata.tenantID, event.GetHeader().GetEventType(),
		event.GetHeader().GetSchemaVersion(), metadata.kind, metadata.projectionID,
		event.GetPartitionKey(), event.GetHeader().GetAggregateType(), event.GetHeader().GetAggregateId(),
		event.GetHeader().GetAggregateVersion(), metadata.sourceEventID, metadata.sourceSystem,
		metadata.sourceSHA256, metadata.projectionSHA256, metadata.revoked,
		message.Topic, message.Partition, message.Offset, message.Time.UnixMilli(),
		message.Value, payloadSHA, event.GetHeader().GetTraceId(), event.GetHeader().GetOccurredAt(),
	)
	if err != nil {
		return "", fmt.Errorf("insert graph projection inbox event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit graph projection inbox event: %w", err)
	}
	return InboxAccepted, nil
}

type projectionMetadata struct {
	tenantID         string
	kind             string
	projectionID     string
	projectionSHA256 string
	sourceEventID    string
	sourceSystem     string
	sourceSHA256     string
	aggregateVersion uint64
	revoked          bool
	validFrom        int64
	validTo          int64
}

func metadataOf(event *trafficv1.GraphProjectionEvent) (projectionMetadata, error) {
	header := event.GetHeader()
	metadata := projectionMetadata{tenantID: header.GetTenantId(), aggregateVersion: header.GetAggregateVersion()}
	if entity := event.GetEntity(); entity != nil {
		metadata.kind = ProjectionKindEntity
		metadata.projectionID = entity.GetIdentity().GetVertexId()
		metadata.projectionSHA256 = entity.GetProjectionSha256()
		metadata.sourceEventID = entity.GetSource().GetSourceEventId()
		metadata.sourceSystem = entity.GetSource().GetSourceSystem()
		metadata.sourceSHA256 = entity.GetSource().GetSourceSha256()
		metadata.revoked = entity.GetRevoked()
		metadata.validFrom = entity.GetValidFrom()
		metadata.validTo = entity.GetValidTo()
		return metadata, nil
	}
	if relation := event.GetRelation(); relation != nil {
		metadata.kind = ProjectionKindRelation
		metadata.projectionID = relation.GetEdgeId()
		metadata.projectionSHA256 = relation.GetProjectionSha256()
		metadata.sourceEventID = relation.GetSource().GetSourceEventId()
		metadata.sourceSystem = relation.GetSource().GetSourceSystem()
		metadata.sourceSHA256 = relation.GetSource().GetSourceSha256()
		metadata.revoked = relation.GetRevoked()
		metadata.validFrom = relation.GetValidFrom()
		metadata.validTo = relation.GetValidTo()
		return metadata, nil
	}
	return projectionMetadata{}, fmt.Errorf("%w: missing projection payload", ErrInvalidProjection)
}

func validateKafkaEnvelope(
	message *commonkafka.ReceivedMessage,
	event *trafficv1.GraphProjectionEvent,
	metadata projectionMetadata,
) error {
	header := event.GetHeader()
	expected := map[string]string{
		"event_id": header.GetEventId(), "event_type": header.GetEventType(),
		"schema_version": header.GetSchemaVersion(), "aggregate_type": header.GetAggregateType(),
		"aggregate_id": header.GetAggregateId(), "aggregate_version": strconv.FormatUint(header.GetAggregateVersion(), 10),
		"tenant_id": metadata.tenantID, "projection_kind": metadata.kind,
		"projection_id": metadata.projectionID, "projection_sha256": metadata.projectionSHA256,
		"source_event_id": metadata.sourceEventID, "trace_id": header.GetTraceId(),
	}
	for name, value := range expected {
		if message.GetHeader(name) != value {
			return fmt.Errorf("%w: %s header differs from payload", ErrProjectionEnvelope, name)
		}
	}
	if string(message.Key) != event.GetPartitionKey() {
		return fmt.Errorf("%w: Kafka key differs from canonical partition key", ErrProjectionEnvelope)
	}
	return nil
}

func classifyExisting(
	ctx context.Context,
	tx *sql.Tx,
	eventID string,
	metadata projectionMetadata,
	payloadSHA string,
) (InboxDisposition, bool, error) {
	var existingPayloadSHA, existingProjectionSHA string
	err := tx.QueryRowContext(ctx, `
		SELECT payload_sha256,projection_sha256
		FROM graph_projection_inbox_v1 WHERE event_id=$1`, eventID,
	).Scan(&existingPayloadSHA, &existingProjectionSHA)
	if err == nil {
		if existingPayloadSHA == payloadSHA && existingProjectionSHA == metadata.projectionSHA256 {
			return InboxDuplicate, true, nil
		}
		return "", false, commonkafka.Permanent(fmt.Errorf("%w: event ID was reused with different bytes", ErrProjectionConflict))
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("read graph projection event identity: %w", err)
	}

	var existingEventID, existingHash string
	err = tx.QueryRowContext(ctx, `
		SELECT event_id,projection_sha256
		FROM graph_projection_inbox_v1
		WHERE tenant_id=$1 AND projection_kind=$2 AND projection_id=$3 AND aggregate_version=$4`,
		metadata.tenantID, metadata.kind, metadata.projectionID, metadata.aggregateVersion,
	).Scan(&existingEventID, &existingHash)
	if err == nil {
		if existingHash == metadata.projectionSHA256 {
			return InboxDuplicate, true, nil
		}
		return "", false, commonkafka.Permanent(fmt.Errorf("%w: aggregate version has a different projection hash", ErrProjectionConflict))
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("read graph projection aggregate version: %w", err)
	}

	var highestVersion sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT max(aggregate_version) FROM graph_projection_inbox_v1
		WHERE tenant_id=$1 AND projection_kind=$2 AND projection_id=$3`,
		metadata.tenantID, metadata.kind, metadata.projectionID,
	).Scan(&highestVersion); err != nil {
		return "", false, fmt.Errorf("read graph projection high-water version: %w", err)
	}
	if highestVersion.Valid && highestVersion.Int64 > int64(metadata.aggregateVersion) {
		return "", false, commonkafka.Permanent(fmt.Errorf("%w: version %d is below %d",
			ErrProjectionLate, metadata.aggregateVersion, highestVersion.Int64))
	}
	return "", false, nil
}

func IsPermanentAdmissionError(err error) bool {
	return commonkafka.IsPermanent(err) || errors.Is(err, ErrInvalidProjection) ||
		errors.Is(err, ErrProjectionEnvelope) || errors.Is(err, ErrProjectionConflict) ||
		errors.Is(err, ErrProjectionLate)
}

func normalizeCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "_")
	return value
}
