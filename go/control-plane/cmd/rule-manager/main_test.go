package main

import "testing"

func TestLoadMLOpsOrchestratorConfigReadsAutomaticScope(t *testing.T) {
	t.Setenv("MLOPS_AUTOMATIC_CANDIDATE_V1_ENABLED", "true")
	t.Setenv("MLOPS_AUTOMATED_TENANT_ID", "tenant-r336")
	t.Setenv("MLOPS_AUTOMATED_MODEL_NAME", "behavior-classifier-r336")
	t.Setenv("MLOPS_MIN_FEATURE_SAMPLES", "2048")
	t.Setenv("MLOPS_MAX_FEATURE_SIGNAL_AGE", "90m")

	cfg := loadMLOpsOrchestratorConfigFromEnv()
	if !cfg.AutomaticCandidatesEnabled {
		t.Fatal("automatic candidate flag env was ignored")
	}
	if cfg.AutomatedTenantID != "tenant-r336" {
		t.Fatalf("automatic tenant env was ignored: %q", cfg.AutomatedTenantID)
	}
	if cfg.AutomatedModelName != "behavior-classifier-r336" {
		t.Fatalf("automatic model env was ignored: %q", cfg.AutomatedModelName)
	}
	if cfg.MinFeatureSamples != 2048 || cfg.MaxFeatureSignalAge.String() != "1h30m0s" {
		t.Fatalf("drift signal limits were ignored: %+v", cfg)
	}
}
