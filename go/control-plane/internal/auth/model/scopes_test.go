package model

import "testing"

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
	valid, invalid := ValidateScopes([]string{ScopeAssetRead, ScopeAssetDiscover})
	if len(invalid) != 0 {
		t.Fatalf("invalid asset scopes = %v, want none", invalid)
	}
	if len(valid) != 2 {
		t.Fatalf("valid asset scopes = %v, want two scopes", valid)
	}

	foundDiscover := false
	for _, info := range GetAllScopeInfos() {
		if info.Name == ScopeAssetDiscover {
			foundDiscover = true
			if info.Category != "asset" {
				t.Fatalf("asset discovery scope category = %q, want asset", info.Category)
			}
		}
	}
	if !foundDiscover {
		t.Fatalf("%s missing from scope infos", ScopeAssetDiscover)
	}

	if !HasScope(GetScopesForRoles([]string{"operator"}), ScopeAssetDiscover) {
		t.Fatalf("operator role should include %s", ScopeAssetDiscover)
	}
	if HasScope(GetScopesForRoles([]string{"viewer"}), ScopeAssetDiscover) {
		t.Fatalf("viewer role should not include %s", ScopeAssetDiscover)
	}
	if !HasScope(GetScopesForRoles([]string{"viewer"}), ScopeAssetRead) {
		t.Fatalf("viewer role should include %s", ScopeAssetRead)
	}
}
