# Codex Loop Deployment Plan

- run_id: `mvp-22-deploy-sqlite-pool-plan`
- status: `DEPLOY_PLAN_READY`
- profile: `sqlite_pool_plan`
- target: `kubernetes`

## Outputs
- `deploy/deploy-plan.json`
- `deploy/deploy-report.md`
- `deploy/codex-loop-pvc.yaml`
- `deploy/codex-loop-cronjob.yaml`
- `deploy/kustomization.yaml`

## Findings
- none

## Guardrail
- This script only renders manifests; it does not install systemd units or apply Kubernetes YAML.
- The default profile keeps one worker, prepare stage, no live write, and no external Codex execution.
