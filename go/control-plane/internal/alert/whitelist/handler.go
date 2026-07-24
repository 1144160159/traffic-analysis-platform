package whitelist

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

// Handler 白名单 API 处理器
type Handler struct {
	repo   *Repository
	logger *zap.Logger
}

func NewHandler(repo *Repository, logger *zap.Logger) *Handler {
	return &Handler{repo: repo, logger: logger}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/whitelist", h.List).Methods("GET")
	r.HandleFunc("/whitelist", h.Create).Methods("POST")
	r.HandleFunc("/whitelist/{id}", h.Update).Methods("PATCH")
	r.HandleFunc("/whitelist/{id}", h.Delete).Methods("DELETE")
	r.HandleFunc("/whitelist/check", h.Check).Methods("POST")
}

// List 列出租户白名单
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireRead(w, r) || !h.ensureRepo(w, r) {
		return
	}
	tenantID := tenantFromContext(ctx)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	entries, total, err := h.repo.List(ctx, tenantID, limit, offset)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"entries": entries, "total": total, "limit": limit, "offset": offset})
}

// Create 创建白名单条目
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireWrite(w, r) || !h.ensureRepo(w, r) {
		return
	}
	var entry Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid body")
		return
	}
	entry.TenantID = tenantFromContext(ctx)
	entry.CreatedBy = httpx.GetUserID(ctx)
	entry.Type = normalizeType(entry.Type)
	if entry.Type == "" || strings.TrimSpace(entry.Value) == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "MISSING_PARAM", "type and value required")
		return
	}
	if entry.Status != "" && !strings.EqualFold(strings.TrimSpace(entry.Status), "draft") {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_INITIAL_STATE", "new whitelist entries must start in draft status")
		return
	}
	if entry.ApprovalStatus != "" && !strings.EqualFold(strings.TrimSpace(entry.ApprovalStatus), "draft") {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_INITIAL_STATE", "new whitelist entries must start with draft approval_status")
		return
	}
	entry.Status = "draft"
	entry.ApprovalStatus = "draft"
	tx, err := h.repo.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	if err := h.repo.CreateTx(ctx, tx, &entry); err != nil {
		if err == ErrAlreadyExists {
			httpx.JSONError(w, ctx, http.StatusConflict, "WHITELIST_ALREADY_EXISTS", "a whitelist entry with the same tenant, type and value already exists")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := h.recordAuditWithRunner(ctx, r, tx, "WHITELIST_CREATED", entry.ID, map[string]interface{}{
		"type":            entry.Type,
		"value":           entry.Value,
		"status":          entry.Status,
		"approval_status": entry.ApprovalStatus,
		"source_alert_id": entry.SourceAlertID,
		"feedback_id":     entry.FeedbackID,
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "whitelist entry was not created because its audit record could not be committed")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONCreated(w, ctx, &entry)
}

// Update 更新白名单治理状态: 审批、延期、停用与说明变更
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireWrite(w, r) || !h.ensureRepo(w, r) {
		return
	}
	id := mux.Vars(r)["id"]
	if id == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "MISSING_PARAM", "id required")
		return
	}
	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "INVALID_REQUEST", "invalid body")
		return
	}
	tx, err := h.repo.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	current, err := h.repo.getWithRunner(ctx, tx, tenantFromContext(ctx), id)
	if err != nil {
		if err == sql.ErrNoRows {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "whitelist entry not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if req.ExpectedVersion == nil {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "MISSING_PARAM", "expected_version required for whitelist updates")
		return
	}
	if req.ExpectedVersion != nil && *req.ExpectedVersion != current.Version {
		httpx.JSONError(w, ctx, http.StatusConflict, "WHITELIST_VERSION_CONFLICT", "expected_version must match the current whitelist entry")
		return
	}
	if code, message := validateWhitelistTransition(current, req, httpx.GetUserID(ctx), canApproveWhitelist(ctx)); code != "" {
		status := http.StatusConflict
		if code == "PERMISSION_DENIED" {
			status = http.StatusForbidden
		}
		httpx.JSONError(w, ctx, status, code, message)
		return
	}
	entry, err := h.repo.UpdateTx(ctx, tx, tenantFromContext(ctx), id, req, httpx.GetUserID(ctx))
	if err != nil {
		if err == ErrVersionConflict {
			httpx.JSONError(w, ctx, http.StatusConflict, "WHITELIST_VERSION_CONFLICT", "whitelist entry changed; reload before retrying")
			return
		}
		if err == sql.ErrNoRows {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "whitelist entry not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	action := whitelistAuditAction(req)
	if err := h.recordAuditWithRunner(ctx, r, tx, action, entry.ID, map[string]interface{}{
		"type":            entry.Type,
		"value":           entry.Value,
		"status":          entry.Status,
		"approval_status": entry.ApprovalStatus,
		"expires_at":      entry.ExpiresAt,
		"source_alert_id": entry.SourceAlertID,
	}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "whitelist update was rolled back because its audit record could not be committed")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, entry)
}

// Delete 删除白名单条目
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireWrite(w, r) || !h.ensureRepo(w, r) {
		return
	}
	id := mux.Vars(r)["id"]
	tenantID := tenantFromContext(ctx)
	tx, err := h.repo.db.BeginTx(ctx, nil)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	defer tx.Rollback()
	current, err := h.repo.getWithRunner(ctx, tx, tenantID, id)
	if err != nil {
		if err == sql.ErrNoRows {
			httpx.JSONError(w, ctx, http.StatusNotFound, "NOT_FOUND", "whitelist entry not found")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	expectedVersion, err := strconv.Atoi(r.URL.Query().Get("expected_version"))
	if err != nil || expectedVersion <= 0 {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "MISSING_PARAM", "expected_version query parameter required for whitelist deletion")
		return
	}
	if expectedVersion != current.Version {
		httpx.JSONError(w, ctx, http.StatusConflict, "WHITELIST_VERSION_CONFLICT", "expected_version must match the current whitelist entry")
		return
	}
	if current.Status != "draft" && current.Status != "disabled" {
		httpx.JSONError(w, ctx, http.StatusConflict, "WHITELIST_DELETE_REQUIRES_DISABLED", "pending or active whitelist entries must be disabled before deletion")
		return
	}
	if err := h.repo.DeleteTx(ctx, tx, tenantID, id, expectedVersion); err != nil {
		if err == sql.ErrNoRows {
			httpx.JSONError(w, ctx, http.StatusConflict, "WHITELIST_VERSION_CONFLICT", "whitelist entry changed before deletion")
			return
		}
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if err := h.recordAuditWithRunner(ctx, r, tx, "WHITELIST_DELETED", id, map[string]interface{}{"version": expectedVersion, "previous_status": current.Status}); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_WRITE_FAILED", "whitelist deletion was rolled back because its audit record could not be committed")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]string{"status": "deleted", "id": id})
}

// Check 检查值是否在白名单中
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !h.requireRead(w, r) || !h.ensureRepo(w, r) {
		return
	}
	var req struct {
		TenantID string `json:"tenant_id"`
		Value    string `json:"value"`
		Type     string `json:"type"` // ip | domain | fingerprint
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Value == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "MISSING_PARAM", "value required")
		return
	}
	whitelisted := h.repo.IsWhitelisted(ctx, tenantFromContext(ctx), req.Value)
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"value": req.Value, "whitelisted": whitelisted})
}

