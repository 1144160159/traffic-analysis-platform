package logging

// 标准字段名定义，确保全系统日志字段一致
const (
	// 追踪字段
	FieldTraceID   = "trace_id"
	FieldSpanID    = "span_id"
	FieldRequestID = "request_id"
	FieldParentID  = "parent_id"

	// 租户与用户
	FieldTenantID = "tenant_id"
	FieldUserID   = "user_id"
	FieldUsername = "username"
	FieldProbeID  = "probe_id"

	// 业务字段
	FieldRunID       = "run_id"
	FieldEventID     = "event_id"
	FieldAlertID     = "alert_id"
	FieldRuleID      = "rule_id"
	FieldSessionID   = "session_id"
	FieldCommunityID = "community_id"
	FieldFlowID      = "flow_id"

	// 请求字段
	FieldMethod    = "method"
	FieldPath      = "path"
	FieldStatus    = "status"
	FieldLatency   = "latency_ms"
	FieldClientIP  = "client_ip"
	FieldUserAgent = "user_agent"

	// 错误字段
	FieldError      = "error"
	FieldErrorCode  = "error_code"
	FieldStackTrace = "stack_trace"

	// 组件字段
	FieldComponent   = "component"
	FieldService     = "service"
	FieldVersion     = "version"
	FieldEnvironment = "environment"
)

// LogContext 日志上下文结构
type LogContext struct {
	TraceID   string
	SpanID    string
	RequestID string
	TenantID  string
	UserID    string
	Username  string
	ProbeID   string
	RunID     string
	Component string
	Service   string
}

// ToFields 转换为zap字段
func (c *LogContext) ToFields() map[string]interface{} {
	fields := make(map[string]interface{})

	if c.TraceID != "" {
		fields[FieldTraceID] = c.TraceID
	}
	if c.SpanID != "" {
		fields[FieldSpanID] = c.SpanID
	}
	if c.RequestID != "" {
		fields[FieldRequestID] = c.RequestID
	}
	if c.TenantID != "" {
		fields[FieldTenantID] = c.TenantID
	}
	if c.UserID != "" {
		fields[FieldUserID] = c.UserID
	}
	if c.Username != "" {
		fields[FieldUsername] = c.Username
	}
	if c.ProbeID != "" {
		fields[FieldProbeID] = c.ProbeID
	}
	if c.RunID != "" {
		fields[FieldRunID] = c.RunID
	}
	if c.Component != "" {
		fields[FieldComponent] = c.Component
	}
	if c.Service != "" {
		fields[FieldService] = c.Service
	}

	return fields
}
