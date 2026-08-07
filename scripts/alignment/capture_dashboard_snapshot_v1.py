#!/usr/bin/env python3
"""Capture immutable F-DASHBOARD-001 unified snapshot evidence."""

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
SOURCE_ARTIFACTS = (
    "scripts/alignment/capture_dashboard_snapshot_v1.py",
    "go/control-plane/internal/alert/api/dashboard_snapshot_v1.go",
    "go/control-plane/internal/alert/api/dashboard_snapshot_v1_test.go",
    "go/control-plane/internal/alert/api/dashboard_task_v2_integration_test.go",
    "go/control-plane/internal/alert/api/handler_dashboard.go",
    "go/control-plane/cmd/alert-service/main.go",
    "web/ui/src/services/dashboardSnapshotApi.ts",
    "web/ui/src/services/dashboardSnapshotApi.test.ts",
    "web/ui/src/services/api.ts",
    "web/ui/src/services/pageApiPlans.ts",
    "web/ui/src/services/pageApiPlans.test.ts",
    "web/ui/src/services/pageSnapshotAdapters.ts",
    "web/ui/src/services/mockData.ts",
    "web/ui/src/pages/DashboardOperationsPage.tsx",
    "web/ui/src/routes/routeManifest.tsx",
    "contracts/alignment/features/F-DASHBOARD-001.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "doc/07_alignment/runbooks/F-DASHBOARD-001-rollback.md",
    "tests/alignment/test_alignment_registry.py",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[dashboard-snapshot] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=os.environ.copy(),
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
    print(f"[dashboard-snapshot] {name}: {result['status']}", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    g0_manifest = args.g0_manifest.resolve()
    if not g0_manifest.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_manifest}")
    g0 = json.loads(g0_manifest.read_text(encoding="utf-8"))
    if g0.get("status") != "PASS" or g0.get("gate") != "G0":
        raise SystemExit("referenced G0 manifest is not a PASS G0 result")

    candidate_before = build_snapshot()
    g0_hash = g0.get("candidate_source", {}).get("content_sha256")
    if not g0_hash or g0_hash != candidate_before["content_sha256"]:
        raise SystemExit("referenced G0 manifest does not cover the current candidate source")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)

    commands = (
        (
            "dashboard-go",
            ["go", "-C", "go/control-plane", "test", "./internal/alert/api", "./cmd/alert-service", "-count=1"],
        ),
        (
            "dashboard-web",
            [
                "npm", "--prefix", "web/ui", "test", "--", "--run",
                "src/services/dashboardSnapshotApi.test.ts",
                "src/services/dashboardTaskApi.test.ts",
                "src/services/pageApiPlans.test.ts",
            ],
        ),
        ("production-web-build", ["npm", "--prefix", "web/ui", "run", "build"]),
        (
            "sentinel-postgres",
            [
                "python3", "scripts/alignment/verify_asset_atomic_ephemeral.py",
                "--run-id", args.run_id + "-pg",
            ],
        ),
        ("openapi", ["python3", "scripts/alignment/check_openapi.py"]),
        (
            "dashboard-regression",
            [
                "python3", "-m", "unittest",
                "tests.alignment.test_alignment_registry.AlignmentRegistryTest.test_dashboard_snapshot_is_one_tenant_bound_partial_aware_query",
                "-v",
            ],
        ),
        ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
    )
    results: list[dict[str, Any]] = []
    for name, command in commands:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break

    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    scoped_pass = (
        len(results) == len(commands)
        and all(item["status"] == "PASS" for item in results)
        and candidate_stable
    )
    artifacts = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        artifacts.append(
            {"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size}
        )

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "F-DASHBOARD-001",
        "related_ids": ["F-DASHBOARD-002", "T-PG-002", "T-OS-001", "T-REDIS-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL",
        "scoped_evidence_status": "PASS" if scoped_pass else "FAIL",
        "candidate_source": candidate_before,
        "candidate_source_stable": candidate_stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_manifest.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_manifest),
            "status": g0.get("status"),
            "candidate_source_sha256": g0_hash,
        },
        "gate_status": {
            "G0": "PASS" if scoped_pass else "FAIL",
            "G1": "PASS_FOR_TENANT_BOUND_SINGLE_SNAPSHOT_SCHEMA_COMPONENTS_AND_SENTINEL_POSTGRES",
            "G2": "OPEN_FOR_RELEASE_CANDIDATE_CLICKHOUSE_OPENSEARCH_REDIS_AND_POSTGRES",
            "G3": "OPEN_FOR_SAME_WINDOW_CROSS_STORE_RECONCILIATION",
            "G4": "OPEN_FOR_24H_COLD_CACHE_PARTIAL_SOURCE_AND_P99_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_MOCK_OFF",
            "G6": "HOLD_FOR_SHADOW_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": artifacts,
        "proven": [
            "the production page performs one GET /v1/dashboard/snapshot and stores the server snapshot_id instead of a client view fingerprint",
            "the handler derives tenant and user only from authenticated context, requires alert:read and rejects tenant_id query overrides",
            "one start/end window, deterministic snapshot_id, trace_id, partial, missing_sections and per-source watermarks cover every returned section",
            "real zero, unknown and source failure remain distinct; unavailable sources produce null/unknown and explicit missing sections instead of invented zero",
            "ClickHouse alert, trend, phase, queue and top-talker queries and PostgreSQL task/audit queries bind the authenticated tenant",
            "OpenSearch and Redis failures are isolated into partial metadata rather than converted to success",
            "the Web UI no longer generates synthetic queue rows or combines the legacy stats/trend/phases requests",
            "the existing legacy Dashboard GET routes remain available but no longer accept query tenant identity",
            "sentinel PostgreSQL 16 proves one tenant sees its accepted task while another tenant sees zero and the disposable container is removed",
            "the rollout flag is default-off in both Kubernetes alert-service entrypoints and the rollback runbook forbids mock or fill-zero fallback",
        ],
        "open": [
            "run the exact release candidate against real ClickHouse, PostgreSQL, OpenSearch and Redis in one trace",
            "reconcile ClickHouse and OpenSearch counts plus watermarks for empty, zero, partial and stale windows",
            "replace the explicitly unknown SLA, feedback and review metrics with approved authoritative read models",
            "measure 24-hour warm/cold cache and one-source-timeout P50/P95/P99 plus resource budgets",
            "capture the production bundle in designated Windows Chrome with mock disabled, HAR, console, trace and source queries",
            "execute shadow read, canary, rollback and T+0/T+1/T+3/T+7 observation with independent approval",
            "complete G7 sign-off and retain G8 external milestones",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "scoped_evidence_status": manifest["scoped_evidence_status"],
                "manifest": str(manifest_path),
                "manifest_sha256": sha256(manifest_path),
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
