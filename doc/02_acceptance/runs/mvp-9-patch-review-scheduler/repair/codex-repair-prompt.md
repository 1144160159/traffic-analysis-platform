# Codex Repair Prompt: CLE-P0-SCREEN-001

Use `doc/02_acceptance/runs/mvp-9-patch-review-scheduler/repair/repair-plan.json` as the repair source.

Repair objective:
- Move the task from `DESIGN_ITERATING` toward verified evidence, not direct closure.

Allowed paths:
- `web/ui/src/`
- `web/ui/e2e/`
- `go/control-plane/internal/auth/`
- `doc/02_acceptance/`

Required repair steps:
- `evidence` `PREP_EVIDENCE_CANNOT_CLOSE`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer.
- `evidence` `RUN_STATUS_NOT_CLOSABLE`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer.
- `design` `GUIDANCE_BLOCKER`: Regenerate or update the design package, then rerun guidance before implementation.
- `implement` `PATCH_SCOPE_INVALID`: Repair patch scope or contract declarations, then rerun implement.py with the corrected patch.
- `implement` `PATCH_RUNNER_BLOCKER`: Repair patch scope or contract declarations, then rerun implement.py with the corrected patch.
- `verify` `LOCAL_REPORT_MISSING`: Run the declared local verification and preserve command output with exit codes.
- `review` `REVIEW_NOT_PASSED`: Complete the third-view reviewer decision and resolve any blocking findings.
- `evidence` `EVIDENCE_REPORT_MISSING`: Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer.

After repairing, run the declared verification, complete reviewer output, and rerun `evidence_check.py`.
