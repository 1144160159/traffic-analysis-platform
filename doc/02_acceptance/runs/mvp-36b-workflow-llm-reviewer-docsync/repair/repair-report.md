# Repair Plan: CLE-P0-REVIEWER-001

- checked_run: `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync`
- source_check_status: `EVIDENCE_REJECTED`
- recommended_next_status: `REPAIRING`
- blockers: `5`
- warnings: `1`

## Ordered Repair Steps
1. `evidence` `PREP_EVIDENCE_CANNOT_CLOSE` `run-summary.json`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer. Source: Evidence type `acceptance-prep` is preparation evidence and cannot close `regression`.
2. `evidence` `RUN_STATUS_NOT_CLOSABLE` `run-summary.json`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer. Source: Run status `WORKFLOW_PREPARED` is not eligible for task closure.
3. `verify` `LOCAL_REPORT_MISSING` `local-report.md`: Run the declared local verification and preserve command output with exit codes. Source: Task declares local verification but `local-report.md` is missing.
4. `review` `REVIEW_NOT_PASSED` `review-report.md`: Complete the third-view reviewer decision and resolve any blocking findings. Source: Reviewer decision is `pending`.
5. `evidence` `EVIDENCE_REPORT_MISSING` `evidence-report.md`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer. Source: `evidence-report.md` is missing, so close_when cannot be proven.
6. `triage` `LLM_REVIEW_NOT_EXECUTED` `llm-review/llm-review-summary.json`: Triage `LLM_REVIEW_NOT_EXECUTED` and decide whether the task should repair, iterate design, or enter human gate. Source: LLM reviewer is planned but no model output was ingested.

## Guardrail
- Do not skip back to CLOSED from this plan.
- After repair, rerun implementation/verification/review as appropriate, then rerun evidence_check.py.
