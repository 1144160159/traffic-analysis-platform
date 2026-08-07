package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AlertFeedbackProjectionInput struct {
	EventID          string
	FeedbackID       string
	TenantID         string
	AlertID          string
	UserID           string
	Label            string
	ReasonCode       string
	ModelVersion     string
	RuleVersion      string
	EventTimestampMS int64
	Payload          map[string]interface{}
	KafkaPartition   int
	KafkaOffset      int64
}

type AlertFeedbackProjectionApplier interface {
	ApplyAlertFeedbackProjection(context.Context, AlertFeedbackProjectionInput) error
}

type AlertFeedbackEventConsumer struct {
	consumer *commonkafka.Consumer
	applier  AlertFeedbackProjectionApplier
	logger   *zap.Logger
}

func NewAlertFeedbackEventConsumer(
	consumer *commonkafka.Consumer,
	applier AlertFeedbackProjectionApplier,
	logger *zap.Logger,
) (*AlertFeedbackEventConsumer, error) {
	if consumer == nil || applier == nil {
		return nil, fmt.Errorf("alert feedback consumer and projection applier are required")
	}
	return &AlertFeedbackEventConsumer{consumer: consumer, applier: applier, logger: logger}, nil
}

func (consumer *AlertFeedbackEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *AlertFeedbackEventConsumer) Close() error {
	return consumer.consumer.Close()
}

type alertFeedbackEventV1 struct {
	EventID          string   `json:"event_id"`
	EventType        string   `json:"event_type"`
	SchemaVersion    int      `json:"schema_version"`
	AggregateVersion int64    `json:"aggregate_version"`
	AlertID          string   `json:"alert_id"`
	TenantID         string   `json:"tenant_id"`
	Label            string   `json:"label"`
	ReasonCode       string   `json:"reason_code,omitempty"`
	Comment          string   `json:"comment,omitempty"`
	UserID           string   `json:"user_id"`
	Timestamp        int64    `json:"timestamp"`
	AddToWhitelist   bool     `json:"add_to_whitelist"`
	FeedbackID       string   `json:"feedback_id"`
	AlertType        string   `json:"alert_type,omitempty"`
	Severity         string   `json:"severity,omitempty"`
	Labels           []string `json:"labels,omitempty"`
	ModelVersion     string   `json:"model_version,omitempty"`
	RuleVersion      string   `json:"rule_version,omitempty"`
}

func (consumer *AlertFeedbackEventConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return fmt.Errorf("alert feedback Kafka message is nil")
	}
	var event alertFeedbackEventV1
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return fmt.Errorf("decode alert feedback event: %w", err)
	}
	if err := rejectTrailingAlertFeedbackJSON(decoder); err != nil {
		return err
	}
	if event.EventType != "alert.feedback.v1" || event.SchemaVersion != 1 ||
		event.AggregateVersion != 1 {
		return fmt.Errorf("unsupported alert feedback event contract")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid alert feedback event_id")
	}
	if _, err := uuid.Parse(event.FeedbackID); err != nil {
		return fmt.Errorf("invalid alert feedback feedback_id")
	}
	if event.EventID != event.FeedbackID {
		return fmt.Errorf("alert feedback event_id/feedback_id mismatch")
	}
	if strings.TrimSpace(event.TenantID) == "" || strings.TrimSpace(event.AlertID) == "" ||
		event.Timestamp <= 0 || (event.Label != "TP" && event.Label != "FP") {
		return fmt.Errorf("incomplete alert feedback event contract")
	}
	if event.Label == "FP" && strings.TrimSpace(event.ReasonCode) == "" {
		return fmt.Errorf("FP alert feedback requires reason_code")
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"schema_version": "1", "aggregate_version": "1",
		"tenant_id": event.TenantID, "alert_id": event.AlertID,
		"feedback_id": event.FeedbackID,
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return fmt.Errorf("alert feedback %s header/body mismatch", key)
		}
	}
	if actualKey := string(message.Key); actualKey != event.TenantID+":"+event.AlertID {
		return fmt.Errorf("alert feedback partition key/body mismatch")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(message.Value, &payload); err != nil {
		return fmt.Errorf("retain alert feedback payload: %w", err)
	}
	input := AlertFeedbackProjectionInput{
		EventID: event.EventID, FeedbackID: event.FeedbackID,
		TenantID: event.TenantID, AlertID: event.AlertID, UserID: event.UserID,
		Label: event.Label, ReasonCode: event.ReasonCode,
		ModelVersion: event.ModelVersion, RuleVersion: event.RuleVersion,
		EventTimestampMS: event.Timestamp, Payload: payload,
		KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyAlertFeedbackProjection(ctx, input); err != nil {
		return fmt.Errorf("apply alert feedback projection %s: %w", event.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info(
			"Alert feedback MLOps inbox projection committed",
			zap.String("event_id", event.EventID),
			zap.String("feedback_id", event.FeedbackID),
			zap.String("tenant_id", event.TenantID),
			zap.String("alert_id", event.AlertID),
			zap.Int64("kafka_offset", message.Offset),
		)
	}
	return nil
}

func rejectTrailingAlertFeedbackJSON(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode alert feedback event: multiple JSON values")
		}
		return fmt.Errorf("decode alert feedback event trailing data: %w", err)
	}
	return nil
}

