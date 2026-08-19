# Codex Loop Deployment Plan

- run_id: `mvp-43-control-only-k8s-guard`
- status: `DEPLOY_PLAN_BLOCKED`
- profile: `http_queue_worker_k8s`
- target: `kubernetes`
- image_layout: `control-only`

## Outputs
- `deploy/deploy-plan.json`
- `deploy/deploy-report.md`
- `deploy/codex-loop-pvc.yaml`
- `deploy/codex-loop-cronjob.yaml`
- `deploy/kustomization.yaml`
- `deploy/validation.json`
- `deploy/kubectl-dry-run.txt`

## Findings
- `blocker` `K8S_FULL_REPO_IMAGE_REQUIRED`: Kubernetes profiles that execute service/executor workflows require a full-repo image or mounted repo workspace; control-only images are limited to queue service and synthetic remote-pool workers.

## Validation
- `kubectl_dry_run` exit `0`

## Guardrail
- This script only renders manifests; it does not install systemd units or apply Kubernetes YAML.
- The default profile keeps one worker, prepare stage, no live write, and no external Codex execution.
