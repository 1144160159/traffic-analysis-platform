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

type DeploymentEventProjectionInput struct {
	EventID        string
	DeploymentID   string
	TenantID       string
	Action         string
	Status         string
	OperatorID     string
	TimestampMS    int64
	Payload        map[string]interface{}
	KafkaPartition int
	KafkaOffset    int64
}

type DeploymentEventProjectionApplier interface {
	ApplyDeploymentEventProjection(context.Context, DeploymentEventProjectionInput) error
}

type DeploymentEventConsumer struct {
	consumer *commonkafka.Consumer
	applier  DeploymentEventProjectionApplier
	logger   *zap.Logger
}

func NewDeploymentEventConsumer(
	consumer *commonkafka.Consumer,
	applier DeploymentEventProjectionApplier,
	logger *zap.Logger,
) (*DeploymentEventConsumer, error) {
	if consumer == nil || applier == nil {
		return nil, fmt.Errorf("deployment event consumer and projection applier are required")
	}
	return &DeploymentEventConsumer{consumer: consumer, applier: applier, logger: logger}, nil
}

func (consumer *DeploymentEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *DeploymentEventConsumer) Close() error {
	return consumer.consumer.Close()
}

type deploymentEventV1 struct {
	EventID       string                 `json:"event_id"`
	SchemaVersion int                    `json:"schema_version"`
	EventType     string                 `json:"event_type"`
	Action        string                 `json:"action"`
	DeploymentID  string                 `json:"deployment_id"`
	TenantID      string                 `json:"tenant_id"`
	RuleVersion   string                 `json:"rule_version"`
	ModelVersion  string                 `json:"model_version"`
	FeatureSetID  string                 `json:"feature_set_id"`
	Scope         map[string]interface{} `json:"scope"`
	Status        string                 `json:"status"`
	OperatorID    string                 `json:"operator_id"`
	TimestampMS   int64                  `json:"timestamp"`
}

func (consumer *DeploymentEventConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return fmt.Errorf("deployment Kafka message is nil")
	}
	var event deploymentEventV1
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return fmt.Errorf("decode deployment event: %w", err)
	}
	if err := rejectTrailingDeploymentEventJSON(decoder); err != nil {
		return err
	}
	if event.SchemaVersion != 1 || event.EventType != "deployment_event" {
		return fmt.Errorf("unsupported deployment event contract")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid deployment event_id")
	}
	if strings.TrimSpace(event.Action) == "" || strings.TrimSpace(event.DeploymentID) == "" ||
		strings.TrimSpace(event.TenantID) == "" || strings.TrimSpace(event.Status) == "" ||
		event.TimestampMS <= 0 {
		return fmt.Errorf("incomplete deployment event contract")
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "schema_version": "1",
		"event_type": "deployment_event", "action": event.Action,
		"tenant_id": event.TenantID, "deployment_id": event.DeploymentID,
		"event_ts": fmt.Sprint(event.TimestampMS),
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return fmt.Errorf("deployment event %s header/body mismatch", key)
		}
	}
	if actualKey := string(message.Key); actualKey != event.DeploymentID {
		return fmt.Errorf("deployment event partition key/body mismatch")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(message.Value, &payload); err != nil {
		return fmt.Errorf("retain deployment event payload: %w", err)
	}
	input := DeploymentEventProjectionInput{
		EventID: event.EventID, DeploymentID: event.DeploymentID,
		TenantID: event.TenantID, Action: event.Action, Status: event.Status,
		OperatorID: event.OperatorID, TimestampMS: event.TimestampMS, Payload: payload,
		KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyDeploymentEventProjection(ctx, input); err != nil {
		return fmt.Errorf("apply deployment projection %s: %w", event.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info(
			"Deployment lifecycle projection committed",
			zap.String("event_id", event.EventID),
			zap.String("deployment_id", event.DeploymentID),
			zap.String("tenant_id", event.TenantID),
			zap.Int64("event_timestamp_ms", event.TimestampMS),
			zap.Int64("kafka_offset", message.Offset),
		)
	}
	return nil
}

func rejectTrailingDeploymentEventJSON(decoder *json.Decoder) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("decode deployment event: multiple JSON values")
		}
		return fmt.Errorf("decode deployment event trailing data: %w", err)
	}
	return nil
}

