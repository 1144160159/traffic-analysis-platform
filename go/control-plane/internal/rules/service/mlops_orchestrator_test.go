////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/service/mlops_orchestrator_test.go
// MLOps Orchestrator Unit Tests
////////////////////////////////////////////////////////////////////////////////

package service

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/lib/pq"

	"go.uber.org/zap"
)

func TestMLOpsOrchestratorConfig(t *testing.T) {
	cfg := DefaultMLOpsOrchestratorConfig()

	if cfg.CheckInterval != 1*time.Hour {
		t.Errorf("expected CheckInterval=1h, got %v", cfg.CheckInterval)
	}
	if cfg.MinNewFeedbackCount != 500 {
		t.Errorf("expected MinNewFeedbackCount=500, got %d", cfg.MinNewFeedbackCount)
	}
	if cfg.MaxFPRate != 0.15 {
		t.Errorf("expected MaxFPRate=0.15, got %f", cfg.MaxFPRate)
	}
	if cfg.MaxPSI != 0.25 {
		t.Errorf("expected MaxPSI=0.25, got %f", cfg.MaxPSI)
	}
	if cfg.MinRetrainInterval != 12*time.Hour {
		t.Errorf("expected MinRetrainInterval=12h, got %v", cfg.MinRetrainInterval)
	}
	if cfg.ArgoServerURL != "http://argo-server.argo.svc:2746" {
		t.Errorf("expected ArgoServerURL default, got %q", cfg.ArgoServerURL)
	}
}

func TestNewMLOpsOrchestrator(t *testing.T) {
	logger := zap.NewNop()
	cfg := DefaultMLOpsOrchestratorConfig()

	// Test with nil ClickHouse (graceful degradation)
	orch := NewMLOpsOrchestrator(nil, nil, cfg, logger)
	if orch == nil {
		t.Fatal("NewMLOpsOrchestrator returned nil")
	}
	if orch.chDB != nil {
		t.Error("expected nil chDB")
	}
	if orch.config.CheckInterval != cfg.CheckInterval {
		t.Error("config mismatch")
	}
}

