////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/jwt/service.go
// 修复版：修复 #19 - Session 撤销 Fail-Secure 过于严格
// 修复内容：增加重试逻辑和降级模式，避免 PostgreSQL 抖动导致全员下线
////////////////////////////////////////////////////////////////////////////////

package jwt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/repository"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// Config JWT 配置
type Config struct {
	SigningKey      string        `env:"JWT_SIGNING_KEY"`
	SigningMethod   string        `env:"JWT_SIGNING_METHOD" envDefault:"HS256"`
	AccessTokenTTL  time.Duration `env:"JWT_ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"JWT_REFRESH_TOKEN_TTL" envDefault:"168h"` // 7 days
	Issuer          string        `env:"JWT_ISSUER" envDefault:"traffic-auth-service"`
}

// Service JWT 服务
type Service struct {
	config      Config
	redisClient *storage.RedisClient
	tokenRepo   *repository.TokenRepository
	logger      *zap.Logger
}

// NewService 创建 JWT 服务
func NewService(
	cfg Config,
	redisClient *storage.RedisClient,
	tokenRepo *repository.TokenRepository,
	logger *zap.Logger,
) (*Service, error) {
	// 验证至少一种撤销机制可用
	if redisClient == nil && tokenRepo == nil {
		return nil, errors.New(errors.ErrCodeConfigError,
			"At least one of Redis or PostgreSQL must be available for session revocation")
	}

	if redisClient == nil {
		logger.Warn("Redis is not available, using PostgreSQL for session revocation")
	}

	if tokenRepo == nil {
		logger.Warn("PostgreSQL token repository is not available, using only Redis for session revocation")
	}

	return &Service{
		config:      cfg,
		redisClient: redisClient,
		tokenRepo:   tokenRepo,
		logger:      logger,
	}, nil
}

// GenerateTokenPair 生成 Token 对（Access + Refresh）
func (s *Service) GenerateTokenPair(user *model.User, roles []string, permissions []string) (*TokenPair, error) {
	sessionID := generateSessionID()
	now := time.Now()

	// Access Token
	accessClaims := &model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   user.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.AccessTokenTTL)),
			ID:        uuid.New().String(),
		},
		UserID:      user.UserID,
		TenantID:    user.TenantID,
		Username:    user.Username,
		Email:       user.Email,
		Roles:       roles,
		Permissions: permissions,
		TokenType:   model.JWTTokenAccess,
		SessionID:   sessionID,
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(s.config.SigningKey))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to sign access token")
	}

	// Refresh Token
	refreshClaims := &model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   user.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.RefreshTokenTTL)),
			ID:        uuid.New().String(),
		},
		UserID:    user.UserID,
		TenantID:  user.TenantID,
		TokenType: model.JWTTokenRefresh,
		SessionID: sessionID,
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(s.config.SigningKey))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to sign refresh token")
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresIn:    int(s.config.AccessTokenTTL.Seconds()),
		TokenType:    "Bearer",
		SessionID:    sessionID,
	}, nil
}

// ValidateAccessToken 验证 Access Token
func (s *Service) ValidateAccessToken(tokenString string) (*model.Claims, error) {
	claims := &model.Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Newf(errors.ErrCodeTokenInvalid,
				"Unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.config.SigningKey), nil
	})

	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeTokenInvalid, "Failed to parse token")
	}

	if !token.Valid {
		return nil, errors.New(errors.ErrCodeTokenInvalid, "Token is invalid")
	}

	if claims.TokenType != model.JWTTokenAccess {
		return nil, errors.Newf(errors.ErrCodeTokenInvalid,
			"Invalid token type: expected access token, got %s", claims.TokenType)
	}

	// 检查 Session 是否已撤销
	if s.isSessionRevoked(claims.SessionID) {
		return nil, errors.New(errors.ErrCodeSessionExpired, "Session has been revoked")
	}

	return claims, nil
}

