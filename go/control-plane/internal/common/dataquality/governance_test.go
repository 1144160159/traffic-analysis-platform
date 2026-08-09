package dataquality

import (
	"errors"
	"testing"
)

func TestRuleGovernanceTransitions(t *testing.T) {
	tests := []struct {
		current, action, next, operation string
	}{
		{"draft", "start_shadow", "shadow", "shadow_started"},
		{"shadow", "submit_approval", "approval_pending", "approval_submitted"},
		{"approval_pending", "approve", "active", "approved"},
		{"approval_pending", "reject", "rejected", "rejected"},
		{"active", "retire", "retired", "retired"},
	}
	for _, test := range tests {
		next, operation, err := nextRuleStatus(test.current, test.action)
		if err != nil || next != test.next || operation != test.operation {
			t.Fatalf("%s/%s => %s/%s, %v", test.current, test.action, next, operation, err)
		}
	}
}

func TestRuleGovernanceRejectsSkippedApproval(t *testing.T) {
	_, _, err := nextRuleStatus("draft", "approve")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

func TestDatasetGovernanceValidationRequiresStableCommandIdentity(t *testing.T) {
	command := DatasetCommand{
		TenantID: "tenant-a", DatasetID: "flows_raw", DisplayName: "Flows", Owner: "data-platform",
		SchemaVersion: 1, SignalContractVersion: "v1", BusinessKeys: []string{"event_id"},
		RetentionSeconds: 86400, SLOTarget: .999, Status: "active", ExpectedRevision: 0,
		ActionID: "dq-dataset-create", IdempotencyKey: "short", Reason: "valid governance reason",
		Actor: "operator-a", TraceID: "trace-a",
	}
	if err := validateDatasetCommand(command); err == nil {
		t.Fatal("short idempotency key must be rejected")
	}
	command.IdempotencyKey = "dataset-command-key-0001"
	if err := validateDatasetCommand(command); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}
}

func TestRuleGovernanceCommandHashIgnoresTransportTraceOnly(t *testing.T) {
	command := RuleTransitionCommand{TenantID: "tenant-a", RuleID: "991a4e37-bf00-49bc-bfce-672dbe4bf138", Action: "approve", ExpectedRevision: 3, ActionID: "approve-rule", IdempotencyKey: "rule-command-key-00001", Reason: "independent approval", Actor: "reviewer-a", TraceID: "trace-a"}
	first, err := commandSHA(command)
	if err != nil {
		t.Fatal(err)
	}
	command.TraceID = "trace-b"
	second, err := commandSHA(command)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("transport trace must not turn an exact idempotent retry into a collision")
	}
}
