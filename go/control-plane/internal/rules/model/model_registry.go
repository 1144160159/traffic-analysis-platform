////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/model/model_registry.go
// Model Registry 数据模型 - MLOps 模型注册与版本管理
//
// 对齐 PostgreSQL 表: models + model_versions (common/sql/pg/03-models-deploy.sql)
// 集成: Flink Behavior Job 模型热更新 + Argo Workflows 训练流水线
////////////////////////////////////////////////////////////////////////////////

package model

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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
	ModelVersion               string                 `json:"model_version" db:"model_version"`
	ModelID                    string                 `json:"model_id" db:"model_id"`
	TenantID                   string                 `json:"tenant_id" db:"tenant_id"`
	FeatureSetID               string                 `json:"feature_set_id" db:"feature_set_id"`
	ArtifactURI                string                 `json:"artifact_uri" db:"artifact_uri"`
	ArtifactManifestURI        string                 `json:"artifact_manifest_uri,omitempty" db:"artifact_manifest_uri"`
	PackageID                  string                 `json:"package_id,omitempty" db:"package_id"`
	PackageSHA256              string                 `json:"package_sha256,omitempty" db:"package_sha256"`
	ArtifactManifestSHA256     string                 `json:"artifact_manifest_sha256,omitempty" db:"artifact_manifest_sha256"`
	EvaluationSHA256           string                 `json:"evaluation_sha256,omitempty" db:"evaluation_sha256"`
	ExplanationSHA256          string                 `json:"explanation_sha256,omitempty" db:"explanation_sha256"`
	GraphSnapshotID            string                 `json:"graph_snapshot_id,omitempty" db:"graph_snapshot_id"`
	GraphSnapshotSHA256        string                 `json:"graph_snapshot_sha256,omitempty" db:"graph_snapshot_sha256"`
	SigningKeyID               string                 `json:"signing_key_id,omitempty" db:"signing_key_id"`
	Compatibility              map[string]interface{} `json:"compatibility,omitempty" db:"-"`
	CompatibilityJSON          []byte                 `json:"-" db:"compatibility"`
	Revision                   int64                  `json:"revision" db:"revision"`
	RegistrationIdempotencyKey string                 `json:"-" db:"registration_idempotency_key"`
	RegistrationRequestSHA256  string                 `json:"registration_request_sha256,omitempty" db:"registration_request_sha256"`
	Metrics                    map[string]interface{} `json:"metrics,omitempty" db:"-"`
	MetricsJSON                []byte                 `json:"-" db:"metrics"`
	ModelType                  string                 `json:"model_type,omitempty" db:"-"`
	Status                     string                 `json:"status" db:"status"`
	CreatedBy                  string                 `json:"created_by,omitempty" db:"created_by"`
	CreatedAt                  time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt                  time.Time              `json:"updated_at" db:"updated_at"`
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

// UnmarshalCompatibility decodes the immutable runtime compatibility contract.
func (mv *ModelVersion) UnmarshalCompatibility() error {
	if len(mv.CompatibilityJSON) == 0 {
		mv.Compatibility = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal(mv.CompatibilityJSON, &mv.Compatibility)
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
	ModelID                string                 `json:"model_id"`
	ModelType              string                 `json:"model_type"`
	Version                string                 `json:"version"`
	ArtifactURI            string                 `json:"artifact_uri"`
	ArtifactManifestURI    string                 `json:"artifact_manifest_uri,omitempty"`
	PackageID              string                 `json:"package_id,omitempty"`
	PackageSHA256          string                 `json:"package_sha256,omitempty"`
	ArtifactManifestSHA256 string                 `json:"artifact_manifest_sha256,omitempty"`
	EvaluationSHA256       string                 `json:"evaluation_sha256,omitempty"`
	ExplanationSHA256      string                 `json:"explanation_sha256,omitempty"`
	GraphSnapshotID        string                 `json:"graph_snapshot_id,omitempty"`
	GraphSnapshotSHA256    string                 `json:"graph_snapshot_sha256,omitempty"`
	SigningKeyID           string                 `json:"signing_key_id,omitempty"`
	Compatibility          map[string]interface{} `json:"compatibility,omitempty"`
	GovernanceVersion      string                 `json:"governance_version,omitempty"`
	ExpectedRevision       *int64                 `json:"expected_revision"`
	IdempotencyKey         string                 `json:"-"`
	FeatureSetID           string                 `json:"feature_set_id"`
	TenantID               string                 `json:"tenant_id"`
	Metrics                map[string]interface{} `json:"metrics"`
	Status                 string                 `json:"status,omitempty"`
	Description            string                 `json:"description,omitempty"`
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
	if len(strings.TrimSpace(r.IdempotencyKey)) < 16 || len(strings.TrimSpace(r.IdempotencyKey)) > 200 {
		errs = append(errs, "Idempotency-Key must contain 16 to 200 characters")
	}
	if r.ExpectedRevision == nil || *r.ExpectedRevision != 0 {
		errs = append(errs, "expected_revision must be present and equal to 0 for model registration")
	}
	if r.Status != "" && r.Status != string(ModelStatusRegistered) {
		errs = append(errs, "model registration status must be registered")
	}
	if r.GovernanceVersion != "" && r.GovernanceVersion != "model-registration.v1" {
		errs = append(errs, "unsupported governance_version")
	}
	if r.GovernanceVersion == "model-registration.v1" {
		required := map[string]string{
			"artifact_manifest_uri":    r.ArtifactManifestURI,
			"package_id":               r.PackageID,
			"package_sha256":           r.PackageSHA256,
			"artifact_manifest_sha256": r.ArtifactManifestSHA256,
			"evaluation_sha256":        r.EvaluationSHA256,
			"explanation_sha256":       r.ExplanationSHA256,
			"graph_snapshot_id":        r.GraphSnapshotID,
			"graph_snapshot_sha256":    r.GraphSnapshotSHA256,
			"signing_key_id":           r.SigningKeyID,
		}
		for field, value := range required {
			if strings.TrimSpace(value) == "" {
				errs = append(errs, field+" is required for governed model registration")
			}
		}
		for field, value := range map[string]string{
			"package_sha256":           r.PackageSHA256,
			"artifact_manifest_sha256": r.ArtifactManifestSHA256,
			"evaluation_sha256":        r.EvaluationSHA256,
			"explanation_sha256":       r.ExplanationSHA256,
			"graph_snapshot_sha256":    r.GraphSnapshotSHA256,
		} {
			if !isLowerSHA256(value) {
				errs = append(errs, field+" must be a lowercase SHA-256 digest")
			}
		}
		if len(r.Compatibility) == 0 {
			errs = append(errs, "compatibility is required for governed model registration")
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

func isLowerSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
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