func TestOrchestratorStartStop(t *testing.T) {
	logger := zap.NewNop()
	cfg := DefaultMLOpsOrchestratorConfig()
	cfg.CheckInterval = 100 * time.Millisecond

	orch := NewMLOpsOrchestrator(nil, nil, cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start in goroutine
	go orch.Start(ctx)

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// Stop
	orch.Stop()

	// Should not panic
	orch.Stop() // double-stop is safe
}

func TestOrchestratorWithNilClickHouse(t *testing.T) {
	logger := zap.NewNop()
	cfg := DefaultMLOpsOrchestratorConfig()

	orch := NewMLOpsOrchestrator(nil, nil, cfg, logger)

	ctx := context.Background()

	// All CH-based checks should return nil (no-op) without error
	decision := orch.checkFeedbackAccumulation(ctx)
	if decision != nil {
		t.Error("checkFeedbackAccumulation should return nil when chDB is nil")
	}

	decision = orch.checkFPRate(ctx)
	if decision != nil {
		t.Error("checkFPRate should return nil when chDB is nil")
	}

	decision = orch.checkDataDrift(ctx)
	if decision != nil {
		t.Error("checkDataDrift should return nil when chDB is nil")
	}
}

func TestOrchestratorEvaluateConditionsNoDB(t *testing.T) {
	logger := zap.NewNop()
	cfg := DefaultMLOpsOrchestratorConfig()

	orch := NewMLOpsOrchestrator(nil, nil, cfg, logger)

	ctx := context.Background()
	decision := orch.evaluateConditions(ctx)

	if decision.ShouldRetrain {
		t.Error("should not retrain without database connections")
	}
	if decision.Metrics == nil {
		t.Error("decision.Metrics should not be nil")
	}
}

func TestOrchestratorGetStatus(t *testing.T) {
	logger := zap.NewNop()
	cfg := DefaultMLOpsOrchestratorConfig()

	orch := NewMLOpsOrchestrator(nil, nil, cfg, logger)

	status := orch.GetStatus()

	requiredFields := []string{
		"last_retrain_time", "running_workflows",
		"max_concurrent", "min_retrain_interval",
		"check_interval", "min_feedback_count", "max_fp_rate",
	}
	for _, field := range requiredFields {
		if _, ok := status[field]; !ok {
			t.Errorf("status missing field: %s", field)
		}
	}
}

func TestRetrainTriggerConstants(t *testing.T) {
	triggers := []RetrainTrigger{
		TriggerManual,
		TriggerScheduled,
		TriggerFeedback,
		TriggerFPRate,
		TriggerDrift,
		TriggerDataVolume,
	}

	for _, trigger := range triggers {
		if trigger == "" {
			t.Error("RetrainTrigger should not be empty")
		}
	}

	// Verify uniqueness
	seen := make(map[RetrainTrigger]bool)
	for _, trigger := range triggers {
		if seen[trigger] {
			t.Errorf("duplicate trigger: %s", trigger)
		}
		seen[trigger] = true
	}
}

func TestManualRetrainRequest(t *testing.T) {
	req := &ManualRetrainRequest{
		ModelType:    "xgboost",
		LookbackDays: 14,
		TenantID:     "test-tenant",
		FeatureSetID: "v2",
	}

	if req.ModelType != "xgboost" {
		t.Error("ModelType mismatch")
	}
	if req.LookbackDays != 14 {
		t.Error("LookbackDays mismatch")
	}
}

func TestAllowedManualRetrainParameters(t *testing.T) {
	params := allowedManualRetrainParameters(map[string]interface{}{
		"min-feedback-count": 1,
		"trainer-image":      "traffic/mlops-trainer:latest",
		"unknown":            "ignored",
		"trigger-reason":     "manual validation",
		"bad-newline":        "a\nb",
	})

	seen := make(map[string]bool)
	for _, param := range params {
		seen[param] = true
	}

	if !seen["min-feedback-count=1"] {
		t.Error("expected min-feedback-count to be forwarded")
	}
	if !seen["trainer-image=traffic/mlops-trainer:latest"] {
		t.Error("expected trainer-image to be forwarded")
	}
	if !seen["trigger-reason=manual validation"] {
		t.Error("expected trigger-reason to be forwarded")
	}
	if seen["unknown=ignored"] {
		t.Error("unknown parameter should be ignored")
	}
}

func TestOrchestratorDecisionStructure(t *testing.T) {
	decision := &RetrainDecision{
		ShouldRetrain: true,
		Trigger:       TriggerFeedback,
		Reason:        "test reason",
		Metrics: map[string]interface{}{
			"count": 100,
		},
	}

	if !decision.ShouldRetrain {
		t.Error("ShouldRetrain should be true")
	}
	if decision.Trigger != TriggerFeedback {
		t.Error("Trigger mismatch")
	}
	if decision.Reason != "test reason" {
		t.Error("Reason mismatch")
	}
}

// getTestDBs 获取测试数据库连接 (K8s 环境自动连接)
func getTestDBs(t *testing.T) (*sql.DB, *sql.DB) {
	t.Helper()

	pgHost := "postgres.databases.svc"
	if h := os.Getenv("PG_HOST"); h != "" {
		pgHost = h
	}
	pgPort := "5432"
	if p := os.Getenv("PG_PORT"); p != "" {
		pgPort = p
	}
	pgPassword := os.Getenv("PG_PASSWORD")
	if pgPassword == "" {
		t.Log("PG_PASSWORD is not set; skipping DB test")
		return nil, nil
	}
	pgDSN := "host=" + pgHost + " port=" + pgPort + " user=postgres password=" + pgPassword + " dbname=traffic_platform sslmode=disable connect_timeout=5"
	pgDB, err := sql.Open("postgres", pgDSN)
	if err != nil || pgDB.Ping() != nil {
		t.Logf("PG unavailable (%s) — skipping DB test", pgHost)
		return nil, nil
	}

	chHost := "clickhouse-1.middleware.svc"
	if h := os.Getenv("CH_HOST"); h != "" {
		chHost = h
	}
	chPort := "9000"
	if p := os.Getenv("CH_PORT"); p != "" {
		chPort = p
	}
	chDSN := "clickhouse://default:@" + chHost + ":" + chPort + "/traffic?dial_timeout=5s"
	chDB, err := sql.Open("clickhouse", chDSN)
	if err != nil || chDB.Ping() != nil {
		t.Logf("CH unavailable (%s) — will test PG only", chHost)
		chDB = nil
	}

	return chDB, pgDB
}

func TestOrchestratorWithRealDB(t *testing.T) {
	chDB, pgDB := getTestDBs(t)
	if pgDB == nil {
		t.Skip("No database connection available (run in K8s Pod or set PG_HOST/CH_HOST)")
	}
	defer pgDB.Close()
	if chDB != nil {
		defer chDB.Close()
	}

	logger := zap.NewNop()
	cfg := DefaultMLOpsOrchestratorConfig()
	cfg.FeedbackLookbackHours = 720 // 30 days for testing
	cfg.MinNewFeedbackCount = 1     // Lower threshold for testing

	orch := NewMLOpsOrchestrator(chDB, pgDB, cfg, logger)

	// Query real ClickHouse for feedback accumulation
	decision := orch.checkFeedbackAccumulation(context.Background())
	if decision != nil {
		t.Logf("Feedback check: shouldRetrain=%v, trigger=%s, reason=%s",
			decision.ShouldRetrain, decision.Trigger, decision.Reason)
	} else {
		t.Log("Feedback check: DB returned nil (no data or CH unavailable)")
	}

	// Check FP rate
	decision = orch.checkFPRate(context.Background())
	if decision != nil {
		t.Logf("FP rate check: shouldRetrain=%v, trigger=%s, reason=%s",
			decision.ShouldRetrain, decision.Trigger, decision.Reason)
	} else {
		t.Log("FP rate check: DB returned nil (insufficient data or CH unavailable)")
	}

	// Check drift (uses both CH + PG)
	decision = orch.checkDataDrift(context.Background())
	if decision != nil {
		t.Logf("Drift check: shouldRetrain=%v, trigger=%s, reason=%s",
			decision.ShouldRetrain, decision.Trigger, decision.Reason)
	} else {
		t.Log("Drift check: DB returned nil (insufficient data or DB unavailable)")
	}
}

// Benchmark for evaluate loop
func BenchmarkEvaluateConditions(b *testing.B) {
	logger := zap.NewNop()
	cfg := DefaultMLOpsOrchestratorConfig()
	orch := NewMLOpsOrchestrator(nil, nil, cfg, logger)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = orch.evaluateConditions(ctx)
	}
}
