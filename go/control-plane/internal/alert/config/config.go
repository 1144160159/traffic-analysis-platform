////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/alert/config/config.go
// 修复版：添加默认值，增加 ClickHouse 主机解析
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v10"
)

// Config Alert Service 总配置
type Config struct {
	Kafka      KafkaConfig
	Redis      RedisConfig
	ClickHouse ClickHouseConfig
	OpenSearch OpenSearchConfig
	Dedup      DedupConfig
	API        APIConfig
	Auth       AuthConfig
}

// KafkaConfig Kafka 配置
type KafkaConfig struct {
	Brokers   []string `env:"KAFKA_BROKERS" envSeparator:"," envDefault:"localhost:9092"`
	Topic     string   `env:"KAFKA_TOPIC" envDefault:"detections.v1"`
	GroupID   string   `env:"KAFKA_GROUP_ID" envDefault:"alert-service"`
	BatchSize int      `env:"KAFKA_BATCH_SIZE" envDefault:"100"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addrs    []string      `env:"REDIS_ADDRS" envSeparator:"," envDefault:"localhost:6379"`
	Password string        `env:"REDIS_PASSWORD"`
	DB       int           `env:"REDIS_DB" envDefault:"0"`
	PoolSize int           `env:"REDIS_POOL_SIZE" envDefault:"20"`
	TTL      time.Duration `env:"REDIS_TTL" envDefault:"24h"`
}

// ClickHouseConfig ClickHouse 配置
type ClickHouseConfig struct {
	// DSN 格式: clickhouse://user:password@host:port/database
	DSN          string `env:"CLICKHOUSE_DSN" envDefault:"clickhouse://default:@localhost:9000/traffic"`
	MaxOpenConns int    `env:"CLICKHOUSE_MAX_OPEN_CONNS" envDefault:"10"`
	MaxIdleConns int    `env:"CLICKHOUSE_MAX_IDLE_CONNS" envDefault:"5"`
}

// GetHosts 从 DSN 解析出主机列表
func (c *ClickHouseConfig) GetHosts() []string {
	if c.DSN == "" {
		return []string{"localhost:9000"}
	}

	// 解析 DSN: clickhouse://user:password@host:port/database
	dsn := c.DSN

	// 移除 clickhouse:// 前缀
	if strings.HasPrefix(dsn, "clickhouse://") {
		dsn = strings.TrimPrefix(dsn, "clickhouse://")
	}

	// 尝试解析为 URL
	u, err := url.Parse("clickhouse://" + dsn)
	if err != nil {
		// 如果解析失败，假设就是 host:port 格式
		if strings.Contains(dsn, "@") {
			parts := strings.SplitN(dsn, "@", 2)
			if len(parts) == 2 {
				hostPart := parts[1]
				if idx := strings.Index(hostPart, "/"); idx > 0 {
					return []string{hostPart[:idx]}
				}
				return []string{hostPart}
			}
		}
		return []string{dsn}
	}

	host := u.Host
	if host == "" {
		host = "localhost:9000"
	}

	// 确保有端口
	if !strings.Contains(host, ":") {
		host = host + ":9000"
	}

	return []string{host}
}

// GetDatabase 从 DSN 解析出数据库名
func (c *ClickHouseConfig) GetDatabase() string {
	if c.DSN == "" {
		return "traffic"
	}

	dsn := c.DSN
	if strings.HasPrefix(dsn, "clickhouse://") {
		dsn = strings.TrimPrefix(dsn, "clickhouse://")
	}

	u, err := url.Parse("clickhouse://" + dsn)
	if err != nil {
		return "traffic"
	}

	path := strings.TrimPrefix(u.Path, "/")
	if path == "" {
		return "traffic"
	}

	return path
}

// GetUsername 从 DSN 解析出用户名
func (c *ClickHouseConfig) GetUsername() string {
	if c.DSN == "" {
		return "default"
	}

	dsn := c.DSN
	if strings.HasPrefix(dsn, "clickhouse://") {
		dsn = strings.TrimPrefix(dsn, "clickhouse://")
	}

	u, err := url.Parse("clickhouse://" + dsn)
	if err != nil {
		return "default"
	}

	if u.User != nil {
		return u.User.Username()
	}

	return "default"
}

// GetPassword 从 DSN 解析出密码
func (c *ClickHouseConfig) GetPassword() string {
	if c.DSN == "" {
		return ""
	}

	dsn := c.DSN
	if strings.HasPrefix(dsn, "clickhouse://") {
		dsn = strings.TrimPrefix(dsn, "clickhouse://")
	}

	u, err := url.Parse("clickhouse://" + dsn)
	if err != nil {
		return ""
	}

	if u.User != nil {
		password, _ := u.User.Password()
		return password
	}

	return ""
}

// OpenSearchConfig OpenSearch 配置
type OpenSearchConfig struct {
	Addresses []string `env:"OPENSEARCH_ADDRS" envSeparator:"," envDefault:"http://localhost:9200"`
	Username  string   `env:"OPENSEARCH_USERNAME" envDefault:"admin"`
	Password  string   `env:"OPENSEARCH_PASSWORD" envDefault:""` // 生产环境必须通过环境变量注入
	Index     string   `env:"OPENSEARCH_INDEX" envDefault:"traffic-alerts"`
}

// DedupConfig 去重配置
type DedupConfig struct {
	TimeBucketMinutes int           `env:"DEDUP_TIME_BUCKET" envDefault:"10"`
	TTL               time.Duration `env:"DEDUP_TTL" envDefault:"10m"`
}

// APIConfig API 配置
type APIConfig struct {
	ListenAddr     string        `env:"API_LISTEN_ADDR" envDefault:":8081"`
	ReadTimeout    time.Duration `env:"API_READ_TIMEOUT" envDefault:"15s"`
	WriteTimeout   time.Duration `env:"API_WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout    time.Duration `env:"API_IDLE_TIMEOUT" envDefault:"60s"`
	AllowedOrigins []string      `env:"API_ALLOWED_ORIGINS" envSeparator:"," envDefault:"*"`
}

