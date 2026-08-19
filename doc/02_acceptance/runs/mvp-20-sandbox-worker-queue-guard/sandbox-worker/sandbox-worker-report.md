# Codex Loop Sandbox Worker Report

- run_id: `mvp-20-sandbox-worker-queue-guard`
- status: `SANDBOX_WORKER_QUEUE_GUARD_BLOCKED`
- stage: `prepare`
- driver: `kubernetes-job`
- execute_sandbox: `False`
- claim_queue: `True`
- lock_status: `not_required`
- selected: `1`

## Sandbox Plans
- none

## Sandbox Executions
- none

## Queue
- not used

## Guardrail
- Plan-only mode does not claim or mutate the persistent queue.
- Queue mutation requires --claim-queue and --execute-sandbox.
- Real sandbox execution is still gated by sandbox_executor.py and its environment policy.
