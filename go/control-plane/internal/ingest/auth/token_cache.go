////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/auth/token_cache.go
// 优化版 v7（完整无删减）：
// 1. 移除所有硬编码，使用 config 常量
// 2. 统一错误处理（使用 errors.AppError）
// 3. 统一日志（结构化日志 + 上下文）
// 4. 完整的指标统计
// 5. 修复 Token 过期时间处理（问题 7）
////////////////////////////////////////////////////////////////////////////////

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

// TokenCacheConfig Token 缓存配置
type TokenCacheConfig struct {
	TTL            time.Duration `env:"TOKEN_TTL" envDefault:"5m"`
	Prefix         string        `env:"TOKEN_PREFIX" envDefault:"token:"`
	LocalTTL       time.Duration `env:"LOCAL_TOKEN_TTL" envDefault:"30s"`
	LocalCacheSize int           `env:"LOCAL_CACHE_SIZE" envDefault:"10000"`
}

// localCacheEntry 本地缓存条目（包含完整信息）
type localCacheEntry struct {
	TokenInfo *TokenInfo
	CachedAt  time.Time
}

// redisTokenData Redis 中存储的 Token 数据
type redisTokenData struct {
	TenantID  string   `json:"tenant_id"`
	ProbeID   string   `json:"probe_id"`
	Scopes    []string `json:"scopes"`
	ExpiresAt int64    `json:"expires_at,omitempty"`
}

// TokenCache Token 缓存（Redis + 本地 LRU 缓存 + PG 降级）
type TokenCache struct {
	redis       redis.UniversalClient
	pgValidator *PGTokenValidator
	logger      *zap.Logger
	config      TokenCacheConfig

	// 本地 LRU 缓存（L1）
	localCache *lru.Cache[string, *localCacheEntry]
	localTTL   time.Duration

	// 统计（原子操作）
	hitRedis    int64
	hitLocal    int64
	hitPG       int64
	missTotal   int64
	redisErrors int64
}

// NewTokenCache 创建 Token 缓存
func NewTokenCache(rdb redis.UniversalClient, logger *zap.Logger, cfg TokenCacheConfig) *TokenCache {
	// 应用默认值
	if cfg.TTL == 0 {
		cfg.TTL = config.DefaultTokenTTL
	}
	if cfg.Prefix == "" {
		cfg.Prefix = config.RedisTokenPrefix
	}
	if cfg.LocalTTL == 0 {
		cfg.LocalTTL = config.DefaultLocalCacheTTL
	}
	if cfg.LocalCacheSize <= 0 {
		cfg.LocalCacheSize = config.DefaultLocalCacheSize
	}

	// 创建 LRU 缓存
	localCache, err := lru.New[string, *localCacheEntry](cfg.LocalCacheSize)
	if err != nil {
		logger.Warn("Failed to create LRU cache with configured size, using default",
			zap.Int("configured_size", cfg.LocalCacheSize),
			zap.Int("default_size", config.DefaultLocalCacheSize),
			zap.Error(err))
		localCache, _ = lru.New[string, *localCacheEntry](config.DefaultLocalCacheSize)
	}

	logger.Info("Token cache initialized",
		zap.Duration("ttl", cfg.TTL),
		zap.Duration("local_ttl", cfg.LocalTTL),
		zap.Int("local_cache_size", cfg.LocalCacheSize),
		zap.String("prefix", cfg.Prefix))

	return &TokenCache{
		redis:      rdb,
		logger:     logger,
		config:     cfg,
		localCache: localCache,
		localTTL:   cfg.LocalTTL,
	}
}

// SetPGValidator 设置 PG 验证器（用于降级）
func (tc *TokenCache) SetPGValidator(validator *PGTokenValidator) {
	tc.pgValidator = validator
	tc.logger.Info("PostgreSQL validator set for token cache fallback")
}

// Validate 验证 Token（保持向后兼容，只返回 tenantID）
func (tc *TokenCache) Validate(ctx context.Context, probeID, token string) (string, error) {
	tokenInfo, err := tc.ValidateWithScopes(ctx, probeID, token)
	if err != nil {
		return "", err
	}
	return tokenInfo.TenantID, nil
}

