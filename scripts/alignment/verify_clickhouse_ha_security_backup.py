#!/usr/bin/env python3
"""Verify the repository-side T-CH-006 HA, security and restore guards."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/clickhouse/ha-security-backup.v1.json")
CLICKHOUSE_MANIFEST = Path("deployments/kubernetes/infrastructure/02-clickhouse.yaml")
ALERT_RULES = Path("deployments/kubernetes/observability/clickhouse-ha-alert-rules.yaml")
REQUIRED_SIGNALS = {
    "keeper_quorum",
    "replica_queue",
    "replica_delay",
    "parts_per_partition",
    "mutation_activity",
    "merge_saturation",
    "disk_available_ratio",
    "distributed_queue",
    "readonly_replica",
    "scrape_availability",
}
REQUIRED_ALERTS = {
    "ClickHouseMetricsEndpointMissing",
    "ClickHouseKeeperQuorumAtRisk",
    "ClickHouseReplicaReadonly",
    "ClickHouseReplicaQueueHigh",
    "ClickHouseReplicaDelayHigh",
    "ClickHousePartCountHigh",
    "ClickHouseMutationLongRunning",
    "ClickHouseMergeSaturation",
    "ClickHouseDiskFreeLow",
    "ClickHouseDistributedQueueStuck",
}
REQUIRED_RESTORE_CHECKS = {
    "manifest and object sha256",
    "database table and partition inventory",
    "row counts by tenant and partition",
    "stable primary-key sample",
    "aggregate count sum min max comparison",
    "replica queue and readonly state",
    "application query oracle before read cutover",
}


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    missing = [
        path.as_posix()
        for path in (CONTRACT, CLICKHOUSE_MANIFEST, ALERT_RULES)
        if not (root / path).is_file()
    ]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = json.loads((root / CONTRACT).read_text(encoding="utf-8"))
    manifest = (root / CLICKHOUSE_MANIFEST).read_text(encoding="utf-8")
    alerts = (root / ALERT_RULES).read_text(encoding="utf-8")

    if contract.get("remediation_id") != "T-CH-006":
        errors.append("contract remediation_id must be T-CH-006")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("partial HA/security/backup slice must not claim T-CH-006 closure")
    if contract.get("production_applied") is not False:
        errors.append("repository candidate must not claim production apply")

    policy = contract.get("policy", {})
    for key in (
        "current_health_is_not_ha_proof",
        "preferred_anti_affinity_is_not_failure_domain_proof",
        "silent_undercount_is_forbidden",
        "destructive_drills_require_approved_window",
        "backup_success_is_not_restore_success",
        "v2_migration_and_restore_drill_must_not_overlap",
        "tls_and_per_service_identity_are_required_before_closure",
    ):
        if policy.get(key) is not True:
            errors.append(f"HA guard must remain true: {key}")

    topology = contract.get("topology_target", {})
    keeper = topology.get("keeper", {})
    if keeper.get("members") != 3 or keeper.get("quorum") != 2:
        errors.append("Keeper target must preserve three members and quorum two")
    if keeper.get("minimum_distinct_failure_domains") != 3:
        errors.append("Keeper target must require three distinct failure domains")
    if topology.get("formal_failure_domain_proof") is not False:
        errors.append("two-node unlabeled live topology must not claim formal failure-domain proof")

    monitoring = contract.get("monitoring", {})
    signals = set(monitoring.get("required_signals", []))
    if signals != REQUIRED_SIGNALS:
        errors.append(f"monitoring signal catalog drift: missing={sorted(REQUIRED_SIGNALS - signals)}")
    for key in ("runtime_collector_present", "runtime_rule_evaluator_present", "notification_route_verified"):
        if monitoring.get(key) is not False:
            errors.append(f"repository candidate must keep unverified runtime monitoring false: {key}")

    for token in (
        "<endpoint>/metrics</endpoint>",
        "<port>9363</port>",
        "<metrics>true</metrics>",
        "<asynchronous_metrics>true</asynchronous_metrics>",
        "prometheus.io/scrape: \"true\"",
        "mountPath: /etc/clickhouse-server/users.d/traffic-runtime.xml",
        "subPath: users-governance.xml",
    ):
        if token not in manifest:
            errors.append(f"ClickHouse manifest missing metrics/profile token: {token}")
    if manifest.count("containerPort: 9363") != 4:
        errors.append("all three ClickHouse StatefulSets and Keeper must expose metrics port 9363")
    if manifest.count("cp /config-src/prometheus.xml /ch-config/prometheus.xml") != 3:
        errors.append("all ClickHouse StatefulSets must load the metrics endpoint config")
    if manifest.count("subPath: users-governance.xml") != 3:
        errors.append("all ClickHouse StatefulSets must load the candidate user profile")

    profile = contract.get("query_profile", {})
    settings = profile.get("settings", {})
    required_settings = {
        "max_memory_usage": 8589934592,
        "max_memory_usage_for_user": 12884901888,
        "max_threads": 8,
        "max_execution_time": 120,
        "max_bytes_to_read": 1099511627776,
        "max_distributed_connections": 64,
        "max_distributed_depth": 2,
        "skip_unavailable_shards": 0,
        "fallback_to_stale_replicas_for_distributed_queries": 0,
    }
    if settings != required_settings:
        errors.append("traffic_runtime profile settings drift from the reviewed fail-closed candidate")
    if profile.get("bound_users") != [] or profile.get(
        "candidate_is_inactive_until_per_service_users_are_bound"
    ) is not True:
        errors.append("candidate profile must remain explicitly unbound until identity/TLS rollout")
    for name, value in required_settings.items():
        token = f"<{name}>{value}</{name}>"
        if token not in manifest:
            errors.append(f"users-governance.xml missing setting: {name}")

    identity = contract.get("service_identity_target", {})
    if identity.get("default_user_allowed_for_runtime") is not False:
        errors.append("default user must remain forbidden by the target contract")
    if identity.get("transport") != "tls" or identity.get("current_target_applied") is not False:
        errors.append("TLS target must be explicit without claiming it is already applied")
    if len(identity.get("users", [])) < 9:
        errors.append("per-service identity target is incomplete")

    semantics = contract.get("dataset_failure_semantics", [])
    if len(semantics) < 3:
        errors.append("dataset failure semantics must cover raw, alert/evidence and rollup classes")
    for item in semantics:
        shard = str(item.get("one_shard_unavailable", ""))
        response = str(item.get("response_requirement", ""))
        if "fail" not in shard and "partial" not in shard:
            errors.append(f"dataset class lacks fail/partial shard semantics: {item.get('dataset_class')}")
        if not any(token in response for token in ("error", "partial=true")):
            errors.append(f"dataset class can silently undercount: {item.get('dataset_class')}")

    present_alerts = {
        line.split(":", 1)[1].strip()
        for line in alerts.splitlines()
        if line.lstrip().startswith("- alert:")
    }
    if present_alerts != REQUIRED_ALERTS:
        errors.append(
            f"ClickHouse alert catalog drift: missing={sorted(REQUIRED_ALERTS - present_alerts)} "
            f"extra={sorted(present_alerts - REQUIRED_ALERTS)}"
        )
    for metric in (
        "ClickHouseMetrics_ReadonlyReplica",
        "ClickHouseAsyncMetrics_ReplicasMaxQueueSize",
        "ClickHouseAsyncMetrics_ReplicasMaxAbsoluteDelay",
        "ClickHouseAsyncMetrics_MaxPartCountForPartition",
        "ClickHouseMetrics_PartMutation",
        "ClickHouseMetrics_Merge",
        "ClickHouseAsyncMetrics_DiskAvailable_default",
        "ClickHouseAsyncMetrics_DiskTotal_default",
        "ClickHouseMetrics_DistributedFilesToInsert",
    ):
        if metric not in alerts:
            errors.append(f"alert rules missing runtime-verified metric name: {metric}")

    backup = contract.get("backup_restore_plan", {})
    if backup.get("backup_destination_approved") is not False:
        errors.append("unapproved backup destination must remain false")
    if backup.get("encryption_key_recovery_verified") is not False:
        errors.append("unverified encryption key recovery must remain false")
    if set(backup.get("required_validation", [])) != REQUIRED_RESTORE_CHECKS:
        errors.append("restore validation oracle is incomplete")
    if backup.get("last_successful_restore_evidence") is not None:
        errors.append("repository slice must not invent successful restore evidence")
    if len(backup.get("backup_sets", [])) < 2:
        errors.append("critical facts and DDL/dictionary backup sets are both required")

    observed = contract.get("observed_live_before_candidate", {})
    if (
        observed.get("clickhouse_pods") != 4
        or observed.get("keeper_members") != 3
        or observed.get("distinct_nodes") != 2
        or observed.get("distinct_zones") != 0
        or observed.get("runtime_users") != ["default"]
        or observed.get("centralized_clickhouse_alerts_active") is not False
        or observed.get("observation_is_not_ha_security_or_restore_proof") is not True
    ):
        errors.append("pre-candidate live limitations must remain explicit")
    if len(contract.get("required_drills", [])) != 6:
        errors.append("all six T-CH-006 fault drills must be registered")
    if not contract.get("closure_blockers"):
        errors.append("T-CH-006 closure blockers must remain explicit")

    return {
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": contract.get("coverage_status"),
        "production_applied": False,
        "required_signals": len(signals),
        "alert_rules": len(present_alerts),
        "dataset_semantic_classes": len(semantics),
        "backup_sets": len(backup.get("backup_sets", [])),
        "formal_failure_domain_proof": topology.get("formal_failure_domain_proof"),
        "runtime_collector_present": monitoring.get("runtime_collector_present"),
        "tls_target_applied": identity.get("current_target_applied"),
        "restore_evidence": backup.get("last_successful_restore_evidence"),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
