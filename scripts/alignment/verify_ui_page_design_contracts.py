#!/usr/bin/env python3
"""Verify UI-ADD-001 page design contracts against routes, API plans and evidence."""

from __future__ import annotations

import hashlib
import json
import re
import struct
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
REGISTRY_PATH = ROOT / "web/ui/src/routes/pageDesignContracts.v1.json"
CANONICAL_PATH = ROOT / "contracts/alignment/canonical-registry.json"
CAPTURE_PLAN_PATH = ROOT / "doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.json"
ROUTE_MANIFEST_PATH = ROOT / "web/ui/src/routes/routeManifest.tsx"
API_PLANS_PATH = ROOT / "web/ui/src/services/pageApiPlans.ts"
TOKENS_PATH = ROOT / "web/ui/src/styles/tokens.css"
CANDIDATE_DOCKERFILE_PATH = ROOT / "web/ui/deployments/Dockerfile.candidate-runtime"


def load_json(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as handle:
        value = json.load(handle)
    if not isinstance(value, dict):
        raise ValueError(f"{path.relative_to(ROOT)} must contain a JSON object")
    return value


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def png_size(path: Path) -> tuple[int, int]:
    with path.open("rb") as handle:
        header = handle.read(24)
    if len(header) != 24 or header[:8] != b"\x89PNG\r\n\x1a\n" or header[12:16] != b"IHDR":
        raise ValueError("not a valid PNG header")
    return struct.unpack(">II", header[16:24])


def top_level_api_plan_keys(source: str) -> set[str]:
    start = source.find("export const pageApiPlans")
    if start < 0:
        return set()
    pattern = re.compile(r'^ {2}(?:"([a-z0-9-]+)"|([a-z0-9-]+)):', re.MULTILINE)
    return {quoted or bare for quoted, bare in pattern.findall(source[start:])}


def main() -> int:
    errors: list[str] = []
    registry = load_json(REGISTRY_PATH)
    canonical = load_json(CANONICAL_PATH)
    capture_plan = load_json(CAPTURE_PLAN_PATH)

    pages = registry.get("pages")
    aliases = registry.get("compatibility_aliases")
    if not isinstance(pages, list):
        errors.append("pages must be an array")
        pages = []
    if not isinstance(aliases, list):
        errors.append("compatibility_aliases must be an array")
        aliases = []

    if registry.get("schema_version") != 1 or registry.get("contract_id") != "UI-ADD-001":
        errors.append("registry identity must be schema_version=1 and contract_id=UI-ADD-001")
    viewport = registry.get("baseline_viewport", {})
    if viewport != {
        "width": 1920,
        "height": 1080,
        "device_scale_factor": 1,
        "browser": "Windows Chrome",
        "mock_enabled": False,
    }:
        errors.append("baseline_viewport must be Windows Chrome 1920x1080 DPR1 with mock disabled")

    page_ids = [str(page.get("page_id", "")) for page in pages if isinstance(page, dict)]
    routes = [str(page.get("route", "")) for page in pages if isinstance(page, dict)]
    duplicate_page_ids = sorted({value for value in page_ids if page_ids.count(value) > 1})
    duplicate_routes = sorted({value for value in routes if routes.count(value) > 1})
    if len(pages) != 28:
        errors.append(f"page contract count={len(pages)} want=28")
    if duplicate_page_ids:
        errors.append(f"duplicate page IDs: {duplicate_page_ids}")
    if duplicate_routes:
        errors.append(f"duplicate routes: {duplicate_routes}")

    interactions = capture_plan.get("interactions", [])
    visual_targets = capture_plan.get("visual_targets", [])
    interaction_map = {
        item.get("route_id"): item
        for item in interactions
        if isinstance(item, dict) and isinstance(item.get("route_id"), str)
    }
    visual_map = {
        item.get("target_id"): item
        for item in visual_targets
        if isinstance(item, dict) and isinstance(item.get("target_id"), str)
    }
    contract_map = {page.get("page_id"): page for page in pages if isinstance(page, dict)}
    unknown_route_ids = sorted(set(contract_map) - set(interaction_map))
    orphan_capture_ids = sorted(set(interaction_map) - set(contract_map))
    if unknown_route_ids:
        errors.append(f"unknown contract route IDs: {unknown_route_ids}")
    if orphan_capture_ids:
        errors.append(f"orphan capture route IDs: {orphan_capture_ids}")

    canonical_ids = {
        item.get("id")
        for item in canonical.get("items", [])
        if isinstance(item, dict) and isinstance(item.get("id"), str)
    }
    required_page_fields = {
        "page_id",
        "route",
        "component",
        "auth_mode",
        "baseline_state",
        "target_task",
        "visual_truth",
        "information_architecture",
        "api_plan_key",
        "typescript_contracts",
        "feature_ids",
        "required_scopes",
        "must_preserve",
        "owner",
        "reviewer",
    }
    for page_id, page in contract_map.items():
        missing_fields = sorted(required_page_fields - set(page))
        if missing_fields:
            errors.append(f"{page_id}: missing fields {missing_fields}")
            continue
        interaction = interaction_map.get(page_id)
        if interaction:
            if page.get("route") != interaction.get("route"):
                errors.append(f"{page_id}: route differs from capture plan")
            if page.get("component") != interaction.get("page_component"):
                errors.append(f"{page_id}: component differs from capture plan")
        truth = page.get("visual_truth")
        if not isinstance(truth, list) or not truth:
            errors.append(f"{page_id}: visual_truth must be non-empty")
            truth = []
        expected_source = visual_map.get(page_id, {}).get("source_image")
        if expected_source and (not truth or truth[0] != expected_source):
            errors.append(f"{page_id}: primary visual truth differs from capture plan")
        for relative in truth:
            path = ROOT / str(relative)
            if not path.is_file():
                errors.append(f"{page_id}: missing visual truth {relative}")
                continue
            try:
                width, height = png_size(path)
            except ValueError as exc:
                errors.append(f"{page_id}: {relative}: {exc}")
            else:
                if (width, height) != (1920, 1080):
                    errors.append(f"{page_id}: {relative} is {width}x{height}, want 1920x1080")
        for relative in page.get("typescript_contracts", []):
            if not (ROOT / str(relative)).is_file():
                errors.append(f"{page_id}: missing TypeScript contract {relative}")
        feature_ids = page.get("feature_ids", [])
        unknown_features = sorted(set(feature_ids) - canonical_ids) if isinstance(feature_ids, list) else []
        if unknown_features:
            errors.append(f"{page_id}: unknown canonical IDs {unknown_features}")
        if page.get("reviewer") != "QA-UI-INDEPENDENT":
            errors.append(f"{page_id}: reviewer must remain independent")

    tokens_source = ROOT / str(registry.get("design_system", {}).get("tokens_source", ""))
    if not tokens_source.is_file():
        errors.append("design token source is missing")
    else:
        tokens_text = tokens_source.read_text(encoding="utf-8")
        for token in registry.get("design_system", {}).get("required_tokens", []):
            if f"{token}:" not in tokens_text:
                errors.append(f"missing required design token {token}")

    alias_ids = [alias.get("route_id") for alias in aliases if isinstance(alias, dict)]
    if len(aliases) != 3 or set(alias_ids) != {"topic-tunnel", "topic-exfil", "topic-apt"}:
        errors.append("compatibility aliases must be exactly topic-tunnel/topic-exfil/topic-apt")
    if len(set(alias_ids)) != len(alias_ids):
        errors.append("duplicate compatibility alias IDs")
    route_source = ROUTE_MANIFEST_PATH.read_text(encoding="utf-8")
    for alias in aliases:
        if not isinstance(alias, dict):
            continue
        if f'"{alias.get("route_id")}"' not in route_source or f'"{alias.get("legacy_path")}"' not in route_source:
            errors.append(f"{alias.get('route_id')}: alias is not registered in routeManifest")

    api_plan_keys = top_level_api_plan_keys(API_PLANS_PATH.read_text(encoding="utf-8"))
    owned_plan_keys = [
        page.get("api_plan_key")
        for page in pages
        if isinstance(page, dict) and isinstance(page.get("api_plan_key"), str)
    ] + [
        alias.get("api_plan_key")
        for alias in aliases
        if isinstance(alias, dict) and isinstance(alias.get("api_plan_key"), str)
    ]
    duplicate_api_owners = sorted({key for key in owned_plan_keys if owned_plan_keys.count(key) > 1})
    unknown_api_plans = sorted(set(owned_plan_keys) - api_plan_keys)
    orphan_api_plans = sorted(api_plan_keys - set(owned_plan_keys))
    if duplicate_api_owners:
        errors.append(f"API plans with duplicate owners: {duplicate_api_owners}")
    if unknown_api_plans:
        errors.append(f"unknown owned API plans: {unknown_api_plans}")
    if orphan_api_plans:
        errors.append(f"orphan API plans: {orphan_api_plans}")

    binding = registry.get("candidate_binding", {})
    manifest_path = ROOT / str(binding.get("manifest_path", ""))
    manifest: dict[str, Any] = {}
    if binding.get("required") is not True:
        errors.append("candidate binding must be required")
    if not manifest_path.is_file():
        errors.append(f"candidate manifest is missing: {manifest_path.relative_to(ROOT)}")
    else:
        manifest = load_json(manifest_path)
        expected_hashes = {
            "contract_sha256": sha256(REGISTRY_PATH),
            "route_manifest_sha256": sha256(ROUTE_MANIFEST_PATH),
            "page_api_plans_sha256": sha256(API_PLANS_PATH),
            "tokens_sha256": sha256(TOKENS_PATH),
            "capture_plan_sha256": sha256(CAPTURE_PLAN_PATH),
        }
        for key, expected in expected_hashes.items():
            if manifest.get(key) != expected:
                errors.append(f"candidate manifest {key} does not match current source")
        if (
            manifest.get("schema_version") != 1
            or manifest.get("gate") != "UI-ADD-001_PAGE_DESIGN_CONTRACT"
            or manifest.get("result") != "pass"
        ):
            errors.append("candidate manifest identity/result is invalid")
        if manifest.get("contract_version") != registry.get("contract_version"):
            errors.append("candidate manifest contract_version does not match registry")
        if manifest.get("route_count") != 28 or manifest.get("mock_enabled") is not False:
            errors.append("candidate manifest must bind 28 routes with mock disabled")
        if manifest.get("compatibility_alias_count") != 3:
            errors.append("candidate manifest must bind all three compatibility aliases")
        if manifest.get("full_ui_acceptance") is not False:
            errors.append("candidate manifest must not claim full UI acceptance")
        if manifest.get("contract_embedded_in_bundle") is not False:
            errors.append("candidate manifest must disclose that this contract is not embedded in the candidate bundle")
        if manifest.get("sampled_business_page_coverage") != "6/28":
            errors.append("candidate manifest must bind the current sampled business-page coverage as 6/28")
        not_proven = manifest.get("not_proven")
        if not isinstance(not_proven, list) or not not_proven:
            errors.append("candidate manifest must retain explicit not-proven acceptance work")
        image = manifest.get("image")
        if not isinstance(image, str) or not image or image.endswith(":latest"):
            errors.append("candidate manifest image must use an immutable candidate tag")
        for key in binding.get("required_hashes", []):
            value = manifest.get(key)
            if key in {"image_id", "image_manifest_digest"}:
                if not isinstance(value, str) or re.fullmatch(r"sha256:[0-9a-f]{64}", value) is None:
                    errors.append(f"candidate manifest {key} is not an immutable SHA-256")
            elif not isinstance(value, str) or re.fullmatch(r"[0-9a-f]{64}", value) is None:
                errors.append(f"candidate manifest {key} is not a SHA-256")

        runtime_observation_path = ROOT / str(manifest.get("runtime_observation", ""))
        if not runtime_observation_path.is_file():
            errors.append("candidate runtime observation is missing")
        else:
            if manifest.get("runtime_observation_sha256") != sha256(runtime_observation_path):
                errors.append("candidate runtime observation hash does not match")
            observation = load_json(runtime_observation_path)
            deployment = observation.get("deployment", {})
            pod = observation.get("pod", {})
            if deployment.get("image") != image or pod.get("image") != image:
                errors.append("candidate runtime observation image does not match manifest")
            if deployment.get("image_id") != manifest.get("image_id") or pod.get("image_id") != manifest.get("image_id"):
                errors.append("candidate runtime observation image ID does not match manifest")
            if deployment.get("image_manifest_digest") != manifest.get("image_manifest_digest"):
                errors.append("candidate runtime observation image manifest digest does not match")
            if deployment.get("generation") != deployment.get("observed_generation"):
                errors.append("candidate runtime observation deployment is not fully observed")
            if pod.get("ready") is not True or pod.get("restart_count") != 0:
                errors.append("candidate runtime observation pod is not a clean ready instance")

        browser_evidence_path = ROOT / str(manifest.get("windows_chrome_evidence", ""))
        if not browser_evidence_path.is_file():
            errors.append("candidate Windows Chrome evidence is missing")
        else:
            if manifest.get("windows_chrome_evidence_sha256") != sha256(browser_evidence_path):
                errors.append("candidate Windows Chrome evidence hash does not match")
            browser_evidence = load_json(browser_evidence_path)
            if (
                browser_evidence.get("gate") != "G5_WINDOWS_CHROME_SIX_SAMPLE_READ"
                or browser_evidence.get("result") != "pass"
                or browser_evidence.get("runtime_mock") is not False
            ):
                errors.append("candidate Windows Chrome evidence must pass with runtime mock disabled")
            journeys = browser_evidence.get("journeys")
            expected_journeys = {"assets", "alerts", "topics", "graph", "models", "forensics"}
            journey_ids = {
                item.get("id")
                for item in journeys
                if isinstance(item, dict) and item.get("status") == "pass"
            } if isinstance(journeys, list) else set()
            if not isinstance(journeys, list) or len(journeys) != 6 or journey_ids != expected_journeys:
                errors.append("candidate Windows Chrome evidence must pass assets/alerts/topics/graph/models/forensics")
            if browser_evidence.get("journey_count") != 6:
                errors.append("candidate Windows Chrome evidence journey_count must be 6")
            runtime_errors = browser_evidence.get("runtime_errors")
            if not isinstance(runtime_errors, dict) or any(runtime_errors.get(key) for key in (
                "bad_responses",
                "request_failures",
                "console_errors",
                "page_errors",
            )):
                errors.append("candidate Windows Chrome evidence contains first-party runtime errors")
            if browser_evidence.get("token_material_redacted") is not True:
                errors.append("candidate Windows Chrome evidence must confirm token redaction")
            if isinstance(journeys, list):
                for journey in journeys:
                    if not isinstance(journey, dict):
                        continue
                    screenshot_path = ROOT / str(journey.get("screenshot", ""))
                    if not screenshot_path.is_file():
                        errors.append(f"candidate Windows Chrome screenshot is missing for {journey.get('id')}")
                        continue
                    if journey.get("screenshot_sha256") != sha256(screenshot_path):
                        errors.append(f"candidate Windows Chrome screenshot hash does not match for {journey.get('id')}")
                    try:
                        screenshot_size = png_size(screenshot_path)
                    except ValueError as exc:
                        errors.append(f"candidate Windows Chrome screenshot is invalid for {journey.get('id')}: {exc}")
                    else:
                        if screenshot_size != (1920, 1080):
                            errors.append(f"candidate Windows Chrome screenshot has wrong size for {journey.get('id')}")
                model_journey = next(
                    (item for item in journeys if isinstance(item, dict) and item.get("id") == "models"),
                    {},
                )
                model_layout = model_journey.get("dom", {}).get("model_metrics_layout", {})
                if (
                    not isinstance(model_layout, dict)
                    or model_layout.get("clipped") is not False
                    or model_layout.get("client_height") != model_layout.get("scroll_height")
                    or model_layout.get("card_count") != 7
                ):
                    errors.append("candidate Windows Chrome model metrics layout is clipped or incomplete")
                manifest_model_layout = manifest.get("model_metrics_layout")
                if not isinstance(manifest_model_layout, dict) or any(
                    manifest_model_layout.get(key) != model_layout.get(key)
                    for key in ("client_height", "scroll_height", "clipped", "card_count")
                ):
                    errors.append("candidate manifest model metrics layout does not match Windows Chrome evidence")

        forensics_evidence_path = ROOT / str(manifest.get("forensics_source_reference_evidence", ""))
        if not forensics_evidence_path.is_file():
            errors.append("candidate Forensics source-reference evidence is missing")
        else:
            if manifest.get("forensics_source_reference_evidence_sha256") != sha256(forensics_evidence_path):
                errors.append("candidate Forensics source-reference evidence hash does not match")
            forensics_evidence = load_json(forensics_evidence_path)
            forensics_candidate = forensics_evidence.get("candidate", {})
            if (
                forensics_evidence.get("gate") != "G2_G3_FORENSICS_SOURCE_REFERENCE_WRITE_FILTER_RECONCILE"
                or forensics_evidence.get("result") != "pass"
                or forensics_evidence.get("checks_total") != 9
                or forensics_evidence.get("checks_passed") != 9
                or forensics_evidence.get("checks_failed") != 0
                or forensics_evidence.get("secret_material_redacted") is not True
            ):
                errors.append("candidate Forensics source-reference evidence is not a clean 9/9 pass")
            if (
                not isinstance(forensics_candidate, dict)
                or forensics_candidate.get("ready") is not True
                or forensics_candidate.get("restarts") != 0
            ):
                errors.append("candidate Forensics source-reference runtime is not clean and ready")
            if 'browser_evidence' in locals() and isinstance(browser_evidence.get("journeys"), list):
                forensics_journey = next(
                    (item for item in browser_evidence["journeys"] if isinstance(item, dict) and item.get("id") == "forensics"),
                    {},
                )
                if (
                    forensics_journey.get("expected_request_params") != forensics_evidence.get("source_refs")
                    or forensics_journey.get("expected_text") != forensics_evidence.get("job_id")
                ):
                    errors.append("candidate Windows Chrome Forensics journey is not bound to source-reference evidence")

        static_evidence_path = ROOT / str(manifest.get("static_bundle_reconciliation", ""))
        if not static_evidence_path.is_file():
            errors.append("candidate static-bundle reconciliation evidence is missing")
        else:
            if manifest.get("static_bundle_reconciliation_sha256") != sha256(static_evidence_path):
                errors.append("candidate static-bundle reconciliation evidence hash does not match")
            static_evidence = load_json(static_evidence_path)
            static_candidate = static_evidence.get("candidate", {})
            if (
                static_evidence.get("gate") != "G6_WEB_UI_STATIC_BUNDLE_RECONCILIATION"
                or static_evidence.get("result") != "pass"
                or not isinstance(static_candidate, dict)
                or static_candidate.get("image") != image
                or static_candidate.get("image_id") != manifest.get("image_id")
                or static_candidate.get("image_manifest_digest") != manifest.get("image_manifest_digest")
                or static_candidate.get("source_sha256") != manifest.get("source_sha256")
            ):
                errors.append("candidate static-bundle reconciliation identity/result does not match manifest")
            if (
                static_evidence.get("local_list_sha256") != static_evidence.get("live_list_sha256")
                or static_evidence.get("compared_file_count", 0) <= 0
                or static_evidence.get("runtime_only_files") != ["config.js", "config.js.template"]
            ):
                errors.append("candidate static-bundle reconciliation does not prove an exact non-runtime file set")
            static_checks = static_evidence.get("checks")
            if not isinstance(static_checks, list) or not static_checks or any(
                not isinstance(check, dict) or check.get("pass") is not True for check in static_checks
            ):
                errors.append("candidate static-bundle reconciliation contains a failed or malformed check")
            if static_evidence.get("secret_material_redacted") is not True:
                errors.append("candidate static-bundle reconciliation must confirm secret redaction")

        if manifest.get("candidate_dockerfile_sha256") != sha256(CANDIDATE_DOCKERFILE_PATH):
            errors.append("candidate Dockerfile hash does not match current source")
        source_material = (
            f"{manifest.get('dist_sha256', '')}\n"
            f"{manifest.get('candidate_dockerfile_sha256', '')}\n"
        ).encode("utf-8")
        if hashlib.sha256(source_material).hexdigest() != manifest.get("source_sha256"):
            errors.append("candidate source hash composition does not match dist and Dockerfile hashes")

    result = {
        "result": "pass" if not errors else "fail",
        "contract_version": registry.get("contract_version"),
        "page_count": len(pages),
        "route_coverage": f"{len(set(contract_map) & set(interaction_map))}/28",
        "compatibility_alias_count": len(aliases),
        "canonical_id_count": len(canonical_ids),
        "api_plan_count": len(api_plan_keys),
        "duplicate_page_ids": duplicate_page_ids,
        "duplicate_routes": duplicate_routes,
        "unknown_route_ids": unknown_route_ids,
        "orphan_capture_ids": orphan_capture_ids,
        "unknown_api_plans": unknown_api_plans,
        "orphan_api_plans": orphan_api_plans,
        "candidate_manifest": str(manifest_path.relative_to(ROOT)),
        "candidate_image": manifest.get("image"),
        "errors": errors,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
