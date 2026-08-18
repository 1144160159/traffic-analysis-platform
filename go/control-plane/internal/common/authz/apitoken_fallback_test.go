package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/httpx"
)

// TestHandlerFallbackAPIToken P2-c:非 JWT 凭证回退机器凭证校验器,
// 命中后同样强制租户绑定并产出 Principal(与人类 OIDC 同一判定链)。
func TestHandlerFallbackAPIToken(t *testing.T) {
	fallback := func(ctx context.Context, r *http.Request, tokenString string) (*Principal, error) {
		if tokenString != "tap_abcdefgh12345678_wxyz5678" {
			return nil, nil // 未命中 → 中间件 401
		}
		return &Principal{
			Kind:        PrincipalKindAPIToken,
			Subject:     "api-token:test",
			Username:    "ci-token",
			TenantID:    "default",
			Permissions: []string{"alert:read"},
		}, nil
	}
	m := New(Config{
		JWKSURL: "http://unreachable.invalid/certs",
		Mode:    "enforce",
		Fallback: func(ctx context.Context, r *http.Request, tokenString string) (*Principal, error) {
			p, _ := fallback(ctx, r, tokenString)
			return p, nil
		},
	}, zap.NewNop())

	hit := &Principal{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = PrincipalFromContext(r.Context())
		// ADR-6:审计链读取 httpx 键得到 api-token:<id> actor
		if got := httpx.GetUserID(r.Context()); got != "api-token:test" {
			t.Errorf("httpx user id = %q, want api-token:test", got)
		}
		if got := httpx.GetPermissions(r.Context()); len(got) != 1 || got[0] != "alert:read" {
			t.Errorf("httpx permissions = %v", got)
		}
		w.WriteHeader(http.StatusOK)
	})

	// 1) 正确 API Key + 匹配租户 → 200 + principal 注入
	req := httptest.NewRequest("GET", "/api/v1/analysis/runs", nil)
	req.Header.Set("Authorization", "Bearer tap_abcdefgh12345678_wxyz5678")
	req.Header.Set("X-Tenant-ID", "default")
	rec := httptest.NewRecorder()
	m.Handler(next).ServeHTTP(rec, req)
	if rec.Code != 200 || hit == nil || hit.Kind != PrincipalKindAPIToken || hit.Subject != "api-token:test" {
		t.Fatalf("api token allow: code=%d principal=%+v", rec.Code, hit)
	}

	// 2) 伪造租户 → 403(机器凭证同样强制租户绑定)
	req = httptest.NewRequest("GET", "/api/v1/analysis/runs", nil)
	req.Header.Set("Authorization", "Bearer tap_abcdefgh12345678_wxyz5678")
	req.Header.Set("X-Tenant-ID", "tenant-b")
	rec = httptest.NewRecorder()
	m.Handler(next).ServeHTTP(rec, req)
	if rec.Code != 403 {
		t.Fatalf("forged tenant = %d, want 403", rec.Code)
	}

	// 3) 无效 API Key → 401(fail-closed)
	req = httptest.NewRequest("GET", "/api/v1/analysis/runs", nil)
	req.Header.Set("Authorization", "Bearer tap_abcdefgh12345678_badvalue")
	req.Header.Set("X-Tenant-ID", "default")
	rec = httptest.NewRecorder()
	m.Handler(next).ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("invalid api key = %d, want 401", rec.Code)
	}

	// 4) JWT 形态(garbage with 2 dots)不触发回退 → 401
	req = httptest.NewRequest("GET", "/api/v1/analysis/runs", nil)
	req.Header.Set("Authorization", "Bearer not.a.jwt")
	req.Header.Set("X-Tenant-ID", "default")
	rec = httptest.NewRecorder()
	m.Handler(next).ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("jwt-like garbage = %d, want 401", rec.Code)
	}
}
