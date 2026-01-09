////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/api/token_handler.go
// 完整修复版 v5：
// 1. 新增完整的 UpdateToken 方法（修复 #6）
// 2. 完善审计日志记录
// 3. 统一错误处理
////////////////////////////////////////////////////////////////////////////////

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/middleware"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

// TokenHandler API Token HTTP 处理器
type TokenHandler struct {
	tokenService   *service.TokenService
	authMiddleware *middleware.AuthMiddleware
	auditLogger    *audit.Logger
	logger         *zap.Logger
}

// NewTokenHandler 创建 Token Handler（基础版本）
func NewTokenHandler(
	tokenService *service.TokenService,
	authMiddleware *middleware.AuthMiddleware,
	logger *zap.Logger,
) *TokenHandler {
	return &TokenHandler{
		tokenService:   tokenService,
		authMiddleware: authMiddleware,
		logger:         logger,
	}
}

// NewTokenHandlerWithAudit 创建带审计的 Token Handler（推荐）
func NewTokenHandlerWithAudit(
	tokenService *service.TokenService,
	authMiddleware *middleware.AuthMiddleware,
	auditLogger *audit.Logger,
	logger *zap.Logger,
) *TokenHandler {
	return &TokenHandler{
		tokenService:   tokenService,
		authMiddleware: authMiddleware,
		auditLogger:    auditLogger,
		logger:         logger,
	}
}

// RegisterRoutes 注册路由（修复 #6：新增 UpdateToken 路由）
func (h *TokenHandler) RegisterRoutes(r *mux.Router) {
	// Token 管理路由（需要认证）
	tokenRouter := r.PathPrefix("/api/v1/tokens").Subrouter()
	tokenRouter.Use(h.authMiddleware.Authenticate)

	// CRUD 操作
	tokenRouter.HandleFunc("", h.ListTokens).Methods("GET")
	tokenRouter.HandleFunc("", h.CreateToken).Methods("POST")
	tokenRouter.HandleFunc("/{token_id}", h.GetToken).Methods("GET")
	tokenRouter.HandleFunc("/{token_id}", h.UpdateToken).Methods("PUT") // ✅ 新增
	tokenRouter.HandleFunc("/{token_id}", h.DeleteToken).Methods("DELETE")
	tokenRouter.HandleFunc("/{token_id}/revoke", h.RevokeToken).Methods("POST")
	tokenRouter.HandleFunc("/{token_id}/scopes", h.UpdateScopes).Methods("PUT")
	tokenRouter.HandleFunc("/{token_id}/regenerate", h.RegenerateToken).Methods("POST")

	// 探针专用端点
	tokenRouter.HandleFunc("/probe", h.CreateProbeToken).Methods("POST")

	// 验证端点（用于探针自检）
	r.HandleFunc("/api/v1/tokens/validate", h.ValidateToken).Methods("POST")

	// 获取有效 scopes 列表
	tokenRouter.HandleFunc("/scopes", h.GetScopes).Methods("GET")
	tokenRouter.HandleFunc("/scopes/probe", h.GetProbeScopes).Methods("GET")
}

// CreateTokenRequest 创建 Token 请求体
type CreateTokenRequest struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Scopes       []string `json:"scopes"`
	ProbeID      string   `json:"probe_id,omitempty"`
	ExpiresInSec *int64   `json:"expires_in_sec,omitempty"` // 过期时间（秒）
}