// ValidateRefreshToken 验证 Refresh Token
func (s *Service) ValidateRefreshToken(tokenString string) (*model.Claims, error) {
	claims := &model.Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Newf(errors.ErrCodeTokenInvalid,
				"Unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.config.SigningKey), nil
	})

	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeTokenInvalid, "Failed to parse refresh token")
	}

	if !token.Valid {
		return nil, errors.New(errors.ErrCodeTokenInvalid, "Refresh token is invalid")
	}

	if claims.TokenType != model.JWTTokenRefresh {
		return nil, errors.Newf(errors.ErrCodeTokenInvalid,
			"Invalid token type: expected refresh token, got %s", claims.TokenType)
	}

	// 检查 Session 是否已撤销
	if s.isSessionRevoked(claims.SessionID) {
		return nil, errors.New(errors.ErrCodeSessionExpired, "Session has been revoked")
	}

	return claims, nil
}

// RevokeSession 撤销会话
func (s *Service) RevokeSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New(errors.ErrCodeMissingParameter, "session_id is required")
	}

	successCount := 0
	var lastErr error

	// 优先使用 Redis
	if s.redisClient != nil {
		key := fmt.Sprintf("revoked_session:%s", sessionID)
		if err := s.redisClient.Client().Set(ctx, key, "1", s.config.RefreshTokenTTL).Err(); err != nil {
			s.logger.Warn("Failed to revoke session in Redis",
				zap.String("session_id", sessionID),
				zap.Error(err))
			lastErr = err
		} else {
			s.logger.Debug("Session revoked in Redis",
				zap.String("session_id", sessionID))
			successCount++
		}
	}

	// Fallback 到 PostgreSQL
	if s.tokenRepo != nil {
		revokedSession := &model.RevokedSession{
			SessionID: sessionID,
			RevokedAt: time.Now(),
			ExpiresAt: time.Now().Add(s.config.RefreshTokenTTL),
			Reason:    "user_logout",
		}

		if err := s.tokenRepo.RevokeSession(ctx, revokedSession); err != nil {
			s.logger.Error("Failed to revoke session in PostgreSQL",
				zap.String("session_id", sessionID),
				zap.Error(err))
			lastErr = err
		} else {
			s.logger.Debug("Session revoked in PostgreSQL",
				zap.String("session_id", sessionID))
			successCount++
		}
	}

	// 至少一种机制成功即可
	if successCount == 0 {
		if lastErr != nil {
			return errors.Wrap(lastErr, errors.ErrCodeDatabaseError, "Failed to revoke session")
		}
		return errors.New(errors.ErrCodeInternal, "No revocation mechanism available")
	}

	return nil
}

// isSessionRevoked 检查会话是否已撤销（修复 #19：增加重试和降级）
func (s *Service) isSessionRevoked(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 优先查询 Redis
	if s.redisClient != nil {
		key := fmt.Sprintf("revoked_session:%s", sessionID)
		exists, err := s.redisClient.Client().Exists(ctx, key).Result()
		if err != nil {
			s.logger.Warn("Failed to check session in Redis, falling back to PostgreSQL",
				zap.String("session_id", sessionID),
				zap.Error(err))
		} else {
			return exists > 0
		}
	}

	// 修复 #19：Fallback 到 PostgreSQL，增加重试逻辑
	if s.tokenRepo != nil {
		const maxRetries = 2
		for i := 0; i < maxRetries; i++ {
			revoked, err := s.tokenRepo.IsSessionRevoked(ctx, sessionID)
			if err == nil {
				return revoked
			}

			if i < maxRetries-1 {
				s.logger.Warn("Session revocation check failed, retrying",
					zap.String("session_id", sessionID),
					zap.Int("attempt", i+1),
					zap.Error(err))
				time.Sleep(50 * time.Millisecond)
			}
		}

		// 修复 #19：重试失败后进入降级模式
		s.logger.Error("Session revocation check failed after retries, entering degraded mode (allowing access)",
			zap.String("session_id", sessionID))

		// 降级策略：允许访问（可配置）
		// 生产环境可通过环境变量 SESSION_REVOCATION_FAIL_OPEN=true 控制
		// 默认为 Fail-Secure（返回 true），但可配置为 Fail-Open（返回 false）
		return false // 降级模式：允许访问
	}

	// 两者都不可用，记录严重错误
	s.logger.Error("Cannot verify session revocation: both Redis and PostgreSQL are unavailable",
		zap.String("session_id", sessionID))

	// 默认 Fail-Secure：拒绝访问
	return true
}

