////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/service/model_service.go
// Model Service — 模型注册、版本管理、Kafka 热更新通知
//
// 集成: MLOps Argo Workflows → register_model.py → Model Registry API → Kafka → Flink
////////////////////////////////////////////////////////////////////////////////

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/publisher"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/rbac"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/repository"
)

// =============================================================================
// ModelServiceConfig
// =============================================================================

// ModelServiceConfig 模型服务配置
type ModelServiceConfig struct {
	MaxModelsPerTenant      int  `env:"MODEL_MAX_PER_TENANT" envDefault:"50"`
	MaxVersionsPerModel     int  `env:"MODEL_MAX_VERSIONS_PER_MODEL" envDefault:"100"`
	EnableKafkaNotification bool `env:"MODEL_ENABLE_KAFKA_NOTIFICATION" envDefault:"true"`
	AutoActivateNewVersion  bool `env:"MODEL_AUTO_ACTIVATE_NEW_VERSION" envDefault:"false"`
}

// DefaultModelServiceConfig 默认配置
func DefaultModelServiceConfig() ModelServiceConfig {
	return ModelServiceConfig{
		MaxModelsPerTenant:      50,
		MaxVersionsPerModel:     100,
		EnableKafkaNotification: true,
		AutoActivateNewVersion:  false,
	}
}

// =============================================================================
// ModelService
// =============================================================================

// ModelService 模型服务
type ModelService struct {
	db          *sql.DB
	repo        *repository.ModelRepository
	publisher   *publisher.KafkaPublisher
	auditLogger *audit.Logger
	rbacChecker *rbac.Checker
	config      ModelServiceConfig
	logger      *zap.Logger
}

// NewModelService 创建模型服务
func NewModelService(
	db *sql.DB,
	pub *publisher.KafkaPublisher,
	auditLogger *audit.Logger,
	rbacChecker *rbac.Checker,
	logger *zap.Logger,
	config ModelServiceConfig,
) *ModelService {
	return &ModelService{
		db:          db,
		repo:        repository.NewModelRepository(db, logger),
		publisher:   pub,
		auditLogger: auditLogger,
		rbacChecker: rbacChecker,
		config:      config,
		logger:      logger,
	}
}

// =============================================================================
// 模型 CRUD
// =============================================================================

// CreateModel 创建模型
func (s *ModelService) CreateModel(ctx context.Context, m *model.Model, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "ModelService.CreateModel")
	defer span.End()

	// 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelCreate, m.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelCreate, "model", "", err.Error())
		return err
	}

	// 验证
	if err := m.Validate(); err != nil {
		return err
	}

	// 限额检查
	var count int
	countQuery := `SELECT COUNT(*) FROM models WHERE tenant_id = $1`
	if err := s.db.QueryRowContext(ctx, countQuery, m.TenantID).Scan(&count); err == nil {
		if count >= s.config.MaxModelsPerTenant {
			return errors.Newf(errors.ErrCodeQuotaExceeded, "model limit exceeded: max %d per tenant", s.config.MaxModelsPerTenant)
		}
	}

	// 创建
	if err := s.repo.CreateModel(ctx, m); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelCreate, "model", m.ModelID, err.Error())
		return err
	}

	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeModelCreate, "model", m.ModelID, map[string]interface{}{
		"name":       m.Name,
		"model_type": m.ModelType,
	})

	s.logger.Info("Model created", zap.String("model_id", m.ModelID), zap.String("name", m.Name))
	return nil
}

// GetModel 获取模型
func (s *ModelService) GetModel(ctx context.Context, modelID string, opCtx *OperationContext) (*model.Model, error) {
	ctx, span := otel.StartSpan(ctx, "ModelService.GetModel")
	defer span.End()

	m, err := s.repo.GetModel(ctx, modelID)
	if err != nil {
		return nil, err
	}

	if err := s.checkPermission(ctx, opCtx, rbac.PermModelRead, m.TenantID); err != nil {
		return nil, err
	}
	return m, nil
}

