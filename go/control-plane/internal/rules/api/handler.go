////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/api/handler.go
// Rule Manager API Handler - 完整修复版
// 修复内容：
// 1. ✅ extractOperationContext 返回 error，添加必填字段验证
// 2. ✅ 所有 Handler 方法检查 extractOperationContext 的返回值
// 3. ✅ 统一错误响应格式（全部使用 errors.WriteError）
// 4. ✅ 完整的输入验证（长度、格式、业务逻辑）
// 5. ✅ 分页参数处理（限制最大值）
// 6. ✅ 批量操作 API
// 7. ✅ 导入导出 API
// 8. ✅ 规则同步 API
// 9. ✅ 统计 API
////////////////////////////////////////////////////////////////////////////////

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/rbac"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/service"
)

// =============================================================================
// Handler 配置
// =============================================================================

// HandlerConfig Handler 配置
type HandlerConfig struct {
	EnableRBAC       bool          `env:"HANDLER_ENABLE_RBAC" envDefault:"true"`
	EnableAudit      bool          `env:"HANDLER_ENABLE_AUDIT" envDefault:"true"`
	MaxPageSize      int           `env:"HANDLER_MAX_PAGE_SIZE" envDefault:"100"`
	DefaultPageSize  int           `env:"HANDLER_DEFAULT_PAGE_SIZE" envDefault:"20"`
	RequestTimeout   time.Duration `env:"HANDLER_REQUEST_TIMEOUT" envDefault:"30s"`
	EnableRequestLog bool          `env:"HANDLER_ENABLE_REQUEST_LOG" envDefault:"true"`
	MaxRequestSize   int64         `env:"HANDLER_MAX_REQUEST_SIZE" envDefault:"10485760"` // 10MB
}

// DefaultHandlerConfig 默认配置
func DefaultHandlerConfig() HandlerConfig {
	return HandlerConfig{
		EnableRBAC:       true,
		EnableAudit:      true,
		MaxPageSize:      100,
		DefaultPageSize:  20,
		RequestTimeout:   30 * time.Second,
		EnableRequestLog: true,
		MaxRequestSize:   10 * 1024 * 1024, // 10MB
	}
}

// =============================================================================
// Handler 定义
// =============================================================================

// Handler API 处理器
type Handler struct {
	ruleService       *service.RuleService
	deploymentService *service.DeploymentService
	auditLogger       *audit.Logger
	rbacChecker       *rbac.Checker
	config            HandlerConfig
	logger            *zap.Logger
}

// NewHandler 创建 Handler
func NewHandler(
	ruleService *service.RuleService,
	deploymentService *service.DeploymentService,
	auditLogger *audit.Logger,
	rbacChecker *rbac.Checker,
	logger *zap.Logger,
	config HandlerConfig,
) *Handler {
	return &Handler{
		ruleService:       ruleService,
		deploymentService: deploymentService,
		auditLogger:       auditLogger,
		rbacChecker:       rbacChecker,
		config:            config,
		logger:            logger,
	}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *mux.Router) {
	// API 版本前缀
	api := r.PathPrefix("/api/v1").Subrouter()

	// 规则管理
	rules := api.PathPrefix("/rules").Subrouter()
	rules.HandleFunc("", h.ListRules).Methods("GET")
	rules.HandleFunc("", h.CreateRule).Methods("POST")
	rules.HandleFunc("/search", h.SearchRules).Methods("GET")
	rules.HandleFunc("/batch/enable", h.BatchEnableRules).Methods("POST")
	rules.HandleFunc("/batch/disable", h.BatchDisableRules).Methods("POST")
	rules.HandleFunc("/batch/delete", h.BatchDeleteRules).Methods("POST")
	rules.HandleFunc("/export", h.ExportRules).Methods("GET")
	rules.HandleFunc("/import", h.ImportRules).Methods("POST")
	rules.HandleFunc("/sync", h.SyncRules).Methods("POST")
	rules.HandleFunc("/stats", h.GetRuleStats).Methods("GET")
	rules.HandleFunc("/{id}", h.GetRule).Methods("GET")
	rules.HandleFunc("/{id}", h.UpdateRule).Methods("PUT")
	rules.HandleFunc("/{id}", h.DeleteRule).Methods("DELETE")
	rules.HandleFunc("/{id}/enable", h.EnableRule).Methods("POST")
	rules.HandleFunc("/{id}/disable", h.DisableRule).Methods("POST")
	rules.HandleFunc("/{id}/versions", h.GetRuleVersions).Methods("GET")

	// 部署管理
	deployments := api.PathPrefix("/deployments").Subrouter()
	deployments.HandleFunc("", h.ListDeployments).Methods("GET")
	deployments.HandleFunc("", h.CreateDeployment).Methods("POST")
	deployments.HandleFunc("/{id}", h.GetDeployment).Methods("GET")
	deployments.HandleFunc("/{id}/gray", h.StartGrayDeployment).Methods("POST")
	deployments.HandleFunc("/{id}/activate", h.ActivateDeployment).Methods("POST")
	deployments.HandleFunc("/{id}/rollback", h.RollbackDeployment).Methods("POST")
	deployments.HandleFunc("/{id}/pause", h.PauseDeployment).Methods("POST")
	deployments.HandleFunc("/{id}/resume", h.ResumeDeployment).Methods("POST")
	deployments.HandleFunc("/{id}/history", h.GetDeploymentHistory).Methods("GET")
	deployments.HandleFunc("/{id}/progress", h.GetGrayProgress).Methods("GET")
}

