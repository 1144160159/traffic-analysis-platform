# Codex Loop Release Manifest

- run_id: `mvp-32-release-with-executor-pool-stress`
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
- workspace_isolation: `doc/02_acceptance/runs/mvp-32-executor-pool-stress-local-clone-i001/executor-pool/workspace-isolation.json`
- workspace_isolation_status: `WORKSPACE_ISOLATION_READY`
- workspace_cleanup: `doc/02_acceptance/runs/mvp-32-executor-pool-stress-local-clone-i001-cleanup/workspace-cleanup/cleanup-plan.json`
- workspace_cleanup_status: `WORKSPACE_CLEANUP_COMPLETED`
- executor_pool: `doc/02_acceptance/runs/mvp-32-executor-pool-stress-local-clone-i001/executor-pool/executor-pool-summary.json`
- executor_pool_status: `EXECUTOR_POOL_PLANNED`
- executor_pool_activate_workspaces: `True`
- executor_pool_stress: `doc/02_acceptance/runs/mvp-32-executor-pool-stress-local-clone/executor-pool-stress/stress-summary.json`
- executor_pool_stress_status: `EXECUTOR_POOL_STRESS_COMPLETED`
- executor_pool_stress_workspace_backend: `local-clone`
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
