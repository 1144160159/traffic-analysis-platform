////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/model/deployment_state.go
// 部署状态机定义 - 完整修复版
// 修复内容:
// 1. ✅ 添加缺失字段: GrayStartedAt, GrayExpiredAt, ActivatedAt, RolledBackAt, RollbackFrom, RollbackReason, Metadata
// 2. ✅ 添加 Metadata 序列化/反序列化方法
// 3. ✅ 完善状态转换验证
////////////////////////////////////////////////////////////////////////////////

package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// =============================================================================
// 部署状态常量
// =============================================================================

// DeploymentStatus 部署状态
type DeploymentStatus string

const (
	DeploymentStatusPlanned    DeploymentStatus = "planned"     // 已计划
	DeploymentStatusGray       DeploymentStatus = "gray"        // 灰度中
	DeploymentStatusActive     DeploymentStatus = "active"      // 已激活
	DeploymentStatusPaused     DeploymentStatus = "paused"      // 已暂停
	DeploymentStatusRolledBack DeploymentStatus = "rolled_back" // 已回滚
	DeploymentStatusFailed     DeploymentStatus = "failed"      // 失败
	DeploymentStatusCancelled  DeploymentStatus = "cancelled"   // 已取消
	DeploymentStatusSuperseded DeploymentStatus = "superseded"  // 已被新版本取代
)

// ValidDeploymentStatuses 有效的部署状态
var ValidDeploymentStatuses = map[DeploymentStatus]bool{
	DeploymentStatusPlanned:    true,
	DeploymentStatusGray:       true,
	DeploymentStatusActive:     true,
	DeploymentStatusPaused:     true,
	DeploymentStatusRolledBack: true,
	DeploymentStatusFailed:     true,
	DeploymentStatusCancelled:  true,
	DeploymentStatusSuperseded: true,
}

// IsValidDeploymentStatus 检查部署状态是否有效
func IsValidDeploymentStatus(s string) bool {
	return ValidDeploymentStatuses[DeploymentStatus(s)]
}

// =============================================================================
// 状态转换规则
// =============================================================================

// 状态转换矩阵：定义每个状态可以转换到哪些状态
var stateTransitions = map[DeploymentStatus][]DeploymentStatus{
	DeploymentStatusPlanned: {
		DeploymentStatusGray,      // 开始灰度
		DeploymentStatusActive,    // 直接激活
		DeploymentStatusCancelled, // 取消
	},
	DeploymentStatusGray: {
		DeploymentStatusActive,     // 灰度成功，激活
		DeploymentStatusRolledBack, // 灰度失败，回滚
		DeploymentStatusPaused,     // 暂停灰度
		DeploymentStatusFailed,     // 失败
	},
	DeploymentStatusActive: {
		DeploymentStatusRolledBack, // 回滚
		DeploymentStatusPaused,     // 暂停
		DeploymentStatusSuperseded, // 被新版本取代
	},
	DeploymentStatusPaused: {
		DeploymentStatusGray,       // 恢复灰度
		DeploymentStatusActive,     // 恢复激活
		DeploymentStatusRolledBack, // 回滚
		DeploymentStatusCancelled,  // 取消
	},
	DeploymentStatusRolledBack: {
		// 终态，不能转换
	},
	DeploymentStatusFailed: {
		DeploymentStatusRolledBack, // 失败后回滚
		DeploymentStatusCancelled,  // 取消
	},
	DeploymentStatusCancelled: {
		// 终态，不能转换
	},
	DeploymentStatusSuperseded: {
		// 终态，不能转换
	},
}

// CanTransition 检查是否可以进行状态转换
func CanTransition(from, to DeploymentStatus) bool {
	allowedStates, ok := stateTransitions[from]
	if !ok {
		return false
	}
	for _, allowed := range allowedStates {
		if allowed == to {
			return true
		}
	}
	return false
}

// ValidateTransition 验证状态转换，返回详细错误
func ValidateTransition(from, to DeploymentStatus) error {
	if !ValidDeploymentStatuses[from] {
		return fmt.Errorf("invalid source status: %s", from)
	}
	if !ValidDeploymentStatuses[to] {
		return fmt.Errorf("invalid target status: %s", to)
	}
	if !CanTransition(from, to) {
		return &StateTransitionError{
			From:    from,
			To:      to,
			Allowed: stateTransitions[from],
		}
	}
	return nil
}

