package audit

import (
	"time"
)

type EventType string

const (
	EventTypeLogin            EventType = "AUTH_LOGIN"
	EventTypeLogout           EventType = "AUTH_LOGOUT"
	EventTypeTokenRefresh     EventType = "AUTH_TOKEN_REFRESH"
	EventTypeLoginFailed      EventType = "AUTH_LOGIN_FAILED"
	EventTypeAuthFailure      EventType = "AUTH_FAILURE"
	EventTypeAccessDenied     EventType = "AUTH_ACCESS_DENIED"
	EventTypePermissionDenied EventType = "AUTH_PERMISSION_DENIED"
	EventTypeRateLimit        EventType = "AUTH_RATE_LIMIT"

	EventTypeUserCreate EventType = "USER_CREATE"
	EventTypeUserUpdate EventType = "USER_UPDATE"
	EventTypeUserDelete EventType = "USER_DELETE"
	EventTypeRoleAssign EventType = "USER_ROLE_ASSIGN"

	EventTypeRuleCreate  EventType = "RULE_CREATE"
	EventTypeRuleUpdate  EventType = "RULE_UPDATE"
	EventTypeRuleDelete  EventType = "RULE_DELETE"
	EventTypeRuleEnable  EventType = "RULE_ENABLE"
	EventTypeRuleDisable EventType = "RULE_DISABLE"

	EventTypeDeployCreate   EventType = "DEPLOY_CREATE"
	EventTypeDeployGray     EventType = "DEPLOY_GRAY"
	EventTypeDeployActivate EventType = "DEPLOY_ACTIVATE"
	EventTypeDeployRollback EventType = "DEPLOY_ROLLBACK"
	EventTypeDeployPause    EventType = "DEPLOY_PAUSE"
	EventTypeDeployResume   EventType = "DEPLOY_RESUME"

	EventTypeAlertTriage   EventType = "ALERT_TRIAGE"
	EventTypeAlertAssign   EventType = "ALERT_ASSIGN"
	EventTypeAlertClose    EventType = "ALERT_CLOSE"
	EventTypeAlertFeedback EventType = "ALERT_FEEDBACK"

	// Model Registry audit events (MLOps integration)
	EventTypeModelCreate           EventType = "MODEL_CREATE"
	EventTypeModelUpdate           EventType = "MODEL_UPDATE"
	EventTypeModelDelete           EventType = "MODEL_DELETE"
	EventTypeModelVersionCreate    EventType = "MODEL_VERSION_CREATE"
	EventTypeModelVersionActivate  EventType = "MODEL_VERSION_ACTIVATE"
	EventTypeModelVersionDeprecate EventType = "MODEL_VERSION_DEPRECATE"

	EventTypePcapCut      EventType = "PCAP_CUT"
	EventTypePcapCancel   EventType = "PCAP_CANCEL"
	EventTypePcapDownload EventType = "PCAP_DOWNLOAD"
	EventTypeArkimeAccess EventType = "ARKIME_ACCESS"

	EventTypeExportAlerts   EventType = "EXPORT_ALERTS"
	EventTypeExportSessions EventType = "EXPORT_SESSIONS"
	EventTypeExportReport   EventType = "EXPORT_REPORT"
	EventTypeDataIngested   EventType = "DATA_INGESTED"
	EventTypeDataExport     EventType = "DATA_EXPORT"
	EventTypeDataDelete     EventType = "DATA_DELETE"

	EventTypeTokenCreate EventType = "TOKEN_CREATE"
	EventTypeTokenRevoke EventType = "TOKEN_REVOKE"

	EventTypeProbeRegister   EventType = "PROBE_REGISTER"
	EventTypeProbeUnregister EventType = "PROBE_UNREGISTER"
	EventTypeProbeHeartbeat  EventType = "PROBE_HEARTBEAT"

	EventTypeConfigUpdate EventType = "CONFIG_UPDATE"
	EventTypeConfigChange EventType = "CONFIG_CHANGE"
	EventTypeSystemPurge  EventType = "SYSTEM_PURGE"
	EventTypeSystemError  EventType = "SYSTEM_ERROR"
	EventTypeSystemStatus EventType = "SYSTEM_STATUS"
)

