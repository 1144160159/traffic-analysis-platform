package httpx

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

type contextKey string

const (
	ContextKeyUserID      contextKey = "user_id"
	ContextKeyTenantID    contextKey = "tenant_id"
	ContextKeyUsername    contextKey = "username"
	ContextKeyRoles       contextKey = "roles"
	ContextKeyPermissions contextKey = "permissions"
	ContextKeyRequestID   contextKey = "request_id"
	ContextKeyTraceID     contextKey = "trace_id"
	ContextKeyClaims      contextKey = "claims"
)

type Claims interface {
	GetUserID() string
	GetTenantID() string
	GetUsername() string
	GetRoles() []string
	GetPermissions() []string
}

type ExtendedClaims interface {
	Claims
	GetEmail() string
	GetSessionID() string
	HasRole(role string) bool
	HasPermission(permission string) bool
}

type TokenValidator interface {
	ValidateToken(tokenString string) (Claims, error)
}

func Auth(validator TokenValidator, logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				err := errors.New(errors.ErrCodeUnauthorized, "Authorization header required")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				err := errors.New(errors.ErrCodeUnauthorized, "Invalid authorization header format")
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			tokenString := parts[1]

			claims, err := validator.ValidateToken(tokenString)
			if err != nil {
				if logger != nil {
					logger.Debug("Token validation failed",
						zap.Error(err),
						zap.String("path", r.URL.Path))
				}

				appErr := errors.Wrap(err, errors.ErrCodeTokenInvalid, "Invalid or expired token")
				errors.WriteError(w, appErr, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyClaims, claims)
			ctx = context.WithValue(ctx, ContextKeyUserID, claims.GetUserID())
			ctx = context.WithValue(ctx, ContextKeyTenantID, claims.GetTenantID())
			ctx = context.WithValue(ctx, ContextKeyUsername, claims.GetUsername())
			ctx = context.WithValue(ctx, ContextKeyRoles, claims.GetRoles())
			ctx = context.WithValue(ctx, ContextKeyPermissions, claims.GetPermissions())

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequirePermission(permission string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			permissions := GetPermissions(r.Context())

			hasPermission := false
			for _, p := range permissions {
				if p == permission {
					hasPermission = true
					break
				}
			}

			if !hasPermission {
				err := errors.Newf(errors.ErrCodePermissionDenied, "Permission denied: %s required", permission)
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RequireRole(role string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			roles := GetRoles(r.Context())

			hasRole := false
			for _, ro := range roles {
				if ro == role {
					hasRole = true
					break
				}
			}

			if !hasRole {
				err := errors.Newf(errors.ErrCodePermissionDenied, "Role denied: %s required", role)
				errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RequireAnyRole(requiredRoles ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			roles := GetRoles(r.Context())

			for _, role := range roles {
				for _, required := range requiredRoles {
					if role == required {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			err := errors.New(errors.ErrCodePermissionDenied, "Insufficient role")
			errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
		})
	}
}

func RequireAnyPermission(requiredPermissions ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			permissions := GetPermissions(r.Context())

			for _, perm := range permissions {
				for _, required := range requiredPermissions {
					if perm == required {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			err := errors.New(errors.ErrCodePermissionDenied, "Insufficient permission")
			errors.WriteError(w, err, GetTraceID(r.Context()), r.URL.Path)
		})
	}
}

func OptionalAuth(validator TokenValidator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := validator.ValidateToken(parts[1])
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyClaims, claims)
			ctx = context.WithValue(ctx, ContextKeyUserID, claims.GetUserID())
			ctx = context.WithValue(ctx, ContextKeyTenantID, claims.GetTenantID())
			ctx = context.WithValue(ctx, ContextKeyUsername, claims.GetUsername())
			ctx = context.WithValue(ctx, ContextKeyRoles, claims.GetRoles())
			ctx = context.WithValue(ctx, ContextKeyPermissions, claims.GetPermissions())

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyUserID); v != nil {
		return v.(string)
	}
	return ""
}

func GetTenantID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyTenantID); v != nil {
		return v.(string)
	}
	return ""
}

func GetUsername(ctx context.Context) string {
	if v := ctx.Value(ContextKeyUsername); v != nil {
		return v.(string)
	}
	return ""
}

func GetRoles(ctx context.Context) []string {
	if v := ctx.Value(ContextKeyRoles); v != nil {
		return v.([]string)
	}
	return nil
}

func GetPermissions(ctx context.Context) []string {
	if v := ctx.Value(ContextKeyPermissions); v != nil {
		return v.([]string)
	}
	return nil
}

func GetClaims(ctx context.Context) Claims {
	if v := ctx.Value(ContextKeyClaims); v != nil {
		return v.(Claims)
	}
	return nil
}

func GetExtendedClaims(ctx context.Context) ExtendedClaims {
	if v := ctx.Value(ContextKeyClaims); v != nil {
		if ec, ok := v.(ExtendedClaims); ok {
			return ec
		}
	}
	return nil
}

func GetRequestID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyRequestID); v != nil {
		return v.(string)
	}
	return ""
}

func GetTraceID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyTraceID); v != nil {
		return v.(string)
	}
	return ""
}

func HasRole(ctx context.Context, role string) bool {
	roles := GetRoles(ctx)
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}

func HasPermission(ctx context.Context, permission string) bool {
	permissions := GetPermissions(ctx)
	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}
