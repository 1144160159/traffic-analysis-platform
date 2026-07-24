package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

const (
	maxAuditExportRows    = 10_000
	maxAuditIntegrityRows = 10_000
	maxAuditIntegritySpan = 31 * 24 * time.Hour
)

type auditGovernanceLog struct {
	LogID        string                 `json:"log_id"`
	TenantID     string                 `json:"tenant_id"`
	UserID       string                 `json:"user_id"`
	Action       string                 `json:"action"`
	ResourceType string                 `json:"resource_type"`
	ResourceID   string                 `json:"resource_id"`
	Details      map[string]interface{} `json:"details"`
	IPAddress    string                 `json:"ip_address"`
	UserAgent    string                 `json:"user_agent"`
	RequestID    string                 `json:"request_id"`
	TraceID      string                 `json:"trace_id"`
	Result       string                 `json:"result"`
	Risk         string                 `json:"risk"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Timestamp    int64                  `json:"timestamp"`
}

type auditTimeValue string

func (value *auditTimeValue) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*value = ""
		return nil
	}
	if data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*value = auditTimeValue(text)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("audit time must be a string or Unix timestamp: %w", err)
	}
	if _, err := strconv.ParseInt(number.String(), 10, 64); err != nil {
		return fmt.Errorf("audit time must be an integer Unix timestamp: %w", err)
	}
	*value = auditTimeValue(number.String())
	return nil
}

type auditLogFilters struct {
	LogID       string         `json:"log_id,omitempty"`
	Action      string         `json:"action,omitempty"`
	UserID      string         `json:"user_id,omitempty"`
	ObjectType  string         `json:"object_type,omitempty"`
	ObjectID    string         `json:"object_id,omitempty"`
	Result      string         `json:"result,omitempty"`
	Risk        string         `json:"risk,omitempty"`
	RequestID   string         `json:"request_id,omitempty"`
	TraceID     string         `json:"trace_id,omitempty"`
	Start       auditTimeValue `json:"start,omitempty"`
	End         auditTimeValue `json:"end,omitempty"`
	parsedStart *time.Time
	parsedEnd   *time.Time
}

type auditSavedQueryRequest struct {
	Name    string          `json:"name"`
	Filters auditLogFilters `json:"filters"`
}

type auditExportRequest struct {
	Format        string          `json:"format"`
	Filters       auditLogFilters `json:"filters"`
	MaskSensitive *bool           `json:"mask_sensitive,omitempty"`
}

type auditReviewRequest struct {
	AuditLogID string `json:"audit_log_id,omitempty"`
	LogID      string `json:"log_id,omitempty"`
	Decision   string `json:"decision,omitempty"`
	Comment    string `json:"comment,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Risk       string `json:"risk,omitempty"`
}

type auditIntegrityRequest struct {
	Start   auditTimeValue  `json:"start,omitempty"`
	End     auditTimeValue  `json:"end,omitempty"`
	Filters auditLogFilters `json:"filters,omitempty"`
}

type auditQueryer interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

type auditLogSummary struct {
	Today         int     `json:"today"`
	Failed        int     `json:"failed"`
	HighRisk      int     `json:"high_risk"`
	Exports       int     `json:"exports"`
	PCAPAccess    int     `json:"pcap_access"`
	IntegrityRate float64 `json:"integrity_rate"`
}

type auditRetentionStatus struct {
	RetentionDays   int     `json:"retention_days"`
	ArchivedUntil   int64   `json:"archived_until"`
	ArchiveLocation string  `json:"archive_location"`
	IntegrityRate   float64 `json:"integrity_rate"`
	MaskedRate      float64 `json:"masked_rate"`
	LastCheckedAt   int64   `json:"last_checked_at,omitempty"`
}

type auditIntegrityEvidence struct {
	Matched    int `json:"matched"`
	Baselined  int `json:"baselined"`
	Mismatched int `json:"mismatched"`
	Added      int `json:"added"`
	Missing    int `json:"missing"`
}

func (h *SystemHandler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuditReadPermission(w, r) || !h.requirePostgres(w, r.Context()) {
		return
	}
	tenantID := writeTenantID(r)
	logID := strings.TrimSpace(mux.Vars(r)["id"])
	if logID == "" {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_ARGUMENT", "audit log id is required")
		return
	}
	log, err := queryAuditLogByID(r.Context(), h.pgDB, tenantID, logID)
	if err == sql.ErrNoRows {
		httpx.JSONError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "audit log not found")
		return
	}
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_QUERY_FAILED", err.Error())
		return
	}
	httpx.JSONSuccess(w, r.Context(), log)
}

