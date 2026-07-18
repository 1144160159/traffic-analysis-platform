package main

import "testing"

func TestLoadMLOpsOrchestratorConfigReadsAutomaticScope(t *testing.T) {
	t.Setenv("MLOPS_AUTOMATED_TENANT_ID", "tenant-r336")
	t.Setenv("MLOPS_AUTOMATED_MODEL_NAME", "behavior-classifier-r336")

	cfg := loadMLOpsOrchestratorConfigFromEnv()
	if cfg.AutomatedTenantID != "tenant-r336" {
		t.Fatalf("automatic tenant env was ignored: %q", cfg.AutomatedTenantID)
	}
	if cfg.AutomatedModelName != "behavior-classifier-r336" {
		t.Fatalf("automatic model env was ignored: %q", cfg.AutomatedModelName)
	}
}
