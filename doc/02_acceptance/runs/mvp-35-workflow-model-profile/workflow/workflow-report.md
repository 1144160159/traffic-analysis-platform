# Codex Loop Workflow Report

- run_id: `mvp-35-workflow-model-profile`
- selected_task: `CLE-P0-REVIEWER-001` 开启第三视角 Reviewer Gate
- selected_from: `explicit_task`
- stage: `prepare`
- status: `WORKFLOW_PREPARED`
- evidence_type: `acceptance-prep`
- blocker_count: `0`

## Stage Flow
- `scout.py` exit `0`
- `guide.py` exit `0`
- `design.py` exit `0`
- `context_pack.py` exit `0`
- `plan.py` exit `0`
- `implement.py` exit `0`
- `patch_runner.py` exit `0`
- `model_profile.py` exit `0`
- `codex_adapter.py` exit `0`
- `codex_runner.py` exit `0`
- `review.py` exit `0`
- `semantic_reviewer.py` exit `0`
- `run_task.py` skipped: stage prepare does not run task execution
- `collect_evidence.py` exit `0`
- `evidence_check.py` exit `0`
- `repair.py` exit `0`
- `auto_repair_loop.py` exit `0`

## Blockers
- none

## Outputs
- `task.yaml`
- `context/context.snapshot.json`
- `guidance/guidance.json`
- `design/design-summary.json`
- `context-pack/task-context-pack.md`
- `implementation/implementation-brief.md`
- `implementation/patch-scope.json`
- `implementation/patch-validation.json`
- `patch-runner/patch-request.md`
- `patch-runner/patch-request.json`
- `patch-runner/codex-output-contract.json`
- `patch-runner/codex-output-schema.json`
- `patch-runner/patch-intake.json`
- `patch-runner/patch-runner-summary.json`
- `model-profile/model-profile.json`
- `model-profile/model-profile.md`
- `model-profile/command-template.txt`
- `codex-adapter/invocation-plan.md`
- `codex-adapter/invocation.json`
- `codex-runner/invocation.json`
- `codex-runner/codex-runner-report.md`
- `plan.md`
- `review-report.md`
- `review/review-summary.json`
- `semantic-review/semantic-review.json`
- `design-delta.md`
- `git-status.txt`
- `changed-files.txt`
- `workflow/workflow-summary.json`
- `workflow/workflow-report.md`
- `patch-runner/patch-request.md`
- `patch-runner/patch-request.json`
- `patch-runner/codex-output-contract.json`
- `patch-runner/codex-output-schema.json`
- `patch-runner/patch-intake.json`
- `patch-runner/patch-runner-summary.json`
- `patch-runner/patch-runner-report.md`
- `model-profile/model-profile.json`
- `model-profile/model-profile.md`
- `model-profile/command-template.txt`
- `review/review-summary.json`
- `codex-adapter/invocation-plan.md`
- `codex-adapter/invocation.json`
- `codex-adapter/stdout.txt`
- `codex-adapter/stderr.txt`
- `codex-runner/invocation.json`
- `codex-runner/codex-runner-report.md`
- `codex-runner/stdout.txt`
- `codex-runner/stderr.txt`
- `semantic-review/semantic-review.json`
- `semantic-review/semantic-review-report.md`
- `evidence-check/evidence-check.json`
- `evidence-check/evidence-check-report.md`
- `repair/repair-plan.json`
- `repair/repair-report.md`
- `repair/codex-repair-prompt.md`
- `auto-repair/auto-repair-summary.json`
- `auto-repair/auto-repair-report.md`

## Guardrail
- This workflow report does not close a task by itself.
- `execute-local` is required before local verification commands are run.
- Live write and destructive actions remain out of scope for this workflow MVP.
