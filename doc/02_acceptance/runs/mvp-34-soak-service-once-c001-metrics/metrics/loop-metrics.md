# Codex Loop Runtime Metrics

- generated_at: `2026-06-24T06:48:13`
- run_total: `260`
- queue_total: `1`
- queue_counts: `{'done': 1}`
- resource_monitor: `RESOURCE_MONITOR_DEGRADED`
- lock_present: `False`
- lock_expired: `False`

## Run Status
- `CODEX_RUNNER_BLOCKED`: `1`
- `CODEX_RUNNER_PLANNED`: `1`
- `CONTEXT_PACKED`: `1`
- `CONTEXT_SCOUTED`: `45`
- `DAEMON_COMPLETED`: `11`
- `DEPLOY_PLAN_READY`: `8`
- `DESIGN_ITERATING`: `11`
- `EXECUTOR_POOL_BLOCKED`: `4`
- `EXECUTOR_POOL_PLANNED`: `13`
- `EXECUTOR_POOL_STRESS_BLOCKED`: `1`
- `EXECUTOR_POOL_STRESS_COMPLETED`: `3`
- `GUIDANCE_GENERATED`: `13`
- `HISTORICAL_SCAFFOLD`: `2`
- `IMPLEMENTATION_BLOCKED`: `1`
- `LOCK_ACQUIRED`: `11`
- `METRICS_COLLECTED`: `12`
- `PLANNED`: `1`
- `QUEUE_SERVICE_SMOKE_PASSED`: `4`
- `RELEASE_BLOCKED`: `2`
- `RELEASE_FROZEN`: `17`
- `RESOURCE_MONITOR_DEGRADED`: `2`
- `RESOURCE_QUOTA_READY`: `2`
- `RUNTIME_PREFLIGHT_DEGRADED`: `3`
- `RUNTIME_PREFLIGHT_READY`: `4`
- `SANDBOX_EXECUTION_BLOCKED`: `2`
- `SANDBOX_EXECUTION_PLANNED`: `1`
- `SANDBOX_PLAN_BLOCKED`: `1`
- `SANDBOX_PLAN_READY`: `21`
- `SANDBOX_WORKER_EXECUTION_BLOCKED`: `1`
- `SANDBOX_WORKER_PLANNED`: `18`
- `SANDBOX_WORKER_QUEUE_GUARD_BLOCKED`: `1`
- `SCHEDULER_PLANNED`: `5`
- `SERVICE_COMPLETED`: `2`
- `SERVICE_HEALTHY`: `8`
- `SERVICE_ONCE_COMPLETED`: `5`
- `SERVICE_RECOVERED`: `1`
- `TASK_STATE_PLANNED`: `1`
- `WORKER_COMPLETED`: `8`
- `WORKSPACE_CLEANUP_COMPLETED`: `10`
- `WORKSPACE_CLEANUP_PLANNED`: `1`
- `WORKSPACE_ISOLATION_DEGRADED`: `1`

## Run Kinds
- `codex_runner`: `2`
- `context_pack`: `1`
- `context_scout`: `45`
- `daemon`: `11`
- `deploy_plan`: `8`
- `design_package`: `1`
- `executor_pool`: `17`
- `executor_pool_stress`: `4`
- `guidance`: `13`
- `historical_scaffold`: `2`
- `implementation_guard`: `1`
- `metrics`: `12`
- `queue_service`: `4`
- `release_freeze`: `19`
- `resource_monitor`: `2`
- `resource_quota`: `2`
- `runtime_preflight`: `7`
- `sandbox_execution`: `3`
- `sandbox_plan`: `22`
- `sandbox_worker`: `20`
- `scheduler`: `16`
- `service`: `7`
- `service_health`: `8`
- `service_recover`: `1`
- `task_state`: `1`
- `unknown`: `1`
- `worker`: `8`
- `workflow_run`: `10`
- `workspace_cleanup`: `11`
- `workspace_isolation`: `1`

## Latest Runs
- `mvp-34-soak-service-once-c001-resource-monitor` `resource_monitor` `RESOURCE_MONITOR_DEGRADED`
- `mvp-24-resource-monitor` `resource_monitor` `RESOURCE_MONITOR_DEGRADED`
- `mvp-34-soak-service-once-c001-health` `service_health` `SERVICE_HEALTHY`
- `mvp-34-soak-service-once-c001-runner` `service` `SERVICE_ONCE_COMPLETED`
- `mvp-34-soak-service-once-c001-runner-daemon` `daemon` `DAEMON_COMPLETED`
- `mvp-34-soak-service-once-c001-runner-daemon-i1-metrics` `metrics` `METRICS_COLLECTED`
- `mvp-34-soak-service-once-c001-runner-daemon-i1-scheduler` `scheduler` `LOCK_ACQUIRED`
- `mvp-34-soak-service-once-c001-runner-daemon-i1-worker` `sandbox_worker` `SANDBOX_WORKER_PLANNED`
- `mvp-34-soak-service-once-c001-runner-daemon-i1-worker-cle-p0-screen-001-sandbox` `sandbox_plan` `SANDBOX_PLAN_READY`
- `mvp-34-soak-service-once-c001-runner-daemon-i1` `guidance` `GUIDANCE_GENERATED`
- `mvp-34-soak-service-once-c001-runner-daemon-i1-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-33-final-executor-pool-stress-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-33-release-with-executor-pool-stress` `release_freeze` `RELEASE_FROZEN`
- `mvp-33-executor-pool-stress-local-clone` `executor_pool_stress` `EXECUTOR_POOL_STRESS_COMPLETED`
- `mvp-33-executor-pool-stress-local-clone-i002-cleanup` `workspace_cleanup` `WORKSPACE_CLEANUP_COMPLETED`
- `mvp-33-executor-pool-stress-local-clone-i002` `executor_pool` `EXECUTOR_POOL_PLANNED`
- `mvp-33-executor-pool-stress-local-clone-i002-cle-p0-screen-001-sandbox-plan` `sandbox_worker` `SANDBOX_WORKER_PLANNED`
- `mvp-33-executor-pool-stress-local-clone-i002-cle-p0-screen-001-sandbox-plan-cle-p0-screen-001-sandbox` `sandbox_plan` `SANDBOX_PLAN_READY`
- `mvp-33-executor-pool-stress-local-clone-i001-cleanup` `workspace_cleanup` `WORKSPACE_CLEANUP_COMPLETED`
- `mvp-33-executor-pool-stress-local-clone-i001` `executor_pool` `EXECUTOR_POOL_PLANNED`

## Queue Items
- `CLE-P0-SCREEN-001` state `done` attempts `2/3`

## Guardrail
- Metrics are observational evidence only; they do not close tasks or mutate task YAML.
