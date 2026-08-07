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
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
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
		logger:       logger,
		auditLogger:  auditLogger,
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

// RefreshToken 刷新 Token（修复 #27：先生成新 Token，再撤销旧 Session）
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*LoginResponse, error) {
	// 验证 Refresh Token
	claims, err := s.jwtService.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, err
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

	// 修复 #27：成功后再异步撤销旧 Session（避免阻塞）
	go func() {
		// 使用新的 context，避免原 context 取消导致撤销失败
		bgCtx := context.Background()
		if err := s.jwtService.RevokeSession(bgCtx, claims.SessionID); err != nil {
			s.logger.Warn("Failed to revoke old session after refresh",
				zap.String("old_session_id", claims.SessionID),
				zap.Error(err))
		}
	}()

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
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	return s.jwtService.RevokeSession(ctx, sessionID)
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
	meta.TenantID = tenantID
	meta.ActionID = req.ActionID
	meta.Reason = req.Reason
	meta.ExpectedRevision = req.ExpectedRevision
	result, err := s.userRepo.UpdateProfileAtomic(ctx, userID, email, meta)
	if err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, userID)
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
	if len(newPassword) < 8 {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "new_password must be at least 8 characters")
	}
	meta.TenantID = tenantID
	return s.userRepo.ChangePasswordAtomic(ctx, userID, currentPassword, newPassword, meta)
}

// GetUserSettings 获取当前用户某类偏好设置；没有保存过时返回服务端默认值。
func (s *AuthService) GetUserSettings(ctx context.Context, tenantID string, userID uuid.UUID, category string) (*UserSettingsResponse, error) {
	category = normalizeSettingsCategory(category)
	if category == "" {
		return nil, errors.New(errors.ErrCodeInvalidParameter, "invalid settings category")
	}
	values := defaultUserSettings(category)
	if s.settingsRepo == nil {
		return &UserSettingsResponse{Category: category, Settings: values, Revision: 0}, nil
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
	stored, err := s.settingsRepo.SaveCommand(ctx, repository.UserSettingsCommand{
		TenantID: tenantID, UserID: userID, Username: command.Username, Category: category, Settings: merged,
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

// GetOIDCAuthURL 获取 OIDC 认证 URL
func (s *AuthService) GetOIDCAuthURL(state string) string {
	if s.oidcProvider == nil {
		return ""
	}
	return s.oidcProvider.GetAuthURL(state)
}

// HandleOIDCCallback 处理 OIDC 回调（修复 #A13：角色持久化）
func (s *AuthService) HandleOIDCCallback(ctx context.Context, code, tenantID string, meta repository.UserCommandMetadata) (*LoginResponse, error) {
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

	// 映射 OIDC 角色到本地角色
	clientID := s.getOIDCClientID()
	oidcRoles := claims.GetRoles(clientID)
	roles := s.mapOIDCRoles(oidcRoles)
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
	}, nil
}

// ValidateToken 验证 Token
func (s *AuthService) ValidateToken(tokenString string) (*model.Claims, error) {
	return s.jwtService.ValidateAccessToken(tokenString)
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

	return permissions
}

// mapOIDCRoles 映射 OIDC 角色到内部角色
func (s *AuthService) mapOIDCRoles(oidcRoles []string) []string {
	// 映射 Keycloak 角色到内部角色
	roleMapping := map[string]string{
		"traffic-admin":   "admin",
		"traffic-analyst": "analyst",
		"traffic-viewer":  "viewer",
		"admin":           "admin",
		"analyst":         "analyst",
		"viewer":          "viewer",
	}

	roleSet := make(map[string]bool)
	for _, oidcRole := range oidcRoles {
		if mappedRole, ok := roleMapping[strings.ToLower(oidcRole)]; ok {
			roleSet[mappedRole] = true
		}
	}

	roles := make([]string, 0, len(roleSet))
	for role := range roleSet {
		roles = append(roles, role)
	}

	// 默认角色：如果没有映射到任何角色，使用 viewer
	if len(roles) == 0 {
		roles = append(roles, "viewer")
	}
	sort.Strings(roles)

	return roles
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
