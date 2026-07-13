////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/service/rule_service.go
// 规则服务 - 完整修复版
// 修复内容：
// 1. ✅ 集成 Outbox 模式补偿机制
// 2. ✅ 修复 CreateRule 事务顺序（DB + Outbox 在同一事务）
// 3. ✅ 修复 UpdateRule 乐观锁冲突处理（拒绝冲突）
// 4. ✅ 删除 Service 层的重复唯一性检查（依赖数据库约束）
// 5. ✅ 增强错误处理和审计日志
// 6. ✅ 添加 Outbox 后台处理器
////////////////////////////////////////////////////////////////////////////////

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/publisher"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/rbac"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/repository"
)

const (
	maxRetries        = 3
	kafkaPublishRetry = 3

	// 缓存键前缀（修复：添加租户 ID 防止跨租户数据泄露）
	ruleCachePrefix     = "rule:tenant:%s:id:%s"   // rule:tenant:{tenant_id}:id:{rule_id}
	ruleListCachePrefix = "rule:tenant:%s:list:%s" // rule:tenant:{tenant_id}:list:{filter_hash}
	ruleCacheTTL        = 5 * time.Minute
	ruleListCacheTTL    = 1 * time.Minute

	// 限制
	maxRulesPerTenant    = 10000
	maxLabelsPerRule     = 50
	maxConditionDepth    = 10
	maxRuleNameLength    = 256
	maxDescriptionLength = 4096

	// Outbox 配置
	outboxProcessInterval = 5 * time.Second
	outboxRetryDelay      = 1 * time.Minute
	outboxMaxRetries      = 10
)

// RuleServiceConfig 规则服务配置
type RuleServiceConfig struct {
	MaxRulesPerTenant     int           `env:"MAX_RULES_PER_TENANT" envDefault:"10000"`
	EnableCache           bool          `env:"RULE_CACHE_ENABLED" envDefault:"true"`
	CacheTTL              time.Duration `env:"RULE_CACHE_TTL" envDefault:"5m"`
	EnableAudit           bool          `env:"RULE_AUDIT_ENABLED" envDefault:"true"`
	KafkaPublishRetries   int           `env:"KAFKA_PUBLISH_RETRIES" envDefault:"3"`
	KafkaPublishTimeout   time.Duration `env:"KAFKA_PUBLISH_TIMEOUT" envDefault:"10s"`
	EnableOutbox          bool          `env:"RULE_OUTBOX_ENABLED" envDefault:"true"`
	OutboxProcessInterval time.Duration `env:"RULE_OUTBOX_PROCESS_INTERVAL" envDefault:"5s"`
}

// DefaultRuleServiceConfig 默认配置
func DefaultRuleServiceConfig() RuleServiceConfig {
	return RuleServiceConfig{
		MaxRulesPerTenant:     10000,
		EnableCache:           true,
		CacheTTL:              5 * time.Minute,
		EnableAudit:           true,
		KafkaPublishRetries:   3,
		KafkaPublishTimeout:   10 * time.Second,
		EnableOutbox:          true,
		OutboxProcessInterval: 5 * time.Second,
	}
}

// RuleService 规则服务
type RuleService struct {
	repo        *repository.RuleRepository
	publisher   *publisher.KafkaPublisher
	redis       *redis.Client
	auditLogger *audit.Logger
	rbacChecker *rbac.Checker
	config      RuleServiceConfig
	logger      *zap.Logger

	// Outbox 处理器
	outboxStopCh chan struct{}
	db           *sql.DB
}

// NewRuleService 创建规则服务（简化版本）
func NewRuleService(
	repo *repository.RuleRepository,
	publisher *publisher.KafkaPublisher,
	logger *zap.Logger,
) *RuleService {
	return &RuleService{
		repo:      repo,
		publisher: publisher,
		config:    DefaultRuleServiceConfig(),
		logger:    logger,
	}
}

// NewRuleServiceWithDeps 创建带完整依赖的规则服务
func NewRuleServiceWithDeps(
	repo *repository.RuleRepository,
	publisher *publisher.KafkaPublisher,
	auditLogger *audit.Logger,
	rbacChecker *rbac.Checker,
	redisClient *redis.Client,
	db *sql.DB,
	logger *zap.Logger,
	config RuleServiceConfig,
) *RuleService {
	svc := &RuleService{
		repo:         repo,
		publisher:    publisher,
		redis:        redisClient,
		auditLogger:  auditLogger,
		rbacChecker:  rbacChecker,
		config:       config,
		logger:       logger,
		db:           db,
		outboxStopCh: make(chan struct{}),
	}

	// 启动 Outbox 处理器
	if config.EnableOutbox && db != nil {
		go svc.startOutboxProcessor()
		logger.Info("Outbox processor started", zap.Duration("interval", config.OutboxProcessInterval))
	}

	return svc
}

// Stop 停止服务（用于优雅关闭）
func (s *RuleService) Stop() {
	if s.outboxStopCh != nil {
		close(s.outboxStopCh)
		s.logger.Info("Outbox processor stopped")
	}
}

// OperationContext 操作上下文（包含用户信息用于权限检查和审计）
type OperationContext struct {
	TenantID    string
	UserID      string
	Username    string
	Roles       []string
	Permissions []string
	IPAddr      string
	UserAgent   string
}

// =============================================================================
// 规则 CRUD 操作
// =============================================================================

