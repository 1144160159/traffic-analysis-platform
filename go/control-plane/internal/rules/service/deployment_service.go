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
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
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
	db             *sql.DB
	publisher      *publisher.KafkaPublisher
	auditLogger    *audit.Logger
	rbacChecker    *rbac.Checker
	config         DeploymentServiceConfig
	logger         *zap.Logger
	outboxStopCh   chan struct{}
	outboxStopOnce sync.Once
	outboxWG       sync.WaitGroup
	outboxInstance string
}

const (
	deploymentOutboxProcessInterval = 5 * time.Second
	deploymentOutboxRetryDelay      = 5 * time.Second
	deploymentOutboxMaxRetries      = 10
	deploymentReleaseLineSQL        = `COALESCE(NULLIF(scope->>'release_line', ''), CASE
		WHEN (CASE WHEN COALESCE(rule_version, '') <> '' THEN 1 ELSE 0 END + CASE WHEN COALESCE(model_version, '') <> '' THEN 1 ELSE 0 END + CASE WHEN COALESCE(feature_set_id, '') <> '' THEN 1 ELSE 0 END) > 1 THEN 'detection-bundle'
		WHEN COALESCE(rule_version, '') <> '' THEN 'ruleset'
		WHEN COALESCE(model_version, '') <> '' THEN 'model'
		WHEN COALESCE(feature_set_id, '') <> '' THEN 'feature-set'
		ELSE 'deployment' END)`
)

// NewDeploymentService 创建部署服务（简化版本）
func NewDeploymentService(
	db *sql.DB,
	publisher *publisher.KafkaPublisher,
	logger *zap.Logger,
) *DeploymentService {
	service := &DeploymentService{
		db:             db,
		publisher:      publisher,
		config:         DefaultDeploymentServiceConfig(),
		logger:         logger,
		outboxStopCh:   make(chan struct{}),
		outboxInstance: uuid.NewString(),
	}
	service.startDeploymentOutboxProcessorIfEnabled()
	return service
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
	service := &DeploymentService{
		db:             db,
		publisher:      publisher,
		auditLogger:    auditLogger,
		rbacChecker:    rbacChecker,
		config:         config,
		logger:         logger,
		outboxStopCh:   make(chan struct{}),
		outboxInstance: uuid.NewString(),
	}
	service.startDeploymentOutboxProcessorIfEnabled()
	return service
}

// Close stops the durable deployment-event dispatcher. Database and publisher
// ownership stays with main, so this method only stops the service goroutine.
func (s *DeploymentService) Close() {
	if s == nil || s.outboxStopCh == nil {
		return
	}
	s.outboxStopOnce.Do(func() { close(s.outboxStopCh) })
	s.outboxWG.Wait()
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

	// 3. 初始化部署。活跃配额在真正开始灰度时检查；保存 planned
	// 草案不会消耗运行配额。
	deployment.DeploymentID = uuid.New().String()
	deployment.Status = string(model.DeploymentStatusPlanned)
	deployment.CreatedAt = time.Now()
	deployment.UpdatedAt = time.Now()
	deployment.CreatedBy = opCtx.UserID
	ensureDeploymentReleaseLine(deployment)
	if deployment.Metadata == nil {
		deployment.Metadata = map[string]interface{}{}
	}
	// Creating a deployment is the persisted "save draft" transition. Keep the
	// workflow state in the same transaction so the next precheck never depends
	// on a second client-side click that can be lost during navigation or retry.
	deployment.Metadata["workflow"] = map[string]interface{}{
		"stage":            "draft_saved",
		"operation":        "deploy",
		"updated_at":       deployment.UpdatedAt,
		"updated_by":       opCtx.UserID,
		"configuration":    map[string]interface{}{"scope": deployment.Scope},
		"precheck_status":  "",
		"precheck_results": []interface{}{},
	}

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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin deployment create transaction")
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, query,
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

	detail := map[string]interface{}{
		"name": deployment.Name, "rule_version": deployment.RuleVersion, "model_version": deployment.ModelVersion,
		"feature_set_id": deployment.FeatureSetID, "scope": deployment.Scope, "status": deployment.Status,
	}
	if err := insertDeploymentHistoryTx(ctx, tx, deployment.DeploymentID, deployment.Status, opCtx.UserID, "created", detail); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment create history")
	}
	if err := insertDeploymentAuditTx(ctx, tx, opCtx, audit.EventTypeDeployCreate, "deployment", deployment.DeploymentID, detail); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment create audit")
	}
	if err := s.insertDeploymentOutboxTx(ctx, tx, deployment, "created", opCtx.UserID, string(model.DeploymentStatusPlanned)); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment create event")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit deployment create transaction")
	}

	if s.auditLogger != nil {
		s.auditLogger.LogDeployment(ctx, audit.EventTypeDeployCreate, opCtx.TenantID, opCtx.UserID, deployment.DeploymentID, detail)
	}

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
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode deployment scope")
	}
	if err := d.UnmarshalMetadata(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode deployment metadata")
	}

	return &d, nil
}

