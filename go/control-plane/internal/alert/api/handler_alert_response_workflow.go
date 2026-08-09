package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type alertResponseApprovalRequest struct {
	Decision         string `json:"decision"`
	ExpectedRevision *int64 `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type alertResponseControlRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type alertResponseActionState struct {
	JobID          string
	EventID        string
	TenantID       string
	AlertID        string
	ActionID       string
	Action         string
	Target         string
	Reason         string
	RequestedBy    string
	TraceID        string
	Status         string
	ApprovalStatus string
	DryRun         bool
	Revision       int64
}

type alertResponseApprovalRecord struct {
	JobID             string
	AlertID           string
	Decision          string
	ExpectedRevision  int64
	Reason            string
	DecidedBy         string
	ResultingRevision int64
	ResultingStatus   string
	ApprovalStatus    string
}

type alertResponseControlRecord struct {
	JobID             string
	AlertID           string
	Operation         string
	ExpectedRevision  int64
	Reason            string
	RequestedBy       string
	State             string
	ResultingRevision int64
	ResultingStatus   string
}

func (h *Handler) DecideAlertResponseAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireAlertPermission(w, r, authmodel.ScopePlaybookApprove) {
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert response approval persistence is unavailable")
		return
	}
	tenantID, alertID, jobID, actor, idempotencyKey, ok := h.alertResponseWorkflowIdentity(w, r)
	if !ok {
		return
	}
	var request alertResponseApprovalRequest
	if !decodeSingleJSON(w, r, &request) {
		return
	}
	request.Decision = strings.ToLower(strings.TrimSpace(request.Decision))
	request.Reason = strings.TrimSpace(request.Reason)
	if (request.Decision != "approve" && request.Decision != "reject") ||
		request.ExpectedRevision == nil || *request.ExpectedRevision <= 0 ||
		len(request.Reason) < 8 || len(request.Reason) > 1000 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "decision approve/reject, positive expected_revision and reason (8 to 1000 characters) are required")
		return
	}

	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin alert response approval transaction")
		return
	}
	defer tx.Rollback()

	if existing, found, err := loadAlertResponseApprovalByKey(ctx, tx, tenantID, idempotencyKey); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve approval idempotency")
		return
	} else if found {
		if !alertResponseApprovalMatches(existing, alertID, jobID, actor, request) {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different approval decision")
			return
		}
		writeAlertResponseWorkflowAccepted(w, ctx, existing.JobID, existing.ResultingStatus,
			existing.ApprovalStatus, existing.ResultingRevision, true, responseActionOutboxStatus(existing.ResultingStatus, false))
		return
	}

	action, found, err := lockAlertResponseAction(ctx, tx, tenantID, alertID, jobID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock alert response action")
		return
	}
	if !found {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "alert response action not found")
		return
	}
	// A concurrent exact retry can become visible only after the action lock is
	// acquired. Re-read the immutable decision before evaluating the new state.
	if existing, replayFound, replayErr := loadAlertResponseApprovalByKey(ctx, tx, tenantID, idempotencyKey); replayErr != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to reconcile concurrent approval idempotency")
		return
	} else if replayFound {
		if !alertResponseApprovalMatches(existing, alertID, jobID, actor, request) {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different approval decision")
			return
		}
		writeAlertResponseWorkflowAccepted(w, ctx, existing.JobID, existing.ResultingStatus,
			existing.ApprovalStatus, existing.ResultingRevision, true, responseActionOutboxStatus(existing.ResultingStatus, false))
		return
	}
	if action.DryRun {
		httpx.JSONError(w, ctx, http.StatusConflict, "APPROVAL_NOT_REQUIRED", "dry-run actions do not require approval")
		return
	}
	if actor == action.RequestedBy {
		httpx.JSONError(w, ctx, http.StatusForbidden, "INDEPENDENT_APPROVER_REQUIRED", "the requester cannot approve or reject the same response action")
		return
	}
	if action.Status != "pending_approval" || action.ApprovalStatus != "pending" {
		httpx.JSONError(w, ctx, http.StatusConflict, "INVALID_STATE", "only a pending response action can be approved or rejected")
		return
	}
	if action.Revision != *request.ExpectedRevision {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", fmt.Sprintf("expected revision %d but current revision is %d", *request.ExpectedRevision, action.Revision))
		return
	}

	newRevision := action.Revision + 1
	newStatus := "approved_awaiting_executor"
	approvalStatus := "approved"
	if request.Decision == "reject" {
		newStatus = "cancelled"
		approvalStatus = "rejected"
	}
	insert, err := tx.ExecContext(ctx, `INSERT INTO alert_response_approvals
		(approval_id,job_id,tenant_id,alert_id,decision,expected_revision,idempotency_key,
		 reason,decided_by,resulting_revision,resulting_status,approval_status)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (tenant_id,idempotency_key) DO NOTHING`,
		uuid.NewString(), jobID, tenantID, alertID, request.Decision,
		*request.ExpectedRevision, idempotencyKey, request.Reason, actor,
		newRevision, newStatus, approvalStatus,
	)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist approval decision")
		return
	}
	inserted, err := insert.RowsAffected()
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to inspect approval persistence")
		return
	}
	if inserted != 1 {
		existing, found, loadErr := loadAlertResponseApprovalByKey(ctx, tx, tenantID, idempotencyKey)
		if loadErr != nil || !found {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "approval idempotency collision")
			return
		}
		if !alertResponseApprovalMatches(existing, alertID, jobID, actor, request) {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different approval decision")
			return
		}
		writeAlertResponseWorkflowAccepted(w, ctx, existing.JobID, existing.ResultingStatus,
			existing.ApprovalStatus, existing.ResultingRevision, true, responseActionOutboxStatus(existing.ResultingStatus, false))
		return
	}

	result, err := tx.ExecContext(ctx, `UPDATE alert_response_actions
		SET status=$1,approval_status=$2,revision=$3,
		    approved_by=CASE WHEN $2='approved' THEN $4 ELSE approved_by END,
		    approved_at=CASE WHEN $2='approved' THEN now() ELSE approved_at END,
		    error=CASE WHEN $2='rejected' THEN $5 ELSE '' END,
		    updated_at=now()
		WHERE tenant_id=$6 AND alert_id=$7 AND job_id=$8
		  AND status='pending_approval' AND approval_status='pending' AND revision=$9`,
		newStatus, approvalStatus, newRevision, actor, "rejected: "+request.Reason,
		tenantID, alertID, jobID, action.Revision,
	)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to apply approval decision")
		return
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "alert response action changed while approval was being applied")
		return
	}

	outboxStatus := "not_required"
	if request.Decision == "approve" {
		traceID := strings.TrimSpace(action.TraceID)
		if traceID == "" {
			traceID = action.EventID
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"event_id": action.EventID, "event_type": "alert.response.requested.v1",
			"schema_version": 1, "aggregate_version": newRevision,
			"job_id": action.JobID, "tenant_id": action.TenantID, "alert_id": action.AlertID,
			"action_id": action.ActionID, "action": action.Action,
			"target": action.Target, "reason": action.Reason,
			"requested_by": action.RequestedBy, "approved_by": actor,
			"approval_reason": request.Reason, "trace_id": traceID, "dry_run": false,
		})
		if _, err = tx.ExecContext(ctx, `INSERT INTO alert_response_outbox
			(job_id,event_id,tenant_id,event_type,schema_version,aggregate_version,partition_key,payload)
			VALUES ($1,$2::uuid,$3,'alert.response.requested.v1',1,$4,$5,$6::jsonb)`,
			action.JobID, action.EventID, action.TenantID, newRevision,
			action.TenantID+":"+action.JobID, string(payload),
		); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to enqueue approved response action")
			return
		}
		outboxStatus = "pending_retry"
	}
	auditAction := "ALERT_RESPONSE_ACTION_APPROVED"
	if request.Decision == "reject" {
		auditAction = "ALERT_RESPONSE_ACTION_REJECTED"
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: auditAction, ObjectType: "alert_response_action", ObjectID: jobID,
		TenantID: tenantID, UserID: actor, AlertID: alertID, Reason: request.Reason,
		Result: newStatus, Detail: map[string]interface{}{
			"action_id": action.ActionID, "decision": request.Decision,
			"expected_revision": action.Revision, "revision": newRevision,
			"event_id": action.EventID, "idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
		},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit approval decision")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit approval decision")
		return
	}
	writeAlertResponseWorkflowAccepted(w, ctx, jobID, newStatus, approvalStatus, newRevision, false, outboxStatus)
}

func (h *Handler) CancelAlertResponseAction(w http.ResponseWriter, r *http.Request) {
	if !h.requireAlertWritePermission(w, r) {
		return
	}
	h.applyAlertResponseControl(w, r, "cancel")
}

func (h *Handler) RequestAlertResponseCompensation(w http.ResponseWriter, r *http.Request) {
	if !h.requireAlertPermission(w, r, authmodel.ScopePlaybookApprove) {
		return
	}
	h.applyAlertResponseControl(w, r, "compensate")
}

func (h *Handler) applyAlertResponseControl(w http.ResponseWriter, r *http.Request, operation string) {
	ctx := r.Context()
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert response control persistence is unavailable")
		return
	}
	tenantID, alertID, jobID, actor, idempotencyKey, ok := h.alertResponseWorkflowIdentity(w, r)
	if !ok {
		return
	}
	var request alertResponseControlRequest
	if !decodeSingleJSON(w, r, &request) {
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ExpectedRevision == nil || *request.ExpectedRevision <= 0 ||
		len(request.Reason) < 8 || len(request.Reason) > 1000 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "positive expected_revision and reason (8 to 1000 characters) are required")
		return
	}
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin alert response control transaction")
		return
	}
	defer tx.Rollback()
	if existing, found, err := loadAlertResponseControlByKey(ctx, tx, tenantID, idempotencyKey); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve control idempotency")
		return
	} else if found {
		if !alertResponseControlMatches(existing, alertID, jobID, actor, operation, request) {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different response control request")
			return
		}
		writeAlertResponseWorkflowAccepted(w, ctx, existing.JobID, existing.ResultingStatus,
			"", existing.ResultingRevision, true, responseActionOutboxStatus(existing.ResultingStatus, false))
		return
	}
	action, found, err := lockAlertResponseAction(ctx, tx, tenantID, alertID, jobID)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock alert response action")
		return
	}
	if !found {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "alert response action not found")
		return
	}
	if existing, replayFound, replayErr := loadAlertResponseControlByKey(ctx, tx, tenantID, idempotencyKey); replayErr != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to reconcile concurrent control idempotency")
		return
	} else if replayFound {
		if !alertResponseControlMatches(existing, alertID, jobID, actor, operation, request) {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different response control request")
			return
		}
		writeAlertResponseWorkflowAccepted(w, ctx, existing.JobID, existing.ResultingStatus,
			"", existing.ResultingRevision, true, responseActionOutboxStatus(existing.ResultingStatus, false))
		return
	}
	if action.Revision != *request.ExpectedRevision {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", fmt.Sprintf("expected revision %d but current revision is %d", *request.ExpectedRevision, action.Revision))
		return
	}
	if operation == "cancel" {
		h.cancelLockedAlertResponseAction(w, r, tx, action, actor, idempotencyKey, request)
		return
	}
	h.compensateLockedAlertResponseAction(w, r, tx, action, actor, idempotencyKey, request)
}

func (h *Handler) cancelLockedAlertResponseAction(
	w http.ResponseWriter,
	r *http.Request,
	tx *sql.Tx,
	action alertResponseActionState,
	actor, idempotencyKey string,
	request alertResponseControlRequest,
) {
	ctx := r.Context()
	switch action.Status {
	case "pending_approval":
		// No outbox exists before approval.
	case "accepted", "approved_awaiting_executor":
		result, err := tx.ExecContext(ctx, `UPDATE alert_response_outbox
			SET cancelled_at=now(),locked_until=NULL,locked_by=''
			WHERE job_id=$1 AND event_id=$2::uuid AND published=false AND cancelled_at IS NULL
			  AND (locked_until IS NULL OR locked_until < now())`,
			action.JobID, action.EventID,
		)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to cancel queued response event")
			return
		}
		if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
			httpx.JSONError(w, ctx, http.StatusConflict, "TOO_LATE_TO_CANCEL", "the response event is already leased or published")
			return
		}
	default:
		httpx.JSONError(w, ctx, http.StatusConflict, "TERMINAL_STATE", "the response action can no longer be cancelled")
		return
	}
	newRevision := action.Revision + 1
	if !h.insertAlertResponseControl(w, r, tx, action, actor, idempotencyKey,
		"cancel", "cancelled", newRevision, "cancelled", request) {
		return
	}
	result, err := tx.ExecContext(ctx, `UPDATE alert_response_actions
		SET status='cancelled',approval_status=CASE WHEN approval_status='pending' THEN 'cancelled' ELSE approval_status END,
		    error=$1,revision=$2,updated_at=now()
		WHERE tenant_id=$3 AND alert_id=$4 AND job_id=$5 AND revision=$6 AND status=$7`,
		"cancelled: "+request.Reason, newRevision, action.TenantID, action.AlertID,
		action.JobID, action.Revision, action.Status,
	)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to cancel alert response action")
		return
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "alert response action changed while cancellation was being applied")
		return
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: "ALERT_RESPONSE_ACTION_CANCELLED", ObjectType: "alert_response_action", ObjectID: action.JobID,
		TenantID: action.TenantID, UserID: actor, AlertID: action.AlertID,
		Reason: request.Reason, Result: "cancelled", Detail: map[string]interface{}{
			"action_id": action.ActionID, "expected_revision": action.Revision,
			"revision": newRevision, "idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
		},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit alert response cancellation")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit alert response cancellation")
		return
	}
	writeAlertResponseWorkflowAccepted(w, ctx, action.JobID, "cancelled", action.ApprovalStatus, newRevision, false, "cancelled")
}

var alertResponseCompensationCatalog = map[string]string{
	"alert-response-block-ip":         "alert-response-unblock-ip",
	"alert-response-block-connection": "alert-response-unblock-connection",
	"alert-response-isolate-host":     "alert-response-restore-host",
	"alert-response-add-whitelist":    "alert-response-remove-whitelist",
}

func (h *Handler) compensateLockedAlertResponseAction(
	w http.ResponseWriter,
	r *http.Request,
	tx *sql.Tx,
	action alertResponseActionState,
	actor, idempotencyKey string,
	request alertResponseControlRequest,
) {
	ctx := r.Context()
	compensationActionID, catalogued := alertResponseCompensationCatalog[action.ActionID]
	if !catalogued {
		httpx.JSONError(w, ctx, http.StatusConflict, "COMPENSATION_NOT_CATALOGUED", "the immutable action_id has no registered compensation")
		return
	}
	if actor == action.RequestedBy {
		httpx.JSONError(w, ctx, http.StatusForbidden, "INDEPENDENT_APPROVER_REQUIRED", "the original requester cannot authorize compensation")
		return
	}
	var receiptState string
	var externalEffect bool
	err := tx.QueryRowContext(ctx, `SELECT state,external_effect
		FROM alert_response_execution_receipts
		WHERE job_id=$1 AND tenant_id=$2 AND alert_id=$3
		FOR SHARE`,
		action.JobID, action.TenantID, action.AlertID,
	).Scan(&receiptState, &externalEffect)
	if err == sql.ErrNoRows || !externalEffect {
		httpx.JSONError(w, ctx, http.StatusConflict, "NO_EXTERNAL_EFFECT", "no confirmed external effect exists to compensate")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to inspect execution receipt")
		return
	}
	if action.Status != "completed" && action.Status != "partial" {
		httpx.JSONError(w, ctx, http.StatusConflict, "TERMINAL_STATE", "only a completed or partial external effect can be compensated")
		return
	}

	// No external compensation adapter is currently wired. Persist the request
	// and its blocked state so an operator sees the truth and no success can be
	// inferred from HTTP acceptance.
	newRevision := action.Revision + 1
	blockedStatus := "compensation_blocked_external_executor"
	if !h.insertAlertResponseControl(w, r, tx, action, actor, idempotencyKey,
		"compensate", "blocked_external_executor", newRevision, blockedStatus, request) {
		return
	}
	result, err := tx.ExecContext(ctx, `UPDATE alert_response_actions
		SET status=$1,error=$2,revision=$3,updated_at=now()
		WHERE tenant_id=$4 AND alert_id=$5 AND job_id=$6 AND revision=$7 AND status=$8`,
		blockedStatus, "compensation executor is not configured", newRevision,
		action.TenantID, action.AlertID, action.JobID, action.Revision, action.Status,
	)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist blocked compensation request")
		return
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "alert response action changed while compensation was being recorded")
		return
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: "ALERT_RESPONSE_COMPENSATION_BLOCKED", ObjectType: "alert_response_action", ObjectID: action.JobID,
		TenantID: action.TenantID, UserID: actor, AlertID: action.AlertID,
		Reason: request.Reason, Result: blockedStatus, Detail: map[string]interface{}{
			"action_id": action.ActionID, "compensation_action_id": compensationActionID,
			"receipt_state": receiptState, "external_effect": true,
			"expected_revision": action.Revision, "revision": newRevision,
			"idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
		},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit blocked compensation request")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit blocked compensation request")
		return
	}
	writeAlertResponseWorkflowAccepted(w, ctx, action.JobID, blockedStatus,
		action.ApprovalStatus, newRevision, false, "not_enqueued")
}

func (h *Handler) insertAlertResponseControl(
	w http.ResponseWriter,
	r *http.Request,
	tx *sql.Tx,
	action alertResponseActionState,
	actor, idempotencyKey, operation, state string,
	newRevision int64,
	resultingStatus string,
	request alertResponseControlRequest,
) bool {
	result, err := tx.ExecContext(r.Context(), `INSERT INTO alert_response_control_requests
		(request_id,job_id,tenant_id,alert_id,operation,expected_revision,idempotency_key,
		 reason,requested_by,state,resulting_revision,resulting_status)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (tenant_id,idempotency_key) DO NOTHING`,
		uuid.NewString(), action.JobID, action.TenantID, action.AlertID, operation,
		*request.ExpectedRevision, idempotencyKey, request.Reason, actor, state,
		newRevision, resultingStatus,
	)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert response control request")
		return false
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		httpx.JSONError(w, r.Context(), http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "response control idempotency collision")
		return false
	}
	return true
}

func (h *Handler) alertResponseWorkflowIdentity(
	w http.ResponseWriter,
	r *http.Request,
) (tenantID, alertID, jobID, actor, idempotencyKey string, ok bool) {
	tenantID = strings.TrimSpace(h.extractTenantID(r))
	alertID = strings.TrimSpace(mux.Vars(r)["id"])
	jobID = strings.TrimSpace(mux.Vars(r)["job_id"])
	actor = strings.TrimSpace(h.extractUserID(r))
	idempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if tenantID == "" || alertID == "" || jobID == "" {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "tenant, alert id and job id are required")
		return "", "", "", "", "", false
	}
	if actor == "" {
		httpx.JSONError(w, r.Context(), http.StatusUnauthorized, "ACTOR_REQUIRED", "an authenticated actor is required")
		return "", "", "", "", "", false
	}
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 16 to 200 characters")
		return "", "", "", "", "", false
	}
	return tenantID, alertID, jobID, actor, idempotencyKey, true
}

func decodeSingleJSON(w http.ResponseWriter, r *http.Request, target interface{}) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid alert response workflow request")
		return false
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "request body must contain exactly one JSON object")
		return false
	}
	return true
}

func lockAlertResponseAction(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, alertID, jobID string,
) (alertResponseActionState, bool, error) {
	var action alertResponseActionState
	err := tx.QueryRowContext(ctx, `SELECT job_id,event_id::text,tenant_id,alert_id,
			action_id,action,target,reason,requested_by,trace_id,status,approval_status,dry_run,revision
		FROM alert_response_actions
		WHERE tenant_id=$1 AND alert_id=$2 AND job_id=$3
		FOR UPDATE`,
		tenantID, alertID, jobID,
	).Scan(
		&action.JobID, &action.EventID, &action.TenantID, &action.AlertID,
		&action.ActionID, &action.Action, &action.Target, &action.Reason,
		&action.RequestedBy, &action.TraceID, &action.Status, &action.ApprovalStatus,
		&action.DryRun, &action.Revision,
	)
	if err == sql.ErrNoRows {
		return alertResponseActionState{}, false, nil
	}
	return action, err == nil, err
}

func loadAlertResponseApprovalByKey(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, idempotencyKey string,
) (alertResponseApprovalRecord, bool, error) {
	var record alertResponseApprovalRecord
	err := tx.QueryRowContext(ctx, `SELECT job_id,alert_id,decision,expected_revision,reason,
		decided_by,resulting_revision,resulting_status,approval_status
		FROM alert_response_approvals
		WHERE tenant_id=$1 AND idempotency_key=$2`,
		tenantID, idempotencyKey,
	).Scan(
		&record.JobID, &record.AlertID, &record.Decision, &record.ExpectedRevision,
		&record.Reason, &record.DecidedBy, &record.ResultingRevision,
		&record.ResultingStatus, &record.ApprovalStatus,
	)
	if err == sql.ErrNoRows {
		return alertResponseApprovalRecord{}, false, nil
	}
	return record, err == nil, err
}

func alertResponseApprovalMatches(
	existing alertResponseApprovalRecord,
	alertID, jobID, actor string,
	request alertResponseApprovalRequest,
) bool {
	return existing.AlertID == alertID && existing.JobID == jobID &&
		existing.Decision == request.Decision &&
		existing.ExpectedRevision == *request.ExpectedRevision &&
		existing.Reason == request.Reason && existing.DecidedBy == actor
}

func loadAlertResponseControlByKey(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, idempotencyKey string,
) (alertResponseControlRecord, bool, error) {
	var record alertResponseControlRecord
	err := tx.QueryRowContext(ctx, `SELECT job_id,alert_id,operation,expected_revision,reason,
		requested_by,state,resulting_revision,resulting_status
		FROM alert_response_control_requests
		WHERE tenant_id=$1 AND idempotency_key=$2`,
		tenantID, idempotencyKey,
	).Scan(
		&record.JobID, &record.AlertID, &record.Operation, &record.ExpectedRevision,
		&record.Reason, &record.RequestedBy, &record.State,
		&record.ResultingRevision, &record.ResultingStatus,
	)
	if err == sql.ErrNoRows {
		return alertResponseControlRecord{}, false, nil
	}
	return record, err == nil, err
}

func alertResponseControlMatches(
	existing alertResponseControlRecord,
	alertID, jobID, actor, operation string,
	request alertResponseControlRequest,
) bool {
	return existing.AlertID == alertID && existing.JobID == jobID &&
		existing.Operation == operation &&
		existing.ExpectedRevision == *request.ExpectedRevision &&
		existing.Reason == request.Reason && existing.RequestedBy == actor
}

func writeAlertResponseWorkflowAccepted(
	w http.ResponseWriter,
	ctx context.Context,
	jobID, status, approvalStatus string,
	revision int64,
	idempotentReuse bool,
	outboxStatus string,
) {
	result := map[string]interface{}{
		"job_id": jobID, "status": status, "revision": revision,
		"idempotent_reuse": idempotentReuse, "outbox_status": outboxStatus,
	}
	if approvalStatus != "" {
		result["approval_status"] = approvalStatus
	}
	httpx.JSONContractAccepted(w, ctx, result, alertResponseContractMeta(ctx, jobID, revision))
}

func alertResponseContractMeta(ctx context.Context, jobID string, revision int64) httpx.ContractMeta {
	return httpx.ContractMeta{
		ContractVersion: 1,
		SnapshotID:      jobID,
		TraceID:         httpx.GetTraceID(ctx),
		Partial:         false,
		MissingSections: []string{},
		SourceWatermarks: map[string]string{
			"postgresql.alert_response_actions.revision": fmt.Sprint(revision),
		},
	}
}
