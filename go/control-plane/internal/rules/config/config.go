////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/config/config.go
// 完整修复版：添加 Redis、Audit、RBAC、超时、限流配置
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"strconv"
	"time"

	"github.com/caarlos0/env/v10"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

// MetricsConfig Metrics 配置
type MetricsConfig struct {
	Enabled    bool   `env:"METRICS_ENABLED" envDefault:"true"`
	ListenAddr string `env:"METRICS_LISTEN_ADDR" envDefault:":9091"`
}

// ServiceConfig 服务配置
type ServiceConfig struct {
	MaxRetries       int           `env:"SERVICE_MAX_RETRIES" envDefault:"3"`
	CacheEnabled     bool          `env:"SERVICE_CACHE_ENABLED" envDefault:"true"`
	CacheTTL         time.Duration `env:"SERVICE_CACHE_TTL" envDefault:"5m"`
	SyncEnabled      bool          `env:"SERVICE_SYNC_ENABLED" envDefault:"true"`
	ValidationStrict bool          `env:"SERVICE_VALIDATION_STRICT" envDefault:"true"`
}

// DeploymentConfig 部署配置
type DeploymentConfig struct {
	EnableGrayValidation  bool          `env:"DEPLOYMENT_ENABLE_GRAY_VALIDATION" envDefault:"true"`
	MaxGrayDuration       time.Duration `env:"DEPLOYMENT_MAX_GRAY_DURATION" envDefault:"24h"`
	RequireRollbackReason bool          `env:"DEPLOYMENT_REQUIRE_ROLLBACK_REASON" envDefault:"true"`
	EnableAutoRollback    bool          `env:"DEPLOYMENT_ENABLE_AUTO_ROLLBACK" envDefault:"true"`
	AutoRollbackThreshold float64       `env:"DEPLOYMENT_AUTO_ROLLBACK_THRESHOLD" envDefault:"0.05"`
}

// Config 完整配置
type Config struct {
	API        APIConfig
	PostgreSQL PostgreSQLConfig
	ClickHouse ClickHouseConfig
	Kafka      KafkaConfig
	Redis      RedisConfig
	Audit      AuditConfig
	RBAC       RBACConfig
	RateLimit  RateLimitConfig
	Cache      CacheConfig
	Metrics    MetricsConfig
	Service    ServiceConfig
	Deployment DeploymentConfig
}

