# Codex Loop Deployment Plan

- run_id: `mvp-15-sqlite-deploy-plan`
- status: `DEPLOY_PLAN_READY`
- profile: `sqlite_conservative`
- target: `all`

## Outputs
- `deploy/deploy-plan.json`
- `deploy/deploy-report.md`
- `deploy/codex-loop.service`
- `deploy/codex-loop-pvc.yaml`
- `deploy/codex-loop-cronjob.yaml`
- `deploy/kustomization.yaml`
- `deploy/validation.json`
- `deploy/kubectl-dry-run.txt`
- `deploy/systemd-verify.txt`

## Findings
- none

## Validation
- `kubectl_dry_run` exit `0`
- `systemd_verify` exit `0`

## Guardrail
- This script only renders manifests; it does not install systemd units or apply Kubernetes YAML.
- The default profile keeps one worker, prepare stage, no live write, and no external Codex execution.
