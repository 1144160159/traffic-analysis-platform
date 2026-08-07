#!/usr/bin/env python3
"""Capture immutable G1 evidence for T-DQ-001 PostgreSQL entrypoints."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path

from candidate_snapshot import build_snapshot
from sync_data_quality_postgres_entrypoints import check as check_generated_mirrors


ROOT = Path(__file__).resolve().parents[2]
EXPECTED_TABLES = {
    "data_quality_datasets",
    "data_quality_rules",
    "data_quality_baselines",
    "data_quality_watermarks",
    "data_quality_events",
    "data_quality_repairs",
    "data_quality_outbox",
    "data_quality_dataset_history",
    "data_quality_rule_history",
    "data_quality_command_requests",
    "data_quality_rule_evaluations",
    "data_quality_repair_history",
    "data_quality_repair_requests",
    "data_quality_flow_replay_projection",
    "data_quality_replay_projection_receipts",
}
EXPECTED_MIGRATIONS = {"202608041400", "202608041500", "202608041600", "202608041700", "202608041800"}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    args = parser.parse_args()
    output = ROOT / "doc/02_acceptance/runs" / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite {output}")
    output.mkdir(parents=True)
    command = [
        "python3",
        "scripts/alignment/verify_pg_schema_entrypoints_ephemeral.py",
        "--run-id",
        args.run_id,
    ]
    completed = subprocess.run(
        command,
        cwd=ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    stdout_path = output / "schema-entrypoints.stdout"
    stderr_path = output / "schema-entrypoints.stderr.log"
    stdout_path.write_bytes(completed.stdout)
    stderr_path.write_bytes(completed.stderr)
    errors: list[str] = []
    result: dict = {}
    if completed.returncode:
        errors.append(f"ephemeral schema verifier exited {completed.returncode}")
    else:
        try:
            result = json.loads(completed.stdout)
        except json.JSONDecodeError as exc:
            errors.append(f"ephemeral schema verifier returned invalid JSON: {exc}")

    snapshots = result.get("snapshots", {})
    snapshot_hashes = {
        item.get("sha256") for item in snapshots.values() if isinstance(item, dict)
    }
    checks = {
        "ephemeral_verifier_passed": result.get("result") == "pass",
        "all_three_entrypoints_present": set(snapshots)
        == {"common", "docker_merged", "kubernetes_configmap"},
        "all_three_entrypoints_replayed_twice": result.get("passes_per_entrypoint") == 2,
        "schema_hashes_equal": len(snapshot_hashes) == 1,
        "all_data_quality_tables_present": bool(snapshots)
        and all(EXPECTED_TABLES.issubset(set(item.get("tables", []))) for item in snapshots.values()),
        "versioned_migrations_registered": EXPECTED_MIGRATIONS.issubset(
            set(result.get("migration_versions", []))
        ),
        "generated_mirrors_current": not check_generated_mirrors(ROOT),
        "ephemeral_databases_removed": len(result.get("temporary_databases_removed", [])) == 3,
        "shared_environment_untouched": result.get("shared_environment_touched") is False,
        "secrets_not_captured": result.get("secrets_captured") is False,
    }
    status = "PASS" if not errors and all(checks.values()) else "FAIL"
    candidate = build_snapshot()
    artifacts = []
    for path in (stdout_path, stderr_path):
        artifacts.append(
            {
                "path": path.name,
                "sha256": sha256(path),
                "size_bytes": path.stat().st_size,
            }
        )
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "status": status,
        "feature_id": "F-DATAQUALITY-001",
        "remediation_id": "T-DQ-001",
        "scope": "G1_EPHEMERAL_POSTGRESQL_SCHEMA_ENTRYPOINT_REPLAY",
        "production_applied": False,
        "production_mutations": [],
        "ephemeral_resources_removed": result.get("temporary_databases_removed", []),
        "candidate_source": {
            "content_sha256": candidate["content_sha256"],
            "file_count": candidate["file_count"],
        },
        "checks": checks,
        "schema_summary": {
            "entrypoints": sorted(snapshots),
            "sha256": next(iter(snapshot_hashes), None),
            "column_count": next(
                (item.get("columns") for item in snapshots.values()), None
            ),
            "data_quality_tables": sorted(EXPECTED_TABLES),
            "migration_versions": result.get("migration_versions", []),
        },
        "errors": errors,
        "artifacts": artifacts,
        "remaining_gates": [
            "approved production migration apply",
            "candidate alert-service rollout",
            "real PostgreSQL handoff persistence and cross-store reconciliation",
            "performance failure rollback Windows Chrome and observation gates",
        ],
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(
        json.dumps(
            {
                "status": status,
                "manifest": manifest_path.relative_to(ROOT).as_posix(),
                "manifest_sha256": sha256(manifest_path),
                "candidate_source_sha256": candidate["content_sha256"],
                "checks": checks,
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0 if status == "PASS" else 1


if __name__ == "__main__":
    sys.exit(main())
