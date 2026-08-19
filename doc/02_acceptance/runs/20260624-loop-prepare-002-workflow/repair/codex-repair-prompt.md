# Codex Repair Prompt: CLE-P0-P95-001

Use `doc/02_acceptance/runs/20260624-loop-prepare-002-workflow/repair/repair-plan.json` as the repair source.

Repair objective:
- Move the task from `REPAIRING` toward verified evidence, not direct closure.

Allowed paths:
- `go/control-plane/`
- `java/flink-jobs/`
- `web/ui/src/`
- `proto/traffic/v1/`
- `doc/02_acceptance/`
- `doc/01_design/`

Required repair steps:
- `evidence` `PREP_EVIDENCE_CANNOT_CLOSE`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer.
- `evidence` `RUN_STATUS_NOT_CLOSABLE`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer.
- `verify` `LOCAL_REPORT_MISSING`: Run the declared local verification and preserve command output with exit codes.
- `review` `REVIEW_NOT_PASSED`: Complete the third-view reviewer decision and resolve any blocking findings.
- `evidence` `EVIDENCE_REPORT_MISSING`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer.
- `triage` `LLM_REVIEW_NOT_EXECUTED`: Triage `LLM_REVIEW_NOT_EXECUTED` and decide whether the task should repair, iterate design, or enter human gate.

After repairing, run the declared verification, complete reviewer output, and rerun `evidence_check.py`.
