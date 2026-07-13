# Deployment state-machine live preflight

- Run ID: `20260629-deployment-state-machine-r3-model-version-state`
- Result: `pass`
- APISIX: `http://10.0.5.8:30180`
- Deployment: `e4cec613-5a60-4574-b20e-90dc97637870`
- Checks: 25/25 passed, blockers=0, warnings=0

## Evidence

- NDJSON: `doc/02_acceptance/runs/20260629-deployment-state-machine/live-deployment-state-machine-20260629-deployment-state-machine-r3-model-version-state.ndjson`
- Summary: `doc/02_acceptance/runs/20260629-deployment-state-machine/live-deployment-state-machine-20260629-deployment-state-machine-r3-model-version-state-summary.json`
- API/DB/Audit responses: `doc/02_acceptance/runs/20260629-deployment-state-machine/20260629-deployment-state-machine-r3-model-version-state-*.json`, `doc/02_acceptance/runs/20260629-deployment-state-machine/20260629-deployment-state-machine-r3-model-version-state-*.txt`

## Scope

This report validates the deployment action state machine: planned deployments can enter gray, gray deployments can activate, active deployments can pause/resume/rollback, activation supersedes previous active deployments, invalid planned rollback returns 409, cross-tenant and read-only requests return 403, deployment history is persisted, and `DEPLOY_GRAY` / `DEPLOY_ACTIVATE` / `DEPLOY_PAUSE` / `DEPLOY_RESUME` / `DEPLOY_ROLLBACK` are queryable through `audit_logs`.
