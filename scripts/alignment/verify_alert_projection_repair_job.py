#!/usr/bin/env python3
"""Verify immutable tool-image and suspended repair-candidate guardrails."""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/opensearch/projection-repair-execution.v1.json")
DOCKERFILE = Path("go/control-plane/deployments/docker/Dockerfile.alert-projection-tools")
TEMPLATE = Path("deployments/kubernetes/migrations/opensearch/T-OS-004-alert-projection-repair.template.yaml")
RENDERER = Path("scripts/alignment/render_alert_projection_repair_job.py")
SUPPLY_CHAIN = Path("scripts/alignment/capture_alert_projection_tool_supply_chain.py")
APPROVAL = Path("go/control-plane/cmd/alert-projection-reconcile/approval.go")
MAIN = Path("go/control-plane/cmd/alert-projection-reconcile/main.go")
TESTS = (
    Path("go/control-plane/cmd/alert-projection-reconcile/approval_test.go"),
    Path("tests/alignment/test_alert_projection_repair_job.py"),
    Path("tests/alignment/test_alert_projection_tool_supply_chain.py"),
)


def verify(root: Path = ROOT) -> dict[str, Any]:
    required = (CONTRACT, DOCKERFILE, TEMPLATE, RENDERER, SUPPLY_CHAIN, APPROVAL, MAIN, *TESTS)
    missing = [str(path) for path in required if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}
    errors: list[str] = []
    contract = json.loads((root / CONTRACT).read_text(encoding="utf-8"))
    if contract.get("remediation_ids") != ["T-OS-002", "T-OS-004"]:
        errors.append("repair execution contract identity drifted")
    if contract.get("coverage_status") != "PARTIAL" or contract.get("production_applied") is not False or contract.get("production_mutations") != []:
        errors.append("repair execution contract must remain PARTIAL and non-production")
    candidate = contract.get("repair_candidate", {})
    for key in (
        "default_suspended", "automatic_retries", "automatic_apply",
        "review_package_must_remain_non_authorizing", "separate_four_party_approval_bundle_required",
        "requester_self_approval_forbidden", "approval_identities_must_be_distinct",
        "credentials_from_secret_refs_only", "exact_repository_image_digest_required",
    ):
        expected = key not in {"automatic_retries", "automatic_apply"}
        if candidate.get(key) is not expected:
            errors.append(f"repair execution guard drifted: {key}")
    if candidate.get("required_approval_roles") != ["sre", "qa", "security", "domain_accountable"]:
        errors.append("four-party approval roles drifted")
    tool_image = contract.get("tool_image", {})
    expected_supply_chain = {
        "supply_chain_capture": "scripts/alignment/capture_alert_projection_tool_supply_chain.py",
        "sbom_format": "CycloneDX 1.5",
        "sbom_required_for_execution": True,
        "binary_vulnerability_scanner": "govulncheck",
        "scanner_tool_version_and_database_timestamp_required": True,
        "maximum_vulnerability_database_age_seconds": 86400,
        "reachable_known_vulnerabilities_allowed": 0,
        "nonreachable_findings_require_security_adjudication": True,
        "registry_signature_verification_required": True,
        "slsa_provenance_verification_required": True,
        "source_g0_image_sbom_scan_signature_and_provenance_must_match": True,
        "prepublish_local_capture_is_non_authorizing": True,
    }
    for key, value in expected_supply_chain.items():
        if tool_image.get(key) != value:
            errors.append(f"tool image supply-chain guard drifted: {key}")

    dockerfile = (root / DOCKERFILE).read_text(encoding="utf-8")
    if len(re.findall(r"^FROM .*@sha256:[0-9a-f]{64}", dockerfile, re.MULTILINE)) < 2:
        errors.append("tool image build stages are not digest pinned")
    for token in (
        "FROM scratch", "ARG SOURCE_REVISION", "ARG SOURCE_CONTENT_SHA256", "CGO_ENABLED=0",
        "/usr/local/bin/alert-projection-shadow", "/usr/local/bin/alert-projection-reconcile", "USER 65532:65532",
    ):
        if token not in dockerfile:
            errors.append(f"tool image guard missing: {token}")

    template = next(yaml.safe_load_all((root / TEMPLATE).read_text(encoding="utf-8")))
    spec = template.get("spec", {})
    if template.get("kind") != "Job" or spec.get("suspend") is not True or spec.get("backoffLimit") != 0:
        errors.append("checked-in repair template must be a suspended zero-retry Job")
    renderer = (root / RENDERER).read_text(encoding="utf-8")
    supply_chain = (root / SUPPLY_CHAIN).read_text(encoding="utf-8")
    approval = (root / APPROVAL).read_text(encoding="utf-8")
    main = (root / MAIN).read_text(encoding="utf-8")
    for token in (
        '"suspend": True', '"backoffLimit": 0', '"automountServiceAccountToken": False',
        '"readOnlyRootFilesystem": True', '"execution_authorized") is not True',
        "approval identities must be distinct", "APPROVED_OPERATOR_REQUIRED", "production_mutations",
    ):
        if token not in renderer:
            errors.append(f"repair renderer guard missing: {token}")
    for token in (
        "validateRepairApproval", "repair requires immutable review and approval bundle files",
        "repair requester cannot self-approve", "repair approval identities must be distinct",
        "repair review package must remain non-authorizing and non-mutating",
        "repair review shadow is expired or captured in the future",
        "repair actual argv does not match the approved review argv",
    ):
        if token not in approval + main:
            errors.append(f"repair runtime guard missing: {token}")
    for token in (
        '"bomFormat": "CycloneDX"', '"specVersion": "1.5"',
        "govulncheck SARIF output is invalid", "text scan reports reachable vulnerabilities",
        "govulncheck database is stale or dated in the future",
        "local image has no approval-eligible repository manifest digest",
        "registry signature verification is absent", "SLSA provenance verification is absent",
        "cosign", "verify-attestation", '"approval_eligible": approval_eligible',
        '"production_applied": False', '"production_mutations": []',
    ):
        if token not in supply_chain:
            errors.append(f"projection tool supply-chain guard missing: {token}")
    return {
        "status": "PASS" if not errors else "FAIL",
        "remediation_ids": contract.get("remediation_ids"),
        "coverage_status": contract.get("coverage_status"),
        "production_applied": contract.get("production_applied"),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
