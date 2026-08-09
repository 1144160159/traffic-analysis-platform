#!/usr/bin/env python3
"""Verify F-ALERT-005 manifests and object integrity in owned PG/MinIO."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import socket
import subprocess
import time
import urllib.request
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
POSTGRES_IMAGE = "postgres@sha256:33f923b05f64ca54ac4401c01126a6b92afe839a0aa0a52bc5aeb5cc958e5f20"
MINIO_IMAGE = "minio/minio@sha256:14cea493d9a34af32f524e538b8346cf79f3321eff8e708c1e2960462bd8936e"
PG_PASSWORD = "codex-alert-evidence-pg-ephemeral-only"
MINIO_ACCESS_KEY = "codexalertevidence"
MINIO_SECRET_KEY = "codex-alert-evidence-minio-ephemeral-only"
PG_SENTINEL = "codex_ephemeral_alert_evidence_sentinel"
MINIO_SENTINEL_PATH = "/tmp/codex-alert-evidence-ephemeral-only"
SENTINEL_VALUE = "ephemeral-only"
MEMORY_LIMIT = "512m"
CPU_LIMIT = "1"
PASS_MARKERS = (
    "alert_evidence_postgres_manifest=pass",
    "alert_evidence_minio_integrity=pass",
)


def run(command: list[str], *, input_bytes: bytes | None = None, env: dict[str, str] | None = None, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(command, cwd=ROOT, input=input_bytes, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=check)


def loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


def owned_name(run_id: str, suffix: str) -> str:
    if not run_id.strip():
        raise ValueError("run_id is required")
    return f"codex-alert-evidence-{suffix}-" + hashlib.sha256(run_id.encode()).hexdigest()[:12]


def inspect_container(name: str) -> dict[str, Any] | None:
    completed = run(["docker", "container", "inspect", name], check=False)
    if completed.returncode != 0:
        return None
    return json.loads(completed.stdout)[0]


def absent(name: str) -> bool:
    return inspect_container(name) is None


def resource_snapshot(name: str) -> dict[str, Any] | None:
    completed = run(["docker", "stats", "--no-stream", "--format", "{{json .}}", name], check=False)
    if completed.returncode != 0 or not completed.stdout.strip():
        return None
    raw = json.loads(completed.stdout)
    return {
        "cpu_percent": raw.get("CPUPerc"), "memory_usage": raw.get("MemUsage"),
        "memory_percent": raw.get("MemPerc"), "block_io": raw.get("BlockIO"), "pids": raw.get("PIDs"),
        "measurement": "post_workload_single_snapshot_not_performance_evidence",
    }


def wait_minio(port: int) -> None:
    endpoint = f"http://127.0.0.1:{port}/minio/health/ready"
    for _ in range(45):
        try:
            with urllib.request.urlopen(endpoint, timeout=1) as response:
                if response.status == 200:
                    return
        except Exception:
            pass
        time.sleep(1)
    raise RuntimeError("ephemeral MinIO did not become ready")


def verify_identity(name: str, container_id: str, image_id: str, run_id: str, tmpfs_path: str) -> bool:
    inspected = inspect_container(name)
    if not inspected:
        return False
    labels = (inspected.get("Config") or {}).get("Labels") or {}
    tmpfs = (inspected.get("HostConfig") or {}).get("Tmpfs") or {}
    mounts = inspected.get("Mounts") or []
    return bool(
        inspected.get("Id") == container_id
        and inspected.get("Image") == image_id
        and labels.get("codex.owned") == "alert-evidence-ephemeral"
        and labels.get("codex.run-id") == run_id
        and tmpfs_path in tmpfs
        and not any(item.get("Type") == "volume" for item in mounts)
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()

    pg_name = owned_name(args.run_id, "pg")
    minio_name = owned_name(args.run_id, "minio")
    pg_port, minio_port = loopback_port(), loopback_port()
    while minio_port == pg_port:
        minio_port = loopback_port()
    containers = {
        "postgres": {"name": pg_name, "id": "", "image": POSTGRES_IMAGE, "image_id": "", "created": False, "tmpfs": "/var/lib/postgresql/data", "sentinel": False},
        "minio": {"name": minio_name, "id": "", "image": MINIO_IMAGE, "image_id": "", "created": False, "tmpfs": "/data", "sentinel": False},
    }
    result: dict[str, Any] = {
        "schema_version": 1,
        "run_id": args.run_id,
        "status": "FAIL",
        "coverage_status": "OWNED_REAL_POSTGRES_MINIO_ALERT_EVIDENCE_MANIFEST_INTEGRITY_G1",
        "production_applied": False,
        "shared_environment_touched": False,
        "loopback_only": True,
        "container_resource_limits": {"memory": MEMORY_LIMIT, "cpus": CPU_LIMIT},
        "containers": {},
        "schema_entrypoints": [
            "go/control-plane/deployments/docker/init/postgres_merged.sql",
            "deployments/postgres/migrations/202608091700_alert_evidence_manifest_v1.sql",
            "common/sql/pg/18-alert-evidence-manifest-v1.sql",
        ],
        "schema_replay_count": 0,
        "test_output": "",
        "asserted_facts": {
            "postgres_manifest_schema_verified": False,
            "tenant_isolation": False,
            "monotonic_revision": False,
            "immutable_object_identity": False,
            "append_only_manifest_history": False,
            "minio_version_identity": False,
            "minio_sha256_match": False,
            "minio_sha256_mismatch_rejected": False,
            "all_data_mounts_tmpfs": False,
            "no_persistent_volume": False,
        },
        "errors": [],
        "secrets_captured": False,
    }
    try:
        for entry in containers.values():
            if not absent(entry["name"]):
                raise RuntimeError(f"refusing to reuse existing container: {entry['name']}")
            image = json.loads(run(["docker", "image", "inspect", entry["image"]]).stdout)[0]
            entry["image_id"] = image.get("Id", "")

        pg_created = run([
            "docker", "run", "--name", pg_name,
            "--label", "codex.owned=alert-evidence-ephemeral", "--label", f"codex.run-id={args.run_id}",
            "--memory", MEMORY_LIMIT, "--cpus", CPU_LIMIT, "--tmpfs", "/var/lib/postgresql/data:rw,nosuid,nodev,size=384m",
            "-e", f"POSTGRES_PASSWORD={PG_PASSWORD}", "-e", "POSTGRES_DB=traffic_platform",
            "-p", f"127.0.0.1:{pg_port}:5432", "-d", POSTGRES_IMAGE,
        ])
        containers["postgres"]["id"] = pg_created.stdout.decode().strip()
        containers["postgres"]["created"] = True

        minio_created = run([
            "docker", "run", "--name", minio_name,
            "--label", "codex.owned=alert-evidence-ephemeral", "--label", f"codex.run-id={args.run_id}",
            "--memory", MEMORY_LIMIT, "--cpus", CPU_LIMIT, "--tmpfs", "/data:rw,nosuid,nodev,size=384m",
            "-e", f"MINIO_ROOT_USER={MINIO_ACCESS_KEY}", "-e", f"MINIO_ROOT_PASSWORD={MINIO_SECRET_KEY}",
            "-p", f"127.0.0.1:{minio_port}:9000", "-d", MINIO_IMAGE,
            "server", "/data", "--console-address", ":9001",
        ])
        containers["minio"]["id"] = minio_created.stdout.decode().strip()
        containers["minio"]["created"] = True

        for _ in range(30):
            ready = run(["docker", "exec", pg_name, "pg_isready", "-U", "postgres", "-d", "traffic_platform"], check=False)
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")
        wait_minio(minio_port)

        for entry in containers.values():
            if not verify_identity(entry["name"], entry["id"], entry["image_id"], args.run_id, entry["tmpfs"]):
                raise RuntimeError(f"container identity, label, tmpfs or volume check failed: {entry['name']}")

        for path in result["schema_entrypoints"]:
            repeats = 1 if path.endswith("postgres_merged.sql") else 2
            for _ in range(repeats):
                run([
                    "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i", pg_name,
                    "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform",
                ], input_bytes=(ROOT / path).read_bytes())
                result["schema_replay_count"] += 1
        run([
            "docker", "exec", pg_name, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform", "-c",
            f"CREATE TABLE {PG_SENTINEL}(marker text primary key); INSERT INTO {PG_SENTINEL}(marker) VALUES ('{SENTINEL_VALUE}');",
        ])
        containers["postgres"]["sentinel"] = True
        run(["docker", "exec", minio_name, "sh", "-c", f"printf '%s' '{SENTINEL_VALUE}' > {MINIO_SENTINEL_PATH}"])
        containers["minio"]["sentinel"] = True

        test_env = os.environ.copy()
        test_env["GOSUMDB"] = test_env.get("TRAFFIC_GO_SUMDB") or "sum.golang.org"
        test_env["ALERT_EVIDENCE_EPHEMERAL_PG_DSN"] = f"postgres://postgres:{PG_PASSWORD}@127.0.0.1:{pg_port}/traffic_platform?sslmode=disable"
        test_env["ALERT_EVIDENCE_EPHEMERAL_MINIO"] = SENTINEL_VALUE
        test_env["S3_ENDPOINT"] = f"127.0.0.1:{minio_port}"
        test_env["S3_ACCESS_KEY"] = MINIO_ACCESS_KEY
        test_env["S3_SECRET_KEY"] = MINIO_SECRET_KEY
        test_env["S3_USE_SSL"] = "false"
        completed = run([
            "go", "-C", "go/control-plane", "test", "./internal/alert/api", "-run",
            "^(TestAlertEvidenceManifestPostgresIntegration|TestAlertEvidenceMinIOIntegrityIntegration)$", "-count=1", "-v",
        ], env=test_env, check=False)
        result["test_output"] = completed.stdout.decode(errors="replace").strip()
        if completed.returncode != 0 or not all(marker in result["test_output"] for marker in PASS_MARKERS):
            raise RuntimeError(f"alert-evidence PG/MinIO integration exited {completed.returncode}")
        result["asserted_facts"] = {key: True for key in result["asserted_facts"]}
        result["status"] = "PASS"
    except Exception as exc:
        result["errors"] = [str(exc)]
    finally:
        for kind, entry in reversed(list(containers.items())):
            name = entry["name"]
            result["containers"].setdefault(kind, {})["resource_snapshot"] = resource_snapshot(name) if entry["created"] else None
            identity_ok = bool(entry["created"] and verify_identity(name, entry["id"], entry["image_id"], args.run_id, entry["tmpfs"]))
            sentinel_ok = False
            if identity_ok and entry["sentinel"]:
                if kind == "postgres":
                    checked = run(["docker", "exec", name, "psql", "-X", "-U", "postgres", "-d", "traffic_platform", "-Atc", f"SELECT marker FROM {PG_SENTINEL} WHERE marker='{SENTINEL_VALUE}'"], check=False)
                else:
                    checked = run(["docker", "exec", name, "sh", "-c", f"test \"$(cat {MINIO_SENTINEL_PATH})\" = '{SENTINEL_VALUE}'"], check=False)
                sentinel_ok = checked.returncode == 0 and (kind != "postgres" or checked.stdout.decode().strip() == SENTINEL_VALUE)
            removed = False
            if identity_ok and sentinel_ok:
                run(["docker", "rm", "-f", "-v", name], check=False)
                removed = absent(name)
            elif entry["created"]:
                result["errors"].append(f"refusing to remove {kind} after identity or sentinel drift")
                result["status"] = "FAIL"
            result["containers"][kind].update({
                "name": name, "container_id": entry["id"], "image": entry["image"], "image_id": entry["image_id"],
                "identity_verified": identity_ok, "cleanup_sentinel_verified": sentinel_ok, "data_mount_type": "tmpfs" if identity_ok else "unknown",
                "persistent_volume_attached": False if identity_ok else None, "removed": removed,
            })

    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite alert-evidence evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    scoped_pass = result["status"] == "PASS" and all(result["asserted_facts"].values()) and all(
        item.get("identity_verified") and item.get("cleanup_sentinel_verified") and item.get("removed") and not item.get("persistent_volume_attached")
        for item in result["containers"].values()
    )
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
