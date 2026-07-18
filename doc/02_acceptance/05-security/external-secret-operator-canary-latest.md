# ExternalSecret Operator Canary

Run: `20260629-external-secret-operator-canary-r2-local-chart`

Result: `pass`

This live canary installs or verifies External Secrets Operator and proves one
operator-backed Secret reconciliation without touching production
`traffic-credentials` or Kafka TLS secrets.

## Summary

| Metric | Count |
|---|---:|
| Checks | 11 |
| Passed | 11 |
| Blockers | 0 |
| Warnings | 0 |
| ESO ready deployments | 3 |
| ESO digest-pinned images | 3 |
| ESO unpinned images | 0 |
| Canary SecretStore ready | true |
| Canary ExternalSecret ready | true |
| Canary target Secret exists | true |
| Canary source/target data match | true |

## Key Artifacts

- `doc/02_acceptance/runs/20260629-external-secret-operator-canary-r2-local-chart/live-external-secret-operator-canary-20260629-external-secret-operator-canary-r2-local-chart-summary.json`
- `doc/02_acceptance/runs/20260629-external-secret-operator-canary-r2-local-chart/live-external-secret-operator-canary-20260629-external-secret-operator-canary-r2-local-chart.ndjson`
- `eso-deployment-summary.json`
- `eso-crds.json`
- `canary-reconciliation-summary.json`
- `canary-secretstore.json`
- `canary-externalsecret.json`
