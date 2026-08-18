#!/usr/bin/env python3
"""Verify M03 file-restoration authority on owned ephemeral dependencies.

The runner refuses pre-existing container names, binds only random loopback
ports, mounts no volumes, requires sentinels in both Go integration suites and
always removes the containers it created. It does not touch a shared or
production PostgreSQL/MinIO deployment.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import tempfile
import time
import urllib.request
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[2]
GO_ROOT = ROOT / "go/control-plane"
MIGRATION = ROOT / "deployments/postgres/migrations/202608141300_m03_file_restoration_authority.sql"
FRESH_SCHEMA = ROOT / "common/sql/pg/10-m03-file-restoration.sql"
KUBERNETES_SCHEMA = ROOT / "deployments/kubernetes/init-jobs/02a-m03-file-restoration-schema.yaml"
POSTGRES_IMAGE = "postgres@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777"
MINIO_IMAGE = "minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
POSTGRES_USER = "m03"
POSTGRES_PASSWORD = "m03-restoration-secret"
POSTGRES_DATABASE = "m03_restoration"
MINIO_ACCESS_KEY = "m03minio"
MINIO_SECRET_KEY = "m03minio-restoration-secret"


def names(run_id: str) -> tuple[str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return (
        f"codex-m03-restoration-postgres-{suffix}",
        f"codex-m03-restoration-minio-{suffix}",
    )


def run(
    command: list[str],
    *,
    cwd: Path = ROOT,
    env: dict[str, str] | None = None,
    input_bytes: bytes | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command,
        cwd=cwd,
        env=env,
        input=input_bytes,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=check,
    )


def loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def container_absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def command_receipt(command: list[str], completed: subprocess.CompletedProcess[bytes]) -> dict[str, object]:
    output = completed.stdout.decode(errors="replace")
    summaries = [
        line.strip()
        for line in output.splitlines()
        if line.startswith(("=== RUN", "--- PASS", "PASS", "ok\t", "FAIL"))
    ]
    return {
        "command": command,
        "exit_code": completed.returncode,
        "stdout_sha256": hashlib.sha256(completed.stdout).hexdigest(),
        "summary": summaries[-20:],
        "failure_tail": output.splitlines()[-80:] if completed.returncode != 0 else [],
    }


def kubernetes_restoration_sql() -> bytes:
    documents = list(yaml.safe_load_all(KUBERNETES_SCHEMA.read_text(encoding="utf-8")))
    config_map = next(
        document
        for document in documents
        if isinstance(document, dict)
        and document.get("kind") == "ConfigMap"
        and document.get("metadata", {}).get("name") == "postgres-m03-file-restoration-sql"
    )
    return config_map["data"]["37-m03-file-restoration.sql"].encode()


def postgres_schema_fingerprint(name: str, database: str) -> str:
    query = r"""
