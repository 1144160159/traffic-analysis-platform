# T1-M10-N005 approved additive apply

## Purpose and current state

This operation may apply only exact migration, Kafka topic, ACL, and retention artifacts that were frozen by their responsible milestone, passed G1 for the same deployable candidate, passed the N004 site preflight/G6 boundary, and received independent release approval. Runtime is default-off. The current repository state is `BLOCKED_UNAPPROVED`; no shared Kubernetes, PostgreSQL, ClickHouse, or Kafka mutation is authorized.

## Code entrypoints

- `build_m10_approved_additive_plan.py::build(root)` inventories the six bounded proposal artifacts, computes their hashes, performs conservative SQL destructive-operation detection, and derives blockers.
- `verify_m10_approved_additive_plan.py::validate(expected, actual)` rejects hash, order, policy, blocker, compatibility, and authorization drift.
- `guard_m10_approved_additive_apply.py::evaluate_apply_authorization(root, plan)` is the mandatory pre-client boundary. A blocked decision has `mutating_client_started=false`.
- `run_m10_approved_additive_plan_k8s.py` proves the blocked boundary inside a non-root, read-only, no-service-account-token Kubernetes Job. It does not run a migration or Kafka client.

## Required sequence once external authority exists

1. Freeze one N001 candidate and exact site profile.
2. Resolve every N004 blocker and record `acceptance_status=PASS`, `G6=PASS` for that candidate and site.
3. Replace any `NON_ADDITIVE_BLOCKED` artifact with a separately reviewed expand-only artifact; do not waive a DROP finding.
4. Produce protected current-candidate G1 and independent release approval receipts. Historical or self-reported receipts are invalid.
5. Rebuild the plan and require `status=AUTHORIZED`, `apply_allowed=true`, and an empty blocker list before starting any mutating client.
6. Before each artifact, verify its exact SHA-256. After each artifact, verify the effect and persist a checkpoint. On half failure, stop and resume from the first unverified artifact; never run an automatic destructive rollback.
7. Keep legacy reads and writes enabled. Keep new writers off until post-apply verification passes.

## Rollback

Disable new writers and retain accepted tables, topics, ACL evidence, offsets, receipts, and audit facts. Do not DROP, truncate, delete, rename, rewrite a temporary contract, or hide a half-applied state. Reconcile the exact artifact/effect checkpoint before any retry.

## Current blockers

- No frozen deployable candidate.
- N004 site preflight and G6 are blocked.
- No same-candidate G1 receipt.
- No independent release approval.
- PostgreSQL migration `202608160030_m09_alert_evidence_links_v1.sql` contains `DROP TRIGGER` and therefore cannot enter an additive-only apply window.
