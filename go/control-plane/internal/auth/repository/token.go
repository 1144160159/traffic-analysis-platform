////////////////////////////////////////////////////////////////////////////////
// FILE: control-plane/internal/auth/repository/token.go
// 完整修复版 v2：
// 修复内容：
// 1. 修复 #20：CleanupExpiredSessions 增加分批处理，避免锁表
// 2. 修复 #24：ValidateToken 增加 Redis 缓存优化
// 3. 修复 #25：GetTokensNeedingRotation 性能优化（需配合数据库迁移）
// 4. 新增 GetDB() 方法供事务使用
// 5. 完整的 CRUD + 轮转 + 统计方法（1000+ 行完整代码）
////////////////////////////////////////////////////////////////////////////////

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// TokenRepository Token 仓储（完整版）
type TokenRepository struct {
	db          *sql.DB
	redisClient *storage.RedisClient // 修复 #24：增加 Redis 缓存
	logger      *zap.Logger
}

type tokenSQLExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

// NewTokenRepository 创建 Token 仓储（修复 #24：支持 Redis）
func NewTokenRepository(db *sql.DB, logger *zap.Logger) *TokenRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &TokenRepository{
		db:     db,
		logger: logger,
	}
}

// WithRedis 设置 Redis 客户端（可选，用于缓存优化）
func (r *TokenRepository) WithRedis(redisClient *storage.RedisClient) *TokenRepository {
	r.redisClient = redisClient
	return r
}

// GetDB 获取数据库连接（修复 #29：供事务使用）
func (r *TokenRepository) GetDB() *sql.DB {
	return r.db
}

// ==================== 核心 CRUD 方法 ====================

// Create 创建 Token（返回完整对象，但不包含明文 token）
func (r *TokenRepository) Create(ctx context.Context, token *model.APIToken) error {
	if err := prepareTokenForInsert(token); err != nil {
		return err
	}
	if err := insertToken(ctx, r.db, token); err != nil {
		r.logger.Error("Failed to create token", zap.String("tenant_id", token.TenantID), zap.String("name", token.Name), zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to create token")
	}
	r.logger.Info("Token created", zap.String("token_id", token.TokenID.String()), zap.String("tenant_id", token.TenantID), zap.String("name", token.Name))
	return nil
}

func prepareTokenForInsert(token *model.APIToken) error {
	if token == nil {
		return errors.New(errors.ErrCodeInvalidParameter, "token cannot be nil")
	}

	// 验证必填字段
	if token.TenantID == "" {
		return errors.New(errors.ErrCodeMissingParameter, "tenant_id is required")
	}
	if token.Name == "" {
		return errors.New(errors.ErrCodeMissingParameter, "name is required")
	}
	if token.TokenHash == "" {
		return errors.New(errors.ErrCodeMissingParameter, "token_hash is required")
	}
	if len(token.Scopes) == 0 {
		return errors.New(errors.ErrCodeMissingParameter, "scopes is required")
	}

	// 生成 UUID（如果未提供）
	if token.TokenID == uuid.Nil {
		token.TokenID = uuid.New()
	}

	// 设置默认值
	now := time.Now()
	token.CreatedAt = now
	token.UpdatedAt = now
	if token.Status == "" {
		token.Status = model.TokenStatusActive
	}
	if token.TokenType == "" {
		token.TokenType = model.TokenTypeAPI
	}
	return nil
}

func insertToken(ctx context.Context, exec tokenSQLExecutor, token *model.APIToken) error {
	query := `
		INSERT INTO api_tokens (
			token_id, tenant_id, user_id, name, description, token_type,
			token_hash, token_prefix, scopes, status, expires_at,
			last_used_at, usage_count, created_by, created_at, updated_at,
			revoked_at, rotation_enabled, rotation_interval, last_rotated_at,
			previous_token_id, ip_whitelist, metadata, probe_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21, $22, $23, $24
		)
	`

	_, err := exec.ExecContext(ctx, query,
		token.TokenID,
		token.TenantID,
		token.UserID,
		token.Name,
		token.Description,
		token.TokenType,
		token.TokenHash,
		token.TokenPrefix,
		token.Scopes,
		token.Status,
		token.ExpiresAt,
		token.LastUsedAt,
		token.UsageCount,
		token.CreatedBy,
		token.CreatedAt,
		token.UpdatedAt,
		token.RevokedAt,
		token.RotationEnabled,
		token.RotationInterval,
		token.LastRotatedAt,
		token.PreviousTokenID,
		token.IPWhitelist,
		token.Metadata,
		token.ProbeID,
	)

	return err
}

func insertTokenAudit(ctx context.Context, exec tokenSQLExecutor, tenantID string, userID interface{}, action, objectID string, detail map[string]interface{}) error {
	if detail == nil {
		detail = map[string]interface{}{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `
		INSERT INTO audit_logs (tenant_id, user_id, action, object_type, object_id, detail, created_at)
		VALUES ($1, $2, $3, 'api_token', $4, $5::jsonb, NOW())
	`, tenantID, userID, action, objectID, string(raw))
	return err
}

// CreateWithAudit makes the credential and its durable audit record one commit.
func (r *TokenRepository) CreateWithAudit(ctx context.Context, token *model.APIToken, actor uuid.UUID) error {
	if err := prepareTokenForInsert(token); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to start token transaction")
	}
	defer func() { _ = tx.Rollback() }()
	if err = insertToken(ctx, tx, token); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to create token")
	}
	if err = insertTokenAudit(ctx, tx, token.TenantID, actor.String(), "create_token", token.TokenID.String(), map[string]interface{}{"name": token.Name, "scopes": token.Scopes, "probe_id": token.ProbeID}); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to audit token creation")
	}
	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to commit token creation")
	}
	return nil
}

