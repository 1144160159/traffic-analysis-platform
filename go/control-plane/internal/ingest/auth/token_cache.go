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

type TokenCacheConfig struct {
	TTL            time.Duration `env:"TOKEN_TTL" envDefault:"5m"`
	Prefix         string        `env:"TOKEN_PREFIX" envDefault:"token:"`
	LocalTTL       time.Duration `env:"LOCAL_TOKEN_TTL" envDefault:"30s"`
	LocalCacheSize int           `env:"LOCAL_CACHE_SIZE" envDefault:"10000"`
}

type localCacheEntry struct {
	TokenInfo *TokenInfo
	CachedAt  time.Time
}

type redisTokenData struct {
	TenantID  string   `json:"tenant_id"`
	ProbeID   string   `json:"probe_id"`
	Scopes    []string `json:"scopes"`
	ExpiresAt int64    `json:"expires_at,omitempty"`
}

type TokenCache struct {
	redis       redis.UniversalClient
	pgValidator *PGTokenValidator
	logger      *zap.Logger
	config      TokenCacheConfig

	localCache *lru.Cache[string, *localCacheEntry]
	localTTL   time.Duration

	hitRedis    int64
	hitLocal    int64
	hitPG       int64
	missTotal   int64
	redisErrors int64
}

func NewTokenCache(rdb redis.UniversalClient, logger *zap.Logger, cfg TokenCacheConfig) *TokenCache {

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

func (tc *TokenCache) SetPGValidator(validator *PGTokenValidator) {
	tc.pgValidator = validator
	tc.logger.Info("PostgreSQL validator set for token cache fallback")
}

func (tc *TokenCache) Validate(ctx context.Context, probeID, token string) (string, error) {
	tokenInfo, err := tc.ValidateWithScopes(ctx, probeID, token)
	if err != nil {
		return "", err
	}
	return tokenInfo.TenantID, nil
}

func (tc *TokenCache) ValidateWithScopes(ctx context.Context, probeID, token string) (*TokenInfo, error) {
	ctx, span := otel.StartSpan(ctx, "token_cache.validate_with_scopes")
	defer span.End()

	logger := logging.L(ctx)

	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s%s:%s", tc.config.Prefix, probeID, tokenHash)

	if entry, ok := tc.localCache.Get(cacheKey); ok {
		if time.Since(entry.CachedAt) < tc.localTTL {

			if entry.TokenInfo.ExpiresAt > 0 && time.Now().Unix() > entry.TokenInfo.ExpiresAt {
				tc.localCache.Remove(cacheKey)
				atomic.AddInt64(&tc.missTotal, 1)
				logger.Debug("Token expired in local cache",
					zap.String("probe_id", probeID),
					zap.Time("expires_at", time.Unix(entry.TokenInfo.ExpiresAt, 0)))
				return nil, errors.New(errors.ErrCodeUnauthorized, "token expired")
			}

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

	tokenInfo, err := tc.getFromRedis(ctx, cacheKey, probeID)
	if err == nil && tokenInfo != nil {

		if tokenInfo.ExpiresAt > 0 && time.Now().Unix() > tokenInfo.ExpiresAt {

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

		tc.setLocalCache(cacheKey, tokenInfo)
		return tokenInfo, nil
	}

	if err != nil && err != redis.Nil {
		atomic.AddInt64(&tc.redisErrors, 1)
		logger.Warn("Redis error, falling back to PostgreSQL",
			zap.String("probe_id", probeID),
			zap.Error(err))
	}

	if tc.pgValidator != nil {
		tokenInfo, err = tc.pgValidator.ValidateWithScopes(ctx, probeID, token)
		if err == nil {

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

			go tc.setToRedis(context.Background(), cacheKey, tokenInfo)

			tc.setLocalCache(cacheKey, tokenInfo)

			return tokenInfo, nil
		}

		logger.Debug("Token validation failed in PostgreSQL",
			zap.String("probe_id", probeID),
			zap.Error(err))
	}

	atomic.AddInt64(&tc.missTotal, 1)
	logger.Warn("Token validation failed: all sources exhausted",
		zap.String("probe_id", probeID))

	return nil, errors.New(errors.ErrCodeUnauthorized, "invalid or expired token")
}

func (tc *TokenCache) getFromRedis(ctx context.Context, key, probeID string) (*TokenInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, config.RedisReadTimeout)
	defer cancel()

	data, err := tc.redis.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var tokenData redisTokenData
	if err := json.Unmarshal(data, &tokenData); err != nil {

		tc.logger.Debug("Legacy token format detected, converting",
			zap.String("probe_id", probeID))
		return &TokenInfo{
			TenantID: string(data),
			ProbeID:  probeID,
			Scopes:   []string{config.ScopeIngestWrite},
		}, nil
	}

	return &TokenInfo{
		TenantID:  tokenData.TenantID,
		ProbeID:   tokenData.ProbeID,
		Scopes:    tokenData.Scopes,
		ExpiresAt: tokenData.ExpiresAt,
	}, nil
}

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

	ttl := tc.config.TTL

	if tokenInfo.ExpiresAt > 0 {
		expiresIn := time.Until(time.Unix(tokenInfo.ExpiresAt, 0))
		if expiresIn > 0 {

			if expiresIn < ttl {
				ttl = expiresIn
				tc.logger.Debug("Using token expiration time as Redis TTL",
					zap.Duration("ttl", ttl),
					zap.Time("expires_at", time.Unix(tokenInfo.ExpiresAt, 0)))
			}
		} else {

			tc.logger.Debug("Token already expired, skip caching",
				zap.Time("expires_at", time.Unix(tokenInfo.ExpiresAt, 0)))
			return
		}
	}

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

func (tc *TokenCache) setLocalCache(key string, tokenInfo *TokenInfo) {
	tc.localCache.Add(key, &localCacheEntry{
		TokenInfo: tokenInfo,
		CachedAt:  time.Now(),
	})
}

func (tc *TokenCache) Invalidate(ctx context.Context, probeID, token string) error {
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s%s:%s", tc.config.Prefix, probeID, tokenHash)

	tc.localCache.Remove(cacheKey)

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

func (tc *TokenCache) ClearLocalCache() {
	tc.localCache.Purge()
	tc.logger.Info("Local token cache cleared")
}

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

type TokenCacheStats struct {
	HitLocal    int64 `json:"hit_local"`
	HitRedis    int64 `json:"hit_redis"`
	HitPG       int64 `json:"hit_pg"`
	MissTotal   int64 `json:"miss_total"`
	RedisErrors int64 `json:"redis_errors"`
	LocalSize   int   `json:"local_size"`
}

func (s TokenCacheStats) HitRate() float64 {
	total := s.HitLocal + s.HitRedis + s.HitPG + s.MissTotal
	if total == 0 {
		return 0
	}
	hits := s.HitLocal + s.HitRedis + s.HitPG
	return float64(hits) / float64(total)
}

func (tc *TokenCache) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, config.HealthCheckTimeout)
	defer cancel()

	if err := tc.redis.Ping(ctx).Err(); err == nil {
		return true
	}

	if tc.pgValidator != nil && tc.pgValidator.Healthy(ctx) {
		return true
	}

	return false
}
