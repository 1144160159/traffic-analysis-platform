package service

import (
	"context"
	"errors"
	"testing"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/analysis/contract"
)

func TestSubmitTemplateLoaderFailurePropagates(t *testing.T) {
	compiler := NewPlanCompiler()
	svc := NewTriggerService(nil, NewDefaultPlanResolver(compiler), NewCustomPlanResolver(compiler), compiler)
	want := errors.New("loader boom")
	var gotTenant, gotDef string
	svc.SetTemplateLoader(func(_ context.Context, tenantID, taskDefinitionID string) (*DefaultTemplate, CatalogSnapshot, error) {
		gotTenant, gotDef = tenantID, taskDefinitionID
		return nil, CatalogSnapshot{}, want
	})
	_, err := svc.Submit(context.Background(), SubmitRequest{
		TenantID:             "t1",
		TaskDefinitionID:     "d1",
		PlanSource:           "AUTO_DEFAULT",
		ClientIdempotencyKey: "k1",
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected loader error to propagate, got %v", err)
	}
	if gotTenant != "t1" || gotDef != "d1" {
		t.Fatalf("loader received wrong args: %q %q", gotTenant, gotDef)
	}
}

func TestSubmitMissingIdempotencyKeyRejectedBeforeLoader(t *testing.T) {
	compiler := NewPlanCompiler()
	svc := NewTriggerService(nil, NewDefaultPlanResolver(compiler), NewCustomPlanResolver(compiler), compiler)
	called := false
	svc.SetTemplateLoader(func(context.Context, string, string) (*DefaultTemplate, CatalogSnapshot, error) {
		called = true
		return nil, CatalogSnapshot{}, nil
	})
	_, err := svc.Submit(context.Background(), SubmitRequest{
		TenantID:         "t1",
		TaskDefinitionID: "d1",
		PlanSource:       "AUTO_DEFAULT",
	})
	if err == nil || !containsCode(err, contract.ErrCodeMissingIdempotencyKey) {
		t.Fatalf("expected missing idempotency key error, got %v", err)
	}
	if called {
		t.Fatalf("loader must not run when idempotency key is missing")
	}
}

func containsCode(err error, code commonerrors.ErrorCode) bool {
	var ae *commonerrors.AppError
	return errors.As(err, &ae) && ae.Code == code
}
