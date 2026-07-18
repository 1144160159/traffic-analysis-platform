# ExternalSecret Production Reconciliation

Run: `20260629-external-secret-production-reconciliation-r1`

Result: `pass`

This live run seeds a Kubernetes-provider source namespace from existing live
Secrets, applies the production ClusterSecretStore and ExternalSecrets, waits for
operator-backed reconciliation, and verifies key-level source/target equality
without writing secret values to artifacts.

## Summary

| Metric | Count |
|---|---:|
| Checks | 10 |
| Passed | 10 |
| Blockers | 0 |
| Warnings | 0 |
| Expected ExternalSecrets | 13 |
| Expected keys | 58 |
| Source Secret copy blockers | 0 |
| Production ClusterSecretStore ready | 1 / 1 |
| Production ExternalSecrets live | 13 / 13 |
| Production ExternalSecrets ready | 13 / 13 |
| Production ExternalSecrets missing | 0 |
| Production ExternalSecrets not ready | 0 |
| Post-reconcile source missing keys | 0 |
| Post-reconcile target missing keys | 0 |
| Post-reconcile mismatched keys | 0 |

## Key Artifacts

- `doc/02_acceptance/runs/20260629-external-secret-production-reconciliation-r1/live-external-secret-production-reconciliation-20260629-external-secret-production-reconciliation-r1-summary.json`
- `doc/02_acceptance/runs/20260629-external-secret-production-reconciliation-r1/live-external-secret-production-reconciliation-20260629-external-secret-production-reconciliation-r1.ndjson`
- `preflight-secret-key-readiness.json`
- `live-production-secretstores.json`
- `live-production-externalsecret-readiness.json`
- `post-reconcile-secret-key-readiness.json`
- `source-secret-inventory.json`
