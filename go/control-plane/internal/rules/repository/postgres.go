////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/repository/postgres.go
// PostgreSQL Repository - 完整修复版（1200+行完整代码）
// 修复内容：
// 1. ✅ 修复问题 3.1: GetByName 返回专用错误而非 nil
// 2. ✅ 修复问题 3.2: ListWithFilter 添加参数校验
// 3. ✅ 添加 ExistsByName 方法
// 4. ✅ 完善错误处理
// 5. ✅ 添加批量操作、统计方法、搜索、健康检查、软删除
////////////////////////////////////////////////////////////////////////////////

package repository

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

// =============================================================================
// 错误定义
// =============================================================================

// ✅ 修复问题 3.1: 定义专用错误
var (
	ErrVersionConflict    = errors.New(errors.ErrCodeVersionConflict, "version conflict, please retry")
	ErrNotFound           = errors.New(errors.ErrCodeRuleNotFound, "rule not found")
	ErrRuleNotFoundByName = errors.New(errors.ErrCodeRuleNotFound, "rule not found by name")
	ErrRuleNotFoundByID   = errors.New(errors.ErrCodeRuleNotFound, "rule not found by id")
)

// =============================================================================
// Repository 定义
// =============================================================================

// RuleRepository 规则仓库
type RuleRepository struct {
	client *storage.PostgresClient
	db     *sql.DB
	logger *zap.Logger
}

// NewRuleRepository 创建规则仓库
func NewRuleRepository(client *storage.PostgresClient, logger *zap.Logger) *RuleRepository {
	return &RuleRepository{
		client: client,
		db:     client.DB(),
		logger: logger,
	}
}

// NewRuleRepositoryWithDB 使用原生 DB 创建规则仓库
func NewRuleRepositoryWithDB(db *sql.DB, logger *zap.Logger) *RuleRepository {
	return &RuleRepository{
		db:     db,
		logger: logger,
	}
}

// =============================================================================
// 基础 CRUD 操作
// =============================================================================

// Create 创建规则（自动生成 ID）
func (r *RuleRepository) Create(ctx context.Context, rule *model.Rule) error {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.Create")
	defer span.End()

	if rule.RuleID == "" {
		rule.RuleID = uuid.New().String()
	}
	return r.insertRule(ctx, rule)
}

// CreateWithID 创建规则（使用预生成的 ID）
func (r *RuleRepository) CreateWithID(ctx context.Context, rule *model.Rule) error {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.CreateWithID")
	defer span.End()

	if rule.RuleID == "" {
		return errors.New(errors.ErrCodeInvalidParameter, "rule_id is required for CreateWithID")
	}
	return r.insertRule(ctx, rule)
}

// insertRule 内部方法：插入规则
func (r *RuleRepository) insertRule(ctx context.Context, rule *model.Rule) error {
	// 设置默认值
	if rule.Version == 0 {
		rule.Version = 1
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = time.Now()
	}
	if rule.Status == "" {
		rule.Status = string(model.RuleStatusDraft)
	}

	// 序列化条件
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

	_, err := r.db.ExecContext(ctx, query,
		rule.RuleID, rule.TenantID, rule.Name, rule.Type, rule.Engine, rule.Description,
		rule.ConditionsJSON, pq.Array(rule.Labels), rule.Severity, rule.Enabled,
		rule.Priority, rule.Version, rule.Status, rule.CreatedBy, rule.CreatedBy,
		rule.CreatedAt, rule.UpdatedAt,
	)

	if err != nil {
		// 检查唯一约束冲突
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return errors.Newf(errors.ErrCodeResourceExists, "rule already exists: %s", rule.RuleID)
		}
		r.logger.Error("Failed to create rule", zap.String("rule_id", rule.RuleID), zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to insert rule")
	}

	r.logger.Info("Rule created", zap.String("rule_id", rule.RuleID), zap.String("tenant_id", rule.TenantID), zap.String("name", rule.Name))
	return nil
}

