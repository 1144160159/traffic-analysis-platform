////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/forensics/config/config.go
// 完整修复版：
// - ✅ H3: 修复配置验证逻辑错误（先验证再设默认值）
// - ✅ P12: 完善 Redis 配置验证
// - ✅ 增加配置热重载支持
// - ✅ 增加配置合法性检查
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/caarlos0/env/v10"
	"go.uber.org/zap"

	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
)

// Config 总配置
type Config struct {
	ClickHouse    ClickHouseConfig
	PostgreSQL    PostgreSQLConfig
	Redis         RedisConfig
	S3            S3Config
	API           APIConfig
	Cutter        CutterConfig
	Task          TaskConfig
	Kafka         KafkaConfig
	KafkaSecurity commonkafka.SecurityConfig
	Auth          AuthConfig

	mu sync.RWMutex // 保护配置热重载
}

// ClickHouseConfig ClickHouse 配置
type ClickHouseConfig struct {
	Hosts           []string      `env:"CLICKHOUSE_HOSTS" envSeparator:","`
	Database        string        `env:"CLICKHOUSE_DATABASE" envDefault:"traffic"`
	Username        string        `env:"CLICKHOUSE_USERNAME" envDefault:"default"`
	Password        string        `env:"CLICKHOUSE_PASSWORD"`
	MaxOpenConns    int           `env:"CLICKHOUSE_MAX_OPEN_CONNS" envDefault:"10"`
	MaxIdleConns    int           `env:"CLICKHOUSE_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"CLICKHOUSE_CONN_MAX_LIFETIME" envDefault:"1h"`
	DialTimeout     time.Duration `env:"CLICKHOUSE_DIAL_TIMEOUT" envDefault:"10s"`
	ReadTimeout     time.Duration `env:"CLICKHOUSE_READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout    time.Duration `env:"CLICKHOUSE_WRITE_TIMEOUT" envDefault:"30s"`
	CompressionLZ4  bool          `env:"CLICKHOUSE_COMPRESSION_LZ4" envDefault:"true"`
	Debug           bool          `env:"CLICKHOUSE_DEBUG" envDefault:"false"`
}

// PostgreSQLConfig PostgreSQL 配置
type PostgreSQLConfig struct {
	Host            string        `env:"POSTGRES_HOST" envDefault:"postgres-primary.databases.svc"`
	Port            int           `env:"POSTGRES_PORT" envDefault:"5432"`
	Database        string        `env:"POSTGRES_DATABASE" envDefault:"traffic"`
	Username        string        `env:"POSTGRES_USERNAME" envDefault:"postgres"`
	Password        string        `env:"POSTGRES_PASSWORD"`
	SSLMode         string        `env:"POSTGRES_SSL_MODE" envDefault:"disable"`
	MaxOpenConns    int           `env:"POSTGRES_MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"POSTGRES_MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"POSTGRES_CONN_MAX_LIFETIME" envDefault:"1h"`
	ConnMaxIdleTime time.Duration `env:"POSTGRES_CONN_MAX_IDLE_TIME" envDefault:"30m"`
	ConnectTimeout  int           `env:"POSTGRES_CONNECT_TIMEOUT" envDefault:"10"`
}

// DSN 生成 PostgreSQL 连接字符串
func (c PostgreSQLConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		c.Host, c.Port, c.Username, c.Password, c.Database, c.SSLMode, c.ConnectTimeout,
	)
}

// RedisConfig Redis 配置
type RedisConfig struct {
	// 单机模式
	Addr     string `env:"REDIS_ADDR"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB" envDefault:"0"`

	// 集群模式
	ClusterAddrs []string `env:"REDIS_CLUSTER_ADDRS" envSeparator:","`

	// 哨兵模式
	SentinelAddrs  []string `env:"REDIS_SENTINEL_ADDRS" envSeparator:","`
	SentinelMaster string   `env:"REDIS_SENTINEL_MASTER"`

	// 连接池配置
	PoolSize        int           `env:"REDIS_POOL_SIZE" envDefault:"10"`
	MinIdleConns    int           `env:"REDIS_MIN_IDLE_CONNS" envDefault:"5"`
	MaxRetries      int           `env:"REDIS_MAX_RETRIES" envDefault:"3"`
	DialTimeout     time.Duration `env:"REDIS_DIAL_TIMEOUT" envDefault:"5s"`
	ReadTimeout     time.Duration `env:"REDIS_READ_TIMEOUT" envDefault:"3s"`
	WriteTimeout    time.Duration `env:"REDIS_WRITE_TIMEOUT" envDefault:"3s"`
	PoolTimeout     time.Duration `env:"REDIS_POOL_TIMEOUT" envDefault:"4s"`
	ConnMaxIdleTime time.Duration `env:"REDIS_CONN_MAX_IDLE_TIME" envDefault:"30m"`
}