// CreateToken 创建新 Token
// POST /api/v1/tokens
func (h *TokenHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 检查权限
	if !claims.HasPermission("admin:write") && !claims.HasPermission("token:write") {
		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "create_token", "permission_denied")
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied,
			"Permission denied: admin:write or token:write required",
			httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	var req CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidRequest,
			"Invalid request body", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 验证必填字段
	if req.Name == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"name is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if len(req.Scopes) == 0 {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"scopes is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 构建请求
	createReq := &service.CreateTokenRequest{
		TenantID:    claims.TenantID,
		Name:        req.Name,
		Description: req.Description,
		Scopes:      req.Scopes,
		ProbeID:     req.ProbeID,
		CreatedBy:   claims.UserID,
	}

	if req.ExpiresInSec != nil {
		d := time.Duration(*req.ExpiresInSec) * time.Second
		createReq.ExpiresIn = &d
	}

	// 创建 Token
	resp, err := h.tokenService.CreateToken(r.Context(), createReq)
	if err != nil {
		h.logger.Error("Failed to create token",
			zap.String("tenant_id", claims.TenantID),
			zap.String("name", req.Name),
			zap.Error(err))

		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "create_token", err.Error())

		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	h.logger.Info("Token created via API",
		zap.String("token_id", resp.TokenID.String()),
		zap.String("tenant_id", claims.TenantID),
		zap.String("created_by", claims.UserID.String()))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// CreateProbeTokenRequest 创建探针 Token 请求
type CreateProbeTokenRequest struct {
	ProbeID string `json:"probe_id"`
	Name    string `json:"name"`
}

// CreateProbeToken 创建探针专用 Token（预设 scopes）
// POST /api/v1/tokens/probe
func (h *TokenHandler) CreateProbeToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 检查权限
	if !claims.HasPermission("admin:write") && !claims.HasPermission("token:write") {
		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "create_probe_token", "permission_denied")
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied,
			"Permission denied: admin:write or token:write required",
			httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	var req CreateProbeTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidRequest,
			"Invalid request body", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 验证必填字段
	if req.ProbeID == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"probe_id is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if req.Name == "" {
		req.Name = "Probe Token - " + req.ProbeID
	}

	// 创建探针 Token
	resp, err := h.tokenService.CreateProbeToken(r.Context(), claims.TenantID, req.ProbeID, req.Name, claims.UserID)
	if err != nil {
		h.logger.Error("Failed to create probe token",
			zap.String("tenant_id", claims.TenantID),
			zap.String("probe_id", req.ProbeID),
			zap.Error(err))

		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "create_probe_token", err.Error())

		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 记录审计成功
	if h.auditLogger != nil {
		h.auditLogger.LogEvent(r.Context(), &audit.AuditEvent{
			EventType:    audit.EventTypeTokenCreate,
			TenantID:     claims.TenantID,
			UserID:       claims.UserID.String(),
			Action:       "create_probe_token",
			ResourceType: "api_token",
			ResourceID:   resp.TokenID.String(),
			Detail: map[string]interface{}{
				"probe_id": req.ProbeID,
				"name":     req.Name,
				"scopes":   resp.Scopes,
			},
			Result:    audit.ResultSuccess,
			IPAddr:    httpx.GetClientIP(r),
			UserAgent: r.UserAgent(),
		})
	}

	h.logger.Info("Probe token created via API",
		zap.String("token_id", resp.TokenID.String()),
		zap.String("probe_id", req.ProbeID),
		zap.String("tenant_id", claims.TenantID))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// ListTokens 列出 Token
