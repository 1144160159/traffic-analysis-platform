# Codex Loop Executor Pool

- run_id: `mvp-22-executor-pool-parallel-guard`
- status: `EXECUTOR_POOL_BLOCKED`
- runner: `sandbox-execute`
- max_workers: `2`
- selected: `1`
- executed: `0`

## Findings
- `blocker` `PARALLEL_EXECUTION_GATE_NOT_SET`: Set --allow-parallel-execution before running non-plan workers with max_workers > 1.
- `blocker` `PARALLEL_EXECUTION_REQUIRES_SQLITE`: Parallel queue mutation requires the sqlite queue backend.

## Children
- none

## Guardrail
- The pool is bounded by --max-workers and --max-tasks.
- Parallel queue mutation requires sqlite and --allow-parallel-execution.
- sandbox-plan is the default safe runner and does not claim queue items.
