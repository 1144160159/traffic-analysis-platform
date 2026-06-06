////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/service/token_service.go
// 完整修复版 v3：
// 1. 修复 #A9：Token创建使用数据库唯一约束防止竞态条件
// 2. 修复 #A10：改进 goroutine 管理
// 3. 修复 #A6：完整的 UpdateToken 方法
// 4. 保留所有原有代码（900+行完整）
////////////////////////////////////////////////////////////////////////////////

package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/security"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/audit"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// TokenService API Token 服务
type TokenService struct {
	tokenRepo   *repository.TokenRepository
	tokenHasher *security.TokenHasher
	auditLogger *audit.Logger
	logger      *zap.Logger

	// 配置
	maxTokensPerTenant int
	defaultTTL         time.Duration

	// 修复 #A10：有界工作池
	usageUpdateChan chan uuid.UUID
	usageUpdateWg   sync.WaitGroup
	shutdownOnce    sync.Once
	shutdownChan    chan struct{}
}

// TokenServiceConfig Token 服务配置
type TokenServiceConfig struct {
	MaxTokensPerTenant int           `env:"MAX_TOKENS_PER_TENANT" envDefault:"100"`
	DefaultTTL         time.Duration `env:"DEFAULT_TOKEN_TTL" envDefault:"8760h"` // 1 year
}

// NewTokenService 创建 Token 服务（修复 #A10：初始化工作池）
func NewTokenService(
	tokenRepo *repository.TokenRepository,
	auditLogger *audit.Logger,
	logger *zap.Logger,
	cfg TokenServiceConfig,
) *TokenService {
	if cfg.MaxTokensPerTenant <= 0 {
		cfg.MaxTokensPerTenant = 100
	}
	if cfg.DefaultTTL <= 0 {
		cfg.DefaultTTL = 8760 * time.Hour
	}

	s := &TokenService{
		tokenRepo:          tokenRepo,
		tokenHasher:        security.NewTokenHasher(),
		auditLogger:        auditLogger,
		logger:             logger,
		maxTokensPerTenant: cfg.MaxTokensPerTenant,
		defaultTTL:         cfg.DefaultTTL,
		usageUpdateChan:    make(chan uuid.UUID, 1000),
		shutdownChan:       make(chan struct{}),
	}

	// 修复 #A10：启动有界工作池（10个worker）
	for i := 0; i < 10; i++ {
		s.usageUpdateWg.Add(1)
		go s.usageUpdateWorker()
	}

	return s
}

// Shutdown 关闭服务（修复 #A10：优雅关闭）
func (s *TokenService) Shutdown() {
	s.shutdownOnce.Do(func() {
		close(s.shutdownChan)
		close(s.usageUpdateChan)
		s.usageUpdateWg.Wait()
		s.logger.Info("Token service shutdown completed")
	})
}

// usageUpdateWorker 使用统计更新工作协程（修复 #A10）
func (s *TokenService) usageUpdateWorker() {
	defer s.usageUpdateWg.Done()

	ctx := context.Background()
	for {
		select {
		case <-s.shutdownChan:
			return
		case tokenID, ok := <-s.usageUpdateChan:
			if !ok {
				return
			}
			if err := s.tokenRepo.UpdateUsageStats(ctx, tokenID); err != nil {
				s.logger.Warn("Failed to update token usage stats",
					zap.String("token_id", tokenID.String()),
					zap.Error(err))
			}
		}
	}
}

// CreateTokenRequest 创建 Token 请求
type CreateTokenRequest struct {
	TenantID    string            `json:"tenant_id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Scopes      []string          `json:"scopes"`
	ExpiresIn   *time.Duration    `json:"expires_in,omitempty"`
	ProbeID     string            `json:"probe_id,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedBy   uuid.UUID         `json:"-"`
}