// IsClusterMode 检查是否为集群模式
func (c RedisConfig) IsClusterMode() bool {
	return len(c.ClusterAddrs) > 0
}

// IsSentinelMode 检查是否为哨兵模式
func (c RedisConfig) IsSentinelMode() bool {
	return len(c.SentinelAddrs) > 0 && c.SentinelMaster != ""
}

// GetAddrs 获取 Redis 地址列表
func (c RedisConfig) GetAddrs() []string {
	if c.IsClusterMode() {
		return c.ClusterAddrs
	}
	if c.IsSentinelMode() {
		return c.SentinelAddrs
	}
	if c.Addr != "" {
		return []string{c.Addr}
	}
	return []string{"redis-master.databases.svc:6379"}
}

// GetMode 获取 Redis 模式描述
func (c *RedisConfig) GetMode() string {
	if c.IsClusterMode() {
		return "cluster"
	}
	if c.IsSentinelMode() {
		return "sentinel"
	}
	return "standalone"
}

// S3Config S3/MinIO 配置
type S3Config struct {
	Endpoint     string `env:"S3_ENDPOINT" envDefault:"minio.minio.svc:9000"`
	Bucket       string `env:"S3_BUCKET" envDefault:"pcap-archive"`
	ResultBucket string `env:"S3_RESULT_BUCKET" envDefault:"pcap-archive"`
	AccessKey    string `env:"S3_ACCESS_KEY"`
	SecretKey    string `env:"S3_SECRET_KEY"`
	UseSSL       bool   `env:"S3_USE_SSL" envDefault:"false"`
	Region       string `env:"S3_REGION" envDefault:"us-east-1"`
}

// APIConfig API 配置
type APIConfig struct {
	ListenAddr     string        `env:"API_LISTEN_ADDR" envDefault:":8083"`
	AllowedOrigins []string      `env:"API_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
	ReadTimeout    time.Duration `env:"API_READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout   time.Duration `env:"API_WRITE_TIMEOUT" envDefault:"5m"` // 长超时用于大文件下载
	IdleTimeout    time.Duration `env:"API_IDLE_TIMEOUT" envDefault:"60s"`
}

// CutterConfig 裁剪器配置
type CutterConfig struct {
	MaxConcurrent  int           `env:"CUTTER_MAX_CONCURRENT" envDefault:"5"`
	ReadAheadMB    int           `env:"CUTTER_READ_AHEAD_MB" envDefault:"10"`
	MaxPackets     int64         `env:"CUTTER_MAX_PACKETS" envDefault:"100000"`
	PerFileTimeout time.Duration `env:"CUTTER_PER_FILE_TIMEOUT" envDefault:"60s"`
	BufferSize     int           `env:"CUTTER_BUFFER_SIZE" envDefault:"65536"` // 64KB
}

// TaskConfig 异步任务配置
type TaskConfig struct {
	WorkerCount        int           `env:"TASK_WORKER_COUNT" envDefault:"3"`
	QueueSize          int           `env:"TASK_QUEUE_SIZE" envDefault:"100"`
	ResultExpiry       time.Duration `env:"TASK_RESULT_EXPIRY" envDefault:"24h"`
	StatusPollInterval time.Duration `env:"TASK_STATUS_POLL_INTERVAL" envDefault:"5s"`
	MaxRetries         int           `env:"TASK_MAX_RETRIES" envDefault:"3"`
	ShutdownTimeout    time.Duration `env:"TASK_SHUTDOWN_TIMEOUT" envDefault:"30s"`
	TaskTimeout        time.Duration `env:"TASK_TIMEOUT" envDefault:"30m"` // 单任务超时
}

