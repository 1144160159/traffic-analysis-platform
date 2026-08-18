# T1-M09-N011 Forensics Workbench rollback

Scope: the additive typed forensics client, task detail panel, immutable manifest receipts, refresh recovery, verify action, purpose-bound presign action, and their OpenAPI/page-plan declarations.

Rollback does not delete tasks, checkpoints, manifests, source objects, result objects, restoration objects, audits, or outbox records. It does not disable or mutate the M09-N009/N010 worker data plane.

1. Stop publishing the N011 Web UI candidate image and restore the previously approved Web UI image digest.
2. Keep `/forensics`, legacy task list/status panels, and existing `/api/v1/pcap/*` routes available. Do not redirect versioned result keys to a legacy object key.
3. Keep exact-version backend verification, presign, and download behavior in place while any N010 manifest exists. Reverting those checks would allow a newer object version to replace the bytes authorized by the manifest.
4. Revoke unexpired download grants through the existing access-control procedure when rollback was triggered by an authorization defect; preserve the corresponding download audit.
5. Confirm no N011 canary Job or Pod remains by its `traffic.analysis/canary-run` label. This cleanup applies only to run-scoped Kubernetes resources.
6. Re-open N011 only after the candidate bundle again proves: accepted is not completed, partial remains distinct, `job_id` refresh recovery works, and download is purpose- and object-version-bound.

The feature contract and task registry remain `DRAFT` / blocked until the later browser, reconciliation, rollout and approval gates are complete. A successful N011 bundle check is not project promotion evidence.