// UpdateWithAudit updates token metadata and its audit row atomically.
func (r *TokenRepository) UpdateWithAudit(ctx context.Context, token *model.APIToken, actor uuid.UUID, oldValue, newValue map[string]interface{}) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to start token transaction")
	}
	defer func() { _ = tx.Rollback() }()
	token.UpdatedAt = time.Now()
	result, err := tx.ExecContext(ctx, `UPDATE api_tokens SET name=$2, description=$3, scopes=$4, expires_at=$5, rotation_enabled=$6, rotation_interval=$7, ip_whitelist=$8, metadata=$9, updated_at=$10, status=$11 WHERE token_id=$1 AND tenant_id=$12`, token.TokenID, token.Name, token.Description, token.Scopes, token.ExpiresAt, token.RotationEnabled, token.RotationInterval, token.IPWhitelist, token.Metadata, token.UpdatedAt, token.Status, token.TenantID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to update token")
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}
	if err = insertTokenAudit(ctx, tx, token.TenantID, actor.String(), "update_token", token.TokenID.String(), map[string]interface{}{"old_value": oldValue, "new_value": newValue}); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to audit token update")
	}
	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to commit token update")
	}
	return nil
}

func (r *TokenRepository) RevokeWithAudit(ctx context.Context, tenantID string, tokenID uuid.UUID, actor uuid.UUID, name, reason string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to start token transaction")
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE api_tokens SET status='revoked', revoked_at=NOW(), updated_at=NOW() WHERE token_id=$1 AND tenant_id=$2 AND status='active'`, tokenID, tenantID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to revoke token")
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found or already revoked")
	}
	if err = insertTokenAudit(ctx, tx, tenantID, actor.String(), "revoke_token", tokenID.String(), map[string]interface{}{"name": name, "reason": reason}); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to audit token revocation")
	}
	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to commit token revocation")
	}
	return nil
}

func (r *TokenRepository) DeleteWithAudit(ctx context.Context, tenantID string, tokenID uuid.UUID, actor uuid.UUID, name string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to start token transaction")
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM api_tokens WHERE token_id=$1 AND tenant_id=$2`, tokenID, tenantID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to delete token")
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}
	if err = insertTokenAudit(ctx, tx, tenantID, actor.String(), "delete_token", tokenID.String(), map[string]interface{}{"name": name}); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to audit token deletion")
	}
	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to commit token deletion")
	}
	return nil
}

