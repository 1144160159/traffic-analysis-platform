# Codex Loop Runtime Metrics

- generated_at: `2026-06-24T03:11:59`
- run_total: `179`
- queue_total: `1`
- queue_counts: `{'done': 1}`
- resource_monitor: `RESOURCE_MONITOR_DEGRADED`
- lock_present: `False`
- lock_expired: `False`

## Run Status
- `CODEX_RUNNER_BLOCKED`: `1`
- `CODEX_RUNNER_PLANNED`: `1`
- `CONTEXT_PACKED`: `1`
- `CONTEXT_SCOUTED`: `37`
- `DAEMON_COMPLETED`: `10`
- `DEPLOY_PLAN_READY`: `5`
- `DESIGN_ITERATING`: `11`
- `EXECUTOR_POOL_BLOCKED`: `1`
- `EXECUTOR_POOL_PLANNED`: `3`
- `GUIDANCE_GENERATED`: `12`
- `HISTORICAL_SCAFFOLD`: `2`
- `IMPLEMENTATION_BLOCKED`: `1`
- `LOCK_ACQUIRED`: `10`
- `METRICS_COLLECTED`: `10`
- `PLANNED`: `1`
- `QUEUE_SERVICE_SMOKE_PASSED`: `2`
- `RELEASE_BLOCKED`: `2`
- `RELEASE_FROZEN`: `10`
- `RESOURCE_MONITOR_DEGRADED`: `1`
- `RESOURCE_QUOTA_READY`: `2`
- `RUNTIME_PREFLIGHT_DEGRADED`: `1`
- `RUNTIME_PREFLIGHT_READY`: `4`
- `SANDBOX_EXECUTION_BLOCKED`: `2`
- `SANDBOX_EXECUTION_PLANNED`: `1`
- `SANDBOX_PLAN_BLOCKED`: `1`
- `SANDBOX_PLAN_READY`: `10`
- `SANDBOX_WORKER_EXECUTION_BLOCKED`: `1`
- `SANDBOX_WORKER_PLANNED`: `7`
- `SANDBOX_WORKER_QUEUE_GUARD_BLOCKED`: `1`
- `SCHEDULER_PLANNED`: `5`
- `SERVICE_COMPLETED`: `2`
- `SERVICE_HEALTHY`: `7`
- `SERVICE_ONCE_COMPLETED`: `4`
- `SERVICE_RECOVERED`: `1`
- `TASK_STATE_PLANNED`: `1`
- `WORKER_COMPLETED`: `8`

## Run Kinds
- `codex_runner`: `2`
- `context_pack`: `1`
- `context_scout`: `37`
- `daemon`: `10`
- `deploy_plan`: `5`
- `design_package`: `1`
- `executor_pool`: `4`
- `guidance`: `12`
- `historical_scaffold`: `2`
- `implementation_guard`: `1`
- `metrics`: `10`
- `queue_service`: `2`
- `release_freeze`: `12`
- `resource_monitor`: `1`
- `resource_quota`: `2`
- `runtime_preflight`: `5`
- `sandbox_execution`: `3`
- `sandbox_plan`: `11`
- `sandbox_worker`: `9`
- `scheduler`: `15`
- `service`: `6`
- `service_health`: `7`
- `service_recover`: `1`
- `task_state`: `1`
- `unknown`: `1`
- `worker`: `8`
- `workflow_run`: `10`

## Latest Runs
- `mvp-24-resource-monitor` `resource_monitor` `RESOURCE_MONITOR_DEGRADED`
- `mvp-24-release-with-resource-monitor` `release_freeze` `RELEASE_FROZEN`
- `mvp-24-executor-pool-resource-admission` `executor_pool` `EXECUTOR_POOL_PLANNED`
- `mvp-24-executor-pool-resource-admission-cle-p0-screen-001-sandbox-plan` `sandbox_worker` `SANDBOX_WORKER_PLANNED`
- `mvp-24-executor-pool-resource-admission-cle-p0-screen-001-sandbox-plan-cle-p0-screen-001-sandbox` `sandbox_plan` `SANDBOX_PLAN_READY`
- `mvp-24-preflight-resource-monitor` `runtime_preflight` `RUNTIME_PREFLIGHT_DEGRADED`
- `mvp-23-post-queue-service-auth-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-23-release-with-queue-service-auth` `release_freeze` `RELEASE_FROZEN`
- `mvp-23-queue-service-smoke-auth` `queue_service` `QUEUE_SERVICE_SMOKE_PASSED`
- `mvp-23-post-queue-service-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-23-release-with-queue-service` `release_freeze` `RELEASE_FROZEN`
- `mvp-23-deploy-queue-service` `deploy_plan` `DEPLOY_PLAN_READY`
- `mvp-23-preflight-queue-service` `runtime_preflight` `RUNTIME_PREFLIGHT_READY`
- `mvp-23-queue-service-smoke` `queue_service` `QUEUE_SERVICE_SMOKE_PASSED`
- `mvp-22-post-executor-pool-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-22-release-with-executor-pool` `release_freeze` `RELEASE_FROZEN`
- `mvp-22-executor-pool-queue-plan` `executor_pool` `EXECUTOR_POOL_PLANNED`
- `mvp-22-executor-pool-queue-plan-cle-p0-screen-001-sandbox-plan` `sandbox_worker` `SANDBOX_WORKER_PLANNED`
- `mvp-22-executor-pool-queue-plan-cle-p0-screen-001-sandbox-plan-cle-p0-screen-001-sandbox` `sandbox_plan` `SANDBOX_PLAN_READY`
- `mvp-22-executor-pool-parallel-guard` `executor_pool` `EXECUTOR_POOL_BLOCKED`

## Queue Items
- `CLE-P0-SCREEN-001` state `done` attempts `2/3`

## Guardrail
- Metrics are observational evidence only; they do not close tasks or mutate task YAML.
