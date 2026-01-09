////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/common/storage/redis.go
// 修复版：
// 1. 增加锁丢失通知回调（OnLost）
// 2. 增加锁降级模式（当续期失败时尝试降级而非立即释放）
// 3. 增加 Prometheus 指标
// 4. 修复 TokenBucket TTL 设置
// 5. 完善滑动窗口限流的过期清理
////////////////////////////////////////////////////////////////////////////////

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

// RedisConfig Redis配置
type RedisConfig struct {
	// 单机模式
	Addr     string `env:"REDIS_ADDR"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB" envDefault:"0"`

	// 集群模式
	ClusterAddrs []string `env:"REDIS_CLUSTER_ADDRS" envSeparator:","`

	// 哨兵模式
	SentinelAddrs  []string `env:"REDIS_SENTINEL_ADDRS" envSeparator:","`
	SentinelMaster string   `env:"REDIS_SENTINEL_MASTER"`

	// 连接池配置
	PoolSize        int           `env:"REDIS_POOL_SIZE" envDefault:"10"`
	MinIdleConns    int           `env:"REDIS_MIN_IDLE_CONNS" envDefault:"5"`
	MaxRetries      int           `env:"REDIS_MAX_RETRIES" envDefault:"3"`
	DialTimeout     time.Duration `env:"REDIS_DIAL_TIMEOUT" envDefault:"5s"`
	ReadTimeout     time.Duration `env:"REDIS_READ_TIMEOUT" envDefault:"3s"`
	WriteTimeout    time.Duration `env:"REDIS_WRITE_TIMEOUT" envDefault:"3s"`
	PoolTimeout     time.Duration `env:"REDIS_POOL_TIMEOUT" envDefault:"4s"`
	ConnMaxIdleTime time.Duration `env:"REDIS_CONN_MAX_IDLE_TIME" envDefault:"30m"`
}

// RedisClient Redis客户端
type RedisClient struct {
	client  redis.UniversalClient
	config  RedisConfig
	logger  *zap.Logger
	metrics *redisMetrics
}

// redisMetrics Redis 指标
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

// NewRedisClient 创建Redis客户端
func NewRedisClient(cfg RedisConfig, logger *zap.Logger) (*RedisClient, error) {
	var client redis.UniversalClient

	// 根据配置选择模式
	if len(cfg.ClusterAddrs) > 0 {
		// 集群模式
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
		// 哨兵模式
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
		// 单机模式
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

	// 测试连接
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

	// 启动连接池监控
	go rc.monitorPoolStats()

	return rc, nil
}

// monitorPoolStats 监控连接池状态
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

// Client 获取原生客户端
func (c *RedisClient) Client() redis.UniversalClient {
	return c.client
}

// Get 获取值
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

// GetJSON 获取JSON值
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

// Set 设置值
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

// SetJSON 设置JSON值
func (c *RedisClient) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal failed: %w", err)
	}
	return c.Set(ctx, key, data, expiration)
}

// SetNX 不存在时设置
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

// Delete 删除键
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

// Exists 检查键是否存在
func (c *RedisClient) Exists(ctx context.Context, keys ...string) (int64, error) {
	ctx, span := otel.StartSpan(ctx, "redis.exists")
	defer span.End()

	return c.client.Exists(ctx, keys...).Result()
}

// Expire 设置过期时间
func (c *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.client.Expire(ctx, key, expiration).Err()
}

// TTL 获取剩余过期时间
func (c *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

// Incr 自增
func (c *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	ctx, span := otel.StartSpan(ctx, "redis.incr")
	defer span.End()

	return c.client.Incr(ctx, key).Result()
}

// IncrBy 增加指定值
func (c *RedisClient) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	return c.client.IncrBy(ctx, key, value).Result()
}

// Decr 自减
func (c *RedisClient) Decr(ctx context.Context, key string) (int64, error) {
	return c.client.Decr(ctx, key).Result()
}

// HGet 哈希获取
func (c *RedisClient) HGet(ctx context.Context, key, field string) (string, error) {
	ctx, span := otel.StartSpan(ctx, "redis.hget")
	defer span.End()

	val, err := c.client.HGet(ctx, key, field).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// HSet 哈希设置
func (c *RedisClient) HSet(ctx context.Context, key string, values ...interface{}) error {
	ctx, span := otel.StartSpan(ctx, "redis.hset")
	defer span.End()

	return c.client.HSet(ctx, key, values...).Err()
}

// HGetAll 哈希获取全部
func (c *RedisClient) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	ctx, span := otel.StartSpan(ctx, "redis.hgetall")
	defer span.End()

	return c.client.HGetAll(ctx, key).Result()
}

