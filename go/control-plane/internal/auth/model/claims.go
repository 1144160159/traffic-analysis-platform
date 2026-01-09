////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/model/claims.go
// 修复版：修复 #22 - 权限通配符匹配过于宽松
// 修复内容：严格要求通配符必须在冒号后（admin:* 而非 ad*）
////////////////////////////////////////////////////////////////////////////////

package model

import (
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenType Token 类型
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// Claims JWT Claims 结构
type Claims struct {
	jwt.RegisteredClaims

	// 用户信息
	UserID   uuid.UUID `json:"user_id"`
	TenantID string    `json:"tenant_id"`
	Username string    `json:"username"`
	Email    string    `json:"email,omitempty"`

	// 权限信息
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"permissions,omitempty"`

	// Token 类型
	TokenType TokenType `json:"token_type"`

	// 会话信息
	SessionID string `json:"session_id,omitempty"`

	// 扩展信息（可选）
	ProbeID  string            `json:"probe_id,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// =============================================================================
// 实现 httpx.Claims 接口
// =============================================================================

// GetUserID 获取用户 ID
func (c *Claims) GetUserID() string {
	return c.UserID.String()
}

// GetTenantID 获取租户 ID
func (c *Claims) GetTenantID() string {
	return c.TenantID
}

// GetUsername 获取用户名
func (c *Claims) GetUsername() string {
	return c.Username
}

// GetRoles 获取角色列表
func (c *Claims) GetRoles() []string {
	if c.Roles == nil {
		return []string{}
	}
	return c.Roles
}

// GetPermissions 获取权限列表
func (c *Claims) GetPermissions() []string {
	if c.Permissions == nil {
		return []string{}
	}
	return c.Permissions
}

// =============================================================================
// 实现 httpx.ExtendedClaims 接口
// =============================================================================

// GetEmail 获取邮箱
func (c *Claims) GetEmail() string {
	return c.Email
}

// GetSessionID 获取会话 ID
func (c *Claims) GetSessionID() string {
	return c.SessionID
}

// HasRole 检查是否有指定角色
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasPermission 检查是否有指定权限（修复 #22：严格通配符匹配）
func (c *Claims) HasPermission(permission string) bool {
	for _, p := range c.Permissions {
		// 精确匹配
		if p == permission {
			return true
		}
		// 全局通配符
		if p == "*" {
			return true
		}
		// 修复 #22：严格要求通配符必须在冒号后
		// 只允许 "admin:*" 这样的格式，不允许 "ad*"
		if strings.HasSuffix(p, ":*") {
			prefix := p[:len(p)-1] // 包含冒号，如 "admin:"
			if strings.HasPrefix(permission, prefix) {
				return true
			}
		}
	}
	return false
}

// HasAnyRole 检查是否有任一角色
func (c *Claims) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if c.HasRole(role) {
			return true
		}
	}
	return false
}

// HasAllRoles 检查是否有所有角色
func (c *Claims) HasAllRoles(roles ...string) bool {
	for _, role := range roles {
		if !c.HasRole(role) {
			return false
		}
	}
	return true
}

// HasAnyPermission 检查是否有任一权限
func (c *Claims) HasAnyPermission(permissions ...string) bool {
	for _, perm := range permissions {
		if c.HasPermission(perm) {
			return true
		}
	}
	return false
}

// HasAllPermissions 检查是否有所有权限
func (c *Claims) HasAllPermissions(permissions ...string) bool {
	for _, perm := range permissions {
		if !c.HasPermission(perm) {
			return false
		}
	}
	return true
}

// IsAdmin 检查是否为管理员
func (c *Claims) IsAdmin() bool {
	return c.HasRole("admin") || c.HasPermission("admin:*")
}

// IsExpired 检查 Token 是否过期
func (c *Claims) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false
	}
	return c.ExpiresAt.Time.Before(jwt.NewNumericDate(jwt.TimeFunc()).Time)
}

// =============================================================================
// Claims 构建辅助函数
// =============================================================================

// NewAccessClaims 创建 Access Token Claims
func NewAccessClaims(userID uuid.UUID, tenantID, username, email string, roles, permissions []string, sessionID string) *Claims {
	return &Claims{
		UserID:      userID,
		TenantID:    tenantID,
		Username:    username,
		Email:       email,
		Roles:       roles,
		Permissions: permissions,
		TokenType:   TokenTypeAccess,
		SessionID:   sessionID,
	}
}

// NewRefreshClaims 创建 Refresh Token Claims
func NewRefreshClaims(userID uuid.UUID, tenantID, sessionID string) *Claims {
	return &Claims{
		UserID:    userID,
		TenantID:  tenantID,
		TokenType: TokenTypeRefresh,
		SessionID: sessionID,
	}
}

// Clone 克隆 Claims
func (c *Claims) Clone() *Claims {
	clone := &Claims{
		RegisteredClaims: c.RegisteredClaims,
		UserID:           c.UserID,
		TenantID:         c.TenantID,
		Username:         c.Username,
		Email:            c.Email,
		TokenType:        c.TokenType,
		SessionID:        c.SessionID,
		ProbeID:          c.ProbeID,
	}

	if c.Roles != nil {
		clone.Roles = make([]string, len(c.Roles))
		copy(clone.Roles, c.Roles)
	}

	if c.Permissions != nil {
		clone.Permissions = make([]string, len(c.Permissions))
		copy(clone.Permissions, c.Permissions)
	}

	if c.Metadata != nil {
		clone.Metadata = make(map[string]string)
		for k, v := range c.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// AddMetadata 添加元数据
func (c *Claims) AddMetadata(key, value string) {
	if c.Metadata == nil {
		c.Metadata = make(map[string]string)
	}
	c.Metadata[key] = value
}

// GetMetadata 获取元数据
func (c *Claims) GetMetadata(key string) string {
	if c.Metadata == nil {
		return ""
	}
	return c.Metadata[key]
}
