package consumer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const modelFeedbackEventType = "model.feedback.v1"

var (
	errModelFeedbackConflict   = errors.New("model feedback revision conflicts with durable history")
	errModelFeedbackOutOfOrder = errors.New("model feedback revision is out of order")
	errModelFeedbackRetracted  = errors.New("model feedback was already retracted")
)

// ModelFeedbackPrediction is the immutable prediction identity that a feedback
// event must match before it can reach the MLOps inbox.
type ModelFeedbackPrediction struct {
	TenantID     string
	PredictionID string
	AlertID      string
	ModelVersion string
	RuleVersion  string
}

type ModelFeedbackPredictionAuthority interface {
	LookupModelFeedbackPrediction(context.Context, string) (ModelFeedbackPrediction, error)
}

type ModelFeedbackRevisionProjectionInput struct {
	EventID           string
	FeedbackID        string
	TenantID          string
	PredictionID      string
	AlertID           string
	Label             string
	LabelRevision     int64
	AdjudicationState string
	ReasonCode        string
	ModelVersion      string
	RuleVersion       string
	AdjudicatedBy     string
	PreviousEventID   string
	OccurredAtMS      int64
	TraceID           string
	Payload           []byte
	PayloadSHA256     string
	KafkaTopic        string
	KafkaPartition    int
	KafkaOffset       int64
}

type ModelFeedbackRevisionProjectionApplier interface {
	ApplyModelFeedbackRevision(context.Context, ModelFeedbackRevisionProjectionInput) error
}

type ModelFeedbackRevisionConsumer struct {
	consumer  *commonkafka.Consumer
	authority ModelFeedbackPredictionAuthority
	applier   ModelFeedbackRevisionProjectionApplier
	logger    *zap.Logger
}

func NewModelFeedbackRevisionConsumer(
	consumer *commonkafka.Consumer,
	authority ModelFeedbackPredictionAuthority,
	applier ModelFeedbackRevisionProjectionApplier,
	logger *zap.Logger,
) (*ModelFeedbackRevisionConsumer, error) {
	if consumer == nil || authority == nil || applier == nil {
		return nil, fmt.Errorf("model feedback consumer, prediction authority and projection applier are required")
	}
	return &ModelFeedbackRevisionConsumer{
		consumer: consumer, authority: authority, applier: applier, logger: logger,
	}, nil
}

func (consumer *ModelFeedbackRevisionConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *ModelFeedbackRevisionConsumer) Close() error {
	return consumer.consumer.Close()
}

type modelFeedbackEventV1 struct {
	EventID           string `json:"event_id"`
	EventType         string `json:"event_type"`
	SchemaVersion     int    `json:"schema_version"`
	AggregateVersion  int64  `json:"aggregate_version"`
	FeedbackID        string `json:"feedback_id"`
	TenantID          string `json:"tenant_id"`
	PredictionID      string `json:"prediction_id"`
	AlertID           string `json:"alert_id"`
	Label             string `json:"label"`
	LabelRevision     int64  `json:"label_revision"`
	AdjudicationState string `json:"adjudication_state"`
	ReasonCode        string `json:"reason_code"`
	ModelVersion      string `json:"model_version"`
	RuleVersion       string `json:"rule_version"`
	AdjudicatedBy     string `json:"adjudicated_by"`
	PreviousEventID   string `json:"previous_event_id,omitempty"`
	OccurredAtMS      int64  `json:"occurred_at_ms"`
	TraceID           string `json:"trace_id"`
}

func decodeModelFeedbackEvent(payload []byte) (modelFeedbackEventV1, error) {
	var event modelFeedbackEventV1
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, fmt.Errorf("decode model feedback event: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return event, fmt.Errorf("decode model feedback event: trailing JSON")
	}
	if err := validateModelFeedbackEvent(event); err != nil {
		return event, err
	}
	return event, nil
}