type PostgresDeploymentEventProjection struct {
	db *sql.DB
}

func NewPostgresDeploymentEventProjection(db *sql.DB) (*PostgresDeploymentEventProjection, error) {
	if db == nil {
		return nil, fmt.Errorf("deployment projection database is required")
	}
	return &PostgresDeploymentEventProjection{db: db}, nil
}

func (projection *PostgresDeploymentEventProjection) VerifySchema(ctx context.Context) error {
	var columnCount int
	err := projection.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND table_name IN ('deployment_event_projection','deployment_state_projection')`,
	).Scan(&columnCount)
	if err != nil {
		return fmt.Errorf("verify deployment projection schema: %w", err)
	}
	if columnCount < 22 {
		return fmt.Errorf("deployment projection schema is incomplete: columns=%d want>=22", columnCount)
	}
	return nil
}

func (projection *PostgresDeploymentEventProjection) ApplyDeploymentEventProjection(
	ctx context.Context,
	input DeploymentEventProjectionInput,
) error {
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return fmt.Errorf("marshal deployment projection payload: %w", err)
	}
	tx, err := projection.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin deployment projection: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO deployment_event_projection
			(event_id,deployment_id,tenant_id,action,status,operator_id,event_timestamp_ms,
			 payload,kafka_partition,kafka_offset)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10)
		ON CONFLICT DO NOTHING`,
		input.EventID, input.DeploymentID, input.TenantID, input.Action, input.Status,
		input.OperatorID, input.TimestampMS, string(payload), input.KafkaPartition, input.KafkaOffset,
	)
	if err != nil {
		return fmt.Errorf("insert deployment event projection: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect deployment event insert: %w", err)
	}
	if inserted == 0 {
		var exactDuplicate bool
		err = tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM deployment_event_projection
				WHERE event_id=$1::uuid AND deployment_id=$2 AND tenant_id=$3
				  AND action=$4 AND status=$5 AND operator_id=$6
				  AND event_timestamp_ms=$7 AND payload=$8::jsonb
				  AND kafka_partition=$9 AND kafka_offset=$10
			)`,
			input.EventID, input.DeploymentID, input.TenantID, input.Action, input.Status,
			input.OperatorID, input.TimestampMS, string(payload), input.KafkaPartition, input.KafkaOffset,
		).Scan(&exactDuplicate)
		if err != nil {
			return fmt.Errorf("verify duplicate deployment event: %w", err)
		}
		if !exactDuplicate {
			return fmt.Errorf("deployment event identity or Kafka offset collision")
		}
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO deployment_state_projection
			(tenant_id,deployment_id,action,status,operator_id,event_timestamp_ms,
			 last_event_id,payload,kafka_partition,kafka_offset)
		VALUES ($1,$2,$3,$4,$5,$6,$7::uuid,$8::jsonb,$9,$10)
		ON CONFLICT (tenant_id,deployment_id) DO UPDATE SET
			action=EXCLUDED.action,status=EXCLUDED.status,operator_id=EXCLUDED.operator_id,
			event_timestamp_ms=EXCLUDED.event_timestamp_ms,last_event_id=EXCLUDED.last_event_id,
			payload=EXCLUDED.payload,kafka_partition=EXCLUDED.kafka_partition,
			kafka_offset=EXCLUDED.kafka_offset,updated_at=now()
		WHERE EXCLUDED.event_timestamp_ms > deployment_state_projection.event_timestamp_ms
		   OR (EXCLUDED.event_timestamp_ms = deployment_state_projection.event_timestamp_ms
		       AND EXCLUDED.kafka_partition = deployment_state_projection.kafka_partition
		       AND EXCLUDED.kafka_offset > deployment_state_projection.kafka_offset)`,
		input.TenantID, input.DeploymentID, input.Action, input.Status, input.OperatorID,
		input.TimestampMS, input.EventID, string(payload), input.KafkaPartition, input.KafkaOffset,
	); err != nil {
		return fmt.Errorf("upsert deployment state projection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deployment projection: %w", err)
	}
	return nil
}
