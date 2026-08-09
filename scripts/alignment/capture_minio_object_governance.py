#!/usr/bin/env python3
"""Capture immutable G1 evidence for MinIO object, identity and TLS-cutover governance."""

from __future__ import annotations

import argparse
import hashlib
import json
from datetime import datetime, timezone
from pathlib import Path

from candidate_snapshot import build_snapshot
from verify_minio_object_governance import CONTRACT, ROOT, verify as verify_object_governance
from verify_minio_service_identities import (
    CONTRACT as IDENTITY_CONTRACT,
    verify as verify_service_identities,
)
from verify_minio_tls_material import (
    CONTRACT as TLS_CONTRACT,
    verify as verify_tls_material,
)
from verify_minio_tls_cutover import (
    CONTRACT as TLS_CUTOVER_CONTRACT,
    verify as verify_tls_cutover,
)


DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"


def resolve_artifact(path: Path) -> tuple[Path, str]:
    """Return an absolute artifact path and its repository-relative name."""
    absolute = path if path.is_absolute() else ROOT / path
    return absolute, str(absolute.relative_to(ROOT))


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


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

    snapshot = build_snapshot()
    g0_source = (g0.get("candidate_source") or {}).get("content_sha256")
    if g0_source != snapshot["content_sha256"]:
        raise SystemExit(
            f"current source {snapshot['content_sha256']} does not match G0 candidate {g0_source}"
        )
    object_result = verify_object_governance()
    if object_result["status"] != "PASS":
        raise SystemExit(f"MinIO object-governance verifier failed: {object_result['errors']}")
    identity_result = verify_service_identities()
    if identity_result["status"] != "PASS":
        raise SystemExit(f"MinIO service-identity verifier failed: {identity_result['errors']}")
    tls_result = verify_tls_material()
    if tls_result["status"] != "PASS":
        raise SystemExit(f"MinIO TLS-material verifier failed: {tls_result['errors']}")
    tls_cutover_result = verify_tls_cutover()
    if tls_cutover_result["status"] != "PASS":
        raise SystemExit(f"MinIO TLS-cutover verifier failed: {tls_cutover_result['errors']}")

    object_contract, object_contract_name = resolve_artifact(CONTRACT)
    identity_contract, identity_contract_name = resolve_artifact(IDENTITY_CONTRACT)
    tls_contract, tls_contract_name = resolve_artifact(TLS_CONTRACT)
    tls_cutover_contract, tls_cutover_contract_name = resolve_artifact(TLS_CUTOVER_CONTRACT)

    output.mkdir(parents=True)
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "gate": "G1_MINIO_OBJECT_IDENTITY_AND_TLS_CUTOVER_GOVERNANCE",
        "status": "PASS",
        "coverage_status": "REPOSITORY_OBJECT_IDENTITY_AND_DEFAULT_OFF_TLS_CUTOVER_BASELINE_PARTIAL",
        "production_applied": False,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "candidate_source": {
            "content_sha256": snapshot["content_sha256"],
            "file_count": snapshot["file_count"],
        },
        "g0": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
        },
        "contracts": {
            "object_governance": {
                "path": object_contract_name,
                "sha256": sha256(object_contract),
            },
            "service_identities": {
                "path": identity_contract_name,
                "sha256": sha256(identity_contract),
            },
            "tls_material": {
                "path": tls_contract_name,
                "sha256": sha256(tls_contract),
            },
            "tls_cutover": {
                "path": tls_cutover_contract_name,
                "sha256": sha256(tls_cutover_contract),
            },
        },
        "verification": {
            "object_governance": object_result,
            "service_identities": identity_result,
            "tls_material": tls_result,
            "tls_cutover": tls_cutover_result,
        },
        "scope": {
            "included": [
                "bucket and object-class registry",
                "lifecycle catalog drift",
                "fail-closed model-writer configuration",
                "workflow Secret references and explicit transport mode",
                "seven scoped service identities and exact IAM action/resource pairs",
                "ExternalSecret property mapping and eleven consumer manifests",
                "suspended expand-only identity bootstrap",
                "candidate-only MinIO server certificate and three-namespace client CA material contract",
                "default-off 14-component server proxy Go Rust Flink Python Argo and operations TLS cutover overlay",
                "eight required DNS SANs and false-cutover negative tests",
                "wildcard, root-credential, missing-secret and unsuspended-job negative tests",
                "four-server and PDB repository baseline",
                "false-closure negative tests",
            ],
            "excluded": [
                "production apply",
                "MinIO server certificate activation and live all-consumer CA adapter cutover",
                "live bucket policy SAN chain expiry hostname and TLS handshake verification",
                "object scrub and destructive cleanup",
                "credential rotation",
                "fault injection restore rollback and observation",
                "G8 external milestones",
            ],
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
        "status": "PASS",
        "manifest": str(manifest_path),
        "manifest_sha256": sha256(manifest_path),
    }, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