// GET /api/v1/tokens
func (h *TokenHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 检查权限
	if !claims.HasPermission("admin:read") && !claims.HasPermission("token:read") {
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied,
			"Permission denied", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 获取分页参数
	limit := parseLimit(r, 20, 100)
	offset := parseOffset(r)

	tokens, total, err := h.tokenService.ListTokens(r.Context(), claims.TenantID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to list tokens",
			zap.String("tenant_id", claims.TenantID),
			zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tokens": tokens,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetToken 获取单个 Token
// GET /api/v1/tokens/{token_id}
func (h *TokenHandler) GetToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 解析 token_id（修复 #10：增强 UUID 解析）
	vars := mux.Vars(r)
	tokenIDStr := vars["token_id"]
	if tokenIDStr == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"token_id is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidParameter,
			"Invalid token_id format: must be UUID", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	token, err := h.tokenService.GetToken(r.Context(), claims.TenantID, tokenID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeEntityNotFound) {
			errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		} else {
			errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(token)
}

// UpdateTokenRequest 更新 Token 请求（修复 #6：新增完整更新请求）
type UpdateTokenRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Scopes      []string `json:"scopes,omitempty"`
	IPWhitelist []string `json:"ip_whitelist,omitempty"`
	ExpiresAt   *int64   `json:"expires_at,omitempty"` // Unix 时间戳（秒）
}

// UpdateToken 更新 Token（修复 #6：新增完整 UpdateToken 方法）
// PUT /api/v1/tokens/{token_id}
func (h *TokenHandler) UpdateToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 检查权限
	if !claims.HasPermission("admin:write") && !claims.HasPermission("token:write") {
		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "update_token", "permission_denied")
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied,
			"Permission denied", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 解析 token_id
	vars := mux.Vars(r)
	tokenIDStr := vars["token_id"]
	if tokenIDStr == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"token_id is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidParameter,
			"Invalid token_id format: must be UUID", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 解析请求体
	var req UpdateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidRequest,
			"Invalid request body", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 验证至少有一个字段需要更新
	if req.Name == nil && req.Description == nil && len(req.Scopes) == 0 &&
		len(req.IPWhitelist) == 0 && req.ExpiresAt == nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidRequest,
			"At least one field must be provided for update", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 转换 ExpiresAt
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t := time.Unix(*req.ExpiresAt, 0)
		expiresAt = &t
	}

	// 构建 Service 层请求
	serviceReq := &service.UpdateTokenRequest{
		Name:        req.Name,
		Description: req.Description,
		Scopes:      req.Scopes,
		IPWhitelist: req.IPWhitelist,
		ExpiresAt:   expiresAt,
	}

	// 调用 Service 层更新
	updatedToken, err := h.tokenService.UpdateToken(r.Context(), claims.TenantID, tokenID, serviceReq, claims.UserID)
	if err != nil {
		h.logger.Error("Failed to update token",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))

		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "update_token", err.Error())

		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 记录审计日志
	if h.auditLogger != nil {
		h.auditLogger.LogEvent(r.Context(), &audit.AuditEvent{
			EventType:    audit.EventTypeTokenCreate,
			TenantID:     claims.TenantID,
			UserID:       claims.UserID.String(),
			Action:       "update_token",
			ResourceType: "api_token",
			ResourceID:   tokenID.String(),
			Detail: map[string]interface{}{
				"updated_fields": buildUpdatedFieldsList(&req),
			},
			Result:    audit.ResultSuccess,
			IPAddr:    httpx.GetClientIP(r),
			UserAgent: r.UserAgent(),
		})
	}

	h.logger.Info("Token updated",
		zap.String("token_id", tokenID.String()),
		zap.String("updated_by", claims.UserID.String()))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedToken)
}

// RevokeToken 撤销 Token
// POST /api/v1/tokens/{token_id}/revoke
func (h *TokenHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 检查权限
	if !claims.HasPermission("admin:write") && !claims.HasPermission("token:write") {
		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "revoke_token", "permission_denied")
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied,
			"Permission denied", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	vars := mux.Vars(r)
	tokenIDStr := vars["token_id"]
	if tokenIDStr == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"token_id is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidParameter,
			"Invalid token_id format: must be UUID", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if err := h.tokenService.RevokeToken(r.Context(), claims.TenantID, tokenID, claims.UserID); err != nil {
		h.logger.Error("Failed to revoke token",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Token revoked successfully",
	})
}

