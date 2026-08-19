# Clickhouse Failover Review Template

Run ID: `20260630-ha-drill-evidence-bootstrap-r1`

Formal report target after the approved drill: `clickhouse-failover.md`

## Target

- Phase: `clickhouse-replica-loss`
- Target RTO seconds: `300`
- Target RPO: `no_committed_row_loss`

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
| system_replicas_not_readonly | TBD | TBD |
| absolute_delay_returns_below_60s | TBD | TBD |
| queue_size_returns_below_100 | TBD | TBD |
| drill_query_counts_match_before_after | TBD | TBD |

## Reviewer Signoff

| Role | Name | Date | Decision | Note |
|---|---|---|---|---|
| Operator | TBD | TBD | TBD | TBD |
| SRE | TBD | TBD | TBD | TBD |
| User representative | TBD | TBD | TBD | TBD |