// =============================================================================
// 操作上下文提取 - 修复版（带必填字段验证）
// =============================================================================

// extractOperationContext 从请求中提取操作上下文（完整修复版）
// ✅ 修复：返回 error，添加必填字段验证
func (h *Handler) extractOperationContext(r *http.Request) (*service.OperationContext, error) {
	ctx := r.Context()

	// 第一层：从 Context 获取基础用户信息
	userID := httpx.GetUserID(ctx)
	tenantID := httpx.GetTenantID(ctx)
	username := httpx.GetUsername(ctx)
	roles := httpx.GetRoles(ctx)
	permissions := httpx.GetPermissions(ctx)

	// 第二层：如果 Context 中没有，尝试从 Header 获取
	if userID == "" {
		userID = r.Header.Get("X-User-ID")
	}
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-ID")
	}
	if username == "" {
		username = r.Header.Get("X-Username")
	}
	if len(roles) == 0 {
		if rolesHeader := r.Header.Get("X-Roles"); rolesHeader != "" {
			roles = strings.Split(rolesHeader, ",")
			for i := range roles {
				roles[i] = strings.TrimSpace(roles[i])
			}
		}
	}

	// 第三层：获取权限（优先级：Context > Header > 从角色推导）
	if len(permissions) == 0 {
		// 尝试从 Header 获取
		if permHeader := r.Header.Get("X-Permissions"); permHeader != "" {
			parts := strings.Split(permHeader, ",")
			permissions = make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					permissions = append(permissions, p)
				}
			}
		}
	}
	if len(permissions) == 0 && len(roles) > 0 {
		// 从角色推导权限
		permissions = h.derivePermissionsFromRoles(roles)
	}

	// ✅ 修复：必填字段验证
	if tenantID == "" {
		return nil, errors.New(errors.ErrCodeUnauthorized, "missing tenant_id in request context or header")
	}
	if userID == "" {
		return nil, errors.New(errors.ErrCodeUnauthorized, "missing user_id in request context or header")
	}

	// 构建操作上下文
	opCtx := &service.OperationContext{
		TenantID:    tenantID,
		UserID:      userID,
		Username:    username,
		Roles:       roles,
		Permissions: permissions,
		IPAddr:      httpx.GetClientIP(r),
		UserAgent:   r.Header.Get("User-Agent"),
	}

	// 日志记录（用于调试）
	h.logger.Debug("Extracted operation context",
		zap.String("tenant_id", opCtx.TenantID),
		zap.String("user_id", opCtx.UserID),
		zap.Strings("roles", opCtx.Roles),
		zap.Strings("permissions", opCtx.Permissions))

	return opCtx, nil
}

