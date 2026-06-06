////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/model/rule.go
// 完整版：添加验证方法、常量定义、状态枚举、幂等键
// 修复内容:
// 1. RuleCommand 添加 EventID 和 Version 字段（幂等支持）
// 2. 完善验证逻辑
// 3. 添加 BatchError 结构体（支持详细错误信息）
////////////////////////////////////////////////////////////////////////////////

package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// =============================================================================
// 规则类型常量
// =============================================================================

// RuleType 规则类型
type RuleType string

const (
	RuleTypeThreshold   RuleType = "threshold"   // 阈值规则
	RuleTypeAnomaly     RuleType = "anomaly"     // 异常规则
	RuleTypeSignature   RuleType = "signature"   // 特征规则
	RuleTypeCorrelation RuleType = "correlation" // 关联规则
	RuleTypeML          RuleType = "ml"          // 机器学习规则
	RuleTypeCustom      RuleType = "custom"      // 自定义规则
)

// ValidRuleTypes 有效的规则类型
var ValidRuleTypes = map[RuleType]bool{
	RuleTypeThreshold:   true,
	RuleTypeAnomaly:     true,
	RuleTypeSignature:   true,
	RuleTypeCorrelation: true,
	RuleTypeML:          true,
	RuleTypeCustom:      true,
}

// IsValidRuleType 检查规则类型是否有效
func IsValidRuleType(t string) bool {
	return ValidRuleTypes[RuleType(t)]
}

// =============================================================================
// 规则引擎常量
// =============================================================================

// RuleEngine 规则引擎
type RuleEngine string

const (
	EngineInternal RuleEngine = "internal" // 内置引擎
	EngineSuricata RuleEngine = "suricata" // Suricata IDS
	EngineYara     RuleEngine = "yara"     // YARA 规则
	EngineSigma    RuleEngine = "sigma"    // Sigma 规则
	EngineCEP      RuleEngine = "cep"      // CEP 复杂事件处理
)

// ValidRuleEngines 有效的规则引擎
var ValidRuleEngines = map[RuleEngine]bool{
	EngineInternal: true,
	EngineSuricata: true,
	EngineYara:     true,
	EngineSigma:    true,
	EngineCEP:      true,
}

// IsValidRuleEngine 检查规则引擎是否有效
func IsValidRuleEngine(e string) bool {
	return ValidRuleEngines[RuleEngine(e)]
}

// =============================================================================
// 规则严重程度
// =============================================================================

// Severity 严重程度
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// ValidSeverities 有效的严重程度
var ValidSeverities = map[Severity]bool{
	SeverityLow:      true,
	SeverityMedium:   true,
	SeverityHigh:     true,
	SeverityCritical: true,
}

// IsValidSeverity 检查严重程度是否有效
func IsValidSeverity(s string) bool {
	return ValidSeverities[Severity(s)]
}

// SeverityToScore 严重程度转分数
func SeverityToScore(s Severity) float32 {
	scores := map[Severity]float32{
		SeverityLow:      0.25,
		SeverityMedium:   0.5,
		SeverityHigh:     0.75,
		SeverityCritical: 1.0,
	}
	if score, ok := scores[s]; ok {
		return score
	}
	return 0.5
}

// =============================================================================
// 规则状态
// =============================================================================

// RuleStatus 规则状态
type RuleStatus string

const (
	RuleStatusDraft      RuleStatus = "draft"      // 草稿
	RuleStatusActive     RuleStatus = "active"     // 活跃
	RuleStatusDisabled   RuleStatus = "disabled"   // 禁用
	RuleStatusArchived   RuleStatus = "archived"   // 归档
	RuleStatusDeprecated RuleStatus = "deprecated" // 废弃
	RuleStatusDeleted    RuleStatus = "deleted"    // 已删除（软删除）
)

// ValidRuleStatuses 有效的规则状态
var ValidRuleStatuses = map[RuleStatus]bool{
	RuleStatusDraft:      true,
	RuleStatusActive:     true,
	RuleStatusDisabled:   true,
	RuleStatusArchived:   true,
	RuleStatusDeprecated: true,
	RuleStatusDeleted:    true,
}

// =============================================================================
// 规则模型
// =============================================================================

