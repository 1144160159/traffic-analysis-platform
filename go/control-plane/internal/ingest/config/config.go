////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/ingest/config/config.go
// 完整版：完整配置定义 - 包含所有 Ingest Gateway 配置项
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/caarlos0/env/v10"
)

// Config 完整配置
type Config struct {
	Server   ServerConfig
	HTTP     HTTPConfig
	Kafka    KafkaConfig
	Redis    RedisConfig
	Postgres PostgresConfig
	Auth     AuthConfig
	Metrics  MetricsConfig
	Handler  HandlerConfig
	Quota    QuotaConfig
	Dedup    DedupConfig
	Probe    ProbeConfig
	Audit    AuditConfig
	Token    TokenConfig
	OIDC     OIDCConfig
	JWT      JWTConfig
	API      APIConfig
}

// ServerConfig gRPC 服务器配置
type ServerConfig struct {
	GRPCAddr    string `env:"GRPC_ADDR" envDefault:":9090"`
	TLSCertFile string `env:"TLS_CERT_FILE"`
	TLSKeyFile  string `env:"TLS_KEY_FILE"`
	TLSCAFile   string `env:"TLS_CA_FILE"`

	// gRPC 选项
	MaxRecvMsgSize int `env:"GRPC_MAX_RECV_MSG_SIZE" envDefault:"67108864"` // 64MB
	MaxSendMsgSize int `env:"GRPC_MAX_SEND_MSG_SIZE" envDefault:"67108864"`

	// Keepalive
	KeepaliveTime         time.Duration `env:"GRPC_KEEPALIVE_TIME" envDefault:"5m"`
	KeepaliveTimeout      time.Duration `env:"GRPC_KEEPALIVE_TIMEOUT" envDefault:"20s"`
	MaxConnectionIdle     time.Duration `env:"GRPC_MAX_CONNECTION_IDLE" envDefault:"15m"`
	MaxConnectionAge      time.Duration `env:"GRPC_MAX_CONNECTION_AGE" envDefault:"30m"`
	MaxConnectionAgeGrace time.Duration `env:"GRPC_MAX_CONNECTION_AGE_GRACE" envDefault:"5m"`
}

// HTTPConfig HTTP/REST 服务器配置
type HTTPConfig struct {
	Enabled      bool          `env:"HTTP_ENABLED" envDefault:"true"`
	Addr         string        `env:"HTTP_ADDR" envDefault:":8080"`
	ReadTimeout  time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout  time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"120s"`

	// CORS
	AllowedOrigins []string `env:"HTTP_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
	AllowedMethods []string `env:"HTTP_ALLOWED_METHODS" envSeparator:"," envDefault:"GET,POST,PUT,DELETE,OPTIONS"`
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers      []string      `env:"KAFKA_BROKERS" envSeparator:","`
	FlowTopic    string        `env:"KAFKA_FLOW_TOPIC" envDefault:"flow.events.v1"`
	PcapTopic    string        `env:"KAFKA_PCAP_TOPIC" envDefault:"pcap.index.v1"`
	DLQTopic     string        `env:"KAFKA_DLQ_TOPIC" envDefault:"dlq.ingest-gateway"`
	BatchSize    int           `env:"KAFKA_BATCH_SIZE" envDefault:"1000"`
	BatchTimeout time.Duration `env:"KAFKA_BATCH_TIMEOUT" envDefault:"100ms"`
	Compression  string        `env:"KAFKA_COMPRESSION" envDefault:"lz4"`
	RequiredAcks string        `env:"KAFKA_REQUIRED_ACKS" envDefault:"all"`
	MaxRetries   int           `env:"KAFKA_MAX_RETRIES" envDefault:"3"`

	// 幂等配置
	EnableIdempotence bool `env:"KAFKA_ENABLE_IDEMPOTENCE" envDefault:"true"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addrs           []string      `env:"REDIS_ADDRS" envSeparator:","`
	Password        string        `env:"REDIS_PASSWORD"`
	DB              int           `env:"REDIS_DB" envDefault:"0"`
	PoolSize        int           `env:"REDIS_POOL_SIZE" envDefault:"100"`
	MinIdleConns    int           `env:"REDIS_MIN_IDLE_CONNS" envDefault:"10"`
	DialTimeout     time.Duration `env:"REDIS_DIAL_TIMEOUT" envDefault:"5s"`
	ReadTimeout     time.Duration `env:"REDIS_READ_TIMEOUT" envDefault:"3s"`
	WriteTimeout    time.Duration `env:"REDIS_WRITE_TIMEOUT" envDefault:"3s"`
	PoolTimeout     time.Duration `env:"REDIS_POOL_TIMEOUT" envDefault:"4s"`
	ConnMaxIdleTime time.Duration `env:"REDIS_CONN_MAX_IDLE_TIME" envDefault:"30m"`
}

// PostgresConfig PostgreSQL 配置（用于 Token 验证降级）
type PostgresConfig struct {
	DSN          string        `env:"POSTGRES_DSN"`
	MaxOpenConns int           `env:"POSTGRES_MAX_OPEN_CONNS" envDefault:"10"`
	MaxIdleConns int           `env:"POSTGRES_MAX_IDLE_CONNS" envDefault:"5"`
	ConnLifetime time.Duration `env:"POSTGRES_CONN_LIFETIME" envDefault:"1h"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	RequireMTLS     bool          `env:"REQUIRE_MTLS" envDefault:"false"`
	AllowNoToken    bool          `env:"ALLOW_NO_TOKEN" envDefault:"false"`
	TokenTTL        time.Duration `env:"TOKEN_TTL" envDefault:"5m"`
	LocalCacheTTL   time.Duration `env:"LOCAL_CACHE_TTL" envDefault:"30s"`
	LocalCacheSize  int           `env:"LOCAL_CACHE_SIZE" envDefault:"10000"`
	DefaultTenantID string        `env:"DEFAULT_TENANT_ID" envDefault:""`

	// 权限要求
	RequireScopes  bool     `env:"REQUIRE_SCOPES" envDefault:"true"`
	RequiredScopes []string `env:"REQUIRED_SCOPES" envSeparator:"," envDefault:"ingest:write"`

	// 审计
	EnableAudit bool `env:"ENABLE_AUDIT" envDefault:"true"`

	// 探针 RBAC
	EnableProbeRBAC bool `env:"ENABLE_PROBE_RBAC" envDefault:"true"`
}

