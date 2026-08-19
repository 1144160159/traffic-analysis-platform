# Codex Loop Release Manifest

- run_id: `mvp-19-release-blocks-planned-execution`
- status: `RELEASE_BLOCKED`
- commit: `e3316aec4ac1d6592e28aefc86853128ecde7408`
- health: `HEALTHY`
- queue_counts: `{'done': 1}`
- deploy_plan: `None`
- sandbox_plan: `doc/02_acceptance/runs/mvp-18-sandbox-k8s-plan/sandbox/sandbox-plan.json`
- sandbox_status: `SANDBOX_PLAN_READY`
- sandbox_execution: `doc/02_acceptance/runs/mvp-19-sandbox-execution-plan/sandbox-executor/execution.json`
- sandbox_execution_status: `SANDBOX_EXECUTION_PLANNED`

## Evidence
- `release/release-manifest.json`
- `release/release-manifest.md`
- `release/rollback-plan.md`
- `release/git-status.txt`
- `release/loop-diff.patch`

## Guardrail
- This manifest freezes loop-engine evidence only; it is not a business acceptance pass.
