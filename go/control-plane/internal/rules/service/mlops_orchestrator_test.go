////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/rules/service/mlops_orchestrator_test.go
// MLOps Orchestrator Unit Tests
////////////////////////////////////////////////////////////////////////////////

package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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
	scope := &automatedMLOpsScope{TenantID: "default", ModelID: "model-1", FeatureSetID: "v1"}

	// All CH-based checks should return nil (no-op) without error
	decision := orch.checkFeedbackAccumulation(ctx, scope)
	if decision != nil {
		t.Error("checkFeedbackAccumulation should return nil when chDB is nil")
	}

	decision = orch.checkFPRate(ctx, scope)
	if decision != nil {
		t.Error("checkFPRate should return nil when chDB is nil")
	}

	decision = orch.checkDataDrift(ctx, scope)
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
	allowed := map[string]interface{}{
		"model-id":       "model-1",
		"trigger-reason": "manual validation",
	}
	if err := ValidateManualRetrainParameters(allowed); err != nil {
		t.Fatalf("trusted manual parameters should validate: %v", err)
	}
	params := allowedManualRetrainParameters(allowed)

	seen := make(map[string]bool)
	for _, param := range params {
		seen[param] = true
	}

	if !seen["model-id=model-1"] {
		t.Error("expected model-id to be forwarded")
	}
	if !seen["trigger-reason=manual validation"] {
		t.Error("expected trigger-reason to be forwarded")
	}
	for _, unsafe := range []map[string]interface{}{
		{"min-feedback-count": 1},
		{"trainer-image": "traffic/mlops-trainer:latest"},
		{"auto-activate": true},
		{"unknown": "must-not-be-silent"},
		{"trigger-reason": "a\nb"},
		{"model-id": "checked", " model-id ": "ambiguous"},
	} {
		if err := ValidateManualRetrainParameters(unsafe); err == nil {
			t.Fatalf("unsafe or malformed parameters must be rejected: %+v", unsafe)
		}
	}
}

func TestTriggerManualRetrainFailsClosedWithoutSubmittedIdentity(t *testing.T) {
	for name, response := range map[string]string{
		"missing":  `{"status":{"phase":"Running"}}`,
		"mismatch": `{"metadata":{"name":"mlops-manual-other"},"status":{"phase":"Running"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			cfg := DefaultMLOpsOrchestratorConfig()
			cfg.ArgoServerURL = server.URL
			cfg.ArgoNamespace = "argo"
			orch := NewMLOpsOrchestrator(nil, nil, cfg, zap.NewNop())
			request := &ManualRetrainRequest{TenantID: "default", WorkflowName: "mlops-manual-preaudited", Params: map[string]interface{}{"model-id": "model-1"}}
			if _, err := orch.TriggerManualRetrain(context.Background(), request); err == nil {
				t.Fatalf("missing or mismatched submit identity must fail closed, got %v", err)
			}
		})
	}
}

func TestAutomatedRetrainAuditFailurePreventsArgoMutation(t *testing.T) {
	argoCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		argoCalls++
		_, _ = w.Write([]byte(`{"metadata":{"name":"must-not-exist"}}`))
	}))
	defer server.Close()
	db := sql.OpenDB(failingAuditConnector{})
	defer db.Close()
	modelService := NewModelService(db, nil, nil, nil, zap.NewNop(), DefaultModelServiceConfig())
	cfg := DefaultMLOpsOrchestratorConfig()
	cfg.ArgoServerURL = server.URL
	cfg.ArgoNamespace = "argo"
	orch := NewMLOpsOrchestrator(nil, nil, cfg, zap.NewNop())
	orch.SetAuditService(modelService)

	err := orch.submitAutomatedRetrain(context.Background(), &RetrainDecision{ShouldRetrain: true, Trigger: TriggerFPRate, Reason: "test", TenantID: "default", ModelID: "model-1", FeatureSetID: "v1"})
	if err == nil {
		t.Fatal("automatic submit must fail when its required audit intent cannot persist")
	}
	if argoCalls != 0 {
		t.Fatalf("Argo must not be called before automatic audit intent, calls=%d", argoCalls)
	}
}

func TestAutomatedRetrainFailsClosedWithoutSubmittedIdentity(t *testing.T) {
	for name, response := range map[string]string{
		"missing":  `{"status":{"phase":"Running"}}`,
		"mismatch": `{"metadata":{"name":"mlops-fp-rate-other"},"status":{"phase":"Running"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			cfg := DefaultMLOpsOrchestratorConfig()
			cfg.ArgoServerURL = server.URL
			cfg.ArgoNamespace = "argo"
			orch := NewMLOpsOrchestrator(nil, nil, cfg, zap.NewNop())
			decision := &RetrainDecision{ShouldRetrain: true, Trigger: TriggerFPRate, Reason: "test", TenantID: "default", ModelID: "model-1", FeatureSetID: "v1"}
			if err := orch.submitArgoWorkflow(context.Background(), decision, "mlops-fp-rate-preaudited"); err == nil {
				t.Fatal("automatic submit must fail closed when Argo identity is missing or mismatched")
			}
		})
	}
}

