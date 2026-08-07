#!/usr/bin/env python3
"""Verify alert repair receipts across owned PostgreSQL and OpenSearch."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
OPENSEARCH_IMAGE = "docker.io/opensearchproject/opensearch@sha256:466a49f379bb8889af29d615475e69b7b990898c6987d28470cd7105df9046ff"
MIGRATION = Path("deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql")
ALERT_MAPPING = Path("common/opensearch/alerts-v2/mappings-component.json")
SENTINEL = "ephemeral-only"


def run(command: list[str], *, input_bytes: bytes | None = None, env: dict[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, input=input_bytes, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=check)


def names(run_id: str) -> tuple[str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    digest = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return f"codex-alert-reconcile-pg-{digest}", f"codex-alert-reconcile-os-{digest}"


def absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


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


def mapped_port(container: str, port: str) -> str:
    output = run(["docker", "port", container, port]).stdout.decode().strip()
    value = output.rsplit(":", 1)[-1]
    if not value.isdigit():
        raise RuntimeError(f"invalid loopback port mapping for {container}: {output!r}")
    return value


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    pg_container, os_container = names(args.run_id)
    result: dict[str, Any] = {
        "schema_version": 1, "run_id": args.run_id, "status": "FAIL",
        "postgres_container": pg_container, "opensearch_container": os_container,
        "postgres_image": POSTGRES_IMAGE, "opensearch_image": OPENSEARCH_IMAGE,
        "postgres_image_id": None, "opensearch_image_id": None,
        "migration": MIGRATION.as_posix(), "alert_mapping": ALERT_MAPPING.as_posix(),
        "postgres_sentinel_verified": False, "opensearch_sentinel_verified": False,
        "missing_stale_extra_seed_verified": False, "opensearch_terminal_requery_verified": False,
        "postgres_watermarks_converged": False, "deleted_receipt_recovered_without_projection_rewrite": False,
        "durable_run_manifest_verified": False, "loopback_only": True,
        "persistent_volume_attached": False, "shared_environment_touched": False,
        "production_applied": False, "postgres_container_removed": False,
        "opensearch_container_removed": False, "test_output": "", "errors": [],
    }
    created: list[str] = []
    try:
        if not absent(pg_container) or not absent(os_container):
            raise RuntimeError("refusing to reuse existing combined-reconcile containers")
        result["postgres_image_id"] = json.loads(run(["docker", "image", "inspect", POSTGRES_IMAGE]).stdout)[0].get("Id")
        result["opensearch_image_id"] = json.loads(run(["docker", "image", "inspect", OPENSEARCH_IMAGE]).stdout)[0].get("Id")
        run(["docker", "run", "--name", pg_container, "-e", "POSTGRES_PASSWORD=ephemeral-alert-reconcile-only",
             "-e", "POSTGRES_DB=traffic_platform", "-p", "127.0.0.1::5432", "-d", POSTGRES_IMAGE])
        created.append(pg_container)
        run(["docker", "run", "--name", os_container, "-e", "discovery.type=single-node",
             "-e", "DISABLE_SECURITY_PLUGIN=true", "-e", "DISABLE_INSTALL_DEMO_CONFIG=true",
             "-e", "OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m", "-p", "127.0.0.1::9200", "-d", OPENSEARCH_IMAGE])
        created.append(os_container)
        for _ in range(60):
            if run(["docker", "exec", pg_container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False).returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")
        pg_port = mapped_port(pg_container, "5432/tcp")
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

        run(["docker", "exec", "-i", pg_container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform"],
            input_bytes=(ROOT / MIGRATION).read_bytes())
        run(["docker", "exec", pg_container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform", "-c",
             "CREATE TABLE codex_ephemeral_alert_projection_sentinel(marker text primary key); "
             "INSERT INTO codex_ephemeral_alert_projection_sentinel(marker) VALUES ('ephemeral-only');"])
        result["postgres_sentinel_verified"] = True
        request(base_url, "PUT", "/codex-ephemeral-alert-reconcile-sentinel", {"settings": {"number_of_replicas": 0}})
        request(base_url, "PUT", f"/codex-ephemeral-alert-reconcile-sentinel/_doc/{SENTINEL}?refresh=true", {"marker": SENTINEL})
        sentinel = request(base_url, "GET", f"/codex-ephemeral-alert-reconcile-sentinel/_doc/{SENTINEL}")
        if sentinel.get("_source", {}).get("marker") != SENTINEL:
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
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_URL": base_url,
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_SENTINEL": SENTINEL,
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_PG_DSN":
                f"postgres://postgres:ephemeral-alert-reconcile-only@127.0.0.1:{pg_port}/traffic_platform?sslmode=disable",
        })
        completed = run(["go", "-C", "go/control-plane", "test", "./internal/alert/projection",
                         "-run", "^TestAlertProjectionRepairRealPostgresAndOpenSearch$", "-count=1", "-v"], env=env, check=False)
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            raise RuntimeError(f"combined alert projection integration exited {completed.returncode}")
        result["missing_stale_extra_seed_verified"] = True
        result["opensearch_terminal_requery_verified"] = True
        result["postgres_watermarks_converged"] = True
        result["deleted_receipt_recovered_without_projection_rewrite"] = True
        result["durable_run_manifest_verified"] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        for container in reversed(created):
            run(["docker", "rm", "-f", container], check=False)
        result["postgres_container_removed"] = absent(pg_container)
        result["opensearch_container_removed"] = absent(os_container)
    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite combined alert projection evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and result["postgres_container_removed"] and result["opensearch_container_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