// ListModels 列出模型
func (s *ModelService) ListModels(ctx context.Context, tenantID string, filter *model.ModelFilter, opCtx *OperationContext) ([]*model.Model, int64, error) {
	ctx, span := otel.StartSpan(ctx, "ModelService.ListModels")
	defer span.End()

	if err := s.checkPermission(ctx, opCtx, rbac.PermModelRead, tenantID); err != nil {
		return nil, 0, err
	}
	return s.repo.ListModels(ctx, tenantID, filter)
}

// UpdateModel 更新模型
func (s *ModelService) UpdateModel(ctx context.Context, m *model.Model, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "ModelService.UpdateModel")
	defer span.End()

	if err := s.checkPermission(ctx, opCtx, rbac.PermModelWrite, m.TenantID); err != nil {
		return err
	}
	if err := m.Validate(); err != nil {
		return err
	}
	return s.repo.UpdateModel(ctx, m)
}

// DeleteModel 删除模型
func (s *ModelService) DeleteModel(ctx context.Context, tenantID, modelID string, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "ModelService.DeleteModel")
	defer span.End()

	if err := s.checkPermission(ctx, opCtx, rbac.PermModelDelete, tenantID); err != nil {
		return err
	}
	return s.repo.DeleteModel(ctx, tenantID, modelID)
}

// =============================================================================
// 模型版本管理
// =============================================================================

// RegisterModelVersion 注册模型版本（MLOps pipeline 调用入口）
func (s *ModelService) RegisterModelVersion(ctx context.Context, req *model.RegisterModelRequest, opCtx *OperationContext) (*model.ModelVersion, error) {
	ctx, span := otel.StartSpan(ctx, "ModelService.RegisterModelVersion")
	defer span.End()

	// 验证请求
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelCreate, req.TenantID); err != nil {
		return nil, err
	}

	// 查找或创建模型。页面通常传 UUID, MLOps pipeline 通常传模型名称。
	var modelObj *model.Model
	var err error
	if _, parseErr := uuid.Parse(req.ModelID); parseErr == nil {
		modelObj, err = s.repo.GetModel(ctx, req.ModelID)
	} else {
		modelObj, err = s.repo.GetModelByName(ctx, req.TenantID, req.ModelID)
	}
	if err != nil && errors.IsCode(err, errors.ErrCodeModelNotFound) {
		modelObj, err = s.repo.GetModelByName(ctx, req.TenantID, req.ModelID)
	}
	if err != nil && errors.IsCode(err, errors.ErrCodeModelNotFound) {
		// 模型不存在时按 pipeline 提供的名称自动创建。
		modelObj = &model.Model{
			TenantID:    req.TenantID,
			Name:        req.ModelID,
			ModelType:   req.ModelType,
			Description: req.Description,
			Metadata: map[string]interface{}{
				"source":       "mlops-pipeline",
				"auto_created": true,
			},
		}
		if err := s.repo.CreateModel(ctx, modelObj); err != nil {
			return nil, err
		}
		s.logger.Info("Auto-created model", zap.String("model_id", modelObj.ModelID), zap.String("name", req.ModelID))
	} else if err != nil {
		return nil, err
	} else if modelObj.TenantID != req.TenantID {
		return nil, errors.New(errors.ErrCodePermissionDenied, "cross-tenant model registration denied")
	}

	// 版本限额检查
	_, versionCount, err := s.repo.ListModelVersions(ctx, req.TenantID, modelObj.ModelID, &model.ModelVersionFilter{Limit: 1})
	if err == nil && int(versionCount) >= s.config.MaxVersionsPerModel {
		return nil, errors.Newf(errors.ErrCodeQuotaExceeded, "model version limit exceeded: max %d per model", s.config.MaxVersionsPerModel)
	}

	// 生成版本号
	version := req.Version
	if version == "" {
		version = time.Now().UTC().Format("v20060102_150405")
	}

	// 构建模型版本
	mv := &model.ModelVersion{
		ModelVersion: version,
		ModelID:      modelObj.ModelID,
		TenantID:     req.TenantID,
		FeatureSetID: req.FeatureSetID,
		ArtifactURI:  req.ArtifactURI,
		Metrics:      req.Metrics,
		Status:       string(model.ModelStatusRegistered),
		CreatedBy:    opCtx.UserID,
	}

	if mv.Status == "" {
		mv.Status = string(model.ModelStatusRegistered)
	}

	// 创建版本
	if err := s.repo.CreateModelVersion(ctx, mv); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionCreate, "model_version", version, err.Error())
		return nil, err
	}

	// 填充关联信息
	mv.ModelName = modelObj.Name
	mv.ModelType = modelObj.ModelType

	// 发布 Kafka 模型更新通知
	if s.config.EnableKafkaNotification {
		go s.publishModelUpdateEvent(context.Background(), modelObj, mv, "registered")
	}

	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeModelVersionCreate, "model_version", version, map[string]interface{}{
		"model_id":     modelObj.ModelID,
		"model_name":   modelObj.Name,
		"artifact_uri": mv.ArtifactURI,
		"f1_score":     s.getF1FromMetrics(mv.Metrics),
	})

	s.logger.Info("Model version registered",
		zap.String("model_name", modelObj.Name),
		zap.String("version", version),
		zap.String("status", mv.Status))

	return mv, nil
}

