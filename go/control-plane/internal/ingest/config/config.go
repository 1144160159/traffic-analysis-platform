package config

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	kafkaCommon "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

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

type ServerConfig struct {
	GRPCAddr              string        `env:"GRPC_ADDR" envDefault:":50051"`
	TLSCertFile           string        `env:"TLS_CERT_FILE"`
	TLSKeyFile            string        `env:"TLS_KEY_FILE"`
	TLSCAFile             string        `env:"TLS_CA_FILE"`
	MaxRecvMsgSize        int           `env:"GRPC_MAX_RECV_MSG_SIZE" envDefault:"67108864"`
	MaxSendMsgSize        int           `env:"GRPC_MAX_SEND_MSG_SIZE" envDefault:"67108864"`
	KeepaliveTime         time.Duration `env:"GRPC_KEEPALIVE_TIME" envDefault:"5m"`
	KeepaliveTimeout      time.Duration `env:"GRPC_KEEPALIVE_TIMEOUT" envDefault:"20s"`
	MaxConnectionIdle     time.Duration `env:"GRPC_MAX_CONNECTION_IDLE" envDefault:"15m"`
	MaxConnectionAge      time.Duration `env:"GRPC_MAX_CONNECTION_AGE" envDefault:"30m"`
	MaxConnectionAgeGrace time.Duration `env:"GRPC_MAX_CONNECTION_AGE_GRACE" envDefault:"5m"`
}

type HTTPConfig struct {
	Enabled        bool          `env:"HTTP_ENABLED" envDefault:"true"`
	Addr           string        `env:"HTTP_ADDR" envDefault:":8080"`
	ReadTimeout    time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout   time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout    time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"120s"`
	AllowedOrigins []string      `env:"HTTP_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
	AllowedMethods []string      `env:"HTTP_ALLOWED_METHODS" envSeparator:"," envDefault:"GET,POST,PUT,DELETE,OPTIONS"`
}

type KafkaConfig struct {
	Brokers           []string      `env:"KAFKA_BROKERS" envSeparator:","`
	FlowTopic         string        `env:"KAFKA_FLOW_TOPIC" envDefault:"flow.events.v1"`
	PcapTopic         string        `env:"KAFKA_PCAP_TOPIC" envDefault:"pcap.index.v1"`
	SessionTopic      string        `env:"KAFKA_SESSION_TOPIC" envDefault:"session.events.v1"`
	DLQTopic          string        `env:"KAFKA_DLQ_TOPIC" envDefault:"dlq.ingest-gateway"`
	BatchSize         int           `env:"KAFKA_BATCH_SIZE" envDefault:"1000"`
	BatchTimeout      time.Duration `env:"KAFKA_BATCH_TIMEOUT" envDefault:"100ms"`
	Compression       string        `env:"KAFKA_COMPRESSION" envDefault:"lz4"`
	RequiredAcks      string        `env:"KAFKA_REQUIRED_ACKS" envDefault:"all"`
	MaxRetries        int           `env:"KAFKA_MAX_RETRIES" envDefault:"3"`
	EnableIdempotence bool          `env:"KAFKA_ENABLE_IDEMPOTENCE" envDefault:"true"`
	Security          kafkaCommon.SecurityConfig
}

type RedisConfig struct {
	Addrs           []string      `env:"REDIS_ADDRS" envSeparator:","`
	Password        string        `env:"REDIS_PASSWORD"`
	DB              int           `env:"REDIS_DB" envDefault:"0"`
	SentinelAddrs   []string      `env:"REDIS_SENTINEL_ADDRS" envSeparator:","`
	SentinelMaster  string        `env:"REDIS_SENTINEL_MASTER"`
	PoolSize        int           `env:"REDIS_POOL_SIZE" envDefault:"100"`
	MinIdleConns    int           `env:"REDIS_MIN_IDLE_CONNS" envDefault:"10"`
	DialTimeout     time.Duration `env:"REDIS_DIAL_TIMEOUT" envDefault:"5s"`
	ReadTimeout     time.Duration `env:"REDIS_READ_TIMEOUT" envDefault:"3s"`
	WriteTimeout    time.Duration `env:"REDIS_WRITE_TIMEOUT" envDefault:"3s"`
	PoolTimeout     time.Duration `env:"REDIS_POOL_TIMEOUT" envDefault:"4s"`
	ConnMaxIdleTime time.Duration `env:"REDIS_CONN_MAX_IDLE_TIME" envDefault:"30m"`
}

