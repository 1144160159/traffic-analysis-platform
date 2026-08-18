# T1-M09-N015 OpenSearch PIT/cursor rollback

This runbook covers `T1-M09-P035-PRJ-n015-s1` and `T1-M09-P036-UI-n015-s2`.

## Stop conditions

- cross-tenant results, repeated/missing page identities or a cursor accepted after query drift;
- a live cursor accepted after its alias resolves to a different physical index set;
- a timed-out/failed-shard response presented as complete, or any UI fallback to mock/legacy rows after cursor failure;
- PIT contexts not returning to the approved baseline, or latency/resource budgets crossing the approved canary threshold.

## Procedure

1. Set the Web runtime `ALERT_SEARCH_CURSOR_V1_ENABLED=false` first so browsers stop creating new PIT traversals.
2. Set alert-service `OPENSEARCH_SEARCH_CURSOR_V1_ENABLED=false` and roll back only to the previously approved image/config digest. Do not change `max_result_window` and do not switch or delete any production index/alias as part of this rollback.
3. Allow the two-minute TTL to expire, or close only PIT IDs attributable to the failed candidate. Never close unrelated cluster search contexts.
4. Verify the preserved shallow `POST /v1/alerts/search` and `/v1/alerts` page path with authenticated tenant and `alert:read`; a cursor request must return disabled/unavailable rather than fabricated data.
5. Record candidate/image/config hashes, affected tenant/query fingerprints, OpenSearch context counts, traces and the exact rollback timestamps. Keep durable alerts, projection receipts and audit facts unchanged.

## Recovery gate

Re-enable the backend only after target alias resolution, stable ordering, tenant isolation, failure closure and PIT cleanup pass on the replacement image. Re-enable the Web flag only after that backend receipt and an immutable bundle check. This runbook does not authorize production rollout.