// APIConfig API 服务配置
type APIConfig struct {
	ListenAddr     string        `env:"API_LISTEN_ADDR" envDefault:":8082"`
	ReadTimeout    time.Duration `env:"API_READ_TIMEOUT" envDefault:"15s"`
	WriteTimeout   time.Duration `env:"API_WRITE_TIMEOUT" envDefault:"15s"`
	IdleTimeout    time.Duration `env:"API_IDLE_TIMEOUT" envDefault:"60s"`
	RequestTimeout time.Duration `env:"API_REQUEST_TIMEOUT" envDefault:"30s"`
	MaxRequestSize int64         `env:"API_MAX_REQUEST_SIZE" envDefault:"10485760"` // 10MB
	// CORS
	AllowedOrigins []string `env:"API_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
	// Metrics
	MetricsEnabled bool   `env:"API_METRICS_ENABLED" envDefault:"true"`
	MetricsAddr    string `env:"API_METRICS_ADDR" envDefault:":9091"`
	// Pagination
	MaxPageSize     int `env:"API_MAX_PAGE_SIZE" envDefault:"1000"`
	DefaultPageSize int `env:"API_DEFAULT_PAGE_SIZE" envDefault:"20"`
	// Request Log
	EnableRequestLog bool `env:"API_ENABLE_REQUEST_LOG" envDefault:"true"`
}

// PostgreSQLConfig PostgreSQL 配置
type PostgreSQLConfig struct {
	DSN             string        `env:"POSTGRES_DSN"`
	Host            string        `env:"POSTGRES_HOST" envDefault:"postgres-primary.databases.svc"`
	Port            int           `env:"POSTGRES_PORT" envDefault:"5432"`
	Database        string        `env:"POSTGRES_DATABASE" envDefault:"traffic"`
	Username        string        `env:"POSTGRES_USERNAME" envDefault:"postgres"`
	Password        string        `env:"POSTGRES_PASSWORD" envDefault:""`
	SSLMode         string        `env:"POSTGRES_SSL_MODE" envDefault:"disable"`
	MaxOpenConns    int           `env:"POSTGRES_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"POSTGRES_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"POSTGRES_CONN_MAX_LIFETIME" envDefault:"1h"`
	ConnMaxIdleTime time.Duration `env:"POSTGRES_CONN_MAX_IDLE_TIME" envDefault:"30m"`
	ConnectTimeout  time.Duration `env:"POSTGRES_CONNECT_TIMEOUT" envDefault:"10s"`
}

// ClickHouseConfig ClickHouse 配置（用于 MLOps 自编排评估）
type ClickHouseConfig struct {
	Enabled         bool          `env:"CLICKHOUSE_ENABLED" envDefault:"true"`
	Hosts           []string      `env:"CLICKHOUSE_HOSTS" envSeparator:","`
	Database        string        `env:"CLICKHOUSE_DATABASE" envDefault:"traffic"`
	Username        string        `env:"CLICKHOUSE_USERNAME" envDefault:"default"`
	Password        string        `env:"CLICKHOUSE_PASSWORD"`
	MaxOpenConns    int           `env:"CLICKHOUSE_MAX_OPEN_CONNS" envDefault:"10"`
	MaxIdleConns    int           `env:"CLICKHOUSE_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"CLICKHOUSE_CONN_MAX_LIFETIME" envDefault:"1h"`
	DialTimeout     time.Duration `env:"CLICKHOUSE_DIAL_TIMEOUT" envDefault:"10s"`
	ReadTimeout     time.Duration `env:"CLICKHOUSE_READ_TIMEOUT" envDefault:"30s"`
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers                         []string      `env:"KAFKA_BROKERS" envSeparator:","`
	RuleTopic                       string        `env:"KAFKA_RULE_TOPIC" envDefault:"rule.updates"`
	ModelTopic                      string        `env:"KAFKA_MODEL_TOPIC" envDefault:"model-updates"`
	ModelActionTopic                string        `env:"KAFKA_MODEL_ACTION_TOPIC" envDefault:"model-actions.v1"`
	ModelAppliedTopic               string        `env:"KAFKA_MODEL_APPLIED_TOPIC" envDefault:"model-update-applied.v1"`
	ModelAppliedExpectedParallelism int           `env:"MODEL_APPLIED_ACK_EXPECTED_PARALLELISM" envDefault:"4"`
	DeploymentTopic                 string        `env:"KAFKA_DEPLOYMENT_TOPIC" envDefault:"deployment.events.v1"`
	AuditTopic                      string        `env:"KAFKA_AUDIT_TOPIC" envDefault:"audit.logs"`
	DLQTopicPrefix                  string        `env:"KAFKA_DLQ_TOPIC_PREFIX" envDefault:"dlq."`
	BatchSize                       int           `env:"KAFKA_BATCH_SIZE" envDefault:"100"`
	BatchTimeout                    time.Duration `env:"KAFKA_BATCH_TIMEOUT" envDefault:"100ms"`
	MaxRetries                      int           `env:"KAFKA_MAX_RETRIES" envDefault:"3"`
	RequiredAcks                    string        `env:"KAFKA_REQUIRED_ACKS" envDefault:"all"`
	Compression                     string        `env:"KAFKA_COMPRESSION" envDefault:"lz4"`
	// Producer 超时
	ProduceTimeout time.Duration `env:"KAFKA_PRODUCE_TIMEOUT" envDefault:"10s"`
	SendTimeout    time.Duration `env:"KAFKA_SEND_TIMEOUT" envDefault:"5s"`
	PublishTimeout time.Duration `env:"KAFKA_PUBLISH_TIMEOUT" envDefault:"30s"`
	RetryBackoff   time.Duration `env:"KAFKA_RETRY_BACKOFF" envDefault:"100ms"`
	Security       kafkaCommon.SecurityConfig
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Enabled bool `env:"REDIS_ENABLED" envDefault:"true"`
	// 单机模式
	Addr     string `env:"REDIS_ADDR" envDefault:"redis-master.databases.svc:6379"`
	Password string `env:"REDIS_PASSWORD" envDefault:""`
	DB       int    `env:"REDIS_DB" envDefault:"0"`
	// 集群模式（修复：使用 Addrs 替代 ClusterAddrs）
	Addrs          []string `env:"REDIS_ADDRS" envSeparator:","`
	ClusterEnabled bool     `env:"REDIS_CLUSTER_ENABLED" envDefault:"false"`
	SentinelAddrs  []string `env:"REDIS_SENTINEL_ADDRS" envSeparator:","`
	SentinelMaster string   `env:"REDIS_SENTINEL_MASTER"`
	// 连接池
	PoolSize        int           `env:"REDIS_POOL_SIZE" envDefault:"10"`
	MinIdleConns    int           `env:"REDIS_MIN_IDLE_CONNS" envDefault:"5"`
	MaxRetries      int           `env:"REDIS_MAX_RETRIES" envDefault:"3"`
	DialTimeout     time.Duration `env:"REDIS_DIAL_TIMEOUT" envDefault:"5s"`
	ReadTimeout     time.Duration `env:"REDIS_READ_TIMEOUT" envDefault:"3s"`
	WriteTimeout    time.Duration `env:"REDIS_WRITE_TIMEOUT" envDefault:"3s"`
	PoolTimeout     time.Duration `env:"REDIS_POOL_TIMEOUT" envDefault:"4s"`
	ConnMaxIdleTime time.Duration `env:"REDIS_CONN_MAX_IDLE_TIME" envDefault:"30m"`
}