type PostgresConfig struct {
	DSN            string        `env:"POSTGRES_DSN"`
	Host           string        `env:"POSTGRES_HOST" envDefault:"postgres-primary.databases.svc"`
	Port           int           `env:"POSTGRES_PORT" envDefault:"5432"`
	Database       string        `env:"POSTGRES_DATABASE" envDefault:"traffic_platform"`
	Username       string        `env:"POSTGRES_USERNAME" envDefault:"postgres"`
	Password       string        `env:"POSTGRES_PASSWORD"`
	SSLMode        string        `env:"POSTGRES_SSL_MODE" envDefault:"disable"`
	ConnectTimeout int           `env:"POSTGRES_CONNECT_TIMEOUT" envDefault:"10"`
	MaxOpenConns   int           `env:"POSTGRES_MAX_OPEN_CONNS" envDefault:"10"`
	MaxIdleConns   int           `env:"POSTGRES_MAX_IDLE_CONNS" envDefault:"5"`
	ConnLifetime   time.Duration `env:"POSTGRES_CONN_LIFETIME" envDefault:"1h"`
}

func (c PostgresConfig) ConnectionString() string {
	if c.DSN != "" {
		return c.DSN
	}
	if c.Host == "" || c.Database == "" || c.Username == "" {
		return ""
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	connectTimeout := c.ConnectTimeout
	if connectTimeout == 0 {
		connectTimeout = 10
	}

	dsn := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.Username, c.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, port),
		Path:   "/" + c.Database,
	}
	query := dsn.Query()
	query.Set("sslmode", sslMode)
	query.Set("connect_timeout", strconv.Itoa(connectTimeout))
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

type AuthConfig struct {
	RequireMTLS     bool          `env:"REQUIRE_MTLS" envDefault:"false"`
	AllowNoToken    bool          `env:"ALLOW_NO_TOKEN" envDefault:"false"`
	TokenTTL        time.Duration `env:"TOKEN_TTL" envDefault:"5m"`
	LocalCacheTTL   time.Duration `env:"LOCAL_CACHE_TTL" envDefault:"30s"`
	LocalCacheSize  int           `env:"LOCAL_CACHE_SIZE" envDefault:"10000"`
	DefaultTenantID string        `env:"DEFAULT_TENANT_ID" envDefault:""`
	RequireScopes   bool          `env:"REQUIRE_SCOPES" envDefault:"true"`
	RequiredScopes  []string      `env:"REQUIRED_SCOPES" envSeparator:"," envDefault:"ingest:write"`
	EnableAudit     bool          `env:"ENABLE_AUDIT" envDefault:"true"`
	EnableProbeRBAC bool          `env:"ENABLE_PROBE_RBAC" envDefault:"true"`
}

type MetricsConfig struct {
	Enabled    bool   `env:"METRICS_ENABLED" envDefault:"true"`
	ListenAddr string `env:"METRICS_ADDR" envDefault:":9090"`
}

type HandlerConfig struct {
	MaxBatchSize       int           `env:"MAX_BATCH_SIZE" envDefault:"10000"`
	MaxEventSize       int           `env:"MAX_EVENT_SIZE" envDefault:"65536"`
	StreamBufferSize   int           `env:"STREAM_BUFFER_SIZE" envDefault:"1000"`
	HeartbeatInterval  time.Duration `env:"HEARTBEAT_INTERVAL" envDefault:"30s"`
	EnableDLQ          bool          `env:"ENABLE_DLQ" envDefault:"true"`
	EnableDedup        bool          `env:"ENABLE_DEDUP" envDefault:"true"`
	ProbeStatusTimeout time.Duration `env:"PROBE_STATUS_TIMEOUT" envDefault:"5m"`
}

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

