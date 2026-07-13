# HA RTO/RPO Drill Bootstrap Runbook

Run ID: `20260630-ha-drill-evidence-bootstrap-r1`

This directory is a preparation package for GATE-P0-08. It is intentionally not a formal HA pass.

## Operator Flow

1. Run the non-destructive preflight and confirm the only blocker is missing destructive drill evidence.
2. Fill `operator-approval.review-template.yaml` with maintenance-window approval.
3. Execute phases from `tests/chaos/ha_drill_plan.yaml` in the approved window.
4. Record every event in `timeline.review-template.jsonl`.
5. Fill component reports under `reports/` and the `rto-rpo-table.review-template.csv`.
6. Fill `data-consistency-report.review-template.md` from immutable before/after snapshots.
7. Only after review, copy filled component reports to `doc/02_acceptance/06-resilience/` using the formal names listed in the plan, and write a real `ha-rto-rpo-latest.json`.

## Boundary

This bootstrap does not delete pods, scale workloads, force failover, or prove RTO/RPO. It only makes the evidence package ready for an approved drill.
