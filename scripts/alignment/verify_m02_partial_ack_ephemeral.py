#!/usr/bin/env python3
"""Verify M02 Flow producer acknowledgements against owned ephemeral Kafka."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import time
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
KAFKA_IMAGE = "redpandadata/redpanda:v24.1.12"
TOPICS = ("m02.flow.events.v1", "m02.pcap.index.v1", "m02.session.events.v1")


def run(command: list[str], *, env: dict[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command,
        cwd=ROOT,
        env=env,
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


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument(
        "--result",
        default="doc/02_acceptance/topic1/m02/n007-n008/partial-ack-test-result.json",
    )
    args = parser.parse_args()

    suffix = hashlib.sha256(args.run_id.encode()).hexdigest()[:12]
    container = f"codex-m02-partial-ack-{suffix}"
    port = loopback_port()
    result: dict[str, object] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "environment_id": "owned-loopback-redpanda",
        "container": container,
        "image": KAFKA_IMAGE,
        "topics": list(TOPICS),
        "loopback_only": True,
        "shared_environment_touched": False,
        "persistent_volume_attached": False,
        "container_removed": False,
        "test_output": "",
        "errors": [],
    }
    created = False
    try:
        if not container_absent(container):
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        run(["docker", "image", "inspect", KAFKA_IMAGE])
        run([
            "docker", "run", "--name", container,
            "-p", f"127.0.0.1:{port}:19092", "-d", KAFKA_IMAGE,
            "redpanda", "start", "--mode", "dev-container", "--check=false",
            "--smp", "1", "--memory", "512M", "--reserve-memory", "0M",
            "--kafka-addr", "internal://0.0.0.0:9092,external://0.0.0.0:19092",
            "--advertise-kafka-addr",
            f"internal://127.0.0.1:9092,external://127.0.0.1:{port}",
            "--rpc-addr", "0.0.0.0:33145",
            "--advertise-rpc-addr", "127.0.0.1:33145",
        ])
        created = True
        for _ in range(90):
            ready = run([
                "docker", "exec", container, "rpk", "topic", "list",
                "--brokers", "127.0.0.1:9092",
            ], check=False)
            if ready.returncode == 0:
                break
            time.sleep(0.5)
        else:
            logs = run(["docker", "logs", "--tail", "100", container], check=False)
            raise RuntimeError("ephemeral Kafka did not become healthy: " + logs.stdout.decode(errors="replace"))

        for topic in TOPICS:
            run([
                "docker", "exec", container, "rpk", "topic", "create", topic,
                "--brokers", "127.0.0.1:9092", "--partitions", "3", "--replicas", "1",
                "-c", "cleanup.policy=delete", "-c", "retention.ms=3600000",
            ])

        env = os.environ.copy()
        env.update({
            "M02_PARTIAL_ACK_EPHEMERAL_KAFKA_BROKER": f"127.0.0.1:{port}",
            "M02_PARTIAL_ACK_EPHEMERAL_FLOW_TOPIC": TOPICS[0],
            "M02_PARTIAL_ACK_EPHEMERAL_PCAP_TOPIC": TOPICS[1],
            "M02_PARTIAL_ACK_EPHEMERAL_SESSION_TOPIC": TOPICS[2],
        })
        command = [
            "go", "-C", "go/control-plane", "test", "./internal/ingest/queue",
            "-run", "^TestFlowPartialAckProducerRealKafka$", "-count=1", "-v",
        ]
        completed = run(command, env=env, check=False)
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        result["test_output_sha256"] = hashlib.sha256(completed.stdout).hexdigest()
        if completed.returncode != 0:
            raise RuntimeError(f"real Kafka partial ACK test exited {completed.returncode}")
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        if created:
            run(["docker", "rm", "-f", container], check=False)
        result["container_removed"] = container_absent(container)

    output_path = ROOT / args.result
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" and result["container_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