type DedupConfig struct {
	Enabled        bool          `env:"DEDUP_ENABLED" envDefault:"true"`
	LocalCacheSize int           `env:"DEDUP_LOCAL_CACHE_SIZE" envDefault:"100000"`
	LocalTTL       time.Duration `env:"DEDUP_LOCAL_TTL" envDefault:"5m"`
	RedisEnabled   bool          `env:"DEDUP_REDIS_ENABLED" envDefault:"false"`
	RedisPrefix    string        `env:"DEDUP_REDIS_PREFIX" envDefault:"dedup:"`
	RedisTTL       time.Duration `env:"DEDUP_REDIS_TTL" envDefault:"10m"`
}

type ProbeConfig struct {
	DefaultSampleRate     float32       `env:"PROBE_DEFAULT_SAMPLE_RATE" envDefault:"1.0"`
	DefaultIdleTimeout    time.Duration `env:"PROBE_DEFAULT_IDLE_TIMEOUT" envDefault:"60s"`
	DefaultActiveTimeout  time.Duration `env:"PROBE_DEFAULT_ACTIVE_TIMEOUT" envDefault:"300s"`
	DefaultBatchSize      int           `env:"PROBE_DEFAULT_BATCH_SIZE" envDefault:"1000"`
	DefaultBPFFilter      string        `env:"PROBE_DEFAULT_BPF_FILTER" envDefault:""`
	ConfigRefreshInterval time.Duration `env:"PROBE_CONFIG_REFRESH_INTERVAL" envDefault:"1m"`
	EnableDynamicConfig   bool          `env:"PROBE_ENABLE_DYNAMIC_CONFIG" envDefault:"true"`
	StatusTimeout         time.Duration `env:"PROBE_STATUS_TIMEOUT" envDefault:"5m"`
	StatusCleanupInterval time.Duration `env:"PROBE_STATUS_CLEANUP_INTERVAL" envDefault:"1m"`
}

type AuditConfig struct {
	Enabled       bool          `env:"AUDIT_ENABLED" envDefault:"true"`
	Topic         string        `env:"AUDIT_TOPIC" envDefault:"audit.logs"`
	BufferSize    int           `env:"AUDIT_BUFFER_SIZE" envDefault:"1000"`
	BatchSize     int           `env:"AUDIT_BATCH_SIZE" envDefault:"100"`
	FlushInterval time.Duration `env:"AUDIT_FLUSH_INTERVAL" envDefault:"1s"`
}

type TokenConfig struct {
	MaxTokensPerTenant int           `env:"MAX_TOKENS_PER_TENANT" envDefault:"100"`
	DefaultTTL         time.Duration `env:"DEFAULT_TOKEN_TTL" envDefault:"8760h"`
}

type OIDCConfig struct {
	Enabled      bool   `env:"OIDC_ENABLED" envDefault:"false"`
	IssuerURL    string `env:"OIDC_ISSUER_URL"`
	ClientID     string `env:"OIDC_CLIENT_ID"`
	ClientSecret string `env:"OIDC_CLIENT_SECRET"`
	RedirectURL  string `env:"OIDC_REDIRECT_URL"`
	Scopes       string `env:"OIDC_SCOPES" envDefault:"openid profile email"`
}

type JWTConfig struct {
	SigningKey      string        `env:"JWT_SIGNING_KEY" envDefault:"your-256-bit-secret-key-here"`
	SigningMethod   string        `env:"JWT_SIGNING_METHOD" envDefault:"HS256"`
	AccessTokenTTL  time.Duration `env:"JWT_ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"JWT_REFRESH_TOKEN_TTL" envDefault:"168h"`
	Issuer          string        `env:"JWT_ISSUER" envDefault:"traffic-auth-service"`
}

