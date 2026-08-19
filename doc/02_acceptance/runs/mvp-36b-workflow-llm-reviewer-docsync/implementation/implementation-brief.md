# Implementation Brief: CLE-P0-REVIEWER-001

- run_id: `mvp-36b-workflow-llm-reviewer-docsync`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- current_status: `DISCOVERED`
- execution_mode: `review`
- patch_valid: `True`

## Inputs
- context_pack: `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/context-pack/task-context-pack.md`
- design_dir: `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/design`
- plan: `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/plan.md`

## Allowed Paths
- `doc/02_acceptance/`
- `scripts/codex_loop/`

## Close Conditions
- review-report.md exists
- design-delta.md exists
- review decision cannot close P0/P1 blockers silently

## Local Verification
- `python scripts/codex_loop/review.py --task scripts/codex_loop/tasks/CLE-P0-REVIEWER-001.yaml --run-id ${run_id}`

## Implementation Rules
- Refresh `git status --short` before editing.
- Read the context pack and exact source refs before relying on summarized text.
- Keep changes inside workspace.allowed_paths unless a design delta expands scope.
- If touching Proto, Kafka, DB schema or APISIX routes, the task contract must declare it and consumers must be checked.
- Do not treat this brief, patch validation or context evidence as task closure.
- After patching, run the task's smallest relevant verification and third-view review.

## Patch Scope
- no patch supplied

## Blocking Findings
- none
