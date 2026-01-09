////////////////////////////////////////////////////////////////////////////////
// FILE: internal/auth/model/token.go
// 完整修复版 v4：
// 1. 修复 StringSlice.Scan 方法（处理 string 和 nil 类型）
// 2. 修复 JSONMap.Scan 方法（完整类型处理）
// 3. 与 PostgreSQL DDL 完全对齐
// 4. 完善业务逻辑方法
////////////////////////////////////////////////////////////////////////////////

package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TokenType Token 类型
type TokenType string

const (
	TokenTypeUser    TokenType = "user"
	TokenTypeAPI     TokenType = "api"
	TokenTypeProbe   TokenType = "probe"
	TokenTypeService TokenType = "service"
)

// String 返回 Token 类型字符串
func (t TokenType) String() string {
	return string(t)
}

// IsValid 验证 Token 类型是否有效
func (t TokenType) IsValid() bool {
	switch t {
	case TokenTypeUser, TokenTypeAPI, TokenTypeProbe, TokenTypeService:
		return true
	default:
		return false
	}
}

// TokenStatus Token 状态
type TokenStatus string

const (
	TokenStatusActive  TokenStatus = "active"
	TokenStatusRevoked TokenStatus = "revoked"
	TokenStatusExpired TokenStatus = "expired"
)

// APIToken API Token 完整模型（与 PostgreSQL DDL 完全对齐）
type APIToken struct {
	// ========== 主键 ==========
	TokenID uuid.UUID `json:"token_id" db:"token_id"`

	// ========== 租户和用户 ==========
	TenantID string     `json:"tenant_id" db:"tenant_id"`
	UserID   *uuid.UUID `json:"user_id,omitempty" db:"user_id"`

	// ========== Token 基本信息 ==========
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description,omitempty" db:"description"`
	TokenType   TokenType `json:"token_type" db:"token_type"`

	// ========== Token 存储（仅存储哈希值） ==========
	TokenHash   string `json:"-" db:"token_hash"`
	TokenPrefix string `json:"token_prefix,omitempty" db:"token_prefix"`

	// ========== 权限控制 ==========
	Scopes StringSlice `json:"scopes" db:"scopes"`

	// ========== 状态管理 ==========
	Status TokenStatus `json:"status" db:"status"`

	// ========== 过期控制 ==========
	ExpiresAt *time.Time `json:"expires_at,omitempty" db:"expires_at"`

	// ========== 使用统计 ==========
	LastUsedAt *time.Time `json:"last_used_at,omitempty" db:"last_used_at"`
	UsageCount int64      `json:"usage_count" db:"usage_count"`

	// ========== 审计信息 ==========
	CreatedBy *uuid.UUID `json:"created_by,omitempty" db:"created_by"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`

	// ========== 轮转配置 ==========
	RotationEnabled  bool       `json:"rotation_enabled" db:"rotation_enabled"`
	RotationInterval *int       `json:"rotation_interval,omitempty" db:"rotation_interval"`
	LastRotatedAt    *time.Time `json:"last_rotated_at,omitempty" db:"last_rotated_at"`
	PreviousTokenID  *uuid.UUID `json:"previous_token_id,omitempty" db:"previous_token_id"`

	// ========== 安全控制 ==========
	IPWhitelist StringSlice `json:"ip_whitelist,omitempty" db:"ip_whitelist"`

	// ========== 扩展元数据 ==========
	Metadata JSONMap `json:"metadata,omitempty" db:"metadata"`

	// ========== 探针专用字段 ==========
	ProbeID string `json:"probe_id,omitempty" db:"probe_id"`
}

// ==================== 业务逻辑方法 ====================

// IsExpired 检查 Token 是否过期
func (t *APIToken) IsExpired() bool {
	if t.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*t.ExpiresAt)
}

// IsRevoked 检查 Token 是否已撤销
func (t *APIToken) IsRevoked() bool {
	return t.Status == TokenStatusRevoked || t.RevokedAt != nil
}

// IsActive 检查 Token 是否可用
func (t *APIToken) IsActive() bool {
	return t.Status == TokenStatusActive && !t.IsExpired() && !t.IsRevoked()
}

// NeedsRotation 检查是否需要轮转
func (t *APIToken) NeedsRotation() bool {
	if !t.RotationEnabled || t.RotationInterval == nil || *t.RotationInterval <= 0 {
		return false
	}

	intervalDuration := time.Duration(*t.RotationInterval) * 24 * time.Hour

	if t.LastRotatedAt == nil {
		return time.Since(t.CreatedAt) > intervalDuration
	}

	return time.Since(*t.LastRotatedAt) > intervalDuration
}

// CanAccessFromIP 检查 IP 是否在白名单中
func (t *APIToken) CanAccessFromIP(ip string) bool {
	if len(t.IPWhitelist) == 0 {
		return true
	}

	for _, allowedIP := range t.IPWhitelist {
		if allowedIP == ip || allowedIP == "*" {
			return true
		}
	}

	return false
}

