# Acceptance Cases: CLE-P0-REVIEWER-001

- run_id: `mvp-36-workflow-llm-reviewer`
- task: 开启第三视角 Reviewer Gate
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Mission / Acceptance`
- dependent_lanes: Product Design
- acceptance_type: `regression`

## Evidence Layer
- declared_acceptance_type: `regression`
- This design package is `acceptance-prep`; it cannot be reported as regression passed by itself.

## Close Conditions To Preserve
- review-report.md exists
- design-delta.md exists
- review decision cannot close P0/P1 blockers silently

## Local Verification Candidates
- `python scripts/codex_loop/review.py --task scripts/codex_loop/tasks/CLE-P0-REVIEWER-001.yaml --run-id ${run_id}`

## Live-readonly Verification Candidates
- `curl --noproxy '*' http://10.0.5.8:30180/login`
