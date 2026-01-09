////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/config/tenant_config.go
// 租户配置加载器（完整实现）
// 功能：
// 1. 从 PostgreSQL 加载租户级查询/缓存配置
// 2. 内存缓存 + 定期刷新
// 3. 配置变更通知
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TenantQueryConfig 租户查询配置（从 PostgreSQL graph_query_config 表加载）
type TenantQueryConfig struct {
	TenantID              string
	MaxDepth              int
	DefaultDepth          int
	MaxNodes              int
	MaxNeighborsPerHop    int
	DefaultTimeRangeHours int
	MaxBatchExploreIPs    int
	MaxPathSearchHops     int
	AlertBatchSize        int
	QueryTimeoutSec       int
	MaxConcurrentQueries  int
}

// ToQueryConfig 转换为 QueryConfig
func (t *TenantQueryConfig) ToQueryConfig() QueryConfig {
	return QueryConfig{
		MaxDepth:              t.MaxDepth,
		DefaultDepth:          t.DefaultDepth,
		MaxNodes:              t.MaxNodes,
		MaxNeighborsPerHop:    t.MaxNeighborsPerHop,
		DefaultTimeRangeHours: t.DefaultTimeRangeHours,
		MaxBatchExploreIPs:    t.MaxBatchExploreIPs,
		MaxPathSearchHops:     t.MaxPathSearchHops,
		AlertBatchSize:        t.AlertBatchSize,
		QueryTimeout:          time.Duration(t.QueryTimeoutSec) * time.Second,
		MaxConcurrentQueries:  t.MaxConcurrentQueries,
	}
}

// TenantCacheConfig 租户缓存配置（从 PostgreSQL graph_cache_config 表加载）
type TenantCacheConfig struct {
	TenantID           string
	NeighborTTLSec     int
	EntityTTLSec       int
	GraphTTLSec        int
	MaxNodesPerCache   int
	MaxEdgesPerCache   int
	TimeGranularitySec int
	Enabled            bool
}

// ToCacheConfig 转换为 CacheConfig
func (t *TenantCacheConfig) ToCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:         t.Enabled,
		NeighborTTL:     time.Duration(t.NeighborTTLSec) * time.Second,
		EntityTTL:       time.Duration(t.EntityTTLSec) * time.Second,
		GraphTTL:        time.Duration(t.GraphTTLSec) * time.Second,
		MaxNodesPerItem: t.MaxNodesPerCache,
		MaxEdgesPerItem: t.MaxEdgesPerCache,
		TimeGranularity: time.Duration(t.TimeGranularitySec) * time.Second,
	}
}

// TenantConfigLoader 租户配置加载器
type TenantConfigLoader struct {
	db            *sql.DB
	defaultConfig *Config
	logger        *zap.Logger

	queryConfigCache sync.Map // tenantID -> *TenantQueryConfig
	cacheConfigCache sync.Map // tenantID -> *TenantCacheConfig

	refreshInterval time.Duration
	stopChan        chan struct{}
	wg              sync.WaitGroup
}

// NewTenantConfigLoader 创建租户配置加载器
func NewTenantConfigLoader(db *sql.DB, cfg *Config, logger *zap.Logger) *TenantConfigLoader {
	loader := &TenantConfigLoader{
		db:              db,
		defaultConfig:   cfg,
		logger:          logger,
		refreshInterval: 5 * time.Minute,
		stopChan:        make(chan struct{}),
	}

	// 启动定期刷新
	loader.wg.Add(1)
	go loader.refreshLoop()

	return loader
}

