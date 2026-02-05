////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/auth/pg_token_validator.go
// 优化版 v3：
// 1. 移除所有硬编码（SQL、缓存 TTL、字段名）
// 2. 统一错误处理（errors.AppError）
// 3. 统一日志（结构化日志 + 上下文）
// 4. 完整的指标统计
// 5. 支持 scopes 验证
////////////////////////////////////////////////////////////////////////////////

package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/logging"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

// SQL 查询常量（移除硬编码）
const (
	sqlQueryTokenByHash = `
		SELECT tenant_id, probe_id, scopes, expires_at
		FROM api_tokens
		WHERE token_hash = $1 AND (probe_id = $2 OR probe_id IS NULL)
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > NOW())
		LIMIT 1
	`
	sqlQueryTokenCount = `
		SELECT COUNT(*) FROM api_tokens WHERE status = 'active'
	`
)

// 数据库字段常量
const (
	dbFieldTenantID  = "tenant_id"
	dbFieldProbeID   = "probe_id"
	dbFieldScopes    = "scopes"
	dbFieldExpiresAt = "expires_at"
	dbFieldStatus    = "status"
	dbStatusActive   = "active"
)

// PGTokenValidator PostgreSQL Token 验证器（用于 Redis 降级）
type PGTokenValidator struct {
	db     *sql.DB
	logger *zap.Logger
	config PGTokenValidatorConfig

	// 本地缓存（减少 PG 查询）
	cache    sync.Map // map[string]*cachedToken
	cacheTTL time.Duration

	// 统计（原子安全）
	cacheHits   int64
	cacheMisses int64
	dbQueries   int64
	dbErrors    int64
}

// cachedToken 缓存的 Token 信息（包含 scopes）
type cachedToken struct {
	TokenInfo *TokenInfo
	CachedAt  time.Time
	ExpiresAt time.Time
}

// PGTokenValidatorConfig 配置
type PGTokenValidatorConfig struct {
	CacheTTL        time.Duration `env:"PG_TOKEN_CACHE_TTL" envDefault:"1m"`
	QueryTimeout    time.Duration `env:"PG_TOKEN_QUERY_TIMEOUT" envDefault:"3s"`
	MaxCacheSize    int           `env:"PG_TOKEN_MAX_CACHE_SIZE" envDefault:"10000"`
	EnableMetrics   bool          `env:"PG_TOKEN_ENABLE_METRICS" envDefault:"true"`
	CleanupInterval time.Duration `env:"PG_TOKEN_CLEANUP_INTERVAL" envDefault:"5m"`
}

// NewPGTokenValidator 创建 PG Token 验证器
func NewPGTokenValidator(db *sql.DB, cfg PGTokenValidatorConfig, logger *zap.Logger) *PGTokenValidator {
	// 应用默认值
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = time.Minute
	}
	if cfg.QueryTimeout == 0 {
		cfg.QueryTimeout = 3 * time.Second
	}
	if cfg.MaxCacheSize <= 0 {
		cfg.MaxCacheSize = 10000
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}

	v := &PGTokenValidator{
		db:       db,
		logger:   logger,
		config:   cfg,
		cacheTTL: cfg.CacheTTL,
	}

	// 启动缓存清理器
	go v.startCacheCleanup()

	logger.Info("PG Token Validator initialized",
		zap.Duration("cache_ttl", cfg.CacheTTL),
		zap.Int("max_cache_size", cfg.MaxCacheSize),
		zap.Bool("metrics_enabled", cfg.EnableMetrics))

	return v
}

// Validate 验证 Token（向后兼容，只返回 tenantID）
func (v *PGTokenValidator) Validate(ctx context.Context, probeID, token string) (string, error) {
	tokenInfo, err := v.ValidateWithScopes(ctx, probeID, token)
	if err != nil {
		return "", err
	}
	return tokenInfo.TenantID, nil
}

