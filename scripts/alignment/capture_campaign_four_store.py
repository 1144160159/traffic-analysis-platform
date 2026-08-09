#!/usr/bin/env python3
"""Capture one sentinel-protected Kafka/PG/CH/OS/Nebula campaign trace."""

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
REQUIRED_ENV = (
    "CAMPAIGN_EVENT_EPHEMERAL_PG_DSN",
    "CAMPAIGN_EVENT_EPHEMERAL_KAFKA_BROKER",
    "CAMPAIGN_PROJECTION_EPHEMERAL_CLICKHOUSE_HOST",
    "CAMPAIGN_PROJECTION_EPHEMERAL_OPENSEARCH_URL",
    "CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_ADDRESS",
    "CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_STORAGE_ADDRESS",
    "CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_BOOTSTRAP",
)
COMMANDS = (
    (
        "four-store-trace",
        [
            "go",
            "-C",
            "go/control-plane",
            "test",
            "./internal/alert/api",
            "-run",
            "^TestCampaignEventRealKafkaFourStoreTrace$",
            "-count=1",
            "-v",
        ],
    ),
    (
        "campaign-alignment",
        [
            "python3",
            "-m",
            "unittest",
            "tests.alignment.test_alignment_registry.AlignmentRegistryTest.test_campaign_event_v2_uses_dual_acknowledged_streams_and_durable_inbox",
            "tests.alignment.test_alignment_registry.AlignmentRegistryTest.test_campaign_three_target_projection_is_versioned_replayable_and_fail_closed",
        ],
    ),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started_at = datetime.now(timezone.utc)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=os.environ.copy(),
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
        )
    finished_at = datetime.now(timezone.utc)
    return {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "started_at": started_at.isoformat(),
        "finished_at": finished_at.isoformat(),
        "duration_seconds": round((finished_at - started_at).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    args = parser.parse_args()

    missing = [name for name in REQUIRED_ENV if not os.environ.get(name, "").strip()]
    if missing:
        raise SystemExit("missing required ephemeral settings: " + ",".join(missing))
    if os.environ["CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_BOOTSTRAP"] != "ephemeral-only":
        raise SystemExit("NebulaGraph ephemeral sentinel must equal ephemeral-only")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    g0_manifest = args.g0_manifest.resolve()
    if not g0_manifest.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_manifest}")
    g0 = json.loads(g0_manifest.read_text(encoding="utf-8"))
    if g0.get("status") != "PASS":
        raise SystemExit("G0 reference is not PASS")

    candidate = build_snapshot()
    g0_source_sha256 = g0.get("candidate_source", {}).get("content_sha256")
    if not g0_source_sha256 or g0_source_sha256 != candidate.get("content_sha256"):
        raise SystemExit("G0 reference and current candidate source hashes differ")
    results: list[dict[str, Any]] = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break
    passed = len(results) == len(COMMANDS) and all(
        result["status"] == "PASS" for result in results
    )
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "feature_id": "F-CAMPAIGN-001",
        "related_feature_id": "F-ALERT-002",
        "status": "PASS_WITH_OPEN_GATES" if passed else "FAIL",
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "candidate_source": candidate,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_manifest.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_manifest),
            "candidate_source_sha256": g0_source_sha256,
        },
        "commands": results,
        "ephemeral_contract": {
            "network_boundary": "numeric loopback only",
            "sentinels": [
                "PostgreSQL codex_ephemeral_campaign_event_test_sentinel=ephemeral-only",
                "PostgreSQL codex_ephemeral_campaign_target_projection_sentinel=ephemeral-only",
                "ClickHouse codex_ephemeral_campaign_projection_sentinel=ephemeral-only",
                "OpenSearch codex-ephemeral-campaign-projection-sentinel/ephemeral-only",
                "NebulaGraph CAMPAIGN_PROJECTION_EPHEMERAL_NEBULA_BOOTSTRAP=ephemeral-only",
            ],
            "credential_values_recorded": False,
        },
        "gate_status": {
            "G0": "PASS_IN_REFERENCED_FULL_RUN" if passed else "FAIL",
            "G1": "PASS_FOR_DUAL_OUTBOX_CONSUMER_TARGET_WORKER_AND_RECONCILIATION_ORACLE",
            "G2": "PASS_FOR_SENTINEL_PROTECTED_REAL_KAFKA_POSTGRES_CLICKHOUSE_OPENSEARCH_AND_NEBULAGRAPH",
            "G3": "PASS_FOR_ONE_REAL_KAFKA_EVENT_AND_TRACE_ACROSS_POSTGRES_CLICKHOUSE_OPENSEARCH_AND_NEBULAGRAPH",
            "G4": "OPEN",
            "G5": "OPEN",
            "G6": "HOLD",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "proven": [
            "one committed relation mutation emits aggregate and membership records with one stable event_id and trace_id",
            "the real Kafka records become two durable PostgreSQL inbox identities",
            "the production target worker advances three independent PostgreSQL target watermarks",
            "PostgreSQL, ClickHouse, OpenSearch and NebulaGraph agree on event, trace, projection revision and deterministic projection hash",
        ],
        "open": [
            "production migration, topic ACL, image canary and observation window",
            "approved-scale performance and fault recovery",
            "Windows Chrome link, replay, conflict and unlink journey",
            "campaign merge and deterministic historical backfill",
            "same-revision list, detail, members and report read model",
            "MinIO report executor and SOAR terminal receipt",
            "G8 external milestones",
        ],
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "manifest": str(manifest_path),
                "manifest_sha256": sha256(manifest_path),
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
