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
	"strings"
	"sync"
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
	MaxModelsPerTenant            int  `env:"MODEL_MAX_PER_TENANT" envDefault:"50"`
	MaxVersionsPerModel           int  `env:"MODEL_MAX_VERSIONS_PER_MODEL" envDefault:"100"`
	AppliedAckExpectedParallelism int  `env:"MODEL_APPLIED_ACK_EXPECTED_PARALLELISM" envDefault:"4"`
	EnableKafkaNotification       bool `env:"MODEL_ENABLE_KAFKA_NOTIFICATION" envDefault:"true"`
	AutoActivateNewVersion        bool `env:"MODEL_AUTO_ACTIVATE_NEW_VERSION" envDefault:"false"`
}

// DefaultModelServiceConfig 默认配置
func DefaultModelServiceConfig() ModelServiceConfig {
	return ModelServiceConfig{
		MaxModelsPerTenant:            50,
		MaxVersionsPerModel:           100,
		AppliedAckExpectedParallelism: 4,
		EnableKafkaNotification:       true,
		AutoActivateNewVersion:        false,
	}
}

// =============================================================================
// ModelService
// =============================================================================

// ModelService 模型服务
type ModelService struct {
	db             *sql.DB
	repo           *repository.ModelRepository
	publisher      *publisher.KafkaPublisher
	auditLogger    *audit.Logger
	rbacChecker    *rbac.Checker
	config         ModelServiceConfig
	logger         *zap.Logger
	workerCancel   context.CancelFunc
	workerWG       sync.WaitGroup
	outboxWorkerID string
}

