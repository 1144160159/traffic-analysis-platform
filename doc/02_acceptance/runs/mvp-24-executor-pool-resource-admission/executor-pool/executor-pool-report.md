# Codex Loop Executor Pool

- run_id: `mvp-24-executor-pool-resource-admission`
- status: `EXECUTOR_POOL_PLANNED`
- runner: `sandbox-plan`
- max_workers: `1`
- requested_max_workers: `2`
- selected: `1`
- executed: `1`

## Resource Admission
- monitor_status: `RESOURCE_MONITOR_DEGRADED`
- adjustment: `{'reason': 'resource monitor degraded; serializing pool execution', 'requested_max_workers': 2, 'effective_max_workers': 1}`

## Findings
- none

## Children
- `CLE-P0-SCREEN-001` -> `mvp-24-executor-pool-resource-admission-cle-p0-screen-001-sandbox-plan` exit `0` status `SANDBOX_WORKER_PLANNED`

## Guardrail
- The pool is bounded by --max-workers and --max-tasks.
- Parallel queue mutation requires sqlite and --allow-parallel-execution.
- sandbox-plan is the default safe runner and does not claim queue items.
