// //////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/api/handler.go
// 修复版：完善证据 API、添加导出 API、修复 CSV 转义、时间解析错误处理
// 修复：CSV 导出协议名称字段缺失问题
// //////////////////////////////////////////////////////////////////////////////
package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/state"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// Handler Alert API处理器
type Handler struct {
	alertService    *service.AlertService
	feedbackHandler *FeedbackHandler
	auditLogger     *audit.AlertAuditLogger
	logger          *zap.Logger
}

// NewHandler 创建Handler
func NewHandler(
	alertService *service.AlertService,
	auditLogger *audit.AlertAuditLogger,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		alertService: alertService,
		auditLogger:  auditLogger,
		logger:       logger,
	}
}

// NewHandlerWithFeedback 创建带 FeedbackHandler 的 Handler
func NewHandlerWithFeedback(
	alertService *service.AlertService,
	kafkaProducer *kafka.Producer,
	auditLogger *audit.AlertAuditLogger,
	logger *zap.Logger,
) *Handler {
	h := &Handler{
		alertService: alertService,
		auditLogger:  auditLogger,
		logger:       logger,
	}
	// 初始化 FeedbackHandler
	h.feedbackHandler = NewFeedbackHandler(alertService, kafkaProducer, auditLogger, nil, logger)
	return h
}

// SetFeedbackRepo 设置反馈持久化仓库（由 main.go 在 CH 初始化后调用）
func (h *Handler) SetFeedbackRepo(repo *FeedbackRepository) {
	if h.feedbackHandler != nil {
		h.feedbackHandler.repo = repo
	}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *mux.Router) {
	// 告警列表与详情
	r.HandleFunc("/alerts", h.ListAlerts).Methods("GET")
	r.HandleFunc("/alerts/search", h.SearchAlerts).Methods("POST")
	r.HandleFunc("/alerts/{id}", h.GetAlert).Methods("GET")
	r.HandleFunc("/alerts/{id}/evidence", h.GetAlertEvidence).Methods("GET")
	// 告警操作
	r.HandleFunc("/alerts/{id}/status", h.UpdateStatus).Methods("PUT")
	r.HandleFunc("/alerts/{id}/assign", h.AssignAlert).Methods("PUT")
	r.HandleFunc("/alerts/batch/status", h.BatchUpdateStatus).Methods("PUT")
	r.HandleFunc("/alerts/{id}/close", h.CloseAlert).Methods("POST")
	r.HandleFunc("/alerts/{id}/reopen", h.ReopenAlert).Methods("POST")
	// 统计与趋势
	r.HandleFunc("/alerts/stats", h.GetStats).Methods("GET")
	r.HandleFunc("/alerts/trend", h.GetTrend).Methods("GET")
	// 导出功能
	r.HandleFunc("/alerts/export", h.ExportAlerts).Methods("POST")
	r.HandleFunc("/alerts/export/csv", h.ExportAlertsCSV).Methods("POST")
	// 证据相关 API
	r.HandleFunc("/evidence/{id}", h.GetEvidenceByID).Methods("GET")
	r.HandleFunc("/evidence/alert/{alert_id}", h.GetEvidenceByAlertID).Methods("GET")
	// 存储健康状态
	r.HandleFunc("/alerts/storage/status", h.GetStorageStatus).Methods("GET")
	// 注册 Feedback 路由（如果可用）
	if h.feedbackHandler != nil {
		h.feedbackHandler.RegisterRoutes(r)
	} else {
		// 提供基本的反馈接口（无 Kafka）
		r.HandleFunc("/alerts/{id}/feedback", h.SubmitFeedbackBasic).Methods("POST")
		r.HandleFunc("/alerts/{id}/feedback", h.GetFeedbackBasic).Methods("GET")
		r.HandleFunc("/feedback/reason-codes", h.GetReasonCodesBasic).Methods("GET")
	}
}

// HealthCheck 健康检查
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	httpx.JSONSuccess(w, r.Context(), map[string]string{
		"status":  "healthy",
		"service": "alert-service",
	})
}

// ReadinessCheck 就绪检查
func (h *Handler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	status := h.alertService.GetStorageStatus()
	// 至少一个存储可用即为就绪
	ready := false
	for _, s := range status {
		if available, ok := s["available"].(bool); ok && available {
			ready = true
			break
		}
	}
	if ready {
		httpx.JSONSuccess(w, r.Context(), map[string]interface{}{
			"status":  "ready",
			"storage": status,
		})
	} else {
		httpx.JSONError(w, r.Context(), http.StatusServiceUnavailable, "NOT_READY", "No storage backend available")
	}
}