// derivePermissionsFromRoles 从角色推导权限
func (h *Handler) derivePermissionsFromRoles(roles []string) []string {
	perms := rbac.GetPermissionsFromRoles(roles)
	result := make([]string, len(perms))
	for i, p := range perms {
		result[i] = string(p)
	}
	return result
}

// =============================================================================
// 规则 API
// =============================================================================

// CreateRule 创建规则
func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	// 检查请求大小
	if r.ContentLength > h.config.MaxRequestSize {
		h.writeError(w, r, errors.Newf(errors.ErrCodeInvalidRequest, "request too large: max %d bytes", h.config.MaxRequestSize))
		return
	}

	// 解析请求体
	var req CreateRuleRequest
	if err := h.decodeJSON(r, &req); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}

	// 验证请求
	if err := req.Validate(); err != nil {
		h.writeError(w, r, err)
		return
	}

	// 构建规则
	rule := &model.Rule{
		TenantID:    opCtx.TenantID,
		Name:        req.Name,
		Type:        req.Type,
		Engine:      req.Engine,
		Description: req.Description,
		Conditions:  req.Conditions,
		Labels:      req.Labels,
		Severity:    req.Severity,
		Enabled:     req.Enabled,
		Priority:    req.Priority,
	}

	// 设置默认值
	if rule.Engine == "" {
		rule.Engine = string(model.EngineInternal)
	}
	if rule.Severity == "" {
		rule.Severity = string(model.SeverityMedium)
	}

	// 创建规则
	if err := h.ruleService.CreateRule(ctx, rule, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	// 返回结果
	h.writeJSON(w, http.StatusCreated, RuleResponse{
		Success: true,
		Data:    h.ruleToDTO(rule),
	})
}

// GetRule 获取规则
func (h *Handler) GetRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	ruleID := mux.Vars(r)["id"]
	if ruleID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "rule id is required"))
		return
	}

	rule, err := h.ruleService.GetRule(ctx, ruleID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, RuleResponse{
		Success: true,
		Data:    h.ruleToDTO(rule),
	})
}

// UpdateRule 更新规则
func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	ruleID := mux.Vars(r)["id"]
	if ruleID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "rule id is required"))
		return
	}

	// 解析请求体
	var req UpdateRuleRequest
	if err := h.decodeJSON(r, &req); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}

	// 验证请求
	if err := req.Validate(); err != nil {
		h.writeError(w, r, err)
		return
	}

	// 构建规则
	rule := &model.Rule{
		RuleID:      ruleID,
		Name:        req.Name,
		Type:        req.Type,
		Engine:      req.Engine,
		Description: req.Description,
		Conditions:  req.Conditions,
		Labels:      req.Labels,
		Severity:    req.Severity,
		Enabled:     req.Enabled,
		Priority:    req.Priority,
	}

	// 更新规则
	if err := h.ruleService.UpdateRule(ctx, rule, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	// 重新获取更新后的规则
	updatedRule, err := h.ruleService.GetRule(ctx, ruleID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, RuleResponse{
		Success: true,
		Data:    h.ruleToDTO(updatedRule),
	})
}

// DeleteRule 删除规则
func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	ruleID := mux.Vars(r)["id"]
	if ruleID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "rule id is required"))
		return
	}

	if err := h.ruleService.DeleteRule(ctx, ruleID, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "rule deleted successfully",
	})
}

// ListRules 列出规则
func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	// 解析查询参数
	filter := h.parseRuleFilter(r)

	rules, total, err := h.ruleService.ListRules(ctx, opCtx.TenantID, filter, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	// 转换为 DTO
	dtos := make([]*RuleDTO, len(rules))
	for i, rule := range rules {
		dtos[i] = h.ruleToDTO(rule)
	}

	h.writeJSON(w, http.StatusOK, RuleListResponse{
		Success: true,
		Data:    dtos,
		Pagination: PaginationInfo{
			Total:   total,
			Limit:   filter.Limit,
			Offset:  filter.Offset,
			HasMore: int64(filter.Offset+filter.Limit) < total,
		},
	})
}