// Update 更新规则（使用 CAS 乐观锁）
func (r *RuleRepository) Update(ctx context.Context, rule *model.Rule, expectedVersion int64) error {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.Update")
	defer span.End()

	newVersion := expectedVersion + 1
	rule.Version = newVersion
	rule.UpdatedAt = time.Now()

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

	result, err := r.db.ExecContext(ctx, query,
		rule.Name, rule.Type, rule.Engine, rule.Description,
		rule.ConditionsJSON, pq.Array(rule.Labels), rule.Severity, rule.Enabled,
		rule.Priority, newVersion, rule.Status, rule.UpdatedBy, rule.UpdatedAt,
		rule.RuleID, expectedVersion,
	)

	if err != nil {
		r.logger.Error("Failed to update rule", zap.String("rule_id", rule.RuleID), zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update rule")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		exists, _ := r.Exists(ctx, rule.RuleID)
		if !exists {
			return errors.Newf(errors.ErrCodeRuleNotFound, "rule not found: %s", rule.RuleID)
		}
		r.logger.Warn("Version conflict during rule update", zap.String("rule_id", rule.RuleID), zap.Int64("expected_version", expectedVersion))
		return ErrVersionConflict
	}

	r.logger.Info("Rule updated", zap.String("rule_id", rule.RuleID), zap.Int64("old_version", expectedVersion), zap.Int64("new_version", newVersion))
	return nil
}

// GetByID 根据 ID 获取规则
func (r *RuleRepository) GetByID(ctx context.Context, ruleID string) (*model.Rule, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.GetByID")
	defer span.End()

	query := `
		SELECT rule_id, tenant_id, name, rule_type, engine, description,
			   conditions, labels, severity, enabled, priority, version, status,
			   created_by, updated_by, created_at, updated_at
		FROM rules
		WHERE rule_id = $1 AND status != 'deleted'
	`

	var rule model.Rule
	var labels pq.StringArray
	var updatedBy sql.NullString

	err := r.db.QueryRowContext(ctx, query, ruleID).Scan(
		&rule.RuleID, &rule.TenantID, &rule.Name, &rule.Type, &rule.Engine, &rule.Description,
		&rule.ConditionsJSON, &labels, &rule.Severity, &rule.Enabled, &rule.Priority,
		&rule.Version, &rule.Status, &rule.CreatedBy, &updatedBy, &rule.CreatedAt, &rule.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeRuleNotFound, "rule not found: %s", ruleID)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get rule")
	}

	if err := rule.UnmarshalConditions(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to unmarshal conditions")
	}

	rule.Labels = labels
	if updatedBy.Valid {
		rule.UpdatedBy = updatedBy.String
	}

	return &rule, nil
}

// ✅ 修复问题 3.1: GetByName 返回专用错误
func (r *RuleRepository) GetByName(ctx context.Context, tenantID, name string) (*model.Rule, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.GetByName")
	defer span.End()

	query := `
		SELECT rule_id, tenant_id, name, rule_type, engine, description,
			   conditions, labels, severity, enabled, priority, version, status,
			   created_by, updated_by, created_at, updated_at
		FROM rules
		WHERE tenant_id = $1 AND name = $2 AND status != 'deleted'
	`

	var rule model.Rule
	var labels pq.StringArray
	var updatedBy sql.NullString

	err := r.db.QueryRowContext(ctx, query, tenantID, name).Scan(
		&rule.RuleID, &rule.TenantID, &rule.Name, &rule.Type, &rule.Engine, &rule.Description,
		&rule.ConditionsJSON, &labels, &rule.Severity, &rule.Enabled, &rule.Priority,
		&rule.Version, &rule.Status, &rule.CreatedBy, &updatedBy, &rule.CreatedAt, &rule.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrRuleNotFoundByName // ✅ 返回专用错误
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get rule by name")
	}

	if err := rule.UnmarshalConditions(); err != nil {
		return nil, err
	}

	rule.Labels = labels
	if updatedBy.Valid {
		rule.UpdatedBy = updatedBy.String
	}

	return &rule, nil
}

// Exists 检查规则是否存在
func (r *RuleRepository) Exists(ctx context.Context, ruleID string) (bool, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.Exists")
	defer span.End()

	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rules WHERE rule_id = $1 AND status != 'deleted'", ruleID).Scan(&count)
	if err != nil {
		return false, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to check rule existence")
	}
	return count > 0, nil
}

// ✅ 新增方法: ExistsByName
func (r *RuleRepository) ExistsByName(ctx context.Context, tenantID, name string) (bool, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.ExistsByName")
	defer span.End()

	var count int
	query := `SELECT COUNT(*) FROM rules WHERE tenant_id = $1 AND name = $2 AND status != 'deleted'`
	err := r.db.QueryRowContext(ctx, query, tenantID, name).Scan(&count)
	if err != nil {
		return false, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to check rule name existence")
	}
	return count > 0, nil
}

