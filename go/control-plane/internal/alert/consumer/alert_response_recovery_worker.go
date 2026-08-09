package consumer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// AlertResponseRecoveryConfig keeps every recovery path bounded. In
// particular, an expired compensation lease changes the path to authority
// lookup only; it never permits a second inverse-operation call.
type AlertResponseRecoveryConfig struct {
	ExecutionEnabled    bool
	CompensationEnabled bool
	Interval            time.Duration
	Lease               time.Duration
	RequestTimeout      time.Duration
	RetryBase           time.Duration
	BatchSize           int
	WorkerID            string
}

type AlertResponseRecoveryWorker struct {
	db                    *sql.DB
	executionAuthority    AlertResponseExecutionAuthority
	compensator           AlertResponseCompensator
	compensationAuthority AlertResponseCompensationAuthority
	config                AlertResponseRecoveryConfig
	logger                *zap.Logger
}

func NewAlertResponseRecoveryWorker(
	db *sql.DB,
	executionAuthority AlertResponseExecutionAuthority,
	compensator AlertResponseCompensator,
	compensationAuthority AlertResponseCompensationAuthority,
	config AlertResponseRecoveryConfig,
	logger *zap.Logger,
) (*AlertResponseRecoveryWorker, error) {
	if db == nil {
		return nil, fmt.Errorf("alert response recovery database is required")
	}
	if !config.ExecutionEnabled && !config.CompensationEnabled {
		return nil, fmt.Errorf("at least one alert response recovery path must be enabled")
	}
	if config.ExecutionEnabled && executionAuthority == nil {
		return nil, fmt.Errorf("alert response execution authority is required")
	}
	if config.CompensationEnabled && (compensator == nil || compensationAuthority == nil) {
		return nil, fmt.Errorf("alert response compensator and compensation authority are required")
	}
	if config.Interval <= 0 || config.Interval > time.Hour {
		config.Interval = 5 * time.Second
	}
	if config.Lease <= 0 || config.Lease > time.Hour {
		config.Lease = 45 * time.Second
	}
	if config.RequestTimeout <= 0 || config.RequestTimeout > 10*time.Minute {
		config.RequestTimeout = 30 * time.Second
	}
	if config.Lease <= config.RequestTimeout {
		return nil, fmt.Errorf("alert response recovery lease must exceed one provider request timeout")
	}
	if config.RetryBase <= 0 || config.RetryBase > time.Hour {
		config.RetryBase = 15 * time.Second
	}
	if config.BatchSize < 1 || config.BatchSize > 1000 {
		config.BatchSize = 25
	}
	if strings.TrimSpace(config.WorkerID) == "" {
		config.WorkerID = "alert-response-recovery-" + uuid.NewString()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AlertResponseRecoveryWorker{
		db: db, executionAuthority: executionAuthority, compensator: compensator,
		compensationAuthority: compensationAuthority, config: config, logger: logger,
	}, nil
}

func (worker *AlertResponseRecoveryWorker) VerifySchema(ctx context.Context) error {
	var columns int
	if err := worker.db.QueryRowContext(ctx, `SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema=current_schema() AND (
		  (table_name='alert_response_compensation_attempts' AND column_name IN
		    ('request_id','event_id','job_id','tenant_id','original_effect_ids','provider_idempotency_key',
		     'status','attempts','max_attempts','next_attempt_at','locked_until','locked_by'))
		  OR (table_name='alert_response_compensation_receipts' AND column_name IN
		    ('request_id','provider_receipt_id','state','effect_state','receipt_sha256','authority_lookup'))
		  OR (table_name='alert_response_authority_check_history' AND column_name IN
		    ('subject_type','subject_id','attempt','authority_state','detail','checked_at'))
		)`).Scan(&columns); err != nil {
		return fmt.Errorf("verify alert response recovery schema: %w", err)
	}
	if columns != 24 {
		return fmt.Errorf("alert response recovery schema is incomplete: columns=%d want=24", columns)
	}
	return nil
}

func (worker *AlertResponseRecoveryWorker) Start(ctx context.Context) error {
	if _, err := worker.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
		worker.logger.Error("Alert response recovery pass failed", zap.Error(err))
	}
	ticker := time.NewTicker(worker.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := worker.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				worker.logger.Error("Alert response recovery pass failed", zap.Error(err))
			}
		}
	}
}

