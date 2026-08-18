package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
)

type m10RolePolicyDocument struct {
	Roles []struct {
		RoleID      string   `json:"role_id"`
		Scopes      []string `json:"scopes"`
		CrossTenant bool     `json:"cross_tenant"`
	} `json:"roles"`
}

func TestM10RolePolicyContractMatchesRuntimeMap(t *testing.T) {
	policyPath := os.Getenv("M10_AUTHZ_POLICY_PATH")
	if policyPath == "" {
		_, source, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("cannot resolve test source path")
		}
		policyPath = filepath.Join(filepath.Dir(source), "../../../../../contracts/authz/m10-minimal-role-policy.v1.json")
	}
	content, err := os.ReadFile(filepath.Clean(policyPath))
	if err != nil {
		t.Fatalf("read M10 authz policy: %v", err)
	}
	var document m10RolePolicyDocument
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("decode M10 authz policy: %v", err)
	}
	if len(document.Roles) != len(DefaultRoleScopes) {
		t.Fatalf("contract roles=%d runtime roles=%d", len(document.Roles), len(DefaultRoleScopes))
	}
	for _, role := range document.Roles {
		if role.CrossTenant {
			t.Fatalf("contract role %s permits cross-tenant access", role.RoleID)
		}
		runtimeScopes, ok := DefaultRoleScopes[role.RoleID]
		if !ok {
			t.Fatalf("contract role %s is absent at runtime", role.RoleID)
		}
		runtimeScopes = append([]string(nil), runtimeScopes...)
		sort.Strings(runtimeScopes)
		if !reflect.DeepEqual(runtimeScopes, role.Scopes) {
			t.Fatalf("role %s runtime scopes=%v contract scopes=%v", role.RoleID, runtimeScopes, role.Scopes)
		}
	}
}