func (r *TokenRepository) UpdateScopesWithAudit(ctx context.Context, tenantID string, tokenID uuid.UUID, scopes, oldScopes []string, actor uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to start token transaction")
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE api_tokens SET scopes=$3, updated_at=NOW() WHERE token_id=$1 AND tenant_id=$2`, tokenID, tenantID, model.StringSlice(scopes))
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to update token scopes")
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}
	if err = insertTokenAudit(ctx, tx, tenantID, actor.String(), "update_token_scopes", tokenID.String(), map[string]interface{}{"old_value": oldScopes, "new_value": scopes}); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to audit token scopes")
	}
	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to commit token scopes")
	}
	return nil
}

func (r *TokenRepository) RotateWithAudit(ctx context.Context, tenantID string, oldTokenID uuid.UUID, newToken *model.APIToken, actor uuid.UUID) error {
	if err := prepareTokenForInsert(newToken); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to start token rotation")
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `UPDATE api_tokens SET status='revoked', revoked_at=NOW(), updated_at=NOW() WHERE token_id=$1 AND tenant_id=$2 AND status='active'`, oldTokenID, tenantID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to revoke old token")
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found or already revoked")
	}
	newToken.PreviousTokenID = &oldTokenID
	if err = insertToken(ctx, tx, newToken); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to create rotated token")
	}
	if err = insertTokenAudit(ctx, tx, tenantID, actor.String(), "regenerate_token", newToken.TokenID.String(), map[string]interface{}{"old_token_id": oldTokenID.String(), "new_token_id": newToken.TokenID.String(), "scopes": newToken.Scopes}); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to audit token rotation")
	}
	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to commit token rotation")
	}
	return nil
}

// InsertAuditLog 同步写入 token 管理审计行，供 live 闭环直接核对 audit_logs。
func (r *TokenRepository) InsertAuditLog(
	ctx context.Context,
	tenantID string,
	userID string,
	action string,
	objectType string,
	objectID string,
	detail map[string]interface{},
) error {
	if detail == nil {
		detail = map[string]interface{}{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeSerializationError, "Failed to marshal audit detail")
	}

	query := `
		INSERT INTO audit_logs (
			tenant_id, user_id, action, object_type, object_id, detail, created_at
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb, NOW())
	`
	_, err = r.db.ExecContext(ctx, query, tenantID, userID, action, objectType, objectID, string(detailJSON))
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to insert audit log")
	}
	return nil
}

// GetByID 根据 ID 获取 Token
func (r *TokenRepository) GetByID(ctx context.Context, tokenID uuid.UUID) (*model.APIToken, error) {
	query := `
		SELECT 
			token_id, tenant_id, user_id, name, description, token_type,
			token_hash, token_prefix, scopes, status, expires_at,
			last_used_at, usage_count, created_by, created_at, updated_at,
			revoked_at, rotation_enabled, rotation_interval, last_rotated_at,
			previous_token_id, ip_whitelist, metadata, probe_id
		FROM api_tokens
		WHERE token_id = $1
	`

	token := &model.APIToken{}
	err := r.db.QueryRowContext(ctx, query, tokenID).Scan(
		&token.TokenID,
		&token.TenantID,
		&token.UserID,
		&token.Name,
		&token.Description,
		&token.TokenType,
		&token.TokenHash,
		&token.TokenPrefix,
		&token.Scopes,
		&token.Status,
		&token.ExpiresAt,
		&token.LastUsedAt,
		&token.UsageCount,
		&token.CreatedBy,
		&token.CreatedAt,
		&token.UpdatedAt,
		&token.RevokedAt,
		&token.RotationEnabled,
		&token.RotationInterval,
		&token.LastRotatedAt,
		&token.PreviousTokenID,
		&token.IPWhitelist,
		&token.Metadata,
		&token.ProbeID,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		r.logger.Error("Failed to get token by ID",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token")
	}

	return token, nil
}

// GetByHash 根据 token hash 获取 Token（用于验证）
func (r *TokenRepository) GetByHash(ctx context.Context, tokenHash string) (*model.APIToken, error) {
	query := `
		SELECT 
			token_id, tenant_id, user_id, name, description, token_type,
			token_hash, token_prefix, scopes, status, expires_at,
			last_used_at, usage_count, created_by, created_at, updated_at,
			revoked_at, rotation_enabled, rotation_interval, last_rotated_at,
			previous_token_id, ip_whitelist, metadata, probe_id
		FROM api_tokens
		WHERE token_hash = $1 AND status = 'active'
	`

	token := &model.APIToken{}
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&token.TokenID,
		&token.TenantID,
		&token.UserID,
		&token.Name,
		&token.Description,
		&token.TokenType,
		&token.TokenHash,
		&token.TokenPrefix,
		&token.Scopes,
		&token.Status,
		&token.ExpiresAt,
		&token.LastUsedAt,
		&token.UsageCount,
		&token.CreatedBy,
		&token.CreatedAt,
		&token.UpdatedAt,
		&token.RevokedAt,
		&token.RotationEnabled,
		&token.RotationInterval,
		&token.LastRotatedAt,
		&token.PreviousTokenID,
		&token.IPWhitelist,
		&token.Metadata,
		&token.ProbeID,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		r.logger.Error("Failed to get token by hash", zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token by hash")
	}

	return token, nil
}

// Update 更新 Token
func (r *TokenRepository) Update(ctx context.Context, token *model.APIToken) error {
	if token == nil {
		return errors.New(errors.ErrCodeInvalidParameter, "token cannot be nil")
	}

	token.UpdatedAt = time.Now()

	query := `
		UPDATE api_tokens
		SET 
			name = $2,
			description = $3,
			scopes = $4,
			expires_at = $5,
			rotation_enabled = $6,
			rotation_interval = $7,
			ip_whitelist = $8,
			metadata = $9,
			updated_at = $10,
			status = $11
		WHERE token_id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		token.TokenID,
		token.Name,
		token.Description,
		token.Scopes,
		token.ExpiresAt,
		token.RotationEnabled,
		token.RotationInterval,
		token.IPWhitelist,
		token.Metadata,
		token.UpdatedAt,
		token.Status,
	)

	if err != nil {
		r.logger.Error("Failed to update token",
			zap.String("token_id", token.TokenID.String()),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to update token")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	r.logger.Info("Token updated",
		zap.String("token_id", token.TokenID.String()),
		zap.String("name", token.Name))

	return nil
}

// Revoke 撤销 Token
func (r *TokenRepository) Revoke(ctx context.Context, tokenID uuid.UUID, reason string) error {
	query := `
		UPDATE api_tokens
		SET 
			status = 'revoked',
			revoked_at = NOW(),
			updated_at = NOW()
		WHERE token_id = $1 AND status = 'active'
	`

	result, err := r.db.ExecContext(ctx, query, tokenID)
	if err != nil {
		r.logger.Error("Failed to revoke token",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to revoke token")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found or already revoked")
	}

	r.logger.Info("Token revoked",
		zap.String("token_id", tokenID.String()),
		zap.String("reason", reason))

	return nil
}