// Rule 规则模型
type Rule struct {
	RuleID         string                 `json:"rule_id" db:"rule_id"`
	TenantID       string                 `json:"tenant_id" db:"tenant_id"`
	Name           string                 `json:"name" db:"name"`
	Type           string                 `json:"type" db:"rule_type"`
	Engine         string                 `json:"engine" db:"engine"`
	Description    string                 `json:"description" db:"description"`
	Conditions     map[string]interface{} `json:"conditions" db:"-"`
	ConditionsJSON []byte                 `json:"-" db:"conditions"`
	Labels         []string               `json:"labels" db:"labels"`
	Severity       string                 `json:"severity" db:"severity"`
	Enabled        bool                   `json:"enabled" db:"enabled"`
	Priority       int                    `json:"priority" db:"priority"`
	Version        int64                  `json:"version" db:"version"`
	Status         string                 `json:"status" db:"status"`
	CreatedBy      string                 `json:"created_by" db:"created_by"`
	UpdatedBy      string                 `json:"updated_by" db:"updated_by"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at" db:"updated_at"`
	// 元数据
	Metadata map[string]interface{} `json:"metadata,omitempty" db:"-"`
}

// MarshalConditions 序列化条件
func (r *Rule) MarshalConditions() error {
	if r.Conditions == nil {
		r.Conditions = make(map[string]interface{})
	}
	data, err := json.Marshal(r.Conditions)
	if err != nil {
		return fmt.Errorf("failed to marshal conditions: %w", err)
	}
	r.ConditionsJSON = data
	return nil
}

