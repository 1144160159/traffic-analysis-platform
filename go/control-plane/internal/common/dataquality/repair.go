package dataquality

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrRepairConflict           = errors.New("data quality repair revision conflict")
	ErrRepairNotFound           = errors.New("data quality repair not found")
	ErrRepairExecutionDisabled  = errors.New("data quality repair execution is disabled")
	ErrRepairApprovalSeparation = errors.New("data quality repair requester cannot approve the same repair")
)

type RepairCreateCommand struct {
	TenantID       string                 `json:"tenant_id"`
	QualityEventID string                 `json:"quality_event_id"`
	OperationID    string                 `json:"operation_id"`
	InputScope     map[string]interface{} `json:"input_scope"`
	ResourceBudget map[string]interface{} `json:"resource_budget"`
	ActionID       string                 `json:"action_id"`
	IdempotencyKey string                 `json:"-"`
	Reason         string                 `json:"reason"`
	Actor          string                 `json:"actor"`
	TraceID        string                 `json:"-"`
}

type RepairTransitionCommand struct {
	TenantID         string                 `json:"tenant_id"`
	RepairID         string                 `json:"repair_id"`
	Action           string                 `json:"action"`
	ExpectedRevision int64                  `json:"expected_revision"`
	Summary          map[string]interface{} `json:"summary"`
	ActionID         string                 `json:"action_id"`
	IdempotencyKey   string                 `json:"-"`
	Reason           string                 `json:"reason"`
	Actor            string                 `json:"actor"`
	TraceID          string                 `json:"-"`
}

type RepairRecord struct {
	RepairID         string                 `json:"repair_id"`
	TenantID         string                 `json:"tenant_id"`
	QualityEventID   string                 `json:"quality_event_id"`
	OperationID      string                 `json:"operation_id"`
	Status           string                 `json:"status"`
	InputScope       map[string]interface{} `json:"input_scope"`
	ResourceBudget   map[string]interface{} `json:"resource_budget"`
	RepairSummary    map[string]interface{} `json:"repair_summary"`
	ReconcileSummary map[string]interface{} `json:"reconcile_summary"`
	RequestedBy      string                 `json:"requested_by"`
	ApprovedBy       string                 `json:"approved_by"`
	Reason           string                 `json:"reason"`
	Revision         int64                  `json:"revision"`
	TraceID          string                 `json:"trace_id"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	CompletedAt      *time.Time             `json:"completed_at,omitempty"`
	Replayed         bool                   `json:"replayed"`
}

func (m *Monitor) CreateRepair(ctx context.Context, command RepairCreateCommand) (*RepairRecord, error) {
	if m == nil || m.controlDB == nil {
		return nil, ErrGovernanceUnavailable
	}
	if err := validateRepairCreate(command); err != nil {
		return nil, err
	}
	requestSHA, err := commandSHA(command)
	if err != nil {
		return nil, err
	}
	repairID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("data-quality-repair:"+command.TenantID+":"+command.IdempotencyKey))
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("data-quality-repair-planned:"+command.TenantID+":"+command.IdempotencyKey))
	tx, err := m.controlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin repair create: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, command.TenantID+":"+command.IdempotencyKey); err != nil {
		return nil, fmt.Errorf("lock repair create: %w", err)
	}
	var replay RepairRecord
	if found, err := loadRepairReceipt(ctx, tx, command.TenantID, command.IdempotencyKey, requestSHA, &replay); err != nil || found {
		if found {
			replay.Replayed = true
			return &replay, nil
		}
		return nil, err
	}
	var eventStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM data_quality_events WHERE tenant_id=$1 AND quality_event_id=$2 FOR UPDATE`, command.TenantID, command.QualityEventID).Scan(&eventStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrGovernanceNotFound
		}
		return nil, fmt.Errorf("load quality event for repair: %w", err)
	}
	if eventStatus != "detected" && eventStatus != "triaged" && eventStatus != "failed" {
		return nil, fmt.Errorf("quality event in %s cannot start a repair", eventStatus)
	}
	inputJSON, _ := json.Marshal(command.InputScope)
	budgetJSON, _ := json.Marshal(command.ResourceBudget)
	row := RepairRecord{}
	var scopeStored, budgetStored, repairStored, reconcileStored []byte
	var completed sql.NullTime
	err = tx.QueryRowContext(ctx, `
		INSERT INTO data_quality_repairs (
			repair_id,tenant_id,quality_event_id,operation_id,idempotency_key,status,input_scope,
			resource_budget,requested_by,reason,revision,trace_id
		) VALUES ($1,$2,$3,$4,$5,'planned',$6::jsonb,$7::jsonb,$8,$9,1,$10)
		RETURNING repair_id::text,tenant_id,quality_event_id::text,operation_id,status,input_scope,
			resource_budget,repair_summary,reconcile_summary,requested_by,approved_by,reason,revision,
			trace_id,created_at,updated_at,completed_at
	`, repairID, command.TenantID, command.QualityEventID, command.OperationID, command.IdempotencyKey,
		string(inputJSON), string(budgetJSON), command.Actor, command.Reason, command.TraceID).Scan(
		&row.RepairID, &row.TenantID, &row.QualityEventID, &row.OperationID, &row.Status,
		&scopeStored, &budgetStored, &repairStored, &reconcileStored, &row.RequestedBy, &row.ApprovedBy,
		&row.Reason, &row.Revision, &row.TraceID, &row.CreatedAt, &row.UpdatedAt, &completed,
	)
	if err != nil {
		return nil, fmt.Errorf("insert planned data quality repair: %w", err)
	}
	decodeRepairJSON(&row, scopeStored, budgetStored, repairStored, reconcileStored, completed)
	if _, err := tx.ExecContext(ctx, `UPDATE data_quality_events SET status='repair_planned',revision=revision+1,trace_id=$3,updated_at=now() WHERE tenant_id=$1 AND quality_event_id=$2`, command.TenantID, command.QualityEventID, command.TraceID); err != nil {
		return nil, fmt.Errorf("mark quality event repair planned: %w", err)
	}
	if err := persistRepairCommand(ctx, tx, row, "planned", "none", command.Actor, command.Reason,
		command.TraceID, command.ActionID, command.IdempotencyKey, requestSHA, eventID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit planned repair: %w", err)
	}
	return &row, nil
}

