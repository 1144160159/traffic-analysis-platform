# F-CAMPAIGN-001 rollback

## Scope

This runbook covers the additive `campaign_aggregate_v2` command path, its leased MinIO report executor, its independently approved leased SOAR provider adapter, its dual Kafka event pipeline, and the independently acknowledged ClickHouse, OpenSearch and NebulaGraph projection worker. It does not authorize deletion of accepted command, approval, control, provider receipt, history, report, audit, outbox, inbox, object manifest, MinIO artifact, or external projection rows.

## Stop conditions

- campaign member counts diverge between PostgreSQL authority and the candidate read model;
- cross-tenant data is observed;
- aggregate revisions skip, regress, or accept stale commands;
- an accepted report or SOAR request is displayed as final success without its object or executor receipt;
- a requester can approve its own SOAR execution or compensation, a provider HTTP status is treated as an effect without a durable receipt, or a receipt identity/hash collides;
- a SOAR lease expires while the provider effect is ambiguous, compensation is accepted without a receipt proving the original external effect, or workflow/action/aggregate revisions diverge;
- a completed report has no PostgreSQL object manifest, or the MinIO size/SHA-256 differs from that manifest;
- report attempts, lease loss, or object-store failures produce a completed action job, or the report queue exceeds its approved age/attempt budget;
- command error rate, latency, PostgreSQL locks, or outbox backlog exceed the approved canary budget.
- any target reports an identity collision, a dead target state, or an expanding gap against the PostgreSQL target watermark;
- ClickHouse event/hash counts, OpenSearch current-state revision, NebulaGraph entity/edge revision, and PostgreSQL source revision no longer reconcile;
- an OpenSearch alias points to the wrong candidate index, a ClickHouse table is missing or read-only, or the NebulaGraph space/tag/edge schema is incompatible;
- target projection readiness remains unavailable beyond the approved dependency budget.

## Rollback procedure

1. Stop canary expansion. For a SOAR provider fault, clear `CAMPAIGN_SOAR_EXECUTOR_URL` first and roll the alert-service pods so no new SOAR lease is claimed; approved rows remain `approved_awaiting_executor` or `compensation_queued` and must not be relabelled terminal. For a target projection fault, set `CAMPAIGN_TARGET_PROJECTION_V2_ENABLED=false` first so no new projection effects are attempted. Set `CAMPAIGN_EVENT_PIPELINE_V2_ENABLED=false` next when the trigger is delivery, consumer, DLQ, or a growing inbox backlog. Set `CAMPAIGN_AGGREGATE_V2_ENABLED=false` when command/report/SOAR acceptance is unsafe. These controls preserve committed report, SOAR, action, history, audit, outbox, and inbox rows.
2. Allow in-flight PostgreSQL transactions to finish; do not delete their jobs, history, audit, or outbox rows.
3. Reconcile `campaign_action_jobs`, `campaign_soar_jobs`, `campaign_soar_approvals`, `campaign_soar_control_requests`, `campaign_soar_execution_receipts`, `campaign_membership_commands`, `campaign_membership_backfill_runs`, `campaign_membership_backfill_campaigns`, `campaign_membership_backfill_items`, `campaign_merge_receipts`, `campaign_merge_items`, `campaign_alert_links`, both membership and aggregate history/outbox pairs, `campaign_event_projection_inbox`, delivery positions, source watermarks, `campaign_target_projection_watermarks`, `campaign_workbench_state`, `campaign_reports`, and `audit_logs` by tenant, campaign, relation, stream, command, event, revision, topic, partition, offset, target and projection hash. Treat `(stream,event_id)` as the consumer identity because a membership transaction intentionally shares one event ID across two streams.
4. Roll back the service and Web UI to the previous content-addressed candidate if disabling the flag is insufficient.
5. Keep the additive Schema in place. The legacy status/owner/link paths remain compatible; unlink, report, and SOAR commands must remain explicitly unavailable rather than falling back to direct row edits or pseudo-success.
6. Record the rollback candidate, flag revision, in-flight jobs, reconciliation result, residual outbox, and operator approval in an immutable run directory.

