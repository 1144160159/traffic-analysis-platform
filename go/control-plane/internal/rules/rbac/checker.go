////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/rbac/checker.go
// RBAC 权限检查器 - 完整修复版
////////////////////////////////////////////////////////////////////////////////

package rbac

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// =============================================================================
// 上下文键
// =============================================================================

type contextKey string

const (
	ContextKeyUserID      contextKey = "user_id"
	ContextKeyTenantID    contextKey = "tenant_id"
	ContextKeyUsername    contextKey = "username"
	ContextKeyRoles       contextKey = "roles"
	ContextKeyPermissions contextKey = "permissions"
)

// =============================================================================
// Checker 权限检查器
// =============================================================================

// CheckerConfig 检查器配置
type CheckerConfig struct {
	Enabled      bool
	CacheEnabled bool
	CacheTTL     time.Duration
	CacheSize    int
	DefaultPerms []Permission
	Logger       *zap.Logger
}

// DefaultCheckerConfig 默认配置
func DefaultCheckerConfig() CheckerConfig {
	return CheckerConfig{
		Enabled:      true,
		CacheEnabled: true,
		CacheTTL:     5 * time.Minute,
		CacheSize:    10000,
		DefaultPerms: []Permission{PermRuleRead},
	}
}

// Checker 权限检查器
type Checker struct {
	config CheckerConfig
	cache  map[string]*cachedPermissions // 简化实现，使用 map 替代 lru.Cache
	mu     sync.RWMutex
	logger *zap.Logger
}

// cachedPermissions 缓存的权限
type cachedPermissions struct {
	Permissions []Permission
	ExpiresAt   time.Time
}

// NewChecker 创建权限检查器（简化版本，只需要 logger）
func NewChecker(logger *zap.Logger) *Checker {
	config := DefaultCheckerConfig()
	config.Logger = logger
	return NewCheckerWithConfig(config)
}

// NewCheckerWithConfig 创建权限检查器（完整版本）
func NewCheckerWithConfig(config CheckerConfig) *Checker {
	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	var cache map[string]*cachedPermissions
	if config.CacheEnabled {
		cache = make(map[string]*cachedPermissions)
	}

	return &Checker{
		config: config,
		cache:  cache,
		logger: logger,
	}
}

// =============================================================================
// 权限检查方法
// =============================================================================

// CheckPermission 检查单个权限
func (c *Checker) CheckPermission(ctx context.Context, required Permission) error {
	if !c.config.Enabled {
		return nil
	}

	permissions := c.GetPermissionsFromContext(ctx)
	if len(permissions) == 0 {
		return &PermissionDeniedError{
			Required: required,
			Message:  "no permissions found in context",
		}
	}

	if !HasPermission(permissions, required) {
		c.logger.Debug("Permission denied",
			zap.String("user_id", GetUserIDFromContext(ctx)),
			zap.String("required", string(required)),
			zap.Any("permissions", permissions))

		return &PermissionDeniedError{
			Required: required,
			Message:  fmt.Sprintf("permission denied: %s required", required),
		}
	}

	return nil
}

// CheckAnyPermission 检查任一权限
func (c *Checker) CheckAnyPermission(ctx context.Context, required ...Permission) error {
	if !c.config.Enabled {
		return nil
	}

	permissions := c.GetPermissionsFromContext(ctx)
	if len(permissions) == 0 {
		return &PermissionDeniedError{
			Required: required[0],
			Message:  "no permissions found in context",
		}
	}

	if !HasAnyPermission(permissions, required...) {
		return &PermissionDeniedError{
			Required: required[0],
			Message:  fmt.Sprintf("permission denied: one of %v required", required),
		}
	}

	return nil
}

// CheckAllPermissions 检查所有权限
func (c *Checker) CheckAllPermissions(ctx context.Context, required ...Permission) error {
	if !c.config.Enabled {
		return nil
	}

	permissions := c.GetPermissionsFromContext(ctx)
	if len(permissions) == 0 {
		return &PermissionDeniedError{
			Required: required[0],
			Message:  "no permissions found in context",
		}
	}

	for _, req := range required {
		if !HasPermission(permissions, req) {
			return &PermissionDeniedError{
				Required: req,
				Message:  fmt.Sprintf("permission denied: %s required", req),
			}
		}
	}

	return nil
}