// TransitionRepair advances the bounded state machine. executionEnabled must
// come from the fail-closed runtime flag; callers cannot enable it via JSON.
func (m *Monitor) TransitionRepair(ctx context.Context, command RepairTransitionCommand, executionEnabled bool) (*RepairRecord, error) {
	if m == nil || m.controlDB == nil {
		return nil, ErrGovernanceUnavailable
	}
	if err := validateRepairTransition(command); err != nil {
		return nil, err
	}
	requestSHA, err := commandSHA(command)
	if err != nil {
		return nil, err
	}
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("data-quality-repair-transition:"+command.TenantID+":"+command.IdempotencyKey))
	tx, err := m.controlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, fmt.Errorf("begin repair transition: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, command.TenantID+":"+command.IdempotencyKey); err != nil {
		return nil, fmt.Errorf("lock repair transition: %w", err)
	}
	var replay RepairRecord
	if found, err := loadRepairReceipt(ctx, tx, command.TenantID, command.IdempotencyKey, requestSHA, &replay); err != nil || found {
		if found {
			replay.Replayed = true
			return &replay, nil
		}
		return nil, err
	}
	current, err := loadRepairForUpdate(ctx, tx, command.TenantID, command.RepairID)
	if err != nil {
		return nil, err
	}
	if current.Revision != command.ExpectedRevision {
		return nil, ErrRepairConflict
	}
	next, operation, eventStatus, err := nextRepairStatus(current, command, executionEnabled)
	if err != nil {
		return nil, err
	}
	repairSummary := current.RepairSummary
	reconcileSummary := current.ReconcileSummary
	if command.Action == "complete_dry_run" || command.Action == "record_executed" || command.Action == "record_failed" {
		repairSummary = command.Summary
	}
	if command.Action == "reconcile" {
		reconcileSummary = command.Summary
	}
	repairJSON, _ := json.Marshal(repairSummary)
	reconcileJSON, _ := json.Marshal(reconcileSummary)
	approvedBy := current.ApprovedBy
	if command.Action == "approve" {
		approvedBy = command.Actor
	}
	completed := next == "reconciled" || next == "cancelled" || next == "failed"
	row := RepairRecord{}
	var scopeStored, budgetStored, repairStored, reconcileStored []byte
	var completedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		UPDATE data_quality_repairs SET status=$3,repair_summary=$4::jsonb,reconcile_summary=$5::jsonb,
			approved_by=$6,revision=revision+1,reason=$7,trace_id=$8,updated_at=now(),
			completed_at=CASE WHEN $9 THEN now() ELSE completed_at END
		WHERE tenant_id=$1 AND repair_id=$2
		RETURNING repair_id::text,tenant_id,quality_event_id::text,operation_id,status,input_scope,
			resource_budget,repair_summary,reconcile_summary,requested_by,approved_by,reason,revision,
			trace_id,created_at,updated_at,completed_at
	`, command.TenantID, command.RepairID, next, string(repairJSON), string(reconcileJSON), approvedBy,
		command.Reason, command.TraceID, completed).Scan(
		&row.RepairID, &row.TenantID, &row.QualityEventID, &row.OperationID, &row.Status,
		&scopeStored, &budgetStored, &repairStored, &reconcileStored, &row.RequestedBy, &row.ApprovedBy,
		&row.Reason, &row.Revision, &row.TraceID, &row.CreatedAt, &row.UpdatedAt, &completedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("update data quality repair: %w", err)
	}
	decodeRepairJSON(&row, scopeStored, budgetStored, repairStored, reconcileStored, completedAt)
	if eventStatus != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE data_quality_events SET status=$3,revision=revision+1,trace_id=$4,updated_at=now(),closed_at=CASE WHEN $3='closed' THEN now() ELSE closed_at END WHERE tenant_id=$1 AND quality_event_id=$2`, command.TenantID, row.QualityEventID, eventStatus, command.TraceID); err != nil {
			return nil, fmt.Errorf("advance quality event with repair: %w", err)
		}
	}
	if err := persistRepairCommand(ctx, tx, row, operation, current.Status, command.Actor, command.Reason,
		command.TraceID, command.ActionID, command.IdempotencyKey, requestSHA, eventID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit repair transition: %w", err)
	}
	return &row, nil
}