// ValidateWithScopes 验证 Token 并返回完整信息（包括 scopes）
// 优先级: 本地缓存 (L1) -> Redis (L2) -> PostgreSQL (L3)
func (tc *TokenCache) ValidateWithScopes(ctx context.Context, probeID, token string) (*TokenInfo, error) {
	ctx, span := otel.StartSpan(ctx, "token_cache.validate_with_scopes")
	defer span.End()

	logger := logging.L(ctx)

	// 计算 Token Hash
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s%s:%s", tc.config.Prefix, probeID, tokenHash)

	// === 阶段 1: 检查本地 LRU 缓存 (L1) ===
	if entry, ok := tc.localCache.Get(cacheKey); ok {
		if time.Since(entry.CachedAt) < tc.localTTL {
			// 修复问题 7：检查 Token 是否过期
			if entry.TokenInfo.ExpiresAt > 0 && time.Now().Unix() > entry.TokenInfo.ExpiresAt {
				tc.localCache.Remove(cacheKey)
				atomic.AddInt64(&tc.missTotal, 1)
				logger.Debug("Token expired in local cache",
					zap.String("probe_id", probeID),
					zap.Time("expires_at", time.Unix(entry.TokenInfo.ExpiresAt, 0)))
				return nil, errors.New(errors.ErrCodeUnauthorized, "token expired")
			}

			// 验证 ProbeID 绑定
			if entry.TokenInfo.ProbeID != "" && entry.TokenInfo.ProbeID != probeID {
				tc.localCache.Remove(cacheKey)
				logger.Warn("ProbeID mismatch in local cache",
					zap.String("expected", entry.TokenInfo.ProbeID),
					zap.String("actual", probeID))
				return nil, errors.New(errors.ErrCodePermissionDenied, "probe ID mismatch")
			}

			atomic.AddInt64(&tc.hitLocal, 1)
			logger.Debug("Token cache hit (local)",
				zap.String("probe_id", probeID),
				zap.Duration("age", time.Since(entry.CachedAt)))
			return entry.TokenInfo, nil
		}
		tc.localCache.Remove(cacheKey)
	}

	// === 阶段 2: 检查 Redis (L2) ===
	tokenInfo, err := tc.getFromRedis(ctx, cacheKey, probeID)
	if err == nil && tokenInfo != nil {
		// 修复问题 7：检查 Token 是否过期
		if tokenInfo.ExpiresAt > 0 && time.Now().Unix() > tokenInfo.ExpiresAt {
			// Token 已过期，删除缓存
			tc.redis.Del(ctx, cacheKey)
			atomic.AddInt64(&tc.missTotal, 1)
			logger.Debug("Token expired in Redis",
				zap.String("probe_id", probeID),
				zap.Time("expires_at", time.Unix(tokenInfo.ExpiresAt, 0)))
			return nil, errors.New(errors.ErrCodeUnauthorized, "token expired")
		}

		atomic.AddInt64(&tc.hitRedis, 1)
		logger.Debug("Token cache hit (Redis)",
			zap.String("probe_id", probeID))

		// 写入本地缓存
		tc.setLocalCache(cacheKey, tokenInfo)
		return tokenInfo, nil
	}

	// Redis 错误或未命中
	if err != nil && err != redis.Nil {
		atomic.AddInt64(&tc.redisErrors, 1)
		logger.Warn("Redis error, falling back to PostgreSQL",
			zap.String("probe_id", probeID),
			zap.Error(err))
	}

	// === 阶段 3: 降级到 PostgreSQL (L3) ===
	if tc.pgValidator != nil {
		tokenInfo, err = tc.pgValidator.ValidateWithScopes(ctx, probeID, token)
		if err == nil {
			// 修复问题 7：检查 Token 是否过期
			if tokenInfo.ExpiresAt > 0 && time.Now().Unix() > tokenInfo.ExpiresAt {
				atomic.AddInt64(&tc.missTotal, 1)
				logger.Debug("Token expired in PostgreSQL",
					zap.String("probe_id", probeID),
					zap.Time("expires_at", time.Unix(tokenInfo.ExpiresAt, 0)))
				return nil, errors.New(errors.ErrCodeUnauthorized, "token expired")
			}

			atomic.AddInt64(&tc.hitPG, 1)
			logger.Info("Token validated via PostgreSQL fallback",
				zap.String("probe_id", probeID),
				zap.String("tenant_id", tokenInfo.TenantID))

			// 异步写回 Redis
			go tc.setToRedis(context.Background(), cacheKey, tokenInfo)

			// 写入本地缓存
			tc.setLocalCache(cacheKey, tokenInfo)

			return tokenInfo, nil
		}

		logger.Debug("Token validation failed in PostgreSQL",
			zap.String("probe_id", probeID),
			zap.Error(err))
	}

	// === 所有方式都失败 ===
	atomic.AddInt64(&tc.missTotal, 1)
	logger.Warn("Token validation failed: all sources exhausted",
		zap.String("probe_id", probeID))

	return nil, errors.New(errors.ErrCodeUnauthorized, "invalid or expired token")
}

// getFromRedis 从 Redis 获取 Token 信息
func (tc *TokenCache) getFromRedis(ctx context.Context, key, probeID string) (*TokenInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, config.RedisReadTimeout)
	defer cancel()

	data, err := tc.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var tokenData redisTokenData
	if err := json.Unmarshal(data, &tokenData); err != nil {
		// 兼容旧格式（只存储 tenantID 字符串）
		tc.logger.Debug("Legacy token format detected, converting",
			zap.String("probe_id", probeID))
		return &TokenInfo{
			TenantID: string(data),
			ProbeID:  probeID,
			Scopes:   []string{config.ScopeIngestWrite}, // 默认权限
		}, nil
	}

	return &TokenInfo{
		TenantID:  tokenData.TenantID,
		ProbeID:   tokenData.ProbeID,
		Scopes:    tokenData.Scopes,
		ExpiresAt: tokenData.ExpiresAt,
	}, nil
}

