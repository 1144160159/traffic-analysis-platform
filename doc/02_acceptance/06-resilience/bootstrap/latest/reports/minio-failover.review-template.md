# Minio Failover Review Template

Run ID: `20260630-ha-drill-evidence-bootstrap-r1`

Formal report target after the approved drill: `minio-failover.md`

## Target

- Phase: `minio-pod-loss`
- Target RTO seconds: `300`
- Target RPO: `no_object_loss_for_completed_writes`

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
| health_endpoint_recovers | TBD | TBD |
| pcap_object_head_or_verify_passes | TBD | TBD |
| presigned_download_still_matches_sha256 | TBD | TBD |
| lifecycle_or_retention_config_unchanged | TBD | TBD |

## Reviewer Signoff

| Role | Name | Date | Decision | Note |
|---|---|---|---|---|
| Operator | TBD | TBD | TBD | TBD |
| SRE | TBD | TBD | TBD | TBD |
| User representative | TBD | TBD | TBD | TBD |