// StartActionWorker starts the durable model-action dispatcher. It is safe to
// call once per service instance; database row claims coordinate replicas.
func (s *ModelService) StartActionWorker(parent context.Context) {
	if s.workerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.workerCancel = cancel
	if err := s.repo.RecoverStaleModelActions(ctx, 5*time.Minute); err != nil {
		s.logger.Warn("Failed to recover stale model actions", zap.Error(err))
	}
	s.workerWG.Add(1)
	go func() {
		defer s.workerWG.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		lastRecovery := time.Now()
		for {
			if err := s.processModelUpdateOutbox(ctx); err != nil {
				s.logger.Error("Model update outbox dispatch failed", zap.Error(err))
			}
			if err := s.dispatchNextModelAction(ctx); err != nil {
				s.logger.Error("Model action dispatch failed", zap.Error(err))
			}
			if time.Since(lastRecovery) >= time.Minute {
				if err := s.repo.RecoverStaleModelActions(ctx, 5*time.Minute); err != nil {
					s.logger.Warn("Failed to recover stale model actions", zap.Error(err))
				}
				lastRecovery = time.Now()
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *ModelService) Close() {
	if s.workerCancel != nil {
		s.workerCancel()
		s.workerWG.Wait()
		s.workerCancel = nil
	}
}

func (s *ModelService) dispatchNextModelAction(ctx context.Context) error {
	job, err := s.repo.ClaimNextModelAction(ctx)
	if err != nil || job == nil {
		return err
	}

	auditAction := "MODEL_ACTION_DISPATCHED"
	if job.Action == "inspect-context" {
		auditAction = "MODEL_CONTEXT_INSPECTION_COMPLETED"
		return s.repo.FinishModelAction(ctx, job, "completed", auditAction, "")
	}
	if job.Action == "rollback-version" {
		if s.config.EnableKafkaNotification && s.publisher == nil {
			err := errors.New(errors.ErrCodeServiceUnavailable, "model update publisher is not configured")
			return s.repo.FinishModelAction(ctx, job, "failed", "MODEL_VERSION_ROLLBACK_FAILED", err.Error())
		}
		opCtx := &OperationContext{TenantID: job.TenantID, UserID: job.RequestedBy, Username: "model-action-worker", Permissions: []string{"*"}, Authenticated: true}
		if err := s.rollbackModelVersion(ctx, job.ModelID, job.Version, opCtx, job); err != nil {
			if finishErr := s.repo.FinishModelAction(ctx, job, "failed", "MODEL_VERSION_ROLLBACK_FAILED", err.Error()); finishErr != nil {
				return finishErr
			}
			return nil
		}
		// The state transaction also wrote a linked model_update_outbox row.
		// Broker acknowledgement atomically closes that row, the durable job and
		// MODEL_VERSION_ROLLBACK_COMPLETED on the next outbox pass.
		return nil
	}
	if s.publisher == nil {
		err := errors.New(errors.ErrCodeServiceUnavailable, "model action publisher is not configured")
		if finishErr := s.repo.FinishModelAction(ctx, job, "failed", "MODEL_ACTION_DISPATCH_FAILED", err.Error()); finishErr != nil {
			return finishErr
		}
		return nil
	}
	event, err := json.Marshal(map[string]interface{}{
		"event_type": "model_action_requested",
		"job_id":     job.JobID, "action_id": job.ActionID, "tenant_id": job.TenantID,
		"model_id": job.ModelID, "version": job.Version, "action": job.Action,
		"target": job.Target, "payload": job.Payload, "requested_by": job.RequestedBy,
		"requested_at": job.CreatedAt,
	})
	if err == nil {
		err = s.publisher.PublishModelAction(ctx, job.ModelID, event)
	}
	if err != nil {
		if finishErr := s.repo.FinishModelAction(ctx, job, "failed", "MODEL_ACTION_DISPATCH_FAILED", err.Error()); finishErr != nil {
			return finishErr
		}
		return nil
	}
	return s.repo.FinishModelAction(ctx, job, "completed", auditAction, "")
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
		db:             db,
		repo:           repository.NewModelRepository(db, logger),
		publisher:      pub,
		auditLogger:    auditLogger,
		rbacChecker:    rbacChecker,
		config:         config,
		logger:         logger,
		outboxWorkerID: uuid.NewString(),
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

	existing, err := s.repo.GetModel(ctx, m.ModelID)
	if err != nil {
		return err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelWrite, existing.TenantID); err != nil {
		return err
	}
	// The authenticated resource is authoritative. Never let a request-supplied
	// tenant turn an object-scoped update into a cross-tenant write.
	m.TenantID = existing.TenantID
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
	modelIDIsUUID := false
	if _, parseErr := uuid.Parse(req.ModelID); parseErr == nil {
		modelIDIsUUID = true
		modelObj, err = s.repo.GetModel(ctx, req.ModelID)
	} else {
		modelObj, err = s.repo.GetModelByName(ctx, req.TenantID, req.ModelID)
	}
	// A UUID is an authoritative resource path, never a model name. Missing
	// UUIDs must remain 404 and must not fall through to auto-creation under a
	// different generated identifier.
	if modelIDIsUUID && err != nil && errors.IsCode(err, errors.ErrCodeModelNotFound) {
		return nil, err
	}
	if !modelIDIsUUID && err != nil && errors.IsCode(err, errors.ErrCodeModelNotFound) {
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
func (s *ModelService) ActivateModelVersion(ctx context.Context, expectedModelID, modelVersion string, grayPercent int, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "ModelService.ActivateModelVersion")
	defer span.End()
	if grayPercent != 100 {
		return errors.New(errors.ErrCodeOutOfRange, "model registry activation currently requires gray_percent=100; use the deployment workflow for staged traffic rollout")
	}

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
	if err := s.validateModelActivationGatesTx(ctx, tx, mv.TenantID, mv.ModelID); err != nil {
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
	auditDetail := map[string]interface{}{
		"model_id":        mv.ModelID,
		"previous_status": string(currentStatus),
		"new_status":      string(model.ModelStatusActive),
		"gray_percent":    grayPercent,
	}
	if err := s.recordAuditLogTx(ctx, tx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, auditDetail); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return err
	}
	// The registry endpoint performs only an all-at-once hot reload. Commit the
	// state, audit and durable outbox event atomically. The outbox uses a stable
	// event identity, so a broker acknowledgement followed by an acknowledgement
	// transaction failure can only produce an idempotent at-least-once retry.
	if s.config.EnableKafkaNotification {
		mv.Status = string(model.ModelStatusActive)
		if _, err := s.insertModelUpdateOutboxTx(ctx, tx, mv, "activated", ""); err != nil {
			s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist model activation outbox")
		}
	}
	if err := tx.Commit(); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, err.Error())
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model activation")
	}

	s.recordAuditStreamSuccess(ctx, opCtx, audit.EventTypeModelVersionActivate, "model_version", modelVersion, auditDetail)

	s.logger.Info("Model version activated", zap.String("version", modelVersion))
	return nil
}

// RollbackModelVersion reactivates a previously deprecated version through a
// rollback-specific transition. It atomically persists the displaced active
// version, applied audit and a linked model-update outbox event. The outbox
// acknowledgement later closes the durable job and completion audit in one
// transaction after Kafka broker acknowledgement.
func (s *ModelService) RollbackModelVersion(ctx context.Context, expectedModelID, modelVersion string, opCtx *OperationContext) error {
	return s.rollbackModelVersion(ctx, expectedModelID, modelVersion, opCtx, nil)
}

func (s *ModelService) rollbackModelVersion(ctx context.Context, expectedModelID, modelVersion string, opCtx *OperationContext, job *model.ModelActionJob) error {
	ctx, span := otel.StartSpan(ctx, "ModelService.RollbackModelVersion")
	defer span.End()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to start model rollback transaction")
	}
	defer tx.Rollback()

	mv, err := s.getModelVersionForUpdate(ctx, tx, modelVersion)
	if err != nil {
		return err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermModelActivate, mv.TenantID); err != nil {
		return err
	}
	if expectedModelID != "" && mv.ModelID != expectedModelID {
		return errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found under model: %s", expectedModelID)
	}
	if model.ModelStatus(mv.Status) != model.ModelStatusDeprecated {
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "rollback target must be deprecated, got: %s", mv.Status)
	}
	if err := s.validateModelActivationGatesTx(ctx, tx, mv.TenantID, mv.ModelID); err != nil {
		return err
	}

	previousActive := ""
	if err := tx.QueryRowContext(ctx, `
		SELECT model_version
		FROM model_versions
		WHERE model_id = $1::uuid AND tenant_id = $2 AND status = 'active' AND model_version <> $3
		ORDER BY updated_at DESC
		LIMIT 1
		FOR UPDATE
	`, mv.ModelID, mv.TenantID, modelVersion).Scan(&previousActive); err != nil && err != sql.ErrNoRows {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock previous active model version")
	}
	if err := s.deprecateOtherVersionsTx(ctx, tx, mv.ModelID, modelVersion); err != nil {
		return err
	}
	if err := s.updateModelVersionStatusTx(ctx, tx, modelVersion, model.ModelStatusActive); err != nil {
		return err
	}
	detail := map[string]interface{}{
		"model_id":                mv.ModelID,
		"target_version":          modelVersion,
		"previous_active_version": previousActive,
		"previous_status":         mv.Status,
		"new_status":              string(model.ModelStatusActive),
	}
	if job != nil {
		detail["job_id"] = job.JobID
		detail["reason"] = stringPayload(job.Payload, "reason")
	}
	if err := s.recordAuditLogTx(ctx, tx, opCtx, audit.EventType("MODEL_VERSION_ROLLBACK_APPLIED"), "model_version", modelVersion, detail); err != nil {
		return err
	}
	if s.config.EnableKafkaNotification {
		mv.Status = string(model.ModelStatusActive)
		jobID := ""
		if job != nil {
			jobID = job.JobID
		}
		if _, err := s.insertModelUpdateOutboxTx(ctx, tx, mv, "rollback-activated", jobID); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist rollback model update outbox")
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model rollback")
	}
	s.recordAuditStreamSuccess(ctx, opCtx, audit.EventType("MODEL_VERSION_ROLLBACK_APPLIED"), "model_version", modelVersion, detail)
	// Without Kafka notification there is no outbox acknowledgement to close a
	// linked action job, so finish it transactionally through the repository.
	if job != nil && !s.config.EnableKafkaNotification {
		return s.repo.FinishModelAction(ctx, job, "completed", "MODEL_VERSION_ROLLBACK_COMPLETED", "")
	}
	return nil
}