func validateModelFeedbackEvent(event modelFeedbackEventV1) error {
	if event.EventType != modelFeedbackEventType || event.SchemaVersion != 1 ||
		event.AggregateVersion != event.LabelRevision || event.LabelRevision < 1 {
		return fmt.Errorf("unsupported model feedback event contract")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid model feedback event_id")
	}
	if _, err := uuid.Parse(event.FeedbackID); err != nil {
		return fmt.Errorf("invalid model feedback feedback_id")
	}
	if event.PreviousEventID != "" {
		if _, err := uuid.Parse(event.PreviousEventID); err != nil {
			return fmt.Errorf("invalid model feedback previous_event_id")
		}
	}
	if strings.TrimSpace(event.TenantID) == "" || strings.TrimSpace(event.PredictionID) == "" ||
		strings.TrimSpace(event.AlertID) == "" || strings.TrimSpace(event.ModelVersion) == "" ||
		strings.TrimSpace(event.RuleVersion) == "" || strings.TrimSpace(event.AdjudicatedBy) == "" ||
		event.OccurredAtMS <= 0 {
		return fmt.Errorf("incomplete model feedback event contract")
	}
	if len(event.TraceID) != 32 {
		return fmt.Errorf("invalid model feedback trace_id")
	}
	if _, err := hex.DecodeString(event.TraceID); err != nil || event.TraceID != strings.ToLower(event.TraceID) {
		return fmt.Errorf("invalid model feedback trace_id")
	}
	if event.Label != "TP" && event.Label != "FP" {
		return fmt.Errorf("invalid model feedback label")
	}
	switch event.AdjudicationState {
	case "PROPOSED", "ADJUDICATED", "RETRACTED":
	default:
		return fmt.Errorf("invalid model feedback adjudication_state")
	}
	if event.LabelRevision == 1 && event.PreviousEventID != "" {
		return fmt.Errorf("first model feedback revision cannot have previous_event_id")
	}
	if event.LabelRevision > 1 && event.PreviousEventID == "" {
		return fmt.Errorf("later model feedback revision requires previous_event_id")
	}
	if (event.Label == "FP" || event.AdjudicationState == "RETRACTED") &&
		strings.TrimSpace(event.ReasonCode) == "" {
		return fmt.Errorf("FP or retracted model feedback requires reason_code")
	}
	return nil
}

func (consumer *ModelFeedbackRevisionConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return commonkafka.Permanent(fmt.Errorf("model feedback Kafka message is nil"))
	}
	if message.Topic != modelFeedbackEventType {
		return commonkafka.Permanent(fmt.Errorf("model feedback source topic mismatch"))
	}
	if duplicates := message.DuplicateHeaderNames(); len(duplicates) != 0 {
		return commonkafka.Permanent(fmt.Errorf("model feedback has duplicate headers: %v", duplicates))
	}
	event, err := decodeModelFeedbackEvent(message.Value)
	if err != nil {
		return commonkafka.Permanent(err)
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"schema_version": "1", "aggregate_version": fmt.Sprint(event.AggregateVersion),
		"tenant_id": event.TenantID, "feedback_id": event.FeedbackID,
		"prediction_id": event.PredictionID, "label_revision": fmt.Sprint(event.LabelRevision),
	}
	for name, expected := range expectedHeaders {
		if actual := message.GetHeader(name); actual != expected {
			return commonkafka.Permanent(fmt.Errorf("model feedback %s header/body mismatch", name))
		}
	}
	if string(message.Key) != event.TenantID+":"+event.FeedbackID {
		return commonkafka.Permanent(fmt.Errorf("model feedback partition key/body mismatch"))
	}
	authority, err := consumer.authority.LookupModelFeedbackPrediction(ctx, event.PredictionID)
	if err != nil {
		return fmt.Errorf("look up model feedback prediction: %w", err)
	}
	if authority.TenantID != event.TenantID || authority.PredictionID != event.PredictionID ||
		authority.AlertID != event.AlertID || authority.ModelVersion != event.ModelVersion ||
		authority.RuleVersion != event.RuleVersion {
		return commonkafka.Permanent(fmt.Errorf("model feedback prediction authority mismatch"))
	}
	payloadDigest := sha256.Sum256(message.Value)
	input := ModelFeedbackRevisionProjectionInput{
		EventID: event.EventID, FeedbackID: event.FeedbackID, TenantID: event.TenantID,
		PredictionID: event.PredictionID, AlertID: event.AlertID, Label: event.Label,
		LabelRevision: event.LabelRevision, AdjudicationState: event.AdjudicationState,
		ReasonCode: event.ReasonCode, ModelVersion: event.ModelVersion,
		RuleVersion: event.RuleVersion, AdjudicatedBy: event.AdjudicatedBy,
		PreviousEventID: event.PreviousEventID, OccurredAtMS: event.OccurredAtMS,
		TraceID: event.TraceID, Payload: append([]byte(nil), message.Value...),
		PayloadSHA256: hex.EncodeToString(payloadDigest[:]), KafkaTopic: message.Topic,
		KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyModelFeedbackRevision(ctx, input); err != nil {
		if errors.Is(err, errModelFeedbackConflict) ||
			errors.Is(err, errModelFeedbackOutOfOrder) ||
			errors.Is(err, errModelFeedbackRetracted) {
			return commonkafka.Permanent(err)
		}
		return err
	}
	if consumer.logger != nil {
		consumer.logger.Info("Model feedback revision committed",
			zap.String("event_id", event.EventID),
			zap.String("feedback_id", event.FeedbackID),
			zap.String("tenant_id", event.TenantID),
			zap.Int64("label_revision", event.LabelRevision),
			zap.String("adjudication_state", event.AdjudicationState))
	}
	return nil
}