// StateTransitionError 状态转换错误
type StateTransitionError struct {
	From    DeploymentStatus
	To      DeploymentStatus
	Allowed []DeploymentStatus
}

func (e *StateTransitionError) Error() string {
	return fmt.Sprintf("invalid state transition from %s to %s, allowed: %v",
		e.From, e.To, e.Allowed)
}

// IsFinalState 检查是否为终态
func IsFinalState(status DeploymentStatus) bool {
	finalStates := map[DeploymentStatus]bool{
		DeploymentStatusRolledBack: true,
		DeploymentStatusCancelled:  true,
		DeploymentStatusSuperseded: true,
	}
	return finalStates[status]
}

// =============================================================================
// 部署模型
// =============================================================================

// Deployment 部署模型（完整修复版）
type Deployment struct {
	DeploymentID string                 `json:"deployment_id" db:"deployment_id"`
	TenantID     string                 `json:"tenant_id" db:"tenant_id"`
	Name         string                 `json:"name" db:"name"`
	Description  string                 `json:"description" db:"description"`
	RuleVersion  string                 `json:"rule_version" db:"rule_version"`
	ModelVersion string                 `json:"model_version" db:"model_version"`
	FeatureSetID string                 `json:"feature_set_id" db:"feature_set_id"`
	Scope        map[string]interface{} `json:"scope" db:"-"`
	ScopeJSON    []byte                 `json:"-" db:"scope"`
	Status       string                 `json:"status" db:"status"`

	// ✅ 新增字段：灰度时间
	GrayStartedAt *time.Time `json:"gray_started_at,omitempty" db:"gray_started_at"`
	GrayExpiredAt *time.Time `json:"gray_expired_at,omitempty" db:"gray_expired_at"`

	// ✅ 新增字段：激活和回滚时间
	ActivatedAt  *time.Time `json:"activated_at,omitempty" db:"activated_at"`
	RolledBackAt *time.Time `json:"rolled_back_at,omitempty" db:"rolled_back_at"`

	// ✅ 新增字段：回滚关联
	RollbackFrom   *string `json:"rollback_from,omitempty" db:"rollback_from"`
	RollbackReason string  `json:"rollback_reason,omitempty" db:"rollback_reason"`

	// ✅ 新增字段：元数据
	Metadata     map[string]interface{} `json:"metadata,omitempty" db:"-"`
	MetadataJSON []byte                 `json:"-" db:"metadata"`

	// 灰度配置（可选，存储在 Metadata 或单独字段）
	GrayConfig *GrayConfig `json:"gray_config,omitempty" db:"-"`

	// 进度信息（运行时计算，不持久化）
	Progress *DeploymentProgress `json:"progress,omitempty" db:"-"`

	// 错误信息
	ErrorMessage string `json:"error_message,omitempty" db:"error_message"`

	// 审计字段
	CreatedBy string    `json:"created_by" db:"created_by"`
	UpdatedBy string    `json:"updated_by" db:"updated_by"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// GrayConfig 灰度配置
type GrayConfig struct {
	// 灰度范围
	Percentage  int      `json:"percentage"`        // 灰度百分比 (0-100)
	AssetGroups []string `json:"asset_groups"`      // 目标资产组
	Probes      []string `json:"probes"`            // 目标探针
	Regions     []string `json:"regions,omitempty"` // 目标区域

	// 灰度策略
	Strategy  string `json:"strategy"`             // canary, blue_green, rolling
	BatchSize int    `json:"batch_size,omitempty"` // 每批次数量
	Interval  int    `json:"interval_seconds"`     // 批次间隔（秒）

	// 自动化配置
	AutoRollback    bool    `json:"auto_rollback"`          // 自动回滚
	RollbackOnError float64 `json:"rollback_on_error_rate"` // 错误率阈值
	MinSuccessRate  float64 `json:"min_success_rate"`       // 最小成功率

	// 观察期
	ObservationMinutes int `json:"observation_minutes"` // 观察期（分钟）
}

// DefaultGrayConfig 默认灰度配置
func DefaultGrayConfig() *GrayConfig {
	return &GrayConfig{
		Percentage:         10,
		Strategy:           "canary",
		BatchSize:          1,
		Interval:           60,
		AutoRollback:       true,
		RollbackOnError:    0.05, // 5% 错误率触发回滚
		MinSuccessRate:     0.95, // 95% 成功率
		ObservationMinutes: 30,
	}
}

// Validate 验证灰度配置
func (g *GrayConfig) Validate() error {
	if g.Percentage < 0 || g.Percentage > 100 {
		return fmt.Errorf("percentage must be between 0 and 100")
	}
	if g.Strategy == "" {
		g.Strategy = "canary"
	}
	validStrategies := map[string]bool{"canary": true, "blue_green": true, "rolling": true}
	if !validStrategies[g.Strategy] {
		return fmt.Errorf("invalid strategy: %s", g.Strategy)
	}
	if g.RollbackOnError < 0 || g.RollbackOnError > 1 {
		return fmt.Errorf("rollback_on_error_rate must be between 0 and 1")
	}
	if g.MinSuccessRate < 0 || g.MinSuccessRate > 1 {
		return fmt.Errorf("min_success_rate must be between 0 and 1")
	}
	return nil
}

// DeploymentProgress 部署进度
type DeploymentProgress struct {
	Phase           string    `json:"phase"` // preparing, deploying, observing, completing
	CurrentBatch    int       `json:"current_batch"`
	TotalBatches    int       `json:"total_batches"`
	SuccessCount    int       `json:"success_count"`
	FailCount       int       `json:"fail_count"`
	PendingCount    int       `json:"pending_count"`
	ErrorRate       float64   `json:"error_rate"`
	SuccessRate     float64   `json:"success_rate"`
	StartTime       time.Time `json:"start_time"`
	LastUpdateTime  time.Time `json:"last_update_time"`
	EstimatedFinish time.Time `json:"estimated_finish,omitempty"`
}

// ✅ MarshalScope 序列化 Scope
func (d *Deployment) MarshalScope() error {
	if d.Scope == nil {
		d.Scope = make(map[string]interface{})
	}
	data, err := json.Marshal(d.Scope)
	if err != nil {
		return fmt.Errorf("failed to marshal scope: %w", err)
	}
	d.ScopeJSON = data
	return nil
}

// ✅ UnmarshalScope 反序列化 Scope
func (d *Deployment) UnmarshalScope() error {
	if len(d.ScopeJSON) == 0 {
		d.Scope = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal(d.ScopeJSON, &d.Scope)
}

// ✅ MarshalMetadata 序列化 Metadata
func (d *Deployment) MarshalMetadata() error {
	if d.Metadata == nil {
		d.Metadata = make(map[string]interface{})
	}
	data, err := json.Marshal(d.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	d.MetadataJSON = data
	return nil
}

// ✅ UnmarshalMetadata 反序列化 Metadata
func (d *Deployment) UnmarshalMetadata() error {
	if len(d.MetadataJSON) == 0 {
		d.Metadata = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal(d.MetadataJSON, &d.Metadata)
}

// Validate 验证部署
func (d *Deployment) Validate() error {
	var errors []string

	if d.TenantID == "" {
		errors = append(errors, "tenant_id is required")
	}
	if d.RuleVersion == "" && d.ModelVersion == "" {
		errors = append(errors, "rule_version or model_version is required")
	}

	// 验证状态
	if d.Status != "" && !IsValidDeploymentStatus(d.Status) {
		errors = append(errors, fmt.Sprintf("invalid status: %s", d.Status))
	}

	// 验证灰度配置
	if d.GrayConfig != nil {
		if err := d.GrayConfig.Validate(); err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		return &ValidationError{Errors: errors}
	}

	return nil
}

// SetDefaults 设置默认值
func (d *Deployment) SetDefaults() {
	if d.Status == "" {
		d.Status = string(DeploymentStatusPlanned)
	}
	if d.Scope == nil {
		d.Scope = make(map[string]interface{})
	}
	if d.Metadata == nil {
		d.Metadata = make(map[string]interface{})
	}
}

// CanStartGray 检查是否可以开始灰度
func (d *Deployment) CanStartGray() bool {
	return CanTransition(DeploymentStatus(d.Status), DeploymentStatusGray)
}

// CanActivate 检查是否可以激活
func (d *Deployment) CanActivate() bool {
	return CanTransition(DeploymentStatus(d.Status), DeploymentStatusActive)
}

// CanRollback 检查是否可以回滚
func (d *Deployment) CanRollback() bool {
	return CanTransition(DeploymentStatus(d.Status), DeploymentStatusRolledBack)
}

// CanPause 检查是否可以暂停
func (d *Deployment) CanPause() bool {
	return CanTransition(DeploymentStatus(d.Status), DeploymentStatusPaused)
}

// CanResume 检查是否可以恢复
func (d *Deployment) CanResume() bool {
	current := DeploymentStatus(d.Status)
	return current == DeploymentStatusPaused
}

// TransitionTo 转换到新状态
func (d *Deployment) TransitionTo(newStatus DeploymentStatus, updatedBy string) error {
	current := DeploymentStatus(d.Status)

	if err := ValidateTransition(current, newStatus); err != nil {
		return err
	}

	d.Status = string(newStatus)
	d.UpdatedBy = updatedBy
	d.UpdatedAt = time.Now()

	// 设置时间戳
	now := time.Now()
	switch newStatus {
	case DeploymentStatusGray:
		if d.GrayStartedAt == nil {
			d.GrayStartedAt = &now
		}
	case DeploymentStatusActive:
		if d.ActivatedAt == nil {
			d.ActivatedAt = &now
		}
	case DeploymentStatusRolledBack:
		d.RolledBackAt = &now
	}

	return nil
}

// =============================================================================
// 部署历史
// =============================================================================

// DeploymentHistory 部署历史记录
type DeploymentHistory struct {
	HistoryID    string                 `json:"history_id" db:"id"`
	DeploymentID string                 `json:"deployment_id" db:"deployment_id"`
	TenantID     string                 `json:"tenant_id" db:"tenant_id"`
	Action       string                 `json:"action" db:"action"`
	FromStatus   string                 `json:"from_status" db:"from_status"`
	ToStatus     string                 `json:"to_status" db:"to_status"`
	Reason       string                 `json:"reason" db:"reason"`
	OperatorID   string                 `json:"operator_id" db:"operator_id"`
	Detail       map[string]interface{} `json:"detail,omitempty" db:"-"`
	DetailJSON   []byte                 `json:"-" db:"detail"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
}