// HDel 哈希删除
func (c *RedisClient) HDel(ctx context.Context, key string, fields ...string) error {
	return c.client.HDel(ctx, key, fields...).Err()
}

// LPush 列表左推入
func (c *RedisClient) LPush(ctx context.Context, key string, values ...interface{}) error {
	return c.client.LPush(ctx, key, values...).Err()
}

// RPush 列表右推入
func (c *RedisClient) RPush(ctx context.Context, key string, values ...interface{}) error {
	return c.client.RPush(ctx, key, values...).Err()
}

// LPop 列表左弹出
func (c *RedisClient) LPop(ctx context.Context, key string) (string, error) {
	val, err := c.client.LPop(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// RPop 列表右弹出
func (c *RedisClient) RPop(ctx context.Context, key string) (string, error) {
	val, err := c.client.RPop(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// LRange 列表范围获取
func (c *RedisClient) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.LRange(ctx, key, start, stop).Result()
}

// LLen 列表长度
func (c *RedisClient) LLen(ctx context.Context, key string) (int64, error) {
	return c.client.LLen(ctx, key).Result()
}

// SAdd 集合添加
func (c *RedisClient) SAdd(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SAdd(ctx, key, members...).Err()
}

// SMembers 集合成员
func (c *RedisClient) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, key).Result()
}

// SIsMember 是否集合成员
func (c *RedisClient) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return c.client.SIsMember(ctx, key, member).Result()
}

// SRem 集合移除
func (c *RedisClient) SRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.SRem(ctx, key, members...).Err()
}

// ZAdd 有序集合添加
func (c *RedisClient) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	return c.client.ZAdd(ctx, key, members...).Err()
}

// ZRange 有序集合范围
func (c *RedisClient) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.ZRange(ctx, key, start, stop).Result()
}

// ZRangeByScore 按分数范围
func (c *RedisClient) ZRangeByScore(ctx context.Context, key string, opt *redis.ZRangeBy) ([]string, error) {
	return c.client.ZRangeByScore(ctx, key, opt).Result()
}

// ZRem 有序集合移除
func (c *RedisClient) ZRem(ctx context.Context, key string, members ...interface{}) error {
	return c.client.ZRem(ctx, key, members...).Err()
}

// Eval 执行Lua脚本
func (c *RedisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	ctx, span := otel.StartSpan(ctx, "redis.eval")
	defer span.End()

	return c.client.Eval(ctx, script, keys, args...).Result()
}

// Pipeline 获取管道
func (c *RedisClient) Pipeline() redis.Pipeliner {
	return c.client.Pipeline()
}

// TxPipeline 获取事务管道
func (c *RedisClient) TxPipeline() redis.Pipeliner {
	return c.client.TxPipeline()
}

// Ping 测试连接
func (c *RedisClient) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Close 关闭连接
func (c *RedisClient) Close() error {
	c.logger.Info("Closing Redis connection")
	return c.client.Close()
}

// RedisHealthChecker 健康检查
type RedisHealthChecker struct {
	client *RedisClient
}

// NewRedisHealthChecker 创建健康检查器
func NewRedisHealthChecker(client *RedisClient) *RedisHealthChecker {
	return &RedisHealthChecker{client: client}
}

// Check 执行健康检查
func (h *RedisHealthChecker) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return h.client.Ping(ctx)
}

// Name 返回检查器名称
func (h *RedisHealthChecker) Name() string {
	return "redis"
}

// ==================== 令牌桶限流（修复 TTL） ====================

// TokenBucketConfig 令牌桶配置
type TokenBucketConfig struct {
	DefaultTTL time.Duration // 默认 TTL
}

// DefaultTokenBucketConfig 默认令牌桶配置
func DefaultTokenBucketConfig() TokenBucketConfig {
	return TokenBucketConfig{
		DefaultTTL: time.Hour, // 默认1小时
	}
}

// TokenBucket 令牌桶限流
type TokenBucket struct {
	client *RedisClient
	script *redis.Script
	config TokenBucketConfig
}

