package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/ingest/config"
)

type ScopedTokenValidator interface {
	ValidateWithScopes(ctx context.Context, probeID, token string) (*TokenInfo, error)
}

type ReplayTokenValidator struct {
	apiTokenValidator ScopedTokenValidator
	jwtSigningKey     string
	jwtIssuer         string
	logger            *zap.Logger
}

func NewReplayTokenValidator(apiTokenValidator ScopedTokenValidator, jwtCfg config.JWTConfig, logger *zap.Logger) *ReplayTokenValidator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ReplayTokenValidator{
		apiTokenValidator: apiTokenValidator,
		jwtSigningKey:     jwtCfg.SigningKey,
		jwtIssuer:         jwtCfg.Issuer,
		logger:            logger,
	}
}

func (v *ReplayTokenValidator) ValidateWithScopes(ctx context.Context, probeID, token string) (*TokenInfo, error) {
	if looksLikeJWT(token) {
		if tokenInfo, err := v.validateUserJWT(token); err == nil {
			return tokenInfo, nil
		} else {
			v.logger.Debug("DLQ replay user JWT validation failed, trying API token", zap.Error(err))
		}
	}

	if v.apiTokenValidator == nil {
		return nil, fmt.Errorf("api token validator is not configured")
	}
	return v.apiTokenValidator.ValidateWithScopes(ctx, probeID, token)
}

type replayJWTClaims struct {
	UserID      string   `json:"user_id"`
	TenantID    string   `json:"tenant_id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	TokenType   string   `json:"token_type"`
	SessionID   string   `json:"session_id"`
	jwt.RegisteredClaims
}

func (v *ReplayTokenValidator) validateUserJWT(token string) (*TokenInfo, error) {
	if strings.TrimSpace(v.jwtSigningKey) == "" || v.jwtSigningKey == "your-256-bit-secret-key-here" {
		return nil, fmt.Errorf("jwt signing key is not configured")
	}

	claims := &replayJWTClaims{}
	parserOptions := []jwt.ParserOption{}
	if strings.TrimSpace(v.jwtIssuer) != "" {
		parserOptions = append(parserOptions, jwt.WithIssuer(v.jwtIssuer))
	}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(v.jwtSigningKey), nil
	}, parserOptions...)
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("jwt is invalid")
	}
	if claims.TokenType != "access" {
		return nil, fmt.Errorf("jwt token_type must be access")
	}
	if claims.ExpiresAt == nil {
		return nil, fmt.Errorf("jwt exp is required")
	}
	if strings.TrimSpace(claims.TenantID) == "" {
		return nil, fmt.Errorf("jwt tenant_id is required")
	}

	actor := strings.TrimSpace(claims.Username)
	if actor == "" {
		actor = strings.TrimSpace(claims.Subject)
	}
	if actor == "" {
		actor = strings.TrimSpace(claims.UserID)
	}

	return &TokenInfo{
		TenantID:  claims.TenantID,
		ProbeID:   actor,
		Scopes:    claims.Permissions,
		ExpiresAt: claims.ExpiresAt.Unix(),
	}, nil
}

func looksLikeJWT(token string) bool {
	return strings.Count(strings.TrimSpace(token), ".") == 2
}
