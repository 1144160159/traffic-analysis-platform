# Postgresql Failover Review Template

Run ID: `20260630-ha-drill-evidence-bootstrap-r1`

Formal report target after the approved drill: `postgres-failover.md`

## Target

- Phase: `postgresql-replica-loss`
- Target RTO seconds: `300`
- Target RPO: `no_committed_transaction_loss`

## Execution

| Item | Value |
|---|---|
| Approval ticket | TBD |
| Injection command | TBD |
| Injection start | TBD |
| Failure observed | TBD |
| Recovery observed | TBD |
| Observed RTO seconds | TBD |
| Observed RPO | TBD |
| Rollback action | TBD |

## Consistency Checks

| Check | Status | Evidence |
|---|---|---|
| primary_or_promoted_primary_accepts_connections | TBD | TBD |
| replicas_resume_streaming | TBD | TBD |
| replay_lag_returns_to_zero_or_site_threshold | TBD | TBD |
| control_plane_crud_smoke_passes | TBD | TBD |

## Reviewer Signoff

| Role | Name | Date | Decision | Note |
|---|---|---|---|---|
| Operator | TBD | TBD | TBD | TBD |
| SRE | TBD | TBD | TBD | TBD |
| User representative | TBD | TBD | TBD | TBD |
