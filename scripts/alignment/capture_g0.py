#!/usr/bin/env python3
"""Run the repository G0 gates and persist an immutable, hash-addressed evidence package."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
FULL_COMMANDS = (
    ("alignment", ["make", "alignment-test"]),
    ("full", ["tests/run_tests.sh", "full"]),
    ("python", ["make", "python-test"]),
)
PROBE_PUBLISHER_COMMANDS = (
    ("alignment", ["make", "alignment-test"]),
    ("go", ["go", "-C", "go/control-plane", "test", "./...", "-count=1"]),
    ("topic-script", ["bash", "-n", "common/kafka/create-topics.sh"]),
)
PROBE_CONTROL_EDGE_COMMANDS = (
    ("alignment", ["make", "alignment-test"]),
    ("proto-lint", ["buf", "lint", "proto"]),
    ("go", ["go", "-C", "go/control-plane", "test", "./...", "-count=1"]),
    (
        "rust-control",
        [
            "cargo",
            "test",
            "--manifest-path",
            "rust/probe-agent/Cargo.toml",
            "-p",
            "probe-agent",
            "control::",
            "--lib",
        ],
    ),
    (
        "rust-agent-check",
        [
            "cargo",
            "check",
            "--manifest-path",
            "rust/probe-agent/Cargo.toml",
            "-p",
            "probe-agent",
            "--bin",
            "probe-agent",
        ],
    ),
    (
        "java-proto-consumer",
        [
            "mvn",
            "-q",
            "-f",
            "java/flink-jobs/pom.xml",
            "-pl",
            "flink-common",
            "-am",
            "test",
        ],
    ),
    ("topic-script", ["bash", "-n", "common/kafka/create-topics.sh"]),
)


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _git_snapshot() -> dict[str, Any]:
    def output(command: list[str]) -> str:
        return subprocess.run(
            command,
            cwd=ROOT,
            check=True,
            text=True,
            capture_output=True,
        ).stdout.strip()

    status = output(["git", "status", "--porcelain=v2", "--untracked-files=all"])
    return {
        "root": str(ROOT),
        "head": output(["git", "rev-parse", "HEAD"]),
        "branch": output(["git", "branch", "--show-current"]),
        "status": status.splitlines(),
        "status_sha256": hashlib.sha256(status.encode()).hexdigest(),
    }


def _run_gate(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started_at = datetime.now(timezone.utc)
    print(f"[G0] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        environment = os.environ.copy()
        environment["GOSUMDB"] = environment.get("TRAFFIC_GO_SUMDB", "sum.golang.org")
        completed = subprocess.run(
            command,
            cwd=ROOT,
            stdout=log,
            stderr=subprocess.STDOUT,
            env=environment,
            check=False,
        )
    finished_at = datetime.now(timezone.utc)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "started_at": started_at.isoformat(),
        "finished_at": finished_at.isoformat(),
        "duration_seconds": round((finished_at - started_at).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": _sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[G0] {name}: {result['status']} ({result['duration_seconds']}s)", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument(
        "--profile",
        choices=("full", "probe-publisher", "probe-control-edge"),
        default="full",
    )
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    parser.add_argument(
        "--inventory-manifest",
        type=Path,
        help="Optional immutable inventory/compatibility manifest produced by capture_baseline.py.",
    )
    args = parser.parse_args()

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)

    before = _git_snapshot()
    source_snapshot = build_snapshot()
    provenance = source_snapshot.get("artifact_provenance", {})
    if provenance.get("status") != "PASS":
        after = _git_snapshot()
        manifest = {
            "schema_version": 1,
            "run_id": args.run_id,
            "gate": "G0",
            "profile": args.profile,
            "status": "BLOCKED",
            "captured_at": datetime.now(timezone.utc).isoformat(),
            "candidate_before": before,
            "candidate_after": after,
            "candidate_source": source_snapshot,
            "commands": [],
            "inventory_reference": None,
            "blocking_stage": "candidate-artifact-provenance",
            "blocking_codes": provenance.get("blocking_codes", []),
            "scope": {
                "included": ["pre-execution candidate artifact provenance guard"],
                "excluded": ["all G0 commands because provenance failed closed"],
            },
            "g7_status": "OPEN",
            "g8_status": "BLOCKED",
        }
        manifest_path = output / "manifest.json"
        manifest_path.write_text(
            json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        print(json.dumps({
            "status": "BLOCKED",
            "blocking_stage": manifest["blocking_stage"],
            "blocking_codes": manifest["blocking_codes"],
            "manifest": str(manifest_path),
            "manifest_sha256": _sha256(manifest_path),
        }, ensure_ascii=False, indent=2), flush=True)
        return 2
    commands = {
        "full": FULL_COMMANDS,
        "probe-publisher": PROBE_PUBLISHER_COMMANDS,
        "probe-control-edge": PROBE_CONTROL_EDGE_COMMANDS,
    }[args.profile]
    results: list[dict[str, Any]] = []
    for name, command in commands:
        results.append(_run_gate(name, list(command), output))
        if results[-1]["exit_code"] != 0:
            break
    after = _git_snapshot()

    inventory_reference = None
    if args.inventory_manifest:
        inventory_path = args.inventory_manifest.resolve()
        if not inventory_path.is_file():
            raise SystemExit(f"inventory manifest does not exist: {inventory_path}")
        inventory_reference = {
            "path": str(inventory_path),
            "sha256": _sha256(inventory_path),
        }

    status = "PASS" if len(results) == len(commands) and all(item["status"] == "PASS" for item in results) else "FAIL"
    if args.profile == "full":
        included = [
            "alignment registry/contracts/OpenAPI/migrations/IAM checks",
            "Proto generation and lint",
            "Go tests",
            "Web ESLint, production build and tests",
            "Java/Flink reactor tests",
            "Rust workspace tests",
            "Python/MLOps tests",
        ]
    elif args.profile == "probe-control-edge":
        included = [
            "alignment registry/contracts/OpenAPI/migrations/IAM checks",
            "Proto lint",
            "all Go tests including Gateway control routing",
            "Rust Agent control processor tests",
            "Rust Agent binary compile check",
            "Java Flink common Proto consumer tests",
            "Kafka topic shell syntax",
        ]
    else:
        included = [
            "alignment registry/contracts/OpenAPI/migrations/IAM checks",
            "all Go tests",
            "Kafka topic shell syntax",
        ]
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "gate": "G0",
        "profile": args.profile,
        "status": status,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "candidate_before": before,
        "candidate_after": after,
        "candidate_source": source_snapshot,
        "commands": results,
        "inventory_reference": inventory_reference,
        "scope": {
            "included": included,
            "excluded": [
                "100-round live smoke",
                "production candidate deployment",
                "Windows Chrome acceptance",
                "G2-G6 real-service, reconciliation, performance, fault and rollback gates",
                "G8 100G/Mpps, HA, detection quality, site and third-party milestones",
            ],
        },
        "g7_status": "OPEN",
        "g8_status": "BLOCKED",
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": status,
        "manifest": str(manifest_path),
        "manifest_sha256": _sha256(manifest_path),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
