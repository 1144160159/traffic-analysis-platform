package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/rules/model"
)

func TestNextDeploymentWorkflowStage(t *testing.T) {
	tests := []struct {
		name      string
		previous  map[string]interface{}
		action    string
		operation string
		want      string
		wantError bool
	}{
		{name: "new draft", previous: nil, action: "draft", operation: "deploy", want: "draft_saved"},
		{name: "precheck after draft", previous: map[string]interface{}{"stage": "draft_saved", "operation": "deploy"}, action: "precheck", operation: "deploy", want: "precheck_completed"},
		{name: "submit after precheck", previous: map[string]interface{}{"stage": "precheck_completed", "operation": "deploy"}, action: "submit_approval", operation: "deploy", want: "approval_pending"},
		{name: "approve pending", previous: map[string]interface{}{"stage": "approval_pending", "operation": "deploy"}, action: "approve", operation: "deploy", want: "approved"},
		{name: "reject pending", previous: map[string]interface{}{"stage": "approval_pending", "operation": "rollback"}, action: "reject", operation: "rollback", want: "rejected"},
		{name: "draft rollback after approved deploy", previous: map[string]interface{}{"stage": "approved", "operation": "deploy"}, action: "draft", operation: "rollback", want: "draft_saved"},
		{name: "stale precheck cannot overwrite approval", previous: map[string]interface{}{"stage": "approval_pending", "operation": "deploy"}, action: "precheck", operation: "deploy", wantError: true},
		{name: "wrong operation cannot approve", previous: map[string]interface{}{"stage": "approval_pending", "operation": "deploy"}, action: "approve", operation: "rollback", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := nextDeploymentWorkflowStage(test.previous, test.action, test.operation)
			if test.wantError {
				if err == nil {
					t.Fatalf("nextDeploymentWorkflowStage() = %q, want error", got)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("nextDeploymentWorkflowStage() = %q, %v; want %q, nil", got, err, test.want)
			}
		})
	}
}

func TestMergeWorkflowConfigurationOnlyBeforeSubmission(t *testing.T) {
	previous := map[string]interface{}{
		"configuration": map[string]interface{}{
			"target_deployment_id": "target-1",
			"reason":               "原始审批原因不少于十个字",
		},
	}
	merged := mergeWorkflowConfiguration(previous, map[string]interface{}{"reason": "预检查前更新后的原因不少于十个字"})
	if merged["target_deployment_id"] != "target-1" {
		t.Fatalf("target_deployment_id = %v, want target-1", merged["target_deployment_id"])
	}
	if merged["reason"] != "预检查前更新后的原因不少于十个字" {
		t.Fatalf("reason = %v, want updated value", merged["reason"])
	}
}

