#!/bin/sh
set -eu

run_group() {
  binary="$1"
  pattern="$2"
  shift 2
  output="$($binary -test.v -test.run "$pattern" -test.count=1)"
  printf '%s\n' "$output"
  if printf '%s\n' "$output" | grep -Eq -- '--- (FAIL|SKIP):'; then
    exit 1
  fi
  for test_name in "$@"; do
    if ! printf '%s\n' "$output" | grep -F -- "--- PASS: $test_name " >/dev/null; then
      printf 'missing required PASS event: %s\n' "$test_name" >&2
      exit 1
    fi
  done
}

run_group /opt/m10-authz/httpx.test \
  '^(TestAuthorizeResourceFailsClosedAcrossEveryPolicyDimension|TestAuthorizeResourceAllowsTenantObjectAndNormalizesFields|TestPermissionAllowsOnlyCompleteWildcards|TestAuthRejectsMissingExpiredAndTenantSpoofing|TestBusinessContextExtractorNeverCreatesTenantFromRequest)$' \
  TestAuthorizeResourceFailsClosedAcrossEveryPolicyDimension \
  TestAuthorizeResourceAllowsTenantObjectAndNormalizesFields \
  TestPermissionAllowsOnlyCompleteWildcards \
  TestAuthRejectsMissingExpiredAndTenantSpoofing \
  TestBusinessContextExtractorNeverCreatesTenantFromRequest

run_group /opt/m10-authz/model.test \
  '^(TestAdminRoleUsesOnlyCurrentConcreteTenantBoundScopes|TestM10RolePolicyContractMatchesRuntimeMap)$' \
  TestAdminRoleUsesOnlyCurrentConcreteTenantBoundScopes \
  TestM10RolePolicyContractMatchesRuntimeMap

run_group /opt/m10-authz/service.test \
  '^(TestMapOIDCRolesIncludesOperatorAndIsDeterministic|TestRolePermissionsAreDeterministicallySorted)$' \
  TestMapOIDCRolesIncludesOperatorAndIsDeterministic \
  TestRolePermissionsAreDeterministicallySorted

run_group /opt/m10-authz/asset.test \
  '^(TestRequireAssetReadEnforcesScopeAndVerifiedTenant|TestRequestIdentityRejectsSpoofedIdentityHeaders|TestRequestIdentityRejectsCrossTenantAssertion)$' \
  TestRequireAssetReadEnforcesScopeAndVerifiedTenant \
  TestRequestIdentityRejectsSpoofedIdentityHeaders \
  TestRequestIdentityRejectsCrossTenantAssertion

run_group /opt/m10-authz/graph.test \
  '^(TestProtectGraphBusinessAPIRequiresBearerToken|TestProtectGraphBusinessAPIRejectsTenantOverride|TestProtectGraphBusinessAPIAcceptsMatchingTokenTenant|TestProtectGraphBusinessAPIRejectsInvalidTokenAndMissingScope)$' \
  TestProtectGraphBusinessAPIRequiresBearerToken \
  TestProtectGraphBusinessAPIRejectsTenantOverride \
  TestProtectGraphBusinessAPIAcceptsMatchingTokenTenant \
  TestProtectGraphBusinessAPIRejectsInvalidTokenAndMissingScope

printf '%s\n' '{"status":"PASS","profile_id":"M10-N007-K8S-FAIL-CLOSED-AUTHZ-V1","test_groups":5,"top_level_tests":16,"fail_closed_dimensions":["missing_token","expired_token","scope_escalation","cross_tenant","guessed_object_id","field_escalation"]}'
