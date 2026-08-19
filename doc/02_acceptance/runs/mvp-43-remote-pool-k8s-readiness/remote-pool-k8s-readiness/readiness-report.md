# Codex Loop Remote Pool K8s Readiness

- run_id: `mvp-43-remote-pool-k8s-readiness`
- status: `REMOTE_POOL_K8S_READINESS_BLOCKED`
- namespace: `traffic-analysis`
- service_name: `codex-loop-queue-service`
- service_url: `http://codex-loop-queue-service.traffic-analysis.svc:8765`
- pvc_name: `codex-loop-workspace`
- secret_name: `codex-loop-queue-token`
- token_key_present: `False`
- worker_job: `doc/02_acceptance/runs/mvp-43-remote-pool-k8s-plan/remote-pool-k8s-stress/remote-pool-worker-job.yaml`
- execute_gate_present: `False`
- execute_gate_accepted: `False`

## Findings
- `blocker` `REMOTE_POOL_K8S_SERVICE_MISSING`: Service traffic-analysis/codex-loop-queue-service is not readable.
- `blocker` `REMOTE_POOL_K8S_PVC_MISSING`: PVC traffic-analysis/codex-loop-workspace is not readable.
- `blocker` `REMOTE_POOL_K8S_SECRET_MISSING`: Secret traffic-analysis/codex-loop-queue-token is not readable.
- `warning` `REMOTE_POOL_K8S_EXECUTE_GATE_NOT_ACCEPTED`: CODEX_LOOP_ALLOW_K8S_REMOTE_POOL_STRESS=1 is required before real K8s stress execution.

## Outputs
- `remote-pool-k8s-readiness/readiness-summary.json`
- `remote-pool-k8s-readiness/readiness-report.md`
- `remote-pool-k8s-readiness/kubectl-checks.json`
- `remote-pool-k8s-readiness/kubectl-dry-run.txt`

## Guardrail
- This audit is read-only and does not create, patch, delete, or execute Kubernetes workloads.
- Secret values are never written to evidence; only key presence is recorded.
- READY means the remote-pool K8s stress can be executed after the explicit execution gate is accepted.
