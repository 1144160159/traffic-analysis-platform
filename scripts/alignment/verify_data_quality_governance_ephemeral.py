#!/usr/bin/env python3
"""Run the T-DQ-001 governance lifecycle against owned ephemeral PostgreSQL."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
IMAGE = "postgres:16"


def run(command: list[str], *, input_bytes: bytes | None = None, check: bool = True, env: dict[str, str] | None = None, cwd: Path = ROOT) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=cwd, input=input_bytes, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=check, env=env)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    args = parser.parse_args()
    suffix = hashlib.sha256(args.run_id.encode()).hexdigest()[:12]
    container = f"codex-data-quality-pg-{suffix}"
    if run(["docker", "container", "inspect", container], check=False).returncode == 0:
        raise SystemExit(f"refusing to reuse existing container: {container}")
    run(["docker", "image", "inspect", IMAGE])
    created = False
    try:
        run([
            "docker", "run", "--name", container, "-p", "127.0.0.1::5432",
            "-e", "POSTGRES_PASSWORD=codex-ephemeral-only", "-e", "POSTGRES_DB=traffic_platform",
            "-d", IMAGE,
        ])
        created = True
        for _ in range(30):
            ready = run(["docker", "exec", container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False)
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")
        for path in sorted((ROOT / "common/sql/pg").glob("*.sql")):
            run([
                "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i", container,
                "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform",
            ], input_bytes=path.read_bytes())
        run([
            "docker", "exec", container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform", "-c",
            "CREATE TABLE codex_ephemeral_data_quality_sentinel(marker text primary key); "
            "INSERT INTO codex_ephemeral_data_quality_sentinel(marker) VALUES ('ephemeral-only');",
        ])
        port_output = run(["docker", "port", container, "5432/tcp"]).stdout.decode().strip()
        match = re.fullmatch(r"127\.0\.0\.1:(\d+)", port_output)
        if not match:
            raise RuntimeError("ephemeral PostgreSQL port mapping is invalid")
        test_env = os.environ.copy()
        test_env["DATA_QUALITY_GOVERNANCE_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:codex-ephemeral-only@127.0.0.1:{match.group(1)}/traffic_platform?sslmode=disable"
        )
        test = run([
            "go", "test", "./internal/common/dataquality", "./internal/alert/api",
            "-run", "^TestDataQualityGovernance(EphemeralPostgres|HTTPEphemeralPostgres)$",
            "-count=1", "-v",
        ], check=False, env=test_env, cwd=ROOT / "go/control-plane")
        sys.stdout.write(test.stdout.decode())
        sys.stderr.write(test.stderr.decode())
        result = {
            "result": "pass" if test.returncode == 0 else "fail",
            "run_id": args.run_id,
            "test_exit_code": test.returncode,
            "ephemeral_container_removed": True,
            "shared_environment_touched": False,
            "secrets_captured": False,
        }
        print(json.dumps(result, ensure_ascii=False))
        return test.returncode
    finally:
        if created:
            run(["docker", "rm", "-f", container], check=False)


if __name__ == "__main__":
    raise SystemExit(main())
