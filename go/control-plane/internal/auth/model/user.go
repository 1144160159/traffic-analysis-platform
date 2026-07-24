////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/model/user.go
// 修复版 v3：
// 1. 修复 #A2：添加 LastLoginAt 字段
// 2. 修复 #A19：移除重复的权限定义，统一使用 scopes.go
// 3. 保留所有原有功能（200+行完整代码）
////////////////////////////////////////////////////////////////////////////////

package model

import (
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// 实体模型定义
// =============================================================================

// User 用户模型
type User struct {
	UserID       uuid.UUID         `json:"user_id" db:"user_id"`
	TenantID     string            `json:"tenant_id" db:"tenant_id"`
	Username     string            `json:"username" db:"username"`
	Email        string            `json:"email" db:"email"`
	PasswordHash string            `json:"-" db:"password_hash"`
	Status       string            `json:"status" db:"status"`
	ExternalID   string            `json:"external_id,omitempty" db:"external_id"`     // OIDC subject
	LastLoginAt  *time.Time        `json:"last_login_at,omitempty" db:"last_login_at"` // 修复 #A2：新增字段
	Metadata     map[string]string `json:"metadata,omitempty" db:"-"`
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" db:"updated_at"`
}

// UserStatus 用户状态常量
const (
	UserStatusActive   = "active"
	UserStatusInactive = "inactive"
	UserStatusLocked   = "locked"
	UserStatusPending  = "pending"
)

// Role 角色模型
type Role struct {
	RoleID      uuid.UUID              `json:"role_id" db:"role_id"`
	TenantID    string                 `json:"tenant_id" db:"tenant_id"`
	Name        string                 `json:"name" db:"name"`
	Description string                 `json:"description,omitempty" db:"description"`
	Permissions map[string]interface{} `json:"permissions" db:"-"`
	IsSystem    bool                   `json:"is_system" db:"is_system"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
}

// UserRole 用户角色关联
type UserRole struct {
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	RoleID    uuid.UUID `json:"role_id" db:"role_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// APIKeyRecord API 密钥记录 (DB 持久化模型)
type APIKeyRecord struct {
	TokenID     uuid.UUID       `json:"token_id" db:"token_id"`
	TenantID    string          `json:"tenant_id" db:"tenant_id"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description,omitempty" db:"description"`
	TokenHash   string          `json:"-" db:"token_hash"`
	TokenPrefix string          `json:"token_prefix,omitempty" db:"token_prefix"` // 用于识别 Token
	Scopes      map[string]bool `json:"scopes" db:"-"`
	Status      string          `json:"status" db:"status"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty" db:"expires_at"`
	LastUsedAt  *time.Time      `json:"last_used_at,omitempty" db:"last_used_at"`
	CreatedBy   uuid.UUID       `json:"created_by" db:"created_by"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
	ProbeID     string          `json:"probe_id,omitempty" db:"probe_id"` // 关联探针 ID
}

// APIKeyStatus 常量 (与 token.go TokenStatus 互补)
const (
	APIKeyStatusActive  = "active"
	APIKeyStatusRevoked = "revoked"
	APIKeyStatusExpired = "expired"
)

// Session 会话模型
type Session struct {
	SessionID    string    `json:"session_id"`
	UserID       uuid.UUID `json:"user_id"`
	TenantID     string    `json:"tenant_id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	Roles        []string  `json:"roles"`
	Permissions  []string  `json:"permissions"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	ExpiresAt    time.Time `json:"expires_at"`
	RefreshToken string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// =============================================================================
// 默认角色权限映射（修复 #A19：使用统一的 Scope 定义）
// =============================================================================

// DefaultRoleScopes 默认角色权限映射（使用统一的 Scope）
var DefaultRoleScopes = map[string][]string{
	"admin": {
		ScopeAlertRead, ScopeAlertWrite, ScopeAlertExport,
		ScopePlaybookRead, ScopePlaybookWrite, ScopePlaybookDrill, ScopePlaybookApprove, ScopePlaybookExport,
		ScopeRuleRead, ScopeRuleWrite, ScopeRuleDelete,
		ScopePcapRead, ScopePcapDownload, ScopePcapCut,
		ScopeGraphRead,
		ScopeAssetRead, ScopeAssetDiscover,
		ScopeScreenView,
		ScopeAdminAll,
		ScopeTokenRead, ScopeTokenWrite,
		ScopeDeployRead, ScopeDeployCreate, ScopeDeployActivate, ScopeDeployRollback,
		ScopeDataQualityRead, ScopeDataQualityWrite,
		ScopeComplianceRead, ScopeComplianceWrite, ScopeComplianceExport, ScopeComplianceFinalize, ScopeComplianceRemediate,
		ScopeAuditRead, ScopeAuditWrite, ScopeAuditExport,
		ScopeUserRead, ScopeUserWrite, ScopeUserDelete,
		ScopeAll,
	},
	"analyst": {
		ScopeAlertRead, ScopeAlertWrite, ScopeAlertExport,
		ScopePlaybookRead, ScopePlaybookWrite, ScopePlaybookDrill, ScopePlaybookExport,
		ScopeRuleRead,
		ScopePcapRead, ScopePcapDownload,
		ScopeGraphRead,
		ScopeAssetRead,
		ScopeScreenView,
		ScopeDataQualityRead,
		ScopeComplianceRead,
		ScopeAuditRead, ScopeAuditWrite, ScopeAuditExport,
	},
	"viewer": {
		ScopeAlertRead,
		ScopePlaybookRead,
		ScopeRuleRead,
		ScopeGraphRead,
		ScopeAssetRead,
		ScopeScreenView,
		ScopeDataQualityRead,
		ScopeComplianceRead,
		ScopeAuditRead,
	},
	"operator": {
		ScopeAlertRead, ScopeAlertWrite,
		ScopePlaybookRead, ScopePlaybookWrite, ScopePlaybookDrill,
		ScopeRuleRead,
		ScopePcapRead,
		ScopeGraphRead,
		ScopeAssetRead, ScopeAssetDiscover,
		ScopeScreenView,
		ScopeDeployRead,
		ScopeDataQualityRead, ScopeDataQualityWrite,
		ScopeComplianceRead, ScopeComplianceWrite, ScopeComplianceExport, ScopeComplianceRemediate,
		ScopeAuditRead,
	},
}

// =============================================================================
// 辅助方法
// =============================================================================

// IsActive 检查用户是否活跃
func (u *User) IsActive() bool {
	return u.Status == UserStatusActive
}

// IsTokenActive 检查 Token 是否活跃
func (t *APIToken) IsTokenActive() bool {
	if t.Status != TokenStatusActive {
		return false
	}
	if t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now()) {
		return false
	}
	return true
}

// IsSessionValid 检查会话是否有效
func (s *Session) IsSessionValid() bool {
	return time.Now().Before(s.ExpiresAt)
}

// GetScopesForRoles 根据角色获取权限 Scopes
func GetScopesForRoles(roles []string) []string {
	scopeSet := make(map[string]bool)

	for _, role := range roles {
		if scopes, ok := DefaultRoleScopes[role]; ok {
			for _, scope := range scopes {
				scopeSet[scope] = true
			}
		}
	}

	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}

	return scopes
}

// HasScope 检查是否拥有指定权限
func HasScope(scopes []string, requiredScope string) bool {
	for _, scope := range scopes {
		// 精确匹配
		if scope == requiredScope {
			return true
		}
		// 通配符匹配
		if scope == ScopeAll {
			return true
		}
		// 前缀匹配（如 admin:* 匹配 admin:read）
		if len(scope) > 1 && scope[len(scope)-1] == '*' {
			prefix := scope[:len(scope)-1]
			if len(requiredScope) >= len(prefix) && requiredScope[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}

// HasAnyScope 检查是否有任一权限
func HasAnyScope(scopes []string, requiredScopes ...string) bool {
	for _, required := range requiredScopes {
		if HasScope(scopes, required) {
			return true
		}
	}
	return false
}

// HasAllScopes 检查是否有所有权限
func HasAllScopes(scopes []string, requiredScopes ...string) bool {
	for _, required := range requiredScopes {
		if !HasScope(scopes, required) {
			return false
		}
	}
	return true
}

// ScopesMapToList 将 scopes map 转换为列表
func ScopesMapToList(scopes map[string]bool) []string {
	result := make([]string, 0, len(scopes))
	for scope, enabled := range scopes {
		if enabled {
			result = append(result, scope)
		}
	}
	return result
}

// ListToScopesMap 将 scopes 列表转换为 map
func ListToScopesMap(list []string) map[string]bool {
	result := make(map[string]bool)
	for _, scope := range list {
		result[scope] = true
	}
	return result
}