func TestAutomatedRetrainSubmitCarriesCanonicalOwnershipParameters(t *testing.T) {
	var submittedName string
	parameters := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SubmitOptions struct {
				Name       string   `json:"name"`
				Parameters []string `json:"parameters"`
			} `json:"submitOptions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode submit request: %v", err)
		}
		submittedName = body.SubmitOptions.Name
		for _, parameter := range body.SubmitOptions.Parameters {
			parts := strings.SplitN(parameter, "=", 2)
			if len(parts) == 2 {
				parameters[parts[0]] = parts[1]
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"metadata": map[string]string{"name": submittedName}})
	}))
	defer server.Close()
	cfg := DefaultMLOpsOrchestratorConfig()
	cfg.ArgoServerURL = server.URL
	cfg.ArgoNamespace = "argo"
	orch := NewMLOpsOrchestrator(nil, nil, cfg, zap.NewNop())
	decision := &RetrainDecision{
		ShouldRetrain: true, Trigger: TriggerFPRate, Reason: "tenant-scoped test",
		TenantID: "tenant-a", ModelID: "model-a", FeatureSetID: "feature-a",
	}
	if err := orch.submitArgoWorkflow(context.Background(), decision, "mlops-fp-rate-owned"); err != nil {
		t.Fatalf("automatic submit failed: %v", err)
	}
	if submittedName != "mlops-fp-rate-owned" {
		t.Fatalf("exact pre-audited identity was not submitted: %q", submittedName)
	}
	for key, expected := range map[string]string{"tenant-id": "tenant-a", "model-id": "model-a", "feature-set-id": "feature-a", "trigger": "fp_rate"} {
		if parameters[key] != expected {
			t.Fatalf("missing canonical ownership parameter %s=%s in %+v", key, expected, parameters)
		}
	}
}

func TestReconcileRunningWorkflowsDropsCompletedAutomaticTask(t *testing.T) {
	phase := "Running"
	createdAt := time.Now().UTC().Add(-15 * time.Minute).Truncate(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"items": []interface{}{
			map[string]interface{}{
				"metadata": map[string]interface{}{"name": "mlops-fp-rate-owned", "creationTimestamp": createdAt.Format(time.RFC3339)},
				"spec":     map[string]interface{}{"workflowTemplateRef": map[string]string{"name": "mlops-training-template"}, "arguments": map[string]interface{}{"parameters": []interface{}{map[string]string{"name": "tenant-id", "value": "default"}}}},
				"status":   map[string]string{"phase": phase},
			},
		}})
	}))
	defer server.Close()
	cfg := DefaultMLOpsOrchestratorConfig()
	cfg.ArgoServerURL = server.URL
	cfg.ArgoNamespace = "argo"
	orch := NewMLOpsOrchestrator(nil, nil, cfg, zap.NewNop())
	if running, err := orch.reconcileRunningWorkflows(context.Background(), "default"); err != nil || running != 1 {
		t.Fatalf("running workflow must count toward concurrency: running=%d err=%v", running, err)
	}
	if !orch.lastRetrainTime.Equal(createdAt) {
		t.Fatalf("restart must restore durable cooldown from owned workflow: got=%s want=%s", orch.lastRetrainTime, createdAt)
	}
	phase = "Succeeded"
	if running, err := orch.reconcileRunningWorkflows(context.Background(), "default"); err != nil || running != 0 {
		t.Fatalf("completed workflow must release concurrency: running=%d err=%v", running, err)
	}
	if status := orch.GetStatus(); status["running_workflows"] != 0 {
		t.Fatalf("reported status must use reconciled counter: %+v", status)
	}
}

type advisoryLockTestState struct {
	mu       sync.Mutex
	nextConn int
	owner    int
}

type advisoryLockTestConnector struct{ state *advisoryLockTestState }

func (c advisoryLockTestConnector) Connect(context.Context) (driver.Conn, error) {
	c.state.mu.Lock()
	c.state.nextConn++
	id := c.state.nextConn
	c.state.mu.Unlock()
	return &advisoryLockTestConn{state: c.state, id: id}, nil
}
func (c advisoryLockTestConnector) Driver() driver.Driver {
	return advisoryLockTestDriver{state: c.state}
}

type advisoryLockTestDriver struct{ state *advisoryLockTestState }

func (d advisoryLockTestDriver) Open(string) (driver.Conn, error) {
	return advisoryLockTestConnector{state: d.state}.Connect(context.Background())
}

type advisoryLockTestConn struct {
	state *advisoryLockTestState
	id    int
}

func (c *advisoryLockTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *advisoryLockTestConn) Close() error                        { return nil }
func (c *advisoryLockTestConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c *advisoryLockTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	value := false
	switch {
	case strings.Contains(query, "pg_try_advisory_lock"):
		if c.state.owner == 0 {
			c.state.owner = c.id
			value = true
		}
	case strings.Contains(query, "pg_advisory_unlock"):
		if c.state.owner == c.id {
			c.state.owner = 0
			value = true
		}
	}
	return &advisoryLockTestRows{value: value}, nil
}

type advisoryLockTestRows struct {
	value bool
	done  bool
}

func (r *advisoryLockTestRows) Columns() []string { return []string{"locked"} }
func (r *advisoryLockTestRows) Close() error      { return nil }
func (r *advisoryLockTestRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	values[0] = r.value
	return nil
}

func TestAutomaticSchedulerAdvisoryLockAllowsOneConcurrentArgoPost(t *testing.T) {
	state := &advisoryLockTestState{}
	db := sql.OpenDB(advisoryLockTestConnector{state: state})
	defer db.Close()
	first := NewMLOpsOrchestrator(nil, db, DefaultMLOpsOrchestratorConfig(), zap.NewNop())
	second := NewMLOpsOrchestrator(nil, db, DefaultMLOpsOrchestratorConfig(), zap.NewNop())

	postCount := 0
	var postMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		postMu.Lock()
		postCount++
		postMu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	releaseFirst, acquired, err := first.acquireAutomaticSchedulerLock(context.Background())
	if err != nil || !acquired {
		t.Fatalf("first orchestrator did not acquire scheduler lock: acquired=%v err=%v", acquired, err)
	}
	secondResult := make(chan bool, 1)
	go func() {
		releaseSecond, secondAcquired, acquireErr := second.acquireAutomaticSchedulerLock(context.Background())
		if acquireErr != nil {
			secondResult <- false
			return
		}
		if secondAcquired {
			defer releaseSecond()
			_, _ = http.Post(server.URL, "application/json", strings.NewReader(`{}`))
		}
		secondResult <- secondAcquired
	}()
	if <-secondResult {
		t.Fatal("overlapping orchestrator acquired the same PostgreSQL scheduler lock")
	}
	_, _ = http.Post(server.URL, "application/json", strings.NewReader(`{}`))
	releaseFirst()

	postMu.Lock()
	defer postMu.Unlock()
	if postCount != 1 {
		t.Fatalf("two concurrent orchestrators must yield exactly one Argo POST, got %d", postCount)
	}
}

func TestListAndMutateRealArgoWorkflows(t *testing.T) {
	phase := "Running"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workflows/argo":
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"mlops-manual-live","namespace":"argo","creationTimestamp":"2026-07-19T00:00:00Z"},"spec":{"workflowTemplateRef":{"name":"mlops-training-template"},"arguments":{"parameters":[{"name":"model-id","value":"model-1"}]}},"status":{"phase":"` + phase + `","progress":"2/8","startedAt":"2026-07-19T00:00:01Z"}},{"metadata":{"name":"unrelated","namespace":"argo"},"status":{"phase":"Succeeded"}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workflows/argo/mlops-manual-live":
			_, _ = w.Write([]byte(`{"metadata":{"name":"mlops-manual-live","namespace":"argo","creationTimestamp":"2026-07-19T00:00:00Z"},"spec":{"workflowTemplateRef":{"name":"mlops-training-template"}},"status":{"phase":"` + phase + `","progress":"2/8"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/workflows/argo/mlops-manual-live/stop":
			phase = "Failed"
			_, _ = w.Write([]byte(`{"metadata":{"name":"mlops-manual-live","namespace":"argo"},"status":{"phase":"Failed","message":"Stopped with strategy Stop"}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/workflows/argo/mlops-manual-live/resubmit":
			phase = "Running"
			_, _ = w.Write([]byte(`{"metadata":{"name":"mlops-manual-resubmitted","namespace":"argo"},"spec":{"workflowTemplateRef":{"name":"mlops-training-template"}},"status":{"phase":"Running","progress":"0/8"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := DefaultMLOpsOrchestratorConfig()
	cfg.ArgoServerURL = server.URL
	cfg.ArgoNamespace = "argo"
	orch := NewMLOpsOrchestrator(nil, nil, cfg, zap.NewNop())
	workflows, err := orch.ListWorkflows(context.Background())
	if err != nil || len(workflows) != 1 {
		t.Fatalf("expected one projected workflow, got %d, err=%v", len(workflows), err)
	}
	if !workflows[0].CanStop || workflows[0].CanRetry || workflows[0].Parameters["model-id"] != "model-1" {
		t.Fatalf("unexpected running workflow projection: %+v", workflows[0])
	}
	stopped, err := orch.StopWorkflow(context.Background(), workflows[0].Name)
	if err != nil || stopped.Phase != "Failed" {
		t.Fatalf("expected stopped workflow, got %+v, err=%v", stopped, err)
	}
	retried, err := orch.RetryWorkflow(context.Background(), workflows[0].Name)
	if err != nil || retried.Phase != "Running" || retried.Name != "mlops-manual-resubmitted" {
		t.Fatalf("expected retried workflow, got %+v, err=%v", retried, err)
	}
	if _, err := orch.GetWorkflow(context.Background(), "other-workflow"); err == nil || !strings.Contains(err.Error(), "invalid MLOps workflow name") {
		t.Fatalf("expected strict workflow name validation, got %v", err)
	}
}

func TestRetryWorkflowFailsClosedWithoutResubmittedIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"metadata":{"name":"mlops-manual-failed","namespace":"argo"},"spec":{"arguments":{"parameters":[{"name":"tenant-id","value":"default"}]}},"status":{"phase":"Failed"}}`))
			return
		}
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/resubmit") {
			_, _ = w.Write([]byte(`{"status":{"phase":"Running"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	cfg := DefaultMLOpsOrchestratorConfig()
	cfg.ArgoServerURL = server.URL
	cfg.ArgoNamespace = "argo"
	orch := NewMLOpsOrchestrator(nil, nil, cfg, zap.NewNop())
	if _, err := orch.RetryWorkflow(context.Background(), "mlops-manual-failed"); err == nil || !strings.Contains(err.Error(), "new workflow identity") {
		t.Fatalf("missing resubmit identity must fail closed, got %v", err)
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

	scope, err := orch.resolveAutomatedScope(context.Background())
	if err != nil {
		t.Fatalf("resolve automatic scope: %v", err)
	}

	// Query real ClickHouse for feedback accumulation
	decision := orch.checkFeedbackAccumulation(context.Background(), scope)
	if decision != nil {
		t.Logf("Feedback check: shouldRetrain=%v, trigger=%s, reason=%s",
			decision.ShouldRetrain, decision.Trigger, decision.Reason)
	} else {
		t.Log("Feedback check: DB returned nil (no data or CH unavailable)")
	}

	// Check FP rate
	decision = orch.checkFPRate(context.Background(), scope)
	if decision != nil {
		t.Logf("FP rate check: shouldRetrain=%v, trigger=%s, reason=%s",
			decision.ShouldRetrain, decision.Trigger, decision.Reason)
	} else {
		t.Log("FP rate check: DB returned nil (insufficient data or CH unavailable)")
	}

	// Check drift (uses both CH + PG)
	decision = orch.checkDataDrift(context.Background(), scope)
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