// DeleteToken 删除 Token
// DELETE /api/v1/tokens/{token_id}
func (h *TokenHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 检查权限
	if !claims.HasPermission("admin:write") && !claims.HasPermission("token:write") {
		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "delete_token", "permission_denied")
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied,
			"Permission denied", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	vars := mux.Vars(r)
	tokenIDStr := vars["token_id"]
	if tokenIDStr == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"token_id is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidParameter,
			"Invalid token_id format: must be UUID", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if err := h.tokenService.DeleteToken(r.Context(), claims.TenantID, tokenID, claims.UserID); err != nil {
		h.logger.Error("Failed to delete token",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateScopesRequest 更新权限请求
type UpdateScopesRequest struct {
	Scopes []string `json:"scopes"`
}

// UpdateScopes 更新 Token 权限
// PUT /api/v1/tokens/{token_id}/scopes
func (h *TokenHandler) UpdateScopes(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 检查权限
	if !claims.HasPermission("admin:write") && !claims.HasPermission("token:write") {
		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "update_scopes", "permission_denied")
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied,
			"Permission denied", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	vars := mux.Vars(r)
	tokenIDStr := vars["token_id"]
	if tokenIDStr == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"token_id is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidParameter,
			"Invalid token_id format: must be UUID", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	var req UpdateScopesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidRequest,
			"Invalid request body", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if len(req.Scopes) == 0 {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"scopes is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if err := h.tokenService.UpdateTokenScopes(r.Context(), claims.TenantID, tokenID, req.Scopes, claims.UserID); err != nil {
		h.logger.Error("Failed to update token scopes",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Scopes updated successfully",
	})
}

// RegenerateToken 重新生成 Token
// POST /api/v1/tokens/{token_id}/regenerate
func (h *TokenHandler) RegenerateToken(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetClaims(r.Context())
	if claims == nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeUnauthorized,
			"Unauthorized", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	// 检查权限
	if !claims.HasPermission("admin:write") && !claims.HasPermission("token:write") {
		h.recordAuditFailure(r.Context(), claims.TenantID, claims.UserID.String(), "regenerate_token", "permission_denied")
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied,
			"Permission denied", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	vars := mux.Vars(r)
	tokenIDStr := vars["token_id"]
	if tokenIDStr == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"token_id is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	tokenID, err := uuid.Parse(tokenIDStr)
	if err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidParameter,
			"Invalid token_id format: must be UUID", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	resp, err := h.tokenService.RegenerateToken(r.Context(), claims.TenantID, tokenID, claims.UserID)
	if err != nil {
		h.logger.Error("Failed to regenerate token",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// ValidateTokenRequest 验证 Token 请求
type ValidateTokenRequest struct {
	Token string `json:"token"`
}

// ValidateToken 验证 Token（用于探针自检）
// POST /api/v1/tokens/validate
func (h *TokenHandler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	var req ValidateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeInvalidRequest,
			"Invalid request body", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	if req.Token == "" {
		errors.WriteErrorWithStatus(w, http.StatusBadRequest, errors.ErrCodeMissingParameter,
			"token is required", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	token, err := h.tokenService.ValidateToken(r.Context(), req.Token)
	if err != nil {
		errors.WriteErrorWithStatus(w, http.StatusUnauthorized, errors.ErrCodeTokenInvalid,
			"Invalid token", httpx.GetTraceID(r.Context()), r.URL.Path)
		return
	}

	scopes := model.ScopesToList(token.Scopes)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"valid":      true,
		"tenant_id":  token.TenantID,
		"name":       token.Name,
		"scopes":     scopes,
		"probe_id":   token.ProbeID,
		"expires_at": token.ExpiresAt,
	})
}

// GetScopes 获取有效的 scopes 列表
// GET /api/v1/tokens/scopes
func (h *TokenHandler) GetScopes(w http.ResponseWriter, r *http.Request) {
	scopes := h.tokenService.GetAvailableScopes()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scopes": scopes,
	})
}

// GetProbeScopes 获取探针专用 scopes 列表
// GET /api/v1/tokens/scopes/probe
func (h *TokenHandler) GetProbeScopes(w http.ResponseWriter, r *http.Request) {
	scopes := h.tokenService.GetProbeScopes()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"scopes":         scopes,
		"default_scopes": model.DefaultProbeScopes,
		"full_scopes":    model.ProbeFullScopes,
		"minimal_scopes": model.ProbeMinimalScopes,
	})
}

// =============================================================================
// 辅助函数
// =============================================================================

// parseLimit 解析 limit 参数
func parseLimit(r *http.Request, defaultLimit, maxLimit int) int {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return defaultLimit
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// parseOffset 解析 offset 参数
func parseOffset(r *http.Request) int {
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		return 0
	}
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

// recordAuditFailure 记录审计失败
func (h *TokenHandler) recordAuditFailure(ctx context.Context, tenantID, userID, action, errorMsg string) {
	if h.auditLogger == nil {
		return
	}

	h.auditLogger.LogEvent(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeTokenCreate,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       action + "_failed",
		ResourceType: "api_token",
		Result:       audit.ResultFailure,
		ErrorMsg:     errorMsg,
	})
}

// buildUpdatedFieldsList 构建已更新字段列表（用于审计）
func buildUpdatedFieldsList(req *UpdateTokenRequest) []string {
	fields := []string{}
	if req.Name != nil {
		fields = append(fields, "name")
	}
	if req.Description != nil {
		fields = append(fields, "description")
	}
	if len(req.Scopes) > 0 {
		fields = append(fields, "scopes")
	}
	if len(req.IPWhitelist) > 0 {
		fields = append(fields, "ip_whitelist")
	}
	if req.ExpiresAt != nil {
		fields = append(fields, "expires_at")
	}
	return fields
}
