////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/dedup/redis_dedup.go
// 完整修复版：
// 1. 修复 tenantID 指标标签错误（所有方法都传入 tenantID）
// 2. 添加批量去重支持
// 3. 完善错误处理
// 4. 增加指标和监控
// 5. 废弃不安全的 CheckAndIncrement 方法
////////////////////////////////////////////////////////////////////////////////

package dedup

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Dedup metrics
var (
	dedupCheckTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_dedup_check_total",
			Help: "Total number of dedup checks",
		},
		[]string{"tenant_id", "result"}, // result: new, duplicate, error
	)

	dedupCheckLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alert_dedup_check_latency_seconds",
			Help:    "Dedup check latency in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25},
		},
		[]string{"tenant_id"},
	)

	dedupKeyCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "alert_dedup_active_keys",
			Help: "Number of active dedup keys (estimated)",
		},
		[]string{"tenant_id"},
	)

	dedupRedisErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_dedup_redis_errors_total",
			Help: "Total number of Redis errors in dedup",
		},
		[]string{"tenant_id", "operation"},
	)
)

// RedisDedup Redis 去重器
type RedisDedup struct {
	client *redis.Client
	ttl    time.Duration
	logger *zap.Logger
	mu     sync.RWMutex
}

// NewRedisDedup 创建 Redis 去重器
func NewRedisDedup(client *redis.Client, ttl time.Duration, logger *zap.Logger) *RedisDedup {
	return &RedisDedup{
		client: client,
		ttl:    ttl,
		logger: logger,
	}
}

// DedupResult 去重结果
type DedupResult struct {
	IsNew     bool  // 是否为新告警
	Count     int64 // 累计次数
	FirstSeen int64 // 首次时间戳（毫秒）
	LastSeen  int64 // 最后时间戳（毫秒）
}