## Report executor reconciliation

Before resuming the worker, reconcile the PostgreSQL authority and MinIO bytes for the exact tenant and report IDs. Never infer completion from an object listing alone.

```sql
SELECT r.tenant_id,r.campaign_id,r.report_id,r.job_id,r.status,r.attempts,
       r.campaign_revision,r.snapshot_id,r.snapshot_sha256,
       r.object_bucket,r.object_key,r.mime_type,r.artifact_sha256,r.size_bytes,
       r.error_message,r.locked_by,r.locked_until,j.status AS action_status
FROM campaign_reports r
JOIN campaign_action_jobs j ON j.job_id=r.job_id
WHERE r.status IN ('accepted','running','failed')
   OR j.status IS DISTINCT FROM r.status
ORDER BY r.tenant_id,r.created_at,r.report_id;

SELECT tenant_id,campaign_id,event_type,aggregate_revision,event_id,trace_id
FROM campaign_aggregate_history
WHERE event_type IN ('traffic.campaign.v2.ReportRequested',
                     'traffic.campaign.v2.ReportCompleted',
                     'traffic.campaign.v2.ReportFailed')
ORDER BY tenant_id,campaign_id,aggregate_revision;
```

For every `completed` row, read the exact `object_bucket/object_key`, enforce the recorded size budget, compute SHA-256 over the returned bytes, and compare it with `artifact_sha256`. Then verify the matching terminal action job, aggregate history/outbox event, audit row, campaign revision, frozen `snapshot_id` and `snapshot_sha256`. A retryable object-store failure must remain accepted with an incremented attempt count; the fifth failure must atomically set both report and action job to failed and append `ReportFailed` history/outbox plus `CAMPAIGN_REPORT_FAILED` audit. Do not overwrite a mismatched object or manually change a terminal row to completed.

Resume only from the additive schema and a repaired content-addressed image. Expired running leases may be reclaimed by the worker; accepted jobs retain their frozen snapshot and deterministic object key. If object bytes were written but the PostgreSQL terminal transaction did not commit, a retry may replace only that same deterministic key with bytes rendered from the identical frozen snapshot, then commit a newly verified manifest.

## SOAR executor reconciliation

Do not retry an ambiguous provider effect until the provider can answer the stable `job_id:execute` or `job_id:compensate` idempotency identity. Reconcile the PostgreSQL workflow and provider ledger first:

```sql
SELECT s.tenant_id,s.campaign_id,s.job_id,s.playbook_id,s.status,
       s.approval_status,s.executor_status,s.revision,s.attempts,
       s.locked_by,s.locked_until,s.requested_by,s.approved_by,
       a.status AS action_status,a.resource_revision
FROM campaign_soar_jobs s
JOIN campaign_action_jobs a ON a.tenant_id=s.tenant_id AND a.job_id=s.job_id
WHERE s.status IN ('approved_awaiting_executor','running','compensation_queued','compensating','failed','compensation_failed')
   OR a.status IS DISTINCT FROM s.status
ORDER BY s.tenant_id,s.created_at,s.job_id;

SELECT tenant_id,campaign_id,job_id,phase,attempt,provider,
       provider_receipt_id,status,external_effect,payload_sha256,created_at
FROM campaign_soar_execution_receipts
ORDER BY tenant_id,job_id,phase,attempt;

SELECT tenant_id,campaign_id,event_type,aggregate_revision,event_id
FROM campaign_aggregate_history
WHERE event_type IN ('traffic.campaign.v2.SoarRequested',
                     'traffic.campaign.v2.SoarCompleted',
                     'traffic.campaign.v2.SoarPartial',
                     'traffic.campaign.v2.SoarFailed',
                     'traffic.campaign.v2.SoarCompensated',
                     'traffic.campaign.v2.SoarCompensationFailed')
ORDER BY tenant_id,campaign_id,aggregate_revision;
```

