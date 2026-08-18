package authz

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// 生成测试 RSA 密钥 + 本地 JWKS 端点 + 签发 token,验证租户绑定三态。
func TestMiddlewareTenantBinding(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "test-kid"
	jwksHandler := func(w http.ResponseWriter, _ *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1})
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{"kid": kid, "kty": "RSA", "alg": "RS256", "n": n, "e": e}},
		})
	}
	jwksSrv := httptest.NewServer(http.HandlerFunc(jwksHandler))
	defer jwksSrv.Close()

	mw := New(Config{JWKSURL: jwksSrv.URL, DefaultTenant: "default", Mode: "enforce"}, zap.NewNop())

	sign := func(tenantClaim interface{}) string {
		claims := jwt.MapClaims{
			"sub": "u1", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
		}
		if tenantClaim != nil {
			claims["tenant_id"] = tenantClaim
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = kid
		s, err := tok.SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	call := func(token, headerTenant string) int {
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if headerTenant != "" {
			req.Header.Set("X-Tenant-ID", headerTenant)
		}
		rec := httptest.NewRecorder()
		mw.Handler(inner).ServeHTTP(rec, req)
		return rec.Code
	}

	if got := call(sign("default"), "default"); got != 200 {
		t.Fatalf("matching tenant: got %d", got)
	}
	if got := call(sign("tenant-a"), "tenant-b"); got != 403 {
		t.Fatalf("mismatched tenant: got %d", got)
	}
	if got := call(sign(nil), "default"); got != 200 {
		t.Fatalf("missing claim defaults to default tenant: got %d", got)
	}
	if got := call("garbage", "default"); got != 401 {
		t.Fatalf("invalid token: got %d", got)
	}
	if got := call("", "default"); got != 401 {
		t.Fatalf("missing token: got %d", got)
	}
}

// shadow 模式不阻断:全量放行并记录。
func TestMiddlewareShadowMode(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	kid := "k1"
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{{"kid": kid, "kty": "RSA", "alg": "RS256", "n": n, "e": "AQAB"}},
		})
	}))
	defer jwksSrv.Close()
	mw := New(Config{JWKSURL: jwksSrv.URL, Mode: "shadow"}, zap.NewNop())
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("X-Tenant-ID", "whatever")
	rec := httptest.NewRecorder()
	mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })).ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("shadow mode must pass through, got %d", rec.Code)
	}
}
