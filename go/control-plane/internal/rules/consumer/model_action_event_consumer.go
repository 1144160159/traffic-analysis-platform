package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ModelActionExecutionInput struct {
	EventID          string
	JobID            string
	ActionID         string
	AggregateVersion int64
	TenantID         string
	ModelID          string
	Version          string
	Action           string
	Target           string
	Payload          map[string]interface{}
	Status           string
	RequestedBy      string
	TraceID          string
	CreatedAt        time.Time
	RawPayload       map[string]interface{}
	KafkaPartition   int
	KafkaOffset      int64
}

type ModelActionExecutionApplier interface {
	ApplyModelActionExecution(context.Context, ModelActionExecutionInput) error
}

type ModelActionEventConsumer struct {
	consumer *commonkafka.Consumer
	applier  ModelActionExecutionApplier
	logger   *zap.Logger
}

func NewModelActionEventConsumer(
	kafkaConsumer *commonkafka.Consumer,
	applier ModelActionExecutionApplier,
	logger *zap.Logger,
) (*ModelActionEventConsumer, error) {
	if kafkaConsumer == nil || applier == nil {
		return nil, fmt.Errorf("model action consumer and execution applier are required")
	}
	return &ModelActionEventConsumer{
		consumer: kafkaConsumer,
		applier:  applier,
		logger:   logger,
	}, nil
}

func (consumer *ModelActionEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *ModelActionEventConsumer) Close() error {
	return consumer.consumer.Close()
}

type modelActionRequestedV1 struct {
	EventID          string                 `json:"event_id"`
	EventType        string                 `json:"event_type"`
	SchemaVersion    int                    `json:"schema_version"`
	AggregateVersion int64                  `json:"aggregate_version"`
	JobID            string                 `json:"job_id"`
	ActionID         string                 `json:"action_id"`
	TenantID         string                 `json:"tenant_id"`
	ModelID          string                 `json:"model_id"`
	Version          string                 `json:"version"`
	Action           string                 `json:"action"`
	Target           string                 `json:"target"`
	Payload          map[string]interface{} `json:"payload"`
	Status           string                 `json:"status"`
	RequestedBy      string                 `json:"requested_by"`
	TraceID          string                 `json:"trace_id"`
	CreatedAt        string                 `json:"created_at"`
}