// ValidateWithScopes 验证 Token 并返回完整信息（包括 scopes）
func (v *PGTokenValidator) ValidateWithScopes(ctx context.Context, probeID, token string) (*TokenInfo, error) {
	ctx, span := otel.StartSpan(ctx, "pg_token_validator.validate_with_scopes")
	defer span.End()

	logger := logging.L(ctx)

	// 计算 Token Hash
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s:%s", probeID, tokenHash)

	// 1. 检查本地缓存
	if cached, ok := v.cache.Load(cacheKey); ok {
		ct := cached.(*cachedToken)

		// 检查缓存是否过期
		if time.Now().Before(ct.ExpiresAt) {
			// 检查 Token 是否过期
			if ct.TokenInfo.ExpiresAt > 0 && time.Now().Unix() > ct.TokenInfo.ExpiresAt {
				v.cache.Delete(cacheKey)
				atomic.AddInt64(&v.cacheMisses, 1)
				logger.Debug("Token expired in cache",
					zap.String("probe_id", probeID),
					zap.Time("expires_at", time.Unix(ct.TokenInfo.ExpiresAt, 0)))
				return nil, errors.New(errors.ErrCodeUnauthorized, "token expired")
			}

			atomic.AddInt64(&v.cacheHits, 1)
			logger.Debug("Token cache hit",
				zap.String("probe_id", probeID),
				zap.String("tenant_id", ct.TokenInfo.TenantID))
			return ct.TokenInfo, nil
		}

		// 缓存过期，删除
		v.cache.Delete(cacheKey)
	}

	atomic.AddInt64(&v.cacheMisses, 1)

	// 2. 查询 PostgreSQL
	tokenInfo, err := v.queryFromDB(ctx, tokenHash, probeID)
	if err != nil {
		atomic.AddInt64(&v.dbErrors, 1)
		logger.Error("Failed to query token from PostgreSQL",
			zap.String("probe_id", probeID),
			zap.Error(err))
		return nil, err
	}

	atomic.AddInt64(&v.dbQueries, 1)

	// 3. 缓存结果
	v.cacheToken(cacheKey, tokenInfo)

	logger.Info("Token validated via PostgreSQL",
		zap.String("probe_id", probeID),
		zap.String("tenant_id", tokenInfo.TenantID),
		zap.Strings("scopes", tokenInfo.Scopes))

	return tokenInfo, nil
}

// queryFromDB 从数据库查询 Token（包含 scopes）
func (v *PGTokenValidator) queryFromDB(ctx context.Context, tokenHash, probeID string) (*TokenInfo, error) {
	ctx, span := otel.StartSpan(ctx, "pg_token_validator.query_db")
	defer span.End()

	// 添加查询超时
	queryCtx, cancel := context.WithTimeout(ctx, v.config.QueryTimeout)
	defer cancel()

	var tenantID string
	var probeIDResult sql.NullString
	var scopesJSON sql.NullString
	var expiresAt sql.NullTime

	err := v.db.QueryRowContext(queryCtx, sqlQueryTokenByHash, tokenHash, probeID).Scan(
		&tenantID,
		&probeIDResult,
		&scopesJSON,
		&expiresAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New(errors.ErrCodeUnauthorized, "token not found or expired")
		}
		return nil, errors.Wrap(err, errors.ErrCodeDatabaseError, "failed to query token")
	}

	// 解析 scopes
	scopes := v.parseScopes(scopesJSON, probeID)

	// 解析过期时间
	var expiresUnix int64
	if expiresAt.Valid {
		expiresUnix = expiresAt.Time.Unix()
	}

	// 解析 probe_id（可能为空，表示通配）
	var boundProbeID string
	if probeIDResult.Valid {
		boundProbeID = probeIDResult.String
	}

	return &TokenInfo{
		TenantID:  tenantID,
		ProbeID:   boundProbeID,
		Scopes:    scopes,
		ExpiresAt: expiresUnix,
	}, nil
}

// parseScopes 解析 scopes JSON（支持对象和数组格式）
func (v *PGTokenValidator) parseScopes(scopesJSON sql.NullString, probeID string) []string {
	if !scopesJSON.Valid || scopesJSON.String == "" {
		// 默认权限
		return []string{config.ScopeIngestWrite}
	}

	// 尝试解析为对象格式 {"scope1": true, "scope2": true}
	var scopesMap map[string]interface{}
	if err := json.Unmarshal([]byte(scopesJSON.String), &scopesMap); err == nil {
		scopes := make([]string, 0, len(scopesMap))
		for scope, enabled := range scopesMap {
			if b, ok := enabled.(bool); ok && b {
				scopes = append(scopes, scope)
			}
		}
		if len(scopes) > 0 {
			return scopes
		}
	}

	// 尝试解析为数组格式 ["scope1", "scope2"]
	var scopesArray []string
	if err := json.Unmarshal([]byte(scopesJSON.String), &scopesArray); err == nil {
		if len(scopesArray) > 0 {
			return scopesArray
		}
	}

	// 解析失败，记录警告并返回默认权限
	v.logger.Warn("Failed to parse scopes JSON, using default",
		zap.String("probe_id", probeID),
		zap.String("scopes_json", scopesJSON.String))

	return []string{config.ScopeIngestWrite}
}

