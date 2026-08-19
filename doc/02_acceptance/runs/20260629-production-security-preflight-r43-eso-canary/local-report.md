# Production Security Preflight

Run: `20260629-production-security-preflight-r43-eso-canary`

Result: `blocked`

This is a non-destructive live/repo preflight for GATE-P0-07 and GATE-P0-10. It does not apply NetworkPolicy, rotate secrets, or switch Kafka listeners.

## Summary

| Metric | Count |
|---|---:|
| Checks | 20 |
| Passed | 12 |
| Blockers | 4 |
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
| Secret operator CRDs | 1 |
| Secret operator ready pods | 3 |
| ExternalSecrets ready | 1 / 1 |
| SealedSecrets synced | 0 / 0 |
| Production ExternalSecrets ready | 0 / 13 |
| Production ExternalSecrets missing | 13 |
| Production ExternalSecrets not ready | 0 |
| Secret reconciliation blockers | 1 |
| Keycloak live profile blockers | 0 |

## Key Artifacts

- `doc/02_acceptance/runs/20260629-production-security-preflight-r43-eso-canary/live-production-security-preflight-20260629-production-security-preflight-r43-eso-canary-summary.json`
- `doc/02_acceptance/runs/20260629-production-security-preflight-r43-eso-canary/live-production-security-preflight-20260629-production-security-preflight-r43-eso-canary.ndjson`
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
- `live-external-secret-reconciliation-summary.json`
- `live-secret-operator-crds.json`
- `live-secret-operator-pods.json`
- `live-externalsecrets-inventory.json`
- `live-sealedsecrets-inventory.json`
- `expected-production-externalsecrets.json`
- `live-production-externalsecret-readiness.json`
- `live-keycloak-profile-summary.json`

## Interpretation

The starter NetworkPolicy profile can be client-side dry-run checked, production manifests no longer contain Kafka plaintext listener definitions, repo image references are covered by an explicit evidence lock, repo Service exposure is limited to the APISIX business port, and kubectl apply convergence behavior is explicitly checked. The live cluster remains blocked for production security when the live Kafka SASL_SSL/TLS listener, NetworkPolicy-capable CNI, operator-backed ExternalSecret/SealedSecret reconciliation, or the production ExternalSecret templates report blockers. Keycloak TLS/SecretRef readiness and digest-pinned live workload images are measured by their own counters in this report instead of being assumed.