// GetDeploymentForOperation prevents a valid ID from becoming a cross-tenant
// read primitive at the HTTP boundary.
func (s *DeploymentService) GetDeploymentForOperation(ctx context.Context, deploymentID string, opCtx *OperationContext) (*model.Deployment, error) {
	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployRead, deployment.TenantID); err != nil {
		return nil, err
	}
	return deployment, nil
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
			   gray_started_at, gray_expired_at, activated_at, rolled_back_at,
			   rollback_from, rollback_reason, metadata, error_message
	` + baseQuery + " ORDER BY created_at DESC, deployment_id DESC"

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
		var rollbackFrom, rollbackReason, errorMessage sql.NullString
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
			&rollbackFrom,
			&rollbackReason,
			&d.MetadataJSON,
			&errorMessage,
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
		if rollbackFrom.Valid {
			d.RollbackFrom = &rollbackFrom.String
		}
		if rollbackReason.Valid {
			d.RollbackReason = rollbackReason.String
		}
		if errorMessage.Valid {
			d.ErrorMessage = errorMessage.String
		}

		if len(d.ScopeJSON) > 0 {
			if err := json.Unmarshal(d.ScopeJSON, &d.Scope); err != nil {
				return nil, 0, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode deployment scope")
			}
		}
		if len(d.MetadataJSON) > 0 {
			if err := json.Unmarshal(d.MetadataJSON, &d.Metadata); err != nil {
				return nil, 0, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode deployment metadata")
			}
		}

		deployments = append(deployments, &d)
	}

	return deployments, total, nil
}

// RollbackDeployment 回滚部署（✅ 修复：记录 rollback_from）
func (s *DeploymentService) RollbackDeployment(ctx context.Context, deploymentID, targetDeploymentID, reason string, opCtx *OperationContext) (rollbackErr error) {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.RollbackDeployment")
	defer span.End()
	defer func() {
		if rollbackErr != nil {
			s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployRollback, "deployment", deploymentID, rollbackErr.Error())
		}
	}()

	// 1. 获取部署信息
	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}

	// 2. 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployRollback, deployment.TenantID); err != nil {
		return err
	}
	approvedConfiguration, err := approvedDeploymentConfiguration(deployment, "rollback")
	if err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if s.config.RequireRollbackReason && reason == "" {
		err := errors.New(errors.ErrCodeMissingParameter, "rollback reason is required")
		return err
	}

	// 3. 状态机验证
	currentStatus := model.DeploymentStatus(deployment.Status)
	if !model.CanTransition(currentStatus, model.DeploymentStatusRolledBack) {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition,
			"cannot transition from %s to %s",
			currentStatus, model.DeploymentStatusRolledBack)
		return err
	}

	// 4. 回滚目标必须由调用方显式选择，避免并发版本变化时静默回滚到错误版本。
	targetDeploymentID = strings.TrimSpace(targetDeploymentID)
	if targetDeploymentID == "" {
		return errors.New(errors.ErrCodeMissingParameter, "rollback target deployment is required")
	}
	approvedTargetID := strings.TrimSpace(fmt.Sprint(approvedConfiguration["target_deployment_id"]))
	approvedReason := strings.TrimSpace(fmt.Sprint(approvedConfiguration["reason"]))
	if targetDeploymentID != approvedTargetID || reason != approvedReason {
		return errors.New(errors.ErrCodeVersionConflict, "rollback target or reason does not match the approved snapshot")
	}
	rollbackTarget, err := s.validateRollbackTarget(ctx, deployment, targetDeploymentID)
	if err != nil {
		return err
	}
	if rollbackTarget == nil {
		return errors.New(errors.ErrCodeInvalidParameter, "no recoverable rollback target is available")
	}

	rolledBackAt := time.Now()
	rollbackTargetID := rollbackTarget.DeploymentID

	// 5. 状态、目标版本、历史与审计必须在同一事务内提交。
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin rollback transaction")
	}
	defer tx.Rollback()
	lockedDeployment, err := lockDeploymentSnapshotTx(ctx, tx, deploymentID, deployment.TenantID)
	if err != nil {
		return err
	}
	if lockedDeployment.Status != deployment.Status {
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "deployment state changed concurrently from %s to %s", deployment.Status, lockedDeployment.Status)
	}
	releaseLine := deploymentReleaseLine(lockedDeployment)
	if err := lockDeploymentCapacityTx(ctx, tx, deployment.TenantID); err != nil {
		return err
	}
	if err := lockDeploymentReleaseLineTx(ctx, tx, deployment.TenantID, releaseLine); err != nil {
		return err
	}
	lockedApprovedConfiguration, err := approvedDeploymentConfiguration(lockedDeployment, "rollback")
	if err != nil {
		return err
	}
	if strings.TrimSpace(fmt.Sprint(lockedApprovedConfiguration["target_deployment_id"])) != targetDeploymentID || strings.TrimSpace(fmt.Sprint(lockedApprovedConfiguration["reason"])) != reason {
		return errors.New(errors.ErrCodeVersionConflict, "rollback request changed after the deployment lock was acquired")
	}
	var lockedTargetStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM deployments WHERE deployment_id = $1 AND tenant_id = $2 FOR UPDATE`, rollbackTarget.DeploymentID, deployment.TenantID).Scan(&lockedTargetStatus); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock rollback target")
	}
	if lockedTargetStatus != rollbackTarget.Status {
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "rollback target changed concurrently from %s to %s", rollbackTarget.Status, lockedTargetStatus)
	}
	if err := s.supersedeActiveReleaseLineTx(ctx, tx, deployment.TenantID, releaseLine, []string{deploymentID, rollbackTarget.DeploymentID}, opCtx.UserID); err != nil {
		return err
	}
	var activeCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployments WHERE tenant_id = $1 AND status IN ('gray', 'active') AND deployment_id NOT IN ($2, $3)`, deployment.TenantID, deploymentID, rollbackTarget.DeploymentID).Scan(&activeCount); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count active deployments before rollback target reactivation")
	}
	if activeCount >= s.config.MaxActiveDeploymentsPerTenant {
		return errors.Newf(errors.ErrCodeQuotaExceeded, "active deployment limit exceeded: max %d", s.config.MaxActiveDeploymentsPerTenant)
	}
	query := `
		UPDATE deployments 
		SET status = $1, updated_at = $2, rolled_back_at = $3, rollback_from = $4, rollback_reason = $5
		WHERE deployment_id = $6 AND tenant_id = $7 AND status = $8
	`
	result, err := tx.ExecContext(ctx, query,
		string(model.DeploymentStatusRolledBack),
		time.Now(),
		rolledBackAt,
		rollbackTargetID,
		reason,
		deploymentID,
		deployment.TenantID,
		deployment.Status,
	)
	if err != nil {
		s.logger.Error("Failed to update deployment status",
			zap.String("deployment_id", deploymentID),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to rollback deployment")
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected != 1 {
		return errors.Newf(errors.ErrCodeDeploymentNotFound, "deployment not found: %s", deploymentID)
	}

	// 6. 如果有回滚目标，在同一事务中重新激活它。
	if rollbackTarget != nil {
		result, err := tx.ExecContext(ctx, `UPDATE deployments SET status = $1, updated_at = $2, gray_started_at = NULL, gray_expired_at = NULL WHERE deployment_id = $3 AND tenant_id = $4 AND status = $5`, string(model.DeploymentStatusActive), time.Now(), rollbackTarget.DeploymentID, deployment.TenantID, rollbackTarget.Status)
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to reactivate rollback target")
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return errors.Newf(errors.ErrCodeDeploymentNotFound, "rollback target not found: %s", rollbackTarget.DeploymentID)
		}
	}

	detail := map[string]interface{}{
		"rolled_back_at":          rolledBackAt,
		"rollback_target_id":      rollbackTargetID,
		"rollback_reason":         reason,
		"previous_status":         deployment.Status,
		"new_status":              string(model.DeploymentStatusRolledBack),
		"release_line":            releaseLine,
		"execution_configuration": approvedConfiguration,
	}
	if err := insertDeploymentHistoryTx(ctx, tx, deployment.DeploymentID, deployment.Status, opCtx.UserID, "rolled_back", detail); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist rollback history")
	}
	if err := insertDeploymentAuditTx(ctx, tx, opCtx, audit.EventTypeDeployRollback, "deployment", deploymentID, detail); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist rollback audit")
	}
	if err := s.insertDeploymentOutboxTx(ctx, tx, deployment, "rolled_back", opCtx.UserID, string(model.DeploymentStatusRolledBack)); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist rollback event")
	}
	if rollbackTarget != nil {
		if err := s.insertDeploymentOutboxTx(ctx, tx, rollbackTarget, "reactivated", opCtx.UserID, string(model.DeploymentStatusActive)); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist rollback target reactivation event")
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit rollback transaction")
	}

	if s.auditLogger != nil {
		s.auditLogger.LogDeployment(ctx, audit.EventTypeDeployRollback, opCtx.TenantID, opCtx.UserID, deploymentID, detail)
	}

	s.logger.Info("Deployment rolled back",
		zap.String("deployment_id", deploymentID),
		zap.String("rollback_target", rollbackTargetID))

	return nil
}

func (s *DeploymentService) validateRollbackTarget(ctx context.Context, deployment *model.Deployment, targetDeploymentID string) (*model.Deployment, error) {
	rollbackTarget, err := s.GetDeployment(ctx, strings.TrimSpace(targetDeploymentID))
	if err != nil {
		return nil, err
	}
	if rollbackTarget.TenantID != deployment.TenantID || rollbackTarget.DeploymentID == deployment.DeploymentID {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "rollback target must be another deployment in the same tenant")
	}
	if rollbackTarget.Status != string(model.DeploymentStatusActive) && rollbackTarget.Status != string(model.DeploymentStatusRolledBack) && rollbackTarget.Status != string(model.DeploymentStatusSuperseded) {
		return nil, errors.Newf(errors.ErrCodeInvalidParameter, "rollback target status %s is not recoverable", rollbackTarget.Status)
	}
	if deploymentReleaseLine(rollbackTarget) != deploymentReleaseLine(deployment) {
		return nil, errors.Newf(errors.ErrCodeInvalidParameter, "rollback target release line %s does not match %s", deploymentReleaseLine(rollbackTarget), deploymentReleaseLine(deployment))
	}
	return rollbackTarget, nil
}

func requireApprovedDeploymentWorkflow(deployment *model.Deployment, operation string) error {
	workflow, ok := deployment.Metadata["workflow"].(map[string]interface{})
	if !ok || workflowString(workflow, "stage") != "approved" || workflowString(workflow, "operation") != operation {
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "%s action requires an approved %s workflow", operation, operation)
	}
	snapshot, ok := workflow["approval_snapshot"].(map[string]interface{})
	if !ok {
		return errors.New(errors.ErrCodeInvalidStateTransition, "approved workflow is missing its immutable approval snapshot")
	}
	hash, err := canonicalValueHash(snapshot)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to verify approved workflow snapshot")
	}
	if workflowString(workflow, "approval_snapshot_hash") != hash {
		return errors.New(errors.ErrCodeInvalidStateTransition, "approved workflow snapshot integrity check failed")
	}
	if workflowString(snapshot, "operation") != operation {
		return errors.New(errors.ErrCodeInvalidStateTransition, "approved workflow operation no longer matches the requested action")
	}
	approvedScope, ok := snapshot["scope"].(map[string]interface{})
	if !ok || !reflect.DeepEqual(normalizeJSONValue(approvedScope), normalizeJSONValue(deploymentScopeForApproval(deployment))) {
		return errors.New(errors.ErrCodeInvalidStateTransition, "deployment scope changed after approval; run precheck and approval again")
	}
	if workflowString(snapshot, "release_line") != deploymentReleaseLine(deployment) {
		return errors.New(errors.ErrCodeInvalidStateTransition, "deployment release line changed after approval")
	}
	if err := requireFreshDeploymentPrecheck(workflow, time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

func requireFreshDeploymentPrecheck(workflow map[string]interface{}, now time.Time) error {
	results, ok := workflow["precheck_results"].([]interface{})
	if !ok || len(results) != 7 {
		return errors.New(errors.ErrCodeInvalidStateTransition, "exactly seven deployment precheck results are required")
	}
	for _, raw := range results {
		result, ok := raw.(map[string]interface{})
		if !ok {
			return errors.New(errors.ErrCodeInvalidStateTransition, "deployment precheck evidence is malformed")
		}
		freshUntil, err := workflowTime(result["fresh_until"])
		if err != nil || !freshUntil.After(now) {
			return errors.Newf(errors.ErrCodeInvalidStateTransition, "deployment precheck %s expired; run precheck again", workflowString(result, "label"))
		}
	}
	return nil
}

func workflowTime(value interface{}) (time.Time, error) {
	switch typed := value.(type) {
	case time.Time:
		return typed, nil
	case string:
		return time.Parse(time.RFC3339Nano, typed)
	default:
		return time.Time{}, fmt.Errorf("unsupported workflow time %T", value)
	}
}

func approvedDeploymentConfiguration(deployment *model.Deployment, operation string) (map[string]interface{}, error) {
	if err := requireApprovedDeploymentWorkflow(deployment, operation); err != nil {
		return nil, err
	}
	workflow := deployment.Metadata["workflow"].(map[string]interface{})
	snapshot := workflow["approval_snapshot"].(map[string]interface{})
	configuration, ok := snapshot["configuration"].(map[string]interface{})
	if !ok {
		return nil, errors.New(errors.ErrCodeInvalidStateTransition, "approved workflow configuration is missing")
	}
	return cloneStringMap(configuration), nil
}

func canonicalValueHash(value interface{}) (string, error) {
	payload, err := json.Marshal(normalizeJSONValue(value))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(payload)), nil
}

func normalizeJSONValue(value interface{}) interface{} {
	payload, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var normalized interface{}
	if err := json.Unmarshal(payload, &normalized); err != nil {
		return value
	}
	return normalized
}

func cloneStringMap(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return map[string]interface{}{}
	}
	cloned, _ := normalizeJSONValue(source).(map[string]interface{})
	if cloned == nil {
		return map[string]interface{}{}
	}
	return cloned
}

func deploymentReleaseLine(deployment *model.Deployment) string {
	if deployment != nil && deployment.Scope != nil {
		if value := strings.TrimSpace(fmt.Sprint(deployment.Scope["release_line"])); value != "" && value != "<nil>" {
			return value
		}
	}
	if deployment == nil {
		return "deployment"
	}
	bound := 0
	if strings.TrimSpace(deployment.RuleVersion) != "" {
		bound++
	}
	if strings.TrimSpace(deployment.ModelVersion) != "" {
		bound++
	}
	if strings.TrimSpace(deployment.FeatureSetID) != "" {
		bound++
	}
	if bound > 1 {
		return "detection-bundle"
	}
	if strings.TrimSpace(deployment.RuleVersion) != "" {
		return "ruleset"
	}
	if strings.TrimSpace(deployment.ModelVersion) != "" {
		return "model"
	}
	if strings.TrimSpace(deployment.FeatureSetID) != "" {
		return "feature-set"
	}
	return "deployment"
}

func ensureDeploymentReleaseLine(deployment *model.Deployment) {
	if deployment.Scope == nil {
		deployment.Scope = map[string]interface{}{}
	}
	if value := strings.TrimSpace(fmt.Sprint(deployment.Scope["release_line"])); value == "" || value == "<nil>" {
		deployment.Scope["release_line"] = deploymentReleaseLine(deployment)
	}
}

func deploymentScopeForApproval(deployment *model.Deployment) map[string]interface{} {
	scope := cloneStringMap(deployment.Scope)
	if value := strings.TrimSpace(fmt.Sprint(scope["release_line"])); value == "" || value == "<nil>" {
		scope["release_line"] = deploymentReleaseLine(deployment)
	}
	return scope
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
	if err := requireApprovedDeploymentWorkflow(deployment, "deploy"); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployGray, "deployment", deploymentID, err.Error())
		return err
	}
	approvedConfiguration, err := approvedDeploymentConfiguration(deployment, "deploy")
	if err != nil {
		return err
	}
	grayPercentage := workflowNumber(approvedConfiguration, "gray_percentage")
	if grayPercentage <= 0 || grayPercentage > 100 {
		return errors.New(errors.ErrCodeInvalidParameter, "approved gray percentage must be greater than 0 and no more than 100")
	}
	executionConfigurationJSON, err := json.Marshal(approvedConfiguration)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode approved execution configuration")
	}

	currentStatus := model.DeploymentStatus(deployment.Status)
	if !model.CanTransition(currentStatus, model.DeploymentStatusGray) {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition,
			"cannot transition from %s to %s",
			currentStatus, model.DeploymentStatusGray)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployGray, "deployment", deploymentID, err.Error())
		return err
	}

	grayStartedAt := time.Now()
	grayExpiredAt := grayStartedAt.Add(s.config.GrayTimeout)
	releaseLine := deploymentReleaseLine(deployment)
	detail := map[string]interface{}{
		"previous_status":         deployment.Status,
		"new_status":              string(model.DeploymentStatusGray),
		"gray_started_at":         grayStartedAt,
		"gray_expired_at":         grayExpiredAt,
		"scope":                   deployment.Scope,
		"release_line":            releaseLine,
		"execution_configuration": approvedConfiguration,
	}
	if err := s.commitDeploymentTransition(ctx, deployment, opCtx, audit.EventTypeDeployGray, "gray_started", "deploy", detail, func(tx *sql.Tx) error {
		if err := lockDeploymentCapacityTx(ctx, tx, deployment.TenantID); err != nil {
			return err
		}
		if err := lockDeploymentReleaseLineTx(ctx, tx, deployment.TenantID, releaseLine); err != nil {
			return err
		}
		var activeCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployments WHERE tenant_id = $1 AND status IN ('gray', 'active') AND deployment_id != $2`, deployment.TenantID, deploymentID).Scan(&activeCount); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count active deployments")
		}
		if activeCount >= s.config.MaxActiveDeploymentsPerTenant {
			return errors.Newf(errors.ErrCodeQuotaExceeded, "active deployment limit exceeded: max %d", s.config.MaxActiveDeploymentsPerTenant)
		}
		var hasActiveGray bool
		grayQuery := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM deployments WHERE tenant_id = $1 AND status = 'gray' AND deployment_id != $2 AND %s = $3)`, deploymentReleaseLineSQL)
		if err := tx.QueryRowContext(ctx, grayQuery, deployment.TenantID, deploymentID, releaseLine).Scan(&hasActiveGray); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to check active gray deployments")
		}
		if hasActiveGray {
			return errors.New(errors.ErrCodeGrayDeploymentActive, "another gray deployment is already active")
		}
		result, err := tx.ExecContext(ctx, `UPDATE deployments SET status = $1, updated_at = $2, gray_started_at = $3, gray_expired_at = $4, scope = jsonb_set(COALESCE(scope, '{}'::jsonb), '{percentage}', to_jsonb($5::numeric), true), metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('execution_plan', $6::jsonb) WHERE deployment_id = $7 AND tenant_id = $8 AND status = $9`, string(model.DeploymentStatusGray), time.Now(), grayStartedAt, grayExpiredAt, grayPercentage, string(executionConfigurationJSON), deploymentID, deployment.TenantID, deployment.Status)
		if err != nil {
			return err
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return errors.Newf(errors.ErrCodeDeploymentNotFound, "deployment not found: %s", deploymentID)
		}
		return nil
	}); err != nil {
		s.logger.Error("Failed to update deployment status",
			zap.String("deployment_id", deploymentID),
			zap.Error(err))
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployGray, "deployment", deploymentID, err.Error())
		return err
	}

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
	if err := requireApprovedDeploymentWorkflow(deployment, "deploy"); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployActivate, "deployment", deploymentID, err.Error())
		return err
	}
	approvedConfiguration, err := approvedDeploymentConfiguration(deployment, "deploy")
	if err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployActivate, "deployment", deploymentID, err.Error())
		return err
	}
	if model.DeploymentStatus(deployment.Status) == model.DeploymentStatusPlanned && workflowNumber(approvedConfiguration, "gray_percentage") != 100 {
		err := errors.New(errors.ErrCodeInvalidStateTransition, "planned deployment must start gray unless the approved percentage is 100")
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployActivate, "deployment", deploymentID, err.Error())
		return err
	}
	executionConfigurationJSON, err := json.Marshal(approvedConfiguration)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to encode approved activation configuration")
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
	releaseLine := deploymentReleaseLine(deployment)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin activation transaction")
	}
	defer tx.Rollback()
	lockedDeployment, err := lockDeploymentSnapshotTx(ctx, tx, deploymentID, deployment.TenantID)
	if err != nil {
		return err
	}
	if lockedDeployment.Status != deployment.Status {
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "deployment state changed concurrently from %s to %s", deployment.Status, lockedDeployment.Status)
	}
	if err := requireApprovedDeploymentWorkflow(lockedDeployment, "deploy"); err != nil {
		return err
	}
	lockedApprovedConfiguration, err := approvedDeploymentConfiguration(lockedDeployment, "deploy")
	if err != nil {
		return err
	}
	if model.DeploymentStatus(lockedDeployment.Status) == model.DeploymentStatusPlanned && workflowNumber(lockedApprovedConfiguration, "gray_percentage") != 100 {
		return errors.New(errors.ErrCodeInvalidStateTransition, "planned deployment must start gray unless the approved percentage is 100")
	}
	if err := lockDeploymentCapacityTx(ctx, tx, deployment.TenantID); err != nil {
		return err
	}
	if err := lockDeploymentReleaseLineTx(ctx, tx, deployment.TenantID, releaseLine); err != nil {
		return err
	}
	lockPreviousQuery := fmt.Sprintf(`SELECT deployment_id::text, COALESCE(rule_version, ''), COALESCE(model_version, ''), COALESCE(feature_set_id, ''), COALESCE(scope, '{}'::jsonb) FROM deployments WHERE tenant_id = $1 AND status = 'active' AND deployment_id != $2 AND %s = $3 FOR UPDATE`, deploymentReleaseLineSQL)
	lockedRows, err := tx.QueryContext(ctx, lockPreviousQuery, deployment.TenantID, deploymentID, releaseLine)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock previous active deployments")
	}
	supersededDeployments := make([]*model.Deployment, 0)
	for lockedRows.Next() {
		previous := &model.Deployment{TenantID: deployment.TenantID, Status: string(model.DeploymentStatusActive)}
		var previousScopeJSON []byte
		if err := lockedRows.Scan(&previous.DeploymentID, &previous.RuleVersion, &previous.ModelVersion, &previous.FeatureSetID, &previousScopeJSON); err != nil {
			lockedRows.Close()
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan previous active deployment")
		}
		if err := json.Unmarshal(previousScopeJSON, &previous.Scope); err != nil {
			lockedRows.Close()
			return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode previous active deployment scope")
		}
		supersededDeployments = append(supersededDeployments, previous)
	}
	if err := lockedRows.Err(); err != nil {
		lockedRows.Close()
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate previous active deployments")
	}
	lockedRows.Close()
	supersedeQuery := fmt.Sprintf(`UPDATE deployments SET status = 'superseded', updated_at = $1 WHERE tenant_id = $2 AND status = 'active' AND deployment_id != $3 AND %s = $4`, deploymentReleaseLineSQL)
	if _, err := tx.ExecContext(ctx, supersedeQuery, time.Now(), deployment.TenantID, deploymentID, releaseLine); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to supersede previous deployments")
	}
	var activeCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployments WHERE tenant_id = $1 AND status IN ('gray', 'active') AND deployment_id != $2`, deployment.TenantID, deploymentID).Scan(&activeCount); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count active deployments before activation")
	}
	if activeCount >= s.config.MaxActiveDeploymentsPerTenant {
		return errors.Newf(errors.ErrCodeQuotaExceeded, "active deployment limit exceeded: max %d", s.config.MaxActiveDeploymentsPerTenant)
	}
	result, err := tx.ExecContext(ctx, `UPDATE deployments SET status = $1, updated_at = $2, activated_at = $3, scope = jsonb_set(COALESCE(scope, '{}'::jsonb), '{percentage}', to_jsonb($4::numeric), true), metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('execution_plan', $5::jsonb) WHERE deployment_id = $6 AND tenant_id = $7 AND status = $8`, string(model.DeploymentStatusActive), time.Now(), activatedAt, workflowNumber(lockedApprovedConfiguration, "gray_percentage"), string(executionConfigurationJSON), deploymentID, deployment.TenantID, deployment.Status)
	if err != nil {
		s.logger.Error("Failed to update deployment status",
			zap.String("deployment_id", deploymentID),
			zap.Error(err))
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployActivate, "deployment", deploymentID, err.Error())
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return errors.Newf(errors.ErrCodeDeploymentNotFound, "deployment not found: %s", deploymentID)
	}
	detail := map[string]interface{}{
		"activated_at":            activatedAt,
		"gray_metrics":            grayMetrics,
		"previous_status":         deployment.Status,
		"new_status":              string(model.DeploymentStatusActive),
		"release_line":            releaseLine,
		"execution_configuration": approvedConfiguration,
	}
	if err := insertDeploymentHistoryTx(ctx, tx, deployment.DeploymentID, deployment.Status, opCtx.UserID, "activated", detail); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist activation history")
	}
	if err := insertDeploymentAuditTx(ctx, tx, opCtx, audit.EventTypeDeployActivate, "deployment", deploymentID, detail); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist activation audit")
	}
	if err := s.insertDeploymentOutboxTx(ctx, tx, deployment, "activated", opCtx.UserID, string(model.DeploymentStatusActive)); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist activation event")
	}
	for _, previous := range supersededDeployments {
		if err := s.insertDeploymentOutboxTx(ctx, tx, previous, "superseded", opCtx.UserID, string(model.DeploymentStatusSuperseded)); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist superseded deployment event")
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit activation transaction")
	}

	if s.auditLogger != nil {
		s.auditLogger.LogDeployment(ctx, audit.EventTypeDeployActivate, opCtx.TenantID, opCtx.UserID, deploymentID, detail)
	}

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

	pausedAt := time.Now().UTC()
	remainingGraySeconds := int64(0)
	if currentStatus == model.DeploymentStatusGray && deployment.GrayExpiredAt != nil && deployment.GrayExpiredAt.After(pausedAt) {
		remainingGraySeconds = int64(deployment.GrayExpiredAt.Sub(pausedAt).Seconds())
	}
	detail := map[string]interface{}{"action": "pause", "previous_status": deployment.Status, "new_status": string(model.DeploymentStatusPaused), "paused_at": pausedAt, "remaining_gray_seconds": remainingGraySeconds}
	if err := s.commitDeploymentTransition(ctx, deployment, opCtx, audit.EventTypeDeployPause, "paused", "", detail, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE deployments SET status = $1, updated_at = $2, metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('paused_from', $3::text, 'paused_at', $4::timestamptz, 'remaining_gray_seconds', $5::bigint) WHERE deployment_id = $6 AND tenant_id = $7 AND status = $3`, string(model.DeploymentStatusPaused), pausedAt, deployment.Status, pausedAt, remainingGraySeconds, deploymentID, deployment.TenantID)
		if err != nil {
			return err
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return errors.Newf(errors.ErrCodeDeploymentNotFound, "deployment not found: %s", deploymentID)
		}
		return nil
	}); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployPause, "deployment", deploymentID, err.Error())
		return err
	}

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

	resumeStatus := resumeDeploymentStatus(deployment.Metadata)
	releaseLine := deploymentReleaseLine(deployment)
	remainingGraySeconds := int64(workflowNumber(deployment.Metadata, "remaining_gray_seconds"))
	var resumedGrayExpiresAt interface{}
	if resumeStatus == model.DeploymentStatusGray && remainingGraySeconds > 0 {
		resumedGrayExpiresAt = time.Now().UTC().Add(time.Duration(remainingGraySeconds) * time.Second)
	}
	detail := map[string]interface{}{"action": "resume", "previous_status": deployment.Status, "new_status": string(resumeStatus), "remaining_gray_seconds": remainingGraySeconds, "gray_expired_at": resumedGrayExpiresAt, "release_line": releaseLine}
	if err := s.commitDeploymentTransition(ctx, deployment, opCtx, audit.EventTypeDeployResume, "resumed", "", detail, func(tx *sql.Tx) error {
		if err := lockDeploymentCapacityTx(ctx, tx, deployment.TenantID); err != nil {
			return err
		}
		if err := lockDeploymentReleaseLineTx(ctx, tx, deployment.TenantID, releaseLine); err != nil {
			return err
		}
		if resumeStatus == model.DeploymentStatusActive {
			if err := s.supersedeActiveReleaseLineTx(ctx, tx, deployment.TenantID, releaseLine, []string{deploymentID}, opCtx.UserID); err != nil {
				return err
			}
		}
		var activeCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM deployments WHERE tenant_id = $1 AND status IN ('gray', 'active') AND deployment_id != $2`, deployment.TenantID, deploymentID).Scan(&activeCount); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count active deployments before resume")
		}
		if activeCount >= s.config.MaxActiveDeploymentsPerTenant {
			return errors.Newf(errors.ErrCodeQuotaExceeded, "active deployment limit exceeded: max %d", s.config.MaxActiveDeploymentsPerTenant)
		}
		if resumeStatus == model.DeploymentStatusGray {
			var hasActiveGray bool
			grayQuery := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM deployments WHERE tenant_id = $1 AND status = 'gray' AND deployment_id != $2 AND %s = $3)`, deploymentReleaseLineSQL)
			if err := tx.QueryRowContext(ctx, grayQuery, deployment.TenantID, deploymentID, releaseLine).Scan(&hasActiveGray); err != nil {
				return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to check release-line gray deployment before resume")
			}
			if hasActiveGray {
				return errors.New(errors.ErrCodeGrayDeploymentActive, "another gray deployment is already active")
			}
		}
		result, err := tx.ExecContext(ctx, `UPDATE deployments SET status = $1, updated_at = $2, gray_expired_at = CASE WHEN $1 = 'gray' AND $3::timestamptz IS NOT NULL THEN $3::timestamptz ELSE gray_expired_at END, metadata = COALESCE(metadata, '{}'::jsonb) - 'paused_from' - 'paused_at' - 'remaining_gray_seconds' WHERE deployment_id = $4 AND tenant_id = $5 AND status = $6`, string(resumeStatus), time.Now(), resumedGrayExpiresAt, deploymentID, deployment.TenantID, deployment.Status)
		if err != nil {
			return err
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return errors.Newf(errors.ErrCodeDeploymentNotFound, "deployment not found: %s", deploymentID)
		}
		return nil
	}); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployResume, "deployment", deploymentID, err.Error())
		return err
	}

	return nil
}

func resumeDeploymentStatus(metadata map[string]interface{}) model.DeploymentStatus {
	if pausedFrom, ok := metadata["paused_from"].(string); ok && pausedFrom == string(model.DeploymentStatusGray) {
		return model.DeploymentStatusGray
	}
	return model.DeploymentStatusActive
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
			if err := json.Unmarshal([]byte(detailJSON.String), &h.Detail); err != nil {
				return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode deployment history detail")
			}
		}

		history = append(history, &h)
	}

	return history, nil
}

// GetDeploymentWorkbench returns the selected deployment plus all data-backed
// supporting panels used by the deployment-management page.
func (s *DeploymentService) GetDeploymentWorkbench(ctx context.Context, deploymentID string, opCtx *OperationContext) (*model.DeploymentWorkbench, error) {
	deployment, err := s.GetDeploymentForOperation(ctx, deploymentID, opCtx)
	if err != nil {
		return nil, err
	}
	history, err := s.GetDeploymentHistory(ctx, deploymentID, opCtx)
	if err != nil {
		return nil, err
	}
	items, err := s.listDeploymentWorkbenchItems(ctx, deployment.TenantID, deploymentID)
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]map[string]interface{})
	for _, item := range items {
		grouped[item.Category] = append(grouped[item.Category], item.Payload)
	}
	rollbackCandidates, err := s.listRollbackCandidates(ctx, deployment)
	if err != nil {
		return nil, err
	}
	if len(rollbackCandidates) > 0 {
		grouped["rollback_versions"] = rollbackCandidates
	}
	return &model.DeploymentWorkbench{
		Deployment: deployment,
		History:    history,
		Items:      grouped,
		Source:     "postgresql",
	}, nil
}

func (s *DeploymentService) listRollbackCandidates(ctx context.Context, deployment *model.Deployment) ([]map[string]interface{}, error) {
	query := fmt.Sprintf(`
			SELECT deployment_id::text,
		       COALESCE(NULLIF(scope->>'version', ''), NULLIF(rule_version, ''), NULLIF(model_version, ''), deployment_id::text),
		       updated_at, COALESCE(scope->>'campus', scope->>'tenant', '当前租户'), COALESCE(created_by::text, 'system'), status,
		       COALESCE(rule_version, ''), COALESCE(model_version, '')
		FROM deployments
			WHERE tenant_id = $1 AND deployment_id != $2 AND status IN ('active', 'rolled_back', 'superseded') AND %s = $3
			ORDER BY updated_at DESC
			LIMIT 3
		`, deploymentReleaseLineSQL)
	rows, err := s.db.QueryContext(ctx, query, deployment.TenantID, deployment.DeploymentID, deploymentReleaseLine(deployment))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query rollback candidates")
	}
	defer rows.Close()
	items := make([]map[string]interface{}, 0, 3)
	for rows.Next() {
		var deploymentID, version, scope, owner, status, ruleVersion, modelVersion string
		var releasedAt time.Time
		if err := rows.Scan(&deploymentID, &version, &releasedAt, &scope, &owner, &status, &ruleVersion, &modelVersion); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan rollback candidate")
		}
		items = append(items, map[string]interface{}{
			"deployment_id": deploymentID,
			"version":       version,
			"released_at":   releasedAt.Format("2006-01-02 15:04"),
			"scope":         scope,
			"owner":         owner,
			"status":        status,
			"rule_version":  ruleVersion,
			"model_version": modelVersion,
		})
	}
	return items, rows.Err()
}

// UpdateDeploymentScope persists the tenant/campus/probe/asset/percentage gray
// strategy shown by the deployment-management page.
func (s *DeploymentService) UpdateDeploymentScope(ctx context.Context, deploymentID string, patch map[string]interface{}, opCtx *OperationContext) (*model.Deployment, error) {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.UpdateDeploymentScope")
	defer span.End()

	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	if err := s.checkPermission(ctx, opCtx, rbac.PermDeployGray, deployment.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployScopeUpdate, "deployment", deploymentID, err.Error())
		return nil, err
	}
	status := model.DeploymentStatus(deployment.Status)
	if status != model.DeploymentStatusPlanned {
		err := errors.Newf(errors.ErrCodeInvalidStateTransition, "cannot update deployment scope while status is %s", status)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployScopeUpdate, "deployment", deploymentID, err.Error())
		return nil, err
	}

	merged, err := mergeDeploymentScope(deployment.Scope, patch)
	if err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployScopeUpdate, "deployment", deploymentID, err.Error())
		return nil, err
	}
	if reflect.DeepEqual(normalizeJSONValue(merged), normalizeJSONValue(deployment.Scope)) {
		return deployment, nil
	}
	if value := strings.TrimSpace(fmt.Sprint(merged["release_line"])); value == "" || value == "<nil>" {
		merged["release_line"] = deploymentReleaseLine(deployment)
	}
	scopeJSON, err := json.Marshal(merged)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal deployment scope")
	}
	updatedAt := time.Now()
	resetWorkflow := map[string]interface{}{
		"stage":                  "draft_saved",
		"operation":              "deploy",
		"updated_at":             updatedAt,
		"updated_by":             opCtx.UserID,
		"configuration":          map[string]interface{}{"scope": merged},
		"precheck_status":        "",
		"precheck_results":       []interface{}{},
		"precheck_snapshot_hash": "",
	}
	resetWorkflowJSON, err := json.Marshal(resetWorkflow)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to reset deployment workflow")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin deployment scope transaction")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE deployments
		SET scope = $1, metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('workflow', $2::jsonb), updated_at = $3
		WHERE deployment_id = $4 AND tenant_id = $5 AND status = $6
	`, scopeJSON, string(resetWorkflowJSON), updatedAt, deploymentID, deployment.TenantID, deployment.Status)
	if err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployScopeUpdate, "deployment", deploymentID, err.Error())
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update deployment scope")
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected != 1 {
		return nil, errors.Newf(errors.ErrCodeInvalidStateTransition, "deployment state changed before scope update: %s", deploymentID)
	}
	detail := map[string]interface{}{
		"previous_scope":       deployment.Scope,
		"new_scope":            merged,
		"approval_invalidated": true,
		"workflow_stage":       "draft_saved",
	}
	if err := insertDeploymentHistoryTx(ctx, tx, deployment.DeploymentID, deployment.Status, opCtx.UserID, "scope_updated", detail); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployScopeUpdate, "deployment", deploymentID, err.Error())
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment scope history")
	}
	if err := insertDeploymentAuditTx(ctx, tx, opCtx, audit.EventTypeDeployScopeUpdate, "deployment", deploymentID, detail); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment scope audit")
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit deployment scope transaction")
	}
	if s.auditLogger != nil {
		s.auditLogger.LogDeployment(ctx, audit.EventTypeDeployScopeUpdate, opCtx.TenantID, opCtx.UserID, deploymentID, detail)
	}
	return s.GetDeployment(ctx, deploymentID)
}

