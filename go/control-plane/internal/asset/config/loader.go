package config

import (
	"fmt"
	"os"

	"github.com/caarlos0/env/v10"
)

// Load 从环境变量加载配置（使用 env struct tags，与 config.go 保持一致）
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if cfg.Auth.JWTSigningKey == "" {
		cfg.Auth.JWTSigningKey = os.Getenv("JWT_SECRET_KEY")
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
	if c.Discovery.MaxHosts <= 0 || c.Discovery.MaxHosts > 4096 {
		return fmt.Errorf("invalid discovery max hosts: %d", c.Discovery.MaxHosts)
	}
	if c.Discovery.SchedulerEnabled && c.Discovery.TargetCIDR == "" && c.Discovery.CredentialID == "" {
		return fmt.Errorf("asset discovery scheduler requires ASSET_DISCOVERY_TARGET_CIDR or ASSET_DISCOVERY_CREDENTIAL_ID")
	}
	return nil
}
