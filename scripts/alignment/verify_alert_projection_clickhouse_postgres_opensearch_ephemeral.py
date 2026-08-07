#!/usr/bin/env python3
"""Verify alert projection receipts across owned ClickHouse, PG and OS."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import socket
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CLICKHOUSE_IMAGE = "docker.io/clickhouse/clickhouse-server@sha256:458f72c00e4abc80c2950d8070bc5723b538544d72ea4a6a58d9ff4f8fadf8d7"
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
OPENSEARCH_IMAGE = "docker.io/opensearchproject/opensearch@sha256:466a49f379bb8889af29d615475e69b7b990898c6987d28470cd7105df9046ff"
CLICKHOUSE_SCHEMA = Path("common/sql/ch/00-all-tables.sql")
POSTGRES_MIGRATION = Path("deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql")
ALERT_MAPPING = Path("common/opensearch/alerts-v2/mappings-component.json")
SENTINEL = "ephemeral-only"
CLICKHOUSE_USER = "codex_ephemeral"
CLICKHOUSE_PASSWORD = "codex-ephemeral-only"


def run(command: list[str], *, input_bytes: bytes | None = None, env: dict[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, input=input_bytes, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=check)


def names(run_id: str) -> tuple[str, str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    digest = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return tuple(f"codex-alert-three-store-{kind}-{digest}" for kind in ("ch", "pg", "os"))


def absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def mapped_port(container: str, port: str) -> int:
    output = run(["docker", "port", container, port]).stdout.decode().strip()
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


def request(base_url: str, method: str, path: str, body: Any | None = None) -> dict[str, Any]:
    payload = None if body is None else json.dumps(body, separators=(",", ":")).encode()
    req = urllib.request.Request(base_url + path, data=payload, method=method, headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            content = response.read()
    except urllib.error.HTTPError as exc:
        content = exc.read()
        raise RuntimeError(f"OpenSearch {method} {path} returned {exc.code}: {content.decode(errors='replace')[:4096]}") from exc
    return json.loads(content) if content else {}


def clickhouse_bootstrap() -> tuple[bytes, str]:
    source_bytes = (ROOT / CLICKHOUSE_SCHEMA).read_bytes()
    source = source_bytes.decode()
    pattern = re.compile(
        r"CREATE TABLE IF NOT EXISTS traffic\.alerts_local ON CLUSTER traffic_cluster \(.*?\n"
        r"SETTINGS index_granularity = 8192;",
        re.DOTALL,
    )
    matches = pattern.findall(source)
    if len(matches) != 1:
        raise RuntimeError(f"expected one canonical alerts_local DDL, found {len(matches)}")
    alerts = matches[0].replace(
        "CREATE TABLE IF NOT EXISTS traffic.alerts_local ON CLUSTER traffic_cluster",
        "CREATE TABLE traffic.alerts",
        1,
    )
    alerts, count = re.subn(
        r"ENGINE = ReplicatedMergeTree\('[^']+', '\{replica\}'\)",
        "ENGINE = MergeTree",
        alerts,
        count=1,
    )
    if count != 1 or " ON CLUSTER " in alerts or "Replicated" in alerts:
        raise RuntimeError("canonical alerts DDL could not be made standalone")
    sql = "\n".join([
        "CREATE DATABASE IF NOT EXISTS traffic;",
        alerts,
        "CREATE TABLE traffic.alerts_latest AS traffic.alerts ENGINE=ReplacingMergeTree(updated_at) ORDER BY (tenant_id,alert_id);",
        "CREATE MATERIALIZED VIEW traffic.mv_alerts_latest TO traffic.alerts_latest AS SELECT * FROM traffic.alerts;",
        "CREATE TABLE traffic.codex_ephemeral_alert_reconcile_sentinel(marker String) ENGINE=Memory;",
        f"INSERT INTO traffic.codex_ephemeral_alert_reconcile_sentinel VALUES ('{SENTINEL}');",
    ]) + "\n"
    return sql.encode(), hashlib.sha256(source_bytes).hexdigest()


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    ch_container, pg_container, os_container = names(args.run_id)
    result: dict[str, Any] = {
        "schema_version": 1, "run_id": args.run_id, "status": "FAIL",
        "clickhouse_container": ch_container, "postgres_container": pg_container, "opensearch_container": os_container,
        "clickhouse_image": CLICKHOUSE_IMAGE, "postgres_image": POSTGRES_IMAGE, "opensearch_image": OPENSEARCH_IMAGE,
        "clickhouse_image_id": None, "postgres_image_id": None, "opensearch_image_id": None,
        "clickhouse_schema": CLICKHOUSE_SCHEMA.as_posix(), "clickhouse_schema_sha256": None,
        "postgres_migration": POSTGRES_MIGRATION.as_posix(), "alert_mapping": ALERT_MAPPING.as_posix(),
        "clickhouse_sentinel_verified": False, "postgres_sentinel_verified": False, "opensearch_sentinel_verified": False,
        "production_clickhouse_writer_verified": False, "production_clickhouse_authority_verified": False,
        "missing_stale_extra_seed_verified": False, "opensearch_terminal_requery_verified": False,
        "postgres_watermarks_converged": False, "three_store_hash_and_version_reconciliation_verified": False,
        "durable_run_manifest_verified": False, "loopback_only": True, "persistent_volume_attached": False,
        "shared_environment_touched": False, "production_applied": False,
        "clickhouse_container_removed": False, "postgres_container_removed": False, "opensearch_container_removed": False,
        "test_output": "", "errors": [],
    }
    created: list[str] = []
    try:
        if any(not absent(name) for name in (ch_container, pg_container, os_container)):
            raise RuntimeError("refusing to reuse existing three-store containers")
        for field, image in (("clickhouse_image_id", CLICKHOUSE_IMAGE), ("postgres_image_id", POSTGRES_IMAGE), ("opensearch_image_id", OPENSEARCH_IMAGE)):
            result[field] = json.loads(run(["docker", "image", "inspect", image]).stdout)[0].get("Id")
        run(["docker", "run", "--name", ch_container, "--ulimit", "nofile=262144:262144", "-p", "127.0.0.1::9000",
             "-e", f"CLICKHOUSE_USER={CLICKHOUSE_USER}", "-e", f"CLICKHOUSE_PASSWORD={CLICKHOUSE_PASSWORD}", "-d", CLICKHOUSE_IMAGE])
        created.append(ch_container)
        run(["docker", "run", "--name", pg_container, "-e", "POSTGRES_PASSWORD=ephemeral-alert-reconcile-only",
             "-e", "POSTGRES_DB=traffic_platform", "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE])
        created.append(pg_container)
        run(["docker", "run", "--name", os_container, "-e", "discovery.type=single-node",
             "-e", "DISABLE_SECURITY_PLUGIN=true", "-e", "DISABLE_INSTALL_DEMO_CONFIG=true",
             "-e", "OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m", "-p", "127.0.0.1::9200", "-d", OPENSEARCH_IMAGE])
        created.append(os_container)
        ch_port = mapped_port(ch_container, "9000/tcp")
        wait_tcp(ch_port, 60)
        ch_sql, result["clickhouse_schema_sha256"] = clickhouse_bootstrap()
        bootstrap = None
        for _ in range(60):
            bootstrap = run(["docker", "exec", "-i", ch_container, "clickhouse-client", "--user", CLICKHOUSE_USER,
                             "--password", CLICKHOUSE_PASSWORD, "--multiquery"], input_bytes=ch_sql, check=False)
            if bootstrap.returncode == 0:
                break
            time.sleep(1)
        if bootstrap is None or bootstrap.returncode != 0:
            output = "" if bootstrap is None else bootstrap.stdout.decode(errors="replace")
            raise RuntimeError(f"ClickHouse schema bootstrap failed: {output[-4096:]}")
        result["clickhouse_sentinel_verified"] = True
        for _ in range(60):
            if run(["docker", "exec", pg_container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False).returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")
        pg_port = mapped_port(pg_container, "5432/tcp")
        run(["docker", "exec", "-i", pg_container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform"],
            input_bytes=(ROOT / POSTGRES_MIGRATION).read_bytes())
        run(["docker", "exec", pg_container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform", "-c",
             "CREATE TABLE codex_ephemeral_alert_projection_sentinel(marker text primary key); "
             "INSERT INTO codex_ephemeral_alert_projection_sentinel(marker) VALUES ('ephemeral-only');"])
        result["postgres_sentinel_verified"] = True
        os_port = mapped_port(os_container, "9200/tcp")
        base_url = f"http://127.0.0.1:{os_port}"
        for _ in range(120):
            try:
                if request(base_url, "GET", "/").get("version", {}).get("number") == "2.14.0":
                    break
            except (OSError, RuntimeError, json.JSONDecodeError):
                pass
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral OpenSearch did not become ready")
        request(base_url, "PUT", "/codex-ephemeral-alert-reconcile-sentinel", {"settings": {"number_of_replicas": 0}})
        request(base_url, "PUT", f"/codex-ephemeral-alert-reconcile-sentinel/_doc/{SENTINEL}?refresh=true", {"marker": SENTINEL})
        if request(base_url, "GET", f"/codex-ephemeral-alert-reconcile-sentinel/_doc/{SENTINEL}").get("_source", {}).get("marker") != SENTINEL:
            raise RuntimeError("ephemeral OpenSearch sentinel could not be read back")
        result["opensearch_sentinel_verified"] = True
        mapping = json.loads((ROOT / ALERT_MAPPING).read_text(encoding="utf-8"))
        request(base_url, "PUT", "/alerts-v2-000001", {
            "settings": {"number_of_shards": 1, "number_of_replicas": 0},
            "mappings": mapping["template"]["mappings"],
            "aliases": {"alerts-v2-write": {"is_write_index": True}, "alerts-v2-read": {}},
        })
        env = os.environ.copy()
        env.update({
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_HOST": f"127.0.0.1:{ch_port}",
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_USER": CLICKHOUSE_USER,
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_PASSWORD": CLICKHOUSE_PASSWORD,
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_CH_SENTINEL": SENTINEL,
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_PG_DSN":
                f"postgres://postgres:ephemeral-alert-reconcile-only@127.0.0.1:{pg_port}/traffic_platform?sslmode=disable",
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_URL": base_url,
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_SENTINEL": SENTINEL,
        })
        completed = run(["go", "-C", "go/control-plane", "test", "./internal/alert/projection",
                         "-run", "^TestAlertProjectionRepairRealClickHousePostgresAndOpenSearch$", "-count=1", "-v"], env=env, check=False)
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            raise RuntimeError(f"three-store alert projection integration exited {completed.returncode}")
        for field in (
            "production_clickhouse_writer_verified", "production_clickhouse_authority_verified",
            "missing_stale_extra_seed_verified", "opensearch_terminal_requery_verified",
            "postgres_watermarks_converged", "three_store_hash_and_version_reconciliation_verified",
            "durable_run_manifest_verified",
        ):
            result[field] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        for container in reversed(created):
            run(["docker", "rm", "-f", container], check=False)
        result["clickhouse_container_removed"] = absent(ch_container)
        result["postgres_container_removed"] = absent(pg_container)
        result["opensearch_container_removed"] = absent(os_container)
    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite three-store alert projection evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    containers_removed = all(result[key] for key in ("clickhouse_container_removed", "postgres_container_removed", "opensearch_container_removed"))
    return 0 if result["status"] == "PASS" and containers_removed else 1


if __name__ == "__main__":
    raise SystemExit(main())
