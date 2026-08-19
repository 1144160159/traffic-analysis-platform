# Codex Loop Remote Pool Stress

- run_id: `mvp-38-remote-pool-embedded-regression`
- status: `REMOTE_POOL_STRESS_COMPLETED`
- workers: `4`
- tasks: `8`
- rounds: `3`
- backend: `http`
- service_mode: `embedded-loopback`
- queue_path: `doc/02_acceptance/runs/mvp-38-remote-pool-embedded-regression/remote-pool-stress/queue.sqlite3`
- target_counts: `{'counts': {'done': 8}, 'missing': [], 'seen': 8}`
- duplicate_successful_claims: `0`
- successful_completions: `8`

## Worker Results
- `mvp-38-remote-pool-embedded-regression-remote-worker-01` claims `4` completions `4`
- `mvp-38-remote-pool-embedded-regression-remote-worker-02` claims `0` completions `0`
- `mvp-38-remote-pool-embedded-regression-remote-worker-03` claims `3` completions `3`
- `mvp-38-remote-pool-embedded-regression-remote-worker-04` claims `0` completions `0`

## Lease Integrity
- passed: `True` task: `REMOTE-POOL-STRESS-mvp-38-remote-pool-embedded-regression-001`

## Findings
- none

## Guardrail
- This stress validates remote queue arbitration over HTTP and only counts this run's synthetic tasks.
- It does not execute product tasks, live writes, external Codex, or sandbox jobs.
- External queue services require explicit authorization and are not stopped unless --stop-external-service is set.
