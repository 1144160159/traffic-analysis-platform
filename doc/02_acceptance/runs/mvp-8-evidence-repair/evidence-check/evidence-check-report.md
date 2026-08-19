# Evidence Check: CLE-P0-SCREEN-001

- checked_run: `doc/02_acceptance/runs/mvp-8-evidence-repair`
- status: `EVIDENCE_REJECTED`
- recommended_next_state: `DESIGN_ITERATING`
- expected_evidence_type: `regression`
- actual_evidence_type: `acceptance-prep`
- summary_status: `DESIGN_ITERATING`
- blockers: `7`
- warnings: `0`

## Findings
- `blocker` `PREP_EVIDENCE_CANNOT_CLOSE` `run-summary.json`: Evidence type `acceptance-prep` is preparation evidence and cannot close `regression`. Suggestion: Run the concrete verification gate and publish the matching evidence layer.
- `blocker` `RUN_STATUS_NOT_CLOSABLE` `run-summary.json`: Run status `DESIGN_ITERATING` is not eligible for task closure. Suggestion: Resolve the blocked/prep/failure status and regenerate evidence.
- `blocker` `GUIDANCE_BLOCKER` `guidance/guidance.json`: SCREEN_AUTH_BOUNDARY: /screen is outside ProtectedLayout. Suggestion: Resolve the /screen public/protected/readonly strategy before claiming UI auth boundary closure.
- `blocker` `PATCH_SCOPE_INVALID` `implementation/patch-validation.json`: Implementation patch validation is false. Suggestion: Fix scope, contract declaration, or guidance blockers before applying or closing the task.
- `blocker` `LOCAL_REPORT_MISSING` `local-report.md`: Task declares local verification but `local-report.md` is missing. Suggestion: Run the smallest relevant local verification through `run_task.py --execute-local` or attach an equivalent report.
- `blocker` `REVIEW_NOT_PASSED` `review-report.md`: Reviewer decision is `pending`. Suggestion: Resolve reviewer findings before using this evidence to close the task.
- `blocker` `EVIDENCE_REPORT_MISSING` `evidence-report.md`: `evidence-report.md` is missing, so close_when cannot be proven. Suggestion: Write an evidence report that maps each close_when item to concrete files, commands, screenshots, SQL, or logs.

## Rule
- This checker is conservative: prep evidence, pending review, missing local reports, and unproven close_when items cannot close a task.
- The checker does not modify task status. Use `task_state.py --apply` only after reviewing this result.