// SearchRules 搜索规则
func (h *Handler) SearchRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "search query is required"))
		return
	}

	// 长度限制
	if len(query) > 256 {
		h.writeError(w, r, errors.New(errors.ErrCodeInvalidParameter, "query too long, max 256 characters"))
		return
	}

	rules, err := h.ruleService.SearchRules(ctx, opCtx.TenantID, query, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	dtos := make([]*RuleDTO, len(rules))
	for i, rule := range rules {
		dtos[i] = h.ruleToDTO(rule)
	}

	h.writeJSON(w, http.StatusOK, RuleListResponse{
		Success: true,
		Data:    dtos,
		Pagination: PaginationInfo{
			Total: int64(len(rules)),
		},
	})
}

// EnableRule 启用规则
func (h *Handler) EnableRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	ruleID := mux.Vars(r)["id"]
	if ruleID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "rule id is required"))
		return
	}

	if err := h.ruleService.EnableRule(ctx, ruleID, true, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "rule enabled successfully",
	})
}

// DisableRule 禁用规则
func (h *Handler) DisableRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	ruleID := mux.Vars(r)["id"]
	if ruleID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "rule id is required"))
		return
	}

	if err := h.ruleService.EnableRule(ctx, ruleID, false, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "rule disabled successfully",
	})
}

// GetRuleVersions 获取规则版本历史
func (h *Handler) GetRuleVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	ruleID := mux.Vars(r)["id"]
	if ruleID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "rule id is required"))
		return
	}

	versions, err := h.ruleService.GetRuleVersions(ctx, ruleID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    versions,
	})
}

// BatchEnableRules 批量启用规则
func (h *Handler) BatchEnableRules(w http.ResponseWriter, r *http.Request) {
	h.handleBatchEnable(w, r, true)
}

// BatchDisableRules 批量禁用规则
func (h *Handler) BatchDisableRules(w http.ResponseWriter, r *http.Request) {
	h.handleBatchEnable(w, r, false)
}

// handleBatchEnable 处理批量启用/禁用
func (h *Handler) handleBatchEnable(w http.ResponseWriter, r *http.Request, enabled bool) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	var req BatchOperationRequest
	if err := h.decodeJSON(r, &req); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}

	if len(req.RuleIDs) == 0 {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "rule_ids is required"))
		return
	}

	if len(req.RuleIDs) > 100 {
		h.writeError(w, r, errors.New(errors.ErrCodeInvalidParameter, "too many rule_ids, max 100"))
		return
	}

	result, err := h.ruleService.BatchEnableRules(ctx, req.RuleIDs, enabled, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, BatchOperationResponse{
		Success: true,
		Data:    result,
	})
}

// BatchDeleteRules 批量删除规则
func (h *Handler) BatchDeleteRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	var req BatchOperationRequest
	if err := h.decodeJSON(r, &req); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}

	if len(req.RuleIDs) == 0 {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "rule_ids is required"))
		return
	}

	if len(req.RuleIDs) > 100 {
		h.writeError(w, r, errors.New(errors.ErrCodeInvalidParameter, "too many rule_ids, max 100"))
		return
	}

	result, err := h.ruleService.BatchDeleteRules(ctx, req.RuleIDs, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, BatchOperationResponse{
		Success: true,
		Data:    result,
	})
}

// ExportRules 导出规则
func (h *Handler) ExportRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	// 解析规则 ID 列表（可选）
	var ruleIDs []string
	if ids := r.URL.Query().Get("ids"); ids != "" {
		ruleIDs = strings.Split(ids, ",")
		if len(ruleIDs) > 1000 {
			h.writeError(w, r, errors.New(errors.ErrCodeInvalidParameter, "too many rule ids, max 1000"))
			return
		}
	}

	export, err := h.ruleService.ExportRules(ctx, opCtx.TenantID, ruleIDs, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	// 设置下载头
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=rules_export.json")

	json.NewEncoder(w).Encode(export)
}

