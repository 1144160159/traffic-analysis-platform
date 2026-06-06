package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
)

type RedisConfig struct {
	Addr     string `env:"REDIS_ADDR"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB" envDefault:"0"`

	ClusterAddrs []string `env:"REDIS_CLUSTER_ADDRS" envSeparator:","`

	SentinelAddrs  []string `env:"REDIS_SENTINEL_ADDRS" envSeparator:","`
	SentinelMaster string   `env:"REDIS_SENTINEL_MASTER"`

	PoolSize        int           `env:"REDIS_POOL_SIZE" envDefault:"10"`
	MinIdleConns    int           `env:"REDIS_MIN_IDLE_CONNS" envDefault:"5"`
	MaxRetries      int           `env:"REDIS_MAX_RETRIES" envDefault:"3"`
	DialTimeout     time.Duration `env:"REDIS_DIAL_TIMEOUT" envDefault:"5s"`
	ReadTimeout     time.Duration `env:"REDIS_READ_TIMEOUT" envDefault:"3s"`
	WriteTimeout    time.Duration `env:"REDIS_WRITE_TIMEOUT" envDefault:"3s"`
	PoolTimeout     time.Duration `env:"REDIS_POOL_TIMEOUT" envDefault:"4s"`
	ConnMaxIdleTime time.Duration `env:"REDIS_CONN_MAX_IDLE_TIME" envDefault:"30m"`
}

type RedisClient struct {
	client  redis.UniversalClient
	config  RedisConfig
	logger  *zap.Logger
	metrics *redisMetrics
}

type redisMetrics struct {
	commandDuration prometheus.Histogram
	commandErrors   prometheus.Counter
	poolStats       *prometheus.GaugeVec
}

func newRedisMetrics(serviceName string) *redisMetrics {
	return &redisMetrics{
		commandDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name:        "redis_command_duration_seconds",
			Help:        "Redis command duration in seconds",
			ConstLabels: prometheus.Labels{"service": serviceName},
			Buckets:     []float64{.0001, .0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1},
		}),
		commandErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name:        "redis_command_errors_total",
			Help:        "Total number of Redis command errors",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}),
		poolStats: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "redis_pool_connections",
			Help:        "Redis connection pool statistics",
			ConstLabels: prometheus.Labels{"service": serviceName},
		}, []string{"state"}),
	}
}

func NewRedisClient(cfg RedisConfig, logger *zap.Logger) (*RedisClient, error) {
	var client redis.UniversalClient

	if len(cfg.ClusterAddrs) > 0 {

		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:           cfg.ClusterAddrs,
			Password:        cfg.Password,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			MaxRetries:      cfg.MaxRetries,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		})
		logger.Info("Connecting to Redis Cluster", zap.Strings("addrs", cfg.ClusterAddrs))
	} else if len(cfg.SentinelAddrs) > 0 {

		client = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:      cfg.SentinelMaster,
			SentinelAddrs:   cfg.SentinelAddrs,
			Password:        cfg.Password,
			DB:              cfg.DB,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			MaxRetries:      cfg.MaxRetries,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		})
		logger.Info("Connecting to Redis Sentinel",
			zap.Strings("sentinels", cfg.SentinelAddrs),
			zap.String("master", cfg.SentinelMaster))
	} else {

		client = redis.NewClient(&redis.Options{
			Addr:            cfg.Addr,
			Password:        cfg.Password,
			DB:              cfg.DB,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			MaxRetries:      cfg.MaxRetries,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.ConnMaxIdleTime,
		})
		logger.Info("Connecting to Redis", zap.String("addr", cfg.Addr), zap.Int("db", cfg.DB))
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	logger.Info("Connected to Redis")

	rc := &RedisClient{
		client:  client,
		config:  cfg,
		logger:  logger,
		metrics: newRedisMetrics("redis"),
	}

	go rc.monitorPoolStats()

	return rc, nil
}