type ClickHouseModelFeedbackPredictionAuthority struct {
	db *sql.DB
}

func NewClickHouseModelFeedbackPredictionAuthority(db *sql.DB) (*ClickHouseModelFeedbackPredictionAuthority, error) {
	if db == nil {
		return nil, fmt.Errorf("model feedback ClickHouse prediction authority is required")
	}
	return &ClickHouseModelFeedbackPredictionAuthority{db: db}, nil
}

func (authority *ClickHouseModelFeedbackPredictionAuthority) VerifySchema(ctx context.Context) error {
	var columns int
	if err := authority.db.QueryRowContext(ctx, `
		SELECT count() FROM system.columns
		WHERE database='traffic' AND table='alerts_latest'
		  AND name IN ('tenant_id','alert_id','event_id','model_version','rule_version')`,
	).Scan(&columns); err != nil {
		return fmt.Errorf("verify model feedback prediction authority: %w", err)
	}
	if columns != 5 {
		return fmt.Errorf("model feedback prediction authority schema is incomplete: columns=%d want=5", columns)
	}
	return nil
}

func (authority *ClickHouseModelFeedbackPredictionAuthority) LookupModelFeedbackPrediction(
	ctx context.Context, predictionID string,
) (ModelFeedbackPrediction, error) {
	rows, err := authority.db.QueryContext(ctx, `
		SELECT tenant_id,event_id,alert_id,model_version,rule_version
		FROM traffic.alerts_latest FINAL
		WHERE event_id=?
		ORDER BY updated_at DESC
		LIMIT 2`, predictionID)
	if err != nil {
		return ModelFeedbackPrediction{}, err
	}
	defer rows.Close()
	var matches []ModelFeedbackPrediction
	for rows.Next() {
		var prediction ModelFeedbackPrediction
		if err := rows.Scan(&prediction.TenantID, &prediction.PredictionID, &prediction.AlertID,
			&prediction.ModelVersion, &prediction.RuleVersion); err != nil {
			return ModelFeedbackPrediction{}, err
		}
		matches = append(matches, prediction)
	}
	if err := rows.Err(); err != nil {
		return ModelFeedbackPrediction{}, err
	}
	if len(matches) == 0 {
		return ModelFeedbackPrediction{}, fmt.Errorf("model feedback prediction is not materialized yet")
	}
	if len(matches) != 1 {
		return ModelFeedbackPrediction{}, commonkafka.Permanent(fmt.Errorf("model feedback prediction identity is ambiguous"))
	}
	return matches[0], nil
}

