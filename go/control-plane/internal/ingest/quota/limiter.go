package quota

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

type LimiterConfig struct {
	RedisEnabled bool   `env:"RATE_LIMIT_REDIS_ENABLED" envDefault:"true"`
	RedisPrefix  string `env:"RATE_LIMIT_REDIS_PREFIX" envDefault:"ratelimit:"`

	GlobalRPS   float64 `env:"RATE_LIMIT_GLOBAL_RPS" envDefault:"100000"`
	GlobalBurst int     `env:"RATE_LIMIT_GLOBAL_BURST" envDefault:"200000"`

	TenantRPS   float64 `env:"RATE_LIMIT_TENANT_RPS" envDefault:"10000"`
	TenantBurst int     `env:"RATE_LIMIT_TENANT_BURST" envDefault:"20000"`

	ProbeRPS   float64 `env:"RATE_LIMIT_PROBE_RPS" envDefault:"5000"`
	ProbeBurst int     `env:"RATE_LIMIT_PROBE_BURST" envDefault:"10000"`

	LocalFallbackEnabled bool `env:"RATE_LIMIT_LOCAL_FALLBACK" envDefault:"true"`
}

type Limiter struct {
	redis       redis.UniversalClient
	localGlobal *LocalTokenBucket
	localTenant *LocalBucketManager
	localProbe  *LocalBucketManager
	logger      *zap.Logger
	config      LimiterConfig

	tokenBucketScript *redis.Script

	redisAvailable int32
	lastRedisCheck int64

	allowedTotal int64
	deniedTotal  int64
	fallbackUsed int64

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

const tokenBucketLuaScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

-- 获取当前桶状态
local data = redis.call('HMGET', key, 'tokens', 'last_update')
local tokens = tonumber(data[1])
local last_update = tonumber(data[2])

-- 初始化
if tokens == nil then
    tokens = capacity
    last_update = now
end

-- 计算新令牌（基于时间流逝）
local elapsed = (now - last_update) / 1000.0
local new_tokens = elapsed * rate
tokens = math.min(capacity, tokens + new_tokens)

-- 检查是否有足够令牌
if tokens < requested then
    -- 更新时间戳但不扣减令牌
    redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
    -- 动态计算过期时间 = (容量/速率) * 2，并限制最大值
    local ttl = math.ceil((capacity / rate) * 2)
    if ttl > 86400 then
        ttl = 86400  -- 限制最大 1 天（防止 Redis TTL 溢出）
    end
    redis.call('EXPIRE', key, ttl)
    return 0
end

-- 扣减令牌
tokens = tokens - requested
redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
local ttl = math.ceil((capacity / rate) * 2)
if ttl > 86400 then
    ttl = 86400  -- 限制最大 1 天
end
redis.call('EXPIRE', key, ttl)
return 1
`

func NewLimiter(rdb redis.UniversalClient, cfg LimiterConfig, logger *zap.Logger) *Limiter {
	ctx, cancel := context.WithCancel(context.Background())

	if cfg.RedisPrefix == "" {
		cfg.RedisPrefix = config.RedisRateLimitPrefix
	}

	l := &Limiter{
		redis:  rdb,
		logger: logger,
		config: cfg,

		tokenBucketScript: redis.NewScript(tokenBucketLuaScript),

		localGlobal: NewLocalTokenBucket(float64(cfg.GlobalBurst), cfg.GlobalRPS),
		localTenant: NewLocalBucketManager(float64(cfg.TenantBurst), cfg.TenantRPS),
		localProbe:  NewLocalBucketManager(float64(cfg.ProbeBurst), cfg.ProbeRPS),

		redisAvailable: 1,

		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	go l.healthCheckLoop()

	return l
}

func (l *Limiter) Allow(ctx context.Context, tenantID, probeID string) bool {
	return l.AllowN(ctx, tenantID, probeID, 1)
}

func (l *Limiter) AllowN(ctx context.Context, tenantID, probeID string, n int) bool {
	ctx, span := otel.StartSpan(ctx, "limiter.allow")
	defer span.End()

	if l.config.RedisEnabled && atomic.LoadInt32(&l.redisAvailable) == 1 {
		allowed, err := l.checkRedis(ctx, tenantID, probeID, n)
		if err == nil {
			if allowed {
				atomic.AddInt64(&l.allowedTotal, int64(n))
			} else {
				atomic.AddInt64(&l.deniedTotal, int64(n))
			}
			return allowed
		}

		l.logger.Warn("Redis rate limit failed, falling back to local",
			zap.Error(err))
		atomic.StoreInt32(&l.redisAvailable, 0)
	}

	if l.config.LocalFallbackEnabled {
		atomic.AddInt64(&l.fallbackUsed, int64(n))
		return l.checkLocal(tenantID, probeID, n)
	}

	l.logger.Warn("Rate limiting disabled, allowing request",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID))
	return true
}

func (l *Limiter) checkRedis(ctx context.Context, tenantID, probeID string, n int) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	now := time.Now().UnixMilli()

	globalKey := l.config.RedisPrefix + "global"
	result, err := l.tokenBucketScript.Run(ctx, l.redis, []string{globalKey},
		l.config.GlobalBurst, l.config.GlobalRPS, now, n).Int()
	if err != nil {
		return false, fmt.Errorf("global rate limit check failed: %w", err)
	}
	if result == 0 {
		l.logger.Debug("Global rate limit exceeded")
		return false, nil
	}

	tenantKey := l.config.RedisPrefix + "tenant:" + tenantID
	result, err = l.tokenBucketScript.Run(ctx, l.redis, []string{tenantKey},
		l.config.TenantBurst, l.config.TenantRPS, now, n).Int()
	if err != nil {
		return false, fmt.Errorf("tenant rate limit check failed: %w", err)
	}
	if result == 0 {
		l.logger.Debug("Tenant rate limit exceeded", zap.String("tenant_id", tenantID))
		return false, nil
	}

	probeKey := l.config.RedisPrefix + "probe:" + probeID
	result, err = l.tokenBucketScript.Run(ctx, l.redis, []string{probeKey},
		l.config.ProbeBurst, l.config.ProbeRPS, now, n).Int()
	if err != nil {
		return false, fmt.Errorf("probe rate limit check failed: %w", err)
	}
	if result == 0 {
		l.logger.Debug("Probe rate limit exceeded", zap.String("probe_id", probeID))
		return false, nil
	}

	return true, nil
}

func (l *Limiter) checkLocal(tenantID, probeID string, n int) bool {

	if !l.localGlobal.AllowN(n) {
		l.logger.Debug("Local global rate limit exceeded")
		return false
	}

	if !l.localTenant.AllowN(tenantID, n) {
		l.logger.Debug("Local tenant rate limit exceeded", zap.String("tenant_id", tenantID))
		return false
	}

	if !l.localProbe.AllowN(probeID, n) {
		l.logger.Debug("Local probe rate limit exceeded", zap.String("probe_id", probeID))
		return false
	}

	return true
}

func (l *Limiter) healthCheckLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	defer close(l.done)

	for {
		select {
		case <-l.ctx.Done():
			l.logger.Info("Health check loop stopped")
			return

		case <-ticker.C:

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := l.redis.Ping(ctx).Err()
			cancel()

			if err == nil {
				if atomic.LoadInt32(&l.redisAvailable) == 0 {
					l.logger.Info("Redis rate limiter recovered")
					atomic.StoreInt32(&l.redisAvailable, 1)
				}
			} else {
				if atomic.LoadInt32(&l.redisAvailable) == 1 {
					l.logger.Warn("Redis rate limiter unavailable", zap.Error(err))
					atomic.StoreInt32(&l.redisAvailable, 0)
				}
			}
		}
	}
}

func (l *Limiter) Close() error {
	l.cancel()

	select {
	case <-l.done:
		l.logger.Info("Limiter closed gracefully")
	case <-time.After(5 * time.Second):
		l.logger.Warn("Limiter close timeout")
	}

	return nil
}

func (l *Limiter) GetStats() LimiterStats {
	return LimiterStats{
		AllowedTotal:   atomic.LoadInt64(&l.allowedTotal),
		DeniedTotal:    atomic.LoadInt64(&l.deniedTotal),
		FallbackUsed:   atomic.LoadInt64(&l.fallbackUsed),
		RedisAvailable: atomic.LoadInt32(&l.redisAvailable) == 1,
		LocalBuckets:   l.localTenant.Size() + l.localProbe.Size(),
	}
}

type LimiterStats struct {
	AllowedTotal   int64
	DeniedTotal    int64
	FallbackUsed   int64
	RedisAvailable bool
	LocalBuckets   int
}

func (l *Limiter) Reset() {
	l.localGlobal.Reset()
	l.localTenant.Clear()
	l.localProbe.Clear()

	atomic.StoreInt64(&l.allowedTotal, 0)
	atomic.StoreInt64(&l.deniedTotal, 0)
	atomic.StoreInt64(&l.fallbackUsed, 0)
}

func (l *Limiter) ResetTenant(ctx context.Context, tenantID string) error {

	l.localTenant.Reset(tenantID)

	if l.config.RedisEnabled && atomic.LoadInt32(&l.redisAvailable) == 1 {
		tenantKey := l.config.RedisPrefix + "tenant:" + tenantID
		return l.redis.Del(ctx, tenantKey).Err()
	}
	return nil
}

func (l *Limiter) ResetProbe(ctx context.Context, probeID string) error {

	l.localProbe.Reset(probeID)

	if l.config.RedisEnabled && atomic.LoadInt32(&l.redisAvailable) == 1 {
		probeKey := l.config.RedisPrefix + "probe:" + probeID
		return l.redis.Del(ctx, probeKey).Err()
	}
	return nil
}

func (l *Limiter) GetTokensRemaining(ctx context.Context, tenantID, probeID string) (global, tenant, probe float64, err error) {
	if !l.config.RedisEnabled || atomic.LoadInt32(&l.redisAvailable) == 0 {

		return l.localGlobal.Available(),
			l.localTenant.GetBucket(tenantID).Available(),
			l.localProbe.GetBucket(probeID).Available(),
			nil
	}

	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	globalKey := l.config.RedisPrefix + "global"
	globalTokens, err := l.redis.HGet(ctx, globalKey, "tokens").Float64()
	if err != nil && err != redis.Nil {
		return 0, 0, 0, err
	}

	tenantKey := l.config.RedisPrefix + "tenant:" + tenantID
	tenantTokens, err := l.redis.HGet(ctx, tenantKey, "tokens").Float64()
	if err != nil && err != redis.Nil {
		return 0, 0, 0, err
	}

	probeKey := l.config.RedisPrefix + "probe:" + probeID
	probeTokens, err := l.redis.HGet(ctx, probeKey, "tokens").Float64()
	if err != nil && err != redis.Nil {
		return 0, 0, 0, err
	}

	return globalTokens, tenantTokens, probeTokens, nil
}