// HasScope 检查是否拥有指定权限
func (t *APIToken) HasScope(scope string) bool {
	for _, s := range t.Scopes {
		if s == scope || s == "*" {
			return true
		}
		if len(s) > 1 && s[len(s)-1] == '*' {
			prefix := s[:len(s)-1]
			if len(scope) >= len(prefix) && scope[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}

// HasAnyScope 检查是否有任一权限
func (t *APIToken) HasAnyScope(scopes ...string) bool {
	for _, scope := range scopes {
		if t.HasScope(scope) {
			return true
		}
	}
	return false
}

// HasAllScopes 检查是否有所有权限
func (t *APIToken) HasAllScopes(scopes ...string) bool {
	for _, scope := range scopes {
		if !t.HasScope(scope) {
			return false
		}
	}
	return true
}

// ==================== 自定义类型（JSONB 支持） ====================

// StringSlice 字符串切片（JSONB 数组）
type StringSlice []string

// Value 实现 driver.Valuer 接口
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(s)
}

// Scan 实现 sql.Scanner 接口（修复 #4：完整类型处理）
func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = []string{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan type %T into StringSlice", value)
	}

	if len(bytes) == 0 {
		*s = []string{}
		return nil
	}

	return json.Unmarshal(bytes, s)
}

// JSONMap JSON 对象（JSONB 键值对）
type JSONMap map[string]interface{}

// Value 实现 driver.Valuer 接口
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

// Scan 实现 sql.Scanner 接口（修复 #4：完整类型处理）
func (m *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*m = make(map[string]interface{})
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan type %T into JSONMap", value)
	}

	if len(bytes) == 0 {
		*m = make(map[string]interface{})
		return nil
	}

	return json.Unmarshal(bytes, m)
}

// ==================== TokenRotationHistory Token 轮转历史 ====================

type TokenRotationHistory struct {
	ID              uuid.UUID `json:"id" db:"id"`
	TokenID         uuid.UUID `json:"token_id" db:"token_id"`
	OldTokenHash    string    `json:"-" db:"old_token_hash"`
	NewTokenHash    string    `json:"-" db:"new_token_hash"`
	RotatedAt       time.Time `json:"rotated_at" db:"rotated_at"`
	RotatedBy       string    `json:"rotated_by" db:"rotated_by"`
	Reason          string    `json:"reason" db:"reason"`
	GracePeriodEnds time.Time `json:"grace_period_ends" db:"grace_period_ends"`
}

// IsGracePeriodActive 检查宽限期是否仍然有效
func (h *TokenRotationHistory) IsGracePeriodActive() bool {
	return time.Now().Before(h.GracePeriodEnds)
}

// ==================== TokenUsageLog Token 使用日志 ====================

type TokenUsageLog struct {
	ID             int64     `json:"id" db:"id"`
	TokenID        uuid.UUID `json:"token_id" db:"token_id"`
	TenantID       string    `json:"tenant_id" db:"tenant_id"`
	UsedAt         time.Time `json:"used_at" db:"used_at"`
	IPAddr         string    `json:"ip_addr" db:"ip_addr"`
	UserAgent      string    `json:"user_agent,omitempty" db:"user_agent"`
	Endpoint       string    `json:"endpoint" db:"endpoint"`
	Method         string    `json:"method" db:"method"`
	StatusCode     int       `json:"status_code" db:"status_code"`
	ResponseTimeMs int       `json:"response_time_ms" db:"response_time_ms"`
}

// ==================== RevokedSession Session 撤销记录 ====================

type RevokedSession struct {
	SessionID string    `json:"session_id" db:"session_id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	TenantID  string    `json:"tenant_id" db:"tenant_id"`
	RevokedAt time.Time `json:"revoked_at" db:"revoked_at"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	Reason    string    `json:"reason,omitempty" db:"reason"`
}

