# Codex Loop Sandbox Worker Report

- run_id: `mvp-20-service-sandbox-plan-daemon-i1-worker`
- status: `SANDBOX_WORKER_PLANNED`
- stage: `prepare`
- driver: `local-container`
- execute_sandbox: `False`
- claim_queue: `False`
- lock_status: `valid`
- selected: `1`

## Sandbox Plans
- `CLE-P0-SCREEN-001` -> `mvp-20-service-sandbox-plan-daemon-i1-worker-cle-p0-screen-001-sandbox` exit `0` status `SANDBOX_PLAN_READY`

## Sandbox Executions
- none

## Queue
- not used

## Guardrail
- Plan-only mode does not claim or mutate the persistent queue.
- Queue mutation requires --claim-queue and --execute-sandbox.
- Real sandbox execution is still gated by sandbox_executor.py and its environment policy.
