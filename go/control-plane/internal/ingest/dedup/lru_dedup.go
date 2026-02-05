////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/dedup/lru_dedup.go
// event_id LRU 去重器 - 详细设计要求实现
// 优化版：移除硬编码，使用 config 常量
////////////////////////////////////////////////////////////////////////////////

package dedup

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

// DedupConfig 去重器配置
type DedupConfig struct {
	// 本地 LRU 缓存大小
	LocalCacheSize int `env:"DEDUP_LOCAL_CACHE_SIZE" envDefault:"100000"`

	// 本地缓存 TTL
	LocalTTL time.Duration `env:"DEDUP_LOCAL_TTL" envDefault:"5m"`

	// 是否启用 Redis 去重（分布式）
	RedisEnabled bool `env:"DEDUP_REDIS_ENABLED" envDefault:"false"`

	// Redis key 前缀
	RedisPrefix string `env:"DEDUP_REDIS_PREFIX" envDefault:"dedup:"`

	// Redis TTL
	RedisTTL time.Duration `env:"DEDUP_REDIS_TTL" envDefault:"10m"`
}

// DefaultDedupConfig 默认配置
func DefaultDedupConfig() DedupConfig {
	return DedupConfig{
		LocalCacheSize: config.DefaultDedupLocalCacheSize,
		LocalTTL:       config.DefaultDedupLocalTTL,
		RedisEnabled:   false,
		RedisPrefix:    config.RedisDedupPrefix,
		RedisTTL:       config.DefaultDedupRedisTTL,
	}
}

// dedupEntry 去重条目
type dedupEntry struct {
	FirstSeen time.Time
}

// Deduplicator event_id 去重器
type Deduplicator struct {
	config DedupConfig
	logger *zap.Logger

	// 本地 LRU 缓存
	localCache *lru.Cache[string, *dedupEntry]
	localMu    sync.RWMutex

	// Redis 客户端（可选）
	redis redis.UniversalClient

	// 统计
	hitLocal   int64
	hitRedis   int64
	missTotal  int64
	dupDropped int64
}

// NewDeduplicator 创建去重器
func NewDeduplicator(cfg DedupConfig, rdb redis.UniversalClient, logger *zap.Logger) (*Deduplicator, error) {
	cacheSize := cfg.LocalCacheSize
	if cacheSize <= 0 {
		cacheSize = config.DefaultDedupLocalCacheSize
	}

	localCache, err := lru.New[string, *dedupEntry](cacheSize)
	if err != nil {
		return nil, err
	}

	// 默认前缀
	if cfg.RedisPrefix == "" {
		cfg.RedisPrefix = config.RedisDedupPrefix
	}

	d := &Deduplicator{
		config:     cfg,
		logger:     logger,
		localCache: localCache,
	}

	if cfg.RedisEnabled && rdb != nil {
		d.redis = rdb
	}

	return d, nil
}

// IsDuplicate 检查 event_id 是否重复
// 返回 true 表示是重复事件，应该丢弃
func (d *Deduplicator) IsDuplicate(ctx context.Context, eventID string) bool {
	if eventID == "" {
		return false
	}

	// 1. 检查本地缓存
	d.localMu.RLock()
	entry, found := d.localCache.Get(eventID)
	d.localMu.RUnlock()

	if found {
		// 检查是否过期
		if time.Since(entry.FirstSeen) < d.config.LocalTTL {
			atomic.AddInt64(&d.hitLocal, 1)
			atomic.AddInt64(&d.dupDropped, 1)
			return true
		}
		// 过期则删除
		d.localMu.Lock()
		d.localCache.Remove(eventID)
		d.localMu.Unlock()
	}

	// 2. 检查 Redis（如果启用）
	if d.redis != nil {
		key := d.config.RedisPrefix + eventID
		exists, err := d.redis.Exists(ctx, key).Result()
		if err == nil && exists > 0 {
			atomic.AddInt64(&d.hitRedis, 1)
			atomic.AddInt64(&d.dupDropped, 1)
			// 同时写入本地缓存
			d.markSeen(eventID)
			return true
		}
	}

	atomic.AddInt64(&d.missTotal, 1)
	return false
}

// MarkSeen 标记 event_id 已处理
func (d *Deduplicator) MarkSeen(ctx context.Context, eventID string) {
	if eventID == "" {
		return
	}

	d.markSeen(eventID)

	// 写入 Redis（如果启用）
	if d.redis != nil {
		key := d.config.RedisPrefix + eventID
		d.redis.Set(ctx, key, "1", d.config.RedisTTL)
	}
}

// markSeen 标记到本地缓存
func (d *Deduplicator) markSeen(eventID string) {
	d.localMu.Lock()
	d.localCache.Add(eventID, &dedupEntry{
		FirstSeen: time.Now(),
	})
	d.localMu.Unlock()
}

// MarkSeenBatch 批量标记 event_id 已处理
func (d *Deduplicator) MarkSeenBatch(ctx context.Context, eventIDs []string) {
	if len(eventIDs) == 0 {
		return
	}

	now := time.Now()

	// 批量写入本地缓存
	d.localMu.Lock()
	for _, eventID := range eventIDs {
		if eventID != "" {
			d.localCache.Add(eventID, &dedupEntry{
				FirstSeen: now,
			})
		}
	}
	d.localMu.Unlock()

	// 批量写入 Redis（如果启用）
	if d.redis != nil {
		pipe := d.redis.Pipeline()
		for _, eventID := range eventIDs {
			if eventID != "" {
				key := d.config.RedisPrefix + eventID
				pipe.Set(ctx, key, "1", d.config.RedisTTL)
			}
		}
		pipe.Exec(ctx)
	}
}

// FilterDuplicates 过滤重复事件，返回非重复事件列表
func (d *Deduplicator) FilterDuplicates(ctx context.Context, eventIDs []string) []string {
	if len(eventIDs) == 0 {
		return eventIDs
	}

	result := make([]string, 0, len(eventIDs))

	for _, eventID := range eventIDs {
		if !d.IsDuplicate(ctx, eventID) {
			result = append(result, eventID)
		}
	}

	return result
}

// GetStats 获取统计信息
func (d *Deduplicator) GetStats() DedupStats {
	return DedupStats{
		HitLocal:       atomic.LoadInt64(&d.hitLocal),
		HitRedis:       atomic.LoadInt64(&d.hitRedis),
		MissTotal:      atomic.LoadInt64(&d.missTotal),
		DupDropped:     atomic.LoadInt64(&d.dupDropped),
		LocalCacheSize: d.localCache.Len(),
	}
}

// DedupStats 去重统计
type DedupStats struct {
	HitLocal       int64
	HitRedis       int64
	MissTotal      int64
	DupDropped     int64
	LocalCacheSize int
}

// DedupRate 计算去重率
func (s DedupStats) DedupRate() float64 {
	total := s.HitLocal + s.HitRedis + s.MissTotal
	if total == 0 {
		return 0
	}
	return float64(s.HitLocal+s.HitRedis) / float64(total)
}

// Reset 重置统计
func (d *Deduplicator) Reset() {
	atomic.StoreInt64(&d.hitLocal, 0)
	atomic.StoreInt64(&d.hitRedis, 0)
	atomic.StoreInt64(&d.missTotal, 0)
	atomic.StoreInt64(&d.dupDropped, 0)
}

// Clear 清空缓存
func (d *Deduplicator) Clear() {
	d.localMu.Lock()
	d.localCache.Purge()
	d.localMu.Unlock()
}

// Close 关闭去重器
func (d *Deduplicator) Close() error {
	d.Clear()
	return nil
}
