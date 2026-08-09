package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type AlertResponseProjectionInput struct {
	EventID          string
	JobID            string
	TenantID         string
	AlertID          string
	ActionID         string
	Action           string
	Target           string
	Reason           string
	RequestedBy      string
	DryRun           bool
	AggregateVersion int64
	KafkaPartition   int
	KafkaOffset      int64
}

type AlertResponseProjectionApplier interface {
	ApplyAlertResponseProjection(context.Context, AlertResponseProjectionInput) error
}

type AlertResponseEventConsumer struct {
	consumer *commonkafka.Consumer
	applier  AlertResponseProjectionApplier
	logger   *zap.Logger
}

func NewAlertResponseEventConsumer(
	consumer *commonkafka.Consumer,
	applier AlertResponseProjectionApplier,
	logger *zap.Logger,
) (*AlertResponseEventConsumer, error) {
	if consumer == nil || applier == nil {
		return nil, fmt.Errorf("alert response consumer and projection applier are required")
	}
	return &AlertResponseEventConsumer{consumer: consumer, applier: applier, logger: logger}, nil
}

func (consumer *AlertResponseEventConsumer) Start(ctx context.Context) error {
	return consumer.consumer.Consume(ctx, consumer.handle)
}

func (consumer *AlertResponseEventConsumer) Close() error {
	return consumer.consumer.Close()
}

type alertResponseRequestedV1 struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	SchemaVersion    int    `json:"schema_version"`
	AggregateVersion int64  `json:"aggregate_version"`
	JobID            string `json:"job_id"`
	TenantID         string `json:"tenant_id"`
	AlertID          string `json:"alert_id"`
	ActionID         string `json:"action_id"`
	Action           string `json:"action"`
	Target           string `json:"target"`
	Reason           string `json:"reason"`
	RequestedBy      string `json:"requested_by"`
	ApprovedBy       string `json:"approved_by,omitempty"`
	ApprovalReason   string `json:"approval_reason,omitempty"`
	DryRun           bool   `json:"dry_run"`
}

func (consumer *AlertResponseEventConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return fmt.Errorf("alert response Kafka message is nil")
	}
	var event alertResponseRequestedV1
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return fmt.Errorf("decode alert response event: %w", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("decode alert response trailing data")
	}
	if event.EventType != "alert.response.requested.v1" ||
		event.SchemaVersion != 1 || event.AggregateVersion <= 0 {
		return fmt.Errorf("unsupported alert response event contract")
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return fmt.Errorf("invalid alert response event_id")
	}
	if strings.TrimSpace(event.JobID) == "" || strings.TrimSpace(event.TenantID) == "" ||
		strings.TrimSpace(event.AlertID) == "" || strings.TrimSpace(event.ActionID) == "" ||
		strings.TrimSpace(event.Action) == "" || strings.TrimSpace(event.Reason) == "" {
		return fmt.Errorf("incomplete alert response event contract")
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"schema_version": "1", "aggregate_version": strconv.FormatInt(event.AggregateVersion, 10),
		"tenant_id": event.TenantID, "alert_id": event.AlertID, "job_id": event.JobID,
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return fmt.Errorf("alert response %s header/body mismatch", key)
		}
	}
	if string(message.Key) != event.TenantID+":"+event.JobID {
		return fmt.Errorf("alert response partition key/body mismatch")
	}
	input := AlertResponseProjectionInput{
		EventID: event.EventID, JobID: event.JobID, TenantID: event.TenantID,
		AlertID: event.AlertID, ActionID: event.ActionID, Action: event.Action,
		Target: event.Target, Reason: event.Reason, RequestedBy: event.RequestedBy,
		DryRun: event.DryRun, AggregateVersion: event.AggregateVersion,
		KafkaPartition: message.Partition, KafkaOffset: message.Offset,
	}
	if err := consumer.applier.ApplyAlertResponseProjection(ctx, input); err != nil {
		return fmt.Errorf("apply alert response projection %s: %w", event.EventID, err)
	}
	if consumer.logger != nil {
		consumer.logger.Info(
			"Alert response execution receipt committed",
			zap.String("event_id", event.EventID),
			zap.String("job_id", event.JobID),
			zap.Bool("simulated", event.DryRun),
		)
	}
	return nil
}

type PostgresAlertResponseProjection struct {
	db *sql.DB
}

func NewPostgresAlertResponseProjection(db *sql.DB) (*PostgresAlertResponseProjection, error) {
	if db == nil {
		return nil, fmt.Errorf("alert response projection database is required")
	}
	return &PostgresAlertResponseProjection{db: db}, nil
}