// ✅ 修复问题 3.2: ListWithFilter 添加参数校验
func (r *RuleRepository) ListWithFilter(ctx context.Context, tenantID string, filter *RuleFilter) ([]*model.Rule, int64, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.ListWithFilter")
	defer span.End()

	if filter == nil {
		filter = &RuleFilter{Limit: 20}
	}

	// ✅ 参数校验
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000 // ✅ 硬限制
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	// ✅ OrderBy 白名单校验
	validOrderBy := map[string]bool{
		"name": true, "created_at": true, "updated_at": true, "severity": true, "priority": true,
	}
	if filter.OrderBy != "" && !validOrderBy[filter.OrderBy] {
		return nil, 0, errors.Newf(errors.ErrCodeInvalidParameter, "invalid order_by: %s", filter.OrderBy)
	}

	// ✅ OrderDir 校验
	if filter.OrderDir != "" {
		filter.OrderDir = strings.ToUpper(filter.OrderDir)
		if filter.OrderDir != "ASC" && filter.OrderDir != "DESC" {
			return nil, 0, errors.New(errors.ErrCodeInvalidParameter, "order_dir must be ASC or DESC")
		}
	}

	// 构建查询
	baseQuery := `FROM rules WHERE tenant_id = $1 AND status != 'deleted'`
	args := []interface{}{tenantID}
	argIdx := 2

	if filter.Type != "" {
		baseQuery += fmt.Sprintf(" AND rule_type = $%d", argIdx)
		args = append(args, filter.Type)
		argIdx++
	}
	if filter.Engine != "" {
		baseQuery += fmt.Sprintf(" AND engine = $%d", argIdx)
		args = append(args, filter.Engine)
		argIdx++
	}
	if filter.Severity != "" {
		baseQuery += fmt.Sprintf(" AND severity = $%d", argIdx)
		args = append(args, filter.Severity)
		argIdx++
	}
	if filter.Enabled != nil {
		baseQuery += fmt.Sprintf(" AND enabled = $%d", argIdx)
		args = append(args, *filter.Enabled)
		argIdx++
	}
	if len(filter.Labels) > 0 {
		baseQuery += fmt.Sprintf(" AND labels && $%d", argIdx)
		args = append(args, pq.Array(filter.Labels))
		argIdx++
	}
	if filter.Keyword != "" {
		baseQuery += fmt.Sprintf(" AND (name ILIKE $%d OR description ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+filter.Keyword+"%")
		argIdx++
	}

	// 获取总数
	var total int64
	countQuery := "SELECT COUNT(*) " + baseQuery
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count rules")
	}

	// 排序
	orderBy := "updated_at"
	orderDir := "DESC"
	if filter.OrderBy != "" {
		orderBy = filter.OrderBy
	}
	if filter.OrderDir != "" {
		orderDir = filter.OrderDir
	}

	// 获取列表
	selectQuery := `
		SELECT rule_id, tenant_id, name, rule_type, engine, description,
			   conditions, labels, severity, enabled, priority, version, status,
			   created_by, updated_by, created_at, updated_at
	` + baseQuery + fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d", orderBy, orderDir, argIdx, argIdx+1)

	args = append(args, filter.Limit, filter.Offset)

	rules, _, err := r.queryRulesWithArgs(ctx, selectQuery, int(total), args...)
	return rules, total, err
}

// Delete 删除规则（硬删除）
func (r *RuleRepository) Delete(ctx context.Context, ruleID string) error {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.Delete")
	defer span.End()

	query := "DELETE FROM rules WHERE rule_id = $1"
	result, err := r.db.ExecContext(ctx, query, ruleID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to delete rule")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.Newf(errors.ErrCodeRuleNotFound, "rule not found: %s", ruleID)
	}

	r.logger.Info("Rule deleted", zap.String("rule_id", ruleID))
	return nil
}

