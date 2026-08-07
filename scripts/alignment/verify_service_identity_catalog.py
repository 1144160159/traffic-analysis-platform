#!/usr/bin/env python3
"""Fail closed on T-SEC-001 identity, secret and tenant-authority drift."""

from __future__ import annotations

import hashlib
import json
from collections import defaultdict
from typing import Any

from build_service_identity_catalog import (
    GO_SERVICE_NAMES,
    OUTPUT,
    PRIVILEGED_EXCEPTIONS,
    ROOT,
    SENSITIVE_NAME,
    _tenant_fallback_sites,
    build_catalog,
)


EXPECTED_WORKLOADS = GO_SERVICE_NAMES | {"web-ui", "probe-agent", "flink-log-job"}
REQUIRED_WORKLOAD_FIELDS = {
    "workload_id",
    "name",
    "namespace",
    "kind",
    "owner",
    "source",
    "service_account",
    "containers",
    "images",
    "secret_references",
    "literal_secret_findings",
    "duplicate_environment_bindings",
    "kafka_identity",
    "network_policy_authority",
    "privileged_exception",
    "blocking_gaps",
}


def _canonical_sha256(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def verify() -> dict[str, Any]:
    errors: list[str] = []
    if not OUTPUT.is_file():
        return {"status": "FAIL", "errors": [f"missing {OUTPUT.relative_to(ROOT)}"]}
    actual = json.loads(OUTPUT.read_text(encoding="utf-8"))
    expected = build_catalog()
    if actual != expected:
        errors.append("service identity catalog is stale relative to governed authorities")
    if actual.get("schema_version") != 1 or actual.get("control_id") != "T-SEC-001":
        errors.append("catalog identity must be schema v1 and T-SEC-001")
    if actual.get("status") != "candidate_default_off" or actual.get("production_applied") is not False:
        errors.append("candidate must remain default-off until production rollout evidence exists")
    content = dict(actual)
    catalog_sha256 = content.pop("catalog_sha256", None)
    if catalog_sha256 != _canonical_sha256(content):
        errors.append("catalog_sha256 does not match canonical catalog content")

    for authority in actual.get("authorities") or []:
        path = ROOT / str(authority.get("path") or "")
        if not path.is_file():
            errors.append(f"authority missing: {authority.get('domain')}")
        elif authority.get("sha256") != hashlib.sha256(path.read_bytes()).hexdigest():
            errors.append(f"authority hash drift: {authority.get('domain')}")

    workloads = actual.get("workloads") or []
    names = {str(workload.get("name") or "") for workload in workloads}
    if names != EXPECTED_WORKLOADS:
        errors.append(
            f"workload inventory drift: missing={sorted(EXPECTED_WORKLOADS-names)} "
            f"extra={sorted(names-EXPECTED_WORKLOADS)}"
        )
    ids = [str(workload.get("workload_id") or "") for workload in workloads]
    if len(ids) != len(set(ids)) or any(not value for value in ids):
        errors.append("workload IDs must be non-empty and unique")

    observed_secret_consumers: dict[tuple[str, str, str], set[str]] = defaultdict(set)
    for workload in workloads:
        name = str(workload.get("name") or "")
        missing = sorted(REQUIRED_WORKLOAD_FIELDS - set(workload))
        if missing:
            errors.append(f"{name}: missing metadata {missing}")
        service_account = workload.get("service_account") or {}
        if name in GO_SERVICE_NAMES | {"web-ui", "flink-log-job"}:
            if service_account.get("name") in {None, "", "default"} or not service_account.get("declared"):
                errors.append(f"{name}: dedicated ServiceAccount is required")
            if service_account.get("pod_token_automount") is not False:
                errors.append(f"{name}: Kubernetes API token automount must be disabled")
        if name == "probe-agent":
            exception = workload.get("privileged_exception") or {}
            if not exception.get("service_account_token_required"):
                errors.append("probe-agent: privileged token exception must be explicit")
        for container in workload.get("containers") or []:
            if name in GO_SERVICE_NAMES and not container.get("hardened"):
                errors.append(f"{name}/{container.get('name')}: container hardening is incomplete")
            if container.get("privileged") and name not in PRIVILEGED_EXCEPTIONS:
                errors.append(f"{name}/{container.get('name')}: unapproved privileged container")
        if workload.get("literal_secret_findings"):
            errors.append(f"{name}: literal secret material is forbidden")
        if workload.get("duplicate_environment_bindings"):
            errors.append(f"{name}: duplicate environment keys are forbidden")
        for reference in workload.get("secret_references") or []:
            if "value" in reference:
                errors.append(f"{name}: secret values must never be serialized")
            key = (
                str(reference.get("namespace") or ""),
                str(reference.get("secret_name") or ""),
                str(reference.get("key") or ""),
            )
            observed_secret_consumers[key].add(name)

    declared_shared = {
        (str(item.get("namespace")), str(item.get("secret_name")), str(item.get("key"))): set(
            str(value) for value in item.get("consumers") or []
        )
        for item in actual.get("shared_sensitive_credentials") or []
    }
    for binding, consumers in observed_secret_consumers.items():
        if len(consumers) <= 1 or binding[2] in {"*", "mounted_items_or_all"}:
            continue
        if not SENSITIVE_NAME.search(binding[2]):
            continue
        if binding not in declared_shared:
            errors.append(f"shared sensitive credential was hidden: {binding}")
            continue
        if declared_shared[binding] != consumers:
            errors.append(f"shared credential consumer drift: {binding}")
    for binding, consumers in declared_shared.items():
        if observed_secret_consumers.get(binding) != consumers:
            errors.append(f"shared credential declaration has no exact binding match: {binding}")

    principals = actual.get("kafka_principals") or []
    principal_names = [str(item.get("principal") or "") for item in principals]
    if len(principal_names) != len(set(principal_names)):
        errors.append("Kafka principals must be unique")
    if any("*" in principal or principal in {"User:ANONYMOUS", "User:*"} for principal in principal_names):
        errors.append("wildcard or anonymous Kafka principal is forbidden")
    if any((item.get("credential") or {}).get("secret_name") == "traffic-credentials" for item in principals):
        errors.append("Kafka workload principals must not use the shared traffic-credentials Secret")

    tenant_findings = (actual.get("tenant_authority_findings") or {}).get(
        "untrusted_fallback_sites"
    ) or []
    if tenant_findings != _tenant_fallback_sites():
        errors.append("tenant authority fallback inventory is stale or hidden")
    if (actual.get("tenant_authority_findings") or {}).get("status") != (
        "PARTIAL" if tenant_findings else "PASS"
    ):
        errors.append("tenant authority status does not match fallback findings")

    counts = actual.get("counts") or {}
    if counts.get("workloads") != len(workloads):
        errors.append("workload count drift")
    if counts.get("literal_secret_findings") != 0:
        errors.append("literal secret finding count must be zero")
    compliance = (
        "PASS"
        if not counts.get("workloads_with_blocking_gaps")
        and not counts.get("shared_sensitive_credentials")
        and not tenant_findings
        else "PARTIAL"
    )
    return {
        "status": "PASS" if not errors else "FAIL",
        "control_id": "T-SEC-001",
        "catalog_integrity": "PASS" if not errors else "FAIL",
        "security_compliance": compliance,
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
