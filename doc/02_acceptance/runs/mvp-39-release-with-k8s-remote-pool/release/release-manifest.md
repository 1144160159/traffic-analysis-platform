# Codex Loop Release Manifest

- run_id: `mvp-39-release-with-k8s-remote-pool`
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
- remote_pool_stress: `doc/02_acceptance/runs/mvp-38-remote-pool-external-service/remote-pool-stress/stress-summary.json`
- remote_pool_stress_status: `REMOTE_POOL_STRESS_COMPLETED`
- remote_pool_stress_mode: `external`
- remote_pool_stress_workers: `3`
- remote_pool_k8s_stress: `doc/02_acceptance/runs/mvp-39-remote-pool-k8s-plan/remote-pool-k8s-stress/stress-summary.json`
- remote_pool_k8s_stress_status: `REMOTE_POOL_K8S_STRESS_VALIDATED`
- remote_pool_k8s_stress_workers: `3`
- soak: `doc/02_acceptance/runs/mvp-34-soak-service-once/soak/soak-summary.json`
- soak_status: `SOAK_DEGRADED`
- soak_cycles_completed: `1`
- model_profile: `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/model-profile/model-profile.json`
- model_profile_status: `MODEL_PROFILE_SELECTED`
- model_profile_selected: `gpt5_high_risk_patch`
- llm_review: `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/llm-review/llm-review-summary.json`
- llm_review_status: `LLM_REVIEW_PLANNED`
- llm_review_decision: `None`
- queue_service: `doc/02_acceptance/runs/mvp-37-queue-service-smoke-after-lease-owner/queue-service/queue-service-summary.json`
- queue_service_status: `QUEUE_SERVICE_SMOKE_PASSED`

## Evidence
- `release/release-manifest.json`
- `release/release-manifest.md`
- `release/rollback-plan.md`
- `release/git-status.txt`
- `release/loop-diff.patch`

## Guardrail
- This manifest freezes loop-engine evidence only; it is not a business acceptance pass.
