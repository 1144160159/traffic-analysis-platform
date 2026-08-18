package model

import (
	"strings"
	"testing"
)

func TestAdminRoleUsesOnlyCurrentConcreteTenantBoundScopes(t *testing.T) {
	adminScopes := GetScopesForRoles([]string{"admin"})
	actual := make(map[string]struct{}, len(adminScopes))
	for _, scope := range adminScopes {
		// P1 判定合一:admin 角色补发 admin:* 通配(唯一允许的域通配);
		// ScopeAll(*) 与 admin:cross-tenant 依旧禁止,其他域通配(alert:* 等)也不允许。
		if scope == ScopeAll || scope == ScopeAdminCrossTenant || (strings.HasSuffix(scope, ":*") && scope != ScopeAdminAll) {
			t.Fatalf("admin role contains non-minimal scope %q", scope)
		}
		actual[scope] = struct{}{}
	}
	for _, scope := range AllValidScopes {
		if scope == ScopeAll || scope == ScopeAdminCrossTenant || strings.HasSuffix(scope, ":*") {
			continue
		}
		if _, ok := actual[scope]; !ok {
			t.Fatalf("admin role is missing registered concrete scope %q", scope)
		}
	}
}

func TestScreenViewScopeIsValidAndDocumented(t *testing.T) {
	valid, invalid := ValidateScopes([]string{ScopeScreenView})
	if len(invalid) != 0 {
		t.Fatalf("invalid scopes = %v, want none", invalid)
	}
	if len(valid) != 1 || valid[0] != ScopeScreenView {
		t.Fatalf("valid scopes = %v, want [%s]", valid, ScopeScreenView)
	}

	found := false
	for _, info := range GetAllScopeInfos() {
		if info.Name == ScopeScreenView {
			found = true
			if info.Category != "screen" {
				t.Fatalf("screen scope category = %q, want screen", info.Category)
			}
		}
	}
	if !found {
		t.Fatalf("%s missing from scope infos", ScopeScreenView)
	}
}

func TestDefaultViewerRoleIncludesScreenView(t *testing.T) {
	if !HasScope(GetScopesForRoles([]string{"viewer"}), ScopeScreenView) {
		t.Fatalf("viewer role should include %s", ScopeScreenView)
	}
}

func TestAssetDiscoveryScopeIsValidAndRoleBounded(t *testing.T) {
	valid, invalid := ValidateScopes([]string{ScopeAssetRead, ScopeAssetDiscover, ScopeAssetExport, ScopeAssetGovern})
	if len(invalid) != 0 {
		t.Fatalf("invalid asset scopes = %v, want none", invalid)
	}
	if len(valid) != 4 {
		t.Fatalf("valid asset scopes = %v, want four scopes", valid)
	}

	foundDiscover := false
	foundExport := false
	foundGovern := false
	for _, info := range GetAllScopeInfos() {
		if info.Name == ScopeAssetDiscover {
			foundDiscover = true
			if info.Category != "asset" {
				t.Fatalf("asset discovery scope category = %q, want asset", info.Category)
			}
		}
		if info.Name == ScopeAssetExport {
			foundExport = true
			if info.Category != "asset" {
				t.Fatalf("asset export scope category = %q, want asset", info.Category)
			}
		}
		if info.Name == ScopeAssetGovern {
			foundGovern = true
			if info.Category != "asset" {
				t.Fatalf("asset governance scope category = %q, want asset", info.Category)
			}
		}
	}
	if !foundDiscover {
		t.Fatalf("%s missing from scope infos", ScopeAssetDiscover)
	}
	if !foundExport {
		t.Fatalf("%s missing from scope infos", ScopeAssetExport)
	}
	if !foundGovern {
		t.Fatalf("%s missing from scope infos", ScopeAssetGovern)
	}

	if !HasScope(GetScopesForRoles([]string{"operator"}), ScopeAssetDiscover) {
		t.Fatalf("operator role should include %s", ScopeAssetDiscover)
	}
	if !HasScope(GetScopesForRoles([]string{"operator"}), ScopeAssetGovern) {
		t.Fatalf("operator role should include %s", ScopeAssetGovern)
	}
	if HasScope(GetScopesForRoles([]string{"viewer"}), ScopeAssetGovern) {
		t.Fatalf("viewer role should not include %s", ScopeAssetGovern)
	}
	if HasScope(GetScopesForRoles([]string{"viewer"}), ScopeAssetDiscover) {
		t.Fatalf("viewer role should not include %s", ScopeAssetDiscover)
	}
	if !HasScope(GetScopesForRoles([]string{"viewer"}), ScopeAssetRead) {
		t.Fatalf("viewer role should include %s", ScopeAssetRead)
	}
	if HasScope(GetScopesForRoles([]string{"viewer"}), ScopeAssetExport) {
		t.Fatalf("viewer role should not include %s", ScopeAssetExport)
	}
	if !HasScope(GetScopesForRoles([]string{"analyst"}), ScopeAssetExport) {
		t.Fatalf("analyst role should include %s", ScopeAssetExport)
	}
}

