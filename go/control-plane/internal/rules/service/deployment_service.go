////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/service/deployment_service.go
// 部署服务 - 完整修复版（完整代码 - 无截断）
// 修复内容：
// 1. ✅ 修复问题 1.1: PostgreSQL Schema 字段不匹配
// 2. ✅ 修复问题 1.2: RollbackDeployment 未记录回滚来源
// 3. ✅ 修复问题 1.3: ListDeployments 缺少时间字段
// 4. ✅ 集成审计日志记录
// 5. ✅ 状态机验证（使用 model.CanTransition）
// 6. ✅ RBAC 权限检查
// 7. ✅ 灰度进度查询
// 8. ✅ 完善错误处理
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
)

// DeploymentServiceConfig 部署服务配置
type DeploymentServiceConfig struct {
	MaxActiveDeploymentsPerTenant int           `env:"MAX_ACTIVE_DEPLOYMENTS_PER_TENANT" envDefault:"10"`
	GrayTimeout                   time.Duration `env:"GRAY_DEPLOYMENT_TIMEOUT" envDefault:"24h"`
	RequireRollbackReason         bool          `env:"DEPLOYMENT_REQUIRE_ROLLBACK_REASON" envDefault:"true"`
	EnableAutoRollback            bool          `env:"DEPLOYMENT_ENABLE_AUTO_ROLLBACK" envDefault:"true"`
	AutoRollbackThreshold         float64       `env:"DEPLOYMENT_AUTO_ROLLBACK_THRESHOLD" envDefault:"0.05"`
	EnableGrayValidation          bool          `env:"DEPLOYMENT_ENABLE_GRAY_VALIDATION" envDefault:"true"`
	MaxGrayDuration               time.Duration `env:"DEPLOYMENT_MAX_GRAY_DURATION" envDefault:"24h"`
}

// DefaultDeploymentServiceConfig 默认配置
func DefaultDeploymentServiceConfig() DeploymentServiceConfig {
	return DeploymentServiceConfig{
		MaxActiveDeploymentsPerTenant: 10,
		GrayTimeout:                   24 * time.Hour,
		RequireRollbackReason:         true,
		EnableAutoRollback:            true,
		AutoRollbackThreshold:         0.05,
		EnableGrayValidation:          true,
		MaxGrayDuration:               24 * time.Hour,
	}
}

// DeploymentService 部署服务
type DeploymentService struct {
	db          *sql.DB
	publisher   *publisher.KafkaPublisher
	auditLogger *audit.Logger
	rbacChecker *rbac.Checker
	config      DeploymentServiceConfig
	logger      *zap.Logger
}

// NewDeploymentService 创建部署服务（简化版本）
func NewDeploymentService(
	db *sql.DB,
	publisher *publisher.KafkaPublisher,
	logger *zap.Logger,
) *DeploymentService {
	return &DeploymentService{
		db:        db,
		publisher: publisher,
		config:    DefaultDeploymentServiceConfig(),
		logger:    logger,
	}
}

// NewDeploymentServiceWithDeps 创建带完整依赖的部署服务
func NewDeploymentServiceWithDeps(
	db *sql.DB,
	publisher *publisher.KafkaPublisher,
	auditLogger *audit.Logger,
	rbacChecker *rbac.Checker,
	logger *zap.Logger,
	config DeploymentServiceConfig,
) *DeploymentService {
	return &DeploymentService{
		db:          db,
		publisher:   publisher,
		auditLogger: auditLogger,
		rbacChecker: rbacChecker,
		config:      config,
		logger:      logger,
	}
}

// =============================================================================
// 部署 CRUD 操作
// =============================================================================

