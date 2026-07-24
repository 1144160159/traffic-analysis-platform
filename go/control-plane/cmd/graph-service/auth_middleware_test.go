package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

type graphTestClaims struct {
	tenantID    string
	permissions []string
}

func (c graphTestClaims) GetUserID() string        { return "user-1" }
func (c graphTestClaims) GetTenantID() string      { return c.tenantID }
func (c graphTestClaims) GetUsername() string      { return "graph-test" }
func (c graphTestClaims) GetRoles() []string       { return []string{"analyst"} }
func (c graphTestClaims) GetPermissions() []string { return c.permissions }

type graphTestValidator struct {
	claims httpx.Claims
	err    error
}

func (v graphTestValidator) ValidateToken(string) (httpx.Claims, error) {
	return v.claims, v.err
}

func TestProtectGraphBusinessAPIRequiresBearerToken(t *testing.T) {
	handler := protectGraphBusinessAPI(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), httpx.Auth(graphTestValidator{claims: graphTestClaims{tenantID: "default", permissions: []string{"graph:read"}}}, nil))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/graph/workbench", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer token, got %d", recorder.Code)
	}
}

func TestProtectGraphBusinessAPIAcceptsGraphReadAndUsesTokenTenant(t *testing.T) {
	handler := protectGraphBusinessAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tenantID := httpx.GetTenantID(r.Context()); tenantID != "tenant-from-token" {
			t.Fatalf("expected token tenant, got %q", tenantID)
		}
		w.WriteHeader(http.StatusNoContent)
	}), httpx.Auth(graphTestValidator{claims: graphTestClaims{tenantID: "tenant-from-token", permissions: []string{"graph:read"}}}, nil))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/graph/workbench?tenant_id=other", nil)
	request.Header.Set("Authorization", "Bearer valid")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected protected handler success, got %d", recorder.Code)
	}
}

func TestProtectGraphBusinessAPIRejectsInvalidTokenAndMissingScope(t *testing.T) {
	tests := []struct {
		name      string
		validator graphTestValidator
		want      int
	}{
		{name: "invalid token", validator: graphTestValidator{err: errors.New("invalid")}, want: http.StatusUnauthorized},
		{name: "missing graph read", validator: graphTestValidator{claims: graphTestClaims{tenantID: "default", permissions: []string{"asset:read"}}}, want: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := protectGraphBusinessAPI(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}), httpx.Auth(test.validator, nil))
			request := httptest.NewRequest(http.MethodGet, "/api/v1/graph/workbench", nil)
			request.Header.Set("Authorization", "Bearer token")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("expected %d, got %d", test.want, recorder.Code)
			}
		})
	}
}

func TestProtectGraphBusinessAPIPreservesPublicHealthAndOptions(t *testing.T) {
	handler := protectGraphBusinessAPI(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), httpx.Auth(graphTestValidator{claims: graphTestClaims{}}, nil))
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/health", nil),
		httptest.NewRequest(http.MethodOptions, "/api/v1/graph/workbench", nil),
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("expected public request to pass, got %d", recorder.Code)
		}
	}
}
