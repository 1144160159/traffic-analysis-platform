# ExternalSecret Operator Canary

Run: `20260629-external-secret-operator-canary-r1`

Result: `blocked`

This live canary installs or verifies External Secrets Operator and proves one
operator-backed Secret reconciliation without touching production
`traffic-credentials` or Kafka TLS secrets.

## Summary

| Metric | Count |
|---|---:|
| Checks | 11 |
| Passed | 3 |
| Blockers | 8 |
| Warnings | 0 |
| ESO ready deployments | 0 |
| ESO digest-pinned images | 0 |
| ESO unpinned images | 3 |
| Canary SecretStore ready | false |
| Canary ExternalSecret ready | false |
| Canary target Secret exists | false |
| Canary source/target data match | false |

## Key Artifacts

- `doc/02_acceptance/runs/20260629-external-secret-operator-canary-r1/live-external-secret-operator-canary-20260629-external-secret-operator-canary-r1-summary.json`
- `doc/02_acceptance/runs/20260629-external-secret-operator-canary-r1/live-external-secret-operator-canary-20260629-external-secret-operator-canary-r1.ndjson`
- `eso-deployment-summary.json`
- `eso-crds.json`
- `canary-reconciliation-summary.json`
- `canary-secretstore.json`
- `canary-externalsecret.json`