// AuditConfig 审计配置
type AuditConfig struct {
	Enabled         bool          `env:"AUDIT_ENABLED" envDefault:"true"`
	Topic           string        `env:"AUDIT_TOPIC" envDefault:"audit.logs"`
	BufferSize      int           `env:"AUDIT_BUFFER_SIZE" envDefault:"1000"`
	BatchSize       int           `env:"AUDIT_BATCH_SIZE" envDefault:"100"`
	FlushInterval   time.Duration `env:"AUDIT_FLUSH_INTERVAL" envDefault:"1s"`
	ShutdownTimeout time.Duration `env:"AUDIT_SHUTDOWN_TIMEOUT" envDefault:"10s"`
	// 本地备份
	BackupEnabled bool   `env:"AUDIT_BACKUP_ENABLED" envDefault:"true"`
	BackupDir     string `env:"AUDIT_BACKUP_DIR" envDefault:"/var/log/audit"`
}

// RBACConfig RBAC 配置
type RBACConfig struct {
	Enabled bool `env:"RBAC_ENABLED" envDefault:"true"`
	// 权限缓存
	CacheEnabled bool          `env:"RBAC_CACHE_ENABLED" envDefault:"true"`
	CacheTTL     time.Duration `env:"RBAC_CACHE_TTL" envDefault:"5m"`
	// 默认权限（当 RBAC 关闭时使用）
	DefaultPermissions []string `env:"RBAC_DEFAULT_PERMISSIONS" envSeparator:"," envDefault:"rule:read"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Enabled bool `env:"RATE_LIMIT_ENABLED" envDefault:"true"`
	// 全局限流
	GlobalRPS   float64 `env:"RATE_LIMIT_GLOBAL_RPS" envDefault:"1000"`
	GlobalBurst int     `env:"RATE_LIMIT_GLOBAL_BURST" envDefault:"2000"`
	// 租户限流
	TenantRPS   float64 `env:"RATE_LIMIT_TENANT_RPS" envDefault:"100"`
	TenantBurst int     `env:"RATE_LIMIT_TENANT_BURST" envDefault:"200"`
	// 用户限流
	UserRPS   float64 `env:"RATE_LIMIT_USER_RPS" envDefault:"50"`
	UserBurst int     `env:"RATE_LIMIT_USER_BURST" envDefault:"100"`
	// Redis 限流前缀
	RedisPrefix string `env:"RATE_LIMIT_REDIS_PREFIX" envDefault:"ratelimit:rule:"`
}

// CacheConfig 缓存配置
type CacheConfig struct {
	Enabled bool `env:"CACHE_ENABLED" envDefault:"true"`
	// 规则缓存
	RuleCacheTTL     time.Duration `env:"CACHE_RULE_TTL" envDefault:"5m"`
	RuleCacheMaxSize int           `env:"CACHE_RULE_MAX_SIZE" envDefault:"10000"`
	// 版本缓存
	VersionCacheTTL time.Duration `env:"CACHE_VERSION_TTL" envDefault:"1m"`
	// Redis 缓存前缀
	RedisPrefix string `env:"CACHE_REDIS_PREFIX" envDefault:"rule:cache:"`
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// 设置默认值
	if len(cfg.Kafka.Brokers) == 0 {
		cfg.Kafka.Brokers = []string{"kafka-bootstrap.middleware.svc:9092"}
	}

	if len(cfg.API.AllowedOrigins) == 0 {
		cfg.API.AllowedOrigins = []string{"*"}
	}

	if cfg.ClickHouse.Enabled && len(cfg.ClickHouse.Hosts) == 0 {
		cfg.ClickHouse.Hosts = []string{"clickhouse-1.middleware.svc:9000"}
	}

	// Redis 地址兼容处理
	if len(cfg.Redis.Addrs) == 0 && cfg.Redis.Addr != "" {
		cfg.Redis.Addrs = []string{cfg.Redis.Addr}
	} else if len(cfg.Redis.Addrs) > 0 && cfg.Redis.Addr == "" {
		cfg.Redis.Addr = cfg.Redis.Addrs[0]
	}

	return cfg, nil
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 验证必要配置
	if len(c.Kafka.Brokers) == 0 {
		return &ConfigError{Field: "Kafka.Brokers", Message: "at least one broker is required"}
	}

	if c.PostgreSQL.DSN == "" && c.PostgreSQL.Host == "" {
		return &ConfigError{Field: "PostgreSQL", Message: "DSN or Host is required"}
	}

	if c.API.ListenAddr == "" {
		return &ConfigError{Field: "API.ListenAddr", Message: "listen address is required"}
	}

	if c.ClickHouse.Enabled && len(c.ClickHouse.Hosts) == 0 {
		return &ConfigError{Field: "ClickHouse.Hosts", Message: "at least one host is required when ClickHouse is enabled"}
	}

	// 验证限流配置
	if c.RateLimit.Enabled {
		if c.RateLimit.GlobalRPS <= 0 {
			c.RateLimit.GlobalRPS = 1000
		}
		if c.RateLimit.TenantRPS <= 0 {
			c.RateLimit.TenantRPS = 100
		}
	}

	// 验证分页配置
	if c.API.MaxPageSize <= 0 {
		c.API.MaxPageSize = 1000
	}
	if c.API.DefaultPageSize <= 0 {
		c.API.DefaultPageSize = 20
	}
	if c.API.DefaultPageSize > c.API.MaxPageSize {
		c.API.DefaultPageSize = c.API.MaxPageSize
	}

	return nil
}

// ConfigError 配置错误
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config error: " + e.Field + ": " + e.Message
}

// GetPostgresDSN 获取 PostgreSQL DSN
func (c *Config) GetPostgresDSN() string {
	if c.PostgreSQL.DSN != "" {
		return c.PostgreSQL.DSN
	}

	// 构建 DSN
	return "host=" + c.PostgreSQL.Host +
		" port=" + strconv.Itoa(c.PostgreSQL.Port) +
		" user=" + c.PostgreSQL.Username +
		" password=" + c.PostgreSQL.Password +
		" dbname=" + c.PostgreSQL.Database +
		" sslmode=" + c.PostgreSQL.SSLMode
}

// IsRedisEnabled 检查 Redis 是否启用
func (c *Config) IsRedisEnabled() bool {
	return c.Redis.Enabled && (c.Redis.Addr != "" || len(c.Redis.Addrs) > 0 || len(c.Redis.SentinelAddrs) > 0)
}

// IsAuditEnabled 检查审计是否启用
func (c *Config) IsAuditEnabled() bool {
	return c.Audit.Enabled && len(c.Kafka.Brokers) > 0
}

// IsRBACEnabled 检查 RBAC 是否启用
func (c *Config) IsRBACEnabled() bool {
	return c.RBAC.Enabled
}

// IsRateLimitEnabled 检查限流是否启用
func (c *Config) IsRateLimitEnabled() bool {
	return c.RateLimit.Enabled
}

// IsCacheEnabled 检查缓存是否启用
func (c *Config) IsCacheEnabled() bool {
	return c.Cache.Enabled && c.IsRedisEnabled()
}
