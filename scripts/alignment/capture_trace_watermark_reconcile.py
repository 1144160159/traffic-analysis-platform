#!/usr/bin/env python3
"""Capture immutable T-OBS-001 repository and read-only pre-rollout evidence."""

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
    ("trace-watermark-contract", ["python3", "scripts/alignment/verify_trace_watermark_reconcile.py"]),
    ("trace-watermark-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_trace_watermark_reconcile", "tests.alignment.test_cross_store_reconcile", "-v"]),
    ("trace-go-tests", ["bash", "-lc", "cd go/control-plane && go test ./internal/common/httpx ./internal/common/otel ./internal/common/kafka ./internal/alert/persistence ./internal/alert/consumer -count=1"]),
    ("trace-flink-tests", ["mvn", "-f", "java/flink-jobs/pom.xml", "-pl", "flink-alert-generator-job", "-am", "-DskipITs", "-Dcheckstyle.skip=true", "-Dspotbugs.skip=true", "test"]),
    ("proto-lint", ["buf", "lint", "proto"]),
    ("migration-contract", ["python3", "scripts/alignment/check_migrations.py"]),
    ("opensearch-schema-contract", ["python3", "scripts/alignment/verify_opensearch_index_governance.py"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCES = (
    "contracts/observability/trace-watermark-reconcile.v1.json",
    "proto/traffic/v1/alert.proto",
    "go/control-plane/internal/common/httpx/request_id.go",
    "go/control-plane/internal/common/kafka/producer.go",
    "go/control-plane/internal/common/kafka/consumer.go",
    "go/control-plane/internal/common/otel/tracer.go",
    "go/control-plane/internal/alert/persistence/alert.go",
    "go/control-plane/internal/alert/persistence/clickhouse.go",
    "go/control-plane/internal/alert/consumer/kafka_consumer.go",
    "go/control-plane/internal/alert/consumer/alert_consumer.go",
    "java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/generator/AlertGenerator.java",
    "java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/generator/BusinessAlertGenerator.java",
    "java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/sink/ClickHouseAlertSinkFactory.java",
    "java/flink-jobs/flink-alert-generator-job/src/main/java/com/traffic/flink/alert/sink/OpenSearchAlertSinkFactory.java",
    "deployments/clickhouse/migrations/202608041300_alert_trace_correlation_v1.sql",
    "contracts/clickhouse/schema-authority.v1.json",
    "common/sql/ch/00-all-tables.sql",
    "go/control-plane/deployments/docker/init/clickhouse_merged.sql",
    "deployments/kubernetes/init-jobs/03-clickhouse-schema.yaml",
    "common/opensearch/alerts-v2/mappings-component.json",
    "deployments/kubernetes/init-jobs/04-opensearch-templates.yaml",
    "scripts/alignment/cross_store_reconcile.py",
    "scripts/alignment/verify_trace_watermark_reconcile.py",
    "scripts/alignment/capture_trace_watermark_reconcile.py",
    "tests/alignment/test_trace_watermark_reconcile.py",
    "tests/alignment/test_cross_store_reconcile.py",
    "doc/07_alignment/runbooks/T-OBS-001-trace-watermark-reconcile.md",
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


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[trace-watermark] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[trace-watermark] {name}: {result['status']}", flush=True)
    return result


def kubectl(args: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(["kubectl", "--request-timeout=25s", *args], cwd=ROOT,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)


def save_result(output: Path, name: str, result: subprocess.CompletedProcess[bytes]) -> list[dict[str, Any]]:
    stdout = output / f"live-{name}.stdout"
    stderr = output / f"live-{name}.stderr.log"
    stdout.write_bytes(result.stdout)
    stderr.write_bytes(result.stderr)
    return [artifact(stdout), artifact(stderr)]


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

    pods_result = kubectl(["-n", "middleware", "get", "pods", "-l", "app=clickhouse", "-o", "json"])
    artifacts.extend(save_result(output, "clickhouse-pods", pods_result))
    pod_payload = parse_json(pods_result, "clickhouse-pods", errors) or {}
    pods = sorted(item.get("metadata", {}).get("name") for item in pod_payload.get("items", [])
                  if item.get("status", {}).get("phase") == "Running")
    clickhouse_columns: dict[str, list[dict[str, Any]]] = {}
    query = """
SELECT hostName() AS observer, table, name, type, position
FROM system.columns
WHERE database='traffic'
  AND table IN ('alerts_local','alerts','alerts_latest_local','alerts_latest')
  AND name='trace_id'
ORDER BY table
FORMAT JSONEachRow
""".strip()
    for pod in pods:
        result = kubectl([
            "-n", "middleware", "exec", "-c", "clickhouse", pod, "--", "sh", "-lc",
            'exec clickhouse-client --password "$CLICKHOUSE_PASSWORD" --query "$1"',
            "capture-trace-watermark", query,
        ])
        artifacts.extend(save_result(output, f"clickhouse-trace-columns-{pod}", result))
        if result.returncode:
            errors.append({"scope": f"clickhouse:{pod}", "exit_code": result.returncode})
            continue
        rows: list[dict[str, Any]] = []
        try:
            rows = [json.loads(line) for line in result.stdout.decode().splitlines() if line.strip()]
        except json.JSONDecodeError as exc:
            errors.append({"scope": f"clickhouse:{pod}", "error": f"decode failed: {exc}"})
        clickhouse_columns[pod] = rows

    os_result = kubectl([
        "-n", "middleware", "exec", "opensearch-0", "--", "curl", "-fsS",
        "http://127.0.0.1:9200/_mapping?filter_path=*.mappings.properties.trace_id",
    ])
    artifacts.extend(save_result(output, "opensearch-trace-mapping", os_result))
    os_mapping = parse_json(os_result, "opensearch-trace-mapping", errors) or {}

    deployments_result = kubectl(["get", "deployments", "-A", "-o", "json"])
    artifacts.extend(save_result(output, "deployments", deployments_result))
    deployments_payload = parse_json(deployments_result, "deployments", errors) or {}
    workloads: list[dict[str, Any]] = []
    for item in deployments_payload.get("items", []):
        name = str(item.get("metadata", {}).get("name", ""))
        if "alert" not in name and "flink" not in name:
            continue
        workloads.append({
            "namespace": item.get("metadata", {}).get("namespace"),
            "name": name,
            "images": [container.get("image") for container in item.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])],
            "ready_replicas": item.get("status", {}).get("readyReplicas", 0),
        })
    workloads.sort(key=lambda item: (str(item["namespace"]), str(item["name"])))

    metrics_result = kubectl([
        "-n", "traffic-analysis", "exec", "deployment/alert-service", "--", "sh", "-lc",
        "wget -qO- http://127.0.0.1:9093/metrics 2>/dev/null | grep -E 'trace|watermark|reconcile' || true",
    ])
    artifacts.extend(save_result(output, "alert-trace-watermark-metrics", metrics_result))
    if metrics_result.returncode:
        errors.append({"scope": "alert-service-metrics", "exit_code": metrics_result.returncode})
    metrics_lines = [line for line in metrics_result.stdout.decode(errors="replace").splitlines() if line.strip()]

    ch_trace_tables = sorted({str(row.get("table")) for rows in clickhouse_columns.values() for row in rows})
    os_trace_indices = sorted(name for name, body in os_mapping.items()
                              if body.get("mappings", {}).get("properties", {}).get("trace_id", {}).get("type") == "keyword")
    live = {
        "read_only": True,
        "clickhouse_pods": pods,
        "clickhouse_trace_columns": clickhouse_columns,
        "clickhouse_trace_tables": ch_trace_tables,
        "all_four_clickhouse_alert_tables_have_trace_id": set(ch_trace_tables) == {"alerts", "alerts_latest", "alerts_latest_local", "alerts_local"},
        "opensearch_indices_with_keyword_trace_id": os_trace_indices,
        "deployed_candidate_workloads": workloads,
        "trace_watermark_reconcile_metric_lines": metrics_lines,
        "six_store_same_trace_manifest": None,
        "candidate_applied": bool(set(ch_trace_tables) == {"alerts", "alerts_latest", "alerts_latest_local", "alerts_local"} and os_trace_indices),
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
        raise SystemExit("current candidate does not match referenced G0 manifest")

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
    after = build_snapshot()
    stable = before["content_sha256"] == after["content_sha256"]
    scoped = "PASS" if repository_pass and stable and not live.get("errors") else "FAIL"

    source_artifacts: list[dict[str, Any]] = []
    for relative in SOURCES:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        source_artifacts.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})
    contract = json.loads(
        (ROOT / "contracts/observability/trace-watermark-reconcile.v1.json").read_text(encoding="utf-8")
    )
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-OBS-001",
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_TRACE_VERTICAL_SLICE_PLAN_ONLY_RECONCILE_AND_READ_ONLY_PRE_ROLLOUT_CAPTURE",
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
            "G1": "PASS_FOR_HTTP_KAFKA_ALERT_PROTO_FLINK_CH_OS_AND_PLAN_ONLY_RECONCILE_GUARDS",
            "G2": "OPEN_FOR_RELEASE_CANDIDATE_PG_OUTBOX_KAFKA_FLINK_CH_OS_NEBULA_MINIO_AND_AUDIT_TRACE",
            "G3": "OPEN_FOR_SIX_STORE_COUNT_ID_VERSION_HASH_TRACE_AND_WATERMARK_RECONCILIATION",
            "G4": "OPEN_FOR_FIXED_SCALE_P50_P95_P99_AND_RESOURCE_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_SNAPSHOT_PARTIAL_AND_WATERMARK_EVIDENCE",
            "G6": "HOLD_FOR_CANARY_ROLLBACK_AND_T_PLUS_OBSERVATION",
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