// CreateRule 创建规则（修复版：DB + Outbox 在同一事务）
func (s *RuleService) CreateRule(ctx context.Context, rule *model.Rule, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "RuleService.CreateRule")
	defer span.End()

	// 1. 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleWrite, rule.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleCreate, "rule", "", err.Error())
		return err
	}

	// 2. 验证规则
	if err := s.validateRule(rule); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleCreate, "rule", "", err.Error())
		return err
	}

	// 3. 检查租户规则数量限制
	count, err := s.repo.CountByTenant(ctx, rule.TenantID)
	if err != nil {
		s.logger.Warn("Failed to count tenant rules", zap.Error(err))
	} else if count >= int64(s.config.MaxRulesPerTenant) {
		err := errors.Newf(errors.ErrCodeQuotaExceeded, "rule limit exceeded: max %d rules per tenant", s.config.MaxRulesPerTenant)
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleCreate, "rule", "", err.Error())
		return err
	}

	// 4. ✅ 删除重复的唯一性检查，依赖数据库约束

	// 5. 生成规则 ID 和版本
	rule.RuleID = uuid.New().String()
	rule.Version = 1
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	rule.CreatedBy = opCtx.UserID

	// 设置初始状态
	if rule.Enabled {
		rule.Status = string(model.RuleStatusActive)
	} else {
		rule.Status = string(model.RuleStatusDraft)
	}

	s.logger.Info("Creating rule",
		zap.String("rule_id", rule.RuleID),
		zap.String("tenant_id", rule.TenantID),
		zap.String("name", rule.Name),
		zap.String("created_by", opCtx.UserID))

	// 6. ✅ 修复：使用事务同时写入 rules 和 rule_outbox
	err = s.repo.Transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// 6.1 插入规则
		if err := s.createRuleInTx(ctx, tx, rule); err != nil {
			return err
		}

		// 6.2 创建版本记录
		if err := s.createRuleVersionInTx(ctx, tx, rule); err != nil {
			s.logger.Warn("Failed to create rule version record", zap.Error(err))
			// 不阻塞主流程
		}

		// 6.3 如果规则启用，写入 Outbox
		if rule.Enabled && s.config.EnableOutbox {
			cmd := &model.RuleCommand{
				Action:     string(model.ActionCreate),
				Rule:       rule,
				Timestamp:  time.Now(),
				OperatorID: opCtx.UserID,
			}

			if err := s.insertOutboxInTx(ctx, tx, rule.RuleID, "create", cmd); err != nil {
				return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to insert outbox event")
			}
		}

		return nil
	})

	if err != nil {
		// ✅ 检查是否是唯一约束冲突
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "23505") {
			err = errors.Newf(errors.ErrCodeDuplicateValue, "rule with name '%s' already exists", rule.Name)
		}

		s.logger.Error("Failed to create rule",
			zap.String("rule_id", rule.RuleID),
			zap.Error(err))
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleCreate, "rule", rule.RuleID, err.Error())
		return err
	}

	// 7. 清除缓存
	s.invalidateRuleCache(ctx, rule.RuleID, rule.TenantID)

	// 8. 记录审计日志
	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeRuleCreate, "rule", rule.RuleID, map[string]interface{}{
		"name":     rule.Name,
		"type":     rule.Type,
		"engine":   rule.Engine,
		"enabled":  rule.Enabled,
		"severity": rule.Severity,
		"version":  rule.Version,
		"labels":   rule.Labels,
	})

	s.logger.Info("Rule created successfully",
		zap.String("rule_id", rule.RuleID),
		zap.String("tenant_id", rule.TenantID),
		zap.String("name", rule.Name),
		zap.Bool("enabled", rule.Enabled),
		zap.Int64("version", rule.Version))

	return nil
}

// UpdateRule 更新规则（修复版：拒绝版本冲突）
func (s *RuleService) UpdateRule(ctx context.Context, rule *model.Rule, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "RuleService.UpdateRule")
	defer span.End()

	// 1. 获取当前规则
	oldRule, err := s.repo.GetByID(ctx, rule.RuleID)
	if err != nil {
		return err
	}

	// 2. 权限检查（验证租户归属）
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleWrite, oldRule.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleUpdate, "rule", rule.RuleID, err.Error())
		return err
	}

	// 3. 保留不可变字段。验证依赖 tenant_id，必须在 validateRule 前补齐。
	rule.TenantID = oldRule.TenantID
	rule.CreatedBy = oldRule.CreatedBy
	rule.CreatedAt = oldRule.CreatedAt
	rule.Status = oldRule.Status
	currentVersion := oldRule.Version

	// 4. 验证规则
	if err := s.validateRule(rule); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleUpdate, "rule", rule.RuleID, err.Error())
		return err
	}

	// 5. ✅ 删除重复的唯一性检查，依赖数据库约束

	// 6. 预设新版本
	newVersion := currentVersion + 1
	rule.Version = newVersion
	rule.UpdatedAt = time.Now()
	rule.UpdatedBy = opCtx.UserID

	// 7. ✅ 修复：使用事务更新规则和 Outbox，不重试版本冲突
	err = s.repo.Transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// 7.1 使用 CAS 更新规则
		updateErr := s.updateRuleInTx(ctx, tx, rule, currentVersion)
		if updateErr != nil {
			if updateErr == repository.ErrVersionConflict {
				// ✅ 修复：直接拒绝冲突，不重试
				return errors.Wrap(updateErr, errors.ErrCodeVersionConflict,
					"rule has been modified by another user, please refresh and try again")
			}
			return updateErr
		}

		// 7.2 创建版本记录
		if err := s.createRuleVersionInTx(ctx, tx, rule); err != nil {
			s.logger.Warn("Failed to create rule version record", zap.Error(err))
		}

		// 7.3 写入 Outbox
		if s.config.EnableOutbox {
			cmd := &model.RuleCommand{
				Action:     string(model.ActionUpdate),
				Rule:       rule,
				Timestamp:  time.Now(),
				OperatorID: opCtx.UserID,
			}

			if err := s.insertOutboxInTx(ctx, tx, rule.RuleID, "update", cmd); err != nil {
				return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to insert outbox event")
			}
		}

		return nil
	})

	if err != nil {
		s.logger.Error("Failed to update rule",
			zap.String("rule_id", rule.RuleID),
			zap.Error(err))
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleUpdate, "rule", rule.RuleID, err.Error())
		return err
	}

	// 8. 清除缓存
	s.invalidateRuleCache(ctx, rule.RuleID, rule.TenantID)

	// 9. 记录审计日志
	s.recordRuleChangeAudit(ctx, opCtx, audit.EventTypeRuleUpdate, rule.RuleID, oldRule, rule)

	s.logger.Info("Rule updated successfully",
		zap.String("rule_id", rule.RuleID),
		zap.Int64("old_version", oldRule.Version),
		zap.Int64("new_version", rule.Version))

	return nil
}