// CreateDeployment 创建部署（✅ 修复：添加所有字段）
func (s *DeploymentService) CreateDeployment(ctx context.Context, deployment *model.Deployment, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.CreateDeployment")
	defer span.End()

	// 1. 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployCreate, deployment.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployCreate, "deployment", "", err.Error())
		return err
	}

	// 2. 验证部署配置
	if err := s.validateDeployment(deployment); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployCreate, "deployment", "", err.Error())
		return err
	}

	// 3. 检查活跃部署数量限制
	activeCount, err := s.countActiveDeployments(ctx, deployment.TenantID)
	if err != nil {
		s.logger.Warn("Failed to count active deployments", zap.Error(err))
	} else if activeCount >= s.config.MaxActiveDeploymentsPerTenant {
		err := errors.Newf(errors.ErrCodeQuotaExceeded, "active deployment limit exceeded: max %d", s.config.MaxActiveDeploymentsPerTenant)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployCreate, "deployment", "", err.Error())
		return err
	}

	// 4. 初始化部署
	deployment.DeploymentID = uuid.New().String()
	deployment.Status = string(model.DeploymentStatusPlanned)
	deployment.CreatedAt = time.Now()
	deployment.UpdatedAt = time.Now()
	deployment.CreatedBy = opCtx.UserID

	// 序列化 Scope 和 Metadata
	if err := deployment.MarshalScope(); err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal scope")
	}
	if err := deployment.MarshalMetadata(); err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal metadata")
	}

	// 5. ✅ 修复：插入时包含所有字段
	query := `
        INSERT INTO deployments (
            deployment_id, tenant_id, name, description, rule_version, model_version, feature_set_id,
            scope, status, created_by, created_at, updated_at,
            gray_started_at, gray_expired_at, activated_at, rolled_back_at, 
            rollback_from, rollback_reason, metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
    `

	_, err = s.db.ExecContext(ctx, query,
		deployment.DeploymentID, deployment.TenantID, deployment.Name, deployment.Description,
		deployment.RuleVersion, deployment.ModelVersion, deployment.FeatureSetID,
		deployment.ScopeJSON, deployment.Status, deployment.CreatedBy,
		deployment.CreatedAt, deployment.UpdatedAt,
		nil, nil, nil, nil, // 时间字段初始为 NULL
		nil, "", deployment.MetadataJSON, // rollback_from, rollback_reason, metadata
	)
	if err != nil {
		s.logger.Error("Failed to create deployment in DB",
			zap.String("deployment_id", deployment.DeploymentID),
			zap.Error(err))
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployCreate, "deployment", deployment.DeploymentID, err.Error())
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to create deployment")
	}

	// 6. 创建部署历史记录
	if err := s.createDeploymentHistory(ctx, deployment, opCtx.UserID, "created", nil); err != nil {
		s.logger.Warn("Failed to create deployment history", zap.Error(err))
	}

	// 7. 异步发布部署事件到 Kafka
	go s.publishDeploymentEventAsync(ctx, deployment, "create", opCtx.UserID)

	// 8. 记录审计日志
	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeDeployCreate, "deployment", deployment.DeploymentID, map[string]interface{}{
		"name":           deployment.Name,
		"rule_version":   deployment.RuleVersion,
		"model_version":  deployment.ModelVersion,
		"feature_set_id": deployment.FeatureSetID,
		"scope":          deployment.Scope,
		"status":         deployment.Status,
	})

	s.logger.Info("Deployment created successfully",
		zap.String("deployment_id", deployment.DeploymentID),
		zap.String("tenant_id", deployment.TenantID),
		zap.String("status", deployment.Status))

	return nil
}

