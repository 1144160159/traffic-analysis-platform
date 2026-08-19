# Codex Loop Deployment Plan

- run_id: `mvp-28-deploy-sqlite-pool-default`
- status: `DEPLOY_PLAN_READY`
- profile: `sqlite_pool_plan`
- target: `systemd`

## Outputs
- `deploy/deploy-plan.json`
- `deploy/deploy-report.md`
- `deploy/codex-loop.service`

## Findings
- none

## Guardrail
- This script only renders manifests; it does not install systemd units or apply Kubernetes YAML.
- The default profile keeps one worker, prepare stage, no live write, and no external Codex execution.
