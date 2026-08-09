#!/usr/bin/env python3
"""Verify an asset export manifest against owned PostgreSQL and MinIO containers."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
MINIO_IMAGE = "quay.io/minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
SENTINEL_TABLE = "codex_ephemeral_asset_atomic_test_sentinel"
SENTINEL_VALUE = "ephemeral-only"
POSTGRES_PASSWORD = "codex-asset-export-minio-ephemeral-only"
MINIO_ACCESS_KEY = "codexassetexport"
MINIO_SECRET_KEY = "codex-asset-export-minio-ephemeral-secret"
MINIO_BUCKET = "asset-export-g1"


def run(
    command: list[str],
    *,
    input_bytes: bytes | None = None,
    env: dict[str, str] | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command,
        cwd=ROOT,
        input=input_bytes,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=check,
    )


def loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def names(run_id: str) -> tuple[str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return f"codex-asset-export-minio-pg-{suffix}", f"codex-asset-export-minio-store-{suffix}"


def container_absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    postgres_container, minio_container = names(args.run_id)
    minio_port = loopback_port()
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "postgres_container": postgres_container,
        "minio_container": minio_container,
        "postgres_image": POSTGRES_IMAGE,
        "minio_image": MINIO_IMAGE,
        "postgres_image_id": None,
        "minio_image_id": None,
        "schema_files": 0,
        "postgres_sentinel_verified": False,
        "minio_health_verified": False,
        "same_trace_reconciliation_verified": False,
        "manifest_sha256_verified": False,
        "object_stat_verified": False,
        "audit_and_outbox_verified": False,
        "loopback_only": True,
        "persistent_volume_attached": False,
        "shared_environment_touched": False,
        "production_applied": False,
        "postgres_container_removed": False,
        "minio_container_removed": False,
        "test_output": "",
        "errors": [],
    }
    postgres_created = False
    minio_created = False
    try:
        for container in (postgres_container, minio_container):
            if not container_absent(container):
                raise RuntimeError(f"refusing to reuse existing container: {container}")
        postgres_image = json.loads(run(["docker", "image", "inspect", POSTGRES_IMAGE]).stdout)[0]
        minio_image = json.loads(run(["docker", "image", "inspect", MINIO_IMAGE]).stdout)[0]
        result["postgres_image_id"] = postgres_image.get("Id")
        result["minio_image_id"] = minio_image.get("Id")

        run([
            "docker", "run", "--name", minio_container,
            "-e", f"MINIO_ROOT_USER={MINIO_ACCESS_KEY}",
            "-e", f"MINIO_ROOT_PASSWORD={MINIO_SECRET_KEY}",
            "-p", f"127.0.0.1:{minio_port}:9000", "-d", MINIO_IMAGE,
            "server", "/data", "--console-address", ":9001",
        ])
        minio_created = True
        minio_endpoint = f"http://127.0.0.1:{minio_port}"
        for _ in range(60):
            try:
                with urllib.request.urlopen(minio_endpoint + "/minio/health/ready", timeout=2) as response:
                    if response.status == 200:
                        break
            except (OSError, urllib.error.URLError):
                pass
            time.sleep(1)
        else:
            logs = run(["docker", "logs", "--tail", "100", minio_container], check=False)
            raise RuntimeError("ephemeral MinIO did not become ready: " + logs.stdout.decode(errors="replace")[-8192:])
        result["minio_health_verified"] = True

        run([
            "docker", "run", "--name", postgres_container,
            "-e", f"POSTGRES_PASSWORD={POSTGRES_PASSWORD}", "-e", "POSTGRES_DB=traffic_platform",
            "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE,
        ])
        postgres_created = True
        for _ in range(30):
            ready = run([
                "docker", "exec", postgres_container, "pg_isready",
                "-U", "postgres", "-d", "traffic_platform",
            ], check=False)
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")
        schema_files = sorted((ROOT / "common/sql/pg").glob("*.sql"))
        for path in schema_files:
            run([
                "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i",
                postgres_container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
                "-d", "traffic_platform",
            ], input_bytes=path.read_bytes())
        result["schema_files"] = len(schema_files)
        run([
            "docker", "exec", postgres_container, "psql", "-X", "-v", "ON_ERROR_STOP=1",
            "-U", "postgres", "-d", "traffic_platform", "-c",
            f"CREATE TABLE {SENTINEL_TABLE}(marker text primary key); "
            f"INSERT INTO {SENTINEL_TABLE}(marker) VALUES ('{SENTINEL_VALUE}');",
        ])
        result["postgres_sentinel_verified"] = True
        port_output = run(["docker", "port", postgres_container, "5432/tcp"]).stdout.decode().strip()
        postgres_port = port_output.rsplit(":", 1)[-1]
        if not postgres_port.isdigit():
            raise RuntimeError(f"invalid loopback PostgreSQL port mapping: {port_output!r}")

        test_env = os.environ.copy()
        test_env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:{POSTGRES_PASSWORD}@127.0.0.1:{postgres_port}/traffic_platform?sslmode=disable"
        )
        test_env["ASSET_EXPORT_EPHEMERAL_S3_ENDPOINT"] = minio_endpoint
        test_env["ASSET_EXPORT_EPHEMERAL_S3_ACCESS_KEY"] = MINIO_ACCESS_KEY
        test_env["ASSET_EXPORT_EPHEMERAL_S3_SECRET_KEY"] = MINIO_SECRET_KEY
        test_env["ASSET_EXPORT_EPHEMERAL_S3_BUCKET"] = MINIO_BUCKET
        completed = run([
            "go", "-C", "go/control-plane", "test", "./internal/asset/service",
            "-run", "^TestAssetExportRealMinIOArtifactLifecycle$", "-count=1", "-v",
        ], env=test_env, check=False)
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            raise RuntimeError(f"asset MinIO export integration exited {completed.returncode}")
        result["same_trace_reconciliation_verified"] = True
        result["manifest_sha256_verified"] = True
        result["object_stat_verified"] = True
        result["audit_and_outbox_verified"] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if postgres_created:
            run(["docker", "rm", "-f", postgres_container], check=False)
        if minio_created:
            run(["docker", "rm", "-f", minio_container], check=False)
        result["postgres_container_removed"] = container_absent(postgres_container)
        result["minio_container_removed"] = container_absent(minio_container)

    payload = json.dumps(result, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite asset MinIO G1 evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if (
        result["status"] == "PASS"
        and result["postgres_container_removed"]
        and result["minio_container_removed"]
    ) else 1


if __name__ == "__main__":
    raise SystemExit(main())