func (h *SystemHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuditReadPermission(w, r) || !h.requirePostgres(w, r.Context()) {
		return
	}
	filters, err := auditFiltersFromRequest(r)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_FILTER", err.Error())
		return
	}
	limit, offset := parsePageLimitOffset(r, 50, 500)
	where, args := buildAuditLogWhere(writeTenantID(r), filters)
	var total int
	if err := h.pgDB.QueryRowContext(r.Context(), "SELECT count(*) FROM audit_logs WHERE "+where, args...).Scan(&total); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_QUERY_FAILED", err.Error())
		return
	}
	logs, err := queryAuditLogs(r.Context(), h.pgDB, where, args, limit, offset)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_QUERY_FAILED", err.Error())
		return
	}
	summary, err := queryAuditLogSummary(r.Context(), h.pgDB, where, args)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_QUERY_FAILED", err.Error())
		return
	}
	retention, err := queryAuditRetentionStatus(r.Context(), h.pgDB, writeTenantID(r))
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_QUERY_FAILED", err.Error())
		return
	}
	summary.IntegrityRate = retention.IntegrityRate
	// Keep trails for compatibility with existing consumers while exposing the
	// governance-specific logs key for new clients.
	httpx.JSONSuccess(w, r.Context(), map[string]interface{}{"logs": logs, "trails": logs, "total": total, "summary": summary, "retention": retention})
}

func (h *SystemHandler) CreateAuditSavedQuery(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuditWritePermission(w, r) || !h.requirePostgres(w, r.Context()) {
		return
	}
	var request auditSavedQueryRequest
	if err := decodeAuditGovernanceJSON(r, &request); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len(request.Name) > 120 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "name is required and must not exceed 120 characters")
		return
	}
	if err := normalizeAuditFilters(&request.Filters); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_FILTER", err.Error())
		return
	}
	filtersJSON, err := json.Marshal(request.Filters)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_FILTER", err.Error())
		return
	}
	tx, err := h.pgDB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_WRITE_FAILED", err.Error())
		return
	}
	defer tx.Rollback()
	id := uuid.NewString()
	var createdAt time.Time
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO audit_saved_queries (saved_query_id, tenant_id, name, filters, created_by)
		VALUES ($1::uuid, $2, $3, $4::jsonb, $5)
		RETURNING created_at`, id, writeTenantID(r), request.Name, string(filtersJSON), httpx.GetUserID(r.Context())).Scan(&createdAt); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_SAVED_QUERY_WRITE_FAILED", err.Error())
		return
	}
	if err := h.insertAuditGovernanceEvent(r.Context(), tx, r, "AUDIT_SAVED_QUERY_CREATED", "audit_saved_query", id, "low", map[string]interface{}{"name": request.Name, "filters": request.Filters}); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_LOG_WRITE_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_WRITE_FAILED", err.Error())
		return
	}
	httpx.JSONCreated(w, r.Context(), map[string]interface{}{"query_id": id, "saved_query_id": id, "name": request.Name, "filters": request.Filters, "created_at": createdAt.UnixMilli()})
}

func (h *SystemHandler) CreateAuditExport(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuditExportPermission(w, r) || !h.requirePostgres(w, r.Context()) {
		return
	}
	var request auditExportRequest
	if err := decodeAuditGovernanceJSON(r, &request); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	request.Format = strings.ToLower(strings.TrimSpace(request.Format))
	if request.Format != "pdf" && request.Format != "csv" && request.Format != "json" {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_FORMAT", "format must be pdf, csv, or json")
		return
	}
	if err := normalizeAuditFilters(&request.Filters); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_FILTER", err.Error())
		return
	}
	tx, err := h.pgDB.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: false})
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_EXPORT_FAILED", err.Error())
		return
	}
	defer tx.Rollback()
	where, args := buildAuditLogWhere(writeTenantID(r), request.Filters)
	var totalMatching int
	if err := tx.QueryRowContext(r.Context(), "SELECT count(*) FROM audit_logs WHERE "+where, args...).Scan(&totalMatching); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_EXPORT_FAILED", err.Error())
		return
	}
	if totalMatching > maxAuditExportRows {
		httpx.JSONError(w, r.Context(), http.StatusRequestEntityTooLarge, "AUDIT_EXPORT_RANGE_TOO_LARGE", fmt.Sprintf("export contains %d rows; narrow it to at most %d so evidence is never truncated", totalMatching, maxAuditExportRows))
		return
	}
	logs, err := queryAuditLogs(r.Context(), tx, where, args, maxAuditExportRows, 0)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_EXPORT_FAILED", err.Error())
		return
	}
	maskSensitive := request.MaskSensitive == nil || *request.MaskSensitive
	exportLogs := maskAuditExportLogs(logs, maskSensitive)
	content, mimeType, extension, err := buildAuditExportArtifact(request.Format, writeTenantID(r), request.Filters, exportLogs)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_EXPORT_FAILED", err.Error())
		return
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
	exportID := uuid.NewString()
	filename := fmt.Sprintf("audit-export-%s.%s", exportID, extension)
	filtersJSON, _ := json.Marshal(request.Filters)
	var createdAt time.Time
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO audit_exports (export_id, tenant_id, format, filters, row_count, total_matching, truncated, mask_sensitive, filename, mime_type, sha256, size_bytes, created_by)
		VALUES ($1::uuid, $2, $3, $4::jsonb, $5, $6, false, $7, $8, $9, $10, $11, $12)
		RETURNING created_at`, exportID, writeTenantID(r), request.Format, string(filtersJSON), len(exportLogs), totalMatching, maskSensitive, filename, mimeType, digest, len(content), httpx.GetUserID(r.Context())).Scan(&createdAt); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_EXPORT_METADATA_WRITE_FAILED", err.Error())
		return
	}
	if err := h.insertAuditGovernanceEvent(r.Context(), tx, r, "AUDIT_EVIDENCE_EXPORTED", "audit_export", exportID, "medium", map[string]interface{}{"format": request.Format, "row_count": len(exportLogs), "total_matching": totalMatching, "truncated": false, "mask_sensitive": maskSensitive, "sha256": digest, "size_bytes": len(content)}); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_LOG_WRITE_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_EXPORT_FAILED", err.Error())
		return
	}
	httpx.JSONSuccess(w, r.Context(), map[string]interface{}{
		"export_id": exportID, "format": request.Format, "filename": filename, "mime_type": mimeType,
		"row_count": len(exportLogs), "total_matching": totalMatching, "truncated": false, "mask_sensitive": maskSensitive, "size_bytes": len(content), "sha256": digest,
		"content_base64": base64.StdEncoding.EncodeToString(content), "created_at": createdAt.UnixMilli(), "generated_at": createdAt.UnixMilli(),
	})
}

