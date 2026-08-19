# Evidence Check: CLE-P0-SCREEN-001

- checked_run: `doc/02_acceptance/runs/mvp-47-cle-p0-screen-001-fix`
- status: `EVIDENCE_REJECTED`
- recommended_next_state: `DESIGN_ITERATING`
- expected_evidence_type: `regression`
- actual_evidence_type: `regression`
- summary_status: `LOCAL_VERIFIED`
- blockers: `2`
- warnings: `1`

## Findings
- `blocker` `GUIDANCE_BLOCKER` `guidance/guidance.json`: PREREQUISITE_OPEN: Task cannot be executed or closed before prerequisites close: CLE-P0-ROUTE-001. Suggestion: Close the prerequisite tasks first, then rerun guidance and evidence check.
- `blocker` `PREREQUISITE_OPEN` `scripts/codex_loop/tasks/CLE-P0-ROUTE-001.yaml`: Prerequisite task `CLE-P0-ROUTE-001` is `DISCOVERED`, not `CLOSED`. Suggestion: Close the prerequisite task first, then rerun evidence_check.py.
- `warning` `LIVE_REPORT_MISSING` `live-report.md`: Task declares live_readonly verification but `live-report.md` is missing. Suggestion: Attach live readonly evidence before claiming live-backed regression or acceptance.

## Rule
- This checker is conservative: prep evidence, pending review, open prerequisites, missing local reports, and unproven close_when items cannot close a task.
- The checker does not modify task status. Use `task_state.py --apply` only after reviewing this result.
