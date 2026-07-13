////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/model/model_registry.go
// Model Registry 数据模型 - MLOps 模型注册与版本管理
//
// 对齐 PostgreSQL 表: models + model_versions (common/sql/pg/03-models-deploy.sql)
// 集成: Flink Behavior Job 模型热更新 + Argo Workflows 训练流水线
////////////////////////////////////////////////////////////////////////////////

package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// =============================================================================
// 模型状态常量
// =============================================================================

// ModelStatus 模型版本状态
type ModelStatus string

const (
	ModelStatusRegistered ModelStatus = "registered" // 已注册
	ModelStatusValidating ModelStatus = "validating" // 验证中
	ModelStatusActive     ModelStatus = "active"     // 已激活（生产中）
	ModelStatusDeprecated ModelStatus = "deprecated" // 已弃用
	ModelStatusFailed     ModelStatus = "failed"     // 验证失败
	ModelStatusArchived   ModelStatus = "archived"   // 已归档
)

// ValidModelStatuses 有效的模型状态
var ValidModelStatuses = map[ModelStatus]bool{
	ModelStatusRegistered: true,
	ModelStatusValidating: true,
	ModelStatusActive:     true,
	ModelStatusDeprecated: true,
	ModelStatusFailed:     true,
	ModelStatusArchived:   true,
}

// IsValidModelStatus 检查模型状态是否有效
func IsValidModelStatus(s string) bool {
	return ValidModelStatuses[ModelStatus(s)]
}

// 模型状态转换规则
var modelStatusTransitions = map[ModelStatus][]ModelStatus{
	ModelStatusRegistered: {ModelStatusValidating, ModelStatusActive, ModelStatusFailed, ModelStatusArchived},
	ModelStatusValidating: {ModelStatusActive, ModelStatusFailed, ModelStatusDeprecated},
	ModelStatusActive:     {ModelStatusDeprecated, ModelStatusArchived},
	ModelStatusDeprecated: {ModelStatusArchived},
	ModelStatusFailed:     {ModelStatusRegistered, ModelStatusArchived},
	ModelStatusArchived:   {}, // 终态
}

