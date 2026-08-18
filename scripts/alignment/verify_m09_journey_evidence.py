#!/usr/bin/env python3
"""Validate T1-M09-N023 browser journeys and their cross-store trace evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_RELATIVE = Path("contracts/alignment/m09-journey-evidence.v1.json")
INPUT_RELATIVE = Path(
    "doc/02_acceptance/topic1/tasks/t1-m09-n023/journey-evidence-input.json"
)
KUBERNETES_RELATIVE = Path(
    "doc/02_acceptance/topic1/tasks/t1-m09-n023/k8s-journey-evidence-latest.json"
)
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
ALLOWED_JOURNEY_STATUSES = {"VERIFIED", "BLOCKED_MISSING_WINDOWS_CHROME"}
NULL_WHEN_BLOCKED = (
    "browser",
    "candidate",
    "checks",
    "network",
    "console",
    "cross_storage_trace",
    "dirty",
    "source_hash_match",
)


def load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"JSON document must be an object: {path}")
    return value


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def resolve_repository_path(root: Path, relative: str) -> Path:
    candidate = (root / relative).resolve(strict=False)
    repository = root.resolve()
    if not candidate.is_relative_to(repository):
        raise ValueError(f"repository path escapes root: {relative}")
    return candidate


def validate_source_evidence(root: Path, contract: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    bindings = contract.get("source_evidence")
    if not isinstance(bindings, dict) or not bindings:
        return ["source_evidence must be a non-empty object"]
    for evidence_id, binding in bindings.items():
        if not isinstance(binding, dict):
            errors.append(f"source evidence binding is not an object: {evidence_id}")
            continue
        relative = binding.get("path")
        if not isinstance(relative, str):
            errors.append(f"source evidence path is absent: {evidence_id}")
            continue
        try:
            path = resolve_repository_path(root, relative)
        except ValueError as error:
            errors.append(str(error))
            continue
        if not path.is_file():
            errors.append(f"source evidence file is absent: {relative}")
            continue
        if sha256_file(path) != binding.get("sha256"):
            errors.append(f"source evidence hash drifted: {evidence_id}")
            continue
        try:
            evidence = load_json(path)
        except (OSError, ValueError, json.JSONDecodeError) as error:
            errors.append(f"source evidence is not readable JSON: {evidence_id}: {error}")
            continue
        if evidence.get("task_id") != binding.get("task_id"):
            errors.append(f"source evidence task identity mismatch: {evidence_id}")
        if evidence.get("run_id") != binding.get("run_id"):
            errors.append(f"source evidence run identity mismatch: {evidence_id}")
        if evidence.get("status") != "PASS":
            errors.append(f"source evidence is not PASS: {evidence_id}")
        if evidence.get("production_applied") is not False:
            errors.append(f"source evidence production boundary drifted: {evidence_id}")
    return errors


def _validate_browser(
    journey_id: str, browser: Any, requirement: dict[str, Any], route: str
) -> list[str]:
    if not isinstance(browser, dict):
        return [f"{journey_id}: browser evidence must be an object"]
    errors: list[str] = []
    for field in ("name", "os", "version", "backend", "viewport", "url", "captured_at"):
        if not isinstance(browser.get(field), str) or not browser[field].strip():
            errors.append(f"{journey_id}: browser field is absent: {field}")
    for field in ("name", "os", "backend"):
        if browser.get(field) != requirement.get(field):
            errors.append(f"{journey_id}: browser {field} does not match the designated client")
    if browser.get("viewport") not in requirement.get("allowed_viewports", []):
        errors.append(f"{journey_id}: browser viewport is not approved")
    url = browser.get("url")
    prefix = requirement.get("url_prefix")
    if isinstance(url, str) and isinstance(prefix, str) and not url.startswith(prefix):
        errors.append(f"{journey_id}: browser URL does not use the designated tunnel")
    if browser.get("route_pattern") != route:
        errors.append(f"{journey_id}: browser route pattern mismatch")
    return errors


def _validate_candidate(
    journey_id: str, candidate: Any, expected: dict[str, Any]
) -> list[str]:
    if not isinstance(candidate, dict):
        return [f"{journey_id}: candidate binding must be an object"]
    errors: list[str] = []
    for field in ("app_image", "app_image_id", "config_sha256", "route_config_sha256"):
        if candidate.get(field) != expected.get(field):
            errors.append(f"{journey_id}: candidate binding mismatch: {field}")
    return errors


def _validate_checks(
    journey_id: str, checks: Any, required_kinds: list[str]
) -> list[str]:
    if not isinstance(checks, dict):
        return [f"{journey_id}: journey checks must be an object"]
    errors: list[str] = []
    if set(checks) != set(required_kinds):
        errors.append(f"{journey_id}: journey checks do not cover the exact required kinds")
    for kind in required_kinds:
        check = checks.get(kind)
        if not isinstance(check, dict) or check.get("status") != "PASS":
            errors.append(f"{journey_id}: required check is not PASS: {kind}")
        elif not isinstance(check.get("oracle"), str) or not check["oracle"].strip():
            errors.append(f"{journey_id}: required check has no oracle: {kind}")
    return errors


def _validate_runtime_logs(journey_id: str, journey: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    network = journey.get("network")
    if not isinstance(network, dict):
        errors.append(f"{journey_id}: network evidence must be an object")
    else:
        if not SHA256_RE.fullmatch(str(network.get("artifact_sha256", ""))):
            errors.append(f"{journey_id}: network artifact hash is invalid")
        for field in ("request_failed_count", "http_4xx_count", "http_5xx_count"):
            if network.get(field) != 0:
                errors.append(f"{journey_id}: network evidence is not clean: {field}")
    console = journey.get("console")
    if not isinstance(console, dict):
        errors.append(f"{journey_id}: console evidence must be an object")
    else:
        if not SHA256_RE.fullmatch(str(console.get("artifact_sha256", ""))):
            errors.append(f"{journey_id}: console artifact hash is invalid")
        for field in ("error_count", "page_error_count", "runtime_exception_count"):
            if console.get(field) != 0:
                errors.append(f"{journey_id}: console evidence is not clean: {field}")
    return errors


def _validate_trace(
    journey_id: str,
    trace: Any,
    required_stores: list[str],
    candidate_hash: str,
) -> list[str]:
    if not isinstance(trace, dict):
        return [f"{journey_id}: cross-storage trace must be an object"]
    errors: list[str] = []
    trace_id = trace.get("trace_id")
    if not isinstance(trace_id, str) or not trace_id.strip():
        errors.append(f"{journey_id}: trace_id is absent")
    receipts = trace.get("receipts")
    observed_stores: set[str] = set()
    if not isinstance(receipts, list) or not receipts:
        errors.append(f"{journey_id}: cross-storage receipts are absent")
        receipts = []
    for index, receipt in enumerate(receipts):
        if not isinstance(receipt, dict):
            errors.append(f"{journey_id}: receipt {index} is not an object")
            continue
        store = receipt.get("store")
        if isinstance(store, str):
            observed_stores.add(store)
        for field in ("receipt_id", "observed_at"):
            if not isinstance(receipt.get(field), str) or not receipt[field].strip():
                errors.append(f"{journey_id}: receipt {index} field is absent: {field}")
        if receipt.get("trace_id") != trace_id:
            errors.append(f"{journey_id}: receipt {index} trace identity mismatch")
        if receipt.get("candidate_hash") != candidate_hash:
            errors.append(f"{journey_id}: receipt {index} candidate hash mismatch")
    missing_stores = sorted(set(required_stores) - observed_stores)
    if missing_stores:
        errors.append(f"{journey_id}: required trace stores are absent: {','.join(missing_stores)}")
    final_fact = trace.get("final_fact")
    if not isinstance(final_fact, dict):
        errors.append(f"{journey_id}: final business fact is absent")
    else:
        for field in ("store", "fact_id", "observed_at"):
            if not isinstance(final_fact.get(field), str) or not final_fact[field].strip():
                errors.append(f"{journey_id}: final fact field is absent: {field}")
        if final_fact.get("trace_id") != trace_id:
            errors.append(f"{journey_id}: final fact trace identity mismatch")
        if final_fact.get("candidate_hash") != candidate_hash:
            errors.append(f"{journey_id}: final fact candidate hash mismatch")
        if final_fact.get("store") not in required_stores:
            errors.append(f"{journey_id}: final fact store is outside the required trace")
    return errors


def validate_journey(
    journey: dict[str, Any],
    required: dict[str, Any],
    contract: dict[str, Any],
) -> list[str]:
    journey_id = str(required.get("journey_id"))
    errors: list[str] = []
    for field in ("atomic_pr_id", "journey_id", "route"):
        if journey.get(field) != required.get(field):
            errors.append(f"{journey_id}: journey identity mismatch: {field}")
    source_ids = journey.get("source_evidence_ids")
    known_sources = contract.get("source_evidence", {})
    if not isinstance(source_ids, list) or not source_ids:
        errors.append(f"{journey_id}: source evidence bindings are absent")
    elif any(source_id not in known_sources for source_id in source_ids):
        errors.append(f"{journey_id}: source evidence binding is unknown")
    status = journey.get("status")
    if status not in ALLOWED_JOURNEY_STATUSES:
        errors.append(f"{journey_id}: unsupported journey status")
        return errors
    if status == "BLOCKED_MISSING_WINDOWS_CHROME":
        for field in NULL_WHEN_BLOCKED:
            if journey.get(field) is not None:
                errors.append(f"{journey_id}: blocked journey carries unusable evidence: {field}")
        blockers = journey.get("blockers")
        if not isinstance(blockers, list) or not blockers:
            errors.append(f"{journey_id}: blocked journey has no blocker")
        return errors
    if journey.get("dirty") is not False:
        errors.append(f"{journey_id}: dirty browser capture is unusable")
    if journey.get("source_hash_match") is not True:
        errors.append(f"{journey_id}: source hash mismatch makes the capture unusable")
    if journey.get("blockers") != []:
        errors.append(f"{journey_id}: verified journey must not retain blockers")
    errors.extend(
        _validate_browser(
            journey_id,
            journey.get("browser"),
            contract.get("required_browser", {}),
            str(required.get("route")),
        )
    )
    errors.extend(
        _validate_candidate(
            journey_id, journey.get("candidate"), contract.get("candidate_binding", {})
        )
    )
    errors.extend(
        _validate_checks(
            journey_id,
            journey.get("checks"),
            list(contract.get("required_check_kinds", [])),
        )
    )
    errors.extend(_validate_runtime_logs(journey_id, journey))
    errors.extend(
        _validate_trace(
            journey_id,
            journey.get("cross_storage_trace"),
            list(required.get("required_stores", [])),
            str(contract.get("candidate_binding", {}).get("app_image_id")),
        )
    )
    return errors


def validate_manifest(
    root: Path, contract: dict[str, Any], manifest: dict[str, Any]
) -> tuple[list[str], dict[str, Any]]:
    errors: list[str] = []
    if contract.get("task_id") != "T1-M09-N023" or contract.get("status") != "PARTIAL":
        errors.append("N023 contract identity/status must remain truthful PARTIAL")
    if contract.get("aggregation_mode") != "READ_ONLY":
        errors.append("N023 aggregator must remain READ_ONLY")
    if contract.get("production_applied") is not False:
        errors.append("N023 contract must not claim production application")
    binding = contract.get("candidate_binding", {})
    for path_field, hash_field in (
        ("config_path", "config_sha256"),
        ("route_config_path", "route_config_sha256"),
    ):
        relative = binding.get(path_field)
        if not isinstance(relative, str):
            errors.append(f"candidate binding path is absent: {path_field}")
            continue
        try:
            path = resolve_repository_path(root, relative)
        except ValueError as error:
            errors.append(str(error))
            continue
        if not path.is_file() or sha256_file(path) != binding.get(hash_field):
            errors.append(f"candidate Kubernetes config hash drifted: {relative}")
    errors.extend(validate_source_evidence(root, contract))
    if manifest.get("task_id") != "T1-M09-N023":
        errors.append("journey manifest task identity mismatch")
    if manifest.get("production_applied") is not False:
        errors.append("journey manifest must not claim production application")
    required_journeys = contract.get("required_journeys")
    journeys = manifest.get("journeys")
    if not isinstance(required_journeys, list) or len(required_journeys) != 7:
        errors.append("contract must declare exactly seven N023 journeys")
        required_journeys = []
    if not isinstance(journeys, list) or len(journeys) != len(required_journeys):
        errors.append("journey manifest does not contain the exact seven journeys")
        journeys = []
    verified = 0
    blocked = 0
    blockers: list[str] = []
    if journeys:
        for journey, required in zip(journeys, required_journeys, strict=True):
            if not isinstance(journey, dict) or not isinstance(required, dict):
                errors.append("journey entries must be objects")
                continue
            errors.extend(validate_journey(journey, required, contract))
            if journey.get("status") == "VERIFIED":
                verified += 1
            elif journey.get("status") == "BLOCKED_MISSING_WINDOWS_CHROME":
                blocked += 1
                blockers.extend(str(item) for item in journey.get("blockers", []))
    expected_status = "COMPLETE" if verified == len(required_journeys) else "PARTIAL"
    if manifest.get("candidate_status") != expected_status:
        errors.append("journey manifest candidate_status does not match journey results")
    summary = {
        "status": "PASS" if not errors else "FAIL",
        "task_id": "T1-M09-N023",
        "coverage_status": expected_status,
        "journey_count": len(required_journeys),
        "verified_journey_count": verified,
        "blocked_journey_count": blocked,
        "promotion_eligible": verified == len(required_journeys) and not errors,
        "production_applied": False,
        "blockers": sorted(set(blockers)),
        "errors": errors,
    }
    return errors, summary


def validate_kubernetes_evidence(
    root: Path,
    contract: dict[str, Any],
    summary: dict[str, Any],
    evidence: dict[str, Any],
) -> list[str]:
    errors: list[str] = []
    latest = contract.get("latest_kubernetes_evidence", {})
    if evidence.get("task_id") != "T1-M09-N023" or evidence.get("status") != "PASS":
        errors.append("N023 Kubernetes evidence identity/status mismatch")
    if evidence.get("run_id") != latest.get("run_id") or not latest.get("run_id"):
        errors.append("N023 Kubernetes run identity mismatch")
    if evidence.get("validation") != summary:
        errors.append("N023 Kubernetes validation result does not match current aggregation")
    if evidence.get("run_scoped_resources_removed") is not True:
        errors.append("N023 Kubernetes resources were not proven removed")
    for field in (
        "production_applied",
        "shared_postgres_touched",
        "shared_clickhouse_touched",
        "shared_kafka_touched",
        "shared_minio_touched",
        "shared_nebulagraph_touched",
    ):
        if evidence.get(field) is not False:
            errors.append(f"N023 Kubernetes evidence boundary drifted: {field}")
    job = evidence.get("kubernetes_job")
    if not isinstance(job, dict):
        errors.append("N023 Kubernetes Job identity is absent")
    else:
        for field in ("job_name", "job_uid", "pod_name", "pod_uid", "node", "image", "image_id"):
            if not isinstance(job.get(field), str) or not job[field].strip():
                errors.append(f"N023 Kubernetes Job field is absent: {field}")
    recorded = evidence.get("inputs", {}).get("source_sha256", {})
    expected_paths = (
        CONTRACT_RELATIVE,
        INPUT_RELATIVE,
        Path("scripts/alignment/verify_m09_journey_evidence.py"),
        Path("scripts/alignment/run_m09_journey_evidence_k8s.py"),
        Path("scripts/alignment/Dockerfile.m09-journey-evidence"),
    )
    for relative in expected_paths:
        path = root / relative
        if not path.is_file() or recorded.get(str(relative)) != sha256_file(path):
            errors.append(f"N023 Kubernetes source hash drifted: {relative}")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--input-only", action="store_true")
    parser.add_argument("--json", action="store_true")
    args = parser.parse_args()
    root = args.root.resolve()
    contract = load_json(root / CONTRACT_RELATIVE)
    manifest = load_json(root / INPUT_RELATIVE)
    errors, summary = validate_manifest(root, contract, manifest)
    if not args.input_only:
        evidence_path = root / KUBERNETES_RELATIVE
        if not evidence_path.is_file():
            errors.append("N023 Kubernetes evidence file is absent")
        else:
            errors.extend(
                validate_kubernetes_evidence(
                    root, contract, summary, load_json(evidence_path)
                )
            )
    if errors:
        summary = dict(summary)
        summary["status"] = "FAIL"
        summary["errors"] = errors
    if args.json:
        print(json.dumps(summary, sort_keys=True))
    elif errors:
        for error in errors:
            print(f"FAIL: {error}")
    else:
        print(
            "PASS: T1-M09-N023 journey evidence is truthfully "
            f"{summary['coverage_status']}"
        )
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
