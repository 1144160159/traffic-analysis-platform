package config

import "testing"

func TestSetDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	if cfg.Handler.MaxBatchSize <= 0 { t.Error("MaxBatchSize must be positive") }
	if cfg.Handler.MaxEventSize <= 0 { t.Error("MaxEventSize must be positive") }
	if cfg.Handler.HeartbeatInterval <= 0 { t.Error("HeartbeatInterval must be positive") }
	if cfg.Handler.ProbeStatusTimeout <= 0 { t.Error("ProbeStatusTimeout must be >0") }
}

func TestValidate(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	cfg.Kafka.Brokers = []string{"localhost:9092"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error=%v", err)
	}
}

func TestDedupConfig(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	if cfg.Dedup.LocalCacheSize <= 0 { t.Error("Dedup LocalCacheSize must be >0") }
	if cfg.Dedup.LocalTTL <= 0 { t.Error("Dedup LocalTTL must be >0") }
}
