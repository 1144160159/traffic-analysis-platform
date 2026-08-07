# F-CAMPAIGN-001 historical membership backfill

## Purpose and authority

This procedure binds historical campaign membership to the PostgreSQL campaign aggregate before enabling `CAMPAIGN_AGGREGATE_V2_ENABLED`. The input is an immutable, tenant-scoped ClickHouse export manifest retained in MinIO. The command is additive: it binds legacy linked rows, inserts missing linked rows, leaves already-versioned links unchanged, and records explicit unlinks as `skipped_explicit_unlink`. It never resurrects an unlinked relation or removes a PostgreSQL relation that is absent from the export.

Apply `deployments/postgres/migrations/202608010930_campaign_membership_backfill_v1.sql` before running the command. A run is limited to 100 campaigns and 1,000 source members per campaign. Split larger inventories into independently signed manifests.

## Manifest

Use a new UUID for each immutable manifest. `source.sha256` is the SHA-256 of the retained ClickHouse export object, not the manifest. `expected_campaign_revision` must be read from PostgreSQL immediately before approval.

```json
{
  "contract_version": 1,
  "run_id": "11111111-1111-4111-8111-111111111111",
  "tenant_id": "tenant-canary",
  "source": {
    "kind": "clickhouse_export",
    "uri": "minio://alignment-evidence/campaign-members/tenant-canary.json",
    "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "snapshot_id": "ch-campaign-members-20260801T120000Z",
    "as_of": "2026-08-01T12:00:00Z"
  },
  "reason": "bind approved historical campaign memberships for canary",
  "created_by": "alignment-operator",
  "campaigns": [
    {
      "campaign_id": "CAM-001",
      "expected_campaign_revision": 7,
      "alert_ids": ["AL-001", "AL-002"]
    }
  ]
}
```

## Preflight and execution

1. Freeze the ClickHouse query range and record its query, row count, `snapshot_id`, `as_of`, object URI, size and SHA-256. Independently verify the object hash after upload.
2. Confirm the tenant and each campaign exist. Capture `campaign_workbench_state.state_version`, its recorded `member_count`, the actual linked count, and every `campaign_revision=0` relation.
3. Review all source members that currently have `status='unlinked'`. They will be retained as unlinked and must be accepted as explicit exceptions before execution.
4. Run from the approved candidate image without printing the DSN:

   ```bash
   POSTGRES_DSN='postgres://...' go run ./cmd/campaign-membership-backfill \
     -manifest /evidence/campaign-membership-backfill.json
   ```

5. Retain stdout as the run summary. A `partial` result exits non-zero. Re-running the exact same manifest and `run_id` skips completed campaigns and retries failed campaigns. Reusing a `run_id` with changed bytes is rejected.

## Reconciliation

All queries must be tenant- and run-scoped:

```sql
SELECT run_id,status,manifest_sha256,campaign_count,source_member_count,
       completed_campaign_count,failed_campaign_count,
       inserted_count,bound_count,unchanged_count,skipped_unlinked_count
FROM campaign_membership_backfill_runs
WHERE tenant_id=$1 AND run_id=$2::uuid;

SELECT campaign_id,status,expected_campaign_revision,starting_campaign_revision,
       resulting_campaign_revision,source_member_count,resulting_member_count,
       inserted_count,bound_count,unchanged_count,skipped_unlinked_count,
       aggregate_event_id,error_code,error_message
FROM campaign_membership_backfill_campaigns
WHERE tenant_id=$1 AND run_id=$2::uuid
ORDER BY campaign_id;

SELECT campaign_id,outcome,count(*)
FROM campaign_membership_backfill_items
WHERE tenant_id=$1 AND run_id=$2::uuid
GROUP BY campaign_id,outcome
ORDER BY campaign_id,outcome;

SELECT tenant_id,campaign_id,
       count(*) FILTER (WHERE status='linked') AS actual_members,
       count(*) FILTER (WHERE status='linked' AND campaign_revision=0) AS unbound_members
FROM campaign_alert_links
WHERE tenant_id=$1
GROUP BY tenant_id,campaign_id
ORDER BY campaign_id;
```

For each completed campaign, the four outcome counts must equal the source member count; `campaign_workbench_state.member_count` must equal the actual linked count; every inserted or bound item must have one membership history row and outbox row; a changed campaign must have one `MembershipBackfilled` aggregate history/outbox pair; and one `CAMPAIGN_MEMBERSHIP_BACKFILLED` audit row must reference the run and manifest hashes. Reconcile emitted events through Kafka and all three target watermarks before enabling the feature flag.

## Stop and recovery

Stop on a tenant mismatch, source hash mismatch, manifest reuse conflict, revision conflict, relation identity collision, explicit-unlink count above the approved exception set, audit failure, or history/outbox mismatch. Do not edit receipts or bulk-update `campaign_revision`.

For a transient failed campaign, remove the transient condition and rerun the exact manifest; completed campaigns are not emitted again. A revision conflict requires a new export, a new approval and a new `run_id`. Rollback is an explicitly approved compensating command or manifest, never deletion of the completed run, items, histories, outboxes or audit evidence.
