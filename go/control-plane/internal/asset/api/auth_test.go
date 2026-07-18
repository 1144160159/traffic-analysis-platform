package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

func TestRequireAssetReadEnforcesScopeAndVerifiedTenant(t *testing.T) {
	const signingKey = "asset-read-test-signing-key"
	tests := []struct {
		name       string
		tenant     string
		scopes     []string
		withToken  bool
		wantOK     bool
		wantStatus int
	}{
		{name: "asset read", tenant: "tenant-a", scopes: []string{authmodel.ScopeAssetRead}, withToken: true, wantOK: true},
		{name: "asset wildcard", tenant: "tenant-a", scopes: []string{"asset:*"}, withToken: true, wantOK: true},
		{name: "wrong scope", tenant: "tenant-a", scopes: []string{authmodel.ScopeGraphRead}, withToken: true, wantStatus: http.StatusForbidden},
		{name: "missing tenant", scopes: []string{authmodel.ScopeAssetRead}, withToken: true, wantStatus: http.StatusForbidden},
		{name: "missing authorization", tenant: "tenant-a", wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
			if tc.withToken {
				req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, tc.tenant, tc.scopes))
			}
			rr := httptest.NewRecorder()
			identity, ok := (&HTTPHandler{jwtSigningKey: signingKey}).requireAssetRead(rr, req)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v; status=%d body=%s", ok, tc.wantOK, rr.Code, rr.Body.String())
			}
			if tc.wantOK {
				if identity.TenantID != tc.tenant {
					t.Fatalf("tenant = %q, want %q", identity.TenantID, tc.tenant)
				}
				return
			}
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rr.Code, tc.wantStatus, rr.Body.String())
			}
		})
	}
}

func TestRequestIdentityRejectsSpoofedIdentityHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req.Header.Set("X-Scopes", authmodel.ScopeAll)
	req.Header.Set("X-User-ID", uuid.NewString())
	rr := httptest.NewRecorder()
	if _, ok := (&HTTPHandler{jwtSigningKey: "configured"}).requireAssetRead(rr, req); ok {
		t.Fatal("unsigned identity headers must not pass the asset read gate")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusUnauthorized, rr.Body.String())
	}
}

func TestTenantFromRequestIgnoresQueryOverride(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?tenant_id=tenant-b", nil)
	req.Header.Set("X-Tenant-ID", "tenant-a")
	if got := tenantFromRequest(req); got != "tenant-a" {
		t.Fatalf("tenant = %q, want trusted header tenant-a", got)
	}
}

func TestRequestIdentityExtractsSignedAccessTokenScopes(t *testing.T) {
	signingKey := "asset-discovery-test-signing-key"
	userID := uuid.New()
	claims := &authmodel.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		UserID:      userID,
		TenantID:    "tenant-a",
		Username:    "asset-admin",
		Permissions: []string{authmodel.ScopeAssetDiscover},
		TokenType:   authmodel.JWTTokenAccess,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(signingKey))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/v1/assets/discovery/runs", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	handler := &HTTPHandler{jwtSigningKey: signingKey}

	identity, status, message := handler.requestIdentity(req)
	if status != 0 || message != "" {
		t.Fatalf("requestIdentity status=%d message=%q", status, message)
	}
	if identity.TenantID != "tenant-a" || identity.UserID != userID.String() || identity.Username != "asset-admin" {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if !hasDiscoveryWriteScope(identity.Scopes) {
		t.Fatalf("expected discovery write scope in %#v", identity.Scopes)
	}
}

func TestRequireAssetDiscoveryWriteRejectsViewerScope(t *testing.T) {
	const signingKey = "asset-discovery-viewer-test-signing-key"
	req := httptest.NewRequest("POST", "/api/v1/assets/discovery/runs", nil)
	req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", []string{authmodel.ScopeAssetRead}))
	rr := httptest.NewRecorder()
	handler := &HTTPHandler{jwtSigningKey: signingKey}

	if _, ok := handler.requireAssetDiscoveryWrite(rr, req); ok {
		t.Fatal("viewer asset read scope should not pass discovery write gate")
	}
	if rr.Code != 403 {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func signAccessToken(t *testing.T, signingKey, tenantID string, permissions []string) string {
	t.Helper()
	now := time.Now()
	claims := &authmodel.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID:      uuid.New(),
		TenantID:    tenantID,
		Username:    "asset-test-user",
		Permissions: permissions,
		TokenType:   authmodel.JWTTokenAccess,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(signingKey))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func TestHasDiscoveryWriteScopeAcceptsWildcards(t *testing.T) {
	for _, scopes := range [][]string{
		{authmodel.ScopeAssetDiscover},
		{"asset:*"},
		{authmodel.ScopeAdminAll},
		{authmodel.ScopeAll},
	} {
		if !hasDiscoveryWriteScope(scopes) {
			t.Fatalf("scope set %v should pass discovery write gate", scopes)
		}
	}
}
