package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

type fakeScopedTokenValidator struct {
	info  *TokenInfo
	err   error
	calls int
}

func (v *fakeScopedTokenValidator) ValidateWithScopes(context.Context, string, string) (*TokenInfo, error) {
	v.calls++
	if v.err != nil {
		return nil, v.err
	}
	return v.info, nil
}

func TestReplayTokenValidatorAcceptsUserAccessJWT(t *testing.T) {
	cfg := config.JWTConfig{SigningKey: "test-signing-key", Issuer: "traffic-auth-service"}
	apiValidator := &fakeScopedTokenValidator{err: errors.New("api token should not be needed")}
	validator := NewReplayTokenValidator(apiValidator, cfg, zap.NewNop())
	token := signReplayJWT(t, cfg, replayJWTClaims{
		TenantID:    "tenant-a",
		Username:    "operator-1",
		Permissions: []string{config.ScopeDLQReplay},
		TokenType:   "access",
	})

	info, err := validator.ValidateWithScopes(context.Background(), "", token)
	if err != nil {
		t.Fatalf("ValidateWithScopes returned error: %v", err)
	}
	if apiValidator.calls != 0 {
		t.Fatalf("JWT validation should avoid API-token lookup, calls=%d", apiValidator.calls)
	}
	if info.TenantID != "tenant-a" || info.ProbeID != "operator-1" {
		t.Fatalf("unexpected token info: %+v", info)
	}
	if len(info.Scopes) != 1 || info.Scopes[0] != config.ScopeDLQReplay {
		t.Fatalf("unexpected scopes: %+v", info.Scopes)
	}
}

func TestReplayTokenValidatorRejectsRefreshJWTThenFallsBackToAPIToken(t *testing.T) {
	cfg := config.JWTConfig{SigningKey: "test-signing-key", Issuer: "traffic-auth-service"}
	apiValidator := &fakeScopedTokenValidator{err: errors.New("api token lookup failed")}
	validator := NewReplayTokenValidator(apiValidator, cfg, zap.NewNop())
	token := signReplayJWT(t, cfg, replayJWTClaims{
		TenantID:    "tenant-a",
		Username:    "operator-1",
		Permissions: []string{config.ScopeDLQReplay},
		TokenType:   "refresh",
	})

	if _, err := validator.ValidateWithScopes(context.Background(), "", token); err == nil {
		t.Fatalf("refresh JWT must not be accepted")
	}
	if apiValidator.calls != 1 {
		t.Fatalf("invalid JWT should fall back to API-token validator once, calls=%d", apiValidator.calls)
	}
}

func TestReplayTokenValidatorUsesAPITokenForOpaqueToken(t *testing.T) {
	cfg := config.JWTConfig{SigningKey: "test-signing-key", Issuer: "traffic-auth-service"}
	apiInfo := &TokenInfo{TenantID: "tenant-b", ProbeID: "probe-1", Scopes: []string{config.ScopeDLQReplay}}
	apiValidator := &fakeScopedTokenValidator{info: apiInfo}
	validator := NewReplayTokenValidator(apiValidator, cfg, zap.NewNop())

	info, err := validator.ValidateWithScopes(context.Background(), "probe-1", "opaque-api-token")
	if err != nil {
		t.Fatalf("ValidateWithScopes returned error: %v", err)
	}
	if apiValidator.calls != 1 {
		t.Fatalf("opaque token should use API-token validator once, calls=%d", apiValidator.calls)
	}
	if info != apiInfo {
		t.Fatalf("unexpected API token info: %+v", info)
	}
}

func signReplayJWT(t *testing.T, cfg config.JWTConfig, claims replayJWTClaims) string {
	t.Helper()
	now := time.Now()
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    cfg.Issuer,
		Subject:   "user-1",
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		ID:        "jwt-1",
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.SigningKey))
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return token
}
