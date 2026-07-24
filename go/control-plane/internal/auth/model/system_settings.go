package model

import "time"

// SystemSite describes a tenant-visible campus or network isolation scope.
type SystemSite struct {
	ID              string `json:"id"`
	ParentID        string `json:"parent_id,omitempty"`
	Name            string `json:"name"`
	CIDR            string `json:"cidr,omitempty"`
	Kind            string `json:"kind"`
	IsolationStatus string `json:"isolation_status"`
}

// RetentionPolicy is the tenant-owned lifecycle policy for one data class.
type RetentionPolicy struct {
	DataType    string `json:"data_type"`
	Retention   int    `json:"retention_days"`
	Action      string `json:"action"`
	Status      string `json:"status"`
	NextAction  string `json:"next_action"`
	Description string `json:"description,omitempty"`
}

// IntegrationSetting exposes connection state without returning secret values.
type IntegrationSetting struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Enabled      bool       `json:"enabled"`
	Status       string     `json:"status"`
	SecretRef    string     `json:"secret_ref,omitempty"`
	EndpointHint string     `json:"endpoint_hint,omitempty"`
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
}

// SecuritySettings contains editable tenant-level security and display policy.
type SecuritySettings struct {
	LoginPolicy        string   `json:"login_policy"`
	PasswordPolicy     string   `json:"password_policy"`
	MFAEnabled         bool     `json:"mfa_enabled"`
	IPAccessRules      int      `json:"ip_access_rules"`
	MaskingPolicy      string   `json:"masking_policy"`
	DefaultTimeRange   string   `json:"default_time_range"`
	AlertThreshold     string   `json:"alert_threshold"`
	RefreshIntervalSec int      `json:"refresh_interval_sec"`
	ScreenMasking      bool     `json:"screen_masking"`
	FeatureFlags       []string `json:"feature_flags"`
}

// SystemSettings is the persisted tenant-level configuration document.
type SystemSettings struct {
	Sites             []SystemSite         `json:"sites"`
	RetentionPolicies []RetentionPolicy    `json:"retention_policies"`
	Integrations      []IntegrationSetting `json:"integrations"`
	Security          SecuritySettings     `json:"security"`
}

// SystemRole is a role row sourced from the RBAC tables.
type SystemRole struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	System      bool     `json:"system"`
}

// SystemTokenSummary contains tenant-scoped token aggregates only.
type SystemTokenSummary struct {
	Total        int `json:"total"`
	Active       int `json:"active"`
	ExpiringSoon int `json:"expiring_soon"`
	Revoked      int `json:"revoked"`
}

// SystemSettingsWorkbench is the read model consumed by the settings page.
type SystemSettingsWorkbench struct {
	TenantID     string             `json:"tenant_id"`
	TenantName   string             `json:"tenant_name"`
	TenantStatus string             `json:"tenant_status"`
	Revision     int64              `json:"revision"`
	Settings     SystemSettings     `json:"settings"`
	Roles        []SystemRole       `json:"roles"`
	Tokens       SystemTokenSummary `json:"tokens"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// DefaultSystemSettings provides the first persisted tenant configuration.
func DefaultSystemSettings() SystemSettings {
	return SystemSettings{
		Sites: []SystemSite{
			{ID: "east-campus", Name: "华东园区（主租户）", Kind: "site", IsolationStatus: "isolated"},
			{ID: "core-campus", ParentID: "east-campus", Name: "核心园区", Kind: "campus", IsolationStatus: "isolated"},
			{ID: "campus-a", ParentID: "east-campus", Name: "分园区A", Kind: "campus", IsolationStatus: "isolated"},
			{ID: "campus-b", ParentID: "east-campus", Name: "分园区B", Kind: "campus", IsolationStatus: "isolated"},
			{ID: "rd-network", ParentID: "east-campus", Name: "研发网络", CIDR: "10.10.0.0/16", Kind: "folder", IsolationStatus: "isolated"},
			{ID: "teaching-segment", ParentID: "rd-network", Name: "教学网段", CIDR: "10.10.0.0/16", Kind: "segment", IsolationStatus: "isolated"},
			{ID: "test-segment", ParentID: "rd-network", Name: "测试网段", CIDR: "10.11.0.0/16", Kind: "segment", IsolationStatus: "isolated"},
			{ID: "office-segment", ParentID: "rd-network", Name: "办公网段", CIDR: "10.12.0.0/16", Kind: "segment", IsolationStatus: "isolated"},
			{ID: "production-network", ParentID: "east-campus", Name: "生产网段", Kind: "folder", IsolationStatus: "isolated"},
			{ID: "south-campus", Name: "华南园区", Kind: "site", IsolationStatus: "partial"},
			{ID: "overseas-campus", Name: "海外园区", Kind: "site", IsolationStatus: "unisolated"},
		},
		RetentionPolicies: []RetentionPolicy{
			{DataType: "Flow", Retention: 90, Action: "delete", Status: "healthy", NextAction: "到期删除"},
			{DataType: "Session", Retention: 180, Action: "delete", Status: "healthy", NextAction: "到期删除"},
			{DataType: "Alert", Retention: 365, Action: "delete", Status: "healthy", NextAction: "到期删除"},
			{DataType: "Evidence", Retention: 1095, Action: "archive", Status: "healthy", NextAction: "到期归档"},
			{DataType: "PCAP", Retention: 30, Action: "delete", Status: "expiring", NextAction: "即将删除"},
			{DataType: "Audit", Retention: 1825, Action: "archive", Status: "healthy", NextAction: "到期归档"},
		},
		Integrations: []IntegrationSetting{
			{ID: "keycloak", Name: "Keycloak", Enabled: true, Status: "healthy", SecretRef: "secret://traffic-credentials/keycloak"},
			{ID: "apisix", Name: "APISIX", Enabled: true, Status: "healthy"},
			{ID: "kafka", Name: "Kafka", Enabled: true, Status: "healthy", SecretRef: "secret://traffic-credentials/kafka"},
			{ID: "minio", Name: "MinIO", Enabled: true, Status: "healthy", SecretRef: "secret://traffic-credentials/minio"},
			{ID: "opensearch", Name: "OpenSearch", Enabled: true, Status: "healthy", SecretRef: "secret://traffic-credentials/opensearch"},
			{ID: "nebula", Name: "NebulaGraph", Enabled: true, Status: "healthy", SecretRef: "secret://traffic-credentials/nebula"},
			{ID: "webhook", Name: "Webhook", Enabled: true, Status: "healthy", SecretRef: "secret://traffic-credentials/webhook"},
		},
		Security: SecuritySettings{
			LoginPolicy: "SSO 强制登录", PasswordPolicy: "强度：高 / 90 天", MFAEnabled: true,
			IPAccessRules: 24, MaskingPolicy: "中等脱敏", DefaultTimeRange: "last_24h",
			AlertThreshold: "默认策略", RefreshIntervalSec: 30, ScreenMasking: true,
			FeatureFlags: []string{"pcap_search", "asset_search", "rule_search", "script_center", "audit_export", "token_rotation", "mfa", "masking", "notifications", "evidence_export", "model_activation", "deployment_audit"},
		},
	}
}
