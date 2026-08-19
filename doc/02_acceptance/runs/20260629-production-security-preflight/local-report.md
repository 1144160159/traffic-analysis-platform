# Production Security Preflight

Run: `20260629-production-security-preflight-r41-storage-flink-ha`

Result: `blocked`

This is a non-destructive live/repo preflight for GATE-P0-07 and GATE-P0-10. It does not apply NetworkPolicy, rotate secrets, or switch Kafka listeners.

## Summary

| Metric | Count |
|---|---:|
| Checks | 18 |
| Passed | 12 |
| Blockers | 2 |
| Warnings | 4 |
| External service ports | 1 |
| Non-business external ports | 0 |
| Live NetworkPolicies | 20 |
| Live unpinned/latest images | 0 |
| Repo Kafka plaintext files | 0 |
| Repo image lock missing files | 0 |
| Repo mutable/latest image lines with lock evidence | 0 |
| Repo non-business external service ports | 0 |
| Apply-convergence non-business external service ports | 0 |
| Repo default credential pattern files | 0 |
| Repo raw Secret manifest files | 1 |
| NetworkPolicy-capable CNI pods | 0 |
| Keycloak live profile blockers | 0 |

## Key Artifacts

- `doc/02_acceptance/runs/20260629-production-security-preflight/live-production-security-preflight-20260629-production-security-preflight-r41-storage-flink-ha-summary.json`
- `doc/02_acceptance/runs/20260629-production-security-preflight/live-production-security-preflight-20260629-production-security-preflight-r41-storage-flink-ha.ndjson`
- `network-policy-dry-run.txt`
- `live-external-service-blockers.json`
- `live-networkpolicies.json`
- `live-cni-policy-capability-summary.json`
- `repo-default-credential-pattern-files.txt`
- `repo-kafka-plaintext-files.txt`
- `repo-unpinned-or-latest-image-files.txt`
- `repo-image-lock-summary.json`
- `repo-latest-or-mutable-image-lines.txt`
- `repo-service-exposure-summary.json`
- `repo-service-exposure-blockers.json`
- `apply-convergence-service-exposure-summary.json`
- `apply-convergence-service-exposure-blockers.json`
- `repo-external-secret-files.txt`
- `live-keycloak-profile-summary.json`

## Interpretation

The starter NetworkPolicy profile can be client-side dry-run checked, production manifests no longer contain Kafka plaintext listener definitions, repo image references are covered by an explicit evidence lock, repo Service exposure is limited to the APISIX business port, and kubectl apply convergence behavior is explicitly checked. The live cluster remains blocked for production security while the new Kafka SASL_SSL profile is not rolled out, NetworkPolicy enforcement-capable CNI is not present, ExternalSecret/SealedSecret is not actually reconciled, live Keycloak is not on the TLS/SecretRef profile, and live workload image specs are still not digest-pinned.
