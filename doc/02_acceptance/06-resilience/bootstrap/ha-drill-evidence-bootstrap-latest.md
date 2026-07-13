# HA Drill Evidence Bootstrap

- Run ID: `20260630-ha-drill-evidence-bootstrap-r1`
- Result: `pass`
- Bootstrap dir: `doc/02_acceptance/runs/20260630-ha-drill-evidence-bootstrap-r1/ha-rto-rpo-drill.bootstrap`
- Stable bootstrap dir: `doc/02_acceptance/06-resilience/bootstrap/latest`
- Summary: `doc/02_acceptance/runs/20260630-ha-drill-evidence-bootstrap-r1/ha-drill-evidence-bootstrap-20260630-ha-drill-evidence-bootstrap-r1-summary.json`

This package prepares the evidence structure for a future destructive RTO/RPO maintenance-window drill. It does not delete pods, scale workloads, trigger failover, restart storage, or write production traffic records.

## Boundary

Do not move the review-template files to `doc/02_acceptance/06-resilience/` root or rename them to formal report names until the approved drill has been executed and reviewed.

## Failed Checks

- [warn] Formal destructive drill evidence remains missing: bootstrap only; approved maintenance-window drill has not been executed
