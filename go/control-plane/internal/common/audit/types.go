////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/audit/types.go
// 优化版 v2：补全 Ingest Gateway 所需的所有事件类型定义
////////////////////////////////////////////////////////////////////////////////

package audit

import (
	"time"
)

// EventType 审计事件类型
type EventType string

const (
	// ==================== 认证与授权 ====================
	EventTypeLogin            EventType = "AUTH_LOGIN"
	EventTypeLogout           EventType = "AUTH_LOGOUT"
	EventTypeTokenRefresh     EventType = "AUTH_TOKEN_REFRESH"
	EventTypeLoginFailed      EventType = "AUTH_LOGIN_FAILED"
	EventTypeAuthFailure      EventType = "AUTH_FAILURE"           // 新增
	EventTypeAccessDenied     EventType = "AUTH_ACCESS_DENIED"     // 新增
	EventTypePermissionDenied EventType = "AUTH_PERMISSION_DENIED" // 新增
	EventTypeRateLimit        EventType = "AUTH_RATE_LIMIT"        // 新增

	// ==================== 用户管理 ====================
	EventTypeUserCreate EventType = "USER_CREATE"
	EventTypeUserUpdate EventType = "USER_UPDATE"
	EventTypeUserDelete EventType = "USER_DELETE"
	EventTypeRoleAssign EventType = "USER_ROLE_ASSIGN"

	// ==================== 规则管理 ====================
	EventTypeRuleCreate  EventType = "RULE_CREATE"
	EventTypeRuleUpdate  EventType = "RULE_UPDATE"
	EventTypeRuleDelete  EventType = "RULE_DELETE"
	EventTypeRuleEnable  EventType = "RULE_ENABLE"
	EventTypeRuleDisable EventType = "RULE_DISABLE"

	// ==================== 部署管理 ====================
	EventTypeDeployCreate   EventType = "DEPLOY_CREATE"
	EventTypeDeployGray     EventType = "DEPLOY_GRAY"
	EventTypeDeployActivate EventType = "DEPLOY_ACTIVATE"
	EventTypeDeployRollback EventType = "DEPLOY_ROLLBACK"

	// ==================== 告警操作 ====================
	EventTypeAlertTriage   EventType = "ALERT_TRIAGE"
	EventTypeAlertAssign   EventType = "ALERT_ASSIGN"
	EventTypeAlertClose    EventType = "ALERT_CLOSE"
	EventTypeAlertFeedback EventType = "ALERT_FEEDBACK"

	// ==================== 取证操作 ====================
	EventTypePcapCut      EventType = "PCAP_CUT"
	EventTypePcapDownload EventType = "PCAP_DOWNLOAD"
	EventTypeArkimeAccess EventType = "ARKIME_ACCESS"

	// ==================== 数据操作 ====================
	EventTypeExportAlerts   EventType = "EXPORT_ALERTS"
	EventTypeExportSessions EventType = "EXPORT_SESSIONS"
	EventTypeExportReport   EventType = "EXPORT_REPORT"
	EventTypeDataIngested   EventType = "DATA_INGESTED" // 新增
	EventTypeDataExport     EventType = "DATA_EXPORT"   // 新增
	EventTypeDataDelete     EventType = "DATA_DELETE"   // 新增

	// ==================== API Token ====================
	EventTypeTokenCreate EventType = "TOKEN_CREATE"
	EventTypeTokenRevoke EventType = "TOKEN_REVOKE"

	// ==================== 探针管理 ====================
	EventTypeProbeRegister   EventType = "PROBE_REGISTER"   // 新增
	EventTypeProbeUnregister EventType = "PROBE_UNREGISTER" // 新增
	EventTypeProbeHeartbeat  EventType = "PROBE_HEARTBEAT"  // 新增

	// ==================== 系统操作 ====================
	EventTypeConfigUpdate EventType = "CONFIG_UPDATE"
	EventTypeConfigChange EventType = "CONFIG_CHANGE" // 兼容别名
	EventTypeSystemPurge  EventType = "SYSTEM_PURGE"
	EventTypeSystemError  EventType = "SYSTEM_ERROR"  // 新增
	EventTypeSystemStatus EventType = "SYSTEM_STATUS" // 新增
)

// Sensitivity 敏感级别
type Sensitivity string

const (
	SensitivityLow      Sensitivity = "low"
	SensitivityMedium   Sensitivity = "medium"
	SensitivityHigh     Sensitivity = "high"
	SensitivityCritical Sensitivity = "critical"
)

// Result 操作结果
type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
	ResultPartial Result = "partial"
	ResultUnknown Result = "unknown"
)

// AuditEvent 审计事件
type AuditEvent struct {
	// 基础信息
	EventID   string    `json:"event_id"`
	EventType EventType `json:"event_type"`
	Timestamp time.Time `json:"timestamp"`

	// 主体信息
	TenantID    string `json:"tenant_id"`
	UserID      string `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	ProbeID     string `json:"probe_id,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	SourceIP    string `json:"source_ip,omitempty"` // 兼容性字段

	// 操作信息
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id,omitempty"`

	// 详情
	Detail   map[string]interface{} `json:"detail,omitempty"`
	OldValue interface{}            `json:"old_value,omitempty"`
	NewValue interface{}            `json:"new_value,omitempty"`

	// 结果
	Result    Result `json:"result"`
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`

	// 上下文
	IPAddr    string `json:"ip_addr,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`

	// 敏感级别
	Sensitivity Sensitivity `json:"sensitivity"`
}