// UpdateDeploymentWorkflow persists precheck and approval stages for the
// source-matched deployment dialogs. It never executes a deploy or rollback;
// state-machine actions remain explicit endpoints after approval.
func (s *DeploymentService) UpdateDeploymentWorkflow(ctx context.Context, deploymentID, stage, operation string, configuration map[string]interface{}, opCtx *OperationContext) (workflowResult map[string]interface{}, workflowErr error) {
	defer func() {
		if workflowErr != nil {
			s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployWorkflowUpdate, "deployment", deploymentID, workflowErr.Error())
		}
	}()
	stage = strings.TrimSpace(stage)
	operation = strings.TrimSpace(operation)
	if stage != "draft" && stage != "precheck" && stage != "submit_approval" && stage != "approve" && stage != "reject" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "workflow stage must be draft, precheck, submit_approval, approve or reject")
	}
	if operation != "deploy" && operation != "rollback" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "workflow operation must be deploy or rollback")
	}
	deployment, err := s.GetDeployment(ctx, deploymentID)
	if err != nil {
		return nil, err
	}
	permission := rbac.PermDeployCreate
	if stage == "approve" || stage == "reject" {
		permission = rbac.PermDeployApprove
	} else if operation == "rollback" {
		permission = rbac.PermDeployRollback
	}
	if err := s.checkPermission(ctx, opCtx, permission, deployment.TenantID); err != nil {
		return nil, err
	}
	if configuration == nil {
		configuration = map[string]interface{}{}
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin workflow transaction")
	}
	defer tx.Rollback()
	var lockedStatus, lockedRuleVersion, lockedModelVersion, lockedFeatureSetID string
	var scopeJSON, metadataJSON []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT status, COALESCE(rule_version, ''), COALESCE(model_version, ''), COALESCE(feature_set_id, ''),
		       COALESCE(scope, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb)
		FROM deployments WHERE deployment_id = $1 AND tenant_id = $2 FOR UPDATE
	`, deploymentID, deployment.TenantID).Scan(&lockedStatus, &lockedRuleVersion, &lockedModelVersion, &lockedFeatureSetID, &scopeJSON, &metadataJSON); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock deployment workflow")
	}
	if operation == "deploy" && lockedStatus != string(model.DeploymentStatusPlanned) {
		return nil, errors.Newf(errors.ErrCodeInvalidStateTransition, "deployment approval requires planned status, current: %s", lockedStatus)
	}
	if operation == "rollback" && !model.CanTransition(model.DeploymentStatus(lockedStatus), model.DeploymentStatusRolledBack) {
		return nil, errors.Newf(errors.ErrCodeInvalidStateTransition, "rollback approval is not valid while status is %s", lockedStatus)
	}
	metadata := map[string]interface{}{}
	lockedScope := map[string]interface{}{}
	if err := json.Unmarshal(scopeJSON, &lockedScope); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode locked deployment scope")
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode locked deployment metadata")
		}
	}
	previous, _ := metadata["workflow"].(map[string]interface{})
	workflowStage, err := nextDeploymentWorkflowStage(previous, stage, operation)
	if err != nil {
		return nil, err
	}
	lockedDeployment := *deployment
	lockedDeployment.Status = lockedStatus
	lockedDeployment.RuleVersion = lockedRuleVersion
	lockedDeployment.ModelVersion = lockedModelVersion
	lockedDeployment.FeatureSetID = lockedFeatureSetID
	lockedDeployment.Scope = lockedScope
	ensureDeploymentReleaseLine(&lockedDeployment)

	if stage == "approve" || stage == "reject" {
		if !opCtx.Authenticated {
			return nil, errors.New(errors.ErrCodeUnauthorized, "deployment approval requires an authenticated token identity")
		}
		requestedBy := workflowString(previous, "requested_by")
		if requestedBy == "" {
			return nil, errors.New(errors.ErrCodeInvalidStateTransition, "approval request identity is missing; submit the workflow again")
		}
		if requestedBy == opCtx.UserID {
			return nil, errors.New(errors.ErrCodePermissionDenied, "approval requester cannot approve or reject their own deployment")
		}
		approvedConfiguration, ok := previous["configuration"].(map[string]interface{})
		if !ok {
			return nil, errors.New(errors.ErrCodeInvalidStateTransition, "approval configuration is missing")
		}
		if len(configuration) > 0 && !reflect.DeepEqual(normalizeJSONValue(configuration), normalizeJSONValue(approvedConfiguration)) {
			return nil, errors.New(errors.ErrCodeVersionConflict, "approval configuration changed after submission")
		}
		configuration = cloneStringMap(approvedConfiguration)
	} else {
		configuration = mergeWorkflowConfiguration(previous, configuration)
	}
	if operation == "deploy" {
		grayPercentage := workflowNumber(configuration, "gray_percentage")
		scopePercentage := workflowNumber(lockedScope, "percentage")
		if grayPercentage <= 0 || grayPercentage > 100 {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "gray percentage must be greater than 0 and no more than 100")
		}
		if scopePercentage > 0 && grayPercentage != scopePercentage {
			return nil, errors.New(errors.ErrCodeVersionConflict, "gray percentage must match the deployment scope; update scope and rerun precheck")
		}
	}
	if operation == "rollback" {
		targetDeploymentID := strings.TrimSpace(fmt.Sprint(configuration["target_deployment_id"]))
		if targetDeploymentID == "" {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "rollback target is required before precheck or approval")
		}
		if err := validateRollbackTargetTx(ctx, tx, &lockedDeployment, targetDeploymentID); err != nil {
			return nil, err
		}
		if (stage == "submit_approval" || stage == "approve") && len([]rune(strings.TrimSpace(fmt.Sprint(configuration["reason"])))) < 10 {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "rollback reason must contain at least 10 characters")
		}
	}

	precheckStatus := workflowString(previous, "precheck_status")
	precheckResults, _ := previous["precheck_results"].([]interface{})
	precheckSnapshotHash := workflowString(previous, "precheck_snapshot_hash")
	precheckCompletedAt := previous["precheck_completed_at"]
	currentSnapshot := buildDeploymentApprovalSnapshot(&lockedDeployment, operation, configuration)
	currentSnapshotHash, err := canonicalValueHash(currentSnapshot)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to hash deployment workflow snapshot")
	}
	if stage == "precheck" {
		results, status, err := s.buildDeploymentPrecheckTx(ctx, tx, &lockedDeployment, operation, configuration)
		if err != nil {
			return nil, err
		}
		precheckStatus = status
		precheckResults = results
		precheckSnapshotHash = currentSnapshotHash
		precheckCompletedAt = now
		if status == "failed" {
			return nil, errors.New(errors.ErrCodeInvalidStateTransition, "deployment precheck failed; resolve blocking dependencies before approval")
		}
	}
	if stage == "submit_approval" {
		if precheckStatus != "passed" && precheckStatus != "passed_with_warnings" {
			return nil, errors.New(errors.ErrCodeInvalidStateTransition, "a successful precheck is required before approval")
		}
		precheckWorkflow := cloneStringMap(previous)
		precheckWorkflow["precheck_results"] = precheckResults
		if err := requireFreshDeploymentPrecheck(precheckWorkflow, now); err != nil {
			return nil, err
		}
		if precheckSnapshotHash == "" || precheckSnapshotHash != currentSnapshotHash {
			return nil, errors.New(errors.ErrCodeVersionConflict, "deployment configuration changed after precheck; run precheck again")
		}
	}
	approvalID := workflowString(previous, "approval_id")
	if stage == "submit_approval" {
		approvalID = "DEP-APPROVAL-" + strings.ToUpper(uuid.NewString()[:8])
	}
	workflow := cloneStringMap(previous)
	if stage == "draft" {
		workflow = map[string]interface{}{}
		approvalID = ""
		precheckStatus = ""
		precheckResults = []interface{}{}
		precheckSnapshotHash = ""
		precheckCompletedAt = nil
	}
	workflow["stage"] = workflowStage
	workflow["operation"] = operation
	workflow["updated_at"] = now
	workflow["updated_by"] = opCtx.UserID
	workflow["configuration"] = configuration
	workflow["precheck_status"] = precheckStatus
	workflow["precheck_results"] = precheckResults
	workflow["precheck_snapshot_hash"] = precheckSnapshotHash
	if precheckCompletedAt != nil {
		workflow["precheck_completed_at"] = precheckCompletedAt
	}
	if approvalID != "" {
		workflow["approval_id"] = approvalID
	}
	if stage == "submit_approval" {
		workflow["approval_snapshot"] = currentSnapshot
		workflow["approval_snapshot_hash"] = currentSnapshotHash
		workflow["requested_at"] = now
		workflow["requested_by"] = opCtx.UserID
		delete(workflow, "approved_at")
		delete(workflow, "approved_by")
		delete(workflow, "rejected_at")
		delete(workflow, "rejected_by")
	}
	if stage == "approve" {
		if err := requireFreshDeploymentPrecheck(previous, now); err != nil {
			return nil, err
		}
		storedSnapshot, ok := previous["approval_snapshot"].(map[string]interface{})
		if !ok {
			return nil, errors.New(errors.ErrCodeInvalidStateTransition, "approval snapshot is missing")
		}
		storedHash, err := canonicalValueHash(storedSnapshot)
		if err != nil || workflowString(previous, "approval_snapshot_hash") != storedHash {
			return nil, errors.New(errors.ErrCodeVersionConflict, "approval snapshot integrity check failed")
		}
		lockedSnapshot := buildDeploymentApprovalSnapshot(&lockedDeployment, operation, configuration)
		lockedHash, err := canonicalValueHash(lockedSnapshot)
		if err != nil || lockedHash != storedHash {
			return nil, errors.New(errors.ErrCodeVersionConflict, "deployment changed after approval submission")
		}
		workflow["approved_at"] = now
		workflow["approved_by"] = opCtx.UserID
	}
	if stage == "reject" {
		workflow["rejected_at"] = now
		workflow["rejected_by"] = opCtx.UserID
	}
	workflowJSON, err := json.Marshal(workflow)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal deployment workflow")
	}
	result, err := tx.ExecContext(ctx, `UPDATE deployments SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('workflow', $1::jsonb), updated_at = $2 WHERE deployment_id = $3 AND tenant_id = $4 AND status = $5`, string(workflowJSON), now, deploymentID, deployment.TenantID, lockedStatus)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update deployment workflow")
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return nil, errors.New(errors.ErrCodeInvalidStateTransition, "deployment changed before workflow update")
	}
	detail := map[string]interface{}{"stage": workflowStage, "operation": operation, "configuration": configuration, "approval_id": approvalID, "approval_snapshot_hash": workflowString(workflow, "approval_snapshot_hash"), "precheck_status": precheckStatus, "previous_stage": workflowString(previous, "stage")}
	if err := insertDeploymentHistoryTx(ctx, tx, deploymentID, lockedStatus, opCtx.UserID, "workflow_"+workflowStage, detail); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment workflow history")
	}
	if err := insertDeploymentAuditTx(ctx, tx, opCtx, audit.EventTypeDeployWorkflowUpdate, "deployment", deploymentID, detail); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment workflow audit")
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit deployment workflow")
	}
	if s.auditLogger != nil {
		s.auditLogger.LogDeployment(ctx, audit.EventTypeDeployWorkflowUpdate, opCtx.TenantID, opCtx.UserID, deploymentID, detail)
	}
	return workflow, nil
}

func nextDeploymentWorkflowStage(previous map[string]interface{}, action, operation string) (string, error) {
	currentStage := workflowString(previous, "stage")
	currentOperation := workflowString(previous, "operation")
	sameOperation := currentOperation == "" || currentOperation == operation
	switch action {
	case "draft":
		if currentStage == "" || (sameOperation && (currentStage == "draft_saved" || currentStage == "rejected")) || (currentStage == "approved" && currentOperation != operation) {
			return "draft_saved", nil
		}
	case "precheck":
		if sameOperation && (currentStage == "draft_saved" || currentStage == "precheck_completed") {
			return "precheck_completed", nil
		}
	case "submit_approval":
		if sameOperation && currentStage == "precheck_completed" {
			return "approval_pending", nil
		}
	case "approve":
		if sameOperation && currentStage == "approval_pending" {
			return "approved", nil
		}
	case "reject":
		if sameOperation && currentStage == "approval_pending" {
			return "rejected", nil
		}
	}
	return "", errors.Newf(errors.ErrCodeInvalidStateTransition, "workflow action %s is not allowed from stage %s for operation %s", action, currentStage, operation)
}

func workflowString(workflow map[string]interface{}, key string) string {
	value, ok := workflow[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func workflowNumber(workflow map[string]interface{}, key string) float64 {
	value, ok := workflow[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		return 0
	}
}

func mergeWorkflowConfiguration(previous, incoming map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	if current, ok := previous["configuration"].(map[string]interface{}); ok {
		for key, value := range current {
			merged[key] = value
		}
	}
	for key, value := range incoming {
		merged[key] = value
	}
	return merged
}

func buildDeploymentApprovalSnapshot(deployment *model.Deployment, operation string, configuration map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"schema_version": 1,
		"deployment_id":  deployment.DeploymentID,
		"tenant_id":      deployment.TenantID,
		"operation":      operation,
		"rule_version":   deployment.RuleVersion,
		"model_version":  deployment.ModelVersion,
		"feature_set_id": deployment.FeatureSetID,
		"release_line":   deploymentReleaseLine(deployment),
		"scope":          deploymentScopeForApproval(deployment),
		"configuration":  cloneStringMap(configuration),
	}
}

func validateRollbackTargetTx(ctx context.Context, tx *sql.Tx, deployment *model.Deployment, targetDeploymentID string) error {
	var targetTenantID, targetStatus, targetRuleVersion, targetModelVersion, targetFeatureSetID string
	var targetScopeJSON []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT tenant_id, status, COALESCE(rule_version, ''), COALESCE(model_version, ''),
		       COALESCE(feature_set_id, ''), COALESCE(scope, '{}'::jsonb)
		FROM deployments WHERE deployment_id = $1 FOR SHARE
	`, targetDeploymentID).Scan(&targetTenantID, &targetStatus, &targetRuleVersion, &targetModelVersion, &targetFeatureSetID, &targetScopeJSON); err != nil {
		if err == sql.ErrNoRows {
			return errors.Newf(errors.ErrCodeDeploymentNotFound, "rollback target not found: %s", targetDeploymentID)
		}
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock rollback target")
	}
	if targetTenantID != deployment.TenantID || targetDeploymentID == deployment.DeploymentID {
		return errors.New(errors.ErrCodeInvalidParameter, "rollback target must be another deployment in the same tenant")
	}
	if targetStatus != string(model.DeploymentStatusActive) && targetStatus != string(model.DeploymentStatusRolledBack) && targetStatus != string(model.DeploymentStatusSuperseded) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "rollback target status %s is not recoverable", targetStatus)
	}
	targetScope := map[string]interface{}{}
	if err := json.Unmarshal(targetScopeJSON, &targetScope); err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode rollback target scope")
	}
	target := &model.Deployment{DeploymentID: targetDeploymentID, TenantID: targetTenantID, Status: targetStatus, RuleVersion: targetRuleVersion, ModelVersion: targetModelVersion, FeatureSetID: targetFeatureSetID, Scope: targetScope}
	if deploymentReleaseLine(target) != deploymentReleaseLine(deployment) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "rollback target release line %s does not match %s", deploymentReleaseLine(target), deploymentReleaseLine(deployment))
	}
	return nil
}

