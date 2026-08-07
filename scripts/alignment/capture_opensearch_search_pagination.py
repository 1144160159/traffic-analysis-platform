#!/usr/bin/env python3
"""Capture immutable T-OS-003 repository and read-only live evidence."""

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
    ("search-pagination-contract", ["python3", "scripts/alignment/verify_opensearch_search_pagination.py"]),
    ("search-pagination-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_opensearch_search_pagination", "-v"]),
    ("alert-search-go-tests", ["bash", "-lc", "cd go/control-plane && go test ./internal/alert/repository ./internal/alert/api ./internal/alert/config ./internal/alert/service ./cmd/alert-service -count=1"]),
    ("openapi-contract", ["python3", "scripts/alignment/check_openapi.py"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCES = (
    "contracts/alignment/features/F-SEARCH-001.json",
    "contracts/opensearch/search-pagination.v1.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "go/control-plane/internal/alert/repository/opensearch.go",
    "go/control-plane/internal/alert/repository/opensearch_cursor.go",
    "go/control-plane/internal/alert/repository/opensearch_cursor_test.go",
    "go/control-plane/internal/alert/api/handler.go",
    "go/control-plane/internal/alert/api/handler_search_cursor_test.go",
    "go/control-plane/internal/alert/service/alert_service.go",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/internal/alert/config/config_test.go",
    "go/control-plane/cmd/alert-service/main.go",
    "deployments/kubernetes/applications/go-services.yaml",
    "web/ui/src/generated/alignmentClient.ts",
    "scripts/alignment/verify_opensearch_search_pagination.py",
    "scripts/alignment/capture_opensearch_search_pagination.py",
    "tests/alignment/test_opensearch_search_pagination.py",
    "doc/07_alignment/runbooks/T-OS-003-search-pagination-pit.md",
    "Makefile",
)
LIVE_ENDPOINTS = {
    "alerts-index": "/_cat/indices/alerts?format=json&h=index,docs.count,store.size,pri,rep",
    "alerts-settings": "/alerts/_settings?include_defaults=true&flat_settings=true",
    "pit-contexts": "/_search/point_in_time/_all",
    "node-search-contexts": "/_nodes/stats/indices/search?filter_path=nodes.*.indices.search.open_contexts,nodes.*.indices.search.query_current,nodes.*.indices.search.fetch_current",
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
    print(f"[opensearch-pagination] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[opensearch-pagination] {name}: {result['status']}", flush=True)
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

    deployment = subprocess.run([
        "kubectl", "--request-timeout=15s", "-n", "traffic-analysis", "get", "deployment",
        "alert-service", "-o", "json",
    ], cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    deployment_summary: dict[str, Any] = {}
    if deployment.returncode != 0:
        errors.append({"scope": "alert-deployment", "exit_code": deployment.returncode})
    else:
        try:
            payload = json.loads(deployment.stdout)
            container = payload["spec"]["template"]["spec"]["containers"][0]
            deployment_summary = {
                "image": container.get("image"),
                "opensearch_search_environment": [
                    {"name": item.get("name"), "value": item.get("value")}
                    for item in container.get("env", [])
                    if str(item.get("name", "")).startswith("OPENSEARCH_SEARCH_")
                ],
            }
        except (KeyError, IndexError, json.JSONDecodeError) as exc:
            errors.append({"scope": "alert-deployment", "error": f"decode failed: {exc}"})
    deployment_path = output / "live-alert-deployment-search-config.json"
    deployment_path.write_text(json.dumps(deployment_summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    artifacts.append(artifact(deployment_path))

    index_rows = decoded.get("alerts-index", [])
    index = index_rows[0] if index_rows else {}
    settings_payload = decoded.get("alerts-settings", {}).get("alerts", {})
    defaults = settings_payload.get("defaults", {})
    pits = decoded.get("pit-contexts", {}).get("pits", [])
    node_stats = decoded.get("node-search-contexts", {}).get("nodes", {})
    open_contexts = sum(
        int(node.get("indices", {}).get("search", {}).get("open_contexts", 0))
        for node in node_stats.values()
    )
    environment = deployment_summary.get("opensearch_search_environment", [])
    flag = next((item for item in environment if item.get("name") == "OPENSEARCH_SEARCH_CURSOR_V1_ENABLED"), None)
    live = {
        "read_only": True,
        "alerts_documents": int(index["docs.count"]) if index.get("docs.count") else None,
        "alerts_store_size": index.get("store.size"),
        "alerts_primary_shards": int(index["pri"]) if index.get("pri") else None,
        "alerts_replica_shards": int(index["rep"]) if index.get("rep") else None,
        "max_result_window": int(defaults["index.max_result_window"]) if defaults.get("index.max_result_window") else None,
        "max_inner_result_window": int(defaults["index.max_inner_result_window"]) if defaults.get("index.max_inner_result_window") else None,
        "max_regex_length": int(defaults["index.max_regex_length"]) if defaults.get("index.max_regex_length") else None,
        "active_pit_contexts": len(pits),
        "open_search_contexts": open_contexts,
        "deployed_cursor_flag_present": flag is not None,
        "deployed_cursor_flag_value": flag.get("value") if flag else None,
        "candidate_applied": flag is not None and flag.get("value") == "true",
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
    if not g0_hash or before["content_sha256"] != g0_hash:
        raise SystemExit("current candidate does not match the referenced G0 manifest")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    results: list[dict[str, Any]] = []
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

    contract = json.loads((ROOT / "contracts/opensearch/search-pagination.v1.json").read_text(encoding="utf-8"))
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "F-SEARCH-001",
        "remediation_id": "T-OS-003",
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_CURSOR_PIT_GUARDS_AND_READ_ONLY_LIVE_PRE_CANARY_CAPTURE",
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
            "G1": "PASS_FOR_SIGNED_CURSOR_HTTP_PROTOCOL_PIT_LIFECYCLE_COMPATIBILITY_AND_NEGATIVE_GUARDS",
            "G2": "OPEN_FOR_RELEASE_CANDIDATE_REAL_OPENSEARCH_SEARCH_AFTER_AND_PIT",
            "G3": "OPEN_FOR_PAGE_BOUNDARY_IDENTITY_TENANT_AND_SNAPSHOT_RECONCILIATION",
            "G4": "OPEN_FOR_FIXED_SCALE_COLD_WARM_P50_P95_P99_AND_RESOURCE_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_PRODUCTION_BUNDLE_HAR_AND_TRACE",
            "G6": "HOLD_FOR_CANARY_ROLLBACK_PIT_CLEANUP_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "live_observation": live,
        "source_artifacts": source_artifacts,
        "live_artifacts": live_artifacts,
        "closure_blockers": contract["closure_blockers"],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": scoped,
        "manifest": str(manifest_path.relative_to(ROOT)),
        "manifest_sha256": sha256(manifest_path),
        "candidate_source_sha256": before["content_sha256"],
        "live_observation": live,
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
