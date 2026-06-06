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

const (
	dbFieldTenantID  = "tenant_id"
	dbFieldProbeID   = "probe_id"
	dbFieldScopes    = "scopes"
	dbFieldExpiresAt = "expires_at"
	dbFieldStatus    = "status"
	dbStatusActive   = "active"
)

type PGTokenValidator struct {
	db     *sql.DB
	logger *zap.Logger
	config PGTokenValidatorConfig

	cache    sync.Map
	cacheTTL time.Duration

	cacheHits   int64
	cacheMisses int64
	dbQueries   int64
	dbErrors    int64
}

type cachedToken struct {
	TokenInfo *TokenInfo
	CachedAt  time.Time
	ExpiresAt time.Time
}

type PGTokenValidatorConfig struct {
	CacheTTL        time.Duration `env:"PG_TOKEN_CACHE_TTL" envDefault:"1m"`
	QueryTimeout    time.Duration `env:"PG_TOKEN_QUERY_TIMEOUT" envDefault:"3s"`
	MaxCacheSize    int           `env:"PG_TOKEN_MAX_CACHE_SIZE" envDefault:"10000"`
	EnableMetrics   bool          `env:"PG_TOKEN_ENABLE_METRICS" envDefault:"true"`
	CleanupInterval time.Duration `env:"PG_TOKEN_CLEANUP_INTERVAL" envDefault:"5m"`
}

func NewPGTokenValidator(db *sql.DB, cfg PGTokenValidatorConfig, logger *zap.Logger) *PGTokenValidator {

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

	go v.startCacheCleanup()

	logger.Info("PG Token Validator initialized",
		zap.Duration("cache_ttl", cfg.CacheTTL),
		zap.Int("max_cache_size", cfg.MaxCacheSize),
		zap.Bool("metrics_enabled", cfg.EnableMetrics))

	return v
}

func (v *PGTokenValidator) Validate(ctx context.Context, probeID, token string) (string, error) {
	tokenInfo, err := v.ValidateWithScopes(ctx, probeID, token)
	if err != nil {
		return "", err
	}
	return tokenInfo.TenantID, nil
}

func (v *PGTokenValidator) ValidateWithScopes(ctx context.Context, probeID, token string) (*TokenInfo, error) {
	ctx, span := otel.StartSpan(ctx, "pg_token_validator.validate_with_scopes")
	defer span.End()

	logger := logging.L(ctx)

	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s:%s", probeID, tokenHash)

	if cached, ok := v.cache.Load(cacheKey); ok {
		ct := cached.(*cachedToken)

		if time.Now().Before(ct.ExpiresAt) {

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

		v.cache.Delete(cacheKey)
	}

	atomic.AddInt64(&v.cacheMisses, 1)

	tokenInfo, err := v.queryFromDB(ctx, tokenHash, probeID)
	if err != nil {
		atomic.AddInt64(&v.dbErrors, 1)
		logger.Error("Failed to query token from PostgreSQL",
			zap.String("probe_id", probeID),
			zap.Error(err))
		return nil, err
	}

	atomic.AddInt64(&v.dbQueries, 1)

	v.cacheToken(cacheKey, tokenInfo)

	logger.Info("Token validated via PostgreSQL",
		zap.String("probe_id", probeID),
		zap.String("tenant_id", tokenInfo.TenantID),
		zap.Strings("scopes", tokenInfo.Scopes))

	return tokenInfo, nil
}

func (v *PGTokenValidator) queryFromDB(ctx context.Context, tokenHash, probeID string) (*TokenInfo, error) {
	ctx, span := otel.StartSpan(ctx, "pg_token_validator.query_db")
	defer span.End()

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

	scopes := v.parseScopes(scopesJSON, probeID)

	var expiresUnix int64
	if expiresAt.Valid {
		expiresUnix = expiresAt.Time.Unix()
	}

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

func (v *PGTokenValidator) parseScopes(scopesJSON sql.NullString, probeID string) []string {
	if !scopesJSON.Valid || scopesJSON.String == "" {

		return []string{config.ScopeIngestWrite}
	}

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

	var scopesArray []string
	if err := json.Unmarshal([]byte(scopesJSON.String), &scopesArray); err == nil {
		if len(scopesArray) > 0 {
			return scopesArray
		}
	}

	v.logger.Warn("Failed to parse scopes JSON, using default",
		zap.String("probe_id", probeID),
		zap.String("scopes_json", scopesJSON.String))

	return []string{config.ScopeIngestWrite}
}

func (v *PGTokenValidator) cacheToken(key string, tokenInfo *TokenInfo) {

	expiresAt := time.Now().Add(v.cacheTTL)

	if tokenInfo.ExpiresAt > 0 {
		tokenExpiry := time.Unix(tokenInfo.ExpiresAt, 0)
		if tokenExpiry.Before(expiresAt) {
			expiresAt = tokenExpiry
		}
	}

	if v.getCacheSize() >= v.config.MaxCacheSize {
		v.evictOldestCache()
	}

	v.cache.Store(key, &cachedToken{
		TokenInfo: tokenInfo,
		CachedAt:  time.Now(),
		ExpiresAt: expiresAt,
	})
}

func (v *PGTokenValidator) getCacheSize() int {
	count := 0
	v.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

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

func (v *PGTokenValidator) startCacheCleanup() {
	ticker := time.NewTicker(v.config.CleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		v.cleanupExpiredCache()
	}
}

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

func (v *PGTokenValidator) Invalidate(probeID, token string) {
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s:%s", probeID, tokenHash)
	v.cache.Delete(cacheKey)

	v.logger.Debug("Token cache invalidated",
		zap.String("probe_id", probeID))
}

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

func (v *PGTokenValidator) CacheSize() int {
	return v.getCacheSize()
}

func (v *PGTokenValidator) GetStats() PGValidatorStats {
	return PGValidatorStats{
		CacheHits:   atomic.LoadInt64(&v.cacheHits),
		CacheMisses: atomic.LoadInt64(&v.cacheMisses),
		DBQueries:   atomic.LoadInt64(&v.dbQueries),
		DBErrors:    atomic.LoadInt64(&v.dbErrors),
		CacheSize:   v.getCacheSize(),
	}
}

type PGValidatorStats struct {
	CacheHits   int64 `json:"cache_hits"`
	CacheMisses int64 `json:"cache_misses"`
	DBQueries   int64 `json:"db_queries"`
	DBErrors    int64 `json:"db_errors"`
	CacheSize   int   `json:"cache_size"`
}

func (s PGValidatorStats) HitRate() float64 {
	total := s.CacheHits + s.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.CacheHits) / float64(total)
}

func (v *PGTokenValidator) Healthy(ctx context.Context) bool {

	ctx, cancel := context.WithTimeout(context.Background(), config.HealthCheckTimeout)
	defer cancel()

	var count int
	err := v.db.QueryRowContext(ctx, sqlQueryTokenCount).Scan(&count)

	if err != nil {
		v.logger.Warn("PostgreSQL health check failed", zap.Error(err))
		return false
	}

	return true
}

func (v *PGTokenValidator) Close() error {
	v.ClearCache()
	v.logger.Info("PG Token Validator closed")
	return nil
}
