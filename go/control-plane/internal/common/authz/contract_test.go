package authz

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// TestEmbeddedPolicyParityWithContract 内嵌副本必须与 contracts/authz 权威副本逐字节一致。
func TestEmbeddedPolicyParityWithContract(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	// authz/contract_test.go -> repo_root/contracts/authz/m10-minimal-role-policy.v1.json
	contractPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "..", "contracts", "authz", "m10-minimal-role-policy.v1.json")
	contractRaw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	sum := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	if sum(contractRaw) != sum(embeddedPolicyJSON) {
		t.Fatal("embedded policy differs from contracts/authz copy; re-sync go/control-plane/internal/common/authz/policy/")
	}
}

func TestParsePolicy(t *testing.T) {
	p, err := DefaultPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if p.OpCount != 178 {
		t.Fatalf("op count = %d, want 178", p.OpCount)
	}
	if len(p.Roles) != 5 {
		t.Fatalf("roles = %d, want 5", len(p.Roles))
	}
	op := p.OperationFor("GET", "/api/v1/auth/scopes")
	if op == nil || op.OperationID != "getScopeCatalog" || op.RequiredScope != "admin:read" {
		t.Fatalf("getScopeCatalog lookup failed: %+v", op)
	}
	op = p.OperationFor("PATCH", "/v1/auth/me")
	if op == nil || op.OperationID != "updateAuthenticatedUserProfileAtomic" || op.RequiredScope != "user:write" {
		t.Fatalf("updateAuthenticatedUserProfileAtomic lookup failed: %+v", op)
	}
	if p.OperationFor("GET", "/not/in/contract") != nil {
		t.Fatal("unmatched path must return nil")
	}
}

func TestScopeCovers(t *testing.T) {
	cases := []struct {
		held, required string
		want           bool
	}{
		{"alert:read", "alert:read", true},
		{"admin:*", "admin:read", true},
		{"admin:*", "admin:*", true},
		{"admin:*", "alert:read", false},
		{"*", "alert:read", true},
		{"alert:read", "alert:write", false},
		{"alert:read", "alert", false},
	}
	for _, c := range cases {
		if got := ScopeCovers(c.held, c.required); got != c.want {
			t.Errorf("ScopeCovers(%q,%q)=%v want %v", c.held, c.required, got, c.want)
		}
	}
}