// CanTransitionModelStatus 检查模型状态转换是否合法
func CanTransitionModelStatus(from, to ModelStatus) bool {
	allowed, ok := modelStatusTransitions[from]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// =============================================================================
// 模型类型常量
// =============================================================================

// ModelType 模型类型
type ModelType string

const (
	ModelTypeXGBoost  ModelType = "xgboost"
	ModelTypeLightGBM ModelType = "lightgbm"
	ModelTypeONNX     ModelType = "onnx"
	ModelTypePMML     ModelType = "pmml"
	ModelTypeCustom   ModelType = "custom"
)

// ValidModelTypes 有效的模型类型
var ValidModelTypes = map[ModelType]bool{
	ModelTypeXGBoost:  true,
	ModelTypeLightGBM: true,
	ModelTypeONNX:     true,
	ModelTypePMML:     true,
	ModelTypeCustom:   true,
}

// IsValidModelType 检查模型类型是否有效
func IsValidModelType(s string) bool {
	return ValidModelTypes[ModelType(s)]
}

// =============================================================================
// 模型定义（对齐 models 表）
// =============================================================================

// Model 模型定义
type Model struct {
	ModelID      string                 `json:"model_id" db:"model_id"`
	TenantID     string                 `json:"tenant_id" db:"tenant_id"`
	Name         string                 `json:"name" db:"name"`
	ModelType    string                 `json:"model_type" db:"model_type"`
	Description  string                 `json:"description,omitempty" db:"description"`
	Metadata     map[string]interface{} `json:"metadata,omitempty" db:"-"`
	MetadataJSON []byte                 `json:"-" db:"metadata"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" db:"updated_at"`
}

// MarshalMetadata 序列化 Metadata
func (m *Model) MarshalMetadata() error {
	if m.Metadata == nil {
		m.Metadata = make(map[string]interface{})
	}
	data, err := json.Marshal(m.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal model metadata: %w", err)
	}
	m.MetadataJSON = data
	return nil
}

// UnmarshalMetadata 反序列化 Metadata
func (m *Model) UnmarshalMetadata() error {
	if len(m.MetadataJSON) == 0 {
		m.Metadata = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal(m.MetadataJSON, &m.Metadata)
}

// Validate 验证模型
func (m *Model) Validate() error {
	var errs []string
	if m.TenantID == "" {
		errs = append(errs, "tenant_id is required")
	}
	if m.Name == "" {
		errs = append(errs, "name is required")
	}
	if len(m.Name) > 256 {
		errs = append(errs, "name too long, max 256 characters")
	}
	if m.ModelType == "" {
		errs = append(errs, "model_type is required")
	}
	if !IsValidModelType(m.ModelType) {
		errs = append(errs, fmt.Sprintf("invalid model_type: %s", m.ModelType))
	}
	if len(m.Description) > 4096 {
		errs = append(errs, "description too long, max 4096 characters")
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// =============================================================================
// 模型版本定义（对齐 model_versions 表）
// =============================================================================

// ModelVersion 模型版本
type ModelVersion struct {
	ModelVersion string                 `json:"model_version" db:"model_version"`
	ModelID      string                 `json:"model_id" db:"model_id"`
	TenantID     string                 `json:"tenant_id" db:"tenant_id"`
	FeatureSetID string                 `json:"feature_set_id" db:"feature_set_id"`
	ArtifactURI  string                 `json:"artifact_uri" db:"artifact_uri"`
	Metrics      map[string]interface{} `json:"metrics,omitempty" db:"-"`
	MetricsJSON  []byte                 `json:"-" db:"metrics"`
	ModelType    string                 `json:"model_type,omitempty" db:"-"`
	Status       string                 `json:"status" db:"status"`
	CreatedBy    string                 `json:"created_by,omitempty" db:"created_by"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" db:"updated_at"`
	// 运行时填充字段（JOIN 查询结果）
	ModelName   string `json:"model_name,omitempty" db:"-"`
	Description string `json:"description,omitempty" db:"-"`
}

// MarshalMetrics 序列化 Metrics
func (mv *ModelVersion) MarshalMetrics() error {
	if mv.Metrics == nil {
		mv.Metrics = make(map[string]interface{})
	}
	data, err := json.Marshal(mv.Metrics)
	if err != nil {
		return fmt.Errorf("failed to marshal model version metrics: %w", err)
	}
	mv.MetricsJSON = data
	return nil
}

// UnmarshalMetrics 反序列化 Metrics
func (mv *ModelVersion) UnmarshalMetrics() error {
	if len(mv.MetricsJSON) == 0 {
		mv.Metrics = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal(mv.MetricsJSON, &mv.Metrics)
}

// Validate 验证模型版本
func (mv *ModelVersion) Validate() error {
	var errs []string
	if mv.ModelID == "" {
		errs = append(errs, "model_id is required")
	}
	if mv.TenantID == "" {
		errs = append(errs, "tenant_id is required")
	}
	if mv.FeatureSetID == "" {
		errs = append(errs, "feature_set_id is required")
	}
	if mv.ArtifactURI == "" {
		errs = append(errs, "artifact_uri is required")
	}
	if mv.Status != "" && !IsValidModelStatus(mv.Status) {
		errs = append(errs, fmt.Sprintf("invalid status: %s", mv.Status))
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// SetDefaults 设置默认值
func (mv *ModelVersion) SetDefaults() {
	if mv.Status == "" {
		mv.Status = string(ModelStatusRegistered)
	}
	if mv.Metrics == nil {
		mv.Metrics = make(map[string]interface{})
	}
}

// GetF1Score 从 Metrics 中提取 F1 Score
func (mv *ModelVersion) GetF1Score() (float64, bool) {
	if mv.Metrics == nil {
		return 0, false
	}
	if f1, ok := mv.Metrics["f1_score"]; ok {
		switch v := f1.(type) {
		case float64:
			return v, true
		case float32:
			return float64(v), true
		case int:
			return float64(v), true
		}
	}
	return 0, false
}

// =============================================================================
// 模型注册请求/响应（对应 register_model.py 的调用）
// =============================================================================

// RegisterModelRequest MLOps 训练流水线上报的模型注册请求
type RegisterModelRequest struct {
	ModelID      string                 `json:"model_id"`
	ModelType    string                 `json:"model_type"`
	Version      string                 `json:"version"`
	ArtifactURI  string                 `json:"artifact_uri"`
	FeatureSetID string                 `json:"feature_set_id"`
	TenantID     string                 `json:"tenant_id"`
	Metrics      map[string]interface{} `json:"metrics"`
	Status       string                 `json:"status,omitempty"`
	Description  string                 `json:"description,omitempty"`
}

// Validate 验证注册请求
func (r *RegisterModelRequest) Validate() error {
	var errs []string
	if r.ModelID == "" {
		errs = append(errs, "model_id is required")
	}
	if r.ModelType == "" {
		errs = append(errs, "model_type is required")
	}
	if r.Version == "" {
		errs = append(errs, "version is required")
	}
	if r.ArtifactURI == "" {
		errs = append(errs, "artifact_uri is required")
	}
	if r.FeatureSetID == "" {
		errs = append(errs, "feature_set_id is required")
	}
	if r.TenantID == "" {
		errs = append(errs, "tenant_id is required")
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// =============================================================================
// 模型列表请求/响应
// =============================================================================

// ModelFilter 模型过滤条件
type ModelFilter struct {
	ModelType string `json:"model_type,omitempty"`
	Keyword   string `json:"keyword,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	OrderBy   string `json:"order_by,omitempty"`
	OrderDir  string `json:"order_dir,omitempty"`
}

// ModelVersionFilter 模型版本过滤条件
type ModelVersionFilter struct {
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

// =============================================================================
// 模型摘要（用于 Dashboard 展示）
// =============================================================================

// ModelSummary 模型摘要信息
type ModelSummary struct {
	ModelID       string  `json:"model_id"`
	Name          string  `json:"name"`
	ModelType     string  `json:"model_type"`
	ActiveVersion string  `json:"active_version,omitempty"`
	TotalVersions int     `json:"total_versions"`
	BestF1Score   float64 `json:"best_f1_score,omitempty"`
	LastTrained   string  `json:"last_trained,omitempty"`
	Status        string  `json:"status"`
}
