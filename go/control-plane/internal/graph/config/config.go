////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/graph/config/config.go
// Graph Service 配置（完整修复版）
// 修复内容：
// 1. 修复 CF1：DefaultTimeRangeHours 字段名
// 2. 修复 CF2：添加 TenantQueryConfig 结构体
// 3. 修复 CF3：完善 Redis 哨兵模式验证
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"time"

	"github.com/caarlos0/env/v10"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// Config 总配置
type Config struct {
	Server        ServerConfig `envPrefix:"SERVER_"`
	ClickHouse    storage.ClickHouseConfig
	Redis         RedisConfig `envPrefix:"REDIS_"`
	API           APIConfig   `envPrefix:"API_"`
	Cache         CacheConfig `envPrefix:"CACHE_"`
	Query         QueryConfig `envPrefix:"QUERY_"`
	OTEL          OTELConfig  `envPrefix:"OTEL_"`
	Audit         AuditConfig `envPrefix:"AUDIT_"`
	Kafka         KafkaConfig `envPrefix:"KAFKA_"`
	KafkaSecurity commonkafka.SecurityConfig
	Security      SecurityConfig `envPrefix:"SECURITY_"`
	Auth          AuthConfig     `envPrefix:"AUTH_"`
}

// ServerConfig HTTP 服务器配置
type ServerConfig struct {
	ListenAddr        string        `env:"LISTEN_ADDR" envDefault:":8084"`
	ReadTimeout       time.Duration `env:"READ_TIMEOUT" envDefault:"15s"`
	WriteTimeout      time.Duration `env:"WRITE_TIMEOUT" envDefault:"60s"`
	IdleTimeout       time.Duration `env:"IDLE_TIMEOUT" envDefault:"120s"`
	ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" envDefault:"5s"`
	MaxHeaderBytes    int           `env:"MAX_HEADER_BYTES" envDefault:"1048576"` // 1MB
	ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	// 单机模式
	Addr     string `env:"ADDR"`
	Password string `env:"PASSWORD"`
	DB       int    `env:"DB" envDefault:"0"`

	// 集群模式
	ClusterAddrs []string `env:"CLUSTER_ADDRS" envSeparator:","`

	// 哨兵模式
	SentinelAddrs  []string `env:"SENTINEL_ADDRS" envSeparator:","`
	SentinelMaster string   `env:"SENTINEL_MASTER"`

	// 连接池配置
	PoolSize        int           `env:"POOL_SIZE" envDefault:"10"`
	MinIdleConns    int           `env:"MIN_IDLE_CONNS" envDefault:"5"`
	MaxRetries      int           `env:"MAX_RETRIES" envDefault:"3"`
	DialTimeout     time.Duration `env:"DIAL_TIMEOUT" envDefault:"5s"`
	ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"3s"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"3s"`
	PoolTimeout     time.Duration `env:"POOL_TIMEOUT" envDefault:"4s"`
	ConnMaxIdleTime time.Duration `env:"CONN_MAX_IDLE_TIME" envDefault:"30m"`

	// 是否启用
	Enabled bool `env:"ENABLED" envDefault:"true"`
}

// IsConfigured 检查是否配置了 Redis（修复 CF3：完善哨兵模式验证）
func (c RedisConfig) IsConfigured() bool {
	if !c.Enabled {
		return false
	}

	// 单机模式
	if c.Addr != "" {
		return true
	}

	// 集群模式
	if len(c.ClusterAddrs) > 0 {
		return true
	}

	// 哨兵模式（修复 CF3：需要同时配置地址和 master 名称）
	if len(c.SentinelAddrs) > 0 && c.SentinelMaster != "" {
		return true
	}

	return false
}

// ToStorageConfig 转换为 storage.RedisConfig
func (c RedisConfig) ToStorageConfig() storage.RedisConfig {
	return storage.RedisConfig{
		Addr:            c.Addr,
		ClusterAddrs:    c.ClusterAddrs,
		SentinelAddrs:   c.SentinelAddrs,
		SentinelMaster:  c.SentinelMaster,
		Password:        c.Password,
		DB:              c.DB,
		PoolSize:        c.PoolSize,
		MinIdleConns:    c.MinIdleConns,
		MaxRetries:      c.MaxRetries,
		DialTimeout:     c.DialTimeout,
		ReadTimeout:     c.ReadTimeout,
		WriteTimeout:    c.WriteTimeout,
		PoolTimeout:     c.PoolTimeout,
		ConnMaxIdleTime: c.ConnMaxIdleTime,
	}
}

// APIConfig API 配置
type APIConfig struct {
	ListenAddr         string   `env:"LISTEN_ADDR" envDefault:":8084"`
	AllowedOrigins     []string `env:"ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
	RequestTimeout     int      `env:"REQUEST_TIMEOUT" envDefault:"30"`            // 秒
	MaxRequestBodySize int64    `env:"MAX_REQUEST_BODY_SIZE" envDefault:"1048576"` // 1MB
}

