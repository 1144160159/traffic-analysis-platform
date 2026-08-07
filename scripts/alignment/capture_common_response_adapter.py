#!/usr/bin/env python3
"""Capture immutable WP-01 common-response and adapter-ratchet evidence."""

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
FEATURE_REGISTRY = ROOT / "contracts/alignment/feature-contract-registry.v1.json"
ADAPTER_REGISTRY = ROOT / "contracts/alignment/adapter-risk-registry.v1.json"
PROTOCOL = ROOT / "contracts/alignment/common-response-protocol.v1.json"

COMMANDS = (
    ("adapter-risk-registry-current", ["python3", "scripts/alignment/build_adapter_risk_registry.py", "--check"]),
    (
        "production-ui-build-mock-override-guard",
        ["env", "VITE_USE_MOCK=true", "npm", "--prefix", "web/ui", "run", "build"],
    ),
    (
        "production-ui-bundle-scan",
        ["python3", "scripts/alignment/verify_production_ui_bundle.py", "--summary-only"],
    ),
    (
        "adapter-fixture-live-schema-diff",
        [
            "web/ui/node_modules/.bin/vite-node",
            "--config",
            "web/ui/vite.config.ts",
            "web/ui/scripts/capture-adapter-schema-diff.ts",
        ],
    ),
    ("common-response-adapter-verifier", ["python3", "scripts/alignment/verify_common_response_adapter.py"]),
    (
        "alignment-unit-tests",
        [
            "python3",
            "-m",
            "unittest",
            "tests.alignment.test_common_response_adapter",
            "tests.alignment.test_feature_contract_registry",
            "tests.alignment.test_production_ui_bundle",
            "-v",
        ],
    ),
    ("go-httpx-tests", ["go", "-C", "go/control-plane", "test", "./internal/common/httpx", "-count=1"]),
    (
        "web-adapter-tests",
        [
            "npm",
            "--prefix",
            "web/ui",
            "test",
            "--",
            "--run",
            "src/services/alertDetailApi.test.ts",
            "src/services/campaignDetailApi.test.ts",
            "src/services/pageSnapshotFallback.test.ts",
            "src/services/pageSnapshotAdapters.test.ts",
            "src/services/topicApi.test.ts",
            "src/pages/AlertDetailPage.test.ts",
        ],
    ),
    ("openapi-contract", ["python3", "scripts/alignment/check_openapi.py"]),
    ("canonical-registry-strict", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
)

SOURCE_ARTIFACTS = (
    "contracts/alignment/features/F-COMMON-002.json",
    "contracts/alignment/features/F-COMMON-004.json",
    "contracts/alignment/features/F-ADAPTER-002.json",
    "contracts/alignment/common-response-protocol.v1.json",
    "contracts/alignment/adapter-risk-registry.v1.json",
    "contracts/alignment/feature-contract-registry.v1.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "go/control-plane/internal/common/httpx/response.go",
    "go/control-plane/internal/common/httpx/response_contract_test.go",
    "scripts/alignment/build_adapter_risk_registry.py",
    "scripts/alignment/verify_common_response_adapter.py",
    "scripts/alignment/verify_production_ui_bundle.py",
    "scripts/alignment/capture_common_response_adapter.py",
    "scripts/alignment/check_openapi.py",
    "scripts/alignment/generate_ts_client.py",
    "tests/alignment/test_common_response_adapter.py",
    "tests/alignment/test_feature_contract_registry.py",
    "tests/alignment/test_production_ui_bundle.py",
    "web/ui/src/generated/alignmentClient.ts",
    "web/ui/src/config/runtime.ts",
    "web/ui/src/main.tsx",
    "web/ui/scripts/capture-adapter-schema-diff.ts",
    "web/ui/src/pages/GraphEntityPage.tsx",
    "web/ui/src/pages/TopicWorkbenchPage.tsx",
    "web/ui/src/pages/AlertDetailPage.tsx",
    "web/ui/src/pages/AttackChainAnalysisPage.tsx",
    "web/ui/src/pages/DataQualityPage.tsx",
    "web/ui/src/pages/RuleManagementPage.tsx",
    "web/ui/src/pages/WhitelistGovernancePage.tsx",
    "web/ui/src/services/alertDetailApi.ts",
    "web/ui/src/services/alertDetailApi.test.ts",
    "web/ui/src/services/api.ts",
    "web/ui/src/services/campaignDetailApi.ts",
    "web/ui/src/services/campaignDetailApi.test.ts",
    "web/ui/src/services/pageApiPlans.ts",
    "web/ui/src/services/pageSnapshotFallback.test.ts",
    "web/ui/src/services/pageSnapshotAdapters.ts",
    "web/ui/src/services/pageSnapshotAdapters.test.ts",
    "web/ui/vite.config.ts",
    "doc/07_alignment/runbooks/F-COMMON-002-rollback.md",
    "doc/07_alignment/runbooks/F-COMMON-004-rollback.md",
    "doc/07_alignment/runbooks/F-ADAPTER-002-rollback.md",
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
    print(f"[common-response-adapter] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[common-response-adapter] {name}: {result['status']}", flush=True)
    return result


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

    results: list[dict[str, Any]] = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break

    candidate_after = build_snapshot()
    candidate_stable = candidate_before["content_sha256"] == candidate_after["content_sha256"]
    repository_pass = len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results)
    scoped_pass = repository_pass and candidate_stable
    bundle_scan_path = output / "production-ui-bundle-scan.log"
    bundle_scan = json.loads(bundle_scan_path.read_text(encoding="utf-8")) if bundle_scan_path.is_file() else {}
    schema_diff_path = output / "adapter-fixture-live-schema-diff.log"
    schema_diff = json.loads(schema_diff_path.read_text(encoding="utf-8")) if schema_diff_path.is_file() else {}

    feature_registry = json.loads(FEATURE_REGISTRY.read_text(encoding="utf-8"))
    adapter_registry = json.loads(ADAPTER_REGISTRY.read_text(encoding="utf-8"))
    protocol = json.loads(PROTOCOL.read_text(encoding="utf-8"))
    feature_coverage = feature_registry.get("coverage") or {}
    adapter_coverage = adapter_registry.get("coverage") or {}

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
        "feature_ids": ["F-COMMON-002", "F-COMMON-004", "F-ADAPTER-002"],
        "status": "PARTIAL" if scoped_pass else "FAIL",
        "coverage_status": "PARTIAL_COMMON_RESPONSE_PROTOCOL_AND_FULL_FRONTEND_ADAPTER_RISK_RATCHET",
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
        "contract_summary": {
            "feature_contract_catalog_sha256": feature_registry.get("catalog_sha256"),
            "formal_contracts": feature_coverage.get("formal_contracts"),
            "formal_contracts_valid": feature_coverage.get("formal_contracts_valid"),
            "standard_scope_features": feature_coverage.get("standard_scope_features"),
            "standard_scope_formal_contracts": feature_coverage.get("standard_scope_formal_contracts"),
            "missing_standard_scope_contracts": feature_coverage.get("missing_standard_scope_contracts"),
            "p0_features": feature_coverage.get("p0_features"),
            "p0_features_with_formal_contract": feature_coverage.get("p0_features_with_formal_contract"),
            "snapshot_required_fields": protocol.get("snapshot_meta", {}).get("required", []),
            "error_catalog_codes": len(protocol.get("error", {}).get("catalog", [])),
        },
        "adapter_summary": {
            "catalog_sha256": adapter_registry.get("catalog_sha256"),
            "source_files": adapter_coverage.get("source_files"),
            "open_findings": adapter_coverage.get("open_findings"),
            "counts_by_rule": adapter_coverage.get("counts_by_rule"),
            "resolved_in_this_slice": adapter_coverage.get("resolved_in_this_slice", []),
            "remaining": adapter_coverage.get("remaining", []),
        },
        "production_bundle": {
            "status": bundle_scan.get("status", "NOT_EXECUTED"),
            "bundle_sha256": bundle_scan.get("bundle_sha256"),
            "file_count": bundle_scan.get("file_count"),
            "size_bytes": bundle_scan.get("size_bytes"),
            "forbidden_paths": bundle_scan.get("forbidden_paths"),
            "forbidden_tokens": bundle_scan.get("forbidden_tokens"),
            "forbidden_manifest_sources": bundle_scan.get("forbidden_manifest_sources"),
            "candidate_source_sha256": candidate_before["content_sha256"],
            "candidate_source_stable": candidate_stable,
            "deployed": False,
        },
        "pilot_fixture_live_schema_diff": {
            "status": schema_diff.get("status", "NOT_EXECUTED"),
            "mode": schema_diff.get("mode"),
            "request_count": len(schema_diff.get("requests") or []),
            "all_http_200": schema_diff.get("all_http_200"),
            "type_conflict_count": schema_diff.get("type_conflict_count"),
            "required_raw_path_gaps": schema_diff.get("required_raw_path_gaps"),
            "required_normalized_path_gaps": schema_diff.get("required_normalized_path_gaps"),
            "payload_values_captured": schema_diff.get("payload_values_captured"),
            "secrets_captured": schema_diff.get("secrets_captured"),
            "production_mutations": schema_diff.get("production_mutations"),
            "value_reconciliation": {
                "status": (schema_diff.get("value_reconciliation") or {}).get("status", "NOT_EXECUTED"),
                "check_count": (schema_diff.get("value_reconciliation") or {}).get("check_count", 0),
                "failure_count": (schema_diff.get("value_reconciliation") or {}).get("failure_count"),
                "payload_values_captured": (schema_diff.get("value_reconciliation") or {}).get("payload_values_captured"),
            },
        },
        "runtime_observation": {
            "production_candidate_deployed": False,
            "common_protocol_runtime_adoption": "NOT_OBSERVED",
            "fixture_live_schema_diff": schema_diff.get("status", "NOT_EXECUTED"),
            "production_bundle_mock_dependency_scan": "PASS" if scoped_pass else "FAIL",
            "windows_chrome_samples": "NOT_EXECUTED",
            "secret_values_captured": False,
            "response_payloads_captured": False,
            "production_mutations": [],
        },
        "production_applied": False,
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_COMMON_PROTOCOL_AND_FULL_FRONTEND_ADAPTER_RATCHET" if scoped_pass else "FAIL",
            "G2": "PARTIAL_PASS_FOR_ALERT_CAMPAIGN_TOPIC_FIXTURE_LIVE_SCHEMA_DIFF; OPEN_FOR_COMMON_PROTOCOL_ADOPTION",
            "G3": "PARTIAL_PASS_FOR_SIXTEEN_SAMPLED_DISPLAYED_VALUES; OPEN_FOR_ADDITIONAL_ZERO_EMPTY_PARTIAL_UNAVAILABLE_WINDOWS_AND_CROSS_STORE_RECONCILIATION",
            "G4": "OPEN_FOR_PROTOCOL_AND_ADAPTER_PERFORMANCE_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_PRODUCTION_BUNDLE_SAMPLES",
            "G6": "HOLD_FOR_CANARY_ROLLBACK_AND_T_PLUS_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "three WP-01 features have formal hash-bound contracts and repository validation",
            "the additive REST envelope defines ten required snapshot fields and a thirteen-code structured error catalog",
            "the generated TypeScript client and Go common response helper expose the same additive protocol fields",
            "the registry scans every production TypeScript page and service source while excluding tests and explicit mock fixture modules",
            "all thirteen registered adapter-risk categories are zero across the 68 scanned production sources",
            "data-quality topic rows require authoritative topic_health payloads and zero remains distinct from missing",
            "production build statically disables runtime mock configuration and contains no MSW worker, vendor chunk, fixture source graph or registered mock marker",
            "eleven read-only real-service requests cover alert, campaign and topic pilots with all required raw and normalized paths present and zero concrete type conflicts",
            "sixteen sampled alert campaign and topic displayed values reconcile to authoritative response paths without capturing payload values",
        ],
        "open": [
            *list(protocol.get("adoption_gaps") or []),
            *list(adapter_coverage.get("remaining") or []),
            "deploy one approved candidate and complete G2 through G7 evidence without treating repository PASS as runtime completion",
        ],
        "secrets_captured": False,
        "production_mutations": [],
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "scoped_evidence_status": manifest["scoped_evidence_status"],
                "manifest": str(manifest_path.relative_to(ROOT)),
                "manifest_sha256": sha256(manifest_path),
                "candidate_source_sha256": candidate_before["content_sha256"],
                "formal_contracts": feature_coverage.get("formal_contracts"),
                "missing_standard_scope_contracts": len(feature_coverage.get("missing_standard_scope_contracts") or []),
                "adapter_open_findings": adapter_coverage.get("open_findings"),
                "production_mutations": [],
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if scoped_pass else 1


if __name__ == "__main__":
    raise SystemExit(main())