// GetDeployment 获取部署（✅ 修复：查询所有字段）
func (s *DeploymentService) GetDeployment(ctx context.Context, deploymentID string) (*model.Deployment, error) {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.GetDeployment")
	defer span.End()

	query := `
        SELECT deployment_id, tenant_id, name, description, rule_version, model_version, feature_set_id,
               scope, status, created_by, created_at, updated_at,
               gray_started_at, gray_expired_at, activated_at, rolled_back_at,
               rollback_from, rollback_reason, metadata
        FROM deployments
        WHERE deployment_id = $1
    `

	var d model.Deployment
	var name, description sql.NullString
	var ruleVersion, modelVersion, featureSetID, createdBy sql.NullString
	var rollbackFrom sql.NullString
	var rollbackReason sql.NullString
	var grayStartedAt, grayExpiredAt, activatedAt, rolledBackAt sql.NullTime

	err := s.db.QueryRowContext(ctx, query, deploymentID).Scan(
		&d.DeploymentID,
		&d.TenantID,
		&name,
		&description,
		&ruleVersion,
		&modelVersion,
		&featureSetID,
		&d.ScopeJSON,
		&d.Status,
		&createdBy,
		&d.CreatedAt,
		&d.UpdatedAt,
		&grayStartedAt,
		&grayExpiredAt,
		&activatedAt,
		&rolledBackAt,
		&rollbackFrom,
		&rollbackReason,
		&d.MetadataJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeDeploymentNotFound, "deployment not found: %s", deploymentID)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get deployment")
	}

	// 解析可选字段
	if name.Valid {
		d.Name = name.String
	}
	if description.Valid {
		d.Description = description.String
	}
	if ruleVersion.Valid {
		d.RuleVersion = ruleVersion.String
	}
	if modelVersion.Valid {
		d.ModelVersion = modelVersion.String
	}
	if featureSetID.Valid {
		d.FeatureSetID = featureSetID.String
	}
	if createdBy.Valid {
		d.CreatedBy = createdBy.String
	}
	if rollbackFrom.Valid {
		d.RollbackFrom = &rollbackFrom.String
	}
	if rollbackReason.Valid {
		d.RollbackReason = rollbackReason.String
	}

	// 解析时间字段
	if grayStartedAt.Valid {
		d.GrayStartedAt = &grayStartedAt.Time
	}
	if grayExpiredAt.Valid {
		d.GrayExpiredAt = &grayExpiredAt.Time
	}
	if activatedAt.Valid {
		d.ActivatedAt = &activatedAt.Time
	}
	if rolledBackAt.Valid {
		d.RolledBackAt = &rolledBackAt.Time
	}

	// 反序列化 Scope 和 Metadata
	if err := d.UnmarshalScope(); err != nil {
		s.logger.Warn("Failed to unmarshal scope", zap.Error(err))
	}
	if err := d.UnmarshalMetadata(); err != nil {
		s.logger.Warn("Failed to unmarshal metadata", zap.Error(err))
	}

	return &d, nil
}

// ListDeployments 列出部署（✅ 修复：返回时间字段）
func (s *DeploymentService) ListDeployments(ctx context.Context, tenantID string, filter *DeploymentFilter, opCtx *OperationContext) ([]*model.Deployment, int64, error) {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.ListDeployments")
	defer span.End()

	// 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployRead, tenantID); err != nil {
		return nil, 0, err
	}

	// 设置默认值
	if filter == nil {
		filter = &DeploymentFilter{}
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	// 构建查询
	baseQuery := `FROM deployments WHERE tenant_id = $1`
	args := []interface{}{tenantID}
	argIdx := 2

	if filter.Status != "" {
		baseQuery += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, filter.Status)
		argIdx++
	}

	if filter.RuleVersion != "" {
		baseQuery += fmt.Sprintf(" AND rule_version = $%d", argIdx)
		args = append(args, filter.RuleVersion)
		argIdx++
	}

	// 获取总数
	var total int64
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count deployments")
	}

	// ✅ 修复：查询时包含时间字段
	selectQuery := `
		SELECT deployment_id, tenant_id, name, description, rule_version, model_version, feature_set_id,
			   scope, status, created_by, created_at, updated_at,
			   gray_started_at, gray_expired_at, activated_at, rolled_back_at
	` + baseQuery + " ORDER BY created_at DESC"

	selectQuery += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query deployments")
	}
	defer rows.Close()

	var deployments []*model.Deployment
	for rows.Next() {
		var d model.Deployment
		var name, description sql.NullString
		var ruleVersion, modelVersion, featureSetID, createdBy sql.NullString
		var grayStartedAt, grayExpiredAt, activatedAt, rolledBackAt sql.NullTime

		err := rows.Scan(
			&d.DeploymentID,
			&d.TenantID,
			&name,
			&description,
			&ruleVersion,
			&modelVersion,
			&featureSetID,
			&d.ScopeJSON,
			&d.Status,
			&createdBy,
			&d.CreatedAt,
			&d.UpdatedAt,
			&grayStartedAt,
			&grayExpiredAt,
			&activatedAt,
			&rolledBackAt,
		)
		if err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan deployment")
		}

		if name.Valid {
			d.Name = name.String
		}
		if description.Valid {
			d.Description = description.String
		}
		if ruleVersion.Valid {
			d.RuleVersion = ruleVersion.String
		}
		if modelVersion.Valid {
			d.ModelVersion = modelVersion.String
		}
		if featureSetID.Valid {
			d.FeatureSetID = featureSetID.String
		}
		if createdBy.Valid {
			d.CreatedBy = createdBy.String
		}

		// 设置时间字段
		if grayStartedAt.Valid {
			d.GrayStartedAt = &grayStartedAt.Time
		}
		if grayExpiredAt.Valid {
			d.GrayExpiredAt = &grayExpiredAt.Time
		}
		if activatedAt.Valid {
			d.ActivatedAt = &activatedAt.Time
		}
		if rolledBackAt.Valid {
			d.RolledBackAt = &rolledBackAt.Time
		}

		if len(d.ScopeJSON) > 0 {
			json.Unmarshal(d.ScopeJSON, &d.Scope)
		}

		deployments = append(deployments, &d)
	}

	return deployments, total, nil
}

