# Flink Failover Review Template

Run ID: `20260630-ha-drill-evidence-bootstrap-r1`

Formal report target after the approved drill: `flink-failover.md`

## Target

- Phase: `flink-taskmanager-loss`
- Target RTO seconds: `300`
- Target RPO: `exactly_once_checkpoint_restore`

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
| all_expected_jobs_return_running | TBD | TBD |
| latest_completed_checkpoint_advances_after_recovery | TBD | TBD |
| no_root_exceptions | TBD | TBD |
| output_tables_do_not_duplicate_drill_records | TBD | TBD |

## Reviewer Signoff

| Role | Name | Date | Decision | Note |
|---|---|---|---|---|
| Operator | TBD | TBD | TBD | TBD |
| SRE | TBD | TBD | TBD | TBD |
| User representative | TBD | TBD | TBD | TBD |
