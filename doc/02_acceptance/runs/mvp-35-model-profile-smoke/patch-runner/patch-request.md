# Codex Patch Request: CLE-P0-REVIEWER-001

- run_id: `mvp-35-model-profile-smoke`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- acceptance_type: `regression`

## Inputs
- guidance: `doc/02_acceptance/runs/mvp-35-model-profile-smoke/guidance/guidance.json`

## Allowed Paths
- `doc/02_acceptance/`
- `scripts/codex_loop/`

## Close Conditions
- review-report.md exists
- design-delta.md exists
- review decision cannot close P0/P1 blockers silently

## Verification
- `python scripts/codex_loop/review.py --task scripts/codex_loop/tasks/CLE-P0-REVIEWER-001.yaml --run-id ${run_id}`

## Required Codex Output
- Provide a unified diff patch file.
- Provide `codex-output.json` matching `patch-runner/codex-output-contract.json`.
- Do not edit outside Allowed Paths.
- Do not mark the task closed; evidence_check.py decides closure eligibility.