func (c *RedisClient) monitorPoolStats() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stats := c.client.PoolStats()
		c.metrics.poolStats.WithLabelValues("hits").Set(float64(stats.Hits))
		c.metrics.poolStats.WithLabelValues("misses").Set(float64(stats.Misses))
		c.metrics.poolStats.WithLabelValues("timeouts").Set(float64(stats.Timeouts))
		c.metrics.poolStats.WithLabelValues("total_conns").Set(float64(stats.TotalConns))
		c.metrics.poolStats.WithLabelValues("idle_conns").Set(float64(stats.IdleConns))
		c.metrics.poolStats.WithLabelValues("stale_conns").Set(float64(stats.StaleConns))
	}
}

func (c *RedisClient) Client() redis.UniversalClient {
	return c.client
}

func (c *RedisClient) Get(ctx context.Context, key string) (string, error) {
	ctx, span := otel.StartSpan(ctx, "redis.get")
	defer span.End()

	start := time.Now()
	val, err := c.client.Get(ctx, key).Result()
	c.metrics.commandDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		c.metrics.commandErrors.Inc()
		c.logger.Error("Redis GET failed", zap.Error(err), zap.String("key", key))
		return "", fmt.Errorf("get failed: %w", err)
	}

	return val, nil
}

func (c *RedisClient) GetJSON(ctx context.Context, key string, v interface{}) error {
	val, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	if val == "" {
		return nil
	}
	return json.Unmarshal([]byte(val), v)
}

func (c *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	ctx, span := otel.StartSpan(ctx, "redis.set")
	defer span.End()

	start := time.Now()
	err := c.client.Set(ctx, key, value, expiration).Err()
	c.metrics.commandDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		c.metrics.commandErrors.Inc()
		c.logger.Error("Redis SET failed", zap.Error(err), zap.String("key", key))
		return fmt.Errorf("set failed: %w", err)
	}

	return nil
}

func (c *RedisClient) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}
	return c.Set(ctx, key, data, expiration)
}

func (c *RedisClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	ctx, span := otel.StartSpan(ctx, "redis.setnx")
	defer span.End()

	start := time.Now()
	ok, err := c.client.SetNX(ctx, key, value, expiration).Result()
	c.metrics.commandDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		c.metrics.commandErrors.Inc()
		c.logger.Error("Redis SETNX failed", zap.Error(err), zap.String("key", key))
		return false, fmt.Errorf("setnx failed: %w", err)
	}

	return ok, nil
}

func (c *RedisClient) Delete(ctx context.Context, keys ...string) error {
	ctx, span := otel.StartSpan(ctx, "redis.delete")
	defer span.End()

	start := time.Now()
	err := c.client.Del(ctx, keys...).Err()
	c.metrics.commandDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		c.metrics.commandErrors.Inc()
		c.logger.Error("Redis DEL failed", zap.Error(err), zap.Strings("keys", keys))
		return fmt.Errorf("delete failed: %w", err)
	}

	return nil
}

func (c *RedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	ctx, span := otel.StartSpan(ctx, "redis.exists")
	defer span.End()

	return c.client.Exists(ctx, keys...).Result()
}

func (c *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.client.Expire(ctx, key, expiration).Err()
}

func (c *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

func (c *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	ctx, span := otel.StartSpan(ctx, "redis.incr")
	defer span.End()

	return c.client.Incr(ctx, key).Result()
}

func (c *RedisClient) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.client.IncrBy(ctx, key, value).Result()
}

func (c *RedisClient) Decr(ctx context.Context, key string) (int64, error) {
	return c.client.Decr(ctx, key).Result()
}

