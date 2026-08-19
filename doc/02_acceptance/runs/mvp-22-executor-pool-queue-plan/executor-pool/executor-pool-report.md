# Codex Loop Executor Pool

- run_id: `mvp-22-executor-pool-queue-plan`
- status: `EXECUTOR_POOL_PLANNED`
- runner: `sandbox-plan`
- max_workers: `2`
- selected: `1`
- executed: `1`

## Findings
- none

## Children
- `CLE-P0-SCREEN-001` -> `mvp-22-executor-pool-queue-plan-cle-p0-screen-001-sandbox-plan` exit `0` status `SANDBOX_WORKER_PLANNED`

## Guardrail
- The pool is bounded by --max-workers and --max-tasks.
- Parallel queue mutation requires sqlite and --allow-parallel-execution.
- sandbox-plan is the default safe runner and does not claim queue items.
