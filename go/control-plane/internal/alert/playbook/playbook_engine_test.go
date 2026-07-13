package playbook

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestPlaybookEngineDefaults(t *testing.T) {
	executor := NewActionExecutor(zap.NewNop())
	engine := NewPlaybookEngine(executor, zap.NewNop())

	defaults := DefaultPlaybooks()
	if len(defaults) != 6 {
		t.Errorf("expected 6 default playbooks, got %d", len(defaults))
	}

	for _, pb := range defaults {
		engine.RegisterPlaybook(pb)
	}
	t.Logf("Registered %d playbooks", len(engine.playbooks))
}

func TestMatchTrigger(t *testing.T) {
	engine := NewPlaybookEngine(NewActionExecutor(zap.NewNop()), zap.NewNop())
	pb := &Playbook{
		Name:    "test",
		Enabled: true,
		Trigger: Trigger{AlertType: "c2", SeverityMin: "high", ScoreMin: 0.8},
		MaxRuns: 5,
	}

	// Match
	alert := &AlertContext{AlertID: "a1", AlertType: "c2", Severity: "critical", Score: 0.95}
	if !engine.matchTrigger(pb, alert) {
		t.Error("should match c2+critical+0.95")
	}

	// No match: wrong type
	alert2 := &AlertContext{AlertID: "a2", AlertType: "scan", Severity: "high", Score: 0.9}
	if engine.matchTrigger(pb, alert2) {
		t.Error("should not match scan type")
	}

	// No match: low score
	alert3 := &AlertContext{AlertID: "a3", AlertType: "c2", Severity: "high", Score: 0.5}
	if engine.matchTrigger(pb, alert3) {
		t.Error("should not match low score")
	}
}

func TestPlaybookExecution(t *testing.T) {
	executor := NewActionExecutor(zap.NewNop())
	engine := NewPlaybookEngine(executor, zap.NewNop())

	pb := &Playbook{
		Name:    "test-playbook",
		Enabled: true,
		Trigger: Trigger{AlertType: "scan", SeverityMin: "high"},
		Actions: []Action{
			{Type: "block_ip", Parameters: map[string]interface{}{"duration": "24h"}, Timeout: 10 * time.Second},
			{Type: "tag", Parameters: map[string]interface{}{"tags": []string{"test"}}, Timeout: 5 * time.Second},
			{Type: "notify", Parameters: map[string]interface{}{"channel": "slack"}, Timeout: 5 * time.Second},
		},
		MaxRuns: 3,
	}
	engine.RegisterPlaybook(pb)

	alert := &AlertContext{AlertID: "a1", AlertType: "scan", Severity: "high", Score: 0.9}
	results := engine.Evaluate(context.Background(), alert)

	if len(results) != 1 {
		t.Fatalf("expected 1 execution result, got %d", len(results))
	}
	result := results[0]
	if result.PlaybookName != "test-playbook" {
		t.Errorf("expected playbook 'test-playbook', got '%s'", result.PlaybookName)
	}
	if result.SuccessActions != 3 {
		t.Errorf("expected 3 successful actions, got %d", result.SuccessActions)
	}
	if result.FailedActions != 0 {
		t.Errorf("expected 0 failed actions, got %d", result.FailedActions)
	}
}

func TestPlaybookMaxRuns(t *testing.T) {
	executor := NewActionExecutor(zap.NewNop())
	engine := NewPlaybookEngine(executor, zap.NewNop())

	pb := &Playbook{
		Name:    "limited",
		Enabled: true,
		Trigger: Trigger{AlertType: "scan"},
		Actions: []Action{{Type: "tag", Parameters: map[string]interface{}{}, Timeout: 1 * time.Second}},
		MaxRuns: 2,
	}
	engine.RegisterPlaybook(pb)

	alert := &AlertContext{AlertID: "a1", AlertType: "scan", Severity: "medium"}
	r1 := engine.Evaluate(context.Background(), alert)
	r2 := engine.Evaluate(context.Background(), alert)
	r3 := engine.Evaluate(context.Background(), alert)

	if len(r1) != 1 {
		t.Error("run 1 should execute")
	}
	if len(r2) != 1 {
		t.Error("run 2 should execute")
	}
	if len(r3) != 0 {
		t.Error("run 3 should be blocked by MaxRuns")
	}
}

func TestExecuteByNameRespectsMaxRuns(t *testing.T) {
	executor := NewActionExecutor(zap.NewNop())
	engine := NewPlaybookEngine(executor, zap.NewNop())

	pb := &Playbook{
		Name:    "manual-limited",
		Enabled: true,
		Actions: []Action{{Type: "tag", Parameters: map[string]interface{}{"tags": []string{"manual"}}, Timeout: time.Second}},
		MaxRuns: 1,
	}
	engine.RegisterPlaybook(pb)

	alert := &AlertContext{AlertID: "a1", AlertType: "scan", Severity: "high"}
	if _, err := engine.ExecuteByName(context.Background(), "manual-limited", alert); err != nil {
		t.Fatalf("first manual execution should pass: %v", err)
	}
	if _, err := engine.ExecuteByName(context.Background(), "manual-limited", alert); err == nil {
		t.Fatal("second manual execution should be blocked by MaxRuns")
	}

	updated, err := engine.UpdatePlaybook("manual-limited", nil, intPtr(2), nil)
	if err != nil {
		t.Fatalf("update max runs: %v", err)
	}
	if updated.RunCount != 1 {
		t.Fatalf("run count should remain 1 after max_runs update, got %d", updated.RunCount)
	}
	if _, err := engine.ExecuteByName(context.Background(), "manual-limited", alert); err != nil {
		t.Fatalf("second slot after max_runs update should pass: %v", err)
	}
}

func TestActionTypes(t *testing.T) {
	executor := NewActionExecutor(zap.NewNop())
	actionTypes := []string{"block_ip", "block_domain", "quarantine", "capture_pcap",
		"rate_limit", "tag", "enrich", "escalate", "notify"}

	for _, at := range actionTypes {
		action := Action{Type: at, Parameters: map[string]interface{}{}, Timeout: 1 * time.Second}
		alert := &AlertContext{AlertID: "a1", SourceIP: "10.0.0.1"}
		result := executor.Execute(context.Background(), action, alert)
		if result.Error != "" && at != "unknown" {
			t.Errorf("action type '%s' should not error: %s", at, result.Error)
		}
	}
}

func TestIsSeverityAtLeast(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"critical", "high", true},
		{"high", "medium", true},
		{"medium", "low", true},
		{"low", "high", false},
		{"medium", "critical", false},
	}
	for _, tc := range tests {
		if isSeverityAtLeast(tc.a, tc.b) != tc.want {
			t.Errorf("isSeverityAtLeast(%s, %s) != %v", tc.a, tc.b, tc.want)
		}
	}
}

func TestAllDefaultPlaybooksHaveRequiredFields(t *testing.T) {
	for _, pb := range DefaultPlaybooks() {
		if pb.Name == "" {
			t.Error("playbook name is required")
		}
		if len(pb.Actions) == 0 {
			t.Errorf("playbook %s has no actions", pb.Name)
		}
		if pb.Trigger.AlertType == "" {
			t.Errorf("playbook %s has no alert_type trigger", pb.Name)
		}
	}
}

func intPtr(v int) *int {
	return &v
}