// SoftDelete 软删除规则
func (r *RuleRepository) SoftDelete(ctx context.Context, ruleID string) error {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.SoftDelete")
	defer span.End()

	query := `UPDATE rules SET status = 'deleted', updated_at = $1 WHERE rule_id = $2 AND status != 'deleted'`
	result, err := r.db.ExecContext(ctx, query, time.Now(), ruleID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to soft delete rule")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.Newf(errors.ErrCodeRuleNotFound, "rule not found: %s", ruleID)
	}

	r.logger.Info("Rule soft deleted", zap.String("rule_id", ruleID))
	return nil
}

// SetEnabled 设置规则启用状态
func (r *RuleRepository) SetEnabled(ctx context.Context, ruleID string, enabled bool) error {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.SetEnabled")
	defer span.End()

	status := string(model.RuleStatusDisabled)
	if enabled {
		status = string(model.RuleStatusActive)
	}

	query := `UPDATE rules SET enabled = $1, status = $2, updated_at = $3 WHERE rule_id = $4 AND status != 'deleted'`
	result, err := r.db.ExecContext(ctx, query, enabled, status, time.Now(), ruleID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to update enabled status")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return errors.Newf(errors.ErrCodeRuleNotFound, "rule not found: %s", ruleID)
	}

	r.logger.Info("Rule enabled status updated", zap.String("rule_id", ruleID), zap.Bool("enabled", enabled))
	return nil
}

// =============================================================================
// 版本管理
// =============================================================================

// CreateVersion 创建规则版本记录
func (r *RuleRepository) CreateVersion(ctx context.Context, rule *model.Rule) error {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.CreateVersion")
	defer span.End()

	contentJSON, err := json.Marshal(rule)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal rule content")
	}

	versionID := fmt.Sprintf("%s-v%d", rule.RuleID, rule.Version)
	checksum := fmt.Sprintf("%x", md5Sum(contentJSON))

	query := `
		INSERT INTO rule_versions (rule_version, rule_id, tenant_id, version, content_uri, checksum, status, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (rule_version) DO NOTHING
	`

	_, err = r.db.ExecContext(ctx, query,
		versionID, rule.RuleID, rule.TenantID, rule.Version,
		fmt.Sprintf("inline:%s", string(contentJSON)),
		checksum, "active", rule.CreatedBy, time.Now(),
	)

	if err != nil {
		r.logger.Error("Failed to create rule version", zap.String("version_id", versionID), zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to insert rule version")
	}

	return nil
}

// GetVersions 获取规则版本列表
func (r *RuleRepository) GetVersions(ctx context.Context, ruleID string) ([]*model.RuleVersion, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.GetVersions")
	defer span.End()

	query := `
		SELECT rule_version, rule_id, tenant_id, version, content_uri, checksum, status, created_by, created_at
		FROM rule_versions
		WHERE rule_id = $1
		ORDER BY version DESC
	`

	rows, err := r.db.QueryContext(ctx, query, ruleID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query rule versions")
	}
	defer rows.Close()

	var versions []*model.RuleVersion
	for rows.Next() {
		var v model.RuleVersion
		var checksum sql.NullString
		var version sql.NullInt64

		err := rows.Scan(&v.RuleVersionID, &v.RuleID, &v.TenantID, &version, &v.ContentURI, &checksum, &v.Status, &v.CreatedBy, &v.CreatedAt)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan version")
		}

		if checksum.Valid {
			v.Checksum = checksum.String
		}
		if version.Valid {
			v.Version = version.Int64
		}

		versions = append(versions, &v)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "error iterating versions")
	}

	return versions, nil
}

// GetVersion 获取特定版本
func (r *RuleRepository) GetVersion(ctx context.Context, ruleID string, version int64) (*model.RuleVersion, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.GetVersion")
	defer span.End()

	versionID := fmt.Sprintf("%s-v%d", ruleID, version)

	query := `
		SELECT rule_version, rule_id, tenant_id, version, content_uri, checksum, status, created_by, created_at
		FROM rule_versions
		WHERE rule_version = $1
	`

	var v model.RuleVersion
	var checksum sql.NullString
	var versionNum sql.NullInt64

	err := r.db.QueryRowContext(ctx, query, versionID).Scan(
		&v.RuleVersionID, &v.RuleID, &v.TenantID, &versionNum, &v.ContentURI, &checksum, &v.Status, &v.CreatedBy, &v.CreatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.Newf(errors.ErrCodeRuleNotFound, "rule version not found: %s", versionID)
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to get rule version")
	}

	if checksum.Valid {
		v.Checksum = checksum.String
	}
	if versionNum.Valid {
		v.Version = versionNum.Int64
	}

	return &v, nil
}