func (worker *AlertResponseRecoveryWorker) RunOnce(ctx context.Context) (int, error) {
	processed := 0
	if worker.config.ExecutionEnabled {
		for processed < worker.config.BatchSize {
			claim, found, err := worker.claimExecutionRecheck(ctx)
			if err != nil {
				return processed, err
			}
			if !found {
				break
			}
			if err := worker.resolveExecutionRecheck(ctx, claim); err != nil {
				return processed, err
			}
			processed++
		}
	}
	if worker.config.CompensationEnabled {
		compensations := 0
		for compensations < worker.config.BatchSize {
			claim, found, err := worker.claimCompensation(ctx)
			if err != nil {
				return processed, err
			}
			if !found {
				break
			}
			if err := worker.resolveCompensation(ctx, claim); err != nil {
				return processed, err
			}
			processed++
			compensations++
		}
	}
	return processed, nil
}

type alertResponseExecutionRecheckClaim struct {
	RecheckID  string
	Attempt    int
	MaxAttempt int
	Command    AlertResponseExecutionCommand
}

func (worker *AlertResponseRecoveryWorker) claimExecutionRecheck(
	ctx context.Context,
) (alertResponseExecutionRecheckClaim, bool, error) {
	var claim alertResponseExecutionRecheckClaim
	tx, err := worker.db.BeginTx(ctx, nil)
	if err != nil {
		return claim, false, fmt.Errorf("begin alert response execution recheck claim: %w", err)
	}
	defer tx.Rollback()
	var attempts int
	err = tx.QueryRowContext(ctx, `SELECT q.recheck_id::text,q.attempts,q.max_attempts,
		a.event_id::text,a.job_id,a.tenant_id,a.alert_id,a.action_id,a.action,a.target,a.reason,
		a.requested_by,a.approved_by,
		COALESCE((SELECT approval.reason FROM alert_response_approvals approval
		  WHERE approval.job_id=a.job_id AND approval.tenant_id=a.tenant_id
		    AND approval.decision='approve' AND approval.resulting_revision=r.aggregate_version
		  ORDER BY approval.created_at DESC LIMIT 1),''),
		r.trace_id,r.aggregate_version
		FROM alert_response_execution_authority_rechecks q
		JOIN alert_response_execution_receipts r ON r.event_id=q.event_id
		JOIN alert_response_actions a ON a.job_id=q.job_id AND a.event_id=q.event_id
		WHERE q.status IN ('pending','checking') AND q.next_attempt_at<=now()
		  AND (q.locked_until IS NULL OR q.locked_until<now())
		  AND r.state='partial' AND r.effect_state='unknown' AND a.status='partial'
		ORDER BY q.next_attempt_at,q.recheck_id
		FOR UPDATE OF q SKIP LOCKED LIMIT 1`).Scan(
		&claim.RecheckID, &attempts, &claim.MaxAttempt,
		&claim.Command.EventID, &claim.Command.JobID, &claim.Command.TenantID,
		&claim.Command.AlertID, &claim.Command.ActionID, &claim.Command.Action,
		&claim.Command.Target, &claim.Command.Reason, &claim.Command.RequestedBy,
		&claim.Command.ApprovedBy, &claim.Command.ApprovalReason, &claim.Command.TraceID,
		&claim.Command.AggregateVersion,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return claim, false, nil
	}
	if err != nil {
		return claim, false, fmt.Errorf("claim alert response execution authority recheck: %w", err)
	}
	claim.Attempt = attempts + 1
	claim.Command.IdempotencyKey = "alert-response:" + claim.Command.EventID
	if err := validateAlertResponseExecutionCommand(claim.Command); err != nil {
		return claim, false, fmt.Errorf("reconstruct alert response execution command: %w", err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE alert_response_execution_authority_rechecks
		SET status='checking',attempts=$1,locked_until=now()+$2::interval,locked_by=$3,updated_at=now()
		WHERE recheck_id=$4::uuid AND attempts=$5`,
		claim.Attempt, postgresInterval(worker.config.Lease), worker.config.WorkerID,
		claim.RecheckID, attempts,
	)
	if err != nil {
		return claim, false, fmt.Errorf("lease alert response execution authority recheck: %w", err)
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		return claim, false, fmt.Errorf("alert response execution authority recheck lease conflict")
	}
	if err := tx.Commit(); err != nil {
		return claim, false, fmt.Errorf("commit alert response execution recheck lease: %w", err)
	}
	return claim, true, nil
}

type alertResponseAuthorityResult struct {
	State        string
	Provider     string
	ErrorCode    string
	ErrorMessage string
	CheckedAt    time.Time
	Detail       map[string]interface{}
	Receipt      *AlertResponseExecutionReceipt
}

func (worker *AlertResponseRecoveryWorker) resolveExecutionRecheck(
	ctx context.Context,
	claim alertResponseExecutionRecheckClaim,
) error {
	requestCtx, cancel := context.WithTimeout(ctx, worker.config.RequestTimeout)
	lookup, lookupErr := worker.executionAuthority.LookupAlertResponseExecution(requestCtx, claim.Command)
	cancel()
	now := time.Now().UTC()
	resolution := alertResponseAuthorityResult{
		State: "lookup_failed", ErrorCode: "EXECUTOR_AUTHORITY_LOOKUP_FAILED",
		CheckedAt: now, Detail: map[string]interface{}{"source": "periodic_authority_recheck"},
	}
	if lookupErr != nil {
		resolution.ErrorMessage = truncateAlertResponseError(lookupErr.Error())
	} else {
		lookup = normalizeAlertResponseExecutionAuthorityLookup(lookup)
		if err := validateAlertResponseExecutionAuthorityLookup(claim.Command, lookup); err != nil {
			resolution.State = "invalid_authority_response"
			resolution.ErrorCode = "EXECUTOR_AUTHORITY_INVALID"
			resolution.ErrorMessage = truncateAlertResponseError(err.Error())
		} else {
			resolution.State = lookup.State
			resolution.Provider = lookup.Provider
			resolution.CheckedAt = lookup.CheckedAt.UTC()
			resolution.Detail["provider"] = lookup.Provider
			if lookup.State != "receipt_found" {
				resolution.ErrorCode = "EXECUTOR_EFFECT_STILL_UNKNOWN"
				resolution.ErrorMessage = "provider authority state is " + lookup.State
			}
			if lookup.Receipt != nil {
				resolution.Detail["receipt"] = lookup.Receipt
				if lookup.Receipt.EffectState != "unknown" {
					resolution.Receipt = lookup.Receipt
				} else {
					resolution.State = "receipt_effect_unknown"
					resolution.ErrorCode = "EXECUTOR_EFFECT_STILL_UNKNOWN"
					resolution.ErrorMessage = "provider authority receipt still reports an unknown effect"
				}
			}
		}
	}
	return worker.persistExecutionResolution(ctx, claim, resolution)
}

func (worker *AlertResponseRecoveryWorker) persistExecutionResolution(
	ctx context.Context,
	claim alertResponseExecutionRecheckClaim,
	resolution alertResponseAuthorityResult,
) error {
	tx, err := worker.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alert response execution resolution: %w", err)
	}
	defer tx.Rollback()
	detailJSON, _ := json.Marshal(resolution.Detail)
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_response_authority_check_history
		(subject_type,subject_id,attempt,tenant_id,job_id,trace_id,provider,authority_state,
		 error_code,error_message,detail,checked_at)
		VALUES ('execution',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)`,
		claim.RecheckID, claim.Attempt, claim.Command.TenantID, claim.Command.JobID,
		claim.Command.TraceID, resolution.Provider, resolution.State, resolution.ErrorCode,
		resolution.ErrorMessage, string(detailJSON), resolution.CheckedAt,
	); err != nil {
		return fmt.Errorf("record alert response execution authority history: %w", err)
	}
	if resolution.Receipt == nil {
		return worker.rescheduleExecutionResolution(ctx, tx, claim, resolution)
	}
	receipt := normalizeAlertResponseExecutionReceipt(*resolution.Receipt)
	if err := validateAlertResponseExecutionReceipt(receipt); err != nil {
		return fmt.Errorf("validate recovered alert response execution receipt: %w", err)
	}
	receiptJSON, _ := json.Marshal(receipt)
	digest := sha256.Sum256(receiptJSON)
	digestHex := hex.EncodeToString(digest[:])
	authority := map[string]interface{}{
		"attempted": true, "state": "receipt_found", "recovered_receipt": true,
		"provider": resolution.Provider, "checked_at": resolution.CheckedAt.Format(time.RFC3339Nano),
		"recheck_id": claim.RecheckID, "attempt": claim.Attempt,
	}
	authorityJSON, _ := json.Marshal(authority)
	effectIDsJSON, _ := json.Marshal(receipt.EffectIDs)
	result := map[string]interface{}{
		"mode": "external", "provider": receipt.Provider,
		"provider_receipt_id": receipt.ProviderReceiptID, "effect_state": receipt.EffectState,
		"effect_ids": receipt.EffectIDs, "result": receipt.Result,
		"error_code": receipt.ErrorCode, "receipt_sha256": digestHex,
		"authority_lookup": authority,
	}
	resultJSON, _ := json.Marshal(result)
	externalEffect := receipt.EffectState == "confirmed"
	updated, err := tx.ExecContext(ctx, `UPDATE alert_response_execution_receipts
		SET state=$1,external_effect=$2,result=$3::jsonb,error=$4,provider=$5,
		    provider_receipt_id=$6,effect_state=$7,effect_ids=$8::jsonb,
		    receipt_sha256=$9,authority_lookup=$10::jsonb,executed_at=$11
		WHERE event_id=$12::uuid AND job_id=$13 AND tenant_id=$14
		  AND state='partial' AND effect_state='unknown'`,
		receipt.Status, externalEffect, string(resultJSON), receipt.ErrorMessage,
		receipt.Provider, receipt.ProviderReceiptID, receipt.EffectState,
		string(effectIDsJSON), digestHex, string(authorityJSON), receipt.ExecutedAt.UTC(),
		claim.Command.EventID, claim.Command.JobID, claim.Command.TenantID,
	)
	if err != nil {
		return fmt.Errorf("apply recovered alert response execution receipt: %w", err)
	}
	if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
		return fmt.Errorf("alert response execution receipt changed before authority resolution")
	}
	updated, err = tx.ExecContext(ctx, `UPDATE alert_response_actions
		SET status=$1,result=$2::jsonb,error=$3,revision=revision+1,updated_at=now()
		WHERE job_id=$4 AND event_id=$5::uuid AND tenant_id=$6
		  AND status='partial' AND revision=$7`,
		receipt.Status, string(resultJSON), receipt.ErrorMessage, claim.Command.JobID,
		claim.Command.EventID, claim.Command.TenantID, claim.Command.AggregateVersion+1,
	)
	if err != nil {
		return fmt.Errorf("apply recovered alert response authoritative state: %w", err)
	}
	if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
		return fmt.Errorf("alert response action changed before authority resolution")
	}
	updated, err = tx.ExecContext(ctx, `UPDATE alert_response_execution_authority_rechecks
		SET status='resolved',last_authority_state=$1,last_error='',locked_until=NULL,locked_by='',
		    resolved_at=$2,updated_at=now()
		WHERE recheck_id=$3::uuid AND status='checking' AND attempts=$4 AND locked_by=$5`,
		resolution.State, resolution.CheckedAt, claim.RecheckID, claim.Attempt, worker.config.WorkerID,
	)
	if err != nil {
		return fmt.Errorf("complete alert response execution authority recheck: %w", err)
	}
	if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
		return fmt.Errorf("alert response execution authority lease was lost")
	}
	auditDetail, _ := json.Marshal(map[string]interface{}{
		"recheck_id": claim.RecheckID, "attempt": claim.Attempt,
		"event_id": claim.Command.EventID, "job_id": claim.Command.JobID,
		"provider": receipt.Provider, "provider_receipt_id": receipt.ProviderReceiptID,
		"effect_state": receipt.EffectState, "effect_ids": receipt.EffectIDs,
		"receipt_sha256": digestHex, "trace_id": claim.Command.TraceID,
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,created_at)
		VALUES($1,$2,NULL,$3,'alert_response_action',$4,$5::jsonb,$6,$7,$8,$9)`,
		"audit-alert-response-authority-"+claim.Command.EventID, claim.Command.TenantID,
		"ALERT_RESPONSE_EXECUTION_AUTHORITY_RESOLVED", claim.Command.JobID,
		string(auditDetail), claim.Command.TraceID, receipt.Status,
		receipt.Status == "completed", resolution.CheckedAt,
	); err != nil {
		return fmt.Errorf("audit alert response execution authority resolution: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert response execution authority resolution: %w", err)
	}
	return nil
}

func (worker *AlertResponseRecoveryWorker) rescheduleExecutionResolution(
	ctx context.Context,
	tx *sql.Tx,
	claim alertResponseExecutionRecheckClaim,
	resolution alertResponseAuthorityResult,
) error {
	status := "pending"
	nextAttempt := time.Now().UTC().Add(worker.retryDelay(claim.Attempt))
	var resolvedAt interface{}
	if claim.Attempt >= claim.MaxAttempt {
		status = "exhausted"
		nextAttempt = time.Now().UTC()
		resolvedAt = time.Now().UTC()
	}
	updated, err := tx.ExecContext(ctx, `UPDATE alert_response_execution_authority_rechecks
		SET status=$1,next_attempt_at=$2,locked_until=NULL,locked_by='',last_authority_state=$3,
		    last_error=$4,resolved_at=$5,updated_at=now()
		WHERE recheck_id=$6::uuid AND status='checking' AND attempts=$7 AND locked_by=$8`,
		status, nextAttempt, resolution.State, resolution.ErrorMessage, resolvedAt,
		claim.RecheckID, claim.Attempt, worker.config.WorkerID,
	)
	if err != nil {
		return fmt.Errorf("reschedule alert response execution authority recheck: %w", err)
	}
	if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
		return fmt.Errorf("alert response execution authority lease was lost")
	}
	if status == "exhausted" {
		auditDetail, _ := json.Marshal(map[string]interface{}{
			"recheck_id": claim.RecheckID, "attempts": claim.Attempt,
			"last_authority_state": resolution.State, "last_error": resolution.ErrorMessage,
			"event_id": claim.Command.EventID, "trace_id": claim.Command.TraceID,
		})
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
			(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,created_at)
			VALUES($1,$2,NULL,'ALERT_RESPONSE_EXECUTION_AUTHORITY_EXHAUSTED','alert_response_action',
			       $3,$4::jsonb,$5,'partial',false,now())`,
			"audit-alert-response-authority-exhausted-"+claim.Command.EventID,
			claim.Command.TenantID, claim.Command.JobID, string(auditDetail), claim.Command.TraceID,
		); err != nil {
			return fmt.Errorf("audit exhausted alert response execution authority recheck: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert response execution authority reschedule: %w", err)
	}
	return nil
}

