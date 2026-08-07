package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"strconv"
	"strings"
	"time"

	alertservice "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/miniohttp"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

const (
	alertReportContractVersion = 1
	alertReportMaxDownloadSize = 50 << 20
)

type alertReportRequest struct {
	ActionID   string `json:"action_id"`
	Format     string `json:"format"`
	SnapshotID string `json:"snapshot_id"`
	Reason     string `json:"reason"`
}

type alertReportControlRequest struct {
	ActionID         string `json:"action_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

type AlertReportModel struct {
	SchemaVersion    int                          `json:"schema_version"`
	ContractVersion  int                          `json:"contract_version"`
	SnapshotID       string                       `json:"snapshot_id"`
	TenantID         string                       `json:"tenant_id"`
	AlertID          string                       `json:"alert_id"`
	Alert            *alertservice.AlertDetailDTO `json:"alert"`
	Evidence         []*alertservice.EvidenceDTO  `json:"evidence"`
	Assets           []map[string]interface{}     `json:"assets"`
	ResponseActions  []map[string]interface{}     `json:"response_actions"`
	AuditTrail       []map[string]interface{}     `json:"audit_trail"`
	MissingSections  []string                     `json:"missing_sections"`
	SourceWatermarks map[string]string            `json:"source_watermarks"`
}

type AlertReportBuilder interface {
	Build(context.Context, string, string, string) (AlertReportModel, error)
}

type defaultAlertReportBuilder struct {
	alerts *alertservice.AlertService
	db     *sql.DB
}

type AlertReportObjectStore interface {
	Put(context.Context, string, string, io.Reader, int64, string) error
	Open(context.Context, string, string) (io.ReadCloser, error)
	Remove(context.Context, string, string) error
}

type minioAlertReportObjectStore struct {
	client *minio.Client
}

type alertReportJob struct {
	JobID             string
	TenantID          string
	AlertID           string
	Format            string
	Status            string
	Revision          int64
	RequestedSnapshot string
	SnapshotSHA256    string
	MissingSections   []string
	SourceWatermarks  map[string]string
	ObjectBucket      string
	ObjectKey         string
	MIMEType          string
	ArtifactSHA256    string
	SizeBytes         int64
	ErrorMessage      string
	CreatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CompletedAt       *time.Time
	CancelRequestedAt *time.Time
	CancelledAt       *time.Time
}

func (h *Handler) CreateAlertReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.alertReportEnabled {
		http.NotFound(w, r)
		return
	}
	if !h.requireAlertExportPermission(w, r) {
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert report persistence is unavailable")
		return
	}
	tenantID := h.extractTenantID(r)
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	if tenantID == "" || alertID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "tenant and alert id are required")
		return
	}
	request, ok := decodeAlertReportRequest(w, r)
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain at least 16 characters")
		return
	}
	if existing, found, err := loadAlertReportByIdempotencyKey(ctx, h.actionAudit.db, tenantID, idempotencyKey); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to inspect report idempotency")
		return
	} else if found {
		if existing.AlertID != alertID || existing.Format != request.Format || existing.RequestedSnapshot != request.SnapshotID {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for a different report")
			return
		}
		writeAlertReportJobResponse(w, ctx, http.StatusAccepted, existing)
		return
	}

	builder := h.reportBuilder
	if builder == nil {
		if h.alertService == nil {
			httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "ALERT_SOURCE_UNAVAILABLE", "alert report source is unavailable")
			return
		}
		builder = &defaultAlertReportBuilder{alerts: h.alertService, db: h.actionAudit.db}
	}
	model, err := builder.Build(ctx, tenantID, alertID, request.SnapshotID)
	if err != nil {
		if commonerrors.IsCode(err, commonerrors.ErrCodeAlertNotFound) {
			httpx.JSONError(w, ctx, http.StatusNotFound, "ALERT_NOT_FOUND", "alert not found")
		} else {
			httpx.JSONError(w, ctx, http.StatusBadGateway, "REPORT_SOURCE_UNAVAILABLE", "failed to freeze alert report snapshot")
		}
		return
	}
	if model.Alert == nil || model.TenantID != tenantID || model.AlertID != alertID ||
		model.Alert.TenantID != tenantID || model.Alert.AlertID != alertID {
		httpx.JSONError(w, ctx, http.StatusBadGateway, "REPORT_MODEL_IDENTITY_MISMATCH", "report model identity does not match the requested tenant and alert")
		return
	}
	expectedSnapshotID := fmt.Sprintf("alert:%s:revision:%d", alertID, model.Alert.StateVersion)
	if request.SnapshotID != expectedSnapshotID || model.SnapshotID != expectedSnapshotID {
		httpx.JSONError(w, ctx, http.StatusConflict, "SNAPSHOT_CONFLICT", "alert revision changed; refresh the alert before exporting")
		return
	}
	snapshot, err := json.Marshal(model)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "REPORT_MODEL_FAILED", "failed to encode alert report snapshot")
		return
	}
	snapshotDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(snapshot))
	now := time.Now().UTC()
	job := alertReportJob{
		JobID: "alert-report-" + uuid.NewString(), TenantID: tenantID, AlertID: alertID,
		Format: request.Format, Status: "accepted", Revision: 1, RequestedSnapshot: request.SnapshotID,
		SnapshotSHA256: snapshotDigest, MissingSections: model.MissingSections,
		SourceWatermarks: model.SourceWatermarks, CreatedBy: h.extractUserID(r), CreatedAt: now, UpdatedAt: now,
	}
	missingJSON, _ := json.Marshal(model.MissingSections)
	watermarksJSON, _ := json.Marshal(model.SourceWatermarks)
	eventID := uuid.NewString()
	eventPayload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "alert_report", "aggregate_id": job.JobID, "aggregate_version": 1,
		"partition_key": tenantID + ":" + alertID, "alert_id": alertID, "format": request.Format,
		"snapshot_id": request.SnapshotID, "snapshot_sha256": snapshotDigest,
	})

	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin alert report transaction")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_jobs
		(job_id,tenant_id,alert_id,format,status,revision,idempotency_key,requested_snapshot_id,snapshot,snapshot_sha256,missing_sections,source_watermarks,created_by)
		VALUES ($1,$2,$3,$4,'accepted',1,$5,$6,$7::jsonb,$8,$9::jsonb,$10::jsonb,$11)`,
		job.JobID, tenantID, alertID, request.Format, idempotencyKey, request.SnapshotID,
		string(snapshot), snapshotDigest, string(missingJSON), string(watermarksJSON), job.CreatedBy); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert report job")
		return
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_job_history
		(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail)
		VALUES ($1,$2,'','accepted',1,$3,$4,$5,$6::jsonb)`,
		job.JobID, tenantID, job.CreatedBy, request.Reason, httpx.GetTraceID(ctx), `{"action_id":"alert-report-export"}`); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert report history")
		return
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_outbox
		(event_id,job_id,tenant_id,event_type,aggregate_version,partition_key,payload)
		VALUES ($1::uuid,$2,$3,'traffic.alert.v2.AlertReportRequested',1,$4,$5::jsonb)`,
		eventID, job.JobID, tenantID, tenantID+":"+alertID, string(eventPayload)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to enqueue alert report event")
		return
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: "ALERT_REPORT_EXPORT_REQUESTED", ObjectType: "alert_report", ObjectID: job.JobID,
		TenantID: tenantID, UserID: job.CreatedBy, AlertID: alertID, Reason: request.Reason, Result: "accepted",
		Detail: map[string]interface{}{
			"action_id": request.ActionID, "format": request.Format, "snapshot_id": request.SnapshotID,
			"snapshot_sha256": snapshotDigest, "event_id": eventID, "idempotency_key_sha256": opaqueKeyDigest(idempotencyKey),
		},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit alert report request")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit alert report job")
		return
	}
	writeAlertReportJobResponse(w, ctx, http.StatusAccepted, job)
}

