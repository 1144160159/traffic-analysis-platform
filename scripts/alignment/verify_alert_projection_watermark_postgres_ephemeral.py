#!/usr/bin/env python3
"""Verify T-OS-004 projection watermarks in an owned PostgreSQL container."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
MIGRATION = Path("deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql")


def run(command: list[str], *, input_bytes: bytes | None = None, env: dict[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, input=input_bytes, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=check)


def container_name(run_id: str) -> str:
    if not run_id.strip():
        raise ValueError("run_id is required")
    return "codex-alert-projection-pg-" + hashlib.sha256(run_id.encode()).hexdigest()[:12]


def absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    container = container_name(args.run_id)
    result = {
        "schema_version": 1, "run_id": args.run_id, "status": "FAIL", "container": container,
        "image": POSTGRES_IMAGE, "image_id": None, "migration": MIGRATION.as_posix(),
        "schema_ready": False, "sentinel_verified": False, "initial_missing_receipt_detected": False,
        "receipt_write_and_requery_verified": False, "same_version_hash_drift_detected": False,
        "same_version_hash_repair_verified": False, "target_version_isolation_verified": False,
        "loopback_only": True, "persistent_volume_attached": False,
        "shared_environment_touched": False, "production_applied": False, "container_removed": False,
        "test_output": "", "errors": [],
    }
    created = False
    try:
        if not absent(container):
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        inspected = json.loads(run(["docker", "image", "inspect", POSTGRES_IMAGE]).stdout)[0]
        result["image_id"] = inspected.get("Id")
        run(["docker", "run", "--name", container, "-e", "POSTGRES_PASSWORD=ephemeral-alert-projection-only",
             "-e", "POSTGRES_DB=traffic_platform", "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE])
        created = True
        for _ in range(60):
            if run(["docker", "exec", container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False).returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")
        run(["docker", "exec", "-i", container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform"],
            input_bytes=(ROOT / MIGRATION).read_bytes())
        result["schema_ready"] = True
        run(["docker", "exec", container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform", "-c",
             "CREATE TABLE codex_ephemeral_alert_projection_sentinel(marker text primary key); "
             "INSERT INTO codex_ephemeral_alert_projection_sentinel(marker) VALUES ('ephemeral-only');"])
        result["sentinel_verified"] = True
        port_text = run(["docker", "port", container, "5432/tcp"]).stdout.decode().strip()
        port = port_text.rsplit(":", 1)[-1]
        if not port.isdigit():
            raise RuntimeError(f"invalid loopback PostgreSQL port mapping: {port_text!r}")
        env = os.environ.copy()
        env["ALERT_PROJECTION_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:ephemeral-alert-projection-only@127.0.0.1:{port}/traffic_platform?sslmode=disable"
        )
        completed = run(["go", "-C", "go/control-plane", "test", "./internal/alert/persistence",
                         "-run", "^TestProjectionWatermarkReceiptRealPostgres$", "-count=1", "-v"], env=env, check=False)
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            raise RuntimeError(f"projection watermark PostgreSQL integration exited {completed.returncode}")
        result["initial_missing_receipt_detected"] = True
        result["receipt_write_and_requery_verified"] = True
        result["same_version_hash_drift_detected"] = True
        result["same_version_hash_repair_verified"] = True
        result["target_version_isolation_verified"] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if created:
            run(["docker", "rm", "-f", container], check=False)
        result["container_removed"] = absent(container)
    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite projection watermark G1 evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and result["container_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