// CreateTokenResponse 创建 Token 响应
type CreateTokenResponse struct {
	TokenID     uuid.UUID  `json:"token_id"`
	Token       string     `json:"token"`
	TokenPrefix string     `json:"token_prefix"`
	Name        string     `json:"name"`
	Scopes      []string   `json:"scopes"`
	ProbeID     string     `json:"probe_id,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// CreateToken 创建新的 API Token（修复 #A9：使用数据库唯一约束）
func (s *TokenService) CreateToken(ctx context.Context, req *CreateTokenRequest) (*CreateTokenResponse, error) {
	// 验证请求
	if req.TenantID == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "tenant_id is required")
	}
	if req.Name == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "name is required")
	}
	if len(req.Scopes) == 0 {
		return nil, errors.New(errors.ErrCodeMissingParameter, "at least one scope is required")
	}

	// 验证 Scopes
	validScopes, invalidScopes := model.ValidateScopes(req.Scopes)
	if len(invalidScopes) > 0 {
		s.logger.Warn("Invalid scopes provided",
			zap.Strings("invalid_scopes", invalidScopes))
		return nil, errors.Newf(errors.ErrCodeInvalidParameter, "invalid scopes: %v", invalidScopes)
	}

	// 修复 #A9：先检查配额（仍需检查，但允许轻微超配）
	count, err := s.tokenRepo.GetActiveTokensByTenant(ctx, req.TenantID)
	if err != nil {
		s.logger.Error("Failed to get token count", zap.Error(err))
		// 继续执行，不因统计失败而阻止创建
	} else if count >= int64(s.maxTokensPerTenant) {
		s.recordAuditFailure(ctx, req, "quota_exceeded")
		return nil, errors.Newf(errors.ErrCodeQuotaExceeded, "token limit exceeded: max %d tokens per tenant", s.maxTokensPerTenant)
	}

	// 确定 Token 类型
	tokenType := model.TokenTypeAPI
	if req.ProbeID != "" {
		tokenType = model.TokenTypeProbe
	}

	// 生成明文 Token 和前缀
	plainToken, err := s.tokenHasher.GenerateAPIKey(string(tokenType))
	if err != nil {
		s.logger.Error("Failed to generate token", zap.Error(err))
		s.recordAuditFailure(ctx, req, "token_generation_failed")
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to generate token")
	}

	// 哈希 Token
	tokenHash, err := s.tokenHasher.HashToken(plainToken)
	if err != nil {
		s.logger.Error("Failed to hash token", zap.Error(err))
		s.recordAuditFailure(ctx, req, "token_hash_failed")
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to hash token")
	}

	// 计算过期时间
	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		t := time.Now().Add(*req.ExpiresIn)
		expiresAt = &t
	} else if s.defaultTTL > 0 {
		t := time.Now().Add(s.defaultTTL)
		expiresAt = &t
	}

	// 转换 Metadata
	var metadata model.JSONMap
	if req.Metadata != nil {
		metadata = make(model.JSONMap)
		for k, v := range req.Metadata {
			metadata[k] = v
		}
	}

	// 构建 Token 对象
	token := &model.APIToken{
		TenantID:    req.TenantID,
		Name:        req.Name,
		Description: req.Description,
		TokenType:   tokenType,
		TokenHash:   tokenHash,
		TokenPrefix: "",
		Scopes:      model.StringSlice(validScopes),
		Status:      model.TokenStatusActive,
		ExpiresAt:   expiresAt,
		CreatedBy:   &req.CreatedBy,
		ProbeID:     req.ProbeID,
		Metadata:    metadata,
	}

	// 修复 #A9：创建 Token（数据库唯一约束会防止重复）
	err = s.tokenRepo.Create(ctx, token)
	if err != nil {
		// 检查是否是唯一约束冲突
		if isUniqueViolation(err) {
			s.logger.Warn("Token name already exists",
				zap.String("tenant_id", req.TenantID),
				zap.String("name", req.Name))
			s.recordAuditFailure(ctx, req, "duplicate_name")
			return nil, errors.Newf(errors.ErrCodeInvalidParameter, "token name already exists: %s", req.Name)
		}

		s.logger.Error("Failed to create token",
			zap.String("tenant_id", req.TenantID),
			zap.String("name", req.Name),
			zap.Error(err))
		s.recordAuditFailure(ctx, req, err.Error())
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to create token")
	}

	// 记录成功审计日志
	s.recordAuditSuccess(ctx, req, token.TokenID.String())

	s.logger.Info("API Token created",
		zap.String("token_id", token.TokenID.String()),
		zap.String("tenant_id", req.TenantID),
		zap.String("name", req.Name),
		zap.Strings("scopes", validScopes),
		zap.String("probe_id", req.ProbeID))

	return &CreateTokenResponse{
		TokenID:     token.TokenID,
		Token:       plainToken,
		TokenPrefix: "",
		Name:        token.Name,
		Scopes:      validScopes,
		ProbeID:     req.ProbeID,
		ExpiresAt:   expiresAt,
		CreatedAt:   token.CreatedAt,
	}, nil
}

// isUniqueViolation 检查是否是唯一约束冲突
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL unique violation error code: 23505
	errMsg := err.Error()
	return contains(errMsg, "duplicate key") || contains(errMsg, "unique constraint") || contains(errMsg, "23505")
}

// contains 字符串包含检查（不区分大小写）
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(indexOfIgnoreCase(s, substr) >= 0))
}

// indexOfIgnoreCase 不区分大小写查找子串
func indexOfIgnoreCase(s, substr string) int {
	sLower := toLower(s)
	substrLower := toLower(substr)
	for i := 0; i <= len(sLower)-len(substrLower); i++ {
		if sLower[i:i+len(substrLower)] == substrLower {
			return i
		}
	}
	return -1
}

// toLower 简单转小写
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// CreateProbeToken 创建探针专用 Token
func (s *TokenService) CreateProbeToken(ctx context.Context, tenantID, probeID, name string, createdBy uuid.UUID) (*CreateTokenResponse, error) {
	if probeID == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "probe_id is required")
	}

	if name == "" {
		name = "Probe Token - " + probeID
	}

	req := &CreateTokenRequest{
		TenantID:  tenantID,
		Name:      name,
		Scopes:    model.DefaultProbeScopes,
		ProbeID:   probeID,
		CreatedBy: createdBy,
	}

	return s.CreateToken(ctx, req)
}

// GetToken 获取 Token 信息
func (s *TokenService) GetToken(ctx context.Context, tenantID string, tokenID uuid.UUID) (*model.APIToken, error) {
	token, err := s.tokenRepo.GetByID(ctx, tokenID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token")
	}

	if token == nil {
		return nil, errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	if token.TenantID != tenantID {
		return nil, errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	token.TokenHash = ""
	return token, nil
}

// ListTokens 列出租户的所有 Token
func (s *TokenService) ListTokens(ctx context.Context, tenantID string, limit, offset int) ([]*model.APIToken, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	tokens, total, err := s.tokenRepo.ListByTenant(ctx, tenantID, limit, offset)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to list tokens")
	}

	for _, token := range tokens {
		token.TokenHash = ""
	}

	return tokens, total, nil
}

// UpdateTokenRequest 更新 Token 请求
type UpdateTokenRequest struct {
	Name        *string    `json:"name,omitempty"`
	Description *string    `json:"description,omitempty"`
	Scopes      []string   `json:"scopes,omitempty"`
	IPWhitelist []string   `json:"ip_whitelist,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// UpdateToken 更新 Token（修复 #A6：完整实现）
func (s *TokenService) UpdateToken(ctx context.Context, tenantID string, tokenID uuid.UUID, req *UpdateTokenRequest, updatedBy uuid.UUID) (*model.APIToken, error) {
	// 获取现有 Token
	token, err := s.tokenRepo.GetByID(ctx, tokenID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token")
	}

	if token == nil {
		return nil, errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	if token.TenantID != tenantID {
		return nil, errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	// 记录旧值（用于审计）
	oldValues := map[string]interface{}{
		"name":         token.Name,
		"description":  token.Description,
		"scopes":       model.ScopesToList(token.Scopes),
		"ip_whitelist": model.ScopesToList(token.IPWhitelist),
		"expires_at":   token.ExpiresAt,
	}

	// 更新字段
	updated := false

	if req.Name != nil && *req.Name != token.Name {
		// 检查新名称是否已存在
		exists, err := s.tokenRepo.ExistsByName(ctx, tenantID, *req.Name)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to check token name")
		}
		if exists {
			return nil, errors.Newf(errors.ErrCodeInvalidParameter, "token name already exists: %s", *req.Name)
		}
		token.Name = *req.Name
		updated = true
	}

	if req.Description != nil && *req.Description != token.Description {
		token.Description = *req.Description
		updated = true
	}

	if len(req.Scopes) > 0 {
		validScopes, invalidScopes := model.ValidateScopes(req.Scopes)
		if len(invalidScopes) > 0 {
			return nil, errors.Newf(errors.ErrCodeInvalidParameter, "invalid scopes: %v", invalidScopes)
		}
		token.Scopes = model.StringSlice(validScopes)
		updated = true
	}

	if len(req.IPWhitelist) > 0 {
		token.IPWhitelist = model.StringSlice(req.IPWhitelist)
		updated = true
	}

	if req.ExpiresAt != nil {
		token.ExpiresAt = req.ExpiresAt
		updated = true
	}

	if !updated {
		return token, nil
	}

	// 更新到数据库
	if err := s.tokenRepo.Update(ctx, token); err != nil {
		s.logger.Error("Failed to update token",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to update token")
	}

	// 记录审计日志
	if s.auditLogger != nil {
		newValues := map[string]interface{}{
			"name":         token.Name,
			"description":  token.Description,
			"scopes":       model.ScopesToList(token.Scopes),
			"ip_whitelist": model.ScopesToList(token.IPWhitelist),
			"expires_at":   token.ExpiresAt,
		}

		s.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    audit.EventTypeTokenCreate,
			TenantID:     tenantID,
			UserID:       updatedBy.String(),
			Action:       "update_token",
			ResourceType: "api_token",
			ResourceID:   tokenID.String(),
			OldValue:     oldValues,
			NewValue:     newValues,
			Result:       audit.ResultSuccess,
		})
	}

	s.logger.Info("API Token updated",
		zap.String("token_id", tokenID.String()),
		zap.String("updated_by", updatedBy.String()))

	token.TokenHash = ""
	return token, nil
}

// RevokeToken 撤销 Token
func (s *TokenService) RevokeToken(ctx context.Context, tenantID string, tokenID uuid.UUID, revokedBy uuid.UUID) error {
	token, err := s.tokenRepo.GetByID(ctx, tokenID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token")
	}

	if token == nil {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	if token.TenantID != tenantID {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	if err := s.tokenRepo.Revoke(ctx, tokenID, "user_revoked"); err != nil {
		s.logger.Error("Failed to revoke token",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		s.recordRevokeAuditFailure(ctx, tenantID, tokenID.String(), revokedBy, err.Error())
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to revoke token")
	}

	s.recordRevokeAuditSuccess(ctx, tenantID, token.Name, tokenID.String(), revokedBy)

	s.logger.Info("API Token revoked",
		zap.String("token_id", tokenID.String()),
		zap.String("tenant_id", tenantID),
		zap.String("revoked_by", revokedBy.String()))

	return nil
}

// DeleteToken 删除 Token
func (s *TokenService) DeleteToken(ctx context.Context, tenantID string, tokenID uuid.UUID, deletedBy uuid.UUID) error {
	token, err := s.tokenRepo.GetByID(ctx, tokenID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token")
	}

	if token == nil {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	if token.TenantID != tenantID {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	if err := s.tokenRepo.Delete(ctx, tokenID); err != nil {
		s.logger.Error("Failed to delete token",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to delete token")
	}

	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    audit.EventTypeTokenRevoke,
			TenantID:     tenantID,
			UserID:       deletedBy.String(),
			Action:       "delete_token",
			ResourceType: "api_token",
			ResourceID:   tokenID.String(),
			Detail: map[string]interface{}{
				"name":     token.Name,
				"probe_id": token.ProbeID,
			},
			Result: audit.ResultSuccess,
		})
	}

	s.logger.Info("API Token deleted",
		zap.String("token_id", tokenID.String()),
		zap.String("tenant_id", tenantID),
		zap.String("deleted_by", deletedBy.String()))

	return nil
}

// UpdateTokenScopes 更新 Token 权限
func (s *TokenService) UpdateTokenScopes(ctx context.Context, tenantID string, tokenID uuid.UUID, scopes []string, updatedBy uuid.UUID) error {
	validScopes, invalidScopes := model.ValidateScopes(scopes)
	if len(invalidScopes) > 0 {
		return errors.Newf(errors.ErrCodeInvalidParameter, "invalid scopes: %v", invalidScopes)
	}

	token, err := s.tokenRepo.GetByID(ctx, tokenID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token")
	}

	if token == nil {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	if token.TenantID != tenantID {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	if err := s.tokenRepo.UpdateScopes(ctx, tokenID, validScopes); err != nil {
		s.logger.Error("Failed to update token scopes",
			zap.String("token_id", tokenID.String()),
			zap.Error(err))
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to update token scopes")
	}

	if s.auditLogger != nil {
		oldScopes := []string{}
		for _, scope := range token.Scopes {
			oldScopes = append(oldScopes, scope)
		}

		s.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    audit.EventTypeTokenCreate,
			TenantID:     tenantID,
			UserID:       updatedBy.String(),
			Action:       "update_token_scopes",
			ResourceType: "api_token",
			ResourceID:   tokenID.String(),
			OldValue:     oldScopes,
			NewValue:     validScopes,
			Result:       audit.ResultSuccess,
		})
	}

	s.logger.Info("API Token scopes updated",
		zap.String("token_id", tokenID.String()),
		zap.Strings("new_scopes", validScopes))

	return nil
}

// ValidateToken 验证 Token（修复 #A10：使用工作池）
func (s *TokenService) ValidateToken(ctx context.Context, rawToken string) (*model.APIToken, error) {
	tokenHash, err := s.tokenHasher.HashToken(rawToken)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to hash token")
	}

	token, err := s.tokenRepo.ValidateToken(ctx, tokenHash)
	if err != nil {
		return nil, err
	}

	// 修复 #A10：使用工作池更新使用统计
	select {
	case s.usageUpdateChan <- token.TokenID:
		// 成功发送到工作池
	default:
		// 队列满，记录警告但不阻塞
		s.logger.Warn("Usage update queue full, dropping update",
			zap.String("token_id", token.TokenID.String()))
	}

	return token, nil
}

// RegenerateToken 重新生成 Token
func (s *TokenService) RegenerateToken(ctx context.Context, tenantID string, tokenID uuid.UUID, regeneratedBy uuid.UUID) (*CreateTokenResponse, error) {
	oldToken, err := s.tokenRepo.GetByID(ctx, tokenID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get old token")
	}

	if oldToken == nil {
		return nil, errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	if oldToken.TenantID != tenantID {
		return nil, errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	scopes := []string{}
	for _, scope := range oldToken.Scopes {
		scopes = append(scopes, scope)
	}

	var expiresIn *time.Duration
	if oldToken.ExpiresAt != nil {
		d := time.Until(*oldToken.ExpiresAt)
		if d < 0 {
			d = s.defaultTTL
		}
		expiresIn = &d
	}

	newTokenResp, err := s.CreateToken(ctx, &CreateTokenRequest{
		TenantID:    oldToken.TenantID,
		Name:        oldToken.Name + " (regenerated)",
		Description: oldToken.Description,
		Scopes:      scopes,
		ExpiresIn:   expiresIn,
		ProbeID:     oldToken.ProbeID,
		CreatedBy:   regeneratedBy,
	})
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to create new token")
	}

	if err := s.tokenRepo.Revoke(ctx, tokenID, "regenerated"); err != nil {
		s.logger.Warn("Failed to revoke old token after regeneration",
			zap.String("old_token_id", tokenID.String()),
			zap.Error(err))
	}

	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    audit.EventTypeTokenCreate,
			TenantID:     tenantID,
			UserID:       regeneratedBy.String(),
			Action:       "regenerate_token",
			ResourceType: "api_token",
			ResourceID:   newTokenResp.TokenID.String(),
			Detail: map[string]interface{}{
				"old_token_id": tokenID.String(),
				"new_token_id": newTokenResp.TokenID.String(),
			},
			Result: audit.ResultSuccess,
		})
	}

	s.logger.Info("API Token regenerated",
		zap.String("old_token_id", tokenID.String()),
		zap.String("new_token_id", newTokenResp.TokenID.String()))

	return newTokenResp, nil
}

// CleanupExpiredTokens 清理过期的 Token
func (s *TokenService) CleanupExpiredTokens(ctx context.Context) (int64, error) {
	count, err := s.tokenRepo.CleanupExpiredTokens(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		s.logger.Error("Failed to cleanup expired tokens", zap.Error(err))
		return 0, err
	}

	if count > 0 {
		s.logger.Info("Cleaned up expired tokens", zap.Int64("count", count))
	}

	return count, nil
}

// GetAvailableScopes 获取可用的 Scope 列表
func (s *TokenService) GetAvailableScopes() []ScopeInfo {
	allScopes := model.GetAllScopeInfos()
	result := make([]ScopeInfo, len(allScopes))
	for i, info := range allScopes {
		result[i] = ScopeInfo{
			Name:        info.Name,
			Description: info.Description,
			Category:    info.Category,
		}
	}
	return result
}

// GetProbeScopes 获取探针相关 scopes
func (s *TokenService) GetProbeScopes() []ScopeInfo {
	probeScopes := model.GetProbeScopes()
	result := make([]ScopeInfo, len(probeScopes))
	for i, info := range probeScopes {
		result[i] = ScopeInfo{
			Name:        info.Name,
			Description: info.Description,
			Category:    info.Category,
		}
	}
	return result
}

// ScopeInfo Scope 信息
type ScopeInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// =============================================================================
// 审计日志辅助函数
// =============================================================================

func (s *TokenService) recordAuditSuccess(ctx context.Context, req *CreateTokenRequest, tokenID string) {
	if s.auditLogger == nil {
		return
	}

	s.auditLogger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeTokenCreate,
		TenantID:     req.TenantID,
		UserID:       req.CreatedBy.String(),
		Action:       "create_token",
		ResourceType: "api_token",
		ResourceID:   tokenID,
		Detail: map[string]interface{}{
			"name":     req.Name,
			"scopes":   req.Scopes,
			"probe_id": req.ProbeID,
		},
		Result: audit.ResultSuccess,
	})
}

