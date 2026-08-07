#!/usr/bin/env python3
"""Verify fail-closed shadow comparison and non-authorizing repair packaging."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/opensearch/projection-shadow-backfill.v1.json")
SHADOW = Path("go/control-plane/internal/alert/projection/shadow.go")
CLI = Path("go/control-plane/cmd/alert-projection-shadow/main.go")
REPAIR_CLI = Path("go/control-plane/cmd/alert-projection-reconcile/main.go")
METADATA = Path("go/control-plane/internal/alert/persistence/opensearch.go")
RENDERER = Path("scripts/alignment/render_alert_projection_shadow_approval.py")
G1_RUNNER = Path("scripts/alignment/verify_alert_projection_shadow_ephemeral.py")
TESTS = (
    Path("go/control-plane/internal/alert/projection/shadow_test.go"),
    Path("go/control-plane/internal/alert/persistence/opensearch_external_version_test.go"),
    Path("go/control-plane/cmd/alert-projection-reconcile/main_test.go"),
    Path("tests/alignment/test_alert_projection_shadow_backfill.py"),
    Path("tests/alignment/test_alert_projection_shadow_ephemeral_guard.py"),
)


def verify(root: Path = ROOT) -> dict[str, Any]:
    required = (CONTRACT, SHADOW, CLI, REPAIR_CLI, METADATA, RENDERER, G1_RUNNER, *TESTS)
    missing = [str(path) for path in required if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}
    errors: list[str] = []
    contract = json.loads((root / CONTRACT).read_text(encoding="utf-8"))
    if contract.get("remediation_ids") != ["T-OS-002", "T-OS-004"]:
        errors.append("contract must bind T-OS-002 and T-OS-004")
    if contract.get("coverage_status") != "PARTIAL" or contract.get("production_applied") is not False or contract.get("production_mutations") != []:
        errors.append("contract must remain PARTIAL and make no production mutation claim")
    shadow_contract = contract.get("shadow", {})
    if shadow_contract.get("forbidden_dependencies") != ["postgresql", "projection_reconcile_run_store"]:
        errors.append("strict shadow dependency boundary drifted")
    if shadow_contract.get("maximum_window_seconds") != 3600 or shadow_contract.get("minimum_window_age_seconds") != 900 or shadow_contract.get("maximum_documents") != 10000:
        errors.append("bounded shadow scope drifted")
    if shadow_contract.get("automatic_delete_extra") is not False or shadow_contract.get("automatic_repair_classes") != ["missing", "stale"]:
        errors.append("repair classification boundary drifted")
    gap = contract.get("historical_gap", {})
    if gap.get("delta") != 883172 or gap.get("reference_manifest_present_in_candidate") is not False:
        errors.append("historical missing-manifest gap must remain explicit")
    if contract.get("approval_package", {}).get("execution_authorized") is not False:
        errors.append("review package must never authorize execution")

    tokens = {
        SHADOW: (
            "READ_ONLY_SHADOW", "MinimumWindowAge", "sourceTruncated || targetTruncated",
            "READY_FOR_BOUNDED_REPAIR_REVIEW", "must never be auto-deleted", "ProductionMutations",
        ),
        METADATA: ("ProjectionMetadata", "IndicesGetAliasRequest", "ClusterUUID", "WriteIndices"),
        CLI: (
            '"environment-id"', '"target-write-alias"', "BuildShadowManifest",
            "ProjectionMetadata", "production_mutations=0",
        ),
        REPAIR_CLI: (
            '"expected-cluster-uuid"', '"expected-read-target"', '"expected-write-alias"',
            '"expected-write-index"', "validateRepairTargetBinding", "ProjectionMetadata",
        ),
        RENDERER: (
            '"execution_authorized": False', '"production_mutations": []',
            '"status": "PENDING"', '"shell": None', "G0 candidate head does not match",
        ),
        G1_RUNNER: (
            '"postgres_dependency_present": False', '"production_mutations": []',
            "TestAlertProjectionShadowRealClickHouseAndOpenSearch", "clickhouse_container_removed",
            "opensearch_container_removed",
        ),
    }
    for path, expected in tokens.items():
        text = (root / path).read_text(encoding="utf-8")
        for token in expected:
            if token not in text:
                errors.append(f"implementation guard missing in {path}: {token}")
    cli_text = (root / CLI).read_text(encoding="utf-8")
    for forbidden in (
        '"database/sql"', "NewProjectionDebtStore", "WriteAlert(",
        "RefreshProjectionTarget(", "_reindex", "_bulk", "UpdateAliases",
    ):
        if forbidden in cli_text:
            errors.append(f"read-only shadow CLI contains forbidden operation: {forbidden}")
    test_text = "\n".join((root / path).read_text(encoding="utf-8") for path in TESTS)
    for token in (
        "ClassifiesAndBindsReadOnlyDiff", "BindingIsDeterministic", "BlocksTruncatedAndAmbiguousAlias",
        "RejectsUnsafeScopesBeforeReads", "ProjectionMetadataBindsClusterAndSingleWriteIndexReadOnly",
        "RepairRequiresExactObservedTargetBinding",
        "TestAlertProjectionShadowRealClickHouseAndOpenSearch", "test_runner_invokes_production_readers_and_checks_unchanged_hashes",
        "test_rejects_expired_tampered_or_mutating_shadow", "test_rejects_candidate_drift_or_dirty_g0",
    ):
        if token not in test_text:
            errors.append(f"negative test guard missing: {token}")
    return {
        "status": "PASS" if not errors else "FAIL",
        "remediation_ids": contract.get("remediation_ids"),
        "coverage_status": contract.get("coverage_status"),
        "production_applied": contract.get("production_applied"),
        "historical_gap": gap,
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