func (s *DeploymentService) buildDeploymentPrecheckTx(ctx context.Context, tx *sql.Tx, deployment *model.Deployment, operation string, configuration map[string]interface{}) ([]interface{}, string, error) {
	results := make([]interface{}, 0, 7)
	warnings := 0
	failures := 0
	checkedAt := time.Now().UTC()
	const evidenceFreshness = 30 * time.Minute
	appendResult := func(label, status, evidence, recommendation string, observedAt time.Time) {
		if observedAt.IsZero() {
			observedAt = checkedAt
		}
		freshUntil := observedAt.Add(evidenceFreshness)
		if !freshUntil.After(checkedAt) {
			// The source may be stale or absent, but the warning decision itself was
			// evaluated now and remains actionable for one approval window.
			freshUntil = checkedAt.Add(evidenceFreshness)
		}
		results = append(results, map[string]interface{}{
			"label": label, "status": status, "evidence": evidence, "recommendation": recommendation,
			"checked_at": checkedAt, "source_observed_at": observedAt, "fresh_until": freshUntil,
		})
		if status == "warning" {
			warnings++
		} else if status == "failed" {
			failures++
		}
	}
	exists := func(query string, args ...interface{}) (bool, error) {
		var ok bool
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&ok); err != nil {
			return false, err
		}
		return ok, nil
	}
	if deployment.RuleVersion != "" {
		ok, err := exists(`SELECT EXISTS(SELECT 1 FROM rule_versions WHERE rule_version = $1 AND tenant_id = $2)`, deployment.RuleVersion, deployment.TenantID)
		if err != nil {
			return nil, "failed", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to precheck rule dependency")
		}
		if ok {
			appendResult("规则依赖", "passed", "规则版本存在且租户一致", "继续", checkedAt)
		} else {
			appendResult("规则依赖", "failed", "规则版本不存在或租户不一致", "修复规则依赖", checkedAt)
		}
	} else {
		appendResult("规则依赖", "passed", "当前发布线未绑定规则包", "无需检查", checkedAt)
	}
	if deployment.ModelVersion != "" {
		ok, err := exists(`SELECT EXISTS(SELECT 1 FROM model_versions WHERE model_version = $1 AND tenant_id = $2)`, deployment.ModelVersion, deployment.TenantID)
		if err != nil {
			return nil, "failed", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to precheck model compatibility")
		}
		if ok {
			appendResult("模型兼容", "passed", "模型版本存在且租户一致", "继续", checkedAt)
		} else {
			appendResult("模型兼容", "failed", "模型版本不存在或租户不一致", "修复模型依赖", checkedAt)
		}
	} else {
		appendResult("模型兼容", "passed", "当前发布线未绑定模型包", "无需检查", checkedAt)
	}
	if deployment.FeatureSetID != "" {
		ok, err := exists(`SELECT EXISTS(SELECT 1 FROM feature_sets WHERE feature_set_id = $1 AND tenant_id = $2)`, deployment.FeatureSetID, deployment.TenantID)
		if err != nil {
			return nil, "failed", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to precheck feature set")
		}
		if ok {
			appendResult("特征集", "passed", "特征集存在且租户一致", "继续", checkedAt)
		} else {
			appendResult("特征集", "failed", "特征集不存在或租户不一致", "修复特征依赖", checkedAt)
		}
	} else {
		appendResult("特征集", "passed", "当前发布线未绑定特征集", "无需检查", checkedAt)
	}
	var checkpointObserved sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT MAX(occurred_at) FROM deployment_workbench_items WHERE tenant_id = $1 AND deployment_id IN ($2, '*') AND category = 'health' AND payload->>'label' ILIKE '%Checkpoint%' AND COALESCE(payload->>'tone', 'ok') != 'risk'`, deployment.TenantID, deployment.DeploymentID).Scan(&checkpointObserved); err != nil {
		return nil, "failed", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to precheck Flink checkpoint")
	}
	if checkpointObserved.Valid && checkedAt.Sub(checkpointObserved.Time) <= evidenceFreshness {
		appendResult("Flink checkpoint", "passed", "数据库健康证据显示 checkpoint 可用且在 30 分钟新鲜度窗口内", "保留状态快照", checkpointObserved.Time)
	} else {
		evidence := "未找到当前部署的 checkpoint 健康证据"
		observed := checkedAt.Add(-evidenceFreshness)
		if checkpointObserved.Valid {
			evidence = "checkpoint 健康证据已超过 30 分钟新鲜度窗口"
			observed = checkpointObserved.Time
		}
		appendResult("Flink checkpoint", "warning", evidence, "发布前刷新并确认最新 checkpoint", observed)
	}
	var topicObserved sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT MAX(occurred_at) FROM deployment_workbench_items WHERE tenant_id = $1 AND deployment_id IN ($2, '*') AND category = 'evidence' AND lower(payload->>'label') = 'topic' AND payload->>'status' IN ('已通过', 'passed')`, deployment.TenantID, deployment.DeploymentID).Scan(&topicObserved); err != nil {
		return nil, "failed", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to precheck Kafka topic")
	}
	if topicObserved.Valid && checkedAt.Sub(topicObserved.Time) <= evidenceFreshness {
		appendResult("Kafka Topic", "passed", "Topic 证据已通过且在 30 分钟新鲜度窗口内", "继续", topicObserved.Time)
	} else {
		evidence := "未找到已通过的 Topic 证据"
		observed := checkedAt.Add(-evidenceFreshness)
		if topicObserved.Valid {
			evidence = "Topic 证据已超过 30 分钟新鲜度窗口"
			observed = topicObserved.Time
		}
		appendResult("Kafka Topic", "warning", evidence, "刷新并确认 Topic 与 offset", observed)
	}
	if operation == "rollback" {
		appendResult("回滚目标", "passed", fmt.Sprintf("目标部署 %s 已锁定且属于发布线 %s", strings.TrimSpace(fmt.Sprint(configuration["target_deployment_id"])), deploymentReleaseLine(deployment)), "保留目标快照", checkedAt)
	} else {
		rollbackQuery := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM deployments WHERE tenant_id = $1 AND deployment_id != $2 AND status IN ('active', 'rolled_back', 'superseded') AND %s = $3)`, deploymentReleaseLineSQL)
		rollbackReady, err := exists(rollbackQuery, deployment.TenantID, deployment.DeploymentID, deploymentReleaseLine(deployment))
		if err != nil {
			return nil, "failed", errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to precheck rollback package")
		}
		if rollbackReady {
			appendResult("回滚包", "passed", fmt.Sprintf("发布线 %s 存在可恢复版本", deploymentReleaseLine(deployment)), "继续", checkedAt)
		} else {
			appendResult("回滚包", "warning", fmt.Sprintf("发布线 %s 未找到可恢复版本", deploymentReleaseLine(deployment)), "生成同发布线回滚包", checkedAt)
		}
	}
	appendResult("审批权限", "passed", "申请人权限与租户边界已校验；批准必须由另一名具有 deploy:approve 权限的用户完成", "提交独立审批", checkedAt)
	if failures > 0 {
		return results, "failed", nil
	}
	if warnings > 0 {
		return results, "passed_with_warnings", nil
	}
	return results, "passed", nil
}

// ExportDeploymentEvidence builds the download on the server and writes a
// queryable audit event before returning it to the browser.
func (s *DeploymentService) ExportDeploymentEvidence(ctx context.Context, deploymentID string, opCtx *OperationContext) (*model.DeploymentEvidenceBundle, error) {
	ctx, span := otel.StartSpan(ctx, "DeploymentService.ExportDeploymentEvidence")
	defer span.End()

	workbench, err := s.GetDeploymentWorkbench(ctx, deploymentID, opCtx)
	if err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeDeployEvidenceExport, "deployment", deploymentID, err.Error())
		return nil, err
	}
	bundle := &model.DeploymentEvidenceBundle{
		ExportID:    "DEP-EVIDENCE-" + strings.ToUpper(uuid.NewString()[:8]),
		GeneratedAt: time.Now().UTC(),
		GeneratedBy: opCtx.UserID,
		Deployment:  workbench.Deployment,
		History:     workbench.History,
		Evidence:    workbench.Items["evidence"],
		Source:      workbench.Source,
	}
	checksumPayload, err := json.Marshal(bundle)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal deployment evidence")
	}
	bundle.BundleChecksum = fmt.Sprintf("sha256:%x", sha256.Sum256(checksumPayload))
	bundle.DownloadContent = string(checksumPayload)
	auditDetail := map[string]interface{}{
		"export_id":       bundle.ExportID,
		"bundle_checksum": bundle.BundleChecksum,
		"evidence_count":  len(bundle.Evidence),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin evidence audit transaction")
	}
	defer tx.Rollback()
	if err := insertDeploymentAuditTx(ctx, tx, opCtx, audit.EventTypeDeployEvidenceExport, "deployment", deploymentID, auditDetail); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist evidence export audit")
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit evidence export audit")
	}
	if s.auditLogger != nil {
		s.auditLogger.LogDeployment(ctx, audit.EventTypeDeployEvidenceExport, opCtx.TenantID, opCtx.UserID, deploymentID, auditDetail)
	}
	return bundle, nil
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

type DeploymentHistoryEntry = model.DeploymentHistoryRecord

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

func mergeDeploymentScope(current, patch map[string]interface{}) (map[string]interface{}, error) {
	merged := make(map[string]interface{}, len(current)+len(patch))
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range patch {
		merged[key] = value
	}
	percentage, ok := merged["percentage"]
	if !ok {
		return nil, errors.New(errors.ErrCodeMissingParameter, "scope percentage is required")
	}
	var numeric float64
	switch value := percentage.(type) {
	case float64:
		numeric = value
	case float32:
		numeric = float64(value)
	case int:
		numeric = float64(value)
	case int64:
		numeric = float64(value)
	default:
		return nil, errors.New(errors.ErrCodeInvalidParameter, "scope percentage must be numeric")
	}
	if numeric < 0 || numeric > 100 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "scope percentage must be between 0 and 100")
	}
	merged["percentage"] = numeric
	for _, key := range []string{"tenant", "campus", "probe_group", "asset_group"} {
		if value, exists := merged[key]; exists && strings.TrimSpace(fmt.Sprint(value)) == "" {
			return nil, errors.Newf(errors.ErrCodeInvalidParameter, "scope %s cannot be empty", key)
		}
	}
	return merged, nil
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

func (s *DeploymentService) listDeploymentWorkbenchItems(ctx context.Context, tenantID, deploymentID string) ([]*model.DeploymentWorkbenchItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT item_id, tenant_id, deployment_id, category, ordinal, payload, scenario_id, occurred_at
		FROM deployment_workbench_items
		WHERE tenant_id = $1 AND deployment_id IN ($2, '*')
		ORDER BY category, CASE WHEN deployment_id = $2 THEN 0 ELSE 1 END, ordinal, item_id
	`, tenantID, deploymentID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query deployment workbench items")
	}
	defer rows.Close()

	items := make([]*model.DeploymentWorkbenchItem, 0)
	for rows.Next() {
		var item model.DeploymentWorkbenchItem
		var payload []byte
		if err := rows.Scan(&item.ItemID, &item.TenantID, &item.DeploymentID, &item.Category, &item.Ordinal, &payload, &item.ScenarioID, &item.OccurredAt); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan deployment workbench item")
		}
		if err := json.Unmarshal(payload, &item.Payload); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to unmarshal deployment workbench payload")
		}
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate deployment workbench items")
	}
	return items, nil
}

