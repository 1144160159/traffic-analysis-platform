////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/service/auth_service.go
// 完整修复版 v4：
// 1. 修复 #27 - RefreshToken 撤销时机错误（先生成新 Token，再撤销旧 Session）
// 2. 修复 #A2 - 登录成功后更新 last_login_at
// 3. 修复 #A13 - OIDC 用户角色持久化
// 4. 统一错误处理（修复 #3）
// 5. 完整保留所有原有代码（450+ 行）
////////////////////////////////////////////////////////////////////////////////

package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/config"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/jwt"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/oidc"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/security"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/authz"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"golang.org/x/crypto/bcrypt"
)

// AuthService 认证服务
type AuthService struct {
	userRepo     *repository.UserRepository
	settingsRepo *repository.UserSettingsRepository
	jwtService   *jwt.Service
	oidcProvider *oidc.Provider
	config       *config.Config
	logger       *zap.Logger
	auditLogger  *audit.Logger
	passwordHash *security.PasswordHasher
}

// NewAuthService 创建认证服务
func NewAuthService(
	userRepo *repository.UserRepository,
	jwtService *jwt.Service,
	oidcProvider *oidc.Provider,
	cfg *config.Config,
	logger *zap.Logger,
	auditLogger *audit.Logger,
) *AuthService {
	var settingsRepo *repository.UserSettingsRepository
	if userRepo != nil {
		settingsRepo = repository.NewUserSettingsRepository(userRepo.DB(), logger)
	}
	return &AuthService{
		userRepo:     userRepo,
		settingsRepo: settingsRepo,
		jwtService:   jwtService,
		oidcProvider: oidcProvider,
		config:       cfg,
		passwordHash: security.NewPasswordHasher(passwordPolicyConfig(cfg)),
		logger:       logger,
		auditLogger:  auditLogger,
	}
}

// passwordPolicyConfig 把配置的 SecurityConfig 转为密码策略配置。
// cfg 为 nil 时使用默认策略（长度>=8，不强制复杂度，避免行为突变）。
func passwordPolicyConfig(cfg *config.Config) security.PasswordConfig {
	if cfg == nil {
		return security.PasswordConfig{MinLength: 8, BcryptCost: bcrypt.DefaultCost}
	}
	return security.PasswordConfig{
		MinLength:        cfg.Security.MinPasswordLength,
		RequireUppercase: cfg.Security.RequireUppercase,
		RequireLowercase: cfg.Security.RequireLowercase,
		RequireDigit:     cfg.Security.RequireDigit,
		RequireSpecial:   cfg.Security.RequireSpecial,
		BcryptCost:       cfg.Security.BcryptCost,
	}
}

// LoginRequest 登录请求
type LoginRequest struct {
	TenantID    string `json:"tenant_id"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	CaptchaID   string `json:"captcha_id,omitempty"`
	CaptchaCode string `json:"captcha_code,omitempty"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"`
	TokenType    string   `json:"token_type"`
	User         UserInfo `json:"user"`
	// KCAccessToken OIDC 登录时 Keycloak 签发的访问令牌(网关数据路由按 OIDC 校验,
	// 前端对 /v1/auth/ 之外的数据 API 使用该令牌;不进 JSON 响应,仅经 OIDC 回调
	// 重定向 fragment 交付)。账号密码登录路径无此字段。
	KCAccessToken string `json:"kc_access_token,omitempty"`
	// KCRefreshToken OIDC 登录时 Keycloak 签发的刷新令牌(前端 KC 令牌过期后经
	// POST /v1/auth/refresh 服务端兑换;不进 JSON 响应,仅经回调 fragment 交付)。
	KCRefreshToken string `json:"kc_refresh_token,omitempty"`
}

// UserInfo 用户信息
type UserInfo struct {
	UserID          string   `json:"user_id"`
	TenantID        string   `json:"tenant_id"`
	Username        string   `json:"username"`
	Email           string   `json:"email"`
	Roles           []string `json:"roles"`
	Revision        int64    `json:"revision,omitempty"`
	EventID         string   `json:"event_id,omitempty"`
	OutboxStatus    string   `json:"outbox_status,omitempty"`
	IdempotentReuse bool     `json:"idempotent_reuse,omitempty"`
}

