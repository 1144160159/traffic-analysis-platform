package config

import "testing"

func TestWhitelistEventPipelineDefaultsFailClosed(t *testing.T) {
	t.Setenv("WHITELIST_EVENT_PIPELINE_V2_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.WhitelistEventPipelineEnabled {
		t.Fatal("whitelist event pipeline must be disabled by default")
	}
	if cfg.Kafka.WhitelistEventTopic != "whitelist.events.v2" ||
		cfg.Kafka.WhitelistEventGroup != "rule-manager-whitelist-rule-effect-v2" {
		t.Fatalf("unexpected whitelist Kafka defaults: %+v", cfg.Kafka)
	}
}
