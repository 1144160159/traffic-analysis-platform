# F-ENCRYPTED-002 action-job rollback

Scope: stop the additive durable action-job path while preserving every accepted command, history row, audit record, outbox event, executor receipt and final entity.

## Stop and rollback

1. Stop canary expansion and set `ENCRYPTED_ACTION_JOBS_V1_ENABLED=false` plus every action-specific dispatcher flag to false.
2. Stop accepting new durable jobs. Snapshot the candidate, configuration, job revisions, outbox/inbox offsets and executor receipts.
3. Classify already accepted jobs as safe to finish, fail, cancel or compensate under their frozen policy. Never relabel `accepted` as `completed`.
4. Restore the legacy egress/evidence request adapter for new requests. It may report only its historical recorded/accepted semantics.
5. Retain final alerts, evidence objects, preservation holds, analysis links and provider receipts. External effects require their approved provider compensation path.
6. Reconcile PostgreSQL job/history/audit/outbox, Kafka offsets, executor receipts, final entities and MinIO manifests. Resolve no discrepancy by deletion or in-place receipt rewrite.

Rollback passes only when new dispatch is absent, all accepted jobs have a queryable terminal or explicit recovery state, no duplicate final entity exists, and the legacy UI labels remain truthful.