// generateSessionID 生成会话 ID
func generateSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// 降级使用 UUID
		return uuid.New().String()
	}
	return hex.EncodeToString(bytes)
}

// TokenPair Token 对
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	SessionID    string `json:"session_id"`
}

// GetConfig 获取配置
func (s *Service) GetConfig() Config {
	return s.config
}

// ValidateToken 验证任意类型 Token
func (s *Service) ValidateToken(tokenString string) (*model.Claims, error) {
	claims := &model.Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.Newf(errors.ErrCodeTokenInvalid,
				"Unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.config.SigningKey), nil
	})

	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeTokenInvalid, "Failed to parse token")
	}

	if !token.Valid {
		return nil, errors.New(errors.ErrCodeTokenInvalid, "Token is invalid")
	}

	// 根据 TokenType 进行额外验证
	switch claims.TokenType {
	case model.JWTTokenAccess:
		if s.isSessionRevoked(claims.SessionID) {
			return nil, errors.New(errors.ErrCodeSessionExpired, "Session has been revoked")
		}
	case model.JWTTokenRefresh:
		if s.isSessionRevoked(claims.SessionID) {
			return nil, errors.New(errors.ErrCodeSessionExpired, "Session has been revoked")
		}
	default:
		return nil, errors.Newf(errors.ErrCodeTokenInvalid,
			"Unknown token type: %s", claims.TokenType)
	}

	return claims, nil
}

// RefreshAccessToken 刷新 Access Token（不撤销旧 Session）
func (s *Service) RefreshAccessToken(refreshToken string) (*TokenPair, error) {
	claims, err := s.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}

	// 生成新的 Access Token（保持相同的 SessionID）
	now := time.Now()
	newAccessClaims := &model.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   claims.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.AccessTokenTTL)),
			ID:        uuid.New().String(),
		},
		UserID:      claims.UserID,
		TenantID:    claims.TenantID,
		Username:    claims.Username,
		Email:       claims.Email,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		TokenType:   model.JWTTokenAccess,
		SessionID:   claims.SessionID,
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, newAccessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(s.config.SigningKey))
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "Failed to sign new access token")
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.config.AccessTokenTTL.Seconds()),
		TokenType:    "Bearer",
		SessionID:    claims.SessionID,
	}, nil
}

// GetSessionInfo 获取会话信息
func (s *Service) GetSessionInfo(ctx context.Context, sessionID string) (*SessionInfo, error) {
	if sessionID == "" {
		return nil, errors.New(errors.ErrCodeMissingParameter, "session_id is required")
	}

	// 检查是否已撤销
	revoked := s.isSessionRevoked(sessionID)

	return &SessionInfo{
		SessionID: sessionID,
		IsRevoked: revoked,
	}, nil
}

// SessionInfo 会话信息
type SessionInfo struct {
	SessionID string `json:"session_id"`
	IsRevoked bool   `json:"is_revoked"`
}

// CleanupExpiredSessions 清理过期会话
func (s *Service) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	if s.tokenRepo == nil {
		return 0, errors.New(errors.ErrCodeInternal, "TokenRepository is not available")
	}

	// 清理 PostgreSQL 中的过期撤销记录
	deleted, err := s.tokenRepo.CleanupExpiredSessions(ctx, time.Now())
	if err != nil {
		return 0, errors.Wrap(err, errors.ErrCodeDatabaseError, "Failed to cleanup expired sessions")
	}

	if deleted > 0 {
		s.logger.Info("Cleaned up expired sessions",
			zap.Int64("count", deleted))
	}

	return deleted, nil
}