// Delete 删除 Token（物理删除）
func (r *TokenRepository) Delete(ctx context.Context, tokenID uuid.UUID) error {
	query := `DELETE FROM api_tokens WHERE token_id = $1`

	result, err := r.db.ExecContext(ctx, query, tokenID)
	if err != nil {
		r.logger.Error("Failed to delete token",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to delete token")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	r.logger.Info("Token deleted",
		zap.String("token_id", tokenID.String()))

	return nil
}

// ==================== 查询方法 ====================

// ListByTenant 列出租户的所有 Token
func (r *TokenRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*model.APIToken, int64, error) {
	// 设置默认值和上限
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// 获取总数
	countQuery := `SELECT COUNT(*) FROM api_tokens WHERE tenant_id = $1`
	var total int64
	err := r.db.QueryRowContext(ctx, countQuery, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to count tokens")
	}

	// 获取列表
	query := `
		SELECT 
			token_id, tenant_id, user_id, name, description, token_type,
			token_hash, token_prefix, scopes, status, expires_at,
			last_used_at, usage_count, created_by, created_at, updated_at,
			revoked_at, rotation_enabled, rotation_interval, last_rotated_at,
			previous_token_id, ip_whitelist, metadata, probe_id
		FROM api_tokens
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query tokens")
	}
	defer rows.Close()

	var tokens []*model.APIToken
	for rows.Next() {
		token := &model.APIToken{}
		err := rows.Scan(
			&token.TokenID,
			&token.TenantID,
			&token.UserID,
			&token.Name,
			&token.Description,
			&token.TokenType,
			&token.TokenHash,
			&token.TokenPrefix,
			&token.Scopes,
			&token.Status,
			&token.ExpiresAt,
			&token.LastUsedAt,
			&token.UsageCount,
			&token.CreatedBy,
			&token.CreatedAt,
			&token.UpdatedAt,
			&token.RevokedAt,
			&token.RotationEnabled,
			&token.RotationInterval,
			&token.LastRotatedAt,
			&token.PreviousTokenID,
			&token.IPWhitelist,
			&token.Metadata,
			&token.ProbeID,
		)
		if err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to scan token")
		}
		tokens = append(tokens, token)
	}

	return tokens, total, nil
}

// GetActiveTokensByTenant 获取租户的活跃 Token 数量
func (r *TokenRepository) GetActiveTokensByTenant(ctx context.Context, tenantID string) (int64, error) {
	query := `
		SELECT COUNT(*)
		FROM api_tokens
		WHERE tenant_id = $1 
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > NOW())
	`

	var count int64
	err := r.db.QueryRowContext(ctx, query, tenantID).Scan(&count)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to count active tokens")
	}

	return count, nil
}

// CountByTenant 统计租户的 Token 数量（所有状态）
func (r *TokenRepository) CountByTenant(ctx context.Context, tenantID string) (int64, error) {
	query := `SELECT COUNT(*) FROM api_tokens WHERE tenant_id = $1`

	var count int64
	err := r.db.QueryRowContext(ctx, query, tenantID).Scan(&count)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to count tokens")
	}

	return count, nil
}

// ValidateToken 验证 Token（修复 #24：增加 Redis 缓存）
func (r *TokenRepository) ValidateToken(ctx context.Context, tokenHash string) (*model.APIToken, error) {
	if tokenHash == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "token_hash is required")
	}

	// 修复 #24：尝试从 Redis 缓存获取
	if r.redisClient != nil {
		cacheKey := "token_valid:" + tokenHash[:16] // 使用 hash 前缀作为 key
		cachedTokenID, err := r.redisClient.Client().Get(ctx, cacheKey).Result()
		if err == nil && cachedTokenID != "" {
			// 缓存命中，获取完整 Token
			tokenID, err := uuid.Parse(cachedTokenID)
			if err == nil {
				token, err := r.GetByID(ctx, tokenID)
				if err == nil && token != nil && token.Status == model.TokenStatusActive {
					r.logger.Debug("Token validation cache hit",
						zap.String("token_id", tokenID.String()))
					return token, nil
				}
			}
		}
	}

	// 缓存未命中，查询数据库
	token, err := r.getTokenByHashIncludingGrace(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, errors.New(errors.ErrCodeTokenInvalid, "Token not found")
	}

	// 检查状态
	if token.Status != model.TokenStatusActive {
		return nil, errors.Newf(errors.ErrCodeTokenInvalid, "Token is %s", token.Status)
	}

	// 检查过期
	if token.IsExpired() {
		// 自动标记为过期（最佳努力）
		_ = r.Revoke(ctx, token.TokenID, "expired")
		return nil, errors.New(errors.ErrCodeTokenExpired, "Token has expired")
	}

	// 修复 #24：写入 Redis 缓存
	if r.redisClient != nil {
		cacheKey := "token_valid:" + tokenHash[:16]
		ttl := 5 * time.Minute
		if token.ExpiresAt != nil {
			remaining := time.Until(*token.ExpiresAt)
			if remaining < ttl {
				ttl = remaining
			}
		}
		_ = r.redisClient.Client().Set(ctx, cacheKey, token.TokenID.String(), ttl).Err()
	}

	return token, nil
}

// getTokenByHashIncludingGrace 先按当前 token_hash 查找；如果未命中则尝试按轮转历史 old_token_hash 在宽限期内查找。
func (r *TokenRepository) getTokenByHashIncludingGrace(ctx context.Context, tokenHash string) (*model.APIToken, error) {
	// 1) current hash
	token, err := r.GetByHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if token != nil {
		return token, nil
	}

	// 2) grace period old hash -> token_id
	var tokenID uuid.UUID
	lookup := `
		SELECT token_id
		FROM token_rotation_history
		WHERE old_token_hash = $1
		  AND grace_period_ends > NOW()
		ORDER BY rotated_at DESC
		LIMIT 1
	`
	if err := r.db.QueryRowContext(ctx, lookup, tokenHash).Scan(&tokenID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to lookup token by rotation history")
	}

	return r.GetByID(ctx, tokenID)
}

// UpdateUsageStats 更新使用统计
func (r *TokenRepository) UpdateUsageStats(ctx context.Context, tokenID uuid.UUID) error {
	query := `
		UPDATE api_tokens
		SET 
			usage_count = usage_count + 1,
			last_used_at = NOW(),
			updated_at = NOW()
		WHERE token_id = $1
	`

	_, err := r.db.ExecContext(ctx, query, tokenID)
	if err != nil {
		r.logger.Warn("Failed to update usage stats",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		// 不返回错误，避免影响主流程
	}

	return nil
}

// UpdateScopes 更新 Token 权限
func (r *TokenRepository) UpdateScopes(ctx context.Context, tokenID uuid.UUID, scopes []string) error {
	query := `
		UPDATE api_tokens
		SET 
			scopes = $2,
			updated_at = NOW()
		WHERE token_id = $1
	`

	result, err := r.db.ExecContext(ctx, query, tokenID, model.StringSlice(scopes))
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to update scopes")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	r.logger.Info("Token scopes updated",
		zap.String("token_id", tokenID.String()),
		zap.Strings("scopes", scopes))

	return nil
}

// ==================== 轮转相关方法 ====================

// GetTokensNeedingRotation 获取需要轮转的 token（修复 #25：性能优化）
func (r *TokenRepository) GetTokensNeedingRotation(ctx context.Context, limit int) ([]*model.APIToken, error) {
	if limit <= 0 {
		limit = 100
	}

	// 修复 #25：优化查询（如果有 next_rotation_at 字段）
	// 注意：这需要数据库迁移添加 next_rotation_at 字段和触发器
	// 如果数据库未迁移，使用原始查询

	// 尝试使用优化查询
	optimizedQuery := `
		SELECT 
			token_id, tenant_id, user_id, name, description, token_type,
			token_hash, token_prefix, scopes, status, expires_at,
			last_used_at, usage_count, created_by, created_at, updated_at,
			revoked_at, rotation_enabled, rotation_interval, last_rotated_at,
			previous_token_id, ip_whitelist, metadata, probe_id
		FROM api_tokens
		WHERE rotation_enabled = TRUE
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > NOW())
		  AND next_rotation_at IS NOT NULL
		  AND next_rotation_at < NOW()
		ORDER BY next_rotation_at ASC
		LIMIT $1
	`

	tokens, err := r.queryTokens(ctx, optimizedQuery, limit)
	if err == nil {
		return tokens, nil
	}

	// 降级：使用原始查询（兼容未迁移的数据库）
	r.logger.Debug("Falling back to legacy rotation query (next_rotation_at column not available)")

	legacyQuery := `
		SELECT 
			token_id, tenant_id, user_id, name, description, token_type,
			token_hash, token_prefix, scopes, status, expires_at,
			last_used_at, usage_count, created_by, created_at, updated_at,
			revoked_at, rotation_enabled, rotation_interval, last_rotated_at,
			previous_token_id, ip_whitelist, metadata, probe_id
		FROM api_tokens
		WHERE rotation_enabled = TRUE
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > NOW())
		  AND rotation_interval IS NOT NULL
		  AND (
			  (last_rotated_at IS NULL AND created_at < NOW() - (rotation_interval || ' days')::INTERVAL)
			  OR
			  (last_rotated_at IS NOT NULL AND last_rotated_at < NOW() - (rotation_interval || ' days')::INTERVAL)
		  )
		ORDER BY created_at ASC
		LIMIT $1
	`

	return r.queryTokens(ctx, legacyQuery, limit)
}

// queryTokens 查询 token 的通用方法
func (r *TokenRepository) queryTokens(ctx context.Context, query string, limit int) ([]*model.APIToken, error) {
	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query tokens needing rotation")
	}
	defer rows.Close()

	var tokens []*model.APIToken
	for rows.Next() {
		token := &model.APIToken{}
		err := rows.Scan(
			&token.TokenID,
			&token.TenantID,
			&token.UserID,
			&token.Name,
			&token.Description,
			&token.TokenType,
			&token.TokenHash,
			&token.TokenPrefix,
			&token.Scopes,
			&token.Status,
			&token.ExpiresAt,
			&token.LastUsedAt,
			&token.UsageCount,
			&token.CreatedBy,
			&token.CreatedAt,
			&token.UpdatedAt,
			&token.RevokedAt,
			&token.RotationEnabled,
			&token.RotationInterval,
			&token.LastRotatedAt,
			&token.PreviousTokenID,
			&token.IPWhitelist,
			&token.Metadata,
			&token.ProbeID,
		)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to scan token")
		}
		tokens = append(tokens, token)
	}

	return tokens, nil
}

// SaveRotationHistory 保存轮转历史
func (r *TokenRepository) SaveRotationHistory(ctx context.Context, history *model.TokenRotationHistory) error {
	if history == nil {
		return errors.New(errors.ErrCodeInvalidParameter, "history cannot be nil")
	}

	// 生成 UUID（如果未提供）
	if history.ID == uuid.Nil {
		history.ID = uuid.New()
	}
	if history.RotatedAt.IsZero() {
		history.RotatedAt = time.Now()
	}

	query := `
		INSERT INTO token_rotation_history (
			id, token_id, old_token_hash, new_token_hash,
			rotated_at, rotated_by, reason, grace_period_ends
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.ExecContext(ctx, query,
		history.ID,
		history.TokenID,
		history.OldTokenHash,
		history.NewTokenHash,
		history.RotatedAt,
		history.RotatedBy,
		history.Reason,
		history.GracePeriodEnds,
	)

	if err != nil {
		r.logger.Error("Failed to save rotation history",
			zap.String("token_id", history.TokenID.String()),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to save rotation history")
	}

	r.logger.Info("Rotation history saved",
		zap.String("token_id", history.TokenID.String()),
		zap.String("reason", history.Reason))

	return nil
}

// GetRotationHistory 获取轮转历史
func (r *TokenRepository) GetRotationHistory(ctx context.Context, tokenID uuid.UUID, limit int) ([]*model.TokenRotationHistory, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT 
			id, token_id, old_token_hash, new_token_hash,
			rotated_at, rotated_by, reason, grace_period_ends
		FROM token_rotation_history
		WHERE token_id = $1
		ORDER BY rotated_at DESC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, tokenID, limit)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query rotation history")
	}
	defer rows.Close()

	var history []*model.TokenRotationHistory
	for rows.Next() {
		h := &model.TokenRotationHistory{}
		err := rows.Scan(
			&h.ID,
			&h.TokenID,
			&h.OldTokenHash,
			&h.NewTokenHash,
			&h.RotatedAt,
			&h.RotatedBy,
			&h.Reason,
			&h.GracePeriodEnds,
		)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to scan rotation history")
		}
		history = append(history, h)
	}

	return history, nil
}

