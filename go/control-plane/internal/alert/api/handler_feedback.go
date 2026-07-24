// //////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/api/handler_feedback.go
// 完整修复版：添加告警存在性检查、完善验证
// //////////////////////////////////////////////////////////////////////////////
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/service"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/alert/whitelist"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// FeedbackHandler 告警反馈处理器 — TP/FP 反馈业务闭环
type FeedbackHandler struct {
	alertService  *service.AlertService
	kafkaProducer *kafka.Producer
	auditLogger   interface{}
	repo          *FeedbackRepository   // 反馈持久化仓库（ClickHouse）
	whitelistRepo *whitelist.Repository // 白名单仓库（PostgreSQL）
	actionAudit   *AlertActionAuditWriter
	logger        *zap.Logger
}

// NewFeedbackHandler 创建反馈处理器
func NewFeedbackHandler(
	alertService *service.AlertService,
	kafkaProducer *kafka.Producer,
	auditLogger interface{},
	repo *FeedbackRepository,
	whitelistRepo *whitelist.Repository,
	logger *zap.Logger,
) *FeedbackHandler {
	return &FeedbackHandler{
		alertService:  alertService,
		kafkaProducer: kafkaProducer,
		auditLogger:   auditLogger,
		repo:          repo,
		whitelistRepo: whitelistRepo,
		logger:        logger,
	}
}

// RegisterRoutes 注册反馈路由（含统计分析）
func (h *FeedbackHandler) RegisterRoutes(r *mux.Router) {
	r.HandleFunc("/alerts/{id}/feedback", h.SubmitFeedback).Methods("POST")
	r.HandleFunc("/alerts/{id}/feedback", h.GetFeedback).Methods("GET")
	r.HandleFunc("/feedback/reason-codes", h.GetReasonCodes).Methods("GET")
	r.HandleFunc("/feedback/stats", h.GetFeedbackStats).Methods("GET")
	r.HandleFunc("/feedback/fp-ranking", h.GetFPRanking).Methods("GET")
}

// FeedbackRequest 反馈请求 - 与 proto AlertFeedback 字段对齐
type FeedbackRequest struct {
	Label          string `json:"label"`            // TP (True Positive) | FP (False Positive) - 对应 proto label
	ReasonCode     string `json:"reason_code"`      // 误报原因码 - 对应 proto reason_code
	Comment        string `json:"comment"`          // 备注 - 对应 proto comment
	AddToWhitelist bool   `json:"add_to_whitelist"` // 是否加入白名单 - 对应 proto add_to_whitelist
}

// FeedbackResponse 反馈响应
type FeedbackResponse struct {
	FeedbackID     string                          `json:"feedback_id"`
	AlertID        string                          `json:"alert_id"`              // 对应 proto alert_id
	TenantID       string                          `json:"tenant_id"`             // 对应 proto tenant_id
	Label          string                          `json:"label"`                 // 对应 proto label
	ReasonCode     string                          `json:"reason_code,omitempty"` // 对应 proto reason_code
	Comment        string                          `json:"comment,omitempty"`     // 对应 proto comment
	UserID         string                          `json:"user_id"`               // 对应 proto user_id
	Timestamp      time.Time                       `json:"timestamp"`             // 对应 proto timestamp (int64)
	AddToWhitelist bool                            `json:"add_to_whitelist"`      // 对应 proto add_to_whitelist
	WhitelistDraft *FeedbackWhitelistDraftResponse `json:"whitelist_draft,omitempty"`
}

