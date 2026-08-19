# Codex Loop Release Manifest

- run_id: `mvp-28-release-worktree-activated-final`
- status: `RELEASE_FROZEN`
- commit: `e3316aec4ac1d6592e28aefc86853128ecde7408`
- health: `HEALTHY`
- queue_counts: `{'done': 1}`
- deploy_plan: `None`
- sandbox_plan: `none`
- sandbox_status: `none`
- sandbox_execution: `none`
- sandbox_execution_status: `none`
- sandbox_worker: `none`
- sandbox_worker_status: `none`
- resource_quota: `none`
- resource_quota_status: `none`
- resource_monitor: `none`
- resource_monitor_status: `none`
- workspace_isolation: `doc/02_acceptance/runs/mvp-28-executor-pool-worktree-activated/executor-pool/workspace-isolation.json`
- workspace_isolation_status: `WORKSPACE_ISOLATION_READY`
- executor_pool: `doc/02_acceptance/runs/mvp-28-executor-pool-worktree-activated/executor-pool/executor-pool-summary.json`
- executor_pool_status: `EXECUTOR_POOL_PLANNED`
- executor_pool_activate_workspaces: `True`
- queue_service: `none`
- queue_service_status: `none`

## Evidence
- `release/release-manifest.json`
- `release/release-manifest.md`
- `release/rollback-plan.md`
- `release/git-status.txt`
- `release/loop-diff.patch`

## Guardrail
- This manifest freezes loop-engine evidence only; it is not a business acceptance pass.
