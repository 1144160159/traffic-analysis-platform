package consumer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

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
	ApprovedBy       string
	ApprovalReason   string
	TraceID          string
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
	TraceID          string `json:"trace_id"`
	DryRun           bool   `json:"dry_run"`
}

func (consumer *AlertResponseEventConsumer) handle(
	ctx context.Context,
	message *commonkafka.ReceivedMessage,
) error {
	if message == nil {
		return commonkafka.Permanent(fmt.Errorf("alert response Kafka message is nil"))
	}
	var event alertResponseRequestedV1
	decoder := json.NewDecoder(strings.NewReader(string(message.Value)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return commonkafka.Permanent(fmt.Errorf("decode alert response event: %w", err))
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return commonkafka.Permanent(fmt.Errorf("decode alert response trailing data"))
	}
	if event.EventType != "alert.response.requested.v1" ||
		event.SchemaVersion != 1 || event.AggregateVersion <= 0 {
		return commonkafka.Permanent(fmt.Errorf("unsupported alert response event contract"))
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return commonkafka.Permanent(fmt.Errorf("invalid alert response event_id"))
	}
	if strings.TrimSpace(event.JobID) == "" || strings.TrimSpace(event.TenantID) == "" ||
		strings.TrimSpace(event.AlertID) == "" || strings.TrimSpace(event.ActionID) == "" ||
		strings.TrimSpace(event.Action) == "" || strings.TrimSpace(event.Target) == "" ||
		strings.TrimSpace(event.Reason) == "" || strings.TrimSpace(event.RequestedBy) == "" ||
		strings.TrimSpace(event.TraceID) == "" {
		return commonkafka.Permanent(fmt.Errorf("incomplete alert response event contract"))
	}
	if !event.DryRun && event.AggregateVersion >= 2 &&
		(strings.TrimSpace(event.ApprovedBy) == "" || strings.TrimSpace(event.ApprovalReason) == "" ||
			event.RequestedBy == event.ApprovedBy) {
		return commonkafka.Permanent(fmt.Errorf("real alert response event lacks independent approval authority"))
	}
	expectedHeaders := map[string]string{
		"event_id": event.EventID, "event_type": event.EventType,
		"schema_version": "1", "aggregate_version": strconv.FormatInt(event.AggregateVersion, 10),
		"tenant_id": event.TenantID, "alert_id": event.AlertID, "job_id": event.JobID,
		"action_id": event.ActionID, "trace_id": event.TraceID,
	}
	for key, expected := range expectedHeaders {
		if actual := message.GetHeader(key); actual != expected {
			return commonkafka.Permanent(fmt.Errorf("alert response %s header/body mismatch", key))
		}
	}
	if string(message.Key) != event.TenantID+":"+event.JobID {
		return commonkafka.Permanent(fmt.Errorf("alert response partition key/body mismatch"))
	}
	input := AlertResponseProjectionInput{
		EventID: event.EventID, JobID: event.JobID, TenantID: event.TenantID,
		AlertID: event.AlertID, ActionID: event.ActionID, Action: event.Action,
		Target: event.Target, Reason: event.Reason, RequestedBy: event.RequestedBy,
		ApprovedBy: event.ApprovedBy, ApprovalReason: event.ApprovalReason, TraceID: event.TraceID,
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
	db                         *sql.DB
	executor                   AlertResponseExecutor
	unknownRecheckEnabled      bool
	unknownRecheckMaxAttempts  int
	unknownRecheckInitialDelay time.Duration
}

func (projection *PostgresAlertResponseProjection) ConfigureExecutor(executor AlertResponseExecutor) error {
	if executor == nil {
		return fmt.Errorf("alert response external executor is required")
	}
	projection.executor = executor
	return nil
}

// ConfigureUnknownEffectReconciliation enables creation of a durable,
// bounded authority-only recheck. The recovery worker never re-executes the
// original external effect after the provider transport becomes ambiguous.
func (projection *PostgresAlertResponseProjection) ConfigureUnknownEffectReconciliation(
	maxAttempts int,
	initialDelay time.Duration,
) error {
	if maxAttempts < 1 || maxAttempts > 100 {
		return fmt.Errorf("alert response authority recheck max attempts must be between 1 and 100")
	}
	if initialDelay < 0 || initialDelay > 24*time.Hour {
		return fmt.Errorf("alert response authority recheck initial delay must be between 0 and 24h")
	}
	projection.unknownRecheckEnabled = true
	projection.unknownRecheckMaxAttempts = maxAttempts
	projection.unknownRecheckInitialDelay = initialDelay
	return nil
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
		      ('event_id','action_id','revision','approval_status','result','error','trace_id'))
		    OR
			    (table_name='alert_response_execution_receipts' AND column_name IN
			      ('event_id','job_id','state','simulated','external_effect','aggregate_version','kafka_partition','kafka_offset',
			       'provider','provider_receipt_id','effect_state','effect_ids','trace_id','receipt_sha256','authority_lookup','executed_at'))
			    OR
			    (table_name='alert_response_dlq_receipts' AND column_name IN
			      ('source_topic','source_partition','source_offset','dlq_topic','event_id','tenant_id','job_id',
			       'alert_id','action_id','aggregate_version','trace_id','error_code','error_message',
			       'payload_sha256','headers_sha256','headers','acknowledged_at'))
			    OR
			    (table_name='alert_response_execution_authority_rechecks' AND column_name IN
			      ('recheck_id','event_id','job_id','tenant_id','trace_id','status','attempts',
			       'max_attempts','next_attempt_at','locked_until','locked_by','last_authority_state',
			       'last_error','resolved_at'))
		  )`,
	).Scan(&columns); err != nil {
		return fmt.Errorf("verify alert response projection schema: %w", err)
	}
	if columns != 54 {
		return fmt.Errorf("alert response projection schema is incomplete: columns=%d want=54", columns)
	}
	return nil
}

func (projection *PostgresAlertResponseProjection) ApplyAlertResponseProjection(
	ctx context.Context,
	input AlertResponseProjectionInput,
) error {
	committed, err := projection.hasExactCommittedReceipt(ctx, input)
	if err != nil {
		return err
	}
	if committed {
		return nil
	}
	outcome, err := projection.resolveExecutionOutcome(ctx, input)
	if err != nil {
		return err
	}
	resultJSON, err := json.Marshal(outcome.Result)
	if err != nil {
		return fmt.Errorf("marshal alert response receipt result: %w", err)
	}
	effectIDsJSON, err := json.Marshal(outcome.EffectIDs)
	if err != nil {
		return fmt.Errorf("marshal alert response receipt effects: %w", err)
	}
	authorityJSON, err := json.Marshal(outcome.AuthorityLookup)
	if err != nil {
		return fmt.Errorf("marshal alert response authority lookup: %w", err)
	}
	tx, err := projection.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alert response projection: %w", err)
	}
	defer tx.Rollback()
	insert, err := tx.ExecContext(ctx, `
		INSERT INTO alert_response_execution_receipts
		  (event_id,job_id,tenant_id,alert_id,action_id,state,simulated,
		   external_effect,aggregate_version,result,error,kafka_partition,kafka_offset,
		   provider,provider_receipt_id,effect_state,effect_ids,trace_id,receipt_sha256,authority_lookup,executed_at)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13,
		        $14,$15,$16,$17::jsonb,$18,$19,$20::jsonb,$21)
		ON CONFLICT DO NOTHING`,
		input.EventID, input.JobID, input.TenantID, input.AlertID, input.ActionID,
		outcome.State, input.DryRun, outcome.ExternalEffect, input.AggregateVersion,
		string(resultJSON), outcome.ErrorMessage, input.KafkaPartition, input.KafkaOffset,
		outcome.Provider, outcome.ProviderReceiptID, outcome.EffectState, string(effectIDsJSON),
		input.TraceID, outcome.ReceiptSHA256, string(authorityJSON), outcome.ExecutedAt,
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
				    AND external_effect=$8 AND aggregate_version=$9
				    AND result=$10::jsonb AND error=$11 AND provider=$12
				    AND provider_receipt_id=$13 AND effect_state=$14 AND effect_ids=$15::jsonb
				    AND trace_id=$16 AND receipt_sha256=$17 AND authority_lookup=$18::jsonb
				)`,
			input.EventID, input.JobID, input.TenantID, input.AlertID, input.ActionID,
			outcome.State, input.DryRun, outcome.ExternalEffect, input.AggregateVersion,
			string(resultJSON), outcome.ErrorMessage, outcome.Provider, outcome.ProviderReceiptID,
			outcome.EffectState, string(effectIDsJSON), input.TraceID, outcome.ReceiptSHA256, string(authorityJSON),
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
			outcome.State, string(resultJSON), outcome.ErrorMessage,
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
		outcome.State, string(resultJSON), outcome.ErrorMessage, input.JobID, input.EventID,
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
	if projection.unknownRecheckEnabled && outcome.State == "partial" && outcome.EffectState == "unknown" {
		recheckID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert-response-authority-recheck:"+input.EventID)).String()
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_response_execution_authority_rechecks
			(recheck_id,event_id,job_id,tenant_id,trace_id,status,attempts,max_attempts,next_attempt_at)
			VALUES ($1::uuid,$2::uuid,$3,$4,$5,'pending',0,$6,$7)
			ON CONFLICT (event_id) DO NOTHING`,
			recheckID, input.EventID, input.JobID, input.TenantID, input.TraceID,
			projection.unknownRecheckMaxAttempts, outcome.ExecutedAt.Add(projection.unknownRecheckInitialDelay),
		); err != nil {
			return fmt.Errorf("enqueue alert response authority recheck: %w", err)
		}
		var exact bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM alert_response_execution_authority_rechecks
			WHERE recheck_id=$1::uuid AND event_id=$2::uuid AND job_id=$3 AND tenant_id=$4
			  AND trace_id=$5 AND max_attempts=$6)`,
			recheckID, input.EventID, input.JobID, input.TenantID, input.TraceID,
			projection.unknownRecheckMaxAttempts,
		).Scan(&exact); err != nil {
			return fmt.Errorf("verify alert response authority recheck: %w", err)
		}
		if !exact {
			return fmt.Errorf("alert response authority recheck identity collision")
		}
	}
	if outcome.AuditRequired {
		auditDetail, marshalErr := json.Marshal(map[string]interface{}{
			"event_id": input.EventID, "job_id": input.JobID, "alert_id": input.AlertID,
			"action_id": input.ActionID, "action": input.Action, "target": input.Target,
			"requested_by": input.RequestedBy, "approved_by": input.ApprovedBy,
			"aggregate_version": input.AggregateVersion, "provider": outcome.Provider,
			"provider_receipt_id": outcome.ProviderReceiptID, "effect_state": outcome.EffectState,
			"effect_ids": outcome.EffectIDs, "receipt_sha256": outcome.ReceiptSHA256,
			"authority_lookup": outcome.AuthorityLookup, "trace_id": input.TraceID,
		})
		if marshalErr != nil {
			return fmt.Errorf("marshal alert response execution audit: %w", marshalErr)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
			(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,created_at)
			VALUES($1,$2,NULL,$3,'alert_response_action',$4,$5::jsonb,$6,$7,$8,$9)`,
			"audit-alert-response-execution-"+input.EventID, input.TenantID,
			"ALERT_RESPONSE_EXECUTION_"+strings.ToUpper(outcome.State), input.JobID,
			string(auditDetail), input.TraceID, outcome.State, outcome.State == "completed", outcome.ExecutedAt,
		); err != nil {
			return fmt.Errorf("insert alert response execution audit: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert response execution receipt: %w", err)
	}
	return nil
}

type alertResponseExecutionOutcome struct {
	State             string
	ExternalEffect    bool
	Provider          string
	ProviderReceiptID string
	EffectState       string
	EffectIDs         []string
	Result            map[string]interface{}
	ErrorMessage      string
	ReceiptSHA256     string
	AuthorityLookup   map[string]interface{}
	ExecutedAt        time.Time
	AuditRequired     bool
}

func (projection *PostgresAlertResponseProjection) hasExactCommittedReceipt(
	ctx context.Context,
	input AlertResponseProjectionInput,
) (bool, error) {
	var receiptExists, exact bool
	if err := projection.db.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM alert_response_execution_receipts WHERE event_id=$1::uuid),
		EXISTS(SELECT 1 FROM alert_response_execution_receipts r
		  JOIN alert_response_actions a ON a.job_id=r.job_id
		  WHERE r.event_id=$1::uuid AND r.job_id=$2 AND r.tenant_id=$3 AND r.alert_id=$4
		    AND r.action_id=$5 AND r.aggregate_version=$6 AND a.event_id=r.event_id
		    AND a.tenant_id=r.tenant_id AND a.alert_id=r.alert_id AND a.action_id=r.action_id
		    AND a.action=$7 AND a.target=$8 AND a.reason=$9 AND a.requested_by=$10
		    AND a.dry_run=$11 AND a.status=r.state AND a.result=r.result AND a.error=r.error)`,
		input.EventID, input.JobID, input.TenantID, input.AlertID, input.ActionID,
		input.AggregateVersion, input.Action, input.Target, input.Reason, input.RequestedBy, input.DryRun,
	).Scan(&receiptExists, &exact); err != nil {
		return false, fmt.Errorf("inspect existing alert response receipt: %w", err)
	}
	if receiptExists && !exact {
		return false, fmt.Errorf("alert response committed receipt identity collision")
	}
	return exact, nil
}