func (s *TokenService) recordAuditFailure(ctx context.Context, req *CreateTokenRequest, errorMsg string) {
	if s.auditLogger == nil {
		return
	}

	s.auditLogger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeTokenCreate,
		TenantID:     req.TenantID,
		UserID:       req.CreatedBy.String(),
		Action:       "create_token_failed",
		ResourceType: "api_token",
		Result:       audit.ResultFailure,
		ErrorMsg:     errorMsg,
		Detail: map[string]interface{}{
			"name":     req.Name,
			"scopes":   req.Scopes,
			"probe_id": req.ProbeID,
		},
	})
}

func (s *TokenService) recordRevokeAuditSuccess(ctx context.Context, tenantID, tokenName, tokenID string, revokedBy uuid.UUID) {
	if s.auditLogger == nil {
		return
	}

	s.auditLogger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeTokenRevoke,
		TenantID:     tenantID,
		UserID:       revokedBy.String(),
		Action:       "revoke_token",
		ResourceType: "api_token",
		ResourceID:   tokenID,
		Detail: map[string]interface{}{
			"name": tokenName,
		},
		Result: audit.ResultSuccess,
	})
}

func (s *TokenService) recordRevokeAuditFailure(ctx context.Context, tenantID, tokenID string, revokedBy uuid.UUID, errorMsg string) {
	if s.auditLogger == nil {
		return
	}

	s.auditLogger.Log(ctx, &audit.AuditEvent{
		EventType:    audit.EventTypeTokenRevoke,
		TenantID:     tenantID,
		UserID:       revokedBy.String(),
		Action:       "revoke_token_failed",
		ResourceType: "api_token",
		ResourceID:   tokenID,
		Result:       audit.ResultFailure,
		ErrorMsg:     errorMsg,
	})
}
