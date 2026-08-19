# Codex Loop Remote Pool Stress

- run_id: `mvp-38-remote-pool-nonloopback-guard`
- status: `REMOTE_POOL_STRESS_BLOCKED`
- workers: `1`
- tasks: `1`
- rounds: `3`
- backend: `http`
- service_mode: `external`
- queue_path: `http://codex-loop-queue-service.traffic-analysis.svc:8765`
- target_counts: `{'counts': {}, 'missing': ['REMOTE-POOL-STRESS-mvp-38-remote-pool-nonloopback-guard-001'], 'seen': 0}`
- duplicate_successful_claims: `0`
- successful_completions: `0`

## Worker Results

## Lease Integrity
- passed: `None` task: `None`

## Findings
- `blocker` `REMOTE_POOL_EXTERNAL_SERVICE_NOT_ALLOWED`: Non-loopback queue service stress requires --allow-external-service or CODEX_LOOP_ALLOW_REMOTE_POOL_STRESS=1.
- `blocker` `REMOTE_POOL_EXTERNAL_TOKEN_REQUIRED`: Non-loopback queue service stress requires CODEX_LOOP_QUEUE_TOKEN or --auth-token.

## Guardrail
- This stress validates remote queue arbitration over HTTP and only counts this run's synthetic tasks.
- It does not execute product tasks, live writes, external Codex, or sandbox jobs.
- External queue services require explicit authorization and are not stopped unless --stop-external-service is set.
