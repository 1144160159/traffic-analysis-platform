# Deployment Preflight

Run: `20260712-campaign-r302-r214-r3-deployment-r1`

Result: `blocked`

This is a non-destructive preflight for GATE-P0-09. It does not apply manifests, mutate live resources, or read secret values.

## Summary

| Metric | Count |
|---|---:|
| Checks | 16 |
| Passed | 15 |
| Blockers | 1 |
| Warnings | 0 |
| Release package files | 190 |
| Repo image lock missing lines | 0 |
| Repo mutable/latest image lines with lock evidence | 0 |
| Repo non-business external service ports | 0 |
| Apply-convergence non-business external service ports | 0 |
| Live unpinned/latest workload spec image containers | 0 |
| Non-business external ports | 0 |
| Pending PVCs | 0 |

## Key Artifacts

- `doc/02_acceptance/runs/20260712-campaign-r302-r214-r3-deployment-r1/live-deployment-preflight-20260712-campaign-r302-r214-r3-deployment-r1-summary.json`
- `doc/02_acceptance/runs/20260712-campaign-r302-r214-r3-deployment-r1/live-deployment-preflight-20260712-campaign-r302-r214-r3-deployment-r1.ndjson`
- `release-package-manifest.json`
- `repo-image-lock-summary.json`
- `repo-image-lock-inventory.json`
- `repo-service-exposure-summary.json`
- `repo-service-exposure-blockers.json`
- `apply-convergence-service-exposure-summary.json`
- `apply-convergence-service-exposure-blockers.json`
- `live-site-values-observed.json`
- `k8s-core-dry-run.txt`
- `secret-reference-readiness.json`
- `live-workload-readiness.json`
- `live-workload-images.json`
- `live-unpinned-or-latest-images.json`
- `live-non-business-external-ports.json`

## Interpretation

The site-values template, release package manifest, image digest evidence lock, APISIX-only repo Service exposure profile, live APISIX-only Service exposure, and kubectl apply convergence behavior now have evidence artifacts. The script captures enough live state to reproduce or reject a site deployment. GATE-P0-09 remains blocked until live workload image specs are rolled to pullable digest references, production security blockers are cleared, and the release package is promoted from evidence package to signed/released artifact.