func (h *Handler) GetAlertReportJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.alertReportEnabled {
		http.NotFound(w, r)
		return
	}
	if !h.requireAlertExportPermission(w, r) {
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert report persistence is unavailable")
		return
	}
	job, err := loadAlertReportJob(ctx, h.actionAudit.db, h.extractTenantID(r), strings.TrimSpace(mux.Vars(r)["job_id"]))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && job.AlertID != strings.TrimSpace(mux.Vars(r)["id"])) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "REPORT_NOT_FOUND", "alert report job not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load alert report job")
		return
	}
	writeAlertReportJobResponse(w, ctx, http.StatusOK, job)
}

func (h *Handler) CancelAlertReport(w http.ResponseWriter, r *http.Request) {
	h.controlAlertReport(w, r, "cancel", "alert-report-cancel")
}

func (h *Handler) CompensateAlertReport(w http.ResponseWriter, r *http.Request) {
	h.controlAlertReport(w, r, "compensate", "alert-report-compensate")
}

func (h *Handler) controlAlertReport(w http.ResponseWriter, r *http.Request, operation, actionID string) {
	ctx := r.Context()
	if !h.alertReportEnabled {
		http.NotFound(w, r)
		return
	}
	if !h.requireAlertExportPermission(w, r) {
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert report persistence is unavailable")
		return
	}
	tenantID := h.extractTenantID(r)
	alertID := strings.TrimSpace(mux.Vars(r)["id"])
	jobID := strings.TrimSpace(mux.Vars(r)["job_id"])
	request, ok := decodeAlertReportControlRequest(w, r, actionID)
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must contain 16-200 characters")
		return
	}
	if tenantID == "" || alertID == "" || jobID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "tenant, alert id and report job id are required")
		return
	}
	requestPayload, _ := json.Marshal(map[string]interface{}{
		"operation": operation, "alert_id": alertID, "job_id": jobID,
		"expected_revision": request.ExpectedRevision, "reason": request.Reason,
	})
	requestDigest := fmt.Sprintf("%x", sha256.Sum256(requestPayload))
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to begin alert report control request")
		return
	}
	defer tx.Rollback()
	var replayJobID, replayHash string
	err = tx.QueryRowContext(ctx, `SELECT job_id,request_hash FROM alert_report_control_requests
		WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, idempotencyKey).Scan(&replayJobID, &replayHash)
	if err == nil {
		if replayJobID != jobID || replayHash != requestDigest {
			httpx.JSONError(w, ctx, http.StatusConflict, "IDEMPOTENCY_KEY_CONFLICT", "Idempotency-Key was already used for another report control request")
			return
		}
		replayed, loadErr := scanAlertReportJob(tx.QueryRowContext(ctx, alertReportJobSelect+` WHERE tenant_id=$1 AND job_id=$2`, tenantID, jobID))
		if loadErr != nil || replayed.AlertID != alertID {
			httpx.JSONError(w, ctx, http.StatusNotFound, "REPORT_NOT_FOUND", "alert report job not found")
			return
		}
		writeAlertReportJobResponse(w, ctx, http.StatusAccepted, replayed)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to inspect report control idempotency")
		return
	}
	current, err := scanAlertReportJob(tx.QueryRowContext(ctx, alertReportJobSelect+` WHERE tenant_id=$1 AND job_id=$2 FOR UPDATE`, tenantID, jobID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && current.AlertID != alertID) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "REPORT_NOT_FOUND", "alert report job not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load alert report job")
		return
	}
	if current.Revision != request.ExpectedRevision {
		httpx.JSONError(w, ctx, http.StatusConflict, "REVISION_CONFLICT", "alert report revision changed; refresh before retrying the control request")
		return
	}
	nextStatus := ""
	if operation == "cancel" {
		switch current.Status {
		case "accepted":
			nextStatus = "cancelled"
		case "running":
			nextStatus = "cancel_requested"
		default:
			httpx.JSONError(w, ctx, http.StatusConflict, "REPORT_STATE_CONFLICT", "only accepted or running alert reports can be cancelled")
			return
		}
	} else {
		if (current.Status != "partial" && current.Status != "compensation_failed") || current.ObjectKey == "" {
			httpx.JSONError(w, ctx, http.StatusConflict, "REPORT_STATE_CONFLICT", "compensation requires a partial cancellation with a residual object manifest")
			return
		}
		nextStatus = "compensating"
	}
	nextRevision := current.Revision + 1
	now := time.Now().UTC()
	completedAt := interface{}(nil)
	if nextStatus == "cancelled" {
		completedAt = now
		current.CompletedAt = &now
		current.CancelledAt = &now
	} else if nextStatus == "cancel_requested" {
		current.CancelRequestedAt = &now
	}
	if _, err = tx.ExecContext(ctx, `UPDATE alert_report_jobs SET status=$3,revision=$4,cancellation_reason=$5,
		cancel_requested_at=COALESCE(cancel_requested_at,$6),cancelled_at=CASE WHEN $3='cancelled' THEN $6 ELSE cancelled_at END,
		completed_at=COALESCE($7,completed_at),updated_at=$6 WHERE tenant_id=$1 AND job_id=$2`,
		tenantID, jobID, nextStatus, nextRevision, request.Reason, now, completedAt); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to update alert report control state")
		return
	}
	detailJSON, _ := json.Marshal(map[string]interface{}{"expected_revision": request.ExpectedRevision})
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_job_history
		(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`, jobID, tenantID, current.Status, nextStatus,
		nextRevision, h.extractUserID(r), request.Reason, httpx.GetTraceID(ctx), string(detailJSON)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert report control history")
		return
	}
	eventID := uuid.NewString()
	eventType := "traffic.alert.v2.AlertReportCancelRequested"
	if nextStatus == "cancelled" {
		eventType = "traffic.alert.v2.AlertReportCancelled"
	} else if nextStatus == "compensating" {
		eventType = "traffic.alert.v2.AlertReportCompensationRequested"
	}
	eventPayload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "alert_report", "aggregate_id": jobID, "aggregate_version": nextRevision,
		"partition_key": tenantID + ":" + alertID, "alert_id": alertID, "status": nextStatus,
		"reason": request.Reason, "trace_id": httpx.GetTraceID(ctx),
	})
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_outbox
		(event_id,job_id,tenant_id,event_type,aggregate_version,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)`, eventID, jobID, tenantID, eventType,
		nextRevision, tenantID+":"+alertID, string(eventPayload)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to enqueue alert report control event")
		return
	}
	resultPayload, _ := json.Marshal(map[string]interface{}{"job_id": jobID, "status": nextStatus, "revision": nextRevision})
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_control_requests
		(request_id,tenant_id,job_id,operation,idempotency_key,request_hash,expected_revision,resulting_revision,result_payload,actor,reason,trace_id)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12)`, uuid.NewString(), tenantID, jobID,
		operation, idempotencyKey, requestDigest, request.ExpectedRevision, nextRevision, string(resultPayload), h.extractUserID(r), request.Reason, httpx.GetTraceID(ctx)); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to persist alert report control request")
		return
	}
	auditAction := "ALERT_REPORT_EXPORT_CANCEL_REQUESTED"
	if operation == "compensate" {
		auditAction = "ALERT_REPORT_EXPORT_COMPENSATION_REQUESTED"
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, r, AlertActionAuditRecord{
		Action: auditAction, ObjectType: "alert_report", ObjectID: jobID,
		TenantID: tenantID, UserID: h.extractUserID(r), AlertID: alertID, OldStatus: current.Status,
		NewStatus: nextStatus, Reason: request.Reason, Result: nextStatus,
		Detail: map[string]interface{}{"action_id": request.ActionID, "revision": nextRevision, "event_id": eventID},
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to audit alert report control request")
		return
	}
	if err = tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to commit alert report control request")
		return
	}
	current.Status = nextStatus
	current.Revision = nextRevision
	current.UpdatedAt = now
	writeAlertReportJobResponse(w, ctx, http.StatusAccepted, current)
}

