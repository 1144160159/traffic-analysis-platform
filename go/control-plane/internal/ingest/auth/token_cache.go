////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/auth/token_cache.go
// 修复版 v2：
// 1. 修复问题 7：setToRedis 正确处理 Token 实际过期时间
// 2. 添加完整的 scopes 支持
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

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
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

	// 统计
	hitRedis    int64
	hitLocal    int64
	hitPG       int64
	missTotal   int64
	redisErrors int64
}

// NewTokenCache 创建 Token 缓存
func NewTokenCache(rdb redis.UniversalClient, logger *zap.Logger, config TokenCacheConfig) *TokenCache {
	cacheSize := config.LocalCacheSize
	if cacheSize <= 0 {
		cacheSize = 10000
	}

	localCache, err := lru.New[string, *localCacheEntry](cacheSize)
	if err != nil {
		logger.Warn("Failed to create LRU cache with configured size, using default",
			zap.Int("configured_size", cacheSize),
			zap.Error(err))
		localCache, _ = lru.New[string, *localCacheEntry](1000)
	}

	return &TokenCache{
		redis:      rdb,
		logger:     logger,
		config:     config,
		localCache: localCache,
		localTTL:   config.LocalTTL,
	}
}

// SetPGValidator 设置 PG 验证器（用于降级）
func (tc *TokenCache) SetPGValidator(validator *PGTokenValidator) {
	tc.pgValidator = validator
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
// 优先级: 本地缓存 -> Redis -> PostgreSQL
func (tc *TokenCache) ValidateWithScopes(ctx context.Context, probeID, token string) (*TokenInfo, error) {
	ctx, span := otel.StartSpan(ctx, "token_cache.validate_with_scopes")
	defer span.End()

	// 计算 Token Hash
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s%s:%s", tc.config.Prefix, probeID, tokenHash)

	// 1. 检查本地 LRU 缓存 (L1)
	if entry, ok := tc.localCache.Get(cacheKey); ok {
		if time.Since(entry.CachedAt) < tc.localTTL {
			// 修复问题 7：检查 Token 是否过期
			if entry.TokenInfo.ExpiresAt > 0 && time.Now().Unix() > entry.TokenInfo.ExpiresAt {
				tc.localCache.Remove(cacheKey)
				atomic.AddInt64(&tc.missTotal, 1)
				return nil, fmt.Errorf("token expired")
			}
			atomic.AddInt64(&tc.hitLocal, 1)
			return entry.TokenInfo, nil
		}
		// 本地缓存过期，删除
		tc.localCache.Remove(cacheKey)
	}

	// 2. 检查 Redis (L2)
	tokenInfo, err := tc.getFromRedis(ctx, cacheKey, probeID)
	if err == nil && tokenInfo != nil {
		// 修复问题 7：检查 Token 是否过期
		if tokenInfo.ExpiresAt > 0 && time.Now().Unix() > tokenInfo.ExpiresAt {
			// Token 已过期，删除缓存
			tc.redis.Del(ctx, cacheKey)
			atomic.AddInt64(&tc.missTotal, 1)
			return nil, fmt.Errorf("token expired")
		}

		atomic.AddInt64(&tc.hitRedis, 1)
		// 写入本地缓存
		tc.setLocalCache(cacheKey, tokenInfo)
		return tokenInfo, nil
	}

	// Redis 错误或未命中
	if err != nil && err != redis.Nil {
		atomic.AddInt64(&tc.redisErrors, 1)
		tc.logger.Warn("Redis error, falling back to PG",
			zap.String("probe_id", probeID),
			zap.Error(err))
	}

	// 3. 降级到 PostgreSQL (L3)
	if tc.pgValidator != nil {
		tokenInfo, err = tc.pgValidator.ValidateWithScopes(ctx, probeID, token)
		if err == nil {
			// 修复问题 7：检查 Token 是否过期
			if tokenInfo.ExpiresAt > 0 && time.Now().Unix() > tokenInfo.ExpiresAt {
				atomic.AddInt64(&tc.missTotal, 1)
				return nil, fmt.Errorf("token expired")
			}

			atomic.AddInt64(&tc.hitPG, 1)

			// 异步写回 Redis
			go tc.setToRedis(context.Background(), cacheKey, tokenInfo)

			// 写入本地缓存
			tc.setLocalCache(cacheKey, tokenInfo)

			return tokenInfo, nil
		}
	}

	// 4. 所有方式都失败
	atomic.AddInt64(&tc.missTotal, 1)

	return nil, fmt.Errorf("token validation failed: all sources exhausted")
}

// getFromRedis 从 Redis 获取 Token 信息
func (tc *TokenCache) getFromRedis(ctx context.Context, key, probeID string) (*TokenInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	data, err := tc.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var tokenData redisTokenData
	if err := json.Unmarshal(data, &tokenData); err != nil {
		// 兼容旧格式（只存储 tenantID 字符串）
		return &TokenInfo{
			TenantID: string(data),
			ProbeID:  probeID,
			Scopes:   []string{ScopeIngestWrite}, // 默认权限
		}, nil
	}

	return &TokenInfo{
		TenantID:  tokenData.TenantID,
		ProbeID:   probeID,
		Scopes:    tokenData.Scopes,
		ExpiresAt: tokenData.ExpiresAt,
	}, nil
}

// 修复问题 7：setToRedis 正确处理 Token 实际过期时间
func (tc *TokenCache) setToRedis(ctx context.Context, key string, tokenInfo *TokenInfo) {
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	tokenData := redisTokenData{
		TenantID:  tokenInfo.TenantID,
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
	return tc.redis.Del(ctx, cacheKey).Err()
}

// ClearLocalCache 清空本地缓存
func (tc *TokenCache) ClearLocalCache() {
	tc.localCache.Purge()
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
	HitLocal    int64
	HitRedis    int64
	HitPG       int64
	MissTotal   int64
	RedisErrors int64
	LocalSize   int
}

// Healthy 健康检查
func (tc *TokenCache) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
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
