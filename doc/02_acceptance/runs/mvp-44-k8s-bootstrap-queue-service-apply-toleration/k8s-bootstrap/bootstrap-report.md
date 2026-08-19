# Codex Loop K8s Bootstrap

- run_id: `mvp-44-k8s-bootstrap-queue-service-apply-toleration`
- status: `K8S_BOOTSTRAP_APPLIED`
- namespace: `traffic-analysis`
- deploy_dir: `doc/02_acceptance/runs/mvp-44-queue-service-k8s-deploy-plan-pv-toleration/deploy`
- execute_requested: `True`
- execute_gate_accepted: `True`
- token_env_present: `True`

## Findings
- none

## Outputs
- `k8s-bootstrap/bootstrap-summary.json`
- `k8s-bootstrap/bootstrap-report.md`
- `k8s-bootstrap/command-template.txt`
- `k8s-bootstrap/kubectl-dry-run.txt`
- `k8s-bootstrap/secret-dry-run.txt`
- `k8s-bootstrap/apply-pv.txt`
- `k8s-bootstrap/apply-pvc.txt`
- `k8s-bootstrap/apply-deployment.txt`
- `k8s-bootstrap/apply-service.txt`
- `k8s-bootstrap/rollout-status.txt`

## Guardrail
- Default mode validates manifests and secret creation only; it does not apply resources.
- Real apply requires --execute, CODEX_LOOP_ALLOW_K8S_BOOTSTRAP=1, and CODEX_LOOP_QUEUE_TOKEN.
- Secret values are never written to evidence.
