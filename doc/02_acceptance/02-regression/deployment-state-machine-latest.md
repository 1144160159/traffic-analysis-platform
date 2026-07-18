# Deployment state-machine live preflight

- Run ID: `20260718182953-deployment-state-machine`
- Result: `pass`
- APISIX: `http://10.0.5.8:30180`
- Deployment: `4e034fb8-3cf9-4714-a4e0-66f5bbcf7446`
- Checks: 70/70 passed, blockers=0, warnings=0

## Evidence

- NDJSON: `doc/02_acceptance/runs/20260629-deployment-state-machine/live-deployment-state-machine-20260718182953-deployment-state-machine.ndjson`
- Summary: `doc/02_acceptance/runs/20260629-deployment-state-machine/live-deployment-state-machine-20260718182953-deployment-state-machine-summary.json`
- API/DB/Audit responses: `doc/02_acceptance/runs/20260629-deployment-state-machine/20260718182953-deployment-state-machine-*.json`, `doc/02_acceptance/runs/20260629-deployment-state-machine/20260718182953-deployment-state-machine-*.txt`

## Scope

This report validates the deployment action state machine: database-backed precheck and persisted approval are required before gray/rollback, planned deployments can enter gray, cross-record concurrent gray requests commit exactly once per tenant, gray deployments can activate, active deployments can pause/resume/rollback, activation supersedes previous active deployments, invalid planned rollback returns 409, cross-tenant and read-only requests return 403, deployment history is persisted, and `DEPLOY_GRAY` / `DEPLOY_ACTIVATE` / `DEPLOY_PAUSE` / `DEPLOY_RESUME` / `DEPLOY_ROLLBACK` are queryable through `audit_logs`.