func (c *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	ctx, span := otel.StartSpan(ctx, "redis.hget")
	defer span.End()

	val, err := c.client.HGet(ctx, key, field).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (c *RedisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	ctx, span := otel.StartSpan(ctx, "redis.hset")
	defer span.End()

	return c.client.HSet(ctx, key, values...).Err()
}

func (c *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	ctx, span := otel.StartSpan(ctx, "redis.hgetall")
	defer span.End()

	return c.client.HGetAll(ctx, key).Result()
}

func (c *RedisClient) HDel(ctx context.Context, key string, fields ...string) error {
	return c.client.HDel(ctx, key, fields...).Err()
}

func (c *RedisClient) LPush(ctx context.Context, key string, values ...interface{}) error {
	return c.client.LPush(ctx, key, values...).Err()
}

func (c *RedisClient) RPush(ctx context.Context, key string, values ...interface{}) error {
	return c.client.RPush(ctx, key, values...).Err()
}

func (c *RedisClient) LPop(ctx context.Context, key string) (string, error) {
	val, err := c.client.LPop(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (c *RedisClient) RPop(ctx context.Context, key string) (string, error) {
	val, err := c.client.RPop(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (c *RedisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.LRange(ctx, key, start, stop).Result()
}

func (c *RedisClient) LLen(ctx context.Context, key string) (int64, error) {
	return c.client.LLen(ctx, key).Result()
}

func (c *RedisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SAdd(ctx, key, members...).Err()
}

func (c *RedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, key).Result()
}

func (c *RedisClient) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return c.client.SIsMember(ctx, key, member).Result()
}

func (c *RedisClient) SRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SRem(ctx, key, members...).Err()
}

func (c *RedisClient) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	return c.client.ZAdd(ctx, key, members...).Err()
}

func (c *RedisClient) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.ZRange(ctx, key, start, stop).Result()
}

func (c *RedisClient) ZRangeByScore(ctx context.Context, key string, opt *redis.ZRangeBy) ([]string, error) {
	return c.client.ZRangeByScore(ctx, key, opt).Result()
}

func (c *RedisClient) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.ZRem(ctx, key, members...).Err()
}

func (c *RedisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	ctx, span := otel.StartSpan(ctx, "redis.eval")
	defer span.End()

	return c.client.Eval(ctx, script, keys, args...).Result()
}

func (c *RedisClient) Pipeline() redis.Pipeliner {
	return c.client.Pipeline()
}

func (c *RedisClient) TxPipeline() redis.Pipeliner {
	return c.client.TxPipeline()
}

func (c *RedisClient) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisClient) Close() error {
	c.logger.Info("Closing Redis connection")
	return c.client.Close()
}

type RedisHealthChecker struct {
	client *RedisClient
}

func NewRedisHealthChecker(client *RedisClient) *RedisHealthChecker {
	return &RedisHealthChecker{client: client}
}

func (h *RedisHealthChecker) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return h.client.Ping(ctx)
}

func (h *RedisHealthChecker) Name() string {
	return "redis"
}

type TokenBucketConfig struct {
	DefaultTTL time.Duration
}

func DefaultTokenBucketConfig() TokenBucketConfig {
	return TokenBucketConfig{
		DefaultTTL: time.Hour,
	}
}

type TokenBucket struct {
	client *RedisClient
	script *redis.Script
	config TokenBucketConfig
}

func NewTokenBucket(client *RedisClient) *TokenBucket {
	return NewTokenBucketWithConfig(client, DefaultTokenBucketConfig())
}

func NewTokenBucketWithConfig(client *RedisClient, config TokenBucketConfig) *TokenBucket {

	script := redis.NewScript(`
        local key = KEYS[1]
        local capacity = tonumber(ARGV[1])
        local rate = tonumber(ARGV[2])
        local now = tonumber(ARGV[3])
        local requested = tonumber(ARGV[4])
        local ttl = tonumber(ARGV[5]) or 3600
        
        local bucket = redis.call('HMGET', key, 'tokens', 'last_update')
        local tokens = tonumber(bucket[1])
        local last_update = tonumber(bucket[2])
        
        if tokens == nil then
            tokens = capacity
            last_update = now
        end
        
        -- 计算经过的时间（秒）
        local delta = (now - last_update) / 1000
        tokens = math.min(capacity, tokens + delta * rate)
        
        local allowed = tokens >= requested
        if allowed then
            tokens = tokens - requested
        end
        
        redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
        redis.call('EXPIRE', key, ttl)
        
        return allowed and 1 or 0
    `)

	return &TokenBucket{
		client: client,
		script: script,
		config: config,
	}
}

func (tb *TokenBucket) Allow(ctx context.Context, key string, capacity, rate float64, requested int) (bool, error) {
	ttlSeconds := int(tb.config.DefaultTTL.Seconds())
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	return tb.AllowWithTTL(ctx, key, capacity, rate, requested, ttlSeconds)
}