func (h *Handler) DownloadAlertReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.alertReportEnabled {
		http.NotFound(w, r)
		return
	}
	if !h.requireAlertExportPermission(w, r) {
		return
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "PERSISTENCE_UNAVAILABLE", "alert report persistence is unavailable")
		return
	}
	job, err := loadAlertReportJob(ctx, h.actionAudit.db, h.extractTenantID(r), strings.TrimSpace(mux.Vars(r)["job_id"]))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && job.AlertID != strings.TrimSpace(mux.Vars(r)["id"])) {
		httpx.JSONError(w, ctx, http.StatusNotFound, "REPORT_NOT_FOUND", "alert report job not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "PERSISTENCE_FAILED", "failed to load alert report job")
		return
	}
	if job.Status != "completed" || job.ObjectKey == "" {
		httpx.JSONError(w, ctx, http.StatusConflict, "REPORT_NOT_READY", "alert report is not completed")
		return
	}
	store, err := h.alertReportObjectStore()
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "REPORT_STORAGE_UNAVAILABLE", "alert report object storage is unavailable")
		return
	}
	reader, err := store.Open(ctx, job.ObjectBucket, job.ObjectKey)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusBadGateway, "REPORT_STORAGE_FAILED", "failed to open alert report artifact")
		return
	}
	defer reader.Close()
	if job.SizeBytes < 0 || job.SizeBytes > alertReportMaxDownloadSize {
		httpx.JSONError(w, ctx, http.StatusUnprocessableEntity, "REPORT_MANIFEST_INVALID", "alert report size is outside the download budget")
		return
	}
	content, err := io.ReadAll(io.LimitReader(reader, alertReportMaxDownloadSize+1))
	if err != nil || int64(len(content)) != job.SizeBytes || int64(len(content)) > alertReportMaxDownloadSize {
		httpx.JSONError(w, ctx, http.StatusBadGateway, "REPORT_MANIFEST_MISMATCH", "alert report size does not match its manifest")
		return
	}
	actualDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	if actualDigest != job.ArtifactSHA256 {
		httpx.JSONError(w, ctx, http.StatusBadGateway, "REPORT_MANIFEST_MISMATCH", "alert report checksum does not match its manifest")
		return
	}
	h.recordAlertActionAudit(ctx, r, AlertActionAuditRecord{
		Action: "ALERT_REPORT_DOWNLOADED", ObjectType: "alert_report", ObjectID: job.JobID,
		TenantID: job.TenantID, UserID: h.extractUserID(r), AlertID: job.AlertID, Result: "success",
		Detail: map[string]interface{}{"artifact_sha256": job.ArtifactSHA256, "size_bytes": job.SizeBytes},
	})
	filename := pathpkg.Base(job.ObjectKey)
	w.Header().Set("Content-Type", job.MIMEType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-Content-SHA256", job.ArtifactSHA256)
	http.ServeContent(w, r, filename, job.UpdatedAt, bytes.NewReader(content))
}

