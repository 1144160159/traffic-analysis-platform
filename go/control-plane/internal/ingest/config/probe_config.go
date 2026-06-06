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

type ProbeConfigManager struct {
	redis  redis.UniversalClient
	logger *zap.Logger
	config ProbeConfig

	cache   sync.Map
	cacheMu sync.RWMutex

	defaultConfig *pb.ProbeConfig

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

type cachedProbeConfig struct {
	Config    *pb.ProbeConfig
	Version   string
	CachedAt  time.Time
	ExpiresAt time.Time
}

func NewProbeConfigManager(rdb redis.UniversalClient, cfg ProbeConfig, logger *zap.Logger) *ProbeConfigManager {

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

	if cfg.EnableDynamicConfig {
		go m.startRefreshLoop()
	}

	return m
}

func (m *ProbeConfigManager) GetConfig(ctx context.Context, tenantID, probeID string) (*pb.ProbeConfig, error) {
	cacheKey := fmt.Sprintf("%s:%s", tenantID, probeID)

	if cached, ok := m.cache.Load(cacheKey); ok {
		cc := cached.(*cachedProbeConfig)
		if time.Now().Before(cc.ExpiresAt) {
			return cc.Config, nil
		}

		m.cache.Delete(cacheKey)
	}

	if m.redis != nil && m.config.EnableDynamicConfig {
		redisKey := fmt.Sprintf("probe_config:%s:%s", tenantID, probeID)
		data, err := m.redis.Get(ctx, redisKey).Bytes()
		if err == nil && len(data) > 0 {
			var cfg pb.ProbeConfig
			if err := json.Unmarshal(data, &cfg); err == nil {

				m.cacheConfig(cacheKey, &cfg, cfg.ConfigVersion)
				return &cfg, nil
			}
		}

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

	return m.defaultConfig, nil
}

func (m *ProbeConfigManager) cacheConfig(key string, config *pb.ProbeConfig, version string) {
	m.cache.Store(key, &cachedProbeConfig{
		Config:    config,
		Version:   version,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(m.config.ConfigRefreshInterval),
	})
}

func (m *ProbeConfigManager) SetConfig(ctx context.Context, tenantID, probeID string, cfg *pb.ProbeConfig) error {
	if m.redis == nil {
		return fmt.Errorf("redis not available")
	}

	cacheKey := fmt.Sprintf("%s:%s", tenantID, probeID)
	redisKey := fmt.Sprintf("probe_config:%s:%s", tenantID, probeID)

	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := m.redis.Set(ctx, redisKey, data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	m.cacheConfig(cacheKey, cfg, cfg.ConfigVersion)

	m.AddToHistory(ctx, tenantID, probeID, cfg)

	m.logger.Info("Probe config updated",
		zap.String("tenant_id", tenantID),
		zap.String("probe_id", probeID),
		zap.String("version", cfg.ConfigVersion))

	return nil
}

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

func (m *ProbeConfigManager) DeleteConfig(ctx context.Context, tenantID, probeID string) error {
	if m.redis == nil {
		return nil
	}

	cacheKey := fmt.Sprintf("%s:%s", tenantID, probeID)
	redisKey := fmt.Sprintf("probe_config:%s:%s", tenantID, probeID)

	m.redis.Del(ctx, redisKey)

	m.cache.Delete(cacheKey)

	return nil
}

func (m *ProbeConfigManager) RefreshCache(ctx context.Context, tenantID, probeID string) error {
	cacheKey := fmt.Sprintf("%s:%s", tenantID, probeID)
	m.cache.Delete(cacheKey)
	_, err := m.GetConfig(ctx, tenantID, probeID)
	return err
}

func (m *ProbeConfigManager) ClearCache() {
	m.cache.Range(func(key, value interface{}) bool {
		m.cache.Delete(key)
		return true
	})
}

func (m *ProbeConfigManager) GetDefaultConfig() *pb.ProbeConfig {
	return m.defaultConfig
}

func (m *ProbeConfigManager) SetDefaultConfig(cfg *pb.ProbeConfig) {
	m.defaultConfig = cfg
}

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

			m.logger.Info("Probe config refresh loop stopped")
			return

		case <-ticker.C:

			m.refreshExpiredCache()
		}
	}
}

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

func (m *ProbeConfigManager) Close() error {
	m.cancel()

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

type ProbeConfigUpdate struct {
	TenantID  string          `json:"tenant_id"`
	ProbeID   string          `json:"probe_id"`
	Config    *pb.ProbeConfig `json:"config"`
	UpdatedAt time.Time       `json:"updated_at"`
}

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

func (m *ProbeConfigManager) GetProbeConfigsByTenant(ctx context.Context, tenantID string) (map[string]*pb.ProbeConfig, error) {
	result := make(map[string]*pb.ProbeConfig)

	if m.redis == nil || !m.config.EnableDynamicConfig {

		result["default"] = m.defaultConfig
		return result, nil
	}

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

		probeID := key[len(fmt.Sprintf("probe_config:%s:", tenantID)):]
		result[probeID] = &cfg
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	if len(result) == 0 {
		result["default"] = m.defaultConfig
	}

	return result, nil
}

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
	pipe.LTrim(ctx, historyKey, 0, 9)
	pipe.Expire(ctx, historyKey, 30*24*time.Hour)
	_, err = pipe.Exec(ctx)

	return err
}