func (h *SystemHandler) CreateAuditReview(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuditWritePermission(w, r) || !h.requirePostgres(w, r.Context()) {
		return
	}
	var request auditReviewRequest
	if err := decodeAuditGovernanceJSON(r, &request); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	request.AuditLogID = strings.TrimSpace(firstNonEmpty(request.AuditLogID, request.LogID))
	request.Decision = strings.ToLower(strings.TrimSpace(request.Decision))
	request.Comment = strings.TrimSpace(firstNonEmpty(request.Comment, request.Reason))
	request.Risk = strings.ToLower(strings.TrimSpace(request.Risk))
	if request.Decision == "" {
		request.Decision = "escalated"
	}
	if request.AuditLogID == "" || !stringIn(request.Decision, "pending", "approved", "rejected", "escalated") {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "log_id and decision pending, approved, rejected, or escalated are required")
		return
	}
	if request.Decision != "approved" && len(request.Comment) < 8 {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "pending, rejected, or escalated review requires a reason of at least 8 characters")
		return
	}
	if request.Risk == "" {
		request.Risk = "medium"
	}
	if !validAuditRisk(request.Risk) {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", "risk must be low, medium, high, or critical")
		return
	}
	tx, err := h.pgDB.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_REVIEW_FAILED", err.Error())
		return
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM audit_logs WHERE tenant_id=$1 AND (event_id=$2 OR id::text=$2))`, writeTenantID(r), request.AuditLogID).Scan(&exists); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_REVIEW_FAILED", err.Error())
		return
	}
	if !exists {
		httpx.JSONError(w, r.Context(), http.StatusNotFound, "NOT_FOUND", "audit log not found")
		return
	}
	reviewID := uuid.NewString()
	var createdAt time.Time
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO audit_reviews (review_id, tenant_id, audit_log_id, decision, comment, risk_level, reviewed_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
		RETURNING created_at`, reviewID, writeTenantID(r), request.AuditLogID, request.Decision, request.Comment, request.Risk, httpx.GetUserID(r.Context())).Scan(&createdAt); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_REVIEW_FAILED", err.Error())
		return
	}
	if err := h.insertAuditGovernanceEvent(r.Context(), tx, r, "AUDIT_REVIEW_TRIGGERED", "audit_log", request.AuditLogID, request.Risk, map[string]interface{}{"review_id": reviewID, "status": request.Decision, "reason": request.Comment}); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_LOG_WRITE_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_REVIEW_FAILED", err.Error())
		return
	}
	httpx.JSONCreated(w, r.Context(), map[string]interface{}{"review_id": reviewID, "log_id": request.AuditLogID, "audit_log_id": request.AuditLogID, "status": request.Decision, "decision": request.Decision, "reason": request.Comment, "risk": request.Risk, "created_at": createdAt.UnixMilli()})
}

