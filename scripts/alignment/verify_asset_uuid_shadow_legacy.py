#!/usr/bin/env python3
"""Verify asset TEXT-key compatibility and UUID shadow dual-write in owned PostgreSQL."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
IMAGE = "postgres:16"
MIGRATIONS = [
    "202607310015_asset_atomic_upsert_v2.sql",
    "202607310030_asset_projection_inbox.sql",
    "202607310100_asset_discovery_jobs_v2.sql",
    "202607311330_asset_governance_work_orders_v1.sql",
    "202608031510_asset_legacy_mutation_atomic.sql",
    "202608042000_asset_uuid_shadow_v1.sql",
]


def run(command: list[str], *, input_bytes: bytes | None = None,
        env: dict[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, input=input_bytes, env=env,
                          stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=check)


def psql(container: str, sql: str) -> str:
    completed = run([
        "docker", "exec", container, "psql", "-At", "-v", "ON_ERROR_STOP=1",
        "-U", "postgres", "-d", "traffic_platform", "-c", sql,
    ])
    return completed.stdout.decode().strip()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    args = parser.parse_args()
    suffix = hashlib.sha256(args.run_id.encode()).hexdigest()[:12]
    container = f"codex-asset-uuid-shadow-pg-{suffix}"
    result: dict[str, object] = {
        "schema_version": 1, "run_id": args.run_id, "container": container,
        "image": IMAGE, "status": "FAIL", "asset_id_type": "",
        "migrations": MIGRATIONS, "reconcile_before": [], "reconcile_after": [],
        "legacy_write_shadow": False, "mismatch_rejected": False,
        "go_integration": "NOT_RUN", "shared_environment_touched": False,
        "persistent_volume_attached": False, "container_removed": False, "errors": [],
    }
    created = False
    try:
        if run(["docker", "container", "inspect", container], check=False).returncode == 0:
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        run(["docker", "image", "inspect", IMAGE])
        run(["docker", "run", "--name", container,
             "-e", "POSTGRES_PASSWORD=codex-ephemeral-only",
             "-e", "POSTGRES_DB=traffic_platform", "-p", "127.0.0.1::5432", "-d", IMAGE])
        created = True
        for _ in range(30):
            if run(["docker", "exec", container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False).returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")

        fixture = ROOT / "scripts/alignment/fixtures/legacy_asset_text_schema.sql"
        run(["docker", "exec", "-i", container, "psql", "-v", "ON_ERROR_STOP=1",
             "-U", "postgres", "-d", "traffic_platform"], input_bytes=fixture.read_bytes())
        psql(container, "CREATE TABLE codex_ephemeral_asset_atomic_test_sentinel(marker text primary key); INSERT INTO codex_ephemeral_asset_atomic_test_sentinel VALUES ('ephemeral-only')")
        for name in MIGRATIONS:
            path = ROOT / "deployments/postgres/migrations" / name
            run(["docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i", container,
                 "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform"],
                input_bytes=path.read_bytes())

        result["asset_id_type"] = psql(container, "SELECT data_type FROM information_schema.columns WHERE table_name='assets' AND column_name='asset_id'")
        if result["asset_id_type"] != "text":
            raise RuntimeError(f"legacy asset_id changed type unexpectedly: {result['asset_id_type']}")
        before = psql(container, "SELECT domain||':'||row_count||':'||mismatch_count FROM asset_uuid_shadow_reconcile_v1 ORDER BY domain")
        result["reconcile_before"] = before.splitlines()
        if any(not row.endswith(":0") for row in result["reconcile_before"]):
            raise RuntimeError(f"pre-integration shadow mismatch: {result['reconcile_before']}")

        psql(container, "INSERT INTO assets(asset_id,tenant_id,mac_address) VALUES ('22222222-2222-4222-8222-222222222222','legacy-fixture','02:00:00:00:00:02')")
        result["legacy_write_shadow"] = psql(container, "SELECT asset_uuid::text=asset_id FROM assets WHERE asset_id='22222222-2222-4222-8222-222222222222'") == "t"
        mismatch = run(["docker", "exec", container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform", "-c",
                        "UPDATE assets SET asset_uuid='33333333-3333-4333-8333-333333333333' WHERE asset_id='22222222-2222-4222-8222-222222222222'"], check=False)
        result["mismatch_rejected"] = mismatch.returncode != 0
        if not result["legacy_write_shadow"] or not result["mismatch_rejected"]:
            raise RuntimeError("UUID shadow trigger did not dual-write or reject divergence")

        port_output = run(["docker", "port", container, "5432/tcp"]).stdout.decode().strip()
        host_port = port_output.rsplit(":", 1)[-1]
        if not host_port.isdigit():
            raise RuntimeError("could not resolve ephemeral PostgreSQL loopback port")
        env = os.environ.copy()
        env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:codex-ephemeral-only@127.0.0.1:{host_port}/traffic_platform?sslmode=disable"
        )
        completed = run(["go", "-C", "go/control-plane", "test", "./internal/asset/repository",
                         "-run", "TestAssetAtomicUpsertPostgresIntegration", "-count=1", "-v"], env=env, check=False)
        print(completed.stdout.decode(), end="")
        result["go_integration"] = "PASS" if completed.returncode == 0 else "FAIL"
        if completed.returncode != 0:
            raise RuntimeError(f"legacy TEXT asset integration exited {completed.returncode}")

        after = psql(container, "SELECT domain||':'||row_count||':'||mismatch_count FROM asset_uuid_shadow_reconcile_v1 ORDER BY domain")
        result["reconcile_after"] = after.splitlines()
        if any(not row.endswith(":0") for row in result["reconcile_after"]):
            raise RuntimeError(f"post-integration shadow mismatch: {result['reconcile_after']}")
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if created:
            run(["docker", "rm", "-f", container], check=False)
        result["container_removed"] = run(["docker", "container", "inspect", container], check=False).returncode != 0
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" and result["container_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