type alertResponseCompensationClaim struct {
	RequestID       string
	Attempt         int
	MaxAttempt      int
	CallCompensator bool
	Command         AlertResponseCompensationCommand
}

func (worker *AlertResponseRecoveryWorker) claimCompensation(
	ctx context.Context,
) (alertResponseCompensationClaim, bool, error) {
	var claim alertResponseCompensationClaim
	tx, err := worker.db.BeginTx(ctx, nil)
	if err != nil {
		return claim, false, fmt.Errorf("begin alert response compensation claim: %w", err)
	}
	defer tx.Rollback()
	var attempts int
	var previousStatus, effectIDsText string
	err = tx.QueryRowContext(ctx, `SELECT c.request_id::text,c.attempts,c.max_attempts,c.status,
		c.event_id::text,c.job_id,c.tenant_id,c.alert_id,c.original_action_id,
		c.compensation_action_id,c.original_provider,c.original_provider_receipt_id,
		c.original_effect_ids::text,c.requested_by,c.reason,c.trace_id,c.aggregate_version,
		c.provider_idempotency_key
		FROM alert_response_compensation_attempts c
		WHERE c.status IN ('pending','executing','authority_pending') AND c.next_attempt_at<=now()
		  AND (c.locked_until IS NULL OR c.locked_until<now())
		ORDER BY c.next_attempt_at,c.request_id
		FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(
		&claim.RequestID, &attempts, &claim.MaxAttempt, &previousStatus,
		&claim.Command.EventID, &claim.Command.JobID, &claim.Command.TenantID,
		&claim.Command.AlertID, &claim.Command.OriginalActionID,
		&claim.Command.CompensationActionID, &claim.Command.OriginalProvider,
		&claim.Command.OriginalProviderReceiptID, &effectIDsText,
		&claim.Command.RequestedBy, &claim.Command.Reason, &claim.Command.TraceID,
		&claim.Command.AggregateVersion, &claim.Command.IdempotencyKey,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return claim, false, nil
	}
	if err != nil {
		return claim, false, fmt.Errorf("claim alert response compensation: %w", err)
	}
	claim.Command.RequestID = claim.RequestID
	if err := json.Unmarshal([]byte(effectIDsText), &claim.Command.OriginalEffectIDs); err != nil {
		return claim, false, fmt.Errorf("decode alert response compensation effect authority: %w", err)
	}
	if err := validateAlertResponseCompensationCommand(claim.Command); err != nil {
		return claim, false, err
	}
	claim.Attempt = attempts + 1
	claim.CallCompensator = previousStatus == "pending"
	updated, err := tx.ExecContext(ctx, `UPDATE alert_response_compensation_attempts
		SET status='executing',attempts=$1,locked_until=now()+$2::interval,locked_by=$3,updated_at=now()
		WHERE request_id=$4::uuid AND attempts=$5 AND status=$6`,
		claim.Attempt, postgresInterval(worker.config.Lease), worker.config.WorkerID,
		claim.RequestID, attempts, previousStatus,
	)
	if err != nil {
		return claim, false, fmt.Errorf("lease alert response compensation: %w", err)
	}
	if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
		return claim, false, fmt.Errorf("alert response compensation lease conflict")
	}
	if err := tx.Commit(); err != nil {
		return claim, false, fmt.Errorf("commit alert response compensation lease: %w", err)
	}
	return claim, true, nil
}

type alertResponseCompensationResolution struct {
	AuthorityState string
	Provider       string
	ErrorCode      string
	ErrorMessage   string
	CheckedAt      time.Time
	Detail         map[string]interface{}
	Receipt        *AlertResponseCompensationReceipt
}

func (worker *AlertResponseRecoveryWorker) resolveCompensation(
	ctx context.Context,
	claim alertResponseCompensationClaim,
) error {
	resolution := alertResponseCompensationResolution{
		AuthorityState: "not_required", CheckedAt: time.Now().UTC(),
		Detail: map[string]interface{}{"compensator_called": claim.CallCompensator},
	}
	if claim.CallCompensator {
		requestCtx, cancel := context.WithTimeout(ctx, worker.config.RequestTimeout)
		receipt, err := worker.compensator.CompensateAlertResponse(requestCtx, claim.Command)
		cancel()
		if err == nil {
			receipt = normalizeAlertResponseCompensationReceipt(receipt)
			if validateErr := validateAlertResponseCompensationReceipt(claim.Command, receipt); validateErr == nil &&
				(receipt.Status == "compensated" || receipt.Status == "failed") {
				resolution.Provider = receipt.Provider
				resolution.CheckedAt = receipt.CompensatedAt.UTC()
				resolution.Receipt = &receipt
				resolution.Detail["direct_receipt"] = receipt
				return worker.persistCompensationResolution(ctx, claim, resolution)
			} else if validateErr != nil {
				resolution.Detail["compensator_error"] = truncateAlertResponseError(validateErr.Error())
			} else {
				resolution.Detail["direct_receipt"] = receipt
			}
		} else {
			resolution.Detail["compensator_error"] = truncateAlertResponseError(err.Error())
		}
	}

	requestCtx, cancel := context.WithTimeout(ctx, worker.config.RequestTimeout)
	lookup, lookupErr := worker.compensationAuthority.LookupAlertResponseCompensation(requestCtx, claim.Command)
	cancel()
	resolution.AuthorityState = "lookup_failed"
	resolution.ErrorCode = "COMPENSATION_AUTHORITY_LOOKUP_FAILED"
	resolution.CheckedAt = time.Now().UTC()
	if lookupErr != nil {
		resolution.ErrorMessage = truncateAlertResponseError(lookupErr.Error())
	} else {
		lookup = normalizeAlertResponseCompensationAuthorityLookup(lookup)
		if err := validateAlertResponseCompensationAuthorityLookup(claim.Command, lookup); err != nil {
			resolution.AuthorityState = "invalid_authority_response"
			resolution.ErrorCode = "COMPENSATION_AUTHORITY_INVALID"
			resolution.ErrorMessage = truncateAlertResponseError(err.Error())
		} else {
			resolution.AuthorityState = lookup.State
			resolution.Provider = lookup.Provider
			resolution.CheckedAt = lookup.CheckedAt.UTC()
			resolution.Detail["authority_provider"] = lookup.Provider
			if lookup.State != "receipt_found" {
				resolution.ErrorCode = "COMPENSATION_EFFECT_STILL_UNKNOWN"
				resolution.ErrorMessage = "provider compensation authority state is " + lookup.State
			}
			if lookup.Receipt != nil {
				resolution.Detail["authority_receipt"] = lookup.Receipt
				if lookup.Receipt.Status == "compensated" || lookup.Receipt.Status == "failed" {
					resolution.Receipt = lookup.Receipt
				} else {
					resolution.AuthorityState = "receipt_effect_unknown"
					resolution.ErrorCode = "COMPENSATION_EFFECT_STILL_UNKNOWN"
					resolution.ErrorMessage = "provider authority receipt does not prove a terminal inverse effect"
				}
			}
		}
	}
	return worker.persistCompensationResolution(ctx, claim, resolution)
}

func (worker *AlertResponseRecoveryWorker) persistCompensationResolution(
	ctx context.Context,
	claim alertResponseCompensationClaim,
	resolution alertResponseCompensationResolution,
) error {
	tx, err := worker.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alert response compensation resolution: %w", err)
	}
	defer tx.Rollback()
	detailJSON, _ := json.Marshal(resolution.Detail)
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_response_authority_check_history
		(subject_type,subject_id,attempt,tenant_id,job_id,trace_id,provider,authority_state,
		 error_code,error_message,detail,checked_at)
		VALUES ('compensation',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11)`,
		claim.RequestID, claim.Attempt, claim.Command.TenantID, claim.Command.JobID,
		claim.Command.TraceID, resolution.Provider, resolution.AuthorityState,
		resolution.ErrorCode, resolution.ErrorMessage, string(detailJSON), resolution.CheckedAt,
	); err != nil {
		return fmt.Errorf("record alert response compensation authority history: %w", err)
	}
	if resolution.Receipt == nil {
		return worker.rescheduleCompensation(ctx, tx, claim, resolution)
	}
	receipt := normalizeAlertResponseCompensationReceipt(*resolution.Receipt)
	if err := validateAlertResponseCompensationReceipt(claim.Command, receipt); err != nil {
		return fmt.Errorf("validate alert response compensation receipt: %w", err)
	}
	if receipt.Status != "compensated" && receipt.Status != "failed" {
		return fmt.Errorf("alert response compensation is not terminal")
	}
	receiptJSON, _ := json.Marshal(receipt)
	digest := sha256.Sum256(receiptJSON)
	digestHex := hex.EncodeToString(digest[:])
	effectIDsJSON, _ := json.Marshal(receipt.CompensatedEffectIDs)
	authority := map[string]interface{}{
		"state": resolution.AuthorityState, "provider": resolution.Provider,
		"checked_at": resolution.CheckedAt.Format(time.RFC3339Nano), "attempt": claim.Attempt,
	}
	authorityJSON, _ := json.Marshal(authority)
	if _, err := tx.ExecContext(ctx, `INSERT INTO alert_response_compensation_receipts
		(request_id,event_id,job_id,tenant_id,provider,provider_receipt_id,state,effect_state,
		 compensated_effect_ids,result,error_code,error_message,receipt_sha256,authority_lookup,
		 trace_id,compensated_at)
		VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11,$12,$13,$14::jsonb,$15,$16)`,
		claim.RequestID, claim.Command.EventID, claim.Command.JobID, claim.Command.TenantID,
		receipt.Provider, receipt.ProviderReceiptID, receipt.Status, receipt.EffectState,
		string(effectIDsJSON), mustAlertResponseJSON(receipt.Result), receipt.ErrorCode,
		receipt.ErrorMessage, digestHex, string(authorityJSON), claim.Command.TraceID,
		receipt.CompensatedAt.UTC(),
	); err != nil {
		return fmt.Errorf("insert alert response compensation receipt: %w", err)
	}
	actionStatus := "compensated"
	if receipt.Status == "failed" {
		actionStatus = "compensation_failed"
	}
	actionResult := map[string]interface{}{
		"mode": "external_compensation", "request_id": claim.RequestID,
		"provider": receipt.Provider, "provider_receipt_id": receipt.ProviderReceiptID,
		"effect_state": receipt.EffectState, "compensated_effect_ids": receipt.CompensatedEffectIDs,
		"result": receipt.Result, "error_code": receipt.ErrorCode,
		"receipt_sha256": digestHex, "authority_lookup": authority,
	}
	updated, err := tx.ExecContext(ctx, `UPDATE alert_response_actions
		SET status=$1,result=$2::jsonb,error=$3,revision=revision+1,updated_at=now()
		WHERE job_id=$4 AND event_id=$5::uuid AND tenant_id=$6
		  AND status='compensation_queued' AND revision=$7`,
		actionStatus, mustAlertResponseJSON(actionResult), receipt.ErrorMessage,
		claim.Command.JobID, claim.Command.EventID, claim.Command.TenantID,
		claim.Command.AggregateVersion,
	)
	if err != nil {
		return fmt.Errorf("apply alert response compensation authoritative state: %w", err)
	}
	if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
		return fmt.Errorf("alert response action changed before compensation resolution")
	}
	updated, err = tx.ExecContext(ctx, `UPDATE alert_response_compensation_attempts
		SET status=$1,last_authority_state=$2,last_error=$3,locked_until=NULL,locked_by='',
		    completed_at=$4,updated_at=now()
		WHERE request_id=$5::uuid AND status='executing' AND attempts=$6 AND locked_by=$7`,
		receipt.Status, resolution.AuthorityState, receipt.ErrorMessage, receipt.CompensatedAt.UTC(),
		claim.RequestID, claim.Attempt, worker.config.WorkerID,
	)
	if err != nil {
		return fmt.Errorf("complete alert response compensation attempt: %w", err)
	}
	if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
		return fmt.Errorf("alert response compensation lease was lost")
	}
	auditDetail, _ := json.Marshal(map[string]interface{}{
		"request_id": claim.RequestID, "event_id": claim.Command.EventID,
		"attempt": claim.Attempt, "provider": receipt.Provider,
		"provider_receipt_id": receipt.ProviderReceiptID, "effect_state": receipt.EffectState,
		"compensated_effect_ids": receipt.CompensatedEffectIDs, "receipt_sha256": digestHex,
		"trace_id": claim.Command.TraceID,
	})
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
		(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,created_at)
		VALUES($1,$2,NULL,$3,'alert_response_action',$4,$5::jsonb,$6,$7,$8,$9)`,
		"audit-alert-response-compensation-"+claim.RequestID, claim.Command.TenantID,
		"ALERT_RESPONSE_COMPENSATION_"+strings.ToUpper(receipt.Status), claim.Command.JobID,
		string(auditDetail), claim.Command.TraceID, actionStatus, receipt.Status == "compensated",
		receipt.CompensatedAt.UTC(),
	); err != nil {
		return fmt.Errorf("audit alert response compensation resolution: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert response compensation resolution: %w", err)
	}
	return nil
}

func (worker *AlertResponseRecoveryWorker) rescheduleCompensation(
	ctx context.Context,
	tx *sql.Tx,
	claim alertResponseCompensationClaim,
	resolution alertResponseCompensationResolution,
) error {
	status := "authority_pending"
	nextAttempt := time.Now().UTC().Add(worker.retryDelay(claim.Attempt))
	var completedAt interface{}
	if claim.Attempt >= claim.MaxAttempt {
		status = "exhausted_unknown"
		nextAttempt = time.Now().UTC()
		completedAt = time.Now().UTC()
	}
	updated, err := tx.ExecContext(ctx, `UPDATE alert_response_compensation_attempts
		SET status=$1,next_attempt_at=$2,locked_until=NULL,locked_by='',last_authority_state=$3,
		    last_error=$4,completed_at=$5,updated_at=now()
		WHERE request_id=$6::uuid AND status='executing' AND attempts=$7 AND locked_by=$8`,
		status, nextAttempt, resolution.AuthorityState, resolution.ErrorMessage, completedAt,
		claim.RequestID, claim.Attempt, worker.config.WorkerID,
	)
	if err != nil {
		return fmt.Errorf("reschedule alert response compensation authority lookup: %w", err)
	}
	if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
		return fmt.Errorf("alert response compensation lease was lost")
	}
	if status == "exhausted_unknown" {
		updated, err = tx.ExecContext(ctx, `UPDATE alert_response_actions
			SET status='compensation_partial',error=$1,revision=revision+1,updated_at=now()
			WHERE job_id=$2 AND event_id=$3::uuid AND tenant_id=$4
			  AND status='compensation_queued' AND revision=$5`,
			"compensation effect remains unknown after bounded authority reconciliation: "+resolution.ErrorMessage,
			claim.Command.JobID, claim.Command.EventID, claim.Command.TenantID,
			claim.Command.AggregateVersion,
		)
		if err != nil {
			return fmt.Errorf("mark exhausted alert response compensation: %w", err)
		}
		if affected, rowsErr := updated.RowsAffected(); rowsErr != nil || affected != 1 {
			return fmt.Errorf("alert response action changed before compensation exhaustion")
		}
		auditDetail, _ := json.Marshal(map[string]interface{}{
			"request_id": claim.RequestID, "attempts": claim.Attempt,
			"last_authority_state": resolution.AuthorityState,
			"last_error":           resolution.ErrorMessage, "event_id": claim.Command.EventID,
		})
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
			(event_id,tenant_id,user_id,action,object_type,object_id,detail,trace_id,result,success,created_at)
			VALUES($1,$2,NULL,'ALERT_RESPONSE_COMPENSATION_EXHAUSTED_UNKNOWN','alert_response_action',
			       $3,$4::jsonb,$5,'compensation_partial',false,now())`,
			"audit-alert-response-compensation-exhausted-"+claim.RequestID,
			claim.Command.TenantID, claim.Command.JobID, string(auditDetail), claim.Command.TraceID,
		); err != nil {
			return fmt.Errorf("audit exhausted alert response compensation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert response compensation reschedule: %w", err)
	}
	return nil
}

func (worker *AlertResponseRecoveryWorker) retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	delay := worker.config.RetryBase * time.Duration(1<<(attempt-1))
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func postgresInterval(duration time.Duration) string {
	return fmt.Sprintf("%f seconds", duration.Seconds())
}

func mustAlertResponseJSON(value interface{}) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