func (projection *PostgresAlertResponseProjection) resolveExecutionOutcome(
	ctx context.Context,
	input AlertResponseProjectionInput,
) (alertResponseExecutionOutcome, error) {
	now := time.Now().UTC()
	if input.DryRun {
		return newAlertResponseSyntheticOutcome(input, "simulated_completed", "internal-simulation",
			"simulation:"+input.EventID, "none", "", now, false, map[string]interface{}{
				"mode": "dry_run", "validated": true, "external_effect_applied": false,
				"action": input.Action, "target": input.Target,
			}), nil
	}
	if input.AggregateVersion < 2 || strings.TrimSpace(input.ApprovedBy) == "" ||
		strings.TrimSpace(input.ApprovalReason) == "" || input.RequestedBy == input.ApprovedBy {
		return newAlertResponseSyntheticOutcome(input, "blocked_external_executor", "legacy-approval-guard",
			"blocked:"+input.EventID, "none", "real response action lacks independent approval authority",
			now, false, map[string]interface{}{
				"mode": "blocked", "validated": false, "external_effect_applied": false,
				"action": input.Action, "target": input.Target,
			}), nil
	}
	if projection.executor == nil {
		return newAlertResponseSyntheticOutcome(input, "blocked_external_executor", "unconfigured",
			"blocked:"+input.EventID, "none", "real response action requires a configured external executor",
			now, false, map[string]interface{}{
				"mode": "blocked", "validated": false, "external_effect_applied": false,
				"action": input.Action, "target": input.Target,
			}), nil
	}
	command := AlertResponseExecutionCommand{
		EventID: input.EventID, JobID: input.JobID, TenantID: input.TenantID, AlertID: input.AlertID,
		ActionID: input.ActionID, Action: input.Action, Target: input.Target, Reason: input.Reason,
		RequestedBy: input.RequestedBy, ApprovedBy: input.ApprovedBy, ApprovalReason: input.ApprovalReason,
		TraceID: input.TraceID, AggregateVersion: input.AggregateVersion,
		IdempotencyKey: "alert-response:" + input.EventID,
	}
	receipt, executeErr := projection.executor.ExecuteAlertResponse(ctx, command)
	authorityResult := map[string]interface{}{"attempted": false, "state": "not_required", "recovered_receipt": false}
	if executeErr != nil {
		var recovered bool
		receipt, authorityResult, recovered = projection.reconcileExecutionAuthority(ctx, command)
		if !recovered {
			receipt = AlertResponseExecutionReceipt{
				Status: "partial", Provider: "alert-response-executor",
				ProviderReceiptID: "transport-unknown:" + input.EventID,
				EffectState:       "unknown", EffectIDs: []string{}, Result: map[string]interface{}{},
				ErrorCode: "EXECUTOR_EFFECT_UNKNOWN", ErrorMessage: truncateAlertResponseError(executeErr.Error()),
				ExecutedAt: now,
			}
		}
	}
	receipt = normalizeAlertResponseExecutionReceipt(receipt)
	if err := validateAlertResponseExecutionReceipt(receipt); err != nil {
		return alertResponseExecutionOutcome{}, err
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		return alertResponseExecutionOutcome{}, fmt.Errorf("marshal alert response provider receipt: %w", err)
	}
	digest := sha256.Sum256(receiptJSON)
	result := map[string]interface{}{
		"mode": "external", "provider": receipt.Provider,
		"provider_receipt_id": receipt.ProviderReceiptID, "effect_state": receipt.EffectState,
		"effect_ids": receipt.EffectIDs, "result": receipt.Result,
		"error_code": receipt.ErrorCode, "receipt_sha256": hex.EncodeToString(digest[:]),
		"authority_lookup": authorityResult,
	}
	return alertResponseExecutionOutcome{
		State: receipt.Status, ExternalEffect: receipt.EffectState == "confirmed",
		Provider: receipt.Provider, ProviderReceiptID: receipt.ProviderReceiptID,
		EffectState: receipt.EffectState, EffectIDs: receipt.EffectIDs, Result: result,
		ErrorMessage: receipt.ErrorMessage, ReceiptSHA256: hex.EncodeToString(digest[:]),
		AuthorityLookup: authorityResult, ExecutedAt: receipt.ExecutedAt.UTC(), AuditRequired: true,
	}, nil
}