func TestEnforceContract(t *testing.T) {
	admin := &Principal{Roles: []string{"admin"}, Permissions: []string{"admin:*", "user:write"}}
	viewer := &Principal{Roles: []string{"viewer"}, Permissions: []string{"alert:read", "asset:read"}}
	provider := func(r *http.Request) *Principal {
		switch r.Header.Get("X-Test-Principal") {
		case "admin":
			return admin
		case "viewer":
			return viewer
		default:
			return nil
		}
	}
	run := func(mode, principal, path, method string) int {
		h := EnforceContract(provider, mode, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest(method, path, nil)
		if principal != "" {
			req.Header.Set("X-Test-Principal", principal)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if got := run("enforce", "admin", "/api/v1/auth/scopes", "GET"); got != 200 {
		t.Errorf("admin on getScopeCatalog = %d, want 200", got)
	}
	if got := run("enforce", "viewer", "/api/v1/auth/scopes", "GET"); got != 403 {
		t.Errorf("viewer on getScopeCatalog = %d, want 403", got)
	}
	if got := run("enforce", "admin", "/api/v1/auth/me/password", "POST"); got != 200 {
		t.Errorf("admin(user:write via admin:*) on changePassword = %d, want 200", got)
	}
	if got := run("enforce", "", "/api/v1/auth/scopes", "GET"); got != 401 {
		t.Errorf("no principal on getScopeCatalog = %d, want 401", got)
	}
	// 未命中契约:放行且不需要 principal
	if got := run("enforce", "", "/api/v1/auth/captcha", "GET"); got != 200 {
		t.Errorf("non-contract path = %d, want 200", got)
	}
	// shadow:拒绝只审计不阻断
	if got := run("shadow", "viewer", "/api/v1/auth/scopes", "GET"); got != 200 {
		t.Errorf("shadow viewer = %d, want 200 (pass-through)", got)
	}
	// ignorePaths
	if got := run("enforce", "", "/internal/v1/audit/batches", "POST"); got != 401 {
		t.Errorf("internal op without principal = %d, want 401 (contract match)", got)
	}
	ignored := EnforceContract(provider, "enforce", []string{"/internal/v1/audit/batches"}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/internal/v1/audit/batches", nil)
	rec := httptest.NewRecorder()
	ignored.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("ignored internal op = %d, want 200", rec.Code)
	}
}

func TestMapOIDCRoles(t *testing.T) {
	got := MapOIDCRoles([]string{"traffic-admin", "offline_access", "uma_authorization"})
	if len(got) != 1 || got[0] != "admin" {
		t.Fatalf("MapOIDCRoles = %v", got)
	}
	got = MapOIDCRoles([]string{"offline_access"})
	if len(got) != 1 || got[0] != "viewer" {
		t.Fatalf("empty mapping = %v", got)
	}
	got = MapOIDCRoles([]string{"traffic-analyst", "traffic-operator"})
	if len(got) != 2 || got[0] != "analyst" || got[1] != "operator" {
		t.Fatalf("multi mapping = %v", got)
	}
}

func TestExtractOIDCRoles(t *testing.T) {
	claims := jwt.MapClaims{
		"realm_access": map[string]interface{}{"roles": []interface{}{"traffic-admin", "offline_access"}},
		"resource_access": map[string]interface{}{
			"traffic-ui": map[string]interface{}{"roles": []interface{}{"traffic-viewer"}},
		},
	}
	roles := extractOIDCRoles(claims)
	found := map[string]bool{}
	for _, r := range roles {
		found[r] = true
	}
	if !found["traffic-admin"] || !found["traffic-viewer"] || !found["offline_access"] {
		t.Fatalf("extractOIDCRoles = %v", roles)
	}
}

func TestPrincipalFromClaims(t *testing.T) {
	m := New(Config{}, nil)
	claims := jwt.MapClaims{
		"sub":                "u1",
		"preferred_username": "alice",
		"realm_access":       map[string]interface{}{"roles": []interface{}{"traffic-admin"}},
	}
	p := m.principalFromClaims(claims, "default")
	if p.Subject != "u1" || p.Username != "alice" || p.TenantID != "default" {
		t.Fatalf("principal = %+v", p)
	}
	if !p.HasScope("admin:read") || !p.HasScope("user:write") {
		t.Fatalf("admin principal must cover admin:read and user:write via policy scopes")
	}
	if p.HasScope("admin:cross-tenant") {
		t.Fatalf("admin:cross-tenant must not be granted by policy (explicit exclusion)")
	}
}

func TestOperationForTemplatedPaths(t *testing.T) {
	p, err := DefaultPolicy()
	if err != nil {
		t.Fatal(err)
	}
	op := p.OperationFor("POST", "/api/v1/alerts/alert-123/feedback")
	if op == nil || op.OperationID != "adjudicateAlertFeedback" || op.RequiredScope != "alert:write" {
		t.Fatalf("templated lookup failed: %+v", op)
	}
	op = p.OperationFor("GET", "/v1/models/m-1/versions/active")
	if op == nil || op.OperationID != "getActiveModelVersion" || op.RequiredScope != "model:read" {
		t.Fatalf("model templated lookup failed: %+v", op)
	}
	// 段数不一致不命中
	if p.OperationFor("POST", "/api/v1/alerts/alert-123/feedback/extra") != nil {
		t.Fatal("extra segment must not match")
	}
	// 模板段不允许空段
	if p.OperationFor("POST", "/api/v1/alerts//feedback") != nil {
		t.Fatal("empty template segment must not match")
	}
	// 精确匹配优先于模板
	op = p.OperationFor("GET", "/api/v1/auth/scopes")
	if op == nil || op.OperationID != "getScopeCatalog" {
		t.Fatalf("exact match lost: %+v", op)
	}
}