// FeedbackWhitelistDraftResponse 描述由 FP 反馈生成的白名单审批草案。
type FeedbackWhitelistDraftResponse struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	Value         string     `json:"value"`
	Reason        string     `json:"reason"`
	Description   string     `json:"description"`
	Status        string     `json:"status"`
	SourceAlertID string     `json:"source_alert_id"`
	FeedbackID    string     `json:"feedback_id"`
	URL           string     `json:"url"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// AlertFeedbackExtended 扩展的告警反馈结构（用于Kafka，包含额外上下文）
type AlertFeedbackExtended struct {
	// Proto 定义的字段
	AlertID        string `json:"alert_id"`
	TenantID       string `json:"tenant_id"`
	Label          string `json:"label"`
	ReasonCode     string `json:"reason_code,omitempty"`
	Comment        string `json:"comment,omitempty"`
	UserID         string `json:"user_id"`
	Timestamp      int64  `json:"timestamp"` // 使用 int64 与 proto 一致
	AddToWhitelist bool   `json:"add_to_whitelist"`
	// 扩展字段（用��模型训练）
	FeedbackID   string   `json:"feedback_id,omitempty"`
	AlertType    string   `json:"alert_type,omitempty"`
	Severity     string   `json:"severity,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	ModelVersion string   `json:"model_version,omitempty"`
	RuleVersion  string   `json:"rule_version,omitempty"`
}

