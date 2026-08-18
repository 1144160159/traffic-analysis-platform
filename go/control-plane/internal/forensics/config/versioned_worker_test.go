package config

import (
	"testing"
	"time"
)

func TestVersionedForensicsWorkerAndWriterRemainDefaultOff(t *testing.T) {
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.Task.CompatibleWorkerReady || config.Task.WorkerEnabled || config.Task.PipelineV1Enabled {
		t.Fatalf("unsafe default task rollout: %+v", config.Task)
	}
}

func TestForensicsWriterRequiresEnabledCompatibleWorker(t *testing.T) {
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	config.Task.PipelineV1Enabled = true
	if err := config.Validate(); err == nil {
		t.Fatal("writer enabled without compatible consumer")
	}
	config.Task.CompatibleWorkerReady = true
	config.Task.WorkerEnabled = true
	config.Task.WorkerID = "pod-uid-a"
	config.Task.TaskTimeout = time.Minute
	config.Task.ExecutionLease = 2 * time.Minute
	config.S3.AccessKey = "test-access"
	config.S3.SecretKey = "test-secret"
	if err := config.Validate(); err != nil {
		t.Fatalf("valid staged worker/writer rollout rejected: %v", err)
	}
}

func TestIdleCompatibleWorkerRequiresUniqueIdentityButNoConsumption(t *testing.T) {
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	config.Task.CompatibleWorkerReady = true
	if err := config.Validate(); err == nil {
		t.Fatal("compatible worker without unique identity accepted")
	}
	config.Task.WorkerID = "pod-uid-a"
	config.Task.TaskTimeout = time.Minute
	config.Task.ExecutionLease = 2 * time.Minute
	config.S3.AccessKey = "test-access"
	config.S3.SecretKey = "test-secret"
	if err := config.Validate(); err != nil {
		t.Fatalf("idle compatible worker rejected: %v", err)
	}
	if config.Task.WorkerEnabled || config.Task.PipelineV1Enabled {
		t.Fatal("idle compatibility unexpectedly enabled consumption or writer")
	}
}