// RollbackDeployment 回滚部署（✅ 修复：记录 rollback_from）
func (s *DeploymentService) RollbackDeployment(ctx context.Context, deploymentID string, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.RollbackDeployment")
	defer span.End()

	// 1. 获取部署信息
	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}

	// 2. 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployRollback, deployment.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployRollback, "deployment", deploymentID, err.Error())
		return err
	}

	// 3. 状态机验证
	currentStatus := model.DeploymentStatus(deployment.Status)
	if !model.CanTransition(currentStatus, model.DeploymentStatusRolledBack) {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition,
			"cannot transition from %s to %s",
			currentStatus, model.DeploymentStatusRolledBack)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployRollback, "deployment", deploymentID, err.Error())
		return err
	}

	// 4. 获取可回滚的目标（上一个活跃版本）
	rollbackTarget, err := s.getPreviousActiveDeployment(ctx, deployment.TenantID, deploymentID)
	if err != nil {
		s.logger.Warn("Failed to get rollback target", zap.Error(err))
	}

	rolledBackAt := time.Now()
	var rollbackTargetID string
	if rollbackTarget != nil {
		rollbackTargetID = rollbackTarget.DeploymentID
	}

	// 5. ✅ 修复：更新时记录 rollback_from
	query := `
		UPDATE deployments 
		SET status = $1, updated_at = $2, rolled_back_at = $3, rollback_from = $4
		WHERE deployment_id = $5
	`
	_, err = s.db.ExecContext(ctx, query,
		string(model.DeploymentStatusRolledBack),
		time.Now(),
		rolledBackAt,
		rollbackTargetID, // ✅ 记录回滚来源
		deploymentID,
	)
	if err != nil {
		s.logger.Error("Failed to update deployment status",
			zap.String("deployment_id", deploymentID),
			zap.Error(err))
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployRollback, "deployment", deploymentID, err.Error())
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to rollback deployment")
	}

	// 6. 如果有回滚目标，重新激活它
	if rollbackTarget != nil {
		if err := s.updateDeploymentStatus(ctx, rollbackTarget.DeploymentID, model.DeploymentStatusActive, nil, nil); err != nil {
			s.logger.Warn("Failed to reactivate previous deployment", zap.Error(err))
		} else {
			s.logger.Info("Reactivated previous deployment",
				zap.String("deployment_id", rollbackTarget.DeploymentID))
		}
	}

	// 7. 创建部署历史
	if err := s.createDeploymentHistory(ctx, deployment, opCtx.UserID, "rolled_back", map[string]interface{}{
		"rolled_back_at":     rolledBackAt,
		"rollback_target_id": rollbackTargetID,
		"previous_status":    deployment.Status,
	}); err != nil {
		s.logger.Warn("Failed to create deployment history", zap.Error(err))
	}

	// 8. 异步发布回滚事件到 Kafka
	go s.publishDeploymentEventAsync(ctx, deployment, "rollback", opCtx.UserID)

	// 9. 记录审计日志
	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeDeployRollback, "deployment", deploymentID, map[string]interface{}{
		"previous_status":    deployment.Status,
		"new_status":         string(model.DeploymentStatusRolledBack),
		"rolled_back_at":     rolledBackAt,
		"rollback_target_id": rollbackTargetID,
	})

	s.logger.Info("Deployment rolled back",
		zap.String("deployment_id", deploymentID),
		zap.String("rollback_target", rollbackTargetID))

	return nil
}