func (h *SystemHandler) CreateAuditIntegrityCheck(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuditWritePermission(w, r) || !h.requirePostgres(w, r.Context()) {
		return
	}
	request := auditIntegrityRequest{}
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeAuditGovernanceJSON(r, &request); err != nil {
			httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
	}
	filters := request.Filters
	if filters.Start == "" {
		filters.Start = request.Start
	}
	if filters.End == "" {
		filters.End = request.End
	}
	if filters.Start == "" && filters.End == "" {
		now := time.Now().UTC()
		filters.parsedEnd = &now
		start := now.Add(-24 * time.Hour)
		filters.parsedStart = &start
		filters.Start, filters.End = auditTimeValue(start.Format(time.RFC3339)), auditTimeValue(now.Format(time.RFC3339))
	} else if err := normalizeAuditFilters(&filters); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_RANGE", err.Error())
		return
	}
	if filters.parsedStart == nil || filters.parsedEnd == nil || filters.parsedEnd.Sub(*filters.parsedStart) > maxAuditIntegritySpan {
		httpx.JSONError(w, r.Context(), http.StatusBadRequest, "INVALID_RANGE", "integrity checks require start and end within 31 days")
		return
	}
	tx, err := h.pgDB.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	defer tx.Rollback()
	where, args := buildAuditLogWhere(writeTenantID(r), filters)
	where += " AND object_type <> 'audit_integrity_check'"
	var total int
	if err := tx.QueryRowContext(r.Context(), "SELECT count(*) FROM audit_logs WHERE "+where, args...).Scan(&total); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	if total > maxAuditIntegrityRows {
		httpx.JSONError(w, r.Context(), http.StatusRequestEntityTooLarge, "AUDIT_INTEGRITY_RANGE_TOO_LARGE", fmt.Sprintf("integrity range contains %d rows; narrow it to at most %d", total, maxAuditIntegrityRows))
		return
	}
	logs, err := queryAuditLogs(r.Context(), tx, where, args, maxAuditIntegrityRows, 0)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	canonical, err := json.Marshal(logs)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
	evidence, err := verifyAuditLogBaselines(r.Context(), tx, writeTenantID(r), logs)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	previousCheckID, err := latestAuditIntegrityManifestID(r.Context(), tx, writeTenantID(r), *filters.parsedStart, *filters.parsedEnd, string(filtersJSON))
	if err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	if previousCheckID != "" {
		manifestEvidence, err := compareAuditIntegrityManifest(r.Context(), tx, previousCheckID, logs)
		if err != nil {
			httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
			return
		}
		evidence.Added = manifestEvidence.Added
		evidence.Missing = manifestEvidence.Missing
		if manifestEvidence.Mismatched > evidence.Mismatched {
			evidence.Mismatched = manifestEvidence.Mismatched
		}
	}
	status := "passed"
	valid := evidence.Mismatched == 0 && evidence.Missing == 0
	if !valid {
		status = "failed"
	} else if len(logs) == 0 {
		status = "no_records"
	} else if previousCheckID == "" || evidence.Baselined > 0 || evidence.Added > 0 {
		status = "baseline_created"
	}
	checkID := uuid.NewString()
	var createdAt time.Time
	if err := tx.QueryRowContext(r.Context(), `
		INSERT INTO audit_integrity_checks (check_id, tenant_id, time_start, time_end, filters, row_count, root_sha256, status, matched_count, baselined_count, mismatched_count, added_count, missing_count, requested_by)
		VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING created_at`, checkID, writeTenantID(r), *filters.parsedStart, *filters.parsedEnd, string(filtersJSON), len(logs), digest, status, evidence.Matched, evidence.Baselined, evidence.Mismatched, evidence.Added, evidence.Missing, httpx.GetUserID(r.Context())).Scan(&createdAt); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	if err := insertAuditIntegrityManifest(r.Context(), tx, checkID, writeTenantID(r), logs); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	checkRisk := "low"
	if !valid {
		checkRisk = "critical"
	}
	if err := h.insertAuditGovernanceEvent(r.Context(), tx, r, "AUDIT_INTEGRITY_CHECK_COMPLETED", "audit_integrity_check", checkID, checkRisk, map[string]interface{}{"row_count": len(logs), "root_sha256": digest, "time_start": filters.Start, "time_end": filters.End, "status": status, "matched": evidence.Matched, "baselined": evidence.Baselined, "mismatched": evidence.Mismatched, "added": evidence.Added, "missing": evidence.Missing}); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_LOG_WRITE_FAILED", err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, r.Context(), http.StatusInternalServerError, "AUDIT_INTEGRITY_FAILED", err.Error())
		return
	}
	httpx.JSONCreated(w, r.Context(), map[string]interface{}{"check_id": checkID, "status": status, "valid": valid, "baseline_created": status == "baseline_created", "row_count": len(logs), "records_checked": len(logs), "matched": evidence.Matched, "baselined": evidence.Baselined, "mismatched": evidence.Mismatched, "added": evidence.Added, "missing": evidence.Missing, "root_sha256": digest, "start": filters.Start, "end": filters.End, "created_at": createdAt.UnixMilli(), "checked_at": createdAt.UnixMilli()})
}

func (h *SystemHandler) requireAuditWritePermission(w http.ResponseWriter, r *http.Request) bool {
	if hasAnySystemPermission(r.Context(), authmodel.ScopeAuditWrite, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, r.Context(), http.StatusForbidden, "PERMISSION_DENIED", "permission denied: audit:write required")
	return false
}

func (h *SystemHandler) requireAuditExportPermission(w http.ResponseWriter, r *http.Request) bool {
	if hasAnySystemPermission(r.Context(), authmodel.ScopeAuditExport, authmodel.ScopeAdminAll) {
		return true
	}
	httpx.JSONError(w, r.Context(), http.StatusForbidden, "PERMISSION_DENIED", "permission denied: audit:export required")
	return false
}

func auditFiltersFromRequest(r *http.Request) (auditLogFilters, error) {
	query := r.URL.Query()
	filters := auditLogFilters{
		LogID: query.Get("log_id"), Action: query.Get("action"), UserID: query.Get("user_id"),
		ObjectType: firstNonEmpty(query.Get("object_type"), query.Get("resource_type")),
		ObjectID:   firstNonEmpty(query.Get("object_id"), query.Get("resource_id")),
		Result:     query.Get("result"), Risk: firstNonEmpty(query.Get("risk"), query.Get("risk_level")),
		RequestID: query.Get("request_id"), TraceID: query.Get("trace_id"),
		Start: auditTimeValue(query.Get("start")), End: auditTimeValue(query.Get("end")),
	}
	return filters, normalizeAuditFilters(&filters)
}

