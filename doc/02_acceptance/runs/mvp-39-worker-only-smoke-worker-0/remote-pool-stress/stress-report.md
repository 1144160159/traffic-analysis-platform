# Codex Loop Remote Pool Stress

- run_id: `mvp-39-worker-only-smoke-worker-0`
- status: `REMOTE_POOL_WORKER_COMPLETED`
- run_kind: `remote_pool_worker`
- workers: `1`
- tasks: `4`
- rounds: `2`
- backend: `http`
- service_mode: `external`
- queue_path: `http://127.0.0.1:18766`
- target_counts: `None`
- duplicate_successful_claims: `None`
- successful_completions: `4`

## Worker Results
- `mvp-39-worker-only-smoke-worker-0-remote-worker-01` claims `4` completions `4`

## Lease Integrity
- passed: `None` task: `None`

## Findings
- none

## Guardrail
- This stress validates remote queue arbitration over HTTP and only counts this run's synthetic tasks.
- It does not execute product tasks, live writes, external Codex, or sandbox jobs.
- External queue services require explicit authorization and are not stopped unless --stop-external-service is set.