// UpdateCurrentUserRequest 当前用户资料更新请求
type UpdateCurrentUserRequest struct {
	Email            string `json:"email"`
	ActionID         string `json:"action_id,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ExpectedRevision *int64 `json:"expected_revision,omitempty"`
}

// UserSettingsResponse 用户偏好设置响应。
type UserSettingsResponse struct {
	Category        string                 `json:"category"`
	Settings        map[string]interface{} `json:"settings"`
	Revision        int64                  `json:"revision"`
	EventID         string                 `json:"event_id,omitempty"`
	OutboxStatus    string                 `json:"outbox_status,omitempty"`
	IdempotentReuse bool                   `json:"idempotent_reuse,omitempty"`
	UpdatedAt       time.Time              `json:"updated_at,omitempty"`
}

// UserSettingsUpdateCommand contains command metadata separated from the settings document.
type UserSettingsUpdateCommand struct {
	Settings         map[string]interface{}
	ActionID         string
	Reason           string
	IdempotencyKey   string
	ExpectedRevision *int64
	Username         string
	TraceID          string
	SourceIP         string
	UserAgent        string
}

// UpdateCurrentUserRequest 当前用户资料更新请求
type UpdateCurrentUserRequest struct {
	Email string `json:"email"`
}

// UserSettingsResponse 用户偏好设置响应。
type UserSettingsResponse struct {
	Category string                 `json:"category"`
	Settings map[string]interface{} `json:"settings"`
}

// Login 登录（修复 #A2：更新最后登录时间）
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	// 获取用户
	user, err := s.userRepo.GetByUsername(ctx, req.TenantID, req.Username)
	if err != nil {
		s.logger.Warn("Login failed: database error",
			zap.String("username", req.Username),
			zap.String("tenant_id", req.TenantID),
			zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query user")
	}

	if user == nil {
		s.logger.Warn("Login failed: user not found",
			zap.String("username", req.Username),
			zap.String("tenant_id", req.TenantID))
		return nil, errors.New(errors.ErrCodeInvalidCredentials, "Invalid username or password")
	}

	// 验证密码
	if !s.userRepo.VerifyPassword(user, req.Password) {
		s.logger.Warn("Login failed: invalid password",
			zap.String("username", req.Username),
			zap.String("tenant_id", req.TenantID))
		return nil, errors.New(errors.ErrCodeInvalidCredentials, "Invalid username or password")
	}

	// 检查用户状态
	if user.Status != "active" {
		return nil, errors.Newf(errors.ErrCodeUserNotActive, "User account is %s", user.Status)
	}

	// 修复 #A2：更新最后登录时间
	if err := s.userRepo.RecordLoginAtomic(ctx, user.UserID, user.TenantID); err != nil {
		// 记录错误但不阻止登录
		s.logger.Warn("Failed to update last login time",
			zap.String("user_id", user.UserID.String()),
			zap.Error(err))
	}

	// 获取角色和权限
	roles, err := s.userRepo.GetUserRoles(ctx, user.UserID)
	if err != nil {
		s.logger.Error("Failed to get user roles", zap.Error(err))
		// 不阻止登录，使用空角色列表
		roles = []string{}
	}

	permissions := s.getPermissionsFromRoles(roles)

	// 生成令牌
	tokenPair, err := s.jwtService.GenerateTokenPair(user, roles, permissions)
	if err != nil {
		return nil, err
	}

	s.logger.Info("User logged in",
		zap.String("user_id", user.UserID.String()),
		zap.String("username", user.Username))

	s.auditLogin(ctx, user, true, "")

	// 统一令牌模型 P1 终结:密码登录同样交付 Keycloak 令牌(服务端口令兑换),
	// 前端单一 KC 令牌;兑换失败不阻断登录(兼容无 KC 账号的本地用户)。
	var kcAccess, kcRefresh string
	if s.oidcProvider != nil && req.Password != "" {
		if kcResp, err := s.oidcProvider.PasswordGrant(ctx, req.Username, req.Password); err != nil {
			s.logger.Warn("KC password grant failed during login; falling back to app tokens only",
				zap.String("username", req.Username), zap.Error(err))
		} else {
			kcAccess, kcRefresh = kcResp.AccessToken, kcResp.RefreshToken
		}
	}

	return &LoginResponse{
		AccessToken:    tokenPair.AccessToken,
		RefreshToken:   tokenPair.RefreshToken,
		ExpiresIn:      tokenPair.ExpiresIn,
		TokenType:      tokenPair.TokenType,
		KCAccessToken:  kcAccess,
		KCRefreshToken: kcRefresh,
		User: UserInfo{
			UserID:   user.UserID.String(),
			TenantID: user.TenantID,
			Username: user.Username,
			Email:    user.Email,
			Roles:    roles,
		},
	}, nil
}

// RefreshToken 刷新 Token（修复 #27：先生成新 Token，再撤销旧 Session）
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*LoginResponse, error) {
	// 验证 Refresh Token
	claims, err := s.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		// 统一令牌模型(P1):应用 refresh 校验失败时按 Keycloak refresh token
		// 服务端兑换(客户端机密在服务侧,浏览器不持 secret)。
		if s.oidcProvider == nil {
			return nil, err
		}
		kcResp, kcErr := s.oidcProvider.RefreshToken(ctx, refreshToken)
		if kcErr != nil {
			return nil, err
		}
		ac, acErr := s.oidcProvider.ValidateAccessTokenRoles(kcResp.AccessToken)
		if acErr != nil {
			return nil, errors.Wrap(acErr, errors.ErrCodeTokenInvalid, "refreshed access token validation failed")
		}
		roles := s.mapOIDCRoles(ac.GetRoles(s.getOIDCClientID()))
		tenant := strings.TrimSpace(ac.TenantID)
		if tenant == "" {
			tenant = "default"
		}
		return &LoginResponse{
			AccessToken:    kcResp.AccessToken,
			RefreshToken:   kcResp.RefreshToken,
			ExpiresIn:      kcResp.ExpiresIn,
			TokenType:      kcResp.TokenType,
			KCAccessToken:  kcResp.AccessToken,
			KCRefreshToken: kcResp.RefreshToken,
			User: UserInfo{
				UserID:   ac.Subject,
				TenantID: tenant,
				Username: ac.PreferredUsername,
				Email:    ac.Email,
				Roles:    roles,
			},
		}, nil
	}

	// 获取用户
	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query user")
	}

	if user == nil {
		return nil, errors.New(errors.ErrCodeUserNotFound, "User not found")
	}

	// 检查用户状态
	if user.Status != "active" {
		return nil, errors.Newf(errors.ErrCodeUserNotActive, "User account is %s", user.Status)
	}

	// 获取角色和权限
	roles, err := s.userRepo.GetUserRoles(ctx, user.UserID)
	if err != nil {
		roles = []string{}
	}

	permissions := s.getPermissionsFromRoles(roles)

	// 修复 #27：先生成新 Token
	tokenPair, err := s.jwtService.GenerateTokenPair(user, roles, permissions)
	if err != nil {
		return nil, err
	}

	// 修复 #27/#8：生成新 token 后同步撤销旧 Session（带有限重试），
	// 不再 fire-and-forget。撤销失败不阻塞新 token 签发，但必须记审计
	// 并记录错误，避免旧 refresh token 在 TTL 内持续有效且无人感知。
	revokeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var revokeErr error
	for attempt := 0; attempt < 2; attempt++ {
		revokeErr = s.jwtService.RevokeSession(revokeCtx, claims.SessionID)
		if revokeErr == nil {
			break
		}
		if attempt == 0 {
			s.logger.Warn("Failed to revoke old session after refresh, retrying",
				zap.String("old_session_id", claims.SessionID),
				zap.Error(revokeErr))
			time.Sleep(50 * time.Millisecond)
		}
	}
	if revokeErr != nil {
		s.logger.Error("Failed to revoke old session after refresh (token reuse protection degraded)",
			zap.String("old_session_id", claims.SessionID),
			zap.Error(revokeErr))
		if s.auditLogger != nil {
			s.auditLogger.Log(revokeCtx, &audit.AuditEvent{
				EventType:    audit.EventTypeAuthFailure,
				Action:       "refresh_session_revoke_failed",
				ResourceType: "session",
				TenantID:     user.TenantID,
				UserID:       user.UserID.String(),
				Result:       audit.ResultFailure,
				ErrorMsg:     "old session revocation failed after refresh: " + revokeErr.Error(),
			})
		}
	}

	return &LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
		TokenType:    tokenPair.TokenType,
		User: UserInfo{
			UserID:   user.UserID.String(),
			TenantID: user.TenantID,
			Username: user.Username,
			Email:    user.Email,
			Roles:    roles,
		},
	}, nil
}

// Logout 登出
// Logout 撤销本地应用会话;若携带 Keycloak refresh token 则同时调 KC
// logout 端点终止刷新链(服务端登出,前端清 token 之外的纵深)。
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	if err := s.jwtService.RevokeSession(ctx, sessionID); err != nil {
		s.logger.Warn("local session revoke failed during logout", zap.Error(err))
	}
	return nil
}

// LogoutWithKCSession 终止 Keycloak 会话刷新链(best-effort,失败不阻断登出)。
func (s *AuthService) LogoutWithKCSession(ctx context.Context, refreshToken string) error {
	if s.oidcProvider == nil || strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	if err := s.oidcProvider.Logout(ctx, refreshToken); err != nil {
		return err
	}
	return nil
}

// UpdateCurrentUser 更新当前用户可自助维护的资料
func (s *AuthService) UpdateCurrentUser(ctx context.Context, userID uuid.UUID, tenantID string, req *UpdateCurrentUserRequest, meta repository.UserCommandMetadata) (*UserInfo, error) {
	if req == nil {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "profile update is required")
	}
	email := strings.TrimSpace(req.Email)
	if email != "" && !strings.Contains(email, "@") {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid email")
	}
	// 统一令牌模型 P1:KC 会话的 claims.UserID 是 Keycloak subject,本地用户
	// 以 external_id 关联 —— 先按本地主键解析,再按外部 ID 解析,均命中才执行
	// (修复 KC 用户 PATCH /me 的 AUTH_1006 "User not found in authenticated tenant")。
	localUser, err := s.resolveLocalUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if localUser == nil || localUser.TenantID != tenantID {
		return nil, errors.New(errors.ErrCodeUserNotFound, "User not found in authenticated tenant")
	}
	meta.TenantID = tenantID
	meta.ActorID = localUser.UserID
	meta.ActionID = req.ActionID
	meta.Reason = req.Reason
	meta.ExpectedRevision = req.ExpectedRevision
	result, err := s.userRepo.UpdateProfileAtomic(ctx, localUser.UserID, email, meta)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, localUser.UserID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query updated user")
	}
	if user == nil || user.TenantID != tenantID {
		return nil, errors.New(errors.ErrCodeUserNotFound, "User not found in authenticated tenant")
	}
	roles, err := s.userRepo.GetUserRoles(ctx, user.UserID)
	if err != nil {
		s.logger.Warn("Failed to get user roles after profile update",
			zap.String("user_id", user.UserID.String()),
			zap.Error(err))
		roles = []string{}
	}
	return &UserInfo{
		UserID:          user.UserID.String(),
		TenantID:        user.TenantID,
		Username:        user.Username,
		Email:           user.Email,
		Roles:           roles,
		Revision:        result.Revision,
		EventID:         result.EventID,
		OutboxStatus:    result.OutboxStatus,
		IdempotentReuse: result.IdempotentReuse,
	}, nil
}

// ChangePassword 校验当前密码后更新密码
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, tenantID, currentPassword, newPassword string, meta repository.UserCommandMetadata) (*repository.UserCommandResult, error) {
	if currentPassword == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "current_password is required")
	}
	// 密码策略强制：统一走 PasswordHasher.ValidatePassword（长度/大小写/数字/特殊字符）
	if err := s.passwordHash.ValidatePassword(newPassword); err != nil {
		return nil, err
	}
	// 统一令牌模型 P1:同 UpdateCurrentUser,KC subject 按 external_id 解析本地用户。
	localUser, err := s.resolveLocalUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if localUser == nil || localUser.TenantID != tenantID {
		return nil, errors.New(errors.ErrCodeUserNotFound, "User not found in authenticated tenant")
	}
	meta.TenantID = tenantID
	meta.ActorID = localUser.UserID
	return s.userRepo.ChangePasswordAtomic(ctx, localUser.UserID, currentPassword, newPassword, meta)
}

// GetUserSettings 获取当前用户某类偏好设置；没有保存过时返回服务端默认值。

// resolveLocalUser 统一令牌模型 P1:claims.UserID 可能是本地主键也可能是
// Keycloak subject(本地用户以 external_id 关联)——按主键、外部 ID 依次解析。
func (s *AuthService) resolveLocalUser(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	if s.userRepo == nil {
		return nil, errors.New(errors.ErrCodeServiceUnavailable, "user repository is unavailable")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query user")
	}
	if user == nil {
		user, err = s.userRepo.GetByExternalID(ctx, userID.String())
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query user by external id")
		}
	}
	return user, nil
}

func (s *AuthService) GetUserSettings(ctx context.Context, tenantID string, userID uuid.UUID, category string) (*UserSettingsResponse, error) {
	category = normalizeSettingsCategory(category)
	if category == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid settings category")
	}
	values := defaultUserSettings(category)
	if s.settingsRepo == nil {
		return &UserSettingsResponse{Category: category, Settings: values, Revision: 0}, nil
	}
	localUser, err := s.resolveLocalUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if localUser == nil {
		// 未关联本地用户(KC 会话但本地无行):返回默认值,不报错
		return &UserSettingsResponse{Category: category, Settings: values, Revision: 0}, nil
	}
	stored, err := s.settingsRepo.Get(ctx, tenantID, localUser.UserID, category)
	if err != nil {
		return nil, err
	}
	if stored != nil {
		for key, value := range stored.Settings {
			values[key] = value
		}
	}
	revision := int64(0)
	var updatedAt time.Time
	if stored != nil {
		revision = stored.Revision
		updatedAt = stored.UpdatedAt
	}
	return &UserSettingsResponse{Category: category, Settings: values, Revision: revision, UpdatedAt: updatedAt}, nil
}

// UpdateUserSettings 保存当前用户某类偏好设置。
func (s *AuthService) UpdateUserSettings(ctx context.Context, tenantID string, userID uuid.UUID, category string, values map[string]interface{}) (*UserSettingsResponse, error) {
	actionID := uuid.NewString()
	return s.UpdateUserSettingsCommand(ctx, tenantID, userID, category, UserSettingsUpdateCommand{
		Settings: values, ActionID: actionID, IdempotencyKey: actionID, Reason: "update user settings",
	})
}

// UpdateUserSettingsCommand atomically persists state, audit, history, outbox and idempotency facts.
func (s *AuthService) UpdateUserSettingsCommand(ctx context.Context, tenantID string, userID uuid.UUID, category string, command UserSettingsUpdateCommand) (*UserSettingsResponse, error) {
	category = normalizeSettingsCategory(category)
	if category == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid settings category")
	}
	merged := defaultUserSettings(category)
	for key, value := range command.Settings {
		merged[key] = value
	}
	if s.settingsRepo == nil {
		return nil, errors.New(errors.ErrCodeServiceUnavailable, "user settings repository is unavailable")
	}
	localUser, err := s.resolveLocalUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if localUser == nil || localUser.TenantID != tenantID {
		return nil, errors.New(errors.ErrCodeUserNotFound, "User not found in authenticated tenant")
	}
	stored, err := s.settingsRepo.SaveCommand(ctx, repository.UserSettingsCommand{
		TenantID: tenantID, UserID: localUser.UserID, Username: command.Username, Category: category, Settings: merged,
		ActionID: command.ActionID, Reason: command.Reason, IdempotencyKey: command.IdempotencyKey,
		ExpectedRevision: command.ExpectedRevision, TraceID: command.TraceID,
		SourceIP: command.SourceIP, UserAgent: command.UserAgent,
	})
	if err != nil {
		return nil, err
	}
	return &UserSettingsResponse{
		Category: stored.Category, Settings: stored.Settings, Revision: stored.Revision,
		EventID: stored.EventID, OutboxStatus: stored.OutboxStatus,
		IdempotentReuse: stored.IdempotentReuse, UpdatedAt: stored.UpdatedAt,
	}, nil
}

func normalizeSettingsCategory(category string) string {
	category = strings.TrimSpace(strings.ToLower(category))
	switch category {
	case "notifications", "display":
		return category
	default:
		return ""
	}
}

func defaultUserSettings(category string) map[string]interface{} {
	switch category {
	case "notifications":
		return map[string]interface{}{
			"email_enabled":       true,
			"wechat_enabled":      false,
			"webhook_enabled":     false,
			"min_severity":        "medium",
			"alert_types":         []interface{}{},
			"webhook_url":         "",
			"webhook_auth_header": "",
		}
	case "display":
		return map[string]interface{}{
			"page_size":          20,
			"refresh_interval":   30,
			"default_time_range": "last_24h",
			"timezone":           "Asia/Shanghai",
			"show_ws_status":     true,
		}
	default:
		return map[string]interface{}{}
	}
}

// UpdateCurrentUser 更新当前用户可自助维护的资料
func (s *AuthService) UpdateCurrentUser(ctx context.Context, userID uuid.UUID, req *UpdateCurrentUserRequest) (*UserInfo, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query user")
	}
	if user == nil {
		return nil, errors.New(errors.ErrCodeUserNotFound, "User not found")
	}
	if req != nil {
		email := strings.TrimSpace(req.Email)
		if email != "" && !strings.Contains(email, "@") {
			return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid email")
		}
		user.Email = email
	}
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}
	roles, err := s.userRepo.GetUserRoles(ctx, user.UserID)
	if err != nil {
		s.logger.Warn("Failed to get user roles after profile update",
			zap.String("user_id", user.UserID.String()),
			zap.Error(err))
		roles = []string{}
	}
	return &UserInfo{
		UserID:   user.UserID.String(),
		TenantID: user.TenantID,
		Username: user.Username,
		Email:    user.Email,
		Roles:    roles,
	}, nil
}

// ChangePassword 校验当前密码后更新密码
func (s *AuthService) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	if currentPassword == "" {
		return errors.New(errors.ErrCodeMissingParameter, "current_password is required")
	}
	if len(newPassword) < 8 {
		return errors.New(errors.ErrCodeInvalidParameter, "new_password must be at least 8 characters")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to query user")
	}
	if user == nil {
		return errors.New(errors.ErrCodeUserNotFound, "User not found")
	}
	if !s.userRepo.VerifyPassword(user, currentPassword) {
		return errors.New(errors.ErrCodeInvalidCredentials, "Invalid current password")
	}
	return s.userRepo.UpdatePassword(ctx, user.UserID, newPassword)
}

// GetUserSettings 获取当前用户某类偏好设置；没有保存过时返回服务端默认值。
func (s *AuthService) GetUserSettings(ctx context.Context, tenantID string, userID uuid.UUID, category string) (*UserSettingsResponse, error) {
	category = normalizeSettingsCategory(category)
	if category == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid settings category")
	}
	values := defaultUserSettings(category)
	if s.settingsRepo == nil {
		return &UserSettingsResponse{Category: category, Settings: values}, nil
	}
	stored, err := s.settingsRepo.Get(ctx, tenantID, userID, category)
	if err != nil {
		return nil, err
	}
	if stored != nil {
		for key, value := range stored.Settings {
			values[key] = value
		}
	}
	return &UserSettingsResponse{Category: category, Settings: values}, nil
}

// UpdateUserSettings 保存当前用户某类偏好设置。
func (s *AuthService) UpdateUserSettings(ctx context.Context, tenantID string, userID uuid.UUID, category string, values map[string]interface{}) (*UserSettingsResponse, error) {
	category = normalizeSettingsCategory(category)
	if category == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid settings category")
	}
	merged := defaultUserSettings(category)
	for key, value := range values {
		merged[key] = value
	}
	if s.settingsRepo == nil {
		return &UserSettingsResponse{Category: category, Settings: merged}, nil
	}
	stored, err := s.settingsRepo.Upsert(ctx, tenantID, userID, category, merged)
	if err != nil {
		return nil, err
	}
	return &UserSettingsResponse{Category: category, Settings: stored.Settings}, nil
}

func normalizeSettingsCategory(category string) string {
	category = strings.TrimSpace(strings.ToLower(category))
	switch category {
	case "notifications", "display":
		return category
	default:
		return ""
	}
}

func defaultUserSettings(category string) map[string]interface{} {
	switch category {
	case "notifications":
		return map[string]interface{}{
			"email_enabled":       true,
			"wechat_enabled":      false,
			"webhook_enabled":     false,
			"min_severity":        "medium",
			"alert_types":         []interface{}{},
			"webhook_url":         "",
			"webhook_auth_header": "",
		}
	case "display":
		return map[string]interface{}{
			"page_size":          20,
			"refresh_interval":   30,
			"default_time_range": "last_24h",
			"timezone":           "Asia/Shanghai",
			"show_ws_status":     true,
		}
	default:
		return map[string]interface{}{}
	}
}

// GetOIDCAuthURL 获取 OIDC 认证 URL
func (s *AuthService) GetOIDCAuthURL(state, nonce string) string {
	if s.oidcProvider == nil {
		return ""
	}
	return s.oidcProvider.GetAuthURL(state, nonce)
}

// ValidateOIDCTenant 校验 OIDC 登录目标租户是否在白名单内。
// 安全加固：租户不得由客户端任意指定。未配置 ALLOWED_TENANTS 时仅允许
// 默认租户 "default"（fail-closed）。
func (s *AuthService) ValidateOIDCTenant(tenantID string) error {
	if tenantID == "" {
		return errors.New(errors.ErrCodePermissionDenied, "OIDC login requires a tenant")
	}
	var allowed []string
	if s.config != nil {
		allowed = s.config.OIDC.AllowedTenants
	}
	if len(allowed) == 0 {
		if tenantID == "default" {
			return nil
		}
		return errors.Newf(errors.ErrCodePermissionDenied,
			"OIDC login is not allowed for tenant %q", tenantID)
	}
	for _, t := range allowed {
		if t == tenantID {
			return nil
		}
	}
	return errors.Newf(errors.ErrCodePermissionDenied,
		"OIDC login is not allowed for tenant %q", tenantID)
}

// HandleOIDCCallback 处理 OIDC 回调（修复 #A13：角色持久化）
func (s *AuthService) HandleOIDCCallback(ctx context.Context, code, tenantID, expectedNonce string, meta repository.UserCommandMetadata) (*LoginResponse, error) {
	if s.oidcProvider == nil {
		return nil, errors.New(errors.ErrCodeOIDCError, "OIDC is not configured")
	}

	// 交换 code 获取 token
	tokenResp, err := s.oidcProvider.ExchangeCode(ctx, code)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeOIDCError, "Failed to exchange authorization code")
	}

	// 验证 ID token
	claims, err := s.oidcProvider.ValidateIDToken(tokenResp.IDToken)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeOIDCError, "Failed to validate ID token")
	}

	// Nonce 校验（OIDC 抗重放）：ID token 必须回显本次登录会话签发的 nonce。
	// expectedNonce 为空或与 token 内 nonce 不一致一律拒绝。
	if expectedNonce == "" {
		return nil, errors.New(errors.ErrCodeOIDCError, "OIDC callback missing expected nonce")
	}
	if claims.Nonce != expectedNonce {
		s.logger.Warn("OIDC nonce mismatch",
			zap.String("tenant_id", tenantID),
			zap.String("expected", expectedNonce[:min(len(expectedNonce), 8)]))
		return nil, errors.New(errors.ErrCodeOIDCError, "OIDC nonce mismatch: token not bound to this login session")
	}

	// 映射 OIDC 角色到本地角色。
	// 角色事实以 access token 为准(本部署 ID token 不含角色声明,access token 经
	// 同一 token 端点签发且由网关统一校验;此处再经 JWKS 验签读取角色)。
	clientID := s.getOIDCClientID()
	oidcRoles := claims.GetRoles(clientID)
	if len(oidcRoles) == 0 {
		if accessClaims, err := s.oidcProvider.ValidateAccessTokenRoles(tokenResp.AccessToken); err != nil {
			s.logger.Warn("OIDC access token role extraction failed",
				zap.Error(err), zap.String("username", claims.PreferredUsername))
		} else {
			oidcRoles = accessClaims.GetRoles(clientID)
		}
	}
	roles := s.mapOIDCRoles(oidcRoles)
	// 安全加固：回调侧再次校验租户白名单（state 由本服务签发，此处为纵深防御）
	if err := s.ValidateOIDCTenant(tenantID); err != nil {
		return nil, err
	}
	meta.TenantID = tenantID
	user, commandResult, err := s.userRepo.SyncOIDCUserAtomic(ctx, claims, roles, meta)
	if err != nil {
		return nil, err
	}

	permissions := s.getPermissionsFromRoles(roles)

	// 生成我们的令牌
	tokenPair, err := s.jwtService.GenerateTokenPair(user, roles, permissions)
	if err != nil {
		return nil, err
	}

	s.logger.Info("User logged in via OIDC",
		zap.String("user_id", user.UserID.String()),
		zap.String("username", user.Username),
		zap.Strings("roles", roles))

	return &LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		ExpiresIn:    tokenPair.ExpiresIn,
		TokenType:    tokenPair.TokenType,
		User: UserInfo{
			UserID:          user.UserID.String(),
			TenantID:        user.TenantID,
			Username:        user.Username,
			Email:           user.Email,
			Roles:           roles,
			Revision:        commandResult.Revision,
			EventID:         commandResult.EventID,
			OutboxStatus:    commandResult.OutboxStatus,
			IdempotentReuse: commandResult.IdempotentReuse,
		},
		// 数据路由(网关 OIDC)用 Keycloak 令牌;仅 OIDC 回调重定向片段交付
		KCAccessToken:  tokenResp.AccessToken,
		KCRefreshToken: tokenResp.RefreshToken,
	}, nil
}

// ValidateToken 验证 Token
func (s *AuthService) ValidateToken(tokenString string) (*model.Claims, error) {
	claims, err := s.jwtService.ValidateAccessToken(tokenString)
	if err == nil {
		return claims, nil
	}
	// 统一令牌模型(P1):HMAC 应用 token 校验失败时,回退按 Keycloak 访问令牌
	// (JWKS 验签)解析——auth-service/alert-service/graph-service 共用此入口,
	// 一处改动全体接受 Keycloak token。
	if s.oidcProvider == nil {
		return nil, err
	}
	oc, oidcErr := s.oidcProvider.ValidateAccessTokenRoles(tokenString)
	if oidcErr != nil {
		return nil, err
	}
	roles := s.mapOIDCRoles(oc.GetRoles(s.getOIDCClientID()))
	permissions := s.getPermissionsFromRoles(roles)
	tenant := strings.TrimSpace(oc.TenantID)
	if tenant == "" {
		tenant = "default"
	}
	userID, parseErr := uuid.Parse(oc.Subject)
	if parseErr != nil {
		return nil, errors.New(errors.ErrCodeTokenInvalid, "OIDC subject is not a valid user id")
	}
	return &model.Claims{
		RegisteredClaims: oc.RegisteredClaims,
		UserID:           userID,
		TenantID:         tenant,
		Username:         oc.PreferredUsername,
		Email:            oc.Email,
		Roles:            roles,
		Permissions:      permissions,
		TokenType:        model.JWTTokenAccess,
	}, nil
}

// getOIDCClientID 获取 OIDC Client ID
func (s *AuthService) getOIDCClientID() string {
	if s.config != nil {
		return s.config.OIDC.ClientID
	}
	return "traffic-api"
}

// getPermissionsFromRoles 从角色获取权限
func (s *AuthService) getPermissionsFromRoles(roles []string) []string {
	permissionSet := make(map[string]bool)

	for _, role := range roles {
		if perms, ok := model.DefaultRoleScopes[role]; ok {
			for _, perm := range perms {
				permissionSet[perm] = true
			}
		}
	}

	permissions := make([]string, 0, len(permissionSet))
	for perm := range permissionSet {
		permissions = append(permissions, perm)
	}
	sort.Strings(permissions)

	return permissions
}

// mapOIDCRoles 映射 OIDC 角色到内部角色(ADR-3:唯一权威为
// contracts/authz/oidc-role-map.v1.json,经共享解释器 authz.MapOIDCRoles 执行,
// 与共享中间件零漂移)。
func (s *AuthService) mapOIDCRoles(oidcRoles []string) []string {
	return authz.MapOIDCRoles(oidcRoles)
}

// auditLogin records audit event for login attempts
func (s *AuthService) auditLogin(ctx context.Context, user *model.User, success bool, reason string) {
	if s.auditLogger == nil {
		return
	}
	eventType := audit.EventTypeLogin
	if !success {
		eventType = audit.EventTypeLoginFailed
	}
	s.auditLogger.Log(ctx, &audit.AuditEvent{
		EventType:    eventType,
		TenantID:     user.TenantID,
		UserID:       user.UserID.String(),
		Username:     user.Username,
		Action:       "login",
		ResourceType: "auth",
	})
}

// auditLogout records audit event for logout
func (s *AuthService) auditLogout(ctx context.Context, tenantID, userID, sessionID string) {
	if s.auditLogger == nil {
		return
	}
	s.auditLogger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeLogout,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       "logout",
		ResourceType: "auth",
	})
}

// auditLogin records audit event for login attempts
func (s *AuthService) auditLogin(ctx context.Context, user *model.User, success bool, reason string) {
	if s.auditLogger == nil {
		return
	}
	eventType := audit.EventTypeLogin
	if !success {
		eventType = audit.EventTypeLoginFailed
	}
	s.auditLogger.Log(ctx, &audit.AuditEvent{
		EventType:    eventType,
		TenantID:     user.TenantID,
		UserID:       user.UserID.String(),
		Username:     user.Username,
		Action:       "login",
		ResourceType: "auth",
	})
}

// auditLogout records audit event for logout
func (s *AuthService) auditLogout(ctx context.Context, tenantID, userID, sessionID string) {
	if s.auditLogger == nil {
		return
	}
	s.auditLogger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeLogout,
		TenantID:     tenantID,
		UserID:       userID,
		Action:       "logout",
		ResourceType: "auth",
	})
}