// CheckAndIncrementWithTenant 带租户信息的去重检查（推荐使用）
// 修复：正确使用传入的 tenantID 参数记录指标
func (d *RedisDedup) CheckAndIncrementWithTenant(
	ctx context.Context,
	fingerprint string,
	eventTs int64,
	tenantID string,
) (*DedupResult, error) {
	start := time.Now()

	// 使用租户隔离的 key 前缀
	keyPrefix := fmt.Sprintf("alert:dedup:%s:%s", tenantID, fingerprint)
	countKey := keyPrefix + ":count"
	firstSeenKey := keyPrefix + ":first_seen"
	lastSeenKey := keyPrefix + ":last_seen"

	// 使用事务 Pipeline 确保原子性
	pipe := d.client.TxPipeline()

	// INCR count（如果不存在则创建为1）
	incrCmd := pipe.Incr(ctx, countKey)
	pipe.Expire(ctx, countKey, d.ttl)

	// SETNX first_seen（只在不存在时设置）
	setNXCmd := pipe.SetNX(ctx, firstSeenKey, eventTs, d.ttl)

	// SET last_seen（总是更新）
	pipe.Set(ctx, lastSeenKey, eventTs, d.ttl)

	// 执行 Pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		d.logger.Error("Redis pipeline exec failed",
			zap.String("fingerprint", fingerprint),
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		// 修复：使用正确的 tenantID 记录指标
		dedupCheckTotal.WithLabelValues(tenantID, "error").Inc()
		dedupRedisErrors.WithLabelValues(tenantID, "pipeline_exec").Inc()
		return nil, fmt.Errorf("redis pipeline exec failed: %w", err)
	}

	// 获取结果
	count := incrCmd.Val()
	isNew := setNXCmd.Val()

	// 获取 first_seen 和 last_seen 的值
	firstSeen, err := d.client.Get(ctx, firstSeenKey).Int64()
	if err != nil && err != redis.Nil {
		d.logger.Warn("Failed to get first_seen",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		firstSeen = eventTs
	}

	lastSeen, err := d.client.Get(ctx, lastSeenKey).Int64()
	if err != nil && err != redis.Nil {
		d.logger.Warn("Failed to get last_seen",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		lastSeen = eventTs
	}

	result := &DedupResult{
		IsNew:     isNew,
		Count:     count,
		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
	}

	// 记录指标（修复：使用传入的 tenantID）
	duration := time.Since(start).Seconds()
	dedupCheckLatency.WithLabelValues(tenantID).Observe(duration)

	if isNew {
		dedupCheckTotal.WithLabelValues(tenantID, "new").Inc()
	} else {
		dedupCheckTotal.WithLabelValues(tenantID, "duplicate").Inc()
	}

	d.logger.Debug("Dedup check completed",
		zap.String("fingerprint", fingerprint),
		zap.String("tenant_id", tenantID),
		zap.Bool("is_new", result.IsNew),
		zap.Int64("count", result.Count),
		zap.Duration("duration", time.Since(start)))

	return result, nil
}

// CheckAndIncrement 检查并增加计数（已废弃，请使用 CheckAndIncrementWithTenant）
// Deprecated: 此方法无法正确记录 tenantID 指标，请使用 CheckAndIncrementWithTenant
func (d *RedisDedup) CheckAndIncrement(
	ctx context.Context,
	fingerprint string,
	eventTs int64,
) (*DedupResult, error) {
	// 警告日志
	d.logger.Warn("CheckAndIncrement is deprecated, use CheckAndIncrementWithTenant instead",
		zap.String("fingerprint", fingerprint))

	// 使用 "unknown" 作为默认租户 ID
	return d.CheckAndIncrementWithTenant(ctx, fingerprint, eventTs, "unknown")
}

// BatchDedupItem 批量去重项
type BatchDedupItem struct {
	Fingerprint string
	EventTs     int64
	TenantID    string
}

// BatchDedupResult 批量去重结果
type BatchDedupResult struct {
	Fingerprint string
	TenantID    string
	DedupResult
}

// BatchCheckAndIncrement 批量去重检查
// 用于批量消费场景，提高性能
func (d *RedisDedup) BatchCheckAndIncrement(
	ctx context.Context,
	items []BatchDedupItem,
) ([]*BatchDedupResult, error) {
	if len(items) == 0 {
		return nil, nil
	}

	start := time.Now()
	results := make([]*BatchDedupResult, len(items))

	// 使用 Pipeline 批量操作
	pipe := d.client.TxPipeline()

	// 存储命令引用
	type cmdRefs struct {
		incrCmd  *redis.IntCmd
		setNXCmd *redis.BoolCmd
		tenantID string
	}
	refs := make([]cmdRefs, len(items))

	for i, item := range items {
		// 使用租户隔离的 key 前缀
		keyPrefix := fmt.Sprintf("alert:dedup:%s:%s", item.TenantID, item.Fingerprint)
		countKey := keyPrefix + ":count"
		firstSeenKey := keyPrefix + ":first_seen"
		lastSeenKey := keyPrefix + ":last_seen"

		refs[i].incrCmd = pipe.Incr(ctx, countKey)
		pipe.Expire(ctx, countKey, d.ttl)
		refs[i].setNXCmd = pipe.SetNX(ctx, firstSeenKey, item.EventTs, d.ttl)
		pipe.Set(ctx, lastSeenKey, item.EventTs, d.ttl)
		refs[i].tenantID = item.TenantID
	}

	// 执行 Pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		d.logger.Error("Batch redis pipeline exec failed",
			zap.Int("count", len(items)),
			zap.Error(err))
		// 记录第一个租户的错误（批量操作可能跨租户）
		if len(items) > 0 {
			dedupRedisErrors.WithLabelValues(items[0].TenantID, "batch_pipeline_exec").Inc()
		}
		return nil, fmt.Errorf("batch redis pipeline exec failed: %w", err)
	}

	// 收集结果
	for i, item := range items {
		count := refs[i].incrCmd.Val()
		isNew := refs[i].setNXCmd.Val()

		// 获取时间戳
		keyPrefix := fmt.Sprintf("alert:dedup:%s:%s", item.TenantID, item.Fingerprint)
		firstSeenKey := keyPrefix + ":first_seen"
		lastSeenKey := keyPrefix + ":last_seen"

		firstSeen, _ := d.client.Get(ctx, firstSeenKey).Int64()
		lastSeen, _ := d.client.Get(ctx, lastSeenKey).Int64()

		if firstSeen == 0 {
			firstSeen = item.EventTs
		}
		if lastSeen == 0 {
			lastSeen = item.EventTs
		}

		results[i] = &BatchDedupResult{
			Fingerprint: item.Fingerprint,
			TenantID:    item.TenantID,
			DedupResult: DedupResult{
				IsNew:     isNew,
				Count:     count,
				FirstSeen: firstSeen,
				LastSeen:  lastSeen,
			},
		}

		// 记录指标（使用正确的 tenantID）
		if isNew {
			dedupCheckTotal.WithLabelValues(item.TenantID, "new").Inc()
		} else {
			dedupCheckTotal.WithLabelValues(item.TenantID, "duplicate").Inc()
		}
	}

	d.logger.Debug("Batch dedup check completed",
		zap.Int("count", len(items)),
		zap.Duration("duration", time.Since(start)))

	return results, nil
}