func normalizeAuditFilters(filters *auditLogFilters) error {
	filters.LogID = strings.TrimSpace(filters.LogID)
	filters.Action = strings.TrimSpace(filters.Action)
	filters.UserID = strings.TrimSpace(filters.UserID)
	filters.ObjectType = strings.TrimSpace(filters.ObjectType)
	filters.ObjectID = strings.TrimSpace(filters.ObjectID)
	filters.Result = strings.ToLower(strings.TrimSpace(filters.Result))
	filters.Risk = strings.ToLower(strings.TrimSpace(filters.Risk))
	filters.RequestID = strings.TrimSpace(filters.RequestID)
	filters.TraceID = strings.TrimSpace(filters.TraceID)
	filters.Start = auditTimeValue(strings.TrimSpace(string(filters.Start)))
	filters.End = auditTimeValue(strings.TrimSpace(string(filters.End)))
	if filters.Risk != "" && !validAuditRisk(filters.Risk) {
		return fmt.Errorf("risk must be low, medium, high, or critical")
	}
	if filters.Start != "" {
		parsed, err := parseAuditTime(string(filters.Start))
		if err != nil {
			return fmt.Errorf("invalid start: %w", err)
		}
		filters.parsedStart = &parsed
	}
	if filters.End != "" {
		parsed, err := parseAuditTime(string(filters.End))
		if err != nil {
			return fmt.Errorf("invalid end: %w", err)
		}
		filters.parsedEnd = &parsed
	}
	if filters.parsedStart != nil && filters.parsedEnd != nil && !filters.parsedStart.Before(*filters.parsedEnd) {
		return fmt.Errorf("start must be before end")
	}
	return nil
}

func parseAuditTime(value string) (time.Time, error) {
	if milliseconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if milliseconds < 100_000_000_000 {
			return time.Unix(milliseconds, 0).UTC(), nil
		}
		return time.UnixMilli(milliseconds).UTC(), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("must be RFC3339 or Unix milliseconds")
	}
	return parsed.UTC(), nil
}

func buildAuditLogWhere(tenantID string, filters auditLogFilters) (string, []interface{}) {
	args := []interface{}{tenantID}
	clauses := []string{"tenant_id=$1"}
	add := func(clause string, value interface{}) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if filters.LogID != "" {
		add("(event_id=$%[1]d OR id::text=$%[1]d)", filters.LogID)
	}
	if filters.Action != "" {
		if strings.Contains(filters.Action, "*") {
			add("action LIKE $%d", strings.ReplaceAll(filters.Action, "*", "%"))
		} else {
			add("action=$%d", filters.Action)
		}
	}
	if filters.UserID != "" {
		add("user_id::text=$%d", filters.UserID)
	}
	if filters.ObjectType != "" {
		add("object_type=$%d", filters.ObjectType)
	}
	if filters.ObjectID != "" {
		add("object_id=$%d", filters.ObjectID)
	}
	if filters.Result != "" {
		add("COALESCE(NULLIF(result,''),NULLIF(detail->>'result',''),CASE WHEN success THEN 'success' ELSE 'failure' END)=$%d", filters.Result)
	}
	if filters.Risk != "" {
		add("COALESCE(NULLIF(risk_level,''),NULLIF(detail->>'risk',''),NULLIF(detail->>'risk_level',''),'low')=$%d", filters.Risk)
	}
	if filters.RequestID != "" {
		add("COALESCE(NULLIF(request_id,''),detail->>'request_id','')=$%d", filters.RequestID)
	}
	if filters.TraceID != "" {
		add("COALESCE(NULLIF(trace_id,''),detail->>'trace_id','')=$%d", filters.TraceID)
	}
	if filters.parsedStart != nil {
		add("created_at >= $%d", *filters.parsedStart)
	}
	if filters.parsedEnd != nil {
		add("created_at <= $%d", *filters.parsedEnd)
	}
	return strings.Join(clauses, " AND "), args
}

const auditGovernanceSelect = `
	SELECT COALESCE(event_id,id::text), tenant_id, COALESCE(user_id::text,''), action,
		COALESCE(object_type,''), COALESCE(object_id,''), COALESCE(detail,'{}'::jsonb)::text,
		COALESCE(ip_addr,''), COALESCE(user_agent,''),
		COALESCE(NULLIF(request_id,''),detail->>'request_id',''),
		COALESCE(NULLIF(trace_id,''),detail->>'trace_id',''),
		COALESCE(NULLIF(result,''),NULLIF(detail->>'result',''),CASE WHEN success THEN 'success' ELSE 'failure' END),
		COALESCE(NULLIF(risk_level,''),NULLIF(detail->>'risk',''),NULLIF(detail->>'risk_level',''),'low'),
		COALESCE(error_message,''), created_at
	FROM audit_logs`