func validateRepairCreate(command RepairCreateCommand) error {
	if command.TenantID == "" || command.QualityEventID == "" || command.ActionID == "" || command.Actor == "" || command.TraceID == "" {
		return fmt.Errorf("repair tenant, quality event, action, actor and trace are required")
	}
	if _, err := uuid.Parse(command.QualityEventID); err != nil {
		return fmt.Errorf("quality event id is invalid")
	}
	if command.OperationID != "flow_replay_window_v1" {
		return fmt.Errorf("repair operation %q is not allowlisted", command.OperationID)
	}
	if err := validateRepairCommandEnvelope(command.IdempotencyKey, command.Reason); err != nil {
		return err
	}
	return validateRepairScope(command.TenantID, command.InputScope, command.ResourceBudget)
}

func validateRepairTransition(command RepairTransitionCommand) error {
	if command.TenantID == "" || command.RepairID == "" || command.Action == "" || command.ActionID == "" || command.Actor == "" || command.TraceID == "" || command.ExpectedRevision <= 0 {
		return fmt.Errorf("repair transition identity, revision, action, actor and trace are required")
	}
	if _, err := uuid.Parse(command.RepairID); err != nil {
		return fmt.Errorf("repair id is invalid")
	}
	return validateRepairCommandEnvelope(command.IdempotencyKey, command.Reason)
}

func validateRepairCommandEnvelope(key, reason string) error {
	if len(key) < 16 || len(key) > 200 || len([]rune(reason)) < 8 || len([]rune(reason)) > 1000 {
		return fmt.Errorf("idempotency key must be 16-200 characters and reason 8-1000 characters")
	}
	return nil
}