// ImportRules 导入规则
func (h *Handler) ImportRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	// 检查请求大小
	if r.ContentLength > h.config.MaxRequestSize {
		h.writeError(w, r, errors.Newf(errors.ErrCodeInvalidRequest, "request too large: max %d bytes", h.config.MaxRequestSize))
		return
	}

	var export service.RuleExport
	if err := h.decodeJSON(r, &export); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid import data"))
		return
	}

	// 限制导入数量
	if len(export.Rules) > 1000 {
		h.writeError(w, r, errors.New(errors.ErrCodeInvalidParameter, "too many rules, max 1000 per import"))
		return
	}

	result, err := h.ruleService.ImportRules(ctx, opCtx.TenantID, &export, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    result,
	})
}

// SyncRules 同步规则到 Kafka
func (h *Handler) SyncRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	count, err := h.ruleService.SyncRulesToKafka(ctx, opCtx.TenantID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"synced_count": count,
		"message":      "rules synced to kafka successfully",
	})
}

// GetRuleStats 获取规则统计
func (h *Handler) GetRuleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	stats, err := h.ruleService.GetRuleStats(ctx, opCtx.TenantID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    stats,
	})
}

// =============================================================================
// 部署 API
// =============================================================================

// CreateDeployment 创建部署
func (h *Handler) CreateDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	var req CreateDeploymentRequest
	if err := h.decodeJSON(r, &req); err != nil {
		h.writeError(w, r, errors.Wrap(err, errors.ErrCodeInvalidRequest, "invalid request body"))
		return
	}

	// 验证请求
	if err := req.Validate(); err != nil {
		h.writeError(w, r, err)
		return
	}

	deployment := &model.Deployment{
		TenantID:     opCtx.TenantID,
		Name:         req.Name,
		Description:  req.Description,
		RuleVersion:  req.RuleVersion,
		ModelVersion: req.ModelVersion,
		FeatureSetID: req.FeatureSetID,
		Scope:        req.Scope,
	}

	if err := h.deploymentService.CreateDeployment(ctx, deployment, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusCreated, DeploymentResponse{
		Success: true,
		Data:    h.deploymentToDTO(deployment),
	})
}

// GetDeployment 获取部署
func (h *Handler) GetDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误（用于后续权限检查）
	_, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	deploymentID := mux.Vars(r)["id"]
	if deploymentID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "deployment id is required"))
		return
	}

	deployment, err := h.deploymentService.GetDeployment(ctx, deploymentID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, DeploymentResponse{
		Success: true,
		Data:    h.deploymentToDTO(deployment),
	})
}

// ListDeployments 列出部署
func (h *Handler) ListDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	filter := &service.DeploymentFilter{
		Status:      r.URL.Query().Get("status"),
		RuleVersion: r.URL.Query().Get("rule_version"),
		Limit:       h.parseIntParam(r, "limit", h.config.DefaultPageSize),
		Offset:      h.parseIntParam(r, "offset", 0),
	}

	// 限制最大分页
	if filter.Limit > h.config.MaxPageSize {
		filter.Limit = h.config.MaxPageSize
	}

	deployments, total, err := h.deploymentService.ListDeployments(ctx, opCtx.TenantID, filter, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	dtos := make([]*DeploymentDTO, len(deployments))
	for i, d := range deployments {
		dtos[i] = h.deploymentToDTO(d)
	}

	h.writeJSON(w, http.StatusOK, DeploymentListResponse{
		Success: true,
		Data:    dtos,
		Pagination: PaginationInfo{
			Total:   total,
			Limit:   filter.Limit,
			Offset:  filter.Offset,
			HasMore: int64(filter.Offset+filter.Limit) < total,
		},
	})
}

// StartGrayDeployment 开始灰度部署
func (h *Handler) StartGrayDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	deploymentID := mux.Vars(r)["id"]
	if deploymentID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "deployment id is required"))
		return
	}

	if err := h.deploymentService.StartGrayDeployment(ctx, deploymentID, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "gray deployment started",
	})
}

// ActivateDeployment 激活部署
func (h *Handler) ActivateDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	deploymentID := mux.Vars(r)["id"]
	if deploymentID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "deployment id is required"))
		return
	}

	if err := h.deploymentService.ActivateDeployment(ctx, deploymentID, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "deployment activated",
	})
}

// RollbackDeployment 回滚部署
func (h *Handler) RollbackDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	deploymentID := mux.Vars(r)["id"]
	if deploymentID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "deployment id is required"))
		return
	}

	if err := h.deploymentService.RollbackDeployment(ctx, deploymentID, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "deployment rolled back",
	})
}

