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

func TestWhitelistTransitionRequiresTwoPersonApproval(t *testing.T) {
	entry := &Entry{ID: "entry-001", Status: "pending", ApprovalStatus: "pending", CreatedBy: "author-1", Version: 2}
	active := "active"
	approved := "approved"
	req := UpdateRequest{Status: &active, ApprovalStatus: &approved}

	if code, _ := validateWhitelistTransition(entry, req, "author-1", true); code != "WHITELIST_TWO_PERSON_REQUIRED" {
		t.Fatalf("expected creator approval to require two people, got %q", code)
	}
	if code, message := validateWhitelistTransition(entry, req, "reviewer-2", true); code != "" {
		t.Fatalf("expected distinct admin reviewer to approve, got %q: %s", code, message)
	}
	if code, _ := validateWhitelistTransition(entry, req, "reviewer-2", false); code != "PERMISSION_DENIED" {
		t.Fatalf("expected non-admin reviewer rejection, got %q", code)
	}
}

func TestWhitelistTransitionRejectsLifecycleSkips(t *testing.T) {
	entry := &Entry{ID: "entry-001", Status: "draft", ApprovalStatus: "draft", CreatedBy: "author-1", Version: 1}
	active := "active"
	approved := "approved"
	if code, _ := validateWhitelistTransition(entry, UpdateRequest{Status: &active, ApprovalStatus: &approved}, "reviewer-2", true); code != "INVALID_TRANSITION" {
		t.Fatalf("expected direct draft activation to fail, got %q", code)
	}

	pending := "pending"
	if code, message := validateWhitelistTransition(entry, UpdateRequest{Status: &pending, ApprovalStatus: &pending}, "author-1", false); code != "" {
		t.Fatalf("expected author to submit a draft, got %q: %s", code, message)
	}

	disabled := &Entry{ID: "entry-001", Status: "disabled", ApprovalStatus: "approved", Version: 3}
	if code, _ := validateWhitelistTransition(disabled, UpdateRequest{Status: &active}, "reviewer-2", true); code != "INVALID_TRANSITION" {
		t.Fatalf("expected disabled entry reactivation to fail, got %q", code)
	}

	activeApproved := &Entry{ID: "entry-002", Status: "active", ApprovalStatus: "approved", Version: 3}
	if code, _ := validateWhitelistTransition(activeApproved, UpdateRequest{Status: &pending}, "reviewer-2", true); code != "INVALID_TRANSITION" {
		t.Fatalf("expected partial active/approved to pending/approved transition to fail, got %q", code)
	}
	draft := "draft"
	if code, _ := validateWhitelistTransition(activeApproved, UpdateRequest{Status: &draft}, "reviewer-2", true); code != "INVALID_TRANSITION" {
		t.Fatalf("expected active/approved to draft/approved transition to fail, got %q", code)
	}
}

func TestWhitelistStatePairsAreClosed(t *testing.T) {
	valid := [][2]string{{"draft", "draft"}, {"pending", "pending"}, {"active", "approved"}, {"disabled", "approved"}, {"disabled", "rejected"}}
	for _, pair := range valid {
		if !validWhitelistStatePair(pair[0], pair[1]) {
			t.Fatalf("expected valid state pair %s/%s", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{{"active", "draft"}, {"pending", "approved"}, {"draft", "approved"}, {"disabled", "pending"}} {
		if validWhitelistStatePair(pair[0], pair[1]) {
			t.Fatalf("unexpected valid state pair %s/%s", pair[0], pair[1])
		}
	}
}

func TestWhitelistEntrySupportsGovernanceTypesAndRisk(t *testing.T) {
	for _, value := range []string{"ip", "domain", "subnet", "fingerprint", "asset", "account", "rule", "model"} {
		if got := normalizeType(value); got != value {
			t.Fatalf("expected whitelist type %q, got %q", value, got)
		}
	}
	if got := normalizeType("unknown"); got != "" {
		t.Fatalf("unknown type must not be accepted, got %q", got)
	}
	if got := normalizeRiskLevel("HIGH"); got != "high" {
		t.Fatalf("expected normalized high risk, got %q", got)
	}
	if got := normalizeRiskLevel(""); got != "medium" {
		t.Fatalf("expected medium default risk, got %q", got)
	}
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

func TestWhitelistGovernanceRequiresAlertReadPermission(t *testing.T) {
	handler := NewHandler(nil, nil)
	for _, tc := range []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
		body string
	}{
		{name: "list", call: handler.List},
		{name: "check", call: handler.Check, body: `{"type":"domain","value":"example.test"}`},
	} {
		t.Run(tc.name+" denied", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/whitelist", strings.NewReader(tc.body))
			req = requestWithClaims(req, testClaims{userID: "no-read", tenantID: "default", roles: []string{"viewer"}, permissions: []string{"user:read"}})
			recorder := httptest.NewRecorder()
			tc.call(recorder, req)
			if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "alert:read required") {
				t.Fatalf("expected alert:read denial, got status %d body %s", recorder.Code, recorder.Body.String())
			}
		})
		t.Run(tc.name+" allowed", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/whitelist", strings.NewReader(tc.body))
			req = requestWithClaims(req, whitelistViewerClaims())
			recorder := httptest.NewRecorder()
			tc.call(recorder, req)
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected alert:read to reach repository gate, got status %d body %s", recorder.Code, recorder.Body.String())
			}
		})
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
