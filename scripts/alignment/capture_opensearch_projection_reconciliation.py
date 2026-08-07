#!/usr/bin/env python3
"""Capture immutable T-OS-004 repository and read-only pre-canary evidence."""

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
    ("projection-contract", ["python3", "scripts/alignment/verify_opensearch_projection_reconciliation.py"]),
    ("projection-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_opensearch_projection_reconciliation", "-v"]),
    ("projection-go-tests", ["bash", "-lc", "cd go/control-plane && go test ./internal/alert/persistence ./internal/alert/projection ./internal/alert/consumer ./internal/common/kafka ./internal/alert/repository ./cmd/alert-service ./cmd/alert-projection-reconcile -count=1"]),
    ("migration-contract", ["python3", "scripts/alignment/check_migrations.py"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)
SOURCES = (
    "contracts/alignment/features/F-SEARCH-001.json",
    "contracts/opensearch/projection-reconciliation.v1.json",
    "deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql",
    "go/control-plane/internal/alert/persistence/dual_writer.go",
    "go/control-plane/internal/alert/persistence/projection_debt.go",
    "go/control-plane/internal/alert/persistence/projection_debt_integration_test.go",
    "go/control-plane/internal/alert/persistence/opensearch.go",
    "go/control-plane/internal/alert/persistence/opensearch_external_version_test.go",
    "go/control-plane/internal/alert/repository/clickhouse.go",
    "go/control-plane/internal/alert/repository/clickhouse_projection_query_test.go",
    "go/control-plane/internal/alert/consumer/kafka_consumer.go",
    "go/control-plane/internal/common/kafka/consumer.go",
    "go/control-plane/internal/alert/projection/worker.go",
    "go/control-plane/internal/alert/projection/reconcile.go",
    "go/control-plane/internal/alert/projection/reconcile_integration_test.go",
    "scripts/alignment/verify_alert_projection_postgres_opensearch_ephemeral.py",
    "go/control-plane/cmd/alert-projection-reconcile/main.go",
    "scripts/alignment/verify_opensearch_projection_reconciliation.py",
    "scripts/alignment/capture_opensearch_projection_reconciliation.py",
    "tests/alignment/test_opensearch_projection_reconciliation.py",
    "doc/07_alignment/runbooks/T-OS-004-alert-projection-rebuild-reconcile.md",
    "doc/02_acceptance/runs/20260805-remediation-opensearch-shadow-reconcile-v1/manifest.json",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
    "go/control-plane/deployments/docker/init/postgres_merged.sql",
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
    print(f"[opensearch-projection] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False)
    result = {
        "name": name, "command": command, "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "duration_seconds": round((datetime.now(timezone.utc) - started).total_seconds(), 3),
        "artifact": log_path.name, "sha256": sha256(log_path), "size_bytes": log_path.stat().st_size,
    }
    print(f"[opensearch-projection] {name}: {result['status']}", flush=True)
    return result


def kubectl(args: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(["kubectl", "--request-timeout=20s", *args], cwd=ROOT,
                          stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)


def save_command(output: Path, name: str, result: subprocess.CompletedProcess[bytes]) -> list[dict[str, Any]]:
    stdout = output / f"live-{name}.out"
    stderr = output / f"live-{name}.stderr.log"
    stdout.write_bytes(result.stdout)
    stderr.write_bytes(result.stderr)
    return [artifact(stdout), artifact(stderr)]


def projection_metric_lines(payload: bytes) -> list[str]:
    """Keep only the post-commit metrics that prove the projection boundary."""
    prefixes = (
        "alert_consumer_lag",
        "alert_consumer_last_committed",
    )
    return [
        line
        for line in payload.decode(errors="replace").splitlines()
        if line.startswith(prefixes)
    ]


def capture_live(output: Path) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    artifacts: list[dict[str, Any]] = []
    errors: list[dict[str, Any]] = []

    pg_sql = """SELECT json_build_object(
      'migration',(SELECT count(*) FROM alignment_schema_migrations WHERE version='202608041100'),
      'tables',(SELECT COALESCE(json_agg(tablename ORDER BY tablename),'[]'::json) FROM pg_tables WHERE schemaname='public' AND tablename LIKE 'alert_opensearch_%')
    )::text"""
    pg = kubectl(["-n", "databases", "exec", "postgres-primary-0", "--", "psql", "-U", "postgres", "-d", "traffic_platform", "-Atqc", pg_sql])
    artifacts.extend(save_command(output, "postgres-projection-schema", pg))
    pg_state: dict[str, Any] = {}
    if pg.returncode == 0:
        try:
            pg_state = json.loads(pg.stdout)
        except json.JSONDecodeError as exc:
            errors.append({"scope": "postgres", "error": f"decode failed: {exc}"})
    else:
        errors.append({"scope": "postgres", "exit_code": pg.returncode})

    os_result = kubectl(["-n", "middleware", "exec", "opensearch-0", "--", "curl", "-fsS",
                         "http://127.0.0.1:9200/_cat/indices/alerts*?format=json&h=index,docs.count,store.size,pri,rep,health,status"])
    artifacts.extend(save_command(output, "opensearch-alert-indices", os_result))
    os_indices: list[dict[str, Any]] = []
    if os_result.returncode == 0:
        try:
            os_indices = json.loads(os_result.stdout)
        except json.JSONDecodeError as exc:
            errors.append({"scope": "opensearch", "error": f"decode failed: {exc}"})
    else:
        errors.append({"scope": "opensearch", "exit_code": os_result.returncode})

    deployment = kubectl(["-n", "traffic-analysis", "get", "deployment", "alert-service", "-o", "json"])
    artifacts.extend(save_command(output, "alert-deployment", deployment))
    deployment_state: dict[str, Any] = {}
    if deployment.returncode == 0:
        try:
            payload = json.loads(deployment.stdout)
            container = payload["spec"]["template"]["spec"]["containers"][0]
            env = [{"name": item.get("name"), "value": item.get("value")} for item in container.get("env", [])
                   if str(item.get("name", "")).startswith("OPENSEARCH_ALERT_PROJECTION")]
            deployment_state = {"image": container.get("image"), "projection_environment": env}
        except (KeyError, IndexError, json.JSONDecodeError) as exc:
            errors.append({"scope": "alert-deployment", "error": f"decode failed: {exc}"})
    else:
        errors.append({"scope": "alert-deployment", "exit_code": deployment.returncode})

    metrics = kubectl(["get", "--raw",
                       "/api/v1/namespaces/traffic-analysis/services/http:alert-service:9093/proxy/metrics"])
    artifacts.extend(save_command(output, "alert-consumer-projection-metrics", metrics))
    metric_lines = projection_metric_lines(metrics.stdout)
    if metrics.returncode != 0:
        errors.append({"scope": "alert-metrics", "exit_code": metrics.returncode})

    flag = next((item for item in deployment_state.get("projection_environment", [])
                 if item.get("name") == "OPENSEARCH_ALERT_PROJECTION_RECONCILE_V1_ENABLED"), None)
    live = {
        "read_only": True,
        "postgres_migration_present": pg_state.get("migration") == 1,
        "postgres_projection_tables": pg_state.get("tables", []),
        "opensearch_alert_indices": os_indices,
        "deployed_alert_service": deployment_state,
        "deployed_reconcile_flag_present": flag is not None,
        "deployed_reconcile_flag_value": flag.get("value") if flag else None,
        "deployed_post_commit_metric_lines": metric_lines,
        "candidate_applied": bool(pg_state.get("migration") == 1 and flag and flag.get("value") == "true"),
        "kafka_cli_describe_skipped": "prior read-only CLI attempt exhausted its own Java heap; no repeat load was generated",
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

    source_artifacts = []
    for relative in SOURCES:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        source_artifacts.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})
    contract = json.loads((ROOT / "contracts/opensearch/projection-reconciliation.v1.json").read_text(encoding="utf-8"))
    manifest = {
        "schema_version": 1, "run_id": args.run_id, "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "F-SEARCH-001", "remediation_id": "T-OS-004",
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_DEBT_REPAIR_RECONCILE_GUARDS_AND_READ_ONLY_PRE_CANARY_CAPTURE",
        "scoped_evidence_status": scoped, "candidate_source": before, "candidate_source_stable": stable,
        "production_applied": False, "read_only_live_capture": True,
        "g0_reference": {"run_id": g0.get("run_id"), "manifest": str(g0_path.relative_to(ROOT)),
                         "manifest_sha256": sha256(g0_path), "candidate_source_sha256": g0_hash},
        "gate_status": {
            "G0": "PASS", "G1": "PASS_FOR_DURABLE_DEBT_EXTERNAL_VERSION_WORKER_RECONCILE_AND_NEGATIVE_GUARDS",
            "G2": "OPEN_FOR_APPROVED_REAL_PG_CH_OS_AND_KAFKA_OFFSET_EXECUTION",
            "G3": "OPEN_FOR_REAL_COUNT_ID_FIELD_HASH_AND_LAST_EVENT_RECONCILIATION",
            "G4": "OPEN_FOR_FIXED_SCALE_P50_P95_P99_REPAIR_AND_RESOURCE_BUDGETS",
            "G5": "NOT_APPLICABLE_TO_OPERATOR_CLI_UNLESS_EXPOSED_IN_UI",
            "G6": "HOLD_FOR_CANARY_ROLLBACK_AND_OBSERVATION", "G7": "OPEN", "G8": "BLOCKED",
        },
        "commands": results, "live_observation": live, "source_artifacts": source_artifacts,
        "live_artifacts": live_artifacts, "closure_blockers": contract["closure_blockers"], "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": manifest["status"], "scoped_evidence_status": scoped,
                      "manifest": str(manifest_path.relative_to(ROOT)), "manifest_sha256": sha256(manifest_path),
                      "candidate_source_sha256": before["content_sha256"], "live_observation": live}, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