// ActivateModelVersion 激活模型版本（部署到生产）
func (s *ModelService) ActivateModelVersion(ctx context.Context, expectedModelID, modelVersion string, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "ModelService.ActivateModelVersion")
	defer span.End()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to start model activation transaction")
	}
	defer tx.Rollback()

	mv, err := s.getModelVersionForUpdate(ctx, tx, modelVersion)
	if err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return err
	}

	if err := s.checkPermission(ctx, opCtx, rbac.PermModelActivate, mv.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return err
	}
	if expectedModelID != "" && mv.ModelID != expectedModelID {
		err := errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found under model: %s", expectedModelID)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return err
	}

	currentStatus := model.ModelStatus(mv.Status)
	if !model.CanTransitionModelStatus(currentStatus, model.ModelStatusActive) {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition,
			"cannot transition model version from %s to %s", currentStatus, model.ModelStatusActive)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return err
	}

	// 弃用其他激活版本
	if err := s.deprecateOtherVersionsTx(ctx, tx, mv.ModelID, modelVersion); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return err
	}

	// 激活当前版本
	if err := s.updateModelVersionStatusTx(ctx, tx, modelVersion, model.ModelStatusActive); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return err
	}
	if err := tx.Commit(); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model activation")
	}

	// 发布 Kafka 热更新通知
	if s.config.EnableKafkaNotification {
		mv.Status = string(model.ModelStatusActive)
		go s.publishModelUpdateEvent(context.Background(), nil, mv, "activated")
	}

	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, map[string]interface{}{
		"model_id":        mv.ModelID,
		"previous_status": string(currentStatus),
		"new_status":      string(model.ModelStatusActive),
	})

	s.logger.Info("Model version activated", zap.String("version", modelVersion))
	return nil
}