// GetCount 获取指定指纹的计数
func (d *RedisDedup) GetCount(ctx context.Context, tenantID, fingerprint string) (int64, error) {
	keyPrefix := fmt.Sprintf("alert:dedup:%s:%s", tenantID, fingerprint)
	countKey := keyPrefix + ":count"
	count, err := d.client.Get(ctx, countKey).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		dedupRedisErrors.WithLabelValues(tenantID, "get_count").Inc()
		return 0, err
	}
	return count, nil
}

// GetDedupInfo 获取完整的去重信息
func (d *RedisDedup) GetDedupInfo(ctx context.Context, tenantID, fingerprint string) (*DedupResult, error) {
	keyPrefix := fmt.Sprintf("alert:dedup:%s:%s", tenantID, fingerprint)
	countKey := keyPrefix + ":count"
	firstSeenKey := keyPrefix + ":first_seen"
	lastSeenKey := keyPrefix + ":last_seen"

	pipe := d.client.Pipeline()
	countCmd := pipe.Get(ctx, countKey)
	firstSeenCmd := pipe.Get(ctx, firstSeenKey)
	lastSeenCmd := pipe.Get(ctx, lastSeenKey)

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		dedupRedisErrors.WithLabelValues(tenantID, "get_dedup_info").Inc()
		return nil, fmt.Errorf("failed to get dedup info: %w", err)
	}

	count, _ := countCmd.Int64()
	firstSeen, _ := firstSeenCmd.Int64()
	lastSeen, _ := lastSeenCmd.Int64()

	return &DedupResult{
		IsNew:     count == 0,
		Count:     count,
		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
	}, nil
}