func queryAuditLogs(ctx context.Context, queryer auditQueryer, where string, args []interface{}, limit, offset int) ([]auditGovernanceLog, error) {
	query := auditGovernanceSelect + " WHERE " + where + " ORDER BY created_at DESC, id DESC"
	queryArgs := append([]interface{}{}, args...)
	if limit > 0 {
		queryArgs = append(queryArgs, limit, offset)
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(queryArgs)-1, len(queryArgs))
	}
	rows, err := queryer.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := make([]auditGovernanceLog, 0)
	for rows.Next() {
		log, err := scanAuditGovernanceLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func queryAuditLogSummary(ctx context.Context, queryer auditQueryer, where string, args []interface{}) (auditLogSummary, error) {
	var summary auditLogSummary
	err := queryer.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE created_at >= date_trunc('day', now())),
			count(*) FILTER (WHERE lower(COALESCE(NULLIF(result,''),NULLIF(detail->>'result',''),CASE WHEN success THEN 'success' ELSE 'failure' END)) IN ('failure','failed','error','denied')),
			count(*) FILTER (WHERE lower(COALESCE(NULLIF(risk_level,''),NULLIF(detail->>'risk',''),NULLIF(detail->>'risk_level',''),'low')) IN ('high','critical')),
			count(*) FILTER (WHERE action ILIKE '%EXPORT%' OR object_type='audit_export'),
			count(*) FILTER (WHERE action ILIKE '%PCAP%' OR lower(COALESCE(object_type,''))='pcap'),
			COALESCE(100.0 * count(*) FILTER (WHERE lower(COALESCE(NULLIF(result,''),NULLIF(detail->>'result',''),CASE WHEN success THEN 'success' ELSE 'failure' END)) NOT IN ('failure','failed','error','denied')) / NULLIF(count(*),0), 100.0)
		FROM audit_logs WHERE `+where, args...).Scan(&summary.Today, &summary.Failed, &summary.HighRisk, &summary.Exports, &summary.PCAPAccess, &summary.IntegrityRate)
	return summary, err
}

func queryAuditRetentionStatus(ctx context.Context, queryer auditQueryer, tenantID string) (auditRetentionStatus, error) {
	retentionDays := 365
	if configured, err := strconv.Atoi(strings.TrimSpace(os.Getenv("AUDIT_RETENTION_DAYS"))); err == nil && configured > 0 {
		retentionDays = configured
	}
	archiveLocation := strings.TrimSpace(os.Getenv("AUDIT_ARCHIVE_LOCATION"))
	if archiveLocation == "" {
		archiveLocation = "archive-audit"
	}
	status := auditRetentionStatus{
		RetentionDays: retentionDays, ArchivedUntil: time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour).UnixMilli(),
		ArchiveLocation: archiveLocation, IntegrityRate: 0, MaskedRate: 0,
	}
	var exportCount, maskedExportCount int
	if err := queryer.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE mask_sensitive) FROM audit_exports WHERE tenant_id=$1`, tenantID).Scan(&exportCount, &maskedExportCount); err != nil {
		return status, err
	} else if exportCount > 0 {
		status.MaskedRate = 100 * float64(maskedExportCount) / float64(exportCount)
	}
	var integrityStatus string
	var checkedAt time.Time
	err := queryer.QueryRowContext(ctx, `
		SELECT status, created_at FROM audit_integrity_checks
		WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 1`, tenantID).Scan(&integrityStatus, &checkedAt)
	if err == sql.ErrNoRows {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.LastCheckedAt = checkedAt.UnixMilli()
	if integrityStatus == "passed" {
		status.IntegrityRate = 100
	}
	return status, nil
}

func verifyAuditLogBaselines(ctx context.Context, tx *sql.Tx, tenantID string, logs []auditGovernanceLog) (auditIntegrityEvidence, error) {
	evidence := auditIntegrityEvidence{}
	now := time.Now().UTC()
	for _, log := range logs {
		canonical, err := json.Marshal(log)
		if err != nil {
			return evidence, fmt.Errorf("marshal audit log %s: %w", log.LogID, err)
		}
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
		var baseline string
		err = tx.QueryRowContext(ctx, `
			SELECT root_sha256 FROM audit_log_integrity_baselines
			WHERE tenant_id=$1 AND audit_log_id=$2`, tenantID, log.LogID).Scan(&baseline)
		switch err {
		case nil:
			if baseline == digest {
				evidence.Matched++
			} else {
				evidence.Mismatched++
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE audit_log_integrity_baselines SET last_checked_at=$3
				WHERE tenant_id=$1 AND audit_log_id=$2`, tenantID, log.LogID, now); err != nil {
				return evidence, fmt.Errorf("update audit baseline %s: %w", log.LogID, err)
			}
		case sql.ErrNoRows:
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO audit_log_integrity_baselines (tenant_id, audit_log_id, root_sha256, established_at, last_checked_at)
				VALUES ($1,$2,$3,$4,$4)`, tenantID, log.LogID, digest, now); err != nil {
				return evidence, fmt.Errorf("create audit baseline %s: %w", log.LogID, err)
			}
			evidence.Baselined++
		default:
			return evidence, fmt.Errorf("read audit baseline %s: %w", log.LogID, err)
		}
	}
	return evidence, nil
}

