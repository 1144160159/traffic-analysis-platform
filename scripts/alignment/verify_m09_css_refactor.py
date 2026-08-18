#!/usr/bin/env python3
"""Verify T1-M09-N022 route CSS extraction and Kubernetes visual evidence."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/alignment/alert-detail-css-refactor.v1.json"
EVIDENCE_PATH = ROOT / "doc/02_acceptance/topic1/tasks/t1-m09-n022/k8s-css-refactor-latest.json"
TEXT_PATHS = (
    "web/ui/src/main.tsx",
    "web/ui/src/styles/pages.css",
    "web/ui/src/styles/alert-detail.css",
    "web/ui/deployments/Dockerfile.css-visual-diff",
    "web/ui/deployments/css-visual-diff.mjs",
)


def load_texts() -> dict[str, str]:
    return {path: (ROOT / path).read_text(encoding="utf-8") for path in TEXT_PATHS}


def validate_snapshot(texts: dict[str, str], contract: dict[str, Any], evidence: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    if contract.get("task_id") != "T1-M09-N022" or contract.get("status") != "PARTIAL":
        errors.append("N022 identity/status must remain truthful PARTIAL")
    scope = contract.get("scope", {})
    if scope.get("implemented_slice") != "response_and_feedback_components" or scope.get("route_wide_extraction_complete") is not False:
        errors.append("N022 scope must not overclaim a route-wide extraction")

    main = texts["web/ui/src/main.tsx"]
    if "@/styles/pages.css" not in main or "@/styles/alert-detail.css" not in main:
        errors.append("application entrypoint does not load both old and route stylesheets")
    elif main.index("@/styles/pages.css") > main.index("@/styles/alert-detail.css"):
        errors.append("alert-detail stylesheet must load after pages.css")

    pages = texts["web/ui/src/styles/pages.css"]
    for selector in (".taf-alert-detail-response", ".taf-alert-detail-feedback"):
        if selector in pages:
            errors.append(f"moved selector remains in pages.css: {selector}")

    route = texts["web/ui/src/styles/alert-detail.css"]
    for token in (
        "--alert-detail-action-surface", "--alert-detail-action-border",
        "--alert-detail-action-radius", ".taf-alert-detail-response",
        ".taf-alert-detail-feedback", "@media (max-width: 1440px)",
    ):
        if token not in route:
            errors.append(f"route stylesheet is missing {token}")

    visual_script = texts["web/ui/deployments/css-visual-diff.mjs"]
    for token in (
        "browser.newPage({ viewport, deviceScaleFactor: 1 })", "baseline.image.equals(candidate.image)",
        "computedStylesEqual", "1366", "1600",
    ):
        if token not in visual_script:
            errors.append(f"visual comparison implementation is missing {token}")
    dockerfile = texts["web/ui/deployments/Dockerfile.css-visual-diff"]
    for token in ("USER 1000:1000", "HOME=/tmp", "playwright-core@1.61.1"):
        if token not in dockerfile:
            errors.append(f"visual image hardening/version pin is missing {token}")

    latest = contract.get("latest_evidence", {})
    if evidence.get("task_id") != "T1-M09-N022" or evidence.get("status") != "PASS" or evidence.get("run_id") != latest.get("run_id"):
        errors.append("N022 Kubernetes evidence identity/status mismatch")
    if evidence.get("viewports") != ["1366x900", "1600x900"]:
        errors.append("N022 evidence does not contain the two exact declared viewports")
    for field in (
        "computed_styles_equal", "exact_pixel_diff_zero",
        "old_response_feedback_rules_removed_from_pages_css",
        "route_stylesheet_loaded_after_pages_css", "browser_evidence",
        "run_scoped_resources_removed",
    ):
        if evidence.get(field) is not True:
            errors.append(f"N022 positive evidence flag is not true: {field}")
    for field in ("production_applied", "shared_infrastructure_touched", "mock_enabled"):
        if evidence.get(field) is not False:
            errors.append(f"N022 negative evidence flag is not false: {field}")
    if "every route migrated out of pages.css" not in evidence.get("does_not_prove", []):
        errors.append("N022 evidence omits the route-wide non-claim")

    results = evidence.get("visual_result", {}).get("results", [])
    if len(results) != 2:
        errors.append("N022 visual evidence must contain exactly two viewport results")
    else:
        for result in results:
            baseline = result.get("baseline", {})
            candidate = result.get("candidate", {})
            if result.get("screenshot_equal") is not True or result.get("computed_styles_equal") is not True:
                errors.append("viewport result is not exactly equivalent")
            if baseline.get("screenshot_sha256") != candidate.get("screenshot_sha256"):
                errors.append("viewport screenshot hashes differ")
            if baseline.get("computed_styles") != candidate.get("computed_styles"):
                errors.append("viewport computed styles differ")

    recorded_hashes = evidence.get("inputs", {}).get("source_sha256", {})
    for path, text in texts.items():
        actual = hashlib.sha256(text.encode()).hexdigest()
        if recorded_hashes.get(path) != actual:
            errors.append(f"N022 Kubernetes evidence source hash drifted: {path}")
    return errors


def main() -> int:
    contract = json.loads(CONTRACT_PATH.read_text(encoding="utf-8"))
    evidence = json.loads(EVIDENCE_PATH.read_text(encoding="utf-8"))
    errors = validate_snapshot(load_texts(), contract, evidence)
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M09-N022 route CSS refactor and Kubernetes visual evidence are current")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
