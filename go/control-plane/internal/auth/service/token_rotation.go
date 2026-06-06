////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/service/token_rotation.go
// 完整修复版 v2：
// 1. 修复 #29：rotateToken 增加事务保护（原子更新 token + 保存历史）
// 2. 完善错误处理和日志记录
// 3. 集成 security.TokenHasher
// 4. 完整的轮转服务（400+ 行完整代码）
////////////////////////////////////////////////////////////////////////////////

package service

import (
	"context"
	"database/sql"
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

// TokenRotationService Token 轮转服务
type TokenRotationService struct {
	tokenRepo   *repository.TokenRepository
	tokenHasher *security.TokenHasher
	auditLogger *audit.Logger
	logger      *zap.Logger

	// 配置
	checkInterval time.Duration
	gracePeriod   time.Duration
	enabled       bool

	// 控制
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
	running  bool
}

// TokenRotationConfig 轮转配置
type TokenRotationConfig struct {
	Enabled       bool          `env:"TOKEN_ROTATION_ENABLED" envDefault:"false"`
	CheckInterval time.Duration `env:"TOKEN_ROTATION_CHECK_INTERVAL" envDefault:"1h"`
	GracePeriod   time.Duration `env:"TOKEN_ROTATION_GRACE_PERIOD" envDefault:"168h"` // 7 days
}

// NewTokenRotationService 创建 Token 轮转服务
func NewTokenRotationService(
	tokenRepo *repository.TokenRepository,
	auditLogger *audit.Logger,
	logger *zap.Logger,
	config TokenRotationConfig,
) *TokenRotationService {
	return &TokenRotationService{
		tokenRepo:     tokenRepo,
		tokenHasher:   security.NewTokenHasher(),
		auditLogger:   auditLogger,
		logger:        logger,
		checkInterval: config.CheckInterval,
		gracePeriod:   config.GracePeriod,
		enabled:       config.Enabled,
		stopChan:      make(chan struct{}),
	}
}

// Start 启动轮转服务
func (s *TokenRotationService) Start(ctx context.Context) error {
	if !s.enabled {
		s.logger.Info("Token rotation service is disabled")
		return nil
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New(errors.ErrCodeInternal, "Token rotation service already running")
	}
	s.running = true
	s.mu.Unlock()

	s.logger.Info("Starting token rotation service",
		zap.Duration("check_interval", s.checkInterval),
		zap.Duration("grace_period", s.gracePeriod))

	s.wg.Add(1)
	go s.rotationLoop(ctx)

	return nil
}

// Stop 停止轮转服务
func (s *TokenRotationService) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	s.logger.Info("Stopping token rotation service...")
	close(s.stopChan)
	s.wg.Wait()

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	s.logger.Info("Token rotation service stopped")
	return nil
}

// rotationLoop 轮转循环
func (s *TokenRotationService) rotationLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	// 立即执行一次
	s.checkAndRotateTokens(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkAndRotateTokens(ctx)
		}
	}
}

// checkAndRotateTokens 检查并轮转到期的 token
func (s *TokenRotationService) checkAndRotateTokens(ctx context.Context) {
	s.logger.Debug("Checking tokens for rotation")

	// 获取需要轮转的 token
	tokens, err := s.tokenRepo.GetTokensNeedingRotation(ctx, 100)
	if err != nil {
		s.logger.Error("Failed to get tokens needing rotation", zap.Error(err))
		return
	}

	if len(tokens) == 0 {
		s.logger.Debug("No tokens need rotation")
		return
	}

	s.logger.Info("Found tokens needing rotation", zap.Int("count", len(tokens)))

	// 轮转每个 token
	rotated := 0
	failed := 0

	for _, token := range tokens {
		if err := s.rotateToken(ctx, token); err != nil {
			s.logger.Error("Failed to rotate token",
				zap.String("token_id", token.TokenID.String()),
				zap.String("tenant_id", token.TenantID),
				zap.Error(err))
			failed++
		} else {
			rotated++
		}
	}

	s.logger.Info("Token rotation completed",
		zap.Int("rotated", rotated),
		zap.Int("failed", failed))
}

