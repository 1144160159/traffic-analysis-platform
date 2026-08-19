////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/repository/user_repository.go
// 完整修复版 v3：
// 1. 修复 #A2：添加 UpdateLastLoginAt 方法
// 2. 修复 #A5：补充角色管理方法
// 3. 保留所有原有代码（500+行完整）
////////////////////////////////////////////////////////////////////////////////

package repository

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// UserRepository 用户仓储
type UserRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewUserRepository 创建用户仓储
func NewUserRepository(db *sql.DB, logger *zap.Logger) *UserRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &UserRepository{
		db:     db,
		logger: logger,
	}
}

// DB 返回底层数据库连接，供同一 auth 聚合内的轻量仓储复用。
func (r *UserRepository) DB() *sql.DB {
	return r.db
}

// Create 创建用户
func (r *UserRepository) Create(ctx context.Context, user *model.User, password string) error {
	return r.CreateLocalUserAtomic(ctx, user, password)
}

// GetByID 根据 ID 获取用户（修复 #A2：包含 last_login_at）
func (r *UserRepository) GetByID(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	query := `
		SELECT user_id, tenant_id, username, email, password_hash, status, external_id, last_login_at, revision, created_at, updated_at
		FROM users
		WHERE user_id = $1
	`

	var user model.User
	var externalID sql.NullString
	var email sql.NullString
	var passwordHash sql.NullString
	var lastLoginAt sql.NullTime // 修复 #A2：新增字段

	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&user.UserID,
		&user.TenantID,
		&user.Username,
		&email,
		&passwordHash,
		&user.Status,
		&externalID,
		&lastLoginAt, // 修复 #A2：扫描新字段
		&user.Revision,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		r.logger.Error("Failed to get user by ID",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get user")
	}

	// 转换 NullString 到 string
	if email.Valid {
		user.Email = email.String
	}
	if passwordHash.Valid {
		user.PasswordHash = passwordHash.String
	}
	if externalID.Valid {
		user.ExternalID = externalID.String
	}
	// 修复 #A2：转换 NullTime
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return &user, nil
}

// GetByUsername 根据用户名获取用户（修复 #A2）
func (r *UserRepository) GetByUsername(ctx context.Context, tenantID, username string) (*model.User, error) {
	query := `
		SELECT user_id, tenant_id, username, email, password_hash, status, external_id, last_login_at, revision, created_at, updated_at
		FROM users
		WHERE tenant_id = $1 AND username = $2
	`

	var user model.User
	var externalID sql.NullString
	var email sql.NullString
	var passwordHash sql.NullString
	var lastLoginAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, tenantID, username).Scan(
		&user.UserID,
		&user.TenantID,
		&user.Username,
		&email,
		&passwordHash,
		&user.Status,
		&externalID,
		&lastLoginAt,
		&user.Revision,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		r.logger.Error("Failed to get user by username",
			zap.String("username", username),
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get user")
	}

	if email.Valid {
		user.Email = email.String
	}
	if passwordHash.Valid {
		user.PasswordHash = passwordHash.String
	}
	if externalID.Valid {
		user.ExternalID = externalID.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return &user, nil
}

// GetByExternalID 根据外部 ID 获取用户（OIDC）（修复 #A2）
func (r *UserRepository) GetByExternalID(ctx context.Context, externalID string) (*model.User, error) {
	query := `
		SELECT user_id, tenant_id, username, email, password_hash, status, external_id, last_login_at, revision, created_at, updated_at
		FROM users
		WHERE external_id = $1
	`

	var user model.User
	var extID sql.NullString
	var email sql.NullString
	var passwordHash sql.NullString
	var lastLoginAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, externalID).Scan(
		&user.UserID,
		&user.TenantID,
		&user.Username,
		&email,
		&passwordHash,
		&user.Status,
		&extID,
		&lastLoginAt,
		&user.Revision,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		r.logger.Error("Failed to get user by external ID",
			zap.String("external_id", externalID),
			zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get user by external ID")
	}

	if email.Valid {
		user.Email = email.String
	}
	if passwordHash.Valid {
		user.PasswordHash = passwordHash.String
	}
	if extID.Valid {
		user.ExternalID = extID.String
	}
	if lastLoginAt.Valid {
		user.LastLoginAt = &lastLoginAt.Time
	}

	return &user, nil
}

// UpdateLastLoginAt 更新最后登录时间（修复 #A2：新增方法）
func (r *UserRepository) UpdateLastLoginAt(ctx context.Context, userID uuid.UUID) error {
	user, err := r.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New(errors.ErrCodeUserNotFound, "User not found")
	}
	return r.RecordLoginAtomic(ctx, userID, user.TenantID)
}

// CreateOrUpdateFromOIDC 从 OIDC 创建或更新用户
func (r *UserRepository) CreateOrUpdateFromOIDC(ctx context.Context, claims *model.OIDCClaims, tenantID string) (*model.User, error) {
	if claims == nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "claims cannot be nil")
	}

	if claims.Subject == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "OIDC subject is required")
	}

	user, _, err := r.SyncOIDCUserAtomic(ctx, claims, nil, UserCommandMetadata{TenantID: tenantID, CompatibilityMode: true})
	return user, err
}

