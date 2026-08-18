package api

import (
	"bytes"
	"encoding/json"
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
		{name: "missing tenant", scopes: []string{authmodel.ScopeAssetRead}, withToken: true, wantStatus: http.StatusUnauthorized},
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

func TestTenantFromRequestDoesNotTrustUnsignedRequestData(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?tenant_id=tenant-b", nil)
	req.Header.Set("X-Tenant-ID", "tenant-a")
	if got := tenantFromRequest(req); got != "" {
		t.Fatalf("tenant = %q, want empty without authenticated context", got)
	}
}

func TestRequestIdentityRejectsCrossTenantAssertion(t *testing.T) {
	const signingKey = "asset-cross-tenant-test-signing-key"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets?tenant_id=tenant-b", nil)
	req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", []string{authmodel.ScopeAssetRead}))
	identity, status, message := (&HTTPHandler{jwtSigningKey: signingKey}).requestIdentity(req)
	if status != http.StatusForbidden || message != "cross-tenant access denied" || identity.TenantID != "" {
		t.Fatalf("identity=%+v status=%d message=%q", identity, status, message)
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

func TestRequireAssetExportUsesDedicatedScope(t *testing.T) {
	const signingKey = "asset-export-scope-test-signing-key"
	for _, tc := range []struct {
		name       string
		scopes     []string
		wantStatus int
		wantOK     bool
	}{
		{name: "dedicated export", scopes: []string{authmodel.ScopeAssetExport}, wantOK: true},
		{name: "asset wildcard", scopes: []string{authmodel.ScopeAssetAll}, wantOK: true},
		{name: "read only", scopes: []string{authmodel.ScopeAssetRead}, wantStatus: http.StatusForbidden},
		{name: "discover only", scopes: []string{authmodel.ScopeAssetDiscover}, wantStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/assets/exports", nil)
			req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", tc.scopes))
			rr := httptest.NewRecorder()
			identity, ok := (&HTTPHandler{jwtSigningKey: signingKey}).requireAssetExport(rr, req)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want=%v status=%d body=%s", ok, tc.wantOK, rr.Code, rr.Body.String())
			}
			if tc.wantOK && identity.TenantID != "tenant-a" {
				t.Fatalf("tenant=%q want tenant-a", identity.TenantID)
			}
			if !tc.wantOK && rr.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, tc.wantStatus, rr.Body.String())
			}
		})
	}
}

func TestRequireAssetGovernanceUsesDedicatedScope(t *testing.T) {
	const signingKey = "asset-governance-scope-test-signing-key"
	for _, tc := range []struct {
		name   string
		scopes []string
		wantOK bool
	}{
		{name: "dedicated governance", scopes: []string{authmodel.ScopeAssetGovern}, wantOK: true},
		{name: "asset wildcard", scopes: []string{authmodel.ScopeAssetAll}, wantOK: true},
		{name: "read only", scopes: []string{authmodel.ScopeAssetRead}},
		{name: "discover only", scopes: []string{authmodel.ScopeAssetDiscover}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/assets/11111111-1111-1111-1111-111111111111/governance/work-orders", nil)
			req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", tc.scopes))
			rr := httptest.NewRecorder()
			identity, ok := (&HTTPHandler{jwtSigningKey: signingKey}).requireAssetGovernance(rr, req)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want=%v status=%d body=%s", ok, tc.wantOK, rr.Code, rr.Body.String())
			}
			if ok && identity.TenantID != "tenant-a" {
				t.Fatalf("tenant=%q", identity.TenantID)
			}
			if !ok && rr.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestAssetPreferenceUsesStableUserID(t *testing.T) {
	identity := requestIdentity{UserID: "11111111-1111-1111-1111-111111111111", Username: "renameable-user"}
	if got := assetPreferenceUserID(identity); got != identity.UserID {
		t.Fatalf("preference user=%q want stable user_id %q", got, identity.UserID)
	}
}

func TestAssetExportAuthorizationFailureUsesContractEnvelope(t *testing.T) {
	const signingKey = "asset-export-envelope-test-key"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets/exports", nil)
	req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", []string{authmodel.ScopeAssetRead}))
	req.Header.Set("X-Trace-ID", "trace-export-denied")
	rr := httptest.NewRecorder()
	if _, ok := (&HTTPHandler{jwtSigningKey: signingKey}).requireAssetExport(rr, req); ok {
		t.Fatal("asset:read must not pass asset:export")
	}
	var envelope struct {
		Meta struct {
			SnapshotID string `json:"snapshot_id"`
			AsOf       string `json:"as_of"`
			TraceID    string `json:"trace_id"`
		} `json:"meta"`
		Error struct {
			Code    string `json:"code"`
			TraceID string `json:"trace_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if rr.Code != http.StatusForbidden || envelope.Meta.SnapshotID == "" || envelope.Meta.AsOf == "" || envelope.Meta.TraceID != "trace-export-denied" || envelope.Error.Code != "forbidden" || envelope.Error.TraceID != envelope.Meta.TraceID {
		t.Fatalf("unexpected envelope status=%d body=%s", rr.Code, rr.Body.String())
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

func TestAtomicAssetUpsertRejectsBodyTenantConflictBeforeDatabase(t *testing.T) {
	const signingKey = "asset-upsert-tenant-test-key"
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/assets",
		bytes.NewBufferString(`{"asset":{"tenant_id":"tenant-b","mac_address":"00:11:22:33:44:55"},"expected_revision":0}`),
	)
	req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", []string{authmodel.ScopeAssetDiscover}))
	req.Header.Set("Idempotency-Key", "asset-upsert-tenant-conflict")
	rr := httptest.NewRecorder()
	(&HTTPHandler{jwtSigningKey: signingKey}).ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusConflict, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte(`"code":"tenant_conflict"`)) {
		t.Fatalf("missing contract error: %s", rr.Body.String())
	}
}

func TestAtomicAssetUpsertRejectsViewerBeforeDatabase(t *testing.T) {
	const signingKey = "asset-upsert-viewer-test-key"
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/assets",
		bytes.NewBufferString(`{"asset":{"mac_address":"00:11:22:33:44:55"},"expected_revision":0}`),
	)
	req.Header.Set("Authorization", "Bearer "+signAccessToken(t, signingKey, "tenant-a", []string{authmodel.ScopeAssetRead}))
	rr := httptest.NewRecorder()
	(&HTTPHandler{jwtSigningKey: signingKey}).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
	}
}
