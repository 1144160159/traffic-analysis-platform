#!/usr/bin/env python3
"""Fail closed on T-PKI-001 catalog, transport and rotation-guard drift."""

from __future__ import annotations

import hashlib
import json
from typing import Any

from build_pki_catalog import OUTPUT, ROOT, _canonical_sha256, build_catalog


EXPECTED_DOMAINS = {
    "probe_ingest_mtls",
    "kafka_tls_and_scram",
    "keycloak_https",
    "minio_server_tls_target",
    "opensearch_target_mtls",
}

EXPECTED_MINIO_SANS = {
    "minio",
    "minio.minio",
    "minio.minio.svc",
    "minio.minio.svc.cluster.local",
    *{f"minio-{index}.minio.minio.svc.cluster.local" for index in range(4)},
}


def verify() -> dict[str, Any]:
    errors: list[str] = []
    if not OUTPUT.is_file():
        return {"status": "FAIL", "errors": [f"missing {OUTPUT.relative_to(ROOT)}"]}
    actual = json.loads(OUTPUT.read_text(encoding="utf-8"))
    expected = build_catalog()
    if actual != expected:
        errors.append("PKI catalog is stale relative to governed authorities")
    if actual.get("schema_version") != 1 or actual.get("control_id") != "T-PKI-001":
        errors.append("catalog identity must be schema v1 and T-PKI-001")
    if actual.get("status") != "candidate_default_off" or actual.get("production_applied") is not False:
        errors.append("PKI catalog cannot claim rollout or closure without live evidence")

    content = dict(actual)
    catalog_sha256 = content.pop("catalog_sha256", None)
    if catalog_sha256 != _canonical_sha256(content):
        errors.append("catalog_sha256 does not match canonical catalog content")
    serialized = json.dumps(actual, ensure_ascii=False)
    for marker in ("BEGIN PRIVATE KEY", "BEGIN RSA PRIVATE KEY", "BEGIN EC PRIVATE KEY"):
        if marker in serialized:
            errors.append("private key material must never be serialized in the PKI catalog")

    for authority in actual.get("authorities") or []:
        path = ROOT / str(authority.get("path") or "")
        if not path.is_file():
            errors.append(f"authority missing: {authority.get('domain')}")
        elif authority.get("sha256") != hashlib.sha256(path.read_bytes()).hexdigest():
            errors.append(f"authority hash drift: {authority.get('domain')}")

    domains = actual.get("certificate_domains") or []
    domain_ids = {str(item.get("domain_id") or "") for item in domains}
    if domain_ids != EXPECTED_DOMAINS:
        errors.append(
            f"certificate domain inventory drift: missing={sorted(EXPECTED_DOMAINS-domain_ids)} "
            f"extra={sorted(domain_ids-EXPECTED_DOMAINS)}"
        )
    if any(item.get("status") == "PASS" for item in domains):
        errors.append("no certificate domain has enough live rotation evidence to claim PASS")
    for domain in domains:
        if not domain.get("blocking_gaps"):
            errors.append(f"{domain.get('domain_id')}: blocking gaps cannot be hidden")
        validity = domain.get("leaf_validity_days")
        if validity and validity > 90 and "leaf_validity_exceeds_90_days" not in domain.get("blocking_gaps", []):
            errors.append(f"{domain.get('domain_id')}: excessive leaf validity was hidden")

    probe = next((item for item in domains if item.get("domain_id") == "probe_ingest_mtls"), {})
    if not probe or not probe.get("leaf_validity_days") or probe["leaf_validity_days"] > 90:
        errors.append("probe leaf certificates must default to no more than 90 days")
    if probe.get("renewal_guard_days") != 30:
        errors.append("probe deployment readiness must stop at a 30-day renewal window")
    if (probe.get("client_identity") or {}).get("identity_cardinality") != "shared_daemonset_secret":
        errors.append("current shared probe certificate identity cannot be hidden")
    if "per_probe_certificate_identity_missing" not in probe.get("blocking_gaps", []):
        errors.append("per-probe identity gap cannot be hidden")

    minio = next((item for item in domains if item.get("domain_id") == "minio_server_tls_target"), {})
    if minio.get("status") != "CANDIDATE_DEFAULT_OFF":
        errors.append("MinIO TLS domain must remain candidate-default-off before cutover")
    minio_server = minio.get("server_identity") or {}
    if set(minio_server.get("dns_names") or []) != EXPECTED_MINIO_SANS:
        errors.append("MinIO TLS domain SAN inventory cannot be incomplete")
    if minio.get("renewal_guard_days") != 30 or minio.get("server_minimum_tls") != "TLS1.2":
        errors.append("MinIO TLS domain renewal or minimum-version guard drifted")
    if set(minio.get("client_ca_namespaces") or []) != {"argo", "flink", "traffic-analysis"}:
        errors.append("MinIO client CA namespace inventory cannot be hidden")
    for gap in (
        "candidate_images_and_live_cutover_evidence_incomplete",
        "live_minio_san_chain_expiry_hostname_and_handshake_unproven",
        "two_consecutive_live_rotations_unproven",
    ):
        if gap not in minio.get("blocking_gaps", []):
            errors.append(f"MinIO TLS blocking gap cannot be hidden: {gap}")

    guards = actual.get("transport_guards") or {}
    if not guards or not all(value is True for value in guards.values()):
        failed = sorted(key for key, value in guards.items() if value is not True)
        errors.append(f"required transport guards are not fail-closed: {failed}")
    plaintext = actual.get("plaintext_dependencies") or []
    production_gaps = [
        item for item in plaintext if str(item.get("classification") or "").startswith("production_gap")
    ]
    if len(production_gaps) < 3:
        errors.append("production plaintext dependency inventory was hidden")

    counts = actual.get("counts") or {}
    expected_counts = {
        "certificate_domains": len(domains),
        "domains_partial_or_default_off": sum(item.get("status") != "PASS" for item in domains),
        "transport_guards": len(guards),
        "transport_guards_passing": sum(value is True for value in guards.values()),
        "plaintext_dependencies": len(plaintext),
        "production_plaintext_gaps": len(production_gaps),
        "blocking_gaps": len(actual.get("blocking_gaps") or []),
    }
    if counts != expected_counts:
        errors.append("PKI catalog counts do not match catalog content")

    return {
        "status": "PASS" if not errors else "FAIL",
        "control_id": "T-PKI-001",
        "catalog_integrity": "PASS" if not errors else "FAIL",
        "pki_compliance": "PARTIAL",
        "catalog_sha256": actual.get("catalog_sha256"),
        "counts": counts,
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