// PauseDeployment 暂停部署
func (h *Handler) PauseDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	deploymentID := mux.Vars(r)["id"]
	if deploymentID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "deployment id is required"))
		return
	}

	if err := h.deploymentService.PauseDeployment(ctx, deploymentID, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "deployment paused",
	})
}

// ResumeDeployment 恢复部署
func (h *Handler) ResumeDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	deploymentID := mux.Vars(r)["id"]
	if deploymentID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "deployment id is required"))
		return
	}

	if err := h.deploymentService.ResumeDeployment(ctx, deploymentID, opCtx); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, SuccessResponse{
		Success: true,
		Message: "deployment resumed",
	})
}

// GetDeploymentHistory 获取部署历史
func (h *Handler) GetDeploymentHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	deploymentID := mux.Vars(r)["id"]
	if deploymentID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "deployment id is required"))
		return
	}

	history, err := h.deploymentService.GetDeploymentHistory(ctx, deploymentID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    history,
	})
}

// GetGrayProgress 获取灰度进度
func (h *Handler) GetGrayProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ✅ 修复：检查 extractOperationContext 的错误
	opCtx, err := h.extractOperationContext(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	deploymentID := mux.Vars(r)["id"]
	if deploymentID == "" {
		h.writeError(w, r, errors.New(errors.ErrCodeMissingParameter, "deployment id is required"))
		return
	}

	progress, err := h.deploymentService.GetGrayProgress(ctx, deploymentID, opCtx)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"data":    progress,
	})
}

// =============================================================================
// 请求/响应 DTO
// =============================================================================

// CreateRuleRequest 创建规则请求
type CreateRuleRequest struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Engine      string                 `json:"engine,omitempty"`
	Description string                 `json:"description,omitempty"`
	Conditions  map[string]interface{} `json:"conditions"`
	Labels      []string               `json:"labels,omitempty"`
	Severity    string                 `json:"severity,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority,omitempty"`
}

// Validate 验证创建规则请求
func (r *CreateRuleRequest) Validate() error {
	if r.Name == "" {
		return errors.New(errors.ErrCodeMissingParameter, "name is required")
	}
	if len(r.Name) > 256 {
		return errors.New(errors.ErrCodeInvalidParameter, "name too long, max 256 characters")
	}
	if r.Type == "" {
		return errors.New(errors.ErrCodeMissingParameter, "type is required")
	}
	if !model.IsValidRuleType(r.Type) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid rule type: %s", r.Type)
	}
	if r.Engine != "" && !model.IsValidRuleEngine(r.Engine) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid engine: %s", r.Engine)
	}
	if r.Severity != "" && !model.IsValidSeverity(r.Severity) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid severity: %s", r.Severity)
	}
	if len(r.Labels) > 50 {
		return errors.New(errors.ErrCodeInvalidParameter, "too many labels, max 50")
	}
	if len(r.Description) > 4096 {
		return errors.New(errors.ErrCodeInvalidParameter, "description too long, max 4096 characters")
	}
	if r.Priority < 0 || r.Priority > 100 {
		return errors.New(errors.ErrCodeInvalidParameter, "priority must be between 0 and 100")
	}
	return nil
}

// UpdateRuleRequest 更新规则请求
type UpdateRuleRequest struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Engine      string                 `json:"engine,omitempty"`
	Description string                 `json:"description,omitempty"`
	Conditions  map[string]interface{} `json:"conditions"`
	Labels      []string               `json:"labels,omitempty"`
	Severity    string                 `json:"severity,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority,omitempty"`
}