type deploymentOutboxEnvelope struct {
	EventID       string            `json:"event_id"`
	SchemaVersion int               `json:"schema_version"`
	EventType     string            `json:"event_type"`
	Action        string            `json:"action"`
	OccurredAt    time.Time         `json:"occurred_at"`
	OperatorID    string            `json:"operator_id"`
	Deployment    *model.Deployment `json:"deployment"`
}

type deploymentOutboxRecord struct {
	ID           int64
	DeploymentID string
	EventType    string
	Payload      []byte
	AttemptCount int
}

func (s *DeploymentService) insertDeploymentOutboxTx(ctx context.Context, tx *sql.Tx, deployment *model.Deployment, action, operatorID, postStatus string) error {
	if deployment == nil {
		return fmt.Errorf("deployment is required for outbox event")
	}
	postState := *deployment
	postState.Status = postStatus
	postState.Scope = cloneStringMap(deployment.Scope)
	postState.Metadata = cloneStringMap(deployment.Metadata)
	eventID := uuid.NewString()
	occurredAt := time.Now().UTC()
	envelope := deploymentOutboxEnvelope{
		EventID: eventID, SchemaVersion: 1, EventType: "deployment_event", Action: action,
		OccurredAt: occurredAt, OperatorID: operatorID, Deployment: &postState,
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("failed to marshal deployment outbox envelope: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO deployment_outbox (
			event_id, deployment_id, tenant_id, event_type, schema_version, topic,
			partition_key, payload, occurred_at, status, available_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, 1, 'deployment.events.v1', $2, $5::jsonb, $6, 'pending', $6, $6, $6)
	`, eventID, deployment.DeploymentID, deployment.TenantID, action, string(payload), occurredAt)
	return err
}

func (s *DeploymentService) startDeploymentOutboxProcessorIfEnabled() {
	if s == nil || s.db == nil || s.publisher == nil {
		return
	}
	if s.outboxStopCh == nil {
		s.outboxStopCh = make(chan struct{})
	}
	if s.outboxInstance == "" {
		s.outboxInstance = uuid.NewString()
	}
	s.outboxWG.Add(1)
	go s.runDeploymentOutboxProcessor()
}

func (s *DeploymentService) runDeploymentOutboxProcessor() {
	defer s.outboxWG.Done()
	ticker := time.NewTicker(deploymentOutboxProcessInterval)
	defer ticker.Stop()
	for {
		if err := s.processDeploymentOutbox(context.Background()); err != nil && s.logger != nil {
			s.logger.Error("Failed to process deployment outbox", zap.Error(err))
		}
		select {
		case <-ticker.C:
		case <-s.outboxStopCh:
			return
		}
	}
}

func (s *DeploymentService) processDeploymentOutbox(ctx context.Context) error {
	records, err := s.claimDeploymentOutbox(ctx, 100)
	if err != nil {
		return err
	}
	for _, record := range records {
		var envelope deploymentOutboxEnvelope
		if err := json.Unmarshal(record.Payload, &envelope); err != nil {
			s.failDeploymentOutbox(ctx, record, err)
			continue
		}
		publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := s.publisher.PublishDeploymentEventWithID(publishCtx, envelope.Deployment, envelope.Action, envelope.OperatorID, envelope.EventID, envelope.OccurredAt)
		cancel()
		if err != nil {
			s.failDeploymentOutbox(ctx, record, err)
			continue
		}
		if _, err := s.db.ExecContext(ctx, `
			UPDATE deployment_outbox SET status = 'published', published_at = now(), locked_at = NULL,
			       locked_by = NULL, last_error = '', updated_at = now()
			WHERE id = $1 AND status = 'processing' AND locked_by = $2
		`, record.ID, s.outboxInstance); err != nil {
			return fmt.Errorf("failed to acknowledge deployment outbox event %d: %w", record.ID, err)
		}
	}
	return nil
}

func (s *DeploymentService) claimDeploymentOutbox(ctx context.Context, limit int) ([]deploymentOutboxRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin deployment outbox claim: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE deployment_outbox SET status = 'pending', locked_at = NULL, locked_by = NULL,
		       available_at = now(), updated_at = now(), last_error = CASE WHEN last_error = '' THEN 'lease expired' ELSE last_error END
		WHERE status = 'processing' AND locked_at < now() - interval '30 seconds'
	`); err != nil {
		return nil, fmt.Errorf("failed to recover deployment outbox leases: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		WITH candidates AS (
			SELECT current.id
			FROM deployment_outbox current
			WHERE current.status = 'pending' AND current.available_at <= now()
			  AND NOT EXISTS (
				SELECT 1 FROM deployment_outbox prior
				WHERE prior.deployment_id = current.deployment_id AND prior.id < current.id AND prior.status NOT IN ('published', 'dead')
			  )
			ORDER BY current.id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE deployment_outbox target
		SET status = 'processing', locked_at = now(), locked_by = $2,
		    attempt_count = target.attempt_count + 1, updated_at = now()
		FROM candidates WHERE target.id = candidates.id
		RETURNING target.id, target.deployment_id, target.event_type, target.payload, target.attempt_count
	`, limit, s.outboxInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to claim deployment outbox events: %w", err)
	}
	records := make([]deploymentOutboxRecord, 0, limit)
	for rows.Next() {
		var record deploymentOutboxRecord
		if err := rows.Scan(&record.ID, &record.DeploymentID, &record.EventType, &record.Payload, &record.AttemptCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan deployment outbox event: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("failed to iterate deployment outbox events: %w", err)
	}
	rows.Close()
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit deployment outbox claim: %w", err)
	}
	return records, nil
}

func (s *DeploymentService) failDeploymentOutbox(ctx context.Context, record deploymentOutboxRecord, publishErr error) {
	status := "pending"
	availableAt := time.Now().UTC().Add(deploymentOutboxBackoff(record.AttemptCount))
	if record.AttemptCount >= deploymentOutboxMaxRetries {
		status = "dead"
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE deployment_outbox SET status = $1, available_at = $2, locked_at = NULL, locked_by = NULL,
		       last_error = $3, updated_at = now()
		WHERE id = $4 AND status = 'processing' AND locked_by = $5
	`, status, availableAt, publishErr.Error(), record.ID, s.outboxInstance); err != nil && s.logger != nil {
		s.logger.Error("Failed to record deployment outbox publication failure", zap.Int64("outbox_id", record.ID), zap.Error(err))
	}
}

func deploymentOutboxBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := deploymentOutboxRetryDelay * time.Duration(1<<uint(min(attempt-1, 6)))
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
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

func insertDeploymentHistoryTx(ctx context.Context, tx *sql.Tx, deploymentID, status, operatorID, action string, detail map[string]interface{}) error {
	detailCopy := make(map[string]interface{}, len(detail)+1)
	for key, value := range detail {
		detailCopy[key] = value
	}
	detailCopy["status"] = status
	detailJSON, err := json.Marshal(detailCopy)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO deployment_history (deployment_id, action, operator_id, created_at, detail) VALUES ($1, $2, $3, $4, $5)`, deploymentID, action, operatorID, time.Now(), detailJSON)
	return err
}

func insertDeploymentAuditTx(ctx context.Context, tx *sql.Tx, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID string, detail map[string]interface{}) error {
	detailCopy := make(map[string]interface{}, len(detail)+1)
	for key, value := range detail {
		detailCopy[key] = value
	}
	detailCopy["result"] = string(audit.ResultSuccess)
	detailJSON, err := json.Marshal(detailCopy)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
	`, opCtx.TenantID, opCtx.UserID, string(eventType), resourceType, resourceID, string(detailJSON), opCtx.IPAddr, opCtx.UserAgent)
	return err
}