func validateRepairScope(tenantID string, scope, budget map[string]interface{}) error {
	if scope == nil || budget == nil || stringValue(scope["dataset_id"]) != "flows_raw" {
		return fmt.Errorf("repair scope must target flows_raw")
	}
	if scopedTenant := stringValue(scope["tenant_id"]); scopedTenant != "" && scopedTenant != tenantID {
		return fmt.Errorf("repair scope tenant does not match authenticated tenant")
	}
	start, startErr := time.Parse(time.RFC3339, stringValue(scope["window_start"]))
	end, endErr := time.Parse(time.RFC3339, stringValue(scope["window_end"]))
	if startErr != nil || endErr != nil || !end.After(start) || end.Sub(start) > time.Hour {
		return fmt.Errorf("repair scope requires a positive RFC3339 window of at most one hour")
	}
	maxRows := int64Value(budget["max_rows"])
	maxSeconds := int64Value(budget["max_duration_seconds"])
	if maxRows <= 0 || maxRows > 100000 || maxSeconds <= 0 || maxSeconds > 300 {
		return fmt.Errorf("repair budget exceeds max_rows=100000 or max_duration_seconds=300")
	}
	return nil
}

func nextRepairStatus(current RepairRecord, command RepairTransitionCommand, executionEnabled bool) (string, string, string, error) {
	switch command.Action {
	case "complete_dry_run":
		if current.Status != "planned" || !boolValue(command.Summary["within_budget"]) || boolValue(command.Summary["destructive"]) || int64Value(command.Summary["estimated_rows"]) < 0 {
			return "", "", "", fmt.Errorf("dry-run result is missing, destructive or outside budget")
		}
		if int64Value(command.Summary["estimated_rows"]) > int64Value(current.ResourceBudget["max_rows"]) {
			return "", "", "", fmt.Errorf("dry-run estimated rows exceed approved budget")
		}
		return "dry_run_passed", "dry_run_completed", "dry_run_passed", nil
	case "submit_approval":
		if current.Status == "dry_run_passed" {
			return "approval_pending", "approval_submitted", "", nil
		}
	case "approve":
		if current.Status == "approval_pending" {
			if current.RequestedBy == command.Actor {
				return "", "", "", ErrRepairApprovalSeparation
			}
			return "approved", "approved", "approved", nil
		}
	case "reject":
		if current.Status == "approval_pending" {
			if current.RequestedBy == command.Actor {
				return "", "", "", ErrRepairApprovalSeparation
			}
			return "cancelled", "rejected", "failed", nil
		}
	case "start_execution":
		if current.Status == "approved" {
			if !executionEnabled {
				return "", "", "", ErrRepairExecutionDisabled
			}
			return "executing", "execution_started", "replaying", nil
		}
	case "record_partial":
		if current.Status == "executing" {
			return "partial", "execution_partial", "replaying", nil
		}
	case "record_executed":
		if current.Status == "executing" {
			if !boolValue(command.Summary["published"]) || int64Value(command.Summary["published_rows"]) < 0 {
				return "", "", "", fmt.Errorf("execution result is missing a valid published row count")
			}
			if int64Value(command.Summary["published_rows"]) > int64Value(current.ResourceBudget["max_rows"]) {
				return "", "", "", fmt.Errorf("execution rows exceed approved budget")
			}
			return "executed", "execution_completed", "replaying", nil
		}
	case "record_failed":
		if current.Status == "executing" || current.Status == "partial" {
			return "failed", "execution_failed", "failed", nil
		}
	case "reconcile":
		if current.RequestedBy == command.Actor {
			return "", "", "", ErrRepairApprovalSeparation
		}
		if (current.Status == "executed" || current.Status == "partial") && boolValue(command.Summary["all_match"]) && int64Value(command.Summary["missing_count"]) == 0 && int64Value(command.Summary["extra_count"]) == 0 {
			return "reconciled", "reconciled", "reconciled", nil
		}
	case "cancel":
		if current.Status == "planned" || current.Status == "dry_run_passed" || current.Status == "approval_pending" || current.Status == "approved" {
			return "cancelled", "cancelled", "failed", nil
		}
	}
	return "", "", "", fmt.Errorf("%w: repair in %s cannot %s", ErrInvalidTransition, current.Status, command.Action)
}

