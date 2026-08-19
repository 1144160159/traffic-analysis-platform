# HA Drill Review Packet

- Run ID: `20260701-ha-drill-review-r1`
- Result: `pass`
- Input bootstrap: `doc/02_acceptance/06-resilience/bootstrap/latest`
- Target components: 5
- Review files: 7
- Formal artifacts in packet: 0
- Stable packet: `doc/02_acceptance/06-resilience/review/latest`

This package turns the HA RTO/RPO bootstrap templates into an operator review board for a future approved maintenance-window drill. It is not signed drill evidence and cannot close GATE-P0-08.

## Files

- `component-drill-review.csv`: component-by-component drill review worklist
- `rto-rpo-evidence-worklist.csv`: formal evidence and guard-marker worklist
- `maintenance-window-approval.template.md`: approval review template
- `formal-artifact-manifest.template.json`: formal artifact manifest template
- `data-consistency-review-checklist.md`: before/after consistency checklist
- `operator-review-checklist.md`: maintenance-window execution checklist
- `review-summary.json`: package metadata
