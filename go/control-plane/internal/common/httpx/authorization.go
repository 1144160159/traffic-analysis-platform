package httpx

import (
	"context"
	"net/http"
	"sort"
	"strings"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

// ResourceAuthorizationRequest describes every input needed to authorize a
// resource operation. Callers must pass the tenant read from the resource, not
// a tenant supplied by the request.
type ResourceAuthorizationRequest struct {
	RequiredScopes   []string
	RequireAllScopes bool
	ResourceTenantID string
	ObjectID         string
	ObjectRequired   bool
	RequestedFields  []string
	AllowedFields    []string
}

// AuthorizeResource applies authentication, scope, tenant, object and field
// checks in one fail-closed decision. Missing and cross-tenant objects are both
// reported as not found so object identifiers cannot be used as an existence
// oracle across tenants.
func AuthorizeResource(ctx context.Context, request ResourceAuthorizationRequest) error {
	claims := GetClaims(ctx)
	if claims == nil || strings.TrimSpace(claims.GetUserID()) == "" || strings.TrimSpace(claims.GetTenantID()) == "" {
		return commonerrors.New(commonerrors.ErrCodeUnauthorized, "Verified identity is required")
	}

	if len(request.RequiredScopes) > 0 {
		matched := 0
		for _, required := range request.RequiredScopes {
			if PermissionAllows(claims.GetPermissions(), required) {
				matched++
			}
		}
		allowed := matched > 0
		if request.RequireAllScopes {
			allowed = matched == len(request.RequiredScopes)
		}
		if !allowed {
			return commonerrors.New(commonerrors.ErrCodePermissionDenied, "Required scope is missing")
		}
	}

	objectID := strings.TrimSpace(request.ObjectID)
	resourceTenantID := strings.TrimSpace(request.ResourceTenantID)
	if request.ObjectRequired && (objectID == "" || resourceTenantID == "") {
		return commonerrors.New(commonerrors.ErrCodeResourceNotFound, "Resource not found")
	}
	if resourceTenantID != "" && resourceTenantID != strings.TrimSpace(claims.GetTenantID()) {
		return commonerrors.New(commonerrors.ErrCodeResourceNotFound, "Resource not found")
	}

	if len(request.RequestedFields) > 0 {
		allowedFields := make(map[string]struct{}, len(request.AllowedFields))
		for _, field := range request.AllowedFields {
			if normalized := normalizeAuthorizationField(field); normalized != "" {
				allowedFields[normalized] = struct{}{}
			}
		}
		for _, field := range request.RequestedFields {
			normalized := normalizeAuthorizationField(field)
			if normalized == "" {
				continue
			}
			if _, ok := allowedFields[normalized]; !ok {
				return commonerrors.New(commonerrors.ErrCodePermissionDenied, "Requested field is not permitted")
			}
		}
	}

	return nil
}

// PermissionAllows performs exact scope matching plus the two intentional
// wildcard forms: global "*" and a complete domain wildcard such as
// "alert:*". Partial wildcards are never accepted.
func PermissionAllows(granted []string, required string) bool {
	required = strings.TrimSpace(required)
	if required == "" {
		return false
	}
	for _, candidate := range granted {
		candidate = strings.TrimSpace(candidate)
		if candidate == required || candidate == "*" {
			return true
		}
		if strings.HasSuffix(candidate, ":*") && strings.HasPrefix(required, strings.TrimSuffix(candidate, "*")) {
			return true
		}
	}
	return false
}

// ValidateRequestTenant binds optional tenant assertions in a request to an
// already verified token tenant. It never derives identity from request data.
func ValidateRequestTenant(r *http.Request, verifiedTenantID string) error {
	verifiedTenantID = strings.TrimSpace(verifiedTenantID)
	if verifiedTenantID == "" {
		return commonerrors.New(commonerrors.ErrCodeUnauthorized, "Verified tenant identity is required")
	}

	headerTenantID := strings.TrimSpace(r.Header.Get("X-Tenant-ID"))
	queryTenantID := strings.TrimSpace(r.URL.Query().Get("tenant_id"))
	if headerTenantID != "" && queryTenantID != "" && headerTenantID != queryTenantID {
		return commonerrors.New(commonerrors.ErrCodePermissionDenied, "Conflicting tenant assertions")
	}
	for _, requestedTenantID := range []string{headerTenantID, queryTenantID} {
		if requestedTenantID != "" && requestedTenantID != verifiedTenantID {
			return commonerrors.New(commonerrors.ErrCodePermissionDenied, "Cross-tenant access denied")
		}
	}
	return nil
}

// AuthorizedFields returns a deterministic field list after applying the same
// field-level policy used by AuthorizeResource.
func AuthorizedFields(requested, allowed []string) ([]string, error) {
	if err := AuthorizeResourceFields(requested, allowed); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, field := range requested {
		field = normalizeAuthorizationField(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	sort.Strings(result)
	return result, nil
}

func AuthorizeResourceFields(requested, allowed []string) error {
	allowedFields := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		if normalized := normalizeAuthorizationField(field); normalized != "" {
			allowedFields[normalized] = struct{}{}
		}
	}
	for _, field := range requested {
		normalized := normalizeAuthorizationField(field)
		if normalized == "" {
			continue
		}
		if _, ok := allowedFields[normalized]; !ok {
			return commonerrors.New(commonerrors.ErrCodePermissionDenied, "Requested field is not permitted")
		}
	}
	return nil
}

func normalizeAuthorizationField(field string) string {
	return strings.ToLower(strings.TrimSpace(field))
}
