package audit

import (
	"context"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
)

// AlertAuditLogger 告警审计日志记录器
type AlertAuditLogger struct {
	logger *audit.Logger
}

// NewAlertAuditLogger 创建告警审计日志记录器
func NewAlertAuditLogger(auditLogger *audit.Logger) *AlertAuditLogger {
	return &AlertAuditLogger{
		logger: auditLogger,
	}
}

// LogAlertCreate 记录告警创建
func (l *AlertAuditLogger) LogAlertCreate(ctx context.Context, alertID, tenantID, alertType, severity string) {
	lc := logging.LogContextFromContext(ctx)

	l.logger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeAlertTriage,
		TenantID:     tenantID,
		UserID:       lc.UserID,
		Action:       "create",
		ResourceType: "alert",
		ResourceID:   alertID,
		Detail: map[string]interface{}{
			"alert_type": alertType,
			"severity":   severity,
		},
		Result: audit.ResultSuccess,
	})
}

// LogAlertStatusChange 记录告警状态变更
func (l *AlertAuditLogger) LogAlertStatusChange(ctx context.Context, alertID, tenantID, oldStatus, newStatus string) {
	l.LogAlertStatusChangeWithReason(ctx, alertID, tenantID, "", oldStatus, newStatus, "")
}

// LogAlertStatusChangeWithReason 记录带原因的告警状态变更
func (l *AlertAuditLogger) LogAlertStatusChangeWithReason(ctx context.Context, alertID, tenantID, userID, oldStatus, newStatus, reason string) {
	lc := logging.LogContextFromContext(ctx)
	if userID == "" {
		userID = lc.UserID
	}

	var detail map[string]interface{}
	if reason != "" {
		detail = map[string]interface{}{"reason": reason}
	}

	l.logger.LogAlertActionWithDetail(ctx, audit.EventTypeAlertTriage, tenantID, userID, alertID, oldStatus, newStatus, detail)
}

// LogAlertAssign 记录告警分配
func (l *AlertAuditLogger) LogAlertAssign(ctx context.Context, alertID, tenantID, assigneeID string) {
	lc := logging.LogContextFromContext(ctx)

	l.logger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeAlertAssign,
		TenantID:     tenantID,
		UserID:       lc.UserID,
		Action:       "assign",
		ResourceType: "alert",
		ResourceID:   alertID,
		NewValue: map[string]string{
			"assignee": assigneeID,
		},
		Result: audit.ResultSuccess,
	})
}

// LogAlertClose 记录告警关闭
func (l *AlertAuditLogger) LogAlertClose(ctx context.Context, alertID, tenantID, reason string) {
	lc := logging.LogContextFromContext(ctx)

	l.logger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeAlertClose,
		TenantID:     tenantID,
		UserID:       lc.UserID,
		Action:       "close",
		ResourceType: "alert",
		ResourceID:   alertID,
		Detail: map[string]interface{}{
			"reason": reason,
		},
		Result: audit.ResultSuccess,
	})
}

// LogAlertFeedback 记录告警反馈
func (l *AlertAuditLogger) LogAlertFeedback(ctx context.Context, alertID, tenantID, label, comment string) {
	lc := logging.LogContextFromContext(ctx)

	l.logger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeAlertFeedback,
		TenantID:     tenantID,
		UserID:       lc.UserID,
		Action:       "feedback",
		ResourceType: "alert",
		ResourceID:   alertID,
		Detail: map[string]interface{}{
			"label":   label,
			"comment": comment,
		},
		Result: audit.ResultSuccess,
	})
}