// GetQueryConfig 获取租户查询配置（带缓存）
func (t *TenantConfigLoader) GetQueryConfig(ctx context.Context, tenantID string) (*TenantQueryConfig, error) {
	// 先从缓存获取
	if cached, ok := t.queryConfigCache.Load(tenantID); ok {
		return cached.(*TenantQueryConfig), nil
	}

	// 从数据库加载
	config, err := t.loadQueryConfigFromDB(ctx, tenantID)
	if err != nil {
		// 返回默认配置
		t.logger.Warn("Failed to load tenant query config, using defaults",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		return t.getDefaultQueryConfig(tenantID), nil
	}

	// 缓存
	t.queryConfigCache.Store(tenantID, config)

	return config, nil
}

// GetCacheConfig 获取租户缓存配置（带缓存）
func (t *TenantConfigLoader) GetCacheConfig(ctx context.Context, tenantID string) (*TenantCacheConfig, error) {
	// 先从缓存获取
	if cached, ok := t.cacheConfigCache.Load(tenantID); ok {
		return cached.(*TenantCacheConfig), nil
	}

	// 从数据库加载
	config, err := t.loadCacheConfigFromDB(ctx, tenantID)
	if err != nil {
		// 返回默认配置
		t.logger.Warn("Failed to load tenant cache config, using defaults",
			zap.String("tenant_id", tenantID),
			zap.Error(err))
		return t.getDefaultCacheConfig(tenantID), nil
	}

	// 缓存
	t.cacheConfigCache.Store(tenantID, config)

	return config, nil
}

// loadQueryConfigFromDB 从数据库加载查询配置
func (t *TenantConfigLoader) loadQueryConfigFromDB(ctx context.Context, tenantID string) (*TenantQueryConfig, error) {
	query := `
		SELECT
			tenant_id,
			max_depth,
			default_depth,
			max_nodes,
			max_neighbors_per_hop,
			default_time_range_hours,
			max_batch_explore_ips,
			max_path_search_hops,
			alert_batch_size,
			query_timeout_sec,
			max_concurrent_queries
		FROM graph_query_config
		WHERE tenant_id = $1
	`

	var config TenantQueryConfig
	err := t.db.QueryRowContext(ctx, query, tenantID).Scan(
		&config.TenantID,
		&config.MaxDepth,
		&config.DefaultDepth,
		&config.MaxNodes,
		&config.MaxNeighborsPerHop,
		&config.DefaultTimeRangeHours,
		&config.MaxBatchExploreIPs,
		&config.MaxPathSearchHops,
		&config.AlertBatchSize,
		&config.QueryTimeoutSec,
		&config.MaxConcurrentQueries,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("config not found for tenant: %s", tenantID)
		}
		return nil, fmt.Errorf("failed to query config: %w", err)
	}

	return &config, nil
}

// loadCacheConfigFromDB 从数据库加载缓存配置
func (t *TenantConfigLoader) loadCacheConfigFromDB(ctx context.Context, tenantID string) (*TenantCacheConfig, error) {
	query := `
		SELECT
			tenant_id,
			neighbor_ttl_sec,
			entity_ttl_sec,
			graph_ttl_sec,
			max_nodes_per_cache,
			max_edges_per_cache,
			time_granularity_sec,
			enabled
		FROM graph_cache_config
		WHERE tenant_id = $1
	`

	var config TenantCacheConfig
	err := t.db.QueryRowContext(ctx, query, tenantID).Scan(
		&config.TenantID,
		&config.NeighborTTLSec,
		&config.EntityTTLSec,
		&config.GraphTTLSec,
		&config.MaxNodesPerCache,
		&config.MaxEdgesPerCache,
		&config.TimeGranularitySec,
		&config.Enabled,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("cache config not found for tenant: %s", tenantID)
		}
		return nil, fmt.Errorf("failed to query cache config: %w", err)
	}

	return &config, nil
}

// getDefaultQueryConfig 获取默认查询配置
func (t *TenantConfigLoader) getDefaultQueryConfig(tenantID string) *TenantQueryConfig {
	return &TenantQueryConfig{
		TenantID:              tenantID,
		MaxDepth:              t.defaultConfig.Query.MaxDepth,
		DefaultDepth:          t.defaultConfig.Query.DefaultDepth,
		MaxNodes:              t.defaultConfig.Query.MaxNodes,
		MaxNeighborsPerHop:    t.defaultConfig.Query.MaxNeighborsPerHop,
		DefaultTimeRangeHours: t.defaultConfig.Query.DefaultTimeRangeHours,
		MaxBatchExploreIPs:    t.defaultConfig.Query.MaxBatchExploreIPs,
		MaxPathSearchHops:     t.defaultConfig.Query.MaxPathSearchHops,
		AlertBatchSize:        t.defaultConfig.Query.AlertBatchSize,
		QueryTimeoutSec:       int(t.defaultConfig.Query.QueryTimeout.Seconds()),
		MaxConcurrentQueries:  t.defaultConfig.Query.MaxConcurrentQueries,
	}
}