// CheckResourceAccess 检查资源访问权限
func (c *Checker) CheckResourceAccess(ctx context.Context, resourceTenantID string, required Permission) error {
	if !c.config.Enabled {
		return nil
	}

	userTenantID := GetTenantIDFromContext(ctx)
	permissions := c.GetPermissionsFromContext(ctx)

	if !CanAccessResource(userTenantID, resourceTenantID, permissions, required) {
		return &PermissionDeniedError{
			Required: required,
			Message:  "access to resource denied",
		}
	}

	return nil
}

// =============================================================================
// 上下文操作
// =============================================================================

// GetPermissionsFromContext 从上下文获取权限（支持 []string 到 []Permission 的转换）
func (c *Checker) GetPermissionsFromContext(ctx context.Context) []Permission {
	// 优先尝试获取 []Permission 类型
	if perms, ok := ctx.Value(ContextKeyPermissions).([]Permission); ok {
		return perms
	}

	// 尝试从字符串切片转换
	if permStrs, ok := ctx.Value(ContextKeyPermissions).([]string); ok {
		perms := make([]Permission, len(permStrs))
		for i, s := range permStrs {
			perms[i] = Permission(s)
		}
		return perms
	}

	// 尝试从角色获取权限
	if roles, ok := ctx.Value(ContextKeyRoles).([]string); ok {
		return GetPermissionsFromRoles(roles)
	}

	return c.config.DefaultPerms
}

// GetUserIDFromContext 从上下文获取用户ID
func GetUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value(ContextKeyUserID).(string); ok {
		return userID
	}
	return ""
}

// GetTenantIDFromContext 从上下文获取租户ID
func GetTenantIDFromContext(ctx context.Context) string {
	if tenantID, ok := ctx.Value(ContextKeyTenantID).(string); ok {
		return tenantID
	}
	return ""
}

// GetUsernameFromContext 从上下文获取用户名
func GetUsernameFromContext(ctx context.Context) string {
	if username, ok := ctx.Value(ContextKeyUsername).(string); ok {
		return username
	}
	return ""
}

// GetRolesFromContext 从上下文获取角色
func GetRolesFromContext(ctx context.Context) []string {
	if roles, ok := ctx.Value(ContextKeyRoles).([]string); ok {
		return roles
	}
	return nil
}

// WithUserInfo 将用户信息注入上下文
func WithUserInfo(ctx context.Context, userID, tenantID, username string, roles []string, permissions []Permission) context.Context {
	ctx = context.WithValue(ctx, ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, ContextKeyUsername, username)
	ctx = context.WithValue(ctx, ContextKeyRoles, roles)
	ctx = context.WithValue(ctx, ContextKeyPermissions, permissions)
	return ctx
}

// =============================================================================
// 辅助方法（支持 []string 类型的权限）
// =============================================================================

// HasPermission 检查权限（支持 []string 参数）
func (c *Checker) HasPermission(permissions interface{}, required Permission) bool {
	var perms []Permission

	switch p := permissions.(type) {
	case []Permission:
		perms = p
	case []string:
		perms = make([]Permission, len(p))
		for i, s := range p {
			perms[i] = Permission(s)
		}
	default:
		return false
	}

	return HasPermission(perms, required)
}

// HasAnyPermission 检查任一权限（支持 []string 参数）
func (c *Checker) HasAnyPermission(permissions interface{}, required ...Permission) bool {
	var perms []Permission

	switch p := permissions.(type) {
	case []Permission:
		perms = p
	case []string:
		perms = make([]Permission, len(p))
		for i, s := range p {
			perms[i] = Permission(s)
		}
	default:
		return false
	}

	return HasAnyPermission(perms, required...)
}

// HasAllPermissions 检查所有权限（支持 []string 参数）
func (c *Checker) HasAllPermissions(permissions interface{}, required ...Permission) bool {
	var perms []Permission

	switch p := permissions.(type) {
	case []Permission:
		perms = p
	case []string:
		perms = make([]Permission, len(p))
		for i, s := range p {
			perms[i] = Permission(s)
		}
	default:
		return false
	}

	return HasAllPermissions(perms, required...)
}

// =============================================================================
// 缓存操作
// =============================================================================

