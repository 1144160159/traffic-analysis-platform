package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type alertWorkbenchActionRequest struct {
	ActionID         string                 `json:"action_id,omitempty"`
	Action           string                 `json:"action"`
	Target           string                 `json:"target"`
	Reason           string                 `json:"reason"`
	DryRun           bool                   `json:"dry_run"`
	ExpectedRevision *int64                 `json:"expected_revision,omitempty"`
	Detail           map[string]interface{} `json:"detail,omitempty"`
}

type alertSavedViewDTO struct {
	ViewID          string                 `json:"view_id"`
	Name            string                 `json:"name"`
	Filters         map[string]interface{} `json:"filters"`
	Revision        int64                  `json:"revision"`
	EventID         string                 `json:"event_id,omitempty"`
	OutboxStatus    string                 `json:"outbox_status,omitempty"`
	IdempotentReuse bool                   `json:"idempotent_reuse"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

func (h *Handler) CreateAlertResponseAction(w http.ResponseWriter, r *http.Request) {
	h.persistAlertAction(w, r, "ALERT_RESPONSE_ACTION_REQUESTED", "alert_response_action", true)
}

func (h *Handler) CreateAlertInvestigationNote(w http.ResponseWriter, r *http.Request) {
	h.persistAlertAction(w, r, "ALERT_INVESTIGATION_NOTE_RECORDED", "alert_investigation_note", false)
}

func (h *Handler) persistAlertAction(w http.ResponseWriter, r *http.Request, auditEvent, objectType string, responseAction bool) {
	ctx := r.Context()
	if !h.requireAlertWritePermission(w, r) {
		return
	}
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "TENANT_REQUIRED", "tenant_id is required")
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert action persistence is unavailable")
		return
	}
	request, ok := decodeAlertActionRequest(w, r)
	if !ok {
		return
	}
	idempotencyKey := ""
	expectedRevision := int64(0)
	requestedBy := h.extractUserID(r)
	if responseAction {
		idempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 16 to 200 characters")
			return
		}
		if request.ExpectedRevision == nil || *request.ExpectedRevision != 0 {
			httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "a new alert response action requires expected_revision=0")
			return
		}
		if strings.TrimSpace(requestedBy) == "" {
			httpx.JSONError(w, ctx, http.StatusUnauthorized, "ACTOR_REQUIRED", "an authenticated actor is required")
			return
		}
		expectedRevision = *request.ExpectedRevision
	}
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	if alertID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "ALERT_REQUIRED", "alert id is required")
		return
	}
	if h.alertService != nil {
		if _, err := h.alertService.GetAlert(ctx, tenantID, alertID); err != nil {
			if commonerrors.IsCode(err, commonerrors.ErrCodeAlertNotFound) {
				httpx.JSONError(w, ctx, http.StatusNotFound, "ALERT_NOT_FOUND", "alert not found")
			} else {
				httpx.JSONError(w, ctx, http.StatusInternalServerError, "ALERT_LOOKUP_FAILED", "failed to validate alert")
			}
			return
		}
	}
	jobID := "alert-action-" + uuid.NewString()
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert.response.requested.v1:"+jobID)).String()
	traceID := strings.TrimSpace(httpx.GetTraceID(ctx))
	if traceID == "" {
		traceID = eventID
	}
	status := "recorded"
	if responseAction {
		status = "pending_approval"
		if request.DryRun {
			status = "accepted"
		}
	}
	detail := cloneActionDetail(request.Detail)
	detail["job_id"] = jobID
	detail["action_id"] = request.ActionID
	detail["action"] = request.Action
	detail["target"] = request.Target
	detail["dry_run"] = request.DryRun
	if responseAction {
		detail["expected_revision"] = expectedRevision
		detail["idempotency_key_sha256"] = opaqueKeyDigest(idempotencyKey)
	}
	detailJSON, _ := json.Marshal(detail)
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin alert action transaction")
		return
	}
	defer tx.Rollback()
	approvalStatus := "not_required"
	if responseAction && !request.DryRun {
		approvalStatus = "pending"
	}
	insert, err := tx.ExecContext(ctx, `INSERT INTO alert_response_actions
		(job_id,event_id,tenant_id,alert_id,action_id,action,target,reason,dry_run,
		 status,approval_status,revision,trace_id,idempotency_key,expected_revision,detail,requested_by)
		VALUES ($1,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,1,$12,$13,$14,$15::jsonb,$16)
		ON CONFLICT (tenant_id,idempotency_key) WHERE idempotency_key<>'' DO NOTHING`,
		jobID, eventID, tenantID, alertID, request.ActionID, request.Action,
		request.Target, request.Reason, request.DryRun, status, approvalStatus,
		traceID, idempotencyKey, expectedRevision, string(detailJSON), requestedBy)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert action")
		return
	}
	inserted, err := insert.RowsAffected()
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to inspect alert action persistence")
		return
	}
	if inserted == 0 {
		var existingJobID, existingEventID, existingAlertID, existingActionID, existingRequestedBy string
		var existingAction, existingTarget, existingReason, existingStatus, existingApproval string
		var existingDryRun bool
		var existingExpectedRevision, existingRevision int64
		err = tx.QueryRowContext(ctx, `SELECT job_id,event_id::text,alert_id,action_id,action,target,
			reason,dry_run,expected_revision,revision,status,approval_status,requested_by
			FROM alert_response_actions
			WHERE tenant_id=$1 AND idempotency_key=$2`,
			tenantID, idempotencyKey,
		).Scan(
			&existingJobID, &existingEventID, &existingAlertID, &existingActionID,
			&existingAction, &existingTarget, &existingReason, &existingDryRun,
			&existingExpectedRevision, &existingRevision, &existingStatus, &existingApproval,
			&existingRequestedBy,
		)
		if err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve alert action idempotency")
			return
		}
		if existingAlertID != alertID || existingActionID != request.ActionID ||
			existingAction != request.Action || existingTarget != request.Target ||
			existingReason != request.Reason || existingDryRun != request.DryRun ||
			existingExpectedRevision != expectedRevision || existingRequestedBy != requestedBy {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different alert response action")
			return
		}
		replayResult := map[string]interface{}{
			"job_id": existingJobID, "event_id": existingEventID,
			"status": existingStatus, "approval_status": existingApproval,
			"outbox_status": responseActionOutboxStatus(existingStatus, existingDryRun),
			"revision":      existingRevision, "idempotent_reuse": true,
			"action_id": existingActionID, "action": existingAction,
			"target": existingTarget, "dry_run": existingDryRun,
			"audit_event": auditEvent,
		}
		httpx.JSONContractAccepted(w, ctx, replayResult, alertResponseContractMeta(ctx, existingJobID, existingRevision))
		return
	}
	if responseAction && request.DryRun {
		eventPayload, _ := json.Marshal(map[string]interface{}{
			"event_id": eventID, "event_type": "alert.response.requested.v1",
			"schema_version": 1, "aggregate_version": 1,
			"job_id": jobID, "tenant_id": tenantID, "alert_id": alertID,
			"action_id": request.ActionID, "action": request.Action,
			"target": request.Target, "reason": request.Reason,
			"requested_by": requestedBy, "trace_id": traceID, "dry_run": true,
		})
		if _, err = tx.ExecContext(ctx, `INSERT INTO alert_response_outbox
			(job_id,event_id,tenant_id,event_type,schema_version,aggregate_version,
			 partition_key,payload)
			VALUES ($1,$2::uuid,$3,$4,1,1,$5,$6::jsonb)`,
			jobID, eventID, tenantID, "alert.response.requested.v1",
			tenantID+":"+jobID, string(eventPayload)); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to enqueue alert response action")
			return
		}
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{Action: auditEvent, ObjectType: objectType, ObjectID: alertID, TenantID: tenantID, UserID: requestedBy, AlertID: alertID, Reason: request.Reason, Result: status, Detail: detail}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit alert action")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit alert action")
		return
	}
	outboxStatus := "not_required"
	if responseAction {
		outboxStatus = "awaiting_approval"
		if request.DryRun {
			outboxStatus = "pending_retry"
		}
	}
	result := map[string]interface{}{"job_id": jobID, "event_id": eventID, "status": status, "approval_status": approvalStatus, "outbox_status": outboxStatus, "revision": int64(1), "idempotent_reuse": false, "action_id": request.ActionID, "action": request.Action, "target": request.Target, "dry_run": request.DryRun, "audit_event": auditEvent}
	if responseAction {
		httpx.JSONContractAccepted(w, ctx, result, alertResponseContractMeta(ctx, jobID, 1))
		return
	}
	httpx.JSONCreated(w, ctx, result)
}

func responseActionOutboxStatus(status string, dryRun bool) string {
	if status == "cancelled" {
		return "cancelled"
	}
	if status == "pending_approval" {
		return "awaiting_approval"
	}
	if status == "simulated_completed" || status == "blocked_external_executor" ||
		status == "completed" || status == "partial" || status == "failed" {
		return "published"
	}
	if dryRun || status == "approved_awaiting_executor" || status == "accepted" || status == "compensation_queued" {
		return "pending_retry"
	}
	return "not_required"
}

type responseOutboxItem struct {
	OutboxID int64
	JobID    string
	TenantID string
	Payload  map[string]interface{}
}

// StartResponseActionOutboxWorker starts the only delivery path for response
// requests. HTTP handlers commit the action and outbox row atomically; this
// worker claims pending rows with a lease, retries failures with backoff and
// marks a row published only after Kafka acknowledges it.
func (h *Handler) StartResponseActionOutboxWorker(ctx context.Context, interval time.Duration) error {
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return fmt.Errorf("alert response outbox database is unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	workerID := fmt.Sprintf("%s-%d", hostnameOrDefault(), os.Getpid())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if _, err := h.drainResponseActionOutbox(ctx, workerID, 25); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to drain alert response outbox", zap.Error(err))
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func hostnameOrDefault() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "alert-service"
	}
	return hostname
}

func (h *Handler) drainResponseActionOutbox(ctx context.Context, workerID string, limit int) (int, error) {
	if h.responseProducer == nil || h.actionAudit == nil || h.actionAudit.db == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := h.actionAudit.db.QueryContext(ctx, `WITH candidates AS (
		SELECT outbox_id FROM alert_response_outbox
		WHERE published=false AND cancelled_at IS NULL
		  AND next_attempt_at <= now() AND (locked_until IS NULL OR locked_until < now())
		ORDER BY next_attempt_at, outbox_id
		LIMIT $1 FOR UPDATE SKIP LOCKED
	), claimed AS (
		UPDATE alert_response_outbox o
		SET locked_until=now()+interval '60 seconds', locked_by=$2
		FROM candidates c WHERE o.outbox_id=c.outbox_id
		RETURNING o.outbox_id,o.job_id,o.tenant_id,o.payload::text
	) SELECT outbox_id,job_id,tenant_id,payload FROM claimed ORDER BY outbox_id`, limit, workerID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	items := make([]responseOutboxItem, 0, limit)
	for rows.Next() {
		var item responseOutboxItem
		var rawPayload string
		if err := rows.Scan(&item.OutboxID, &item.JobID, &item.TenantID, &rawPayload); err != nil {
			return len(items), err
		}
		if err := json.Unmarshal([]byte(rawPayload), &item.Payload); err != nil {
			_, _ = h.actionAudit.db.ExecContext(ctx, `UPDATE alert_response_outbox SET attempts=attempts+1,last_error=$2,next_attempt_at=now()+interval '5 minutes',locked_until=NULL,locked_by='' WHERE outbox_id=$1 AND locked_by=$3`, item.OutboxID, "invalid outbox payload: "+err.Error(), workerID)
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return len(items), err
	}
	processed := 0
	for _, item := range items {
		if err := h.publishResponseOutboxItem(ctx, workerID, item); err != nil {
			if h.logger != nil {
				h.logger.Warn("Alert response outbox delivery failed", zap.String("job_id", item.JobID), zap.Error(err))
			}
			continue
		}
		processed++
	}
	return processed, nil
}

func (h *Handler) publishResponseOutboxItem(ctx context.Context, workerID string, item responseOutboxItem) error {
	if h.responseProducer == nil || h.actionAudit == nil || h.actionAudit.db == nil {
		return fmt.Errorf("alert response publisher is unavailable")
	}
	alertID, _ := item.Payload["alert_id"].(string)
	eventID, _ := item.Payload["event_id"].(string)
	actionID, _ := item.Payload["action_id"].(string)
	traceID, _ := item.Payload["trace_id"].(string)
	for field, value := range map[string]string{
		"event_id": eventID, "tenant_id": item.TenantID, "job_id": item.JobID,
		"alert_id": alertID, "action_id": actionID, "trace_id": traceID,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("alert response outbox %s is missing", field)
		}
	}
	aggregateVersion := fmt.Sprint(item.Payload["aggregate_version"])
	if aggregateVersion == "" || aggregateVersion == "<nil>" {
		return fmt.Errorf("alert response outbox aggregate_version is missing")
	}
	err := h.responseProducer.SendJSON(ctx, item.TenantID+":"+item.JobID, item.Payload,
		kafka.MessageHeader{Key: "event_id", Value: eventID},
		kafka.MessageHeader{Key: "event_type", Value: "alert.response.requested.v1"},
		kafka.MessageHeader{Key: "schema_version", Value: "1"},
		kafka.MessageHeader{Key: "aggregate_version", Value: aggregateVersion},
		kafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		kafka.MessageHeader{Key: "alert_id", Value: alertID},
		kafka.MessageHeader{Key: "job_id", Value: item.JobID},
		kafka.MessageHeader{Key: "action_id", Value: actionID},
		kafka.MessageHeader{Key: "trace_id", Value: traceID})
	if err != nil {
		_, _ = h.actionAudit.db.ExecContext(ctx, `UPDATE alert_response_outbox SET attempts=attempts+1,last_error=$2,next_attempt_at=now()+(LEAST(300,POWER(2,LEAST(attempts+1,8)))::text || ' seconds')::interval,locked_until=NULL,locked_by='' WHERE outbox_id=$1 AND published=false AND locked_by=$3`, item.OutboxID, err.Error(), workerID)
		return err
	}
	result, err := h.actionAudit.db.ExecContext(ctx, `UPDATE alert_response_outbox SET published=true,attempts=attempts+1,last_error='',published_at=now(),locked_until=NULL,locked_by='' WHERE outbox_id=$1 AND published=false AND locked_by=$2`, item.OutboxID, workerID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("outbox lease lost before publish acknowledgement")
	}
	return nil
}

func (h *Handler) GetAlertResponseAction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireAlertReadPermission(w, r) {
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert action persistence is unavailable")
		return
	}
	tenantID, alertID, jobID := h.extractTenantID(r), strings.TrimSpace(mux.Vars(r)["id"]), strings.TrimSpace(mux.Vars(r)["job_id"])
	var action, actionID, target, status, approvalStatus, reason, approvedBy, lastError, resultJSON, actionError string
	var dryRun bool
	var outboxPublished, outboxCancelled bool
	var outboxAttempts int
	var revision int64
	var approvedAt sql.NullTime
	var createdAt, updatedAt time.Time
	err := h.actionAudit.db.QueryRowContext(ctx, `SELECT a.action_id,a.action,a.target,a.status,
		a.approval_status,a.reason,a.dry_run,a.revision,a.approved_by,a.approved_at,
		a.result::text,a.error,a.created_at,a.updated_at,
		COALESCE(o.published,false),COALESCE(o.cancelled_at IS NOT NULL,false),
		COALESCE(o.attempts,0),COALESCE(o.last_error,'')
		FROM alert_response_actions a
		LEFT JOIN alert_response_outbox o ON o.job_id=a.job_id
		WHERE a.tenant_id=$1 AND a.alert_id=$2 AND a.job_id=$3
		ORDER BY o.outbox_id DESC LIMIT 1`, tenantID, alertID, jobID).Scan(
		&actionID, &action, &target, &status, &approvalStatus, &reason, &dryRun,
		&revision, &approvedBy, &approvedAt, &resultJSON, &actionError,
		&createdAt, &updatedAt, &outboxPublished, &outboxCancelled,
		&outboxAttempts, &lastError,
	)
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "alert response action not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", err.Error())
		return
	}
	var result map[string]interface{}
	_ = json.Unmarshal([]byte(resultJSON), &result)
	var approvedAtValue interface{}
	if approvedAt.Valid {
		approvedAtValue = approvedAt.Time
	}
	response := map[string]interface{}{
		"job_id": jobID, "action_id": actionID, "action": action, "target": target,
		"status": status, "approval_status": approvalStatus, "revision": revision,
		"approved_by": approvedBy, "approved_at": approvedAtValue,
		"reason": reason, "dry_run": dryRun, "result": result, "error": actionError,
		"outbox_published": outboxPublished, "outbox_cancelled": outboxCancelled,
		"outbox_attempts": outboxAttempts, "outbox_last_error": lastError,
		"created_at": createdAt, "updated_at": updatedAt,
	}
	httpx.JSONContractSuccess(w, ctx, response, alertResponseContractMeta(ctx, jobID, revision))
}

func (h *Handler) SaveAlertView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireAlertWritePermission(w, r) {
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert view persistence is unavailable")
		return
	}
	tenantID := h.extractTenantID(r)
	actor := h.extractUserID(r)
	if strings.TrimSpace(tenantID) == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "TENANT_REQUIRED", "tenant_id is required")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 16 to 200 characters")
		return
	}
	request, ok := decodeAlertActionRequest(w, r)
	if !ok {
		return
	}
	filters := make(map[string]interface{})
	if nested, exists := request.Detail["filters"].(map[string]interface{}); exists {
		for key, value := range nested {
			filters[key] = value
		}
	} else {
		for key, value := range request.Detail {
			filters[key] = value
		}
	}
	if timeWindow, exists := request.Detail["time_window"]; exists {
		filters["time_window"] = timeWindow
	}
	filtersJSON, _ := json.Marshal(filters)
	payloadIdentity := []string{
		tenantID, actor, request.ActionID, request.Target, request.Reason, string(filtersJSON),
	}
	// Preserve the legacy request digest when expected_revision is omitted so
	// that in-flight pre-rollout idempotency receipts remain replayable. Strict
	// clients bind the expected revision into the command identity.
	if request.ExpectedRevision != nil {
		if *request.ExpectedRevision < 0 {
			httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REVISION", "expected_revision must be zero for create or the current revision for update")
			return
		}
		payloadIdentity = append(payloadIdentity, fmt.Sprint(*request.ExpectedRevision))
	}
	// The receipt schema stores the canonical 64-character hexadecimal digest;
	// opaqueKeyDigest includes an audit-display prefix that is intentionally not
	// part of the persisted value.
	payloadHash := strings.TrimPrefix(opaqueKeyDigest(strings.Join(payloadIdentity, "\x00")), "sha256:")
	eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("alert.saved-view.v2:"+tenantID+":"+idempotencyKey)).String()
	tx, err := h.actionAudit.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin alert view transaction")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+idempotencyKey); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock alert view request")
		return
	}
	view := alertSavedViewDTO{Filters: filters, EventID: eventID, OutboxStatus: "pending"}
	var existingHash, existingFilters string
	err = tx.QueryRowContext(ctx, `SELECT r.payload_sha256,r.view_id::text,r.resulting_revision,r.event_id::text,
		v.name,v.filters::text,v.created_at,v.updated_at,COALESCE(o.status,'pending')
		FROM alert_saved_view_requests r
		JOIN alert_saved_views v ON v.view_id=r.view_id AND v.tenant_id=r.tenant_id
		LEFT JOIN alert_saved_view_outbox o ON o.event_id=r.event_id
		WHERE r.tenant_id=$1 AND r.idempotency_key=$2 FOR UPDATE OF r`, tenantID, idempotencyKey).Scan(
		&existingHash, &view.ViewID, &view.Revision, &view.EventID,
		&view.Name, &existingFilters, &view.CreatedAt, &view.UpdatedAt, &view.OutboxStatus)
	if err == nil {
		if existingHash != payloadHash {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different saved view")
			return
		}
		if err = json.Unmarshal([]byte(existingFilters), &view.Filters); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to decode persisted alert view")
			return
		}
		view.IdempotentReuse = true
		if err = tx.Commit(); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit alert view replay")
			return
		}
		httpx.JSONContractCreated(w, ctx, view, alertSavedViewContractMeta(ctx, tenantID, view.ViewID, view.Revision, "saveAlertView"))
		return
	}
	if err != sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve alert view idempotency")
		return
	}
	// A name-scoped advisory lock closes the absent-row race for two new
	// idempotency keys targeting the same view. The SQL update predicate below
	// remains the final guard during mixed-version rollout.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":alert-saved-view:"+request.Target); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to lock alert view revision")
		return
	}
	var currentRevision int64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM alert_saved_views
		WHERE tenant_id=$1 AND name=$2 FOR UPDATE`, tenantID, request.Target).Scan(&currentRevision)
	switch {
	case err == nil && request.ExpectedRevision != nil && currentRevision != *request.ExpectedRevision:
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "alert view revision changed; refresh before saving")
		return
	case err == sql.ErrNoRows && request.ExpectedRevision != nil && *request.ExpectedRevision != 0:
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "alert view does not exist at the expected revision")
		return
	case err != nil && err != sql.ErrNoRows:
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to resolve alert view revision")
		return
	}
	var expectedRevision interface{}
	if request.ExpectedRevision != nil {
		expectedRevision = *request.ExpectedRevision
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO alert_saved_views
		(tenant_id,name,filters,created_by,updated_by,trace_id,revision)
		VALUES ($1,$2,$3::jsonb,$4,$4,$5,1)
		ON CONFLICT (tenant_id,name) DO UPDATE SET
		filters=EXCLUDED.filters,updated_by=EXCLUDED.updated_by,trace_id=EXCLUDED.trace_id,
		revision=alert_saved_views.revision+1,updated_at=now()
		WHERE $6::bigint IS NULL OR alert_saved_views.revision=$6
		RETURNING view_id::text,name,revision,created_at,updated_at`,
		tenantID, request.Target, string(filtersJSON), actor, httpx.GetTraceID(ctx), expectedRevision).Scan(
		&view.ViewID, &view.Name, &view.Revision, &view.CreatedAt, &view.UpdatedAt)
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "alert view revision changed; refresh before saving")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert view")
		return
	}
	eventPayload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID, "event_type": "alert.saved-view.saved.v1", "schema_version": 1,
		"aggregate_type": "alert_saved_view", "aggregate_id": view.ViewID,
		"aggregate_version": view.Revision, "tenant_id": tenantID, "view_id": view.ViewID,
		"name": view.Name, "filters": filters, "expected_revision": request.ExpectedRevision,
		"changed_by": actor, "trace_id": httpx.GetTraceID(ctx),
	})
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_saved_view_history
		(event_id,tenant_id,view_id,revision,name,filters,action,changed_by,trace_id)
		VALUES ($1::uuid,$2,$3::uuid,$4,$5,$6::jsonb,'saved',$7,$8)`,
		eventID, tenantID, view.ViewID, view.Revision, view.Name, string(filtersJSON), actor, httpx.GetTraceID(ctx)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert view history")
		return
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_saved_view_outbox
		(event_id,aggregate_id,aggregate_version,tenant_id,event_type,schema_version,partition_key,payload,trace_id)
		VALUES ($1::uuid,$2::uuid,$3,$4,'alert.saved-view.saved.v1',1,$5,$6::jsonb,$7)`,
		eventID, view.ViewID, view.Revision, tenantID, tenantID+":"+view.ViewID, string(eventPayload), httpx.GetTraceID(ctx)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to enqueue alert view event")
		return
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{Action: "ALERT_VIEW_SAVED", ObjectType: "alert_saved_view", ObjectID: view.ViewID, TenantID: tenantID, UserID: actor, Reason: request.Reason, Result: "saved", StateVersion: uint64(view.Revision), Detail: map[string]interface{}{"event_id": eventID, "action_id": request.ActionID, "view_id": view.ViewID, "name": view.Name, "filters": filters, "expected_revision": request.ExpectedRevision}}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit alert view")
		return
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_saved_view_requests
		(tenant_id,idempotency_key,payload_sha256,view_id,resulting_revision,event_id)
		VALUES ($1,$2,$3,$4::uuid,$5,$6::uuid)`,
		tenantID, idempotencyKey, payloadHash, view.ViewID, view.Revision, eventID); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert view request")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit alert view")
		return
	}
	httpx.JSONContractCreated(w, ctx, view, alertSavedViewContractMeta(ctx, tenantID, view.ViewID, view.Revision, "saveAlertView"))
}