// ListWorkbenchItems returns only tenant-scoped rows for the selected rule.
// A rule_id of "*" is a seeded default shared by rules in the same tenant;
// rule-specific rows take precedence through the stable ordering.
func (r *RuleRepository) ListWorkbenchItems(ctx context.Context, tenantID, ruleID string) ([]*model.RuleWorkbenchItem, error) {
	query := `
		SELECT item_id, tenant_id, rule_id, category, ordinal, payload, scenario_id, occurred_at
		FROM rule_workbench_items
		WHERE tenant_id = $1 AND rule_id IN ($2, '*')
		ORDER BY category, CASE WHEN rule_id = $2 THEN 0 ELSE 1 END, ordinal, item_id
	`
	rows, err := r.db.QueryContext(ctx, query, tenantID, ruleID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query rule workbench items")
	}
	defer rows.Close()

	items := make([]*model.RuleWorkbenchItem, 0)
	for rows.Next() {
		var item model.RuleWorkbenchItem
		if err := rows.Scan(&item.ItemID, &item.TenantID, &item.RuleID, &item.Category, &item.Ordinal, &item.Payload, &item.ScenarioID, &item.OccurredAt); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan rule workbench item")
		}
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to iterate rule workbench items")
	}
	return items, nil
}

// CreateWorkbenchAction persists both the action job and its audit row in one
// transaction. A successful response therefore cannot outlive a failed audit.
func (r *RuleRepository) CreateWorkbenchAction(ctx context.Context, job *model.RuleWorkbenchActionJob, ipAddr, userAgent string) error {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal rule workbench action payload")
	}
	return r.Transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO rule_action_jobs (
				job_id, action_id, tenant_id, rule_id, action, target, payload, status, requested_by, created_at
			)
			SELECT $1, $2, $3, rule_id::text, $5, $6, $7::jsonb, $8, $9, $10
			FROM rules
			WHERE rule_id::text = $4 AND tenant_id = $3 AND status != 'deleted'
		`, job.JobID, job.ActionID, job.TenantID, job.RuleID, job.Action, job.Target, string(payload), job.Status, job.RequestedBy, job.CreatedAt)
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to create rule action job")
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return errors.Newf(errors.ErrCodeRuleNotFound, "rule not found for tenant: %s", job.RuleID)
		}

		detail, err := json.Marshal(map[string]interface{}{
			"action_id": job.ActionID,
			"job_id":    job.JobID,
			"target":    job.Target,
			"status":    job.Status,
		})
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeSerializationError, "failed to marshal rule action audit detail")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, ip_addr, user_agent)
			VALUES ($1, $2, 'RULE_WORKBENCH_ACTION', 'rule', $3, $4::jsonb, $5, $6)
		`, job.TenantID, job.RequestedBy, job.RuleID, string(detail), ipAddr, userAgent); err != nil {
			return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to persist rule action audit")
		}
		return nil
	})
}

// =============================================================================
// 批量操作
// =============================================================================

// BatchCreate 批量创建规则
func (r *RuleRepository) BatchCreate(ctx context.Context, rules []*model.Rule) (int, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.BatchCreate")
	defer span.End()

	if len(rules) == 0 {
		return 0, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin transaction")
	}
	defer tx.Rollback()

	successCount := 0
	for _, rule := range rules {
		if rule.RuleID == "" {
			rule.RuleID = uuid.New().String()
		}
		if rule.Version == 0 {
			rule.Version = 1
		}
		if rule.CreatedAt.IsZero() {
			rule.CreatedAt = time.Now()
		}
		rule.UpdatedAt = time.Now()

		if err := rule.MarshalConditions(); err != nil {
			r.logger.Warn("Failed to marshal conditions for rule", zap.String("rule_id", rule.RuleID), zap.Error(err))
			continue
		}

		query := `
			INSERT INTO rules (
				rule_id, tenant_id, name, rule_type, engine, description,
				conditions, labels, severity, enabled, priority, version, status,
				created_by, updated_by, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		`

		_, err := tx.ExecContext(ctx, query,
			rule.RuleID, rule.TenantID, rule.Name, rule.Type, rule.Engine, rule.Description,
			rule.ConditionsJSON, pq.Array(rule.Labels), rule.Severity, rule.Enabled,
			rule.Priority, rule.Version, rule.Status, rule.CreatedBy, rule.CreatedBy,
			rule.CreatedAt, rule.UpdatedAt,
		)

		if err != nil {
			r.logger.Warn("Failed to insert rule in batch", zap.String("rule_id", rule.RuleID), zap.Error(err))
			continue
		}
		successCount++
	}

	if err := tx.Commit(); err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit transaction")
	}

	r.logger.Info("Batch created rules", zap.Int("total", len(rules)), zap.Int("success", successCount))
	return successCount, nil
}