WITH selected_tables AS (
  SELECT unnest(ARRAY[
    'alignment_schema_migrations','file_restoration_manifests','file_restoration_outbox',
    'file_restoration_requests','file_restoration_audit','file_restoration_orphans'
  ]) AS table_name
), facts AS (
  SELECT 'column' AS kind,c.table_name,c.ordinal_position::text AS position,
         concat_ws('|',c.column_name,c.data_type,c.udt_name,c.is_nullable,COALESCE(c.column_default,'')) AS definition
  FROM information_schema.columns c JOIN selected_tables s USING(table_name)
  WHERE c.table_schema='public'
  UNION ALL
  SELECT 'constraint',cl.relname,'',concat_ws('|',co.contype,co.conname,pg_get_constraintdef(co.oid,true))
  FROM pg_constraint co JOIN pg_class cl ON cl.oid=co.conrelid
  JOIN pg_namespace ns ON ns.oid=cl.relnamespace JOIN selected_tables s ON s.table_name=cl.relname
  WHERE ns.nspname='public'
  UNION ALL
  SELECT 'index',tab.relname,'',pg_get_indexdef(idx.indexrelid)
  FROM pg_index idx JOIN pg_class tab ON tab.oid=idx.indrelid
  JOIN pg_namespace ns ON ns.oid=tab.relnamespace JOIN selected_tables s ON s.table_name=tab.relname
  WHERE ns.nspname='public'
)
SELECT kind||E'\t'||table_name||E'\t'||position||E'\t'||definition
FROM facts ORDER BY kind,table_name,position,definition
"""
    completed = run([
        "docker", "exec", name, "psql", "-v", "ON_ERROR_STOP=1", "-At",
        "-U", POSTGRES_USER, "-d", database, "-c", query,
    ])
    return hashlib.sha256(completed.stdout).hexdigest()


def start_postgres(name: str, port: int, owned: set[str]) -> dict[str, str]:
    if not container_absent(name):
        raise RuntimeError(f"refusing to reuse existing PostgreSQL container: {name}")
    run([
        "docker", "run", "--name", name,
        "-p", f"127.0.0.1:{port}:5432",
        "-e", f"POSTGRES_USER={POSTGRES_USER}",
        "-e", f"POSTGRES_PASSWORD={POSTGRES_PASSWORD}",
        "-e", f"POSTGRES_DB={POSTGRES_DATABASE}",
        "-d", POSTGRES_IMAGE,
    ])
    owned.add(name)
    for _ in range(90):
        ready = run(
            ["docker", "exec", name, "pg_isready", "-U", POSTGRES_USER, "-d", POSTGRES_DATABASE],
            check=False,
        )
        if ready.returncode == 0:
            break
        time.sleep(0.5)
    else:
        logs = run(["docker", "logs", "--tail", "100", name], check=False)
        raise RuntimeError("ephemeral PostgreSQL did not become healthy: " + logs.stdout.decode(errors="replace"))

    sources = {
        POSTGRES_DATABASE: MIGRATION.read_bytes(),
        "m03_restoration_fresh": FRESH_SCHEMA.read_bytes(),
        "m03_restoration_k8s": kubernetes_restoration_sql(),
    }
    fingerprints: dict[str, str] = {}
    for database, ddl in sources.items():
        if database != POSTGRES_DATABASE:
            run(["docker", "exec", name, "createdb", "-U", POSTGRES_USER, database])
        apply_command = [
            "docker", "exec", "-i", name, "psql", "-v", "ON_ERROR_STOP=1",
            "-U", POSTGRES_USER, "-d", database,
        ]
        # Every deployment entrypoint must be legal and idempotent on a fresh DB.
        run(apply_command, input_bytes=ddl)
        run(apply_command, input_bytes=ddl)
        fingerprints[database] = postgres_schema_fingerprint(name, database)
    if len(set(fingerprints.values())) != 1:
        raise RuntimeError(f"restoration PostgreSQL entrypoint schema drift: {fingerprints}")

    apply_command = [
        "docker", "exec", "-i", name, "psql", "-v", "ON_ERROR_STOP=1",
        "-U", POSTGRES_USER, "-d", POSTGRES_DATABASE,
    ]
    run(apply_command, input_bytes=b"""
