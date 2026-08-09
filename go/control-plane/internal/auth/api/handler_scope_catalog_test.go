package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/middleware"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

func TestGetScopeCatalogRequiresAdminReadAndReturnsContractEnvelope(t *testing.T) {
	handler := &Handler{}

	for _, test := range []struct {
		name       string
		claims     *model.Claims
		wantStatus int
	}{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorized},
		{name: "under scoped", claims: &model.Claims{TenantID: "tenant-a", Permissions: []string{model.ScopeUserRead}}, wantStatus: http.StatusForbidden},
		{name: "admin read", claims: &model.Claims{TenantID: "tenant-a", Permissions: []string{model.ScopeAdminRead}}, wantStatus: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/scopes", nil)
			ctx := context.WithValue(request.Context(), httpx.ContextKeyTraceID, "trace-scope-catalog")
			ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, "tenant-a")
			if test.claims != nil {
				ctx = context.WithValue(ctx, middleware.ContextKeyClaims, test.claims)
			}
			recorder := httptest.NewRecorder()
			handler.GetScopeCatalog(recorder, request.WithContext(ctx))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			if test.wantStatus != http.StatusOK {
				var response httpx.Response
				if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if response.Error == nil || response.Error.OperationID != "getScopeCatalog" {
					t.Fatalf("missing operation-bound error: %+v", response)
				}
				return
			}
			var response httpx.ContractResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode success response: %v", err)
			}
			if response.Meta.OperationID != "getScopeCatalog" || response.Meta.TenantID != "tenant-a" || response.Meta.SnapshotID != "iam-scope-catalog-v1" {
				t.Fatalf("scope contract metadata=%+v", response.Meta)
			}
		})
	}
}
