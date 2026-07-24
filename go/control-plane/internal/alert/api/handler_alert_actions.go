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
	Action string                 `json:"action"`
	Target string                 `json:"target"`
	Reason string                 `json:"reason"`
	DryRun bool                   `json:"dry_run"`
	Detail map[string]interface{} `json:"detail,omitempty"`
}

type alertSavedViewDTO struct {
	ViewID    string                 `json:"view_id"`
	Name      string                 `json:"name"`
	Filters   map[string]interface{} `json:"filters"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
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
	if err := ensureAlertWorkbenchSchema(ctx, h.actionAudit.db); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "SCHEMA_FAILED", "failed to prepare alert action persistence")
		return
	}

	jobID := "alert-action-" + uuid.NewString()
	status := "recorded"
	if responseAction {
		status = "pending_approval"
	}
	detail := cloneActionDetail(request.Detail)
	detail["job_id"] = jobID
	detail["action"] = request.Action
	detail["target"] = request.Target
	detail["dry_run"] = request.DryRun
	detailJSON, _ := json.Marshal(detail)
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin alert action transaction")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_response_actions (job_id, tenant_id, alert_id, action, target, reason, dry_run, status, detail, requested_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10)`, jobID, tenantID, alertID, request.Action, request.Target, request.Reason, request.DryRun, status, string(detailJSON), h.extractUserID(r)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert action")
		return
	}
	if responseAction {
		eventPayload, _ := json.Marshal(map[string]interface{}{"job_id": jobID, "tenant_id": tenantID, "alert_id": alertID, "action": request.Action, "target": request.Target, "dry_run": request.DryRun})
		if _, err = tx.ExecContext(ctx, `INSERT INTO alert_response_outbox (job_id, tenant_id, event_type, payload) VALUES ($1,$2,$3,$4::jsonb)`, jobID, tenantID, "alert.response.requested.v1", string(eventPayload)); err != nil {
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to enqueue alert response action")
			return
		}
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{Action: auditEvent, ObjectType: objectType, ObjectID: alertID, TenantID: tenantID, UserID: h.extractUserID(r), AlertID: alertID, Reason: request.Reason, Result: status, Detail: detail}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit alert action")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit alert action")
		return
	}
	outboxStatus := "not_required"
	if responseAction {
		outboxStatus = "pending_retry"
	}
	httpx.JSONCreated(w, ctx, map[string]interface{}{"job_id": jobID, "status": status, "outbox_status": outboxStatus, "action": request.Action, "target": request.Target, "dry_run": request.DryRun, "audit_event": auditEvent})
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
	if err := ensureAlertWorkbenchSchema(ctx, h.actionAudit.db); err != nil {
		return err
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
		WHERE published=false AND next_attempt_at <= now() AND (locked_until IS NULL OR locked_until < now())
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
	err := h.responseProducer.SendJSON(ctx, item.TenantID+":"+item.JobID, item.Payload,
		kafka.MessageHeader{Key: "tenant_id", Value: item.TenantID},
		kafka.MessageHeader{Key: "alert_id", Value: alertID},
		kafka.MessageHeader{Key: "job_id", Value: item.JobID})
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
	tenantID, jobID := h.extractTenantID(r), strings.TrimSpace(mux.Vars(r)["job_id"])
	if err := ensureAlertWorkbenchSchema(ctx, h.actionAudit.db); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "SCHEMA_FAILED", err.Error())
		return
	}
	var action, target, status, reason, lastError string
	var dryRun bool
	var outboxPublished bool
	var outboxAttempts int
	var createdAt, updatedAt time.Time
	err := h.actionAudit.db.QueryRowContext(ctx, `SELECT a.action,a.target,a.status,a.reason,a.dry_run,a.created_at,a.updated_at,COALESCE(o.published,false),COALESCE(o.attempts,0),COALESCE(o.last_error,'') FROM alert_response_actions a LEFT JOIN alert_response_outbox o ON o.job_id=a.job_id WHERE a.tenant_id=$1 AND a.job_id=$2 ORDER BY o.outbox_id DESC LIMIT 1`, tenantID, jobID).Scan(&action, &target, &status, &reason, &dryRun, &createdAt, &updatedAt, &outboxPublished, &outboxAttempts, &lastError)
	if err == sql.ErrNoRows {
		httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "alert response action not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"job_id": jobID, "action": action, "target": target, "status": status, "reason": reason, "dry_run": dryRun, "outbox_published": outboxPublished, "outbox_attempts": outboxAttempts, "outbox_last_error": lastError, "created_at": createdAt, "updated_at": updatedAt})
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
	request, ok := decodeAlertActionRequest(w, r)
	if !ok {
		return
	}
	if err := ensureAlertWorkbenchSchema(ctx, h.actionAudit.db); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "SCHEMA_FAILED", err.Error())
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
	view := alertSavedViewDTO{Filters: filters}
	err := h.actionAudit.db.QueryRowContext(ctx, `INSERT INTO alert_saved_views (tenant_id,name,filters,created_by) VALUES ($1,$2,$3::jsonb,$4)
		ON CONFLICT (tenant_id,name) DO UPDATE SET filters=EXCLUDED.filters, updated_at=now()
		RETURNING view_id::text,name,created_at,updated_at`, tenantID, request.Target, string(filtersJSON), h.extractUserID(r)).Scan(&view.ViewID, &view.Name, &view.CreatedAt, &view.UpdatedAt)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert view")
		return
	}
	if err := h.actionAudit.Record(ctx, r, AlertActionAuditRecord{Action: "ALERT_VIEW_SAVED", ObjectType: "alert_saved_view", ObjectID: view.ViewID, TenantID: tenantID, UserID: h.extractUserID(r), Reason: request.Reason, Result: "saved", Detail: map[string]interface{}{"view_id": view.ViewID, "name": view.Name, "filters": filters}}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit alert view")
		return
	}
	httpx.JSONCreated(w, ctx, view)
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
	if err := ensureAlertWorkbenchSchema(ctx, h.actionAudit.db); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "SCHEMA_FAILED", err.Error())
		return
	}
	rows, err := h.actionAudit.db.QueryContext(ctx, `SELECT view_id::text,name,filters::text,created_at,updated_at FROM alert_saved_views WHERE tenant_id=$1 ORDER BY updated_at DESC LIMIT 50`, h.extractTenantID(r))
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", err.Error())
		return
	}
	defer rows.Close()
	views := make([]alertSavedViewDTO, 0)
	for rows.Next() {
		var view alertSavedViewDTO
		var raw string
		if err = rows.Scan(&view.ViewID, &view.Name, &raw, &view.CreatedAt, &view.UpdatedAt); err != nil {
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
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"views": views, "total": len(views)})
}