type PostgresModelFeedbackRevisionProjection struct {
	db        *sql.DB
	readiness *ModelFeedbackConsumerReadinessOptions
}

type ModelFeedbackConsumerReadinessOptions struct {
	ConsumerGroup   string
	CandidateSHA256 string
	ContractSHA256  string
}

type modelFeedbackRevisionHead struct {
	TenantID          string
	PredictionID      string
	AlertID           string
	ModelVersion      string
	RuleVersion       string
	Label             string
	LabelRevision     int64
	AdjudicationState string
	LastEventID       string
	PayloadSHA256     string
}

func NewPostgresModelFeedbackRevisionProjection(db *sql.DB) (*PostgresModelFeedbackRevisionProjection, error) {
	if db == nil {
		return nil, fmt.Errorf("model feedback revision PostgreSQL database is required")
	}
	return &PostgresModelFeedbackRevisionProjection{db: db}, nil
}

func NewPostgresModelFeedbackRevisionProjectionWithReadiness(
	db *sql.DB,
	options ModelFeedbackConsumerReadinessOptions,
) (*PostgresModelFeedbackRevisionProjection, error) {
	projection, err := NewPostgresModelFeedbackRevisionProjection(db)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(options.ConsumerGroup) == "" ||
		!approvedModelFeedbackSHA256(options.CandidateSHA256, true) ||
		!approvedModelFeedbackSHA256(options.ContractSHA256, false) {
		return nil, fmt.Errorf("approved model feedback consumer group, candidate and contract hashes are required")
	}
	copy := options
	projection.readiness = &copy
	return projection, nil
}

func approvedModelFeedbackSHA256(value string, rejectZero bool) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	if _, err := hex.DecodeString(value); err != nil {
		return false
	}
	return !rejectZero || strings.Trim(value, "0") != ""
}

