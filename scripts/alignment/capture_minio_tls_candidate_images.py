#!/usr/bin/env python3
"""Capture immutable, read-only evidence for locally built MinIO TLS candidate images."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot
from verify_minio_tls_cutover import CONTRACT, ROOT, verify as verify_tls_cutover


DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
EXPECTED_REMEDIATION_LABEL = "T-MINIO-003,T-PKI-001"


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _normalized_ref(value: str) -> str:
    return value.removeprefix("docker.io/")


def inspect_image(image: str) -> dict[str, Any]:
    completed = subprocess.run(
        ["docker", "image", "inspect", image],
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode:
        raise RuntimeError(f"docker image inspect failed for {image}: {completed.stderr.strip()}")
    payload = json.loads(completed.stdout)
    if not isinstance(payload, list) or len(payload) != 1 or not isinstance(payload[0], dict):
        raise RuntimeError(f"docker image inspect returned an unexpected payload for {image}")
    return payload[0]


def validate_inspection(contract: dict[str, Any], inspection: dict[str, Any]) -> list[str]:
    workload = str(contract.get("workload") or "unknown")
    errors: list[str] = []
    tags = {_normalized_ref(str(value)) for value in inspection.get("RepoTags") or []}
    if _normalized_ref(str(contract.get("image") or "")) not in tags:
        errors.append(f"{workload} inspected image tag does not match the contract")
    if inspection.get("Id") != contract.get("local_image_id"):
        errors.append(f"{workload} local image ID does not match the contract")
    if inspection.get("RepoDigests"):
        errors.append(f"{workload} unexpectedly has a registry repo digest while the contract is local-only")
    platform = f"{inspection.get('Os')}/{inspection.get('Architecture')}"
    if platform != contract.get("platform"):
        errors.append(f"{workload} image platform does not match the contract")
    config = inspection.get("Config") or {}
    labels = config.get("Labels") or {}
    source_sha = contract.get("component_source_sha256")
    if labels.get("traffic.analysis.component-source-sha256") != source_sha:
        errors.append(f"{workload} component source label does not match the contract")
    if labels.get("org.opencontainers.image.revision") != source_sha:
        errors.append(f"{workload} OCI revision label does not match the component source hash")
    if labels.get("traffic.analysis.remediation-ids") != EXPECTED_REMEDIATION_LABEL:
        errors.append(f"{workload} remediation label is incomplete")
    if contract.get("status") != "BUILT_LOCAL_NOT_DISTRIBUTED":
        errors.append(f"{workload} contract status is not local-only")
    if contract.get("registry_digest") is not None:
        errors.append(f"{workload} contract invents a registry digest")
    if contract.get("distributed_to_nodes") is not False or contract.get("signed") is not False:
        errors.append(f"{workload} contract invents distribution or signing")
    return errors


def sanitized_inspection(contract: dict[str, Any], inspection: dict[str, Any]) -> dict[str, Any]:
    config = inspection.get("Config") or {}
    labels = config.get("Labels") or {}
    return {
        "workload": contract.get("workload"),
        "status": contract.get("status"),
        "image": contract.get("image"),
        "local_image_id": inspection.get("Id"),
        "registry_repo_digests": inspection.get("RepoDigests") or [],
        "size_bytes": inspection.get("Size"),
        "created": inspection.get("Created"),
        "platform": f"{inspection.get('Os')}/{inspection.get('Architecture')}",
        "runtime": {
            "user": config.get("User"),
            "entrypoint": config.get("Entrypoint"),
            "cmd": config.get("Cmd"),
        },
        "labels": {
            "org.opencontainers.image.revision": labels.get("org.opencontainers.image.revision"),
            "traffic.analysis.component-source-sha256": labels.get(
                "traffic.analysis.component-source-sha256"
            ),
            "traffic.analysis.remediation-ids": labels.get("traffic.analysis.remediation-ids"),
        },
        "distributed_to_nodes": False,
        "signed": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("G0 manifest is not a passing G0 run")

    candidate = build_snapshot()
    g0_source = (g0.get("candidate_source") or {}).get("content_sha256")
    if candidate["content_sha256"] != g0_source:
        raise SystemExit(
            f"current source {candidate['content_sha256']} does not match G0 candidate {g0_source}"
        )
    verifier_result = verify_tls_cutover()
    if verifier_result["status"] != "PASS":
        raise SystemExit(f"MinIO TLS cutover verifier failed: {verifier_result['errors']}")

    contract_path = ROOT / CONTRACT
    contract = json.loads(contract_path.read_text(encoding="utf-8"))
    captured: list[dict[str, Any]] = []
    errors: list[str] = []
    for item in contract.get("candidate_images") or []:
        inspection = inspect_image(str(item.get("image") or ""))
        errors.extend(validate_inspection(item, inspection))
        captured.append(sanitized_inspection(item, inspection))
    if errors:
        raise SystemExit("candidate image inspection failed: " + "; ".join(errors))

    output.mkdir(parents=True)
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "gate": "G1_MINIO_TLS_LOCAL_CANDIDATE_IMAGES",
        "status": "PASS",
        "coverage_status": "LOCAL_BUILD_ONLY_NOT_DISTRIBUTED_NOT_SIGNED_NOT_DEPLOYED",
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "candidate_source": {
            "content_sha256": candidate["content_sha256"],
            "file_count": candidate["file_count"],
        },
        "g0": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
        },
        "contract": {
            "path": str(CONTRACT),
            "sha256": sha256(contract_path),
        },
        "verification": verifier_result,
        "images": captured,
        "image_count": len(captured),
        "production_mutations": [],
        "distribution_attempted": False,
        "registry_push_attempted": False,
        "cluster_import_attempted": False,
        "deployment_attempted": False,
        "remaining_gates": [
            "registry signing and immutable repo-digest pinning",
            "distribution to both target nodes",
            "approved external Secret material",
            "approved coordinated cutover window",
            "live handshake savepoint restore reconcile fault rollback and observation evidence",
        ],
        "g7_status": "OPEN",
        "g8_status": "BLOCKED",
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(
        json.dumps(
            {
                "status": "PASS",
                "manifest": str(manifest_path),
                "manifest_sha256": sha256(manifest_path),
                "image_count": len(captured),
                "coverage_status": manifest["coverage_status"],
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