// DeleteExpiredRotationHistory 删除过期的轮转历史
func (r *TokenRepository) DeleteExpiredRotationHistory(ctx context.Context, before time.Time) (int64, error) {
	query := `
		DELETE FROM token_rotation_history
		WHERE grace_period_ends < $1
	`

	result, err := r.db.ExecContext(ctx, query, before)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to delete expired rotation history")
	}

	deleted, _ := result.RowsAffected()

	if deleted > 0 {
		r.logger.Info("Deleted expired rotation history",
			zap.Int64("count", deleted))
	}

	return deleted, nil
}

// ==================== Token 使用日志方法 ====================

// SaveUsageLog 保存使用日志（可选功能）
func (r *TokenRepository) SaveUsageLog(ctx context.Context, log *model.TokenUsageLog) error {
	if log == nil {
		return errors.New(errors.ErrCodeInvalidParameter, "log cannot be nil")
	}

	if log.UsedAt.IsZero() {
		log.UsedAt = time.Now()
	}

	query := `
		INSERT INTO token_usage_logs (
			token_id, tenant_id, used_at, ip_addr, user_agent,
			endpoint, method, status_code, response_time_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.ExecContext(ctx, query,
		log.TokenID,
		log.TenantID,
		log.UsedAt,
		log.IPAddr,
		log.UserAgent,
		log.Endpoint,
		log.Method,
		log.StatusCode,
		log.ResponseTimeMs,
	)

	if err != nil {
		r.logger.Warn("Failed to save usage log",
			zap.String("token_id", log.TokenID.String()),
			zap.Error(err))
	}

	return nil
}

// GetUsageLogs 获取使用日志
func (r *TokenRepository) GetUsageLogs(ctx context.Context, tokenID uuid.UUID, limit int) ([]*model.TokenUsageLog, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT 
			id, token_id, tenant_id, used_at, ip_addr, user_agent,
			endpoint, method, status_code, response_time_ms
		FROM token_usage_logs
		WHERE token_id = $1
		ORDER BY used_at DESC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, tokenID, limit)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query usage logs")
	}
	defer rows.Close()

	var logs []*model.TokenUsageLog
	for rows.Next() {
		log := &model.TokenUsageLog{}
		err := rows.Scan(
			&log.ID,
			&log.TokenID,
			&log.TenantID,
			&log.UsedAt,
			&log.IPAddr,
			&log.UserAgent,
			&log.Endpoint,
			&log.Method,
			&log.StatusCode,
			&log.ResponseTimeMs,
		)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to scan usage log")
		}
		logs = append(logs, log)
	}

	return logs, nil
}

// ==================== 统计方法 ====================

// GetStatistics 获取 Token 统计信息
func (r *TokenRepository) GetStatistics(ctx context.Context, tenantID string) (*model.TokenStatistics, error) {
	query := `
		SELECT
			COUNT(*) as total_tokens,
			COUNT(*) FILTER (WHERE status = 'active' AND (expires_at IS NULL OR expires_at > NOW())) as active_tokens,
			COUNT(*) FILTER (WHERE expires_at IS NOT NULL AND expires_at <= NOW()) as expired_tokens,
			COUNT(*) FILTER (WHERE status = 'revoked') as revoked_tokens,
			COALESCE(SUM(usage_count), 0) as total_usage,
			MAX(created_at) as last_created_at,
			MAX(last_used_at) as last_used_at
		FROM api_tokens
		WHERE tenant_id = $1
	`

	stats := &model.TokenStatistics{
		TenantID: tenantID,
		ByType:   make(map[model.TokenType]int64),
	}

	err := r.db.QueryRowContext(ctx, query, tenantID).Scan(
		&stats.TotalTokens,
		&stats.ActiveTokens,
		&stats.ExpiredTokens,
		&stats.RevokedTokens,
		&stats.TotalUsage,
		&stats.LastCreatedAt,
		&stats.LastUsedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return stats, nil
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token statistics")
	}

	// 按类型统计
	typeQuery := `
		SELECT token_type, COUNT(*) as count
		FROM api_tokens
		WHERE tenant_id = $1
		GROUP BY token_type
	`

	rows, err := r.db.QueryContext(ctx, typeQuery, tenantID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token type statistics")
	}
	defer rows.Close()

	for rows.Next() {
		var tokenType model.TokenType
		var count int64
		if err := rows.Scan(&tokenType, &count); err != nil {
			continue
		}
		stats.ByType[tokenType] = count
	}

	return stats, nil
}

// ==================== 批量操作 ====================

// RevokeTokensByUser 撤销用户的所有 Token
func (r *TokenRepository) RevokeTokensByUser(ctx context.Context, userID uuid.UUID, reason string) (int64, error) {
	query := `
		UPDATE api_tokens
		SET 
			status = 'revoked',
			revoked_at = NOW(),
			updated_at = NOW()
		WHERE user_id = $1 AND status = 'active'
	`

	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to revoke user tokens")
	}

	revoked, _ := result.RowsAffected()

	if revoked > 0 {
		r.logger.Info("Revoked user tokens",
			zap.String("user_id", userID.String()),
			zap.Int64("count", revoked),
			zap.String("reason", reason))
	}

	return revoked, nil
}

// CleanupExpiredTokens deletes retained expired credentials only together with
// a durable per-token audit trail.
func (r *TokenRepository) CleanupExpiredTokens(ctx context.Context, before time.Time) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to start token cleanup")
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT token_id, tenant_id, name FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at < $1 AND status='expired' ORDER BY expires_at LIMIT 1000 FOR UPDATE SKIP LOCKED`, before)
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to select expired tokens")
	}
	type expiredToken struct {
		id             uuid.UUID
		tenantID, name string
	}
	items := make([]expiredToken, 0)
	for rows.Next() {
		var item expiredToken
		if err = rows.Scan(&item.id, &item.tenantID, &item.name); err != nil {
			_ = rows.Close()
			return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to scan expired token")
		}
		items = append(items, item)
	}
	if err = rows.Close(); err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to close expired token rows")
	}
	for _, item := range items {
		if err = insertTokenAudit(ctx, tx, item.tenantID, nil, "cleanup_expired_token", item.id.String(), map[string]interface{}{"name": item.name, "expired_before": before.UTC()}); err != nil {
			return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to audit expired token cleanup")
		}
		result, deleteErr := tx.ExecContext(ctx, `DELETE FROM api_tokens WHERE token_id=$1 AND tenant_id=$2 AND status='expired'`, item.id, item.tenantID)
		if deleteErr != nil {
			return 0, errors.Wrap(deleteErr, errors.ErrCodeDatabaseError, "Failed to cleanup expired token")
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return 0, errors.New(errors.ErrCodeVersionConflict, "expired token changed during cleanup")
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to commit token cleanup")
	}
	deleted := int64(len(items))

	if deleted > 0 {
		r.logger.Info("Cleaned up expired tokens",
			zap.Int64("count", deleted))
	}

	return deleted, nil
}