func (projection *PostgresAlertResponseProjection) reconcileExecutionAuthority(
	ctx context.Context,
	command AlertResponseExecutionCommand,
) (AlertResponseExecutionReceipt, map[string]interface{}, bool) {
	resolution := map[string]interface{}{
		"attempted": false, "state": "unavailable", "recovered_receipt": false,
	}
	authority, ok := projection.executor.(AlertResponseExecutionAuthority)
	if !ok {
		return AlertResponseExecutionReceipt{}, resolution, false
	}
	lookup, err := authority.LookupAlertResponseExecution(ctx, command)
	if errors.Is(err, errAlertResponseAuthorityLookupNotConfigured) {
		return AlertResponseExecutionReceipt{}, resolution, false
	}
	resolution["attempted"] = true
	if err != nil {
		resolution["state"] = "lookup_failed"
		resolution["error_code"] = "EXECUTOR_AUTHORITY_LOOKUP_FAILED"
		return AlertResponseExecutionReceipt{}, resolution, false
	}
	lookup = normalizeAlertResponseExecutionAuthorityLookup(lookup)
	if err := validateAlertResponseExecutionAuthorityLookup(command, lookup); err != nil {
		resolution["state"] = "invalid_authority_response"
		resolution["error_code"] = "EXECUTOR_AUTHORITY_INVALID"
		return AlertResponseExecutionReceipt{}, resolution, false
	}
	resolution["state"] = lookup.State
	resolution["provider"] = lookup.Provider
	resolution["checked_at"] = lookup.CheckedAt.UTC().Format(time.RFC3339Nano)
	if lookup.State != "receipt_found" || lookup.Receipt == nil {
		return AlertResponseExecutionReceipt{}, resolution, false
	}
	resolution["recovered_receipt"] = true
	return *lookup.Receipt, resolution, true
}

func newAlertResponseSyntheticOutcome(
	input AlertResponseProjectionInput,
	state, provider, receiptID, effectState, errorMessage string,
	executedAt time.Time,
	auditRequired bool,
	result map[string]interface{},
) alertResponseExecutionOutcome {
	receipt := map[string]interface{}{
		"state": state, "provider": provider, "provider_receipt_id": receiptID,
		"effect_state": effectState, "effect_ids": []string{}, "result": result,
		"error": errorMessage, "executed_at": executedAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, _ := json.Marshal(receipt)
	digest := sha256.Sum256(encoded)
	result["provider"] = provider
	result["provider_receipt_id"] = receiptID
	result["effect_state"] = effectState
	result["receipt_sha256"] = hex.EncodeToString(digest[:])
	return alertResponseExecutionOutcome{
		State: state, ExternalEffect: false, Provider: provider, ProviderReceiptID: receiptID,
		EffectState: effectState, EffectIDs: []string{}, Result: result, ErrorMessage: errorMessage,
		ReceiptSHA256: hex.EncodeToString(digest[:]), AuthorityLookup: map[string]interface{}{},
		ExecutedAt: executedAt, AuditRequired: auditRequired,
	}
}

func truncateAlertResponseError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	return message
}