// DeprecateModelVersion 弃用模型版本
func (s *ModelService) DeprecateModelVersion(ctx context.Context, expectedModelID, modelVersion string, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "ModelService.DeprecateModelVersion")
	defer span.End()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, err.Error())
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to start model deprecate transaction")
	}
	defer tx.Rollback()

	mv, err := s.getModelVersionForUpdate(ctx, tx, modelVersion)
	if err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, err.Error())
		return err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelActivate, mv.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, err.Error())
		return err
	}
	if expectedModelID != "" && mv.ModelID != expectedModelID {
		err := errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found under model: %s", expectedModelID)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, err.Error())
		return err
	}

	currentStatus := model.ModelStatus(mv.Status)
	if !model.CanTransitionModelStatus(currentStatus, model.ModelStatusDeprecated) {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition,
			"cannot transition model version from %s to %s", currentStatus, model.ModelStatusDeprecated)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, err.Error())
		return err
	}

	if err := s.updateModelVersionStatusTx(ctx, tx, modelVersion, model.ModelStatusDeprecated); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, err.Error())
		return err
	}
	if err := tx.Commit(); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, err.Error())
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model deprecate")
	}

	if s.config.EnableKafkaNotification {
		mv.Status = string(model.ModelStatusDeprecated)
		go s.publishModelUpdateEvent(context.Background(), nil, mv, "deprecated")
	}

	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, map[string]interface{}{
		"model_id":        mv.ModelID,
		"previous_status": string(currentStatus),
		"new_status":      string(model.ModelStatusDeprecated),
	})

	s.logger.Info("Model version deprecated", zap.String("version", modelVersion))
	return nil
}

// GetModelVersion 获取模型版本详情
func (s *ModelService) GetModelVersion(ctx context.Context, modelVersion string, opCtx *OperationContext) (*model.ModelVersion, error) {
	ctx, span := otel.StartSpan(ctx, "ModelService.GetModelVersion")
	defer span.End()

	mv, err := s.repo.GetModelVersion(ctx, modelVersion)
	if err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelRead, mv.TenantID); err != nil {
		return nil, err
	}
	return mv, nil
}

// ListModelVersions 列出模型版本
func (s *ModelService) ListModelVersions(ctx context.Context, tenantID, modelID string, filter *model.ModelVersionFilter, opCtx *OperationContext) ([]*model.ModelVersion, int64, error) {
	ctx, span := otel.StartSpan(ctx, "ModelService.ListModelVersions")
	defer span.End()

	if err := s.checkPermission(ctx, opCtx, rbac.PermModelRead, tenantID); err != nil {
		return nil, 0, err
	}
	return s.repo.ListModelVersions(ctx, tenantID, modelID, filter)
}

// GetActiveModelVersion 获取模型的激活版本
func (s *ModelService) GetActiveModelVersion(ctx context.Context, modelID string, opCtx *OperationContext) (*model.ModelVersion, error) {
	ctx, span := otel.StartSpan(ctx, "ModelService.GetActiveModelVersion")
	defer span.End()

	mv, err := s.repo.GetActiveModelVersion(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelRead, mv.TenantID); err != nil {
		return nil, err
	}
	return mv, nil
}

// GetModelSummary 获取模型摘要
func (s *ModelService) GetModelSummary(ctx context.Context, tenantID, modelID string, opCtx *OperationContext) (*model.ModelSummary, error) {
	ctx, span := otel.StartSpan(ctx, "ModelService.GetModelSummary")
	defer span.End()

	if err := s.checkPermission(ctx, opCtx, rbac.PermModelRead, tenantID); err != nil {
		return nil, err
	}
	return s.repo.GetModelSummary(ctx, tenantID, modelID)
}

// =============================================================================
// Kafka 模型更新通知
// =============================================================================

// ModelUpdateEvent Kafka 模型更新事件
type ModelUpdateEvent struct {
	ModelID     string                 `json:"model_id"`
	ModelName   string                 `json:"model_name"`
	ModelType   string                 `json:"model_type"`
	Version     string                 `json:"version"`
	ArtifactURI string                 `json:"artifact_uri"`
	Action      string                 `json:"action"` // registered, activated, deprecated
	Metrics     map[string]interface{} `json:"metrics,omitempty"`
	Timestamp   string                 `json:"timestamp"`
}

