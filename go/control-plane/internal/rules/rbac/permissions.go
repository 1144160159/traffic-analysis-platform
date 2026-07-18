////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/rbac/permissions.go
// 规则服务权限定义（确认版本 - 无需修改）
////////////////////////////////////////////////////////////////////////////////

package rbac

// =============================================================================
// 权限常量
// =============================================================================

// Permission 权限类型
type Permission string

// 规则管理权限
const (
	// 规则读取
	PermRuleRead       Permission = "rule:read"
	PermissionRuleRead Permission = "rule:read" // 别名，确保兼容性
	// 规则写入（创建、更新）
	PermRuleWrite       Permission = "rule:write"
	PermissionRuleWrite Permission = "rule:write" // 别名
	// 规则删除
	PermRuleDelete Permission = "rule:delete"
	// 规则启用/禁用
	PermRuleEnable Permission = "rule:enable"
	// 规则导出
	PermRuleExport Permission = "rule:export"
	// 规则导入
	PermRuleImport Permission = "rule:import"
)

// 部署管理权限
const (
	// 部署读取
	PermDeployRead       Permission = "deploy:read"
	PermissionDeployRead Permission = "deploy:read" // 别名
	// 部署创建
	PermDeployCreate Permission = "deploy:create"
	// 灰度发布
	PermDeployGray Permission = "deploy:gray"
	// 部署激活
	PermDeployActivate Permission = "deploy:activate"
	// 部署回滚
	PermDeployRollback Permission = "deploy:rollback"
	// 部署审批。审批与发起/执行分离，避免请求人自批。
	PermDeployApprove Permission = "deploy:approve"
	// 部署取消
	PermDeployCancel Permission = "deploy:cancel"
	// 部署写入（别名）
	PermissionDeployWrite Permission = "deploy:write"
)

// 管理权限
const (
	// 管理员读取
	PermAdminRead Permission = "admin:read"
	// 管理员写入
	PermAdminWrite       Permission = "admin:write"
	PermissionAdminWrite Permission = "admin:write" // 别名
	// 审计读取
	PermAuditRead Permission = "audit:read"
)

// 通配符权限
const (
	// 所有权限
	PermAll Permission = "*"
	// 所有规则权限
	PermRuleAll Permission = "rule:*"
	// 所有部署权限
	PermDeployAll Permission = "deploy:*"
	// 所有模型权限
	PermModelAll Permission = "model:*"
)

// =============================================================================
// 模型权限常量
// =============================================================================

const (
	PermModelRead     Permission = "model:read"
	PermModelCreate   Permission = "model:create"
	PermModelWrite    Permission = "model:write"
	PermModelDelete   Permission = "model:delete"
	PermModelActivate Permission = "model:activate"
	PermModelExport   Permission = "model:export"
	PermModelImport   Permission = "model:import"
)

// =============================================================================
// 权限分组
// =============================================================================

// AllRulePermissions 所有规则相关权限
var AllRulePermissions = []Permission{
	PermRuleRead,
	PermRuleWrite,
	PermRuleDelete,
	PermRuleEnable,
	PermRuleExport,
	PermRuleImport,
}

// AllDeployPermissions 所有部署相关权限
var AllDeployPermissions = []Permission{
	PermDeployRead,
	PermDeployCreate,
	PermDeployGray,
	PermDeployActivate,
	PermDeployRollback,
	PermDeployApprove,
	PermDeployCancel,
}

// AllModelPermissions 所有模型相关权限
var AllModelPermissions = []Permission{
	PermModelRead,
	PermModelCreate,
	PermModelWrite,
	PermModelDelete,
	PermModelActivate,
	PermModelExport,
	PermModelImport,
}

// AllAdminPermissions 所有管理权限
var AllAdminPermissions = []Permission{
	PermAdminRead,
	PermAdminWrite,
	PermAuditRead,
}

// AllPermissions 所有权限
var AllPermissions = append(append(append(AllRulePermissions, AllDeployPermissions...), AllModelPermissions...), AllAdminPermissions...)

// =============================================================================
// 角色定义
// =============================================================================

// Role 角色类型
type Role string

const (
	RoleAdmin    Role = "admin"    // 管理员
	RoleOperator Role = "operator" // 运维
	RoleAnalyst  Role = "analyst"  // 分析师
	RoleViewer   Role = "viewer"   // 只读用户
	RoleProbe    Role = "probe"    // 探针（机器账户）
)

// RolePermissions 角色权限映射
var RolePermissions = map[Role][]Permission{
	RoleAdmin: {
		PermAll, // 管理员拥有所有权限
	},
	RoleOperator: {
		// 规则权限
		PermRuleRead,
		PermRuleWrite,
		PermRuleEnable,
		PermRuleExport,
		// 部署权限
		PermDeployRead,
		PermDeployCreate,
		PermDeployGray,
		PermDeployActivate,
		PermDeployRollback,
		PermDeployApprove,
		PermDeployCancel,
		PermModelRead,
		PermModelCreate,
		PermModelWrite,
		PermModelActivate,
	},
	RoleAnalyst: {
		// 规则权限
		PermRuleRead,
		PermRuleWrite,
		PermRuleEnable,
		// 部署权限
		PermDeployRead,
		PermDeployCreate,
		PermModelRead,
	},
	RoleViewer: {
		// 只读权限
		PermRuleRead,
		PermDeployRead,
		PermModelRead,
	},
	RoleProbe: {
		// 探针只能读取规则
		PermRuleRead,
	},
}