func (tb *TokenBucket) AllowWithTTL(ctx context.Context, key string, capacity, rate float64, requested int, ttlSeconds int) (bool, error) {
	now := time.Now().UnixMilli()

	result, err := tb.script.Run(ctx, tb.client.client, []string{key},
		capacity, rate, now, requested, ttlSeconds).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

func (tb *TokenBucket) GetTokens(ctx context.Context, key string) (float64, error) {
	val, err := tb.client.HGet(ctx, key, "tokens")
	if err != nil {
		return 0, err
	}
	if val == "" {
		return 0, nil
	}
	var tokens float64
	_, err = fmt.Sscanf(val, "%f", &tokens)
	return tokens, err
}

func (tb *TokenBucket) Reset(ctx context.Context, key string) error {
	return tb.client.Delete(ctx, key)
}

type SlidingWindowRateLimiter struct {
	client *RedisClient
	script *redis.Script
}

func NewSlidingWindowRateLimiter(client *RedisClient) *SlidingWindowRateLimiter {

	script := redis.NewScript(`
        local key = KEYS[1]
        local limit = tonumber(ARGV[1])
        local window = tonumber(ARGV[2])
        local now = tonumber(ARGV[3])
        
        -- 清理过期数据（保留余量避免精确边界问题）
        local min_score = now - window * 1000 - 1000
        redis.call('ZREMRANGEBYSCORE', key, 0, min_score)
        
        -- 获取当前窗口内的请求数
        local current = redis.call('ZCARD', key)
        
        if current >= limit then
            return 0
        end
        
        -- 添加新请求（使用唯一标识避免冲突）
        local member = now .. ':' .. math.random(1000000, 9999999)
        redis.call('ZADD', key, now, member)
        redis.call('EXPIRE', key, window + 10)
        
        return 1
    `)

	return &SlidingWindowRateLimiter{
		client: client,
		script: script,
	}
}

func (rl *SlidingWindowRateLimiter) Allow(ctx context.Context, key string, limit int, windowSeconds int) (bool, error) {
	now := time.Now().UnixMilli()

	result, err := rl.script.Run(ctx, rl.client.client, []string{key},
		limit, windowSeconds, now).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

func (rl *SlidingWindowRateLimiter) GetCount(ctx context.Context, key string, windowSeconds int) (int64, error) {
	now := time.Now().UnixMilli()
	minScore := now - int64(windowSeconds*1000)

	return rl.client.client.ZCount(ctx, key,
		fmt.Sprintf("%d", minScore),
		fmt.Sprintf("%d", now)).Result()
}

type LockState int32

const (
	LockStateUnlocked LockState = iota
	LockStateLocked
	LockStateReleased
	LockStateLost
)

type LockLostCallback func(lock *DistributedLock, reason string)

type DistributedLockConfig struct {
	Expiration time.Duration

	AutoRenew bool

	RenewInterval time.Duration

	MaxRenewRetries int

	WaitTimeout time.Duration

	RetryInterval time.Duration

	ReacquireOnLost bool
}

func DefaultDistributedLockConfig() DistributedLockConfig {
	return DistributedLockConfig{
		Expiration:      30 * time.Second,
		AutoRenew:       true,
		RenewInterval:   10 * time.Second,
		MaxRenewRetries: 3,
		WaitTimeout:     0,
		RetryInterval:   100 * time.Millisecond,
		ReacquireOnLost: false,
	}
}

type DistributedLock struct {
	client     *RedisClient
	key        string
	value      string
	config     DistributedLockConfig
	state      int32
	cancelFunc context.CancelFunc
	mu         sync.Mutex
	logger     *zap.Logger

	acquiredAt    time.Time
	renewCount    int64
	renewFailures int64

	onLostCallback LockLostCallback
}

func NewDistributedLock(client *RedisClient, key string, expiration time.Duration) *DistributedLock {
	config := DefaultDistributedLockConfig()
	config.Expiration = expiration
	config.RenewInterval = expiration / 3

	return NewDistributedLockWithConfig(client, key, config, nil)
}

func NewDistributedLockWithConfig(client *RedisClient, key string, config DistributedLockConfig, logger *zap.Logger) *DistributedLock {

	if config.RenewInterval <= 0 {
		config.RenewInterval = config.Expiration / 3
	}
	if config.RenewInterval >= config.Expiration {
		config.RenewInterval = config.Expiration / 3
	}
	if config.MaxRenewRetries <= 0 {
		config.MaxRenewRetries = 3
	}

	return &DistributedLock{
		client: client,
		key:    "lock:" + key,
		value:  fmt.Sprintf("%d:%d", time.Now().UnixNano(), time.Now().UnixNano()%1000000),
		config: config,
		state:  int32(LockStateUnlocked),
		logger: logger,
	}
}

func (l *DistributedLock) OnLost(callback LockLostCallback) *DistributedLock {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onLostCallback = callback
	return l
}

func (l *DistributedLock) TryLock(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	state := LockState(atomic.LoadInt32(&l.state))
	if state != LockStateUnlocked && state != LockStateLost {
		return false, fmt.Errorf("lock is already held or released")
	}

	ok, err := l.client.SetNX(ctx, l.key, l.value, l.config.Expiration)
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if ok {
		atomic.StoreInt32(&l.state, int32(LockStateLocked))
		l.acquiredAt = time.Now()

		if l.config.AutoRenew {
			l.startAutoRenew()
		}

		if l.logger != nil {
			l.logger.Debug("Lock acquired",
				zap.String("key", l.key),
				zap.Duration("expiration", l.config.Expiration))
		}
	}

	return ok, nil
}

func (l *DistributedLock) Lock(ctx context.Context) error {
	l.mu.Lock()

	state := LockState(atomic.LoadInt32(&l.state))
	if state != LockStateUnlocked && state != LockStateLost {
		l.mu.Unlock()
		return fmt.Errorf("lock is already held or released")
	}
	l.mu.Unlock()

	deadline := time.Now().Add(l.config.WaitTimeout)
	if l.config.WaitTimeout <= 0 {
		deadline = time.Now().Add(24 * time.Hour)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for lock")
		}

		ok, err := l.TryLock(ctx)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.config.RetryInterval):

		}
	}
}

