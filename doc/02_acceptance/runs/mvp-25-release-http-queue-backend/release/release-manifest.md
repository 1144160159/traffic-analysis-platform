# Codex Loop Release Manifest

- run_id: `mvp-25-release-http-queue-backend`
- status: `RELEASE_FROZEN`
- commit: `e3316aec4ac1d6592e28aefc86853128ecde7408`
- health: `HEALTHY`
- queue_counts: `{'done': 1}`
- deploy_plan: `doc/02_acceptance/runs/mvp-25-deploy-http-queue-worker/deploy/deploy-plan.json`
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
- executor_pool: `doc/02_acceptance/runs/mvp-25-executor-pool-http-queue/executor-pool/executor-pool-summary.json`
- executor_pool_status: `EXECUTOR_POOL_PLANNED`
- queue_service: `doc/02_acceptance/runs/mvp-25-http-queue-backend-smoke-2/queue-service/queue-service-summary.json`
- queue_service_status: `QUEUE_SERVICE_SMOKE_PASSED`

## Evidence
- `release/release-manifest.json`
- `release/release-manifest.md`
- `release/rollback-plan.md`
- `release/git-status.txt`
- `release/loop-diff.patch`

## Guardrail
- This manifest freezes loop-engine evidence only; it is not a business acceptance pass.
