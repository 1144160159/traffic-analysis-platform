#!/usr/bin/env python3
"""Capture immutable campaign SOAR workflow evidence against a sentinel PostgreSQL."""

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
POSTGRES_ENV = "CAMPAIGN_AGGREGATE_EPHEMERAL_PG_DSN"

COMMANDS = (
    (
        "postgres-lifecycle",
        [
            "go", "-C", "go/control-plane", "test", "./internal/alert/api",
            "-run", "^TestCampaignSOARV2PostgresLifecycleIntegration$", "-count=1", "-v",
        ],
    ),
    (
        "alert-api-consumer",
        [
            "go", "-C", "go/control-plane", "test",
            "./internal/alert/api", "./internal/alert/consumer", "-count=1",
        ],
    ),
    (
        "web-contracts",
        [
            "npm", "--prefix", "web/ui", "run", "test", "--", "--run",
            "src/services/campaignActionApi.test.ts", "src/services/pageApiPlans.test.ts",
        ],
    ),
    ("web-build", ["npm", "--prefix", "web/ui", "run", "build"]),
    ("openapi", ["python3", "scripts/alignment/check_openapi.py"]),
    ("event-catalog", ["python3", "scripts/alignment/check_event_catalog.py"]),
    ("migrations", ["python3", "scripts/alignment/check_migrations.py"]),
    ("registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)

SOURCE_ARTIFACTS = (
    "deployments/postgres/migrations/202608020900_campaign_soar_workflow_v1.sql",
    "go/control-plane/internal/alert/api/campaign_soar_v2.go",
    "go/control-plane/internal/alert/api/campaign_soar_v2_integration_test.go",
    "go/control-plane/internal/alert/api/campaign_soar_http_executor.go",
    "go/control-plane/internal/alert/api/campaign_soar_http_executor_test.go",
    "go/control-plane/internal/alert/api/campaign_aggregate_v2.go",
    "go/control-plane/internal/alert/consumer/campaign_event_consumer.go",
    "go/control-plane/cmd/alert-service/main.go",
    "web/ui/src/services/campaignActionApi.ts",
    "web/ui/src/services/campaignActionApi.test.ts",
    "web/ui/src/services/pageApiPlans.ts",
    "web/ui/src/pages/CampaignWorkbenchPage.tsx",
    "contracts/alignment/features/F-CAMPAIGN-001.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "contracts/events/kafka-json-events-v1.schema.json",
    "doc/07_alignment/runbooks/F-CAMPAIGN-001-rollback.md",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run_command(name: str, command: list[str], output: Path, env: dict[str, str]) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[campaign-soar] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=env,
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
    print(f"[campaign-soar] {name}: {result['status']}", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    if not os.environ.get(POSTGRES_ENV):
        raise SystemExit(f"{POSTGRES_ENV} is required; the integration test rejects databases without its sentinel table")
    g0_manifest = args.g0_manifest.resolve()
    if not g0_manifest.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_manifest}")
    g0 = json.loads(g0_manifest.read_text(encoding="utf-8"))
    if g0.get("status") != "PASS" or g0.get("gate") != "G0":
        raise SystemExit("referenced G0 manifest is not a PASS result")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)

    env = os.environ.copy()
    results: list[dict[str, Any]] = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output, env)
        results.append(result)
        if result["status"] != "PASS":
            break

    status = "PASS" if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results) else "FAIL"
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
        "feature_id": "F-CAMPAIGN-001",
        "related_ids": ["F-PLAYBOOK-001", "T-PG-001", "T-SCHEMA-001"],
        "status": status,
        "candidate_source": build_snapshot(),
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_manifest.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_manifest),
            "status": g0.get("status"),
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS" if status == "PASS" else "FAIL",
            "G2": "OPEN",
            "G3": "OPEN",
            "G4": "OPEN",
            "G5": "OPEN",
            "G6": "OPEN",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "SOAR request commits pending_approval instead of treating HTTP acceptance as an external effect",
            "requester and approver are different identities and stale revisions, cross-tenant reads and self-approval fail closed",
            "approval idempotency replay does not dispatch a duplicate provider execution",
            "provider execution and independent compensation persist immutable receipts with stable phase idempotency keys",
            "terminal receipt, action job, campaign revision, aggregate history, outbox and audit commit in one PostgreSQL transaction",
            "an environment without a configured provider remains approved_awaiting_executor/not_configured with zero receipts",
            "the browser displays approval and executor states separately and refreshes the authoritative workflow revision",
            "terminal browser success is rejected when the provider receipt is absent",
        ],
        "open": [
            "approved production SOAR provider and credential integration",
            "real provider timeout, retry exhaustion, partial effect and ambiguous compensation fault drills",
            "real Kafka publication and downstream ClickHouse/OpenSearch/Nebula reconciliation for terminal SOAR events",
            "approved performance budget, production canary, Windows Chrome and rollback-in-flight evidence",
            "T+0/T+1/T+3/T+7 observation and G7/G8 sign-off",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": status,
        "manifest": str(manifest_path),
        "manifest_sha256": sha256(manifest_path),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
