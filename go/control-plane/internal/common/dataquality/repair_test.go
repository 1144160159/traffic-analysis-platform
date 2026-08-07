package dataquality

import (
	"errors"
	"testing"
)

func TestRepairScopeRejectsCrossTenantAndOversizedBudget(t *testing.T) {
	scope := map[string]interface{}{
		"dataset_id": "flows_raw", "tenant_id": "tenant-b",
		"window_start": "2026-08-04T12:00:00Z", "window_end": "2026-08-04T12:30:00Z",
	}
	budget := map[string]interface{}{"max_rows": float64(1000), "max_duration_seconds": float64(60)}
	if err := validateRepairScope("tenant-a", scope, budget); err == nil {
		t.Fatal("cross-tenant repair scope must be rejected")
	}
	scope["tenant_id"] = "tenant-a"
	budget["max_rows"] = float64(100001)
	if err := validateRepairScope("tenant-a", scope, budget); err == nil {
		t.Fatal("oversized repair budget must be rejected")
	}
}

func TestRepairExecutionIsDefaultOffAndApprovalIsIndependent(t *testing.T) {
	record := RepairRecord{Status: "approval_pending", RequestedBy: "requester-a"}
	command := RepairTransitionCommand{Action: "approve", Actor: "requester-a"}
	if _, _, _, err := nextRepairStatus(record, command, false); !errors.Is(err, ErrRepairApprovalSeparation) {
		t.Fatalf("self approval must fail, got %v", err)
	}
	record.Status = "approved"
	command = RepairTransitionCommand{Action: "start_execution", Actor: "reviewer-b"}
	if _, _, _, err := nextRepairStatus(record, command, false); !errors.Is(err, ErrRepairExecutionDisabled) {
		t.Fatalf("disabled execution must fail closed, got %v", err)
	}
}

func TestRepairReconcileRequiresZeroDifference(t *testing.T) {
	record := RepairRecord{Status: "executed", RequestedBy: "requester-a"}
	command := RepairTransitionCommand{Action: "reconcile", Actor: "reconciler-a", Summary: map[string]interface{}{"all_match": true, "missing_count": float64(1), "extra_count": float64(0)}}
	if _, _, _, err := nextRepairStatus(record, command, true); err == nil {
		t.Fatal("non-zero reconciliation difference must not close a repair")
	}
	command.Summary["missing_count"] = float64(0)
	next, operation, eventStatus, err := nextRepairStatus(record, command, true)
	if err != nil || next != "reconciled" || operation != "reconciled" || eventStatus != "reconciled" {
		t.Fatalf("zero-difference reconcile rejected: next=%s operation=%s event=%s err=%v", next, operation, eventStatus, err)
	}
	command.Actor = "requester-a"
	if _, _, _, err := nextRepairStatus(record, command, true); !errors.Is(err, ErrRepairApprovalSeparation) {
		t.Fatalf("requester reconciliation must fail, got %v", err)
	}
}