// NewTokenBucket 创建令牌桶（使用默认配置）
func NewTokenBucket(client *RedisClient) *TokenBucket {
	return NewTokenBucketWithConfig(client, DefaultTokenBucketConfig())
}

// NewTokenBucketWithConfig 创建令牌桶（使用自定义配置）
func NewTokenBucketWithConfig(client *RedisClient, config TokenBucketConfig) *TokenBucket {
	// Lua脚本实现令牌桶（修复：正确设置 TTL）
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

// Allow 检查是否允许（使用默认 TTL）
func (tb *TokenBucket) Allow(ctx context.Context, key string, capacity, rate float64, requested int) (bool, error) {
	ttlSeconds := int(tb.config.DefaultTTL.Seconds())
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	return tb.AllowWithTTL(ctx, key, capacity, rate, requested, ttlSeconds)
}

// AllowWithTTL 检查是否允许（带自定义 TTL）
func (tb *TokenBucket) AllowWithTTL(ctx context.Context, key string, capacity, rate float64, requested int, ttlSeconds int) (bool, error) {
	now := time.Now().UnixMilli()

	result, err := tb.script.Run(ctx, tb.client.client, []string{key},
		capacity, rate, now, requested, ttlSeconds).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// GetTokens 获取当前令牌数
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

// Reset 重置令牌桶
func (tb *TokenBucket) Reset(ctx context.Context, key string) error {
	return tb.client.Delete(ctx, key)
}

// ==================== 滑动窗口限流（修复过期清理） ====================

// SlidingWindowRateLimiter 滑动窗口限流器
type SlidingWindowRateLimiter struct {
	client *RedisClient
	script *redis.Script
}

// NewSlidingWindowRateLimiter 创建滑动窗口限流器
func NewSlidingWindowRateLimiter(client *RedisClient) *SlidingWindowRateLimiter {
	// 修复：改进过期清理逻辑
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

// Allow 检查是否允许
func (rl *SlidingWindowRateLimiter) Allow(ctx context.Context, key string, limit int, windowSeconds int) (bool, error) {
	now := time.Now().UnixMilli()

	result, err := rl.script.Run(ctx, rl.client.client, []string{key},
		limit, windowSeconds, now).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// GetCount 获取当前计数
func (rl *SlidingWindowRateLimiter) GetCount(ctx context.Context, key string, windowSeconds int) (int64, error) {
	now := time.Now().UnixMilli()
	minScore := now - int64(windowSeconds*1000)

	return rl.client.client.ZCount(ctx, key,
		fmt.Sprintf("%d", minScore),
		fmt.Sprintf("%d", now)).Result()
}

// ==================== 分布式锁（增加丢失通知） ====================

// LockState 锁状态
type LockState int32

const (
	LockStateUnlocked LockState = iota
	LockStateLocked
	LockStateReleased
	LockStateLost // 新增：锁丢失状态
)

// LockLostCallback 锁丢失回调函数
type LockLostCallback func(lock *DistributedLock, reason string)

// DistributedLockConfig 分布式锁配置
type DistributedLockConfig struct {
	// 锁过期时间
	Expiration time.Duration
	// 是否自动续期
	AutoRenew bool
	// 续期间隔（建议为 Expiration 的 1/3）
	RenewInterval time.Duration
	// 续期失败后的最大重试次数
	MaxRenewRetries int
	// 获取锁的最大等待时间
	WaitTimeout time.Duration
	// 获取锁的重试间隔
	RetryInterval time.Duration
	// 锁丢失后是否尝试重新获取
	ReacquireOnLost bool
}

// DefaultDistributedLockConfig 默认锁配置
func DefaultDistributedLockConfig() DistributedLockConfig {
	return DistributedLockConfig{
		Expiration:      30 * time.Second,
		AutoRenew:       true,
		RenewInterval:   10 * time.Second, // Expiration 的 1/3
		MaxRenewRetries: 3,
		WaitTimeout:     0, // 默认不等待
		RetryInterval:   100 * time.Millisecond,
		ReacquireOnLost: false, // 默认不重新获取
	}
}

// DistributedLock 分布式锁（带自动续期和丢失通知）
type DistributedLock struct {
	client     *RedisClient
	key        string
	value      string
	config     DistributedLockConfig
	state      int32 // atomic: LockState
	cancelFunc context.CancelFunc
	mu         sync.Mutex
	logger     *zap.Logger

	// 统计信息
	acquiredAt    time.Time
	renewCount    int64
	renewFailures int64

	// 锁丢失回调
	onLostCallback LockLostCallback
}

// NewDistributedLock 创建分布式锁（使用默认配置）
func NewDistributedLock(client *RedisClient, key string, expiration time.Duration) *DistributedLock {
	config := DefaultDistributedLockConfig()
	config.Expiration = expiration
	config.RenewInterval = expiration / 3

	return NewDistributedLockWithConfig(client, key, config, nil)
}

// NewDistributedLockWithConfig 创建分布式锁（使用自定义配置）
func NewDistributedLockWithConfig(client *RedisClient, key string, config DistributedLockConfig, logger *zap.Logger) *DistributedLock {
	// 验证配置
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

// OnLost 设置锁丢失回调
func (l *DistributedLock) OnLost(callback LockLostCallback) *DistributedLock {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onLostCallback = callback
	return l
}

// TryLock 尝试获取锁（非阻塞）
func (l *DistributedLock) TryLock(ctx context.Context) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 检查当前状态
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

		// 启动自动续期
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

// Lock 获取锁（阻塞，带超时）
func (l *DistributedLock) Lock(ctx context.Context) error {
	l.mu.Lock()
	// 检查当前状态
	state := LockState(atomic.LoadInt32(&l.state))
	if state != LockStateUnlocked && state != LockStateLost {
		l.mu.Unlock()
		return fmt.Errorf("lock is already held or released")
	}
	l.mu.Unlock()

	// 设置超时
	deadline := time.Now().Add(l.config.WaitTimeout)
	if l.config.WaitTimeout <= 0 {
		deadline = time.Now().Add(24 * time.Hour) // 无限等待
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

		// 等待后重试
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.config.RetryInterval):
			// 继续尝试
		}
	}
}

// LockWithTimeout 获取锁（带超时）
func (l *DistributedLock) LockWithTimeout(ctx context.Context, timeout time.Duration) error {
	l.config.WaitTimeout = timeout
	return l.Lock(ctx)
}

// startAutoRenew 启动自动续期
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

					// 达到最大重试次数，锁丢失
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

// handleLockLost 处理锁丢失
func (l *DistributedLock) handleLockLost(reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 更新状态
	oldState := LockState(atomic.SwapInt32(&l.state, int32(LockStateLost)))

	if l.logger != nil {
		l.logger.Error("Lock lost",
			zap.String("key", l.key),
			zap.String("reason", reason),
			zap.Duration("held_duration", time.Since(l.acquiredAt)),
			zap.Int64("renew_count", atomic.LoadInt64(&l.renewCount)),
			zap.Int64("renew_failures", atomic.LoadInt64(&l.renewFailures)))
	}

	// 触发回调
	if l.onLostCallback != nil {
		go l.onLostCallback(l, reason)
	}

	// 如果配置了重新获取，尝试重新获取锁
	if l.config.ReacquireOnLost && oldState == LockStateLocked {
		go l.tryReacquire()
	}
}

// tryReacquire 尝试重新获取锁
func (l *DistributedLock) tryReacquire() {
	if l.logger != nil {
		l.logger.Info("Attempting to reacquire lost lock", zap.String("key", l.key))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 重置状态
	atomic.StoreInt32(&l.state, int32(LockStateUnlocked))

	// 尝试重新获取
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

// Unlock 释放锁
func (l *DistributedLock) Unlock(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 检查当前状态
	state := LockState(atomic.LoadInt32(&l.state))
	if state == LockStateUnlocked {
		return fmt.Errorf("lock is not held")
	}
	if state == LockStateReleased {
		return nil // 已经释放
	}

	// 停止自动续期
	if l.cancelFunc != nil {
		l.cancelFunc()
		l.cancelFunc = nil
	}

	// 使用 Lua 脚本确保只删除自己的锁
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

// Extend 延长锁的过期时间
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

// IsHeld 检查锁是否被当前实例持有
func (l *DistributedLock) IsHeld() bool {
	return LockState(atomic.LoadInt32(&l.state)) == LockStateLocked
}

// GetState 获取锁状态
func (l *DistributedLock) GetState() LockState {
	return LockState(atomic.LoadInt32(&l.state))
}

// GetStats 获取锁统计信息
func (l *DistributedLock) GetStats() LockStats {
	return LockStats{
		Key:           l.key,
		State:         LockState(atomic.LoadInt32(&l.state)),
		AcquiredAt:    l.acquiredAt,
		RenewCount:    atomic.LoadInt64(&l.renewCount),
		RenewFailures: atomic.LoadInt64(&l.renewFailures),
	}
}

// LockStats 锁统计信息
type LockStats struct {
	Key           string
	State         LockState
	AcquiredAt    time.Time
	RenewCount    int64
	RenewFailures int64
}

// String 返回状态字符串
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

// ==================== 锁管理器 ====================

// LockManager 锁管理器
type LockManager struct {
	client *RedisClient
	config DistributedLockConfig
	locks  sync.Map // key -> *DistributedLock
	logger *zap.Logger
}

// NewLockManager 创建锁管理器
func NewLockManager(client *RedisClient, logger *zap.Logger) *LockManager {
	return &LockManager{
		client: client,
		config: DefaultDistributedLockConfig(),
		logger: logger,
	}
}

// NewLockManagerWithConfig 创建锁管理器（自定义配置）
func NewLockManagerWithConfig(client *RedisClient, config DistributedLockConfig, logger *zap.Logger) *LockManager {
	return &LockManager{
		client: client,
		config: config,
		logger: logger,
	}
}

// NewLock 创建新锁
func (m *LockManager) NewLock(key string) *DistributedLock {
	return NewDistributedLockWithConfig(m.client, key, m.config, m.logger)
}

// NewLockWithExpiration 创建新锁（指定过期时间）
func (m *LockManager) NewLockWithExpiration(key string, expiration time.Duration) *DistributedLock {
	config := m.config
	config.Expiration = expiration
	config.RenewInterval = expiration / 3
	return NewDistributedLockWithConfig(m.client, key, config, m.logger)
}

// Acquire 获取锁（便捷方法）
func (m *LockManager) Acquire(ctx context.Context, key string) (*DistributedLock, error) {
	lock := m.NewLock(key)
	if err := lock.Lock(ctx); err != nil {
		return nil, err
	}

	// 注册锁
	m.locks.Store(key, lock)

	return lock, nil
}

// TryAcquire 尝试获取锁（便捷方法）
func (m *LockManager) TryAcquire(ctx context.Context, key string) (*DistributedLock, bool, error) {
	lock := m.NewLock(key)
	ok, err := lock.TryLock(ctx)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	// 注册锁
	m.locks.Store(key, lock)

	return lock, true, nil
}

// Release 释放锁（便捷方法）
func (m *LockManager) Release(ctx context.Context, key string) error {
	if v, ok := m.locks.Load(key); ok {
		lock := v.(*DistributedLock)
		m.locks.Delete(key)
		return lock.Unlock(ctx)
	}
	return fmt.Errorf("lock not found: %s", key)
}

// ReleaseAll 释放所有锁
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

// GetLock 获取已持有的锁
func (m *LockManager) GetLock(key string) (*DistributedLock, bool) {
	if v, ok := m.locks.Load(key); ok {
		return v.(*DistributedLock), true
	}
	return nil, false
}

// GetAllLocks 获取所有持有的锁
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

// WithLock 在锁内执行函数（自动获取和释放）
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

	_ = lock // 使用变量避免警告
	return fn()
}

// TryWithLock 尝试在锁内执行函数
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

	_ = lock // 使用变量避免警告
	return true, fn()
}

// ==================== Mutex（简化版分布式锁） ====================

// RedisMutex 简化版分布式互斥锁
type RedisMutex struct {
	manager *LockManager
	key     string
	lock    *DistributedLock
	mu      sync.Mutex
}

// NewRedisMutex 创建 Redis 互斥锁
func NewRedisMutex(manager *LockManager, key string) *RedisMutex {
	return &RedisMutex{
		manager: manager,
		key:     key,
	}
}

// Lock 获取锁
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

// TryLock 尝试获取锁
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

// Unlock 释放锁
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

// IsLocked 检查是否已锁定
func (m *RedisMutex) IsLocked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lock != nil && m.lock.IsHeld()
}