// validateModelActivationGatesTx makes persisted workbench review gates a
// fail-closed server-side invariant. A model without gates cannot activate.
func (s *ModelService) validateModelActivationGatesTx(ctx context.Context, tx *sql.Tx, tenantID, modelID string) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT COALESCE(payload->>'name', 'unnamed gate'), COALESCE(payload->>'status', '')
		FROM model_workbench_items
		WHERE tenant_id = $1 AND model_id = $2::uuid AND category = 'review_gates'
		ORDER BY ordinal, occurred_at DESC
		FOR UPDATE
	`, tenantID, modelID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to validate model activation gates")
	}
	defer rows.Close()

	pending := make([]string, 0)
	gateCount := 0
	for rows.Next() {
		gateCount++
		var name, status string
		if err := rows.Scan(&name, &status); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan model activation gate")
		}
		normalized := strings.ToLower(strings.TrimSpace(status))
		if normalized != "通过" && normalized != "已通过" && normalized != "passed" && normalized != "approved" {
			pending = append(pending, name)
		}
	}
	if err := rows.Err(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate model activation gates")
	}
	if gateCount == 0 {
		return errors.New(errors.ErrCodeInvalidStateTransition, "model activation blocked: no persisted review gates")
	}
	if len(pending) > 0 {
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "model activation blocked by pending review gates: %s", strings.Join(pending, ", "))
	}
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
	auditDetail := map[string]interface{}{
		"model_id":        mv.ModelID,
		"previous_status": string(currentStatus),
		"new_status":      string(model.ModelStatusDeprecated),
	}
	if err := s.recordAuditLogTx(ctx, tx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, auditDetail); err != nil {
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

	s.recordAuditStreamSuccess(ctx, opCtx, audit.EventTypeModelVersionDeprecate, "model_version", modelVersion, auditDetail)

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

func (s *ModelService) GetModelWorkbench(ctx context.Context, modelID string, opCtx *OperationContext) (*model.ModelWorkbench, error) {
	modelObj, err := s.GetModel(ctx, modelID, opCtx)
	if err != nil {
		return nil, err
	}
	versions, _, err := s.repo.ListModelVersions(ctx, modelObj.TenantID, modelID, &model.ModelVersionFilter{Limit: 100})
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListModelWorkbenchItems(ctx, modelObj.TenantID, modelID)
	if err != nil {
		return nil, err
	}
	actions, err := s.repo.ListModelActions(ctx, modelObj.TenantID, modelID, 20)
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]json.RawMessage)
	source := "postgresql"
	for _, item := range items {
		grouped[item.Category] = append(grouped[item.Category], item.Payload)
		if strings.HasPrefix(item.ScenarioID, "acceptance-bootstrap") || strings.HasPrefix(item.ScenarioID, "mlops-reference") {
			source = "postgresql/acceptance-bootstrap"
		}
	}
	return &model.ModelWorkbench{
		Model:    modelObj,
		Versions: versions,
		Items:    grouped,
		Actions:  actions,
		Source:   source,
	}, nil
}

// SubmitModelAction queues a durable model workbench action. The action job and
// audit record are committed atomically by the repository.
func (s *ModelService) SubmitModelAction(
	ctx context.Context,
	modelID string,
	req *model.ModelActionRequest,
	permission rbac.Permission,
	auditAction string,
	opCtx *OperationContext,
) (*model.ModelActionJob, error) {
	if opCtx == nil {
		return nil, errors.New(errors.ErrCodeUnauthorized, "operation context required")
	}
	if err := s.checkPermission(ctx, opCtx, permission, opCtx.TenantID); err != nil {
		return nil, err
	}
	modelObj, err := s.repo.GetModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if modelObj.TenantID != opCtx.TenantID && !s.hasAdminPermission(opCtx) {
		return nil, errors.New(errors.ErrCodePermissionDenied, "cross-tenant model action denied")
	}

	req.Action = strings.TrimSpace(req.Action)
	req.Target = strings.TrimSpace(req.Target)
	req.Version = strings.TrimSpace(req.Version)
	if req.Action == "" || len(req.Action) > 64 || !isSafeModelAction(req.Action) {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid model action")
	}
	if req.Target == "" || len(req.Target) > 512 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid model action target")
	}
	if len(req.Version) > 128 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid model version")
	}
	if err := validateModelActionRequest(req); err != nil {
		return nil, err
	}
	if req.Version != "" {
		mv, err := s.repo.GetModelVersion(ctx, req.Version)
		if err != nil {
			return nil, err
		}
		if mv.ModelID != modelID || mv.TenantID != modelObj.TenantID {
			return nil, errors.Newf(errors.ErrCodeModelVersionNotFound, "model version not found under model: %s", modelID)
		}
	}
	if req.ActionID == "" {
		req.ActionID = uuid.NewString()
	}
	if _, err := uuid.Parse(req.ActionID); err != nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "action_id must be a UUID")
	}

	job := &model.ModelActionJob{
		JobID:       uuid.NewString(),
		ActionID:    req.ActionID,
		TenantID:    modelObj.TenantID,
		ModelID:     modelID,
		Version:     req.Version,
		Action:      req.Action,
		Target:      req.Target,
		Payload:     req.Payload,
		Status:      "queued",
		RequestedBy: opCtx.UserID,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.repo.CreateModelAction(ctx, job, auditAction, opCtx.IPAddr, opCtx.UserAgent); err != nil {
		return nil, err
	}
	return job, nil
}

func isSafeModelAction(value string) bool {
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return false
	}
	return true
}

func validateModelActionRequest(req *model.ModelActionRequest) error {
	if req.Payload == nil {
		req.Payload = make(map[string]interface{})
	}
	datasetID := strings.TrimSpace(stringPayload(req.Payload, "dataset_id"))
	if datasetID != "" && (!isSafeModelReference(datasetID) || len(datasetID) > 160) {
		return errors.New(errors.ErrCodeInvalidParameter, "invalid dataset_id")
	}
	switch req.Action {
	case "append-feedback-samples":
		if datasetID == "" {
			return errors.New(errors.ErrCodeMissingParameter, "dataset_id is required for feedback ingestion")
		}
		for _, key := range []string{"samples", "sample_payloads", "sample_data", "raw_samples"} {
			if _, exists := req.Payload[key]; exists {
				return errors.New(errors.ErrCodeInvalidParameter, "inline sample payloads are forbidden; submit a tenant-scoped dataset reference")
			}
		}
		count, ok := numberPayload(req.Payload, "sample_count")
		if !ok || count <= 0 || count > 10_000_000 {
			return errors.New(errors.ErrCodeOutOfRange, "sample_count must be between 1 and 10000000")
		}
	case "request-retraining":
		if datasetID == "" {
			return errors.New(errors.ErrCodeMissingParameter, "dataset_id is required for retraining")
		}
		strategy := strings.TrimSpace(stringPayload(req.Payload, "strategy"))
		if strategy != "incremental" && strategy != "full" {
			return errors.New(errors.ErrCodeInvalidParameter, "strategy must be incremental or full")
		}
		if strings.TrimSpace(stringPayload(req.Payload, "reason")) == "" {
			return errors.New(errors.ErrCodeMissingParameter, "reason is required for retraining")
		}
	case "request-evaluation":
		if req.Version == "" {
			return errors.New(errors.ErrCodeMissingParameter, "model version is required for evaluation")
		}
		if datasetID == "" {
			return errors.New(errors.ErrCodeMissingParameter, "dataset_id is required for evaluation")
		}
	case "rollback-version":
		if req.Version == "" {
			return errors.New(errors.ErrCodeMissingParameter, "rollback target version is required")
		}
		if strings.TrimSpace(stringPayload(req.Payload, "reason")) == "" {
			return errors.New(errors.ErrCodeMissingParameter, "rollback reason is required")
		}
	}
	return nil
}

func stringPayload(payload map[string]interface{}, key string) string {
	value, _ := payload[key].(string)
	return value
}

func numberPayload(payload map[string]interface{}, key string) (float64, bool) {
	switch value := payload[key].(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	default:
		return 0, false
	}
}

func isSafeModelReference(value string) bool {
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return false
	}
	return value != ""
}

// =============================================================================
// Kafka 模型更新通知
// =============================================================================

// ModelUpdateEvent Kafka 模型更新事件
type ModelUpdateEvent struct {
	EventID                    string                 `json:"event_id"`
	SchemaVersion              int                    `json:"schema_version"`
	TenantID                   string                 `json:"tenant_id"`
	ModelID                    string                 `json:"model_id"`
	ModelName                  string                 `json:"model_name"`
	ModelType                  string                 `json:"model_type"`
	Version                    string                 `json:"version"`
	ArtifactURI                string                 `json:"artifact_uri"`
	Action                     string                 `json:"action"` // registered, activated, deprecated, rollback-activated
	Metrics                    map[string]interface{} `json:"metrics,omitempty"`
	ExpectedAppliedParallelism int                    `json:"expected_applied_parallelism"`
	Timestamp                  string                 `json:"timestamp"`
}

type ModelAppliedAck struct {
	SchemaVersion  int     `json:"schema_version"`
	EventID        string  `json:"event_id"`
	TenantID       string  `json:"tenant_id"`
	ModelID        string  `json:"model_id"`
	Version        string  `json:"version"`
	ArtifactURI    string  `json:"artifact_uri"`
	ArtifactSHA256 string  `json:"artifact_sha256"`
	WarmupScore    float64 `json:"warmup_score"`
	SubtaskIndex   int     `json:"subtask_index"`
	Parallelism    int     `json:"parallelism"`
	Status         string  `json:"status"`
	Error          string  `json:"error"`
	Timestamp      string  `json:"timestamp"`
}

type modelAppliedContract struct {
	ArtifactURI                string
	ArtifactSHA256             string
	ExpectedAppliedParallelism int
}

func parseModelAppliedContract(payload []byte, configuredParallelism int) (modelAppliedContract, error) {
	var event ModelUpdateEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return modelAppliedContract{}, fmt.Errorf("decode model update contract: %w", err)
	}
	expectedParallelism := event.ExpectedAppliedParallelism
	if expectedParallelism <= 0 {
		expectedParallelism = configuredParallelism
	}
	if expectedParallelism <= 0 {
		return modelAppliedContract{}, fmt.Errorf("model update contract has no server-controlled expected parallelism")
	}
	expectedSHA := ""
	if value, ok := event.Metrics["artifact_sha256"].(string); ok {
		expectedSHA = strings.ToLower(strings.TrimSpace(value))
	}
	return modelAppliedContract{
		ArtifactURI:                strings.TrimSpace(event.ArtifactURI),
		ArtifactSHA256:             expectedSHA,
		ExpectedAppliedParallelism: expectedParallelism,
	}, nil
}

func validateModelAppliedAckContract(ack ModelAppliedAck, contract modelAppliedContract, requireFingerprint bool) error {
	if ack.Parallelism != contract.ExpectedAppliedParallelism {
		return fmt.Errorf("model applied acknowledgement parallelism %d does not match server contract %d", ack.Parallelism, contract.ExpectedAppliedParallelism)
	}
	if strings.TrimSpace(ack.ArtifactURI) != contract.ArtifactURI {
		return fmt.Errorf("model applied acknowledgement artifact_uri does not match event contract")
	}
	if ack.Status != "applied" {
		return nil
	}
	if requireFingerprint && contract.ArtifactSHA256 == "" {
		return fmt.Errorf("model update contract is missing artifact_sha256")
	}
	if contract.ArtifactSHA256 != "" && strings.ToLower(strings.TrimSpace(ack.ArtifactSHA256)) != contract.ArtifactSHA256 {
		return fmt.Errorf("model applied acknowledgement artifact_sha256 does not match event contract")
	}
	return nil
}

// HandleModelAppliedAck closes a rollback only after every Flink broadcast
// subtask confirms artifact download, SHA validation, model warmup and atomic
// runtime swap. Kafka broker delivery alone never completes the action.
func (s *ModelService) HandleModelAppliedAck(ctx context.Context, payload []byte) error {
	var ack ModelAppliedAck
	if err := json.Unmarshal(payload, &ack); err != nil {
		return fmt.Errorf("decode model applied acknowledgement: %w", err)
	}
	if strings.TrimSpace(ack.EventID) == "" || strings.TrimSpace(ack.TenantID) == "" ||
		strings.TrimSpace(ack.ModelID) == "" || strings.TrimSpace(ack.Version) == "" {
		return fmt.Errorf("model applied acknowledgement is missing event, tenant, model or version")
	}
	if ack.Status != "applied" && ack.Status != "failed" {
		return fmt.Errorf("invalid model applied acknowledgement status %q", ack.Status)
	}
	if ack.Parallelism <= 0 || ack.SubtaskIndex < 0 || ack.SubtaskIndex >= ack.Parallelism {
		return fmt.Errorf("invalid model applied acknowledgement subtask %d/%d", ack.SubtaskIndex, ack.Parallelism)
	}
	if ack.Status == "applied" && strings.TrimSpace(ack.ArtifactSHA256) == "" {
		return fmt.Errorf("applied acknowledgement requires artifact_sha256")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin model applied acknowledgement: %w", err)
	}
	defer tx.Rollback()

	var actionJobID, expectedTenant, expectedModel, expectedVersion string
	var eventPayload []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT action_job_id, tenant_id, model_id, model_version, payload
		FROM model_update_outbox WHERE event_id = $1
		FOR UPDATE
	`, ack.EventID).Scan(&actionJobID, &expectedTenant, &expectedModel, &expectedVersion, &eventPayload); err != nil {
		return fmt.Errorf("resolve model update outbox event %s: %w", ack.EventID, err)
	}
	if ack.TenantID != expectedTenant || ack.ModelID != expectedModel || ack.Version != expectedVersion {
		return fmt.Errorf("model applied acknowledgement scope mismatch for event %s", ack.EventID)
	}
	contract, err := parseModelAppliedContract(eventPayload, s.config.AppliedAckExpectedParallelism)
	if err != nil {
		return fmt.Errorf("resolve model applied acknowledgement contract for event %s: %w", ack.EventID, err)
	}
	if err := validateModelAppliedAckContract(ack, contract, actionJobID != ""); err != nil {
		return fmt.Errorf("reject model applied acknowledgement for event %s: %w", ack.EventID, err)
	}

	rawPayload, err := json.Marshal(ack)
	if err != nil {
		return fmt.Errorf("encode model applied acknowledgement: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_update_applied_acks (
			event_id, tenant_id, model_id, model_version, subtask_index, parallelism,
			status, artifact_uri, artifact_sha256, warmup_score, error, payload, applied_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,now())
		ON CONFLICT (event_id, subtask_index) DO UPDATE SET
			status = CASE WHEN model_update_applied_acks.status = 'failed' THEN 'failed' ELSE EXCLUDED.status END,
			artifact_uri = EXCLUDED.artifact_uri,
			artifact_sha256 = EXCLUDED.artifact_sha256,
			warmup_score = EXCLUDED.warmup_score,
			error = CASE WHEN model_update_applied_acks.status = 'failed' THEN model_update_applied_acks.error ELSE EXCLUDED.error END,
			payload = EXCLUDED.payload,
			applied_at = now()
	`, ack.EventID, ack.TenantID, ack.ModelID, ack.Version, ack.SubtaskIndex,
		ack.Parallelism, ack.Status, ack.ArtifactURI, ack.ArtifactSHA256,
		ack.WarmupScore, ack.Error, string(rawPayload)); err != nil {
		return fmt.Errorf("persist model applied acknowledgement: %w", err)
	}

	var appliedCount, minAppliedSubtask, maxAppliedSubtask int
	var hasFailure bool
	var aggregateFailureReason string
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT subtask_index) FILTER (WHERE status = 'applied'),
		       COALESCE(BOOL_OR(status = 'failed'), false),
		       COALESCE(MAX(NULLIF(error, '')) FILTER (WHERE status = 'failed'), ''),
		       COALESCE(MIN(subtask_index) FILTER (WHERE status = 'applied'), -1),
		       COALESCE(MAX(subtask_index) FILTER (WHERE status = 'applied'), -1)
		FROM model_update_applied_acks WHERE event_id = $1
	`, ack.EventID).Scan(&appliedCount, &hasFailure, &aggregateFailureReason, &minAppliedSubtask, &maxAppliedSubtask); err != nil {
		return fmt.Errorf("aggregate model applied acknowledgements: %w", err)
	}

	allExpectedSubtasksApplied := appliedCount == contract.ExpectedAppliedParallelism &&
		minAppliedSubtask == 0 && maxAppliedSubtask == contract.ExpectedAppliedParallelism-1
	if actionJobID != "" && (hasFailure || allExpectedSubtasksApplied) {
		status := "completed"
		auditAction := "MODEL_VERSION_ROLLBACK_COMPLETED"
		failureReason := ""
		if hasFailure {
			status = "failed"
			auditAction = "MODEL_VERSION_ROLLBACK_FAILED"
			failureReason = aggregateFailureReason
		}
		var tenantID, requestedBy, modelID, actionID, action, version string
		var actionPayload []byte
		err := tx.QueryRowContext(ctx, `
			UPDATE model_action_jobs SET status = $2, updated_at = now()
			WHERE job_id = $1 AND status = 'running'
			RETURNING tenant_id, requested_by, model_id::text, action_id, action, version, payload
		`, actionJobID, status).Scan(
			&tenantID, &requestedBy, &modelID, &actionID, &action, &version, &actionPayload)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("finalize model action from Flink acknowledgement: %w", err)
		}
		if err == nil {
			var decodedPayload map[string]interface{}
			_ = json.Unmarshal(actionPayload, &decodedPayload)
			detail, _ := json.Marshal(map[string]interface{}{
				"action_id": actionID, "job_id": actionJobID, "action": action,
				"version": version, "status": status, "stage": "flink-artifact-applied",
				"event_id": ack.EventID, "data_plane_applied": !hasFailure,
				"applied_subtasks": appliedCount, "expected_subtasks": contract.ExpectedAppliedParallelism,
				"artifact_sha256": ack.ArtifactSHA256, "warmup_score": ack.WarmupScore,
				"reason": stringPayload(decodedPayload, "reason"), "error": failureReason,
			})
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail)
				VALUES ($1, $2, $3, 'model', $4, $5::jsonb)
			`, tenantID, requestedBy, auditAction, modelID, string(detail)); err != nil {
				return fmt.Errorf("persist Flink-applied model audit: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model applied acknowledgement: %w", err)
	}
	return nil
}

type modelUpdateOutboxRecord struct {
	ID           int64
	EventID      string
	ModelID      string
	PartitionKey string
	Payload      []byte
	ActionJobID  string
	AttemptCount int
	CreatedAt    time.Time
}

func (s *ModelService) insertModelUpdateOutboxTx(ctx context.Context, tx *sql.Tx, mv *model.ModelVersion, action, actionJobID string) (string, error) {
	eventID := uuid.NewString()
	createdAt := time.Now().UTC()
	event := &ModelUpdateEvent{
		EventID: eventID, SchemaVersion: 1, TenantID: mv.TenantID, ModelID: mv.ModelID, ModelName: mv.ModelName,
		ModelType: mv.ModelType, Version: mv.ModelVersion, ArtifactURI: mv.ArtifactURI,
		Action: action, Metrics: mv.Metrics, ExpectedAppliedParallelism: s.config.AppliedAckExpectedParallelism,
		Timestamp: createdAt.Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal model update outbox event")
	}
	partitionKey := mv.ModelID
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO model_update_outbox (
			event_id, tenant_id, model_id, model_version, action, partition_key,
			payload, action_job_id, status, available_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, 'pending', $9, $9, $9)
	`, eventID, mv.TenantID, mv.ModelID, mv.ModelVersion, action, partitionKey, string(payload), actionJobID, createdAt); err != nil {
		return "", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to insert model update outbox event")
	}
	return eventID, nil
}

