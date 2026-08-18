package service

import (
	"context"
	"strings"
	"testing"
)

// §20 服务层纯校验路径(不触库):参数/枚举/幂等键门禁。
func TestTaskDefinitionServiceValidation(t *testing.T) {
	svc := NewTaskDefinitionService(nil)

	if _, _, err := svc.Create(context.Background(), "", "name", "op", "", ""); err == nil {
		t.Fatal("empty tenant must be rejected")
	}
	if _, _, err := svc.Create(context.Background(), "t", "", "op", "", ""); err == nil {
		t.Fatal("empty name must be rejected")
	}
	if _, _, err := svc.Create(context.Background(), "t", "n", "op", "NOT_A_CLASS", ""); err == nil {
		t.Fatal("unknown scheduling class must be rejected")
	}
	if _, err := svc.Activate(context.Background(), "t", "d", -1, "op"); err == nil {
		t.Fatal("negative expected revision must be rejected")
	}
	if _, err := svc.Suspend(context.Background(), "t", "d", -1, "op"); err == nil {
		t.Fatal("negative expected revision must be rejected")
	}
	if _, _, _, err := svc.SaveReportPolicy(context.Background(), "t", "d", "BOGUS_MODE", "", "", 30, ""); err == nil {
		t.Fatal("unknown report policy mode must be rejected")
	}
}

func TestPreflightServiceUnknownPlanSource(t *testing.T) {
	compiler := NewPlanCompiler()
	triggers := NewTriggerService(nil, NewDefaultPlanResolver(compiler), NewCustomPlanResolver(compiler), compiler)
	svc := NewPreflightService(triggers)

	_, err := svc.Preflight(context.Background(), SubmitRequest{
		TenantID: "t", TaskDefinitionID: "d", PlanSource: "BOGUS_SOURCE",
	})
	if err == nil || !strings.Contains(err.Error(), "BOGUS_SOURCE") {
		t.Fatalf("unknown plan source must be rejected, got %v", err)
	}
}

func TestRetryReportMissingKey(t *testing.T) {
	svc := NewHumanReportService(nil)
	if _, _, err := svc.RetryReport(context.Background(), "t", "rpt-1", ""); err == nil ||
		!strings.Contains(err.Error(), "client_idempotency_key") {
		t.Fatalf("missing idempotency key must be rejected, got %v", err)
	}
}
