#!/usr/bin/env python3
"""Verify the production shadow reader on owned loopback ClickHouse/OpenSearch."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import time
from pathlib import Path
from typing import Any

import verify_alert_projection_clickhouse_postgres_opensearch_ephemeral as shared


ROOT = Path(__file__).resolve().parents[2]
CLICKHOUSE_IMAGE = shared.CLICKHOUSE_IMAGE
OPENSEARCH_IMAGE = shared.OPENSEARCH_IMAGE
ALERT_MAPPING = shared.ALERT_MAPPING
SENTINEL = shared.SENTINEL
CLICKHOUSE_USER = shared.CLICKHOUSE_USER
CLICKHOUSE_PASSWORD = shared.CLICKHOUSE_PASSWORD


def names(run_id: str) -> tuple[str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    digest = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return tuple(f"codex-alert-shadow-{kind}-{digest}" for kind in ("ch", "os"))


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    ch_container, os_container = names(args.run_id)
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "gate": "G1",
        "status": "FAIL",
        "remediation_ids": ["T-OS-002", "T-OS-004"],
        "mode": "OWNED_LOOPBACK_READ_ONLY_SHADOW",
        "clickhouse_container": ch_container,
        "opensearch_container": os_container,
        "clickhouse_image": CLICKHOUSE_IMAGE,
        "opensearch_image": OPENSEARCH_IMAGE,
        "clickhouse_image_id": None,
        "opensearch_image_id": None,
        "production_clickhouse_authority_reader_verified": False,
        "production_opensearch_projection_reader_verified": False,
        "missing_stale_extra_classification_verified": False,
        "cluster_alias_and_write_index_binding_verified": False,
        "source_count_and_hash_unchanged_after_shadow": False,
        "target_count_and_hash_unchanged_after_shadow": False,
        "postgres_dependency_present": False,
        "production_mutations": [],
        "shared_environment_touched": False,
        "production_applied": False,
        "persistent_volume_attached": False,
        "loopback_only": True,
        "shadow_binding_sha256": None,
        "clickhouse_container_removed": False,
        "opensearch_container_removed": False,
        "test_output": "",
        "errors": [],
    }
    created: list[str] = []
    try:
        if any(not shared.absent(name) for name in (ch_container, os_container)):
            raise RuntimeError("refusing to reuse existing shadow containers")
        for field, image in (("clickhouse_image_id", CLICKHOUSE_IMAGE), ("opensearch_image_id", OPENSEARCH_IMAGE)):
            result[field] = json.loads(shared.run(["docker", "image", "inspect", image]).stdout)[0].get("Id")
        shared.run([
            "docker", "run", "--name", ch_container, "--ulimit", "nofile=262144:262144",
            "-p", "127.0.0.1::9000", "-e", f"CLICKHOUSE_USER={CLICKHOUSE_USER}",
            "-e", f"CLICKHOUSE_PASSWORD={CLICKHOUSE_PASSWORD}", "-d", CLICKHOUSE_IMAGE,
        ])
        created.append(ch_container)
        shared.run([
            "docker", "run", "--name", os_container, "-e", "discovery.type=single-node",
            "-e", "DISABLE_SECURITY_PLUGIN=true", "-e", "DISABLE_INSTALL_DEMO_CONFIG=true",
            "-e", "OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m", "-p", "127.0.0.1::9200", "-d", OPENSEARCH_IMAGE,
        ])
        created.append(os_container)
        ch_port = shared.mapped_port(ch_container, "9000/tcp")
        shared.wait_tcp(ch_port, 60)
        ch_sql, _ = shared.clickhouse_bootstrap()
        bootstrap = None
        for _ in range(60):
            bootstrap = shared.run([
                "docker", "exec", "-i", ch_container, "clickhouse-client", "--user", CLICKHOUSE_USER,
                "--password", CLICKHOUSE_PASSWORD, "--multiquery",
            ], input_bytes=ch_sql, check=False)
            if bootstrap.returncode == 0:
                break
            time.sleep(1)
        if bootstrap is None or bootstrap.returncode != 0:
            output = "" if bootstrap is None else bootstrap.stdout.decode(errors="replace")
            raise RuntimeError(f"ClickHouse shadow bootstrap failed: {output[-4096:]}")

        os_port = shared.mapped_port(os_container, "9200/tcp")
        base_url = f"http://127.0.0.1:{os_port}"
        for _ in range(120):
            try:
                if shared.request(base_url, "GET", "/").get("version", {}).get("number") == "2.14.0":
                    break
            except (OSError, RuntimeError, json.JSONDecodeError):
                pass
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral OpenSearch did not become ready")
        shared.request(base_url, "PUT", "/codex-ephemeral-alert-reconcile-sentinel", {"settings": {"number_of_replicas": 0}})
        shared.request(base_url, "PUT", f"/codex-ephemeral-alert-reconcile-sentinel/_doc/{SENTINEL}?refresh=true", {"marker": SENTINEL})
        mapping = json.loads((ROOT / ALERT_MAPPING).read_text(encoding="utf-8"))
        shared.request(base_url, "PUT", "/alerts-v2-000001", {
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
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_URL": base_url,
            "ALERT_PROJECTION_RECONCILE_EPHEMERAL_OS_SENTINEL": SENTINEL,
        })
        completed = shared.run([
            "go", "-C", "go/control-plane", "test", "./internal/alert/projection",
            "-run", "^TestAlertProjectionShadowRealClickHouseAndOpenSearch$", "-count=1", "-v",
        ], env=env, check=False)
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            raise RuntimeError(f"two-store shadow integration exited {completed.returncode}")
        match = re.search(r"shadow_binding_sha256=([0-9a-f]{64})", result["test_output"])
        if not match:
            raise RuntimeError("shadow integration did not emit its binding SHA-256")
        result["shadow_binding_sha256"] = match.group(1)
        for field in (
            "production_clickhouse_authority_reader_verified",
            "production_opensearch_projection_reader_verified",
            "missing_stale_extra_classification_verified",
            "cluster_alias_and_write_index_binding_verified",
            "source_count_and_hash_unchanged_after_shadow",
            "target_count_and_hash_unchanged_after_shadow",
        ):
            result[field] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        for container in reversed(created):
            shared.run(["docker", "rm", "-f", container], check=False)
        result["clickhouse_container_removed"] = shared.absent(ch_container)
        result["opensearch_container_removed"] = shared.absent(os_container)
    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite shadow evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    removed = result["clickhouse_container_removed"] and result["opensearch_container_removed"]
    return 0 if result["status"] == "PASS" and removed else 1


if __name__ == "__main__":
    raise SystemExit(main())