// MetricsConfig 指标配置
type MetricsConfig struct {
	Enabled    bool   `env:"METRICS_ENABLED" envDefault:"true"`
	ListenAddr string `env:"METRICS_ADDR" envDefault:":9091"`
}

// HandlerConfig Handler 配置
type HandlerConfig struct {
	MaxBatchSize       int           `env:"MAX_BATCH_SIZE" envDefault:"10000"`
	MaxEventSize       int           `env:"MAX_EVENT_SIZE" envDefault:"65536"` // 64KB
	StreamBufferSize   int           `env:"STREAM_BUFFER_SIZE" envDefault:"1000"`
	HeartbeatInterval  time.Duration `env:"HEARTBEAT_INTERVAL" envDefault:"30s"`
	EnableDLQ          bool          `env:"ENABLE_DLQ" envDefault:"true"`
	EnableDedup        bool          `env:"ENABLE_DEDUP" envDefault:"true"`
	ProbeStatusTimeout time.Duration `env:"PROBE_STATUS_TIMEOUT" envDefault:"5m"`
}

// QuotaConfig 限流配置
type QuotaConfig struct {
	RedisEnabled         bool    `env:"RATE_LIMIT_REDIS_ENABLED" envDefault:"true"`
	RedisPrefix          string  `env:"RATE_LIMIT_REDIS_PREFIX" envDefault:"ratelimit:"`
	GlobalRPS            float64 `env:"RATE_LIMIT_GLOBAL_RPS" envDefault:"100000"`
	GlobalBurst          int     `env:"RATE_LIMIT_GLOBAL_BURST" envDefault:"200000"`
	TenantRPS            float64 `env:"RATE_LIMIT_TENANT_RPS" envDefault:"10000"`
	TenantBurst          int     `env:"RATE_LIMIT_TENANT_BURST" envDefault:"20000"`
	ProbeRPS             float64 `env:"RATE_LIMIT_PROBE_RPS" envDefault:"5000"`
	ProbeBurst           int     `env:"RATE_LIMIT_PROBE_BURST" envDefault:"10000"`
	LocalFallbackEnabled bool    `env:"RATE_LIMIT_LOCAL_FALLBACK" envDefault:"true"`
}

// DedupConfig 去重配置
type DedupConfig struct {
	Enabled        bool          `env:"DEDUP_ENABLED" envDefault:"true"`
	LocalCacheSize int           `env:"DEDUP_LOCAL_CACHE_SIZE" envDefault:"100000"`
	LocalTTL       time.Duration `env:"DEDUP_LOCAL_TTL" envDefault:"5m"`
	RedisEnabled   bool          `env:"DEDUP_REDIS_ENABLED" envDefault:"false"`
	RedisPrefix    string        `env:"DEDUP_REDIS_PREFIX" envDefault:"dedup:"`
	RedisTTL       time.Duration `env:"DEDUP_REDIS_TTL" envDefault:"10m"`
}

// ProbeConfig 探针配置管理
type ProbeConfig struct {
	// 默认配置
	DefaultSampleRate    float32       `env:"PROBE_DEFAULT_SAMPLE_RATE" envDefault:"1.0"`
	DefaultIdleTimeout   time.Duration `env:"PROBE_DEFAULT_IDLE_TIMEOUT" envDefault:"60s"`
	DefaultActiveTimeout time.Duration `env:"PROBE_DEFAULT_ACTIVE_TIMEOUT" envDefault:"300s"`
	DefaultBatchSize     int           `env:"PROBE_DEFAULT_BATCH_SIZE" envDefault:"1000"`
	DefaultBPFFilter     string        `env:"PROBE_DEFAULT_BPF_FILTER" envDefault:""`

	// 配置更新
	ConfigRefreshInterval time.Duration `env:"PROBE_CONFIG_REFRESH_INTERVAL" envDefault:"1m"`
	EnableDynamicConfig   bool          `env:"PROBE_ENABLE_DYNAMIC_CONFIG" envDefault:"true"`

	// 状态监控
	StatusTimeout         time.Duration `env:"PROBE_STATUS_TIMEOUT" envDefault:"5m"`
	StatusCleanupInterval time.Duration `env:"PROBE_STATUS_CLEANUP_INTERVAL" envDefault:"1m"`
}

