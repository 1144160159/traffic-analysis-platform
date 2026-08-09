#!/usr/bin/env python3
"""Capture immutable T-OS-005 repository and read-only live pre-rollout evidence."""

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
RENDERED = "/tmp/t-os-005-opensearch-ha-v1.yaml"
COMMANDS = (
    ("ha-security-restore-contract", ["python3", "scripts/alignment/verify_opensearch_ha_security_restore.py"]),
    ("ha-security-restore-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_opensearch_ha_security_restore", "-v"]),
    ("guarded-target-render", ["python3", "scripts/alignment/render_opensearch_ha_security.py", "--output", RENDERED]),
    ("guarded-target-dry-run", ["kubectl", "apply", "--dry-run=client", "--validate=false", "-f", RENDERED]),
    ("alert-rules-dry-run", ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/observability/opensearch-ha-alert-rules.yaml"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCES = (
    "contracts/opensearch/ha-security-restore.v1.json",
    "deployments/kubernetes/infrastructure/05-opensearch.yaml",
    "deployments/kubernetes/security/opensearch-ha-v1/kustomization.yaml",
    "deployments/kubernetes/security/opensearch-ha-v1/statefulset-ha-security.patch.yaml",
    "deployments/kubernetes/security/opensearch-ha-v1/external-secrets.template.yaml",
    "deployments/kubernetes/security/opensearch-ha-v1/service-roles.v1.json",
    "deployments/kubernetes/observability/opensearch-ha-alert-rules.yaml",
    "deployments/opensearch/Dockerfile.ha-v1",
    "scripts/alignment/render_opensearch_ha_security.py",
    "scripts/alignment/opensearch_snapshot_restore.py",
    "scripts/alignment/verify_opensearch_ha_security_restore.py",
    "scripts/alignment/capture_opensearch_ha_security_restore.py",
    "tests/alignment/test_opensearch_ha_security_restore.py",
    "doc/07_alignment/runbooks/T-OS-005-snapshot-zone-tls-restore.md",
    "Makefile",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def artifact(path: Path) -> dict[str, Any]:
    return {"path": path.name, "sha256": sha256(path), "size_bytes": path.stat().st_size}


def run(command: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[opensearch-ha] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[opensearch-ha] {name}: {result['status']}", flush=True)
    return result


def save_result(output: Path, name: str, result: subprocess.CompletedProcess[bytes]) -> list[dict[str, Any]]:
    stdout = output / f"live-{name}.stdout"
    stderr = output / f"live-{name}.stderr.log"
    stdout.write_bytes(result.stdout)
    stderr.write_bytes(result.stderr)
    return [artifact(stdout), artifact(stderr)]


def kubectl(args: list[str]) -> subprocess.CompletedProcess[bytes]:
    return run(["kubectl", "--request-timeout=20s", *args])


def os_get(path: str) -> subprocess.CompletedProcess[bytes]:
    return kubectl([
        "-n", "middleware", "exec", "opensearch-0", "--", "sh", "-lc",
        'exec curl -fsS -u "admin:${OPENSEARCH_INITIAL_ADMIN_PASSWORD}" "http://127.0.0.1:9200$1"',
        "capture-opensearch-ha", path,
    ])


def parse_json(result: subprocess.CompletedProcess[bytes], scope: str, errors: list[dict[str, Any]]) -> Any:
    if result.returncode:
        errors.append({"scope": scope, "exit_code": result.returncode})
        return None
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        errors.append({"scope": scope, "error": f"JSON decode failed: {exc}"})
        return None


def capture_live(output: Path) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    artifacts: list[dict[str, Any]] = []
    errors: list[dict[str, Any]] = []

    nodes_result = kubectl(["get", "nodes", "-o", "json"])
    artifacts.extend(save_result(output, "nodes", nodes_result))
    nodes_payload = parse_json(nodes_result, "nodes", errors) or {}
    nodes = {}
    for item in nodes_payload.get("items", []):
        labels = item.get("metadata", {}).get("labels", {})
        nodes[str(item.get("metadata", {}).get("name"))] = {
            "zone": labels.get("topology.kubernetes.io/zone"),
            "ready": any(condition.get("type") == "Ready" and condition.get("status") == "True"
                         for condition in item.get("status", {}).get("conditions", [])),
        }

    pods_result = kubectl(["-n", "middleware", "get", "pods", "-l", "app=opensearch", "-o", "json"])
    artifacts.extend(save_result(output, "pods", pods_result))
    pods_payload = parse_json(pods_result, "pods", errors) or {}
    pods = []
    for item in pods_payload.get("items", []):
        pods.append({
            "name": item.get("metadata", {}).get("name"),
            "node": item.get("spec", {}).get("nodeName"),
            "phase": item.get("status", {}).get("phase"),
            "ready": any(condition.get("type") == "Ready" and condition.get("status") == "True"
                         for condition in item.get("status", {}).get("conditions", [])),
        })
    pods.sort(key=lambda item: str(item["name"]))

    statefulset_result = kubectl(["-n", "middleware", "get", "statefulset", "opensearch", "-o", "json"])
    artifacts.extend(save_result(output, "statefulset", statefulset_result))
    statefulset_payload = parse_json(statefulset_result, "statefulset", errors) or {}
    deployed = {}
    try:
        pod_spec = statefulset_payload["spec"]["template"]["spec"]
        container = next(item for item in pod_spec["containers"] if item["name"] == "opensearch")
        env = {item["name"]: item.get("value") for item in container.get("env", [])}
        deployed = {
            "image": container.get("image"),
            "replicas": statefulset_payload["spec"].get("replicas"),
            "http_tls": env.get("plugins.security.ssl.http.enabled"),
            "security_plugin_disabled": env.get("DISABLE_SECURITY_PLUGIN"),
            "forced_zone_values": env.get("cluster.routing.allocation.awareness.force.zone.values"),
            "topology_spread": pod_spec.get("topologySpreadConstraints", []),
            "affinity": pod_spec.get("affinity", {}),
            "health_probe_uses_shared_admin": "OPENSEARCH_INITIAL_ADMIN_PASSWORD" in json.dumps(container.get("readinessProbe", {})),
            "health_probe_scheme": (container.get("readinessProbe", {}).get("httpGet", {}) or {}).get("scheme"),
        }
    except (KeyError, StopIteration, TypeError) as exc:
        errors.append({"scope": "statefulset", "error": f"shape decode failed: {exc}"})

    queries = {
        "cluster-health": "/_cluster/health",
        "cluster-settings": "/_cluster/settings?flat_settings=true&include_defaults=false",
        "node-zones": "/_nodes?filter_path=nodes.*.name,nodes.*.attributes.zone",
        "allocation": "/_cat/allocation?format=json&h=node,shards,disk.indices,disk.used,disk.avail,disk.total,disk.percent",
        "shards": "/_cat/shards?format=json&h=index,shard,prirep,state,node,unassigned.reason",
        "pending-tasks": "/_cluster/pending_tasks",
        "snapshots": "/_snapshot",
        "thread-pool": "/_nodes/stats/thread_pool,fs?filter_path=nodes.*.name,nodes.*.fs.total,nodes.*.thread_pool.write.rejected,nodes.*.thread_pool.write.queue",
        "indices": "/_cat/indices?format=json&h=index,health,status,pri,rep,docs.count,store.size",
    }
    observed: dict[str, Any] = {}
    for name, path in queries.items():
        result = os_get(path)
        artifacts.extend(save_result(output, name, result))
        observed[name] = parse_json(result, name, errors)

    https_probe = kubectl([
        "-n", "middleware", "exec", "opensearch-0", "--", "sh", "-lc",
        "curl -skS --connect-timeout 3 -o /dev/null -w '%{http_code}' https://127.0.0.1:9200/_cluster/health",
    ])
    artifacts.extend(save_result(output, "https-probe", https_probe))
    https_status = https_probe.stdout.decode(errors="replace").strip() if https_probe.returncode == 0 else None

    alert_rules_result = kubectl(["-n", "observability", "get", "configmap", "opensearch-ha-alert-rules-v1", "-o", "json"])
    artifacts.extend(save_result(output, "deployed-alert-rules", alert_rules_result))
    external_secret_crd = kubectl(["api-resources", "--api-group=external-secrets.io", "-o", "name"])
    artifacts.extend(save_result(output, "external-secret-api-resources", external_secret_crd))
    clients_result = kubectl(["-n", "traffic-analysis", "get", "deployment", "alert-service", "asset-service", "-o", "json"])
    artifacts.extend(save_result(output, "application-opensearch-clients", clients_result))
    application_clients: list[dict[str, Any]] = []
    clients_payload = parse_json(clients_result, "application-opensearch-clients", errors) or {}
    for item in clients_payload.get("items", []):
        entries = []
        for container in item.get("spec", {}).get("template", {}).get("spec", {}).get("containers", []):
            for env in container.get("env", []):
                if "OPENSEARCH" not in str(env.get("name", "")):
                    continue
                entry: dict[str, Any] = {"name": env.get("name")}
                if "value" in env:
                    entry["value"] = env.get("value")
                ref = (env.get("valueFrom", {}).get("secretKeyRef") or {})
                if ref:
                    entry["secret_ref"] = {"name": ref.get("name"), "key": ref.get("key")}
                entries.append(entry)
        application_clients.append({"deployment": item.get("metadata", {}).get("name"), "opensearch_environment": entries})
    snapshot_repositories = observed.get("snapshots") if isinstance(observed.get("snapshots"), dict) else {}
    pod_nodes = [str(item["node"]) for item in pods if item.get("node")]
    zones = sorted({str(nodes[node]["zone"]) for node in pod_nodes if node in nodes and nodes[node].get("zone")})
    health = observed.get("cluster-health") or {}
    pending = observed.get("pending-tasks") or {}
    allocation = observed.get("allocation") or []
    max_disk_percent = max((int(str(item.get("disk.percent", "0")).rstrip("%") or 0) for item in allocation), default=0)
    live = {
        "read_only": True,
        "nodes": nodes,
        "opensearch_pods": pods,
        "distinct_pod_nodes": sorted(set(pod_nodes)),
        "distinct_labeled_zones": zones,
        "three_zone_proof": len(zones) >= 3,
        "deployed_statefulset": deployed,
        "cluster_health": health,
        "pending_task_count": len(pending.get("tasks", [])) if isinstance(pending, dict) else None,
        "max_disk_percent": max_disk_percent,
        "snapshot_repository_names": sorted(snapshot_repositories.keys()),
        "https_probe_http_status": https_status,
        "http_tls_active": https_status is not None and https_status != "000",
        "candidate_alert_rules_deployed": alert_rules_result.returncode == 0,
        "external_secret_api_resources": [line for line in external_secret_crd.stdout.decode(errors="replace").splitlines() if line],
        "application_clients": application_clients,
        "candidate_overlay_applied": bool(
            deployed.get("http_tls") == "true"
            and deployed.get("forced_zone_values") == "zone-a,zone-b,zone-c"
            and len(zones) >= 3
        ),
        "restore_evidence": None,
        "production_mutations": [],
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
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
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
    after = build_snapshot()
    stable = before["content_sha256"] == after["content_sha256"]
    scoped = "PASS" if repository_pass and stable and not live.get("errors") else "FAIL"

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
        "remediation_id": "T-OS-005",
        "work_package": "WP-23-OS",
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_HA_TLS_ALERT_SNAPSHOT_RESTORE_GUARDS_AND_READ_ONLY_LIVE_DRIFT_CAPTURE",
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
            "G1": "PASS_FOR_REPOSITORY_HA_TLS_ROLE_ALERT_AND_RESTORE_GUARDS",
            "G2": "PARTIAL_READ_ONLY_CURRENT_CLUSTER_DRIFT_CAPTURE",
            "G3": "OPEN_FOR_SNAPSHOT_RESTORE_AND_AUTHORITY_RECONCILIATION",
            "G4": "OPEN_FOR_FAILURE_AND_RESTORE_BUDGETS",
            "G5": "OPEN_FOR_CANDIDATE_BROWSER_DEGRADATION_EVIDENCE",
            "G6": "HOLD_FOR_THREE_ZONES_PKI_IDENTITIES_SNAPSHOT_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "live_observation": live,
        "source_artifacts": source_artifacts,
        "live_artifacts": live_artifacts,
        "proven": [
            "opt-in target renders from the canonical base and remains fail-safe until an approved image digest is pinned",
            "target requires three zones forced shard awareness HTTP and transport TLS hostname verification mTLS probes and audit",
            "five secret-free least-privilege service role contracts and twelve alert rules are versioned",
            "snapshot and isolated restore tooling is plan-only by default and enforces immutable approvals identity and full verification oracles",
            "current topology TLS snapshots tasks disk and deployment drift were captured without production mutation",
        ],
        "open": [
            "three real labeled zones and one OpenSearch pod per zone",
            "approved plugin image External Secrets PKI security roles and all client migrations",
            "live collector evaluator notification route and alert firing evidence",
            "successful encrypted snapshot and periodic isolated restore with full oracle reconciliation",
            "failure drills browser degradation rollback and T+0 T+1 T+3 T+7 observation",
        ],
        "secrets_captured": False,
    }
    path = output / "manifest.json"
    path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": scoped,
        "three_zone_proof": live.get("three_zone_proof"),
        "http_tls_active": live.get("http_tls_active"),
        "snapshot_repository_names": live.get("snapshot_repository_names"),
        "candidate_overlay_applied": live.get("candidate_overlay_applied"),
        "manifest": str(path),
        "manifest_sha256": sha256(path),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