func TestApprovalSnapshotHashStableAcrossMapOrder(t *testing.T) {
	left := map[string]interface{}{"scope": map[string]interface{}{"campus": "A", "percentage": 20}, "channels": []interface{}{"mail", "sms"}}
	right := map[string]interface{}{"channels": []interface{}{"mail", "sms"}, "scope": map[string]interface{}{"percentage": 20.0, "campus": "A"}}
	leftHash, err := canonicalValueHash(left)
	if err != nil {
		t.Fatal(err)
	}
	rightHash, err := canonicalValueHash(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftHash != rightHash {
		t.Fatalf("canonical hashes differ: %s != %s", leftHash, rightHash)
	}
}

func TestApprovalSnapshotHashChangesWithApprovedInputs(t *testing.T) {
	deployment := &model.Deployment{
		DeploymentID: "deployment-1", TenantID: "tenant-1", RuleVersion: "rule-v1",
		Scope: map[string]interface{}{"percentage": 20, "release_line": "ruleset"},
	}
	base := buildDeploymentApprovalSnapshot(deployment, "deploy", map[string]interface{}{"strategy": "canary"})
	baseHash, _ := canonicalValueHash(base)

	deployment.Scope["percentage"] = 30
	scopeHash, _ := canonicalValueHash(buildDeploymentApprovalSnapshot(deployment, "deploy", map[string]interface{}{"strategy": "canary"}))
	deployment.Scope["percentage"] = 20
	configurationHash, _ := canonicalValueHash(buildDeploymentApprovalSnapshot(deployment, "deploy", map[string]interface{}{"strategy": "blue_green"}))
	deployment.RuleVersion = "rule-v2"
	artifactHash, _ := canonicalValueHash(buildDeploymentApprovalSnapshot(deployment, "deploy", map[string]interface{}{"strategy": "canary"}))

	for label, got := range map[string]string{"scope": scopeHash, "configuration": configurationHash, "artifact": artifactHash} {
		if got == baseHash {
			t.Fatalf("%s mutation did not change approval hash", label)
		}
	}
}

func TestApprovedWorkflowFailsClosedWithoutSnapshot(t *testing.T) {
	deployment := &model.Deployment{Metadata: map[string]interface{}{"workflow": map[string]interface{}{"stage": "approved", "operation": "deploy"}}}
	if err := requireApprovedDeploymentWorkflow(deployment, "deploy"); err == nil {
		t.Fatal("legacy approved workflow without snapshot must fail closed")
	}
}

func TestApprovedRollbackConfigurationBoundToSnapshot(t *testing.T) {
	deployment := &model.Deployment{
		DeploymentID: "deployment-1", TenantID: "tenant-1", RuleVersion: "rule-v1",
		Scope: map[string]interface{}{"percentage": 100, "release_line": "ruleset"}, Metadata: map[string]interface{}{},
	}
	configuration := map[string]interface{}{"target_deployment_id": "deployment-0", "reason": "经独立审批执行回滚操作"}
	snapshot := buildDeploymentApprovalSnapshot(deployment, "rollback", configuration)
	hash, _ := canonicalValueHash(snapshot)
	workflow := map[string]interface{}{
		"stage": "approved", "operation": "rollback", "configuration": configuration,
		"approval_snapshot": snapshot, "approval_snapshot_hash": hash,
	}
	workflow["precheck_results"] = freshPrecheckResults(time.Now().UTC())
	deployment.Metadata["workflow"] = workflow
	got, err := approvedDeploymentConfiguration(deployment, "rollback")
	if err != nil {
		t.Fatal(err)
	}
	if got["target_deployment_id"] != "deployment-0" {
		t.Fatalf("unexpected approved target: %v", got["target_deployment_id"])
	}
	got["target_deployment_id"] = "mutated"
	if configuration["target_deployment_id"] != "deployment-0" {
		t.Fatal("approved configuration helper returned mutable workflow storage")
	}
}

func freshPrecheckResults(now time.Time) []interface{} {
	results := make([]interface{}, 0, 7)
	for index := 0; index < 7; index++ {
		results = append(results, map[string]interface{}{
			"label":       fmt.Sprintf("check-%d", index+1),
			"fresh_until": now.Add(30 * time.Minute),
		})
	}
	return results
}

func TestDeploymentPrecheckFreshnessFailsClosed(t *testing.T) {
	now := time.Now().UTC()
	workflow := map[string]interface{}{"precheck_results": freshPrecheckResults(now)}
	if err := requireFreshDeploymentPrecheck(workflow, now); err != nil {
		t.Fatalf("fresh precheck rejected: %v", err)
	}
	workflow["precheck_results"].([]interface{})[3].(map[string]interface{})["fresh_until"] = now.Add(-time.Second)
	if err := requireFreshDeploymentPrecheck(workflow, now); err == nil {
		t.Fatal("expired precheck must be rejected")
	}
}

func TestDeploymentReleaseLineDerivation(t *testing.T) {
	tests := []struct {
		name       string
		deployment *model.Deployment
		want       string
	}{
		{name: "explicit", deployment: &model.Deployment{Scope: map[string]interface{}{"release_line": "dns-detection"}, RuleVersion: "r1"}, want: "dns-detection"},
		{name: "rule", deployment: &model.Deployment{RuleVersion: "r1"}, want: "ruleset"},
		{name: "model", deployment: &model.Deployment{ModelVersion: "m1"}, want: "model"},
		{name: "bundle", deployment: &model.Deployment{RuleVersion: "r1", ModelVersion: "m1"}, want: "detection-bundle"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := deploymentReleaseLine(test.deployment); got != test.want {
				t.Fatalf("deploymentReleaseLine() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDeploymentOutboxBackoffCaps(t *testing.T) {
	if got := deploymentOutboxBackoff(1); got != deploymentOutboxRetryDelay {
		t.Fatalf("first delay = %s", got)
	}
	if got := deploymentOutboxBackoff(99); got != 5*time.Minute {
		t.Fatalf("capped delay = %s", got)
	}
}