// MarshalDetail 序列化 Detail
func (h *DeploymentHistory) MarshalDetail() error {
	if h.Detail == nil {
		h.Detail = make(map[string]interface{})
	}
	data, err := json.Marshal(h.Detail)
	if err != nil {
		return err
	}
	h.DetailJSON = data
	return nil
}

// UnmarshalDetail 反序列化 Detail
func (h *DeploymentHistory) UnmarshalDetail() error {
	if len(h.DetailJSON) == 0 {
		h.Detail = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal(h.DetailJSON, &h.Detail)
}

// =============================================================================
// 部署命令
// =============================================================================

// DeploymentCommand 部署命令
type DeploymentCommand struct {
	Action     string      `json:"action"` // create, gray, activate, rollback, pause, resume, cancel
	Deployment *Deployment `json:"deployment"`
	GrayConfig *GrayConfig `json:"gray_config,omitempty"`
	Reason     string      `json:"reason,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
	OperatorID string      `json:"operator_id"`
	TraceID    string      `json:"trace_id,omitempty"`
}

// DeploymentAction 部署动作
type DeploymentAction string

const (
	DeployActionCreate   DeploymentAction = "create"
	DeployActionGray     DeploymentAction = "gray"
	DeployActionActivate DeploymentAction = "activate"
	DeployActionRollback DeploymentAction = "rollback"
	DeployActionPause    DeploymentAction = "pause"
	DeployActionResume   DeploymentAction = "resume"
	DeployActionCancel   DeploymentAction = "cancel"
)

// =============================================================================
// 列表请求/响应
// =============================================================================

// ListDeploymentsRequest 列表请求
type ListDeploymentsRequest struct {
	TenantID string   `json:"tenant_id"`
	RuleID   string   `json:"rule_id,omitempty"`
	Status   []string `json:"status,omitempty"`
	Limit    int      `json:"limit"`
	Offset   int      `json:"offset"`
}

// ListDeploymentsResponse 列表响应
type ListDeploymentsResponse struct {
	Deployments []*Deployment `json:"deployments"`
	Total       int           `json:"total"`
	Limit       int           `json:"limit"`
	Offset      int           `json:"offset"`
}

// ValidationError 验证错误
type ValidationError struct {
	Errors []string `json:"errors"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed: %v", e.Errors)
}