func (l *DistributedLock) LockWithTimeout(ctx context.Context, timeout time.Duration) error {
	l.config.WaitTimeout = timeout
	return l.Lock(ctx)
}

func (l *DistributedLock) startAutoRenew() {
	ctx, cancel := context.WithCancel(context.Background())
	l.cancelFunc = cancel

	go func() {
		ticker := time.NewTicker(l.config.RenewInterval)
		defer ticker.Stop()

		consecutiveFailures := 0

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				state := LockState(atomic.LoadInt32(&l.state))
				if state != LockStateLocked {
					return
				}

				success, err := l.Extend(context.Background(), l.config.Expiration)
				if err != nil || !success {
					consecutiveFailures++
					atomic.AddInt64(&l.renewFailures, 1)

					if l.logger != nil {
						l.logger.Warn("Failed to renew lock",
							zap.String("key", l.key),
							zap.Int("consecutive_failures", consecutiveFailures),
							zap.Error(err))
					}

					if consecutiveFailures >= l.config.MaxRenewRetries {
						l.handleLockLost("renewal failed after max retries")
						return
					}
				} else {
					consecutiveFailures = 0
					atomic.AddInt64(&l.renewCount, 1)

					if l.logger != nil {
						l.logger.Debug("Lock renewed",
							zap.String("key", l.key),
							zap.Int64("renew_count", atomic.LoadInt64(&l.renewCount)))
					}
				}
			}
		}
	}()
}

func (l *DistributedLock) handleLockLost(reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	oldState := LockState(atomic.SwapInt32(&l.state, int32(LockStateLost)))

	if l.logger != nil {
		l.logger.Error("Lock lost",
			zap.String("key", l.key),
			zap.String("reason", reason),
			zap.Duration("held_duration", time.Since(l.acquiredAt)),
			zap.Int64("renew_count", atomic.LoadInt64(&l.renewCount)),
			zap.Int64("renew_failures", atomic.LoadInt64(&l.renewFailures)))
	}

	if l.onLostCallback != nil {
		go l.onLostCallback(l, reason)
	}

	if l.config.ReacquireOnLost && oldState == LockStateLocked {
		go l.tryReacquire()
	}
}