func loadRepairForUpdate(ctx context.Context, tx *sql.Tx, tenantID, repairID string) (RepairRecord, error) {
	var row RepairRecord
	var scope, budget, repair, reconcile []byte
	var completed sql.NullTime
	err := tx.QueryRowContext(ctx, `
		SELECT repair_id::text,tenant_id,quality_event_id::text,operation_id,status,input_scope,
			resource_budget,repair_summary,reconcile_summary,requested_by,approved_by,reason,revision,
			trace_id,created_at,updated_at,completed_at
		FROM data_quality_repairs WHERE tenant_id=$1 AND repair_id=$2 FOR UPDATE
	`, tenantID, repairID).Scan(&row.RepairID, &row.TenantID, &row.QualityEventID, &row.OperationID,
		&row.Status, &scope, &budget, &repair, &reconcile, &row.RequestedBy, &row.ApprovedBy,
		&row.Reason, &row.Revision, &row.TraceID, &row.CreatedAt, &row.UpdatedAt, &completed)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrRepairNotFound
	}
	if err != nil {
		return row, fmt.Errorf("load data quality repair: %w", err)
	}
	decodeRepairJSON(&row, scope, budget, repair, reconcile, completed)
	return row, nil
}

func persistRepairCommand(ctx context.Context, tx *sql.Tx, row RepairRecord, operation, previousStatus, actor, reason, traceID, actionID, idempotencyKey, requestSHA string, eventID uuid.UUID) error {
	snapshot, _ := json.Marshal(row)
	if _, err := tx.ExecContext(ctx, `INSERT INTO data_quality_repair_history (event_id,tenant_id,repair_id,revision,operation,previous_status,resulting_status,actor_id,reason,trace_id,snapshot) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb)`, eventID, row.TenantID, row.RepairID, row.Revision, operation, previousStatus, row.Status, actor, reason, traceID, string(snapshot)); err != nil {
		return fmt.Errorf("insert repair history: %w", err)
	}
	eventType := "DATA_QUALITY_REPAIR_" + strings.ToUpper(operation)
	if err := insertGovernanceOutbox(ctx, tx, eventID, row.TenantID, "repair", row.RepairID, row.Revision, eventType, traceID, snapshot); err != nil {
		return err
	}
	if err := insertGovernanceAudit(ctx, tx, eventID, row.TenantID, actor, "data_quality.repair."+operation, "data_quality_repair", row.RepairID, traceID, snapshot); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO data_quality_repair_requests (tenant_id,idempotency_key,request_sha256,action_id,operation,repair_id,resulting_revision,event_id,response_payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`, row.TenantID, idempotencyKey, requestSHA, actionID, operation, row.RepairID, row.Revision, eventID, string(snapshot))
	if err != nil {
		return fmt.Errorf("insert repair command receipt: %w", err)
	}
	return nil
}

func loadRepairReceipt(ctx context.Context, tx *sql.Tx, tenantID, key, requestSHA string, target *RepairRecord) (bool, error) {
	var existingSHA string
	var response []byte
	err := tx.QueryRowContext(ctx, `SELECT request_sha256,response_payload FROM data_quality_repair_requests WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key).Scan(&existingSHA, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load repair command receipt: %w", err)
	}
	if existingSHA != requestSHA {
		return false, ErrIdempotencyConflict
	}
	if err := json.Unmarshal(response, target); err != nil {
		return false, fmt.Errorf("decode repair command receipt: %w", err)
	}
	return true, nil
}

func decodeRepairJSON(row *RepairRecord, scope, budget, repair, reconcile []byte, completed sql.NullTime) {
	_ = json.Unmarshal(scope, &row.InputScope)
	_ = json.Unmarshal(budget, &row.ResourceBudget)
	_ = json.Unmarshal(repair, &row.RepairSummary)
	_ = json.Unmarshal(reconcile, &row.ReconcileSummary)
	if completed.Valid {
		value := completed.Time
		row.CompletedAt = &value
	}
}

func stringValue(value interface{}) string {
	result, _ := value.(string)
	return result
}

func boolValue(value interface{}) bool {
	result, _ := value.(bool)
	return result
}

func int64Value(value interface{}) int64 {
	switch number := value.(type) {
	case int:
		return int64(number)
	case int64:
		return number
	case float64:
		return int64(number)
	case json.Number:
		result, _ := number.Int64()
		return result
	default:
		return 0
	}
}