// BatchSetEnabled 批量设置启用状态
func (r *RuleRepository) BatchSetEnabled(ctx context.Context, ruleIDs []string, enabled bool) (int, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.BatchSetEnabled")
	defer span.End()

	if len(ruleIDs) == 0 {
		return 0, nil
	}

	status := string(model.RuleStatusDisabled)
	if enabled {
		status = string(model.RuleStatusActive)
	}

	query := `UPDATE rules SET enabled = $1, status = $2, updated_at = $3 WHERE rule_id = ANY($4) AND status != 'deleted'`
	result, err := r.db.ExecContext(ctx, query, enabled, status, time.Now(), pq.Array(ruleIDs))
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to batch update enabled status")
	}

	rowsAffected, _ := result.RowsAffected()
	return int(rowsAffected), nil
}

// BatchDelete 批量删除规则
func (r *RuleRepository) BatchDelete(ctx context.Context, ruleIDs []string) (int, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.BatchDelete")
	defer span.End()

	if len(ruleIDs) == 0 {
		return 0, nil
	}

	query := `DELETE FROM rules WHERE rule_id = ANY($1)`
	result, err := r.db.ExecContext(ctx, query, pq.Array(ruleIDs))
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to batch delete rules")
	}

	rowsAffected, _ := result.RowsAffected()
	return int(rowsAffected), nil
}

// =============================================================================
// 统计方法
// =============================================================================

// CountByTenant 统计租户规则数量
func (r *RuleRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.CountByTenant")
	defer span.End()

	var count int64
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM rules WHERE tenant_id = $1 AND status != 'deleted'", tenantID).Scan(&count)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count rules")
	}
	return count, nil
}

// CountByType 统计按类型分组的规则数量
func (r *RuleRepository) CountByType(ctx context.Context, tenantID string) (map[string]int64, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.CountByType")
	defer span.End()

	query := `SELECT rule_type, COUNT(*) FROM rules WHERE tenant_id = $1 AND status != 'deleted' AND status != 'archived' GROUP BY rule_type`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count rules by type")
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var ruleType string
		var count int64
		if err := rows.Scan(&ruleType, &count); err != nil {
			return nil, err
		}
		result[ruleType] = count
	}

	return result, nil
}

// CountByStatus 统计按状态分组的规则数量
func (r *RuleRepository) CountByStatus(ctx context.Context, tenantID string) (map[string]int64, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.CountByStatus")
	defer span.End()

	query := `SELECT enabled, COUNT(*) FROM rules WHERE tenant_id = $1 AND status != 'deleted' GROUP BY enabled`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count rules by status")
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var enabled bool
		var count int64
		if err := rows.Scan(&enabled, &count); err != nil {
			return nil, err
		}
		if enabled {
			result["enabled"] = count
		} else {
			result["disabled"] = count
		}
	}

	return result, nil
}

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