// Reset 重置指定指纹的去重状态
func (d *RedisDedup) Reset(ctx context.Context, tenantID, fingerprint string) error {
	keyPrefix := fmt.Sprintf("alert:dedup:%s:%s", tenantID, fingerprint)
	keys := []string{
		keyPrefix + ":count",
		keyPrefix + ":first_seen",
		keyPrefix + ":last_seen",
	}

	err := d.client.Del(ctx, keys...).Err()
	if err != nil {
		d.logger.Error("Failed to reset dedup keys",
			zap.String("fingerprint", fingerprint),
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		dedupRedisErrors.WithLabelValues(tenantID, "reset").Inc()
		return fmt.Errorf("failed to reset dedup keys: %w", err)
	}

	d.logger.Info("Dedup keys reset",
		zap.String("fingerprint", fingerprint),
		zap.String("tenant_id", tenantID))

	return nil
}

// ResetByTenant 重置指定租户的所有去重状态（慎用）
func (d *RedisDedup) ResetByTenant(ctx context.Context, tenantID string) (int64, error) {
	// 使用租户隔离的 key 前缀进行匹配
	pattern := fmt.Sprintf("alert:dedup:%s:*", tenantID)
	var cursor uint64
	var totalDeleted int64

	for {
		keys, nextCursor, err := d.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			dedupRedisErrors.WithLabelValues(tenantID, "reset_by_tenant_scan").Inc()
			return totalDeleted, fmt.Errorf("failed to scan keys: %w", err)
		}

		if len(keys) > 0 {
			deleted, err := d.client.Del(ctx, keys...).Result()
			if err != nil {
				d.logger.Warn("Failed to delete some keys",
					zap.String("tenant_id", tenantID),
					zap.Error(err))
			}
			totalDeleted += deleted
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	d.logger.Info("Tenant dedup keys reset",
		zap.String("tenant_id", tenantID),
		zap.Int64("deleted", totalDeleted))

	return totalDeleted, nil
}

// UpdateTTL 更新指定指纹的 TTL
func (d *RedisDedup) UpdateTTL(ctx context.Context, tenantID, fingerprint string, ttl time.Duration) error {
	keyPrefix := fmt.Sprintf("alert:dedup:%s:%s", tenantID, fingerprint)
	keys := []string{
		keyPrefix + ":count",
		keyPrefix + ":first_seen",
		keyPrefix + ":last_seen",
	}

	pipe := d.client.Pipeline()
	for _, key := range keys {
		pipe.Expire(ctx, key, ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		dedupRedisErrors.WithLabelValues(tenantID, "update_ttl").Inc()
		return fmt.Errorf("failed to update TTL: %w", err)
	}

	return nil
}

// GetStats 获取去重统计信息
func (d *RedisDedup) GetStats(ctx context.Context) (*DedupStats, error) {
	// 使用 DBSIZE 获取估算的 key 数量
	dbSize, err := d.client.DBSize(ctx).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get db size: %w", err)
	}

	// 获取内存使用情况
	memInfo, err := d.client.Info(ctx, "memory").Result()
	if err != nil {
		d.logger.Warn("Failed to get memory info", zap.Error(err))
	}

	return &DedupStats{
		TotalKeys:  dbSize,
		MemoryInfo: memInfo,
	}, nil
}

// GetStatsByTenant 获取指定租户的去重统计信息
func (d *RedisDedup) GetStatsByTenant(ctx context.Context, tenantID string) (*TenantDedupStats, error) {
	pattern := fmt.Sprintf("alert:dedup:%s:*:count", tenantID)
	var cursor uint64
	var keyCount int64

	for {
		keys, nextCursor, err := d.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan keys: %w", err)
		}

		keyCount += int64(len(keys))

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	// 更新指标
	dedupKeyCount.WithLabelValues(tenantID).Set(float64(keyCount))

	return &TenantDedupStats{
		TenantID:   tenantID,
		ActiveKeys: keyCount,
		CheckedAt:  time.Now(),
	}, nil
}

// DedupStats 去重统计
type DedupStats struct {
	TotalKeys  int64
	MemoryInfo string
}

// TenantDedupStats 租户去重统计
type TenantDedupStats struct {
	TenantID   string    `json:"tenant_id"`
	ActiveKeys int64     `json:"active_keys"`
	CheckedAt  time.Time `json:"checked_at"`
}

// Ping 健康检查
func (d *RedisDedup) Ping(ctx context.Context) error {
	return d.client.Ping(ctx).Err()
}

// Close 关闭连接（如果需要）
func (d *RedisDedup) Close() error {
	// Redis client 通常由调用方管理生命周期
	// 这里只是提供一个接口
	return nil
}

// SetTTL 动态设置 TTL
func (d *RedisDedup) SetTTL(ttl time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ttl = ttl
}

// GetTTL 获取当前 TTL
func (d *RedisDedup) GetTTL() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ttl
}

// ========== 辅助类型和方法 ==========

// DedupKey 去重 key 结构（用于解析）
type DedupKey struct {
	TenantID    string
	Fingerprint string
	KeyType     string // count, first_seen, last_seen
}

// ParseDedupKey 解析去重 key
func ParseDedupKey(key string) (*DedupKey, error) {
	// 格式：alert:dedup:{tenant_id}:{fingerprint}:{key_type}
	// 使用简单的字符串分割
	const prefix = "alert:dedup:"
	if len(key) < len(prefix) {
		return nil, fmt.Errorf("invalid key format: too short")
	}

	rest := key[len(prefix):]

	// 找到最后一个冒号分隔的 key_type
	lastColon := -1
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == ':' {
			lastColon = i
			break
		}
	}

	if lastColon < 0 {
		return nil, fmt.Errorf("invalid key format: missing key_type")
	}

	keyType := rest[lastColon+1:]
	rest = rest[:lastColon]

	// 找到倒数第二个冒号分隔的 fingerprint
	secondLastColon := -1
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == ':' {
			secondLastColon = i
			break
		}
	}

	var tenantID, fingerprint string
	if secondLastColon < 0 {
		// 没有租户 ID（兼容旧格式）
		tenantID = "unknown"
		fingerprint = rest
	} else {
		tenantID = rest[:secondLastColon]
		fingerprint = rest[secondLastColon+1:]
	}

	return &DedupKey{
		TenantID:    tenantID,
		Fingerprint: fingerprint,
		KeyType:     keyType,
	}, nil
}

