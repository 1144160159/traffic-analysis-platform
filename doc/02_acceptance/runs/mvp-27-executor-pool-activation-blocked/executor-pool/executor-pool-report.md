# Codex Loop Executor Pool

- run_id: `mvp-27-executor-pool-activation-blocked`
- status: `EXECUTOR_POOL_BLOCKED`
- runner: `sandbox-plan`
- max_workers: `2`
- requested_max_workers: `2`
- selected: `1`
- executed: `0`

## Resource Admission
- monitor_status: `skipped`
- adjustment: `none`

## Workspace Isolation
- isolation_status: `WORKSPACE_ISOLATION_DEGRADED`
- isolation_mode: `worktree-plan`
- activate_workspaces: `True`

## Findings
- `blocker` `WORKSPACE_NOT_CREATED`: Selected task `CLE-P0-SCREEN-001` workspace has not been created; run with --create-worktrees and CODEX_LOOP_ALLOW_WORKTREE_CREATE=1 first.

## Children
- none

## Guardrail
- The pool is bounded by --max-workers and --max-tasks.
- Parallel queue mutation requires sqlite and --allow-parallel-execution.
- sandbox-plan is the default safe runner and does not claim queue items.