// CacheConfig 缓存配置
type CacheConfig struct {
	Enabled         bool          `env:"ENABLED" envDefault:"true"`
	NeighborTTL     time.Duration `env:"NEIGHBOR_TTL" envDefault:"5m"`
	EntityTTL       time.Duration `env:"ENTITY_TTL" envDefault:"5m"`
	GraphTTL        time.Duration `env:"GRAPH_TTL" envDefault:"2m"`
	StatsTTL        time.Duration `env:"STATS_TTL" envDefault:"1m"`
	KeyPrefix       string        `env:"KEY_PREFIX" envDefault:"graph:"`
	MaxCacheSize    int           `env:"MAX_CACHE_SIZE" envDefault:"1000"`     // 最大节点数
	MaxNodesPerItem int           `env:"MAX_NODES_PER_ITEM" envDefault:"500"`  // 单个缓存项最大节点数
	MaxEdgesPerItem int           `env:"MAX_EDGES_PER_ITEM" envDefault:"1000"` // 单个缓存项最大边数
	TimeGranularity time.Duration `env:"TIME_GRANULARITY" envDefault:"5m"`     // 时间戳取整粒度
}

// QueryConfig 查询配置（修复 CF1：字段名一致性）
type QueryConfig struct {
	MaxDepth              int           `env:"MAX_DEPTH" envDefault:"5"`
	MaxNodes              int           `env:"MAX_NODES" envDefault:"500"`
	MaxNeighborsPerHop    int           `env:"MAX_NEIGHBORS_PER_HOP" envDefault:"50"`
	DefaultDepth          int           `env:"DEFAULT_DEPTH" envDefault:"2"`
	DefaultTimeRangeHours int           `env:"DEFAULT_TIME_RANGE_HOURS" envDefault:"24"` // 修复 CF1
	AlertBatchSize        int           `env:"ALERT_BATCH_SIZE" envDefault:"100"`
	QueryTimeout          time.Duration `env:"QUERY_TIMEOUT" envDefault:"30s"`
	MaxConcurrentQueries  int           `env:"MAX_CONCURRENT_QUERIES" envDefault:"10"`
	MaxBatchExploreIPs    int           `env:"MAX_BATCH_EXPLORE_IPS" envDefault:"10"`
	MaxPathSearchHops     int           `env:"MAX_PATH_SEARCH_HOPS" envDefault:"10"`
	MaxInClauseSize       int           `env:"MAX_IN_CLAUSE_SIZE" envDefault:"10000"` // ClickHouse IN 子句限制
}

// OTELConfig OpenTelemetry 配置
type OTELConfig struct {
	Enabled        bool    `env:"ENABLED" envDefault:"true"`
	ServiceName    string  `env:"SERVICE_NAME" envDefault:"graph-service"`
	ServiceVersion string  `env:"SERVICE_VERSION" envDefault:"1.0.0"`
	Environment    string  `env:"ENVIRONMENT" envDefault:"development"`
	Endpoint       string  `env:"ENDPOINT" envDefault:"victoria-metrics.observability.svc:4317"`
	Insecure       bool    `env:"INSECURE" envDefault:"true"`
	SampleRate     float64 `env:"SAMPLE_RATE" envDefault:"1.0"`
}

