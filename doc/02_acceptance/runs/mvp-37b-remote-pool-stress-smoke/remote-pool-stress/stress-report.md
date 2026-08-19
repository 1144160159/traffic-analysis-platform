# Codex Loop Remote Pool Stress

- run_id: `mvp-37b-remote-pool-stress-smoke`
- status: `REMOTE_POOL_STRESS_COMPLETED`
- workers: `4`
- tasks: `8`
- rounds: `3`
- backend: `http`
- queue_path: `doc/02_acceptance/runs/mvp-37b-remote-pool-stress-smoke/remote-pool-stress/queue.sqlite3`
- duplicate_successful_claims: `0`
- successful_completions: `8`

## Worker Results
- `mvp-37b-remote-pool-stress-smoke-remote-worker-01` claims `0` completions `0`
- `mvp-37b-remote-pool-stress-smoke-remote-worker-02` claims `3` completions `3`
- `mvp-37b-remote-pool-stress-smoke-remote-worker-03` claims `2` completions `2`
- `mvp-37b-remote-pool-stress-smoke-remote-worker-04` claims `2` completions `2`

## Lease Integrity
- passed: `True` task: `REMOTE-POOL-STRESS-001`

## Findings
- none

## Guardrail
- This stress only validates remote queue arbitration over loopback HTTP and SQLite/WAL.
- It does not execute product tasks, live writes, external Codex, or sandbox jobs.