func (h *Handler) StartAlertReportWorker(ctx context.Context, interval time.Duration) error {
	if !h.alertReportEnabled {
		return nil
	}
	if h.actionAudit == nil || h.actionAudit.db == nil {
		return fmt.Errorf("alert report database is unavailable")
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := h.processNextAlertReport(ctx); err != nil && ctx.Err() == nil && h.logger != nil {
				h.logger.Warn("Failed to process alert report job", zap.Error(err))
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

func (h *Handler) processNextAlertReport(ctx context.Context) error {
	workerID := fmt.Sprintf("%s-%d", hostnameOrDefault(), os.Getpid())
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var job alertReportJob
	var snapshot []byte
	var previousStatus string
	err = tx.QueryRowContext(ctx, `WITH candidate AS (
			SELECT job_id,status FROM alert_report_jobs
			WHERE (status='accepted' AND next_attempt_at <= now())
			   OR (status='running' AND locked_until < now())
			   OR (status='cancel_requested' AND next_attempt_at <= now() AND (locked_until IS NULL OR locked_until < now()))
			   OR (status='compensating' AND next_attempt_at <= now() AND (locked_until IS NULL OR locked_until < now()))
			ORDER BY created_at
			LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		UPDATE alert_report_jobs j
		SET status=CASE WHEN c.status IN ('cancel_requested','compensating') THEN c.status ELSE 'running' END,
		    revision=CASE WHEN c.status IN ('cancel_requested','compensating') THEN j.revision ELSE j.revision+1 END,
		    attempts=j.attempts+1,locked_until=now()+interval '5 minutes',
		    locked_by=$1,updated_at=now()
		FROM candidate c WHERE j.job_id=c.job_id
		RETURNING j.job_id,j.tenant_id,j.alert_id,j.format,j.status,j.revision,j.snapshot::text,j.snapshot_sha256,
		          j.object_bucket,j.object_key,j.mime_type,j.artifact_sha256,j.size_bytes,j.created_by,c.status`, workerID).
		Scan(&job.JobID, &job.TenantID, &job.AlertID, &job.Format, &job.Status, &job.Revision, &snapshot,
			&job.SnapshotSHA256, &job.ObjectBucket, &job.ObjectKey, &job.MIMEType, &job.ArtifactSHA256, &job.SizeBytes, &job.CreatedBy, &previousStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if job.Status == "running" {
		runningEventID := uuid.NewString()
		runningPayload, _ := json.Marshal(map[string]interface{}{
			"event_id": runningEventID, "tenant_id": job.TenantID, "schema_version": 2,
			"aggregate_type": "alert_report", "aggregate_id": job.JobID, "aggregate_version": job.Revision,
			"partition_key": job.TenantID + ":" + job.AlertID, "alert_id": job.AlertID, "status": "running",
		})
		if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_job_history
			(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail)
			VALUES ($1,$2,$3,'running',$4,$5,'worker lease acquired','',$6::jsonb)`,
			job.JobID, job.TenantID, previousStatus, job.Revision, workerID, `{"lease_seconds":300}`); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_outbox
			(event_id,job_id,tenant_id,event_type,aggregate_version,partition_key,payload)
			VALUES ($1::uuid,$2,$3,'traffic.alert.v2.AlertReportRunning',$4,$5,$6::jsonb)`, runningEventID,
			job.JobID, job.TenantID, job.Revision, job.TenantID+":"+job.AlertID, string(runningPayload)); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	store, err := h.alertReportObjectStore()
	if err != nil {
		if job.Status == "cancel_requested" || job.Status == "compensating" {
			return h.failAlertReportCancellation(ctx, job, err)
		}
		return h.failAlertReportJob(ctx, job.JobID, err)
	}
	if job.Status == "cancel_requested" || job.Status == "compensating" {
		return h.cleanupAlertReportObject(ctx, job, store, workerID)
	}

	var frozenModel AlertReportModel
	if err = json.Unmarshal(snapshot, &frozenModel); err != nil {
		return h.failAlertReportJob(ctx, job.JobID, fmt.Errorf("invalid frozen report model: %w", err))
	}
	canonicalSnapshot, err := json.Marshal(frozenModel)
	if err != nil {
		return h.failAlertReportJob(ctx, job.JobID, fmt.Errorf("failed to canonicalize frozen report model: %w", err))
	}
	if fmt.Sprintf("sha256:%x", sha256.Sum256(canonicalSnapshot)) != job.SnapshotSHA256 {
		return h.failAlertReportJob(ctx, job.JobID, fmt.Errorf("frozen report snapshot checksum mismatch"))
	}
	content, mimeType, extension, err := buildAlertReportArtifact(job.Format, canonicalSnapshot)
	if err != nil {
		return h.failAlertReportJob(ctx, job.JobID, err)
	}
	job.ObjectBucket = alertReportBucket()
	job.ObjectKey = pathpkg.Join(safeObjectSegment(job.TenantID), "alerts", safeObjectSegment(job.AlertID), job.JobID+"."+extension)
	job.MIMEType = mimeType
	job.SizeBytes = int64(len(content))
	job.ArtifactSHA256 = fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	if err = store.Put(ctx, job.ObjectBucket, job.ObjectKey, bytes.NewReader(content), job.SizeBytes, mimeType); err != nil {
		return h.failAlertReportJob(ctx, job.JobID, err)
	}

	completeTx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer completeTx.Rollback()
	var completedRevision int64
	err = completeTx.QueryRowContext(ctx, `UPDATE alert_report_jobs SET status='completed',revision=revision+1,
		object_bucket=$2,object_key=$3,mime_type=$4,artifact_sha256=$5,size_bytes=$6,error_message='',
		locked_until=NULL,locked_by='',updated_at=now(),completed_at=now()
		WHERE job_id=$1 AND status='running' AND locked_by=$7 RETURNING revision`,
		job.JobID, job.ObjectBucket, job.ObjectKey, job.MIMEType, job.ArtifactSHA256, job.SizeBytes, workerID).Scan(&completedRevision)
	if errors.Is(err, sql.ErrNoRows) {
		_ = completeTx.Rollback()
		current, loadErr := loadAlertReportJob(ctx, h.actionAudit.db, job.TenantID, job.JobID)
		if loadErr == nil && current.Status == "cancel_requested" {
			current.ObjectBucket, current.ObjectKey, current.MIMEType = job.ObjectBucket, job.ObjectKey, job.MIMEType
			current.ArtifactSHA256, current.SizeBytes = job.ArtifactSHA256, job.SizeBytes
			return h.cleanupAlertReportObject(ctx, current, store, workerID)
		}
		return fmt.Errorf("alert report lease lost before manifest commit")
	}
	if err != nil {
		return err
	}
	completedEventID := uuid.NewString()
	completedPayload, _ := json.Marshal(map[string]interface{}{
		"event_id": completedEventID, "tenant_id": job.TenantID, "schema_version": 2,
		"aggregate_type": "alert_report", "aggregate_id": job.JobID, "aggregate_version": completedRevision,
		"partition_key": job.TenantID + ":" + job.AlertID, "alert_id": job.AlertID,
		"snapshot_sha256": job.SnapshotSHA256, "artifact_sha256": job.ArtifactSHA256,
		"size_bytes": job.SizeBytes, "object_bucket": job.ObjectBucket, "object_key": job.ObjectKey,
	})
	if _, err = completeTx.ExecContext(ctx, `INSERT INTO alert_report_outbox
		(event_id,job_id,tenant_id,event_type,aggregate_version,partition_key,payload)
		VALUES ($1::uuid,$2,$3,'traffic.alert.v2.AlertReportCompleted',$4,$5,$6::jsonb)`,
		completedEventID, job.JobID, job.TenantID, completedRevision, job.TenantID+":"+job.AlertID, string(completedPayload)); err != nil {
		return err
	}
	if _, err = completeTx.ExecContext(ctx, `INSERT INTO alert_report_job_history
		(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail)
		VALUES ($1,$2,'running','completed',$3,$4,'report artifact committed','',$5::jsonb)`, job.JobID,
		job.TenantID, completedRevision, workerID, `{"manifest_committed":true}`); err != nil {
		return err
	}
	if err = h.actionAudit.recordWithExecutor(ctx, completeTx, nil, AlertActionAuditRecord{
		Action: "ALERT_REPORT_EXPORT_COMPLETED", ObjectType: "alert_report", ObjectID: job.JobID,
		TenantID: job.TenantID, UserID: job.CreatedBy, AlertID: job.AlertID, Result: "completed",
		Detail: map[string]interface{}{
			"snapshot_sha256": job.SnapshotSHA256, "artifact_sha256": job.ArtifactSHA256,
			"size_bytes": job.SizeBytes, "object_bucket": job.ObjectBucket, "object_key": job.ObjectKey,
			"event_id": completedEventID,
		},
	}); err != nil {
		return err
	}
	return completeTx.Commit()
}

func (h *Handler) cleanupAlertReportObject(ctx context.Context, job alertReportJob, store AlertReportObjectStore, workerID string) error {
	if job.ObjectBucket == "" {
		job.ObjectBucket = alertReportBucket()
	}
	if job.ObjectKey == "" {
		job.ObjectKey = pathpkg.Join(safeObjectSegment(job.TenantID), "alerts", safeObjectSegment(job.AlertID), job.JobID+"."+job.Format)
	}
	if err := store.Remove(ctx, job.ObjectBucket, job.ObjectKey); err != nil {
		return h.failAlertReportCancellation(ctx, job, err)
	}
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentRevision int64
	var reason string
	fromStatus := job.Status
	terminalStatus := "cancelled"
	eventType := "traffic.alert.v2.AlertReportCancelled"
	auditAction := "ALERT_REPORT_EXPORT_CANCELLED"
	if fromStatus == "compensating" {
		terminalStatus = "compensated"
		eventType = "traffic.alert.v2.AlertReportCompensated"
		auditAction = "ALERT_REPORT_EXPORT_COMPENSATED"
	}
	err = tx.QueryRowContext(ctx, `SELECT revision,cancellation_reason FROM alert_report_jobs
		WHERE tenant_id=$1 AND job_id=$2 AND status=$3 FOR UPDATE`, job.TenantID, job.JobID, fromStatus).Scan(&currentRevision, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	nextRevision := currentRevision + 1
	if _, err = tx.ExecContext(ctx, `UPDATE alert_report_jobs SET status=$3,revision=$4,
		object_bucket='',object_key='',mime_type='',artifact_sha256='',size_bytes=0,error_message='',
		locked_until=NULL,locked_by='',updated_at=now(),completed_at=now(),cancelled_at=now()
		WHERE tenant_id=$1 AND job_id=$2 AND status=$5`, job.TenantID, job.JobID, terminalStatus, nextRevision, fromStatus); err != nil {
		return err
	}
	eventID := uuid.NewString()
	payload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID, "tenant_id": job.TenantID, "schema_version": 2,
		"aggregate_type": "alert_report", "aggregate_id": job.JobID, "aggregate_version": nextRevision,
		"partition_key": job.TenantID + ":" + job.AlertID, "alert_id": job.AlertID, "status": terminalStatus,
		"temporary_object_removed": true,
	})
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_outbox
		(event_id,job_id,tenant_id,event_type,aggregate_version,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)`, eventID,
		job.JobID, job.TenantID, eventType, nextRevision, job.TenantID+":"+job.AlertID, string(payload)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_job_history
		(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'',$8::jsonb)`, job.JobID, job.TenantID,
		fromStatus, terminalStatus, nextRevision, workerID, reason, `{"temporary_object_removed":true}`); err != nil {
		return err
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, nil, AlertActionAuditRecord{
		Action: auditAction, ObjectType: "alert_report", ObjectID: job.JobID,
		TenantID: job.TenantID, UserID: job.CreatedBy, AlertID: job.AlertID, OldStatus: fromStatus,
		NewStatus: terminalStatus, Reason: reason, Result: terminalStatus,
		Detail: map[string]interface{}{"revision": nextRevision, "event_id": eventID, "temporary_object_removed": true},
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (h *Handler) failAlertReportCancellation(ctx context.Context, job alertReportJob, cause error) error {
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("report cancellation cleanup failure %v; transaction failed: %w", cause, err)
	}
	defer tx.Rollback()
	var tenantID, alertID, currentStatus, createdBy, cancellationReason string
	var currentRevision int64
	var attempts int
	err = tx.QueryRowContext(ctx, `SELECT tenant_id,alert_id,status,revision,attempts,created_by,cancellation_reason
		FROM alert_report_jobs WHERE job_id=$1 AND status=$2 FOR UPDATE`, job.JobID, job.Status).
		Scan(&tenantID, &alertID, &currentStatus, &currentRevision, &attempts, &createdBy, &cancellationReason)
	if errors.Is(err, sql.ErrNoRows) {
		return cause
	}
	if err != nil {
		return fmt.Errorf("report cancellation cleanup failure %v; state load failed: %w", cause, err)
	}
	terminalFailure := "partial"
	if job.Status == "compensating" {
		terminalFailure = "compensation_failed"
	}
	nextStatus := currentStatus
	if attempts >= 5 {
		nextStatus = terminalFailure
	}
	nextRevision := currentRevision + 1
	_, updateErr := tx.ExecContext(ctx, `UPDATE alert_report_jobs SET
		status=$8,revision=$9,object_bucket=$2,object_key=$3,mime_type=$4,artifact_sha256=$5,size_bytes=$6,
		error_message=$7,next_attempt_at=CASE WHEN attempts < 5
		  THEN now()+(LEAST(300,POWER(2,LEAST(attempts,8)))::text || ' seconds')::interval ELSE next_attempt_at END,
		locked_until=NULL,locked_by='',updated_at=now(),completed_at=CASE WHEN attempts < 5 THEN NULL ELSE now() END
		WHERE job_id=$1 AND status=$10`, job.JobID, job.ObjectBucket, job.ObjectKey,
		job.MIMEType, job.ArtifactSHA256, job.SizeBytes, message, nextStatus, nextRevision, currentStatus)
	if updateErr != nil {
		return fmt.Errorf("report cancellation cleanup failure %v; status update failed: %w", cause, updateErr)
	}
	detailJSON, _ := json.Marshal(map[string]interface{}{"cleanup_error": message, "attempts": attempts})
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_job_history
		(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail)
		VALUES ($1,$2,$3,$4,$5,'report-worker',$6,'',$7::jsonb)`, job.JobID, tenantID, currentStatus,
		nextStatus, nextRevision, cancellationReason, string(detailJSON)); err != nil {
		return fmt.Errorf("report cancellation cleanup failure %v; history failed: %w", cause, err)
	}
	eventID := uuid.NewString()
	eventType := "traffic.alert.v2.AlertReportCancellationRetryScheduled"
	if nextStatus == "partial" {
		eventType = "traffic.alert.v2.AlertReportCancellationPartial"
	} else if nextStatus == "compensation_failed" {
		eventType = "traffic.alert.v2.AlertReportCompensationFailed"
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "alert_report", "aggregate_id": job.JobID, "aggregate_version": nextRevision,
		"partition_key": tenantID + ":" + alertID, "alert_id": alertID, "status": nextStatus,
		"error": message, "attempts": attempts,
	})
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_outbox
		(event_id,job_id,tenant_id,event_type,aggregate_version,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)`, eventID, job.JobID, tenantID, eventType,
		nextRevision, tenantID+":"+alertID, string(payload)); err != nil {
		return fmt.Errorf("report cancellation cleanup failure %v; outbox failed: %w", cause, err)
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, nil, AlertActionAuditRecord{
		Action: "ALERT_REPORT_EXPORT_CLEANUP_FAILED", ObjectType: "alert_report", ObjectID: job.JobID,
		TenantID: tenantID, UserID: createdBy, AlertID: alertID, OldStatus: currentStatus, NewStatus: nextStatus,
		Reason: cancellationReason, Result: nextStatus,
		Detail: map[string]interface{}{"revision": nextRevision, "event_id": eventID, "error": message, "attempts": attempts},
	}); err != nil {
		return fmt.Errorf("report cancellation cleanup failure %v; audit failed: %w", cause, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("report cancellation cleanup failure %v; commit failed: %w", cause, err)
	}
	return cause
}

func (h *Handler) failAlertReportJob(ctx context.Context, jobID string, cause error) error {
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	tx, err := h.actionAudit.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("report failure %v; transaction failed: %w", cause, err)
	}
	defer tx.Rollback()
	var tenantID, alertID, currentStatus, createdBy string
	var currentRevision int64
	var attempts int
	err = tx.QueryRowContext(ctx, `SELECT tenant_id,alert_id,status,revision,attempts,created_by
		FROM alert_report_jobs WHERE job_id=$1 AND status IN ('accepted','running') FOR UPDATE`, jobID).
		Scan(&tenantID, &alertID, &currentStatus, &currentRevision, &attempts, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return cause
	}
	if err != nil {
		return fmt.Errorf("report failure %v; state load failed: %w", cause, err)
	}
	nextStatus := "accepted"
	if attempts >= 5 {
		nextStatus = "failed"
	}
	nextRevision := currentRevision + 1
	_, updateErr := tx.ExecContext(ctx, `UPDATE alert_report_jobs
		SET status=$3,revision=$4,
		    error_message=$2,
		    next_attempt_at=CASE WHEN attempts < 5
		      THEN now()+(LEAST(300,POWER(2,LEAST(attempts,8)))::text || ' seconds')::interval
		      ELSE next_attempt_at END,
		    locked_until=NULL,locked_by='',updated_at=now(),
		    completed_at=CASE WHEN attempts < 5 THEN NULL ELSE now() END
		WHERE job_id=$1 AND status=$5`, jobID, message, nextStatus, nextRevision, currentStatus)
	if updateErr != nil {
		return fmt.Errorf("report failure %v; status update failed: %w", cause, updateErr)
	}
	detailJSON, _ := json.Marshal(map[string]interface{}{"error": message, "attempts": attempts})
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_job_history
		(job_id,tenant_id,from_status,to_status,revision,actor,reason,trace_id,detail)
		VALUES ($1,$2,$3,$4,$5,'report-worker',$6,'',$7::jsonb)`, jobID, tenantID, currentStatus,
		nextStatus, nextRevision, message, string(detailJSON)); err != nil {
		return fmt.Errorf("report failure %v; history failed: %w", cause, err)
	}
	eventID := uuid.NewString()
	eventType := "traffic.alert.v2.AlertReportRetryScheduled"
	if nextStatus == "failed" {
		eventType = "traffic.alert.v2.AlertReportFailed"
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"event_id": eventID, "tenant_id": tenantID, "schema_version": 2,
		"aggregate_type": "alert_report", "aggregate_id": jobID, "aggregate_version": nextRevision,
		"partition_key": tenantID + ":" + alertID, "alert_id": alertID, "status": nextStatus,
		"error": message, "attempts": attempts,
	})
	if _, err = tx.ExecContext(ctx, `INSERT INTO alert_report_outbox
		(event_id,job_id,tenant_id,event_type,aggregate_version,partition_key,payload)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7::jsonb)`, eventID, jobID, tenantID, eventType,
		nextRevision, tenantID+":"+alertID, string(payload)); err != nil {
		return fmt.Errorf("report failure %v; outbox failed: %w", cause, err)
	}
	auditAction := "ALERT_REPORT_EXPORT_RETRY_SCHEDULED"
	if nextStatus == "failed" {
		auditAction = "ALERT_REPORT_EXPORT_FAILED"
	}
	if err = h.actionAudit.recordWithExecutor(ctx, tx, nil, AlertActionAuditRecord{
		Action: auditAction, ObjectType: "alert_report", ObjectID: jobID,
		TenantID: tenantID, UserID: createdBy, AlertID: alertID, OldStatus: currentStatus, NewStatus: nextStatus,
		Reason: message, Result: nextStatus,
		Detail: map[string]interface{}{"revision": nextRevision, "event_id": eventID, "attempts": attempts},
	}); err != nil {
		return fmt.Errorf("report failure %v; audit failed: %w", cause, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("report failure %v; commit failed: %w", cause, err)
	}
	return cause
}

func (b *defaultAlertReportBuilder) Build(ctx context.Context, tenantID, alertID, snapshotID string) (AlertReportModel, error) {
	alert, err := b.alerts.GetAlert(ctx, tenantID, alertID)
	if err != nil {
		return AlertReportModel{}, err
	}
	evidence, err := b.alerts.GetEvidence(ctx, tenantID, alertID)
	if err != nil {
		return AlertReportModel{}, err
	}
	model := AlertReportModel{
		SchemaVersion: 2, ContractVersion: alertReportContractVersion, SnapshotID: snapshotID,
		TenantID: tenantID, AlertID: alertID, Alert: alert, Evidence: evidence,
		Assets: []map[string]interface{}{}, ResponseActions: []map[string]interface{}{},
		AuditTrail: []map[string]interface{}{}, MissingSections: []string{},
		SourceWatermarks: map[string]string{
			"clickhouse.alerts.state_version": strconv.FormatUint(alert.StateVersion, 10),
			"clickhouse.alerts.updated_at":    alert.UpdatedAt.UTC().Format(time.RFC3339Nano),
		},
	}
	if len(evidence) > 0 {
		latest := evidence[0].Timestamp
		for _, item := range evidence[1:] {
			if item.Timestamp.After(latest) {
				latest = item.Timestamp
			}
		}
		model.SourceWatermarks["clickhouse.evidence.updated_at"] = latest.UTC().Format(time.RFC3339Nano)
	} else {
		model.SourceWatermarks["clickhouse.evidence.updated_at"] = "empty"
	}
	b.loadAssets(ctx, &model)
	b.loadResponseActions(ctx, &model)
	b.loadAudit(ctx, &model)
	return model, nil
}

func (b *defaultAlertReportBuilder) loadAssets(ctx context.Context, model *AlertReportModel) {
	rows, err := b.db.QueryContext(ctx, `SELECT asset_id::text,display_code,ip_address,hostname,asset_type,status,criticality,updated_at
		FROM assets WHERE tenant_id=$1 AND ip_address=ANY($2::text[]) ORDER BY ip_address,asset_id`,
		model.TenantID, "{"+model.Alert.SrcIP+","+model.Alert.DstIP+"}")
	if err != nil {
		model.MissingSections = append(model.MissingSections, "asset_context")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var assetID string
		var displayCode, ipAddress, hostname, assetType, status sql.NullString
		var criticality sql.NullInt64
		var updatedAt time.Time
		if err := rows.Scan(&assetID, &displayCode, &ipAddress, &hostname, &assetType, &status, &criticality, &updatedAt); err != nil {
			model.MissingSections = appendUnique(model.MissingSections, "asset_context")
			return
		}
		model.Assets = append(model.Assets, map[string]interface{}{
			"asset_id": assetID, "display_code": displayCode.String, "ip_address": ipAddress.String,
			"hostname": hostname.String, "asset_type": assetType.String, "status": status.String,
			"criticality": criticality.Int64, "updated_at": updatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	if rows.Err() != nil || len(model.Assets) == 0 {
		model.MissingSections = appendUnique(model.MissingSections, "asset_context")
	}
}

func (b *defaultAlertReportBuilder) loadResponseActions(ctx context.Context, model *AlertReportModel) {
	rows, err := b.db.QueryContext(ctx, `SELECT job_id,action,target,reason,dry_run,status,created_at,updated_at
		FROM alert_response_actions WHERE tenant_id=$1 AND alert_id=$2 ORDER BY created_at,job_id`, model.TenantID, model.AlertID)
	if err != nil {
		model.MissingSections = appendUnique(model.MissingSections, "response_actions")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var jobID, action, target, reason, status string
		var dryRun bool
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&jobID, &action, &target, &reason, &dryRun, &status, &createdAt, &updatedAt); err != nil {
			model.MissingSections = appendUnique(model.MissingSections, "response_actions")
			return
		}
		model.ResponseActions = append(model.ResponseActions, map[string]interface{}{
			"job_id": jobID, "action": action, "target": target, "reason": reason, "dry_run": dryRun,
			"status": status, "created_at": createdAt.UTC().Format(time.RFC3339Nano), "updated_at": updatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	if rows.Err() != nil {
		model.MissingSections = appendUnique(model.MissingSections, "response_actions")
	}
}

func (b *defaultAlertReportBuilder) loadAudit(ctx context.Context, model *AlertReportModel) {
	rows, err := b.db.QueryContext(ctx, `SELECT action,object_type,object_id,detail::text,created_at
		FROM audit_logs WHERE tenant_id=$1 AND (object_id=$2 OR detail->>'alert_id'=$2)
		ORDER BY created_at,action LIMIT 1000`, model.TenantID, model.AlertID)
	if err != nil {
		model.MissingSections = appendUnique(model.MissingSections, "audit_trail")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var action, objectType, objectID, detailJSON string
		var createdAt time.Time
		if err := rows.Scan(&action, &objectType, &objectID, &detailJSON, &createdAt); err != nil {
			model.MissingSections = appendUnique(model.MissingSections, "audit_trail")
			return
		}
		var detail map[string]interface{}
		_ = json.Unmarshal([]byte(detailJSON), &detail)
		model.AuditTrail = append(model.AuditTrail, map[string]interface{}{
			"action": action, "object_type": objectType, "object_id": objectID,
			"detail": detail, "created_at": createdAt.UTC().Format(time.RFC3339Nano),
		})
	}
	if rows.Err() != nil {
		model.MissingSections = appendUnique(model.MissingSections, "audit_trail")
	}
}

func decodeAlertReportRequest(w http.ResponseWriter, r *http.Request) (alertReportRequest, bool) {
	var request alertReportRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid alert report request")
		return request, false
	}
	request.ActionID = strings.TrimSpace(request.ActionID)
	request.Format = strings.ToLower(strings.TrimSpace(request.Format))
	request.SnapshotID = strings.TrimSpace(request.SnapshotID)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ActionID != "alert-report-export" || request.SnapshotID == "" || len(request.Reason) < 4 ||
		(request.Format != "json" && request.Format != "pdf" && request.Format != "docx") {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "action_id=alert-report-export, format, snapshot_id and reason (minimum 4 characters) are required")
		return request, false
	}
	return request, true
}

func decodeAlertReportControlRequest(w http.ResponseWriter, r *http.Request, expectedActionID string) (alertReportControlRequest, bool) {
	var request alertReportControlRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "invalid alert report control request")
		return request, false
	}
	request.ActionID = strings.TrimSpace(request.ActionID)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ActionID != expectedActionID || request.ExpectedRevision < 1 || len(request.Reason) < 8 || len(request.Reason) > 1000 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "action_id, expected_revision and reason (8-1000 characters) are required")
		return request, false
	}
	return request, true
}

func loadAlertReportByIdempotencyKey(ctx context.Context, db *sql.DB, tenantID, key string) (alertReportJob, bool, error) {
	job, err := scanAlertReportJob(db.QueryRowContext(ctx, alertReportJobSelect+` WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, key))
	if errors.Is(err, sql.ErrNoRows) {
		return alertReportJob{}, false, nil
	}
	return job, err == nil, err
}