// getDefaultCacheConfig 获取默认缓存配置
func (t *TenantConfigLoader) getDefaultCacheConfig(tenantID string) *TenantCacheConfig {
	return &TenantCacheConfig{
		TenantID:           tenantID,
		NeighborTTLSec:     int(t.defaultConfig.Cache.NeighborTTL.Seconds()),
		EntityTTLSec:       int(t.defaultConfig.Cache.EntityTTL.Seconds()),
		GraphTTLSec:        int(t.defaultConfig.Cache.GraphTTL.Seconds()),
		MaxNodesPerCache:   t.defaultConfig.Cache.MaxNodesPerItem,
		MaxEdgesPerCache:   t.defaultConfig.Cache.MaxEdgesPerItem,
		TimeGranularitySec: int(t.defaultConfig.Cache.TimeGranularity.Seconds()),
		Enabled:            t.defaultConfig.Cache.Enabled,
	}
}

// refreshLoop 定期刷新配置
func (t *TenantConfigLoader) refreshLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopChan:
			t.logger.Info("Tenant config loader stopped")
			return
		case <-ticker.C:
			t.refreshAll()
		}
	}
}

// refreshAll 刷新所有缓存的配置
func (t *TenantConfigLoader) refreshAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 获取所有租户
	tenantIDs, err := t.getAllTenantIDs(ctx)
	if err != nil {
		t.logger.Error("Failed to get tenant IDs for refresh", zap.Error(err))
		return
	}

	t.logger.Debug("Refreshing tenant configs", zap.Int("tenant_count", len(tenantIDs)))

	for _, tenantID := range tenantIDs {
		// 刷新查询配置
		if config, err := t.loadQueryConfigFromDB(ctx, tenantID); err == nil {
			t.queryConfigCache.Store(tenantID, config)
		}

		// 刷新缓存配置
		if config, err := t.loadCacheConfigFromDB(ctx, tenantID); err == nil {
			t.cacheConfigCache.Store(tenantID, config)
		}
	}
}

// getAllTenantIDs 获取所有租户 ID
func (t *TenantConfigLoader) getAllTenantIDs(ctx context.Context) ([]string, error) {
	query := `SELECT DISTINCT tenant_id FROM tenants WHERE status = 'active'`

	rows, err := t.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenantIDs []string
	for rows.Next() {
		var tenantID string
		if err := rows.Scan(&tenantID); err != nil {
			continue
		}
		tenantIDs = append(tenantIDs, tenantID)
	}

	return tenantIDs, rows.Err()
}

// InvalidateCache 使缓存失效
func (t *TenantConfigLoader) InvalidateCache(tenantID string) {
	t.queryConfigCache.Delete(tenantID)
	t.cacheConfigCache.Delete(tenantID)
	t.logger.Debug("Invalidated tenant config cache", zap.String("tenant_id", tenantID))
}

// InvalidateAllCache 使所有缓存失效
func (t *TenantConfigLoader) InvalidateAllCache() {
	t.queryConfigCache = sync.Map{}
	t.cacheConfigCache = sync.Map{}
	t.logger.Info("Invalidated all tenant config cache")
}

// Close 关闭加载器
func (t *TenantConfigLoader) Close() error {
	close(t.stopChan)
	t.wg.Wait()
	t.logger.Info("Tenant config loader closed")
	return nil
}

// GetDB 获取数据库连接（供其他模块使用）
func (t *TenantConfigLoader) GetDB() *sql.DB {
	return t.db
}
