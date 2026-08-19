# Codex Loop K8s Bootstrap

- run_id: `mvp-42-k8s-bootstrap-queue-service-plan`
- status: `K8S_BOOTSTRAP_VALIDATED`
- namespace: `traffic-analysis`
- deploy_dir: `doc/02_acceptance/runs/mvp-42-queue-service-k8s-deploy-plan/deploy`
- execute_requested: `False`
- execute_gate_accepted: `False`
- token_env_present: `False`

## Findings
- `warning` `K8S_BOOTSTRAP_TOKEN_ENV_NOT_SET`: CODEX_LOOP_QUEUE_TOKEN was not set; secret dry-run used a placeholder.

## Outputs
- `k8s-bootstrap/bootstrap-summary.json`
- `k8s-bootstrap/bootstrap-report.md`
- `k8s-bootstrap/command-template.txt`
- `k8s-bootstrap/kubectl-dry-run.txt`
- `k8s-bootstrap/secret-dry-run.txt`

## Guardrail
- Default mode validates manifests and secret creation only; it does not apply resources.
- Real apply requires --execute, CODEX_LOOP_ALLOW_K8S_BOOTSTRAP=1, and CODEX_LOOP_QUEUE_TOKEN.
- Secret values are never written to evidence.