type APIConfig struct {
	ListenAddr     string        `env:"API_LISTEN_ADDR" envDefault:":8080"`
	ReadTimeout    time.Duration `env:"API_READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout   time.Duration `env:"API_WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout    time.Duration `env:"API_IDLE_TIMEOUT" envDefault:"120s"`
	AllowedOrigins []string      `env:"API_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
}

func (c *Config) SetDefaults() {

	if c.Kafka.FlowTopic == "" {
		c.Kafka.FlowTopic = TopicFlowEvents
	}
	if c.Kafka.SessionTopic == "" {
		c.Kafka.SessionTopic = TopicSessionEvents
	}
	if c.Kafka.PcapTopic == "" {
		c.Kafka.PcapTopic = TopicPcapIndex
	}
	if c.Kafka.DLQTopic == "" {
		c.Kafka.DLQTopic = TopicDLQ
	}
	if c.Kafka.BatchSize == 0 {
		c.Kafka.BatchSize = DefaultKafkaBatchSize
	}
	if c.Kafka.Compression == "" {
		c.Kafka.Compression = DefaultKafkaCompression
	}
	if c.Kafka.BatchTimeout == 0 {
		c.Kafka.BatchTimeout = KafkaBatchTimeout
	}
	if c.Kafka.MaxRetries == 0 {
		c.Kafka.MaxRetries = DefaultKafkaMaxRetries
	}
	if c.Kafka.RequiredAcks == "" {
		c.Kafka.RequiredAcks = KafkaRequiredAcksAll
	}

	if len(c.Redis.Addrs) == 0 {
		c.Redis.Addrs = []string{"redis-master.databases.svc:6379"}
	}
	if c.Redis.PoolSize == 0 {
		c.Redis.PoolSize = DefaultRedisPoolSize
	}
	if c.Redis.MinIdleConns == 0 {
		c.Redis.MinIdleConns = DefaultRedisMinIdleConns
	}
	if c.Redis.DialTimeout == 0 {
		c.Redis.DialTimeout = RedisDialTimeout
	}
	if c.Redis.ReadTimeout == 0 {
		c.Redis.ReadTimeout = RedisReadTimeout
	}
	if c.Redis.WriteTimeout == 0 {
		c.Redis.WriteTimeout = RedisWriteTimeout
	}
	if c.Redis.PoolTimeout == 0 {
		c.Redis.PoolTimeout = RedisPoolTimeout
	}
	if c.Redis.ConnMaxIdleTime == 0 {
		c.Redis.ConnMaxIdleTime = RedisConnMaxIdleTime
	}

	if c.Postgres.MaxOpenConns == 0 {
		c.Postgres.MaxOpenConns = 10
	}
	if c.Postgres.Host == "" {
		c.Postgres.Host = "postgres-primary.databases.svc"
	}
	if c.Postgres.Port == 0 {
		c.Postgres.Port = 5432
	}
	if c.Postgres.Database == "" {
		c.Postgres.Database = "traffic_platform"
	}
	if c.Postgres.Username == "" {
		c.Postgres.Username = "postgres"
	}
	if c.Postgres.SSLMode == "" {
		c.Postgres.SSLMode = "disable"
	}
	if c.Postgres.ConnectTimeout == 0 {
		c.Postgres.ConnectTimeout = 10
	}
	if c.Postgres.MaxIdleConns == 0 {
		c.Postgres.MaxIdleConns = 5
	}
	if c.Postgres.ConnLifetime == 0 {
		c.Postgres.ConnLifetime = PostgresConnLifetime
	}

	if c.Auth.TokenTTL == 0 {
		c.Auth.TokenTTL = DefaultTokenTTL
	}
	if c.Auth.LocalCacheTTL == 0 {
		c.Auth.LocalCacheTTL = DefaultLocalCacheTTL
	}
	if c.Auth.LocalCacheSize == 0 {
		c.Auth.LocalCacheSize = DefaultLocalCacheSize
	}
	if len(c.Auth.RequiredScopes) == 0 {
		c.Auth.RequiredScopes = []string{ScopeIngestWrite}
	}

	if c.Handler.MaxBatchSize == 0 {
		c.Handler.MaxBatchSize = DefaultMaxBatchSize
	}
	if c.Handler.MaxEventSize == 0 {
		c.Handler.MaxEventSize = DefaultMaxEventSize
	}
	if c.Handler.StreamBufferSize == 0 {
		c.Handler.StreamBufferSize = DefaultStreamBufferSize
	}
	if c.Handler.HeartbeatInterval == 0 {
		c.Handler.HeartbeatInterval = DefaultHeartbeatInterval
	}
	if c.Handler.ProbeStatusTimeout == 0 {
		c.Handler.ProbeStatusTimeout = DefaultProbeStatusTimeout
	}

	if c.Quota.RedisPrefix == "" {
		c.Quota.RedisPrefix = RedisRateLimitPrefix
	}
	if c.Quota.GlobalRPS == 0 {
		c.Quota.GlobalRPS = DefaultGlobalRPS
	}
	if c.Quota.GlobalBurst == 0 {
		c.Quota.GlobalBurst = DefaultGlobalBurst
	}
	if c.Quota.TenantRPS == 0 {
		c.Quota.TenantRPS = DefaultTenantRPS
	}
	if c.Quota.TenantBurst == 0 {
		c.Quota.TenantBurst = DefaultTenantBurst
	}
	if c.Quota.ProbeRPS == 0 {
		c.Quota.ProbeRPS = DefaultProbeRPS
	}
	if c.Quota.ProbeBurst == 0 {
		c.Quota.ProbeBurst = DefaultProbeBurst
	}

	if c.Dedup.LocalCacheSize == 0 {
		c.Dedup.LocalCacheSize = DefaultDedupLocalCacheSize
	}
	if c.Dedup.LocalTTL == 0 {
		c.Dedup.LocalTTL = DefaultDedupLocalTTL
	}
	if c.Dedup.RedisPrefix == "" {
		c.Dedup.RedisPrefix = RedisDedupPrefix
	}
	if c.Dedup.RedisTTL == 0 {
		c.Dedup.RedisTTL = DefaultDedupRedisTTL
	}

	if c.Probe.DefaultSampleRate == 0 {
		c.Probe.DefaultSampleRate = 1.0
	}
	if c.Probe.DefaultIdleTimeout == 0 {
		c.Probe.DefaultIdleTimeout = 60 * time.Second
	}
	if c.Probe.DefaultActiveTimeout == 0 {
		c.Probe.DefaultActiveTimeout = 300 * time.Second
	}
	if c.Probe.DefaultBatchSize == 0 {
		c.Probe.DefaultBatchSize = DefaultKafkaBatchSize
	}
	if c.Probe.ConfigRefreshInterval == 0 {
		c.Probe.ConfigRefreshInterval = 1 * time.Minute
	}
	if c.Probe.StatusTimeout == 0 {
		c.Probe.StatusTimeout = DefaultProbeStatusTimeout
	}
	if c.Probe.StatusCleanupInterval == 0 {
		c.Probe.StatusCleanupInterval = 1 * time.Minute
	}

	if c.Audit.Topic == "" {
		c.Audit.Topic = TopicAuditLogs
	}
	if c.Audit.BufferSize == 0 {
		c.Audit.BufferSize = DefaultAuditBufferSize
	}
	if c.Audit.BatchSize == 0 {
		c.Audit.BatchSize = DefaultAuditBatchSize
	}
	if c.Audit.FlushInterval == 0 {
		c.Audit.FlushInterval = DefaultAuditFlushInterval
	}

	if c.Server.GRPCAddr == "" {
		c.Server.GRPCAddr = DefaultGRPCAddr
	}
	if c.Server.MaxRecvMsgSize == 0 {
		c.Server.MaxRecvMsgSize = MaxRecvMsgSize
	}
	if c.Server.MaxSendMsgSize == 0 {
		c.Server.MaxSendMsgSize = MaxSendMsgSize
	}
	if c.Server.KeepaliveTime == 0 {
		c.Server.KeepaliveTime = 5 * time.Minute
	}
	if c.Server.KeepaliveTimeout == 0 {
		c.Server.KeepaliveTimeout = 20 * time.Second
	}

	if c.Metrics.ListenAddr == "" {
		c.Metrics.ListenAddr = DefaultMetricsAddr
	}

	if c.HTTP.Addr == "" {
		c.HTTP.Addr = DefaultHTTPAddr
	}
	if c.HTTP.ReadTimeout == 0 {
		c.HTTP.ReadTimeout = HTTPRequestTimeout
	}
	if c.HTTP.WriteTimeout == 0 {
		c.HTTP.WriteTimeout = HTTPRequestTimeout
	}

	if c.Token.DefaultTTL == 0 {
		c.Token.DefaultTTL = 8760 * time.Hour
	}
	if c.Token.MaxTokensPerTenant == 0 {
		c.Token.MaxTokensPerTenant = 100
	}

	if c.JWT.AccessTokenTTL == 0 {
		c.JWT.AccessTokenTTL = 15 * time.Minute
	}
	if c.JWT.RefreshTokenTTL == 0 {
		c.JWT.RefreshTokenTTL = 168 * time.Hour
	}
}

func (c *Config) Validate() error {

	if len(c.Kafka.Brokers) == 0 {
		return &ConfigError{Field: "Kafka.Brokers", Message: "at least one broker required"}
	}
	if c.Kafka.FlowTopic == "" {
		return &ConfigError{Field: "Kafka.FlowTopic", Message: "flow topic required"}
	}
	if c.Kafka.SessionTopic == "" {
		return &ConfigError{Field: "Kafka.SessionTopic", Message: "session topic required"}
	}
	if c.Kafka.PcapTopic == "" {
		return &ConfigError{Field: "Kafka.PcapTopic", Message: "pcap topic required"}
	}
	if c.Kafka.BatchSize <= 0 || c.Kafka.BatchSize > 100000 {
		return &ConfigError{Field: "Kafka.BatchSize", Message: "must be between 1 and 100000"}
	}

	if c.Handler.MaxBatchSize <= 0 {
		return &ConfigError{Field: "Handler.MaxBatchSize", Message: "must be positive"}
	}
	if c.Handler.MaxEventSize <= 0 {
		return &ConfigError{Field: "Handler.MaxEventSize", Message: "must be positive"}
	}
	if c.Handler.MaxEventSize > MaxRecvMsgSize {
		return &ConfigError{Field: "Handler.MaxEventSize",
			Message: fmt.Sprintf("exceeds max recv msg size %d", MaxRecvMsgSize)}
	}

	if c.Quota.GlobalRPS <= 0 {
		return &ConfigError{Field: "Quota.GlobalRPS", Message: "must be positive"}
	}
	if c.Quota.TenantRPS <= 0 {
		return &ConfigError{Field: "Quota.TenantRPS", Message: "must be positive"}
	}
	if c.Quota.ProbeRPS <= 0 {
		return &ConfigError{Field: "Quota.ProbeRPS", Message: "must be positive"}
	}
	if c.Quota.TenantRPS > c.Quota.GlobalRPS {
		return &ConfigError{Field: "Quota.TenantRPS", Message: "cannot exceed global RPS"}
	}

	if c.Dedup.Enabled && c.Dedup.LocalCacheSize <= 0 {
		return &ConfigError{Field: "Dedup.LocalCacheSize", Message: "must be positive when dedup enabled"}
	}

	if c.Auth.RequireScopes && len(c.Auth.RequiredScopes) == 0 {
		return &ConfigError{Field: "Auth.RequiredScopes", Message: "at least one scope required"}
	}

	if c.Auth.RequireMTLS {
		if c.Server.TLSCertFile == "" {
			return &ConfigError{Field: "Server.TLSCertFile", Message: "required when mTLS enabled"}
		}
		if c.Server.TLSKeyFile == "" {
			return &ConfigError{Field: "Server.TLSKeyFile", Message: "required when mTLS enabled"}
		}
		if c.Server.TLSCAFile == "" {
			return &ConfigError{Field: "Server.TLSCAFile", Message: "required when mTLS enabled"}
		}
	}

	return nil
}

type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config validation error [%s]: %s", e.Field, e.Message)
}

func (c *Config) GetJwtConfig() JWTConfig {
	return c.JWT
}

func (c *Config) GetOidcConfig() OIDCConfig {
	return c.OIDC
}

func (c *Config) GetPostgresqlConfig() PostgresConfig {
	return c.Postgres
}
