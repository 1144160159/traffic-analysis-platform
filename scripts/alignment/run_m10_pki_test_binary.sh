#!/bin/sh
set -eu

output="$(/opt/m10-pki/pki.test -test.v -test.count=1 -test.run '^(TestReloaderRejectsWrongCAExpiredAndServerSANMismatch|TestReloaderRejectsClientSANAndRevokedSerial|TestInterruptedRotationKeepsLastValidSnapshot|TestDualTrustRotationThenOldIssuerRetirement)$')"
printf '%s\n' "$output"
if printf '%s\n' "$output" | grep -Eq -- '--- (FAIL|SKIP):'; then
  exit 1
fi
for test_name in \
  TestReloaderRejectsWrongCAExpiredAndServerSANMismatch \
  TestReloaderRejectsClientSANAndRevokedSerial \
  TestInterruptedRotationKeepsLastValidSnapshot \
  TestDualTrustRotationThenOldIssuerRetirement
do
  if ! printf '%s\n' "$output" | grep -F -- "--- PASS: $test_name " >/dev/null; then
    printf 'missing required PASS event: %s\n' "$test_name" >&2
    exit 1
  fi
done

printf '%s\n' '{"status":"PASS","profile_id":"M10-N008-K8S-ATOMIC-PKI-ROTATION-V1","test_groups":1,"top_level_tests":4,"fail_closed_dimensions":["wrong_ca","expired_certificate","san_mismatch","revoked_serial","interrupted_rotation","dual_trust_cutover"]}'