func (projection *PostgresModelFeedbackRevisionProjection) VerifySchema(ctx context.Context) error {
	var columns int
	if err := projection.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND ((table_name='model_feedback_revision_head' AND column_name IN
		        ('feedback_id','tenant_id','prediction_id','alert_id','model_version','rule_version',
		         'label','label_revision','adjudication_state','last_event_id','payload_sha256',
		         'occurred_at_ms','created_at','updated_at'))
		    OR (table_name='model_feedback_revision_inbox' AND column_name IN
		        ('event_id','feedback_id','tenant_id','prediction_id','alert_id','label','label_revision',
		         'adjudication_state','reason_code','model_version','rule_version','adjudicated_by',
		         'previous_event_id','occurred_at_ms','trace_id','payload','payload_sha256','kafka_topic',
		         'kafka_partition','kafka_offset','status','attempts','last_error','next_attempt_at',
		         'locked_until','locked_by','applied_at','created_at','updated_at'))
		    OR (table_name='model_feedback_revision_receipt' AND column_name IN
		        ('event_id','feedback_id','tenant_id','label_revision','outcome','payload_sha256',
		         'kafka_topic','kafka_partition','kafka_offset','recorded_at'))
		    OR (table_name='model_feedback_consumer_readiness_receipt' AND column_name IN
		        ('consumer_group','candidate_sha256','contract_sha256','kafka_topic','state',
		         'event_id','kafka_partition','kafka_offset','observed_at','updated_at')))`,
	).Scan(&columns); err != nil {
		return fmt.Errorf("verify model feedback revision schema: %w", err)
	}
	if columns != 63 {
		return fmt.Errorf("model feedback revision schema is incomplete: columns=%d want=63", columns)
	}
	return nil
}

func validateModelFeedbackRevision(
	head *modelFeedbackRevisionHead,
	input ModelFeedbackRevisionProjectionInput,
) error {
	if head == nil {
		if input.LabelRevision != 1 {
			return errModelFeedbackOutOfOrder
		}
		if input.AdjudicationState == "RETRACTED" {
			return fmt.Errorf("%w: first revision cannot be retracted", errModelFeedbackOutOfOrder)
		}
		return nil
	}
	if head.TenantID != input.TenantID || head.PredictionID != input.PredictionID ||
		head.AlertID != input.AlertID || head.ModelVersion != input.ModelVersion ||
		head.RuleVersion != input.RuleVersion {
		return errModelFeedbackConflict
	}
	if input.LabelRevision != head.LabelRevision+1 || input.PreviousEventID != head.LastEventID {
		return errModelFeedbackOutOfOrder
	}
	if head.AdjudicationState == "RETRACTED" {
		return errModelFeedbackRetracted
	}
	if input.AdjudicationState == "PROPOSED" {
		return fmt.Errorf("%w: later revision cannot return to proposed", errModelFeedbackOutOfOrder)
	}
	if input.AdjudicationState == "RETRACTED" && input.Label != head.Label {
		return fmt.Errorf("%w: retraction label differs from prior revision", errModelFeedbackConflict)
	}
	return nil
}

func (projection *PostgresModelFeedbackRevisionProjection) ApplyModelFeedbackRevision(
	ctx context.Context,
	input ModelFeedbackRevisionProjectionInput,
) error {
	tx, err := projection.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin model feedback revision projection: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, input.FeedbackID); err != nil {
		return fmt.Errorf("lock model feedback revision: %w", err)
	}

	var eventFeedbackID, eventPayloadSHA256 string
	var eventLabelRevision int64
	err = tx.QueryRowContext(ctx, `
		SELECT feedback_id::text,label_revision,payload_sha256
		FROM model_feedback_revision_inbox
		WHERE event_id=$1::uuid`, input.EventID,
	).Scan(&eventFeedbackID, &eventLabelRevision, &eventPayloadSHA256)
	if err == nil {
		if eventFeedbackID == input.FeedbackID && eventLabelRevision == input.LabelRevision &&
			eventPayloadSHA256 == input.PayloadSHA256 {
			return tx.Commit()
		}
		return errModelFeedbackConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect model feedback event replay: %w", err)
	}

	var existingEventID, existingPayloadSHA256 string
	err = tx.QueryRowContext(ctx, `
		SELECT event_id::text,payload_sha256
		FROM model_feedback_revision_inbox
		WHERE feedback_id=$1::uuid AND label_revision=$2`,
		input.FeedbackID, input.LabelRevision,
	).Scan(&existingEventID, &existingPayloadSHA256)
	if err == nil {
		if existingEventID == input.EventID && existingPayloadSHA256 == input.PayloadSHA256 {
			return tx.Commit()
		}
		return errModelFeedbackConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect model feedback revision replay: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
		"prediction:"+input.TenantID+":"+input.PredictionID,
	); err != nil {
		return fmt.Errorf("lock model feedback prediction identity: %w", err)
	}
	var predictionFeedbackID string
	err = tx.QueryRowContext(ctx, `
		SELECT feedback_id::text
		FROM model_feedback_revision_head
		WHERE tenant_id=$1 AND prediction_id=$2
		FOR UPDATE`, input.TenantID, input.PredictionID,
	).Scan(&predictionFeedbackID)
	if err == nil && predictionFeedbackID != input.FeedbackID {
		return fmt.Errorf("%w: prediction already belongs to another feedback aggregate", errModelFeedbackConflict)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inspect model feedback prediction owner: %w", err)
	}

	var head modelFeedbackRevisionHead
	err = tx.QueryRowContext(ctx, `
		SELECT tenant_id,prediction_id,alert_id,model_version,rule_version,label,
		       label_revision,adjudication_state,last_event_id::text,payload_sha256
		FROM model_feedback_revision_head
		WHERE feedback_id=$1::uuid
		FOR UPDATE`, input.FeedbackID,
	).Scan(&head.TenantID, &head.PredictionID, &head.AlertID, &head.ModelVersion,
		&head.RuleVersion, &head.Label, &head.LabelRevision, &head.AdjudicationState,
		&head.LastEventID, &head.PayloadSHA256)
	var durableHead *modelFeedbackRevisionHead
	if err == nil {
		durableHead = &head
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read model feedback revision head: %w", err)
	}
	if err := validateModelFeedbackRevision(durableHead, input); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_feedback_revision_inbox
			(event_id,feedback_id,tenant_id,prediction_id,alert_id,label,label_revision,
			 adjudication_state,reason_code,model_version,rule_version,adjudicated_by,
			 previous_event_id,occurred_at_ms,trace_id,payload,payload_sha256,
			 kafka_topic,kafka_partition,kafka_offset,status)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,
		        NULLIF($13,'')::uuid,$14,$15,$16::jsonb,$17,$18,$19,$20,'pending')`,
		input.EventID, input.FeedbackID, input.TenantID, input.PredictionID, input.AlertID,
		input.Label, input.LabelRevision, input.AdjudicationState, input.ReasonCode,
		input.ModelVersion, input.RuleVersion, input.AdjudicatedBy, input.PreviousEventID,
		input.OccurredAtMS, input.TraceID, string(input.Payload), input.PayloadSHA256,
		input.KafkaTopic, input.KafkaPartition, input.KafkaOffset,
	); err != nil {
		return fmt.Errorf("insert model feedback revision inbox: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_feedback_revision_head
			(feedback_id,tenant_id,prediction_id,alert_id,model_version,rule_version,
			 label,label_revision,adjudication_state,last_event_id,payload_sha256,
			 occurred_at_ms,updated_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10::uuid,$11,$12,now())
		ON CONFLICT (feedback_id) DO UPDATE SET
			label=EXCLUDED.label,label_revision=EXCLUDED.label_revision,
			adjudication_state=EXCLUDED.adjudication_state,
			last_event_id=EXCLUDED.last_event_id,payload_sha256=EXCLUDED.payload_sha256,
			occurred_at_ms=EXCLUDED.occurred_at_ms,updated_at=now()`,
		input.FeedbackID, input.TenantID, input.PredictionID, input.AlertID,
		input.ModelVersion, input.RuleVersion, input.Label, input.LabelRevision,
		input.AdjudicationState, input.EventID, input.PayloadSHA256, input.OccurredAtMS,
	); err != nil {
		return fmt.Errorf("advance model feedback revision head: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_feedback_revision_receipt
			(event_id,feedback_id,tenant_id,label_revision,outcome,payload_sha256,
			 kafka_topic,kafka_partition,kafka_offset,recorded_at)
		VALUES ($1::uuid,$2::uuid,$3,$4,'ACCEPTED',$5,$6,$7,$8,now())`,
		input.EventID, input.FeedbackID, input.TenantID, input.LabelRevision,
		input.PayloadSHA256, input.KafkaTopic, input.KafkaPartition, input.KafkaOffset,
	); err != nil {
		return fmt.Errorf("insert model feedback revision receipt: %w", err)
	}
	if projection.readiness != nil {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO model_feedback_consumer_readiness_receipt
				(consumer_group,candidate_sha256,contract_sha256,kafka_topic,state,
				 event_id,kafka_partition,kafka_offset,observed_at,updated_at)
			VALUES ($1,$2,$3,$4,'READY',$5::uuid,$6,$7,$8,now())
			ON CONFLICT (consumer_group,candidate_sha256) DO UPDATE SET
				state='READY',event_id=EXCLUDED.event_id,
				kafka_partition=EXCLUDED.kafka_partition,kafka_offset=EXCLUDED.kafka_offset,
				observed_at=EXCLUDED.observed_at,updated_at=now()
			WHERE model_feedback_consumer_readiness_receipt.contract_sha256=EXCLUDED.contract_sha256
			  AND model_feedback_consumer_readiness_receipt.kafka_topic=EXCLUDED.kafka_topic`,
			projection.readiness.ConsumerGroup, projection.readiness.CandidateSHA256,
			projection.readiness.ContractSHA256, input.KafkaTopic, input.EventID,
			input.KafkaPartition, input.KafkaOffset, time.Now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("write model feedback consumer readiness receipt: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect model feedback consumer readiness receipt: %w", err)
		}
		if rows != 1 {
			return fmt.Errorf("%w: readiness receipt contract binding differs", errModelFeedbackConflict)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model feedback revision projection: %w", err)
	}
	return nil
}