// AuditConfig 审计日志配置
type AuditConfig struct {
	Enabled       bool          `env:"AUDIT_ENABLED" envDefault:"true"`
	Topic         string        `env:"AUDIT_TOPIC" envDefault:"audit.logs"`
	BufferSize    int           `env:"AUDIT_BUFFER_SIZE" envDefault:"1000"`
	BatchSize     int           `env:"AUDIT_BATCH_SIZE" envDefault:"100"`
	FlushInterval time.Duration `env:"AUDIT_FLUSH_INTERVAL" envDefault:"1s"`
}

// TokenConfig Token 管理配置
type TokenConfig struct {
	MaxTokensPerTenant int           `env:"MAX_TOKENS_PER_TENANT" envDefault:"100"`
	DefaultTTL         time.Duration `env:"DEFAULT_TOKEN_TTL" envDefault:"8760h"` // 1 year
}

// OIDCConfig OIDC 配置
type OIDCConfig struct {
	Enabled      bool   `env:"OIDC_ENABLED" envDefault:"false"`
	IssuerURL    string `env:"OIDC_ISSUER_URL"`
	ClientID     string `env:"OIDC_CLIENT_ID"`
	ClientSecret string `env:"OIDC_CLIENT_SECRET"`
	RedirectURL  string `env:"OIDC_REDIRECT_URL"`
	Scopes       string `env:"OIDC_SCOPES" envDefault:"openid profile email"`
}

// JWTConfig JWT 配置
type JWTConfig struct {
	SigningKey      string        `env:"JWT_SIGNING_KEY" envDefault:"your-256-bit-secret-key-here"`
	SigningMethod   string        `env:"JWT_SIGNING_METHOD" envDefault:"HS256"`
	AccessTokenTTL  time.Duration `env:"JWT_ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"JWT_REFRESH_TOKEN_TTL" envDefault:"7d"`
	Issuer          string        `env:"JWT_ISSUER" envDefault:"traffic-auth-service"`
}

// APIConfig API 服务器配置
type APIConfig struct {
	ListenAddr     string        `env:"API_LISTEN_ADDR" envDefault:":8080"`
	ReadTimeout    time.Duration `env:"API_READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout   time.Duration `env:"API_WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout    time.Duration `env:"API_IDLE_TIMEOUT" envDefault:"120s"`
	AllowedOrigins []string      `env:"API_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// 设置默认值
	if len(cfg.Redis.Addrs) == 0 {
		cfg.Redis.Addrs = []string{"localhost:6379"}
	}
	if len(cfg.Kafka.Brokers) == 0 {
		cfg.Kafka.Brokers = []string{"localhost:9092"}
	}
	if cfg.JWT.SigningKey == "your-256-bit-secret-key-here" {
		// 生产环境必须设置签名密钥
		if os.Getenv("ENVIRONMENT") != "development" {
			return nil, fmt.Errorf("JWT_SIGNING_KEY must be set in production")
		}
	}

	return cfg, nil
}

// Validate 验证配置
func (c *Config) Validate() error {
	if len(c.Kafka.Brokers) == 0 {
		return &ConfigError{Field: "Kafka.Brokers", Message: "at least one broker required"}
	}
	if c.Handler.MaxBatchSize <= 0 {
		return &ConfigError{Field: "Handler.MaxBatchSize", Message: "must be positive"}
	}
	if c.Handler.MaxEventSize <= 0 {
		return &ConfigError{Field: "Handler.MaxEventSize", Message: "must be positive"}
	}
	if c.Quota.GlobalRPS <= 0 {
		return &ConfigError{Field: "Quota.GlobalRPS", Message: "must be positive"}
	}
	if c.Quota.TenantRPS <= 0 {
		return &ConfigError{Field: "Quota.TenantRPS", Message: "must be positive"}
	}
	if c.Dedup.LocalCacheSize <= 0 {
		c.Dedup.LocalCacheSize = 100000
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

// GetJwtConfig 获取 JWT 配置（兼容旧版本）
func (c *Config) GetJwtConfig() JWTConfig {
	return c.JWT
}

// GetOidcConfig 获取 OIDC 配置（兼容旧版本）
func (c *Config) GetOidcConfig() OIDCConfig {
	return c.OIDC
}

// GetPostgresqlConfig 获取 PostgreSQL 配置（兼容旧版本）
func (c *Config) GetPostgresqlConfig() PostgresConfig {
	return c.Postgres
}
