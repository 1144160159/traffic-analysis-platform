# Codex Repair Prompt: CLE-P0-REVIEWER-001

Use `doc/02_acceptance/runs/mvp-36b-workflow-llm-reviewer-docsync/repair/repair-plan.json` as the repair source.

Repair objective:
- Move the task from `REPAIRING` toward verified evidence, not direct closure.

Allowed paths:
- `doc/02_acceptance/`
- `scripts/codex_loop/`

Required repair steps:
- `evidence` `PREP_EVIDENCE_CANNOT_CLOSE`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer.
- `evidence` `RUN_STATUS_NOT_CLOSABLE`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer.
- `verify` `LOCAL_REPORT_MISSING`: Run the declared local verification and preserve command output with exit codes.
- `review` `REVIEW_NOT_PASSED`: Complete the third-view reviewer decision and resolve any blocking findings.
- `evidence` `EVIDENCE_REPORT_MISSING`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer.
- `triage` `LLM_REVIEW_NOT_EXECUTED`: Triage `LLM_REVIEW_NOT_EXECUTED` and decide whether the task should repair, iterate design, or enter human gate.

After repairing, run the declared verification, complete reviewer output, and rerun `evidence_check.py`.
