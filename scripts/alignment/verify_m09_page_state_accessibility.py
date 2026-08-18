#!/usr/bin/env python3
"""Static and Kubernetes-evidence verifier for T1-M09-N021."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/alignment/page-state-accessibility.v1.json")
EVIDENCE = Path("doc/02_acceptance/topic1/tasks/t1-m09-n021/k8s-page-state-accessibility-latest.json")
TEXT_PATHS = (
    "web/ui/src/components/PageStateBoundary.tsx",
    "web/ui/src/components/pageState.ts",
    "web/ui/src/components/useDrawerFocusReturn.ts",
    "web/ui/src/pages/AlertDetailPage.tsx",
    "web/ui/src/pages/CampaignWorkbenchPage.tsx",
    "web/ui/src/styles/page-state.css",
    "web/ui/src/main.tsx",
)


def load_json(path: Path) -> dict[str, Any]:
    return json.loads((ROOT / path).read_text(encoding="utf-8"))


def load_texts() -> dict[str, str]:
    return {relative: (ROOT / relative).read_text(encoding="utf-8") for relative in TEXT_PATHS}


def validate_snapshot(texts: dict[str, str], contract: dict[str, Any], evidence: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    if contract.get("task_id") != "T1-M09-N021" or contract.get("status") != "PARTIAL":
        errors.append("N021 contract identity/status must remain PARTIAL")
    if contract.get("state_machine", {}).get("states") != ["loading", "empty", "partial", "unavailable", "conflict", "final"]:
        errors.append("page state machine does not preserve the six ordered states")

    resolver = texts["web/ui/src/components/pageState.ts"]
    for token in (
        "isLoading && data === undefined", "responseStatus(error) === 409", "data === undefined || data === null",
        "partial || error", "return 'final'",
    ):
        if token not in resolver:
            errors.append(f"page-state resolver missing {token}")

    component = texts["web/ui/src/components/PageStateBoundary.tsx"]
    for token in (
        "data-page-state", "aria-live", "aria-busy", "role === 'alert'", "state === 'partial' ? children : null",
        "刷新权威状态",
    ):
        if token not in component:
            errors.append(f"page-state accessible component missing {token}")

    focus = texts["web/ui/src/components/useDrawerFocusReturn.ts"]
    for token in (
        "captureReturnFocus", "document.activeElement", "requestAnimationFrame", "initialFocusSelector",
        "target?.isConnected", "target.focus({ preventScroll: true })",
    ):
        if token not in focus:
            errors.append(f"Drawer focus lifecycle missing {token}")

    alert_page = texts["web/ui/src/pages/AlertDetailPage.tsx"]
    for token in ("detailPageState", "detailMissingSections", "PageStateBoundary", 'labelledBy="alert-detail-page-title"'):
        if token not in alert_page:
            errors.append(f"alert-detail state adoption missing {token}")
    campaign = texts["web/ui/src/pages/CampaignWorkbenchPage.tsx"]
    for token in (
        "detailDrawerFocus.captureReturnFocus", "afterOpenChange={detailDrawerFocus.afterOpenChange}",
        "data-campaign-detail-initial-focus", "snapshot.missingSections", "PageStateBoundary",
    ):
        if token not in campaign:
            errors.append(f"campaign Drawer state/focus adoption missing {token}")

    styles = texts["web/ui/src/styles/page-state.css"]
    for token in ("overflow-wrap: anywhere", "min-width: 0", "@media (max-width: 1366px)", "@media (min-width: 1600px)"):
        if token not in styles:
            errors.append(f"responsive/long-text contract missing {token}")
    if "@/styles/page-state.css" not in texts["web/ui/src/main.tsx"]:
        errors.append("page-state stylesheet is not loaded by the application entrypoint")

    latest = contract.get("latest_evidence", {})
    if evidence.get("task_id") != "T1-M09-N021" or evidence.get("status") != "PASS" or evidence.get("run_id") != latest.get("run_id"):
        errors.append("N021 Kubernetes evidence identity/status mismatch")
    for field in (
        "six_page_states_present", "unavailable_does_not_render_fallback_children",
        "partial_preserves_available_content", "drawer_initial_focus_contract",
        "drawer_return_focus_contract", "long_text_wrap_contract",
        "viewport_1366_contract", "viewport_1600_contract", "run_scoped_resources_removed",
    ):
        if evidence.get(field) is not True:
            errors.append(f"N021 Kubernetes evidence missing {field}=true")
    for field in ("mock_enabled", "shared_infrastructure_touched", "browser_evidence", "production_applied"):
        if evidence.get(field) is not False:
            errors.append(f"N021 Kubernetes evidence must keep {field}=false")
    if len(evidence.get("kubernetes_jobs", [])) != 2:
        errors.append("N021 Kubernetes evidence does not contain two successful jobs")
    if not any("every product page migrated" in item for item in evidence.get("does_not_prove", [])):
        errors.append("N021 evidence overclaims route-wide adoption")
    if not any("Windows Chrome" in item for item in evidence.get("does_not_prove", [])):
        errors.append("N021 evidence overclaims browser accessibility")
    return errors


def main() -> int:
    contract, evidence, texts = load_json(CONTRACT), load_json(EVIDENCE), load_texts()
    errors = validate_snapshot(texts, contract, evidence)
    for relative, expected in evidence.get("inputs", {}).get("source_sha256", {}).items():
        path = ROOT / relative
        actual = hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else "missing"
        if actual != expected:
            errors.append(f"Kubernetes evidence source hash drifted: {relative}")
    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M09-N021 page-state/accessibility contract and Kubernetes evidence are current")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