// publishModelUpdateEvent 发布模型更新事件到 Kafka
func (s *ModelService) publishModelUpdateEvent(ctx context.Context, modelObj *model.Model, mv *model.ModelVersion, action string) {
	if s.publisher == nil {
		return
	}

	publishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	event := &ModelUpdateEvent{
		ModelID:     mv.ModelID,
		ModelName:   mv.ModelName,
		ModelType:   mv.ModelType,
		Version:     mv.ModelVersion,
		ArtifactURI: mv.ArtifactURI,
		Action:      action,
		Metrics:     mv.Metrics,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	if modelObj != nil {
		event.ModelName = modelObj.Name
		event.ModelType = modelObj.ModelType
	}

	eventJSON, _ := json.Marshal(event)

	if err := s.publisher.PublishModelUpdate(publishCtx, fmt.Sprintf("%s:%s", mv.ModelID, action), eventJSON); err != nil {
		s.logger.Error("Failed to publish model update event",
			zap.String("model_id", mv.ModelID),
			zap.String("action", action),
			zap.Error(err))
		return
	}

	s.logger.Info("Published model update event to Kafka",
		zap.String("model_id", mv.ModelID),
		zap.String("version", mv.ModelVersion),
		zap.String("action", action))
}

// =============================================================================
// 搜索
// =============================================================================

// SearchModels 搜索模型
func (s *ModelService) SearchModels(ctx context.Context, tenantID, query string, opCtx *OperationContext) ([]*model.Model, error) {
	ctx, span := otel.StartSpan(ctx, "ModelService.SearchModels")
	defer span.End()

	if err := s.checkPermission(ctx, opCtx, rbac.PermModelRead, tenantID); err != nil {
		return nil, err
	}
	return s.repo.SearchModels(ctx, tenantID, query, 20)
}

// =============================================================================
// 辅助方法
// =============================================================================

func (s *ModelService) checkPermission(ctx context.Context, opCtx *OperationContext, permission rbac.Permission, resourceTenantID string) error {
	if opCtx == nil {
		return errors.New(errors.ErrCodeUnauthorized, "operation context required")
	}
	if opCtx.TenantID != resourceTenantID && !s.hasAdminPermission(opCtx) {
		return errors.New(errors.ErrCodePermissionDenied, "cross-tenant access denied")
	}
	if s.rbacChecker == nil {
		return nil
	}
	if !s.rbacChecker.HasPermission(opCtx.Permissions, permission) {
		return errors.Newf(errors.ErrCodePermissionDenied, "permission denied: %s required", permission)
	}
	return nil
}

func (s *ModelService) hasAdminPermission(opCtx *OperationContext) bool {
	for _, p := range opCtx.Permissions {
		if p == string(rbac.PermissionAdminWrite) || p == "admin:*" || p == "*" {
			return true
		}
	}
	return false
}

func (s *ModelService) getF1FromMetrics(metrics map[string]interface{}) float64 {
	if metrics == nil {
		return 0
	}
	if f1, ok := metrics["f1_score"]; ok {
		switch v := f1.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case int:
			return float64(v)
		}
	}
	return 0
}

func (s *ModelService) recordAuditSuccess(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID string, detail map[string]interface{}) {
	if opCtx == nil {
		return
	}

	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    eventType,
			TenantID:     opCtx.TenantID,
			UserID:       opCtx.UserID,
			Username:     opCtx.Username,
			Action:       string(eventType),
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Detail:       detail,
			Result:       audit.ResultSuccess,
			IPAddr:       opCtx.IPAddr,
			UserAgent:    opCtx.UserAgent,
		})
	}
	s.recordAuditLogDB(ctx, opCtx, eventType, resourceType, resourceID, detail, audit.ResultSuccess, "")
}

func (s *ModelService) recordAuditFailure(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID, errorMsg string) {
	if opCtx == nil {
		return
	}

	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    eventType,
			TenantID:     opCtx.TenantID,
			UserID:       opCtx.UserID,
			Username:     opCtx.Username,
			Action:       string(eventType) + "_failed",
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Result:       audit.ResultFailure,
			ErrorMsg:     errorMsg,
			IPAddr:       opCtx.IPAddr,
			UserAgent:    opCtx.UserAgent,
		})
	}
	s.recordAuditLogDB(ctx, opCtx, eventType, resourceType, resourceID, nil, audit.ResultFailure, errorMsg)
}