// ==================== Session 撤销（Redis fallback） ====================

// RevokeSession 撤销会话（写入 PostgreSQL）
func (r *TokenRepository) RevokeSession(ctx context.Context, session *model.RevokedSession) error {
	if session == nil {
		return errors.New(errors.ErrCodeInvalidParameter, "session cannot be nil")
	}

	if session.RevokedAt.IsZero() {
		session.RevokedAt = time.Now()
	}

	query := `
		INSERT INTO revoked_sessions (
			session_id, user_id, tenant_id, revoked_at, expires_at, reason
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (session_id) DO UPDATE SET
			revoked_at = EXCLUDED.revoked_at,
			reason = EXCLUDED.reason
	`

	_, err := r.db.ExecContext(ctx, query,
		session.SessionID,
		session.UserID,
		session.TenantID,
		session.RevokedAt,
		session.ExpiresAt,
		session.Reason,
	)

	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to revoke session")
	}

	return nil
}

// IsSessionRevoked 检查会话是否已撤销
func (r *TokenRepository) IsSessionRevoked(ctx context.Context, sessionID string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM revoked_sessions
			WHERE session_id = $1 AND expires_at > NOW()
		)
	`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, sessionID).Scan(&exists)
	if err != nil {
		return false, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to check session")
	}

	return exists, nil
}

// CleanupExpiredSessions 清理过期的撤销记录（修复 #20：分批处理）
func (r *TokenRepository) CleanupExpiredSessions(ctx context.Context, before time.Time) (int64, error) {
	const batchSize = 1000
	var totalDeleted int64

	for {
		// 修复 #20：使用 LIMIT 分批删除，避免锁表
		query := `
			DELETE FROM revoked_sessions
			WHERE session_id IN (
				SELECT session_id FROM revoked_sessions
				WHERE expires_at < $1
				LIMIT $2
			)
		`

		result, err := r.db.ExecContext(ctx, query, before, batchSize)
		if err != nil {
			return totalDeleted, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to cleanup expired sessions")
		}

		deleted, _ := result.RowsAffected()
		totalDeleted += deleted

		if deleted < batchSize {
			// 已删除完毕
			break
		}

		// 短暂休眠，避免持续锁表
		time.Sleep(100 * time.Millisecond)
	}

	if totalDeleted > 0 {
		r.logger.Info("Cleaned up expired sessions (batched)",
			zap.Int64("count", totalDeleted))
	}

	return totalDeleted, nil
}

// ==================== 高级查询 ====================

// GetTokensByType 按类型获取 Token 列表
func (r *TokenRepository) GetTokensByType(ctx context.Context, tenantID string, tokenType model.TokenType, includeRevoked bool, limit, offset int) ([]*model.APIToken, int64, error) {
	whereClause := "WHERE tenant_id = $1 AND token_type = $2"
	args := []interface{}{tenantID, tokenType}

	if !includeRevoked {
		whereClause += " AND status = 'active'"
	}

	countQuery := "SELECT COUNT(*) FROM api_tokens " + whereClause
	var total int64
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to count tokens")
	}

	query := `
		SELECT 
			token_id, tenant_id, user_id, name, description, token_type,
			token_hash, token_prefix, scopes, status, expires_at,
			last_used_at, usage_count, created_by, created_at, updated_at,
			revoked_at, rotation_enabled, rotation_interval, last_rotated_at,
			previous_token_id, ip_whitelist, metadata, probe_id
		FROM api_tokens
		` + whereClause + `
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query tokens")
	}
	defer rows.Close()

	var tokens []*model.APIToken
	for rows.Next() {
		token := &model.APIToken{}
		err := rows.Scan(
			&token.TokenID,
			&token.TenantID,
			&token.UserID,
			&token.Name,
			&token.Description,
			&token.TokenType,
			&token.TokenHash,
			&token.TokenPrefix,
			&token.Scopes,
			&token.Status,
			&token.ExpiresAt,
			&token.LastUsedAt,
			&token.UsageCount,
			&token.CreatedBy,
			&token.CreatedAt,
			&token.UpdatedAt,
			&token.RevokedAt,
			&token.RotationEnabled,
			&token.RotationInterval,
			&token.LastRotatedAt,
			&token.PreviousTokenID,
			&token.IPWhitelist,
			&token.Metadata,
			&token.ProbeID,
		)
		if err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to scan token")
		}
		tokens = append(tokens, token)
	}

	return tokens, total, nil
}

