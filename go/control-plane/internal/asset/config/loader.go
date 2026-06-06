package config

import (
	"fmt"
	"os"
	"strconv"
)

// Load 从环境变量加载配置
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			GRPCPort: getEnvInt("ASSET_GRPC_PORT", 50053),
			HTTPPort: getEnvInt("ASSET_HTTP_PORT", 8083),
		},
		Postgres: PostgresConfig{
			Host:     getEnv("ASSET_PG_HOST", "postgres-primary.databases.svc"),
			Port:     getEnvInt("ASSET_PG_PORT", 5432),
			User:     getEnv("ASSET_PG_USER", "postgres"),
			Password: getEnv("ASSET_PG_PASSWORD", "pgadmin123"),
			Database: getEnv("ASSET_PG_DB", "traffic_platform"),
			SSLMode:  getEnv("ASSET_PG_SSLMODE", "disable"),
		},
		Metrics: MetricsConfig{
			Enabled: getEnvBool("ASSET_METRICS_ENABLED", true),
			Port:    getEnvInt("ASSET_METRICS_PORT", 9094),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// DSN 返回 PostgreSQL 连接字符串
func (c PostgresConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode)
}

func (c *Config) validate() error {
	if c.Server.GRPCPort <= 0 || c.Server.GRPCPort > 65535 {
		return fmt.Errorf("invalid gRPC port: %d", c.Server.GRPCPort)
	}
	if c.Postgres.Host == "" {
		return fmt.Errorf("postgres host is required")
	}
	return nil
}

// =============================================================================
// 环境变量辅助
// =============================================================================

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