func (l *DistributedLock) tryReacquire() {
	if l.logger != nil {
		l.logger.Info("Attempting to reacquire lost lock", zap.String("key", l.key))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	atomic.StoreInt32(&l.state, int32(LockStateUnlocked))

	if err := l.Lock(ctx); err != nil {
		if l.logger != nil {
			l.logger.Error("Failed to reacquire lock",
				zap.String("key", l.key),
				zap.Error(err))
		}
	} else {
		if l.logger != nil {
			l.logger.Info("Successfully reacquired lock", zap.String("key", l.key))
		}
	}
}

func (l *DistributedLock) Unlock(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	state := LockState(atomic.LoadInt32(&l.state))
	if state == LockStateUnlocked {
		return fmt.Errorf("lock is not held")
	}
	if state == LockStateReleased {
		return nil
	}

	if l.cancelFunc != nil {
		l.cancelFunc()
		l.cancelFunc = nil
	}

	script := `
        if redis.call('GET', KEYS[1]) == ARGV[1] then
            return redis.call('DEL', KEYS[1])
        else
            return 0
        end
    `
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	atomic.StoreInt32(&l.state, int32(LockStateReleased))

	if l.logger != nil {
		deleted := result.(int64) == 1
		l.logger.Debug("Lock released",
			zap.String("key", l.key),
			zap.Bool("deleted", deleted),
			zap.Duration("held_duration", time.Since(l.acquiredAt)),
			zap.Int64("renew_count", atomic.LoadInt64(&l.renewCount)))
	}

	return nil
}

func (l *DistributedLock) Extend(ctx context.Context, expiration time.Duration) (bool, error) {
	state := LockState(atomic.LoadInt32(&l.state))
	if state != LockStateLocked {
		return false, fmt.Errorf("lock is not held")
	}

	script := `
        if redis.call('GET', KEYS[1]) == ARGV[1] then
            return redis.call('PEXPIRE', KEYS[1], ARGV[2])
        else
            return 0
        end
    `
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value, int64(expiration/time.Millisecond))
	if err != nil {
		return false, fmt.Errorf("failed to extend lock: %w", err)
	}
	return result.(int64) == 1, nil
}

func (l *DistributedLock) IsHeld() bool {
	return LockState(atomic.LoadInt32(&l.state)) == LockStateLocked
}

func (l *DistributedLock) GetState() LockState {
	return LockState(atomic.LoadInt32(&l.state))
}

func (l *DistributedLock) GetStats() LockStats {
	return LockStats{
		Key:           l.key,
		State:         LockState(atomic.LoadInt32(&l.state)),
		AcquiredAt:    l.acquiredAt,
		RenewCount:    atomic.LoadInt64(&l.renewCount),
		RenewFailures: atomic.LoadInt64(&l.renewFailures),
	}
}

type LockStats struct {
	Key           string
	State         LockState
	AcquiredAt    time.Time
	RenewCount    int64
	RenewFailures int64
}

func (s LockState) String() string {
	switch s {
	case LockStateUnlocked:
		return "unlocked"
	case LockStateLocked:
		return "locked"
	case LockStateReleased:
		return "released"
	case LockStateLost:
		return "lost"
	default:
		return "unknown"
	}
}

type LockManager struct {
	client *RedisClient
	config DistributedLockConfig
	locks  sync.Map
	logger *zap.Logger
}

func NewLockManager(client *RedisClient, logger *zap.Logger) *LockManager {
	return &LockManager{
		client: client,
		config: DefaultDistributedLockConfig(),
		logger: logger,
	}
}

func NewLockManagerWithConfig(client *RedisClient, config DistributedLockConfig, logger *zap.Logger) *LockManager {
	return &LockManager{
		client: client,
		config: config,
		logger: logger,
	}
}

