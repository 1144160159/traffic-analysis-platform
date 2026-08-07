package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	pathpkg "path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/asset/repository"
)

func (h *HTTPHandler) createAssetExport(w http.ResponseWriter, r *http.Request) {
	if !h.exportJobsV1Enabled {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "feature_disabled", "asset export job API is not enabled")
		return
	}
	identity, ok := h.requireAssetExport(w, r)
	if !ok {
		return
	}
	traceID := traceIDFromRequest(r)
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = traceID
	}
	var request config.AssetExportRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceID, "invalid_request", "invalid asset export payload")
		return
	}
	job, err := h.svc.CreateAssetExportJob(
		r.Context(), identity.TenantID, request,
		config.AssetExportCommand{
			IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
			Actor:          auditActor(identity),
			TraceID:        traceID,
			RequestID:      requestID,
			ClientIP:       clientIP(r),
			UserAgent:      r.UserAgent(),
		},
	)
	if err != nil {
		if errors.Is(err, repository.ErrAssetExportIdempotencyConflict) {
			writeAssetCommandError(w, http.StatusConflict, traceID, "idempotency_conflict", err.Error())
			return
		}
		writeAssetCommandError(w, http.StatusBadRequest, traceID, "asset_export_rejected", err.Error())
		return
	}
	writeAssetExportEnvelope(w, http.StatusAccepted, traceID, job, map[string]any{
		"job_id": job.JobID, "state": job.Status,
		"idempotent_replay": job.IdempotentReplay,
		"source_watermarks": assetExportJobWatermarks(job),
	})
}

func (h *HTTPHandler) assetExportResource(w http.ResponseWriter, r *http.Request, suffix string) {
	if !h.exportJobsV1Enabled {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "feature_disabled", "asset export job API is not enabled")
		return
	}
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "not_found", "unknown asset export resource")
		return
	}
	jobID := parts[0]
	if _, err := uuid.Parse(jobID); err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceIDFromRequest(r), "invalid_job_id", "asset export job_id must be a UUID")
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			writeAssetCommandError(w, http.StatusMethodNotAllowed, traceIDFromRequest(r), "method_not_allowed", "method not allowed")
			return
		}
		h.getAssetExportJob(w, r, jobID)
		return
	}
	if parts[1] != "download" {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "not_found", "unknown asset export resource")
		return
	}
	if r.Method != http.MethodGet {
		writeAssetCommandError(w, http.StatusMethodNotAllowed, traceIDFromRequest(r), "method_not_allowed", "method not allowed")
		return
	}
	h.downloadAssetExport(w, r, jobID)
}

func (h *HTTPHandler) getAssetExportJob(w http.ResponseWriter, r *http.Request, jobID string) {
	identity, ok := h.requireAssetExport(w, r)
	if !ok {
		return
	}
	job, err := h.svc.GetAssetExportJob(r.Context(), identity.TenantID, jobID)
	if errors.Is(err, sql.ErrNoRows) {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "not_found", "asset export job not found")
		return
	}
	if err != nil {
		writeAssetCommandError(w, http.StatusInternalServerError, traceIDFromRequest(r), "asset_export_read_failed", err.Error())
		return
	}
	writeAssetExportEnvelope(w, http.StatusOK, traceIDFromRequest(r), job, map[string]any{
		"job_id": job.JobID, "state": job.Status,
		"source_watermarks": assetExportJobWatermarks(job),
	})
}

func (h *HTTPHandler) downloadAssetExport(w http.ResponseWriter, r *http.Request, jobID string) {
	identity, ok := h.requireAssetExport(w, r)
	if !ok {
		return
	}
	traceID := traceIDFromRequest(r)
	job, err := h.svc.GetAssetExportJob(r.Context(), identity.TenantID, jobID)
	if errors.Is(err, sql.ErrNoRows) {
		writeAssetCommandError(w, http.StatusNotFound, traceID, "not_found", "asset export job not found")
		return
	}
	if err != nil {
		writeAssetCommandError(w, http.StatusInternalServerError, traceID, "asset_export_read_failed", err.Error())
		return
	}
	if job.Status != config.AssetExportStatusCompleted {
		writeAssetCommandError(w, http.StatusConflict, traceID, "asset_export_not_ready", "asset export job is not completed")
		return
	}
	if !job.RetentionUntil.IsZero() && time.Now().UTC().After(job.RetentionUntil) {
		writeAssetCommandError(w, http.StatusGone, traceID, "asset_export_expired", "asset export retention has expired")
		return
	}
	content, err := h.svc.ReadAssetExportArtifact(r.Context(), job)
	if err != nil {
		writeAssetCommandError(w, http.StatusBadGateway, traceID, "asset_export_manifest_mismatch", err.Error())
		return
	}
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" {
		requestID = traceID
	}
	if err := h.svc.RecordAssetExportDownload(
		r.Context(), job, auditActor(identity), traceID, requestID,
		clientIP(r), r.UserAgent(),
	); err != nil {
		writeAssetCommandError(w, http.StatusInternalServerError, traceID, "asset_export_audit_failed", "failed to audit asset export download")
		return
	}
	filename := pathpkg.Base(job.ObjectKey)
	w.Header().Set("Content-Type", job.MIMEType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("X-Content-SHA256", job.ArtifactSHA256)
	w.Header().Set("X-Asset-Snapshot-ID", job.SnapshotID)
	http.ServeContent(w, r, filename, job.UpdatedAt, bytes.NewReader(content))
}

