#!/usr/bin/env python3
"""Fail closed on T-DR-001 cross-store recovery catalog drift and false closure."""

from __future__ import annotations

import hashlib
import json
from typing import Any

from build_dr_recovery_catalog import OUTPUT, ROOT, _canonical_sha256, build_catalog


EXPECTED_DOMAINS = {
    "postgresql_authority",
    "kafka_event_log",
    "clickhouse_facts",
    "minio_objects",
    "nebula_projection",
    "opensearch_projection",
    "redis_runtime_state",
    "flink_state",
}
EXPECTED_RECOVERY_STEPS = [
    "identity_dns_configuration_and_certificate_authorities",
    "postgresql_authoritative_state_outbox_and_audit",
    "kafka_topics_acls_offsets_and_event_replay",
    "clickhouse_analytical_facts",
    "minio_objects_against_postgresql_manifests",
    "nebula_and_opensearch_rebuildable_projections",
    "redis_coordination_session_and_cache_domains",
    "flink_from_approved_savepoint_then_sink_reconcile",
    "cross_store_watermark_manifest_and_business_reconciliation",
]


def verify() -> dict[str, Any]:
    errors: list[str] = []
    if not OUTPUT.is_file():
        return {"status": "FAIL", "errors": [f"missing {OUTPUT.relative_to(ROOT)}"]}
    actual = json.loads(OUTPUT.read_text(encoding="utf-8"))
    expected = build_catalog()
    if actual != expected:
        errors.append("DR recovery catalog is stale relative to governed authorities")
    if actual.get("schema_version") != 1 or actual.get("control_id") != "T-DR-001":
        errors.append("catalog identity must be schema v1 and T-DR-001")
    if actual.get("status") != "candidate_default_off" or actual.get("production_applied") is not False:
        errors.append("DR catalog cannot claim rollout or closure without live restore evidence")
    if actual.get("destructive_execution_authorized") is not False:
        errors.append("repository catalog must never auto-authorize destructive DR execution")

    content = dict(actual)
    catalog_sha256 = content.pop("catalog_sha256", None)
    if catalog_sha256 != _canonical_sha256(content):
        errors.append("catalog_sha256 does not match canonical catalog content")
    for authority in actual.get("authorities") or []:
        path = ROOT / str(authority.get("path") or "")
        if not path.is_file():
            errors.append(f"authority missing: {authority.get('domain')}")
        elif authority.get("sha256") != hashlib.sha256(path.read_bytes()).hexdigest():
            errors.append(f"authority hash drift: {authority.get('domain')}")

    domains = actual.get("domains") or []
    domain_ids = {str(item.get("domain_id") or "") for item in domains}
    if domain_ids != EXPECTED_DOMAINS:
        errors.append(
            f"DR domain inventory drift: missing={sorted(EXPECTED_DOMAINS-domain_ids)} "
            f"extra={sorted(domain_ids-EXPECTED_DOMAINS)}"
        )
    if any(item.get("repository_state") == "PASS" for item in domains):
        errors.append("no DR domain has sufficient isolated restore evidence to claim PASS")
    for domain in domains:
        domain_id = domain.get("domain_id")
        if not domain.get("blocking_gaps"):
            errors.append(f"{domain_id}: blocking gaps cannot be hidden")
        if domain.get("rpo_rto_approved") is not False:
            errors.append(f"{domain_id}: RPO/RTO approval cannot be invented")
        restore = domain.get("restore") or {}
        if restore.get("last_successful_evidence") is not None:
            errors.append(f"{domain_id}: repository slice cannot invent successful restore evidence")
        if not domain.get("business_oracles"):
            errors.append(f"{domain_id}: business restore oracle is required")

    policy = actual.get("policy") or {}
    if policy.get("backup_success_is_restore_success") is not False:
        errors.append("backup success must never substitute for restore success")
    if policy.get("same_cluster_restore_is_acceptance_evidence") is not False:
        errors.append("same-cluster restore cannot be DR acceptance evidence")
    for required in (
        "isolated_restore_required",
        "business_oracle_required",
        "immutable_manifest_and_sha256_required",
        "approved_maintenance_window_required",
        "two_person_approval_required",
        "latest_pointer_must_resolve_to_hash",
    ):
        if policy.get(required) is not True:
            errors.append(f"required DR policy guard missing: {required}")

    recovery_order = actual.get("recovery_order") or []
    if [item.get("order") for item in recovery_order] != list(range(1, 10)):
        errors.append("recovery order must be contiguous from 1 through 9")
    if [item.get("step") for item in recovery_order] != EXPECTED_RECOVERY_STEPS:
        errors.append("cross-store recovery order drifted from authoritative sequence")
    order_by_domain = {str(item.get("domain_id")): item.get("restore_order") for item in domains}
    if order_by_domain.get("postgresql_authority") != 2 or order_by_domain.get("kafka_event_log") != 3:
        errors.append("PostgreSQL authority must recover before Kafka replay")
    if order_by_domain.get("minio_objects") != 5:
        errors.append("MinIO objects must reconcile after PostgreSQL manifests")
    if order_by_domain.get("flink_state") != 8:
        errors.append("Flink restore must follow authoritative stores and projections")

    pg = next((item for item in domains if item.get("domain_id") == "postgresql_authority"), {})
    if (pg.get("fencing") or {}).get("old_primary_fenced_before_endpoint_publication") is not False:
        errors.append("PostgreSQL fencing gap cannot be hidden")
    if "isolated_pitr_restore_missing" not in pg.get("blocking_gaps", []):
        errors.append("PostgreSQL PITR gap cannot be hidden")
    ch = next((item for item in domains if item.get("domain_id") == "clickhouse_facts"), {})
    if "failure_domain_proof_missing" not in ch.get("blocking_gaps", []):
        errors.append("ClickHouse failure-domain gap cannot be hidden")
    minio = next((item for item in domains if item.get("domain_id") == "minio_objects"), {})
    if "manifest_object_reconciliation_missing" not in minio.get("blocking_gaps", []):
        errors.append("MinIO manifest/object reconciliation gap cannot be hidden")

    counts = actual.get("counts") or {}
    expected_counts = {
        "domains": len(domains),
        "domains_pass": sum(domain.get("repository_state") == "PASS" for domain in domains),
        "domains_without_restore_evidence": sum(
            (domain.get("restore") or {}).get("last_successful_evidence") is None for domain in domains
        ),
        "domains_without_approved_rpo_rto": sum(
            domain.get("rpo_rto_approved") is not True for domain in domains
        ),
        "blocking_gaps": len(actual.get("blocking_gaps") or []),
    }
    if counts != expected_counts:
        errors.append("DR catalog counts do not match catalog content")

    return {
        "status": "PASS" if not errors else "FAIL",
        "control_id": "T-DR-001",
        "catalog_integrity": "PASS" if not errors else "FAIL",
        "dr_readiness": "PARTIAL",
        "catalog_sha256": actual.get("catalog_sha256"),
        "counts": counts,
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