// ListAlerts 查询告警列表
func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	// 提取租户ID
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 解析查询参数
	query := &service.ListQuery{
		TenantID:  tenantID,
		Severity:  r.URL.Query().Get("severity"),
		Status:    r.URL.Query().Get("status"),
		AlertType: r.URL.Query().Get("alert_type"),
		SrcIP:     r.URL.Query().Get("src_ip"),
		DstIP:     r.URL.Query().Get("dst_ip"),
		SortBy:    r.URL.Query().Get("sort_by"),
		SortOrder: r.URL.Query().Get("sort_order"),
	}
	// 解析标签（逗号分隔）
	if labelsStr := r.URL.Query().Get("labels"); labelsStr != "" {
		query.Labels = splitAndTrim(labelsStr, ",")
	}
	// 解析时间范围 - 修复：返回解析错误
	if startTimeStr := r.URL.Query().Get("start_time"); startTimeStr != "" {
		ts, err := strconv.ParseInt(startTimeStr, 10, 64)
		if err != nil {
			errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid start_time: %s", startTimeStr),
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		query.StartTime = time.UnixMilli(ts)
	}
	if endTimeStr := r.URL.Query().Get("end_time"); endTimeStr != "" {
		ts, err := strconv.ParseInt(endTimeStr, 10, 64)
		if err != nil {
			errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid end_time: %s", endTimeStr),
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		query.EndTime = time.UnixMilli(ts)
	}
	// 解析分页
	query.Limit = parseIntWithDefault(r.URL.Query().Get("limit"), 50, 1, 1000)
	query.Offset = parseIntWithDefault(r.URL.Query().Get("offset"), 0, 0, 100000)
	logger.Debug("List alerts request",
		zap.String("tenant_id", tenantID),
		zap.String("severity", query.Severity),
		zap.String("status", query.Status),
		zap.Int("limit", query.Limit),
		zap.Int("offset", query.Offset))
	// 执行查询
	result, err := h.alertService.ListAlerts(ctx, query)
	if err != nil {
		logger.Error("Failed to list alerts", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 构建响应
	httpx.JSONPaginated(w, ctx, result.Alerts, result.Total, query.Limit, query.Offset)
}

// SearchAlertsRequest 搜索请求
type SearchAlertsRequest struct {
	Query      string   `json:"query"`
	Severity   []string `json:"severity,omitempty"`
	Status     []string `json:"status,omitempty"`
	AlertTypes []string `json:"alert_types,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	SrcIP      string   `json:"src_ip,omitempty"`
	DstIP      string   `json:"dst_ip,omitempty"`
	StartTime  int64    `json:"start_time,omitempty"`
	EndTime    int64    `json:"end_time,omitempty"`
	From       int      `json:"from"`
	Size       int      `json:"size"`
	SortField  string   `json:"sort_field,omitempty"`
	SortOrder  string   `json:"sort_order,omitempty"`
}

// SearchAlerts 全文搜索告警
func (h *Handler) SearchAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	var req SearchAlertsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidRequest, "invalid request body"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 构建搜索查询
	query := &service.SearchQuery{
		TenantID:   tenantID,
		Query:      req.Query,
		Severity:   req.Severity,
		Status:     req.Status,
		AlertTypes: req.AlertTypes,
		Labels:     req.Labels,
		SrcIP:      req.SrcIP,
		DstIP:      req.DstIP,
		From:       req.From,
		Size:       req.Size,
		SortField:  req.SortField,
		SortOrder:  req.SortOrder,
	}
	if req.StartTime > 0 {
		query.StartTime = time.UnixMilli(req.StartTime)
	}
	if req.EndTime > 0 {
		query.EndTime = time.UnixMilli(req.EndTime)
	}
	logger.Debug("Search alerts request",
		zap.String("tenant_id", tenantID),
		zap.String("query", req.Query))
	// 执行搜索
	result, err := h.alertService.SearchAlerts(ctx, query)
	if err != nil {
		logger.Error("Failed to search alerts", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, result)
}

// GetAlert 获取告警详情
func (h *Handler) GetAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	alertID := vars["id"]
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Debug("Get alert request",
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID))
	// 查询告警
	alert, err := h.alertService.GetAlert(ctx, tenantID, alertID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeAlertNotFound, "Alert not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		logger.Error("Failed to get alert", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, alert)
}

// GetAlertEvidence 获取告警证据
func (h *Handler) GetAlertEvidence(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	alertID := vars["id"]
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Debug("Get alert evidence request",
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID))
	// 先验证告警存在
	_, err := h.alertService.GetAlert(ctx, tenantID, alertID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeAlertNotFound, "Alert not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		logger.Error("Failed to get alert", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 查询证据
	evidences, err := h.alertService.GetEvidence(ctx, tenantID, alertID)
	if err != nil {
		logger.Error("Failed to get evidence", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"alert_id":  alertID,
		"evidences": evidences,
		"count":     len(evidences),
	})
}

// GetEvidenceByID 获取单个证据详情
func (h *Handler) GetEvidenceByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	evidenceID := vars["id"]
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 从查询参数获取 alert_id（必需）
	alertID := r.URL.Query().Get("alert_id")
	if alertID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeMissingParameter, "alert_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Debug("Get evidence by ID request",
		zap.String("evidence_id", evidenceID),
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID))
	evidence, err := h.alertService.GetEvidenceByID(ctx, tenantID, alertID, evidenceID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeResourceNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeResourceNotFound, "Evidence not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		logger.Error("Failed to get evidence", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, evidence)
}

// GetEvidenceByAlertID 获取告警的所有证据
func (h *Handler) GetEvidenceByAlertID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	alertID := vars["alert_id"]
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Debug("Get evidence by alert ID request",
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID))
	// 先验证告警存在
	_, err := h.alertService.GetAlert(ctx, tenantID, alertID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeAlertNotFound, "Alert not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		logger.Error("Failed to get alert", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	evidences, err := h.alertService.GetEvidence(ctx, tenantID, alertID)
	if err != nil {
		logger.Error("Failed to get evidences", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"alert_id":  alertID,
		"evidences": evidences,
		"count":     len(evidences),
	})
}

// UpdateStatusRequest 更新状态请求
type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// UpdateStatus 更新告警状态
func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	alertID := vars["id"]
	tenantID := h.extractTenantID(r)
	userID := h.extractUserID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	var req UpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidRequest, "invalid request body"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 验证状态值
	newStatus, err := state.ParseStatus(req.Status)
	if err != nil {
		errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid status: %s", req.Status),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Info("Update alert status request",
		zap.String("alert_id", alertID),
		zap.String("new_status", newStatus.String()),
		zap.String("user_id", userID))
	// 更新状态
	oldStatus, err := h.alertService.UpdateStatus(ctx, tenantID, alertID, newStatus.String(), userID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeAlertNotFound, "Alert not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		if errors.IsCode(err, errors.ErrCodeInvalidStateTransition) {
			errors.WriteErrorWithStatus(w, http.StatusBadRequest,
				errors.ErrCodeInvalidStateTransition, err.Error(),
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		logger.Error("Failed to update alert status", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 记录审计日志
	if h.auditLogger != nil {
		h.auditLogger.LogAlertStatusChange(ctx, alertID, tenantID, oldStatus, newStatus.String())
	}
	httpx.JSONSuccess(w, ctx, map[string]string{
		"alert_id":   alertID,
		"old_status": oldStatus,
		"new_status": newStatus.String(),
	})
}

// BatchUpdateStatusRequest 批量更新状态请求
type BatchUpdateStatusRequest struct {
	AlertIDs []string `json:"alert_ids"`
	Status   string   `json:"status"`
}

// BatchUpdateStatus 批量更新告警状态
func (h *Handler) BatchUpdateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	tenantID := h.extractTenantID(r)
	userID := h.extractUserID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	var req BatchUpdateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidRequest, "invalid request body"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	if len(req.AlertIDs) == 0 {
		errors.WriteError(w, errors.New(errors.ErrCodeMissingParameter, "alert_ids is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 限制批量操作数量
	if len(req.AlertIDs) > 100 {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidParameter, "alert_ids cannot exceed 100"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 验证状态
	_, err := state.ParseStatus(req.Status)
	if err != nil {
		errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid status: %s", req.Status),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Info("Batch update alert status request",
		zap.Int("count", len(req.AlertIDs)),
		zap.String("new_status", req.Status),
		zap.String("user_id", userID))
	result, err := h.alertService.BatchUpdateStatus(ctx, tenantID, req.AlertIDs, req.Status, userID)
	if err != nil {
		logger.Error("Failed to batch update alert status", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, result)
}

// AssignRequest 分配请求
type AssignRequest struct {
	Assignee string `json:"assignee"`
}

// AssignAlert 分配告警
func (h *Handler) AssignAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	alertID := vars["id"]
	tenantID := h.extractTenantID(r)
	userID := h.extractUserID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	var req AssignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidRequest, "invalid request body"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	if req.Assignee == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeMissingParameter, "assignee is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Info("Assign alert request",
		zap.String("alert_id", alertID),
		zap.String("assignee", req.Assignee),
		zap.String("user_id", userID))
	// 更新分配人
	if err := h.alertService.AssignAlert(ctx, tenantID, alertID, req.Assignee, userID); err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeAlertNotFound, "Alert not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		logger.Error("Failed to assign alert", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 记录审计日志
	if h.auditLogger != nil {
		h.auditLogger.LogAlertAssign(ctx, alertID, tenantID, req.Assignee)
	}
	httpx.JSONSuccess(w, ctx, map[string]string{
		"alert_id": alertID,
		"assignee": req.Assignee,
		"status":   state.StatusAssigned.String(),
	})
}

// CloseAlertRequest 关闭告警请求
type CloseAlertRequest struct {
	Reason string `json:"reason,omitempty"`
}

// CloseAlert 关闭告警
func (h *Handler) CloseAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	alertID := vars["id"]
	tenantID := h.extractTenantID(r)
	userID := h.extractUserID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	var req CloseAlertRequest
	// 允许空body
	json.NewDecoder(r.Body).Decode(&req)
	logger.Info("Close alert request",
		zap.String("alert_id", alertID),
		zap.String("reason", req.Reason),
		zap.String("user_id", userID))
	if err := h.alertService.CloseAlert(ctx, tenantID, alertID, req.Reason, userID); err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeAlertNotFound, "Alert not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		if errors.IsCode(err, errors.ErrCodeInvalidStateTransition) {
			errors.WriteErrorWithStatus(w, http.StatusBadRequest,
				errors.ErrCodeInvalidStateTransition, err.Error(),
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		logger.Error("Failed to close alert", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]string{
		"alert_id": alertID,
		"status":   state.StatusClosed.String(),
		"reason":   req.Reason,
	})
}

// ReopenAlert 重新打开告警
func (h *Handler) ReopenAlert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	alertID := vars["id"]
	tenantID := h.extractTenantID(r)
	userID := h.extractUserID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Info("Reopen alert request",
		zap.String("alert_id", alertID),
		zap.String("user_id", userID))
	if err := h.alertService.ReopenAlert(ctx, tenantID, alertID, userID); err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeAlertNotFound, "Alert not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		if errors.IsCode(err, errors.ErrCodeInvalidStateTransition) {
			errors.WriteErrorWithStatus(w, http.StatusBadRequest,
				errors.ErrCodeInvalidStateTransition, err.Error(),
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		logger.Error("Failed to reopen alert", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]string{
		"alert_id": alertID,
		"status":   state.StatusNew.String(),
	})
}

// GetStats 获取告警统计
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 默认时间范围：最近24小时
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)
	if s := r.URL.Query().Get("start_time"); s != "" {
		ts, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid start_time: %s", s),
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		startTime = time.UnixMilli(ts)
	}
	if e := r.URL.Query().Get("end_time"); e != "" {
		ts, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid end_time: %s", e),
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		endTime = time.UnixMilli(ts)
	}
	logger.Debug("Get alert stats request",
		zap.String("tenant_id", tenantID),
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime))
	stats, err := h.alertService.GetStats(ctx, tenantID, startTime, endTime)
	if err != nil {
		logger.Error("Failed to get alert stats", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, stats)
}

// GetTrend 获取告警趋势
func (h *Handler) GetTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 默认时间范围：最近24小时
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "hour"
	}
	// 验证interval
	validIntervals := map[string]bool{"minute": true, "hour": true, "day": true}
	if !validIntervals[interval] {
		errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid interval: %s, must be minute/hour/day", interval),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	if s := r.URL.Query().Get("start_time"); s != "" {
		ts, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid start_time: %s", s),
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		startTime = time.UnixMilli(ts)
	}
	if e := r.URL.Query().Get("end_time"); e != "" {
		ts, err := strconv.ParseInt(e, 10, 64)
		if err != nil {
			errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid end_time: %s", e),
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		endTime = time.UnixMilli(ts)
	}
	logger.Debug("Get alert trend request",
		zap.String("tenant_id", tenantID),
		zap.String("interval", interval),
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime))
	trend, err := h.alertService.GetTrend(ctx, tenantID, startTime, endTime, interval)
	if err != nil {
		logger.Error("Failed to get alert trend", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"interval": interval,
		"trend":    trend,
	})
}

// ExportAlertsRequest 导出告警请求
type ExportAlertsRequest struct {
	Severity  []string `json:"severity,omitempty"`
	Status    []string `json:"status,omitempty"`
	AlertType string   `json:"alert_type,omitempty"`
	StartTime int64    `json:"start_time,omitempty"`
	EndTime   int64    `json:"end_time,omitempty"`
	MaxCount  int      `json:"max_count,omitempty"`
}

// ExportAlerts 导出告警（JSON格式）
func (h *Handler) ExportAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	tenantID := h.extractTenantID(r)
	userID := h.extractUserID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	var req ExportAlertsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidRequest, "invalid request body"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Info("Export alerts request",
		zap.String("tenant_id", tenantID),
		zap.String("user_id", userID),
		zap.Int("max_count", req.MaxCount))
	// 构建导出查询
	query := &service.ExportQuery{
		TenantID:  tenantID,
		Severity:  req.Severity,
		Status:    req.Status,
		AlertType: req.AlertType,
		Format:    "json",
	}
	if req.StartTime > 0 {
		query.StartTime = time.UnixMilli(req.StartTime)
	}
	if req.EndTime > 0 {
		query.EndTime = time.UnixMilli(req.EndTime)
	}
	if req.MaxCount > 0 {
		query.MaxCount = req.MaxCount
	}
	result, err := h.alertService.ExportAlerts(ctx, query, userID)
	if err != nil {
		logger.Error("Failed to export alerts", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 记录审计日志
	if h.auditLogger != nil {
		h.auditLogger.LogAlertStatusChange(ctx, "", tenantID, "", "export")
	}
	// 设置下载头
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=alerts_export.json")
	json.NewEncoder(w).Encode(result)
}

// ExportAlertsCSV 导出告警（CSV格式）- 修复：使用标准库 csv 包进行正确转义，修复协议名称字段
func (h *Handler) ExportAlertsCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	tenantID := h.extractTenantID(r)
	userID := h.extractUserID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	var req ExportAlertsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidRequest, "invalid request body"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Info("Export alerts CSV request",
		zap.String("tenant_id", tenantID),
		zap.String("user_id", userID),
		zap.Int("max_count", req.MaxCount))
	// 构建导出查询
	query := &service.ExportQuery{
		TenantID:  tenantID,
		Severity:  req.Severity,
		Status:    req.Status,
		AlertType: req.AlertType,
		Format:    "csv",
	}
	if req.StartTime > 0 {
		query.StartTime = time.UnixMilli(req.StartTime)
	}
	if req.EndTime > 0 {
		query.EndTime = time.UnixMilli(req.EndTime)
	}
	if req.MaxCount > 0 {
		query.MaxCount = req.MaxCount
	}
	result, err := h.alertService.ExportAlerts(ctx, query, userID)
	if err != nil {
		logger.Error("Failed to export alerts", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 记录审计日志
	if h.auditLogger != nil {
		h.auditLogger.LogAlertStatusChange(ctx, "", tenantID, "", "export_csv")
	}
	// 设置下载头
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=alerts_export.csv")
	// 使用标准库的 csv.Writer 确保正确转义
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()
	// 写入表头
	header := []string{
		"alert_id", "tenant_id", "severity", "status", "alert_type",
		"src_ip", "dst_ip", "src_port", "dst_port", "protocol", "protocol_name",
		"first_seen", "last_seen", "count", "score", "assignee",
		"labels", "community_id",
	}
	if err := csvWriter.Write(header); err != nil {
		logger.Error("Failed to write CSV header", zap.Error(err))
		return
	}
	// 写入数据行
	for _, alert := range result.Alerts {
		// 将标签数组转换为字符串
		labelsStr := ""
		if len(alert.Labels) > 0 {
			labelsStr = strings.Join(alert.Labels, ";")
		}
		// 修复：使用 ProtocolName 字段而不是直接使用 Protocol
		row := []string{
			alert.AlertID,
			alert.TenantID,
			alert.Severity,
			alert.Status,
			alert.AlertType,
			alert.SrcIP,
			alert.DstIP,
			strconv.Itoa(int(alert.SrcPort)),
			strconv.Itoa(int(alert.DstPort)),
			strconv.Itoa(int(alert.Protocol)),
			alert.ProtocolName, // 修复：使用 ProtocolName 字段
			alert.FirstSeen.Format(time.RFC3339),
			alert.LastSeen.Format(time.RFC3339),
			strconv.Itoa(int(alert.Count)),
			strconv.FormatFloat(float64(alert.Score), 'f', 4, 32),
			alert.Assignee,
			labelsStr,
			alert.CommunityID,
		}
		if err := csvWriter.Write(row); err != nil {
			logger.Error("Failed to write CSV row", zap.Error(err), zap.String("alert_id", alert.AlertID))
			// 继续写入其他行
		}
	}
}

// GetStorageStatus 获取存储健康状态
func (h *Handler) GetStorageStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := h.alertService.GetStorageStatus()
	httpx.JSONSuccess(w, ctx, status)
}

// SubmitFeedbackBasic 基本的反馈提交（无 Kafka）
func (h *Handler) SubmitFeedbackBasic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	alertID := vars["id"]
	tenantID := h.extractTenantID(r)
	userID := h.extractUserID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	var req FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidRequest, "invalid request body"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 验证告警存在
	_, err := h.alertService.GetAlert(ctx, tenantID, alertID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeAlertNotFound, "Alert not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		logger.Error("Failed to get alert", zap.Error(err))
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 验证
	if req.Label != "TP" && req.Label != "FP" {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidParameter, "label must be TP or FP"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	if req.Label == "FP" && req.ReasonCode == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeMissingParameter, "reason_code is required for FP"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	if req.Label == "FP" && !isValidReasonCode(req.ReasonCode) {
		errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid reason_code: %s", req.ReasonCode),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger.Info("Submit alert feedback (basic)",
		zap.String("alert_id", alertID),
		zap.String("label", req.Label),
		zap.String("user_id", userID))
	// 记录审计日志
	if h.auditLogger != nil {
		h.auditLogger.LogAlertFeedback(ctx, alertID, tenantID, req.Label, req.Comment)
	}
	httpx.JSONCreated(w, ctx, map[string]interface{}{
		"alert_id":  alertID,
		"label":     req.Label,
		"message":   "Feedback recorded (basic mode, no Kafka)",
		"tenant_id": tenantID,
		"user_id":   userID,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetFeedbackBasic 基本的反馈获取
func (h *Handler) GetFeedbackBasic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	alertID := vars["id"]
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 验证告警存在
	_, err := h.alertService.GetAlert(ctx, tenantID, alertID)
	if err != nil {
		if errors.IsCode(err, errors.ErrCodeAlertNotFound) {
			errors.WriteErrorWithStatus(w, http.StatusNotFound,
				errors.ErrCodeAlertNotFound, "Alert not found",
				httpx.GetTraceID(ctx), r.URL.Path)
			return
		}
		errors.WriteError(w, err, httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"alert_id":  alertID,
		"feedbacks": []interface{}{},
		"message":   "Feedback history not available in basic mode",
	})
}

// GetReasonCodesBasic 基本的原因码获取
func (h *Handler) GetReasonCodesBasic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	codes := make([]map[string]string, 0, len(FPReasonCodes))
	for code, desc := range FPReasonCodes {
		codes = append(codes, map[string]string{
			"code":        code,
			"description": desc,
		})
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"reason_codes": codes,
	})
}

// 辅助方法
func (h *Handler) extractTenantID(r *http.Request) string {
	tenantID := httpx.GetTenantID(r.Context())
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-ID")
	}
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant_id")
	}
	return tenantID
}
func (h *Handler) extractUserID(r *http.Request) string {
	userID := httpx.GetUserID(r.Context())
	if userID == "" {
		userID = r.Header.Get("X-User-ID")
	}
	return userID
}

// parseIntWithDefault 解析整数，带默认值和范围限制
func parseIntWithDefault(s string, defaultVal, min, max int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// splitAndTrim 分割字符串并去除空白
func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