// DedupOptions 去重选项
type DedupOptions struct {
	TTL              time.Duration
	SkipIfExists     bool // 如果已存在则跳过（不更新 last_seen）
	ReturnEarlyOnHit bool // 命中时提前返回（不获取完整信息）
}

// DefaultDedupOptions 默认去重选项
func DefaultDedupOptions() *DedupOptions {
	return &DedupOptions{
		TTL:              10 * time.Minute,
		SkipIfExists:     false,
		ReturnEarlyOnHit: false,
	}
}

// CheckAndIncrementWithOptions 带选项的去重检查
func (d *RedisDedup) CheckAndIncrementWithOptions(
	ctx context.Context,
	fingerprint string,
	eventTs int64,
	tenantID string,
	opts *DedupOptions,
) (*DedupResult, error) {
	if opts == nil {
		opts = DefaultDedupOptions()
	}

	start := time.Now()
	ttl := opts.TTL
	if ttl == 0 {
		ttl = d.ttl
	}

	keyPrefix := fmt.Sprintf("alert:dedup:%s:%s", tenantID, fingerprint)
	countKey := keyPrefix + ":count"
	firstSeenKey := keyPrefix + ":first_seen"
	lastSeenKey := keyPrefix + ":last_seen"

	// 如果设置了 SkipIfExists，先检查是否存在
	if opts.SkipIfExists {
		exists, err := d.client.Exists(ctx, countKey).Result()
		if err != nil {
			dedupCheckTotal.WithLabelValues(tenantID, "error").Inc()
			return nil, fmt.Errorf("failed to check existence: %w", err)
		}
		if exists > 0 {
			// 提前返回
			if opts.ReturnEarlyOnHit {
				dedupCheckTotal.WithLabelValues(tenantID, "duplicate").Inc()
				return &DedupResult{
					IsNew: false,
					Count: -1, // 表示未获取具体值
				}, nil
			}
			// 获取完整信息但不更新
			return d.GetDedupInfo(ctx, tenantID, fingerprint)
		}
	}

	// 正常的去重检查流程
	pipe := d.client.TxPipeline()

	incrCmd := pipe.Incr(ctx, countKey)
	pipe.Expire(ctx, countKey, ttl)
	setNXCmd := pipe.SetNX(ctx, firstSeenKey, eventTs, ttl)
	pipe.Set(ctx, lastSeenKey, eventTs, ttl)

	_, err := pipe.Exec(ctx)
	if err != nil {
		dedupCheckTotal.WithLabelValues(tenantID, "error").Inc()
		dedupRedisErrors.WithLabelValues(tenantID, "pipeline_exec").Inc()
		return nil, fmt.Errorf("redis pipeline exec failed: %w", err)
	}

	count := incrCmd.Val()
	isNew := setNXCmd.Val()

	firstSeen, _ := d.client.Get(ctx, firstSeenKey).Int64()
	lastSeen, _ := d.client.Get(ctx, lastSeenKey).Int64()

	if firstSeen == 0 {
		firstSeen = eventTs
	}
	if lastSeen == 0 {
		lastSeen = eventTs
	}

	result := &DedupResult{
		IsNew:     isNew,
		Count:     count,
		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
	}

	duration := time.Since(start).Seconds()
	dedupCheckLatency.WithLabelValues(tenantID).Observe(duration)

	if isNew {
		dedupCheckTotal.WithLabelValues(tenantID, "new").Inc()
	} else {
		dedupCheckTotal.WithLabelValues(tenantID, "duplicate").Inc()
	}

	return result, nil
}

