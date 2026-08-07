#!/usr/bin/env python3
"""Capture immutable T-OS-002 repository and read-only live evidence."""

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
    ("index-governance-contract", ["python3", "scripts/alignment/verify_opensearch_index_governance.py"]),
    ("index-governance-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_opensearch_index_governance", "-v"]),
    ("go-alert-opensearch-tests", ["bash", "-lc", "cd go/control-plane && go test ./internal/alert/config ./internal/alert/persistence ./internal/alert/repository ./cmd/alert-service -count=1"]),
    ("flink-alert-generator-tests", ["mvn", "-f", "java/flink-jobs/pom.xml", "-pl", "flink-alert-generator-job", "-am", "-DskipITs", "-Dcheckstyle.skip=true", "-Dspotbugs.skip=true", "test"]),
    ("opensearch-routine-init-dry-run", ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/init-jobs/04-opensearch-templates.yaml", "-o", "name"]),
    ("opensearch-alerts-v2-expand-dry-run", ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/migrations/opensearch/T-OS-002-alerts-v2-expand.yaml", "-o", "name"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCES = (
    "contracts/opensearch/index-governance.v1.json",
    "common/opensearch/index-templates.json",
    "common/opensearch/alerts-v2/mappings-component.json",
    "common/opensearch/alerts-v2/settings-component.json",
    "common/opensearch/alerts-v2/index-template.json",
    "common/opensearch/alerts-v2/ism-policy.json",
    "common/opensearch/alerts-v2/bootstrap-index.json",
    "deployments/kubernetes/init-jobs/04-opensearch-templates.yaml",
    "deployments/kubernetes/migrations/opensearch/T-OS-002-alerts-v2-expand.yaml",
    "scripts/alignment/render_opensearch_alerts_v2_expand.py",
    "scripts/alignment/plan_opensearch_alerts_v2_backfill.py",
    "scripts/alignment/render_opensearch_alerts_v2_backfill.py",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/internal/alert/persistence/opensearch.go",
    "go/control-plane/internal/alert/repository/opensearch.go",
    "java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/AlertGeneratorJob.java",
    "java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/sink/OpenSearchAlertSinkFactory.java",
    "scripts/alignment/verify_opensearch_index_governance.py",
    "scripts/alignment/capture_opensearch_index_governance.py",
    "tests/alignment/test_opensearch_index_governance.py",
    "doc/07_alignment/runbooks/T-OS-002-versioned-index-alias-migration.md",
    "Makefile",
)
LIVE_ENDPOINTS = {
    "cluster-health": "/_cluster/health",
    "alerts-index": "/_cat/indices/alerts?format=json&bytes=b",
    "alerts-settings": "/alerts/_settings",
    "alerts-mapping": "/alerts/_mapping",
    "alerts-field-caps": "/alerts/_field_caps?fields=tenant_id,alert_id,src_ip,dst_ip,severity,status",
    "alerts-shards": "/_cat/shards/alerts?format=json&bytes=b",
    "aliases": "/_alias",
    "component-templates": "/_component_template",
    "index-templates": "/_index_template",
    "ism-policies": "/_plugins/_ism/policies",
    "ism-alerts-explain": "/_plugins/_ism/explain/alerts",
}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def artifact(path: Path) -> dict[str, Any]:
    return {"path": path.name, "sha256": sha256(path), "size_bytes": path.stat().st_size}


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[opensearch-governance] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[opensearch-governance] {name}: {result['status']}", flush=True)
    return result


def live_get(path: str) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run([
        "kubectl", "--request-timeout=20s", "-n", "middleware", "exec", "opensearch-0", "--",
        "curl", "-fsS", f"http://127.0.0.1:9200{path}",
    ], cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)


def capture_live(output: Path) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    artifacts: list[dict[str, Any]] = []
    errors: list[dict[str, Any]] = []
    decoded: dict[str, Any] = {}
    pods = subprocess.run([
        "kubectl", "--request-timeout=15s", "-n", "middleware", "get", "pods",
        "-l", "app=opensearch", "-o", "json",
    ], cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    for suffix, payload in (("stdout", pods.stdout), ("stderr.log", pods.stderr)):
        path = output / f"opensearch-pods.{suffix}"
        path.write_bytes(payload)
        artifacts.append(artifact(path))
    if pods.returncode != 0:
        errors.append({"scope": "pods", "exit_code": pods.returncode})

    for name, endpoint in LIVE_ENDPOINTS.items():
        result = live_get(endpoint)
        stdout = output / f"live-{name}.json"
        stderr = output / f"live-{name}.stderr.log"
        stdout.write_bytes(result.stdout)
        stderr.write_bytes(result.stderr)
        artifacts.extend((artifact(stdout), artifact(stderr)))
        if result.returncode != 0:
            errors.append({"scope": name, "exit_code": result.returncode})
            continue
        try:
            decoded[name] = json.loads(result.stdout)
        except json.JSONDecodeError as exc:
            errors.append({"scope": name, "error": f"invalid JSON: {exc}"})

    index_rows = decoded.get("alerts-index", [])
    index = index_rows[0] if index_rows else {}
    mapping = decoded.get("alerts-mapping", {}).get("alerts", {}).get("mappings", {}).get("properties", {})
    aliases = decoded.get("aliases", {}).get("alerts", {}).get("aliases", {})
    components = decoded.get("component-templates", {}).get("component_templates", [])
    policies = decoded.get("ism-policies", {}).get("policies", [])
    live = {
        "read_only": True,
        "cluster_status": decoded.get("cluster-health", {}).get("status"),
        "number_of_nodes": decoded.get("cluster-health", {}).get("number_of_nodes"),
        "unassigned_shards": decoded.get("cluster-health", {}).get("unassigned_shards"),
        "alerts_documents": int(index.get("docs.count", 0)) if index else None,
        "alerts_store_bytes": int(index.get("store.size", 0)) if index else None,
        "alerts_primary_shards": int(index.get("pri", 0)) if index else None,
        "alerts_replica_shards": int(index.get("rep", 0)) if index else None,
        "alerts_alias_count": len(aliases),
        "alerts_mapping_fields": sorted(mapping),
        "component_template_count": len(components),
        "ism_policy_count": len(policies),
        "candidate_applied": any(item.get("name") == "traffic-alerts-mappings-v2" for item in components),
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
    backfill_plan_path = output / "backfill-read-only-plan.json"
    if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results):
        results.append(run_command(
            "opensearch-alerts-v2-backfill-read-only-plan",
            [
                "python3", "scripts/alignment/plan_opensearch_alerts_v2_backfill.py",
                "--tenant-id", "default",
                "--start-time", "2026-08-04T18:47:59Z",
                "--end-time", "2026-08-04T18:48:00Z",
                "--time-field", "last_seen",
                "--max-documents", "100",
                "--slices", "1",
                "--requests-per-second", "10",
                "--min-free-bytes", "161061273600",
                "--output", str(backfill_plan_path),
            ],
            output,
        ))
    repository_pass = len(results) == len(COMMANDS) + 1 and all(item["status"] == "PASS" for item in results)
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

    backfill_preflight: dict[str, Any] = {}
    preflight_artifacts: list[dict[str, Any]] = []
    if backfill_plan_path.is_file():
        backfill_preflight = json.loads(backfill_plan_path.read_text(encoding="utf-8"))
        preflight_artifacts.append(artifact(backfill_plan_path))

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-OS-002",
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_GUARDS_AND_READ_ONLY_LIVE_PRE_MIGRATION_CAPTURE",
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
        "checks": results,
        "live_observation": live,
        "backfill_preflight": backfill_preflight,
        "source_artifacts": source_artifacts,
        "preflight_artifacts": preflight_artifacts,
        "live_artifacts": live_artifacts,
        "closure_blockers": json.loads((ROOT / "contracts/opensearch/index-governance.v1.json").read_text())["closure_blockers"],
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": scoped,
        "manifest": str(manifest_path.relative_to(ROOT)),
        "manifest_sha256": sha256(manifest_path),
        "candidate_source_sha256": before["content_sha256"],
        "live_observation": live,
        "backfill_preflight": {
            "scoped_evidence_status": backfill_preflight.get("scoped_evidence_status"),
            "execution_readiness": backfill_preflight.get("execution_readiness"),
            "plan_sha256": backfill_preflight.get("plan_sha256"),
            "blockers": backfill_preflight.get("blockers", []),
            "production_mutations": backfill_preflight.get("production_mutations", []),
        },
    }, ensure_ascii=False, indent=2))
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