func decodeAlertActionRequest(w http.ResponseWriter, r *http.Request) (alertWorkbenchActionRequest, bool) {
	var request alertWorkbenchActionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid alert action request")
		return request, false
	}
	request.Action, request.Target, request.Reason = strings.TrimSpace(request.Action), strings.TrimSpace(request.Target), strings.TrimSpace(request.Reason)
	if request.Detail == nil {
		request.Detail = map[string]interface{}{}
	}
	if request.Action == "" || request.Target == "" || len(request.Reason) < 4 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "action, target and reason (minimum 4 characters) are required")
		return request, false
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

func ensureAlertWorkbenchSchema(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS alert_saved_views (view_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id TEXT NOT NULL, name TEXT NOT NULL, filters JSONB NOT NULL DEFAULT '{}'::jsonb, created_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(tenant_id,name))`,
		`CREATE TABLE IF NOT EXISTS alert_response_actions (job_id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, alert_id TEXT NOT NULL, action TEXT NOT NULL, target TEXT NOT NULL, reason TEXT NOT NULL, dry_run BOOLEAN NOT NULL DEFAULT true, status TEXT NOT NULL, detail JSONB NOT NULL DEFAULT '{}'::jsonb, requested_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
		`CREATE TABLE IF NOT EXISTS alert_response_outbox (outbox_id BIGSERIAL PRIMARY KEY, job_id TEXT NOT NULL REFERENCES alert_response_actions(job_id) ON DELETE CASCADE, tenant_id TEXT NOT NULL, event_type TEXT NOT NULL, payload JSONB NOT NULL, published BOOLEAN NOT NULL DEFAULT false, attempts INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '', next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(), locked_until TIMESTAMPTZ, locked_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), published_at TIMESTAMPTZ)`,
		`ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now()`,
		`ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS locked_until TIMESTAMPTZ`,
		`ALTER TABLE alert_response_outbox ADD COLUMN IF NOT EXISTS locked_by TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_alert_response_outbox_retry ON alert_response_outbox (next_attempt_at, outbox_id) WHERE published=false`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
