#!/usr/bin/env python3
"""Verify the repository's Product Design audit and finding-closure policy."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
POLICY = Path("contracts/ui/product-design-audit-policy.v1.json")
PROCESS = Path("doc/07_alignment/前端开发流程补充整改清单-2026-08-04.md")
AGENT = Path("agent.md")


def load_json(root: Path, relative: Path) -> dict[str, Any]:
    value = json.loads((root / relative).read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"{relative} must contain a JSON object")
    return value


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    missing = [str(path) for path in (POLICY, PROCESS, AGENT) if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    policy = load_json(root, POLICY)
    if policy.get("schema_version") != 1 or policy.get("policy_id") != "UI-PRODUCT-DESIGN-AUDIT-V1":
        errors.append("Product Design audit policy identity drifted")
    if policy.get("audit_method") != "Product Design" or policy.get("figma_enabled") is not False:
        errors.append("frontend audit must use Product Design with Figma disabled")

    evidence = policy.get("evidence", {})
    for guard in (
        "current_run_screenshots_only",
        "saved_screenshot_must_be_inspected",
        "findings_must_reference_step_or_screenshot",
        "production_candidate_bundle_required_for_closure",
        "mock_must_be_disabled_for_closure",
    ):
        if evidence.get(guard) is not True:
            errors.append(f"audit evidence guard must remain true: {guard}")

    lifecycle = policy.get("finding_lifecycle", {})
    if lifecycle.get("statuses") != ["OPEN", "IN_PROGRESS", "VERIFYING", "CLOSED"]:
        errors.append("finding lifecycle statuses drifted")
    for guard in (
        "closed_is_terminal",
        "close_immediately_when_all_close_conditions_pass",
        "closed_findings_excluded_from_future_open_lists",
        "immutable_closure_evidence_retained",
        "regression_requires_new_occurrence_id",
    ):
        if lifecycle.get(guard) is not True:
            errors.append(f"finding closure guard must remain true: {guard}")
    if lifecycle.get("default_progress_report_statuses") != ["OPEN", "IN_PROGRESS", "VERIFYING"]:
        errors.append("default progress reports must exclude CLOSED findings")
    if len(policy.get("close_conditions", [])) < 6:
        errors.append("issue closure conditions are incomplete")
    if policy.get("audit_output", {}).get("show_closed_findings") is not False:
        errors.append("future audit output must not repeat CLOSED findings")

    process = (root / PROCESS).read_text(encoding="utf-8")
    for token in ("Product Design 页面审计", "本轮实际捕获", "解决即关闭", "CLOSED", "不再进入后续开放问题清单"):
        if token not in process:
            errors.append(f"frontend process policy missing: {token}")
    if "### 3.5 Figma 整理板状态" in process:
        errors.append("active frontend remediation process still routes audits through Figma")

    agent = (root / AGENT).read_text(encoding="utf-8")
    for token in ("前端页面审计统一使用 Product Design", "本轮实际截图", "解决即关闭", "CLOSED 项不再进入后续开放清单"):
        if token not in agent:
            errors.append(f"agent audit/closure rule missing: {token}")

    return {
        "status": "PASS" if not errors else "FAIL",
        "policy_id": policy.get("policy_id"),
        "audit_method": policy.get("audit_method"),
        "figma_enabled": policy.get("figma_enabled"),
        "default_progress_report_statuses": lifecycle.get("default_progress_report_statuses"),
        "closed_findings_excluded": lifecycle.get("closed_findings_excluded_from_future_open_lists"),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
