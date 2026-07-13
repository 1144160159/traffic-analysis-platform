# Kafka Failover Review Template

Run ID: `20260630-ha-drill-evidence-bootstrap-r1`

Formal report target after the approved drill: `kafka-failover.md`

## Target

- Phase: `kafka-broker-loss`
- Target RTO seconds: `180`
- Target RPO: `zero_acknowledged_message_loss`

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
| all_partitions_have_leader | TBD | TBD |
| isr_recovers_to_replication_factor | TBD | TBD |
| live_topic_catalog_unchanged | TBD | TBD |
| consumer_offsets_continue | TBD | TBD |

## Reviewer Signoff

| Role | Name | Date | Decision | Note |
|---|---|---|---|---|
| Operator | TBD | TBD | TBD | TBD |
| SRE | TBD | TBD | TBD | TBD |
| User representative | TBD | TBD | TBD | TBD |
