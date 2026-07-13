////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/config/config.go
// 完整修复版 v4：
// 1. 修复 #9：JWT 签名密钥强制验证（生产环境禁止默认密钥）
// 2. 修复 #17：JWT 密钥默认值检测（增强安全检查）
// 3. 修复 #18：CORS 生产环境 HTTP 检查
// 4. 增强配置验证逻辑
// 5. 完善错误提示信息
////////////////////////////////////////////////////////////////////////////////

package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v10"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
	commonkafka "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/kafka"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/storage"
)

// Config 应用配置
type Config struct {
	Server        ServerConfig     `envPrefix:"SERVER_"`
	PostgreSQL    PostgreSQLConfig `envPrefix:"POSTGRES_"`
	Redis         RedisConfig      `envPrefix:"REDIS_"`
	JWT           JWTConfig        `envPrefix:"JWT_"`
	OIDC          OIDCConfig       `envPrefix:"OIDC_"`
	Token         TokenConfig      `envPrefix:"TOKEN_"`
	Audit         AuditConfig      `envPrefix:"AUDIT_"`
	Kafka         KafkaConfig      `envPrefix:"KAFKA_"`
	KafkaSecurity commonkafka.SecurityConfig
	API           APIConfig      `envPrefix:"API_"`
	OTEL          OTELConfig     `envPrefix:"OTEL_"`
	Security      SecurityConfig `envPrefix:"SECURITY_"`
}

// ServerConfig HTTP 服务器配置
type ServerConfig struct {
	ListenAddr        string        `env:"LISTEN_ADDR" envDefault:":8080"`
	ReadTimeout       time.Duration `env:"READ_TIMEOUT" envDefault:"30s"`
	WriteTimeout      time.Duration `env:"WRITE_TIMEOUT" envDefault:"30s"`
	IdleTimeout       time.Duration `env:"IDLE_TIMEOUT" envDefault:"120s"`
	ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" envDefault:"5s"`
	MaxHeaderBytes    int           `env:"MAX_HEADER_BYTES" envDefault:"1048576"`
	ShutdownTimeout   time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`

	TLSEnabled  bool   `env:"TLS_ENABLED" envDefault:"false"`
	TLSCertFile string `env:"TLS_CERT_FILE"`
	TLSKeyFile  string `env:"TLS_KEY_FILE"`
	TLSCAFile   string `env:"TLS_CA_FILE"`
}

// PostgreSQLConfig PostgreSQL 配置
type PostgreSQLConfig struct {
	Host            string        `env:"HOST" envDefault:"postgres-primary.databases.svc"`
	Port            int           `env:"PORT" envDefault:"5432"`
	Database        string        `env:"DATABASE" envDefault:"traffic"`
	Username        string        `env:"USERNAME" envDefault:"postgres"`
	Password        string        `env:"PASSWORD"`
	SSLMode         string        `env:"SSL_MODE" envDefault:"disable"`
	MaxOpenConns    int           `env:"MAX_OPEN_CONNS" envDefault:"25"`
	MaxIdleConns    int           `env:"MAX_IDLE_CONNS" envDefault:"5"`
	ConnMaxLifetime time.Duration `env:"CONN_MAX_LIFETIME" envDefault:"1h"`
	ConnMaxIdleTime time.Duration `env:"CONN_MAX_IDLE_TIME" envDefault:"30m"`
	ConnectTimeout  int           `env:"CONNECT_TIMEOUT" envDefault:"10"`
}

func (c PostgreSQLConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		c.Host, c.Port, c.Username, c.Password, c.Database, c.SSLMode, c.ConnectTimeout,
	)
}

func (c PostgreSQLConfig) ToStorageConfig() storage.PostgresConfig {
	return storage.PostgresConfig{
		Host:            c.Host,
		Port:            c.Port,
		Database:        c.Database,
		Username:        c.Username,
		Password:        c.Password,
		SSLMode:         c.SSLMode,
		MaxOpenConns:    c.MaxOpenConns,
		MaxIdleConns:    c.MaxIdleConns,
		ConnMaxLifetime: c.ConnMaxLifetime,
		ConnMaxIdleTime: c.ConnMaxIdleTime,
		ConnectTimeout:  c.ConnectTimeout,
	}
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Addrs           []string      `env:"ADDRS" envSeparator:","`
	Addr            string        `env:"ADDR"`
	Password        string        `env:"PASSWORD"`
	DB              int           `env:"DB" envDefault:"0"`
	SentinelAddrs   []string      `env:"SENTINEL_ADDRS" envSeparator:","`
	SentinelMaster  string        `env:"SENTINEL_MASTER"`
	PoolSize        int           `env:"POOL_SIZE" envDefault:"10"`
	MinIdleConns    int           `env:"MIN_IDLE_CONNS" envDefault:"5"`
	MaxRetries      int           `env:"MAX_RETRIES" envDefault:"3"`
	DialTimeout     time.Duration `env:"DIAL_TIMEOUT" envDefault:"5s"`
	ReadTimeout     time.Duration `env:"READ_TIMEOUT" envDefault:"3s"`
	WriteTimeout    time.Duration `env:"WRITE_TIMEOUT" envDefault:"3s"`
	PoolTimeout     time.Duration `env:"POOL_TIMEOUT" envDefault:"4s"`
	ConnMaxIdleTime time.Duration `env:"CONN_MAX_IDLE_TIME" envDefault:"30m"`
	Enabled         bool          `env:"ENABLED" envDefault:"true"`
}

func (c RedisConfig) IsConfigured() bool {
	return c.Enabled && (c.Addr != "" || len(c.Addrs) > 0 || len(c.SentinelAddrs) > 0)
}

func (c RedisConfig) ToStorageConfig() storage.RedisConfig {
	return storage.RedisConfig{
		Addr:            c.Addr,
		ClusterAddrs:    c.Addrs,
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

// JWTConfig JWT 配置
type JWTConfig struct {
	SigningKey      string        `env:"SIGNING_KEY" envDefault:"change-me-in-production-must-be-at-least-32-bytes-long"`
	SigningMethod   string        `env:"SIGNING_METHOD" envDefault:"HS256"`
	AccessTokenTTL  time.Duration `env:"ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL time.Duration `env:"REFRESH_TOKEN_TTL" envDefault:"168h"`
	Issuer          string        `env:"ISSUER" envDefault:"traffic-analysis-platform"`
	Audience        []string      `env:"AUDIENCE" envSeparator:","`
	ValidateExpiry  bool          `env:"VALIDATE_EXPIRY" envDefault:"true"`
	ClockSkew       time.Duration `env:"CLOCK_SKEW" envDefault:"30s"`
	PrivateKeyFile  string        `env:"PRIVATE_KEY_FILE"`
	PublicKeyFile   string        `env:"PUBLIC_KEY_FILE"`
}