// UnmarshalConditions 反序列化条件
func (r *Rule) UnmarshalConditions() error {
	if len(r.ConditionsJSON) == 0 {
		r.Conditions = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal(r.ConditionsJSON, &r.Conditions)
}

// Validate 验证规则
func (r *Rule) Validate() error {
	var errors []string

	// 必填字段验证
	if r.TenantID == "" {
		errors = append(errors, "tenant_id is required")
	}

	if r.Name == "" {
		errors = append(errors, "name is required")
	} else if len(r.Name) > 255 {
		errors = append(errors, "name must be less than 255 characters")
	} else if !isValidRuleName(r.Name) {
		errors = append(errors, "name contains invalid characters")
	}

	if r.Type == "" {
		errors = append(errors, "type is required")
	} else if !IsValidRuleType(r.Type) {
		errors = append(errors, fmt.Sprintf("invalid rule type: %s", r.Type))
	}

	// 可选字段验证（带默认值）
	if r.Engine == "" {
		r.Engine = string(EngineInternal)
	} else if !IsValidRuleEngine(r.Engine) {
		errors = append(errors, fmt.Sprintf("invalid engine: %s", r.Engine))
	}

	if r.Severity == "" {
		r.Severity = string(SeverityMedium)
	} else if !IsValidSeverity(r.Severity) {
		errors = append(errors, fmt.Sprintf("invalid severity: %s", r.Severity))
	}

	if r.Status == "" {
		if r.Enabled {
			r.Status = string(RuleStatusActive)
		} else {
			r.Status = string(RuleStatusDraft)
		}
	}

	// 标签验证
	if len(r.Labels) > 20 {
		errors = append(errors, "too many labels (max 20)")
	}
	for i, label := range r.Labels {
		if len(label) > 100 {
			errors = append(errors, fmt.Sprintf("label %d is too long (max 100)", i))
		}
	}

	// 优先级验证
	if r.Priority < 0 || r.Priority > 100 {
		r.Priority = 50 // 默认中等优先级
	}

	// 描述长度验证
	if len(r.Description) > 4000 {
		errors = append(errors, "description must be less than 4000 characters")
	}

	// 条件验证
	if r.Conditions != nil {
		if err := r.validateConditions(); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return &ValidationError{Errors: errors}
	}

	return nil
}

// validateConditions 验证规则条件
func (r *Rule) validateConditions() error {
	// 根据规则类型验证条件
	switch RuleType(r.Type) {
	case RuleTypeThreshold:
		return r.validateThresholdConditions()
	case RuleTypeSignature:
		return r.validateSignatureConditions()
	case RuleTypeAnomaly:
		return r.validateAnomalyConditions()
	default:
		// 其他类型不做严格验证
		return nil
	}
}

// validateThresholdConditions 验证阈值规则条件
func (r *Rule) validateThresholdConditions() error {
	// 阈值规则必须包含 threshold 字段
	if _, ok := r.Conditions["threshold"]; !ok {
		return fmt.Errorf("threshold rule must have 'threshold' condition")
	}
	return nil
}

// validateSignatureConditions 验证特征规则条件
func (r *Rule) validateSignatureConditions() error {
	// 特征规则必须包含 pattern 或 signature 字段
	if _, ok := r.Conditions["pattern"]; !ok {
		if _, ok := r.Conditions["signature"]; !ok {
			return fmt.Errorf("signature rule must have 'pattern' or 'signature' condition")
		}
	}
	return nil
}

// validateAnomalyConditions 验证异常规则条件
func (r *Rule) validateAnomalyConditions() error {
	// 异常规则可以有 baseline 或 deviation 字段
	return nil
}

// SetDefaults 设置默认值
func (r *Rule) SetDefaults() {
	if r.Engine == "" {
		r.Engine = string(EngineInternal)
	}
	if r.Severity == "" {
		r.Severity = string(SeverityMedium)
	}
	if r.Status == "" {
		r.Status = string(RuleStatusDraft)
	}
	if r.Priority == 0 {
		r.Priority = 50
	}
	if r.Conditions == nil {
		r.Conditions = make(map[string]interface{})
	}
}

// Clone 克隆规则
func (r *Rule) Clone() *Rule {
	clone := *r

	// 深拷贝 Conditions
	if r.Conditions != nil {
		clone.Conditions = make(map[string]interface{})
		for k, v := range r.Conditions {
			clone.Conditions[k] = v
		}
	}

	// 深拷贝 Labels
	if r.Labels != nil {
		clone.Labels = make([]string, len(r.Labels))
		copy(clone.Labels, r.Labels)
	}

	// 深拷贝 ConditionsJSON
	if r.ConditionsJSON != nil {
		clone.ConditionsJSON = make([]byte, len(r.ConditionsJSON))
		copy(clone.ConditionsJSON, r.ConditionsJSON)
	}

	return &clone
}

// =============================================================================
// 规则版本
// =============================================================================

// RuleVersion 规则版本
type RuleVersion struct {
	RuleVersionID string    `json:"rule_version_id" db:"rule_version"`
	RuleID        string    `json:"rule_id" db:"rule_id"`
	TenantID      string    `json:"tenant_id" db:"tenant_id"`
	Version       int64     `json:"version" db:"version"`
	ContentURI    string    `json:"content_uri" db:"content_uri"`
	Checksum      string    `json:"checksum" db:"checksum"`
	Status        string    `json:"status" db:"status"`
	ChangeLog     string    `json:"change_log" db:"change_log"`
	CreatedBy     string    `json:"created_by" db:"created_by"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// VersionStatus 版本状态
type VersionStatus string

const (
	VersionStatusActive   VersionStatus = "active"
	VersionStatusArchived VersionStatus = "archived"
	VersionStatusRollback VersionStatus = "rollback"
)

// =============================================================================
// 规则命令（✅ 添加幂等键）
// =============================================================================

// RuleCommand 规则命令（用于 Kafka 消息）
type RuleCommand struct {
	EventID    string    `json:"event_id"` // ✅ 幂等键（UUID）
	Action     string    `json:"action"`   // create, update, delete, enable, disable, sync
	Rule       *Rule     `json:"rule"`
	Timestamp  time.Time `json:"timestamp"`
	OperatorID string    `json:"operator_id"`
	TraceID    string    `json:"trace_id,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	Version    int64     `json:"version"` // ✅ 规则版本号（用于版本校验）
}

// RuleCommandAction 命令动作
type RuleCommandAction string

const (
	ActionCreate  RuleCommandAction = "create"
	ActionUpdate  RuleCommandAction = "update"
	ActionDelete  RuleCommandAction = "delete"
	ActionEnable  RuleCommandAction = "enable"
	ActionDisable RuleCommandAction = "disable"
	ActionSync    RuleCommandAction = "sync"
)

// =============================================================================
// 审计日志
// =============================================================================

// AuditLog 审计日志
type AuditLog struct {
	TenantID   string                 `json:"tenant_id"`
	UserID     string                 `json:"user_id"`
	Action     string                 `json:"action"`
	ObjectType string                 `json:"object_type"`
	ObjectID   string                 `json:"object_id"`
	OldValue   interface{}            `json:"old_value,omitempty"`
	NewValue   interface{}            `json:"new_value,omitempty"`
	Detail     map[string]interface{} `json:"detail"`
	IPAddr     string                 `json:"ip_addr"`
	UserAgent  string                 `json:"user_agent"`
	Timestamp  time.Time              `json:"timestamp"`
	TraceID    string                 `json:"trace_id,omitempty"`
	RequestID  string                 `json:"request_id,omitempty"`
}

// =============================================================================
// 验证辅助
// =============================================================================

// ValidationError 验证错误
type RuleValidationError struct {
	Errors []string `json:"errors"`
}

func (e *RuleValidationError) Error() string {
	return "validation error: " + strings.Join(e.Errors, "; ")
}

// 规则名称正则：允许字母、数字、下划线、连字符、空格、中文
var ruleNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\s\u4e00-\u9fa5]+$`)

// isValidRuleName 检查规则名称是否有效
func isValidRuleName(name string) bool {
	return ruleNameRegex.MatchString(name)
}

// =============================================================================
// 列表请求/响应
// =============================================================================

// ListRulesRequest 列表请求
type ListRulesRequest struct {
	TenantID string   `json:"tenant_id"`
	Type     string   `json:"type,omitempty"`
	Engine   string   `json:"engine,omitempty"`
	Severity string   `json:"severity,omitempty"`
	Enabled  *bool    `json:"enabled,omitempty"`
	Labels   []string `json:"labels,omitempty"`
	Search   string   `json:"search,omitempty"`
	Limit    int      `json:"limit"`
	Offset   int      `json:"offset"`
	OrderBy  string   `json:"order_by,omitempty"`
	OrderDir string   `json:"order_dir,omitempty"`
}

// ListRulesResponse 列表响应
type ListRulesResponse struct {
	Rules  []*Rule `json:"rules"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