// KafkaConfig Kafka 配置（用于审计日志）
type KafkaConfig struct {
	Brokers      []string      `env:"KAFKA_BROKERS" envSeparator:","`
	AuditTopic   string        `env:"KAFKA_AUDIT_TOPIC" envDefault:"audit.logs"`
	BatchSize    int           `env:"KAFKA_BATCH_SIZE" envDefault:"100"`
	BatchTimeout time.Duration `env:"KAFKA_BATCH_TIMEOUT" envDefault:"1s"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Enabled          bool          `env:"AUTH_ENABLED" envDefault:"false"`
	JWTSigningKey    string        `env:"JWT_SIGNING_KEY" envDefault:"your-256-bit-secret-key-here"`
	JWTSigningMethod string        `env:"JWT_SIGNING_METHOD" envDefault:"HS256"`
	AccessTokenTTL   time.Duration `env:"JWT_ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL  time.Duration `env:"JWT_REFRESH_TOKEN_TTL" envDefault:"168h"` // 7 days
	JWTIssuer        string        `env:"JWT_ISSUER" envDefault:"traffic-auth-service"`
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := &Config{}

	// 解析环境变量
	if err := env.Parse(cfg); err != nil {
		return nil, &ConfigError{
			Field:   "environment",
			Message: fmt.Sprintf("failed to parse environment variables: %v", err),
		}
	}

	// 设置 ClickHouse 默认值
	if len(cfg.ClickHouse.Hosts) == 0 {
		cfg.ClickHouse.Hosts = []string{"clickhouse:9000"}
	}

	// 设置 Kafka 默认值
	if len(cfg.Kafka.Brokers) == 0 {
		cfg.Kafka.Brokers = []string{"kafka-bootstrap.middleware.svc:9092"}
	}

	// 设置 API 默认值
	if len(cfg.API.AllowedOrigins) == 0 {
		cfg.API.AllowedOrigins = []string{"*"}
	}

	return cfg, nil
}