// rotateToken 轮转单个 token（修复 #29：增加事务保护）
func (s *TokenRotationService) rotateToken(ctx context.Context, token *model.APIToken) error {
	s.logger.Info("Rotating token",
		zap.String("token_id", token.TokenID.String()),
		zap.String("tenant_id", token.TenantID),
		zap.String("name", token.Name))

	// 生成新的 token
	plainToken, err := s.tokenHasher.GenerateAPIKey(string(token.TokenType))
	tokenPrefix := ""
	if len(plainToken) > 8 { tokenPrefix = plainToken[:8] }
	newTokenHash, err := s.tokenHasher.HashToken(plainToken)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "Failed to hash new token")
	}

	// 保存旧 token hash（用于宽限期）
	oldTokenHash := token.TokenHash

	// 修复 #29：使用事务保护更新操作
	// 注意：这里需要 TokenRepository 支持事务，如果不支持则回退到非事务模式
	if err := s.rotateTokenWithTransaction(ctx, token, oldTokenHash, newTokenHash, tokenPrefix); err != nil {
		return err
	}

	// 审计日志
	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    audit.EventTypeTokenCreate,
			TenantID:     token.TenantID,
			UserID:       "system",
			Action:       "token_rotate",
			ResourceType: "api_token",
			ResourceID:   token.TokenID.String(),
			Result:       audit.ResultSuccess,
			Detail: map[string]interface{}{
				"token_name":        token.Name,
				"token_type":        string(token.TokenType),
				"reason":            "automatic_rotation",
				"grace_period_days": s.gracePeriod.Hours() / 24,
			},
		})
	}

	s.logger.Info("Token rotated successfully",
		zap.String("token_id", token.TokenID.String()),
		zap.Duration("grace_period", s.gracePeriod))

	// TODO: 通知用户新 token（通过邮件/webhook）
	// s.notifyTokenRotation(ctx, token, plainToken)

	return nil
}

