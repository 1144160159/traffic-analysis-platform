package config

import (
	"strings"
	"testing"
)

func TestWhitelistEventPipelineDefaultsFailClosed(t *testing.T) {
	t.Setenv("WHITELIST_EVENT_PIPELINE_V2_ENABLED", "false")
	t.Setenv("WHITELIST_EVENT_CONSUMER_V2_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.WhitelistEventPipelineEnabled {
		t.Fatal("whitelist event pipeline must be disabled by default")
	}
	if cfg.Kafka.WhitelistEventConsumerEnabled {
		t.Fatal("whitelist event consumer must be disabled by default")
	}
	if cfg.Kafka.WhitelistEventTopic != "whitelist.events.v2" ||
		cfg.Kafka.WhitelistEventGroup != "rule-manager-whitelist-rule-effect-v2" ||
		cfg.Kafka.WhitelistConsumerCandidateSHA256 != strings.Repeat("0", 64) ||
		cfg.Kafka.WhitelistEventContractSHA256 != "d87787272d140c8529686ce45eef30f2a6345fb7f2e918a450399c8f698aad49" {
		t.Fatalf("unexpected whitelist Kafka defaults: %+v", cfg.Kafka)
	}
}

func TestModelFeedbackRevisionConsumerDefaultsFailClosed(t *testing.T) {
	t.Setenv("MODEL_FEEDBACK_REVISION_CONSUMER_V1_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.ModelFeedbackRevisionEnabled {
		t.Fatal("model feedback revision consumer must be disabled by default")
	}
	if cfg.Kafka.ModelFeedbackRevisionTopic != "model.feedback.v1" ||
		cfg.Kafka.ModelFeedbackRevisionEventGroup != "rule-manager-model-feedback-revision-v1" {
		t.Fatalf("unexpected model feedback revision Kafka defaults: %+v", cfg.Kafka)
	}
	if cfg.Kafka.ModelFeedbackCandidateSHA256 != strings.Repeat("0", 64) ||
		cfg.Kafka.ModelFeedbackContractSHA256 != "c60bdb3ed674853da641d2c613530195a1ed9db62ce48606c402870ab283c7d9" {
		t.Fatalf("unexpected model feedback readiness bindings: %+v", cfg.Kafka)
	}
}

func TestModelRollbackV2DefaultsFailClosed(t *testing.T) {
	t.Setenv("MODEL_ROLLBACK_V2_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kafka.ModelRollbackV2Enabled {
		t.Fatal("governed model rollback writer must be disabled by default")
	}
	if cfg.Kafka.ModelRollbackAckTimeout.String() != "2m0s" {
		t.Fatalf("unexpected model rollback ACK timeout: %s", cfg.Kafka.ModelRollbackAckTimeout)
	}
}

func TestDeploymentRuntimeAckGateDefaultsFailClosed(t *testing.T) {
	t.Setenv("DEPLOYMENT_RUNTIME_ACK_GATE_V1_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Deployment.RuntimeAckGateEnabled {
		t.Fatal("deployment runtime ACK gate must be disabled by default")
	}
	if cfg.Kafka.RuleAppliedExpectedParallelism <= 0 || cfg.Kafka.ModelAppliedExpectedParallelism <= 0 {
		t.Fatalf("runtime ACK parallelism must be positive: %+v", cfg.Kafka)
	}
}