func (s *DeploymentService) commitDeploymentTransition(ctx context.Context, deployment *model.Deployment, opCtx *OperationContext, eventType audit.EventType, historyAction, approvalOperation string, detail map[string]interface{}, update func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin deployment transition")
	}
	defer tx.Rollback()
	lockedDeployment, err := lockDeploymentSnapshotTx(ctx, tx, deployment.DeploymentID, deployment.TenantID)
	if err != nil {
		return err
	}
	if lockedDeployment.Status != deployment.Status {
		return errors.Newf(errors.ErrCodeInvalidStateTransition, "deployment state changed concurrently from %s to %s", deployment.Status, lockedDeployment.Status)
	}
	if approvalOperation != "" {
		if err := requireApprovedDeploymentWorkflow(lockedDeployment, approvalOperation); err != nil {
			return err
		}
	}
	if err := update(tx); err != nil {
		return err
	}
	if err := insertDeploymentHistoryTx(ctx, tx, deployment.DeploymentID, deployment.Status, opCtx.UserID, historyAction, detail); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment transition history")
	}
	if err := insertDeploymentAuditTx(ctx, tx, opCtx, eventType, "deployment", deployment.DeploymentID, detail); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment transition audit")
	}
	newStatus := workflowString(detail, "new_status")
	if newStatus == "" {
		newStatus = deployment.Status
	}
	postDeployment, err := lockDeploymentSnapshotTx(ctx, tx, deployment.DeploymentID, deployment.TenantID)
	if err != nil {
		return err
	}
	if err := s.insertDeploymentOutboxTx(ctx, tx, postDeployment, historyAction, opCtx.UserID, newStatus); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist deployment transition event")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit deployment transition")
	}
	if s.auditLogger != nil {
		s.auditLogger.LogDeployment(ctx, eventType, opCtx.TenantID, opCtx.UserID, deployment.DeploymentID, detail)
	}
	return nil
}

