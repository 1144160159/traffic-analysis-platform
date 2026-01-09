////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/auth/pg_token_validator.go
// 修复版：返回 scopes 信息
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
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

// PGTokenValidator PostgreSQL Token 验证器（用于 Redis 降级）
type PGTokenValidator struct {
	db     *sql.DB
	logger *zap.Logger

	// 本地缓存（减少 PG 查询）
	cache    sync.Map // map[string]*cachedToken
	cacheTTL time.Duration
}

// cachedToken 缓存的 Token 信息（包含 scopes）
type cachedToken struct {
	TokenInfo *TokenInfo
	CachedAt  time.Time
}

// PGTokenValidatorConfig 配置
type PGTokenValidatorConfig struct {
	CacheTTL time.Duration `env:"PG_TOKEN_CACHE_TTL" envDefault:"1m"`
}

// NewPGTokenValidator 创建 PG Token 验证器
func NewPGTokenValidator(db *sql.DB, cfg PGTokenValidatorConfig, logger *zap.Logger) *PGTokenValidator {
	return &PGTokenValidator{
		db:       db,
		logger:   logger,
		cacheTTL: cfg.CacheTTL,
	}
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

	// 计算 Token Hash
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s:%s", probeID, tokenHash)

	// 1. 检查本地缓存
	if cached, ok := v.cache.Load(cacheKey); ok {
		ct := cached.(*cachedToken)

		// 检查缓存是否过期
		if time.Since(ct.CachedAt) < v.cacheTTL {
			// 检查 Token 是否过期
			if ct.TokenInfo.ExpiresAt != 0 && time.Now().Unix() > ct.TokenInfo.ExpiresAt {
				v.cache.Delete(cacheKey)
				return nil, fmt.Errorf("token expired")
			}
			return ct.TokenInfo, nil
		}

		// 缓存过期，删除
		v.cache.Delete(cacheKey)
	}

	// 2. 查询 PostgreSQL
	tokenInfo, err := v.queryFromDB(ctx, tokenHash, probeID)
	if err != nil {
		return nil, err
	}

	// 3. 缓存结果
	v.cache.Store(cacheKey, &cachedToken{
		TokenInfo: tokenInfo,
		CachedAt:  time.Now(),
	})

	return tokenInfo, nil
}

// queryFromDB 从数据库查询 Token（包含 scopes）
func (v *PGTokenValidator) queryFromDB(ctx context.Context, tokenHash, probeID string) (*TokenInfo, error) {
	ctx, span := otel.StartSpan(ctx, "pg_token_validator.query_db")
	defer span.End()

	query := `
		SELECT tenant_id, scopes, expires_at
		FROM api_tokens
		WHERE token_hash = $1 
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > NOW())
		LIMIT 1
	`

	var tenantID string
	var scopesJSON sql.NullString
	var expiresAt sql.NullTime

	err := v.db.QueryRowContext(ctx, query, tokenHash).Scan(&tenantID, &scopesJSON, &expiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("token not found or expired")
		}
		v.logger.Error("Failed to query api_tokens", zap.Error(err))
		return nil, fmt.Errorf("database error: %w", err)
	}

	// 解析 scopes
	var scopes []string
	if scopesJSON.Valid && scopesJSON.String != "" {
		// scopes 存储为 JSON 对象 {"scope1": true, "scope2": true}
		var scopesMap map[string]interface{}
		if err := json.Unmarshal([]byte(scopesJSON.String), &scopesMap); err == nil {
			for scope, enabled := range scopesMap {
				if b, ok := enabled.(bool); ok && b {
					scopes = append(scopes, scope)
				}
			}
		} else {
			// 尝试解析为数组
			if err := json.Unmarshal([]byte(scopesJSON.String), &scopes); err != nil {
				v.logger.Warn("Failed to parse scopes JSON",
					zap.String("scopes", scopesJSON.String),
					zap.Error(err))
			}
		}
	}

	// 默认权限
	if len(scopes) == 0 {
		scopes = []string{ScopeIngestWrite}
	}

	var expiresUnix int64
	if expiresAt.Valid {
		expiresUnix = expiresAt.Time.Unix()
	}

	return &TokenInfo{
		TenantID:  tenantID,
		ProbeID:   probeID,
		Scopes:    scopes,
		ExpiresAt: expiresUnix,
	}, nil
}

// Invalidate 使 Token 缓存失效
func (v *PGTokenValidator) Invalidate(probeID, token string) {
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])
	cacheKey := fmt.Sprintf("%s:%s", probeID, tokenHash)
	v.cache.Delete(cacheKey)
}

// ClearCache 清空缓存
func (v *PGTokenValidator) ClearCache() {
	v.cache.Range(func(key, value interface{}) bool {
		v.cache.Delete(key)
		return true
	})
}

// CacheSize 获取缓存大小
func (v *PGTokenValidator) CacheSize() int {
	count := 0
	v.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// Healthy 健康检查
func (v *PGTokenValidator) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return v.db.PingContext(ctx) == nil
}