// OIDCConfig OIDC 配置
type OIDCConfig struct {
	Enabled      bool   `env:"ENABLED" envDefault:"false"`
	IssuerURL    string `env:"ISSUER_URL"`
	ClientID     string `env:"CLIENT_ID"`
	ClientSecret string `env:"CLIENT_SECRET"`
	RedirectURL  string `env:"REDIRECT_URL"`
	Scopes       string `env:"SCOPES" envDefault:"openid,profile,email"`
}

// TokenConfig API Token 配置
type TokenConfig struct {
	MaxTokensPerTenant  int           `env:"MAX_TOKENS_PER_TENANT" envDefault:"100"`
	DefaultTTL          time.Duration `env:"DEFAULT_TTL" envDefault:"8760h"`
	EnableUserTokens    bool          `env:"ENABLE_USER_TOKENS" envDefault:"true"`
	EnableAPITokens     bool          `env:"ENABLE_API_TOKENS" envDefault:"true"`
	EnableProbeTokens   bool          `env:"ENABLE_PROBE_TOKENS" envDefault:"true"`
	EnableRotation      bool          `env:"ENABLE_ROTATION" envDefault:"false"`
	RotationInterval    time.Duration `env:"ROTATION_INTERVAL" envDefault:"720h"`
	RotationGracePeriod time.Duration `env:"ROTATION_GRACE_PERIOD" envDefault:"168h"`
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

// APIConfig API 配置
type APIConfig struct {
	AllowedOrigins     []string `env:"ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`
	RateLimitEnabled   bool     `env:"RATE_LIMIT_ENABLED" envDefault:"true"`
	RateLimitRPS       int      `env:"RATE_LIMIT_RPS" envDefault:"100"`
	MaxRequestBodySize int64    `env:"MAX_REQUEST_BODY_SIZE" envDefault:"1048576"`
}

// OTELConfig OpenTelemetry 配置
type OTELConfig struct {
	Enabled        bool    `env:"ENABLED" envDefault:"true"`
	ServiceName    string  `env:"SERVICE_NAME" envDefault:"auth-service"`
	ServiceVersion string  `env:"SERVICE_VERSION" envDefault:"1.0.0"`
	Environment    string  `env:"ENVIRONMENT" envDefault:"development"`
	Endpoint       string  `env:"ENDPOINT" envDefault:"victoria-metrics.observability.svc:4317"`
	Insecure       bool    `env:"INSECURE" envDefault:"true"`
	SampleRate     float64 `env:"SAMPLE_RATE" envDefault:"1.0"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	MinPasswordLength    int           `env:"MIN_PASSWORD_LENGTH" envDefault:"12"`
	RequireUppercase     bool          `env:"REQUIRE_UPPERCASE" envDefault:"true"`
	RequireLowercase     bool          `env:"REQUIRE_LOWERCASE" envDefault:"true"`
	RequireDigit         bool          `env:"REQUIRE_DIGIT" envDefault:"true"`
	RequireSpecial       bool          `env:"REQUIRE_SPECIAL" envDefault:"true"`
	BcryptCost           int           `env:"BCRYPT_COST" envDefault:"12"`
	MaxLoginAttempts     int           `env:"MAX_LOGIN_ATTEMPTS" envDefault:"5"`
	LockoutDuration      time.Duration `env:"LOCKOUT_DURATION" envDefault:"15m"`
	SessionTimeout       time.Duration `env:"SESSION_TIMEOUT" envDefault:"24h"`
	SessionRefreshWindow time.Duration `env:"SESSION_REFRESH_WINDOW" envDefault:"1h"`
}

// Load 加载配置
func Load() (*Config, error) {
	cfg := &Config{}

	if err := env.Parse(cfg); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigError, "Failed to parse environment variables")
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate 验证配置（修复 #9、#17、#18）
func (c *Config) Validate() error {
	if err := c.validateJWT(); err != nil {
		return err
	}

	if err := c.validatePostgreSQL(); err != nil {
		return err
	}

	if err := c.validateRedis(); err != nil {
		return err
	}

	if err := c.validateToken(); err != nil {
		return err
	}

	if err := c.validateOIDC(); err != nil {
		return err
	}

	if err := c.validateAPI(); err != nil {
		return err
	}

	if err := c.validateTLS(); err != nil {
		return err
	}

	return nil
}

// validateJWT 验证 JWT 配置（修复 #9、#17：强制检查默认密钥）
func (c *Config) validateJWT() error {
	// 修复 #17：扩展默认密钥黑名单
	defaultKeys := []string{
		"change-me-in-production-must-be-at-least-32-bytes-long",
		"your-256-bit-secret-key-here",
		"your-secret-key",
		"secret",
		"changeme",
		"change_me",
		"password",
		"12345678",
		"secretkey",
		"jwt-secret",
		"jwt_secret",
		"my-secret-key",
	}

	// 修复 #9、#17：生产环境强制检查默认密钥
	if c.IsProduction() {
		keyLower := strings.ToLower(c.JWT.SigningKey)

		// 检查是否是黑名单中的默认密钥
		for _, dk := range defaultKeys {
			if keyLower == strings.ToLower(dk) || c.JWT.SigningKey == dk {
				return errors.Newf(errors.ErrCodeConfigError,
					"SECURITY CRITICAL: Default JWT signing key detected in production environment. "+
						"Key: '%s'. You MUST set a secure random key via JWT_SIGNING_KEY environment variable. "+
						"Generate one with: openssl rand -base64 64",
					c.JWT.SigningKey)
			}
		}

		// 修复 #17：检查密钥是否包含明显提示词
		suspiciousWords := []string{"change", "example", "test", "demo", "sample", "default"}
		for _, word := range suspiciousWords {
			if strings.Contains(keyLower, word) {
				return errors.Newf(errors.ErrCodeConfigError,
					"SECURITY WARNING: JWT signing key contains suspicious word '%s' in production. "+
						"Please use a cryptographically random key generated with: openssl rand -base64 64",
					word)
			}
		}

		// 修复 #9：生产环境密钥长度必须 >= 64 字节
		if len(c.JWT.SigningKey) < 64 {
			return errors.Newf(errors.ErrCodeConfigError,
				"SECURITY WARNING: JWT signing key too short in production (%d bytes). "+
					"Recommended minimum: 64 bytes for strong security. "+
					"Generate with: openssl rand -base64 64",
				len(c.JWT.SigningKey))
		}
	} else {
		// 开发环境：如果使用默认密钥，生成临时随机密钥
		if c.JWT.SigningKey == "" || c.JWT.SigningKey == defaultKeys[0] {
			randomKey, err := generateRandomKey(64)
			if err == nil {
				c.JWT.SigningKey = randomKey
				// 注意：不记录实际密钥到日志
				fmt.Println("WARNING: Using auto-generated JWT signing key (development only)")
			}
		}
	}

	// 验证签名方法对应的密钥长度
	if c.JWT.SigningMethod == "HS256" || c.JWT.SigningMethod == "HS384" || c.JWT.SigningMethod == "HS512" {
		minLength := 32
		if c.JWT.SigningMethod == "HS384" {
			minLength = 48
		} else if c.JWT.SigningMethod == "HS512" {
			minLength = 64
		}

		if len(c.JWT.SigningKey) < minLength {
			return errors.Newf(errors.ErrCodeConfigError,
				"JWT signing key must be at least %d bytes for %s, got %d bytes",
				minLength, c.JWT.SigningMethod, len(c.JWT.SigningKey))
		}
	}

	// 验证 RSA/ECDSA 密钥文件
	if strings.HasPrefix(c.JWT.SigningMethod, "RS") || strings.HasPrefix(c.JWT.SigningMethod, "ES") {
		if c.JWT.PrivateKeyFile == "" || c.JWT.PublicKeyFile == "" {
			return errors.New(errors.ErrCodeConfigError,
				"RSA/ECDSA signing requires both private and public key files")
		}
	}

	// 验证 TTL 合理性
	if c.JWT.AccessTokenTTL < time.Minute {
		return errors.Newf(errors.ErrCodeConfigError,
			"Access token TTL too short: %v (minimum 1 minute)", c.JWT.AccessTokenTTL)
	}
	if c.JWT.AccessTokenTTL > 24*time.Hour {
		return errors.Newf(errors.ErrCodeConfigError,
			"Access token TTL too long: %v (maximum 24 hours)", c.JWT.AccessTokenTTL)
	}

	if c.JWT.RefreshTokenTTL < time.Hour {
		return errors.Newf(errors.ErrCodeConfigError,
			"Refresh token TTL too short: %v (minimum 1 hour)", c.JWT.RefreshTokenTTL)
	}

	if c.JWT.RefreshTokenTTL <= c.JWT.AccessTokenTTL {
		return errors.New(errors.ErrCodeConfigError,
			"Refresh token TTL must be longer than access token TTL")
	}

	// 验证 Issuer
	if c.JWT.Issuer == "" {
		return errors.New(errors.ErrCodeConfigError, "JWT issuer is required")
	}

	return nil
}

// validatePostgreSQL 验证 PostgreSQL 配置
func (c *Config) validatePostgreSQL() error {
	if c.PostgreSQL.Host == "" {
		return errors.New(errors.ErrCodeConfigError, "PostgreSQL host is required")
	}

	if c.PostgreSQL.Database == "" {
		return errors.New(errors.ErrCodeConfigError, "PostgreSQL database is required")
	}

	if c.PostgreSQL.Username == "" {
		return errors.New(errors.ErrCodeConfigError, "PostgreSQL username is required")
	}

	if c.PostgreSQL.MaxOpenConns < 1 {
		return errors.New(errors.ErrCodeConfigError, "PostgreSQL max open connections must be >= 1")
	}

	if c.PostgreSQL.MaxIdleConns > c.PostgreSQL.MaxOpenConns {
		return errors.New(errors.ErrCodeConfigError,
			"PostgreSQL max idle connections cannot exceed max open connections")
	}

	return nil
}

// validateRedis 验证 Redis 配置
func (c *Config) validateRedis() error {
	if !c.Redis.Enabled {
		return nil
	}

	if c.Redis.Addr == "" && len(c.Redis.Addrs) == 0 && len(c.Redis.SentinelAddrs) == 0 {
		return errors.New(errors.ErrCodeConfigError,
			"Redis address (REDIS_ADDR, REDIS_ADDRS, or REDIS_SENTINEL_ADDRS) is required when Redis is enabled")
	}

	if c.Redis.Addr != "" && len(c.Redis.Addrs) > 0 {
		return errors.New(errors.ErrCodeConfigError,
			"Cannot specify both REDIS_ADDR and REDIS_ADDRS")
	}

	if len(c.Redis.SentinelAddrs) > 0 && c.Redis.SentinelMaster == "" {
		return errors.New(errors.ErrCodeConfigError,
			"REDIS_SENTINEL_MASTER is required when REDIS_SENTINEL_ADDRS is set")
	}

	if c.Redis.PoolSize < 1 {
		return errors.New(errors.ErrCodeConfigError, "Redis pool size must be >= 1")
	}

	return nil
}

// validateToken 验证 Token 配置
func (c *Config) validateToken() error {
	if c.Token.MaxTokensPerTenant < 1 {
		return errors.New(errors.ErrCodeConfigError, "Max tokens per tenant must be >= 1")
	}

	if c.Token.MaxTokensPerTenant > 10000 {
		return errors.New(errors.ErrCodeConfigError,
			"Max tokens per tenant too large (maximum 10000)")
	}

	if c.Token.EnableRotation {
		if c.Token.RotationInterval < 24*time.Hour {
			return errors.New(errors.ErrCodeConfigError,
				"Token rotation interval must be at least 24 hours")
		}

		if c.Token.RotationGracePeriod < time.Hour {
			return errors.New(errors.ErrCodeConfigError,
				"Token rotation grace period must be at least 1 hour")
		}

		if c.Token.RotationGracePeriod >= c.Token.RotationInterval {
			return errors.New(errors.ErrCodeConfigError,
				"Token rotation grace period must be shorter than rotation interval")
		}
	}

	return nil
}

// validateOIDC 验证 OIDC 配置
func (c *Config) validateOIDC() error {
	if !c.OIDC.Enabled {
		return nil
	}

	if c.OIDC.IssuerURL == "" {
		return errors.New(errors.ErrCodeConfigError, "OIDC issuer URL is required when OIDC is enabled")
	}

	if c.OIDC.ClientID == "" {
		return errors.New(errors.ErrCodeConfigError, "OIDC client ID is required when OIDC is enabled")
	}

	if c.OIDC.ClientSecret == "" {
		return errors.New(errors.ErrCodeConfigError, "OIDC client secret is required when OIDC is enabled")
	}

	if c.OIDC.RedirectURL == "" {
		return errors.New(errors.ErrCodeConfigError, "OIDC redirect URL is required when OIDC is enabled")
	}

	return nil
}

// validateAPI 验证 API 配置（修复 #18：CORS 安全检查）
func (c *Config) validateAPI() error {
	if len(c.API.AllowedOrigins) == 0 {
		return errors.New(errors.ErrCodeConfigError, "At least one allowed origin is required")
	}

	hasWildcard := false
	hasSpecific := false
	for _, origin := range c.API.AllowedOrigins {
		if origin == "*" {
			hasWildcard = true
		} else {
			hasSpecific = true
		}
	}

	if hasWildcard && hasSpecific {
		return errors.New(errors.ErrCodeConfigError,
			"Cannot mix wildcard (*) with specific origins in CORS configuration")
	}

	// 生产环境不允许通配符
	if c.IsProduction() && hasWildcard {
		return errors.New(errors.ErrCodeConfigError,
			"SECURITY: Wildcard CORS origin (*) is not allowed in production")
	}

	// 修复 #18：生产环境强制 HTTPS（除 localhost 外）
	if c.IsProduction() {
		for _, origin := range c.API.AllowedOrigins {
			if origin == "*" {
				continue // 已在上面检查过
			}

			// 检查是否使用 HTTP（非 localhost）
			if strings.HasPrefix(origin, "http://") {
				// 允许 localhost 和 127.0.0.1（用于本地开发/测试）
				if !strings.Contains(origin, "localhost") && !strings.Contains(origin, "127.0.0.1") {
					return errors.Newf(errors.ErrCodeConfigError,
						"SECURITY: HTTP origin not allowed in production (use HTTPS): %s", origin)
				}
			}

			// 确保包含 scheme
			if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
				return errors.Newf(errors.ErrCodeConfigError,
					"CORS origin must include scheme (http:// or https://): %s", origin)
			}
		}
	}

	if c.API.RateLimitEnabled && c.API.RateLimitRPS < 1 {
		return errors.New(errors.ErrCodeConfigError, "Rate limit RPS must be >= 1")
	}

	return nil
}

// validateTLS 验证 TLS 配置
func (c *Config) validateTLS() error {
	if !c.Server.TLSEnabled {
		return nil
	}

	if c.Server.TLSCertFile == "" {
		return errors.New(errors.ErrCodeConfigError, "TLS cert file is required when TLS is enabled")
	}

	if c.Server.TLSKeyFile == "" {
		return errors.New(errors.ErrCodeConfigError, "TLS key file is required when TLS is enabled")
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

// ShouldEnableDebugLogging 是否应该启用调试日志
func (c *Config) ShouldEnableDebugLogging() bool {
	return c.IsDevelopment()
}

// generateRandomKey 生成随机密钥（修复 #17：辅助函数）
func generateRandomKey(length int) (string, error) {
	if length < 32 {
		length = 32
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(bytes), nil
}
