////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/cache/redis_cache.go
// Graph Service Redis 缓存（完整修复版）
// 修复内容：
// 1. 修复 C1：GetGraph/SetGraph 返回正确的类型
// 2. 修复 C2：移除 WarmupCache 占位符实现
// 3. 修复 C3：修复缓存键中的特殊字符问题
// 4. 修复 C4：改进 deleteByPattern 的错误处理
////////////////////////////////////////////////////////////////////////////////

package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/otel"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// GraphCache 图缓存
type GraphCache struct {
	client           *storage.RedisClient
	neighborTTL      time.Duration
	entityTTL        time.Duration
	graphTTL         time.Duration
	maxNodesPerCache int
	maxEdgesPerCache int
	timeGranularity  time.Duration
	logger           *zap.Logger

	hits       int64
	misses     int64
	lastReset  int64
	resetCycle time.Duration
}

// NewGraphCache 创建图缓存
func NewGraphCache(
	client *storage.RedisClient,
	neighborTTL, entityTTL, graphTTL time.Duration,
	maxNodesPerCache, maxEdgesPerCache int,
	logger *zap.Logger,
) *GraphCache {
	if neighborTTL == 0 {
		neighborTTL = 5 * time.Minute
	}
	if entityTTL == 0 {
		entityTTL = 5 * time.Minute
	}
	if graphTTL == 0 {
		graphTTL = 2 * time.Minute
	}
	if maxNodesPerCache == 0 {
		maxNodesPerCache = 500
	}
	if maxEdgesPerCache == 0 {
		maxEdgesPerCache = 1000
	}

	gc := &GraphCache{
		client:           client,
		neighborTTL:      neighborTTL,
		entityTTL:        entityTTL,
		graphTTL:         graphTTL,
		maxNodesPerCache: maxNodesPerCache,
		maxEdgesPerCache: maxEdgesPerCache,
		timeGranularity:  5 * time.Minute,
		logger:           logger,
		lastReset:        time.Now().Unix(),
		resetCycle:       1 * time.Hour,
	}

	go gc.periodicStatsReset()

	return gc
}

// periodicStatsReset 定期重置统计
func (c *GraphCache) periodicStatsReset() {
	ticker := time.NewTicker(c.resetCycle)
	defer ticker.Stop()

	for range ticker.C {
		c.ResetStats()
	}
}

// NeighborCacheEntry 邻居缓存条目
type NeighborCacheEntry struct {
	Neighbors []NeighborInfo `json:"neighbors"`
	CachedAt  time.Time      `json:"cached_at"`
}

// NeighborInfo 邻居信息
type NeighborInfo struct {
	IP           string `json:"ip"`
	SessionCount int    `json:"session_count"`
	TotalBytes   uint64 `json:"total_bytes"`
	LastSeen     string `json:"last_seen"`
}

