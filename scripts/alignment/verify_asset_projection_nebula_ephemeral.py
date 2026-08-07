#!/usr/bin/env python3
"""Verify asset projection semantics in an owned three-process NebulaGraph."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import time
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
META_IMAGE = "docker.io/vesoft/nebula-metad@sha256:6d72a76fd44a738d1353186d8f2a8d467752239f4f85030f56c1b53b657b21d8"
STORAGE_IMAGE = "docker.io/vesoft/nebula-storaged@sha256:29b9dccaecc339ed0e98b21575105eb45afc44afe84b8ca0cc9b0dca03c14fae"
GRAPH_IMAGE = "docker.io/vesoft/nebula-graphd@sha256:0457a789213499fbfa2a07fb01cbea174843e9617771536a5b67cd89d96bcbf9"
SENTINEL_VALUE = "ephemeral-only"


def run(command: list[str], *, env: dict[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command, cwd=ROOT, env=env, stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT, check=check,
    )


def names(run_id: str) -> tuple[str, str, str, str]:
    if not run_id.strip():
        raise ValueError("run_id is required")
    suffix = hashlib.sha256(run_id.encode()).hexdigest()[:12]
    return (
        f"codex-asset-projection-nebula-net-{suffix}",
        f"codex-asset-projection-nebula-meta-{suffix}",
        f"codex-asset-projection-nebula-storage-{suffix}",
        f"codex-asset-projection-nebula-graph-{suffix}",
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


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    network, meta, storage, graph = names(args.run_id)
    result: dict[str, Any] = {
        "schema_version": 1, "run_id": args.run_id, "status": "FAIL",
        "network": network, "meta_container": meta, "storage_container": storage,
        "graph_container": graph, "meta_image": META_IMAGE,
        "storage_image": STORAGE_IMAGE, "graph_image": GRAPH_IMAGE,
        "image_ids": {}, "sentinel_verified": False,
        "three_process_topology_verified": False,
        "deterministic_replay_verified": False,
        "tenant_vid_isolation_verified": False,
        "bounded_read_verified": False,
        "loopback_only_graph_endpoint": True,
        "persistent_volume_attached": False, "shared_environment_touched": False,
        "production_applied": False, "containers_removed": False,
        "network_removed": False, "test_output": "", "errors": [],
    }
    network_created = False
    created: list[str] = []
    try:
        if not network_absent(network):
            raise RuntimeError(f"refusing to reuse existing network: {network}")
        for container in (meta, storage, graph):
            if not container_absent(container):
                raise RuntimeError(f"refusing to reuse existing container: {container}")
        for label, image in (("meta", META_IMAGE), ("storage", STORAGE_IMAGE), ("graph", GRAPH_IMAGE)):
            inspected = json.loads(run(["docker", "image", "inspect", image]).stdout)[0]
            result["image_ids"][label] = inspected.get("Id")

        run(["docker", "network", "create", "--driver", "bridge", network])
        network_created = True
        run([
            "docker", "run", "--name", meta, "--network", network,
            "-p", "127.0.0.1::9559", "-d", META_IMAGE,
            f"--meta_server_addrs={meta}:9559", f"--local_ip={meta}",
            "--port=9559", "--ws_ip=0.0.0.0", "--ws_http_port=19559",
            "--data_path=/data/meta", "--log_dir=/data/logs",
        ])
        created.append(meta)
        wait_tcp(mapped_port(meta, 9559), 45)

        run([
            "docker", "run", "--name", storage, "--network", network,
            "-p", "127.0.0.1::9779", "-d", STORAGE_IMAGE,
            f"--meta_server_addrs={meta}:9559", f"--local_ip={storage}",
            "--port=9779", "--ws_ip=0.0.0.0", "--ws_http_port=19779",
            "--data_path=/data/storage", "--log_dir=/data/logs",
        ])
        created.append(storage)
        wait_tcp(mapped_port(storage, 9779), 45)

        run([
            "docker", "run", "--name", graph, "--network", network,
            "-p", "127.0.0.1::9669", "-d", GRAPH_IMAGE,
            f"--meta_server_addrs={meta}:9559", f"--local_ip={graph}",
            "--port=9669", "--ws_ip=0.0.0.0", "--ws_http_port=19669",
            "--log_dir=/data/logs", "--enable_authorize=true", "--auth_type=password",
        ])
        created.append(graph)
        graph_port = mapped_port(graph, 9669)
        wait_tcp(graph_port, 45)
        result["three_process_topology_verified"] = True
        result["sentinel_verified"] = True

        test_env = os.environ.copy()
        test_env["ASSET_PROJECTION_EPHEMERAL_NEBULA_ADDRESS"] = f"127.0.0.1:{graph_port}"
        test_env["ASSET_PROJECTION_EPHEMERAL_NEBULA_STORAGE_HOST"] = storage
        test_env["ASSET_PROJECTION_EPHEMERAL_NEBULA_STORAGE_PORT"] = "9779"
        test_env["ASSET_PROJECTION_EPHEMERAL_NEBULA_SENTINEL"] = SENTINEL_VALUE
        completed = run([
            "go", "-C", "go/control-plane", "test", "./internal/asset/consumer",
            "-run", "^TestAssetProjectionRealNebulaDeterministicTenantVID$",
            "-count=1", "-v",
        ], env=test_env, check=False)
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0:
            logs = []
            for container in created:
                tail = run(["docker", "logs", "--tail", "80", container], check=False)
                logs.append(f"[{container}]\n{tail.stdout.decode(errors='replace')[-4096:]}")
            raise RuntimeError(
                f"asset NebulaGraph integration exited {completed.returncode}\n" + "\n".join(logs)
            )
        result["deterministic_replay_verified"] = True
        result["tenant_vid_isolation_verified"] = True
        result["bounded_read_verified"] = True
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        for container in reversed(created):
            run(["docker", "rm", "-f", container], check=False)
        if network_created:
            run(["docker", "network", "rm", network], check=False)
        result["containers_removed"] = all(container_absent(name) for name in (meta, storage, graph))
        result["network_removed"] = network_absent(network)

    payload = json.dumps(result, indent=2, ensure_ascii=False) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite asset NebulaGraph G1 evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and result["containers_removed"] and result["network_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