func (h *Handler) ensureRepo(w http.ResponseWriter, r *http.Request) bool {
	if h == nil || h.repo == nil || h.repo.db == nil {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "whitelist repository unavailable")
		return false
	}
	return true
}

func (h *Handler) requireWrite(w http.ResponseWriter, r *http.Request) bool {
	if hasWhitelistWrite(r.Context()) {
		return true
	}
	httpx.JSONError(w, r.Context(), http.StatusForbidden, "PERMISSION_DENIED", "alert:write required")
	return false
}

func (h *Handler) requireRead(w http.ResponseWriter, r *http.Request) bool {
	if hasWhitelistRead(r.Context()) {
		return true
	}
	httpx.JSONError(w, r.Context(), http.StatusForbidden, "PERMISSION_DENIED", "alert:read required")
	return false
}

func hasWhitelistRead(ctx context.Context) bool {
	if hasWhitelistWrite(ctx) {
		return true
	}
	if claims := httpx.GetExtendedClaims(ctx); claims != nil {
		return claims.HasPermission(authmodel.ScopeAlertRead)
	}
	for _, granted := range httpx.GetPermissions(ctx) {
		if permissionMatches(granted, authmodel.ScopeAlertRead) {
			return true
		}
	}
	return false
}

func hasWhitelistWrite(ctx context.Context) bool {
	if claims := httpx.GetExtendedClaims(ctx); claims != nil {
		return claims.HasRole("admin") ||
			claims.HasRole("super_admin") ||
			claims.HasPermission(authmodel.ScopeAll) ||
			claims.HasPermission(authmodel.ScopeAdminAll) ||
			claims.HasPermission(authmodel.ScopeAlertWrite)
	}
	if httpx.HasRole(ctx, "admin") || httpx.HasRole(ctx, "super_admin") {
		return true
	}
	for _, granted := range httpx.GetPermissions(ctx) {
		if permissionMatches(granted, authmodel.ScopeAlertWrite) ||
			permissionMatches(granted, authmodel.ScopeAdminAll) ||
			granted == authmodel.ScopeAll {
			return true
		}
	}
	return false
}