func (m *LockManager) NewLock(key string) *DistributedLock {
	return NewDistributedLockWithConfig(m.client, key, m.config, m.logger)
}

func (m *LockManager) NewLockWithExpiration(key string, expiration time.Duration) *DistributedLock {
	config := m.config
	config.Expiration = expiration
	config.RenewInterval = expiration / 3
	return NewDistributedLockWithConfig(m.client, key, config, m.logger)
}

func (m *LockManager) Acquire(ctx context.Context, key string) (*DistributedLock, error) {
	lock := m.NewLock(key)
	if err := lock.Lock(ctx); err != nil {
		return nil, err
	}

	m.locks.Store(key, lock)

	return lock, nil
}

func (m *LockManager) TryAcquire(ctx context.Context, key string) (*DistributedLock, bool, error) {
	lock := m.NewLock(key)
	ok, err := lock.TryLock(ctx)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	m.locks.Store(key, lock)

	return lock, true, nil
}

func (m *LockManager) Release(ctx context.Context, key string) error {
	if v, ok := m.locks.Load(key); ok {
		lock := v.(*DistributedLock)
		m.locks.Delete(key)
		return lock.Unlock(ctx)
	}
	return fmt.Errorf("lock not found: %s", key)
}

func (m *LockManager) ReleaseAll(ctx context.Context) {
	m.locks.Range(func(key, value interface{}) bool {
		if lock, ok := value.(*DistributedLock); ok {
			if err := lock.Unlock(ctx); err != nil && m.logger != nil {
				m.logger.Warn("Failed to release lock",
					zap.String("key", key.(string)),
					zap.Error(err))
			}
		}
		m.locks.Delete(key)
		return true
	})
}

func (m *LockManager) GetLock(key string) (*DistributedLock, bool) {
	if v, ok := m.locks.Load(key); ok {
		return v.(*DistributedLock), true
	}
	return nil, false
}

func (m *LockManager) GetAllLocks() []*DistributedLock {
	var locks []*DistributedLock
	m.locks.Range(func(key, value interface{}) bool {
		if lock, ok := value.(*DistributedLock); ok {
			locks = append(locks, lock)
		}
		return true
	})
	return locks
}

func (m *LockManager) WithLock(ctx context.Context, key string, fn func() error) error {
	lock, err := m.Acquire(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		if err := m.Release(ctx, key); err != nil && m.logger != nil {
			m.logger.Warn("Failed to release lock in WithLock",
				zap.String("key", key),
				zap.Error(err))
		}
	}()

	_ = lock
	return fn()
}

func (m *LockManager) TryWithLock(ctx context.Context, key string, fn func() error) (bool, error) {
	lock, ok, err := m.TryAcquire(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to try acquire lock: %w", err)
	}
	if !ok {
		return false, nil
	}
	defer func() {
		if err := m.Release(ctx, key); err != nil && m.logger != nil {
			m.logger.Warn("Failed to release lock in TryWithLock",
				zap.String("key", key),
				zap.Error(err))
		}
	}()

	_ = lock
	return true, fn()
}

type RedisMutex struct {
	manager *LockManager
	key     string
	lock    *DistributedLock
	mu      sync.Mutex
}

func NewRedisMutex(manager *LockManager, key string) *RedisMutex {
	return &RedisMutex{
		manager: manager,
		key:     key,
	}
}

func (m *RedisMutex) Lock(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lock != nil {
		return fmt.Errorf("mutex already locked")
	}

	lock, err := m.manager.Acquire(ctx, m.key)
	if err != nil {
		return err
	}

	m.lock = lock
	return nil
}

func (m *RedisMutex) TryLock(ctx context.Context) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lock != nil {
		return false
	}

	lock, ok, err := m.manager.TryAcquire(ctx, m.key)
	if err != nil || !ok {
		return false
	}

	m.lock = lock
	return true
}

func (m *RedisMutex) Unlock(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lock == nil {
		return fmt.Errorf("mutex not locked")
	}

	err := m.lock.Unlock(ctx)
	m.lock = nil
	return err
}

func (m *RedisMutex) IsLocked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lock != nil && m.lock.IsHeld()
}