// ========== Lua 脚本优化版本（原子操作）==========
var dedupScript = redis.NewScript(`
    local count_key = KEYS[1]
    local first_seen_key = KEYS[2]
    local last_seen_key = KEYS[3]
    local event_ts = tonumber(ARGV[1])  -- 毫秒级时间戳
    local ttl = tonumber(ARGV[2])       -- 秒级TTL

    -- INCR count
    local count = redis.call('INCR', count_key)
    redis.call('EXPIRE', count_key, ttl)

    -- SETNX first_seen（仅在不存在时设置）
    local is_new = redis.call('SETNX', first_seen_key, event_ts)
    if is_new == 1 then
        redis.call('EXPIRE', first_seen_key, ttl)
    end

    -- SET last_seen（总是更新）
    redis.call('SET', last_seen_key, event_ts)
    redis.call('EXPIRE', last_seen_key, ttl)

    -- GET timestamps（返回毫秒级）
    local first_seen = redis.call('GET', first_seen_key)
    local last_seen = redis.call('GET', last_seen_key)

    -- 确保返回数字类型（毫秒）
    return {
        count,
        is_new,
        tonumber(first_seen) or event_ts,
        tonumber(last_seen) or event_ts
    }
`)

// CheckAndIncrementAtomic 使用 Lua 脚本的原子去重检查（性能更好）
func (d *RedisDedup) CheckAndIncrementAtomic(
	ctx context.Context,
	fingerprint string,
	eventTs int64,
	tenantID string,
) (*DedupResult, error) {
	start := time.Now()

	keyPrefix := fmt.Sprintf("alert:dedup:%s:%s", tenantID, fingerprint)
	countKey := keyPrefix + ":count"
	firstSeenKey := keyPrefix + ":first_seen"
	lastSeenKey := keyPrefix + ":last_seen"

	ttlSeconds := int(d.ttl.Seconds())

	result, err := dedupScript.Run(ctx, d.client,
		[]string{countKey, firstSeenKey, lastSeenKey},
		eventTs, ttlSeconds,
	).Slice()

	if err != nil {
		d.logger.Error("Lua script exec failed",
			zap.String("fingerprint", fingerprint),
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		dedupCheckTotal.WithLabelValues(tenantID, "error").Inc()
		dedupRedisErrors.WithLabelValues(tenantID, "lua_script").Inc()
		return nil, fmt.Errorf("lua script exec failed: %w", err)
	}

	if len(result) != 4 {
		return nil, fmt.Errorf("unexpected result length: %d", len(result))
	}

	count, _ := result[0].(int64)
	isNewInt, _ := result[1].(int64)
	firstSeen, _ := result[2].(int64)
	lastSeen, _ := result[3].(int64)

	isNew := isNewInt == 1

	dedupResult := &DedupResult{
		IsNew:     isNew,
		Count:     count,
		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
	}

	duration := time.Since(start).Seconds()
	dedupCheckLatency.WithLabelValues(tenantID).Observe(duration)

	if isNew {
		dedupCheckTotal.WithLabelValues(tenantID, "new").Inc()
	} else {
		dedupCheckTotal.WithLabelValues(tenantID, "duplicate").Inc()
	}

	d.logger.Debug("Atomic dedup check completed",
		zap.String("fingerprint", fingerprint),
		zap.String("tenant_id", tenantID),
		zap.Bool("is_new", isNew),
		zap.Int64("count", count),
		zap.Duration("duration", time.Since(start)))

	return dedupResult, nil
}