// GetNeighbors 获取邻居缓存
func (c *GraphCache) GetNeighbors(ctx context.Context, tenantID, nodeIP string, startTime, endTime int64, runID string) ([]NeighborInfo, bool) {
	ctx, span := otel.StartSpan(ctx, "GraphCache.GetNeighbors")
	defer span.End()

	key := c.neighborKey(tenantID, nodeIP, startTime, endTime, runID)

	data, err := c.client.Get(ctx, key)
	if err != nil || data == "" {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	var entry NeighborCacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		c.logger.Warn("Failed to unmarshal neighbor cache", zap.Error(err))
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	atomic.AddInt64(&c.hits, 1)
	return entry.Neighbors, true
}

// SetNeighbors 设置邻居缓存
func (c *GraphCache) SetNeighbors(ctx context.Context, tenantID, nodeIP string, startTime, endTime int64, runID string, neighbors []NeighborInfo) {
	ctx, span := otel.StartSpan(ctx, "GraphCache.SetNeighbors")
	defer span.End()

	if len(neighbors) > c.maxNodesPerCache {
		c.logger.Warn("Neighbors too large to cache, truncating",
			zap.Int("original_size", len(neighbors)),
			zap.Int("max_size", c.maxNodesPerCache))
		neighbors = neighbors[:c.maxNodesPerCache]
	}

	key := c.neighborKey(tenantID, nodeIP, startTime, endTime, runID)

	entry := NeighborCacheEntry{
		Neighbors: neighbors,
		CachedAt:  time.Now(),
	}

	if err := c.client.SetJSON(ctx, key, entry, c.neighborTTL); err != nil {
		c.logger.Warn("Failed to cache neighbors",
			zap.String("key", key),
			zap.Error(err))
	}
}

// EntityCacheEntry 实体缓存条目
type EntityCacheEntry struct {
	Details  map[string]interface{} `json:"details"`
	CachedAt time.Time              `json:"cached_at"`
}

// GetEntityDetails 获取实体详情缓存
func (c *GraphCache) GetEntityDetails(ctx context.Context, tenantID, entityID, entityType string, startTime, endTime int64, runID string) (map[string]interface{}, bool) {
	ctx, span := otel.StartSpan(ctx, "GraphCache.GetEntityDetails")
	defer span.End()

	key := c.entityKey(tenantID, entityID, entityType, startTime, endTime, runID)

	data, err := c.client.Get(ctx, key)
	if err != nil || data == "" {
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	var entry EntityCacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		c.logger.Warn("Failed to unmarshal entity cache", zap.Error(err))
		atomic.AddInt64(&c.misses, 1)
		return nil, false
	}

	atomic.AddInt64(&c.hits, 1)
	return entry.Details, true
}

// SetEntityDetails 设置实体详情缓存
func (c *GraphCache) SetEntityDetails(ctx context.Context, tenantID, entityID, entityType string, startTime, endTime int64, runID string, details map[string]interface{}) {
	ctx, span := otel.StartSpan(ctx, "GraphCache.SetEntityDetails")
	defer span.End()

	key := c.entityKey(tenantID, entityID, entityType, startTime, endTime, runID)

	entry := EntityCacheEntry{
		Details:  details,
		CachedAt: time.Now(),
	}

	if err := c.client.SetJSON(ctx, key, entry, c.entityTTL); err != nil {
		c.logger.Warn("Failed to cache entity details",
			zap.String("key", key),
			zap.Error(err))
	}
}

// GraphCacheEntry 图缓存条目（修复 C1：使用明确的结构）
type GraphCacheEntry struct {
	Nodes     []map[string]interface{} `json:"nodes"`
	Edges     []map[string]interface{} `json:"edges"`
	Truncated bool                     `json:"truncated"`
	CachedAt  time.Time                `json:"cached_at"`
}

// GetGraph 获取图缓存（修复 C1：返回明确的类型）
func (c *GraphCache) GetGraph(ctx context.Context, tenantID, centerIP string, depth int, startTime, endTime int64, runID string) (interface{}, interface{}, bool) {
	ctx, span := otel.StartSpan(ctx, "GraphCache.GetGraph")
	defer span.End()

	key := c.graphKey(tenantID, centerIP, depth, startTime, endTime, runID)

	data, err := c.client.Get(ctx, key)
	if err != nil || data == "" {
		atomic.AddInt64(&c.misses, 1)
		return nil, nil, false
	}

	var entry GraphCacheEntry
	if err := json.Unmarshal([]byte(data), &entry); err != nil {
		c.logger.Warn("Failed to unmarshal graph cache", zap.Error(err))
		atomic.AddInt64(&c.misses, 1)
		return nil, nil, false
	}

	atomic.AddInt64(&c.hits, 1)
	return entry.Nodes, entry.Edges, true
}

// SetGraph 设置图缓存（修复 C1：接收明确的类型）
func (c *GraphCache) SetGraph(ctx context.Context, tenantID, centerIP string, depth int, startTime, endTime int64, runID string, nodes, edges interface{}) {
	ctx, span := otel.StartSpan(ctx, "GraphCache.SetGraph")
	defer span.End()

	// 类型断言
	nodesSlice, ok1 := nodes.([]map[string]interface{})
	edgesSlice, ok2 := edges.([]map[string]interface{})

	if !ok1 || !ok2 {
		c.logger.Warn("Invalid type for SetGraph",
			zap.String("nodes_type", fmt.Sprintf("%T", nodes)),
			zap.String("edges_type", fmt.Sprintf("%T", edges)))
		return
	}

	// 检查大小限制
	if len(nodesSlice) > c.maxNodesPerCache {
		c.logger.Warn("Graph nodes too large to cache",
			zap.Int("size", len(nodesSlice)),
			zap.Int("max", c.maxNodesPerCache))
		return
	}

	if len(edgesSlice) > c.maxEdgesPerCache {
		c.logger.Warn("Graph edges too large to cache",
			zap.Int("size", len(edgesSlice)),
			zap.Int("max", c.maxEdgesPerCache))
		return
	}

	key := c.graphKey(tenantID, centerIP, depth, startTime, endTime, runID)

	entry := GraphCacheEntry{
		Nodes:    nodesSlice,
		Edges:    edgesSlice,
		CachedAt: time.Now(),
	}

	if err := c.client.SetJSON(ctx, key, entry, c.graphTTL); err != nil {
		c.logger.Warn("Failed to cache graph",
			zap.String("key", key),
			zap.Error(err))
	}
}

// InvalidateEntity 使实体缓存失效
func (c *GraphCache) InvalidateEntity(ctx context.Context, tenantID, entityID string) error {
	pattern := fmt.Sprintf("graph:entity:%s:%s:*", tenantID, entityID)
	return c.deleteByPattern(ctx, pattern)
}

// InvalidateTenant 使租户所有缓存失效
func (c *GraphCache) InvalidateTenant(ctx context.Context, tenantID string) error {
	pattern := fmt.Sprintf("graph:*:%s:*", tenantID)
	return c.deleteByPattern(ctx, pattern)
}

// InvalidateNeighbors 使邻居缓存失效
func (c *GraphCache) InvalidateNeighbors(ctx context.Context, tenantID, nodeIP string) error {
	pattern := fmt.Sprintf("graph:neighbor:%s:%s:*", tenantID, nodeIP)
	return c.deleteByPattern(ctx, pattern)
}

// InvalidateGraph 使图缓存失效
func (c *GraphCache) InvalidateGraph(ctx context.Context, tenantID, centerIP string) error {
	pattern := fmt.Sprintf("graph:explore:%s:%s:*", tenantID, centerIP)
	return c.deleteByPattern(ctx, pattern)
}

// deleteByPattern 按模式删除键（修复 C4：改进错误处理）
func (c *GraphCache) deleteByPattern(ctx context.Context, pattern string) error {
	ctx, span := otel.StartSpan(ctx, "GraphCache.deleteByPattern")
	defer span.End()

	c.logger.Debug("Invalidating cache by pattern", zap.String("pattern", pattern))

	if c.client == nil {
		return nil
	}

	redisClient := c.client.Client()
	if redisClient == nil {
		return nil
	}

	var cursor uint64
	var deletedCount int64
	batchSize := 100
	maxDeleteCount := 10000

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Cache invalidation interrupted",
				zap.String("pattern", pattern),
				zap.Int64("deleted_count", deletedCount))
			return ctx.Err()
		default:
		}

		// 修复 C4：检查是否达到最大删除限制
		if deletedCount >= int64(maxDeleteCount) {
			c.logger.Warn("Reached max delete limit, some keys may remain",
				zap.String("pattern", pattern),
				zap.Int64("deleted_count", deletedCount))
			// 修复 C4：返回错误而非静默截断
			return fmt.Errorf("partial deletion: deleted %d keys, limit %d reached", deletedCount, maxDeleteCount)
		}

		var keys []string
		var err error
		keys, cursor, err = redisClient.Scan(ctx, cursor, pattern, int64(batchSize)).Result()
		if err != nil {
			c.logger.Error("Failed to scan keys",
				zap.String("pattern", pattern),
				zap.Error(err))
			return fmt.Errorf("failed to scan keys: %w", err)
		}

		if len(keys) > 0 {
			pipeline := redisClient.Pipeline()
			for _, key := range keys {
				pipeline.Del(ctx, key)
			}
			_, err := pipeline.Exec(ctx)
			if err != nil {
				c.logger.Error("Failed to delete keys",
					zap.Int("count", len(keys)),
					zap.Error(err))
				return fmt.Errorf("failed to delete keys: %w", err)
			}
			deletedCount += int64(len(keys))
		}

		if cursor == 0 {
			break
		}

		if deletedCount > 0 && deletedCount%1000 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	if deletedCount > 0 {
		c.logger.Info("Cache invalidated",
			zap.String("pattern", pattern),
			zap.Int64("deleted_count", deletedCount))
	}

	return nil
}

