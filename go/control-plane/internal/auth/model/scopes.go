////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/model/scopes.go
// 新增文件 - 修复 #A19：统一权限定义
// 说明：将所有 Scope 常量集中到此文件，避免重复定义
////////////////////////////////////////////////////////////////////////////////

package model

// =============================================================================
// 统一权限 Scope 定义
// =============================================================================

// 用户管理权限
const (
	ScopeUserRead   = "user:read"
	ScopeUserWrite  = "user:write"
	ScopeUserDelete = "user:delete"
)

// 告警管理权限
const (
	ScopeAlertRead   = "alert:read"
	ScopeAlertWrite  = "alert:write"
	ScopeAlertExport = "alert:export"
)

// 规则管理权限
const (
	ScopeRuleRead   = "rule:read"
	ScopeRuleWrite  = "rule:write"
	ScopeRuleDelete = "rule:delete"
)

// 部署管理权限
const (
	ScopeDeployRead     = "deploy:read"
	ScopeDeployCreate   = "deploy:create"
	ScopeDeployActivate = "deploy:activate"
	ScopeDeployRollback = "deploy:rollback"
)

// PCAP 管理权限
const (
	ScopePcapRead     = "pcap:read"
	ScopePcapDownload = "pcap:download"
	ScopePcapCut      = "pcap:cut"
)

// 图查询权限
const (
	ScopeGraphRead = "graph:read"
)

// 资产管理权限
const (
	ScopeAssetRead     = "asset:read"
	ScopeAssetDiscover = "asset:discover"
)

// 态势大屏权限
const (
	ScopeScreenView = "screen:view"
)

// 管理员权限
const (
	ScopeAdminAll         = "admin:*"
	ScopeAdminCrossTenant = "admin:cross_tenant"
)

// 探针权限
const (
	ScopeProbeIngest  = "probe:ingest"
	ScopeProbeMetrics = "probe:metrics"
	ScopeProbeWrite   = "probe:write"
)

// Token 管理权限
const (
	ScopeTokenRead  = "token:read"
	ScopeTokenWrite = "token:write"
)

// DLQ 运维权限
const (
	ScopeDLQReplay = "dlq:replay"
)

// 通配符
const (
	ScopeAll = "*"
)

// =============================================================================
// 默认权限集合
// =============================================================================

// DefaultProbeScopes 默认探针权限（生产环境推荐）
var DefaultProbeScopes = []string{
	ScopeProbeIngest,
	ScopeProbeMetrics,
}

// ProbeFullScopes 探针完全访问权限（包含 PCAP 读取）
var ProbeFullScopes = []string{
	ScopeProbeIngest,
	ScopeProbeMetrics,
	ScopePcapRead,
}

// ProbeMinimalScopes 探针最小权限（仅上报）
var ProbeMinimalScopes = []string{
	ScopeProbeIngest,
}

// AllValidScopes 所有有效的 scopes（用户 + 探针）
var AllValidScopes = []string{
	// 探针权限
	ScopeProbeIngest,
	ScopeProbeMetrics,
	ScopeProbeWrite,

	// 用户权限
	ScopeUserRead,
	ScopeUserWrite,
	ScopeUserDelete,

	ScopeAlertRead,
	ScopeAlertWrite,
	ScopeAlertExport,

	ScopeRuleRead,
	ScopeRuleWrite,
	ScopeRuleDelete,

	ScopeDeployRead,
	ScopeDeployCreate,
	ScopeDeployActivate,
	ScopeDeployRollback,

	ScopePcapRead,
	ScopePcapDownload,
	ScopePcapCut,

	ScopeGraphRead,
	ScopeAssetRead,
	ScopeAssetDiscover,
	ScopeScreenView,

	ScopeAdminAll,
	ScopeAdminCrossTenant,

	ScopeTokenRead,
	ScopeTokenWrite,

	ScopeDLQReplay,

	// 通配符
	ScopeAll,
}

// =============================================================================
// Scope 信息
// =============================================================================

// ScopeInfo Scope 信息描述
type ScopeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"` // probe, user, admin
}

