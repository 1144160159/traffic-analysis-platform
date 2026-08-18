////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/model/scopes.go
// 新增文件 - 修复 #A19：统一权限定义
// 说明：将所有 Scope 常量集中到此文件，避免重复定义
////////////////////////////////////////////////////////////////////////////////

package model

import "strings"

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
	ScopeAlertAll    = "alert:*"
)

// 仪表盘任务权限
const (
	ScopeDashboardWrite = "dashboard:write"
	ScopeDashboardAll   = "dashboard:*"
)

// 专题与战役权限
const (
	ScopeTopicRead      = "topic:read"
	ScopeTopicWrite     = "topic:write"
	ScopeTopicExport    = "topic:export"
	ScopeTopicAll       = "topic:*"
	ScopeCampaignRead   = "campaign:read"
	ScopeCampaignWrite  = "campaign:write"
	ScopeCampaignReport = "campaign:report"
)

// SOAR 剧本治理权限
const (
	ScopePlaybookRead    = "playbook:read"
	ScopePlaybookWrite   = "playbook:write"
	ScopePlaybookDrill   = "playbook:drill"
	ScopePlaybookApprove = "playbook:approve"
	ScopePlaybookExport  = "playbook:export"
	ScopePlaybookExecute = "playbook:execute"
)

// 规则管理权限
const (
	ScopeRuleRead   = "rule:read"
	ScopeRuleWrite  = "rule:write"
	ScopeRuleDelete = "rule:delete"
	ScopeRuleEnable = "rule:enable"
	ScopeRuleAll    = "rule:*"
)

// 模型治理权限
const (
	ScopeModelRead     = "model:read"
	ScopeModelCreate   = "model:create"
	ScopeModelWrite    = "model:write"
	ScopeModelActivate = "model:activate"
	ScopeModelAll      = "model:*"
)

// 部署管理权限
const (
	ScopeDeployRead     = "deploy:read"
	ScopeDeployCreate   = "deploy:create"
	ScopeDeployGray     = "deploy:gray"
	ScopeDeployApprove  = "deploy:approve"
	ScopeDeployActivate = "deploy:activate"
	ScopeDeployRollback = "deploy:rollback"
	ScopeDeployAll      = "deploy:*"
)

// PCAP 管理权限
const (
	ScopePcapRead     = "pcap:read"
	ScopePcapDownload = "pcap:download"
	ScopePcapCut      = "pcap:cut"
	ScopePcapWrite    = "pcap:write"
	ScopePcapAll      = "pcap:*"
)

// 图查询权限
const (
	ScopeGraphRead = "graph:read"
)

// 资产管理权限
const (
	ScopeAssetRead     = "asset:read"
	ScopeAssetDiscover = "asset:discover"
	ScopeAssetExport   = "asset:export"
	ScopeAssetGovern   = "asset:govern"
	ScopeAssetAll      = "asset:*"
)

// 态势大屏权限
const (
	ScopeScreenView = "screen:view"
)

// 分析调度权限(深度测试新增:analysis API 面此前无逐操作判定)
const (
	ScopeAnalysisRead  = "analysis:read"
	ScopeAnalysisWrite = "analysis:write"
)

// 管理员权限
const (
	ScopeAdminRead        = "admin:read"
	ScopeAdminWrite       = "admin:write"
	ScopeAdminAll         = "admin:*"
	ScopeAdminCrossTenant = "admin:cross_tenant"
)

// 探针权限
const (
	ScopeProbeIngest  = "probe:ingest"
	ScopeProbeMetrics = "probe:metrics"
	ScopeProbeRead    = "probe:read"
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
	ScopeDLQAll    = "dlq:*"
)

// 数据质量权限
const (
	ScopeDataQualityRead  = "data-quality:read"
	ScopeDataQualityWrite = "data-quality:write"
	ScopeDataQualityAll   = "data-quality:*"
)