// =============================================================================
// 权限检查辅助函数
// =============================================================================

// GetRolePermissions 获取角色的权限列表
func GetRolePermissions(role Role) []Permission {
	if perms, ok := RolePermissions[role]; ok {
		return perms
	}
	return nil
}

// GetPermissionsFromRoles 从多个角色获取权限列表
func GetPermissionsFromRoles(roles []string) []Permission {
	permSet := make(map[Permission]bool)

	for _, roleStr := range roles {
		role := Role(roleStr)
		if perms, ok := RolePermissions[role]; ok {
			for _, perm := range perms {
				permSet[perm] = true
			}
		}
	}

	result := make([]Permission, 0, len(permSet))
	for perm := range permSet {
		result = append(result, perm)
	}

	return result
}

// HasPermission 检查权限列表中是否包含指定权限
func HasPermission(permissions []Permission, required Permission) bool {
	for _, perm := range permissions {
		if perm == PermAll {
			return true
		}
		if perm == required {
			return true
		}
		// 检查通配符权限
		if isWildcardMatch(perm, required) {
			return true
		}
	}
	return false
}

// HasAnyPermission 检查是否有任一权限
func HasAnyPermission(permissions []Permission, required ...Permission) bool {
	for _, req := range required {
		if HasPermission(permissions, req) {
			return true
		}
	}
	return false
}

// HasAllPermissions 检查是否有所有权限
func HasAllPermissions(permissions []Permission, required ...Permission) bool {
	for _, req := range required {
		if !HasPermission(permissions, req) {
			return false
		}
	}
	return true
}

// isWildcardMatch 检查通配符权限匹配
func isWildcardMatch(perm, required Permission) bool {
	permStr := string(perm)
	reqStr := string(required)

	// 检查前缀通配符，如 "rule:*" 匹配 "rule:read"
	if len(permStr) > 1 && permStr[len(permStr)-1] == '*' {
		prefix := permStr[:len(permStr)-1]
		return len(reqStr) >= len(prefix) && reqStr[:len(prefix)] == prefix
	}

	return false
}

// =============================================================================
// 权限信息
// =============================================================================

// PermissionInfo 权限信息
type PermissionInfo struct {
	Name        Permission `json:"name"`
	Description string     `json:"description"`
	Category    string     `json:"category"`
}

// GetAllPermissionInfos 获取所有权限信息
func GetAllPermissionInfos() []PermissionInfo {
	return []PermissionInfo{
		// 规则权限
		{PermRuleRead, "Read rules", "rule"},
		{PermRuleWrite, "Create and update rules", "rule"},
		{PermRuleDelete, "Delete rules", "rule"},
		{PermRuleEnable, "Enable or disable rules", "rule"},
		{PermRuleExport, "Export rules", "rule"},
		{PermRuleImport, "Import rules", "rule"},

		// 部署权限
		{PermDeployRead, "Read deployments", "deploy"},
		{PermDeployCreate, "Create deployments", "deploy"},
		{PermDeployGray, "Start gray deployment", "deploy"},
		{PermDeployActivate, "Activate deployment", "deploy"},
		{PermDeployRollback, "Rollback deployment", "deploy"},
		{PermDeployCancel, "Cancel deployment", "deploy"},

		// 模型权限
		{PermModelRead, "Read models", "model"},
		{PermModelCreate, "Create models", "model"},
		{PermModelWrite, "Update models", "model"},
		{PermModelDelete, "Delete models", "model"},
		{PermModelActivate, "Activate model version", "model"},

		// 管理权限
		{PermAdminRead, "Read admin data", "admin"},
		{PermAdminWrite, "Modify admin settings", "admin"},
		{PermAuditRead, "Read audit logs", "admin"},
	}
}

// =============================================================================
// 资源级权限
// =============================================================================

// ResourcePermission 资源级权限
type ResourcePermission struct {
	Permission Permission `json:"permission"`
	TenantID   string     `json:"tenant_id"`
	ResourceID string     `json:"resource_id,omitempty"`
}

// CanAccessResource 检查是否可以访问资源
func CanAccessResource(userTenantID string, resourceTenantID string, permissions []Permission, required Permission) bool {
	// 首先检查权限
	if !HasPermission(permissions, required) {
		return false
	}

	// 检查租户隔离
	// 管理员可以跨租户
	if HasPermission(permissions, PermAdminWrite) {
		return true
	}

	// 普通用户只能访问自己租户的资源
	return userTenantID == resourceTenantID
}