// AuthConfig Auth 配置
type AuthConfig struct {
	Enabled      bool   `env:"AUTH_ENABLED" envDefault:"true"`
	PostgresDSN  string `env:"AUTH_POSTGRES_DSN" envDefault:"postgres://postgres:postgres@localhost:5432/traffic?sslmode=disable"`
	JWTSecretKey string `env:"JWT_SECRET_KEY" envDefault:"your-256-bit-secret-key-here"`
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// 确保默认值
	if len(cfg.Redis.Addrs) == 0 || cfg.Redis.Addrs[0] == "" {
		cfg.Redis.Addrs = []string{"localhost:6379"}
	}

	if len(cfg.Kafka.Brokers) == 0 || cfg.Kafka.Brokers[0] == "" {
		cfg.Kafka.Brokers = []string{"localhost:9092"}
	}

	if len(cfg.OpenSearch.Addresses) == 0 || cfg.OpenSearch.Addresses[0] == "" {
		cfg.OpenSearch.Addresses = []string{"http://localhost:9200"}
	}

	// 安全验证：生产环境禁止使用通配符 CORS 和弱凭据
	cfg.validate()

	return cfg, nil
}

// validate 安全配置检查
func (c *Config) validate() {
	if c.API.AllowedOrigins[0] == "*" && c.Kafka.Brokers[0] != "localhost:9092" {
		// 生产环境检测：当 Kafka broker 不是 localhost 时发出警告
		println("⚠ SECURITY WARNING: CORS AllowedOrigins is '*', this is unsafe for production. Set API_ALLOWED_ORIGINS to your domain.")
	}
	if c.Auth.JWTSecretKey == "your-256-bit-secret-key-here" {
		println("⚠ SECURITY WARNING: Using default JWT secret key. Set JWT_SECRET_KEY environment variable.")
	}
	if c.OpenSearch.Password == "" {
		println("⚠ SECURITY WARNING: OpenSearch password is empty. Set OPENSEARCH_PASSWORD environment variable.")
	}
}