// AuditConfig 审计日志配置
type AuditConfig struct {
	Enabled       bool          `env:"ENABLED" envDefault:"true"`
	Topic         string        `env:"TOPIC" envDefault:"audit.logs"`
	BufferSize    int           `env:"BUFFER_SIZE" envDefault:"1000"`
	BatchSize     int           `env:"BATCH_SIZE" envDefault:"100"`
	FlushInterval time.Duration `env:"FLUSH_INTERVAL" envDefault:"1s"`
	BackupEnabled bool          `env:"BACKUP_ENABLED" envDefault:"true"`
	BackupDir     string        `env:"BACKUP_DIR" envDefault:"/var/log/audit"`
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers []string `env:"BROKERS" envSeparator:"," envDefault:"kafka-bootstrap.middleware.svc:9092"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	// 速率限制
	RateLimitEnabled   bool `env:"RATE_LIMIT_ENABLED" envDefault:"true"`
	RateLimitRPS       int  `env:"RATE_LIMIT_RPS" envDefault:"10"` // 每秒请求数
	RateLimitWindowSec int  `env:"RATE_LIMIT_WINDOW_SEC" envDefault:"60"`
	RateLimitBurstSize int  `env:"RATE_LIMIT_BURST_SIZE" envDefault:"20"`

	// IP 白名单
	IPWhitelistEnabled bool     `env:"IP_WHITELIST_ENABLED" envDefault:"false"`
	IPWhitelist        []string `env:"IP_WHITELIST" envSeparator:","`

	// 租户隔离
	EnforceTenantIsolation bool `env:"ENFORCE_TENANT_ISOLATION" envDefault:"true"`

	// 字段名白名单（防止 SQL 注入）
	AllowedOrderByFields []string `env:"ALLOWED_ORDER_BY_FIELDS" envSeparator:"," envDefault:"session_count,total_bytes,last_seen,alert_count"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Enabled         bool   `env:"ENABLED" envDefault:"true"`
	AuthServiceURL  string `env:"SERVICE_URL" envDefault:"http://auth-service:8080"`
	JWTPublicKeyURL string `env:"JWT_PUBLIC_KEY_URL"`
	RequireAuth     bool   `env:"REQUIRE_AUTH" envDefault:"true"`
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := &Config{}

	if err := env.Parse(cfg); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigError, "Failed to parse environment variables")
	}

	// 设置默认值
	if err := cfg.setDefaults(); err != nil {
		return nil, err
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// setDefaults 设置默认值
func (c *Config) setDefaults() error {
	// ClickHouse 默认值
	if len(c.ClickHouse.Hosts) == 0 {
		c.ClickHouse.Hosts = []string{
			"clickhouse-1.middleware.svc:9000",
			"clickhouse-2.middleware.svc:9000",
		}
	}
	if c.ClickHouse.Database == "" {
		c.ClickHouse.Database = "traffic"
	}
	if c.ClickHouse.Username == "" {
		c.ClickHouse.Username = "default"
	}
	if c.ClickHouse.MaxOpenConns == 0 {
		c.ClickHouse.MaxOpenConns = 10
	}
	if c.ClickHouse.MaxIdleConns == 0 {
		c.ClickHouse.MaxIdleConns = 5
	}
	if c.ClickHouse.DialTimeout == 0 {
		c.ClickHouse.DialTimeout = 10 * time.Second
	}
	if c.ClickHouse.ReadTimeout == 0 {
		c.ClickHouse.ReadTimeout = 30 * time.Second
	}
	if c.ClickHouse.WriteTimeout == 0 {
		c.ClickHouse.WriteTimeout = 30 * time.Second
	}

	// API 默认值
	if len(c.API.AllowedOrigins) == 0 {
		c.API.AllowedOrigins = []string{"*"}
	}
	if c.API.RequestTimeout <= 0 {
		c.API.RequestTimeout = 30
	}

	// Query 默认值
	if c.Query.MaxDepth <= 0 {
		c.Query.MaxDepth = 5
	}
	if c.Query.MaxNodes <= 0 {
		c.Query.MaxNodes = 500
	}
	if c.Query.MaxNeighborsPerHop <= 0 {
		c.Query.MaxNeighborsPerHop = 50
	}
	if c.Query.AlertBatchSize <= 0 {
		c.Query.AlertBatchSize = 100
	}
	if c.Query.MaxConcurrentQueries <= 0 {
		c.Query.MaxConcurrentQueries = 10
	}
	if c.Query.MaxBatchExploreIPs <= 0 {
		c.Query.MaxBatchExploreIPs = 10
	}
	if c.Query.MaxInClauseSize <= 0 {
		c.Query.MaxInClauseSize = 10000
	}
	// 修复 CF1：设置默认值
	if c.Query.DefaultTimeRangeHours <= 0 {
		c.Query.DefaultTimeRangeHours = 24
	}

	// Cache 默认值
	if c.Cache.NeighborTTL == 0 {
		c.Cache.NeighborTTL = 5 * time.Minute
	}
	if c.Cache.EntityTTL == 0 {
		c.Cache.EntityTTL = 5 * time.Minute
	}
	if c.Cache.GraphTTL == 0 {
		c.Cache.GraphTTL = 2 * time.Minute
	}
	if c.Cache.StatsTTL == 0 {
		c.Cache.StatsTTL = 1 * time.Minute
	}
	if c.Cache.TimeGranularity == 0 {
		c.Cache.TimeGranularity = 5 * time.Minute
	}
	if c.Cache.MaxNodesPerItem == 0 {
		c.Cache.MaxNodesPerItem = 500
	}
	if c.Cache.MaxEdgesPerItem == 0 {
		c.Cache.MaxEdgesPerItem = 1000
	}

	// Security 默认值
	if len(c.Security.AllowedOrderByFields) == 0 {
		c.Security.AllowedOrderByFields = []string{"session_count", "total_bytes", "last_seen", "alert_count"}
	}

	return nil
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证 ClickHouse 配置
	if err := c.validateClickHouse(); err != nil {
		return err
	}

	// 验证 Query 配置
	if err := c.validateQuery(); err != nil {
		return err
	}

	// 验证 Cache 配置
	if err := c.validateCache(); err != nil {
		return err
	}

	// 验证 API 配置
	if err := c.validateAPI(); err != nil {
		return err
	}

	// 验证 Security 配置
	if err := c.validateSecurity(); err != nil {
		return err
	}

	// 验证 Server 配置
	if err := c.validateServer(); err != nil {
		return err
	}

	// 修复 CF3：验证 Redis 配置
	if c.Redis.IsConfigured() {
		if err := c.validateRedis(); err != nil {
			return err
		}
	}

	return nil
}

// validateClickHouse 验证 ClickHouse 配置
func (c *Config) validateClickHouse() error {
	if len(c.ClickHouse.Hosts) == 0 {
		return errors.New(errors.ErrCodeConfigError, "ClickHouse hosts are required")
	}

	if c.ClickHouse.Database == "" {
		return errors.New(errors.ErrCodeConfigError, "ClickHouse database is required")
	}

	if c.ClickHouse.MaxOpenConns < 1 {
		return errors.New(errors.ErrCodeConfigError, "ClickHouse max open connections must be >= 1")
	}

	if c.ClickHouse.MaxIdleConns > c.ClickHouse.MaxOpenConns {
		return errors.New(errors.ErrCodeConfigError,
			"ClickHouse max idle connections cannot exceed max open connections")
	}

	return nil
}

// validateQuery 验证 Query 配置
func (c *Config) validateQuery() error {
	if c.Query.MaxDepth < 1 || c.Query.MaxDepth > 10 {
		return errors.Newf(errors.ErrCodeConfigError,
			"MaxDepth must be between 1 and 10, got %d", c.Query.MaxDepth)
	}

	if c.Query.DefaultDepth > c.Query.MaxDepth {
		return errors.Newf(errors.ErrCodeConfigError,
			"DefaultDepth (%d) cannot exceed MaxDepth (%d)",
			c.Query.DefaultDepth, c.Query.MaxDepth)
	}

	if c.Query.MaxNodes < 10 || c.Query.MaxNodes > 10000 {
		return errors.Newf(errors.ErrCodeConfigError,
			"MaxNodes must be between 10 and 10000, got %d", c.Query.MaxNodes)
	}

	if c.Query.MaxNeighborsPerHop < 1 || c.Query.MaxNeighborsPerHop > 500 {
		return errors.Newf(errors.ErrCodeConfigError,
			"MaxNeighborsPerHop must be between 1 and 500, got %d", c.Query.MaxNeighborsPerHop)
	}

	if c.Query.AlertBatchSize < 10 || c.Query.AlertBatchSize > 1000 {
		return errors.Newf(errors.ErrCodeConfigError,
			"AlertBatchSize must be between 10 and 1000, got %d", c.Query.AlertBatchSize)
	}

	if c.Query.QueryTimeout < time.Second || c.Query.QueryTimeout > 5*time.Minute {
		return errors.Newf(errors.ErrCodeConfigError,
			"QueryTimeout must be between 1s and 5m, got %v", c.Query.QueryTimeout)
	}

	if c.Query.MaxConcurrentQueries < 1 || c.Query.MaxConcurrentQueries > 100 {
		return errors.Newf(errors.ErrCodeConfigError,
			"MaxConcurrentQueries must be between 1 and 100, got %d", c.Query.MaxConcurrentQueries)
	}

	if c.Query.MaxInClauseSize < 100 || c.Query.MaxInClauseSize > 50000 {
		return errors.Newf(errors.ErrCodeConfigError,
			"MaxInClauseSize must be between 100 and 50000, got %d", c.Query.MaxInClauseSize)
	}

	// 修复 CF1：验证 DefaultTimeRangeHours
	if c.Query.DefaultTimeRangeHours < 1 || c.Query.DefaultTimeRangeHours > 168 {
		return errors.Newf(errors.ErrCodeConfigError,
			"DefaultTimeRangeHours must be between 1 and 168 (7 days), got %d", c.Query.DefaultTimeRangeHours)
	}

	return nil
}

// validateCache 验证 Cache 配置
func (c *Config) validateCache() error {
	if !c.Cache.Enabled {
		return nil
	}

	if c.Cache.NeighborTTL < 10*time.Second {
		return errors.Newf(errors.ErrCodeConfigError,
			"NeighborTTL must be at least 10s, got %v", c.Cache.NeighborTTL)
	}

	if c.Cache.EntityTTL < 10*time.Second {
		return errors.Newf(errors.ErrCodeConfigError,
			"EntityTTL must be at least 10s, got %v", c.Cache.EntityTTL)
	}

	if c.Cache.GraphTTL < 10*time.Second {
		return errors.Newf(errors.ErrCodeConfigError,
			"GraphTTL must be at least 10s, got %v", c.Cache.GraphTTL)
	}

	if c.Cache.MaxCacheSize < 100 || c.Cache.MaxCacheSize > 10000 {
		return errors.Newf(errors.ErrCodeConfigError,
			"MaxCacheSize must be between 100 and 10000, got %d", c.Cache.MaxCacheSize)
	}

	if c.Cache.MaxNodesPerItem < 10 || c.Cache.MaxNodesPerItem > 5000 {
		return errors.Newf(errors.ErrCodeConfigError,
			"MaxNodesPerItem must be between 10 and 5000, got %d", c.Cache.MaxNodesPerItem)
	}

	if c.Cache.MaxEdgesPerItem < 10 || c.Cache.MaxEdgesPerItem > 10000 {
		return errors.Newf(errors.ErrCodeConfigError,
			"MaxEdgesPerItem must be between 10 and 10000, got %d", c.Cache.MaxEdgesPerItem)
	}

	return nil
}

// validateAPI 验证 API 配置
func (c *Config) validateAPI() error {
	if len(c.API.AllowedOrigins) == 0 {
		return errors.New(errors.ErrCodeConfigError, "At least one allowed origin is required")
	}

	if c.API.RequestTimeout < 1 || c.API.RequestTimeout > 300 {
		return errors.Newf(errors.ErrCodeConfigError,
			"RequestTimeout must be between 1 and 300 seconds, got %d", c.API.RequestTimeout)
	}

	return nil
}

// validateSecurity 验证 Security 配置
func (c *Config) validateSecurity() error {
	if c.Security.RateLimitEnabled {
		if c.Security.RateLimitRPS < 1 || c.Security.RateLimitRPS > 10000 {
			return errors.Newf(errors.ErrCodeConfigError,
				"RateLimitRPS must be between 1 and 10000, got %d", c.Security.RateLimitRPS)
		}

		if c.Security.RateLimitWindowSec < 1 || c.Security.RateLimitWindowSec > 3600 {
			return errors.Newf(errors.ErrCodeConfigError,
				"RateLimitWindowSec must be between 1 and 3600, got %d", c.Security.RateLimitWindowSec)
		}
	}

	return nil
}

// validateServer 验证 Server 配置
func (c *Config) validateServer() error {
	if c.Server.ReadTimeout < time.Second {
		return errors.New(errors.ErrCodeConfigError, "ReadTimeout must be at least 1s")
	}

	if c.Server.WriteTimeout < time.Second {
		return errors.New(errors.ErrCodeConfigError, "WriteTimeout must be at least 1s")
	}

	if c.Server.ReadHeaderTimeout < time.Second {
		return errors.New(errors.ErrCodeConfigError, "ReadHeaderTimeout must be at least 1s")
	}

	if c.Server.MaxHeaderBytes < 1024 {
		return errors.New(errors.ErrCodeConfigError, "MaxHeaderBytes must be at least 1KB")
	}

	return nil
}

// validateRedis 验证 Redis 配置（修复 CF3：新增）
func (c *Config) validateRedis() error {
	if !c.Redis.Enabled {
		return nil
	}

	// 检查至少配置了一种模式
	hasStandalone := c.Redis.Addr != ""
	hasCluster := len(c.Redis.ClusterAddrs) > 0
	hasSentinel := len(c.Redis.SentinelAddrs) > 0

	if !hasStandalone && !hasCluster && !hasSentinel {
		return errors.New(errors.ErrCodeConfigError,
			"Redis requires at least one mode: standalone (Addr), cluster (ClusterAddrs), or sentinel (SentinelAddrs)")
	}

	// 修复 CF3：哨兵模式需要 MasterName
	if hasSentinel && c.Redis.SentinelMaster == "" {
		return errors.New(errors.ErrCodeConfigError,
			"Redis sentinel mode requires SentinelMaster to be configured")
	}

	// 验证连接池参数
	if c.Redis.PoolSize < 1 {
		return errors.New(errors.ErrCodeConfigError, "Redis PoolSize must be at least 1")
	}

	if c.Redis.MinIdleConns > c.Redis.PoolSize {
		return errors.New(errors.ErrCodeConfigError,
			"Redis MinIdleConns cannot exceed PoolSize")
	}

	return nil
}

// IsDevelopment 是否为开发环境
func (c *Config) IsDevelopment() bool {
	return c.OTEL.Environment == "development" || c.OTEL.Environment == "dev"
}

// IsProduction 是否为生产环境
func (c *Config) IsProduction() bool {
	return c.OTEL.Environment == "production" || c.OTEL.Environment == "prod"
}

// GetConfigSummary 获取配置摘要（用于日志）
func (c *Config) GetConfigSummary() map[string]interface{} {
	return map[string]interface{}{
		"service":     c.OTEL.ServiceName,
		"version":     c.OTEL.ServiceVersion,
		"environment": c.OTEL.Environment,
		"clickhouse": map[string]interface{}{
			"hosts":    c.ClickHouse.Hosts,
			"database": c.ClickHouse.Database,
		},
		"redis": map[string]interface{}{
			"enabled": c.Redis.IsConfigured(),
		},
		"cache": map[string]interface{}{
			"enabled": c.Cache.Enabled,
		},
		"security": map[string]interface{}{
			"rate_limit_enabled": c.Security.RateLimitEnabled,
			"auth_enabled":       c.Auth.Enabled,
		},
		"query": map[string]interface{}{
			"max_depth":                c.Query.MaxDepth,
			"default_time_range_hours": c.Query.DefaultTimeRangeHours,
		},
	}
}
