#!/usr/bin/env python3
"""Replay the asset expand bundle twice in an owned sentinel PostgreSQL."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
import time
from pathlib import Path
from typing import Any

from render_asset_postgres_expand import EXPECTED_TABLES, MIGRATION_DIR, MIGRATIONS, MIGRATION_VERSIONS


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
SENTINEL_TABLE = "codex_ephemeral_asset_expand_sentinel"
SENTINEL_VALUE = "ephemeral-only"
PASSWORD = "codex-asset-expand-ephemeral-only"
BASE_SCHEMA = f"""
CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";
CREATE TABLE tenants (
  tenant_id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO tenants(tenant_id,name) VALUES ('ephemeral-tenant','Ephemeral Asset Expand');
CREATE TABLE assets (
  asset_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE RESTRICT,
  ip TEXT NOT NULL
);
INSERT INTO assets(asset_id,tenant_id,ip)
VALUES ('11111111-1111-4111-8111-111111111111','ephemeral-tenant','192.0.2.10');
CREATE TABLE {SENTINEL_TABLE}(marker TEXT PRIMARY KEY);
INSERT INTO {SENTINEL_TABLE}(marker) VALUES ('{SENTINEL_VALUE}');
"""

FINGERPRINT_SQL = r"""
WITH records AS (
  SELECT format('column|%s|%s|%s|%s|%s', c.table_name, c.ordinal_position,
                c.column_name, c.udt_name, coalesce(c.column_default,'')) AS record
  FROM information_schema.columns c
  WHERE c.table_schema='public'
  UNION ALL
  SELECT format('constraint|%s|%s|%s', rel.relname, con.conname,
                pg_get_constraintdef(con.oid, true))
  FROM pg_constraint con JOIN pg_class rel ON rel.oid=con.conrelid
  JOIN pg_namespace ns ON ns.oid=rel.relnamespace WHERE ns.nspname='public'
  UNION ALL
  SELECT format('index|%s|%s', tablename, indexdef)
  FROM pg_indexes WHERE schemaname='public'
)
SELECT record FROM records ORDER BY record;
"""


def run(
    command: list[str],
    *,
    input_bytes: bytes | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command,
        cwd=ROOT,
        input=input_bytes,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=check,
    )


def psql(container: str, sql: str) -> str:
    completed = run(
        [
            "docker", "exec", "-i", container, "psql", "-X", "-v", "ON_ERROR_STOP=1",
            "-U", "postgres", "-d", "traffic_platform", "-At",
        ],
        input_bytes=sql.encode(),
    )
    return completed.stdout.decode().strip()


def sentinel(container: str) -> None:
    value = psql(container, f"SELECT marker FROM {SENTINEL_TABLE} LIMIT 1;")
    if value != SENTINEL_VALUE:
        raise RuntimeError(f"refusing database without asset expand sentinel: {value!r}")


def apply_migration(container: str, path: Path) -> None:
    sentinel(container)
    run(
        [
            "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i",
            container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
            "-d", "traffic_platform",
        ],
        input_bytes=path.read_bytes(),
    )


def schema_fingerprint(container: str) -> str:
    payload = psql(container, FINGERPRINT_SQL).encode()
    return hashlib.sha256(payload).hexdigest()


def container_name(run_id: str) -> str:
    if not run_id.strip():
        raise ValueError("run_id is required")
    return "codex-asset-expand-pg-" + hashlib.sha256(run_id.encode()).hexdigest()[:12]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    container = container_name(args.run_id)
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "container": container,
        "image": POSTGRES_IMAGE,
        "image_id": None,
        "status": "FAIL",
        "sentinel_verified": False,
        "migration_count": len(MIGRATIONS),
        "replay_count": 0,
        "schema_fingerprints": [],
        "migration_versions": [],
        "expected_table_count": len(EXPECTED_TABLES),
        "actual_table_count": 0,
        "baseline_backfill_verified": False,
        "persistent_volume_attached": False,
        "shared_environment_touched": False,
        "production_applied": False,
        "container_removed": False,
        "errors": [],
    }
    created = False
    try:
        if run(["docker", "container", "inspect", container], check=False).returncode == 0:
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        image = run(["docker", "image", "inspect", POSTGRES_IMAGE])
        inspected = json.loads(image.stdout)[0]
        result["image_id"] = inspected.get("Id")
        run(
            [
                "docker", "run", "--name", container,
                "-e", f"POSTGRES_PASSWORD={PASSWORD}",
                "-e", "POSTGRES_DB=traffic_platform",
                "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE,
            ]
        )
        created = True
        for _ in range(30):
            ready = run(
                ["docker", "exec", container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"],
                check=False,
            )
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")

        psql(container, BASE_SCHEMA)
        sentinel(container)
        result["sentinel_verified"] = True

        for replay in range(1, 3):
            for name in MIGRATIONS:
                apply_migration(container, MIGRATION_DIR / name)
            result["replay_count"] = replay
            result["schema_fingerprints"].append(schema_fingerprint(container))

        if len(set(result["schema_fingerprints"])) != 1:
            raise RuntimeError("asset schema fingerprint changed during second migration replay")
        versions = psql(
            container,
            "SELECT version FROM alignment_schema_migrations "
            + "WHERE version IN ("
            + ",".join(f"'{version}'" for version in MIGRATION_VERSIONS)
            + ") ORDER BY version;",
        ).splitlines()
        result["migration_versions"] = versions
        if versions != list(MIGRATION_VERSIONS):
            raise RuntimeError(f"asset migration registry mismatch: {versions}")

        table_count = int(
            psql(
                container,
                "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename IN ("
                + ",".join(f"'{name}'" for name in EXPECTED_TABLES)
                + ");",
            )
        )
        result["actual_table_count"] = table_count
        if table_count != len(EXPECTED_TABLES):
            raise RuntimeError(f"asset table count mismatch: {table_count} != {len(EXPECTED_TABLES)}")

        backfill = psql(
            container,
            "SELECT ip_address,revision,lifecycle_state FROM assets "
            "WHERE asset_id='11111111-1111-4111-8111-111111111111';",
        )
        if backfill != "192.0.2.10|1|managed":
            raise RuntimeError(f"asset baseline backfill mismatch: {backfill!r}")
        result["baseline_backfill_verified"] = True

        mutation_rows = psql(
            container,
            "SELECT (SELECT count(*) FROM asset_event_outbox),"
            "(SELECT count(*) FROM asset_projection_inbox),"
            "(SELECT count(*) FROM asset_discovery_outbox),"
            "(SELECT count(*) FROM asset_export_outbox),"
            "(SELECT count(*) FROM asset_governance_outbox);",
        )
        if mutation_rows != "0|0|0|0|0":
            raise RuntimeError(f"expand unexpectedly created business outbox rows: {mutation_rows}")
        result["status"] = "PASS"
    except Exception as exc:  # Fail closed and preserve the cleanup result.
        result["errors"] = [str(exc)]
    finally:
        if created:
            run(["docker", "rm", "-f", container], check=False)
        result["container_removed"] = (
            run(["docker", "container", "inspect", container], check=False).returncode != 0
        )

    payload = json.dumps(result, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite asset expand G1 evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and result["container_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