func loadAlertReportJob(ctx context.Context, db *sql.DB, tenantID, jobID string) (alertReportJob, error) {
	return scanAlertReportJob(db.QueryRowContext(ctx, alertReportJobSelect+` WHERE tenant_id=$1 AND job_id=$2`, tenantID, jobID))
}

const alertReportJobSelect = `SELECT job_id,tenant_id,alert_id,format,status,revision,requested_snapshot_id,snapshot_sha256,
	missing_sections::text,source_watermarks::text,object_bucket,object_key,mime_type,artifact_sha256,size_bytes,
	error_message,created_by,created_at,updated_at,completed_at,cancel_requested_at,cancelled_at FROM alert_report_jobs`

func scanAlertReportJob(row *sql.Row) (alertReportJob, error) {
	var job alertReportJob
	var missingJSON, watermarksJSON string
	var completedAt, cancelRequestedAt, cancelledAt sql.NullTime
	err := row.Scan(&job.JobID, &job.TenantID, &job.AlertID, &job.Format, &job.Status, &job.Revision,
		&job.RequestedSnapshot, &job.SnapshotSHA256, &missingJSON, &watermarksJSON, &job.ObjectBucket, &job.ObjectKey, &job.MIMEType,
		&job.ArtifactSHA256, &job.SizeBytes, &job.ErrorMessage, &job.CreatedBy, &job.CreatedAt, &job.UpdatedAt, &completedAt,
		&cancelRequestedAt, &cancelledAt)
	if err != nil {
		return alertReportJob{}, err
	}
	_ = json.Unmarshal([]byte(missingJSON), &job.MissingSections)
	_ = json.Unmarshal([]byte(watermarksJSON), &job.SourceWatermarks)
	if completedAt.Valid {
		value := completedAt.Time
		job.CompletedAt = &value
	}
	if cancelRequestedAt.Valid {
		value := cancelRequestedAt.Time
		job.CancelRequestedAt = &value
	}
	if cancelledAt.Valid {
		value := cancelledAt.Time
		job.CancelledAt = &value
	}
	return job, nil
}