func TestDeploymentWorkflowScopesAreValidAndDocumented(t *testing.T) {
	want := []string{ScopeDeployGray, ScopeDeployApprove}
	valid, invalid := ValidateScopes(want)
	if len(invalid) != 0 || len(valid) != len(want) {
		t.Fatalf("deployment workflow scopes valid=%v invalid=%v, want all valid", valid, invalid)
	}
	documented := map[string]bool{}
	for _, info := range GetAllScopeInfos() {
		if info.Category == "deploy" {
			documented[info.Name] = true
		}
	}
	for _, scope := range want {
		if !documented[scope] {
			t.Fatalf("%s missing from deployment scope infos", scope)
		}
	}
}

func TestFrontendRequiredScopesAreValidAndDocumented(t *testing.T) {
	want := []string{
		ScopeDashboardWrite,
		ScopeTopicRead,
		ScopeTopicWrite,
		ScopeTopicExport,
		ScopeCampaignWrite,
		ScopePlaybookExecute,
		ScopeRuleEnable,
		ScopeModelRead,
		ScopeModelCreate,
		ScopeModelWrite,
		ScopeModelActivate,
		ScopePcapWrite,
	}
	valid, invalid := ValidateScopes(want)
	if len(invalid) != 0 || len(valid) != len(want) {
		t.Fatalf("frontend contract scopes valid=%v invalid=%v, want all valid", valid, invalid)
	}
	documented := map[string]bool{}
	for _, info := range GetAllScopeInfos() {
		documented[info.Name] = true
	}
	for _, scope := range want {
		if !documented[scope] {
			t.Fatalf("%s missing from scope infos", scope)
		}
	}
}

func TestCanDelegateScopesEnforcesCallerCeiling(t *testing.T) {
	tests := []struct {
		name      string
		actor     []string
		requested []string
		want      bool
	}{
		{name: "exact", actor: []string{ScopeAlertRead, ScopeTokenWrite}, requested: []string{ScopeAlertRead}, want: true},
		{name: "token writer cannot mint admin", actor: []string{ScopeTokenWrite}, requested: []string{ScopeAdminAll}, want: false},
		{name: "domain wildcard", actor: []string{"alert:*"}, requested: []string{ScopeAlertRead, ScopeAlertWrite}, want: true},
		{name: "domain wildcard is bounded", actor: []string{"alert:*"}, requested: []string{ScopeAdminCrossTenant}, want: false},
		{name: "global wildcard", actor: []string{ScopeAll}, requested: []string{ScopeAdminAll, ScopeAdminCrossTenant}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanDelegateScopes(tt.actor, tt.requested); got != tt.want {
				t.Fatalf("CanDelegateScopes(%v, %v) = %v, want %v", tt.actor, tt.requested, got, tt.want)
			}
		})
	}
}