// DeleteRule 删除规则
func (s *RuleService) DeleteRule(ctx context.Context, ruleID string, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "RuleService.DeleteRule")
	defer span.End()

	// 1. 获取规则信息
	rule, err := s.repo.GetByID(ctx, ruleID)
	if err != nil {
		return err
	}

	// 2. 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleWrite, rule.TenantID); err != nil {
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleDelete, "rule", ruleID, err.Error())
		return err
	}

	// 3. 检查规则是否被部署使用
	inUse, err := s.isRuleInActiveDeployment(ctx, ruleID)
	if err != nil {
		s.logger.Warn("Failed to check rule deployment status", zap.Error(err))
	} else if inUse {
		err := errors.New(errors.ErrCodeResourceLocked, "rule is used in active deployment, disable it first")
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleDelete, "rule", ruleID, err.Error())
		return err
	}

	// 4. 软删除（使用事务）
	err = s.repo.Transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// 4.1 软删除规则
		if err := s.softDeleteRuleInTx(ctx, tx, ruleID); err != nil {
			return err
		}

		// 4.2 写入 Outbox
		if s.config.EnableOutbox {
			deletedRule := *rule
			deletedRule.Enabled = false
			deletedRule.Status = string(model.RuleStatusArchived)

			cmd := &model.RuleCommand{
				Action:     string(model.ActionDelete),
				Rule:       &deletedRule,
				Timestamp:  time.Now(),
				OperatorID: opCtx.UserID,
			}

			if err := s.insertOutboxInTx(ctx, tx, ruleID, "delete", cmd); err != nil {
				return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to insert outbox event")
			}
		}

		return nil
	})

	if err != nil {
		s.logger.Error("Failed to delete rule",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		s.recordAuditFailure(ctx, opCtx, audit.EventTypeRuleDelete, "rule", ruleID, err.Error())
		return err
	}

	// 5. 清除缓存
	s.invalidateRuleCache(ctx, ruleID, rule.TenantID)

	// 6. 记录审计日志
	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeRuleDelete, "rule", ruleID, map[string]interface{}{
		"name":    rule.Name,
		"version": rule.Version,
		"type":    rule.Type,
		"engine":  rule.Engine,
	})

	s.logger.Info("Rule deleted successfully",
		zap.String("rule_id", ruleID),
		zap.String("tenant_id", rule.TenantID))

	return nil
}

// EnableRule 启用/禁用规则
func (s *RuleService) EnableRule(ctx context.Context, ruleID string, enabled bool, opCtx *OperationContext) error {
	ctx, span := otel.StartSpan(ctx, "RuleService.EnableRule")
	defer span.End()

	// 1. 获取规则信息
	rule, err := s.repo.GetByID(ctx, ruleID)
	if err != nil {
		return err
	}

	// 2. 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermRuleEnable, rule.TenantID); err != nil {
		eventType := audit.EventTypeRuleEnable
		if !enabled {
			eventType = audit.EventTypeRuleDisable
		}
		s.recordAuditFailure(ctx, opCtx, eventType, "rule", ruleID, err.Error())
		return err
	}

	// 3. 如果状态未变，直接返回
	if rule.Enabled == enabled {
		s.logger.Debug("Rule enabled status unchanged",
			zap.String("rule_id", ruleID),
			zap.Bool("enabled", enabled))
		return nil
	}

	status := string(model.RuleStatusDisabled)
	if enabled {
		status = string(model.RuleStatusActive)
	}
	now := time.Now()
	updatedRule := *rule
	updatedRule.Enabled = enabled
	updatedRule.Status = status
	updatedRule.Version = rule.Version + 1
	updatedRule.UpdatedAt = now
	updatedRule.UpdatedBy = opCtx.UserID

	// 4. 使用事务更新状态和 Outbox
	err = s.repo.Transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// 4.1 更新状态并递增版本
		if err := s.setEnabledInTx(ctx, tx, &updatedRule, rule.Version); err != nil {
			return err
		}

		// 4.2 创建状态变更版本记录，保证启用/停用动作可回放
		if err := s.createRuleVersionInTx(ctx, tx, &updatedRule); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to create rule version record")
		}

		// 4.3 写入 Outbox
		if s.config.EnableOutbox {
			action := string(model.ActionEnable)
			if !enabled {
				action = string(model.ActionDisable)
			}

			cmd := &model.RuleCommand{
				Action:     action,
				Rule:       &updatedRule,
				Timestamp:  time.Now(),
				OperatorID: opCtx.UserID,
			}

			if err := s.insertOutboxInTx(ctx, tx, ruleID, action, cmd); err != nil {
				return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to insert outbox event")
			}
		}

		return nil
	})

	if err != nil {
		s.logger.Error("Failed to update rule enabled status",
			zap.String("rule_id", ruleID),
			zap.Error(err))
		return err
	}

	// 5. 清除缓存
	s.invalidateRuleCache(ctx, ruleID, rule.TenantID)

	// 6. 记录审计日志
	eventType := audit.EventTypeRuleEnable
	if !enabled {
		eventType = audit.EventTypeRuleDisable
	}
	s.recordAuditSuccess(ctx, opCtx, eventType, "rule", ruleID, map[string]interface{}{
		"old_enabled": rule.Enabled,
		"new_enabled": enabled,
		"old_status":  rule.Status,
		"new_status":  updatedRule.Status,
		"old_version": rule.Version,
		"new_version": updatedRule.Version,
		"name":        rule.Name,
	})

	s.logger.Info("Rule enabled status changed",
		zap.String("rule_id", ruleID),
		zap.Bool("old_enabled", rule.Enabled),
		zap.Bool("new_enabled", enabled),
		zap.Int64("old_version", rule.Version),
		zap.Int64("new_version", updatedRule.Version))

	return nil
}

// GetRule 获取规则
func (s *RuleService) GetRule(ctx context.Context, ruleID string, opCtx *OperationContext) (*model.Rule, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.GetRule")
	defer span.End()

	// 1. 尝试从缓存获取
	if s.config.EnableCache && s.redis != nil {
		if rule, err := s.getRuleFromCache(ctx, ruleID, opCtx.TenantID); err == nil && rule != nil {
			// 权限检查
			if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleRead, rule.TenantID); err != nil {
				return nil, err
			}
			return rule, nil
		}
	}

	// 2. 从数据库获取
	rule, err := s.repo.GetByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}

	// 3. 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleRead, rule.TenantID); err != nil {
		return nil, err
	}

	// 4. 写入缓存
	if s.config.EnableCache && s.redis != nil {
		s.setRuleToCache(ctx, rule)
	}

	return rule, nil
}

// ListRules 列出规则
func (s *RuleService) ListRules(ctx context.Context, tenantID string, filter *RuleFilter, opCtx *OperationContext) ([]*model.Rule, int64, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.ListRules")
	defer span.End()

	// 1. 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleRead, tenantID); err != nil {
		return nil, 0, err
	}

	// 2. 设置默认值
	if filter == nil {
		filter = &RuleFilter{}
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	// 3. 查询
	rules, total, err := s.repo.ListWithFilter(ctx, tenantID, filter.ToRepoFilter())
	if err != nil {
		return nil, 0, err
	}

	return rules, total, nil
}

// GetRuleVersions 获取规则版本列表
func (s *RuleService) GetRuleVersions(ctx context.Context, ruleID string, opCtx *OperationContext) ([]*model.RuleVersion, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.GetRuleVersions")
	defer span.End()

	// 1. 获取规则以验证权限
	rule, err := s.repo.GetByID(ctx, ruleID)
	if err != nil {
		return nil, err
	}

	// 2. 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleRead, rule.TenantID); err != nil {
		return nil, err
	}

	return s.repo.GetVersions(ctx, ruleID)
}

