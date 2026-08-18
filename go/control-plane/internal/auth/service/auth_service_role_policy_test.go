package service

import (
	"reflect"
	"sort"
	"testing"

	"github.com/1144160159/traffic-analysis-platform/go/control-plane/internal/auth/model"
)

func TestMapOIDCRolesIncludesOperatorAndIsDeterministic(t *testing.T) {
	service := &AuthService{}
	roles := service.mapOIDCRoles([]string{"traffic-viewer", "TRAFFIC-OPERATOR", "traffic-viewer"})
	want := []string{"operator", "viewer"}
	if !reflect.DeepEqual(roles, want) {
		t.Fatalf("roles=%v want=%v", roles, want)
	}
}

func TestRolePermissionsAreDeterministicallySorted(t *testing.T) {
	service := &AuthService{}
	permissions := service.getPermissionsFromRoles([]string{"operator", "viewer"})
	if !sort.StringsAreSorted(permissions) {
		t.Fatalf("permissions are not sorted: %v", permissions)
	}
	want := model.GetScopesForRoles([]string{"viewer", "operator"})
	if !reflect.DeepEqual(permissions, want) {
		t.Fatalf("permissions=%v want=%v", permissions, want)
	}
}
