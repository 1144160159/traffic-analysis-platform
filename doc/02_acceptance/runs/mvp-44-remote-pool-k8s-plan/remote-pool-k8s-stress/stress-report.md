# Codex Loop Remote Pool K8s Stress

- run_id: `mvp-44-remote-pool-k8s-plan`
- status: `REMOTE_POOL_K8S_STRESS_VALIDATED`
- namespace: `traffic-analysis`
- service_url: `http://codex-loop-queue-service.traffic-analysis.svc:8765`
- workers: `3`
- tasks: `6`
- validate_requested: `True`
- execute_requested: `False`
- job_name: `codex-loop-rpks-mvp-44-remote-pool-k8s-plan`

## Findings
- none

## Outputs
- `remote-pool-k8s-stress/stress-summary.json`
- `remote-pool-k8s-stress/stress-report.md`
- `remote-pool-k8s-stress/seed-plan.json`
- `remote-pool-k8s-stress/remote-pool-worker-job.yaml`
- `remote-pool-k8s-stress/command-template.txt`
- `remote-pool-k8s-stress/kubectl-dry-run.txt`

## Guardrail
- Default mode only renders manifests; real apply requires --execute and CODEX_LOOP_ALLOW_K8S_REMOTE_POOL_STRESS=1.
- Worker pods only claim and complete synthetic queue tasks through the HTTP queue service.
- This evidence does not close product tasks or replace long-running soak.
