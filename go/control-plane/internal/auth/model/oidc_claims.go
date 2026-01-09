////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/model/oidc_claims.go
// 修复版：修复 #23 - ToInternalClaims 的 UserID 缺失
// 修复内容：从 Subject 解析 UserID，如果解析失败则生成新 UUID
////////////////////////////////////////////////////////////////////////////////

package model

import (
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// OIDCClaims OIDC ID Token Claims
type OIDCClaims struct {
	jwt.RegisteredClaims

	// 标准 OIDC Claims
	Subject           string `json:"sub"`
	Email             string `json:"email"`
	EmailVerified     bool   `json:"email_verified"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
	GivenName         string `json:"given_name"`
	FamilyName        string `json:"family_name"`
	Locale            string `json:"locale"`

	// Keycloak 特有 Claims
	RealmAccess    *RealmAccess              `json:"realm_access,omitempty"`
	ResourceAccess map[string]*ResourceRoles `json:"resource_access,omitempty"`
	Groups         []string                  `json:"groups,omitempty"`

	// 自定义 Claims（可通过 Keycloak Mapper 添加）
	TenantID   string            `json:"tenant_id,omitempty"`
	Department string            `json:"department,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// RealmAccess Keycloak Realm 级别权限
type RealmAccess struct {
	Roles []string `json:"roles"`
}

// ResourceRoles Keycloak 客户端级别权限
type ResourceRoles struct {
	Roles []string `json:"roles"`
}

// GetRoles 获取所有角色（合并 realm 和 resource 角色）
func (c *OIDCClaims) GetRoles(clientID string) []string {
	roleSet := make(map[string]bool)

	// 添加 realm 角色
	if c.RealmAccess != nil {
		for _, role := range c.RealmAccess.Roles {
			roleSet[role] = true
		}
	}

	// 添加指定客户端的角色
	if clientID != "" && c.ResourceAccess != nil {
		if clientRoles, ok := c.ResourceAccess[clientID]; ok {
			for _, role := range clientRoles.Roles {
				roleSet[role] = true
			}
		}
	}

	// 转换为切片
	roles := make([]string, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}

	return roles
}

// HasRealmRole 检查是否有 realm 角色
func (c *OIDCClaims) HasRealmRole(role string) bool {
	if c.RealmAccess == nil {
		return false
	}
	for _, r := range c.RealmAccess.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasClientRole 检查是否有客户端角色
func (c *OIDCClaims) HasClientRole(clientID, role string) bool {
	if c.ResourceAccess == nil {
		return false
	}
	clientRoles, ok := c.ResourceAccess[clientID]
	if !ok || clientRoles == nil {
		return false
	}
	for _, r := range clientRoles.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsInGroup 检查是否在指定组中
func (c *OIDCClaims) IsInGroup(group string) bool {
	for _, g := range c.Groups {
		if g == group {
			return true
		}
		// 支持层级组路径匹配
		if strings.HasPrefix(g, group+"/") || strings.HasSuffix(g, "/"+group) {
			return true
		}
	}
	return false
}

// GetTenantID 获取租户 ID（优先使用自定义 claim，否则使用默认值）
func (c *OIDCClaims) GetTenantID(defaultTenantID string) string {
	if c.TenantID != "" {
		return c.TenantID
	}
	// 尝试从 attributes 获取
	if c.Attributes != nil {
		if tid, ok := c.Attributes["tenant_id"]; ok && tid != "" {
			return tid
		}
	}
	return defaultTenantID
}

// GetDisplayName 获取显示名称
func (c *OIDCClaims) GetDisplayName() string {
	if c.Name != "" {
		return c.Name
	}
	if c.GivenName != "" || c.FamilyName != "" {
		return strings.TrimSpace(c.GivenName + " " + c.FamilyName)
	}
	if c.PreferredUsername != "" {
		return c.PreferredUsername
	}
	return c.Subject
}

// GetUsername 获取用户名
func (c *OIDCClaims) GetUsername() string {
	if c.PreferredUsername != "" {
		return c.PreferredUsername
	}
	if c.Email != "" {
		return c.Email
	}
	return c.Subject
}

// ToInternalClaims 转换为内部 Claims 格式（修复 #23：正确设置 UserID）
func (c *OIDCClaims) ToInternalClaims(clientID, defaultTenantID string, roleToPermissions func(string) []string) *Claims {
	roles := c.GetRoles(clientID)

	// 转换角色为权限
	permissionSet := make(map[string]bool)
	for _, role := range roles {
		if roleToPermissions != nil {
			for _, perm := range roleToPermissions(role) {
				permissionSet[perm] = true
			}
		}
	}

	permissions := make([]string, 0, len(permissionSet))
	for perm := range permissionSet {
		permissions = append(permissions, perm)
	}

	// 修复 #23：尝试从 Subject 解析 UserID
	var userID uuid.UUID
	var err error

	// 尝试解析 Subject 为 UUID
	userID, err = uuid.Parse(c.Subject)
	if err != nil {
		// 如果 Subject 不是 UUID，生成新的 UUID
		// 注意：这意味着每次 OIDC 登录可能生成不同的 UserID
		// 生产环境应该通过数据库查询或映射表来获取稳定的 UserID
		userID = uuid.New()
	}

	return &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.Issuer,
			Subject:   c.Subject,
			IssuedAt:  c.IssuedAt,
			ExpiresAt: c.ExpiresAt,
		},
		UserID:      userID, // 修复 #23：正确设置 UserID
		TenantID:    c.GetTenantID(defaultTenantID),
		Username:    c.GetUsername(),
		Email:       c.Email,
		Roles:       roles,
		Permissions: permissions,
		TokenType:   TokenTypeAccess,
	}
}

// ValidateStandardClaims 验证标准 Claims
func (c *OIDCClaims) ValidateStandardClaims(expectedIssuer string) error {
	// 验证 issuer
	if expectedIssuer != "" && c.Issuer != expectedIssuer {
		return &OIDCClaimsError{
			Field:   "issuer",
			Message: "issuer mismatch: expected " + expectedIssuer + ", got " + c.Issuer,
		}
	}

	// 验证 subject
	if c.Subject == "" {
		return &OIDCClaimsError{
			Field:   "sub",
			Message: "subject is required",
		}
	}

	// 验证过期时间
	if c.ExpiresAt != nil && c.ExpiresAt.Before(jwt.NewNumericDate(jwt.TimeFunc()).Time) {
		return &OIDCClaimsError{
			Field:   "exp",
			Message: "token is expired",
		}
	}

	return nil
}

// OIDCClaimsError OIDC Claims 验证错误
type OIDCClaimsError struct {
	Field   string
	Message string
}

func (e *OIDCClaimsError) Error() string {
	return "oidc claims error: " + e.Field + ": " + e.Message
}
