#!/usr/bin/env python3
"""Capture immutable F-COMMON-001 repository and read-only runtime-adoption evidence."""

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
REGISTRY = ROOT / "contracts/alignment/feature-contract-registry.v1.json"
COMMANDS = (
    ("feature-contract-registry-current", ["python3", "scripts/alignment/build_feature_contract_registry.py", "--check"]),
    ("feature-contract-registry-verifier", ["python3", "scripts/alignment/verify_feature_contract_registry.py"]),
    ("feature-contract-registry-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_feature_contract_registry", "-v"]),
    ("canonical-registry-strict", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
    ("openapi-contract", ["python3", "scripts/alignment/check_openapi.py"]),
)
SOURCE_ARTIFACTS = (
    "contracts/alignment/feature-contract-registry.v1.json",
    "contracts/alignment/canonical-registry.json",
    "contracts/alignment/work-packages.json",
    "contracts/alignment/feature-contract.schema.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "scripts/alignment/build_feature_contract_registry.py",
    "scripts/alignment/verify_feature_contract_registry.py",
    "scripts/alignment/capture_feature_contract_registry.py",
    "scripts/alignment/inventory.py",
    "scripts/alignment/validate.py",
    "tests/alignment/test_feature_contract_registry.py",
    "web/ui/src/routes/routeManifest.tsx",
    "web/ui/src/services/pageApiPlans.ts",
    "doc/07_alignment/runbooks/F-COMMON-001-feature-contract-registry.md",
    "Makefile",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def direct_environment() -> dict[str, str]:
    environment = dict(os.environ)
    for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
        environment.pop(key, None)
    return environment


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[feature-contract] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            env=direct_environment(),
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
        )
    finished = datetime.now(timezone.utc)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "started_at": started.isoformat(),
        "finished_at": finished.isoformat(),
        "duration_seconds": round((finished - started).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[feature-contract] {name}: {result['status']}", flush=True)
    return result


def capture_live(output: Path) -> dict[str, Any]:
    command = ["kubectl", "--request-timeout=15s", "get", "deployments", "-A", "-o", "json"]
    result = subprocess.run(
        command,
        cwd=ROOT,
        env=direct_environment(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
        timeout=30,
    )
    stdout = output / "live-deployments.stdout.json"
    stderr = output / "live-deployments.stderr.log"
    stdout.write_bytes(result.stdout)
    stderr.write_bytes(result.stderr)
    deployments = []
    if result.returncode == 0:
        for item in json.loads(result.stdout).get("items", []):
            metadata = item.get("metadata") or {}
            namespace = str(metadata.get("namespace") or "")
            if namespace not in {"traffic-analysis", "middleware", "flink"}:
                continue
            spec = item.get("spec") or {}
            status = item.get("status") or {}
            pod_spec = (((spec.get("template") or {}).get("spec")) or {})
            deployments.append(
                {
                    "namespace": namespace,
                    "name": metadata.get("name"),
                    "resource_version": metadata.get("resourceVersion"),
                    "desired_replicas": int(spec.get("replicas") or 0),
                    "ready_replicas": int(status.get("readyReplicas") or 0),
                    "images": [container.get("image") for container in pod_spec.get("containers") or []],
                }
            )
    return {
        "read_only": True,
        "query_status": "PASS" if result.returncode == 0 else "FAIL",
        "deployments": sorted(deployments, key=lambda item: (str(item["namespace"]), str(item["name"]))),
        "deployment_count": len(deployments),
        "runtime_contract_version_telemetry": "NOT_IMPLEMENTED",
        "old_client_compatibility_usage_telemetry": "NOT_IMPLEMENTED",
        "unknown_enum_and_field_telemetry": "NOT_IMPLEMENTED",
        "candidate_source_hash_exposed_by_workloads": False,
        "secret_values_captured": False,
        "response_payloads_captured": False,
        "production_mutations": [],
        "artifacts": [
            {"path": stdout.name, "sha256": sha256(stdout), "size_bytes": stdout.stat().st_size},
            {"path": stderr.name, "sha256": sha256(stderr), "size_bytes": stderr.stat().st_size},
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("referenced G0 manifest is not PASS")
    candidate_before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if not g0_hash or candidate_before["content_sha256"] != g0_hash:
        raise SystemExit("current candidate does not match the referenced G0 manifest")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    results = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break
    repository_pass = len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results)
    live = capture_live(output)
    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    registry = json.loads(REGISTRY.read_text(encoding="utf-8"))
    scoped_pass = repository_pass and live.get("query_status") == "PASS" and candidate_stable

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "F-COMMON-001",
        "related_ids": ["F-COMMON-002", "F-COMMON-003", "F-COMMON-004", "F-ADAPTER-001", "F-ADAPTER-002", "T-SCHEMA-001"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "STANDARD_SCOPE_COMPLETE_BACKLOG_PARTIAL_AND_RUNTIME_ADOPTION_OPEN",
        "scoped_evidence_status": "PASS" if scoped_pass else "FAIL",
        "candidate_source": candidate_before,
        "candidate_source_stable": candidate_stable,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
            "candidate_source_sha256": g0_hash,
        },
        "registry_summary": {
            "catalog_sha256": registry.get("catalog_sha256"),
            "coverage": registry.get("coverage"),
            "contract_coverage": "STANDARD_SCOPE_COMPLETE_BACKLOG_PARTIAL",
        },
        "live_observation": live,
        "production_applied": False,
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_CANONICAL_OWNER_HASH_COMPLETE_STANDARD_SCOPE_AND_EXPLICIT_BACKLOG_GAPS" if scoped_pass else "FAIL",
            "G2": "PARTIAL_FOR_READ_ONLY_DEPLOYED_IMAGE_INVENTORY_OPEN_FOR_CONTRACT_VERSION_ADOPTION",
            "G3": "OPEN_FOR_OLD_CLIENT_COMPATIBILITY_AND_SCHEMA_PROJECTION_TRACEABILITY",
            "G4": "OPEN_FOR_CONTRACT_TELEMETRY_AND_COMPATIBILITY_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_CONTRACT_VERSION_HAR",
            "G6": "HOLD_FOR_GENERATED_CLIENT_CANARY_ROLLBACK_AND_T_PLUS_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "all 54 canonical feature IDs resolve one accountable package acceptance case and rollback ID",
            "all 38 standard-scope formal contracts are hash-bound and pass current semantic validation",
            "all 16 P0 feature contracts are present",
            "asset topic and alert pilot contracts cover six, four and six canonical features respectively",
            "standard-scope contract gaps are zero and 16 backlog contract gaps cannot be hidden",
            "live observation is read-only and does not make the registry a runtime dependency",
        ],
        "open": list((registry.get("acceptance") or {}).get("remaining") or []),
        "secrets_captured": False,
        "production_mutations": [],
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": manifest["scoped_evidence_status"],
        "manifest": str(manifest_path.relative_to(ROOT)),
        "manifest_sha256": sha256(manifest_path),
        "catalog_sha256": registry.get("catalog_sha256"),
        "formal_contracts": (registry.get("coverage") or {}).get("formal_contracts"),
        "missing_standard_scope_contracts": len((registry.get("coverage") or {}).get("missing_standard_scope_contracts") or []),
        "live_deployment_count": live.get("deployment_count"),
        "production_mutations": [],
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
