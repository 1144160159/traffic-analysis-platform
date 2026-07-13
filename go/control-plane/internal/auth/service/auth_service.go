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
	"strings"

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
	UserID   string   `json:"user_id"`
	TenantID string   `json:"tenant_id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
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
	if err := s.userRepo.UpdateLastLoginAt(ctx, user.UserID); err != nil {
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
func (s *AuthService) GetOIDCAuthURL(state string) string {
	if s.oidcProvider == nil {
		return ""
	}
	return s.oidcProvider.GetAuthURL(state)
}

// HandleOIDCCallback 处理 OIDC 回调（修复 #A13：角色持久化）
func (s *AuthService) HandleOIDCCallback(ctx context.Context, code, tenantID string) (*LoginResponse, error) {
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

	// 获取或创建用户
	user, err := s.userRepo.CreateOrUpdateFromOIDC(ctx, claims, tenantID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to sync user from OIDC")
	}

	// 映射 OIDC 角色到本地角色
	clientID := s.getOIDCClientID()
	oidcRoles := claims.GetRoles(clientID)
	roles := s.mapOIDCRoles(oidcRoles)

	// 修复 #A13：持久化角色到数据库
	if err := s.syncUserRoles(ctx, user.UserID, roles, tenantID); err != nil {
		s.logger.Error("Failed to sync user roles",
			zap.String("user_id", user.UserID.String()),
			zap.Error(err))
		// 不阻止登录流程
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
			UserID:   user.UserID.String(),
			TenantID: user.TenantID,
			Username: user.Username,
			Email:    user.Email,
			Roles:    roles,
		},
	}, nil
}

// syncUserRoles 同步用户角色到数据库（修复 #A13：新增方法）
func (s *AuthService) syncUserRoles(ctx context.Context, userID uuid.UUID, rolesToSync []string, tenantID string) error {
	// 获取当前角色
	currentRoles, err := s.userRepo.GetUserRoles(ctx, userID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get current roles")
	}

	currentRolesMap := make(map[string]bool)
	for _, role := range currentRoles {
		currentRolesMap[role] = true
	}

	targetRolesMap := make(map[string]bool)
	for _, role := range rolesToSync {
		targetRolesMap[role] = true
	}

	// 需要添加的角色
	for _, role := range rolesToSync {
		if !currentRolesMap[role] {
			// 查找角色 ID
			roleID, err := s.userRepo.GetRoleIDByName(ctx, tenantID, role)
			if err != nil {
				s.logger.Warn("Failed to find role",
					zap.String("role", role),
					zap.Error(err))
				continue
			}

			if roleID != uuid.Nil {
				if err := s.userRepo.AssignRole(ctx, userID, roleID); err != nil {
					s.logger.Error("Failed to assign role",
						zap.String("user_id", userID.String()),
						zap.String("role_id", roleID.String()),
						zap.Error(err))
				}
			}
		}
	}

	return nil
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