// =============================================================================
// 批量操作（修复版：使用数据库批量方法 + Outbox）
// =============================================================================

// BatchEnableRules 批量启用/禁用规则（修复版）
func (s *RuleService) BatchEnableRules(ctx context.Context, ruleIDs []string, enabled bool, opCtx *OperationContext) (*BatchResult, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.BatchEnableRules")
	defer span.End()

	result := &BatchResult{
		Total:   len(ruleIDs),
		Success: 0,
		Failed:  0,
		Errors:  make([]BatchError, 0),
	}

	if len(ruleIDs) == 0 {
		return result, nil
	}

	// 1. 验证所有规则的权限
	validIDs := make([]string, 0, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		rule, err := s.repo.GetByID(ctx, ruleID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BatchError{
				ID:      ruleID,
				Message: err.Error(),
			})
			continue
		}

		if err := s.checkPermission(ctx, opCtx, rbac.PermRuleEnable, rule.TenantID); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BatchError{
				ID:      ruleID,
				Message: err.Error(),
			})
			continue
		}

		validIDs = append(validIDs, ruleID)
	}

	if len(validIDs) == 0 {
		return result, nil
	}

	// 2. 使用事务批量更新
	err := s.repo.Transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		// 2.1 批量更新状态
		affected, err := s.batchSetEnabledInTx(ctx, tx, validIDs, enabled)
		if err != nil {
			return err
		}
		result.Success = affected
		result.Failed = len(validIDs) - affected

		// 2.2 批量写入 Outbox
		if s.config.EnableOutbox {
			action := "enable"
			if !enabled {
				action = "disable"
			}

			for _, ruleID := range validIDs {
				rule, err := s.repo.GetByID(ctx, ruleID)
				if err != nil {
					continue
				}

				rule.Enabled = enabled
				rule.UpdatedAt = time.Now()
				rule.UpdatedBy = opCtx.UserID

				cmd := &model.RuleCommand{
					Action:     action,
					Rule:       rule,
					Timestamp:  time.Now(),
					OperatorID: opCtx.UserID,
				}

				if err := s.insertOutboxInTx(ctx, tx, ruleID, action, cmd); err != nil {
					s.logger.Warn("Failed to insert batch outbox event",
						zap.String("rule_id", ruleID),
						zap.Error(err))
				}
			}
		}

		return nil
	})

	if err != nil {
		s.logger.Error("Failed to batch update enabled status",
			zap.Error(err),
			zap.Int("count", len(validIDs)))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to batch update rules")
	}

	// 3. 批量清除缓存
	s.invalidateBatchRuleCache(ctx, validIDs, opCtx.TenantID)

	// 4. 记录审计日志
	action := "batch_enable"
	if !enabled {
		action = "batch_disable"
	}
	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeRuleUpdate, "rule", "", map[string]interface{}{
		"action":   action,
		"total":    result.Total,
		"success":  result.Success,
		"failed":   result.Failed,
		"rule_ids": validIDs,
		"enabled":  enabled,
	})

	return result, nil
}

// BatchDeleteRules 批量删除规则（修复版）
func (s *RuleService) BatchDeleteRules(ctx context.Context, ruleIDs []string, opCtx *OperationContext) (*BatchResult, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.BatchDeleteRules")
	defer span.End()

	result := &BatchResult{
		Total:   len(ruleIDs),
		Success: 0,
		Failed:  0,
		Errors:  make([]BatchError, 0),
	}

	if len(ruleIDs) == 0 {
		return result, nil
	}

	// 逐个验证和删除（删除操作需要检查部署状态，不能批量）
	for _, ruleID := range ruleIDs {
		if err := s.DeleteRule(ctx, ruleID, opCtx); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, BatchError{
				ID:      ruleID,
				Message: err.Error(),
			})
		} else {
			result.Success++
		}
	}

	return result, nil
}

// BatchResult 批量操作结果
type BatchResult struct {
	Total   int          `json:"total"`
	Success int          `json:"success"`
	Failed  int          `json:"failed"`
	Errors  []BatchError `json:"errors,omitempty"`
}

// BatchError 批量操作错误
type BatchError struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// =============================================================================
// 规则搜索
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

// ToRepoFilter 转换为仓储层过滤器
func (f *RuleFilter) ToRepoFilter() *repository.RuleFilter {
	return &repository.RuleFilter{
		Type:     f.Type,
		Engine:   f.Engine,
		Severity: f.Severity,
		Enabled:  f.Enabled,
		Labels:   f.Labels,
		Keyword:  f.Keyword,
		Limit:    f.Limit,
		Offset:   f.Offset,
		OrderBy:  f.OrderBy,
		OrderDir: f.OrderDir,
	}
}

// SearchRules 搜索规则
func (s *RuleService) SearchRules(ctx context.Context, tenantID string, query string, opCtx *OperationContext) ([]*model.Rule, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.SearchRules")
	defer span.End()

	// 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleRead, tenantID); err != nil {
		return nil, err
	}

	filter := &repository.RuleFilter{
		Keyword: query,
		Limit:   50,
	}

	rules, _, err := s.repo.ListWithFilter(ctx, tenantID, filter)
	return rules, err
}

// =============================================================================
// 规则导入导出
// =============================================================================

// ExportRules 导出规则
func (s *RuleService) ExportRules(ctx context.Context, tenantID string, ruleIDs []string, opCtx *OperationContext) (*RuleExport, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.ExportRules")
	defer span.End()

	// 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleRead, tenantID); err != nil {
		return nil, err
	}

	var rules []*model.Rule
	var err error

	if len(ruleIDs) > 0 {
		// 导出指定规则
		for _, id := range ruleIDs {
			rule, err := s.repo.GetByID(ctx, id)
			if err != nil {
				s.logger.Warn("Failed to get rule for export",
					zap.String("rule_id", id),
					zap.Error(err))
				continue
			}
			if rule.TenantID == tenantID {
				rules = append(rules, rule)
			}
		}
	} else {
		// 导出全部规则
		rules, _, err = s.repo.ListWithFilter(ctx, tenantID, &repository.RuleFilter{Limit: 10000})
		if err != nil {
			return nil, err
		}
	}

	export := &RuleExport{
		Version:    "1.0",
		ExportedAt: time.Now(),
		ExportedBy: opCtx.UserID,
		TenantID:   tenantID,
		Rules:      make([]RuleExportItem, 0, len(rules)),
	}

	for _, rule := range rules {
		export.Rules = append(export.Rules, RuleExportItem{
			Name:        rule.Name,
			Type:        rule.Type,
			Engine:      rule.Engine,
			Description: rule.Description,
			Conditions:  rule.Conditions,
			Labels:      rule.Labels,
			Severity:    rule.Severity,
			Enabled:     rule.Enabled,
		})
	}

	// 记录审计
	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeExportReport, "rule", "", map[string]interface{}{
		"count":   len(export.Rules),
		"version": export.Version,
	})

	return export, nil
}

