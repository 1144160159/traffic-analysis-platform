package api

import (
	"strings"
	"testing"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

func TestPlaybookTransitionTwoPersonApproval(t *testing.T) {
	draft := PlaybookDefinitionRecord{Stage: "draft"}
	stage, enabled, submittedBy, approvedBy, rejectionReason, auditAction, err := playbookTransition(draft, "submit", "author-1", "")
	if err != nil {
		t.Fatalf("submit draft: %v", err)
	}
	if stage != "approval_pending" || enabled || submittedBy != "author-1" || approvedBy != "" || rejectionReason != "" || auditAction != "PLAYBOOK_APPROVAL_SUBMITTED" {
		t.Fatalf("unexpected submit transition: stage=%s enabled=%t submitted=%s approved=%s rejection=%s audit=%s", stage, enabled, submittedBy, approvedBy, rejectionReason, auditAction)
	}

	pending := PlaybookDefinitionRecord{Stage: stage, SubmittedBy: submittedBy}
	if _, _, _, _, _, _, err := playbookTransition(pending, "approve", "author-1", ""); err == nil || !strings.Contains(err.Error(), "two-person") {
		t.Fatalf("same-person approval should fail with two-person error, got %v", err)
	}

	stage, enabled, submittedBy, approvedBy, rejectionReason, auditAction, err = playbookTransition(pending, "approve", "reviewer-2", "")
	if err != nil {
		t.Fatalf("independent approval: %v", err)
	}
	if stage != "approved" || !enabled || submittedBy != "author-1" || approvedBy != "reviewer-2" || rejectionReason != "" || auditAction != "PLAYBOOK_APPROVED" {
		t.Fatalf("unexpected approval transition: stage=%s enabled=%t submitted=%s approved=%s rejection=%s audit=%s", stage, enabled, submittedBy, approvedBy, rejectionReason, auditAction)
	}
}

func TestPlaybookTransitionRejectAndEnableLifecycle(t *testing.T) {
	pending := PlaybookDefinitionRecord{Stage: "approval_pending", SubmittedBy: "author-1"}
	if _, _, _, _, _, _, err := playbookTransition(pending, "reject", "reviewer-2", "short"); err == nil {
		t.Fatal("short rejection reason should fail")
	}
	stage, enabled, submittedBy, approvedBy, rejectionReason, auditAction, err := playbookTransition(pending, "reject", "reviewer-2", "evidence is incomplete")
	if err != nil {
		t.Fatalf("reject pending: %v", err)
	}
	if stage != "rejected" || enabled || submittedBy != "author-1" || approvedBy != "reviewer-2" || rejectionReason != "evidence is incomplete" || auditAction != "PLAYBOOK_REJECTED" {
		t.Fatalf("unexpected rejection transition: stage=%s enabled=%t submitted=%s approved=%s rejection=%s audit=%s", stage, enabled, submittedBy, approvedBy, rejectionReason, auditAction)
	}

	approved := PlaybookDefinitionRecord{Stage: "approved", Enabled: true, SubmittedBy: "author-1", ApprovedBy: "reviewer-2"}
	stage, enabled, _, _, _, auditAction, err = playbookTransition(approved, "disable", "operator-3", "")
	if err != nil || stage != "approved" || enabled || auditAction != "PLAYBOOK_DISABLED" {
		t.Fatalf("disable transition failed: stage=%s enabled=%t audit=%s err=%v", stage, enabled, auditAction, err)
	}
	disabled := approved
	disabled.Enabled = false
	stage, enabled, _, _, _, auditAction, err = playbookTransition(disabled, "enable", "operator-3", "")
	if err != nil || stage != "approved" || !enabled || auditAction != "PLAYBOOK_ENABLED" {
		t.Fatalf("enable transition failed: stage=%s enabled=%t audit=%s err=%v", stage, enabled, auditAction, err)
	}
}

func TestPlaybookPermissionGates(t *testing.T) {
	router := newAdvancedTestRouter(NewAdvancedHandler(nil, nil, nil, nil, nil))
	readDenied := doAdvancedRequestWithPermissions(t, router, "GET", "/api/v1/playbooks/catalog", "", []string{"viewer"}, []string{"user:read"})
	if readDenied.Code != 403 {
		t.Fatalf("catalog without alert read status=%d body=%s", readDenied.Code, readDenied.Body.String())
	}
	writeDenied := doAdvancedRequestWithPermissions(t, router, "POST", "/api/v1/playbooks/test/drill", `{}`, []string{"viewer"}, []string{authmodel.ScopePlaybookRead})
	if writeDenied.Code != 403 {
		t.Fatalf("drill without alert write status=%d body=%s", writeDenied.Code, writeDenied.Body.String())
	}
	approveDenied := doAdvancedRequestWithPermissions(t, router, "POST", "/api/v1/playbooks/test/approve", `{"expected_version":1}`, []string{"operator"}, []string{authmodel.ScopePlaybookWrite})
	if approveDenied.Code != 403 {
		t.Fatalf("approve without admin status=%d body=%s", approveDenied.Code, approveDenied.Body.String())
	}
	exportDenied := doAdvancedRequestWithPermissions(t, router, "GET", "/api/v1/playbooks/evidence/export", "", []string{"viewer"}, []string{authmodel.ScopePlaybookRead})
	if exportDenied.Code != 403 {
		t.Fatalf("export without alert export status=%d body=%s", exportDenied.Code, exportDenied.Body.String())
	}
}
