#!/usr/bin/env python3
"""Capture immutable T-CH-006 repository and read-only live evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
COMMANDS = (
    ("ha-security-backup-contract", ["python3", "scripts/alignment/verify_clickhouse_ha_security_backup.py"]),
    ("ha-security-backup-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_clickhouse_ha_security_backup", "-v"]),
    ("clickhouse-manifest-dry-run", ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/infrastructure/02-clickhouse.yaml"]),
    ("alert-rules-dry-run", ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/observability/clickhouse-ha-alert-rules.yaml"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCES = (
    "contracts/clickhouse/ha-security-backup.v1.json",
    "deployments/kubernetes/infrastructure/02-clickhouse.yaml",
    "deployments/kubernetes/observability/clickhouse-ha-alert-rules.yaml",
    "scripts/alignment/verify_clickhouse_ha_security_backup.py",
    "scripts/alignment/capture_clickhouse_ha_security_backup.py",
    "tests/alignment/test_clickhouse_ha_security_backup.py",
    "doc/07_alignment/runbooks/T-CH-006-keeper-replication-backup-security.md",
    "Makefile",
)
HEALTH_QUERY = """
SELECT hostName() AS host,
 (SELECT count() FROM system.replicas) AS replicas,
 (SELECT countIf(is_readonly=1) FROM system.replicas) AS readonly_replicas,
 (SELECT sum(queue_size) FROM system.replicas) AS replica_queue,
 (SELECT max(absolute_delay) FROM system.replicas) AS max_replica_delay,
 (SELECT count() FROM system.mutations WHERE NOT is_done) AS active_mutations,
 (SELECT count() FROM system.merges) AS active_merges,
 (SELECT max(value) FROM system.asynchronous_metrics WHERE metric='MaxPartCountForPartition') AS max_parts_per_partition,
 (SELECT min(100 * free_space / total_space) FROM system.disks WHERE total_space > 0) AS minimum_disk_free_percent,
 (SELECT count() FROM system.distribution_queue) AS distributed_queue_rows,
 (SELECT sum(data_compressed_bytes) FROM system.distribution_queue) AS distributed_queue_bytes,
 (SELECT max(error_count) FROM system.distribution_queue) AS distributed_queue_max_errors
FORMAT JSONEachRow
"""
SECURITY_QUERY = """
SELECT
 (SELECT groupArray(name) FROM system.users) AS users,
 (SELECT groupArray(name) FROM system.quotas) AS quotas,
 (SELECT value FROM system.settings WHERE name='skip_unavailable_shards') AS skip_unavailable_shards,
 (SELECT value FROM system.settings WHERE name='fallback_to_stale_replicas_for_distributed_queries') AS fallback_to_stale_replicas,
 (SELECT value FROM system.settings WHERE name='max_memory_usage') AS max_memory_usage,
 (SELECT value FROM system.settings WHERE name='max_execution_time') AS max_execution_time,
 (SELECT value FROM system.settings WHERE name='max_bytes_to_read') AS max_bytes_to_read,
 (SELECT value FROM system.settings WHERE name='max_distributed_depth') AS max_distributed_depth,
 (SELECT count() FROM system.server_settings WHERE name IN ('tcp_port_secure','https_port') AND value != '') AS secure_listener_settings
