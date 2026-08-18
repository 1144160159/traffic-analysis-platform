package config

import (
	"strings"
	"testing"
	"time"
)

func TestProjectionFlagsRemainDefaultOff(t *testing.T) {
	config := Config{}
	if err := config.validateProjection(); err != nil {
		t.Fatalf("default-off projection rejected: %v", err)
	}
}

func TestProjectionWorkerAndFeatureFlagChangeTogether(t *testing.T) {
	config := validProjectionConfig()
	config.Projection.WorkerEnabled = true
	if err := config.validateProjection(); err == nil || !strings.Contains(err.Error(), "change together") {
		t.Fatalf("worker without feature flag was not rejected: %v", err)
	}

	config = validProjectionConfig()
	config.ContinuousProjectionEnabled = true
	if err := config.validateProjection(); err == nil || !strings.Contains(err.Error(), "change together") {
		t.Fatalf("feature flag without worker was not rejected: %v", err)
	}
}

func TestProjectionConsumerCanStageBeforeWorker(t *testing.T) {
	config := validProjectionConfig()
	config.Projection.ConsumerEnabled = true
	if err := config.validateProjection(); err != nil {
		t.Fatalf("consumer-first staging rejected: %v", err)
	}
}

func validProjectionConfig() Config {
	return Config{
		Kafka:  KafkaConfig{Brokers: []string{"kafka:9092"}},
		Nebula: NebulaConfig{Enabled: true},
		Projection: ProjectionConfig{
			Topic: "graph.projections.v1", GroupID: "graph-service-projection-v1",
			DLQTopic: "dlq.v1", Lease: 30 * time.Second,
			Interval: 500 * time.Millisecond, MaxAttempts: 8,
		},
	}
}

func TestGovernedWorkbenchIsDefaultOffAndRequiresNebulaAndSigningSecret(t *testing.T) {
	config := Config{}
	if err := config.validateWorkbench(); err != nil {
		t.Fatalf("default-off workbench rejected: %v", err)
	}
	config.Workbench = WorkbenchConfig{Enabled: true, PageNodes: 100, MaxEdges: 1000, ContinuationTTL: 15 * time.Minute}
	if err := config.validateWorkbench(); err == nil || !strings.Contains(err.Error(), "requires NebulaGraph") {
		t.Fatalf("enabled workbench without NebulaGraph was not rejected: %v", err)
	}
	config.Nebula.Enabled = true
	if err := config.validateWorkbench(); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("enabled workbench without signing secret was not rejected: %v", err)
	}
	config.Workbench.ContinuationSecret = "0123456789abcdef0123456789abcdef"
	if err := config.validateWorkbench(); err != nil {
		t.Fatalf("valid governed workbench rejected: %v", err)
	}
}