func (s *ModelService) processModelUpdateOutbox(ctx context.Context) error {
	if !s.config.EnableKafkaNotification || s.publisher == nil {
		return nil
	}
	records, err := s.claimModelUpdateOutbox(ctx, 20)
	if err != nil {
		return err
	}
	for _, record := range records {
		publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := s.publisher.PublishModelUpdateWithID(publishCtx, record.PartitionKey, record.Payload, record.EventID, record.CreatedAt)
		cancel()
		if err != nil {
			s.failModelUpdateOutbox(ctx, record, err)
			continue
		}
		if err := s.ackModelUpdateOutbox(ctx, record); err != nil {
			// Broker ack already happened. Keep the stable event pending after the
			// lease expires; a retry has the same event_id and is consumer-idempotent.
			return err
		}
	}
	return nil
}

func (s *ModelService) claimModelUpdateOutbox(ctx context.Context, limit int) ([]modelUpdateOutboxRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin model update outbox claim")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_update_outbox
		SET status = 'pending', locked_at = NULL, locked_by = NULL, available_at = now(), updated_at = now(),
		    last_error = CASE WHEN last_error = '' THEN 'lease expired after broker acknowledgement uncertainty' ELSE last_error END
		WHERE status = 'processing' AND locked_at < now() - interval '30 seconds'
	`); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to recover model update outbox leases")
	}
	rows, err := tx.QueryContext(ctx, `
		WITH candidates AS (
			SELECT current.id
			FROM model_update_outbox current
			WHERE current.status = 'pending' AND current.available_at <= now()
			  AND NOT EXISTS (
				SELECT 1 FROM model_update_outbox prior
				WHERE prior.model_id = current.model_id AND prior.id < current.id
				  AND prior.status NOT IN ('published', 'dead')
			  )
			ORDER BY current.id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE model_update_outbox target
		SET status = 'processing', locked_at = now(), locked_by = $2,
		    attempt_count = target.attempt_count + 1, updated_at = now()
		FROM candidates WHERE target.id = candidates.id
		RETURNING target.id, target.event_id, target.model_id, target.partition_key,
		          target.payload, target.action_job_id, target.attempt_count, target.created_at
	`, limit, s.outboxWorkerID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to claim model update outbox events")
	}
	defer rows.Close()
	records := make([]modelUpdateOutboxRecord, 0, limit)
	for rows.Next() {
		var record modelUpdateOutboxRecord
		if err := rows.Scan(&record.ID, &record.EventID, &record.ModelID, &record.PartitionKey, &record.Payload, &record.ActionJobID, &record.AttemptCount, &record.CreatedAt); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan model update outbox event")
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate model update outbox events")
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model update outbox claim")
	}
	return records, nil
}

func (s *ModelService) ackModelUpdateOutbox(ctx context.Context, record modelUpdateOutboxRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin model update outbox acknowledgement")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE model_update_outbox
		SET status = 'published', published_at = now(), locked_at = NULL, locked_by = NULL,
		    last_error = '', updated_at = now()
		WHERE id = $1 AND status = 'processing' AND locked_by = $2
	`, record.ID, s.outboxWorkerID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to acknowledge model update outbox event")
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return errors.New(errors.ErrCodeConcurrentModify, "model update outbox lease is no longer owned")
	}
	// Broker acknowledgement only proves delivery. A linked rollback action is
	// completed later by handleModelAppliedAck after every Flink subtask has
	// downloaded, verified, warmed and atomically switched the artifact.
	if record.ActionJobID != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE model_action_jobs SET updated_at = now()
			WHERE job_id = $1 AND status = 'running'
		`, record.ActionJobID); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to refresh broker-delivered rollback action")
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit model update outbox acknowledgement")
	}
	return nil
}

func (s *ModelService) failModelUpdateOutbox(ctx context.Context, record modelUpdateOutboxRecord, publishErr error) {
	status := "pending"
	delay := 5 * time.Second * time.Duration(1<<uint(min(record.AttemptCount-1, 6)))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	if record.AttemptCount >= 10 {
		status = "dead"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("Failed to begin model update outbox failure transaction", zap.Error(err))
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE model_update_outbox
		SET status = $1, available_at = $2, locked_at = NULL, locked_by = NULL,
		    last_error = $3, updated_at = now()
		WHERE id = $4 AND status = 'processing' AND locked_by = $5
	`, status, time.Now().UTC().Add(delay), publishErr.Error(), record.ID, s.outboxWorkerID); err != nil {
		s.logger.Error("Failed to record model update outbox publication failure", zap.Error(err))
		return
	}
	if status == "dead" && record.ActionJobID != "" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE model_action_jobs SET status = 'failed', updated_at = now()
			WHERE job_id = $1 AND status = 'running'
		`, record.ActionJobID); err != nil {
			s.logger.Error("Failed to close dead model update action", zap.Error(err))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit model update outbox publication failure", zap.Error(err))
	}
}

// publishModelUpdateEvent 发布模型更新事件到 Kafka
func (s *ModelService) publishModelUpdateEvent(ctx context.Context, modelObj *model.Model, mv *model.ModelVersion, action string) error {
	if s.publisher == nil {
		return errors.New(errors.ErrCodeServiceUnavailable, "model update publisher is not configured")
	}

	publishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	eventID := uuid.NewString()
	occurredAt := time.Now().UTC()
	event := &ModelUpdateEvent{
		EventID:       eventID,
		SchemaVersion: 1,
		TenantID:      mv.TenantID,
		ModelID:       mv.ModelID,
		ModelName:     mv.ModelName,
		ModelType:     mv.ModelType,
		Version:       mv.ModelVersion,
		ArtifactURI:   mv.ArtifactURI,
		Action:        action,
		Metrics:       mv.Metrics,
		Timestamp:     occurredAt.Format(time.RFC3339Nano),
	}

	if modelObj != nil {
		event.ModelName = modelObj.Name
		event.ModelType = modelObj.ModelType
	}

	eventJSON, _ := json.Marshal(event)

	if err := s.publisher.PublishModelUpdateWithID(publishCtx, mv.ModelID, eventJSON, eventID, occurredAt); err != nil {
		s.logger.Error("Failed to publish model update event",
			zap.String("model_id", mv.ModelID),
			zap.String("action", action),
			zap.Error(err))
		return err
	}

	s.logger.Info("Published model update event to Kafka",
		zap.String("model_id", mv.ModelID),
		zap.String("version", mv.ModelVersion),
		zap.String("action", action))
	return nil
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

func (s *ModelService) recordAuditStreamSuccess(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID string, detail map[string]interface{}) {
	if opCtx == nil || s.auditLogger == nil {
		return
	}
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

// RecordMLOpsWorkflowAudit persists the authoritative audit event returned by
// the MLOps workflow API. The browser never fabricates this event name.
func (s *ModelService) RecordMLOpsWorkflowAudit(ctx context.Context, opCtx *OperationContext, action, workflowName string, detail map[string]interface{}, actionErr error) error {
	if s.db == nil || opCtx == nil {
		return errors.New(errors.ErrCodeDatabaseError, "MLOps workflow audit database context is required")
	}
	eventType := audit.EventType(action)
	result := audit.ResultSuccess
	errorMessage := ""
	if actionErr != nil {
		result = audit.ResultFailure
		errorMessage = actionErr.Error()
	}
	detailCopy := make(map[string]interface{}, len(detail)+2)
	for key, value := range detail {
		detailCopy[key] = value
	}
	detailCopy["result"] = string(result)
	if errorMessage != "" {
		detailCopy["error"] = errorMessage
	}
	detailJSON, err := json.Marshal(detailCopy)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal MLOps workflow audit")
	}
	actionName := action
	if result == audit.ResultFailure {
		actionName += "_failed"
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, NULLIF($2, '')::uuid, $3, 'mlops_workflow', $4, $5::jsonb, $6, $7)
	`, opCtx.TenantID, opCtx.UserID, actionName, workflowName, string(detailJSON), opCtx.IPAddr, opCtx.UserAgent); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist required MLOps workflow audit")
	}
	if result == audit.ResultSuccess {
		s.recordAuditStreamSuccess(ctx, opCtx, eventType, "mlops_workflow", workflowName, detail)
	}
	return nil
}