// GetTokensByProbeID 获取探针的 Token
func (r *TokenRepository) GetTokensByProbeID(ctx context.Context, tenantID, probeID string) ([]*model.APIToken, error) {
	query := `
		SELECT 
			token_id, tenant_id, user_id, name, description, token_type,
			token_hash, token_prefix, scopes, status, expires_at,
			last_used_at, usage_count, created_by, created_at, updated_at,
			revoked_at, rotation_enabled, rotation_interval, last_rotated_at,
			previous_token_id, ip_whitelist, metadata, probe_id
		FROM api_tokens
		WHERE tenant_id = $1 AND probe_id = $2
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID, probeID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query probe tokens")
	}
	defer rows.Close()

	var tokens []*model.APIToken
	for rows.Next() {
		token := &model.APIToken{}
		err := rows.Scan(
			&token.TokenID,
			&token.TenantID,
			&token.UserID,
			&token.Name,
			&token.Description,
			&token.TokenType,
			&token.TokenHash,
			&token.TokenPrefix,
			&token.Scopes,
			&token.Status,
			&token.ExpiresAt,
			&token.LastUsedAt,
			&token.UsageCount,
			&token.CreatedBy,
			&token.CreatedAt,
			&token.UpdatedAt,
			&token.RevokedAt,
			&token.RotationEnabled,
			&token.RotationInterval,
			&token.LastRotatedAt,
			&token.PreviousTokenID,
			&token.IPWhitelist,
			&token.Metadata,
			&token.ProbeID,
		)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to scan token")
		}
		tokens = append(tokens, token)
	}

	return tokens, nil
}

// ==================== 辅助方法 ====================

// ExistsByName 检查名称是否已存在
func (r *TokenRepository) ExistsByName(ctx context.Context, tenantID, name string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM api_tokens
			WHERE tenant_id = $1 AND name = $2
		)
	`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, tenantID, name).Scan(&exists)
	if err != nil {
		return false, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to check token name")
	}

	return exists, nil
}

// InvalidateCache 清除指定 token 的缓存（修复 #24：缓存失效）
func (r *TokenRepository) InvalidateCache(ctx context.Context, tokenHash string) {
	if r.redisClient == nil {
		return
	}

	cacheKey := fmt.Sprintf("token_valid:%s", tokenHash[:16])
	_ = r.redisClient.Client().Del(ctx, cacheKey).Err()
}