type PostgresAlertFeedbackProjection struct {
	db *sql.DB
}

func NewPostgresAlertFeedbackProjection(db *sql.DB) (*PostgresAlertFeedbackProjection, error) {
	if db == nil {
		return nil, fmt.Errorf("alert feedback projection database is required")
	}
	return &PostgresAlertFeedbackProjection{db: db}, nil
}

func (projection *PostgresAlertFeedbackProjection) VerifySchema(ctx context.Context) error {
	var columnCount int
	err := projection.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name IN ('alert_feedback_event_projection','model_feedback_inbox')`,
	).Scan(&columnCount)
	if err != nil {
		return fmt.Errorf("verify alert feedback projection schema: %w", err)
	}
	if columnCount < 29 {
		return fmt.Errorf("alert feedback projection schema is incomplete: columns=%d want>=29", columnCount)
	}
	return nil
}

func (projection *PostgresAlertFeedbackProjection) ApplyAlertFeedbackProjection(
	ctx context.Context,
	input AlertFeedbackProjectionInput,
) error {
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return fmt.Errorf("marshal alert feedback projection payload: %w", err)
	}
	tx, err := projection.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alert feedback projection: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO alert_feedback_event_projection
			(event_id,feedback_id,tenant_id,alert_id,user_id,label,reason_code,
			 event_timestamp_ms,payload,kafka_partition,kafka_offset)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)
		ON CONFLICT DO NOTHING`,
		input.EventID, input.FeedbackID, input.TenantID, input.AlertID, input.UserID,
		input.Label, input.ReasonCode, input.EventTimestampMS, string(payload),
		input.KafkaPartition, input.KafkaOffset,
	)
	if err != nil {
		return fmt.Errorf("insert alert feedback event projection: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect alert feedback event insert: %w", err)
	}
	if inserted == 0 {
		var exactDuplicate bool
		err = tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM alert_feedback_event_projection
				WHERE event_id=$1::uuid AND feedback_id=$2::uuid
				  AND tenant_id=$3 AND alert_id=$4 AND user_id=$5
				  AND label=$6 AND reason_code=$7 AND event_timestamp_ms=$8
				  AND payload=$9::jsonb AND kafka_partition=$10 AND kafka_offset=$11
			)`,
			input.EventID, input.FeedbackID, input.TenantID, input.AlertID, input.UserID,
			input.Label, input.ReasonCode, input.EventTimestampMS, string(payload),
			input.KafkaPartition, input.KafkaOffset,
		).Scan(&exactDuplicate)
		if err != nil {
			return fmt.Errorf("verify duplicate alert feedback event: %w", err)
		}
		if !exactDuplicate {
			return fmt.Errorf("alert feedback identity or Kafka offset collision")
		}
		return tx.Commit()
	}
	inboxResult, err := tx.ExecContext(ctx, `
		INSERT INTO model_feedback_inbox
			(feedback_id,event_id,tenant_id,alert_id,user_id,label,reason_code,
			 model_version,rule_version,event_timestamp_ms,payload)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb)
		ON CONFLICT DO NOTHING`,
		input.FeedbackID, input.EventID, input.TenantID, input.AlertID, input.UserID,
		input.Label, input.ReasonCode, input.ModelVersion, input.RuleVersion,
		input.EventTimestampMS, string(payload),
	)
	if err != nil {
		return fmt.Errorf("insert model feedback inbox: %w", err)
	}
	inboxInserted, err := inboxResult.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect model feedback inbox insert: %w", err)
	}
	if inboxInserted == 0 {
		var exactInbox bool
		err = tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM model_feedback_inbox
				WHERE feedback_id=$1::uuid AND event_id=$2::uuid
				  AND tenant_id=$3 AND alert_id=$4 AND user_id=$5
				  AND label=$6 AND reason_code=$7 AND model_version=$8
				  AND rule_version=$9 AND event_timestamp_ms=$10
				  AND payload=$11::jsonb
			)`,
			input.FeedbackID, input.EventID, input.TenantID, input.AlertID, input.UserID,
			input.Label, input.ReasonCode, input.ModelVersion, input.RuleVersion,
			input.EventTimestampMS, string(payload),
		).Scan(&exactInbox)
		if err != nil {
			return fmt.Errorf("verify duplicate model feedback inbox: %w", err)
		}
		if !exactInbox {
			return fmt.Errorf("model feedback inbox identity collision")
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert feedback projection: %w", err)
	}
	return nil
}
