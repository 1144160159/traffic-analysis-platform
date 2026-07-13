package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

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
	req := httptest.NewRequest("POST", "/api/v1/assets/discovery/runs", nil)
	req.Header.Set("X-Tenant-ID", "tenant-a")
	req.Header.Set("X-Scopes", authmodel.ScopeAssetRead)
	rr := httptest.NewRecorder()
	handler := &HTTPHandler{}

	if _, ok := handler.requireAssetDiscoveryWrite(rr, req); ok {
		t.Fatal("viewer asset read scope should not pass discovery write gate")
	}
	if rr.Code != 403 {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
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
