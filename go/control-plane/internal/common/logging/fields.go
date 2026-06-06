package logging

const (
	FieldTraceID   = "trace_id"
	FieldSpanID    = "span_id"
	FieldRequestID = "request_id"
	FieldParentID  = "parent_id"

	FieldTenantID = "tenant_id"
	FieldUserID   = "user_id"
	FieldUsername = "username"
	FieldProbeID  = "probe_id"

	FieldRunID       = "run_id"
	FieldEventID     = "event_id"
	FieldAlertID     = "alert_id"
	FieldRuleID      = "rule_id"
	FieldSessionID   = "session_id"
	FieldCommunityID = "community_id"
	FieldFlowID      = "flow_id"

	FieldMethod    = "method"
	FieldPath      = "path"
	FieldStatus    = "status"
	FieldLatency   = "latency_ms"
	FieldClientIP  = "client_ip"
	FieldUserAgent = "user_agent"

	FieldError      = "error"
	FieldErrorCode  = "error_code"
	FieldStackTrace = "stack_trace"

	FieldComponent   = "component"
	FieldService     = "service"
	FieldVersion     = "version"
	FieldEnvironment = "environment"
)

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
