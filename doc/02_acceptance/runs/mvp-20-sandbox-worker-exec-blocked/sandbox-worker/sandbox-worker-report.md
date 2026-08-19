# Codex Loop Sandbox Worker Report

- run_id: `mvp-20-sandbox-worker-exec-blocked`
- status: `SANDBOX_WORKER_EXECUTION_BLOCKED`
- stage: `prepare`
- driver: `kubernetes-job`
- execute_sandbox: `True`
- claim_queue: `False`
- lock_status: `not_required`
- selected: `1`

## Sandbox Plans
- `CLE-P0-SCREEN-001` -> `mvp-20-sandbox-worker-exec-blocked-cle-p0-screen-001-sandbox` exit `0` status `SANDBOX_PLAN_READY`

## Sandbox Executions
- `CLE-P0-SCREEN-001` -> `mvp-20-sandbox-worker-exec-blocked-cle-p0-screen-001-sandbox-exec` exit `2` status `SANDBOX_EXECUTION_BLOCKED`

## Queue
- not used

## Guardrail
- Plan-only mode does not claim or mutate the persistent queue.
- Queue mutation requires --claim-queue and --execute-sandbox.
- Real sandbox execution is still gated by sandbox_executor.py and its environment policy.