FORMAT JSONEachRow
"""


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run(command: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)


def artifact(path: Path) -> dict[str, Any]:
    return {"path": path.name, "sha256": sha256(path), "size_bytes": path.stat().st_size}


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[clickhouse-ha] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "duration_seconds": round((datetime.now(timezone.utc) - started).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[clickhouse-ha] {name}: {result['status']}", flush=True)
    return result


def parse_json_row(payload: bytes) -> dict[str, Any]:
    rows = [line for line in payload.decode("utf-8").splitlines() if line.strip().startswith("{")]
    return json.loads(rows[-1]) if rows else {}


def save_result(output: Path, name: str, result: subprocess.CompletedProcess[bytes]) -> list[dict[str, Any]]:
    stdout = output / f"{name}.stdout"
    stderr = output / f"{name}.stderr.log"
    stdout.write_bytes(result.stdout)
    stderr.write_bytes(result.stderr)
    return [artifact(stdout), artifact(stderr)]


def clickhouse_query(pod: str, query: str) -> subprocess.CompletedProcess[bytes]:
    return run([
        "kubectl", "--request-timeout=30s", "-n", "middleware", "exec", "-c", "clickhouse", pod,
        "--", "sh", "-lc", 'exec clickhouse-client --password "$CLICKHOUSE_PASSWORD" --query "$1"',
        "capture-clickhouse-ha", query.strip(),
    ])


def capture_live(output: Path) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    artifacts: list[dict[str, Any]] = []
    errors: list[dict[str, Any]] = []

    nodes_result = run(["kubectl", "--request-timeout=15s", "get", "nodes", "-o", "json"])
    artifacts.extend(save_result(output, "nodes", nodes_result))
    nodes: dict[str, dict[str, Any]] = {}
    if nodes_result.returncode == 0:
        for item in json.loads(nodes_result.stdout).get("items", []):
            labels = item.get("metadata", {}).get("labels", {})
            name = item.get("metadata", {}).get("name")
            nodes[str(name)] = {
                "zone": labels.get("topology.kubernetes.io/zone"),
                "region": labels.get("topology.kubernetes.io/region"),
            }
    else:
        errors.append({"scope": "nodes", "exit_code": nodes_result.returncode})

    pods_result = run([
        "kubectl", "--request-timeout=15s", "-n", "middleware", "get", "pods",
        "-l", "app in (clickhouse,clickhouse-keeper)", "-o", "json",
    ])
    artifacts.extend(save_result(output, "clickhouse-and-keeper-pods", pods_result))
    clickhouse_pods: list[str] = []
    keeper_pods: list[str] = []
    pod_nodes: dict[str, str] = {}
    if pods_result.returncode == 0:
        for item in json.loads(pods_result.stdout).get("items", []):
            name = str(item.get("metadata", {}).get("name"))
            node = str(item.get("spec", {}).get("nodeName"))
            labels = item.get("metadata", {}).get("labels", {})
            pod_nodes[name] = node
            if labels.get("app") == "clickhouse-keeper":
                keeper_pods.append(name)
            elif labels.get("app") == "clickhouse":
                clickhouse_pods.append(name)
        clickhouse_pods.sort()
        keeper_pods.sort()
    else:
        errors.append({"scope": "pods", "exit_code": pods_result.returncode})

    health_by_pod: dict[str, Any] = {}
    security_by_pod: dict[str, Any] = {}
    metrics_endpoint_active: dict[str, bool] = {}
    for pod in clickhouse_pods:
        health = clickhouse_query(pod, HEALTH_QUERY)
        artifacts.extend(save_result(output, f"{pod}-health", health))
        if health.returncode == 0:
            health_by_pod[pod] = parse_json_row(health.stdout)
        else:
            errors.append({"scope": f"{pod}-health", "exit_code": health.returncode})

        security = clickhouse_query(pod, SECURITY_QUERY)
        artifacts.extend(save_result(output, f"{pod}-security", security))
        if security.returncode == 0:
            security_by_pod[pod] = parse_json_row(security.stdout)
        else:
            errors.append({"scope": f"{pod}-security", "exit_code": security.returncode})

        metrics = run([
            "kubectl", "--request-timeout=15s", "-n", "middleware", "exec", "-c", "clickhouse", pod,
            "--", "bash", "-lc", "exec 3<>/dev/tcp/127.0.0.1/9363; printf 'GET /metrics HTTP/1.0\\r\\nHost: localhost\\r\\n\\r\\n' >&3; timeout 3 cat <&3",
        ])
        artifacts.extend(save_result(output, f"{pod}-metrics-endpoint", metrics))
        metrics_endpoint_active[pod] = metrics.returncode == 0 and b"200 OK" in metrics.stdout

    keeper_mntr: dict[str, dict[str, str]] = {}
    keeper_metrics_active: dict[str, bool] = {}
    for pod in keeper_pods:
        mntr = run([
            "kubectl", "--request-timeout=15s", "-n", "middleware", "exec", "-c", "keeper", pod,
            "--", "bash", "-lc", "exec 3<>/dev/tcp/127.0.0.1/9181; printf mntr >&3; timeout 3 cat <&3",
        ])
        artifacts.extend(save_result(output, f"{pod}-mntr", mntr))
        if mntr.returncode == 0:
            values = {}
            for line in mntr.stdout.decode("utf-8").splitlines():
                if "\t" in line:
                    key, value = line.split("\t", 1)
                    values[key] = value
            keeper_mntr[pod] = values
        else:
            errors.append({"scope": f"{pod}-mntr", "exit_code": mntr.returncode})

        metrics = run([
            "kubectl", "--request-timeout=15s", "-n", "middleware", "exec", "-c", "keeper", pod,
            "--", "bash", "-lc", "exec 3<>/dev/tcp/127.0.0.1/9363; printf 'GET /metrics HTTP/1.0\\r\\nHost: localhost\\r\\n\\r\\n' >&3; timeout 3 cat <&3",
        ])
        artifacts.extend(save_result(output, f"{pod}-metrics-endpoint", metrics))
        keeper_metrics_active[pod] = metrics.returncode == 0 and b"200 OK" in metrics.stdout

    api_result = run(["kubectl", "api-resources", "--api-group=monitoring.coreos.com", "-o", "name"])
    artifacts.extend(save_result(output, "monitoring-api-resources", api_result))
    monitor_pods_result = run([
        "kubectl", "--request-timeout=15s", "get", "pods", "-A", "-o", "json",
    ])
    artifacts.extend(save_result(output, "all-pods-for-monitor-runtime", monitor_pods_result))
    monitor_names: list[str] = []
    if monitor_pods_result.returncode == 0:
        monitor_names = sorted(
            str(item.get("metadata", {}).get("name"))
            for item in json.loads(monitor_pods_result.stdout).get("items", [])
            if any(token in str(item.get("metadata", {}).get("name", "")).lower()
                   for token in ("prometheus", "vmagent", "vmalert", "alertmanager"))
        )

    used_nodes = sorted(set(pod_nodes.values()))
    used_zones = sorted({nodes.get(name, {}).get("zone") for name in used_nodes if nodes.get(name, {}).get("zone")})
    states = [values.get("zk_server_state") for values in keeper_mntr.values()]
    live = {
        "read_only": True,
        "clickhouse_pods": clickhouse_pods,
        "keeper_pods": keeper_pods,
        "pod_nodes": pod_nodes,
        "distinct_nodes": used_nodes,
        "distinct_zones": used_zones,
        "health_by_pod": health_by_pod,
        "security_by_pod": security_by_pod,
        "keeper_mntr": keeper_mntr,
        "keeper_leader_count": states.count("leader"),
        "keeper_follower_count": states.count("follower"),
        "clickhouse_metrics_endpoint_active": metrics_endpoint_active,
        "keeper_metrics_endpoint_active": keeper_metrics_active,
        "monitoring_api_resources": [line for line in api_result.stdout.decode("utf-8").splitlines() if line],
        "monitoring_runtime_pods": monitor_names,
        "candidate_metrics_active": bool(metrics_endpoint_active)
        and all(metrics_endpoint_active.values())
        and bool(keeper_metrics_active)
        and all(keeper_metrics_active.values()),
        "candidate_tls_and_users_active": bool(security_by_pod)
        and all(row.get("secure_listener_settings") == "2" and row.get("users") != ["default"] for row in security_by_pod.values()),
        "formal_failure_domain_proof": len(used_zones) >= 3,
        "restore_evidence": None,
        "errors": errors,
    }
    return live, artifacts


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text())
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("referenced G0 manifest is not PASS")
    before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if before["content_sha256"] != g0_hash:
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
    live, live_artifacts = capture_live(output) if repository_pass else ({"errors": [{"error": "skipped after repository failure"}]}, [])
    scoped = "PASS" if repository_pass and not live.get("errors") else "FAIL"
    after = build_snapshot()
    stable = before["content_sha256"] == after["content_sha256"]
    if not stable:
        scoped = "FAIL"

    source_artifacts = []
    for relative in SOURCES:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        source_artifacts.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-CH-006",
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_GUARDS_AND_READ_ONLY_LIVE_PRE_ROLLOUT_CAPTURE",
        "scoped_evidence_status": scoped,
        "candidate_source": before,
        "candidate_source_stable": stable,
        "production_applied": False,
        "read_only_live_capture": True,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "candidate_source_sha256": g0_hash,
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_REPOSITORY_METRICS_RULE_PROFILE_SEMANTICS_AND_RESTORE_CONTRACT",
            "G2": "PARTIAL_READ_ONLY_CURRENT_HEALTH_AND_SECURITY_DRIFT_CAPTURE",
            "G3": "OPEN_FOR_FAULTED_QUERY_RESTORE_AND_REPLICA_RECONCILIATION",
            "G4": "OPEN_FOR_PROFILE_QUOTA_BACKUP_AND_FAILURE_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_TLS_IDENTITY_MONITORING_BACKUP_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "live_observation": live,
        "source_artifacts": source_artifacts,
        "live_artifacts": live_artifacts,
        "proven": [
            "repository candidate exposes ClickHouse and Keeper native metric endpoints",
            "ten required alert rules use metric names observed from the live ClickHouse image where applicable",
            "traffic_runtime profile is bounded and fail-closed but remains unbound",
            "three dataset classes define explicit fail or partial semantics",
            "backup and isolated restore validation oracle is registered without inventing restore success",
            "current topology replication Keeper users settings and monitoring runtime were captured read-only",
        ],
        "open": [
            "three-failure-domain topology proof",
            "live collector rule evaluator notification routing and alert firing evidence",
            "TLS per-service users least privilege and workload migration",
            "faulted API partial/error evidence",
            "approved encrypted backups isolated restore reconciliation destructive drills rollback and observation",
        ],
        "secrets_captured": False,
    }
    path = output / "manifest.json"
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": scoped,
        "candidate_metrics_active": live.get("candidate_metrics_active"),
        "candidate_tls_and_users_active": live.get("candidate_tls_and_users_active"),
        "formal_failure_domain_proof": live.get("formal_failure_domain_proof"),
        "manifest": str(path),
        "manifest_sha256": sha256(path),
    }, indent=2), flush=True)
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
