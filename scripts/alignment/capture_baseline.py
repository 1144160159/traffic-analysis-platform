#!/usr/bin/env python3
"""Capture an immutable W0 evidence package without mutating live services."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from compat_diff import compare
from inventory import build_inventory
from validate import validate


ROOT = Path(__file__).resolve().parents[2]
REPORT = Path("doc/07_alignment/前后端逻辑对齐整改报告-5万字完整版.md")
HASH_GLOBS = (
    "proto/traffic/v1/*.proto",
    "deployments/kubernetes/**/*.yaml",
    "common/kafka/*.sh",
    "java/flink-jobs/**/*.java",
    "go/control-plane/deployments/docker/init/*.sql",
    "web/ui/src/routes/routeManifest.tsx",
    "web/ui/src/services/pageApiPlans.ts",
    "go/control-plane/internal/auth/model/scopes.go",
    "contracts/alignment/**/*.json",
    "contracts/openapi/*.json",
)


def _run(command: list[str], cwd: Path) -> dict[str, Any]:
    completed = subprocess.run(command, cwd=cwd, text=True, capture_output=True, check=False)
    return {
        "command": command,
        "exit_code": completed.returncode,
        "stdout": completed.stdout,
        "stderr": completed.stderr,
    }


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _git_snapshot(root: Path) -> dict[str, Any]:
    head = _run(["git", "rev-parse", "HEAD"], root)
    branch = _run(["git", "branch", "--show-current"], root)
    status = _run(["git", "status", "--porcelain=v2", "--untracked-files=all"], root)
    return {
        "root": str(root),
        "head": head["stdout"].strip(),
        "branch": branch["stdout"].strip(),
        "status_exit_code": status["exit_code"],
        "status": status["stdout"].splitlines(),
        "status_sha256": hashlib.sha256(status["stdout"].encode()).hexdigest(),
    }


def _source_hashes(root: Path) -> list[dict[str, Any]]:
    files: set[Path] = set()
    for pattern in HASH_GLOBS:
        files.update(path for path in root.glob(pattern) if path.is_file())
    if (root / REPORT).is_file():
        files.add(root / REPORT)
    return [
        {
            "path": path.relative_to(root).as_posix(),
            "sha256": _sha256(path),
            "size_bytes": path.stat().st_size,
        }
        for path in sorted(files)
    ]


def _write_json(path: Path, value: Any) -> None:
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def _capture_live(output: Path, namespace: str) -> dict[str, Any]:
    command = [
        "kubectl",
        "get",
        "deployment,statefulset,daemonset,job,cronjob,service,configmap",
        "-n",
        namespace,
        "-o",
        "json",
    ]
    result = _run(command, ROOT)
    live_path = output / "live-resources.json"
    live_path.write_text(result["stdout"], encoding="utf-8")
    error_path = output / "live-resources.stderr.txt"
    error_path.write_text(result["stderr"], encoding="utf-8")
    return {
        "status": "captured" if result["exit_code"] == 0 else "blocked",
        "exit_code": result["exit_code"],
        "artifact": live_path.name,
        "stderr_artifact": error_path.name,
        "secrets_captured": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-worktree", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=ROOT / "doc/02_acceptance/runs")
    parser.add_argument("--run-id", default=datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ-alignment-w0"))
    parser.add_argument("--live", action="store_true")
    parser.add_argument("--namespace", default="traffic-analysis")
    args = parser.parse_args()

    source_worktree = args.source_worktree.resolve()
    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)

    baseline_inventory = build_inventory(source_worktree)
    candidate_inventory = build_inventory(ROOT)
    validation = validate(strict_w1=True)
    _write_json(output / "baseline-inventory.json", baseline_inventory)
    _write_json(output / "candidate-inventory.json", candidate_inventory)
    _write_json(output / "validation.json", validation)
    compatibility = compare(output / "baseline-inventory.json", output / "candidate-inventory.json")
    _write_json(output / "compatibility-diff.json", compatibility)

    live = _capture_live(output, args.namespace) if args.live else {
        "status": "not_requested",
        "secrets_captured": False,
    }
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "candidate": _git_snapshot(ROOT),
        "source_worktree": _git_snapshot(source_worktree),
        "source_hashes": _source_hashes(source_worktree),
        "candidate_hashes": _source_hashes(ROOT),
        "validation_result": validation["result"],
        "compatibility_result": compatibility["result"],
        "live": live,
        "g8_status": "BLOCKED",
        "g8_note": "100G/Mpps, HA, detection quality, site and third-party milestones are not decided by W0.",
    }
    artifacts = []
    for path in sorted(output.iterdir()):
        if path.name == "manifest.json" or not path.is_file():
            continue
        artifacts.append({"path": path.name, "sha256": _sha256(path), "size_bytes": path.stat().st_size})
    manifest["artifacts"] = artifacts
    _write_json(output / "manifest.json", manifest)
    print(json.dumps({
        "result": "pass" if validation["result"] == "pass" and compatibility["result"] == "pass" else "blocked",
        "run_id": args.run_id,
        "manifest": str(output / "manifest.json"),
        "manifest_sha256": _sha256(output / "manifest.json"),
        "live_status": live["status"],
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