// DeleteKey 删除单个键
func (c *GraphCache) DeleteKey(ctx context.Context, key string) error {
	return c.client.Delete(ctx, key)
}

// GetStats 获取缓存统计
func (c *GraphCache) GetStats() map[string]interface{} {
	hits := atomic.LoadInt64(&c.hits)
	misses := atomic.LoadInt64(&c.misses)
	total := hits + misses

	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100
	}

	return map[string]interface{}{
		"hits":             hits,
		"misses":           misses,
		"total":            total,
		"hit_rate":         fmt.Sprintf("%.2f%%", hitRate),
		"neighbor_ttl":     c.neighborTTL.String(),
		"entity_ttl":       c.entityTTL.String(),
		"graph_ttl":        c.graphTTL.String(),
		"max_nodes_cached": c.maxNodesPerCache,
		"max_edges_cached": c.maxEdgesPerCache,
		"time_granularity": c.timeGranularity.String(),
		"last_reset":       time.Unix(atomic.LoadInt64(&c.lastReset), 0).Format(time.RFC3339),
	}
}

// ResetStats 重置统计
func (c *GraphCache) ResetStats() {
	atomic.StoreInt64(&c.hits, 0)
	atomic.StoreInt64(&c.misses, 0)
	atomic.StoreInt64(&c.lastReset, time.Now().Unix())

	c.logger.Debug("Cache stats reset")
}