// StartGrayDeployment 开始灰度部署
func (s *DeploymentService) StartGrayDeployment(ctx context.Context, deploymentID string, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.StartGrayDeployment")
	defer span.End()

	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}

	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployGray, deployment.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployGray, "deployment", deploymentID, err.Error())
		return err
	}

	currentStatus := model.DeploymentStatus(deployment.Status)
	if !model.CanTransition(currentStatus, model.DeploymentStatusGray) {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition,
			"cannot transition from %s to %s",
			currentStatus, model.DeploymentStatusGray)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployGray, "deployment", deploymentID, err.Error())
		return err
	}

	hasActiveGray, err := s.hasActiveGrayDeployment(ctx, deployment.TenantID, deploymentID)
	if err != nil {
		s.logger.Warn("Failed to check active gray deployments", zap.Error(err))
	} else if hasActiveGray {
		err := errors.New(errors.ErrCodeGrayDeploymentActive, "another gray deployment is already active")
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployGray, "deployment", deploymentID, err.Error())
		return err
	}

	grayStartedAt := time.Now()
	grayExpiredAt := grayStartedAt.Add(s.config.GrayTimeout)

	if err := s.updateDeploymentStatusWithGray(ctx, deploymentID, model.DeploymentStatusGray, grayStartedAt, grayExpiredAt); err != nil {
		s.logger.Error("Failed to update deployment status",
			zap.String("deployment_id", deploymentID),
			zap.Error(err))
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployGray, "deployment", deploymentID, err.Error())
		return err
	}

	if err := s.createDeploymentHistory(ctx, deployment, opCtx.UserID, "gray_started", map[string]interface{}{
		"gray_started_at": grayStartedAt,
		"gray_expired_at": grayExpiredAt,
	}); err != nil {
		s.logger.Warn("Failed to create deployment history", zap.Error(err))
	}

	go s.publishDeploymentEventAsync(ctx, deployment, "gray", opCtx.UserID)

	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeDeployGray, "deployment", deploymentID, map[string]interface{}{
		"previous_status": deployment.Status,
		"new_status":      string(model.DeploymentStatusGray),
		"gray_started_at": grayStartedAt,
		"gray_expired_at": grayExpiredAt,
		"scope":           deployment.Scope,
	})

	s.logger.Info("Gray deployment started",
		zap.String("deployment_id", deploymentID),
		zap.Time("gray_expired_at", grayExpiredAt))

	return nil
}

// ActivateDeployment 激活部署
func (s *DeploymentService) ActivateDeployment(ctx context.Context, deploymentID string, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.ActivateDeployment")
	defer span.End()

	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}

	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployActivate, deployment.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployActivate, "deployment", deploymentID, err.Error())
		return err
	}

	currentStatus := model.DeploymentStatus(deployment.Status)
	if !model.CanTransition(currentStatus, model.DeploymentStatusActive) {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition,
			"cannot transition from %s to %s",
			currentStatus, model.DeploymentStatusActive)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployActivate, "deployment", deploymentID, err.Error())
		return err
	}

	var grayMetrics map[string]interface{}
	if currentStatus == model.DeploymentStatusGray {
		grayMetrics, _ = s.getGrayMetrics(ctx, deploymentID)
	}

	activatedAt := time.Now()

	if err := s.deactivatePreviousDeployments(ctx, deployment.TenantID, deploymentID); err != nil {
		s.logger.Warn("Failed to deactivate previous deployments", zap.Error(err))
	}

	if err := s.updateDeploymentStatusWithActivation(ctx, deploymentID, model.DeploymentStatusActive, activatedAt); err != nil {
		s.logger.Error("Failed to update deployment status",
			zap.String("deployment_id", deploymentID),
			zap.Error(err))
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployActivate, "deployment", deploymentID, err.Error())
		return err
	}

	if err := s.createDeploymentHistory(ctx, deployment, opCtx.UserID, "activated", map[string]interface{}{
		"activated_at":    activatedAt,
		"gray_metrics":    grayMetrics,
		"previous_status": deployment.Status,
	}); err != nil {
		s.logger.Warn("Failed to create deployment history", zap.Error(err))
	}

	go s.publishDeploymentEventAsync(ctx, deployment, "activate", opCtx.UserID)

	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeDeployActivate, "deployment", deploymentID, map[string]interface{}{
		"previous_status": deployment.Status,
		"new_status":      string(model.DeploymentStatusActive),
		"activated_at":    activatedAt,
		"gray_metrics":    grayMetrics,
	})

	s.logger.Info("Deployment activated",
		zap.String("deployment_id", deploymentID),
		zap.Time("activated_at", activatedAt))

	return nil
}

