////////////////////////////////////////////////////////////////////////////////
// FILE PATH: control-plane/internal/auth/middleware/auth_middleware.go
// 修复版：修复 #34 - 中间件未验证 Token 类型
// 修复内容：在 Authenticate 中增加 Token 类型验证，拒绝 Refresh Token
////////////////////////////////////////////////////////////////////////////////

package middleware

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/service"
)

type contextKey string

const (
	ContextKeyClaims   contextKey = "claims"
	ContextKeyUserID   contextKey = "user_id"
	ContextKeyTenantID contextKey = "tenant_id"
)

type AuthMiddleware struct {
	authService *service.AuthService
	logger      *zap.Logger
}

func NewAuthMiddleware(authService *service.AuthService, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
		logger:      logger,
	}
}

// Authenticate 认证中间件（修复 #34：验证 Token 类型）
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]

		claims, err := m.authService.ValidateToken(tokenString)
		if err != nil {
			m.logger.Debug("Token validation failed", zap.Error(err))
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// 修复 #34：验证 Token 类型，只允许 Access Token
		if claims.TokenType != model.JWTTokenAccess {
			m.logger.Warn("Invalid token type in authentication",
				zap.String("expected", string(model.JWTTokenAccess)),
				zap.String("got", string(claims.TokenType)),
				zap.String("user_id", claims.UserID.String()))
			http.Error(w, "Invalid token type: only access tokens are allowed", http.StatusUnauthorized)
			return
		}

		// Add claims to context
		ctx := context.WithValue(r.Context(), ContextKeyClaims, claims)
		ctx = context.WithValue(ctx, ContextKeyUserID, claims.UserID.String())
		ctx = context.WithValue(ctx, ContextKeyTenantID, claims.TenantID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *AuthMiddleware) RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(ContextKeyClaims).(*model.Claims)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			hasPermission := false
			for _, p := range claims.Permissions {
				if p == permission {
					hasPermission = true
					break
				}
			}

			if !hasPermission {
				m.logger.Warn("Permission denied",
					zap.String("user_id", claims.UserID.String()),
					zap.String("required", permission))
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func (m *AuthMiddleware) RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := r.Context().Value(ContextKeyClaims).(*model.Claims)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			hasRole := false
			for _, r := range claims.Roles {
				if r == role {
					hasRole = true
					break
				}
			}

			if !hasRole {
				m.logger.Warn("Role denied",
					zap.String("user_id", claims.UserID.String()),
					zap.String("required", role))
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func GetClaims(ctx context.Context) *model.Claims {
	if claims, ok := ctx.Value(ContextKeyClaims).(*model.Claims); ok {
		return claims
	}
	return nil
}

func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(ContextKeyUserID).(string); ok {
		return userID
	}
	return ""
}

func GetTenantID(ctx context.Context) string {
	if tenantID, ok := ctx.Value(ContextKeyTenantID).(string); ok {
		return tenantID
	}
	return ""
}