// cacheToken 缓存 Token（带过期时间）
func (v *PGTokenValidator) cacheToken(key string, tokenInfo *TokenInfo) {
	// 计算缓存过期时间
	expiresAt := time.Now().Add(v.cacheTTL)

	// 如果 Token 有实际过期时间，使用较小的值
	if tokenInfo.ExpiresAt > 0 {
		tokenExpiry := time.Unix(tokenInfo.ExpiresAt, 0)
		if tokenExpiry.Before(expiresAt) {
			expiresAt = tokenExpiry
		}
	}

	// 检查缓存大小限制
	if v.getCacheSize() >= v.config.MaxCacheSize {
		v.evictOldestCache()
	}

	v.cache.Store(key, &cachedToken{
		TokenInfo: tokenInfo,
		CachedAt:  time.Now(),
		ExpiresAt: expiresAt,
	})
}

// getCacheSize 获取缓存大小
func (v *PGTokenValidator) getCacheSize() int {
	count := 0
	v.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// evictOldestCache 淘汰最老的缓存项
func (v *PGTokenValidator) evictOldestCache() {
	var oldestKey interface{}
	var oldestTime time.Time

	v.cache.Range(func(key, value interface{}) bool {
		ct := value.(*cachedToken)
		if oldestKey == nil || ct.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = ct.CachedAt
		}
		return true
	})

	if oldestKey != nil {
		v.cache.Delete(oldestKey)
		v.logger.Debug("Evicted oldest cache entry",
			zap.String("key", fmt.Sprint(oldestKey)))
	}
}

// startCacheCleanup 启动缓存清理器（定期清理过期缓存）
func (v *PGTokenValidator) startCacheCleanup() {
	ticker := time.NewTicker(v.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		v.cleanupExpiredCache()
	}
}

// cleanupExpiredCache 清理过期缓存
func (v *PGTokenValidator) cleanupExpiredCache() {
	now := time.Now()
	expiredKeys := make([]interface{}, 0)

	v.cache.Range(func(key, value interface{}) bool {
		ct := value.(*cachedToken)
		if now.After(ct.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
		return true
	})

	for _, key := range expiredKeys {
		v.cache.Delete(key)
	}

	if len(expiredKeys) > 0 {
		v.logger.Debug("Cleaned up expired cache entries",
			zap.Int("count", len(expiredKeys)))
	}
}

// Invalidate 使 Token 缓存失效
func (v *PGTokenValidator) Invalidate(probeID, token string) {
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s:%s", probeID, tokenHash)
	v.cache.Delete(cacheKey)

	v.logger.Debug("Token cache invalidated",
		zap.String("probe_id", probeID))
}

// ClearCache 清空缓存
func (v *PGTokenValidator) ClearCache() {
	count := 0
	v.cache.Range(func(key, value interface{}) bool {
		v.cache.Delete(key)
		count++
		return true
	})

	v.logger.Info("Token cache cleared",
		zap.Int("count", count))
}

// CacheSize 获取缓存大小
func (v *PGTokenValidator) CacheSize() int {
	return v.getCacheSize()
}

// GetStats 获取统计信息
func (v *PGTokenValidator) GetStats() PGValidatorStats {
	return PGValidatorStats{
		CacheHits:   atomic.LoadInt64(&v.cacheHits),
		CacheMisses: atomic.LoadInt64(&v.cacheMisses),
		DBQueries:   atomic.LoadInt64(&v.dbQueries),
		DBErrors:    atomic.LoadInt64(&v.dbErrors),
		CacheSize:   v.getCacheSize(),
	}
}

// PGValidatorStats 验证器统计
type PGValidatorStats struct {
	CacheHits   int64 `json:"cache_hits"`
	CacheMisses int64 `json:"cache_misses"`
	DBQueries   int64 `json:"db_queries"`
	DBErrors    int64 `json:"db_errors"`
	CacheSize   int   `json:"cache_size"`
}

// HitRate 计算缓存命中率
func (s PGValidatorStats) HitRate() float64 {
	total := s.CacheHits + s.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.CacheHits) / float64(total)
}

// Healthy 健康检查
func (v *PGTokenValidator) Healthy(ctx context.Context) bool {
	// 使用独立的 context 避免父 context 取消影响
	ctx, cancel := context.WithTimeout(context.Background(), config.HealthCheckTimeout)
	defer cancel()

	// 简单查询测试数据库连接
	var count int
	err := v.db.QueryRowContext(ctx, sqlQueryTokenCount).Scan(&count)

	if err != nil {
		v.logger.Warn("PostgreSQL health check failed", zap.Error(err))
		return false
	}

	return true
}

// Close 关闭验证器
func (v *PGTokenValidator) Close() error {
	v.ClearCache()
	v.logger.Info("PG Token Validator closed")
	return nil
}