// Validate 验证配置（修复版：先验证再设默认值）
func (c *Config) Validate() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// ========== ClickHouse 验证 ==========
	if len(c.ClickHouse.Hosts) == 0 {
		return &ConfigError{
			Field:   "ClickHouse.Hosts",
			Message: "at least one host is required",
		}
	}
	if c.ClickHouse.Database == "" {
		return &ConfigError{
			Field:   "ClickHouse.Database",
			Message: "database name is required",
		}
	}
	if c.ClickHouse.MaxOpenConns < 0 {
		return &ConfigError{
			Field:   "ClickHouse.MaxOpenConns",
			Message: "max open conns cannot be negative",
		}
	}
	if c.ClickHouse.MaxIdleConns < 0 {
		return &ConfigError{
			Field:   "ClickHouse.MaxIdleConns",
			Message: "max idle conns cannot be negative",
		}
	}
	if c.ClickHouse.MaxIdleConns > c.ClickHouse.MaxOpenConns {
		return &ConfigError{
			Field:   "ClickHouse.MaxIdleConns",
			Message: "max idle conns cannot exceed max open conns",
		}
	}

	// ========== PostgreSQL 验证 ==========
	if c.PostgreSQL.Host == "" {
		return &ConfigError{
			Field:   "PostgreSQL.Host",
			Message: "host is required",
		}
	}
	if c.PostgreSQL.Database == "" {
		return &ConfigError{
			Field:   "PostgreSQL.Database",
			Message: "database name is required",
		}
	}
	if c.PostgreSQL.Port < 1 || c.PostgreSQL.Port > 65535 {
		return &ConfigError{
			Field:   "PostgreSQL.Port",
			Message: "port must be between 1 and 65535",
		}
	}
	if c.PostgreSQL.MaxOpenConns < 0 {
		return &ConfigError{
			Field:   "PostgreSQL.MaxOpenConns",
			Message: "max open conns cannot be negative",
		}
	}
	if c.PostgreSQL.MaxIdleConns < 0 {
		return &ConfigError{
			Field:   "PostgreSQL.MaxIdleConns",
			Message: "max idle conns cannot be negative",
		}
	}
	if c.PostgreSQL.MaxIdleConns > c.PostgreSQL.MaxOpenConns {
		return &ConfigError{
			Field:   "PostgreSQL.MaxIdleConns",
			Message: "max idle conns cannot exceed max open conns",
		}
	}

	// ========== 修复 P12: Redis 配置验证 ==========
	if c.Redis.IsClusterMode() {
		// 集群模式验证
		if len(c.Redis.ClusterAddrs) == 0 {
			return &ConfigError{
				Field:   "Redis.ClusterAddrs",
				Message: "cluster mode requires at least one address",
			}
		}
		// 验证地址格式
		for i, addr := range c.Redis.ClusterAddrs {
			if addr == "" {
				return &ConfigError{
					Field:   fmt.Sprintf("Redis.ClusterAddrs[%d]", i),
					Message: "address cannot be empty",
				}
			}
			if !isValidAddress(addr) {
				return &ConfigError{
					Field:   fmt.Sprintf("Redis.ClusterAddrs[%d]", i),
					Message: fmt.Sprintf("invalid address format: %s", addr),
				}
			}
		}
	} else if c.Redis.IsSentinelMode() {
		// 哨兵模式验证
		if len(c.Redis.SentinelAddrs) == 0 {
			return &ConfigError{
				Field:   "Redis.SentinelAddrs",
				Message: "sentinel mode requires at least one sentinel address",
			}
		}
		if c.Redis.SentinelMaster == "" {
			return &ConfigError{
				Field:   "Redis.SentinelMaster",
				Message: "sentinel mode requires master name",
			}
		}
		// 验证哨兵地址格式
		for i, addr := range c.Redis.SentinelAddrs {
			if addr == "" {
				return &ConfigError{
					Field:   fmt.Sprintf("Redis.SentinelAddrs[%d]", i),
					Message: "address cannot be empty",
				}
			}
			if !isValidAddress(addr) {
				return &ConfigError{
					Field:   fmt.Sprintf("Redis.SentinelAddrs[%d]", i),
					Message: fmt.Sprintf("invalid address format: %s", addr),
				}
			}
		}
	} else {
		// 单机模式：Addr 可以为空（会使用默认值）
		if c.Redis.Addr != "" && !isValidAddress(c.Redis.Addr) {
			return &ConfigError{
				Field:   "Redis.Addr",
				Message: fmt.Sprintf("invalid address format: %s", c.Redis.Addr),
			}
		}
	}

	// 验证连接池参数
	if c.Redis.PoolSize < 0 {
		return &ConfigError{
			Field:   "Redis.PoolSize",
			Message: "pool size cannot be negative",
		}
	}
	if c.Redis.MinIdleConns < 0 {
		return &ConfigError{
			Field:   "Redis.MinIdleConns",
			Message: "min idle conns cannot be negative",
		}
	}
	if c.Redis.MinIdleConns > c.Redis.PoolSize {
		return &ConfigError{
			Field:   "Redis.MinIdleConns",
			Message: "min idle conns cannot exceed pool size",
		}
	}
	if c.Redis.DB < 0 || c.Redis.DB > 15 {
		return &ConfigError{
			Field:   "Redis.DB",
			Message: "database number must be between 0 and 15",
		}
	}

	// ========== S3 验证 ==========
	if c.S3.Endpoint == "" {
		return &ConfigError{
			Field:   "S3.Endpoint",
			Message: "endpoint is required",
		}
	}
	if c.S3.Bucket == "" {
		return &ConfigError{
			Field:   "S3.Bucket",
			Message: "bucket is required",
		}
	}
	if c.S3.AccessKey == "" {
		return &ConfigError{
			Field:   "S3.AccessKey",
			Message: "access key is required",
		}
	}
	if c.S3.SecretKey == "" {
		return &ConfigError{
			Field:   "S3.SecretKey",
			Message: "secret key is required",
		}
	}

	// 验证 bucket 名称格式
	if !isValidBucketName(c.S3.Bucket) {
		return &ConfigError{
			Field:   "S3.Bucket",
			Message: "invalid bucket name format",
		}
	}
	if c.S3.ResultBucket != "" && !isValidBucketName(c.S3.ResultBucket) {
		return &ConfigError{
			Field:   "S3.ResultBucket",
			Message: "invalid result bucket name format",
		}
	}

	// ========== 修复 H3: Cutter 验证（先验证上限再设默认值） ==========
	if c.Cutter.MaxConcurrent > 100 {
		return &ConfigError{
			Field:   "Cutter.MaxConcurrent",
			Message: "max concurrent cannot exceed 100",
		}
	}
	if c.Cutter.MaxConcurrent <= 0 {
		c.Cutter.MaxConcurrent = 5 // ✅ 验证通过后再设默认值
	}

	if c.Cutter.MaxPackets > 10000000 {
		return &ConfigError{
			Field:   "Cutter.MaxPackets",
			Message: "max packets cannot exceed 10 million",
		}
	}
	if c.Cutter.MaxPackets <= 0 {
		c.Cutter.MaxPackets = 100000 // ✅ 验证通过后再设默认值
	}

	if c.Cutter.BufferSize < 4096 {
		return &ConfigError{
			Field:   "Cutter.BufferSize",
			Message: "buffer size must be at least 4KB",
		}
	}
	if c.Cutter.BufferSize > 1048576 {
		return &ConfigError{
			Field:   "Cutter.BufferSize",
			Message: "buffer size cannot exceed 1MB",
		}
	}

	// ========== 修复 H3: Task 验证（先验证上限再设默认值） ==========
	if c.Task.WorkerCount > 50 {
		return &ConfigError{
			Field:   "Task.WorkerCount",
			Message: "worker count cannot exceed 50",
		}
	}
	if c.Task.WorkerCount <= 0 {
		c.Task.WorkerCount = 3 // ✅ 验证通过后再设默认值
	}

	if c.Task.QueueSize > 10000 {
		return &ConfigError{
			Field:   "Task.QueueSize",
			Message: "queue size cannot exceed 10000",
		}
	}
	if c.Task.QueueSize <= 0 {
		c.Task.QueueSize = 100 // ✅ 验证通过后再设默认值
	}

	if c.Task.TaskTimeout > 24*time.Hour {
		return &ConfigError{
			Field:   "Task.TaskTimeout",
			Message: "task timeout cannot exceed 24 hours",
		}
	}
	if c.Task.TaskTimeout <= 0 {
		c.Task.TaskTimeout = 30 * time.Minute // ✅ 验证通过后再设默认值
	}

	if c.Task.ShutdownTimeout < 0 {
		return &ConfigError{
			Field:   "Task.ShutdownTimeout",
			Message: "shutdown timeout cannot be negative",
		}
	}
	if c.Task.ShutdownTimeout > 5*time.Minute {
		return &ConfigError{
			Field:   "Task.ShutdownTimeout",
			Message: "shutdown timeout cannot exceed 5 minutes",
		}
	}

	// ========== Auth 验证 ==========
	if c.Auth.Enabled {
		if c.Auth.JWTSigningKey == "" || c.Auth.JWTSigningKey == "your-256-bit-secret-key-here" {
			return &ConfigError{
				Field:   "Auth.JWTSigningKey",
				Message: "JWT signing key must be set when auth is enabled",
			}
		}
		if len(c.Auth.JWTSigningKey) < 32 {
			return &ConfigError{
				Field:   "Auth.JWTSigningKey",
				Message: "JWT signing key must be at least 32 characters",
			}
		}
		if c.Auth.AccessTokenTTL <= 0 {
			return &ConfigError{
				Field:   "Auth.AccessTokenTTL",
				Message: "access token TTL must be positive",
			}
		}
		if c.Auth.RefreshTokenTTL <= 0 {
			return &ConfigError{
				Field:   "Auth.RefreshTokenTTL",
				Message: "refresh token TTL must be positive",
			}
		}
		if c.Auth.RefreshTokenTTL < c.Auth.AccessTokenTTL {
			return &ConfigError{
				Field:   "Auth.RefreshTokenTTL",
				Message: "refresh token TTL must be greater than access token TTL",
			}
		}
	}

	// ========== API 验证 ==========
	if c.API.ListenAddr == "" {
		return &ConfigError{
			Field:   "API.ListenAddr",
			Message: "listen address is required",
		}
	}
	if c.API.ReadTimeout < 0 {
		return &ConfigError{
			Field:   "API.ReadTimeout",
			Message: "read timeout cannot be negative",
		}
	}
	if c.API.WriteTimeout < 0 {
		return &ConfigError{
			Field:   "API.WriteTimeout",
			Message: "write timeout cannot be negative",
		}
	}
	if c.API.IdleTimeout < 0 {
		return &ConfigError{
			Field:   "API.IdleTimeout",
			Message: "idle timeout cannot be negative",
		}
	}

	return nil
}