// RecordMLOpsWorkflowAuditIntent is the durable gate that must succeed before
// any Argo or in-process MLOps mutation. The intent remains authoritative proof
// of the attempted action even if the remote mutation or its completion audit
// later fails.
func (s *ModelService) RecordMLOpsWorkflowAuditIntent(ctx context.Context, opCtx *OperationContext, action, workflowName string, detail map[string]interface{}) (string, error) {
	if s.db == nil || opCtx == nil {
		return "", errors.New(errors.ErrCodeDatabaseError, "MLOps workflow audit database context is required")
	}
	eventID := "mlops-intent-" + uuid.NewString()
	detailCopy := make(map[string]interface{}, len(detail)+2)
	for key, value := range detail {
		detailCopy[key] = value
	}
	detailCopy["result"] = "pending"
	detailCopy["intended_action"] = action
	detailJSON, err := json.Marshal(detailCopy)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal MLOps workflow audit intent")
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent, event_id)
		VALUES ($1, NULLIF($2, '')::uuid, $3, 'mlops_workflow', $4, $5::jsonb, $6, $7, $8)
	`, opCtx.TenantID, opCtx.UserID, action, workflowName, string(detailJSON), opCtx.IPAddr, opCtx.UserAgent, eventID); err != nil {
		return "", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist required MLOps workflow audit intent")
	}
	return eventID, nil
}

// PendingAutomatedMLOpsAuditIntent is a durable reconciliation item. The
// request intent is inserted before Argo and remains pending until the
// completion event and intent state transition commit atomically.
type PendingAutomatedMLOpsAuditIntent struct {
	EventID      string
	TenantID     string
	WorkflowName string
	Trigger      string
	Reason       string
	ModelID      string
	FeatureSetID string
}

// ReconcileCompletedLegacyAutomatedMLOpsAuditIntents closes legacy request
// intents that already have their exact linked completion event. Older
// workflows did not carry canonical ownership parameters, so Argo inspection
// alone cannot safely reconcile them; the immutable intent-event link can.
func (s *ModelService) ReconcileCompletedLegacyAutomatedMLOpsAuditIntents(ctx context.Context) (int64, error) {
	if s.db == nil {
		return 0, errors.New(errors.ErrCodeDatabaseError, "MLOps workflow audit database context is required")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE audit_logs AS intent
		SET detail = intent.detail || jsonb_build_object(
			'reconciliation_state', 'completed',
			'completion_action', completion.action,
			'reconciled_at', completion.created_at,
			'reconciliation_source', 'linked_completion_backfill'
		)
		FROM audit_logs AS completion
		WHERE intent.object_type = 'mlops_workflow'
		  AND intent.action = 'MLOPS_AUTOMATED_RETRAIN_SUBMIT_REQUESTED'
		  AND COALESCE(intent.detail->>'reconciliation_state', 'pending') = 'pending'
		  AND completion.object_type = 'mlops_workflow'
		  AND completion.object_id = intent.object_id
		  AND completion.tenant_id = intent.tenant_id
		  AND completion.action = 'MLOPS_AUTOMATED_RETRAIN_SUBMITTED'
		  AND completion.detail->>'intent_event_id' = intent.event_id
	`)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to backfill linked automatic MLOps audit intents")
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count backfilled automatic MLOps audit intents")
	}
	return count, nil
}