func (h *Handler) ListAlertViews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireAlertReadPermission(w, r) {
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert view persistence is unavailable")
		return
	}
	rows, err := h.actionAudit.db.QueryContext(ctx, `SELECT view_id::text,name,filters::text,revision,created_at,updated_at FROM alert_saved_views WHERE tenant_id=$1 ORDER BY updated_at DESC LIMIT 50`, h.extractTenantID(r))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", err.Error())
		return
	}
	defer rows.Close()
	views := make([]alertSavedViewDTO, 0)
	for rows.Next() {
		var view alertSavedViewDTO
		var raw string
		if err = rows.Scan(&view.ViewID, &view.Name, &raw, &view.Revision, &view.CreatedAt, &view.UpdatedAt); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", err.Error())
			return
		}
		_ = json.Unmarshal([]byte(raw), &view.Filters)
		views = append(views, view)
	}
	if err = rows.Err(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", err.Error())
		return
	}
	httpx.JSONContractSuccess(w, ctx, map[string]interface{}{"views": views, "total": len(views)}, alertSavedViewContractMeta(ctx, h.extractTenantID(r), "alert-views:"+httpx.GetTraceID(ctx), int64(len(views)), "listAlertViews"))
}

func alertSavedViewContractMeta(ctx context.Context, tenantID, snapshotID string, revision int64, operationID string) httpx.ContractMeta {
	watermarkKey := "postgresql.alert_saved_views.revision"
	if operationID == "listAlertViews" {
		watermarkKey = "postgresql.alert_saved_views.result_count"
	}
	return httpx.ContractMeta{
		ContractVersion: 1,
		SnapshotID:      snapshotID,
		OperationID:     operationID,
		TenantID:        tenantID,
		Partial:         false,
		MissingSections: []string{},
		SourceWatermarks: map[string]string{
			watermarkKey: fmt.Sprint(revision),
		},
	}
}

func decodeAlertActionRequest(w http.ResponseWriter, r *http.Request) (alertWorkbenchActionRequest, bool) {
	var request alertWorkbenchActionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid alert action request")
		return request, false
	}
	request.ActionID, request.Action, request.Target, request.Reason = strings.TrimSpace(request.ActionID), strings.TrimSpace(request.Action), strings.TrimSpace(request.Target), strings.TrimSpace(request.Reason)
	if request.Detail == nil {
		request.Detail = map[string]interface{}{}
	}
	if request.Action == "" || request.Target == "" || len(request.Reason) < 4 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "action, target and reason (minimum 4 characters) are required")
		return request, false
	}
	// Compatibility clients may omit action_id during the additive rollout.
	// The endpoint semantics provide a stable legacy identifier; display text
	// is never parsed to select a command.
	if request.ActionID == "" {
		request.ActionID = "legacy.alert-action"
	}
	return request, true
}

func cloneActionDetail(source map[string]interface{}) map[string]interface{} {
	target := make(map[string]interface{}, len(source)+4)
	for k, v := range source {
		target[k] = v
	}
	return target
}