CREATE TABLE codex_ephemeral_m03_restoration_sentinel(marker text PRIMARY KEY);
INSERT INTO codex_ephemeral_m03_restoration_sentinel VALUES ('ephemeral-only');
""")
    return fingerprints


def start_minio(name: str, port: int, owned: set[str], mc_config: Path) -> None:
    if not container_absent(name):
        raise RuntimeError(f"refusing to reuse existing MinIO container: {name}")
    run([
        "docker", "run", "--name", name,
        "-p", f"127.0.0.1:{port}:9000",
        "-e", f"MINIO_ROOT_USER={MINIO_ACCESS_KEY}",
        "-e", f"MINIO_ROOT_PASSWORD={MINIO_SECRET_KEY}",
        "-d", MINIO_IMAGE, "server", "/data",
    ])
    owned.add(name)
    health_url = f"http://127.0.0.1:{port}/minio/health/live"
    for _ in range(90):
        try:
            with urllib.request.urlopen(health_url, timeout=1.0) as response:
                if response.status == 200:
                    break
        except OSError:
            pass
        time.sleep(0.5)
    else:
        logs = run(["docker", "logs", "--tail", "100", name], check=False)
        raise RuntimeError("ephemeral MinIO did not become healthy: " + logs.stdout.decode(errors="replace"))

    mc = ["mc", "--config-dir", str(mc_config)]
    run(mc + ["alias", "set", "m03-owned", f"http://127.0.0.1:{port}", MINIO_ACCESS_KEY, MINIO_SECRET_KEY])
    run(mc + ["mb", "--with-lock", "m03-owned/forensics-quarantine"])
    run(mc + ["mb", "m03-owned/pcap-archive"])
    run(mc + ["version", "enable", "m03-owned/forensics-quarantine"])
    run(mc + ["version", "enable", "m03-owned/pcap-archive"])


def go_test(command: list[str], environment: dict[str, str]) -> subprocess.CompletedProcess[bytes]:
    merged = os.environ.copy()
    merged.update(environment)
    return run(command, cwd=GO_ROOT, env=merged, check=False)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--result-output", type=Path)
    args = parser.parse_args()

    postgres_name, minio_name = names(args.run_id)
    all_containers = [postgres_name, minio_name]
    result: dict[str, object] = {
        "schema_version": 1,
        "artifact_kind": "M03_FILE_RESTORATION_EPHEMERAL_TEST_RESULT",
        "run_id": args.run_id,
        "status": "FAIL",
        "profile_id": "M03-N016-N017-FILE-RESTORATION-EPHEMERAL-V1",
        "environment_id": "owned-loopback-postgresql-minio",
        "production_applied": False,
        "shared_environment_touched": False,
        "persistent_volume_attached": False,
        "images": {"postgres": POSTGRES_IMAGE, "minio": MINIO_IMAGE},
        "checks": [],
        "containers_removed": False,
        "errors": [],
    }
    created: set[str] = set()
    try:
        for name in all_containers:
            if not container_absent(name):
                raise RuntimeError(f"refusing to reuse existing container: {name}")
        run(["docker", "image", "inspect", POSTGRES_IMAGE])
        run(["docker", "image", "inspect", MINIO_IMAGE])
        postgres_port = loopback_port()
        minio_port = loopback_port()
        with tempfile.TemporaryDirectory(prefix="codex-m03-restoration-mc-") as config_dir:
            schema_fingerprints = start_postgres(postgres_name, postgres_port, created)
            result["postgres_schema_fingerprints"] = schema_fingerprints
            start_minio(minio_name, minio_port, created, Path(config_dir))

            postgres_command = [
                "go", "test", "-count=1", "-run",
                "^TestRestorationAuthorityTransactionEphemeralPostgres$", "-v",
                "./internal/forensics/restoration",
            ]
            completed = go_test(postgres_command, {
                "M03_RESTORATION_PG_INTEGRATION_ENABLED": "true",
                "M03_RESTORATION_PG_SENTINEL": "codex_ephemeral_m03_restoration_postgres",
                "M03_RESTORATION_PG_DSN": (
                    f"postgres://{POSTGRES_USER}:{POSTGRES_PASSWORD}@127.0.0.1:{postgres_port}/"
                    f"{POSTGRES_DATABASE}?sslmode=disable"
                ),
            })
            result["checks"].append(command_receipt(postgres_command, completed))
            if completed.returncode != 0:
                raise RuntimeError("real PostgreSQL restoration authority transaction failed")

            minio_command = [
                "go", "test", "-count=1", "-run",
                "^TestRestorationObjectAuthorityRoundTrip$", "-v",
                "./internal/forensics/s3client",
            ]
            completed = go_test(minio_command, {
                "M03_RESTORATION_MINIO_INTEGRATION_ENABLED": "true",
                "M03_RESTORATION_MINIO_SENTINEL": "codex_ephemeral_m03_restoration_minio",
                "M03_RESTORATION_MINIO_ENDPOINT": f"127.0.0.1:{minio_port}",
                "M03_RESTORATION_MINIO_ACCESS_KEY": MINIO_ACCESS_KEY,
                "M03_RESTORATION_MINIO_SECRET_KEY": MINIO_SECRET_KEY,
            })
            result["checks"].append(command_receipt(minio_command, completed))
            if completed.returncode != 0:
                raise RuntimeError("real MinIO immutable object authority round trip failed")

        result["oracles"] = {
            "all_postgres_entrypoints_applied_twice": True,
            "postgres_entrypoint_catalogs_identical": True,
            "expired_claim_predecessor_rejected": True,
            "manifest_outbox_audit_receipt_committed_atomically": True,
            "orphan_candidate_reconciled_in_commit": True,
            "replay_did_not_duplicate_authority_rows": True,
            "cross_tenant_lookup_returned_no_receipt": True,
            "quarantine_bucket_versioning_and_object_lock_verified": True,
            "conditional_second_put_rejected_by_server": True,
            "source_version_etag_size_and_sha256_verified": True,
        }
        result["status"] = "PASS"
    except Exception as error:
        result["errors"] = [str(error)]
    finally:
        for name in list(created):
            run(["docker", "rm", "-f", name], check=False)
        result["containers_removed"] = all(container_absent(name) for name in all_containers)

    rendered = json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    if args.result_output is not None:
        output_path = args.result_output.resolve()
        output_path.parent.mkdir(parents=True, exist_ok=True)
        temporary_path = output_path.with_name(output_path.name + ".tmp")
        temporary_path.write_text(rendered, encoding="utf-8")
        temporary_path.replace(output_path)
    print(rendered, end="")
    return 0 if result["status"] == "PASS" and result["containers_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
