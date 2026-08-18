package httpx

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	commonerrors "github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/common/errors"
)

type authorizationTestClaims struct {
	userID      string
	tenantID    string
	roles       []string
	permissions []string
}

func (c authorizationTestClaims) GetUserID() string        { return c.userID }
func (c authorizationTestClaims) GetTenantID() string      { return c.tenantID }
func (c authorizationTestClaims) GetUsername() string      { return "authorization-test" }
func (c authorizationTestClaims) GetRoles() []string       { return c.roles }
func (c authorizationTestClaims) GetPermissions() []string { return c.permissions }

type authorizationTestValidator struct {
	claims Claims
	err    error
}

func (v authorizationTestValidator) ValidateToken(string) (Claims, error) {
	return v.claims, v.err
}

func authorizationContext(claims Claims) context.Context {
	ctx := context.WithValue(context.Background(), ContextKeyClaims, claims)
	ctx = context.WithValue(ctx, ContextKeyUserID, claims.GetUserID())
	ctx = context.WithValue(ctx, ContextKeyTenantID, claims.GetTenantID())
	ctx = context.WithValue(ctx, ContextKeyRoles, claims.GetRoles())
	return context.WithValue(ctx, ContextKeyPermissions, claims.GetPermissions())
}

func TestAuthorizeResourceFailsClosedAcrossEveryPolicyDimension(t *testing.T) {
	verified := authorizationTestClaims{
		userID:      "user-a",
		tenantID:    "tenant-a",
		roles:       []string{"analyst"},
		permissions: []string{"alert:read"},
	}
	tests := []struct {
		name    string
		ctx     context.Context
		request ResourceAuthorizationRequest
		code    commonerrors.ErrorCode
	}{
		{
			name: "no verified claims",
			ctx:  context.Background(),
			request: ResourceAuthorizationRequest{
				RequiredScopes: []string{"alert:read"},
			},
			code: commonerrors.ErrCodeUnauthorized,
		},
		{
			name: "scope escalation",
			ctx:  authorizationContext(verified),
			request: ResourceAuthorizationRequest{
				RequiredScopes: []string{"alert:write"},
			},
			code: commonerrors.ErrCodePermissionDenied,
		},
		{
			name: "cross tenant object is hidden",
			ctx:  authorizationContext(verified),
			request: ResourceAuthorizationRequest{
				RequiredScopes:   []string{"alert:read"},
				ResourceTenantID: "tenant-b",
				ObjectID:         "alert-guess",
				ObjectRequired:   true,
			},
			code: commonerrors.ErrCodeResourceNotFound,
		},
		{
			name: "guessed missing object is hidden",
			ctx:  authorizationContext(verified),
			request: ResourceAuthorizationRequest{
				RequiredScopes: []string{"alert:read"},
				ObjectID:       "alert-guess",
				ObjectRequired: true,
			},
			code: commonerrors.ErrCodeResourceNotFound,
		},
		{
			name: "field escalation",
			ctx:  authorizationContext(verified),
			request: ResourceAuthorizationRequest{
				RequiredScopes:   []string{"alert:read"},
				ResourceTenantID: "tenant-a",
				ObjectID:         "alert-a",
				ObjectRequired:   true,
				RequestedFields:  []string{"summary", "raw_payload"},
				AllowedFields:    []string{"summary"},
			},
			code: commonerrors.ErrCodePermissionDenied,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := AuthorizeResource(test.ctx, test.request)
			if !commonerrors.IsCode(err, test.code) {
				t.Fatalf("error=%v, want code %s", err, test.code)
			}
		})
	}
}

func TestAuthorizeResourceAllowsTenantObjectAndNormalizesFields(t *testing.T) {
	claims := authorizationTestClaims{
		userID:      "user-a",
		tenantID:    "tenant-a",
		permissions: []string{"alert:*"},
	}
	err := AuthorizeResource(authorizationContext(claims), ResourceAuthorizationRequest{
		RequiredScopes:   []string{"alert:read", "alert:export"},
		RequireAllScopes: true,
		ResourceTenantID: "tenant-a",
		ObjectID:         "alert-a",
		ObjectRequired:   true,
		RequestedFields:  []string{" Summary ", "STATUS"},
		AllowedFields:    []string{"summary", "status"},
	})
	if err != nil {
		t.Fatalf("authorize valid resource: %v", err)
	}
	fields, err := AuthorizedFields([]string{" Status ", "summary", "status"}, []string{"summary", "status"})
	if err != nil || len(fields) != 2 || fields[0] != "status" || fields[1] != "summary" {
		t.Fatalf("fields=%v err=%v", fields, err)
	}
}

func TestPermissionAllowsOnlyCompleteWildcards(t *testing.T) {
	for _, test := range []struct {
		granted  []string
		required string
		want     bool
	}{
		{[]string{"alert:read"}, "alert:read", true},
		{[]string{"alert:*"}, "alert:write", true},
		{[]string{"*"}, "alert:write", true},
		{[]string{"alert*"}, "alert:write", false},
		{[]string{"alert:read"}, "alert:write", false},
	} {
		if got := PermissionAllows(test.granted, test.required); got != test.want {
			t.Fatalf("PermissionAllows(%v,%q)=%v want %v", test.granted, test.required, got, test.want)
		}
	}
}

func TestAuthRejectsMissingExpiredAndTenantSpoofing(t *testing.T) {
	validClaims := authorizationTestClaims{userID: "user-a", tenantID: "tenant-a", permissions: []string{"alert:read"}}
	tests := []struct {
		name      string
		validator authorizationTestValidator
		authorize bool
		header    string
		query     string
		want      int
	}{
		{name: "missing token", validator: authorizationTestValidator{claims: validClaims}, want: http.StatusUnauthorized},
		{name: "expired token", validator: authorizationTestValidator{err: stderrors.New("token expired")}, authorize: true, want: http.StatusUnauthorized},
		{name: "header tenant spoof", validator: authorizationTestValidator{claims: validClaims}, authorize: true, header: "tenant-b", want: http.StatusForbidden},
		{name: "query tenant spoof", validator: authorizationTestValidator{claims: validClaims}, authorize: true, query: "tenant-b", want: http.StatusForbidden},
		{name: "matching tenant", validator: authorizationTestValidator{claims: validClaims}, authorize: true, header: "tenant-a", query: "tenant-a", want: http.StatusNoContent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := Auth(test.validator, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, "/api/v1/alerts?tenant_id="+test.query, nil)
			if test.authorize {
				request.Header.Set("Authorization", "Bearer token")
			}
			if test.header != "" {
				request.Header.Set("X-Tenant-ID", test.header)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestBusinessContextExtractorNeverCreatesTenantFromRequest(t *testing.T) {
	handler := BusinessContextExtractor()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := GetTenantID(r.Context()); got != "" {
			t.Fatalf("unverified tenant entered context: %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/assets?tenant_id=tenant-query", nil)
	request.Header.Set("X-Tenant-ID", "tenant-header")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