func writeAlertReportJobResponse(w http.ResponseWriter, ctx context.Context, statusCode int, job alertReportJob) {
	data := map[string]interface{}{
		"job_id": job.JobID, "alert_id": job.AlertID, "format": job.Format, "status": job.Status, "revision": job.Revision,
		"snapshot_sha256": job.SnapshotSHA256, "artifact_sha256": job.ArtifactSHA256,
		"size_bytes": job.SizeBytes, "mime_type": job.MIMEType, "error_message": job.ErrorMessage,
		"created_at": job.CreatedAt, "updated_at": job.UpdatedAt, "completed_at": job.CompletedAt,
		"cancel_requested_at": job.CancelRequestedAt, "cancelled_at": job.CancelledAt,
	}
	if job.Status == "completed" {
		data["download_url"] = fmt.Sprintf("/v1/alerts/%s/reports/%s/download", job.AlertID, job.JobID)
	}
	meta := httpx.ContractMeta{
		ContractVersion: alertReportContractVersion, SnapshotID: job.RequestedSnapshot,
		Partial: len(job.MissingSections) > 0, MissingSections: job.MissingSections,
		SourceWatermarks: job.SourceWatermarks,
	}
	if statusCode == http.StatusAccepted {
		httpx.JSONContractAccepted(w, ctx, data, meta)
		return
	}
	httpx.JSONContractSuccess(w, ctx, data, meta)
}

