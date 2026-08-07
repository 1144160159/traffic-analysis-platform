#!/usr/bin/env python3
"""Verify the asset search projection against an owned OpenSearch container."""

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

import yaml


ROOT = Path(__file__).resolve().parents[2]
OPENSEARCH_IMAGE = "docker.io/opensearchproject/opensearch@sha256:466a49f379bb8889af29d615475e69b7b990898c6987d28470cd7105df9046ff"
TEMPLATE_AUTHORITY = Path("deployments/kubernetes/init-jobs/04-opensearch-templates.yaml")
ALERT_MAPPING_AUTHORITY = Path("common/opensearch/alerts-v2/mappings-component.json")
SENTINEL_INDEX = "codex-ephemeral-asset-projection-sentinel"
SENTINEL_VALUE = "ephemeral-only"


def run(
    command: list[str],
    *,
    env: dict[str, str] | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command,
        cwd=ROOT,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=check,
    )


def container_name(run_id: str) -> str:
    if not run_id.strip():
        raise ValueError("run_id is required")
    return "codex-asset-projection-os-" + hashlib.sha256(run_id.encode()).hexdigest()[:12]


def container_absent(name: str) -> bool:
    return run(["docker", "container", "inspect", name], check=False).returncode != 0


def request(base_url: str, method: str, path: str, body: Any | None = None) -> dict[str, Any]:
    payload = None if body is None else json.dumps(body, separators=(",", ":")).encode()
    req = urllib.request.Request(
        base_url + path,
        data=payload,
        method=method,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            content = response.read()
    except urllib.error.HTTPError as exc:
        content = exc.read()
        raise RuntimeError(
            f"OpenSearch {method} {path} returned {exc.code}: "
            f"{content.decode(errors='replace')[:4096]}"
        ) from exc
    return json.loads(content) if content else {}


def asset_template() -> dict[str, Any]:
    documents = yaml.safe_load_all((ROOT / TEMPLATE_AUTHORITY).read_text(encoding="utf-8"))
    for document in documents:
        if not isinstance(document, dict) or document.get("kind") != "ConfigMap":
            continue
        if document.get("metadata", {}).get("name") != "opensearch-assets-v2-contract":
            continue
        template = json.loads(document["data"]["index-template.json"])
        # The production authority requires replicas. The isolated single-node
        # test changes only topology counts while preserving the exact mapping.
        settings = template["template"]["settings"]
        settings["number_of_shards"] = 1
        settings["number_of_replicas"] = 0
        return template
    raise RuntimeError("asset OpenSearch template authority not found")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    container = container_name(args.run_id)
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "container": container,
        "image": OPENSEARCH_IMAGE,
        "image_id": None,
        "template_authority": TEMPLATE_AUTHORITY.as_posix(),
        "mapping_sha256": None,
        "write_alias": "assets-v2-write",
        "read_alias": "assets-v2-read",
        "sentinel_verified": False,
        "deterministic_document_id_verified": False,
        "same_version_replay_verified": False,
        "older_version_rejected": False,
        "strict_mapping_verified": False,
        "production_alert_writer_verified": False,
        "loopback_only": True,
        "persistent_volume_attached": False,
        "shared_environment_touched": False,
        "production_applied": False,
        "container_removed": False,
        "test_output": "",
        "errors": [],
    }
    created = False
    try:
        if not container_absent(container):
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        inspected = json.loads(run(["docker", "image", "inspect", OPENSEARCH_IMAGE]).stdout)[0]
        result["image_id"] = inspected.get("Id")
        run(
            [
                "docker", "run", "--name", container,
                "-e", "discovery.type=single-node",
                "-e", "DISABLE_SECURITY_PLUGIN=true",
                "-e", "DISABLE_INSTALL_DEMO_CONFIG=true",
                "-e", "OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m",
                "-p", "127.0.0.1::9200", "-d", OPENSEARCH_IMAGE,
            ]
        )
        created = True
        port_output = run(["docker", "port", container, "9200/tcp"]).stdout.decode().strip()
        port = port_output.rsplit(":", 1)[-1]
        if not port.isdigit():
            raise RuntimeError(f"invalid loopback OpenSearch port mapping: {port_output!r}")
        base_url = f"http://127.0.0.1:{port}"
        for _ in range(120):
            try:
                info = request(base_url, "GET", "/")
                if info.get("version", {}).get("number") == "2.14.0":
                    break
            except (OSError, RuntimeError, json.JSONDecodeError):
                pass
            time.sleep(1)
        else:
            logs = run(["docker", "logs", "--tail", "120", container], check=False)
            raise RuntimeError(
                "ephemeral OpenSearch did not become ready: "
                + logs.stdout.decode(errors="replace")[-8192:]
            )

        request(base_url, "PUT", f"/{SENTINEL_INDEX}", {"settings": {"number_of_replicas": 0}})
        request(
            base_url,
            "PUT",
            f"/{SENTINEL_INDEX}/_doc/{SENTINEL_VALUE}?refresh=true",
            {"marker": SENTINEL_VALUE},
        )
        sentinel = request(base_url, "GET", f"/{SENTINEL_INDEX}/_doc/{SENTINEL_VALUE}")
        if sentinel.get("_source", {}).get("marker") != SENTINEL_VALUE:
            raise RuntimeError("ephemeral OpenSearch sentinel could not be read back")
        result["sentinel_verified"] = True

        template = asset_template()
        mapping_payload = json.dumps(
            template["template"]["mappings"], sort_keys=True, separators=(",", ":")
        ).encode()
        result["mapping_sha256"] = hashlib.sha256(mapping_payload).hexdigest()
        request(base_url, "PUT", "/_index_template/assets-v2-template", template)
        request(
            base_url,
            "PUT",
            "/assets-v2-000001",
            {"aliases": {"assets-v2-write": {"is_write_index": True}, "assets-v2-read": {}}},
        )
        alert_mapping = json.loads((ROOT / ALERT_MAPPING_AUTHORITY).read_text(encoding="utf-8"))
        request(
            base_url,
            "PUT",
            "/alerts-v2-000001",
            {
                "settings": {"number_of_shards": 1, "number_of_replicas": 0},
                "mappings": alert_mapping["template"]["mappings"],
                "aliases": {"alerts-v2-write": {"is_write_index": True}, "alerts-v2-read": {}},
            },
        )
        simulated = request(base_url, "POST", "/_index_template/_simulate_index/assets-v2-canary")
        if simulated.get("template", {}).get("mappings", {}).get("dynamic") != "strict":
            raise RuntimeError("asset index template did not retain dynamic=strict")
        result["strict_mapping_verified"] = True

        test_env = os.environ.copy()
        test_env["ASSET_PROJECTION_EPHEMERAL_OS_URL"] = base_url
        test_env["ASSET_PROJECTION_EPHEMERAL_OS_SENTINEL"] = SENTINEL_VALUE
        completed = run(
            [
                "go", "-C", "go/control-plane", "test", "./internal/asset/consumer",
                "-run", "^TestOpenSearchAssetProjectionEphemeralIntegration$", "-count=1", "-v",
            ],
            env=test_env,
            check=False,
        )
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            raise RuntimeError(f"asset OpenSearch integration exited {completed.returncode}")
        alert_completed = run(
            [
                "go", "-C", "go/control-plane", "test", "./internal/alert/persistence",
                "-run", "^TestAlertWriterRealStrictOpenSearchMapping$", "-count=1", "-v",
            ],
            env={
                **test_env,
                "ALERT_PERSISTENCE_EPHEMERAL_OS_URL": base_url,
                "ALERT_PERSISTENCE_EPHEMERAL_OS_SENTINEL": SENTINEL_VALUE,
            },
            check=False,
        )
        result["test_output"] += "\n" + alert_completed.stdout.decode(errors="replace").strip()
        if alert_completed.returncode != 0:
            raise RuntimeError(f"alert OpenSearch writer integration exited {alert_completed.returncode}")
        result["production_alert_writer_verified"] = True
        # The Go test also proves same-version replay and older external version
        # rejection against this exact ephemeral cluster.
        result["deterministic_document_id_verified"] = True
        result["same_version_replay_verified"] = True
        result["older_version_rejected"] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if created:
            run(["docker", "rm", "-f", container], check=False)
        result["container_removed"] = container_absent(container)

    payload = json.dumps(result, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite asset OpenSearch G1 evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and result["container_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
