# F-ENCRYPTED-001 snapshot rollback

Scope: roll back only the additive encrypted-traffic snapshot adapter. ClickHouse session/feature facts, PostgreSQL rule/model revisions, OpenSearch projections, MinIO evidence objects and audit records are retained.

## Stop and rollback

1. Freeze the candidate identity, failing request IDs, source watermarks, dependency errors and latency profile.
2. Set `ENCRYPTED_TRAFFIC_SNAPSHOT_V1_ENABLED=false`; do not delete snapshot facts or caches before reconciliation.
3. Restore the existing six read operations for stats, sessions, JA fingerprints, tunnels, exfiltration and evidence without changing `/encrypted-traffic`.
4. Invalidate only candidate-derived snapshot cache entries. Do not rewrite ClickHouse facts, OpenSearch documents or MinIO objects.
5. Reconcile the last observed window by tenant: legacy counts, candidate section counts, missing/partial reasons, maximum event time and evidence reference hashes.

Rollback passes only when legacy reads are healthy, no candidate snapshot is served, tenant and field permissions still fail closed, and retained source facts remain byte-identical. A dependency outage or unresolved count/hash difference leaves the feature blocked.