// GetStats 获取规则统计
func (r *RuleRepository) GetStats(ctx context.Context, tenantID string) (*RuleStats, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.GetStats")
	defer span.End()

	stats := &RuleStats{
		ByType:     make(map[string]int64),
		ByEngine:   make(map[string]int64),
		BySeverity: make(map[string]int64),
	}

	// 总数
	total, err := r.CountByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	stats.Total = total

	// 按状态统计
	byStatus, err := r.CountByStatus(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	stats.Enabled = byStatus["enabled"]
	stats.Disabled = byStatus["disabled"]

	// 按类型统计
	byType, err := r.CountByType(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	stats.ByType = byType

	// 按引擎统计
	query := `SELECT engine, COUNT(*) FROM rules WHERE tenant_id = $1 AND status != 'deleted' GROUP BY engine`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var engine string
		var count int64
		if err := rows.Scan(&engine, &count); err != nil {
			return nil, err
		}
		stats.ByEngine[engine] = count
	}

	// 按严重程度统计
	query = `SELECT severity, COUNT(*) FROM rules WHERE tenant_id = $1 AND status != 'deleted' GROUP BY severity`
	rows, err = r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var severity string
		var count int64
		if err := rows.Scan(&severity, &count); err != nil {
			return nil, err
		}
		stats.BySeverity[severity] = count
	}

	// 最近7天新增
	query = `SELECT COUNT(*) FROM rules WHERE tenant_id = $1 AND status != 'deleted' AND created_at > NOW() - INTERVAL '7 days'`
	if err := r.db.QueryRowContext(ctx, query, tenantID).Scan(&stats.RecentAdded); err != nil {
		r.logger.Warn("Failed to count recent rules", zap.Error(err))
	}

	return stats, nil
}

// =============================================================================
// 辅助方法
// =============================================================================

// RuleFilter 规则过滤条件
type RuleFilter struct {
	Type     string
	Engine   string
	Severity string
	Enabled  *bool
	Labels   []string
	Keyword  string
	Limit    int
	Offset   int
	OrderBy  string
	OrderDir string
}

// queryRulesWithArgs 使用参数查询规则列表
func (r *RuleRepository) queryRulesWithArgs(ctx context.Context, query string, total int, args ...interface{}) ([]*model.Rule, int, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query rules")
	}
	defer rows.Close()

	return r.scanRules(rows, total)
}

// scanRules 扫描规则结果
func (r *RuleRepository) scanRules(rows *sql.Rows, total int) ([]*model.Rule, int, error) {
	var rules []*model.Rule
	for rows.Next() {
		var rule model.Rule
		var labels pq.StringArray
		var updatedBy sql.NullString

		err := rows.Scan(
			&rule.RuleID, &rule.TenantID, &rule.Name, &rule.Type, &rule.Engine, &rule.Description,
			&rule.ConditionsJSON, &labels, &rule.Severity, &rule.Enabled, &rule.Priority,
			&rule.Version, &rule.Status, &rule.CreatedBy, &updatedBy, &rule.CreatedAt, &rule.UpdatedAt,
		)

		if err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to scan rule")
		}

		if err := rule.UnmarshalConditions(); err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeSerializationError, "failed to unmarshal conditions")
		}

		rule.Labels = labels
		if updatedBy.Valid {
			rule.UpdatedBy = updatedBy.String
		}

		rules = append(rules, &rule)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "error iterating rules")
	}

	return rules, total, nil
}

// Transaction 执行事务
func (r *RuleRepository) Transaction(ctx context.Context, fn func(ctx context.Context, tx *sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to begin transaction")
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			r.logger.Error("Failed to rollback transaction", zap.Error(rbErr))
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to commit transaction")
	}

	return nil
}

// Ping 健康检查
func (r *RuleRepository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}

// Search 全文搜索规则
func (r *RuleRepository) Search(ctx context.Context, tenantID, keyword string, limit, offset int) ([]*model.Rule, int, error) {
	ctx, span := otel.StartSpan(ctx, "RuleRepository.Search")
	defer span.End()

	searchPattern := "%" + strings.ToLower(keyword) + "%"

	// 获取总数
	countQuery := `
		SELECT COUNT(*) FROM rules 
		WHERE tenant_id = $1 AND status != 'deleted' AND (LOWER(name) LIKE $2 OR LOWER(description) LIKE $2)
	`
	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, tenantID, searchPattern).Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to count search results")
	}

	// 获取结果
	query := `
		SELECT rule_id, tenant_id, name, rule_type, engine, description,
			   conditions, labels, severity, enabled, priority, version, status,
			   created_by, updated_by, created_at, updated_at
		FROM rules
		WHERE tenant_id = $1 AND status != 'deleted' AND (LOWER(name) LIKE $2 OR LOWER(description) LIKE $2)
		ORDER BY updated_at DESC
		LIMIT $3 OFFSET $4
	`

	rules, _, err := r.queryRulesWithArgs(ctx, query, total, tenantID, searchPattern, limit, offset)
	return rules, total, err
}

// md5Sum 计算 MD5
func md5Sum(data []byte) []byte {
	h := md5.New()
	h.Write(data)
	return h.Sum(nil)
}
