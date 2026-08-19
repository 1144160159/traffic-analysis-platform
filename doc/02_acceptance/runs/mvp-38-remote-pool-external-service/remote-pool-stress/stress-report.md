# Codex Loop Remote Pool Stress

- run_id: `mvp-38-remote-pool-external-service`
- status: `REMOTE_POOL_STRESS_COMPLETED`
- workers: `3`
- tasks: `6`
- rounds: `2`
- backend: `http`
- service_mode: `external`
- queue_path: `http://127.0.0.1:18765`
- target_counts: `{'counts': {'done': 6}, 'missing': [], 'seen': 6}`
- duplicate_successful_claims: `0`
- successful_completions: `6`

## Worker Results
- `mvp-38-remote-pool-external-service-remote-worker-01` claims `2` completions `2`
- `mvp-38-remote-pool-external-service-remote-worker-02` claims `2` completions `2`
- `mvp-38-remote-pool-external-service-remote-worker-03` claims `1` completions `1`

## Lease Integrity
- passed: `True` task: `REMOTE-POOL-STRESS-mvp-38-remote-pool-external-service-001`

## Findings
- none

## Guardrail
- This stress validates remote queue arbitration over HTTP and only counts this run's synthetic tasks.
- It does not execute product tasks, live writes, external Codex, or sandbox jobs.
- External queue services require explicit authorization and are not stopped unless --stop-external-service is set.