// =============================================================================
// 批量操作
// =============================================================================

// BatchOperation 批量操作类型
type BatchOperation string

const (
	BatchOpEnable  BatchOperation = "enable"
	BatchOpDisable BatchOperation = "disable"
	BatchOpDelete  BatchOperation = "delete"
	BatchOpExport  BatchOperation = "export"
)

// BatchRequest 批量操作请求
type BatchRequest struct {
	RuleIDs   []string       `json:"rule_ids"`
	Operation BatchOperation `json:"operation"`
}

// BatchResult 批量操作结果
type BatchResult struct {
	SuccessCount int          `json:"success_count"`
	FailCount    int          `json:"fail_count"`
	FailedIDs    []string     `json:"failed_ids,omitempty"`
	Errors       []BatchError `json:"errors,omitempty"`
}

// ✅ BatchError 批量操作错误（增强版）
type BatchError struct {
	Index   int    `json:"index"`          // 在请求中的索引
	ID      string `json:"id"`             // 规则 ID
	Message string `json:"message"`        // 错误消息
	Code    string `json:"code,omitempty"` // 错误码
}

// =============================================================================
// 导入导出
// =============================================================================

// ImportStrategy 导入策略
type ImportStrategy string

const (
	ImportStrategySkip      ImportStrategy = "skip"      // 跳过已存在
	ImportStrategyOverwrite ImportStrategy = "overwrite" // 覆盖
	ImportStrategyRename    ImportStrategy = "rename"    // 重命名
)

// ImportResult 导入结果
type ImportResult struct {
	Total   int           `json:"total"`
	Created int           `json:"created"`
	Updated int           `json:"updated"`
	Skipped int           `json:"skipped"`
	Failed  int           `json:"failed"`
	Errors  []ImportError `json:"errors,omitempty"`
}

// ImportError 导入错误
type ImportError struct {
	Index   int    `json:"index"`
	Name    string `json:"name"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// ExportResult 导出结果
type ExportResult struct {
	Total      int       `json:"total"`
	Exported   int       `json:"exported"`
	Skipped    int       `json:"skipped"`
	ExportedAt time.Time `json:"exported_at"`
	ExportedBy string    `json:"exported_by"`
	Version    string    `json:"version"`
}

// RuleExportItem 规则导出项
type RuleExportItem struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Engine      string                 `json:"engine"`
	Description string                 `json:"description,omitempty"`
	Conditions  map[string]interface{} `json:"conditions"`
	Labels      []string               `json:"labels,omitempty"`
	Severity    string                 `json:"severity"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority,omitempty"`
}