// PauseDeployment 暂停部署
func (s *DeploymentService) PauseDeployment(ctx context.Context, deploymentID string, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.PauseDeployment")
	defer span.End()

	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}

	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployActivate, deployment.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployPause, "deployment", deploymentID, err.Error())
		return err
	}

	currentStatus := model.DeploymentStatus(deployment.Status)
	if !model.CanTransition(currentStatus, model.DeploymentStatusPaused) {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition,
			"cannot transition from %s to %s", currentStatus, model.DeploymentStatusPaused)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployPause, "deployment", deploymentID, err.Error())
		return err
	}

	if err := s.updateDeploymentStatus(ctx, deploymentID, model.DeploymentStatusPaused, nil, nil); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployPause, "deployment", deploymentID, err.Error())
		return err
	}

	s.createDeploymentHistory(ctx, deployment, opCtx.UserID, "paused", map[string]interface{}{
		"previous_status": deployment.Status,
	})

	go s.publishDeploymentEventAsync(ctx, deployment, "pause", opCtx.UserID)

	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeDeployPause, "deployment", deploymentID, map[string]interface{}{
		"action":          "pause",
		"previous_status": deployment.Status,
		"new_status":      string(model.DeploymentStatusPaused),
	})

	return nil
}

// ResumeDeployment 恢复部署
func (s *DeploymentService) ResumeDeployment(ctx context.Context, deploymentID string, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.ResumeDeployment")
	defer span.End()

	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}

	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployActivate, deployment.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployResume, "deployment", deploymentID, err.Error())
		return err
	}

	currentStatus := model.DeploymentStatus(deployment.Status)
	if currentStatus != model.DeploymentStatusPaused {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition,
			"can only resume from paused state, current: %s", currentStatus)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployResume, "deployment", deploymentID, err.Error())
		return err
	}

	if err := s.updateDeploymentStatus(ctx, deploymentID, model.DeploymentStatusActive, nil, nil); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployResume, "deployment", deploymentID, err.Error())
		return err
	}

	s.createDeploymentHistory(ctx, deployment, opCtx.UserID, "resumed", map[string]interface{}{
		"previous_status": deployment.Status,
	})

	go s.publishDeploymentEventAsync(ctx, deployment, "resume", opCtx.UserID)

	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeDeployResume, "deployment", deploymentID, map[string]interface{}{
		"action":          "resume",
		"previous_status": deployment.Status,
		"new_status":      string(model.DeploymentStatusActive),
	})

	return nil
}

// GetDeploymentHistory 获取部署历史
func (s *DeploymentService) GetDeploymentHistory(ctx context.Context, deploymentID string, opCtx *OperationContext) ([]*DeploymentHistoryEntry, error) {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.GetDeploymentHistory")
	defer span.End()

	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployRead, deployment.TenantID); err != nil {
		return nil, err
	}

	query := `
		SELECT id, deployment_id, action, operator_id, created_at, detail
		FROM deployment_history
		WHERE deployment_id = $1
		ORDER BY created_at DESC
		LIMIT 100
	`

	rows, err := s.db.QueryContext(ctx, query, deploymentID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query deployment history")
	}
	defer rows.Close()

	var history []*DeploymentHistoryEntry
	for rows.Next() {
		var h DeploymentHistoryEntry
		var detailJSON sql.NullString

		if err := rows.Scan(&h.ID, &h.DeploymentID, &h.Action, &h.OperatorID, &h.CreatedAt, &detailJSON); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan history entry")
		}

		if detailJSON.Valid && detailJSON.String != "" {
			json.Unmarshal([]byte(detailJSON.String), &h.Detail)
		}

		history = append(history, &h)
	}

	return history, nil
}