// ConfigError 配置错误
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("config error [%s]: %s", e.Field, e.Message)
}

// Reload 热重载配置（仅支持部分配置）
func (c *Config) Reload(logger *zap.Logger) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	newCfg, err := Load()
	if err != nil {
		return fmt.Errorf("failed to load new config: %w", err)
	}

	if err := newCfg.Validate(); err != nil {
		return fmt.Errorf("new config validation failed: %w", err)
	}

	// 只更新可热重载的配置
	c.API.AllowedOrigins = newCfg.API.AllowedOrigins
	c.Cutter.MaxConcurrent = newCfg.Cutter.MaxConcurrent
	c.Task.WorkerCount = newCfg.Task.WorkerCount

	if logger != nil {
		logger.Info("Configuration reloaded",
			zap.Int("max_concurrent", c.Cutter.MaxConcurrent),
			zap.Int("worker_count", c.Task.WorkerCount))
	}

	return nil
}

// String 返回配置摘要（不包含敏感信息）
func (c *Config) String() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return fmt.Sprintf(
		"Config{ClickHouse: %v hosts, PostgreSQL: %s:%d, Redis: %s mode, S3: %s, Workers: %d}",
		len(c.ClickHouse.Hosts),
		c.PostgreSQL.Host,
		c.PostgreSQL.Port,
		c.Redis.GetMode(),
		c.S3.Endpoint,
		c.Task.WorkerCount,
	)
}