// GetCachedPermissions 获取缓存的权限
func (c *Checker) GetCachedPermissions(userID string) ([]Permission, bool) {
	if c.cache == nil {
		return nil, false
	}

	c.mu.RLock()
	cached, ok := c.cache[userID]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Now().After(cached.ExpiresAt) {
		c.mu.Lock()
		delete(c.cache, userID)
		c.mu.Unlock()
		return nil, false
	}

	return cached.Permissions, true
}

// SetCachedPermissions 设置缓存的权限
func (c *Checker) SetCachedPermissions(userID string, permissions []Permission) {
	if c.cache == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[userID] = &cachedPermissions{
		Permissions: permissions,
		ExpiresAt:   time.Now().Add(c.config.CacheTTL),
	}
}

// InvalidateCache 使缓存失效
func (c *Checker) InvalidateCache(userID string) {
	if c.cache == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, userID)
}

// ClearCache 清空缓存
func (c *Checker) ClearCache() {
	if c.cache == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*cachedPermissions)
}

// =============================================================================
// HTTP 中间件
// =============================================================================

// RequirePermission 返回要求指定权限的中间件
func (c *Checker) RequirePermission(required Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := c.CheckPermission(r.Context(), required); err != nil {
				c.writePermissionDenied(w, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyPermission 返回要求任一权限的中间件
func (c *Checker) RequireAnyPermission(required ...Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := c.CheckAnyPermission(r.Context(), required...); err != nil {
				c.writePermissionDenied(w, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAllPermissions 返回要求所有权限的中间件
func (c *Checker) RequireAllPermissions(required ...Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := c.CheckAllPermissions(r.Context(), required...); err != nil {
				c.writePermissionDenied(w, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writePermissionDenied 写入权限拒绝响应
func (c *Checker) writePermissionDenied(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)

	permErr, ok := err.(*PermissionDeniedError)
	if ok {
		fmt.Fprintf(w, `{"code":"PERMISSION_DENIED","message":"%s","required":"%s"}`,
			permErr.Message, permErr.Required)
	} else {
		fmt.Fprintf(w, `{"code":"PERMISSION_DENIED","message":"%s"}`, err.Error())
	}
}

// =============================================================================
// 错误类型
// =============================================================================

// PermissionDeniedError 权限拒绝错误
type PermissionDeniedError struct {
	Required Permission
	Message  string
}

func (e *PermissionDeniedError) Error() string {
	return e.Message
}

// IsPermissionDenied 检查是否为权限拒绝错误
func IsPermissionDenied(err error) bool {
	_, ok := err.(*PermissionDeniedError)
	return ok
}

// =============================================================================
// 辅助函数
// =============================================================================

// ExtractPermissionsFromHeader 从请求头提取权限
func ExtractPermissionsFromHeader(r *http.Request) []Permission {
	// 从 X-Permissions 头提取
	permHeader := r.Header.Get("X-Permissions")
	if permHeader == "" {
		return nil
	}

	permStrs := strings.Split(permHeader, ",")
	permissions := make([]Permission, 0, len(permStrs))
	for _, s := range permStrs {
		s = strings.TrimSpace(s)
		if s != "" {
			permissions = append(permissions, Permission(s))
		}
	}

	return permissions
}

// ExtractRolesFromHeader 从请求头提取角色
func ExtractRolesFromHeader(r *http.Request) []string {
	roleHeader := r.Header.Get("X-Roles")
	if roleHeader == "" {
		return nil
	}

	roleStrs := strings.Split(roleHeader, ",")
	roles := make([]string, 0, len(roleStrs))
	for _, s := range roleStrs {
		s = strings.TrimSpace(s)
		if s != "" {
			roles = append(roles, s)
		}
	}

	return roles
}

// InjectUserContext 注入用户上下文的中间件
func InjectUserContext() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// 从请求头提取用户信息
			userID := r.Header.Get("X-User-ID")
			tenantID := r.Header.Get("X-Tenant-ID")
			username := r.Header.Get("X-Username")
			roles := ExtractRolesFromHeader(r)
			permissions := ExtractPermissionsFromHeader(r)

			// 如果没有权限，尝试从角色获取
			if len(permissions) == 0 && len(roles) > 0 {
				permissions = GetPermissionsFromRoles(roles)
			}

			// 注入上下文
			ctx = WithUserInfo(ctx, userID, tenantID, username, roles, permissions)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