func lockDeploymentSnapshotTx(ctx context.Context, tx *sql.Tx, deploymentID, tenantID string) (*model.Deployment, error) {
	var deployment model.Deployment
	var scopeJSON, metadataJSON []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT deployment_id::text, tenant_id, status, COALESCE(rule_version, ''), COALESCE(model_version, ''),
		       COALESCE(feature_set_id, ''), COALESCE(scope, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb)
		FROM deployments WHERE deployment_id = $1 AND tenant_id = $2 FOR UPDATE
	`, deploymentID, tenantID).Scan(&deployment.DeploymentID, &deployment.TenantID, &deployment.Status, &deployment.RuleVersion, &deployment.ModelVersion, &deployment.FeatureSetID, &scopeJSON, &metadataJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeDeploymentNotFound, "deployment not found: %s", deploymentID)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock deployment transition")
	}
	if err := json.Unmarshal(scopeJSON, &deployment.Scope); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode locked deployment scope")
	}
	if err := json.Unmarshal(metadataJSON, &deployment.Metadata); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode locked deployment metadata")
	}
	return &deployment, nil
}

func lockDeploymentCapacityTx(ctx context.Context, tx *sql.Tx, tenantID string) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, tenantID+"|deployment-capacity"); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock tenant deployment capacity")
	}
	return nil
}

func lockDeploymentReleaseLineTx(ctx context.Context, tx *sql.Tx, tenantID, releaseLine string) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, tenantID+"|release-line|"+releaseLine); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock deployment release line")
	}
	return nil
}

func (s *DeploymentService) supersedeActiveReleaseLineTx(ctx context.Context, tx *sql.Tx, tenantID, releaseLine string, excludedIDs []string, operatorID string) error {
	excludedA, excludedB := "", ""
	if len(excludedIDs) > 0 {
		excludedA = excludedIDs[0]
	}
	if len(excludedIDs) > 1 {
		excludedB = excludedIDs[1]
	}
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`
		SELECT deployment_id::text, COALESCE(rule_version, ''), COALESCE(model_version, ''),
		       COALESCE(feature_set_id, ''), COALESCE(scope, '{}'::jsonb), COALESCE(metadata, '{}'::jsonb)
		FROM deployments
		WHERE tenant_id = $1 AND status = 'active' AND deployment_id::text <> $2 AND deployment_id::text <> $3 AND %s = $4
		FOR UPDATE
	`, deploymentReleaseLineSQL), tenantID, excludedA, excludedB, releaseLine)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to lock active release-line deployments")
	}
	previous := make([]*model.Deployment, 0)
	for rows.Next() {
		item := &model.Deployment{TenantID: tenantID, Status: string(model.DeploymentStatusActive)}
		var scopeJSON, metadataJSON []byte
		if err := rows.Scan(&item.DeploymentID, &item.RuleVersion, &item.ModelVersion, &item.FeatureSetID, &scopeJSON, &metadataJSON); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan active release-line deployment")
		}
		if err := json.Unmarshal(scopeJSON, &item.Scope); err != nil {
			return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode active release-line scope")
		}
		if err := json.Unmarshal(metadataJSON, &item.Metadata); err != nil {
			return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to decode active release-line metadata")
		}
		previous = append(previous, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate active release-line deployments")
	}
	rows.Close()
	if len(previous) == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE deployments SET status = 'superseded', updated_at = now() WHERE tenant_id = $1 AND status = 'active' AND deployment_id::text <> $2 AND deployment_id::text <> $3 AND %s = $4`, deploymentReleaseLineSQL), tenantID, excludedA, excludedB, releaseLine); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to supersede active release-line deployments")
	}
	for _, item := range previous {
		if err := s.insertDeploymentOutboxTx(ctx, tx, item, "superseded", operatorID, string(model.DeploymentStatusSuperseded)); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist superseded release-line event")
		}
	}
	return nil
}