func (consumer *ModelActionEventConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return fmt.Errorf("model action Kafka message is nil")
	}
	var event modelActionRequestedV1
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return fmt.Errorf("decode model action event: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("decode model action trailing data")
	}
	if event.EventType != "model.action.requested.v1" ||
		event.SchemaVersion != 1 || event.AggregateVersion != 1 {
		return fmt.Errorf("unsupported model action event contract")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid model action event_id")
	}
	if _, err := uuid.Parse(event.JobID); err != nil {
		return fmt.Errorf("invalid model action job_id")
	}
	if _, err := uuid.Parse(event.ActionID); err != nil {
		return fmt.Errorf("invalid model action action_id")
	}
	if _, err := uuid.Parse(event.ModelID); err != nil {
		return fmt.Errorf("invalid model action model_id")
	}
	if strings.TrimSpace(event.TenantID) == "" ||
		strings.TrimSpace(event.Action) == "" ||
		strings.TrimSpace(event.Target) == "" ||
		event.Status != "queued" {
		return fmt.Errorf("incomplete model action event contract")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, event.CreatedAt)
	if err != nil {
		return fmt.Errorf("invalid model action created_at")
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"schema_version": "1", "aggregate_version": "1",
		"tenant_id": event.TenantID, "job_id": event.JobID,
		"action_id": event.ActionID, "content_type": "application/json",
		"trace_id": event.TraceID,
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return fmt.Errorf("model action %s header/body mismatch", key)
		}
	}
	if string(message.Key) != event.ModelID {
		return fmt.Errorf("model action partition key/body mismatch")
	}
	var rawPayload map[string]interface{}
	if err := json.Unmarshal(message.Value, &rawPayload); err != nil {
		return fmt.Errorf("retain model action payload: %w", err)
	}
	input := ModelActionExecutionInput{
		EventID: event.EventID, JobID: event.JobID, ActionID: event.ActionID,
		AggregateVersion: event.AggregateVersion, TenantID: event.TenantID,
		ModelID: event.ModelID, Version: event.Version, Action: event.Action,
		Target: event.Target, Payload: event.Payload, Status: event.Status,
		RequestedBy: event.RequestedBy, TraceID: event.TraceID,
		CreatedAt:  createdAt,
		RawPayload: rawPayload, KafkaPartition: message.Partition,
		KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyModelActionExecution(ctx, input); err != nil {
		return fmt.Errorf("apply model action execution inbox %s: %w", event.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info(
			"Model action accepted into execution inbox",
			zap.String("event_id", event.EventID),
			zap.String("job_id", event.JobID),
			zap.String("action", event.Action),
			zap.Bool("business_completed", false),
		)
	}
	return nil
}

type PostgresModelActionExecutionInbox struct {
	db *sql.DB
}

func NewPostgresModelActionExecutionInbox(
	db *sql.DB,
) (*PostgresModelActionExecutionInbox, error) {
	if db == nil {
		return nil, fmt.Errorf("model action execution inbox database is required")
	}
	return &PostgresModelActionExecutionInbox{db: db}, nil
}

func (inbox *PostgresModelActionExecutionInbox) VerifySchema(ctx context.Context) error {
	var columns int
	if err := inbox.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND (
		    (table_name='model_action_jobs' AND column_name IN
		      ('event_id','revision','result','error'))
		    OR
		    (table_name='model_action_execution_inbox' AND column_name IN
		      ('event_id','job_id','tenant_id','model_id','action_id','action',
		       'state','payload','kafka_partition','kafka_offset'))
		  )`,
	).Scan(&columns); err != nil {
		return fmt.Errorf("verify model action execution inbox schema: %w", err)
	}
	if columns != 14 {
		return fmt.Errorf("model action execution inbox schema is incomplete: columns=%d want=14", columns)
	}
	return nil
}

func (inbox *PostgresModelActionExecutionInbox) ApplyModelActionExecution(
	ctx context.Context,
	input ModelActionExecutionInput,
) error {
	rawPayload, err := json.Marshal(input.RawPayload)
	if err != nil {
		return fmt.Errorf("marshal model action execution payload: %w", err)
	}
	tx, err := inbox.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin model action execution inbox: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO model_action_execution_inbox (
			event_id,job_id,tenant_id,model_id,action_id,action,state,payload,
			kafka_partition,kafka_offset
		)
		SELECT
			$1::uuid,$2,$3,$4::uuid,$5,$6,
			CASE
			  WHEN jobs.status IN ('completed','partial','failed','cancelled')
			    THEN jobs.status
			  ELSE 'awaiting_executor'
			END,
			$7::jsonb,$8,$9
		FROM model_action_jobs jobs
		WHERE jobs.job_id=$2 AND jobs.event_id=$1::uuid
		  AND jobs.action_id=$5 AND jobs.revision=$10
		  AND jobs.tenant_id=$3 AND jobs.model_id=$4::uuid
		  AND jobs.version=$11 AND jobs.action=$6 AND jobs.target=$12
		  AND jobs.payload=$13::jsonb AND jobs.requested_by=$14
		  AND jobs.trace_id=$15 AND jobs.created_at=$16
		ON CONFLICT DO NOTHING`,
		input.EventID, input.JobID, input.TenantID, input.ModelID,
		input.ActionID, input.Action, string(rawPayload),
		input.KafkaPartition, input.KafkaOffset, input.AggregateVersion,
		input.Version, input.Target, mustJSON(input.Payload), input.RequestedBy,
		input.TraceID, input.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert model action execution inbox: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect model action execution insert: %w", err)
	}
	if inserted == 0 {
		var exact bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM model_action_execution_inbox
			  WHERE event_id=$1::uuid AND job_id=$2 AND tenant_id=$3
			    AND model_id=$4::uuid AND action_id=$5 AND action=$6
			    AND payload=$7::jsonb
			    AND kafka_partition=$8 AND kafka_offset=$9
			)`,
			input.EventID, input.JobID, input.TenantID, input.ModelID,
			input.ActionID, input.Action, string(rawPayload),
			input.KafkaPartition, input.KafkaOffset,
		).Scan(&exact); err != nil {
			return fmt.Errorf("verify duplicate model action execution event: %w", err)
		}
		if !exact {
			return fmt.Errorf("model action event identity or Kafka offset collision")
		}
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE model_action_jobs
		SET status=CASE
		      WHEN status IN ('queued','running','dispatched') THEN 'awaiting_executor'
		      ELSE status
		    END,
		    result=CASE
		      WHEN status IN ('queued','running','dispatched','awaiting_executor')
		        THEN result || '{"delivery":"execution_inbox","business_completed":false}'::jsonb
		      ELSE result
		    END,
		    error=CASE
		      WHEN status IN ('queued','running','dispatched','awaiting_executor')
		        THEN ''
		      ELSE error
		    END,
		    updated_at=CASE
		      WHEN status IN ('queued','running','dispatched','awaiting_executor')
		        THEN now()
		      ELSE updated_at
		    END
		WHERE job_id=$1 AND event_id=$2::uuid AND action_id=$3
		  AND revision=$4 AND tenant_id=$5 AND model_id=$6::uuid
		  AND version=$7 AND action=$8 AND target=$9
		  AND payload=$10::jsonb AND requested_by=$11
		  AND trace_id=$12 AND created_at=$13
	`, input.JobID, input.EventID, input.ActionID, input.AggregateVersion,
		input.TenantID, input.ModelID, input.Version, input.Action,
		input.Target, mustJSON(input.Payload), input.RequestedBy, input.TraceID,
		input.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("mark model action awaiting executor: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("model action authoritative job is missing, terminal or mismatched")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model action execution inbox: %w", err)
	}
	return nil
}

func mustJSON(value interface{}) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}