func canApproveWhitelist(ctx context.Context) bool {
	if claims := httpx.GetExtendedClaims(ctx); claims != nil {
		return claims.HasRole("admin") || claims.HasRole("super_admin") || claims.HasPermission(authmodel.ScopeAll) || claims.HasPermission(authmodel.ScopeAdminAll)
	}
	return httpx.HasRole(ctx, "admin") || httpx.HasRole(ctx, "super_admin")
}

func validateWhitelistTransition(current *Entry, req UpdateRequest, actor string, canApprove bool) (string, string) {
	if current == nil {
		return "NOT_FOUND", "whitelist entry not found"
	}
	nextStatus := current.Status
	if req.Status != nil {
		nextStatus = normalizeStatus(*req.Status, "")
		if nextStatus == "" {
			return "INVALID_TRANSITION", "unsupported whitelist status"
		}
	}
	nextApproval := current.ApprovalStatus
	if req.ApprovalStatus != nil {
		nextApproval = normalizeApprovalStatus(*req.ApprovalStatus, "")
		if nextApproval == "" {
			return "INVALID_TRANSITION", "unsupported whitelist approval_status"
		}
	}

	if !validWhitelistStatePair(nextStatus, nextApproval) {
		return "INVALID_TRANSITION", "status and approval_status do not form a legal whitelist governance state"
	}
	currentPair := current.Status + "/" + current.ApprovalStatus
	nextPair := nextStatus + "/" + nextApproval
	if currentPair != nextPair {
		switch currentPair + "->" + nextPair {
		case "draft/draft->pending/pending":
		case "pending/pending->active/approved", "pending/pending->disabled/rejected":
			if !canApprove {
				return "PERMISSION_DENIED", "admin approval role required"
			}
			if actor != "" && actor == current.CreatedBy {
				return "WHITELIST_TWO_PERSON_REQUIRED", "creator cannot approve or reject their own whitelist entry"
			}
		case "active/approved->disabled/approved":
		default:
			return "INVALID_TRANSITION", "unsupported whitelist lifecycle transition"
		}
	}
	return "", ""
}

func validWhitelistStatePair(status, approval string) bool {
	return (status == "draft" && approval == "draft") ||
		(status == "pending" && approval == "pending") ||
		(status == "active" && approval == "approved") ||
		(status == "disabled" && (approval == "approved" || approval == "rejected"))
}

func permissionMatches(granted, required string) bool {
	granted = strings.TrimSpace(granted)
	required = strings.TrimSpace(required)
	if granted == "" || required == "" {
		return false
	}
	if granted == authmodel.ScopeAll || granted == required {
		return true
	}
	if strings.HasSuffix(granted, ":*") {
		return strings.HasPrefix(required, strings.TrimSuffix(granted, "*"))
	}
	return false
}