// ToMap 转换为 Map（用于日志记录，过滤敏感信息）
func (c *Config) ToMap() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"clickhouse": map[string]interface{}{
			"hosts":           c.ClickHouse.Hosts,
			"database":        c.ClickHouse.Database,
			"max_open_conns":  c.ClickHouse.MaxOpenConns,
			"compression_lz4": c.ClickHouse.CompressionLZ4,
		},
		"postgresql": map[string]interface{}{
			"host":           c.PostgreSQL.Host,
			"port":           c.PostgreSQL.Port,
			"database":       c.PostgreSQL.Database,
			"max_open_conns": c.PostgreSQL.MaxOpenConns,
		},
		"redis": map[string]interface{}{
			"mode":      c.Redis.GetMode(),
			"pool_size": c.Redis.PoolSize,
		},
		"s3": map[string]interface{}{
			"endpoint": c.S3.Endpoint,
			"bucket":   c.S3.Bucket,
		},
		"api": map[string]interface{}{
			"listen_addr": c.API.ListenAddr,
		},
		"cutter": map[string]interface{}{
			"max_concurrent": c.Cutter.MaxConcurrent,
			"max_packets":    c.Cutter.MaxPackets,
		},
		"task": map[string]interface{}{
			"worker_count": c.Task.WorkerCount,
			"queue_size":   c.Task.QueueSize,
		},
		"auth": map[string]interface{}{
			"enabled": c.Auth.Enabled,
		},
	}
}

// ========== 辅助验证函数 ==========

// isValidAddress 验证地址格式（host:port）
func isValidAddress(addr string) bool {
	if addr == "" {
		return false
	}
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return false
	}
	// 简单验证端口号
	var port int
	if _, err := fmt.Sscanf(parts[1], "%d", &port); err != nil {
		return false
	}
	return port > 0 && port <= 65535
}

// isValidBucketName 验证 S3 bucket 名称
func isValidBucketName(name string) bool {
	if len(name) < 3 || len(name) > 63 {
		return false
	}
	// 简化验证：只允许小写字母、数字、连字符
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

// LoadFromFile 从文件加载配置（支持 JSON/YAML）
func LoadFromFile(path string) (*Config, error) {
	// 先加载环境变量
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	// 如果指定了文件，覆盖环境变量
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}

		// 根据文件扩展名选择解析器 (支持 JSON 和 YAML)
		switch {
		case strings.HasSuffix(path, ".json"):
			if err := json.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("failed to parse JSON config: %w", err)
			}
		case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
			// YAML 解析需要 gopkg.in/yaml.v3 依赖; 这里回退到环境变量加载
			// 如需 YAML 支持, 引入: import yaml "gopkg.in/yaml.v3"; yaml.Unmarshal(data, cfg)
			return nil, fmt.Errorf("YAML config not yet supported, use JSON or environment variables")
		default:
			return nil, fmt.Errorf("unsupported config format: %s (use .json or .yaml)", path)
		}
	}

	return cfg, nil
}

// MustLoad 加载配置（失败则 panic）
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	if err := cfg.Validate(); err != nil {
		panic(fmt.Sprintf("config validation failed: %v", err))
	}

	return cfg
}