type Sensitivity string

const (
	SensitivityLow      Sensitivity = "low"
	SensitivityMedium   Sensitivity = "medium"
	SensitivityHigh     Sensitivity = "high"
	SensitivityCritical Sensitivity = "critical"
)

type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
	ResultPartial Result = "partial"
	ResultUnknown Result = "unknown"
)

type AuditEvent struct {
	EventID   string    `json:"event_id"`
	EventType EventType `json:"event_type"`
	Timestamp time.Time `json:"timestamp"`

	TenantID    string `json:"tenant_id"`
	UserID      string `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	ProbeID     string `json:"probe_id,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
	SourceIP    string `json:"source_ip,omitempty"`

	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id,omitempty"`

	Detail   map[string]interface{} `json:"detail,omitempty"`
	OldValue interface{}            `json:"old_value,omitempty"`
	NewValue interface{}            `json:"new_value,omitempty"`

	Result    Result `json:"result"`
	ErrorCode string `json:"error_code,omitempty"`
	ErrorMsg  string `json:"error_msg,omitempty"`

	IPAddr    string `json:"ip_addr,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
	RequestID string `json:"request_id,omitempty"`

	Sensitivity Sensitivity `json:"sensitivity"`
}

type EventTypeInfo struct {
	Type        EventType
	Description string
	Sensitivity Sensitivity
	Category    string
}

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

	EventTypeLogin:            {EventTypeLogin, "User login", SensitivityMedium, "authentication"},
	EventTypeLogout:           {EventTypeLogout, "User logout", SensitivityLow, "authentication"},
	EventTypeTokenRefresh:     {EventTypeTokenRefresh, "Token refresh", SensitivityLow, "authentication"},
	EventTypeLoginFailed:      {EventTypeLoginFailed, "Login failed", SensitivityHigh, "authentication"},
	EventTypeAuthFailure:      {EventTypeAuthFailure, "Authentication failure", SensitivityHigh, "authentication"},
	EventTypeAccessDenied:     {EventTypeAccessDenied, "Access denied", SensitivityHigh, "authentication"},
	EventTypePermissionDenied: {EventTypePermissionDenied, "Permission denied", SensitivityHigh, "authentication"},
	EventTypeRateLimit:        {EventTypeRateLimit, "Rate limit exceeded", SensitivityMedium, "authentication"},

	EventTypeRuleCreate:  {EventTypeRuleCreate, "Rule created", SensitivityMedium, "rule_management"},
	EventTypeRuleUpdate:  {EventTypeRuleUpdate, "Rule updated", SensitivityMedium, "rule_management"},
	EventTypeRuleDelete:  {EventTypeRuleDelete, "Rule deleted", SensitivityHigh, "rule_management"},
	EventTypeRuleEnable:  {EventTypeRuleEnable, "Rule enabled", SensitivityMedium, "rule_management"},
	EventTypeRuleDisable: {EventTypeRuleDisable, "Rule disabled", SensitivityMedium, "rule_management"},

	EventTypeDeployCreate:   {EventTypeDeployCreate, "Deployment created", SensitivityMedium, "deployment"},
	EventTypeDeployGray:     {EventTypeDeployGray, "Gray deployment started", SensitivityHigh, "deployment"},
	EventTypeDeployActivate: {EventTypeDeployActivate, "Deployment activated", SensitivityHigh, "deployment"},
	EventTypeDeployRollback: {EventTypeDeployRollback, "Deployment rolled back", SensitivityCritical, "deployment"},
	EventTypeDeployPause:    {EventTypeDeployPause, "Deployment paused", SensitivityMedium, "deployment"},
	EventTypeDeployResume:   {EventTypeDeployResume, "Deployment resumed", SensitivityMedium, "deployment"},

	EventTypeModelCreate:           {EventTypeModelCreate, "Model created", SensitivityMedium, "model_registry"},
	EventTypeModelUpdate:           {EventTypeModelUpdate, "Model updated", SensitivityMedium, "model_registry"},
	EventTypeModelDelete:           {EventTypeModelDelete, "Model deleted", SensitivityHigh, "model_registry"},
	EventTypeModelVersionCreate:    {EventTypeModelVersionCreate, "Model version registered", SensitivityMedium, "model_registry"},
	EventTypeModelVersionActivate:  {EventTypeModelVersionActivate, "Model version activated", SensitivityHigh, "model_registry"},
	EventTypeModelVersionDeprecate: {EventTypeModelVersionDeprecate, "Model version deprecated", SensitivityHigh, "model_registry"},

	EventTypeAlertTriage:   {EventTypeAlertTriage, "Alert triaged", SensitivityLow, "alert"},
	EventTypeAlertAssign:   {EventTypeAlertAssign, "Alert assigned", SensitivityLow, "alert"},
	EventTypeAlertClose:    {EventTypeAlertClose, "Alert closed", SensitivityMedium, "alert"},
	EventTypeAlertFeedback: {EventTypeAlertFeedback, "Alert feedback", SensitivityMedium, "alert"},

	EventTypePcapCut:      {EventTypePcapCut, "PCAP cut requested", SensitivityHigh, "forensics"},
	EventTypePcapCancel:   {EventTypePcapCancel, "PCAP cut cancelled", SensitivityHigh, "forensics"},
	EventTypePcapDownload: {EventTypePcapDownload, "PCAP downloaded", SensitivityCritical, "forensics"},
	EventTypeArkimeAccess: {EventTypeArkimeAccess, "Arkime accessed", SensitivityHigh, "forensics"},

	EventTypeExportAlerts:   {EventTypeExportAlerts, "Alerts exported", SensitivityHigh, "export"},
	EventTypeExportSessions: {EventTypeExportSessions, "Sessions exported", SensitivityHigh, "export"},
	EventTypeExportReport:   {EventTypeExportReport, "Report exported", SensitivityMedium, "export"},
	EventTypeDataIngested:   {EventTypeDataIngested, "Data ingested", SensitivityLow, "data"},
	EventTypeDataExport:     {EventTypeDataExport, "Data exported", SensitivityMedium, "data"},
	EventTypeDataDelete:     {EventTypeDataDelete, "Data deleted", SensitivityCritical, "data"},

	EventTypeTokenCreate: {EventTypeTokenCreate, "API token created", SensitivityHigh, "token"},
	EventTypeTokenRevoke: {EventTypeTokenRevoke, "API token revoked", SensitivityHigh, "token"},

	EventTypeProbeRegister:   {EventTypeProbeRegister, "Probe registered", SensitivityHigh, "probe"},
	EventTypeProbeUnregister: {EventTypeProbeUnregister, "Probe unregistered", SensitivityHigh, "probe"},
	EventTypeProbeHeartbeat:  {EventTypeProbeHeartbeat, "Probe heartbeat", SensitivityLow, "probe"},

	EventTypeConfigUpdate: {EventTypeConfigUpdate, "Config updated", SensitivityCritical, "system"},
	EventTypeConfigChange: {EventTypeConfigChange, "Config changed", SensitivityCritical, "system"},
	EventTypeSystemPurge:  {EventTypeSystemPurge, "System data purged", SensitivityCritical, "system"},
	EventTypeSystemError:  {EventTypeSystemError, "System error", SensitivityHigh, "system"},
	EventTypeSystemStatus: {EventTypeSystemStatus, "System status change", SensitivityMedium, "system"},
}