// SubmitFeedback 提交告警反馈
func (h *FeedbackHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasAlertWritePermission(ctx) {
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied, "Permission denied: alert:write required", httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
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
	// 验证label - 必须是 TP 或 FP
	if req.Label != "TP" && req.Label != "FP" {
		errors.WriteError(w, errors.New(errors.ErrCodeInvalidParameter, "label must be TP or FP"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// FP 必须有原因码
	if req.Label == "FP" && req.ReasonCode == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeMissingParameter, "reason_code is required for FP"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 验证原因码是否有效
	if req.Label == "FP" && !isValidReasonCode(req.ReasonCode) {
		errors.WriteError(w, errors.Newf(errors.ErrCodeInvalidParameter, "invalid reason_code: %s", req.ReasonCode),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	if req.AddToWhitelist && req.Label == "FP" && !hasAlertWritePermission(ctx) {
		errors.WriteErrorWithStatus(w, http.StatusForbidden,
			errors.ErrCodePermissionDenied,
			"Permission denied: alert:write required to create a whitelist draft from feedback",
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 权限已经通过后才查询告警，避免无写权限主体借复合操作探测存在性。
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
	logger.Info("Submit alert feedback",
		zap.String("alert_id", alertID),
		zap.String("label", req.Label),
		zap.String("reason_code", req.ReasonCode),
		zap.String("user_id", userID))
	// 生成反馈ID
	feedbackID := uuid.New().String()
	feedbackTimestamp := time.Now()
	// 构建 Proto 兼容的反馈对象（包含告警上下文）
	feedback := &AlertFeedbackExtended{
		// Proto 字段
		AlertID:        alertID,
		TenantID:       tenantID,
		Label:          req.Label,
		ReasonCode:     req.ReasonCode,
		Comment:        req.Comment,
		UserID:         userID,
		Timestamp:      feedbackTimestamp.UnixMilli(),
		AddToWhitelist: req.AddToWhitelist,
		// 扩展字段 - 从告警中提取
		FeedbackID: feedbackID,
		AlertType:  alert.AlertType,
		Severity:   alert.Severity,
		Labels:     alert.Labels,
	}
	// 如果需要加入白名单，额外处理
	var whitelistDraft *FeedbackWhitelistDraftResponse
	if req.AddToWhitelist && req.Label == "FP" {
		entry, err := h.createWhitelistDraft(ctx, r, tenantID, alertID, feedbackID, userID, req.ReasonCode, alert)
		if err != nil {
			logger.Error("Failed to create audited whitelist draft from feedback", zap.Error(err))
			httpx.JSONError(w, ctx, http.StatusInternalServerError, "WHITELIST_DRAFT_CREATE_FAILED", "feedback was not accepted because its whitelist draft and audit record could not be committed")
			return
		} else if entry != nil {
			whitelistDraft = feedbackWhitelistDraftResponse(entry)
		}
	}
	// 响应
	response := &FeedbackResponse{
		FeedbackID:     feedbackID,
		AlertID:        alertID,
		TenantID:       tenantID,
		Label:          req.Label,
		ReasonCode:     req.ReasonCode,
		Comment:        req.Comment,
		UserID:         userID,
		Timestamp:      feedbackTimestamp,
		AddToWhitelist: req.AddToWhitelist,
		WhitelistDraft: whitelistDraft,
	}
	// 可查询反馈记录是成功响应的硬门禁，禁止 ClickHouse 写入失败后仍返回 201。
	if h.repo == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "FEEDBACK_PERSISTENCE_UNAVAILABLE", "feedback persistence is unavailable")
		return
	}
	record := &FeedbackRecord{
		FeedbackID:     feedbackID,
		AlertID:        alertID,
		TenantID:       tenantID,
		UserID:         userID,
		Label:          req.Label,
		ReasonCode:     req.ReasonCode,
		Comment:        req.Comment,
		AddToWhitelist: req.AddToWhitelist,
		AlertType:      alert.AlertType,
		Severity:       alert.Severity,
		ModelVersion:   alert.ModelVersion,
		RuleVersion:    alert.RuleVersion,
		CreatedAt:      feedbackTimestamp,
	}
	if err := h.repo.Insert(ctx, record); err != nil {
		logger.Error("Failed to persist feedback to ClickHouse", zap.Error(err))
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "FEEDBACK_PERSISTENCE_FAILED", "feedback was not accepted because its queryable record could not be persisted")
		return
	}
	if h.kafkaProducer != nil {
		if err := h.publishFeedback(ctx, feedback); err != nil {
			logger.Error("Failed to publish feedback to Kafka", zap.Error(err))
			httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "FEEDBACK_EVENT_FAILED", "feedback record was persisted but the training event could not be published")
			return
		}
	}
	if h.actionAudit == nil {
		httpx.JSONError(w, ctx, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "feedback was persisted but audit persistence is unavailable")
		return
	}
	if err := h.actionAudit.Record(ctx, r, AlertActionAuditRecord{Action: "ALERT_FEEDBACK_SUBMITTED", ObjectType: "alert_feedback", ObjectID: feedbackID, TenantID: tenantID, UserID: userID, AlertID: alertID, Result: "success", Detail: map[string]interface{}{"label": req.Label, "reason_code": req.ReasonCode, "add_to_whitelist": req.AddToWhitelist}}); err != nil {
		logger.Error("Failed to audit feedback", zap.Error(err))
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "AUDIT_FAILED", "feedback was persisted but its audit record could not be committed")
		return
	}

	httpx.JSONCreated(w, ctx, response)
}

// GetFeedback 获取告警反馈历史
func (h *FeedbackHandler) GetFeedback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasAlertReadPermission(ctx) {
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied, "Permission denied: alert:read required", httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	logger := logging.L(ctx)
	vars := mux.Vars(r)
	alertID := vars["id"]
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		errors.WriteError(w, errors.New(errors.ErrCodeTenantNotFound, "tenant_id is required"),
			httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	// 修复：先验证告警是否存在
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
	logger.Debug("Get alert feedback",
		zap.String("alert_id", alertID),
		zap.String("tenant_id", tenantID))
	// 从 ClickHouse 查询反馈历史
	var feedbacks interface{} = []interface{}{}
	if h.repo != nil {
		records, err := h.repo.GetByAlertID(ctx, tenantID, alertID)
		if err == nil && records != nil {
			feedbacks = records
		} else if err != nil {
			logger.Warn("Failed to query feedback history", zap.Error(err))
		}
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{
		"alert_id":  alertID,
		"feedbacks": feedbacks,
	})
}

// GetReasonCodes 获取所有误报原因码
func (h *FeedbackHandler) GetReasonCodes(w http.ResponseWriter, r *http.Request) {
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

// GetFeedbackStats 获取反馈统计（TP/FP 分布）
func (h *FeedbackHandler) GetFeedbackStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !hasAlertReadPermission(ctx) {
		errors.WriteErrorWithStatus(w, http.StatusForbidden, errors.ErrCodePermissionDenied, "Permission denied: alert:read required", httpx.GetTraceID(ctx), r.URL.Path)
		return
	}
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "MISSING_PARAM", "tenant_id required")
		return
	}
	stats, err := h.repo.GetStats(ctx, tenantID, 30*24*time.Hour)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, stats)
}

// GetFPRanking 获取误报原因排行
func (h *FeedbackHandler) GetFPRanking(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := h.extractTenantID(r)
	if tenantID == "" {
		httpx.JSONError(w, ctx, http.StatusBadRequest, "MISSING_PARAM", "tenant_id required")
		return
	}
	ranking, err := h.repo.GetFPRanking(ctx, tenantID, 10)
	if err != nil {
		httpx.JSONError(w, ctx, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	httpx.JSONSuccess(w, ctx, map[string]interface{}{"fp_ranking": ranking})
}

// publishFeedback 发布反馈到Kafka
func (h *FeedbackHandler) publishFeedback(ctx context.Context, feedback *AlertFeedbackExtended) error {
	key := feedback.TenantID + ":" + feedback.AlertID
	headers := []kafka.MessageHeader{
		{Key: "tenant_id", Value: feedback.TenantID},
		{Key: "alert_id", Value: feedback.AlertID},
		{Key: "label", Value: feedback.Label},
		{Key: "feedback_id", Value: feedback.FeedbackID},
	}
	return h.kafkaProducer.SendJSON(ctx, key, feedback, headers...)
}

// createWhitelistDraft 基于误报反馈生成白名单审批草案。
func (h *FeedbackHandler) createWhitelistDraft(ctx context.Context, request *http.Request, tenantID, alertID, feedbackID, userID, reasonCode string, alert *service.AlertDetailDTO) (*whitelist.Entry, error) {
	logger := logging.L(ctx)
	srcIP, dstIP, alertType := "", "", ""
	if alert != nil {
		srcIP = alert.SrcIP
		dstIP = alert.DstIP
		alertType = alert.AlertType
	}
	logger.Info("Creating whitelist draft from alert feedback",
		zap.String("tenant_id", tenantID),
		zap.String("alert_id", alertID),
		zap.String("feedback_id", feedbackID),
		zap.String("reason_code", reasonCode),
		zap.String("src_ip", srcIP),
		zap.String("dst_ip", dstIP),
		zap.String("alert_type", alertType))

	if h.whitelistRepo == nil {
		return nil, fmt.Errorf("whitelist repository is not available")
	}

	entry, err := buildWhitelistDraftEntry(tenantID, alertID, feedbackID, userID, reasonCode, alert)
	if err != nil {
		return nil, err
	}

	detail := map[string]interface{}{
		"type": entry.Type, "value": entry.Value, "status": entry.Status,
		"approval_status": entry.ApprovalStatus, "source_alert_id": alertID,
		"feedback_id": feedbackID, "request_id": httpx.GetRequestID(ctx),
		"trace_id": httpx.GetTraceID(ctx), "api_path": request.URL.Path,
		"creation_source": "alert_fp_feedback",
	}
	if err := h.whitelistRepo.CreateWithAudit(ctx, entry, whitelist.AuditRecord{
		UserID: userID, Action: "WHITELIST_CREATED", ObjectID: entry.ID,
		Detail: detail, IPAddress: feedbackClientIP(request), UserAgent: request.UserAgent(),
	}); err != nil {
		return nil, fmt.Errorf("failed to create whitelist draft: %w", err)
	}

	logger.Info("Whitelist draft created from alert feedback",
		zap.String("alert_id", alertID),
		zap.String("feedback_id", feedbackID),
		zap.String("whitelist_id", entry.ID),
		zap.String("whitelist_type", entry.Type),
		zap.String("value", entry.Value))
	return entry, nil
}

func feedbackClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	return strings.TrimSpace(strings.Split(r.RemoteAddr, ":")[0])
}

func buildWhitelistDraftEntry(tenantID, alertID, feedbackID, userID, reasonCode string, alert *service.AlertDetailDTO) (*whitelist.Entry, error) {
	if alert == nil {
		return nil, fmt.Errorf("alert is required")
	}
	// 白名单表当前只允许 ip/domain/fingerprint/subnet。误报反馈按真实可执行粒度落为 IP 条目。
	whitelistType := "ip"
	value := alert.SrcIP
	if value == "" {
		value = alert.DstIP
	}
	if value == "" {
		return nil, fmt.Errorf("alert has no src_ip or dst_ip to whitelist")
	}
	createdBy := userID
	if createdBy == "" {
		createdBy = "feedback-system"
	}
	return &whitelist.Entry{
		TenantID:      tenantID,
		Type:          whitelistType,
		Value:         value,
		Reason:        reasonCode,
		Description:   fmt.Sprintf("Auto-whitelist draft from FP feedback: %s (alert_id=%s, feedback_id=%s)", reasonCode, alertID, feedbackID),
		Status:        "draft",
		SourceAlertID: alertID,
		FeedbackID:    feedbackID,
		CreatedBy:     createdBy,
		ExpiresAt:     func() *time.Time { t := time.Now().Add(90 * 24 * time.Hour); return &t }(), // 90 天自动过期
	}, nil
}

func feedbackWhitelistDraftResponse(entry *whitelist.Entry) *FeedbackWhitelistDraftResponse {
	if entry == nil {
		return nil
	}
	return &FeedbackWhitelistDraftResponse{
		ID:            entry.ID,
		Type:          entry.Type,
		Value:         entry.Value,
		Reason:        entry.Reason,
		Description:   entry.Description,
		Status:        entry.Status,
		SourceAlertID: entry.SourceAlertID,
		FeedbackID:    entry.FeedbackID,
		URL:           fmt.Sprintf("/whitelist?source_alert=%s&draft_id=%s", url.QueryEscape(entry.SourceAlertID), url.QueryEscape(entry.ID)),
		ExpiresAt:     entry.ExpiresAt,
		CreatedAt:     entry.CreatedAt,
	}
}

// ToProtoFeedback 将扩展反馈转换为 Proto AlertFeedback
func (f *AlertFeedbackExtended) ToProtoFeedback() *pb.AlertFeedback {
	return &pb.AlertFeedback{
		AlertId:        f.AlertID,
		TenantId:       f.TenantID,
		Label:          f.Label,
		ReasonCode:     f.ReasonCode,
		Comment:        f.Comment,
		UserId:         f.UserID,
		AddToWhitelist: 0,
	}
}

// FromProtoFeedback 从 Proto AlertFeedback 创建扩展反馈
func FromProtoFeedback(proto *pb.AlertFeedback) *AlertFeedbackExtended {
	return &AlertFeedbackExtended{
		AlertID:        proto.GetAlertId(),
		TenantID:       proto.GetTenantId(),
		Label:          proto.GetLabel(),
		ReasonCode:     proto.GetReasonCode(),
		Comment:        proto.GetComment(),
		UserID:         proto.GetUserId(),
		AddToWhitelist: proto.GetAddToWhitelist() != 0,
	}
}
func (h *FeedbackHandler) extractTenantID(r *http.Request) string {
	tenantID := httpx.GetTenantID(r.Context())
	if tenantID == "" {
		tenantID = r.Header.Get("X-Tenant-ID")
	}
	if tenantID == "" {
		tenantID = r.URL.Query().Get("tenant_id")
	}
	return tenantID
}
func (h *FeedbackHandler) extractUserID(r *http.Request) string {
	userID := httpx.GetUserID(r.Context())
	if userID == "" {
		userID = r.Header.Get("X-User-ID")
	}
	return userID
}

// 误报原因码定义
var FPReasonCodes = map[string]string{
	"WHITELIST":       "已知白名单行为",
	"FALSE_ALARM":     "规则/模型误报",
	"AUTHORIZED":      "授权行为",
	"TEST":            "测试流量",
	"DUPLICATE":       "重复告警",
	"INSUFFICIENT":    "证据不足",
	"BUSINESS_NORMAL": "正常业务行为",
	"CONFIG_ERROR":    "配置错误导致",
	"TUNING_NEEDED":   "需要调优",
	"OTHER":           "其他原因",
}

// isValidReasonCode 验证原因码是否有效
func isValidReasonCode(code string) bool {
	_, exists := FPReasonCodes[code]
	return exists
}

// GetFPReasonCodes 获取所有误报原因码
func GetFPReasonCodes() map[string]string {
	return FPReasonCodes
}
