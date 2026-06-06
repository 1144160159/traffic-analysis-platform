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

type DedupConfig struct {
	LocalCacheSize int `env:"DEDUP_LOCAL_CACHE_SIZE" envDefault:"100000"`

	LocalTTL time.Duration `env:"DEDUP_LOCAL_TTL" envDefault:"5m"`

	RedisEnabled bool `env:"DEDUP_REDIS_ENABLED" envDefault:"false"`

	RedisPrefix string `env:"DEDUP_REDIS_PREFIX" envDefault:"dedup:"`

	RedisTTL time.Duration `env:"DEDUP_REDIS_TTL" envDefault:"10m"`
}

func DefaultDedupConfig() DedupConfig {
	return DedupConfig{
		LocalCacheSize: config.DefaultDedupLocalCacheSize,
		LocalTTL:       config.DefaultDedupLocalTTL,
		RedisEnabled:   false,
		RedisPrefix:    config.RedisDedupPrefix,
		RedisTTL:       config.DefaultDedupRedisTTL,
	}
}

type dedupEntry struct {
	FirstSeen time.Time
}

type Deduplicator struct {
	config DedupConfig
	logger *zap.Logger

	localCache *lru.Cache[string, *dedupEntry]
	localMu    sync.RWMutex

	redis redis.UniversalClient

	hitLocal   int64
	hitRedis   int64
	missTotal  int64
	dupDropped int64
}

func NewDeduplicator(cfg DedupConfig, rdb redis.UniversalClient, logger *zap.Logger) (*Deduplicator, error) {
	cacheSize := cfg.LocalCacheSize
	if cacheSize <= 0 {
		cacheSize = config.DefaultDedupLocalCacheSize
	}

	localCache, err := lru.New[string, *dedupEntry](cacheSize)
	if err != nil {
		return nil, err
	}

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

func (d *Deduplicator) IsDuplicate(ctx context.Context, eventID string) bool {
	if eventID == "" {
		return false
	}

	d.localMu.RLock()
	entry, found := d.localCache.Get(eventID)
	d.localMu.RUnlock()

	if found {

		if time.Since(entry.FirstSeen) < d.config.LocalTTL {
			atomic.AddInt64(&d.hitLocal, 1)
			atomic.AddInt64(&d.dupDropped, 1)
			return true
		}

		d.localMu.Lock()
		d.localCache.Remove(eventID)
		d.localMu.Unlock()
	}

	if d.redis != nil {
		key := d.config.RedisPrefix + eventID
		exists, err := d.redis.Exists(ctx, key).Result()
		if err == nil && exists > 0 {
			atomic.AddInt64(&d.hitRedis, 1)
			atomic.AddInt64(&d.dupDropped, 1)

			d.markSeen(eventID)
			return true
		}
	}

	atomic.AddInt64(&d.missTotal, 1)
	return false
}

func (d *Deduplicator) MarkSeen(ctx context.Context, eventID string) {
	if eventID == "" {
		return
	}

	d.markSeen(eventID)

	if d.redis != nil {
		key := d.config.RedisPrefix + eventID
		d.redis.Set(ctx, key, "1", d.config.RedisTTL)
	}
}

func (d *Deduplicator) markSeen(eventID string) {
	d.localMu.Lock()
	d.localCache.Add(eventID, &dedupEntry{
		FirstSeen: time.Now(),
	})
	d.localMu.Unlock()
}

func (d *Deduplicator) MarkSeenBatch(ctx context.Context, eventIDs []string) {
	if len(eventIDs) == 0 {
		return
	}

	now := time.Now()

	d.localMu.Lock()
	for _, eventID := range eventIDs {
		if eventID != "" {
			d.localCache.Add(eventID, &dedupEntry{
				FirstSeen: now,
			})
		}
	}
	d.localMu.Unlock()

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

func (d *Deduplicator) GetStats() DedupStats {
	return DedupStats{
		HitLocal:       atomic.LoadInt64(&d.hitLocal),
		HitRedis:       atomic.LoadInt64(&d.hitRedis),
		MissTotal:      atomic.LoadInt64(&d.missTotal),
		DupDropped:     atomic.LoadInt64(&d.dupDropped),
		LocalCacheSize: d.localCache.Len(),
	}
}

type DedupStats struct {
	HitLocal       int64
	HitRedis       int64
	MissTotal      int64
	DupDropped     int64
	LocalCacheSize int
}

func (s DedupStats) DedupRate() float64 {
	total := s.HitLocal + s.HitRedis + s.MissTotal
	if total == 0 {
		return 0
	}
	return float64(s.HitLocal+s.HitRedis) / float64(total)
}

func (d *Deduplicator) Reset() {
	atomic.StoreInt64(&d.hitLocal, 0)
	atomic.StoreInt64(&d.hitRedis, 0)
	atomic.StoreInt64(&d.missTotal, 0)
	atomic.StoreInt64(&d.dupDropped, 0)
}

func (d *Deduplicator) Clear() {
	d.localMu.Lock()
	d.localCache.Purge()
	d.localMu.Unlock()
}

func (d *Deduplicator) Close() error {
	d.Clear()
	return nil
}