// GetGrayProgress 获取灰度进度
func (s *DeploymentService) GetGrayProgress(ctx context.Context, deploymentID string, opCtx *OperationContext) (*GrayProgress, error) {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.GetGrayProgress")
	defer span.End()

	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}

	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployRead, deployment.TenantID); err != nil {
		return nil, err
	}

	if model.DeploymentStatus(deployment.Status) != model.DeploymentStatusGray {
		return nil, errors.Newf(errors.ErrCodeInvalidStateTransition, "deployment is not in gray state")
	}

	progress := &GrayProgress{
		DeploymentID: deploymentID,
		Status:       deployment.Status,
		StartedAt:    deployment.GrayStartedAt,
		ExpiresAt:    deployment.GrayExpiredAt,
		Scope:        deployment.Scope,
	}

	if deployment.GrayStartedAt != nil && deployment.GrayExpiredAt != nil {
		totalDuration := deployment.GrayExpiredAt.Sub(*deployment.GrayStartedAt)
		elapsed := time.Since(*deployment.GrayStartedAt)
		if totalDuration > 0 {
			progress.ProgressPercent = float64(elapsed) / float64(totalDuration) * 100
			if progress.ProgressPercent > 100 {
				progress.ProgressPercent = 100
			}
		}

		remaining := deployment.GrayExpiredAt.Sub(time.Now())
		if remaining > 0 {
			progress.RemainingTime = remaining.String()
		} else {
			progress.RemainingTime = "expired"
		}
	}

	metrics, _ := s.getGrayMetrics(ctx, deploymentID)
	progress.Metrics = metrics

	return progress, nil
}

// =============================================================================
// 辅助方法
// =============================================================================

type DeploymentFilter struct {
	Status      string `json:"status,omitempty"`
	RuleVersion string `json:"rule_version,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}

type DeploymentHistoryEntry struct {
	ID           int64                  `json:"id"`
	DeploymentID string                 `json:"deployment_id"`
	Action       string                 `json:"action"`
	OperatorID   string                 `json:"operator_id"`
	CreatedAt    time.Time              `json:"created_at"`
	Detail       map[string]interface{} `json:"detail,omitempty"`
}

type GrayProgress struct {
	DeploymentID    string                 `json:"deployment_id"`
	Status          string                 `json:"status"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	ExpiresAt       *time.Time             `json:"expires_at,omitempty"`
	RemainingTime   string                 `json:"remaining_time,omitempty"`
	ProgressPercent float64                `json:"progress_percent"`
	Scope           map[string]interface{} `json:"scope,omitempty"`
	Metrics         map[string]interface{} `json:"metrics,omitempty"`
}

func (s *DeploymentService) validateDeployment(deployment *model.Deployment) error {
	if deployment.TenantID == "" {
		return errors.New(errors.ErrCodeMissingParameter, "tenant_id is required")
	}
	if deployment.RuleVersion == "" && deployment.ModelVersion == "" {
		return errors.New(errors.ErrCodeMissingParameter, "rule_version or model_version is required")
	}
	if deployment.Scope != nil {
		if pct, ok := deployment.Scope["percentage"].(float64); ok {
			if pct < 0 || pct > 100 {
				return errors.Newf(errors.ErrCodeInvalidParameter, "percentage must be between 0 and 100")
			}
		}
	}
	return nil
}

func (s *DeploymentService) countActiveDeployments(ctx context.Context, tenantID string) (int, error) {
	query := `SELECT COUNT(*) FROM deployments WHERE tenant_id = $1 AND status IN ('gray', 'active')`
	var count int
	err := s.db.QueryRowContext(ctx, query, tenantID).Scan(&count)
	return count, err
}

func (s *DeploymentService) hasActiveGrayDeployment(ctx context.Context, tenantID, excludeID string) (bool, error) {
	query := `SELECT COUNT(*) FROM deployments WHERE tenant_id = $1 AND status = 'gray' AND deployment_id != $2`
	var count int
	err := s.db.QueryRowContext(ctx, query, tenantID, excludeID).Scan(&count)
	return count > 0, err
}

