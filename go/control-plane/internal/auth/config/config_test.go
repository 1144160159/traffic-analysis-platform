package config

import (
	"testing"

	"github.com/caarlos0/env/v10"
)

func TestKafkaSecurityEnvironmentIsParsed(t *testing.T) {
	t.Setenv("KAFKA_SECURITY_PROTOCOL", "SASL_SSL")
	t.Setenv("KAFKA_SASL_MECHANISM", "SCRAM-SHA-512")
	t.Setenv("KAFKA_SASL_USERNAME", "audit-client")
	t.Setenv("KAFKA_SASL_PASSWORD", "audit-secret")
	t.Setenv("KAFKA_TLS_CA_FILE", "/etc/kafka/tls/ca.crt")
	t.Setenv("KAFKA_TLS_SERVER_NAME", "kafka-bootstrap.middleware.svc")

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.KafkaSecurity.SecurityProtocol != "SASL_SSL" || cfg.KafkaSecurity.SASLUsername != "audit-client" {
		t.Fatalf("Kafka security environment was not parsed: %#v", cfg.KafkaSecurity)
	}
	if cfg.KafkaSecurity.TLSCAFile != "/etc/kafka/tls/ca.crt" || cfg.KafkaSecurity.TLSServerName != "kafka-bootstrap.middleware.svc" {
		t.Fatalf("Kafka TLS environment was not parsed: %#v", cfg.KafkaSecurity)
	}
}

func TestRedisConfigToStorageConfigUsesStandaloneAddr(t *testing.T) {
	cfg := RedisConfig{
		Addr: "redis-master.databases.svc:6379",
	}

	storageCfg := cfg.ToStorageConfig()
	if storageCfg.Addr != cfg.Addr {
		t.Fatalf("expected standalone Redis addr %q, got %q", cfg.Addr, storageCfg.Addr)
	}
	if len(storageCfg.ClusterAddrs) != 0 {
		t.Fatalf("expected REDIS_ADDR to use standalone mode, got cluster addrs %v", storageCfg.ClusterAddrs)
	}
}

func TestRedisConfigToStorageConfigUsesClusterAddrs(t *testing.T) {
	cfg := RedisConfig{
		Addrs: []string{
			"redis-cluster-0.databases.svc:6379",
			"redis-cluster-1.databases.svc:6379",
		},
	}

	storageCfg := cfg.ToStorageConfig()
	if storageCfg.Addr != "" {
		t.Fatalf("expected no standalone addr for REDIS_ADDRS, got %q", storageCfg.Addr)
	}
	if len(storageCfg.ClusterAddrs) != len(cfg.Addrs) {
		t.Fatalf("expected cluster addrs %v, got %v", cfg.Addrs, storageCfg.ClusterAddrs)
	}
}

func TestRedisConfigToStorageConfigUsesSentinel(t *testing.T) {
	cfg := RedisConfig{
		SentinelAddrs:  []string{"redis-sentinel.databases.svc:26379"},
		SentinelMaster: "mymaster",
	}

	storageCfg := cfg.ToStorageConfig()
	if len(storageCfg.SentinelAddrs) != len(cfg.SentinelAddrs) {
		t.Fatalf("expected sentinel addrs %v, got %v", cfg.SentinelAddrs, storageCfg.SentinelAddrs)
	}
	if storageCfg.SentinelMaster != cfg.SentinelMaster {
		t.Fatalf("expected sentinel master %q, got %q", cfg.SentinelMaster, storageCfg.SentinelMaster)
	}
}
