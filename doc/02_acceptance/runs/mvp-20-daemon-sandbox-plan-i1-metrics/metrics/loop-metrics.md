# Codex Loop Runtime Metrics

- generated_at: `2026-06-24T02:36:29`
- run_total: `126`
- queue_total: `1`
- queue_counts: `{'done': 1}`
- lock_present: `False`
- lock_expired: `False`

## Run Status
- `CODEX_RUNNER_BLOCKED`: `1`
- `CODEX_RUNNER_PLANNED`: `1`
- `CONTEXT_PACKED`: `1`
- `CONTEXT_SCOUTED`: `29`
- `DAEMON_COMPLETED`: `7`
- `DEPLOY_PLAN_READY`: `3`
- `DESIGN_ITERATING`: `11`
- `GUIDANCE_GENERATED`: `10`
- `HISTORICAL_SCAFFOLD`: `2`
- `IMPLEMENTATION_BLOCKED`: `1`
- `LOCK_ACQUIRED`: `8`
- `METRICS_COLLECTED`: `7`
- `PLANNED`: `1`
- `RELEASE_BLOCKED`: `2`
- `RELEASE_FROZEN`: `4`
- `RUNTIME_PREFLIGHT_READY`: `1`
- `SANDBOX_EXECUTION_BLOCKED`: `2`
- `SANDBOX_EXECUTION_PLANNED`: `1`
- `SANDBOX_PLAN_BLOCKED`: `1`
- `SANDBOX_PLAN_READY`: `5`
- `SANDBOX_WORKER_EXECUTION_BLOCKED`: `1`
- `SANDBOX_WORKER_PLANNED`: `2`
- `SANDBOX_WORKER_QUEUE_GUARD_BLOCKED`: `1`
- `SCHEDULER_PLANNED`: `2`
- `SERVICE_COMPLETED`: `2`
- `SERVICE_HEALTHY`: `7`
- `SERVICE_ONCE_COMPLETED`: `3`
- `SERVICE_RECOVERED`: `1`
- `TASK_STATE_PLANNED`: `1`
- `WORKER_COMPLETED`: `8`

## Run Kinds
- `codex_runner`: `2`
- `context_pack`: `1`
- `context_scout`: `29`
- `daemon`: `7`
- `deploy_plan`: `3`
- `design_package`: `1`
- `guidance`: `10`
- `historical_scaffold`: `2`
- `implementation_guard`: `1`
- `metrics`: `7`
- `release_freeze`: `6`
- `runtime_preflight`: `1`
- `sandbox_execution`: `3`
- `sandbox_plan`: `6`
- `sandbox_worker`: `4`
- `scheduler`: `10`
- `service`: `5`
- `service_health`: `7`
- `service_recover`: `1`
- `task_state`: `1`
- `unknown`: `1`
- `worker`: `8`
- `workflow_run`: `10`

## Latest Runs
- `mvp-20-daemon-sandbox-plan-i1-worker` `sandbox_worker` `SANDBOX_WORKER_PLANNED`
- `mvp-20-daemon-sandbox-plan-i1-worker-cle-p0-screen-001-sandbox` `sandbox_plan` `SANDBOX_PLAN_READY`
- `mvp-20-daemon-sandbox-plan-i1` `guidance` `GUIDANCE_GENERATED`
- `mvp-20-daemon-sandbox-plan-i1-scheduler` `scheduler` `LOCK_ACQUIRED`
- `mvp-20-daemon-sandbox-plan-i1-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-20-sandbox-worker-queue-guard` `sandbox_worker` `SANDBOX_WORKER_QUEUE_GUARD_BLOCKED`
- `mvp-20-sandbox-worker-exec-blocked` `sandbox_worker` `SANDBOX_WORKER_EXECUTION_BLOCKED`
- `mvp-20-sandbox-worker-exec-blocked-cle-p0-screen-001-sandbox` `sandbox_plan` `SANDBOX_PLAN_READY`
- `mvp-20-sandbox-worker-exec-blocked-cle-p0-screen-001-sandbox-exec` `sandbox_execution` `SANDBOX_EXECUTION_BLOCKED`
- `mvp-20-sandbox-worker-plan` `sandbox_worker` `SANDBOX_WORKER_PLANNED`
- `mvp-20-sandbox-worker-plan-cle-p0-screen-001-sandbox` `sandbox_plan` `SANDBOX_PLAN_READY`
- `mvp-19-post-sandbox-executor-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-19-release-blocks-planned-execution` `release_freeze` `RELEASE_BLOCKED`
- `mvp-19-sandbox-execution-blocked` `sandbox_execution` `SANDBOX_EXECUTION_BLOCKED`
- `mvp-19-sandbox-execution-plan` `sandbox_execution` `SANDBOX_EXECUTION_PLANNED`
- `mvp-18-post-sandbox-scout` `context_scout` `CONTEXT_SCOUTED`
- `mvp-18-release-blocks-bad-sandbox` `release_freeze` `RELEASE_BLOCKED`
- `mvp-18-release-with-sandbox` `release_freeze` `RELEASE_FROZEN`
- `mvp-18-sandbox-execute-local-blocked` `sandbox_plan` `SANDBOX_PLAN_BLOCKED`
- `mvp-18-sandbox-k8s-plan` `sandbox_plan` `SANDBOX_PLAN_READY`

## Queue Items
- `CLE-P0-SCREEN-001` state `done` attempts `2/3`

## Guardrail
- Metrics are observational evidence only; they do not close tasks or mutate task YAML.
