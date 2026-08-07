#!/usr/bin/env python3
"""Verify asset fact and watermark semantics in an owned ClickHouse container."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import socket
import subprocess
import time
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
IMAGE = "docker.io/clickhouse/clickhouse-server@sha256:458f72c00e4abc80c2950d8070bc5723b538544d72ea4a6a58d9ff4f8fadf8d7"
SCHEMA_AUTHORITY = ROOT / "common/sql/ch/00-all-tables.sql"
SENTINEL_VALUE = "ephemeral-only"
EPHEMERAL_USER = "codex_ephemeral"
EPHEMERAL_PASSWORD = "codex-ephemeral-only"


def run(
    command: list[str], *, env: dict[str, str] | None = None,
    input_bytes: bytes | None = None, check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command, cwd=ROOT, env=env, input=input_bytes,
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=check,
    )


def names(run_id: str) -> tuple[str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return (
        f"codex-asset-detail-clickhouse-net-{suffix}",
        f"codex-asset-detail-clickhouse-{suffix}",
    )


def container_absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def network_absent(name: str) -> bool:
    return run(["docker", "network", "inspect", name], check=False).returncode != 0


def mapped_port(container: str, port: int) -> int:
    output = run(["docker", "port", container, f"{port}/tcp"]).stdout.decode().strip()
    value = output.rsplit(":", 1)[-1]
    if not value.isdigit():
        raise RuntimeError(f"invalid loopback port mapping for {container}: {output!r}")
    return int(value)


def wait_tcp(port: int, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=1):
                return
        except OSError:
            time.sleep(0.5)
    raise RuntimeError(f"loopback port {port} did not become ready within {timeout}s")


def standalone_table_ddl(source: str, table: str) -> str:
    pattern = re.compile(
        rf"CREATE TABLE IF NOT EXISTS traffic\.{re.escape(table)}_local ON CLUSTER traffic_cluster \(.*?\n"
        rf"SETTINGS index_granularity = 8192;",
        re.DOTALL,
    )
    matches = pattern.findall(source)
    if len(matches) != 1:
        raise ValueError(f"expected one canonical {table}_local DDL, found {len(matches)}")
    ddl = matches[0]
    ddl = ddl.replace(
        f"CREATE TABLE IF NOT EXISTS traffic.{table}_local ON CLUSTER traffic_cluster",
        f"CREATE TABLE traffic.{table}",
        1,
    )
    ddl, replacements = re.subn(
        r"ENGINE = ReplicatedMergeTree\('[^']+', '\{replica\}'\)",
        "ENGINE = MergeTree",
        ddl,
        count=1,
    )
    if replacements != 1:
        raise ValueError(f"canonical {table} DDL has no expected ReplicatedMergeTree engine")
    if " ON CLUSTER " in ddl or "Replicated" in ddl or "Distributed" in ddl:
        raise ValueError(f"standalone {table} DDL retained clustered engine syntax")
    return ddl


def bootstrap_sql() -> tuple[str, str]:
    source_bytes = SCHEMA_AUTHORITY.read_bytes()
    source = source_bytes.decode("utf-8")
    schema_hash = hashlib.sha256(source_bytes).hexdigest()
    statements = [
        "CREATE DATABASE IF NOT EXISTS traffic;",
        standalone_table_ddl(source, "sessions"),
        standalone_table_ddl(source, "alerts"),
        "CREATE TABLE traffic.codex_ephemeral_asset_detail_sentinel (marker String) ENGINE=Memory;",
        f"INSERT INTO traffic.codex_ephemeral_asset_detail_sentinel VALUES ('{SENTINEL_VALUE}');",
    ]
    return "\n".join(statements) + "\n", schema_hash


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    network, container = names(args.run_id)
    result: dict[str, Any] = {
        "schema_version": 1, "run_id": args.run_id, "status": "FAIL",
        "network": network, "container": container, "image": IMAGE,
        "image_id": "", "schema_authority": str(SCHEMA_AUTHORITY.relative_to(ROOT)),
        "schema_authority_sha256": "", "canonical_schema_derived": False,
        "sentinel_verified": False, "real_reader_verified": False,
        "tenant_isolation_verified": False, "as_of_upper_bound_verified": False,
        "latest_alert_state_verified": False, "hard_limit_verified": False,
        "source_watermarks_verified": False, "unavailable_source_fails_closed": False,
        "loopback_only_native_endpoint": True, "persistent_volume_attached": False,
        "shared_environment_touched": False, "production_applied": False,
        "container_removed": False, "network_removed": False,
        "test_output": "", "errors": [],
    }
    network_created = False
    container_created = False
    try:
        if not network_absent(network):
            raise RuntimeError(f"refusing to reuse existing network: {network}")
        if not container_absent(container):
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        inspected = json.loads(run(["docker", "image", "inspect", IMAGE]).stdout)[0]
        result["image_id"] = inspected.get("Id", "")
        sql, result["schema_authority_sha256"] = bootstrap_sql()
        result["canonical_schema_derived"] = True

        run(["docker", "network", "create", "--driver", "bridge", network])
        network_created = True
        run([
            "docker", "run", "--name", container, "--network", network,
            "--ulimit", "nofile=262144:262144", "-p", "127.0.0.1::9000",
            "-e", f"CLICKHOUSE_USER={EPHEMERAL_USER}",
            "-e", f"CLICKHOUSE_PASSWORD={EPHEMERAL_PASSWORD}",
            "-d", IMAGE,
        ])
        container_created = True
        native_port = mapped_port(container, 9000)
        wait_tcp(native_port, 60)

        deadline = time.monotonic() + 60
        bootstrap = None
        while time.monotonic() < deadline:
            bootstrap = run(
                [
                    "docker", "exec", "-i", container, "clickhouse-client",
                    "--user", EPHEMERAL_USER, "--password", EPHEMERAL_PASSWORD, "--multiquery",
                ],
                input_bytes=sql.encode(), check=False,
            )
            if bootstrap.returncode == 0:
                break
            time.sleep(1)
        if bootstrap is None or bootstrap.returncode != 0:
            output = "" if bootstrap is None else bootstrap.stdout.decode(errors="replace")
            raise RuntimeError(f"ClickHouse schema bootstrap failed: {output[-4096:]}")
        result["sentinel_verified"] = True

        test_env = os.environ.copy()
        test_env["ASSET_DETAIL_EPHEMERAL_CLICKHOUSE_DSN"] = (
            f"clickhouse://{EPHEMERAL_USER}:{EPHEMERAL_PASSWORD}@127.0.0.1:{native_port}/traffic"
            "?dial_timeout=5s&read_timeout=10s"
        )
        test_env["ASSET_DETAIL_EPHEMERAL_CLICKHOUSE_SENTINEL"] = SENTINEL_VALUE
        completed = run([
            "go", "-C", "go/control-plane", "test", "./internal/asset/repository",
            "-run", "^TestAssetDetailRealClickHouseFactsWatermarksAndFailureBoundary$",
            "-count=1", "-v",
        ], env=test_env, check=False)
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            logs = run(["docker", "logs", "--tail", "120", container], check=False)
            raise RuntimeError(
                f"asset ClickHouse integration exited {completed.returncode}\n"
                + logs.stdout.decode(errors="replace")[-8192:]
            )
        for field in (
            "real_reader_verified", "tenant_isolation_verified", "as_of_upper_bound_verified",
            "latest_alert_state_verified", "hard_limit_verified", "source_watermarks_verified",
            "unavailable_source_fails_closed",
        ):
            result[field] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if container_created:
            run(["docker", "rm", "-f", container], check=False)
        if network_created:
            run(["docker", "network", "rm", network], check=False)
        result["container_removed"] = container_absent(container)
        result["network_removed"] = network_absent(network)

    payload = json.dumps(result, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite asset ClickHouse G1 evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and result["container_removed"] and result["network_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