// setToRedis 写入 Redis（修复问题 7：正确处理 Token 实际过期时间）
func (tc *TokenCache) setToRedis(ctx context.Context, key string, tokenInfo *TokenInfo) {
	ctx, cancel := context.WithTimeout(ctx, config.RedisWriteTimeout)
	defer cancel()

	tokenData := redisTokenData{
		TenantID:  tokenInfo.TenantID,
		ProbeID:   tokenInfo.ProbeID,
		Scopes:    tokenInfo.Scopes,
		ExpiresAt: tokenInfo.ExpiresAt,
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		tc.logger.Debug("Failed to marshal token data for Redis", zap.Error(err))
		return
	}

	// 修复问题 7：计算实际 TTL
	ttl := tc.config.TTL

	// 如果 Token 有过期时间，使用实际过期时间
	if tokenInfo.ExpiresAt > 0 {
		expiresIn := time.Until(time.Unix(tokenInfo.ExpiresAt, 0))
		if expiresIn > 0 {
			// 使用较小的 TTL（配置 TTL vs 实际过期时间）
			if expiresIn < ttl {
				ttl = expiresIn
				tc.logger.Debug("Using token expiration time as Redis TTL",
					zap.Duration("ttl", ttl),
					zap.Time("expires_at", time.Unix(tokenInfo.ExpiresAt, 0)))
			}
		} else {
			// Token 已过期，不写入缓存
			tc.logger.Debug("Token already expired, skip caching",
				zap.Time("expires_at", time.Unix(tokenInfo.ExpiresAt, 0)))
			return
		}
	}

	// 限制最大 TTL（防止 Redis TTL 溢出）
	if ttl.Seconds() > float64(config.MaxRedisTTL) {
		ttl = time.Duration(config.MaxRedisTTL) * time.Second
		tc.logger.Debug("TTL limited to max value",
			zap.Duration("original_ttl", tc.config.TTL),
			zap.Duration("limited_ttl", ttl))
	}

	if err := tc.redis.Set(ctx, key, data, ttl).Err(); err != nil {
		tc.logger.Debug("Failed to set token in Redis", zap.Error(err))
	}
}

// setLocalCache 写入本地 LRU 缓存
func (tc *TokenCache) setLocalCache(key string, tokenInfo *TokenInfo) {
	tc.localCache.Add(key, &localCacheEntry{
		TokenInfo: tokenInfo,
		CachedAt:  time.Now(),
	})
}

// Invalidate 使 Token 失效
func (tc *TokenCache) Invalidate(ctx context.Context, probeID, token string) error {
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s%s:%s", tc.config.Prefix, probeID, tokenHash)

	// 删除本地缓存
	tc.localCache.Remove(cacheKey)

	// 删除 Redis 缓存
	ctx, cancel := context.WithTimeout(ctx, config.RedisWriteTimeout)
	defer cancel()

	err := tc.redis.Del(ctx, cacheKey).Err()
	if err != nil {
		tc.logger.Warn("Failed to invalidate token in Redis",
			zap.String("probe_id", probeID),
			zap.Error(err))
	}

	tc.logger.Info("Token invalidated",
		zap.String("probe_id", probeID))

	return err
}

// ClearLocalCache 清空本地缓存
func (tc *TokenCache) ClearLocalCache() {
	tc.localCache.Purge()
	tc.logger.Info("Local token cache cleared")
}

// GetStats 获取统计信息
func (tc *TokenCache) GetStats() TokenCacheStats {
	return TokenCacheStats{
		HitLocal:    atomic.LoadInt64(&tc.hitLocal),
		HitRedis:    atomic.LoadInt64(&tc.hitRedis),
		HitPG:       atomic.LoadInt64(&tc.hitPG),
		MissTotal:   atomic.LoadInt64(&tc.missTotal),
		RedisErrors: atomic.LoadInt64(&tc.redisErrors),
		LocalSize:   tc.localCache.Len(),
	}
}

// TokenCacheStats 缓存统计
type TokenCacheStats struct {
	HitLocal    int64 `json:"hit_local"`
	HitRedis    int64 `json:"hit_redis"`
	HitPG       int64 `json:"hit_pg"`
	MissTotal   int64 `json:"miss_total"`
	RedisErrors int64 `json:"redis_errors"`
	LocalSize   int   `json:"local_size"`
}

// HitRate 计算缓存命中率
func (s TokenCacheStats) HitRate() float64 {
	total := s.HitLocal + s.HitRedis + s.HitPG + s.MissTotal
	if total == 0 {
		return 0
	}
	hits := s.HitLocal + s.HitRedis + s.HitPG
	return float64(hits) / float64(total)
}

// Healthy 健康检查
func (tc *TokenCache) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, config.HealthCheckTimeout)
	defer cancel()

	// Redis 健康
	if err := tc.redis.Ping(ctx).Err(); err == nil {
		return true
	}

	// PG 降级健康
	if tc.pgValidator != nil && tc.pgValidator.Healthy(ctx) {
		return true
	}

	return false
}
