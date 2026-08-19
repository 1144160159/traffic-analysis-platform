# Codex Loop Deployment Plan

- run_id: `mvp-25-deploy-http-queue-worker`
- status: `DEPLOY_PLAN_READY`
- profile: `http_queue_worker_k8s`
- target: `kubernetes`

## Outputs
- `deploy/deploy-plan.json`
- `deploy/deploy-report.md`
- `deploy/codex-loop-pvc.yaml`
- `deploy/codex-loop-cronjob.yaml`
- `deploy/kustomization.yaml`
- `deploy/validation.json`
- `deploy/kubectl-dry-run.txt`

## Findings
- none

## Validation
- `kubectl_dry_run` exit `0`

## Guardrail
- This script only renders manifests; it does not install systemd units or apply Kubernetes YAML.
- The default profile keeps one worker, prepare stage, no live write, and no external Codex execution.