// 合规与审计权限
const (
	ScopeComplianceRead      = "compliance:read"
	ScopeComplianceWrite     = "compliance:write"
	ScopeComplianceExport    = "compliance:export"
	ScopeComplianceFinalize  = "compliance:finalize"
	ScopeComplianceRemediate = "compliance:remediate"
	ScopeAuditRead           = "audit:read"
	ScopeAuditWrite          = "audit:write"
	ScopeAuditExport         = "audit:export"
	ScopeAuditAll            = "audit:*"
	ScopeEvidenceRead        = "evidence:read"
	ScopeFeedbackWrite       = "feedback:write"
	ScopeForensicsWrite      = "forensics:write"
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
	ScopeProbeRead,
	ScopeProbeWrite,

	// 用户权限
	ScopeUserRead,
	ScopeUserWrite,
	ScopeUserDelete,

	ScopeAlertRead,
	ScopeAlertWrite,
	ScopeAlertExport,
	ScopeAlertAll,
	ScopeDashboardWrite,
	ScopeDashboardAll,
	ScopeTopicRead,
	ScopeTopicWrite,
	ScopeTopicExport,
	ScopeTopicAll,
	ScopeCampaignRead,
	ScopeCampaignWrite,
	ScopeCampaignReport,
	ScopePlaybookRead,
	ScopePlaybookWrite,
	ScopePlaybookDrill,
	ScopePlaybookApprove,
	ScopePlaybookExport,
	ScopePlaybookExecute,

	ScopeRuleRead,
	ScopeRuleWrite,
	ScopeRuleDelete,
	ScopeRuleEnable,
	ScopeRuleAll,
	ScopeModelRead,
	ScopeModelCreate,
	ScopeModelWrite,
	ScopeModelActivate,
	ScopeModelAll,

	ScopeDeployRead,
	ScopeDeployCreate,
	ScopeDeployGray,
	ScopeDeployApprove,
	ScopeDeployActivate,
	ScopeDeployRollback,
	ScopeDeployAll,

	ScopePcapRead,
	ScopePcapDownload,
	ScopePcapCut,
	ScopePcapWrite,
	ScopePcapAll,

	ScopeGraphRead,
	ScopeAssetRead,
	ScopeAssetDiscover,
	ScopeAssetExport,
	ScopeAssetGovern,
	ScopeAssetAll,
	ScopeScreenView,
	ScopeAnalysisRead,
	ScopeAnalysisWrite,

	ScopeAdminRead,
	ScopeAdminWrite,
	ScopeAdminAll,
	ScopeAdminCrossTenant,

	ScopeTokenRead,
	ScopeTokenWrite,

	ScopeDLQReplay,
	ScopeDLQAll,
	ScopeDataQualityRead,
	ScopeDataQualityWrite,
	ScopeDataQualityAll,
	ScopeComplianceRead,
	ScopeComplianceWrite,
	ScopeComplianceExport,
	ScopeComplianceFinalize,
	ScopeComplianceRemediate,
	ScopeAuditRead,
	ScopeAuditWrite,
	ScopeAuditExport,
	ScopeAuditAll,
	ScopeEvidenceRead,
	ScopeFeedbackWrite,
	ScopeForensicsWrite,

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
		{Name: ScopeProbeRead, Description: "Read probe inventory, health and operations", Category: "probe"},
		{Name: ScopeProbeWrite, Description: "Manage probe configuration, certificate rotation and upgrades", Category: "probe"},

		// 用户权限
		{Name: ScopeUserRead, Description: "Read users", Category: "user"},
		{Name: ScopeUserWrite, Description: "Create and update users", Category: "user"},
		{Name: ScopeUserDelete, Description: "Delete users", Category: "user"},

		{Name: ScopeAlertRead, Description: "Read alerts", Category: "alert"},
		{Name: ScopeAlertWrite, Description: "Update alert status and feedback", Category: "alert"},
		{Name: ScopeAlertExport, Description: "Export alerts", Category: "alert"},
		{Name: ScopeAlertAll, Description: "All alert-domain permissions", Category: "alert"},
		{Name: ScopeDashboardWrite, Description: "Create audited dashboard follow-up tasks", Category: "dashboard"},
		{Name: ScopeDashboardAll, Description: "All dashboard-domain permissions", Category: "dashboard"},
		{Name: ScopeTopicRead, Description: "Read tenant topic snapshots", Category: "topic"},
		{Name: ScopeTopicWrite, Description: "Manage tenant topic scope, views, subscriptions and actions", Category: "topic"},
		{Name: ScopeTopicExport, Description: "Export tenant topic reports and evidence packages", Category: "topic"},
		{Name: ScopeTopicAll, Description: "All topic-domain permissions", Category: "topic"},
		{Name: ScopeCampaignRead, Description: "Read tenant campaigns and memberships", Category: "campaign"},
		{Name: ScopeCampaignWrite, Description: "Manage tenant campaign lifecycle and memberships", Category: "campaign"},
		{Name: ScopeCampaignReport, Description: "Generate tenant campaign reports", Category: "campaign"},
		{Name: ScopePlaybookRead, Description: "Read tenant SOAR playbooks and evidence", Category: "playbook"},
		{Name: ScopePlaybookWrite, Description: "Create, edit, submit, enable and disable SOAR playbooks", Category: "playbook"},
		{Name: ScopePlaybookDrill, Description: "Run and roll back simulated SOAR drills", Category: "playbook"},
		{Name: ScopePlaybookApprove, Description: "Independently approve or reject SOAR playbooks", Category: "playbook"},
		{Name: ScopePlaybookExport, Description: "Export tenant SOAR evidence packages", Category: "playbook"},
		{Name: ScopePlaybookExecute, Description: "Execute approved tenant SOAR playbooks", Category: "playbook"},

		{Name: ScopeRuleRead, Description: "Read detection rules", Category: "rule"},
		{Name: ScopeRuleWrite, Description: "Create and update detection rules", Category: "rule"},
		{Name: ScopeRuleDelete, Description: "Delete detection rules", Category: "rule"},
		{Name: ScopeRuleEnable, Description: "Enable or disable approved detection rules", Category: "rule"},
		{Name: ScopeRuleAll, Description: "All rule-domain permissions", Category: "rule"},
		{Name: ScopeModelRead, Description: "Read tenant model registry and evaluation state", Category: "model"},
		{Name: ScopeModelCreate, Description: "Register tenant model candidates", Category: "model"},
		{Name: ScopeModelWrite, Description: "Manage tenant model workflows and metadata", Category: "model"},
		{Name: ScopeModelActivate, Description: "Activate or roll back approved tenant models", Category: "model"},
		{Name: ScopeModelAll, Description: "All model-domain permissions", Category: "model"},

		{Name: ScopeDeployRead, Description: "Read deployments", Category: "deploy"},
		{Name: ScopeDeployCreate, Description: "Create deployments", Category: "deploy"},
		{Name: ScopeDeployGray, Description: "Start deployment gray rollout", Category: "deploy"},
		{Name: ScopeDeployApprove, Description: "Independently approve deployment workflows", Category: "deploy"},
		{Name: ScopeDeployActivate, Description: "Activate deployments", Category: "deploy"},
		{Name: ScopeDeployRollback, Description: "Rollback deployments", Category: "deploy"},
		{Name: ScopeDeployAll, Description: "All deployment-domain permissions", Category: "deploy"},

		{Name: ScopePcapRead, Description: "Read PCAP files", Category: "pcap"},
		{Name: ScopePcapDownload, Description: "Download PCAP files", Category: "pcap"},
		{Name: ScopePcapCut, Description: "Cut PCAP files", Category: "pcap"},
		{Name: ScopePcapWrite, Description: "Create, update and cancel PCAP jobs", Category: "pcap"},
		{Name: ScopePcapAll, Description: "All PCAP-domain permissions", Category: "pcap"},

		{Name: ScopeGraphRead, Description: "Query threat graph", Category: "graph"},
		{Name: ScopeAssetRead, Description: "Read asset inventory and topology", Category: "asset"},
		{Name: ScopeAssetDiscover, Description: "Register discovery credentials and run active asset discovery", Category: "asset"},
		{Name: ScopeAssetExport, Description: "Export tenant asset inventory artifacts", Category: "asset"},
		{Name: ScopeAssetGovern, Description: "Create, approve and execute tenant asset governance work orders", Category: "asset"},
		{Name: ScopeAssetAll, Description: "All asset-domain permissions", Category: "asset"},
		{Name: ScopeScreenView, Description: "View readonly situational screen", Category: "screen"},
		{Name: ScopeAnalysisRead, Description: "Read analysis schedules, runs, reports and resources", Category: "analysis"},
		{Name: ScopeAnalysisWrite, Description: "Create and mutate analysis triggers, plans, schedules and task definitions", Category: "analysis"},

		{Name: ScopeAdminRead, Description: "Read tenant administration settings", Category: "admin"},
		{Name: ScopeAdminWrite, Description: "Modify tenant administration settings", Category: "admin"},
		{Name: ScopeAdminAll, Description: "Full admin access", Category: "admin"},
		{Name: ScopeAdminCrossTenant, Description: "Cross-tenant admin access", Category: "admin"},

		{Name: ScopeTokenRead, Description: "Read API tokens", Category: "admin"},
		{Name: ScopeTokenWrite, Description: "Manage API tokens", Category: "admin"},

		{Name: ScopeDLQReplay, Description: "Approve and replay DLQ fallback records", Category: "admin"},
		{Name: ScopeDLQAll, Description: "All DLQ-domain permissions", Category: "admin"},
		{Name: ScopeDataQualityRead, Description: "Read data quality health, evidence and reports", Category: "data-quality"},
		{Name: ScopeDataQualityWrite, Description: "Create audited data quality repair and export actions", Category: "data-quality"},
		{Name: ScopeDataQualityAll, Description: "All data-quality-domain permissions", Category: "data-quality"},
		{Name: ScopeComplianceRead, Description: "Read tenant compliance reports and gate evidence", Category: "compliance"},
		{Name: ScopeComplianceWrite, Description: "Generate tenant compliance reports", Category: "compliance"},
		{Name: ScopeComplianceExport, Description: "Export tenant compliance evidence packages", Category: "compliance"},
		{Name: ScopeComplianceFinalize, Description: "Finalize immutable compliance acceptance records", Category: "compliance"},
		{Name: ScopeComplianceRemediate, Description: "Create and manage compliance remediation tasks", Category: "compliance"},
		{Name: ScopeAuditRead, Description: "Read tenant audit trails", Category: "audit"},
		{Name: ScopeAuditWrite, Description: "Create tenant audit reviews, saved queries and integrity checks", Category: "audit"},
		{Name: ScopeAuditExport, Description: "Export tenant audit evidence", Category: "audit"},
		{Name: ScopeAuditAll, Description: "All audit-domain permissions", Category: "audit"},
		{Name: ScopeEvidenceRead, Description: "Read tenant evidence references and objects", Category: "evidence"},
		{Name: ScopeFeedbackWrite, Description: "Submit tenant detection feedback", Category: "feedback"},
		{Name: ScopeForensicsWrite, Description: "Create tenant forensics tasks", Category: "forensics"},

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
		ScopeProbeRead:    true,
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

// CanDelegateScopes enforces a privilege ceiling for token administration.
// A caller may only mint or assign scopes already covered by its own
// permissions. Global and domain wildcards retain their normal semantics.
func CanDelegateScopes(actorPermissions, requestedScopes []string) bool {
	for _, requested := range requestedScopes {
		covered := false
		for _, permission := range actorPermissions {
			if permission == ScopeAll || permission == requested {
				covered = true
				break
			}
			if strings.HasSuffix(permission, ":*") && strings.HasPrefix(requested, strings.TrimSuffix(permission, "*")) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// ScopesToList 将 scopes 切片转换为列表（兼容旧代码）
func ScopesToList(scopes StringSlice) []string {
	return []string(scopes)
}

// ListToScopes 将列表转换为 scopes 切片（兼容旧代码）
func ListToScopes(list []string) StringSlice {
	return StringSlice(list)
}
