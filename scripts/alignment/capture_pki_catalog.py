#!/usr/bin/env python3
"""Capture immutable T-PKI-001 repository and public-certificate-only live evidence."""

from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import subprocess
import tempfile
from datetime import datetime, timezone
from email.utils import parsedate_to_datetime
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
CATALOG = ROOT / "contracts/security/pki-catalog.v1.json"
COMMANDS = (
    ("pki-catalog-current", ["python3", "scripts/alignment/build_pki_catalog.py", "--check"]),
    ("pki-catalog-verifier", ["python3", "scripts/alignment/verify_pki_catalog.py"]),
    ("pki-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_pki_catalog", "-v"]),
    ("minio-tls-material-verifier", ["python3", "scripts/alignment/verify_minio_tls_material.py"]),
    ("minio-tls-material-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_minio_tls_material", "-v"]),
    ("minio-tls-cutover-verifier", ["python3", "scripts/alignment/verify_minio_tls_cutover.py"]),
    ("minio-tls-cutover-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_minio_tls_cutover", "-v"]),
    ("ingest-production-guard-tests", ["go", "-C", "go/control-plane", "test", "./internal/ingest/config", "-count=1"]),
    ("probe-transport-guard-tests", ["cargo", "test", "--manifest-path", "rust/probe-agent/Cargo.toml", "-p", "probe-agent", "transport_tests", "--lib"]),
    ("deployment-shell-parse", ["bash", "-n", "deployments/kubernetes/deploy.sh"]),
    ("certificate-generator-shell-parse", ["bash", "-n", "rust/probe-agent/scripts/generate-mtls-certs.sh"]),
    ("ingest-workload-dry-run", ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/applications/go-services.yaml"]),
    ("probe-workload-dry-run", ["kubectl", "apply", "--dry-run=client", "-f", "deployments/kubernetes/applications/probe-agent.yaml"]),
)
SOURCE_ARTIFACTS = (
    "contracts/security/pki-catalog.v1.json",
    "scripts/alignment/build_pki_catalog.py",
    "scripts/alignment/verify_pki_catalog.py",
    "scripts/alignment/capture_pki_catalog.py",
    "tests/alignment/test_pki_catalog.py",
    "contracts/minio/tls-material.v1.json",
    "scripts/alignment/verify_minio_tls_material.py",
    "tests/alignment/test_minio_tls_material.py",
    "contracts/minio/tls-cutover.v1.json",
    "scripts/alignment/verify_minio_tls_cutover.py",
    "scripts/alignment/capture_minio_tls_candidate_images.py",
    "tests/alignment/test_minio_tls_cutover.py",
    "tests/alignment/test_minio_tls_candidate_images.py",
    "deployments/kubernetes/security/minio-tls-cutover-v1/kustomization.yaml",
    "deployments/kubernetes/security/minio-tls-cutover-v1/mlops-workflow.patch.json",
    "deployments/kubernetes/security/minio-tls-cutover-v1/README.md",
    "go/control-plane/deployments/docker/Dockerfile.alert-service",
    "go/control-plane/deployments/docker/Dockerfile.asset-service",
    "go/control-plane/deployments/docker/Dockerfile.forensics-service",
    "rust/probe-agent/docker/Dockerfile",
    "mlops/Dockerfile",
    "go/control-plane/internal/ingest/config/loader.go",
    "go/control-plane/internal/ingest/config/config.go",
    "go/control-plane/internal/ingest/config/config_test.go",
    "go/control-plane/cmd/ingest-gateway/main.go",
    "rust/probe-agent/probe-agent/src/config.rs",
    "rust/probe-agent/probe-agent/src/sender/grpc.rs",
    "rust/probe-agent/scripts/generate-mtls-certs.sh",
    "deployments/kubernetes/deploy.sh",
    "deployments/kubernetes/applications/go-services.yaml",
    "deployments/kubernetes/applications/probe-agent.yaml",
    "deployments/kubernetes/infrastructure/01-kafka.yaml",
    "deployments/kubernetes/infrastructure/08-gateway.yaml",
    "deployments/kubernetes/security/external-secrets-template.yaml",
    "deployments/kubernetes/security/minio-tls-material.v1.yaml",
    "deployments/kubernetes/security/opensearch-ha-v1/external-secrets.template.yaml",
    "doc/07_alignment/runbooks/T-PKI-001-certificate-transport-rotation.md",
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
    print(f"[pki] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, env=direct_environment(), stdout=log, stderr=subprocess.STDOUT, check=False)
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
    print(f"[pki] {name}: {result['status']}", flush=True)
    return result


def kubectl_text(arguments: list[str]) -> tuple[bool, str]:
    completed = subprocess.run(
        ["kubectl", "--request-timeout=10s", *arguments],
        cwd=ROOT,
        env=direct_environment(),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
        timeout=20,
    )
    return completed.returncode == 0, completed.stdout.strip()


def _secret_public_certificate(namespace: str, name: str, key: str) -> dict[str, Any]:
    found_meta, resource_version = kubectl_text(
        ["-n", namespace, "get", "secret", name, "-o", "jsonpath={.metadata.resourceVersion}"]
    )
    found_cert, encoded = kubectl_text(
        ["-n", namespace, "get", "secret", name, "-o", f"jsonpath={{.data.{key.replace('.', r'\.')}}}"]
    )
    if not found_meta or not found_cert or not encoded:
        return {
            "namespace": namespace,
            "secret_name": name,
            "certificate_key": key,
            "found": False,
            "resource_version": resource_version if found_meta else None,
            "public_certificate_captured": False,
            "private_key_captured": False,
        }
    try:
        pem = base64.b64decode(encoded, validate=True)
    except Exception:
        return {
            "namespace": namespace,
            "secret_name": name,
            "certificate_key": key,
            "found": True,
            "resource_version": resource_version,
            "parse_status": "INVALID_BASE64",
            "public_certificate_captured": False,
            "private_key_captured": False,
        }
    command = [
        "openssl", "x509", "-in", "/dev/stdin", "-noout", "-subject", "-issuer", "-serial",
        "-startdate", "-enddate", "-fingerprint", "-sha256", "-ext", "subjectAltName",
        "-ext", "extendedKeyUsage", "-ext", "basicConstraints",
    ]
    parsed = subprocess.run(command, input=pem, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    checkend = subprocess.run(
        ["openssl", "x509", "-in", "/dev/stdin", "-noout", "-checkend", "2592000"],
        input=pem,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
    )
    lines = parsed.stdout.decode("utf-8", errors="replace").splitlines()
    not_after = next((line.split("=", 1)[1] for line in lines if line.startswith("notAfter=")), None)
    days_remaining = None
    if not_after:
        days_remaining = round((parsedate_to_datetime(not_after) - datetime.now(timezone.utc)).total_seconds() / 86400, 3)
    return {
        "namespace": namespace,
        "secret_name": name,
        "certificate_key": key,
        "found": True,
        "resource_version": resource_version,
        "parse_status": "PASS" if parsed.returncode == 0 else "FAIL",
        "pem_sha256": hashlib.sha256(pem).hexdigest(),
        "public_metadata": lines if parsed.returncode == 0 else [],
        "days_remaining": days_remaining,
        "valid_beyond_30_days": checkend.returncode == 0,
        "public_certificate_captured": True,
        "private_key_captured": False,
    }


def _certificate_bytes(namespace: str, name: str, key: str) -> bytes | None:
    found, encoded = kubectl_text(
        ["-n", namespace, "get", "secret", name, "-o", f"jsonpath={{.data.{key.replace('.', r'\.')}}}"]
    )
    if not found or not encoded:
        return None
    try:
        return base64.b64decode(encoded, validate=True)
    except Exception:
        return None


def _probe_chain_validation() -> dict[str, Any]:
    client_ca = _certificate_bytes("traffic-analysis", "probe-agent-certs", "ca-cert.pem")
    client = _certificate_bytes("traffic-analysis", "probe-agent-certs", "client-cert.pem")
    server_ca = _certificate_bytes("traffic-analysis", "ingest-gateway-certs", "ca-cert.pem")
    server = _certificate_bytes("traffic-analysis", "ingest-gateway-certs", "server-cert.pem")
    if any(value is None for value in (client_ca, client, server_ca, server)):
        return {"status": "UNKNOWN_MISSING_PUBLIC_CERTIFICATE", "private_key_captured": False}
    assert client_ca is not None and client is not None and server_ca is not None and server is not None
    with tempfile.TemporaryDirectory(prefix="pki-public-cert-") as directory:
        root = Path(directory)
        paths = {}
        for name, value in (("client-ca", client_ca), ("client", client), ("server-ca", server_ca), ("server", server)):
            path = root / f"{name}.pem"
            path.write_bytes(value)
            paths[name] = path
        same_ca = client_ca == server_ca
        client_verify = subprocess.run(
            ["openssl", "verify", "-purpose", "sslclient", "-CAfile", str(paths["client-ca"]), str(paths["client"])],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
        ).returncode == 0
        server_verify = subprocess.run(
            ["openssl", "verify", "-purpose", "sslserver", "-verify_hostname", "ingest-gateway.traffic-analysis.svc", "-CAfile", str(paths["server-ca"]), str(paths["server"])],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=False,
        ).returncode == 0
    return {
        "status": "PASS" if same_ca and client_verify and server_verify else "FAIL",
        "same_ca": same_ca,
        "client_purpose_valid": client_verify,
        "server_purpose_and_hostname_valid": server_verify,
        "private_key_captured": False,
    }


def capture_live() -> dict[str, Any]:
    certificates = [
        _secret_public_certificate("traffic-analysis", "ingest-gateway-certs", "ca-cert.pem"),
        _secret_public_certificate("traffic-analysis", "ingest-gateway-certs", "server-cert.pem"),
        _secret_public_certificate("traffic-analysis", "probe-agent-certs", "ca-cert.pem"),
        _secret_public_certificate("traffic-analysis", "probe-agent-certs", "client-cert.pem"),
        _secret_public_certificate("middleware", "kafka-broker-tls", "ca.crt"),
        _secret_public_certificate("iam", "keycloak-tls", "tls.crt"),
        _secret_public_certificate("middleware", "opensearch-node-tls", "tls.crt"),
    ]
    return {
        "read_only": True,
        "public_certificates": certificates,
        "public_certificate_reference_count": len(certificates),
        "public_certificate_found_count": sum(item["found"] for item in certificates),
        "certificates_expiring_within_30_days": [
            f"{item['namespace']}/{item['secret_name']}:{item['certificate_key']}"
            for item in certificates
            if item.get("found") and item.get("parse_status") == "PASS" and item.get("valid_beyond_30_days") is False
        ],
        "probe_chain_validation": _probe_chain_validation(),
        "kafka_leaf_status": "UNKNOWN_ENCRYPTED_KEYSTORE_PASSWORD_NOT_READ",
        "external_issuer_live_status": "OPEN_NOT_CAPTURED",
        "private_keys_captured": False,
        "secret_passwords_captured": False,
        "production_mutations": [],
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
    try:
        live = capture_live()
        live["query_status"] = "PASS"
    except Exception as exc:
        live = {
            "read_only": True,
            "query_status": "FAIL",
            "error_class": type(exc).__name__,
            "private_keys_captured": False,
            "secret_passwords_captured": False,
            "production_mutations": [],
        }
    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    scoped_pass = repository_pass and live.get("query_status") == "PASS" and candidate_stable
    catalog = json.loads(CATALOG.read_text(encoding="utf-8"))
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
        "feature_id": "T-PKI-001",
        "related_ids": ["T-SEC-001", "T-KAFKA-001", "T-OS-005", "T-GW-001", "T-MINIO-003"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL_REPOSITORY_CERTIFICATE_TRANSPORT_ROTATION_GUARDS_AND_PUBLIC_CERTIFICATE_LIVE_METADATA",
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
        "catalog_summary": {
            "catalog_sha256": catalog.get("catalog_sha256"),
            "counts": catalog.get("counts"),
            "pki_compliance": "PARTIAL",
        },
        "live_observation": live,
        "production_applied": False,
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_VERSIONED_CERTIFICATE_DOMAIN_CATALOG_DOWNGRADE_NEGATIVES_SHORT_LEAF_PROFILE_AND_DEPLOY_READINESS_GUARDS" if scoped_pass else "FAIL",
            "G2": "PARTIAL_FOR_PUBLIC_CERTIFICATE_METADATA_AND_PROBE_CHAIN_VALIDATION",
            "G3": "OPEN_FOR_ALL_ISSUER_SECRET_WORKLOAD_AND_TRUST_BUNDLE_RECONCILIATION",
            "G4": "OPEN_FOR_HANDSHAKE_ROTATION_FAILURE_AND_RESOURCE_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_PUBLIC_CHAIN_HOSTNAME_AND_REVOCATION",
            "G6": "HOLD_FOR_DUAL_TRUST_SERIAL_CANARY_REVOKE_ROLLBACK_AND_TWO_ROTATIONS",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "remote probe transport cannot downgrade to plaintext or partial TLS",
            "production ingest startup requires mTLS token authentication and full validation",
            "probe certificate generation is SAN and EKU bound with a maximum 90-day leaf default",
            "existing probe certificates stop deployment inside the 30-day renewal window",
            "public live certificate metadata can be captured without private keys or passwords",
        ],
        "open": [
            "replace shared probe and broker certificates with unique identities from an approved issuer",
            "shorten and rotate Kafka and Keycloak leaves with dual trust and revocation evidence",
            "materialize and canary OpenSearch ExternalSecret issuer paths",
            "migrate PostgreSQL MinIO and remaining OpenSearch clients away from plaintext",
            "prove two rotations failures rollback performance Windows Chrome and T+ observation",
        ],
        "private_keys_captured": False,
        "secret_passwords_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": manifest["scoped_evidence_status"],
        "manifest": str(manifest_path.relative_to(ROOT)),
        "manifest_sha256": sha256(manifest_path),
        "catalog_sha256": catalog.get("catalog_sha256"),
        "live_query_status": live.get("query_status"),
        "public_certificate_found_count": live.get("public_certificate_found_count"),
        "probe_chain_status": (live.get("probe_chain_validation") or {}).get("status"),
        "private_keys_captured": False,
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