// Validate 验证更新规则请求
func (r *UpdateRuleRequest) Validate() error {
	if r.Name == "" {
		return errors.New(errors.ErrCodeMissingParameter, "name is required")
	}
	if len(r.Name) > 256 {
		return errors.New(errors.ErrCodeInvalidParameter, "name too long, max 256 characters")
	}
	if r.Type == "" {
		return errors.New(errors.ErrCodeMissingParameter, "type is required")
	}
	if !model.IsValidRuleType(r.Type) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid rule type: %s", r.Type)
	}
	if r.Engine != "" && !model.IsValidRuleEngine(r.Engine) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid engine: %s", r.Engine)
	}
	if r.Severity != "" && !model.IsValidSeverity(r.Severity) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid severity: %s", r.Severity)
	}
	if len(r.Labels) > 50 {
		return errors.New(errors.ErrCodeInvalidParameter, "too many labels, max 50")
	}
	if len(r.Description) > 4096 {
		return errors.New(errors.ErrCodeInvalidParameter, "description too long, max 4096 characters")
	}
	if r.Priority < 0 || r.Priority > 100 {
		return errors.New(errors.ErrCodeInvalidParameter, "priority must be between 0 and 100")
	}
	return nil
}

// BatchOperationRequest 批量操作请求
type BatchOperationRequest struct {
	RuleIDs []string `json:"rule_ids"`
}

// CreateDeploymentRequest 创建部署请求
type CreateDeploymentRequest struct {
	Name         string                 `json:"name,omitempty"`
	Description  string                 `json:"description,omitempty"`
	RuleVersion  string                 `json:"rule_version,omitempty"`
	ModelVersion string                 `json:"model_version,omitempty"`
	FeatureSetID string                 `json:"feature_set_id,omitempty"`
	Scope        map[string]interface{} `json:"scope,omitempty"`
}

// Validate 验证创建部署请求
func (r *CreateDeploymentRequest) Validate() error {
	if r.RuleVersion == "" && r.ModelVersion == "" {
		return errors.New(errors.ErrCodeMissingParameter, "rule_version or model_version is required")
	}
	if len(r.Name) > 256 {
		return errors.New(errors.ErrCodeInvalidParameter, "name too long, max 256 characters")
	}
	if len(r.Description) > 1024 {
		return errors.New(errors.ErrCodeInvalidParameter, "description too long, max 1024 characters")
	}
	return nil
}

// RuleDTO 规则 DTO
type RuleDTO struct {
	RuleID      string                 `json:"rule_id"`
	TenantID    string                 `json:"tenant_id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Engine      string                 `json:"engine"`
	Description string                 `json:"description,omitempty"`
	Conditions  map[string]interface{} `json:"conditions,omitempty"`
	Labels      []string               `json:"labels,omitempty"`
	Severity    string                 `json:"severity"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
	Version     int64                  `json:"version"`
	Status      string                 `json:"status"`
	CreatedBy   string                 `json:"created_by"`
	UpdatedBy   string                 `json:"updated_by,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

// DeploymentDTO 部署 DTO
type DeploymentDTO struct {
	DeploymentID string                 `json:"deployment_id"`
	TenantID     string                 `json:"tenant_id"`
	Name         string                 `json:"name,omitempty"`
	Description  string                 `json:"description,omitempty"`
	RuleVersion  string                 `json:"rule_version,omitempty"`
	ModelVersion string                 `json:"model_version,omitempty"`
	FeatureSetID string                 `json:"feature_set_id,omitempty"`
	Scope        map[string]interface{} `json:"scope,omitempty"`
	Status       string                 `json:"status"`
	CreatedBy    string                 `json:"created_by"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
}

// RuleResponse 规则响应
type RuleResponse struct {
	Success bool     `json:"success"`
	Data    *RuleDTO `json:"data,omitempty"`
}

// RuleListResponse 规则列表响应
type RuleListResponse struct {
	Success    bool           `json:"success"`
	Data       []*RuleDTO     `json:"data"`
	Pagination PaginationInfo `json:"pagination"`
}

// DeploymentResponse 部署响应
type DeploymentResponse struct {
	Success bool           `json:"success"`
	Data    *DeploymentDTO `json:"data,omitempty"`
}

// DeploymentListResponse 部署列表响应
type DeploymentListResponse struct {
	Success    bool             `json:"success"`
	Data       []*DeploymentDTO `json:"data"`
	Pagination PaginationInfo   `json:"pagination"`
}

// BatchOperationResponse 批量操作响应
type BatchOperationResponse struct {
	Success bool                 `json:"success"`
	Data    *service.BatchResult `json:"data"`
}