// RuleExport 规则导出结构
type RuleExport struct {
	Version    string           `json:"version"`
	ExportedAt time.Time        `json:"exported_at"`
	ExportedBy string           `json:"exported_by"`
	TenantID   string           `json:"tenant_id"`
	Rules      []RuleExportItem `json:"rules"`
}

// =============================================================================
// 统计信息
// =============================================================================

// RuleStats 规则统计
type RuleStats struct {
	Total       int64            `json:"total"`
	Enabled     int64            `json:"enabled"`
	Disabled    int64            `json:"disabled"`
	ByType      map[string]int64 `json:"by_type"`
	ByEngine    map[string]int64 `json:"by_engine"`
	BySeverity  map[string]int64 `json:"by_severity"`
	RecentAdded int64            `json:"recent_added"`
}

// =============================================================================
// 过滤器
// =============================================================================

// RuleFilter 规则过滤条件
type RuleFilter struct {
	Type     string   `json:"type,omitempty"`
	Engine   string   `json:"engine,omitempty"`
	Severity string   `json:"severity,omitempty"`
	Enabled  *bool    `json:"enabled,omitempty"`
	Labels   []string `json:"labels,omitempty"`
	Keyword  string   `json:"keyword,omitempty"`
	Limit    int      `json:"limit,omitempty"`
	Offset   int      `json:"offset,omitempty"`
	OrderBy  string   `json:"order_by,omitempty"`
	OrderDir string   `json:"order_dir,omitempty"`
}

// Validate 验证过滤器
func (f *RuleFilter) Validate() error {
	if f.Limit < 0 {
		return fmt.Errorf("limit must be non-negative")
	}
	if f.Limit > 1000 {
		return fmt.Errorf("limit must not exceed 1000")
	}
	if f.Offset < 0 {
		return fmt.Errorf("offset must be non-negative")
	}

	// OrderBy 白名单验证
	if f.OrderBy != "" {
		validOrderBy := map[string]bool{
			"name":       true,
			"created_at": true,
			"updated_at": true,
			"severity":   true,
			"priority":   true,
		}
		if !validOrderBy[f.OrderBy] {
			return fmt.Errorf("invalid order_by field: %s", f.OrderBy)
		}
	}

	// OrderDir 验证
	if f.OrderDir != "" {
		f.OrderDir = strings.ToUpper(f.OrderDir)
		if f.OrderDir != "ASC" && f.OrderDir != "DESC" {
			return fmt.Errorf("order_dir must be ASC or DESC")
		}
	}

	return nil
}

// SetDefaults 设置过滤器默认值
func (f *RuleFilter) SetDefaults() {
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Limit > 1000 {
		f.Limit = 1000
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.OrderBy == "" {
		f.OrderBy = "updated_at"
	}
	if f.OrderDir == "" {
		f.OrderDir = "DESC"
	}
}

// =============================================================================
// Outbox 事件（用于 Transactional Outbox 模式）
// =============================================================================

// OutboxEvent Outbox 事件记录
type OutboxEvent struct {
	ID          int64      `json:"id" db:"id"`
	RuleID      string     `json:"rule_id" db:"rule_id"`
	EventType   string     `json:"event_type" db:"event_type"`
	Payload     []byte     `json:"payload" db:"payload"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	Published   bool       `json:"published" db:"published"`
	PublishedAt *time.Time `json:"published_at,omitempty" db:"published_at"`
	RetryCount  int        `json:"retry_count" db:"retry_count"`
	LastError   string     `json:"last_error,omitempty" db:"last_error"`
	NextRetry   *time.Time `json:"next_retry,omitempty" db:"next_retry"`
}

// OutboxEventType Outbox 事件类型
type OutboxEventType string

const (
	OutboxEventCreate  OutboxEventType = "create"
	OutboxEventUpdate  OutboxEventType = "update"
	OutboxEventDelete  OutboxEventType = "delete"
	OutboxEventEnable  OutboxEventType = "enable"
	OutboxEventDisable OutboxEventType = "disable"
	OutboxEventSync    OutboxEventType = "sync"
)
