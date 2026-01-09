////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/config/probe_config.go
// 修复版：添加优雅关闭支持，优化缓存刷新策略
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	pb "github.com/1144160159/traffic-analysis-platform/go/control-plane/pkg/proto/traffic/v1"
)

// ProbeConfigManager 探针配置管理器
type ProbeConfigManager struct {
	redis  redis.UniversalClient
	logger *zap.Logger
	config ProbeConfig

	// 缓存
	cache   sync.Map // map[string]*cachedProbeConfig
	cacheMu sync.RWMutex

	// 默认配置
	defaultConfig *pb.ProbeConfig

	// 修复：添加优雅关闭支持
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// cachedProbeConfig 带过期时间的缓存配置
type cachedProbeConfig struct {
	Config    *pb.ProbeConfig
	Version   string
	CachedAt  time.Time
	ExpiresAt time.Time
}

// NewProbeConfigManager 创建探针配置管理器
func NewProbeConfigManager(rdb redis.UniversalClient, cfg ProbeConfig, logger *zap.Logger) *ProbeConfigManager {
	// 修复：添加 context 用于优雅关闭
	ctx, cancel := context.WithCancel(context.Background())

	m := &ProbeConfigManager{
		redis:  rdb,
		logger: logger,
		config: cfg,
		defaultConfig: &pb.ProbeConfig{
			ConfigVersion:    "v1.0.0",
			SampleRate:       cfg.DefaultSampleRate,
			BpfFilter:        cfg.DefaultBPFFilter,
			IdleTimeoutSec:   uint32(cfg.DefaultIdleTimeout.Seconds()),
			ActiveTimeoutSec: uint32(cfg.DefaultActiveTimeout.Seconds()),
			BatchSize:        uint32(cfg.DefaultBatchSize),
		},
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	// 修复：启动配置刷新循环（带 context）
	if cfg.EnableDynamicConfig {
		go m.startRefreshLoop()
	}

	return m
}

// GetConfig 获取探针配置
func (m *ProbeConfigManager) GetConfig(ctx context.Context, tenantID, probeID string) (*pb.ProbeConfig, error) {
	cacheKey := fmt.Sprintf("%s:%s", tenantID, probeID)

	// 1. 检查本地缓存（修复：检查过期时间）
	if cached, ok := m.cache.Load(cacheKey); ok {
		cc := cached.(*cachedProbeConfig)
		if time.Now().Before(cc.ExpiresAt) {
			return cc.Config, nil
		}
		// 缓存过期，删除
		m.cache.Delete(cacheKey)
	}

	// 2. 从 Redis 获取
	if m.redis != nil && m.config.EnableDynamicConfig {
		redisKey := fmt.Sprintf("probe_config:%s:%s", tenantID, probeID)
		data, err := m.redis.Get(ctx, redisKey).Bytes()
		if err == nil && len(data) > 0 {
			var cfg pb.ProbeConfig
			if err := json.Unmarshal(data, &cfg); err == nil {
				// 修复：缓存时设置过期时间
				m.cacheConfig(cacheKey, &cfg, cfg.ConfigVersion)
				return &cfg, nil
			}
		}

		// 尝试获取租户级配置
		tenantKey := fmt.Sprintf("probe_config:%s:*", tenantID)
		data, err = m.redis.Get(ctx, tenantKey).Bytes()
		if err == nil && len(data) > 0 {
			var cfg pb.ProbeConfig
			if err := json.Unmarshal(data, &cfg); err == nil {
				m.cacheConfig(cacheKey, &cfg, cfg.ConfigVersion)
				return &cfg, nil
			}
		}
	}

	// 3. 返回默认配置
	return m.defaultConfig, nil
}

// cacheConfig 修复：缓存配置时设置过期时间
func (m *ProbeConfigManager) cacheConfig(key string, config *pb.ProbeConfig, version string) {
	m.cache.Store(key, &cachedProbeConfig{
		Config:    config,
		Version:   version,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(m.config.ConfigRefreshInterval),
	})
}

// SetConfig 设置探针配置
func (m *ProbeConfigManager) SetConfig(ctx context.Context, tenantID, probeID string, cfg *pb.ProbeConfig) error {
	if m.redis == nil {
		return fmt.Errorf("redis not available")
	}

	cacheKey := fmt.Sprintf("%s:%s", tenantID, probeID)
	redisKey := fmt.Sprintf("probe_config:%s:%s", tenantID, probeID)

	// 序列化配置
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 写入 Redis
	if err := m.redis.Set(ctx, redisKey, data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// 更新本地缓存
	m.cacheConfig(cacheKey, cfg, cfg.ConfigVersion)

	// 添加到历史
	m.AddToHistory(ctx, tenantID, probeID, cfg)

	m.logger.Info("Probe config updated",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("version", cfg.ConfigVersion))

	return nil
}

// SetTenantConfig 设置租户级配置
func (m *ProbeConfigManager) SetTenantConfig(ctx context.Context, tenantID string, cfg *pb.ProbeConfig) error {
	if m.redis == nil {
		return fmt.Errorf("redis not available")
	}

	redisKey := fmt.Sprintf("probe_config:%s:*", tenantID)

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := m.redis.Set(ctx, redisKey, data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// 清除该租户的所有本地缓存
	m.cache.Range(func(key, value interface{}) bool {
		if k, ok := key.(string); ok && len(k) > len(tenantID) && k[:len(tenantID)] == tenantID {
			m.cache.Delete(key)
		}
		return true
	})

	m.logger.Info("Tenant probe config updated",
		zap.String("tenant_id", tenantID),
		zap.String("version", cfg.ConfigVersion))

	return nil
}

// DeleteConfig 删除探针配置
func (m *ProbeConfigManager) DeleteConfig(ctx context.Context, tenantID, probeID string) error {
	if m.redis == nil {
		return nil
	}

	cacheKey := fmt.Sprintf("%s:%s", tenantID, probeID)
	redisKey := fmt.Sprintf("probe_config:%s:%s", tenantID, probeID)

	// 删除 Redis 记录
	m.redis.Del(ctx, redisKey)

	// 删除本地缓存
	m.cache.Delete(cacheKey)

	return nil
}

// RefreshCache 刷新缓存
func (m *ProbeConfigManager) RefreshCache(ctx context.Context, tenantID, probeID string) error {
	cacheKey := fmt.Sprintf("%s:%s", tenantID, probeID)
	m.cache.Delete(cacheKey)
	_, err := m.GetConfig(ctx, tenantID, probeID)
	return err
}

// ClearCache 清空缓存
func (m *ProbeConfigManager) ClearCache() {
	m.cache.Range(func(key, value interface{}) bool {
		m.cache.Delete(key)
		return true
	})
}

// GetDefaultConfig 获取默认配置
func (m *ProbeConfigManager) GetDefaultConfig() *pb.ProbeConfig {
	return m.defaultConfig
}

// SetDefaultConfig 设置默认配置
func (m *ProbeConfigManager) SetDefaultConfig(cfg *pb.ProbeConfig) {
	m.defaultConfig = cfg
}

// startRefreshLoop 修复：启动配置刷新循环（支持优雅退出 + 优化刷新策略）
func (m *ProbeConfigManager) startRefreshLoop() {
	if !m.config.EnableDynamicConfig {
		return
	}

	ticker := time.NewTicker(m.config.ConfigRefreshInterval)
	defer ticker.Stop()
	defer close(m.done)

	m.logger.Info("Probe config refresh loop started",
		zap.Duration("interval", m.config.ConfigRefreshInterval))

	for {
		select {
		case <-m.ctx.Done():
			// 修复：支持优雅退出
			m.logger.Info("Probe config refresh loop stopped")
			return

		case <-ticker.C:
			// 修复：优化刷新策略 - 只删除过期的缓存
			m.refreshExpiredCache()
		}
	}
}

// refreshExpiredCache 修复：只刷新过期的缓存（而非暴力清空）
func (m *ProbeConfigManager) refreshExpiredCache() {
	now := time.Now()
	expiredCount := 0

	m.cache.Range(func(key, value interface{}) bool {
		cc := value.(*cachedProbeConfig)
		if now.After(cc.ExpiresAt) {
			m.cache.Delete(key)
			expiredCount++
		}
		return true
	})

	if expiredCount > 0 {
		m.logger.Debug("Probe config cache refreshed",
			zap.Int("expired_count", expiredCount))
	}
}

// Close 修复：优雅关闭
func (m *ProbeConfigManager) Close() error {
	m.cancel()

	// 等待刷新循环退出
	if m.config.EnableDynamicConfig {
		select {
		case <-m.done:
			m.logger.Info("Probe config manager closed gracefully")
		case <-time.After(5 * time.Second):
			m.logger.Warn("Probe config manager close timeout")
		}
	}

	return nil
}

// ProbeConfigUpdate 配置更新事件
type ProbeConfigUpdate struct {
	TenantID  string          `json:"tenant_id"`
	ProbeID   string          `json:"probe_id"`
	Config    *pb.ProbeConfig `json:"config"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// PublishConfigUpdate 发布配置更新（用于通知探针）
func (m *ProbeConfigManager) PublishConfigUpdate(ctx context.Context, update *ProbeConfigUpdate) error {
	if m.redis == nil {
		return nil
	}

	channel := fmt.Sprintf("probe_config_update:%s", update.TenantID)
	data, err := json.Marshal(update)
	if err != nil {
		return err
	}

	return m.redis.Publish(ctx, channel, data).Err()
}

// SubscribeConfigUpdates 订阅配置更新（用于探针）
func (m *ProbeConfigManager) SubscribeConfigUpdates(ctx context.Context, tenantID string, handler func(*ProbeConfigUpdate)) error {
	if m.redis == nil {
		return fmt.Errorf("redis not available")
	}

	channel := fmt.Sprintf("probe_config_update:%s", tenantID)
	pubsub := m.redis.Subscribe(ctx, channel)
	defer pubsub.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-pubsub.Channel():
			var update ProbeConfigUpdate
			if err := json.Unmarshal([]byte(msg.Payload), &update); err != nil {
				m.logger.Error("Failed to unmarshal config update", zap.Error(err))
				continue
			}
			handler(&update)
		}
	}
}

// GetProbeConfigsByTenant 获取租户所有探针配置
func (m *ProbeConfigManager) GetProbeConfigsByTenant(ctx context.Context, tenantID string) (map[string]*pb.ProbeConfig, error) {
	result := make(map[string]*pb.ProbeConfig)

	if m.redis == nil || !m.config.EnableDynamicConfig {
		// 返回默认配置
		result["default"] = m.defaultConfig
		return result, nil
	}

	// 扫描 Redis 中的探针配置
	pattern := fmt.Sprintf("probe_config:%s:*", tenantID)
	iter := m.redis.Scan(ctx, 0, pattern, 0).Iterator()

	for iter.Next(ctx) {
		key := iter.Val()
		data, err := m.redis.Get(ctx, key).Bytes()
		if err != nil {
			m.logger.Warn("Failed to get probe config",
				zap.String("key", key),
				zap.Error(err))
			continue
		}

		var cfg pb.ProbeConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			m.logger.Warn("Failed to unmarshal probe config",
				zap.String("key", key),
				zap.Error(err))
			continue
		}

		// 提取 probe_id
		probeID := key[len(fmt.Sprintf("probe_config:%s:", tenantID)):]
		result[probeID] = &cfg
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	// 如果没有找到任何配置，返回默认配置
	if len(result) == 0 {
		result["default"] = m.defaultConfig
	}

	return result, nil
}

// BulkSetProbeConfigs 批量设置探针配置
func (m *ProbeConfigManager) BulkSetProbeConfigs(ctx context.Context, tenantID string, configs map[string]*pb.ProbeConfig) error {
	if m.redis == nil {
		return fmt.Errorf("redis not available")
	}

	pipe := m.redis.TxPipeline()

	for probeID, cfg := range configs {
		redisKey := fmt.Sprintf("probe_config:%s:%s", tenantID, probeID)
		data, err := json.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("failed to marshal config for %s: %w", probeID, err)
		}
		pipe.Set(ctx, redisKey, data, 24*time.Hour)

		// 清除本地缓存
		cacheKey := fmt.Sprintf("%s:%s", tenantID, probeID)
		m.cache.Delete(cacheKey)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("pipeline execution failed: %w", err)
	}

	m.logger.Info("Bulk probe configs updated",
		zap.String("tenant_id", tenantID),
		zap.Int("count", len(configs)))

	return nil
}

// GetConfigHistory 获取配置历史（最近 N 个版本）
func (m *ProbeConfigManager) GetConfigHistory(ctx context.Context, tenantID, probeID string, limit int) ([]*pb.ProbeConfig, error) {
	if m.redis == nil {
		return nil, fmt.Errorf("redis not available")
	}

	if limit <= 0 || limit > 100 {
		limit = 10
	}

	historyKey := fmt.Sprintf("probe_config_history:%s:%s", tenantID, probeID)
	data, err := m.redis.LRange(ctx, historyKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get config history: %w", err)
	}

	configs := make([]*pb.ProbeConfig, 0, len(data))
	for _, item := range data {
		var cfg pb.ProbeConfig
		if err := json.Unmarshal([]byte(item), &cfg); err != nil {
			m.logger.Warn("Failed to unmarshal historical config",
				zap.String("tenant_id", tenantID),
				zap.String("probe_id", probeID),
				zap.Error(err))
			continue
		}
		configs = append(configs, &cfg)
	}

	return configs, nil
}

// AddToHistory 添加配置到历史记录
func (m *ProbeConfigManager) AddToHistory(ctx context.Context, tenantID, probeID string, cfg *pb.ProbeConfig) error {
	if m.redis == nil {
		return nil
	}

	historyKey := fmt.Sprintf("probe_config_history:%s:%s", tenantID, probeID)
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	pipe := m.redis.TxPipeline()
	pipe.LPush(ctx, historyKey, data)
	pipe.LTrim(ctx, historyKey, 0, 9)             // 保留最近 10 个版本
	pipe.Expire(ctx, historyKey, 30*24*time.Hour) // 30天过期
	_, err = pipe.Exec(ctx)

	return err
}