func (h *HTTPHandler) getAssetColumnPreference(w http.ResponseWriter, r *http.Request) {
	if !h.exportJobsV1Enabled {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "feature_disabled", "asset export preference API is not enabled")
		return
	}
	identity, ok := h.requireAssetPreferenceRead(w, r)
	if !ok {
		return
	}
	preference, err := h.svc.GetAssetColumnPreference(
		r.Context(), identity.TenantID, assetPreferenceUserID(identity),
		strings.TrimSpace(r.URL.Query().Get("view_id")),
	)
	if err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceIDFromRequest(r), "asset_column_preference_read_failed", err.Error())
		return
	}
	writeAssetExportEnvelope(w, http.StatusOK, traceIDFromRequest(r), preference, map[string]any{
		"view_id": preference.ViewID, "revision": preference.Revision,
		"source_watermarks": map[string]string{
			"postgresql.asset_column_preferences.revision": fmt.Sprintf("%d", preference.Revision),
		},
	})
}

func (h *HTTPHandler) putAssetColumnPreference(w http.ResponseWriter, r *http.Request) {
	if !h.exportJobsV1Enabled {
		writeAssetCommandError(w, http.StatusNotFound, traceIDFromRequest(r), "feature_disabled", "asset export preference API is not enabled")
		return
	}
	identity, ok := h.requireAssetPreferenceRead(w, r)
	if !ok {
		return
	}
	traceID := traceIDFromRequest(r)
	var command config.AssetColumnPreferenceCommand
	if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
		writeAssetCommandError(w, http.StatusBadRequest, traceID, "invalid_request", "invalid asset column preference payload")
		return
	}
	command.Actor = auditActor(identity)
	command.TraceID = traceID
	command.RequestID = strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if command.RequestID == "" {
		command.RequestID = traceID
	}
	command.ClientIP = clientIP(r)
	command.UserAgent = r.UserAgent()
	preference, err := h.svc.UpsertAssetColumnPreference(
		r.Context(), identity.TenantID, assetPreferenceUserID(identity), command,
	)
	if err != nil {
		if errors.Is(err, repository.ErrAssetColumnPreferenceRevisionConflict) {
			writeAssetCommandError(w, http.StatusConflict, traceID, "revision_conflict", err.Error())
			return
		}
		writeAssetCommandError(w, http.StatusBadRequest, traceID, "asset_column_preference_rejected", err.Error())
		return
	}
	writeAssetExportEnvelope(w, http.StatusOK, traceID, preference, map[string]any{
		"view_id": preference.ViewID, "revision": preference.Revision,
		"source_watermarks": map[string]string{
			"postgresql.asset_column_preferences.revision": fmt.Sprintf("%d", preference.Revision),
		},
	})
}

func assetPreferenceUserID(identity requestIdentity) string {
	if strings.TrimSpace(identity.UserID) != "" {
		return strings.TrimSpace(identity.UserID)
	}
	return auditActor(identity)
}

func assetExportJobWatermarks(job *config.AssetExportJob) map[string]string {
	watermarks := map[string]string{}
	if job != nil {
		for source, watermark := range job.SourceWatermarks {
			watermarks[source] = watermark
		}
		watermarks["postgresql.asset_export_jobs.revision"] = fmt.Sprintf("%d", job.Revision)
	}
	return watermarks
}

func writeAssetExportEnvelope(w http.ResponseWriter, status int, traceID string, data any, extra map[string]any) {
	if traceID == "" {
		traceID = uuid.NewString()
	}
	now := time.Now().UTC()
	meta := map[string]any{
		"contract_version":  1,
		"snapshot_id":       "asset-export-api-" + traceID,
		"as_of":             now.Format(time.RFC3339Nano),
		"trace_id":          traceID,
		"partial":           false,
		"missing_sections":  []string{},
		"source_watermarks": map[string]string{},
	}
	for key, value := range extra {
		meta[key] = value
	}
	writeJSON(w, status, map[string]any{"data": data, "meta": meta, "error": nil})
}