For a provider-confirmed receipt that PostgreSQL lacks, do not insert a receipt or change a status manually. Stop the worker, retain the provider's immutable response and escalate to the release owner for an audited repair command. For a PostgreSQL terminal receipt, recompute the canonical receipt SHA-256, match provider and provider receipt identity, then reconcile the terminal action job, campaign revision, history/outbox event and audit row. Compensation is permitted only for a completed/partial execution receipt with `external_effect=true` and must be approved by an identity different from the original requester.

## Three-target reconciliation

Run read-only reconciliation before any replay. At minimum, retain the result of:

```sql
SELECT tenant_id,projection_status,target_status,count(*)
FROM campaign_event_projection_inbox
GROUP BY tenant_id,projection_status,target_status
ORDER BY tenant_id,projection_status,target_status;

SELECT tenant_id,target,stream,count(*),max(projection_version)
FROM campaign_target_projection_watermarks
GROUP BY tenant_id,target,stream
ORDER BY tenant_id,target,stream;

SELECT inbox.tenant_id,inbox.stream,inbox.event_id,inbox.campaign_id,
       inbox.relation_id,inbox.aggregate_revision,inbox.relation_revision,
       inbox.target_status,inbox.last_error
FROM campaign_event_projection_inbox inbox
WHERE inbox.projection_status IN ('pending','processing','partial','dead')
ORDER BY inbox.tenant_id,inbox.received_at,inbox.stream,inbox.event_id;
```

Compare those rows with `traffic.campaign_projection_events_v2 FINAL` by immutable `projection_id`, with `campaign-projections-v2-read` by deterministic state document ID and external version, and with the tenant-scoped NebulaGraph `entity` vertex `metadata_json.projection_version` or `relation` edge `attributes_json.relation_revision`. Both NebulaGraph metadata documents retain `projection_sha256`, so the event ID, trace, revision and deterministic bytes can be compared with the PostgreSQL target watermark. The business kind remains `entity_type=campaign` or `relation_type=campaign_alert`; those are property values, not separate tag or edge Schema names. An HTTP 2xx, ClickHouse row count, or NebulaGraph vertex alone is not sufficient: revision, event ID and projection SHA must agree with PostgreSQL.

For a newly expanded NebulaGraph space, wait until the `entity` tag and `relation` edge have propagated to storage before starting the projection worker. Treat `No schema found` as dependency-not-ready and keep the feature flag disabled; do not rely on a second deployment or manual retry to hide the propagation window.

## Targeted rebuild

Rebuild only after the worker is disabled and the release owner has approved an exact tenant, projection key, revision range and target. Preserve a before-snapshot of the inbox and watermark rows. Do not reset a target that has an unresolved same-version identity collision.

For a single target, remove only that target's PostgreSQL watermark rows and change only that key in `target_status` back to `pending`; keep other target states unchanged. Set the affected inbox rows to `pending`, clear expired lease fields and make them available in ascending projection revision. Re-enable the worker for the internal tenant, verify that only the selected target receives writes, reconcile all hashes, then expand. Never delete ClickHouse immutable events as part of rebuild; OpenSearch and NebulaGraph current-state projections must use deterministic IDs and monotonic revisions.

## Re-enable criteria

- the triggering defect has a regression test;
- membership backfill and aggregate revision reconciliation pass for the canary tenant;
- link/unlink command receipts replay their original result and relation/member counts reconcile in both API directions;
- accepted report/SOAR states remain distinct from final object/executor receipts;
- report retry, terminal failure, cross-tenant status, premature download, tampered-object rejection, and manifest-verified download tests pass against sentinel-protected disposable PostgreSQL and MinIO;
- duplicate, out-of-order, expired-lease, partial-target, dead-target, same-version collision, tenant-isolation and one-target rebuild tests pass against a sentinel-protected disposable PostgreSQL instance;
- all three external target readiness checks pass and their revision/event/hash reconciliation is exact for the canary tenant;
- the release owner, independent QA, and SRE approve a new content-addressed candidate.