// GetAllScopeInfos 获取所有 Scope 的详细信息
func GetAllScopeInfos() []ScopeInfo {
	return []ScopeInfo{
		// 探针权限
		{Name: ScopeProbeIngest, Description: "Upload flow events and PCAP index", Category: "probe"},
		{Name: ScopeProbeMetrics, Description: "Report probe metrics", Category: "probe"},
		{Name: ScopeProbeWrite, Description: "Manage probe configuration, certificate rotation and upgrades", Category: "probe"},

		// 用户权限
		{Name: ScopeUserRead, Description: "Read users", Category: "user"},
		{Name: ScopeUserWrite, Description: "Create and update users", Category: "user"},
		{Name: ScopeUserDelete, Description: "Delete users", Category: "user"},

		{Name: ScopeAlertRead, Description: "Read alerts", Category: "alert"},
		{Name: ScopeAlertWrite, Description: "Update alert status and feedback", Category: "alert"},
		{Name: ScopeAlertExport, Description: "Export alerts", Category: "alert"},

		{Name: ScopeRuleRead, Description: "Read detection rules", Category: "rule"},
		{Name: ScopeRuleWrite, Description: "Create and update detection rules", Category: "rule"},
		{Name: ScopeRuleDelete, Description: "Delete detection rules", Category: "rule"},

		{Name: ScopeDeployRead, Description: "Read deployments", Category: "deploy"},
		{Name: ScopeDeployCreate, Description: "Create deployments", Category: "deploy"},
		{Name: ScopeDeployActivate, Description: "Activate deployments", Category: "deploy"},
		{Name: ScopeDeployRollback, Description: "Rollback deployments", Category: "deploy"},

		{Name: ScopePcapRead, Description: "Read PCAP files", Category: "pcap"},
		{Name: ScopePcapDownload, Description: "Download PCAP files", Category: "pcap"},
		{Name: ScopePcapCut, Description: "Cut PCAP files", Category: "pcap"},

		{Name: ScopeGraphRead, Description: "Query threat graph", Category: "graph"},
		{Name: ScopeAssetRead, Description: "Read asset inventory and topology", Category: "asset"},
		{Name: ScopeAssetDiscover, Description: "Register discovery credentials and run active asset discovery", Category: "asset"},
		{Name: ScopeScreenView, Description: "View readonly situational screen", Category: "screen"},

		{Name: ScopeAdminAll, Description: "Full admin access", Category: "admin"},
		{Name: ScopeAdminCrossTenant, Description: "Cross-tenant admin access", Category: "admin"},

		{Name: ScopeTokenRead, Description: "Read API tokens", Category: "admin"},
		{Name: ScopeTokenWrite, Description: "Manage API tokens", Category: "admin"},

		{Name: ScopeDLQReplay, Description: "Approve and replay DLQ fallback records", Category: "admin"},

		{Name: ScopeAll, Description: "Full access (all scopes)", Category: "admin"},
	}
}

// GetProbeScopes 获取探针相关 scopes
func GetProbeScopes() []ScopeInfo {
	allScopes := GetAllScopeInfos()
	probeScopes := make([]ScopeInfo, 0)
	for _, info := range allScopes {
		if info.Category == "probe" {
			probeScopes = append(probeScopes, info)
		}
	}
	return probeScopes
}

// =============================================================================
// Scope 验证函数
// =============================================================================

// IsValidScope 检查 scope 是否有效
func IsValidScope(scope string) bool {
	if scope == ScopeAll {
		return true
	}
	for _, s := range AllValidScopes {
		if s == scope {
			return true
		}
	}
	return false
}

// IsProbeScope 检查是否是探针专用 scope
func IsProbeScope(scope string) bool {
	probeScopes := map[string]bool{
		ScopeProbeIngest:  true,
		ScopeProbeMetrics: true,
		ScopeProbeWrite:   true,
	}
	return probeScopes[scope]
}

// ValidateScopes 验证 scopes 列表是否合法
func ValidateScopes(scopes []string) (valid []string, invalid []string) {
	valid = make([]string, 0)
	invalid = make([]string, 0)

	for _, scope := range scopes {
		if IsValidScope(scope) {
			valid = append(valid, scope)
		} else {
			invalid = append(invalid, scope)
		}
	}

	return valid, invalid
}

// ScopesToList 将 scopes 切片转换为列表（兼容旧代码）
func ScopesToList(scopes StringSlice) []string {
	return []string(scopes)
}

// ListToScopes 将列表转换为 scopes 切片（兼容旧代码）
func ListToScopes(list []string) StringSlice {
	return StringSlice(list)
}