func latestAuditIntegrityManifestID(ctx context.Context, tx *sql.Tx, tenantID string, start, end time.Time, filtersJSON string) (string, error) {
	var checkID string
	err := tx.QueryRowContext(ctx, `
		SELECT check_id::text FROM audit_integrity_checks
		WHERE tenant_id=$1 AND time_start=$2 AND time_end=$3 AND filters=$4::jsonb AND status IN ('passed','baseline_created')
		ORDER BY created_at DESC LIMIT 1`, tenantID, start, end, filtersJSON).Scan(&checkID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return checkID, err
}

func compareAuditIntegrityManifest(ctx context.Context, tx *sql.Tx, checkID string, logs []auditGovernanceLog) (auditIntegrityEvidence, error) {
	current := make(map[string]string, len(logs))
	for _, log := range logs {
		digest, err := auditGovernanceLogDigest(log)
		if err != nil {
			return auditIntegrityEvidence{}, err
		}
		current[log.LogID] = digest
	}
	rows, err := tx.QueryContext(ctx, `SELECT audit_log_id, root_sha256 FROM audit_integrity_manifest_entries WHERE check_id=$1::uuid`, checkID)
	if err != nil {
		return auditIntegrityEvidence{}, err
	}
	defer rows.Close()
	evidence := auditIntegrityEvidence{}
	previous := map[string]struct{}{}
	for rows.Next() {
		var logID, digest string
		if err := rows.Scan(&logID, &digest); err != nil {
			return evidence, err
		}
		previous[logID] = struct{}{}
		currentDigest, exists := current[logID]
		if !exists {
			evidence.Missing++
		} else if currentDigest != digest {
			evidence.Mismatched++
		}
	}
	if err := rows.Err(); err != nil {
		return evidence, err
	}
	for logID := range current {
		if _, exists := previous[logID]; !exists {
			evidence.Added++
		}
	}
	return evidence, nil
}

func insertAuditIntegrityManifest(ctx context.Context, tx *sql.Tx, checkID, tenantID string, logs []auditGovernanceLog) error {
	for _, log := range logs {
		digest, err := auditGovernanceLogDigest(log)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO audit_integrity_manifest_entries (check_id, tenant_id, audit_log_id, root_sha256)
			VALUES ($1::uuid,$2,$3,$4)`, checkID, tenantID, log.LogID, digest); err != nil {
			return fmt.Errorf("persist audit integrity manifest %s: %w", log.LogID, err)
		}
	}
	return nil
}

func auditGovernanceLogDigest(log auditGovernanceLog) (string, error) {
	canonical, err := json.Marshal(log)
	if err != nil {
		return "", fmt.Errorf("marshal audit log %s: %w", log.LogID, err)
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(canonical)), nil
}

type auditRowScanner interface {
	Scan(...interface{}) error
}

func queryAuditLogByID(ctx context.Context, queryer auditQueryer, tenantID, logID string) (auditGovernanceLog, error) {
	return scanAuditGovernanceLog(queryer.QueryRowContext(ctx, auditGovernanceSelect+" WHERE tenant_id=$1 AND (event_id=$2 OR id::text=$2)", tenantID, logID))
}

func scanAuditGovernanceLog(scanner auditRowScanner) (auditGovernanceLog, error) {
	var log auditGovernanceLog
	var detailsJSON string
	var createdAt time.Time
	err := scanner.Scan(&log.LogID, &log.TenantID, &log.UserID, &log.Action, &log.ResourceType, &log.ResourceID,
		&detailsJSON, &log.IPAddress, &log.UserAgent, &log.RequestID, &log.TraceID, &log.Result, &log.Risk, &log.ErrorMessage, &createdAt)
	if err != nil {
		return log, err
	}
	log.Details = map[string]interface{}{}
	if err := json.Unmarshal([]byte(detailsJSON), &log.Details); err != nil {
		return log, err
	}
	log.Timestamp = createdAt.UnixMilli()
	return log, nil
}

func (h *SystemHandler) insertAuditGovernanceEvent(ctx context.Context, tx *sql.Tx, r *http.Request, action, objectType, objectID, risk string, detail map[string]interface{}) error {
	requestID := httpx.GetRequestID(ctx)
	traceID := httpx.GetTraceID(ctx)
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["result"] = "success"
	detail["risk_level"] = risk
	detail["request_id"] = requestID
	detail["trace_id"] = traceID
	detail["api_path"] = r.URL.Path
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	userID := httpx.GetUserID(ctx)
	userIDExpression := "NULLIF($3,'')"
	var dataType string
	if err := tx.QueryRowContext(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='audit_logs' AND column_name='user_id'`).Scan(&dataType); err != nil {
		return fmt.Errorf("resolve audit user_id type: %w", err)
	}
	if dataType == "uuid" {
		userIDExpression = "NULLIF($3,'')::uuid"
		if userID != "" {
			if _, err := uuid.Parse(userID); err != nil {
				userID = ""
			}
		}
	}
	query := `INSERT INTO audit_logs
		(event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent, request_id, trace_id, success, error_message, risk_level, result)
		VALUES ($1,$2,` + userIDExpression + `,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,true,'',$12,'success')`
	_, err = tx.ExecContext(ctx, query, "audit-"+uuid.NewString(), writeTenantID(r), userID, action, objectType, objectID, string(detailJSON), clientIP(r), r.UserAgent(), requestID, traceID, risk)
	return err
}

func buildAuditExportArtifact(format, tenantID string, filters auditLogFilters, logs []auditGovernanceLog) ([]byte, string, string, error) {
	switch format {
	case "json":
		content, err := json.MarshalIndent(map[string]interface{}{"tenant_id": tenantID, "filters": filters, "logs": logs}, "", "  ")
		return append(content, '\n'), "application/json", "json", err
	case "csv":
		var buffer bytes.Buffer
		writer := csv.NewWriter(&buffer)
		if err := writer.Write([]string{"log_id", "timestamp", "user_id", "action", "resource_type", "resource_id", "result", "risk", "request_id", "trace_id", "ip_address", "user_agent", "details_json"}); err != nil {
			return nil, "", "", err
		}
		for _, log := range logs {
			detailsJSON, err := json.Marshal(log.Details)
			if err != nil {
				return nil, "", "", fmt.Errorf("marshal audit details %s: %w", log.LogID, err)
			}
			if err := writer.Write([]string{log.LogID, strconv.FormatInt(log.Timestamp, 10), log.UserID, log.Action, log.ResourceType, log.ResourceID, log.Result, log.Risk, log.RequestID, log.TraceID, log.IPAddress, log.UserAgent, string(detailsJSON)}); err != nil {
				return nil, "", "", err
			}
		}
		writer.Flush()
		return buffer.Bytes(), "text/csv; charset=utf-8", "csv", writer.Error()
	case "pdf":
		return buildAuditGovernancePDF(tenantID, logs), "application/pdf", "pdf", nil
	default:
		return nil, "", "", fmt.Errorf("unsupported audit export format %q", format)
	}
}

func maskAuditExportLogs(logs []auditGovernanceLog, enabled bool) []auditGovernanceLog {
	if !enabled {
		return logs
	}
	masked := make([]auditGovernanceLog, len(logs))
	copy(masked, logs)
	for index := range masked {
		if masked[index].IPAddress != "" {
			masked[index].IPAddress = "***masked***"
		}
		if masked[index].UserAgent != "" {
			masked[index].UserAgent = "***masked***"
		}
		if masked[index].Details != nil {
			masked[index].Details = maskAuditExportDetails(masked[index].Details).(map[string]interface{})
		}
	}
	return masked
}

func maskAuditExportDetails(value interface{}) interface{} {
	switch current := value.(type) {
	case map[string]interface{}:
		masked := make(map[string]interface{}, len(current))
		for key, child := range current {
			normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
			if normalized == "ipaddress" || normalized == "ipaddr" || normalized == "sourceip" || normalized == "clientip" || normalized == "remoteip" || normalized == "peerip" || normalized == "realip" || normalized == "remoteaddr" || normalized == "xforwardedfor" || normalized == "forwardedfor" || normalized == "useragent" || normalized == "httpuseragent" {
				masked[key] = "***masked***"
				continue
			}
			masked[key] = maskAuditExportDetails(child)
		}
		return masked
	case []interface{}:
		masked := make([]interface{}, len(current))
		for index, child := range current {
			masked[index] = maskAuditExportDetails(child)
		}
		return masked
	default:
		return current
	}
}

func buildAuditGovernancePDF(tenantID string, logs []auditGovernanceLog) []byte {
	lines := []string{"Audit Governance Export", "Tenant: " + tenantID, fmt.Sprintf("Rows: %d", len(logs))}
	for _, log := range logs {
		lines = append(lines, fmt.Sprintf("%s %s %s %s result=%s risk=%s request=%s trace=%s", time.UnixMilli(log.Timestamp).UTC().Format(time.RFC3339), log.LogID, log.Action, log.ResourceID, log.Result, log.Risk, log.RequestID, log.TraceID))
		detailsJSON, err := json.Marshal(log.Details)
		if err == nil {
			lines = append(lines, "details="+string(detailsJSON))
		}
	}
	const linesPerPage = 72
	pageCount := (len(lines) + linesPerPage - 1) / linesPerPage
	if pageCount == 0 {
		pageCount = 1
	}
	fontID := 3 + pageCount*2
	kids := make([]string, 0, pageCount)
	objects := make([]string, fontID)
	objects[0] = "<< /Type /Catalog /Pages 2 0 R >>"
	for page := 0; page < pageCount; page++ {
		pageID := 3 + page*2
		contentID := pageID + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", pageID))
		start := page * linesPerPage
		end := start + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		var stream strings.Builder
		stream.WriteString("BT /F1 7 Tf 24 760 Td 9 TL ")
		for index, line := range lines[start:end] {
			if index > 0 {
				stream.WriteString("T* ")
			}
			stream.WriteString("(" + pdfEscape(line) + ") Tj ")
		}
		stream.WriteString("ET")
		objects[pageID-1] = fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", fontID, contentID)
		objects[contentID-1] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", stream.Len(), stream.String())
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pageCount)
	objects[fontID-1] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for index, object := range objects {
		offsets = append(offsets, output.Len())
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", index+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer << /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return output.Bytes()
}

func decodeAuditGovernanceJSON(r *http.Request, destination interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON payload: %w", err)
	}
	if err := ensureJSONBodyComplete(decoder); err != nil {
		return fmt.Errorf("invalid JSON payload: %w", err)
	}
	return nil
}

func validAuditRisk(value string) bool {
	return stringIn(value, "low", "medium", "high", "critical")
}

func stringIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
