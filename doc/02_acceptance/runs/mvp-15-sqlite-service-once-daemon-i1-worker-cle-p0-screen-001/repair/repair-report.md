# Repair Plan: CLE-P0-SCREEN-001

- checked_run: `doc/02_acceptance/runs/mvp-15-sqlite-service-once-daemon-i1-worker-cle-p0-screen-001`
- source_check_status: `EVIDENCE_REJECTED`
- recommended_next_status: `DESIGN_ITERATING`
- blockers: `8`
- warnings: `0`

## Ordered Repair Steps
1. `evidence` `PREP_EVIDENCE_CANNOT_CLOSE` `run-summary.json`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer. Source: Evidence type `acceptance-prep` is preparation evidence and cannot close `regression`.
2. `evidence` `RUN_STATUS_NOT_CLOSABLE` `run-summary.json`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer. Source: Run status `DESIGN_ITERATING` is not eligible for task closure.
3. `design` `GUIDANCE_BLOCKER` `guidance/guidance.json`: Regenerate or update the design package, then rerun guidance before implementation. Source: SCREEN_AUTH_BOUNDARY: /screen is outside ProtectedLayout.
4. `implement` `PATCH_SCOPE_INVALID` `implementation/patch-validation.json`: Repair patch scope or contract declarations, then rerun implement.py with the corrected patch. Source: Implementation patch validation is false.
5. `implement` `PATCH_RUNNER_BLOCKER` `patch-runner/patch-intake.json`: Repair patch scope or contract declarations, then rerun implement.py with the corrected patch. Source: GUIDANCE_BLOCKER: SCREEN_AUTH_BOUNDARY: /screen is outside ProtectedLayout.
6. `verify` `LOCAL_REPORT_MISSING` `local-report.md`: Run the declared local verification and preserve command output with exit codes. Source: Task declares local verification but `local-report.md` is missing.
7. `review` `REVIEW_NOT_PASSED` `review-report.md`: Complete the third-view reviewer decision and resolve any blocking findings. Source: Reviewer decision is `design_update_required`.
8. `evidence` `EVIDENCE_REPORT_MISSING` `evidence-report.md`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer. Source: `evidence-report.md` is missing, so close_when cannot be proven.

## Guardrail
- Do not skip back to CLOSED from this plan.
- After repair, rerun implementation/verification/review as appropriate, then rerun evidence_check.py.