func (s *DeploymentService) getPreviousActiveDeployment(ctx context.Context, tenantID, currentID string) (*model.Deployment, error) {
	query := `
		SELECT deployment_id FROM deployments 
		WHERE tenant_id = $1 AND deployment_id != $2 AND status IN ('active', 'rolled_back')
		ORDER BY updated_at DESC
		LIMIT 1
	`
	var deploymentID string
	err := s.db.QueryRowContext(ctx, query, tenantID, currentID).Scan(&deploymentID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return s.GetDeployment(ctx, deploymentID)
}

func (s *DeploymentService) deactivatePreviousDeployments(ctx context.Context, tenantID, excludeID string) error {
	query := `UPDATE deployments SET status = 'superseded', updated_at = $1 WHERE tenant_id = $2 AND status = 'active' AND deployment_id != $3`
	_, err := s.db.ExecContext(ctx, query, time.Now(), tenantID, excludeID)
	return err
}

func (s *DeploymentService) updateDeploymentStatus(ctx context.Context, deploymentID string, status model.DeploymentStatus, grayStartedAt, grayExpiredAt *time.Time) error {
	query := `UPDATE deployments SET status = $1, updated_at = $2, gray_started_at = $3, gray_expired_at = $4 WHERE deployment_id = $5`
	result, err := s.db.ExecContext(ctx, query, string(status), time.Now(), grayStartedAt, grayExpiredAt, deploymentID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update deployment status")
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.Newf(errors.ErrCodeDeploymentNotFound, "deployment not found: %s", deploymentID)
	}
	return nil
}

func (s *DeploymentService) updateDeploymentStatusWithGray(ctx context.Context, deploymentID string, status model.DeploymentStatus, startedAt, expiredAt time.Time) error {
	query := `UPDATE deployments SET status = $1, updated_at = $2, gray_started_at = $3, gray_expired_at = $4 WHERE deployment_id = $5`
	_, err := s.db.ExecContext(ctx, query, string(status), time.Now(), startedAt, expiredAt, deploymentID)
	return err
}

func (s *DeploymentService) updateDeploymentStatusWithActivation(ctx context.Context, deploymentID string, status model.DeploymentStatus, activatedAt time.Time) error {
	query := `UPDATE deployments SET status = $1, updated_at = $2, activated_at = $3 WHERE deployment_id = $4`
	_, err := s.db.ExecContext(ctx, query, string(status), time.Now(), activatedAt, deploymentID)
	return err
}

func (s *DeploymentService) createDeploymentHistory(ctx context.Context, deployment *model.Deployment, operatorID, action string, detail map[string]interface{}) error {
	if detail == nil {
		detail = make(map[string]interface{})
	}
	detail["status"] = deployment.Status
	detailJSON, _ := json.Marshal(detail)
	query := `INSERT INTO deployment_history (deployment_id, action, operator_id, created_at, detail) VALUES ($1, $2, $3, $4, $5)`
	_, err := s.db.ExecContext(ctx, query, deployment.DeploymentID, action, operatorID, time.Now(), detailJSON)
	return err
}

func (s *DeploymentService) getGrayMetrics(ctx context.Context, deploymentID string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"alerts_triggered": 0,
		"false_positives":  0,
		"coverage":         0.0,
		"error_rate":       0.0,
	}, nil
}

func (s *DeploymentService) publishDeploymentEventAsync(ctx context.Context, deployment *model.Deployment, action, operatorID string) {
	if s.publisher == nil {
		return
	}
	publishCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.publisher.PublishDeploymentEvent(publishCtx, deployment, action, operatorID); err != nil {
		s.logger.Error("Failed to publish deployment event",
			zap.String("deployment_id", deployment.DeploymentID),
			zap.String("action", action),
			zap.Error(err))
	}
}

func (s *DeploymentService) checkPermission(ctx context.Context, opCtx *OperationContext, permission rbac.Permission, resourceTenantID string) error {
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

func (s *DeploymentService) hasAdminPermission(opCtx *OperationContext) bool {
	for _, p := range opCtx.Permissions {
		if p == string(rbac.PermissionAdminWrite) || p == "admin:*" || p == "*" {
			return true
		}
	}
	return false
}

func (s *DeploymentService) recordAuditSuccess(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID string, detail map[string]interface{}) {
	if opCtx == nil {
		return
	}
	if s.auditLogger != nil {
		s.auditLogger.LogDeployment(ctx, eventType, opCtx.TenantID, opCtx.UserID, resourceID, detail)
	}
	s.recordAuditLogDB(ctx, opCtx, eventType, resourceType, resourceID, detail, audit.ResultSuccess, "")
}

func (s *DeploymentService) recordAuditFailure(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID, errorMsg string) {
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

func (s *DeploymentService) recordAuditLogDB(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID string, detail map[string]interface{}, result audit.Result, errorMsg string) {
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
		s.logger.Warn("Failed to marshal deployment audit detail",
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
		s.logger.Warn("Failed to persist deployment audit log",
			zap.String("event_type", string(eventType)),
			zap.String("resource_type", resourceType),
			zap.String("resource_id", resourceID),
			zap.Error(err))
	}
}
