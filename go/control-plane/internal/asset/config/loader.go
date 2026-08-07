package config

import (
	"fmt"
	"os"
	"time"

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
	if c.Export.MaxRows <= 0 || c.Export.MaxRows > 1000000 {
		return fmt.Errorf("invalid asset export max rows: %d", c.Export.MaxRows)
	}
	if c.Export.MaxBytes <= 0 || c.Export.MaxBytes > 1<<30 {
		return fmt.Errorf("invalid asset export max bytes: %d", c.Export.MaxBytes)
	}
	if c.Export.Retention <= 0 {
		return fmt.Errorf("asset export retention must be positive")
	}
	if c.Export.WorkerEnabled && (!c.Export.Enabled || c.Export.Bucket == "") {
		return fmt.Errorf("asset export worker requires enabled API and object bucket")
	}
	if c.Export.WorkerEnabled && (c.Export.S3Endpoint == "" || c.Export.S3AccessKey == "" || c.Export.S3SecretKey == "") {
		return fmt.Errorf("asset export worker requires S3 endpoint and credentials")
	}
	if c.Export.OutboxEnabled && c.Export.EventTopic == "" {
		return fmt.Errorf("asset export outbox requires an event topic")
	}
	if c.Kafka.EventOutboxEnabled && c.Kafka.EventTopic == "" {
		return fmt.Errorf("asset event outbox requires an event topic")
	}
	if c.Kafka.DiscoveryOutboxEnabled && c.Kafka.DiscoveryEventTopic == "" {
		return fmt.Errorf("asset discovery outbox requires an event topic")
	}
	if c.Detail.ClickHouseEnabled {
		if len(c.Detail.ClickHouseHosts) == 0 || c.Detail.ClickHouseDatabase == "" || c.Detail.ClickHouseUsername == "" {
			return fmt.Errorf("asset detail ClickHouse reader requires hosts, database and username")
		}
		if c.Detail.ClickHouseDial <= 0 || c.Detail.ClickHouseRead <= 0 || c.Detail.ClickHouseQuery <= 0 {
			return fmt.Errorf("asset detail ClickHouse timeouts must be positive")
		}
		if c.Detail.ClickHouseLookback <= 0 || c.Detail.ClickHouseLookback > 90*24*time.Hour {
			return fmt.Errorf("asset detail ClickHouse lookback must be within 90 days")
		}
		if c.Detail.ClickHouseAlertLimit <= 0 || c.Detail.ClickHouseAlertLimit > 500 {
			return fmt.Errorf("asset detail ClickHouse alert limit must be within 1..500")
		}
		if c.Detail.ClickHouseMaxRows == 0 || c.Detail.ClickHouseMaxBytes == 0 {
			return fmt.Errorf("asset detail ClickHouse read budgets must be positive")
		}
	}
	if c.Detail.NebulaEnabled && (c.Detail.NebulaRelationLimit <= 0 || c.Detail.NebulaRelationLimit > 500) {
		return fmt.Errorf("asset detail Nebula relation limit must be within 1..500")
	}
	if c.Detail.EvidenceEnabled {
		if !c.Detail.ClickHouseEnabled {
			return fmt.Errorf("asset detail evidence reader requires the ClickHouse detail reader")
		}
		if c.Detail.EvidenceLimit <= 0 || c.Detail.EvidenceLimit > 500 {
			return fmt.Errorf("asset detail evidence limit must be within 1..500")
		}
		if c.Export.S3Endpoint == "" || c.Export.S3AccessKey == "" || c.Export.S3SecretKey == "" {
			return fmt.Errorf("asset detail evidence reader requires S3 endpoint and credentials")
		}
	}
	if c.Kafka.ProjectionEnabled {
		if len(c.Projection.OpenSearch.Addresses) == 0 || c.Projection.OpenSearch.WriteAlias == "" {
			return fmt.Errorf("asset projection OpenSearch addresses and write alias are required")
		}
		if len(c.Projection.Nebula.Addresses) == 0 || c.Projection.Nebula.Space == "" {
			return fmt.Errorf("asset projection NebulaGraph addresses and space are required")
		}
	}
	return nil
}
