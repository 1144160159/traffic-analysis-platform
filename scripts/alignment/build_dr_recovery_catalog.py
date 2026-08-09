#!/usr/bin/env python3
"""Build the repository-side T-DR-001 cross-store recovery catalog."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "contracts/reliability/dr-recovery-catalog.v1.json"
AUTHORITY_PATHS = {
    "postgres_ha_pitr": ROOT / "contracts/postgres/ha-pitr-fencing.v1.json",
    "clickhouse_ha_backup": ROOT / "contracts/clickhouse/ha-security-backup.v1.json",
    "opensearch_snapshot_restore": ROOT / "contracts/opensearch/ha-security-restore.v1.json",
    "flink_checkpoint_ha": ROOT / "contracts/flink/checkpoint-ha-upgrade.v1.json",
    "redis_reliability_domains": ROOT / "contracts/redis/reliability-domains.v1.json",
    "postgres_topology": ROOT / "deployments/kubernetes/infrastructure/03-postgresql.yaml",
    "clickhouse_topology": ROOT / "deployments/kubernetes/infrastructure/02-clickhouse.yaml",
    "opensearch_topology": ROOT / "deployments/kubernetes/infrastructure/05-opensearch.yaml",
    "kafka_topology": ROOT / "deployments/kubernetes/infrastructure/01-kafka.yaml",
    "flink_topology": ROOT / "deployments/kubernetes/infrastructure/07-flink.yaml",
    "redis_topology": ROOT / "deployments/kubernetes/infrastructure/04-redis.yaml",
    "minio_topology": ROOT / "deployments/kubernetes/infrastructure/06-minio.yaml",
    "nebula_topology": ROOT / "deployments/kubernetes/infrastructure/09-nebula-graph.yaml",
}


def _relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def _sha256(value: bytes | str) -> str:
    if isinstance(value, str):
        value = value.encode("utf-8")
    return hashlib.sha256(value).hexdigest()


def _canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return _sha256(payload)


def _json(relative: str) -> dict[str, Any]:
    return json.loads((ROOT / relative).read_text(encoding="utf-8"))


def build_catalog() -> dict[str, Any]:
    pg = _json("contracts/postgres/ha-pitr-fencing.v1.json")
    clickhouse = _json("contracts/clickhouse/ha-security-backup.v1.json")
    opensearch = _json("contracts/opensearch/ha-security-restore.v1.json")
    flink = _json("contracts/flink/checkpoint-ha-upgrade.v1.json")
    redis = _json("contracts/redis/reliability-domains.v1.json")

    ch_plan = clickhouse["backup_restore_plan"]
    os_plan = opensearch["snapshot_restore"]
    domains = [
        {
            "domain_id": "postgresql_authority",
            "authority_class": "authoritative_transactional_state",
            "owner": "postgres-platform-owner",
            "restore_order": 2,
            "repository_state": "NOT_IMPLEMENTED",
            "rpo_minutes": None,
            "rto_minutes": None,
            "rpo_rto_approved": False,
            "backup": {"mode": "PITR_WAL_AND_BASE_BACKUP_REQUIRED", "evidence": None},
            "restore": {"target": "isolated_postgresql", "last_successful_evidence": None},
            "fencing": {
                "required": True,
                "controller_state": pg["target_ha_controller"]["repository_state"],
                "old_primary_fenced_before_endpoint_publication": False,
            },
            "business_oracles": pg["drill"]["business_oracles"],
            "blocking_gaps": [
                "approved_rpo_rto_missing",
                "wal_archive_and_base_backup_missing",
                "isolated_pitr_restore_missing",
                "single_writer_controller_and_fencing_missing",
            ],
        },
        {
            "domain_id": "kafka_event_log",
            "authority_class": "durable_event_and_replay_log",
            "owner": "kafka-platform-owner",
            "restore_order": 3,
            "repository_state": "MANIFEST_ONLY_NO_DR_CONTRACT",
            "rpo_minutes": None,
            "rto_minutes": None,
            "rpo_rto_approved": False,
            "backup": {"mode": "topic_configuration_acl_offset_and_payload_strategy_required", "evidence": None},
            "restore": {"target": "isolated_kafka_cluster", "last_successful_evidence": None},
            "fencing": {"required": True, "producer_epoch_or_writer_fence_verified": False},
            "business_oracles": [
                "topic partition replication and ACL inventory matches manifest",
                "bounded offset ranges replay without unexplained gaps",
                "duplicate and out-of-order events remain classifiable",
                "downstream deterministic event IDs reconcile",
            ],
            "blocking_gaps": [
                "approved_rpo_rto_missing",
                "backup_and_restore_strategy_missing",
                "isolated_restore_and_offset_replay_missing",
                "producer_fencing_and_cross_store_reconcile_missing",
            ],
        },
        {
            "domain_id": "clickhouse_facts",
            "authority_class": "analytical_fact_store",
            "owner": "clickhouse-platform-owner",
            "restore_order": 4,
            "repository_state": "PARTIAL_PLAN_DEFAULT_OFF",
            "rpo_minutes": ch_plan["rpo_minutes"],
            "rto_minutes": ch_plan["rto_minutes"],
            "rpo_rto_approved": False,
            "backup": {
                "mode": "native_backup_to_approved_encrypted_object_storage",
                "destination_approved": ch_plan["backup_destination_approved"],
                "evidence": None,
            },
            "restore": {
                "target": ch_plan["restore_target"],
                "isolated_required": True,
                "last_successful_evidence": ch_plan["last_successful_restore_evidence"],
            },
            "failure_domains": clickhouse["topology_target"],
            "business_oracles": ch_plan["required_validation"],
            "blocking_gaps": [
                "backup_destination_and_key_recovery_unapproved",
                "isolated_restore_missing",
                "failure_domain_proof_missing",
                "approved_rpo_rto_missing",
            ],
        },
        {
            "domain_id": "minio_objects",
            "authority_class": "content_addressed_object_payloads",
            "owner": "minio-platform-owner",
            "restore_order": 5,
            "repository_state": "TOPOLOGY_ONLY_NO_DR_CONTRACT",
            "rpo_minutes": None,
            "rto_minutes": None,
            "rpo_rto_approved": False,
            "backup": {"mode": "versioning_replication_or_immutable_backup_required", "evidence": None},
            "restore": {"target": "isolated_minio_tenant", "last_successful_evidence": None},
            "fencing": {"required": True, "writer_and_bucket_cutover_fenced": False},
            "business_oracles": [
                "PostgreSQL manifest object key size sha256 and media type match",
                "missing orphan and corrupt objects are enumerated",
                "retention legal hold and lifecycle remain intact",
                "partial object and metadata success cannot be reported as complete",
            ],
            "blocking_gaps": [
                "approved_rpo_rto_missing",
                "versioning_replication_backup_authority_missing",
                "isolated_restore_missing",
                "manifest_object_reconciliation_missing",
            ],
        },
        {
            "domain_id": "nebula_projection",
            "authority_class": "rebuildable_graph_projection",
            "owner": "graph-platform-owner",
            "restore_order": 6,
            "repository_state": "TOPOLOGY_ONLY_NO_DR_CONTRACT",
            "rpo_minutes": None,
            "rto_minutes": None,
            "rpo_rto_approved": False,
            "backup": {"mode": "projection_backup_optional_if_rebuild_budget_proven", "evidence": None},
            "restore": {"target": "isolated_nebula_or_rebuild_from_authorities", "last_successful_evidence": None},
            "fencing": {"required": True, "projection_writer_watermark_fenced": False},
            "business_oracles": [
                "PG and ClickHouse source watermarks are recorded",
                "deterministic vertex and edge IDs reconcile",
                "tenant bounded node edge and property samples match",
                "query truncation and partial state remain explicit during rebuild",
            ],
            "blocking_gaps": [
                "approved_rebuild_or_backup_budget_missing",
                "isolated_restore_or_full_rebuild_missing",
                "meta_storage_failure_domain_proof_missing",
                "source_watermark_reconciliation_missing",
            ],
        },
        {
            "domain_id": "opensearch_projection",
            "authority_class": "rebuildable_search_projection",
            "owner": "opensearch-platform-owner",
            "restore_order": 6,
            "repository_state": "PARTIAL_TARGET_DEFAULT_OFF",
            "rpo_minutes": None,
            "rto_minutes": None,
            "rpo_rto_approved": False,
            "backup": {
                "mode": "versioned_s3_snapshot",
                "repository": os_plan["bucket"],
                "evidence": None,
            },
            "restore": {
                "target": "isolated_opensearch_cluster",
                "isolated_required": os_plan["restore_target_must_be_isolated"],
                "same_cluster_forbidden": os_plan["same_cluster_restore_forbidden"],
                "last_successful_evidence": None,
            },
            "failure_domains": opensearch["failure_domains"],
            "business_oracles": os_plan["required_verification"],
            "blocking_gaps": [
                "approved_rpo_rto_missing",
                "live_snapshot_repository_missing",
                "isolated_restore_missing",
                "three_failure_domain_proof_missing",
            ],
        },
        {
            "domain_id": "redis_runtime_state",
            "authority_class": "recoverable_coordination_session_and_cache_state",
            "owner": "redis-platform-owner",
            "restore_order": 7,
            "repository_state": "PARTIAL_SAFE_HOLD",
            "rpo_minutes": None,
            "rto_minutes": None,
            "rpo_rto_approved": False,
            "backup": {"mode": "aof_for_coordination_session_cache_discardable", "evidence": None},
            "restore": {"target": "fresh_sentinel_domains_and_rebuilt_cache", "last_successful_evidence": None},
            "fencing": {"required": True, "sentinel_failover_writer_fence_verified": False},
            "business_oracles": [
                "coordination keys fail closed or recover from PostgreSQL authority",
                "session validation falls back only to PostgreSQL authority",
                "cache can be discarded and rebuilt without business data loss",
                "cross-domain key prefixes remain zero",
            ],
            "blocking_gaps": list(redis["remaining_gates"]),
        },
        {
            "domain_id": "flink_state",
            "authority_class": "stream_processor_checkpoint_and_savepoint_state",
            "owner": "flink-platform-owner",
            "restore_order": 8,
            "repository_state": "PARTIAL_CONTRACT_NO_LIVE_RESTORE",
            "rpo_minutes": None,
            "rto_minutes": flink["slo"]["critical_recovery_time_seconds_max"] / 60,
            "rpo_rto_approved": False,
            "backup": {
                "mode": "durable_checkpoint_and_savepoint_on_minio",
                "checkpoint_root": flink["ha"]["checkpoint_root"],
                "savepoint_root": flink["ha"]["savepoint_root"],
                "evidence": None,
            },
            "restore": {"target": "isolated_application_cluster", "last_successful_evidence": None},
            "fencing": {"required": True, "single_job_writer_and_sink_epoch_verified": False},
            "business_oracles": flink["slo"]["required_checkpoint_evidence"],
            "blocking_gaps": [
                "approved_rpo_rto_missing",
                "live_checkpoint_savepoint_manifest_missing",
                "isolated_restore_and_state_compatibility_missing",
                "source_offset_and_sink_reconciliation_missing",
            ],
        },
    ]
    gaps = sorted({gap for domain in domains for gap in domain["blocking_gaps"]})
    catalog: dict[str, Any] = {
        "schema_version": 1,
        "control_id": "T-DR-001",
        "work_package": "WP-28-DR",
        "status": "candidate_default_off",
        "production_applied": False,
        "destructive_execution_authorized": False,
        "authorities": [
            {"domain": name, "path": _relative(path), "sha256": _sha256(path.read_bytes())}
            for name, path in sorted(AUTHORITY_PATHS.items())
        ],
        "policy": {
            "backup_success_is_restore_success": False,
            "same_cluster_restore_is_acceptance_evidence": False,
            "isolated_restore_required": True,
            "business_oracle_required": True,
            "immutable_manifest_and_sha256_required": True,
            "approved_maintenance_window_required": True,
            "two_person_approval_required": True,
            "latest_pointer_must_resolve_to_hash": True,
        },
        "recovery_order": [
            {"order": 1, "step": "identity_dns_configuration_and_certificate_authorities"},
            {"order": 2, "step": "postgresql_authoritative_state_outbox_and_audit"},
            {"order": 3, "step": "kafka_topics_acls_offsets_and_event_replay"},
            {"order": 4, "step": "clickhouse_analytical_facts"},
            {"order": 5, "step": "minio_objects_against_postgresql_manifests"},
            {"order": 6, "step": "nebula_and_opensearch_rebuildable_projections"},
            {"order": 7, "step": "redis_coordination_session_and_cache_domains"},
            {"order": 8, "step": "flink_from_approved_savepoint_then_sink_reconcile"},
            {"order": 9, "step": "cross_store_watermark_manifest_and_business_reconciliation"},
        ],
        "domains": domains,
        "blocking_gaps": gaps,
        "counts": {
            "domains": len(domains),
            "domains_pass": sum(domain["repository_state"] == "PASS" for domain in domains),
            "domains_without_restore_evidence": sum(
                domain["restore"]["last_successful_evidence"] is None for domain in domains
            ),
            "domains_without_approved_rpo_rto": sum(not domain["rpo_rto_approved"] for domain in domains),
            "blocking_gaps": len(gaps),
        },
        "acceptance": {
            "repository": [
                "all eight recovery domains are inventoried with immutable authorities",
                "backup evidence cannot substitute for isolated restore evidence",
                "recovery order preserves PostgreSQL authority and projection rebuild semantics",
                "destructive failover restore and cutover remain unapproved by repository changes",
            ],
            "remaining": [
                "approved per-domain RPO RTO capacity retention and encryption-key recovery",
                "isolated restore manifests object hashes and business oracles",
                "fencing failure-domain and former-writer rejoin proof",
                "cross-store event watermark object and projection reconciliation",
                "fault drills rollback T+0 T+1 T+3 T+7 and independent approval",
                "external G8 HA site and third-party acceptance",
            ],
        },
    }
    catalog["catalog_sha256"] = _canonical_sha256(catalog)
    return catalog


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    catalog = build_catalog()
    rendered = json.dumps(catalog, ensure_ascii=False, indent=2) + "\n"
    if args.check:
        current = OUTPUT.read_text(encoding="utf-8") if OUTPUT.is_file() else ""
        status = "PASS" if current == rendered else "FAIL"
        print(json.dumps({"status": status, "catalog_sha256": catalog["catalog_sha256"], "counts": catalog["counts"]}, ensure_ascii=False, indent=2))
        return 0 if status == "PASS" else 1
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT.write_text(rendered, encoding="utf-8")
    print(json.dumps({"status": "PASS", "output": _relative(OUTPUT), "catalog_sha256": catalog["catalog_sha256"], "counts": catalog["counts"]}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