// IsExpired 检查撤销记录是否过期（可以清理）
func (s *RevokedSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// ==================== DTO 对象 ====================

// CreateTokenRequest 创建 Token 请求
type CreateTokenRequest struct {
	TenantID    string            `json:"tenant_id" validate:"required"`
	Name        string            `json:"name" validate:"required,min=1,max=100"`
	Description string            `json:"description" validate:"max=500"`
	TokenType   TokenType         `json:"token_type" validate:"required"`
	Scopes      []string          `json:"scopes" validate:"required,min=1"`
	ProbeID     string            `json:"probe_id,omitempty"`
	ExpiresIn   *time.Duration    `json:"expires_in,omitempty"`
	IPWhitelist []string          `json:"ip_whitelist,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedBy   uuid.UUID         `json:"-"`
}

// CreateTokenResponse 创建 Token 响应
type CreateTokenResponse struct {
	TokenID     uuid.UUID  `json:"token_id"`
	Token       string     `json:"token"`
	TokenPrefix string     `json:"token_prefix"`
	Name        string     `json:"name"`
	TokenType   TokenType  `json:"token_type"`
	Scopes      []string   `json:"scopes"`
	ProbeID     string     `json:"probe_id,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	Warning     string     `json:"warning,omitempty"`
}

// UpdateTokenRequest 更新 Token 请求
type UpdateTokenRequest struct {
	Name        *string    `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	Description *string    `json:"description,omitempty" validate:"omitempty,max=500"`
	Scopes      []string   `json:"scopes,omitempty"`
	IPWhitelist []string   `json:"ip_whitelist,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// TokenStatistics Token 统计信息
type TokenStatistics struct {
	TenantID      string              `json:"tenant_id"`
	TotalTokens   int64               `json:"total_tokens"`
	ActiveTokens  int64               `json:"active_tokens"`
	ExpiredTokens int64               `json:"expired_tokens"`
	RevokedTokens int64               `json:"revoked_tokens"`
	ByType        map[TokenType]int64 `json:"by_type"`
	TotalUsage    int64               `json:"total_usage"`
	LastCreatedAt *time.Time          `json:"last_created_at,omitempty"`
	LastUsedAt    *time.Time          `json:"last_used_at,omitempty"`
}

// ==================== 权限范围常量 ====================

const (
	ScopeUserRead   = "user:read"
	ScopeUserWrite  = "user:write"
	ScopeUserDelete = "user:delete"

	ScopeAlertRead   = "alert:read"
	ScopeAlertWrite  = "alert:write"
	ScopeAlertExport = "alert:export"

	ScopeRuleRead   = "rule:read"
	ScopeRuleWrite  = "rule:write"
	ScopeRuleDelete = "rule:delete"

	ScopeDeployRead     = "deploy:read"
	ScopeDeployCreate   = "deploy:create"
	ScopeDeployActivate = "deploy:activate"
	ScopeDeployRollback = "deploy:rollback"

	ScopePcapRead     = "pcap:read"
	ScopePcapDownload = "pcap:download"
	ScopePcapCut      = "pcap:cut"

	ScopeAdminAll         = "admin:*"
	ScopeAdminCrossTenant = "admin:cross_tenant"

	ScopeProbeIngest  = "probe:ingest"
	ScopeProbeMetrics = "probe:metrics"

	ScopeTokenRead  = "token:read"
	ScopeTokenWrite = "token:write"
)

// 默认权限集合
var (
	DefaultProbeScopes = []string{
		ScopeProbeIngest,
		ScopeProbeMetrics,
	}

	ProbeFullScopes = []string{
		ScopeProbeIngest,
		ScopeProbeMetrics,
		ScopePcapRead,
	}

	ProbeMinimalScopes = []string{
		ScopeProbeIngest,
	}

	AllValidScopes = []string{
		ScopeProbeIngest,
		ScopeProbeMetrics,
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
		ScopeAdminAll,
		ScopeAdminCrossTenant,
		ScopeTokenRead,
		ScopeTokenWrite,
	}
)

// ValidateScopes 验证权限列表是否合法
func ValidateScopes(scopes []string) (valid []string, invalid []string) {
	validScopesMap := make(map[string]bool)
	for _, scope := range AllValidScopes {
		validScopesMap[scope] = true
	}
	validScopesMap["*"] = true

	valid = make([]string, 0)
	invalid = make([]string, 0)

	for _, scope := range scopes {
		if validScopesMap[scope] {
			valid = append(valid, scope)
		} else {
			invalid = append(invalid, scope)
		}
	}

	return valid, invalid
}

// ScopeInfo Scope 信息描述
type ScopeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// GetAllScopeInfos 获取所有 Scope 的详细信息
func GetAllScopeInfos() []ScopeInfo {
	return []ScopeInfo{
		{Name: ScopeProbeIngest, Description: "Upload flow events and PCAP index", Category: "probe"},
		{Name: ScopeProbeMetrics, Description: "Report probe metrics", Category: "probe"},
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
		{Name: ScopeAdminAll, Description: "Full admin access", Category: "admin"},
		{Name: ScopeAdminCrossTenant, Description: "Cross-tenant admin access", Category: "admin"},
		{Name: ScopeTokenRead, Description: "Read API tokens", Category: "admin"},
		{Name: ScopeTokenWrite, Description: "Manage API tokens", Category: "admin"},
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

// ScopesToList 将 scopes 切片转换为列表（兼容旧代码）
func ScopesToList(scopes StringSlice) []string {
	return []string(scopes)
}

// ListToScopes 将列表转换为 scopes 切片（兼容旧代码）
func ListToScopes(list []string) StringSlice {
	return StringSlice(list)
}

// IsValidScope 检查 scope 是否有效
func IsValidScope(scope string) bool {
	if scope == "*" {
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
	}
	return probeScopes[scope]
}
