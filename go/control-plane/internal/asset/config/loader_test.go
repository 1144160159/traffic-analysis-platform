package config

import (
	"strings"
	"testing"
	"time"
)

func TestAssetExportWorkerConfigurationFailsClosedWithoutObjectCredentials(t *testing.T) {
	cfg := Config{
		Server:    ServerConfig{GRPCPort: 50053},
		Postgres:  PostgresConfig{Host: "ephemeral-postgres"},
		Discovery: DiscoveryConfig{MaxHosts: 128},
		Export: AssetExportConfig{
			Enabled: true, WorkerEnabled: true,
			MaxRows: 100, MaxBytes: 1 << 20, Retention: time.Hour,
			Bucket: "report-artifacts", S3Endpoint: "minio:9000",
		},
	}
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "S3 endpoint and credentials") {
		t.Fatalf("validate err=%v, want fail-closed S3 credential error", err)
	}
	cfg.Export.S3AccessKey = "ephemeral-access"
	cfg.Export.S3SecretKey = "ephemeral-secret"
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid isolated export worker config: %v", err)
	}
}

func TestAssetDetailClickHouseConfigurationFailsClosedOnUnboundedValues(t *testing.T) {
	base := Config{
		Server:    ServerConfig{GRPCPort: 50053},
		Postgres:  PostgresConfig{Host: "ephemeral-postgres"},
		Discovery: DiscoveryConfig{MaxHosts: 128},
		Export:    AssetExportConfig{MaxRows: 100, MaxBytes: 1 << 20, Retention: time.Hour},
		Detail: AssetDetailConfig{
			ClickHouseEnabled: true, ClickHouseHosts: []string{"clickhouse:9000"},
			ClickHouseDatabase: "traffic", ClickHouseUsername: "default",
			ClickHouseDial: time.Second, ClickHouseRead: time.Second, ClickHouseQuery: time.Second,
			ClickHouseLookback: 24 * time.Hour, ClickHouseAlertLimit: 50,
			ClickHouseMaxRows: 1000, ClickHouseMaxBytes: 1 << 20,
		},
	}
	if err := base.validate(); err != nil {
		t.Fatalf("valid bounded ClickHouse detail config: %v", err)
	}
	base.Detail.ClickHouseLookback = 91 * 24 * time.Hour
	if err := base.validate(); err == nil || !strings.Contains(err.Error(), "within 90 days") {
		t.Fatalf("lookback err=%v", err)
	}
	base.Detail.ClickHouseLookback = 24 * time.Hour
	base.Detail.ClickHouseMaxRows = 0
	if err := base.validate(); err == nil || !strings.Contains(err.Error(), "read budgets") {
		t.Fatalf("budget err=%v", err)
	}
}

func TestAssetDetailEvidenceAndNebulaConfigurationFailClosed(t *testing.T) {
	cfg := Config{
		Server:    ServerConfig{GRPCPort: 50053},
		Postgres:  PostgresConfig{Host: "ephemeral-postgres"},
		Discovery: DiscoveryConfig{MaxHosts: 128},
		Export: AssetExportConfig{
			MaxRows: 100, MaxBytes: 1 << 20, Retention: time.Hour,
			S3Endpoint: "minio:9000", S3AccessKey: "access", S3SecretKey: "secret",
		},
		Detail: AssetDetailConfig{EvidenceEnabled: true, EvidenceLimit: 10, NebulaEnabled: true, NebulaRelationLimit: 10},
	}
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "requires the ClickHouse") {
		t.Fatalf("evidence dependency err=%v", err)
	}
	cfg.Detail.ClickHouseEnabled = true
	cfg.Detail.ClickHouseHosts = []string{"clickhouse:9000"}
	cfg.Detail.ClickHouseDatabase = "traffic"
	cfg.Detail.ClickHouseUsername = "default"
	cfg.Detail.ClickHouseDial = time.Second
	cfg.Detail.ClickHouseRead = time.Second
	cfg.Detail.ClickHouseQuery = time.Second
	cfg.Detail.ClickHouseLookback = time.Hour
	cfg.Detail.ClickHouseAlertLimit = 10
	cfg.Detail.ClickHouseMaxRows = 1000
	cfg.Detail.ClickHouseMaxBytes = 1 << 20
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid cross-store detail config: %v", err)
	}
	cfg.Detail.NebulaRelationLimit = 501
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "Nebula relation limit") {
		t.Fatalf("Nebula limit err=%v", err)
	}
}