func (s *ModelService) ListPendingAutomatedMLOpsAuditIntents(ctx context.Context, limit int) ([]PendingAutomatedMLOpsAuditIntent, error) {
	if s.db == nil {
		return nil, errors.New(errors.ErrCodeDatabaseError, "MLOps workflow audit database context is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, tenant_id, object_id,
		       COALESCE(detail->>'trigger', ''), COALESCE(detail->>'reason', ''),
		       COALESCE(detail->>'model_id', ''), COALESCE(detail->>'feature_set_id', '')
		FROM audit_logs
		WHERE object_type = 'mlops_workflow'
		  AND action = 'MLOPS_AUTOMATED_RETRAIN_SUBMIT_REQUESTED'
		  AND COALESCE(detail->>'reconciliation_state', 'pending') = 'pending'
		ORDER BY created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to list pending automatic MLOps audit intents")
	}
	defer rows.Close()
	result := make([]PendingAutomatedMLOpsAuditIntent, 0)
	for rows.Next() {
		var item PendingAutomatedMLOpsAuditIntent
		if err := rows.Scan(&item.EventID, &item.TenantID, &item.WorkflowName, &item.Trigger, &item.Reason, &item.ModelID, &item.FeatureSetID); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan pending automatic MLOps audit intent")
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate pending automatic MLOps audit intents")
	}
	return result, nil
}

// RecordAutomatedMLOpsAuditCompletion atomically writes the completion event
// and marks its durable request intent reconciled. If this transaction fails,
// the intent stays pending and the orchestrator retries it after confirming the
// exact Argo workflow exists.
func (s *ModelService) RecordAutomatedMLOpsAuditCompletion(ctx context.Context, opCtx *OperationContext, action, workflowName, intentEventID string, detail map[string]interface{}) error {
	if s.db == nil || opCtx == nil || strings.TrimSpace(intentEventID) == "" {
		return errors.New(errors.ErrCodeDatabaseError, "automatic MLOps audit completion context is required")
	}
	detailCopy := make(map[string]interface{}, len(detail)+2)
	for key, value := range detail {
		detailCopy[key] = value
	}
	detailCopy["result"] = string(audit.ResultSuccess)
	detailCopy["intent_event_id"] = intentEventID
	detailJSON, err := json.Marshal(detailCopy)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal automatic MLOps audit completion")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin automatic MLOps audit completion")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		SELECT $1, NULLIF($2, '')::uuid, $3, 'mlops_workflow', $4, $5::jsonb, $6, $7
		WHERE NOT EXISTS (
			SELECT 1 FROM audit_logs
			WHERE object_type = 'mlops_workflow' AND object_id = $4 AND action = $3
			  AND detail->>'intent_event_id' = $8
		)
	`, opCtx.TenantID, opCtx.UserID, action, workflowName, string(detailJSON), opCtx.IPAddr, opCtx.UserAgent, intentEventID); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist automatic MLOps audit completion")
	}
	updateResult, err := tx.ExecContext(ctx, `
		UPDATE audit_logs
		SET detail = detail || jsonb_build_object(
			'reconciliation_state', 'completed',
			'completion_action', to_jsonb($2::text),
			'reconciled_at', CURRENT_TIMESTAMP
		)
		WHERE event_id = $1
		  AND action = 'MLOPS_AUTOMATED_RETRAIN_SUBMIT_REQUESTED'
	`, intentEventID, action)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to reconcile automatic MLOps audit intent")
	}
	if rowsAffected, rowsErr := updateResult.RowsAffected(); rowsErr != nil || rowsAffected != 1 {
		return errors.Newf(errors.ErrCodeDatabaseError, "automatic MLOps audit intent reconciliation affected %d rows", rowsAffected)
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit automatic MLOps audit completion")
	}
	s.recordAuditStreamSuccess(ctx, opCtx, audit.EventType(action), "mlops_workflow", workflowName, detailCopy)
	return nil
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

func (s *ModelService) recordAuditLogTx(ctx context.Context, tx *sql.Tx, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID string, detail map[string]interface{}) error {
	if tx == nil || opCtx == nil {
		return errors.New(errors.ErrCodeDatabaseError, "model audit transaction is required")
	}
	detailCopy := make(map[string]interface{}, len(detail)+1)
	for key, value := range detail {
		detailCopy[key] = value
	}
	detailCopy["result"] = string(audit.ResultSuccess)
	detailJSON, err := json.Marshal(detailCopy)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal model audit detail")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
	`, opCtx.TenantID, opCtx.UserID, string(eventType), resourceType, resourceID, string(detailJSON), opCtx.IPAddr, opCtx.UserAgent); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist transactional model audit")
	}
	return nil
}

// ensureUUID generates a UUID string if not already one
func ensureUUID(s string) string {
	if s == "" {
		return uuid.New().String()
	}
	return s
}