// GetUserRoles 获取用户角色
func (r *UserRepository) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]string, error) {
	query := `
		SELECT r.name
		FROM roles r
		JOIN user_roles ur ON r.role_id = ur.role_id
		WHERE ur.user_id = $1
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		r.logger.Error("Failed to get user roles",
			zap.String("user_id", userID.String()),
			zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get user roles")
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			r.logger.Error("Failed to scan role",
				zap.String("user_id", userID.String()),
				zap.Error(err))
			continue
		}
		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Error iterating roles")
	}

	return roles, nil
}

// AssignRole 分配角色
func (r *UserRepository) AssignRole(ctx context.Context, userID, roleID uuid.UUID) error {
	return r.AssignRoleAtomic(ctx, userID, roleID, false)
}

// RemoveRole 移除角色（修复 #A5：新增方法）
func (r *UserRepository) RemoveRole(ctx context.Context, userID, roleID uuid.UUID) error {
	return r.AssignRoleAtomic(ctx, userID, roleID, true)
}

// GetRoleIDByName 根据角色名称获取角色 ID
func (r *UserRepository) GetRoleIDByName(ctx context.Context, tenantID, roleName string) (uuid.UUID, error) {
	query := `SELECT role_id FROM roles WHERE tenant_id = $1 AND name = $2`

	var roleID uuid.UUID
	err := r.db.QueryRowContext(ctx, query, tenantID, roleName).Scan(&roleID)
	if err == sql.ErrNoRows {
		return uuid.Nil, errors.Newf(errors.ErrCodeEntityNotFound, "role %s not found in tenant %s", roleName, tenantID)
	}
	if err != nil {
		r.logger.Error("Failed to query role by name",
			zap.String("tenant_id", tenantID),
			zap.String("role_name", roleName),
			zap.Error(err))
		return uuid.Nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query role by name")
	}

	return roleID, nil
}
func (r *UserRepository) GetUsersByRole(ctx context.Context, tenantID string, roleID uuid.UUID) ([]*model.User, error) {
	query := `
		SELECT u.user_id, u.tenant_id, u.username, u.email, u.password_hash, u.status, u.external_id, u.last_login_at, u.revision, u.created_at, u.updated_at
		FROM users u
		JOIN user_roles ur ON u.user_id = ur.user_id
		WHERE u.tenant_id = $1 AND ur.role_id = $2
		ORDER BY u.created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID, roleID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query users by role")
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		var user model.User
		var email sql.NullString
		var passwordHash sql.NullString
		var externalID sql.NullString
		var lastLoginAt sql.NullTime

		err := rows.Scan(
			&user.UserID,
			&user.TenantID,
			&user.Username,
			&email,
			&passwordHash,
			&user.Status,
			&externalID,
			&lastLoginAt,
			&user.Revision,
			&user.CreatedAt,
			&user.UpdatedAt,
		)

		if err != nil {
			r.logger.Error("Failed to scan user",
				zap.Error(err))
			continue
		}

		if email.Valid {
			user.Email = email.String
		}
		if passwordHash.Valid {
			user.PasswordHash = passwordHash.String
		}
		if externalID.Valid {
			user.ExternalID = externalID.String
		}
		if lastLoginAt.Valid {
			user.LastLoginAt = &lastLoginAt.Time
		}

		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Error iterating users")
	}

	return users, nil
}

// VerifyPassword 验证密码
func (r *UserRepository) VerifyPassword(user *model.User, password string) bool {
	if user == nil || user.PasswordHash == "" || password == "" {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	return err == nil
}

// Update 更新用户
func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	return r.UpdateUserLegacyAtomic(ctx, user)
}

// UpdatePassword 更新密码
func (r *UserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, newPassword string) error {
	if newPassword == "" {
		return errors.New(errors.ErrCodeMissingParameter, "password is required")
	}

	return r.SetPasswordLegacyAtomic(ctx, userID, newPassword)
}

// Delete 删除用户
func (r *UserRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	return r.DeleteUserAtomic(ctx, userID)
}

// ListByTenant 列出租户的所有用户
func (r *UserRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*model.User, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// 获取总数
	countQuery := `SELECT COUNT(*) FROM users WHERE tenant_id = $1`
	var total int64
	err := r.db.QueryRowContext(ctx, countQuery, tenantID).Scan(&total)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to count users")
	}

	// 获取列表
	query := `
		SELECT user_id, tenant_id, username, email, password_hash, status, external_id, last_login_at, revision, created_at, updated_at
		FROM users
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.QueryContext(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query users")
	}
	defer rows.Close()

	var users []*model.User
	for rows.Next() {
		var user model.User
		var email sql.NullString
		var passwordHash sql.NullString
		var externalID sql.NullString
		var lastLoginAt sql.NullTime

		err := rows.Scan(
			&user.UserID,
			&user.TenantID,
			&user.Username,
			&email,
			&passwordHash,
			&user.Status,
			&externalID,
			&lastLoginAt,
			&user.Revision,
			&user.CreatedAt,
			&user.UpdatedAt,
		)

		if err != nil {
			r.logger.Error("Failed to scan user",
				zap.Error(err))
			continue
		}

		if email.Valid {
			user.Email = email.String
		}
		if passwordHash.Valid {
			user.PasswordHash = passwordHash.String
		}
		if externalID.Valid {
			user.ExternalID = externalID.String
		}
		if lastLoginAt.Valid {
			user.LastLoginAt = &lastLoginAt.Time
		}

		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Error iterating users")
	}

	return users, total, nil
}

// UpdateStatus 更新用户状态
func (r *UserRepository) UpdateStatus(ctx context.Context, userID uuid.UUID, status string) error {
	return r.UpdateUserStatusAtomic(ctx, userID, status)
}
