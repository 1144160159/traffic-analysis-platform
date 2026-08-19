# Codex Loop Runtime Metrics

- generated_at: `2026-06-24T02:44:49`
- run_total: `151`
- queue_total: `1`
- queue_counts: `{'done': 1}`
- lock_present: `False`
- lock_expired: `False`

## Run Status
- `CODEX_RUNNER_BLOCKED`: `1`
- `CODEX_RUNNER_PLANNED`: `1`
- `CONTEXT_PACKED`: `1`
- `CONTEXT_SCOUTED`: `33`
- `DAEMON_COMPLETED`: `9`
- `DEPLOY_PLAN_READY`: `3`
- `DESIGN_ITERATING`: `11`
- `GUIDANCE_GENERATED`: `12`
- `HISTORICAL_SCAFFOLD`: `2`
- `IMPLEMENTATION_BLOCKED`: `1`
- `LOCK_ACQUIRED`: `10`
- `METRICS_COLLECTED`: `9`
- `PLANNED`: `1`
- `RELEASE_BLOCKED`: `2`
- `RELEASE_FROZEN`: `6`
- `RESOURCE_QUOTA_READY`: `2`
- `RUNTIME_PREFLIGHT_READY`: `2`
- `SANDBOX_EXECUTION_BLOCKED`: `2`
- `SANDBOX_EXECUTION_PLANNED`: `1`
- `SANDBOX_PLAN_BLOCKED`: `1`
- `SANDBOX_PLAN_READY`: `7`
- `SANDBOX_WORKER_EXECUTION_BLOCKED`: `1`
- `SANDBOX_WORKER_PLANNED`: `4`
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
- `context_scout`: `33`
- `daemon`: `9`
- `deploy_plan`: `3`
- `design_package`: `1`
- `guidance`: `12`
- `historical_scaffold`: `2`
- `implementation_guard`: `1`
- `metrics`: `9`
- `release_freeze`: `8`
- `resource_quota`: `2`
- `runtime_preflight`: `2`
- `sandbox_execution`: `3`
- `sandbox_plan`: `8`
- `sandbox_worker`: `6`
- `scheduler`: `15`
- `service`: `6`
- `service_health`: `7`
- `service_recover`: `1`
- `task_state`: `1`
- `unknown`: `1`
- `worker`: `8`
- `workflow_run`: `10`

## Latest Runs
- `mvp-21-daemon-quota-sandbox-plan-i1-scheduler` `scheduler` `LOCK_ACQUIRED`
- `mvp-21-daemon-quota-sandbox-plan-i1-worker` `sandbox_worker` `SANDBOX_WORKER_PLANNED`
- `mvp-21-daemon-quota-sandbox-plan-i1-worker-cle-p0-screen-001-sandbox` `sandbox_plan` `SANDBOX_PLAN_READY`
- `mvp-21-daemon-quota-sandbox-plan-i1` `guidance` `GUIDANCE_GENERATED`
- `mvp-21-daemon-quota-sandbox-plan-i1-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-21-post-resource-quota-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-21-release-with-resource-quota` `release_freeze` `RELEASE_FROZEN`
- `mvp-21-preflight-quota-path` `runtime_preflight` `RUNTIME_PREFLIGHT_READY`
- `mvp-21-resource-quota-scheduler-audit` `resource_quota` `RESOURCE_QUOTA_READY`
- `mvp-21-quota-scheduler-json-persist` `scheduler` `SCHEDULER_PLANNED`
- `mvp-21-quota-scheduler-sqlite-persist` `scheduler` `SCHEDULER_PLANNED`
- `mvp-21-quota-scheduler` `scheduler` `SCHEDULER_PLANNED`
- `mvp-21-resource-quota-guidance` `resource_quota` `RESOURCE_QUOTA_READY`
- `mvp-20-post-sandbox-worker-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-20-release-with-sandbox-worker` `release_freeze` `RELEASE_FROZEN`
- `mvp-20-service-sandbox-plan` `service` `SERVICE_ONCE_COMPLETED`
- `mvp-20-service-sandbox-plan-daemon` `daemon` `DAEMON_COMPLETED`
- `mvp-20-service-sandbox-plan-daemon-i1` `guidance` `GUIDANCE_GENERATED`
- `mvp-20-service-sandbox-plan-daemon-i1-metrics` `metrics` `METRICS_COLLECTED`
- `mvp-20-service-sandbox-plan-daemon-i1-scheduler` `scheduler` `LOCK_ACQUIRED`

## Queue Items
- `CLE-P0-SCREEN-001` state `done` attempts `2/3`

## Guardrail
- Metrics are observational evidence only; they do not close tasks or mutate task YAML.
