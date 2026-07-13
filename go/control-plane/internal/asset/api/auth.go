package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	authmodel "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

const requiredAssetDiscoverScope = authmodel.ScopeAssetDiscover

type requestIdentity struct {
	TenantID string
	UserID   string
	Username string
	Scopes   []string
}

func (h *HTTPHandler) requireAssetDiscoveryWrite(w http.ResponseWriter, r *http.Request) (requestIdentity, bool) {
	identity, status, message := h.requestIdentity(r)
	if status != 0 {
		writeError(w, status, message)
		return requestIdentity{}, false
	}
	if !hasDiscoveryWriteScope(identity.Scopes) {
		writeError(w, http.StatusForbidden, requiredAssetDiscoverScope+" scope required")
		return requestIdentity{}, false
	}
	if identity.TenantID == "" {
		identity.TenantID = tenantFromRequest(r)
	}
	return identity, true
}

func (h *HTTPHandler) requestIdentity(r *http.Request) (requestIdentity, int, string) {
	if tokenString := bearerToken(r); tokenString != "" {
		if h.jwtSigningKey == "" {
			return requestIdentity{}, http.StatusUnauthorized, "JWT signing key is not configured"
		}
		claims := &authmodel.Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(h.jwtSigningKey), nil
		})
		if err != nil || token == nil || !token.Valid || claims.TokenType != authmodel.JWTTokenAccess {
			return requestIdentity{}, http.StatusUnauthorized, "invalid or expired access token"
		}
		return requestIdentity{
			TenantID: claims.TenantID,
			UserID:   claims.UserID.String(),
			Username: claims.Username,
			Scopes:   claims.GetPermissions(),
		}, 0, ""
	}

	scopes := headerScopes(r)
	if len(scopes) == 0 {
		return requestIdentity{}, http.StatusUnauthorized, "authorization required"
	}
	return requestIdentity{
		TenantID: tenantFromRequest(r),
		UserID:   actorFromRequest(r),
		Username: r.Header.Get("X-Username"),
		Scopes:   scopes,
	}, 0, ""
}

func bearerToken(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func headerScopes(r *http.Request) []string {
	var scopes []string
	for _, header := range []string{"X-Scopes", "X-Permissions", "X-User-Scopes", "X-User-Permissions"} {
		scopes = append(scopes, splitScopes(r.Header.Get(header))...)
	}
	return dedupeScopes(scopes)
}

func splitScopes(value string) []string {
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == ';'
	})
	scopes := make([]string, 0, len(fields))
	for _, field := range fields {
		scope := strings.TrimSpace(field)
		if scope != "" {
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

func dedupeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	return result
}

func hasDiscoveryWriteScope(scopes []string) bool {
	for _, accepted := range []string{authmodel.ScopeAssetDiscover, "asset:*", authmodel.ScopeAdminAll, authmodel.ScopeAll} {
		if authmodel.HasScope(scopes, accepted) {
			return true
		}
	}
	return false
}

func auditUserID(identity requestIdentity) string {
	if identity.UserID == "" {
		return ""
	}
	if _, err := uuid.Parse(identity.UserID); err != nil {
		return ""
	}
	return identity.UserID
}

func auditActor(identity requestIdentity) string {
	if identity.Username != "" {
		return identity.Username
	}
	if identity.UserID != "" {
		return identity.UserID
	}
	return "unknown"
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		if idx := strings.Index(value, ","); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
		}
		if value != "" {
			return value
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