func buildAlertReportArtifact(format string, snapshot []byte) ([]byte, string, string, error) {
	var model AlertReportModel
	if err := json.Unmarshal(snapshot, &model); err != nil {
		return nil, "", "", err
	}
	switch format {
	case "json":
		content, err := json.MarshalIndent(model, "", "  ")
		return append(content, '\n'), "application/json", "json", err
	case "pdf":
		return buildAlertReportPDF(model), "application/pdf", "pdf", nil
	case "docx":
		content, err := buildAlertReportDOCX(model)
		return content, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "docx", err
	default:
		return nil, "", "", fmt.Errorf("unsupported report format %q", format)
	}
}

func buildAlertReportPDF(model AlertReportModel) []byte {
	lines := []string{
		"Traffic Analysis Alert Report",
		"Alert ID: " + model.AlertID,
		"Snapshot ID: " + model.SnapshotID,
		fmt.Sprintf("Evidence: %d", len(model.Evidence)),
		fmt.Sprintf("Assets: %d", len(model.Assets)),
		fmt.Sprintf("Response Actions: %d", len(model.ResponseActions)),
		fmt.Sprintf("Audit Events: %d", len(model.AuditTrail)),
		"Snapshot SHA is stored in the PostgreSQL manifest.",
	}
	var stream strings.Builder
	stream.WriteString("BT /F1 11 Tf 48 780 Td ")
	for index, line := range lines {
		if index > 0 {
			stream.WriteString("0 -18 Td ")
		}
		stream.WriteString("(" + escapePDFText(line) + ") Tj ")
	}
	stream.WriteString("ET")
	body := stream.String()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(body), body),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for index, object := range objects {
		offsets[index+1] = output.Len()
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for index := 1; index <= len(objects); index++ {
		fmt.Fprintf(&output, "%010d 00000 n \n", offsets[index])
	}
	fmt.Fprintf(&output, "trailer << /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}

func buildAlertReportDOCX(model AlertReportModel) ([]byte, error) {
	lines := []string{
		"Traffic Analysis Alert Report", "Alert ID: " + model.AlertID, "Snapshot ID: " + model.SnapshotID,
		fmt.Sprintf("Evidence: %d", len(model.Evidence)), fmt.Sprintf("Assets: %d", len(model.Assets)),
		fmt.Sprintf("Response Actions: %d", len(model.ResponseActions)), fmt.Sprintf("Audit Events: %d", len(model.AuditTrail)),
	}
	var document strings.Builder
	document.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, line := range lines {
		document.WriteString(`<w:p><w:r><w:t>` + html.EscapeString(line) + `</w:t></w:r></w:p>`)
	}
	document.WriteString(`<w:sectPr/></w:body></w:document>`)
	entries := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   document.String(),
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml"} {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(time.Unix(0, 0).UTC())
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(entry, entries[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func alertReportBucket() string {
	if value := strings.TrimSpace(os.Getenv("ALERT_REPORT_BUCKET")); value != "" {
		return value
	}
	return "report-artifacts"
}

func safeObjectSegment(value string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func (h *Handler) alertReportObjectStore() (AlertReportObjectStore, error) {
	if h.reportObjects != nil {
		return h.reportObjects, nil
	}
	endpoint := strings.TrimSpace(os.Getenv("S3_ENDPOINT"))
	if endpoint == "" {
		endpoint = "minio.minio.svc:9000"
	}
	endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
	accessKey := strings.TrimSpace(os.Getenv("S3_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("S3_SECRET_KEY"))
	if accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("S3 credentials are not configured")
	}
	secure := strings.EqualFold(strings.TrimSpace(os.Getenv("S3_USE_SSL")), "true")
	transport, err := miniohttp.NewTransport(secure, os.Getenv("S3_CA_CERT"))
	if err != nil {
		return nil, fmt.Errorf("configure alert report MinIO TLS: %w", err)
	}
	client, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:    secure,
		Transport: transport,
	})
	if err != nil {
		return nil, err
	}
	return &minioAlertReportObjectStore{client: client}, nil
}

func (s *minioAlertReportObjectStore) Put(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *minioAlertReportObjectStore) Open(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	return s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
}

func (s *minioAlertReportObjectStore) Remove(ctx context.Context, bucket, key string) error {
	return s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}
