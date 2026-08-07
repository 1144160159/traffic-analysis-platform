package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const dashboardTaskCompensationAction = "dashboard-task-compensate"

var (
	errDashboardTaskCompensationState    = errors.New("dashboard task is not eligible for compensation")
	errDashboardTaskCompensationRevision = errors.New("dashboard task compensation revision conflict")
)

type DashboardTaskCompensationCreateRequest struct {
	ActionID         string `json:"action_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type DashboardTaskCompensationAccepted struct {
	TaskID         string `json:"task_id"`
	JobID          string `json:"job_id"`
	EventID        string `json:"event_id"`
	ActionID       string `json:"action_id"`
	Status         string `json:"status"`
	Revision       int64  `json:"revision"`
	SnapshotID     string `json:"snapshot_id"`
	TraceID        string `json:"trace_id"`
	IdempotencyKey string `json:"idempotency_key"`
	OutboxStatus   string `json:"outbox_status"`
	Replayed       bool   `json:"replayed"`
}

type dashboardTaskCompensationCommand struct {
	TenantID, ActorID, TaskID, IdempotencyKey, TraceID, SourceIP, UserAgent string
	Request                                                                 DashboardTaskCompensationCreateRequest
}

func (h *DashboardTaskHandler) Compensate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.enabled || !h.compensationEnabled {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "FEATURE_DISABLED", "dashboard task compensation v1 is disabled")
		return
	}
	if h.db == nil {
		h.writeError(w, ctx, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "PostgreSQL is required for dashboard task compensation")
		return
	}
	tenantID, actorID, ok := authenticatedDashboardIdentity(ctx)
	if !ok {
		h.writeError(w, ctx, http.StatusUnauthorized, "UNAUTHENTICATED", "authenticated tenant and user are required")
		return
	}
	if !hasSystemPermission(ctx, authmodel.ScopeDashboardWrite) {
		h.writeError(w, ctx, http.StatusForbidden, "PERMISSION_DENIED", "permission denied: dashboard:write required")
		return
	}
	taskID := strings.TrimSpace(mux.Vars(r)["task_id"])
	if _, err := uuid.Parse(taskID); err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_TASK_ID", "task_id must be a UUID")
		return
	}
	var request DashboardTaskCompensationCreateRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid dashboard task compensation request")
		return
	}
	request.ActionID = strings.TrimSpace(request.ActionID)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ActionID != dashboardTaskCompensationAction || request.ExpectedRevision <= 0 || len(request.Reason) < 8 || len(request.Reason) > 2000 {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "action_id, expected_revision and a reason of 8 to 2000 characters are required")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		h.writeError(w, ctx, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 16 to 200 characters")
		return
	}
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = uuid.NewString()
	}
	receipt, err := h.createCompensation(ctx, dashboardTaskCompensationCommand{
		TenantID: tenantID, ActorID: actorID, TaskID: taskID, IdempotencyKey: idempotencyKey,
		TraceID: traceID, SourceIP: requestSourceIP(r), UserAgent: r.UserAgent(), Request: request,
	})
	if err != nil {
		switch {
		case errors.Is(err, errDashboardTaskNotFound):
			h.writeError(w, ctx, http.StatusNotFound, "NOT_FOUND", "dashboard task not found")
		case errors.Is(err, errDashboardTaskIdempotency):
			h.writeError(w, ctx, http.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error())
		case errors.Is(err, errDashboardTaskCompensationRevision):
			h.writeError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", err.Error())
		case errors.Is(err, errDashboardTaskCompensationState):
			h.writeError(w, ctx, http.StatusConflict, "TASK_STATE_CONFLICT", err.Error())
		case errors.Is(err, errDashboardTaskSchemaMissing):
			h.writeError(w, ctx, http.StatusServiceUnavailable, "SCHEMA_UNAVAILABLE", err.Error())
		default:
			h.logger.Error("Failed to compensate dashboard task", zap.Error(err), zap.String("tenant_id", tenantID), zap.String("task_id", taskID), zap.String("trace_id", traceID))
			h.writeError(w, ctx, http.StatusInternalServerError, "INTERNAL", "failed to accept dashboard task compensation")
		}
		return
	}
	httpx.JSONContractAccepted(w, ctx, receipt, dashboardTaskContractMeta(ctx, receipt.SnapshotID, receipt.Revision))
}

func (h *DashboardTaskHandler) createCompensation(ctx context.Context, command dashboardTaskCompensationCommand) (*DashboardTaskCompensationAccepted, error) {
	normalized, err := json.Marshal(command.Request)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(normalized)
	requestSHA := hex.EncodeToString(digest[:])
	tx, err := h.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var oldSHA string
	var oldPayload []byte
	err = tx.QueryRowContext(ctx, `SELECT request_sha256,response_payload FROM dashboard_task_compensation_requests
		WHERE tenant_id=$1 AND idempotency_key=$2`, command.TenantID, command.IdempotencyKey).Scan(&oldSHA, &oldPayload)
	if err == nil {
		if oldSHA != requestSHA {
			return nil, errDashboardTaskIdempotency
		}
		var receipt DashboardTaskCompensationAccepted
		if err := json.Unmarshal(oldPayload, &receipt); err != nil {
			return nil, err
		}
		receipt.Replayed = true
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &receipt, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		if isUndefinedTable(err) {
			return nil, fmt.Errorf("%w: apply migration 202608082100", errDashboardTaskSchemaMissing)
		}
		return nil, err
	}
	var status, actionID, snapshotID string
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT status,revision,action_id,snapshot_id FROM dashboard_tasks
		WHERE tenant_id=$1 AND task_id=$2 FOR UPDATE`, command.TenantID, command.TaskID).
		Scan(&status, &revision, &actionID, &snapshotID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errDashboardTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	if revision != command.Request.ExpectedRevision {
		return nil, fmt.Errorf("%w: current revision is %d", errDashboardTaskCompensationRevision, revision)
	}
	if status != "completed" && status != "partial" {
		return nil, fmt.Errorf("%w: task status is %s", errDashboardTaskCompensationState, status)
	}
	var effectState string
	var effectIDs []byte
	if err := tx.QueryRowContext(ctx, `SELECT effect_state,effect_ids FROM dashboard_task_execution_receipts
		WHERE tenant_id=$1 AND task_id=$2`, command.TenantID, command.TaskID).Scan(&effectState, &effectIDs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: confirmed provider receipt is missing", errDashboardTaskCompensationState)
		}
		return nil, err
	}
	var ids []string
	if effectState != "confirmed" || json.Unmarshal(effectIDs, &ids) != nil || len(ids) == 0 {
		return nil, fmt.Errorf("%w: original external effects are not confirmed", errDashboardTaskCompensationState)
	}
	eventID := uuid.NewString()
	now := time.Now().UTC()
	nextRevision := revision + 1
	result, err := tx.ExecContext(ctx, `UPDATE dashboard_tasks SET status='compensating',revision=$3,
		updated_at=$4 WHERE tenant_id=$1 AND task_id=$2 AND revision=$5 AND status=$6`, command.TenantID,
		command.TaskID, nextRevision, now, revision, status)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return nil, errDashboardTaskCompensationRevision
	}
	snapshot, _ := json.Marshal(map[string]interface{}{"task_id": command.TaskID, "status": "compensating", "revision": nextRevision, "trace_id": command.TraceID})
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_history
		(event_id,tenant_id,task_id,revision,action_id,previous_status,resulting_status,actor_id,reason,trace_id,snapshot,occurred_at)
		VALUES($1,$2,$3,$4,$5,$6,'compensating',$7,$8,$9,$10::jsonb,$11)`, eventID, command.TenantID,
		command.TaskID, nextRevision, dashboardTaskCompensationAction, status, command.ActorID, command.Request.Reason,
		command.TraceID, string(snapshot), now); err != nil {
		return nil, err
	}
	payload := dashboardTaskLifecycleEnvelope{
		EventID: eventID, EventType: dashboardTaskCompensationRequestedEvent, SchemaVersion: 1,
		AggregateType: "dashboard_task", AggregateID: command.TaskID, AggregateVersion: nextRevision,
		PartitionKey: command.TenantID + ":" + command.TaskID, TenantID: command.TenantID, TaskID: command.TaskID,
		ActionID: dashboardTaskCompensationAction, Status: "compensating", SnapshotID: snapshotID,
		EffectIDs: []string{}, Result: map[string]interface{}{}, TraceID: command.TraceID, OccurredAt: now.Format(time.RFC3339Nano),
	}
	payloadJSON, _ := json.Marshal(payload)
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_outbox
		(event_id,tenant_id,task_id,aggregate_version,event_type,schema_version,partition_key,payload,trace_id,status,occurred_at)
		VALUES($1,$2,$3,$4,$5,1,$6,$7::jsonb,$8,'pending',$9)`, eventID, command.TenantID, command.TaskID,
		nextRevision, dashboardTaskCompensationRequestedEvent, command.TenantID+":"+command.TaskID, string(payloadJSON), command.TraceID, now); err != nil {
		return nil, err
	}
	receipt := DashboardTaskCompensationAccepted{TaskID: command.TaskID, JobID: command.TaskID, EventID: eventID,
		ActionID: dashboardTaskCompensationAction, Status: "compensating", Revision: nextRevision, SnapshotID: snapshotID,
		TraceID: command.TraceID, IdempotencyKey: command.IdempotencyKey, OutboxStatus: "pending"}
	receiptJSON, _ := json.Marshal(receipt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO dashboard_task_compensation_requests
		(request_event_id,tenant_id,task_id,idempotency_key,request_sha256,expected_revision,resulting_revision,
		action_id,reason,requested_by,trace_id,response_payload,created_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13)`, eventID, command.TenantID, command.TaskID,
		command.IdempotencyKey, requestSHA, revision, nextRevision, dashboardTaskCompensationAction, command.Request.Reason,
		command.ActorID, command.TraceID, string(receiptJSON), now); err != nil {
		return nil, err
	}
	if err := insertDashboardTaskPipelineAudit(ctx, tx, eventID, command.TenantID, command.ActorID,
		"DASHBOARD_TASK_COMPENSATION_REQUESTED", command.TaskID, command.TraceID, "compensating", snapshot, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &receipt, nil
}