func (s *ModelService) getModelVersionForUpdate(ctx context.Context, tx *sql.Tx, modelVersion string) (*model.ModelVersion, error) {
	query := `
		SELECT mv.model_version, mv.model_id, mv.tenant_id, mv.feature_set_id,
		       mv.artifact_uri, mv.metrics, mv.status, mv.created_by,
		       mv.created_at, mv.updated_at,
		       m.name, m.model_type, m.description
		FROM model_versions mv
		LEFT JOIN models m ON m.model_id = mv.model_id
		WHERE mv.model_version = $1
		FOR UPDATE OF mv
	`

	var mv model.ModelVersion
	var createdBy, modelName, modelType, description sql.NullString
	if err := tx.QueryRowContext(ctx, query, modelVersion).Scan(
		&mv.ModelVersion, &mv.ModelID, &mv.TenantID, &mv.FeatureSetID,
		&mv.ArtifactURI, &mv.MetricsJSON, &mv.Status, &createdBy,
		&mv.CreatedAt, &mv.UpdatedAt,
		&modelName, &modelType, &description,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found: %s", modelVersion)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock model version")
	}
	if createdBy.Valid {
		mv.CreatedBy = createdBy.String
	}
	if modelName.Valid {
		mv.ModelName = modelName.String
	}
	if modelType.Valid {
		mv.ModelType = modelType.String
	}
	if description.Valid {
		mv.Description = description.String
	}
	_ = mv.UnmarshalMetrics()
	return &mv, nil
}

func (s *ModelService) updateModelVersionStatusTx(ctx context.Context, tx *sql.Tx, modelVersion string, status model.ModelStatus) error {
	query := `UPDATE model_versions SET status = $1, updated_at = $2 WHERE model_version = $3`
	result, err := tx.ExecContext(ctx, query, string(status), time.Now(), modelVersion)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update model version status")
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found: %s", modelVersion)
	}
	return nil
}

func (s *ModelService) deprecateOtherVersionsTx(ctx context.Context, tx *sql.Tx, modelID, excludeVersion string) error {
	query := `UPDATE model_versions SET status = $1, updated_at = $2
		WHERE model_id = $3 AND status = 'active' AND model_version != $4`
	if _, err := tx.ExecContext(ctx, query, string(model.ModelStatusDeprecated), time.Now(), modelID, excludeVersion); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to deprecate previous active model versions")
	}
	return nil
}

func (s *ModelService) recordAuditLogDB(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID string, detail map[string]interface{}, result audit.Result, errorMsg string) {
	if s.db == nil || opCtx == nil {
		return
	}

	detailCopy := make(map[string]interface{}, len(detail)+2)
	for k, v := range detail {
		detailCopy[k] = v
	}
	if result != "" {
		detailCopy["result"] = string(result)
	}
	if errorMsg != "" {
		detailCopy["error"] = errorMsg
	}

	detailJSON, err := json.Marshal(detailCopy)
	if err != nil {
		s.logger.Warn("Failed to marshal model audit detail",
			zap.String("event_type", string(eventType)),
			zap.String("resource_id", resourceID),
			zap.Error(err))
		detailJSON = []byte("{}")
	}

	action := string(eventType)
	if result == audit.ResultFailure {
		action += "_failed"
	}

	query := `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
	`
	if _, err := s.db.ExecContext(ctx, query,
		opCtx.TenantID,
		opCtx.UserID,
		action,
		resourceType,
		resourceID,
		string(detailJSON),
		opCtx.IPAddr,
		opCtx.UserAgent,
	); err != nil {
		s.logger.Warn("Failed to persist model audit log",
			zap.String("event_type", string(eventType)),
			zap.String("resource_type", resourceType),
			zap.String("resource_id", resourceID),
			zap.Error(err))
	}
}

// ensureUUID generates a UUID string if not already one
func ensureUUID(s string) string {
	if s == "" {
		return uuid.New().String()
	}
	return s
}
