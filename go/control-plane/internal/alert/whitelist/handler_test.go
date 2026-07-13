package whitelist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type testClaims struct {
	userID      string
	tenantID    string
	username    string
	roles       []string
	permissions []string
}

func (c testClaims) GetUserID() string        { return c.userID }
func (c testClaims) GetTenantID() string      { return c.tenantID }
func (c testClaims) GetUsername() string      { return c.username }
func (c testClaims) GetRoles() []string       { return c.roles }
func (c testClaims) GetPermissions() []string { return c.permissions }
func (c testClaims) GetEmail() string         { return c.username + "@local" }
func (c testClaims) GetSessionID() string     { return "test-session" }
func (c testClaims) HasRole(role string) bool {
	for _, granted := range c.roles {
		if granted == role {
			return true
		}
	}
	return false
}
func (c testClaims) HasPermission(permission string) bool {
	for _, granted := range c.permissions {
		if permissionMatches(granted, permission) {
			return true
		}
	}
	return false
}

func TestWhitelistGovernanceRequiresAlertWritePermission(t *testing.T) {
	handler := NewHandler(nil, nil)
	cases := []struct {
		name   string
		method string
		path   string
		body   string
		call   func(http.ResponseWriter, *http.Request)
	}{
		{
			name:   "create",
			method: http.MethodPost,
			path:   "/api/v1/whitelist",
			body:   `{"type":"domain","value":"viewer.example.test"}`,
			call:   handler.Create,
		},
		{
			name:   "update",
			method: http.MethodPatch,
			path:   "/api/v1/whitelist/entry-001",
			body:   `{"status":"disabled"}`,
			call:   handler.Update,
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			path:   "/api/v1/whitelist/entry-001",
			call:   handler.Delete,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req = requestWithClaims(req, whitelistViewerClaims())
			recorder := httptest.NewRecorder()

			tc.call(recorder, req)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("expected viewer to be rejected before repository access, got status %d body %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "alert:write required") {
				t.Fatalf("expected alert:write permission error, got body %s", recorder.Body.String())
			}
		})
	}
}

func TestWhitelistGovernanceAdminReachesRepositoryGate(t *testing.T) {
	handler := NewHandler(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/whitelist", strings.NewReader(`{"type":"domain","value":"admin.example.test"}`))
	req = requestWithClaims(req, whitelistAdminClaims())
	recorder := httptest.NewRecorder()

	handler.Create(recorder, req)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected admin to pass permission and hit repository gate, got status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestWhitelistGovernanceRoutesIncludePatch(t *testing.T) {
	handler := NewHandler(nil, nil)
	router := mux.NewRouter()
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	handler.RegisterRoutes(apiRouter)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/whitelist/entry-001", strings.NewReader(`{"status":"disabled"}`))
	req = requestWithClaims(req, whitelistAdminClaims())
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected PATCH /api/v1/whitelist/{id} to be registered, got 404 body %s", recorder.Body.String())
	}
}

func requestWithClaims(req *http.Request, claims testClaims) *http.Request {
	ctx := context.WithValue(req.Context(), httpx.ContextKeyClaims, claims)
	ctx = context.WithValue(ctx, httpx.ContextKeyUserID, claims.userID)
	ctx = context.WithValue(ctx, httpx.ContextKeyTenantID, claims.tenantID)
	ctx = context.WithValue(ctx, httpx.ContextKeyRoles, claims.roles)
	ctx = context.WithValue(ctx, httpx.ContextKeyPermissions, claims.permissions)
	return req.WithContext(ctx)
}

func whitelistViewerClaims() testClaims {
	return testClaims{
		userID:      "00000000-0000-0000-0000-000000000001",
		tenantID:    "default",
		username:    "codex-viewer",
		roles:       []string{"viewer"},
		permissions: []string{"user:read", "audit:read", "alert:read"},
	}
}

func whitelistAdminClaims() testClaims {
	return testClaims{
		userID:      "00000000-0000-0000-0000-000000000002",
		tenantID:    "default",
		username:    "codex-admin",
		roles:       []string{"admin"},
		permissions: []string{"admin:*", "alert:write"},
	}
}
