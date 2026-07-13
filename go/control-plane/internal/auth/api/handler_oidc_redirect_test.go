package api

import (
	"net/url"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
)

func TestBuildOIDCRedirectURLUsesFragmentAndPreservesNext(t *testing.T) {
	resp := &service.LoginResponse{
		AccessToken:  "access-token-1",
		RefreshToken: "refresh-token-1",
		ExpiresIn:    900,
		TokenType:    "Bearer",
	}

	target, err := buildOIDCRedirectURL(
		"http://10.0.5.8:30180/oidc/callback?next=%2Fdashboard",
		"10.0.5.8:30180",
		resp,
	)
	if err != nil {
		t.Fatalf("buildOIDCRedirectURL returned error: %v", err)
	}

	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("redirect target should parse: %v", err)
	}
	if got := parsed.Query().Get("next"); got != "/dashboard" {
		t.Fatalf("expected next query to be preserved, got %q", got)
	}
	if got := parsed.Query().Get("access_token"); got != "" {
		t.Fatalf("access token must not be placed in query, got %q", got)
	}

	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil {
		t.Fatalf("fragment should parse as query string: %v", err)
	}
	if got := fragment.Get("access_token"); got != resp.AccessToken {
		t.Fatalf("expected access token in fragment, got %q", got)
	}
	if got := fragment.Get("refresh_token"); got != resp.RefreshToken {
		t.Fatalf("expected refresh token in fragment, got %q", got)
	}
	if got := fragment.Get("expires_in"); got != "900" {
		t.Fatalf("expected expires_in in fragment, got %q", got)
	}
	if got := fragment.Get("token_type"); got != "Bearer" {
		t.Fatalf("expected token type in fragment, got %q", got)
	}
}

func TestBuildOIDCRedirectURLRejectsExternalHost(t *testing.T) {
	_, err := buildOIDCRedirectURL(
		"https://evil.example/oidc/callback",
		"10.0.5.8:30180",
		&service.LoginResponse{AccessToken: "access-token-1"},
	)
	if err == nil {
		t.Fatal("expected external redirect host to be rejected")
	}
}