// ImportRules 导入规则
func (s *RuleService) ImportRules(ctx context.Context, tenantID string, data *RuleExport, opCtx *OperationContext) (*ImportResult, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.ImportRules")
	defer span.End()

	// 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleWrite, tenantID); err != nil {
		return nil, err
	}

	result := &ImportResult{
		Total:   len(data.Rules),
		Created: 0,
		Updated: 0,
		Skipped: 0,
		Failed:  0,
		Errors:  make([]ImportError, 0),
	}

	for i, item := range data.Rules {
		rule := &model.Rule{
			TenantID:    tenantID,
			Name:        item.Name,
			Type:        item.Type,
			Engine:      item.Engine,
			Description: item.Description,
			Conditions:  item.Conditions,
			Labels:      item.Labels,
			Severity:    item.Severity,
			Enabled:     item.Enabled,
		}

		// 检查是否已存在
		existingRule, err := s.repo.GetByName(ctx, tenantID, item.Name)
		if err == nil && existingRule != nil {
			// 更新现有规则
			rule.RuleID = existingRule.RuleID
			if err := s.UpdateRule(ctx, rule, opCtx); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, ImportError{
					Index:   i,
					Name:    item.Name,
					Message: err.Error(),
				})
			} else {
				result.Updated++
			}
		} else {
			// 创建新规则
			if err := s.CreateRule(ctx, rule, opCtx); err != nil {
				result.Failed++
				result.Errors = append(result.Errors, ImportError{
					Index:   i,
					Name:    item.Name,
					Message: err.Error(),
				})
			} else {
				result.Created++
			}
		}
	}

	return result, nil
}

// RuleExport 规则导出结构
type RuleExport struct {
	Version    string           `json:"version"`
	ExportedAt time.Time        `json:"exported_at"`
	ExportedBy string           `json:"exported_by"`
	TenantID   string           `json:"tenant_id"`
	Rules      []RuleExportItem `json:"rules"`
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
}

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
}

// =============================================================================
// 同步与统计
// =============================================================================

// SyncRulesToKafka 同步所有规则到 Kafka
func (s *RuleService) SyncRulesToKafka(ctx context.Context, tenantID string, opCtx *OperationContext) (int, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.SyncRulesToKafka")
	defer span.End()

	// 权限检查（需要管理员权限）
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionAdminWrite, tenantID); err != nil {
		return 0, err
	}

	limit := 1000
	offset := 0
	syncedCount := 0

	for {
		rules, total, err := s.repo.ListWithFilter(ctx, tenantID, &repository.RuleFilter{
			Limit:  limit,
			Offset: offset,
		})
		if err != nil {
			return syncedCount, err
		}

		if len(rules) == 0 {
			break
		}

		// 使用事务批量写入 Outbox
		err = s.repo.Transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
			for _, rule := range rules {
				if rule.Enabled {
					cmd := &model.RuleCommand{
						Action:     string(model.ActionSync),
						Rule:       rule,
						Timestamp:  time.Now(),
						OperatorID: opCtx.UserID,
					}

					if err := s.insertOutboxInTx(ctx, tx, rule.RuleID, "sync", cmd); err != nil {
						return err
					}
					syncedCount++
				}
			}
			return nil
		})

		if err != nil {
			s.logger.Error("Failed to sync rules batch",
				zap.Int("batch_size", len(rules)),
				zap.Error(err))
			return syncedCount, err
		}

		offset += limit
		if int64(offset) >= total {
			break
		}
	}

	// 记录审计
	s.recordAuditSuccess(ctx, opCtx, audit.EventTypeConfigUpdate, "rule", "", map[string]interface{}{
		"action":       "sync_to_kafka",
		"synced_count": syncedCount,
	})

	s.logger.Info("Rules queued for sync to Kafka",
		zap.String("tenant_id", tenantID),
		zap.Int("synced_count", syncedCount))

	return syncedCount, nil
}

// GetRuleStats 获取规则统计
func (s *RuleService) GetRuleStats(ctx context.Context, tenantID string, opCtx *OperationContext) (*repository.RuleStats, error) {
	ctx, span := otel.StartSpan(ctx, "RuleService.GetRuleStats")
	defer span.End()

	// 权限检查
	if err := s.checkPermission(ctx, opCtx, rbac.PermissionRuleRead, tenantID); err != nil {
		return nil, err
	}

	return s.repo.GetStats(ctx, tenantID)
}

// =============================================================================
// Outbox 模式实现（核心补偿机制）
// =============================================================================