func tenantFromContext(ctx context.Context) string {
	tenantID := httpx.GetTenantID(ctx)
	if tenantID == "" {
		return "default"
	}
	return tenantID
}

func whitelistAuditAction(req UpdateRequest) string {
	if req.Status != nil && strings.EqualFold(strings.TrimSpace(*req.Status), "disabled") {
		return "WHITELIST_DISABLED"
	}
	if req.ApprovalStatus != nil {
		switch strings.ToLower(strings.TrimSpace(*req.ApprovalStatus)) {
		case "pending":
			return "WHITELIST_APPROVAL_SUBMITTED"
		case "approved":
			return "WHITELIST_APPROVED"
		case "rejected":
			return "WHITELIST_REJECTED"
		}
	}
	if req.ExpiresAt != nil {
		return "WHITELIST_EXTENDED"
	}
	return "WHITELIST_UPDATED"
}

func (h *Handler) recordAudit(ctx context.Context, r *http.Request, action, objectID string, detail map[string]interface{}) {
	if err := h.recordAuditWithRunner(ctx, r, h.repo.db, action, objectID, detail); err != nil && h.logger != nil {
		h.logger.Warn("Failed to write whitelist audit log", zap.String("action", action), zap.String("object_id", objectID), zap.Error(err))
	}
}

func (h *Handler) recordAuditWithRunner(ctx context.Context, r *http.Request, runner sqlRunner, action, objectID string, detail map[string]interface{}) error {
	if h == nil || h.repo == nil || h.repo.db == nil {
		return sql.ErrConnDone
	}
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detail["result"] = "success"
	detail["request_id"] = httpx.GetRequestID(ctx)
	detail["trace_id"] = httpx.GetTraceID(ctx)
	if r != nil && r.URL != nil {
		detail["api_path"] = r.URL.Path
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("Failed to marshal whitelist audit detail", zap.Error(err))
		}
		return err
	}

	userIDExpr := "NULLIF($3, '')"
	userID := httpx.GetUserID(ctx)
	if h.pgColumnType(r.Context(), "audit_logs", "user_id") == "uuid" {
		userIDExpr = "NULLIF($3, '')::uuid"
		if userID != "" {
			if _, err := uuid.Parse(userID); err != nil {
				userID = ""
			}
		}
	}

	var query string
	args := []interface{}{tenantFromContext(ctx), userID, action, "whitelist", objectID, string(detailJSON), clientIP(r), r.UserAgent()}
	if h.pgColumnExists(r.Context(), "audit_logs", "event_id") {
		query = `INSERT INTO audit_logs (event_id, tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, $2, ` + userIDExpr + `, $4, $5, $6, $7::jsonb, $8, $9)`
		args = append([]interface{}{"audit-" + uuid.NewString()}, args...)
	} else {
		query = `INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, ` + strings.Replace(userIDExpr, "$3", "$2", 1) + `, $3, $4, $5, $6::jsonb, $7, $8)`
	}
	_, err = runner.ExecContext(r.Context(), query, args...)
	return err
}

func (h *Handler) pgColumnExists(ctx context.Context, tableName, columnName string) bool {
	if h == nil || h.repo == nil || h.repo.db == nil {
		return false
	}
	var exists bool
	err := h.repo.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)`, tableName, columnName).Scan(&exists)
	if err != nil && h.logger != nil {
		h.logger.Debug("Failed to inspect whitelist audit column existence", zap.Error(err))
	}
	return err == nil && exists
}

func (h *Handler) pgColumnType(ctx context.Context, tableName, columnName string) string {
	if h == nil || h.repo == nil || h.repo.db == nil {
		return ""
	}
	var dataType string
	err := h.repo.db.QueryRowContext(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_name = $1 AND column_name = $2
		ORDER BY CASE WHEN table_schema = 'public' THEN 0 ELSE 1 END
		LIMIT 1`, tableName, columnName).Scan(&dataType)
	if err != nil && h.logger != nil {
		h.logger.Debug("Failed to inspect whitelist audit column type", zap.Error(err))
	}
	return dataType
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}