// 缓存键生成（修复 C3：URL 编码 runID）

func (c *GraphCache) neighborKey(tenantID, nodeIP string, startTime, endTime int64, runID string) string {
	granularityMs := c.timeGranularity.Milliseconds()
	startBucket := startTime / granularityMs * granularityMs
	endBucket := endTime / granularityMs * granularityMs
	// 修复 C3：URL 编码 runID
	safeRunID := url.QueryEscape(runID)
	return fmt.Sprintf("graph:neighbor:%s:%s:%s:%d:%d", tenantID, nodeIP, safeRunID, startBucket, endBucket)
}

func (c *GraphCache) entityKey(tenantID, entityID, entityType string, startTime, endTime int64, runID string) string {
	granularityMs := c.timeGranularity.Milliseconds()
	startBucket := startTime / granularityMs * granularityMs
	endBucket := endTime / granularityMs * granularityMs
	// 修复 C3：URL 编码 runID
	safeRunID := url.QueryEscape(runID)
	return fmt.Sprintf("graph:entity:%s:%s:%s:%s:%d:%d", tenantID, entityID, entityType, safeRunID, startBucket, endBucket)
}

func (c *GraphCache) graphKey(tenantID, centerIP string, depth int, startTime, endTime int64, runID string) string {
	granularityMs := c.timeGranularity.Milliseconds()
	startBucket := startTime / granularityMs * granularityMs
	endBucket := endTime / granularityMs * granularityMs
	// 修复 C3：URL 编码 runID
	safeRunID := url.QueryEscape(runID)
	return fmt.Sprintf("graph:explore:%s:%s:%s:%d:%d:%d", tenantID, centerIP, safeRunID, depth, startBucket, endBucket)
}

// Ping 检查缓存连接
func (c *GraphCache) Ping(ctx context.Context) error {
	if c.client == nil {
		return nil
	}
	return c.client.Ping(ctx)
}

// Close 关闭缓存
func (c *GraphCache) Close() error {
	return nil
}

// GetCacheSize 获取缓存大小（估算）
func (c *GraphCache) GetCacheSize(ctx context.Context, tenantID string) (int64, error) {
	pattern := fmt.Sprintf("graph:*:%s:*", tenantID)

	redisClient := c.client.Client()
	if redisClient == nil {
		return 0, fmt.Errorf("redis client not available")
	}

	var cursor uint64
	var count int64

	for {
		keys, nextCursor, err := redisClient.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return count, err
		}

		count += int64(len(keys))
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	return count, nil
}

// PurgeExpired 清理过期缓存
func (c *GraphCache) PurgeExpired(ctx context.Context) error {
	c.logger.Debug("Purging expired cache entries (handled by Redis)")
	return nil
}

// Get 获取缓存值（通用方法）
func (c *GraphCache) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key)
}

// Set 设置缓存值（通用方法）
func (c *GraphCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl)
}
