# T1-M09-N006 alert snapshot repository rollback

## Scope

This change replaces the alert-search read path only. OpenSearch still selects
and orders candidates; ClickHouse supplies alert facts; PostgreSQL supplies
newer analyst state and projection receipts. It adds response evidence for
`missing`, `stale`, `extra`, source watermarks, state source and projection
status. It does not add a schema, writer, topic, worker or destructive repair.

## Rollback trigger

Rollback the application read seam if the new path causes sustained search
availability loss, invalid tenant scoping, incorrect candidate ordering, or an
unacceptable latency regression. Cross-store differences by themselves are
not a rollback trigger: they are the condition this read model must expose.

## Procedure

1. Stop rollout expansion and keep the current production digest available.
2. Roll `alert-service` back to the previous approved digest, or remove only
   the `SetAlertSnapshotRepository(NewAlertSnapshotRepository(...))`
   composition call and rebuild. The existing `AlertService` OpenSearch search
   path remains as the compatibility fallback.
3. Verify `/api/v1/alerts/search` through APISIX for one tenant, including a
   legacy page and a PIT/cursor page. Confirm the former status code, cursor
   behavior and permission checks.
4. Record the failed candidate digest, trace IDs, query shape and observed
   `missing/stale/extra` evidence before closing the rollout attempt.

## Data safety

- Do not delete or rewrite ClickHouse alerts, OpenSearch documents,
  `alert_assignment_states`, or
  `alert_opensearch_projection_watermarks` during application rollback.
- Do not repair or delete `extra` OpenSearch documents from the request path.
- Do not convert OpenSearch fields back into alert or analyst-state authority.
- No database rollback is required because N006 contains no migration.

## Forward recovery

After correcting the candidate, rerun the repository and handler contract
tests, then the isolated K8s Job. A production rollout still requires a
separate canary against shared ClickHouse, OpenSearch and PostgreSQL and must
show tenant isolation, bounded latency and truthful degradation metadata.
