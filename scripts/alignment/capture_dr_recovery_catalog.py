#!/usr/bin/env python3
"""Capture immutable repository and read-only live evidence for T-DR-001."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
CATALOG = ROOT / "contracts/reliability/dr-recovery-catalog.v1.json"
COMMANDS = (
    ("dr-catalog-current", ["python3", "scripts/alignment/build_dr_recovery_catalog.py", "--check"]),
    ("dr-catalog-verifier", ["python3", "scripts/alignment/verify_dr_recovery_catalog.py"]),
    ("dr-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_dr_recovery_catalog", "-v"]),
    ("postgres-dr-contract", ["python3", "scripts/alignment/verify_pg_ha_pitr.py"]),
    ("clickhouse-dr-contract", ["python3", "scripts/alignment/verify_clickhouse_ha_security_backup.py"]),
    ("opensearch-dr-contract", ["python3", "scripts/alignment/verify_opensearch_ha_security_restore.py"]),
    ("flink-dr-contract", ["python3", "scripts/alignment/verify_flink_checkpoint_ha.py"]),
    ("redis-dr-contract", ["python3", "scripts/alignment/verify_redis_reliability_domains.py"]),
)
SOURCE_ARTIFACTS = (
    "contracts/reliability/dr-recovery-catalog.v1.json",
    "scripts/alignment/build_dr_recovery_catalog.py",
    "scripts/alignment/verify_dr_recovery_catalog.py",
    "scripts/alignment/capture_dr_recovery_catalog.py",
    "tests/alignment/test_dr_recovery_catalog.py",
    "contracts/postgres/ha-pitr-fencing.v1.json",
    "contracts/clickhouse/ha-security-backup.v1.json",
    "contracts/opensearch/ha-security-restore.v1.json",
    "contracts/flink/checkpoint-ha-upgrade.v1.json",
    "contracts/redis/reliability-domains.v1.json",
    "deployments/kubernetes/infrastructure/01-kafka.yaml",
    "deployments/kubernetes/infrastructure/02-clickhouse.yaml",
    "deployments/kubernetes/infrastructure/03-postgresql.yaml",
    "deployments/kubernetes/infrastructure/04-redis.yaml",
    "deployments/kubernetes/infrastructure/05-opensearch.yaml",
    "deployments/kubernetes/infrastructure/06-minio.yaml",
    "deployments/kubernetes/infrastructure/07-flink.yaml",
    "deployments/kubernetes/infrastructure/09-nebula-graph.yaml",
    "doc/07_alignment/runbooks/T-DR-001-cross-store-recovery.md",
    "Makefile",
)
DOMAIN_RESOURCES = {
    "postgresql_authority": [
        ("databases", "StatefulSet", "postgres-primary"),
        ("databases", "StatefulSet", "postgres-replica"),
    ],
    "kafka_event_log": [("middleware", "StatefulSet", "kafka")],
    "clickhouse_facts": [
        ("middleware", "StatefulSet", "clickhouse-keeper"),
        ("middleware", "StatefulSet", "clickhouse-1"),
        ("middleware", "StatefulSet", "clickhouse-2"),
        ("middleware", "StatefulSet", "clickhouse-replica"),
    ],
    "minio_objects": [("minio", "StatefulSet", "minio")],
    "nebula_projection": [
        ("middleware", "StatefulSet", "nebula-meta"),
        ("middleware", "StatefulSet", "nebula-storage"),
        ("middleware", "Deployment", "nebula-graph"),
    ],
    "opensearch_projection": [("middleware", "StatefulSet", "opensearch")],
    "redis_runtime_state": [
        ("databases", "StatefulSet", "redis-master"),
        ("databases", "StatefulSet", "redis-replica"),
        ("databases", "StatefulSet", "redis-cache"),
        ("databases", "StatefulSet", "redis-sentinel"),
    ],
    "flink_state": [
        ("flink", "StatefulSet", "flink-jobmanager"),
        ("flink", "StatefulSet", "flink-taskmanager"),
    ],
}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def direct_environment() -> dict[str, str]:
    environment = dict(os.environ)
    for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
        environment.pop(key, None)
    return environment


def run(command: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command,
        cwd=ROOT,
        env=direct_environment(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
        timeout=30,
    )


def artifact(path: Path) -> dict[str, Any]:
    return {"path": path.name, "sha256": sha256(path), "size_bytes": path.stat().st_size}


def save_result(output: Path, name: str, result: subprocess.CompletedProcess[bytes]) -> list[dict[str, Any]]:
    stdout = output / f"{name}.stdout.json"
    stderr = output / f"{name}.stderr.log"
    stdout.write_bytes(result.stdout)
    stderr.write_bytes(result.stderr)
    return [artifact(stdout), artifact(stderr)]


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[dr] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=direct_environment(),
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
        )
    finished = datetime.now(timezone.utc)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "started_at": started.isoformat(),
        "finished_at": finished.isoformat(),
        "duration_seconds": round((finished - started).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[dr] {name}: {result['status']}", flush=True)
    return result


def _ready_replicas(resource: dict[str, Any]) -> int:
    return int((resource.get("status") or {}).get("readyReplicas") or 0)


def _desired_replicas(resource: dict[str, Any]) -> int:
    return int((resource.get("spec") or {}).get("replicas") or 0)


def capture_live(output: Path) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    artifacts: list[dict[str, Any]] = []
    errors: list[dict[str, Any]] = []
    resources: dict[tuple[str, str, str], dict[str, Any]] = {}
    for namespace in sorted({item[0] for values in DOMAIN_RESOURCES.values() for item in values}):
        for kind, cli_kind in (("StatefulSet", "statefulsets"), ("Deployment", "deployments")):
            result = run(["kubectl", "--request-timeout=15s", "-n", namespace, "get", cli_kind, "-o", "json"])
            artifacts.extend(save_result(output, f"{namespace}-{cli_kind}", result))
            if result.returncode != 0:
                errors.append({"scope": f"{namespace}/{cli_kind}", "exit_code": result.returncode})
                continue
            for item in json.loads(result.stdout).get("items", []):
                name = str((item.get("metadata") or {}).get("name") or "")
                resources[(namespace, kind, name)] = item

    nodes_result = run(["kubectl", "--request-timeout=15s", "get", "nodes", "-o", "json"])
    artifacts.extend(save_result(output, "nodes", nodes_result))
    nodes: dict[str, dict[str, Any]] = {}
    if nodes_result.returncode == 0:
        for item in json.loads(nodes_result.stdout).get("items", []):
            metadata = item.get("metadata") or {}
            labels = metadata.get("labels") or {}
            nodes[str(metadata.get("name") or "")] = {
                "zone": labels.get("topology.kubernetes.io/zone"),
                "region": labels.get("topology.kubernetes.io/region"),
            }
    else:
        errors.append({"scope": "nodes", "exit_code": nodes_result.returncode})

    pod_result = run(["kubectl", "--request-timeout=15s", "get", "pods", "-A", "-o", "json"])
    artifacts.extend(save_result(output, "pods-all-namespaces", pod_result))
    pod_rows: list[dict[str, Any]] = []
    if pod_result.returncode == 0:
        for item in json.loads(pod_result.stdout).get("items", []):
            metadata = item.get("metadata") or {}
            status = item.get("status") or {}
            spec = item.get("spec") or {}
            owner_refs = metadata.get("ownerReferences") or []
            pod_rows.append(
                {
                    "namespace": metadata.get("namespace"),
                    "name": metadata.get("name"),
                    "owner_names": [owner.get("name") for owner in owner_refs],
                    "phase": status.get("phase"),
                    "node": spec.get("nodeName"),
                    "ready": all(
                        condition.get("status") == "True"
                        for condition in status.get("conditions") or []
                        if condition.get("type") == "Ready"
                    ),
                }
            )
    else:
        errors.append({"scope": "pods", "exit_code": pod_result.returncode})

    pvc_result = run(["kubectl", "--request-timeout=15s", "get", "pvc", "-A", "-o", "json"])
    artifacts.extend(save_result(output, "pvc-all-namespaces", pvc_result))
    pvc_summary: list[dict[str, Any]] = []
    if pvc_result.returncode == 0:
        for item in json.loads(pvc_result.stdout).get("items", []):
            metadata = item.get("metadata") or {}
            spec = item.get("spec") or {}
            status = item.get("status") or {}
            pvc_summary.append(
                {
                    "namespace": metadata.get("namespace"),
                    "name": metadata.get("name"),
                    "phase": status.get("phase"),
                    "storage_class": spec.get("storageClassName"),
                    "requested_storage": ((spec.get("resources") or {}).get("requests") or {}).get("storage"),
                    "volume_name": spec.get("volumeName"),
                }
            )
    else:
        errors.append({"scope": "pvc", "exit_code": pvc_result.returncode})

    domains: list[dict[str, Any]] = []
    for domain_id, expected_resources in DOMAIN_RESOURCES.items():
        observations = []
        used_nodes: set[str] = set()
        for key in expected_resources:
            resource = resources.get(key)
            namespace, kind, name = key
            matching_pods = [
                pod for pod in pod_rows
                if pod["namespace"] == namespace
                and any(owner == name or str(owner).startswith(f"{name}-") for owner in pod["owner_names"])
            ]
            used_nodes.update(str(pod["node"]) for pod in matching_pods if pod.get("node"))
            observations.append(
                {
                    "namespace": namespace,
                    "kind": kind,
                    "name": name,
                    "found": resource is not None,
                    "resource_version": (resource or {}).get("metadata", {}).get("resourceVersion"),
                    "desired_replicas": _desired_replicas(resource or {}),
                    "ready_replicas": _ready_replicas(resource or {}),
                    "pod_count": len(matching_pods),
                    "ready_pod_count": sum(bool(pod["ready"]) for pod in matching_pods),
                }
            )
        domains.append(
            {
                "domain_id": domain_id,
                "resources": observations,
                "all_expected_resources_found": all(item["found"] for item in observations),
                "nodes": sorted(used_nodes),
                "zones": sorted({str(nodes[node]["zone"]) for node in used_nodes if nodes.get(node, {}).get("zone")}),
                "restore_executed": False,
                "failover_executed": False,
                "rpo_rto_proven": False,
            }
        )
    return (
        {
            "read_only": True,
            "query_status": "PASS" if not errors else "PARTIAL",
            "node_count": len(nodes),
            "labeled_zone_count": len({item["zone"] for item in nodes.values() if item.get("zone")}),
            "domains": domains,
            "domains_observed": len(domains),
            "domains_with_all_expected_resources": sum(item["all_expected_resources_found"] for item in domains),
            "pvc_metadata": sorted(pvc_summary, key=lambda item: (str(item["namespace"]), str(item["name"]))),
            "errors": errors,
            "secret_values_captured": False,
            "database_rows_captured": False,
            "restore_executed": False,
            "failover_executed": False,
            "production_mutations": [],
        },
        artifacts,
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("referenced G0 manifest is not PASS")
    candidate_before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if not g0_hash or candidate_before["content_sha256"] != g0_hash:
        raise SystemExit("current candidate does not match the referenced G0 manifest")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    results = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break
    repository_pass = len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results)
    live, live_artifacts = capture_live(output)

    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
    scoped_pass = (
        repository_pass
        and live.get("domains_observed") == len(catalog.get("domains") or [])
        and candidate_stable
    )
    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-DR-001",
        "related_ids": ["T-PG-006", "T-CH-006", "T-OS-005", "T-FLINK-003", "T-REDIS-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_CROSS_STORE_RECOVERY_AUTHORITY_AND_READ_ONLY_LIVE_TOPOLOGY",
        "scoped_evidence_status": "PASS" if scoped_pass else "FAIL",
        "candidate_source": candidate_before,
        "candidate_source_stable": candidate_stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
            "candidate_source_sha256": g0_hash,
        },
        "catalog_summary": {
            "catalog_sha256": catalog.get("catalog_sha256"),
            "counts": catalog.get("counts"),
            "dr_readiness": "PARTIAL",
        },
        "live_observation": live,
        "live_artifacts": live_artifacts,
        "production_applied": False,
        "destructive_execution_authorized": False,
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_VERSIONED_EIGHT_DOMAIN_RECOVERY_CATALOG_AND_NEGATIVE_GUARDS" if scoped_pass else "FAIL",
            "G2": "PARTIAL_FOR_READ_ONLY_LIVE_RESOURCE_POD_NODE_ZONE_AND_PVC_METADATA",
            "G3": "OPEN_FOR_ISOLATED_RESTORE_AND_CROSS_STORE_BUSINESS_RECONCILIATION",
            "G4": "OPEN_FOR_APPROVED_RPO_RTO_CAPACITY_AND_FAILURE_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_RECOVERED_BUSINESS_JOURNEYS",
            "G6": "HOLD_FOR_APPROVED_DESTRUCTIVE_FAILOVER_RESTORE_ROLLBACK_WINDOW",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "all eight recovery domains are inventoried with immutable authority hashes",
            "backup success cannot substitute for isolated restore and business-oracle evidence",
            "cross-store recovery order preserves PostgreSQL authority and projection rebuild semantics",
            "live topology collection reads only resource pod node zone and PVC metadata",
            "repository changes do not authorize failover restore cutover deletion or object overwrite",
        ],
        "open": list((catalog.get("acceptance") or {}).get("remaining") or []),
        "secrets_captured": False,
        "database_rows_captured": False,
        "production_mutations": [],
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "scoped_evidence_status": manifest["scoped_evidence_status"],
                "manifest": str(manifest_path.relative_to(ROOT)),
                "manifest_sha256": sha256(manifest_path),
                "catalog_sha256": catalog.get("catalog_sha256"),
                "live_query_status": live.get("query_status"),
                "domains_with_all_expected_resources": live.get("domains_with_all_expected_resources"),
                "restore_executed": False,
                "production_mutations": [],
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