// SuccessResponse 成功响应
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// PaginationInfo 分页信息
type PaginationInfo struct {
	Total   int64 `json:"total"`
	Limit   int   `json:"limit"`
	Offset  int   `json:"offset"`
	HasMore bool  `json:"has_more"`
}

// =============================================================================
// 辅助方法
// =============================================================================

// ruleToDTO 转换规则为 DTO
func (h *Handler) ruleToDTO(rule *model.Rule) *RuleDTO {
	if rule == nil {
		return nil
	}
	return &RuleDTO{
		RuleID:      rule.RuleID,
		TenantID:    rule.TenantID,
		Name:        rule.Name,
		Type:        rule.Type,
		Engine:      rule.Engine,
		Description: rule.Description,
		Conditions:  rule.Conditions,
		Labels:      rule.Labels,
		Severity:    rule.Severity,
		Enabled:     rule.Enabled,
		Priority:    rule.Priority,
		Version:     rule.Version,
		Status:      rule.Status,
		CreatedBy:   rule.CreatedBy,
		UpdatedBy:   rule.UpdatedBy,
		CreatedAt:   rule.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   rule.UpdatedAt.Format(time.RFC3339),
	}
}

// deploymentToDTO 转换部署为 DTO
func (h *Handler) deploymentToDTO(d *model.Deployment) *DeploymentDTO {
	if d == nil {
		return nil
	}
	return &DeploymentDTO{
		DeploymentID: d.DeploymentID,
		TenantID:     d.TenantID,
		Name:         d.Name,
		Description:  d.Description,
		RuleVersion:  d.RuleVersion,
		ModelVersion: d.ModelVersion,
		FeatureSetID: d.FeatureSetID,
		Scope:        d.Scope,
		Status:       d.Status,
		CreatedBy:    d.CreatedBy,
		CreatedAt:    d.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    d.UpdatedAt.Format(time.RFC3339),
	}
}

// parseRuleFilter 解析规则过滤参数
func (h *Handler) parseRuleFilter(r *http.Request) *service.RuleFilter {
	filter := &service.RuleFilter{
		Type:     r.URL.Query().Get("type"),
		Engine:   r.URL.Query().Get("engine"),
		Severity: r.URL.Query().Get("severity"),
		Keyword:  r.URL.Query().Get("keyword"),
		Limit:    h.parseIntParam(r, "limit", h.config.DefaultPageSize),
		Offset:   h.parseIntParam(r, "offset", 0),
		OrderBy:  r.URL.Query().Get("order_by"),
		OrderDir: r.URL.Query().Get("order_dir"),
	}

	// 处理 enabled 参数
	if enabled := r.URL.Query().Get("enabled"); enabled != "" {
		b := enabled == "true" || enabled == "1"
		filter.Enabled = &b
	}

	// 处理 labels 参数
	if labels := r.URL.Query().Get("labels"); labels != "" {
		filter.Labels = strings.Split(labels, ",")
	}

	// 限制最大分页大小
	if filter.Limit > h.config.MaxPageSize {
		filter.Limit = h.config.MaxPageSize
	}
	if filter.Limit <= 0 {
		filter.Limit = h.config.DefaultPageSize
	}

	return filter
}

// parseIntParam 解析整数参数
func (h *Handler) parseIntParam(r *http.Request, name string, defaultValue int) int {
	if value := r.URL.Query().Get(name); value != "" {
		if i, err := strconv.Atoi(value); err == nil && i >= 0 {
			return i
		}
	}
	return defaultValue
}

// decodeJSON 解码 JSON 请求体
func (h *Handler) decodeJSON(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, h.config.MaxRequestSize))
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if len(body) == 0 {
		return errors.New(errors.ErrCodeInvalidRequest, "empty request body")
	}

	return json.Unmarshal(body, v)
}

// writeJSON 写入 JSON 响应（统一格式）
func (h *Handler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", zap.Error(err))
	}
}

// writeError 写入错误响应（统一使用 errors.WriteError）
func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	traceID := httpx.GetTraceID(r.Context())
	errors.WriteError(w, err, traceID, r.URL.Path)
}