// EventTypeInfo 事件类型信息
type EventTypeInfo struct {
	Type        EventType
	Description string
	Sensitivity Sensitivity
	Category    string
}

// GetEventTypeInfo 获取事件类型信息
func GetEventTypeInfo(t EventType) EventTypeInfo {
	info, ok := eventTypeRegistry[t]
	if !ok {
		return EventTypeInfo{
			Type:        t,
			Description: string(t),
			Sensitivity: SensitivityMedium,
			Category:    "unknown",
		}
	}
	return info
}

var eventTypeRegistry = map[EventType]EventTypeInfo{
	// 认证事件
	EventTypeLogin:            {EventTypeLogin, "User login", SensitivityMedium, "authentication"},
	EventTypeLogout:           {EventTypeLogout, "User logout", SensitivityLow, "authentication"},
	EventTypeTokenRefresh:     {EventTypeTokenRefresh, "Token refresh", SensitivityLow, "authentication"},
	EventTypeLoginFailed:      {EventTypeLoginFailed, "Login failed", SensitivityHigh, "authentication"},
	EventTypeAuthFailure:      {EventTypeAuthFailure, "Authentication failure", SensitivityHigh, "authentication"},
	EventTypeAccessDenied:     {EventTypeAccessDenied, "Access denied", SensitivityHigh, "authentication"},
	EventTypePermissionDenied: {EventTypePermissionDenied, "Permission denied", SensitivityHigh, "authentication"},
	EventTypeRateLimit:        {EventTypeRateLimit, "Rate limit exceeded", SensitivityMedium, "authentication"},

	// 规则管理
	EventTypeRuleCreate:  {EventTypeRuleCreate, "Rule created", SensitivityMedium, "rule_management"},
	EventTypeRuleUpdate:  {EventTypeRuleUpdate, "Rule updated", SensitivityMedium, "rule_management"},
	EventTypeRuleDelete:  {EventTypeRuleDelete, "Rule deleted", SensitivityHigh, "rule_management"},
	EventTypeRuleEnable:  {EventTypeRuleEnable, "Rule enabled", SensitivityMedium, "rule_management"},
	EventTypeRuleDisable: {EventTypeRuleDisable, "Rule disabled", SensitivityMedium, "rule_management"},

	// 部署管理
	EventTypeDeployCreate:   {EventTypeDeployCreate, "Deployment created", SensitivityMedium, "deployment"},
	EventTypeDeployGray:     {EventTypeDeployGray, "Gray deployment started", SensitivityHigh, "deployment"},
	EventTypeDeployActivate: {EventTypeDeployActivate, "Deployment activated", SensitivityHigh, "deployment"},
	EventTypeDeployRollback: {EventTypeDeployRollback, "Deployment rolled back", SensitivityCritical, "deployment"},

	// 告警操作
	EventTypeAlertTriage:   {EventTypeAlertTriage, "Alert triaged", SensitivityLow, "alert"},
	EventTypeAlertAssign:   {EventTypeAlertAssign, "Alert assigned", SensitivityLow, "alert"},
	EventTypeAlertClose:    {EventTypeAlertClose, "Alert closed", SensitivityMedium, "alert"},
	EventTypeAlertFeedback: {EventTypeAlertFeedback, "Alert feedback", SensitivityMedium, "alert"},

	// 取证操作
	EventTypePcapCut:      {EventTypePcapCut, "PCAP cut requested", SensitivityHigh, "forensics"},
	EventTypePcapDownload: {EventTypePcapDownload, "PCAP downloaded", SensitivityCritical, "forensics"},
	EventTypeArkimeAccess: {EventTypeArkimeAccess, "Arkime accessed", SensitivityHigh, "forensics"},

	// 数据操作
	EventTypeExportAlerts:   {EventTypeExportAlerts, "Alerts exported", SensitivityHigh, "export"},
	EventTypeExportSessions: {EventTypeExportSessions, "Sessions exported", SensitivityHigh, "export"},
	EventTypeExportReport:   {EventTypeExportReport, "Report exported", SensitivityMedium, "export"},
	EventTypeDataIngested:   {EventTypeDataIngested, "Data ingested", SensitivityLow, "data"},
	EventTypeDataExport:     {EventTypeDataExport, "Data exported", SensitivityMedium, "data"},
	EventTypeDataDelete:     {EventTypeDataDelete, "Data deleted", SensitivityCritical, "data"},

	// API Token
	EventTypeTokenCreate: {EventTypeTokenCreate, "API token created", SensitivityHigh, "token"},
	EventTypeTokenRevoke: {EventTypeTokenRevoke, "API token revoked", SensitivityHigh, "token"},

	// 探针管理
	EventTypeProbeRegister:   {EventTypeProbeRegister, "Probe registered", SensitivityHigh, "probe"},
	EventTypeProbeUnregister: {EventTypeProbeUnregister, "Probe unregistered", SensitivityHigh, "probe"},
	EventTypeProbeHeartbeat:  {EventTypeProbeHeartbeat, "Probe heartbeat", SensitivityLow, "probe"},

	// 系统操作
	EventTypeConfigUpdate: {EventTypeConfigUpdate, "Config updated", SensitivityCritical, "system"},
	EventTypeConfigChange: {EventTypeConfigChange, "Config changed", SensitivityCritical, "system"},
	EventTypeSystemPurge:  {EventTypeSystemPurge, "System data purged", SensitivityCritical, "system"},
	EventTypeSystemError:  {EventTypeSystemError, "System error", SensitivityHigh, "system"},
	EventTypeSystemStatus: {EventTypeSystemStatus, "System status change", SensitivityMedium, "system"},
}