// insertOutboxInTx 在事务中插入 Outbox 事件
func (s *RuleService) insertOutboxInTx(ctx context.Context, tx *sql.Tx, ruleID, eventType string, cmd *model.RuleCommand) error {
	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	query := `
		INSERT INTO rule_outbox (rule_id, event_type, payload, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err = tx.ExecContext(ctx, query, ruleID, eventType, string(payload), time.Now())
	return err
}

// startOutboxProcessor 启动 Outbox 后台处理器
func (s *RuleService) startOutboxProcessor() {
	ticker := time.NewTicker(s.config.OutboxProcessInterval)
	defer ticker.Stop()

	s.logger.Info("Outbox processor started")

	for {
		select {
		case <-ticker.C:
			if err := s.processOutbox(); err != nil {
				s.logger.Error("Failed to process outbox", zap.Error(err))
			}
		case <-s.outboxStopCh:
			s.logger.Info("Outbox processor stopping")
			return
		}
	}
}

// processOutbox 处理 Outbox 事件
func (s *RuleService) processOutbox() error {
	ctx := context.Background()

	// 1. 查询待发布的事件（限制每次处理 100 条）
	query := `
		SELECT id, rule_id, event_type, payload, retry_count
		FROM rule_outbox
		WHERE published = false
		  AND (next_retry IS NULL OR next_retry <= $1)
		ORDER BY created_at ASC
		LIMIT 100
	`

	rows, err := s.db.QueryContext(ctx, query, time.Now())
	if err != nil {
		return fmt.Errorf("failed to query outbox: %w", err)
	}
	defer rows.Close()

	type outboxEvent struct {
		ID         int64
		RuleID     string
		EventType  string
		Payload    []byte
		RetryCount int
	}

	events := make([]outboxEvent, 0, 100)
	for rows.Next() {
		var e outboxEvent
		if err := rows.Scan(&e.ID, &e.RuleID, &e.EventType, &e.Payload, &e.RetryCount); err != nil {
			s.logger.Error("Failed to scan outbox event", zap.Error(err))
			continue
		}
		events = append(events, e)
	}

	if len(events) == 0 {
		return nil
	}

	s.logger.Info("Processing outbox events", zap.Int("count", len(events)))

	// 2. 逐条发布到 Kafka
	for _, e := range events {
		var cmd model.RuleCommand
		if err := json.Unmarshal(e.Payload, &cmd); err != nil {
			s.logger.Error("Failed to unmarshal outbox payload",
				zap.Int64("id", e.ID),
				zap.Error(err))
			s.markOutboxFailed(ctx, e.ID, err.Error())
			continue
		}

		// 发布到 Kafka
		publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := s.publisher.PublishRuleCommandWithRetry(publishCtx, &cmd, 3)
		cancel()

		if err != nil {
			s.logger.Warn("Failed to publish outbox event to Kafka",
				zap.Int64("id", e.ID),
				zap.String("rule_id", e.RuleID),
				zap.String("event_type", e.EventType),
				zap.Int("retry_count", e.RetryCount),
				zap.Error(err))

			// 如果重试次数超过限制，标记为失败
			if e.RetryCount >= outboxMaxRetries {
				s.markOutboxFailed(ctx, e.ID, err.Error())
			} else {
				// 安排下次重试（指数退避）
				nextRetry := time.Now().Add(outboxRetryDelay * time.Duration(1<<uint(e.RetryCount)))
				s.scheduleOutboxRetry(ctx, e.ID, e.RetryCount+1, nextRetry, err.Error())
			}
			continue
		}

		// 发布成功，标记为已发布
		if err := s.markOutboxPublished(ctx, e.ID); err != nil {
			s.logger.Error("Failed to mark outbox as published",
				zap.Int64("id", e.ID),
				zap.Error(err))
		} else {
			s.logger.Debug("Outbox event published successfully",
				zap.Int64("id", e.ID),
				zap.String("rule_id", e.RuleID),
				zap.String("event_type", e.EventType))
		}
	}

	return nil
}

// markOutboxPublished 标记 Outbox 事件为已发布
func (s *RuleService) markOutboxPublished(ctx context.Context, id int64) error {
	query := `
		UPDATE rule_outbox
		SET published = true, published_at = $1
		WHERE id = $2
	`
	_, err := s.db.ExecContext(ctx, query, time.Now(), id)
	return err
}

// scheduleOutboxRetry 安排 Outbox 事件重试
func (s *RuleService) scheduleOutboxRetry(ctx context.Context, id int64, retryCount int, nextRetry time.Time, lastError string) error {
	query := `
		UPDATE rule_outbox
		SET retry_count = $1, next_retry = $2, last_error = $3
		WHERE id = $4
	`
	_, err := s.db.ExecContext(ctx, query, retryCount, nextRetry, lastError, id)
	return err
}

// markOutboxFailed 标记 Outbox 事件为失败
func (s *RuleService) markOutboxFailed(ctx context.Context, id int64, lastError string) error {
	query := `
		UPDATE rule_outbox
		SET published = true, published_at = $1, last_error = $2
		WHERE id = $3
	`
	_, err := s.db.ExecContext(ctx, query, time.Now(), "MAX_RETRIES_EXCEEDED: "+lastError, id)
	if err == nil {
		s.logger.Error("Outbox event marked as failed after max retries",
			zap.Int64("id", id),
			zap.String("error", lastError))
	}
	return err
}

// =============================================================================
// 事务内的数据库操作辅助方法
// =============================================================================

// createRuleInTx 在事务中创建规则
func (s *RuleService) createRuleInTx(ctx context.Context, tx *sql.Tx, rule *model.Rule) error {
	if err := rule.MarshalConditions(); err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal conditions")
	}

	query := `
		INSERT INTO rules (
			rule_id, tenant_id, name, rule_type, engine, description,
			conditions, labels, severity, enabled, priority, version, status,
			created_by, updated_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`

	_, err := tx.ExecContext(ctx, query,
		rule.RuleID,
		rule.TenantID,
		rule.Name,
		rule.Type,
		rule.Engine,
		rule.Description,
		rule.ConditionsJSON,
		pq.Array(rule.Labels),
		rule.Severity,
		rule.Enabled,
		rule.Priority,
		rule.Version,
		rule.Status,
		rule.CreatedBy,
		rule.CreatedBy,
		rule.CreatedAt,
		rule.UpdatedAt,
	)

	return err
}

// createRuleVersionInTx 在事务中创建规则版本
func (s *RuleService) createRuleVersionInTx(ctx context.Context, tx *sql.Tx, rule *model.Rule) error {
	contentJSON, err := json.Marshal(rule)
	if err != nil {
		return err
	}

	versionID := fmt.Sprintf("%s-v%d", rule.RuleID, rule.Version)
	createdBy := rule.UpdatedBy
	if createdBy == "" {
		createdBy = rule.CreatedBy
	}

	query := `
		INSERT INTO rule_versions (rule_version, rule_id, tenant_id, version, content_uri, status, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (rule_version) DO NOTHING
	`
	_, err = tx.ExecContext(ctx, query,
		versionID,
		rule.RuleID,
		rule.TenantID,
		rule.Version,
		fmt.Sprintf("inline:%s", string(contentJSON)),
		"active",
		createdBy,
		time.Now(),
	)
	return err
}

// updateRuleInTx 在事务中更新规则（带 CAS）
func (s *RuleService) updateRuleInTx(ctx context.Context, tx *sql.Tx, rule *model.Rule, expectedVersion int64) error {
	if err := rule.MarshalConditions(); err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal conditions")
	}

	query := `
		UPDATE rules
		SET name = $1, rule_type = $2, engine = $3, description = $4,
			conditions = $5, labels = $6, severity = $7, enabled = $8,
			priority = $9, version = $10, status = $11, updated_by = $12, updated_at = $13
		WHERE rule_id = $14 AND version = $15
	`

	result, err := tx.ExecContext(ctx, query,
		rule.Name,
		rule.Type,
		rule.Engine,
		rule.Description,
		rule.ConditionsJSON,
		pq.Array(rule.Labels),
		rule.Severity,
		rule.Enabled,
		rule.Priority,
		rule.Version,
		rule.Status,
		rule.UpdatedBy,
		rule.UpdatedAt,
		rule.RuleID,
		expectedVersion,
	)

	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return repository.ErrVersionConflict
	}

	return nil
}

// softDeleteRuleInTx 在事务中软删除规则
func (s *RuleService) softDeleteRuleInTx(ctx context.Context, tx *sql.Tx, ruleID string) error {
	query := `UPDATE rules SET status = 'deleted', updated_at = $1 WHERE rule_id = $2 AND status != 'deleted'`
	result, err := tx.ExecContext(ctx, query, time.Now(), ruleID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.Newf(errors.ErrCodeRuleNotFound, "rule not found: %s", ruleID)
	}

	return nil
}

// setEnabledInTx 在事务中设置启用状态并使用乐观锁递增版本
func (s *RuleService) setEnabledInTx(ctx context.Context, tx *sql.Tx, rule *model.Rule, expectedVersion int64) error {
	query := `
		UPDATE rules
		SET enabled = $1, status = $2, version = $3, updated_by = $4, updated_at = $5
		WHERE rule_id = $6 AND version = $7 AND status != 'deleted'
	`
	result, err := tx.ExecContext(ctx, query,
		rule.Enabled,
		rule.Status,
		rule.Version,
		rule.UpdatedBy,
		rule.UpdatedAt,
		rule.RuleID,
		expectedVersion,
	)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return repository.ErrVersionConflict
	}

	return nil
}

// batchSetEnabledInTx 在事务中批量设置启用状态
func (s *RuleService) batchSetEnabledInTx(ctx context.Context, tx *sql.Tx, ruleIDs []string, enabled bool) (int, error) {
	status := string(model.RuleStatusDisabled)
	if enabled {
		status = string(model.RuleStatusActive)
	}

	// 构建 IN 子句
	placeholders := make([]string, len(ruleIDs))
	args := make([]interface{}, len(ruleIDs)+3)
	args[0] = enabled
	args[1] = status
	args[2] = time.Now()

	for i, id := range ruleIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+4)
		args[i+3] = id
	}

	query := fmt.Sprintf(`
		UPDATE rules 
		SET enabled = $1, status = $2, updated_at = $3 
		WHERE rule_id IN (%s) AND status != 'deleted'
	`, strings.Join(placeholders, ","))

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}

	rowsAffected, _ := result.RowsAffected()
	return int(rowsAffected), nil
}

// =============================================================================
// 验证方法
// =============================================================================

// validateRule 验证规则
func (s *RuleService) validateRule(rule *model.Rule) error {
	// 基础字段验证
	if rule.Name == "" {
		return errors.New(errors.ErrCodeMissingParameter, "name is required")
	}
	if len(rule.Name) > maxRuleNameLength {
		return errors.Newf(errors.ErrCodeInvalidParameter, "name too long: max %d characters", maxRuleNameLength)
	}
	if rule.TenantID == "" {
		return errors.New(errors.ErrCodeMissingParameter, "tenant_id is required")
	}
	if rule.Type == "" {
		return errors.New(errors.ErrCodeMissingParameter, "type is required")
	}
	if len(rule.Description) > maxDescriptionLength {
		return errors.Newf(errors.ErrCodeInvalidParameter, "description too long: max %d characters", maxDescriptionLength)
	}

	// 设置默认值
	if rule.Severity == "" {
		rule.Severity = string(model.SeverityMedium)
	}
	if rule.Engine == "" {
		rule.Engine = string(model.EngineInternal)
	}

	// 验证 Severity
	if !model.IsValidSeverity(rule.Severity) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid severity: %s", rule.Severity)
	}

	// 验证 Engine
	if !model.IsValidRuleEngine(rule.Engine) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid engine: %s", rule.Engine)
	}

	// 验证 Type
	if !model.IsValidRuleType(rule.Type) {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid type: %s", rule.Type)
	}

	// 验证 Labels
	if len(rule.Labels) > maxLabelsPerRule {
		return errors.Newf(errors.ErrCodeInvalidParameter, "too many labels: max %d", maxLabelsPerRule)
	}

	// 验证 Conditions（基础检查）
	if rule.Conditions != nil {
		if err := validateConditions(rule.Conditions, 0); err != nil {
			return err
		}
	}

	// 名称格式验证
	if !isValidRuleName(rule.Name) {
		return errors.New(errors.ErrCodeInvalidFormat, "invalid name format: only letters, numbers, underscores, hyphens, and spaces are allowed")
	}

	return nil
}

// validateConditions 递归验证条件
func validateConditions(conditions map[string]interface{}, depth int) error {
	if depth > maxConditionDepth {
		return errors.Newf(errors.ErrCodeInvalidParameter, "condition nesting too deep: max %d levels", maxConditionDepth)
	}

	for key, value := range conditions {
		if key == "" {
			return errors.New(errors.ErrCodeInvalidParameter, "empty condition key")
		}

		switch v := value.(type) {
		case map[string]interface{}:
			if err := validateConditions(v, depth+1); err != nil {
				return err
			}
		case []interface{}:
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					if err := validateConditions(m, depth+1); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// isValidRuleName 验证规则名称格式
func isValidRuleName(name string) bool {
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == ' ' ||
			c >= 0x4e00 && c <= 0x9fff) {
			return false
		}
	}
	return true
}

// =============================================================================
// 权限检查
// =============================================================================

// checkPermission 检查权限
func (s *RuleService) checkPermission(ctx context.Context, opCtx *OperationContext, permission rbac.Permission, resourceTenantID string) error {
	if opCtx == nil {
		return errors.New(errors.ErrCodeUnauthorized, "operation context required")
	}

	// 检查租户隔离
	if opCtx.TenantID != resourceTenantID && !s.hasAdminPermission(opCtx) {
		return errors.New(errors.ErrCodePermissionDenied, "cross-tenant access denied")
	}

	// 如果没有 RBAC 检查器，只做租户隔离
	if s.rbacChecker == nil {
		return nil
	}

	// 检查权限
	if !s.rbacChecker.HasPermission(opCtx.Permissions, permission) {
		return errors.Newf(errors.ErrCodePermissionDenied, "permission denied: %s required", permission)
	}

	return nil
}

// hasAdminPermission 检查是否有管理员权限
func (s *RuleService) hasAdminPermission(opCtx *OperationContext) bool {
	for _, p := range opCtx.Permissions {
		if p == string(rbac.PermissionAdminWrite) || p == "admin:*" || p == "*" {
			return true
		}
	}
	return false
}

// =============================================================================
// 缓存方法（修复版：使用租户 ID 前缀）
// =============================================================================

// getRuleFromCache 从缓存获取规则（修复版）
func (s *RuleService) getRuleFromCache(ctx context.Context, ruleID, tenantID string) (*model.Rule, error) {
	if s.redis == nil {
		return nil, nil
	}

	key := fmt.Sprintf(ruleCachePrefix, tenantID, ruleID)
	data, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var rule model.Rule
	if err := json.Unmarshal([]byte(data), &rule); err != nil {
		return nil, err
	}

	return &rule, nil
}

// setRuleToCache 写入规则到缓存（修复版）
func (s *RuleService) setRuleToCache(ctx context.Context, rule *model.Rule) {
	if s.redis == nil || rule == nil {
		return
	}

	key := fmt.Sprintf(ruleCachePrefix, rule.TenantID, rule.RuleID)
	data, err := json.Marshal(rule)
	if err != nil {
		s.logger.Warn("Failed to marshal rule for cache",
			zap.String("rule_id", rule.RuleID),
			zap.Error(err))
		return
	}

	if err := s.redis.Set(ctx, key, data, s.config.CacheTTL).Err(); err != nil {
		s.logger.Warn("Failed to cache rule",
			zap.String("rule_id", rule.RuleID),
			zap.Error(err))
	}
}

// invalidateRuleCache 清除规则缓存（修复版）
func (s *RuleService) invalidateRuleCache(ctx context.Context, ruleID, tenantID string) {
	if s.redis == nil {
		return
	}

	// 清除单个规则缓存
	ruleKey := fmt.Sprintf(ruleCachePrefix, tenantID, ruleID)
	if err := s.redis.Del(ctx, ruleKey).Err(); err != nil {
		s.logger.Warn("Failed to invalidate rule cache",
			zap.String("rule_id", ruleID),
			zap.Error(err))
	}

	// 清除列表缓存（使用模式匹配）
	listPattern := fmt.Sprintf(ruleListCachePrefix, tenantID, "*")
	if err := s.deleteByPattern(ctx, listPattern); err != nil {
		s.logger.Warn("Failed to invalidate rule list cache",
			zap.String("pattern", listPattern),
			zap.Error(err))
	}
}

// invalidateBatchRuleCache 批量清除规则缓存
func (s *RuleService) invalidateBatchRuleCache(ctx context.Context, ruleIDs []string, tenantID string) {
	if s.redis == nil || len(ruleIDs) == 0 {
		return
	}

	keys := make([]string, len(ruleIDs))
	for i, id := range ruleIDs {
		keys[i] = fmt.Sprintf(ruleCachePrefix, tenantID, id)
	}

	if err := s.redis.Del(ctx, keys...).Err(); err != nil {
		s.logger.Warn("Failed to batch invalidate rule cache",
			zap.Int("count", len(keys)),
			zap.Error(err))
	}

	// 清除列表缓存
	listPattern := fmt.Sprintf(ruleListCachePrefix, tenantID, "*")
	s.deleteByPattern(ctx, listPattern)
}

// deleteByPattern 根据模式删除缓存
func (s *RuleService) deleteByPattern(ctx context.Context, pattern string) error {
	if s.redis == nil {
		return nil
	}

	iter := s.redis.Scan(ctx, 0, pattern, 100).Iterator()
	keys := make([]string, 0, 100)

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) >= 100 {
			if err := s.redis.Del(ctx, keys...).Err(); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}

	if len(keys) > 0 {
		if err := s.redis.Del(ctx, keys...).Err(); err != nil {
			return err
		}
	}

	return iter.Err()
}

// =============================================================================
// 辅助方法
// =============================================================================

// isRuleInActiveDeployment 检查规则是否在真实部署中被引用。
// 规则版本处于 active 只表示版本可发布，不能等价为已有部署。
func (s *RuleService) isRuleInActiveDeployment(ctx context.Context, ruleID string) (bool, error) {
	if s.db == nil {
		return false, nil
	}

	versions, err := s.repo.GetVersions(ctx, ruleID)
	if err != nil {
		return false, fmt.Errorf("isRuleInActiveDeployment: get versions: %w", err)
	}
	if len(versions) == 0 {
		return false, nil
	}

	ruleVersions := make([]string, 0, len(versions))
	for _, version := range versions {
		if strings.TrimSpace(version.RuleVersionID) != "" {
			ruleVersions = append(ruleVersions, version.RuleVersionID)
		}
	}
	if len(ruleVersions) == 0 {
		return false, nil
	}

	query := `
		SELECT COUNT(1)
		FROM deployments
		WHERE rule_version = ANY($1)
		  AND status IN ('gray', 'active', 'paused')
	`
	var count int64
	if err := s.db.QueryRowContext(ctx, query, pq.Array(ruleVersions)).Scan(&count); err != nil {
		return false, fmt.Errorf("isRuleInActiveDeployment: query deployments: %w", err)
	}
	return count > 0, nil
}

// =============================================================================
// 审计日志方法
// =============================================================================

// recordAuditSuccess 记录审计成功
func (s *RuleService) recordAuditSuccess(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID string, detail map[string]interface{}) {
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

// recordAuditFailure 记录审计失败
func (s *RuleService) recordAuditFailure(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID, errorMsg string) {
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

func (s *RuleService) recordAuditLogDB(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, resourceType, resourceID string, detail map[string]interface{}, result audit.Result, errorMsg string) {
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
		s.logger.Warn("Failed to marshal rule audit detail",
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
		s.logger.Warn("Failed to persist rule audit log",
			zap.String("event_type", string(eventType)),
			zap.String("resource_type", resourceType),
			zap.String("resource_id", resourceID),
			zap.Error(err))
	}
}

// recordRuleChangeAudit 记录规则变更审计
func (s *RuleService) recordRuleChangeAudit(ctx context.Context, opCtx *OperationContext, eventType audit.EventType, ruleID string, oldRule, newRule *model.Rule) {
	if s.auditLogger == nil || opCtx == nil {
		return
	}

	changes := make(map[string]interface{})

	if oldRule.Name != newRule.Name {
		changes["name"] = map[string]string{"old": oldRule.Name, "new": newRule.Name}
	}
	if oldRule.Type != newRule.Type {
		changes["type"] = map[string]string{"old": oldRule.Type, "new": newRule.Type}
	}
	if oldRule.Severity != newRule.Severity {
		changes["severity"] = map[string]string{"old": oldRule.Severity, "new": newRule.Severity}
	}
	if oldRule.Enabled != newRule.Enabled {
		changes["enabled"] = map[string]bool{"old": oldRule.Enabled, "new": newRule.Enabled}
	}
	if !strings.EqualFold(oldRule.Description, newRule.Description) {
		changes["description"] = "changed"
	}

	s.auditLogger.LogRuleChange(ctx, eventType, opCtx.TenantID, opCtx.UserID, ruleID,
		map[string]interface{}{
			"version": oldRule.Version,
			"name":    oldRule.Name,
		},
		map[string]interface{}{
			"version": newRule.Version,
			"name":    newRule.Name,
			"changes": changes,
		},
	)
}
