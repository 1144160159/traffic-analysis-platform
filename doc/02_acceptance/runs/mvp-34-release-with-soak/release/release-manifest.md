# Codex Loop Release Manifest

- run_id: `mvp-34-release-with-soak`
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
- workspace_isolation: `none`
- workspace_isolation_status: `none`
- workspace_cleanup: `none`
- workspace_cleanup_status: `none`
- executor_pool: `none`
- executor_pool_status: `none`
- executor_pool_activate_workspaces: `none`
- executor_pool_stress: `doc/02_acceptance/runs/mvp-33-executor-pool-stress-local-clone/executor-pool-stress/stress-summary.json`
- executor_pool_stress_status: `EXECUTOR_POOL_STRESS_COMPLETED`
- executor_pool_stress_workspace_backend: `local-clone`
- soak: `doc/02_acceptance/runs/mvp-34-soak-service-once/soak/soak-summary.json`
- soak_status: `SOAK_DEGRADED`
- soak_cycles_completed: `1`
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