func (projection *PostgresAlertResponseProjection) VerifySchema(ctx context.Context) error {
	var columns int
	if err := projection.db.QueryRowContext(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema()
		  AND (
		    (table_name='alert_response_actions' AND column_name IN
		      ('event_id','action_id','revision','approval_status','result','error'))
		    OR
			    (table_name='alert_response_execution_receipts' AND column_name IN
			      ('event_id','job_id','state','simulated','external_effect','aggregate_version','kafka_partition','kafka_offset'))
		  )`,
	).Scan(&columns); err != nil {
		return fmt.Errorf("verify alert response projection schema: %w", err)
	}
	if columns != 14 {
		return fmt.Errorf("alert response projection schema is incomplete: columns=%d want=14", columns)
	}
	return nil
}

func (projection *PostgresAlertResponseProjection) ApplyAlertResponseProjection(
	ctx context.Context,
	input AlertResponseProjectionInput,
) error {
	state := "simulated_completed"
	errorMessage := ""
	result := map[string]interface{}{
		"mode": "dry_run", "validated": true, "external_effect_applied": false,
		"action": input.Action, "target": input.Target,
	}
	if !input.DryRun {
		state = "blocked_external_executor"
		errorMessage = "real response action requires independent approval and a configured external executor"
		result["mode"] = "blocked"
		result["validated"] = false
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal alert response receipt: %w", err)
	}
	tx, err := projection.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alert response projection: %w", err)
	}
	defer tx.Rollback()
	insert, err := tx.ExecContext(ctx, `
		INSERT INTO alert_response_execution_receipts
		  (event_id,job_id,tenant_id,alert_id,action_id,state,simulated,
		   external_effect,aggregate_version,result,error,kafka_partition,kafka_offset)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,false,$8,$9::jsonb,$10,$11,$12)
		ON CONFLICT DO NOTHING`,
		input.EventID, input.JobID, input.TenantID, input.AlertID, input.ActionID,
		state, input.DryRun, input.AggregateVersion, string(resultJSON), errorMessage,
		input.KafkaPartition, input.KafkaOffset,
	)
	if err != nil {
		return fmt.Errorf("insert alert response receipt: %w", err)
	}
	inserted, err := insert.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect alert response receipt insert: %w", err)
	}
	if inserted == 0 {
		var exact bool
		if err := tx.QueryRowContext(ctx, `
				SELECT EXISTS (
				  SELECT 1 FROM alert_response_execution_receipts
				  WHERE event_id=$1::uuid AND job_id=$2 AND tenant_id=$3 AND alert_id=$4
				    AND action_id=$5 AND state=$6 AND simulated=$7
				    AND external_effect=false AND aggregate_version=$8
				    AND result=$9::jsonb AND error=$10
				)`,
			input.EventID, input.JobID, input.TenantID, input.AlertID, input.ActionID,
			state, input.DryRun, input.AggregateVersion, string(resultJSON), errorMessage,
		).Scan(&exact); err != nil {
			return fmt.Errorf("verify duplicate alert response receipt: %w", err)
		}
		if !exact {
			return fmt.Errorf("alert response identity or Kafka offset collision")
		}
		var authoritativeExact bool
		if err := tx.QueryRowContext(ctx, `
				SELECT EXISTS (
				  SELECT 1 FROM alert_response_actions
				  WHERE job_id=$1 AND event_id=$2::uuid AND tenant_id=$3 AND alert_id=$4
				    AND action_id=$5 AND action=$6 AND target=$7 AND reason=$8
				    AND requested_by=$9 AND dry_run=$10 AND status=$11
				    AND result=$12::jsonb AND error=$13
				)`,
			input.JobID, input.EventID, input.TenantID, input.AlertID, input.ActionID,
			input.Action, input.Target, input.Reason, input.RequestedBy, input.DryRun,
			state, string(resultJSON), errorMessage,
		).Scan(&authoritativeExact); err != nil {
			return fmt.Errorf("verify duplicate alert response authoritative state: %w", err)
		}
		if !authoritativeExact {
			return fmt.Errorf("alert response receipt/action state divergence")
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit idempotent alert response receipt: %w", err)
		}
		return nil
	}
	sourceStatus := "accepted"
	sourceApprovalStatus := "not_required"
	if !input.DryRun {
		sourceStatus = "approved_awaiting_executor"
		sourceApprovalStatus = "approved"
	}
	update, err := tx.ExecContext(ctx, `
		UPDATE alert_response_actions
		SET status=$1,
		    result=$2::jsonb,error=$3,revision=revision+1,updated_at=now()
		WHERE job_id=$4 AND event_id=$5::uuid AND tenant_id=$6 AND alert_id=$7
		  AND action_id=$8 AND action=$9 AND target=$10 AND reason=$11
		  AND requested_by=$12 AND dry_run=$13
		  AND (
		    (status=$14 AND approval_status=$15)
		    OR ($16=1 AND $13=false AND status IN ('accepted','pending_approval')
		        AND approval_status='pending')
		  )
		  AND revision=$16`,
		state, string(resultJSON), errorMessage, input.JobID, input.EventID,
		input.TenantID, input.AlertID, input.ActionID, input.Action, input.Target,
		input.Reason, input.RequestedBy, input.DryRun, sourceStatus,
		sourceApprovalStatus, input.AggregateVersion,
	)
	if err != nil {
		return fmt.Errorf("update alert response authoritative state: %w", err)
	}
	affected, err := update.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect alert response state update: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("alert response authoritative action is missing or mismatched")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert response execution receipt: %w", err)
	}
	return nil
}
