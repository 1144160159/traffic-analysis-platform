#!/usr/bin/env python3
"""Build the redacted T-PKI-001 certificate and transport-governance catalog."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "contracts/security/pki-catalog.v1.json"
AUTHORITY_PATHS = {
    "cluster_secret_and_manual_issuer_workflow": ROOT / "deployments/kubernetes/deploy.sh",
    "probe_leaf_certificate_profile": ROOT / "rust/probe-agent/scripts/generate-mtls-certs.sh",
    "ingest_server_configuration": ROOT / "go/control-plane/internal/ingest/config/config.go",
    "ingest_production_guard": ROOT / "go/control-plane/internal/ingest/config/loader.go",
    "ingest_tls_listener": ROOT / "go/control-plane/cmd/ingest-gateway/main.go",
    "ingest_atomic_rotation": ROOT / "go/control-plane/internal/common/pki/reloader.go",
    "probe_transport_guard": ROOT / "rust/probe-agent/probe-agent/src/config.rs",
    "probe_grpc_transport": ROOT / "rust/probe-agent/probe-agent/src/sender/grpc.rs",
    "go_workload_bindings": ROOT / "deployments/kubernetes/applications/go-services.yaml",
    "probe_workload_bindings": ROOT / "deployments/kubernetes/applications/probe-agent.yaml",
    "kafka_broker_transport": ROOT / "deployments/kubernetes/infrastructure/01-kafka.yaml",
    "gateway_and_keycloak_transport": ROOT / "deployments/kubernetes/infrastructure/08-gateway.yaml",
    "external_secret_rotation_metadata": ROOT / "deployments/kubernetes/security/external-secrets-template.yaml",
    "minio_tls_material": ROOT / "deployments/kubernetes/security/minio-tls-material.v1.yaml",
    "opensearch_target_pki": ROOT / "deployments/kubernetes/security/opensearch-ha-v1/external-secrets.template.yaml",
}


def _relative(path: Path) -> str:
    return path.relative_to(ROOT).as_posix()


def _sha256(value: bytes | str) -> str:
    if isinstance(value, str):
        value = value.encode("utf-8")
    return hashlib.sha256(value).hexdigest()


def _canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return _sha256(payload)


def _text(relative: str) -> str:
    return (ROOT / relative).read_text(encoding="utf-8")


def _first_int(pattern: str, text: str, default: int | None = None) -> int | None:
    match = re.search(pattern, text, re.MULTILINE | re.DOTALL)
    return int(match.group(1)) if match else default


def build_catalog() -> dict[str, Any]:
    deploy = _text("deployments/kubernetes/deploy.sh")
    probe_generator = _text("rust/probe-agent/scripts/generate-mtls-certs.sh")
    probe_config = _text("rust/probe-agent/probe-agent/src/config.rs")
    ingest_loader = _text("go/control-plane/internal/ingest/config/loader.go")
    ingest_main = _text("go/control-plane/cmd/ingest-gateway/main.go")
    ingest_rotation = _text("go/control-plane/internal/common/pki/reloader.go")
    go_workloads = _text("deployments/kubernetes/applications/go-services.yaml")
    probe_workload = _text("deployments/kubernetes/applications/probe-agent.yaml")
    kafka = _text("deployments/kubernetes/infrastructure/01-kafka.yaml")
    external_secrets = _text("deployments/kubernetes/security/external-secrets-template.yaml")
    minio_tls_material = _text("deployments/kubernetes/security/minio-tls-material.v1.yaml")
    minio_tls_contract = json.loads(_text("contracts/minio/tls-material.v1.json"))
    minio_tls_cutover_contract = json.loads(_text("contracts/minio/tls-cutover.v1.json"))
    minio_tls_cutover_verifier = _text("scripts/alignment/verify_minio_tls_cutover.py")
    opensearch_external = _text(
        "deployments/kubernetes/security/opensearch-ha-v1/external-secrets.template.yaml"
    )

    probe_leaf_days = _first_int(
        r'LEAF_VALIDITY_DAYS="\$\{LEAF_VALIDITY_DAYS:-([0-9]+)\}"', probe_generator
    )
    kafka_leaf_days = _first_int(r'kafka\.crt" -days ([0-9]+)', deploy)
    keycloak_leaf_days = _first_int(r'tls\.crt".*?-days ([0-9]+)', deploy)
    domains = [
        {
            "domain_id": "probe_ingest_mtls",
            "owner": "probe-domain-owner",
            "current_issuer": "repository_generated_self_signed_ca",
            "server_identity": {
                "workload": "traffic-analysis/Deployment/ingest-gateway",
                "secret": "ingest-gateway-certs",
                "certificate_key": "server-cert.pem",
                "private_key_reference": "server-key.pem",
                "dns_names": [
                    "ingest-gateway",
                    "ingest-gateway.traffic-analysis.svc",
                    "ingest-gateway.traffic-analysis.svc.cluster.local",
                ],
                "extended_key_usage": ["serverAuth"],
            },
            "client_identity": {
                "workload": "traffic-analysis/DaemonSet/probe-agent",
                "secret": "probe-agent-certs",
                "certificate_key": "client-cert.pem",
                "private_key_reference": "client-key.pem",
                "dns_names": ["probe-agent"],
                "extended_key_usage": ["clientAuth"],
                "identity_cardinality": "shared_daemonset_secret",
            },
            "leaf_validity_days": probe_leaf_days,
            "renewal_guard_days": 30,
            "server_minimum_tls": "TLS1.3",
            "production_required": True,
            "rotation_mode": "default_off_atomic_manifest_dual_trust_crl_reload",
            "status": "PARTIAL",
            "blocking_gaps": [
                "per_probe_certificate_identity_missing",
                "approved_online_issuer_and_revocation_missing",
                "dual_trust_canary_and_rollback_unproven",
                "two_consecutive_live_rotations_unproven",
            ],
        },
        {
            "domain_id": "kafka_tls_and_scram",
            "owner": "kafka-platform-owner",
            "current_issuer": "repository_generated_self_signed_ca",
            "server_identity": {
                "workload": "middleware/StatefulSet/kafka",
                "secret": "kafka-broker-tls",
                "certificate_key": "kafka.keystore.p12",
                "private_key_reference": "embedded_in_keystore",
                "dns_name_count": 7,
                "identity_cardinality": "one_certificate_shared_by_three_brokers",
            },
            "client_identity": {
                "secret": "kafka-client-tls",
                "material": ["kafka.truststore.p12", "ca.crt"],
                "client_certificate_present": False,
                "authentication": "SCRAM_SHA_512",
            },
            "leaf_validity_days": kafka_leaf_days,
            "renewal_guard_days": None,
            "server_minimum_tls": "runtime_default_not_explicit",
            "production_required": True,
            "rotation_mode": "manual_regeneration_without_serial_canary",
            "status": "PARTIAL",
            "blocking_gaps": [
                "leaf_validity_exceeds_90_days",
                "per_broker_certificate_identity_missing",
                "client_certificate_identity_missing",
                "expiry_guard_and_dual_trust_rotation_missing",
                "minimum_tls_version_not_explicit",
            ],
        },
        {
            "domain_id": "keycloak_https",
            "owner": "security-platform-owner",
            "current_issuer": "self_signed_leaf",
            "server_identity": {
                "workload": "iam/Deployment/keycloak",
                "secret": "keycloak-tls",
                "certificate_key": "tls.crt",
                "private_key_reference": "tls.key",
                "dns_names": [
                    "keycloak",
                    "keycloak.iam",
                    "keycloak.iam.svc",
                    "keycloak.iam.svc.cluster.local",
                ],
            },
            "client_identity": None,
            "leaf_validity_days": keycloak_leaf_days,
            "renewal_guard_days": None,
            "server_minimum_tls": "runtime_default_not_explicit",
            "production_required": True,
            "rotation_mode": "create_only_if_missing",
            "status": "PARTIAL",
            "blocking_gaps": [
                "self_signed_leaf_without_approved_chain",
                "leaf_validity_exceeds_90_days",
                "expiry_guard_rotation_and_revocation_missing",
                "public_hostname_and_browser_chain_acceptance_unproven",
            ],
        },
        {
            "domain_id": "minio_server_tls_target",
            "owner": "minio-platform-owner",
            "current_issuer": "approved_external_secret_store_placeholder",
            "server_identity": {
                "workload": "minio/StatefulSet/minio",
                "secret": minio_tls_contract["server_identity"]["target_secret"],
                "certificate_key": "public.crt",
                "private_key_reference": "private.key",
                "ca_key": "ca.crt",
                "certs_dir": minio_tls_contract["server_identity"]["certs_dir"],
                "dns_names": minio_tls_contract["server_identity"]["required_dns_sans"],
                "identity_cardinality": "one_multi_san_certificate_for_four_servers",
            },
            "client_ca_namespaces": sorted(
                item["namespace"] for item in minio_tls_contract["client_ca_distribution"]
            ),
            "leaf_validity_days": None,
            "renewal_guard_days": minio_tls_contract["server_identity"]["renew_before_days"],
            "server_minimum_tls": minio_tls_contract["server_identity"]["minimum_tls_version"],
            "production_required": True,
            "rotation_mode": "external_secret_expand_then_all_consumer_serial_cutover",
            "status": "CANDIDATE_DEFAULT_OFF",
            "blocking_gaps": [
                "approved_minio_issuer_path_not_materialized",
                "candidate_images_and_live_cutover_evidence_incomplete",
                "live_minio_san_chain_expiry_hostname_and_handshake_unproven",
                "minio_dual_trust_rotation_revoke_and_rollback_unproven",
                "two_consecutive_live_rotations_unproven",
            ],
        },
        {
            "domain_id": "opensearch_target_mtls",
            "owner": "opensearch-platform-owner",
            "current_issuer": "approved_external_secret_store_placeholder",
            "server_identity": {
                "workload": "middleware/StatefulSet/opensearch",
                "secret": "opensearch-node-tls",
                "certificate_key": "tls.crt",
                "private_key_reference": "tls.key",
            },
            "client_identities": sorted(
                set(re.findall(r"name: ([a-z0-9-]+-tls)", opensearch_external))
            ),
            "leaf_validity_days": None,
            "renewal_guard_days": 14,
            "server_minimum_tls": "TLS1.2_target",
            "production_required": True,
            "rotation_mode": "external_secret_refresh_then_serial_canary_target",
            "status": "CANDIDATE_DEFAULT_OFF",
            "blocking_gaps": [
                "approved_issuer_paths_not_materialized",
                "candidate_not_applied",
                "certificate_san_chain_revocation_and_rotation_unproven",
            ],
        },
    ]

    guards = {
        "probe_remote_plaintext_rejected": (
            "plaintext gateway transport is restricted to loopback development endpoints"
            in probe_config
        ),
        "probe_https_requires_complete_mtls": (
            "https gateway requires the complete mTLS certificate set" in probe_config
        ),
        "probe_partial_identity_rejected": (
            "must include CA certificate, client certificate and client key together" in probe_config
        ),
        "probe_generator_has_no_fixed_node_ip": "IP:10.0.5.210" not in probe_generator,
        "probe_leaf_validity_at_most_90_days": bool(probe_leaf_days and probe_leaf_days <= 90),
        "probe_existing_certificates_checked_for_30_days": (
            "openssl x509 -checkend 2592000" in deploy
        ),
        "probe_existing_chain_and_hostname_verified": (
            "openssl verify -purpose sslclient" in deploy
            and "-verify_hostname ingest-gateway.traffic-analysis.svc" in deploy
        ),
        "ingest_production_requires_mtls": (
            "REQUIRE_MTLS must be enabled in production environment" in ingest_loader
        ),
        "ingest_production_rejects_anonymous_token_mode": (
            "ALLOW_NO_TOKEN must be disabled in production environment" in ingest_loader
        ),
        "ingest_load_runs_full_validation": "if err := cfg.Validate(); err != nil" in ingest_loader,
        "ingest_server_requires_verified_client_certificate": (
            "tls.RequireAndVerifyClientCert" in ingest_main
        ),
        "ingest_server_minimum_tls13": "MinVersion:   tls.VersionTLS13" in ingest_main,
        "ingest_rotation_binds_all_material_by_manifest": all(
            token in ingest_rotation
            for token in (
                "CertificateSHA256",
                "PrivateKeySHA256",
                "TrustSHA256",
                "RevocationSHA256",
                "digest does not match generation manifest",
            )
        ),
        "ingest_rotation_checks_expiry_san_and_revocation": all(
            token in ingest_rotation
            for token in (
                "MinimumRemaining",
                "ServerDNSNames",
                "ClientDNSNames",
                "client certificate is revoked",
                "issuer has no current revocation list",
            )
        ),
        "ingest_rotation_publishes_only_valid_snapshot": all(
            token in ingest_rotation
            for token in (
                "config, err := r.buildTLSConfig(material)",
                "r.current.Store(&snapshot",
                "Invalid candidates",
            )
        ),
        "ingest_rotation_is_workload_default_off": all(
            token in go_workloads
            for token in (
                'name: TLS_ROTATION_V1_ENABLED, value: "false"',
                'name: TLS_GENERATION_MANIFEST_FILE',
                'name: TLS_CRL_FILE',
                'name: TLS_SERVER_DNS_NAMES',
                'name: TLS_CLIENT_DNS_NAMES',
            )
        ),
        "probe_generator_emits_crl_and_atomic_manifest": all(
            token in probe_generator
            for token in (
                "openssl ca -gencrl",
                "client-crl.pem",
                "generation.json",
                '"revocation_sha256"',
            )
        ),
        "ingest_workload_declares_production_mtls": all(
            token in go_workloads
            for token in (
                'name: ENVIRONMENT, value: "production"',
                'name: REQUIRE_MTLS, value: "true"',
                "secretName: ingest-gateway-certs, optional: false",
            )
        ),
        "probe_workload_declares_https_and_identity": all(
            token in probe_workload
            for token in (
                'gateway_addr: "https://ingest-gateway.traffic-analysis.svc:50051"',
                'tls_ca_cert: "/etc/probe-agent/certs/ca-cert.pem"',
                'tls_client_cert: "/etc/probe-agent/certs/client-cert.pem"',
                'tls_client_key: "/etc/probe-agent/certs/client-key.pem"',
            )
        ),
        "kafka_transport_uses_sasl_ssl": (
            "listeners=SASL_SSL://" in kafka and "allow.everyone.if.no.acl.found=false" in kafka
        ),
        "kafka_client_hostname_verification_not_disabled": "ssl.endpoint.identification.algorithm=" not in kafka,
        "external_secret_refresh_metadata_present": "refreshInterval:" in external_secrets,
        "minio_tls_bundle_is_default_off_and_guarded": all(
            (
                minio_tls_contract.get("phase") == "adapter_bundle_built_default_off",
                minio_tls_contract.get("production_applied") is False,
                minio_tls_contract.get("cutover_ready") is False,
                minio_tls_cutover_contract.get("phase") == "default_off_atomic_bundle",
                minio_tls_cutover_contract.get("default_off") is True,
                minio_tls_cutover_contract.get("production_applied") is False,
                minio_tls_cutover_contract.get("cutover_ready") is False,
                minio_tls_material.count('traffic.platform/production-applied: "false"') == 4,
                minio_tls_material.count('traffic.platform/cutover-ready: "false"') == 4,
                "BEGIN PRIVATE KEY" not in minio_tls_material,
                "def verify(" in minio_tls_cutover_verifier,
            )
        ),
    }

    plaintext_dependencies = [
        {
            "binding_id": "postgresql_application_clients",
            "observed": "sslmode=disable",
            "classification": "production_gap",
            "owner": "postgres-platform-owner",
        },
        {
            "binding_id": "minio_application_and_init_clients",
            "observed": "http://minio.minio.svc:9000",
            "classification": "production_gap_with_default_off_tls_bundle",
            "owner": "minio-platform-owner",
        },
        {
            "binding_id": "opensearch_current_application_clients",
            "observed": "http://opensearch.middleware.svc:9200",
            "classification": "production_gap_with_default_off_tls_target",
            "owner": "opensearch-platform-owner",
        },
        {
            "binding_id": "cluster_local_health_and_metrics",
            "observed": "loopback_or_cluster_internal_http",
            "classification": "requires_network_policy_and_non_secret_payload_review",
            "owner": "sre-platform-owner",
        },
    ]
    all_gaps = sorted({gap for domain in domains for gap in domain["blocking_gaps"]})
    catalog: dict[str, Any] = {
        "schema_version": 1,
        "control_id": "T-PKI-001",
        "status": "candidate_default_off",
        "production_applied": False,
        "authorities": [
            {
                "domain": domain,
                "path": _relative(path),
                "sha256": _sha256(path.read_bytes()),
            }
            for domain, path in sorted(AUTHORITY_PATHS.items())
        ],
        "policy": {
            "remote_transport": "TLS with hostname verification; mTLS where client identity is required",
            "plaintext_exception": "loopback development only or explicitly reviewed internal non-secret health/metrics",
            "leaf_maximum_validity_days": 90,
            "renew_before_days": 30,
            "private_key_serialization": "forbidden",
            "rotation": "expand dual trust serial canary cutover revoke rollback and two consecutive live rotations",
            "revocation": "issuer-specific revocation or short-lived identity replacement must be proven",
        },
        "certificate_domains": domains,
        "transport_guards": guards,
        "plaintext_dependencies": plaintext_dependencies,
        "blocking_gaps": all_gaps,
        "counts": {
            "certificate_domains": len(domains),
            "domains_partial_or_default_off": sum(
                domain["status"] != "PASS" for domain in domains
            ),
            "transport_guards": len(guards),
            "transport_guards_passing": sum(bool(value) for value in guards.values()),
            "plaintext_dependencies": len(plaintext_dependencies),
            "production_plaintext_gaps": sum(
                item["classification"].startswith("production_gap")
                for item in plaintext_dependencies
            ),
            "blocking_gaps": len(all_gaps),
        },
        "acceptance": {
            "repository": [
                "catalog and authority hashes are current",
                "remote probe transport cannot downgrade to plaintext or partial TLS",
                "ingest production startup requires mTLS token auth and full config validation",
                "probe leaf profile has bounded validity SAN EKU and key permissions",
                "existing probe certificates fail deploy readiness within the renewal window",
                "default-off ingest rotation binds certificate key trust CRL and generation before atomic publish",
                "repository handshakes reject wrong CA expiry SAN mismatch revocation and interrupted rotation",
                "MinIO server and three client namespaces have an expand-only redacted TLS material contract",
            ],
            "remaining": [
                "approved issuer inventory and offline root custody",
                "per-workload and per-probe unique identities",
                "live SAN chain EKU expiry revocation and trust reconciliation",
                "two consecutive rotations dual trust canary rollback and T+ observation",
                "PostgreSQL MinIO OpenSearch Kafka and browser transport migration",
                "performance fault Windows Chrome independent G7 and external G8 gates",
            ],
        },
    }
    catalog["catalog_sha256"] = _canonical_sha256(catalog)
    return catalog


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    catalog = build_catalog()
    rendered = json.dumps(catalog, ensure_ascii=False, indent=2) + "\n"
    if args.check:
        current = OUTPUT.read_text(encoding="utf-8") if OUTPUT.is_file() else ""
        status = "PASS" if current == rendered else "FAIL"
        print(json.dumps({"status": status, "catalog_sha256": catalog["catalog_sha256"], "counts": catalog["counts"]}, ensure_ascii=False, indent=2))
        return 0 if status == "PASS" else 1
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT.write_text(rendered, encoding="utf-8")
    print(json.dumps({"status": "PASS", "output": _relative(OUTPUT), "catalog_sha256": catalog["catalog_sha256"], "counts": catalog["counts"]}, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
