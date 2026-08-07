#!/usr/bin/env python3
"""Verify the whitelist outbox/projection/enforcement chain in owned PostgreSQL."""

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


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    args = parser.parse_args()
    suffix = hashlib.sha256(args.run_id.encode()).hexdigest()[:12]
    container = f"codex-whitelist-pipeline-pg-{suffix}"
    result: dict[str, object] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "container": container,
        "image": IMAGE,
        "status": "FAIL",
        "schema_files": 0,
        "sentinel_verified": False,
        "loopback_only": True,
        "shared_environment_touched": False,
        "persistent_volume_attached": False,
        "container_removed": False,
        "secrets_captured": False,
        "test_output": "",
        "errors": [],
    }
    created = False
    try:
        if run(["docker", "container", "inspect", container], check=False).returncode == 0:
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        run(["docker", "image", "inspect", IMAGE])
        run(
            [
                "docker", "run", "--name", container,
                "-e", "POSTGRES_PASSWORD=codex-ephemeral-only",
                "-e", "POSTGRES_DB=traffic_platform",
                "-p", "127.0.0.1::5432", "-d", IMAGE,
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

        schema_files = sorted((ROOT / "common/sql/pg").glob("*.sql"))
        for path in schema_files:
            run(
                [
                    "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i",
                    container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
                    "-d", "traffic_platform",
                ],
                input_bytes=path.read_bytes(),
            )
        result["schema_files"] = len(schema_files)
        run(
            [
                "docker", "exec", container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
                "-d", "traffic_platform", "-c",
                "CREATE TABLE codex_ephemeral_whitelist_event_pipeline_sentinel(marker text primary key); "
                "INSERT INTO codex_ephemeral_whitelist_event_pipeline_sentinel(marker) VALUES ('ephemeral-only');",
            ]
        )
        result["sentinel_verified"] = True
        port_output = run(["docker", "port", container, "5432/tcp"]).stdout.decode().strip()
        host_port = port_output.rsplit(":", 1)[-1]
        if not host_port.isdigit():
            raise RuntimeError("could not resolve ephemeral PostgreSQL loopback port")
        test_env = os.environ.copy()
        test_env["WHITELIST_EVENT_PIPELINE_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:codex-ephemeral-only@127.0.0.1:{host_port}/"
            "traffic_platform?sslmode=disable"
        )
        completed = run(
            [
                "go", "-C", "go/control-plane", "test", "./internal/rules/consumer",
                "-run", "TestWhitelistEventPipelineEphemeralPostgres", "-count=1", "-v",
            ],
            env=test_env,
            check=False,
        )
        result["test_output"] = completed.stdout.decode().strip()
        if completed.returncode != 0:
            raise RuntimeError(f"whitelist event pipeline integration exited {completed.returncode}")
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if created:
            run(["docker", "rm", "-f", container], check=False)
        result["container_removed"] = (
            run(["docker", "container", "inspect", container], check=False).returncode != 0
        )

    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" and result["container_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
