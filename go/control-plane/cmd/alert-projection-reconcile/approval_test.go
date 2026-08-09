package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeJSON(t *testing.T, path string, value interface{}) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return fileDigest(payload)
}

func approvalFixtures(t *testing.T, now time.Time) (string, string, string, string, string) {
	t.Helper()
	dir := t.TempDir()
	image := "registry.example.test/traffic/alert-projection-tools@sha256:" + strings.Repeat("a", 64)
	review := map[string]interface{}{
		"schema_version": 1, "mode": "REPAIR_REVIEW_PACKAGE", "execution_authorized": false,
		"production_applied": false, "production_mutations": []string{},
		"bindings": map[string]interface{}{
			"immutable_tool_image_digest": "sha256:" + strings.Repeat("a", 64),
			"shadow_captured_at":          now.Add(-2 * time.Minute).Format(time.RFC3339),
		},
		"proposed_execution": map[string]interface{}{"argv": []string{"alert-projection-reconcile", "--mode", "repair"}},
	}
	reviewPath := filepath.Join(dir, "review.json")
	reviewSHA := writeJSON(t, reviewPath, review)
	approvals := map[string]interface{}{}
	for index, role := range []string{"sre", "qa", "security", "domain_accountable"} {
		approvals[role] = map[string]interface{}{
			"status": "APPROVED", "approved_by": role + "-approver",
			"approved_at": now.Add(time.Duration(index-5) * time.Minute).Format(time.RFC3339),
		}
	}
	approval := map[string]interface{}{
		"schema_version": 1, "mode": "AUTHORIZED_BOUNDED_REPAIR", "execution_authorized": true,
		"review_package_sha256": reviewSHA, "immutable_tool_image": image, "approval_nonce": "change-1",
		"not_before": now.Add(-10 * time.Minute).Format(time.RFC3339),
		"expires_at": now.Add(10 * time.Minute).Format(time.RFC3339), "requested_by": "operator-a",
		"approvals": approvals,
	}
	approvalPath := filepath.Join(dir, "approval.json")
	approvalSHA := writeJSON(t, approvalPath, approval)
	return reviewPath, approvalPath, reviewSHA, approvalSHA, image
}

func TestRepairApprovalRequiresFourBoundIndependentApprovals(t *testing.T) {
	now := time.Date(2026, 8, 8, 2, 0, 0, 0, time.UTC)
	reviewPath, approvalPath, reviewSHA, approvalSHA, image := approvalFixtures(t, now)
	argv := []string{"alert-projection-reconcile", "--mode", "repair", "--review-package", reviewPath, "--approval-bundle", approvalPath, "--expected-review-sha256", reviewSHA, "--expected-approval-sha256", approvalSHA, "--expected-tool-image", image}
	if err := validateRepairApproval("repair", "operator-a", reviewPath, approvalPath, reviewSHA, approvalSHA, image, now, argv); err != nil {
		t.Fatalf("valid approval rejected: %v", err)
	}
}

func TestRepairApprovalFailsClosedOnTamperExpiryAndImageDrift(t *testing.T) {
	now := time.Date(2026, 8, 8, 2, 0, 0, 0, time.UTC)
	reviewPath, approvalPath, reviewSHA, approvalSHA, image := approvalFixtures(t, now)
	argv := []string{"alert-projection-reconcile", "--mode", "repair", "--review-package", reviewPath, "--approval-bundle", approvalPath, "--expected-review-sha256", reviewSHA, "--expected-approval-sha256", approvalSHA, "--expected-tool-image", image}
	tests := []struct {
		name, reviewSHA, approvalSHA, image string
		at                                  time.Time
	}{
		{"review hash", strings.Repeat("b", 64), approvalSHA, image, now},
		{"approval hash", reviewSHA, strings.Repeat("b", 64), image, now},
		{"image drift", reviewSHA, approvalSHA, "registry.example.test/other@sha256:" + strings.Repeat("a", 64), now},
		{"expired", reviewSHA, approvalSHA, image, now.Add(11 * time.Minute)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateRepairApproval("repair", "operator-a", reviewPath, approvalPath, test.reviewSHA, test.approvalSHA, test.image, test.at, argv); err == nil {
				t.Fatal("unsafe approval unexpectedly accepted")
			}
		})
	}
}

func TestRepairApprovalIsMandatoryOnlyForRepair(t *testing.T) {
	if err := validateRepairApproval("plan", "operator-a", "", "", "", "", "", time.Now(), nil); err != nil {
		t.Fatalf("plan mode unexpectedly requires repair approval: %v", err)
	}
	if err := validateRepairApproval("repair", "operator-a", "", "", "", "", "", time.Now(), nil); err == nil {
		t.Fatal("repair without approval artifacts unexpectedly accepted")
	}
}

func TestRepairApprovalRejectsRuntimeArgvDrift(t *testing.T) {
	now := time.Date(2026, 8, 8, 2, 0, 0, 0, time.UTC)
	reviewPath, approvalPath, reviewSHA, approvalSHA, image := approvalFixtures(t, now)
	argv := []string{"alert-projection-reconcile", "--mode", "repair", "--tenant", "different-tenant", "--review-package", reviewPath, "--approval-bundle", approvalPath, "--expected-review-sha256", reviewSHA, "--expected-approval-sha256", approvalSHA, "--expected-tool-image", image}
	if err := validateRepairApproval("repair", "operator-a", reviewPath, approvalPath, reviewSHA, approvalSHA, image, now, argv); err == nil {
		t.Fatal("runtime argv drift unexpectedly accepted")
	}
}
