# F-FORENSICS-001 pipeline rollback

Scope: stop new forensics work and return reads/actions to the legacy route without deleting PCAP sources, derived objects, restoration manifests, legal holds, chain-of-custody records or download audits.

## Stop and rollback

1. Stop canary expansion. Set the forensics command-writer, dispatcher and worker feature flags to false in that order.
2. Freeze the candidate image/config, accepted job set, leases, checkpoints, PostgreSQL revisions, Kafka offsets, ClickHouse PCAP-index watermark and MinIO version/hash inventory.
3. For each accepted job, use its frozen policy to finish, fail or cancel. Do not abandon an active lease silently and do not publish a temporary object.
4. Restore the previous `/forensics` UI and existing PCAP job, cut, verify, presign and download routes. Preserve explicit accepted-versus-completed semantics.
5. Retain published objects and manifests. Delete no source or derived object during rollback; lifecycle cleanup is a separately approved, reference- and legal-hold-aware operation.
6. Reconcile orphan temporary objects, published objects without manifests, manifests without objects, active leases, checkpoints, outbox/inbox events and final job states.
7. Verify that restored file content remains inert bytes and that cross-tenant, expired URL, wrong-purpose, missing-object and wrong-hash requests still fail closed.

Rollback passes only when no new job is admitted to the candidate path, every accepted job is recoverable and queryable, all retained object hashes match their manifests, and every returned byte remains preceded by durable download authorization and audit.