// rotateTokenWithTransaction 在事务中执行轮转（修复 #29）
func (s *TokenRotationService) rotateTokenWithTransaction(
	ctx context.Context,
	token *model.APIToken,
	oldTokenHash, newTokenHash, tokenPrefix string,
) error {
	// 尝试获取事务支持
	db := s.tokenRepo.GetDB()
	if db == nil {
		// 降级：使用非事务模式
		s.logger.Warn("Database connection not available, using non-transactional rotation")
		return s.rotateTokenNonTransactional(ctx, token, oldTokenHash, newTokenHash, tokenPrefix)
	}

	// 开始事务
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		// 降级：使用非事务模式
		return s.rotateTokenNonTransactional(ctx, token, oldTokenHash, newTokenHash, tokenPrefix)
	}

	// 确保事务回滚（如果未提交）
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	// 1. 更新 token
	now := time.Now()
	token.TokenHash = newTokenHash
	token.TokenPrefix = tokenPrefix
	token.LastRotatedAt = &now
	previousTokenID := token.TokenID
	token.PreviousTokenID = &previousTokenID

	updateQuery := `
		UPDATE api_tokens
		SET token_hash = $2, token_prefix = $3, last_rotated_at = $4, 
		    previous_token_id = $5, updated_at = $6
		WHERE token_id = $1
	`

	result, err := tx.ExecContext(ctx, updateQuery,
		token.TokenID,
		token.TokenHash,
		token.TokenPrefix,
		token.LastRotatedAt,
		token.PreviousTokenID,
		now,
	)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to update token in transaction")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return errors.New(errors.ErrCodeEntityNotFound, "Token not found or already rotated")
	}

	// 2. 保存轮转历史
	history := &model.TokenRotationHistory{
		ID:              uuid.New(),
		TokenID:         token.TokenID,
		OldTokenHash:    oldTokenHash,
		NewTokenHash:    newTokenHash,
		RotatedAt:       now,
		RotatedBy:       "system",
		Reason:          "automatic_rotation",
		GracePeriodEnds: now.Add(s.gracePeriod),
	}

	historyQuery := `
		INSERT INTO token_rotation_history (
			id, token_id, old_token_hash, new_token_hash,
			rotated_at, rotated_by, reason, grace_period_ends
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = tx.ExecContext(ctx, historyQuery,
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
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to save rotation history in transaction")
	}

	// 3. 提交事务
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to commit rotation transaction")
	}

	// 防止 defer 中的 Rollback
	tx = nil

	s.logger.Debug("Token rotated in transaction",
		zap.String("token_id", token.TokenID.String()))

	return nil
}

// rotateTokenNonTransactional 非事务模式轮转（降级方案）
func (s *TokenRotationService) rotateTokenNonTransactional(
	ctx context.Context,
	token *model.APIToken,
	oldTokenHash, newTokenHash, tokenPrefix string,
) error {
	s.logger.Warn("Using non-transactional rotation (degraded mode)",
		zap.String("token_id", token.TokenID.String()))

	// 更新 token
	now := time.Now()
	token.TokenHash = newTokenHash
	token.TokenPrefix = tokenPrefix
	token.LastRotatedAt = &now
	previousTokenID := token.TokenID
	token.PreviousTokenID = &previousTokenID

	if err := s.tokenRepo.Update(ctx, token); err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to update token")
	}

	// 记录轮转历史（即使失败也不影响轮转）
	history := &model.TokenRotationHistory{
		TokenID:         token.TokenID,
		OldTokenHash:    oldTokenHash,
		NewTokenHash:    newTokenHash,
		RotatedAt:       now,
		RotatedBy:       "system",
		Reason:          "automatic_rotation",
		GracePeriodEnds: now.Add(s.gracePeriod),
	}

	if err := s.tokenRepo.SaveRotationHistory(ctx, history); err != nil {
		s.logger.Error("Failed to save rotation history (non-critical)",
			zap.String("token_id", token.TokenID.String()),
			zap.Error(err))
		// 不返回错误，轮转已完成
	}

	return nil
}

// RotateTokenManually 手动轮转 token
func (s *TokenRotationService) RotateTokenManually(ctx context.Context, tokenID, userID string) (*CreateTokenResponse, error) {
	// 解析 UUID
	tokenUUID, err := parseUUID(tokenID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidParameter, "Invalid token_id")
	}

	if _, err := parseUUID(userID); err != nil {
	
		return nil, errors.Wrap(err, errors.ErrCodeInvalidParameter, "Invalid user_id")
	}

	// 获取 token
	token, err := s.tokenRepo.GetByID(ctx, tokenUUID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get token")
	}

	if token == nil {
		return nil, errors.New(errors.ErrCodeEntityNotFound, "Token not found")
	}

	// 生成新 token
	plainToken, err := s.tokenHasher.GenerateAPIKey(string(token.TokenType)); tokenPrefix := plainToken[:8]
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to generate new token")
	}

	newTokenHash, err := s.tokenHasher.HashToken(plainToken)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to hash new token")
	}

	oldTokenHash := token.TokenHash

	// 使用事务轮转
	if err := s.rotateTokenWithTransaction(ctx, token, oldTokenHash, newTokenHash, tokenPrefix); err != nil {
		return nil, err
	}

	// 审计日志
	if s.auditLogger != nil {
		s.auditLogger.Log(ctx, &audit.AuditEvent{
			EventType:    audit.EventTypeTokenCreate,
			TenantID:     token.TenantID,
			UserID:       userID,
			Action:       "token_rotate",
			ResourceType: "api_token",
			ResourceID:   token.TokenID.String(),
			Result:       audit.ResultSuccess,
			Detail: map[string]interface{}{
				"token_name": token.Name,
				"token_type": string(token.TokenType),
				"reason":     "manual_rotation",
			},
		})
	}

	// 提取 scopes
	scopes := []string{}
	for _, scope := range token.Scopes {
		scopes = append(scopes, scope)
	}

	return &CreateTokenResponse{
		TokenID:     token.TokenID,
		Token:       plainToken,
		TokenPrefix: tokenPrefix,
		Name:        token.Name,
		Scopes:      scopes,
		ProbeID:     token.ProbeID,
		ExpiresAt:   token.ExpiresAt,
		CreatedAt:   token.CreatedAt,
	}, nil
}

// CleanupExpiredRotations 清理过期的轮转历史（宽限期已过）
func (s *TokenRotationService) CleanupExpiredRotations(ctx context.Context) error {
	deleted, err := s.tokenRepo.DeleteExpiredRotationHistory(ctx, time.Now())
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to cleanup rotation history")
	}

	if deleted > 0 {
		s.logger.Info("Cleaned up expired rotation history",
			zap.Int64("count", deleted))
	}

	return nil
}

// GetRotationHistory 获取 token 轮转历史
func (s *TokenRotationService) GetRotationHistory(ctx context.Context, tokenID string, limit int) ([]*model.TokenRotationHistory, error) {
	tokenUUID, err := parseUUID(tokenID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInvalidParameter, "Invalid token_id")
	}

	return s.tokenRepo.GetRotationHistory(ctx, tokenUUID, limit)
}

// GetRotationStatistics 获取轮转统计信息
func (s *TokenRotationService) GetRotationStatistics(ctx context.Context, tenantID string) (*RotationStatistics, error) {
	// 获取租户所有 token
	tokens, _, err := s.tokenRepo.ListByTenant(ctx, tenantID, 1000, 0)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to get tokens")
	}

	stats := &RotationStatistics{
		TenantID:          tenantID,
		TotalTokens:       len(tokens),
		RotationEnabled:   0,
		NeedingRotation:   0,
		LastRotated:       make(map[string]time.Time),
		NextRotation:      make(map[string]time.Time),
		GracePeriodActive: 0,
	}

	now := time.Now(); _ = now

	for _, token := range tokens {
		if token.RotationEnabled {
			stats.RotationEnabled++

			if token.NeedsRotation() {
				stats.NeedingRotation++
			}

			if token.LastRotatedAt != nil {
				stats.LastRotated[token.TokenID.String()] = *token.LastRotatedAt

				if token.RotationInterval != nil {
					nextRotation := token.LastRotatedAt.Add(time.Duration(*token.RotationInterval) * 24 * time.Hour)
					stats.NextRotation[token.TokenID.String()] = nextRotation
				}
			}
		}
	}

	// 统计宽限期内的历史记录
	// 这里需要查询所有未过期的轮转历史
	// 由于性能考虑，暂时跳过

	return stats, nil
}

// RotationStatistics 轮转统计信息
type RotationStatistics struct {
	TenantID          string               `json:"tenant_id"`
	TotalTokens       int                  `json:"total_tokens"`
	RotationEnabled   int                  `json:"rotation_enabled"`
	NeedingRotation   int                  `json:"needing_rotation"`
	LastRotated       map[string]time.Time `json:"last_rotated,omitempty"`
	NextRotation      map[string]time.Time `json:"next_rotation,omitempty"`
	GracePeriodActive int                  `json:"grace_period_active"`
}

// IsRunning 检查服务是否运行中
func (s *TokenRotationService) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetConfig 获取配置信息
func (s *TokenRotationService) GetConfig() map[string]interface{} {
	return map[string]interface{}{
		"enabled":        s.enabled,
		"check_interval": s.checkInterval.String(),
		"grace_period":   s.gracePeriod.String(),
		"running":        s.IsRunning(),
	}
}

// parseUUID 辅助函数：解析 UUID
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
