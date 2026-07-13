#!/usr/bin/env python3
"""Finalize UI visual and interaction evidence after Desktop Chrome capture.

This helper is intentionally strict. It can generate diff images and metrics
from already-captured screenshots, but it never creates or substitutes missing
Desktop Chrome evidence.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable
from urllib.parse import urlparse

from PIL import Image


DEFAULT_PLAN = Path("doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.json")
DEFAULT_OUTPUT_JSON = Path("doc/02_acceptance/02-regression/ui-visual-interaction/evidence-finalization-latest.json")
DEFAULT_OUTPUT_MD = Path("doc/02_acceptance/02-regression/ui-visual-interaction/evidence-finalization-latest.md")
DEFAULT_EVIDENCE_DIR = Path("doc/02_acceptance/02-regression/ui-visual-interaction/latest")
EXPECTED_BACKENDS = {"codex-desktop-chrome-extension", "extension"}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Finalize UI visual diff and interaction evidence.")
    parser.add_argument("--capture-plan", type=Path, default=DEFAULT_PLAN)
    parser.add_argument("--evidence-dir", type=Path, default=None)
    parser.add_argument("--output-json", type=Path, default=DEFAULT_OUTPUT_JSON)
    parser.add_argument("--output-md", type=Path, default=DEFAULT_OUTPUT_MD)
    parser.add_argument("--run-id", default=os.environ.get("RUN_ID", ""))
    parser.add_argument("--max-pixel-ratio", type=float, default=0.015)
    parser.add_argument("--channel-tolerance", type=int, default=0)
    parser.add_argument("--expected-width", type=int, default=1920)
    parser.add_argument("--expected-height", type=int, default=1080)
    parser.add_argument(
        "--allow-blockers",
        action="store_true",
        default=os.environ.get("ALLOW_BLOCKERS", "").lower() in {"1", "true", "yes", "on"},
    )
    return parser.parse_args()


def now_iso() -> str:
    return datetime.now(timezone.utc).astimezone().isoformat()


def repo_rel(path: Path) -> str:
    try:
        return path.resolve().relative_to(Path.cwd().resolve()).as_posix()
    except ValueError:
        return path.as_posix()


def read_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def read_json_state(path: Path) -> tuple[bool, Any | None, str]:
    if not path.is_file():
        return False, None, "missing"
    try:
        return True, read_json(path), ""
    except Exception as exc:  # noqa: BLE001
        return True, None, str(exc)


def write_json(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def image_size(path: Path) -> tuple[int, int] | None:
    try:
        with Image.open(path) as image:
            return image.size
    except Exception:  # noqa: BLE001
        return None


def any_channel_over(delta: Iterable[int], tolerance: int) -> bool:
    return any(channel > tolerance for channel in delta)


def pixel_data(image: Image.Image):
    if hasattr(image, "get_flattened_data"):
        return image.get_flattened_data()
    return image.getdata()


def size_from_mapping(value: Any) -> tuple[int | None, int | None]:
    if not isinstance(value, dict):
        return None, None
    width = value.get("width")
    height = value.get("height")
    try:
        width_int = int(width)
        height_int = int(height)
    except (TypeError, ValueError):
        return None, None
    return width_int, height_int


def compare_images(
    *,
    target_id: str,
    route: str,
    source_path: Path,
    actual_path: Path,
    diff_path: Path,
    metrics_path: Path,
    max_pixel_ratio: float,
    channel_tolerance: int,
    desktop_status: str,
    finalizer_run_id: str,
) -> tuple[dict[str, Any], list[str]]:
    reasons: list[str] = []
    source = Image.open(source_path).convert("RGBA")
    actual = Image.open(actual_path).convert("RGBA")
    size_ok = source.size == actual.size

    width = min(source.width, actual.width)
    height = min(source.height, actual.height)
    source_crop = source.crop((0, 0, width, height))
    actual_crop = actual.crop((0, 0, width, height))

    mismatch_pixels = 0
    max_channel_delta = 0
    heatmap_pixels = []
    for source_pixel, actual_pixel in zip(pixel_data(source_crop), pixel_data(actual_crop)):
        delta = tuple(abs(int(a) - int(b)) for a, b in zip(source_pixel, actual_pixel))
        max_channel_delta = max(max_channel_delta, max(delta))
        if any_channel_over(delta, channel_tolerance):
            mismatch_pixels += 1
            heatmap_pixels.append((255, 48, 48, 170))
        else:
            heatmap_pixels.append((0, 0, 0, 0))

    compared_pixels = width * height
    if not size_ok:
        missing_pixels = source.width * source.height + actual.width * actual.height - 2 * compared_pixels
        mismatch_pixels += max(0, missing_pixels)
        reasons.append(f"source/actual size mismatch {source.width}x{source.height} vs {actual.width}x{actual.height}")
    total_pixels = max(source.width * source.height, actual.width * actual.height)
    mismatch_ratio = mismatch_pixels / total_pixels if total_pixels else 1.0
    if mismatch_ratio > max_pixel_ratio:
        reasons.append(f"pixel mismatch ratio {mismatch_ratio} > {max_pixel_ratio}")

    base = actual_crop.copy()
    heatmap = Image.new("RGBA", (width, height))
    heatmap.putdata(heatmap_pixels)
    diff = Image.alpha_composite(base, heatmap)
    diff_path.parent.mkdir(parents=True, exist_ok=True)
    diff.save(diff_path)

    passed = size_ok and mismatch_ratio <= max_pixel_ratio
    metrics = {
        "target_id": target_id,
        "route": route,
        "status": "pass" if passed else "fail",
        "generated_at": now_iso(),
        "generated_by": "ui_visual_interaction_evidence_finalize.py",
        "finalizer_run_id": finalizer_run_id,
        "desktop_chrome_backend_status": desktop_status,
        "source_image": repo_rel(source_path),
        "actual_screenshot": repo_rel(actual_path),
        "diff_image": repo_rel(diff_path),
        "viewport": {"width": actual.width, "height": actual.height},
        "visual_diff": {
            "size_ok": size_ok,
            "source_width": source.width,
            "source_height": source.height,
            "actual_width": actual.width,
            "actual_height": actual.height,
            "compared_pixels": compared_pixels,
            "total_pixels": total_pixels,
            "mismatch_pixels": mismatch_pixels,
            "pixel_mismatch_ratio": mismatch_ratio,
            "max_pixel_ratio": max_pixel_ratio,
            "channel_tolerance": channel_tolerance,
            "max_channel_delta": max_channel_delta,
        },
    }
    write_json(metrics_path, metrics)
    return metrics, reasons


def evidence_file(plan_entry: dict[str, Any], evidence_dir: Path, target_id: str, key: str, filename: str) -> Path:
    evidence = plan_entry.get("evidence") if isinstance(plan_entry.get("evidence"), dict) else {}
    value = evidence.get(key)
    if value:
        return Path(value)
    return evidence_dir / target_id / filename


def validate_capture_meta(meta: dict[str, Any], expected_width: int, expected_height: int) -> list[str]:
    reasons: list[str] = []
    if meta.get("status") != "pass":
        reasons.append(f"capture-meta status={meta.get('status')}")
    if meta.get("backend") not in EXPECTED_BACKENDS:
        reasons.append(f"capture-meta backend={meta.get('backend')}")
    for label, key in (
        ("uploaded screenshot", "uploaded_size"),
        ("stored screenshot", "stored_size"),
        ("expected size", "expected_size"),
        ("Desktop Chrome viewport", "desktop_viewport"),
    ):
        width, height = size_from_mapping(meta.get(key))
        if width != expected_width or height != expected_height:
            reasons.append(f"{label} {width}x{height} != {expected_width}x{expected_height}")
    if meta.get("post_capture_resize") is not False:
        reasons.append("post_capture_resize is not false")
    return reasons


def validate_interaction(data: dict[str, Any], route: dict[str, Any]) -> list[str]:
    reasons: list[str] = []
    if data.get("status") != "pass":
        reasons.append(f"status={data.get('status')}")
    if data.get("desktop_chrome_backend_status") != "pass":
        reasons.append(f"desktop_chrome_backend_status={data.get('desktop_chrome_backend_status')}")
    if data.get("desktop_backend") not in EXPECTED_BACKENDS:
        reasons.append(f"desktop_backend={data.get('desktop_backend')}")
    for key in ("no_4xx_5xx", "no_requestfailed", "no_pageerror", "no_console_error"):
        if data.get(key) is not True:
            reasons.append(f"{key} is not true")
    errors = data.get("errors")
    if isinstance(errors, dict):
        for key in ("console_errors", "page_errors", "request_failures", "response_errors"):
            if errors.get(key):
                reasons.append(f"{key} not empty")
    final_url = str(data.get("final_url") or "")
    final_path = data.get("final_path") or urlparse(final_url).path
    expected_path = (
        route.get("interaction_requirements", {}).get("expected_final_path")
        if isinstance(route.get("interaction_requirements"), dict)
        else None
    ) or route.get("resolved_path") or route.get("route")
    if expected_path and final_path != expected_path:
        reasons.append(f"final path {final_path} != {expected_path}")
    if "codex_smoke_token" in final_url or "DESKTOP_SMOKE_TOKEN" in final_url:
        reasons.append("final_url contains smoke token material")
    forbidden_paths = []
    if isinstance(route.get("interaction_requirements"), dict):
        forbidden_paths = route["interaction_requirements"].get("forbidden_final_paths") or []
    if final_path in forbidden_paths:
        reasons.append(f"final path {final_path} is forbidden")
    if route.get("requires_smoke_token") and final_path == "/login":
        reasons.append("protected route resolved to /login")
    if not data.get("business_action"):
        reasons.append("business_action missing")
    assertions = data.get("assertions")
    if not isinstance(assertions, dict) or not assertions:
        reasons.append("assertions missing")
    return reasons


def validate_interaction_screenshot(
    interaction: dict[str, Any],
    interaction_path: Path,
    expected_width: int,
    expected_height: int,
) -> tuple[Path | None, Path, list[str]]:
    reasons: list[str] = []
    raw_screenshot = interaction.get("target_screenshot") or interaction.get("screenshot") or interaction.get("actual_screenshot")
    screenshot_path = Path(raw_screenshot) if raw_screenshot else interaction_path.with_name("interaction.png")
    capture_meta_path = screenshot_path.with_name("interaction-capture-meta.json")

    if not raw_screenshot:
        reasons.append("target_screenshot missing")

    if not screenshot_path.is_file():
        reasons.append(f"interaction screenshot missing: {repo_rel(screenshot_path)}")
    else:
        screenshot_size = image_size(screenshot_path)
        if screenshot_size != (expected_width, expected_height):
            reasons.append(f"interaction screenshot size {screenshot_size} != {expected_width}x{expected_height}")

    exists_meta, meta, meta_error = read_json_state(capture_meta_path)
    if not exists_meta:
        reasons.append(f"interaction-capture-meta missing: {repo_rel(capture_meta_path)}")
    elif not isinstance(meta, dict):
        reasons.append(f"interaction-capture-meta invalid: {meta_error}")
    else:
        reasons.extend(validate_capture_meta(meta, expected_width, expected_height))

    return screenshot_path, capture_meta_path, reasons


def render_markdown(summary: dict[str, Any]) -> str:
    lines: list[str] = []
    lines.append("# UI Visual Interaction Evidence Finalization")
    lines.append("")
    lines.append(f"- Run ID: `{summary['run_id']}`")
    lines.append(f"- Result: `{summary['result']}`")
    lines.append(f"- Visual evidence passed: `{summary['visual_passed_count']}/{summary['visual_target_count']}`")
    lines.append(f"- Interaction evidence passed: `{summary['interaction_passed_count']}/{summary['interaction_route_count']}`")
    lines.append(f"- Metrics generated: `{summary['metrics_generated_count']}`")
    lines.append(f"- Evidence dir: `{summary['evidence_dir']}`")
    lines.append("")
    lines.append("This finalizer only evaluates existing Desktop Chrome evidence. It does not capture screenshots and does not replace the dual gate preflight.")
    lines.append("")
    lines.append("## Visual Blockers")
    lines.append("")
    for item in summary["visual_results"]:
        if item["status"] != "pass":
            lines.append(f"- `{item['target_id']}`: {'; '.join(item['reasons'])}")
    lines.append("")
    lines.append("## Interaction Blockers")
    lines.append("")
    for item in summary["interaction_results"]:
        if item["status"] != "pass":
            lines.append(f"- `{item['route_id']}`: {'; '.join(item['reasons'])}")
    lines.append("")
    lines.append("## Formal Rerun")
    lines.append("")
    lines.append("```bash")
    lines.append("ALLOW_BLOCKERS=false tests/e2e/ui_visual_interaction_evidence_finalize.py")
    lines.append("DESKTOP_CHROME_STATUS=pass ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh")
    lines.append("ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh")
    lines.append("```")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    args = parse_args()
    run_id = args.run_id or f"{datetime.now().strftime('%Y%m%d%H%M%S')}-ui-visual-evidence-finalize"
    plan = read_json(args.capture_plan)
    evidence_dir = args.evidence_dir or Path(plan.get("evidence_dir") or DEFAULT_EVIDENCE_DIR)
    visual_targets = plan.get("visual_targets") or []
    interactions = plan.get("interactions") or []

    visual_results: list[dict[str, Any]] = []
    metrics_generated_count = 0
    for target in visual_targets:
        target_id = str(target.get("target_id") or target.get("id") or "")
        route = str(target.get("route") or target.get("resolved_path") or "")
        if target.get("query"):
            route = f"{route}?{target['query']}"
        source_path = Path(target.get("source_image") or "")
        actual_path = evidence_file(target, evidence_dir, target_id, "actual", "actual-1920.png")
        diff_path = evidence_file(target, evidence_dir, target_id, "diff", "diff-1920.png")
        metrics_path = evidence_file(target, evidence_dir, target_id, "metrics", "metrics.json")
        capture_meta_path = evidence_file(target, evidence_dir, target_id, "capture_meta", "capture-meta.json")
        reasons: list[str] = []

        if not source_path.is_file():
            reasons.append(f"source image missing: {repo_rel(source_path)}")
        else:
            source_size = image_size(source_path)
            if source_size != (args.expected_width, args.expected_height):
                reasons.append(f"source image size {source_size} != {args.expected_width}x{args.expected_height}")

        if not actual_path.is_file():
            reasons.append(f"actual screenshot missing: {repo_rel(actual_path)}")
        else:
            actual_size = image_size(actual_path)
            if actual_size != (args.expected_width, args.expected_height):
                reasons.append(f"actual screenshot size {actual_size} != {args.expected_width}x{args.expected_height}")

        exists_meta, meta, meta_error = read_json_state(capture_meta_path)
        if not exists_meta:
            reasons.append(f"capture-meta missing: {repo_rel(capture_meta_path)}")
        elif not isinstance(meta, dict):
            reasons.append(f"capture-meta invalid: {meta_error}")
        else:
            reasons.extend(validate_capture_meta(meta, args.expected_width, args.expected_height))

        metrics_status = "missing"
        pixel_ratio = None
        if source_path.is_file() and actual_path.is_file():
            try:
                metrics, diff_reasons = compare_images(
                    target_id=target_id,
                    route=route,
                    source_path=source_path,
                    actual_path=actual_path,
                    diff_path=diff_path,
                    metrics_path=metrics_path,
                    max_pixel_ratio=args.max_pixel_ratio,
                    channel_tolerance=args.channel_tolerance,
                    desktop_status="pass",
                    finalizer_run_id=run_id,
                )
                metrics_generated_count += 1
                metrics_status = str(metrics.get("status"))
                pixel_ratio = metrics.get("visual_diff", {}).get("pixel_mismatch_ratio")
                reasons.extend(diff_reasons)
            except Exception as exc:  # noqa: BLE001
                reasons.append(f"metrics generation failed: {exc}")

        visual_results.append(
            {
                "target_id": target_id,
                "route_id": target.get("route_id"),
                "status": "pass" if not reasons and metrics_status == "pass" else "blocked",
                "source": repo_rel(source_path),
                "actual": repo_rel(actual_path),
                "diff": repo_rel(diff_path),
                "metrics": repo_rel(metrics_path),
                "capture_meta": repo_rel(capture_meta_path),
                "metrics_status": metrics_status,
                "pixel_mismatch_ratio": pixel_ratio,
                "reasons": reasons,
            }
        )

    interaction_results: list[dict[str, Any]] = []
    for route in interactions:
        route_id = str(route.get("route_id") or route.get("id") or "")
        interaction_path = evidence_file(route, evidence_dir, route_id, "interaction", "interaction.json")
        reasons: list[str] = []
        exists_interaction, interaction, interaction_error = read_json_state(interaction_path)
        if not exists_interaction:
            reasons.append(f"interaction missing: {repo_rel(interaction_path)}")
        elif not isinstance(interaction, dict):
            reasons.append(f"interaction invalid: {interaction_error}")
        else:
            reasons.extend(validate_interaction(interaction, route))
            screenshot_path, screenshot_meta_path, screenshot_reasons = validate_interaction_screenshot(
                interaction,
                interaction_path,
                args.expected_width,
                args.expected_height,
            )
            reasons.extend(screenshot_reasons)
        if not isinstance(interaction, dict):
            screenshot_path = interaction_path.with_name("interaction.png")
            screenshot_meta_path = interaction_path.with_name("interaction-capture-meta.json")
        interaction_results.append(
            {
                "route_id": route_id,
                "route": route.get("route"),
                "status": "pass" if not reasons else "blocked",
                "interaction": repo_rel(interaction_path),
                "interaction_screenshot": repo_rel(screenshot_path),
                "interaction_capture_meta": repo_rel(screenshot_meta_path),
                "reasons": reasons,
            }
        )

    visual_passed = sum(1 for item in visual_results if item["status"] == "pass")
    interaction_passed = sum(1 for item in interaction_results if item["status"] == "pass")
    blockers = (len(visual_results) - visual_passed) + (len(interaction_results) - interaction_passed)
    result = "pass" if blockers == 0 else "blocked"
    summary = {
        "package_id": "ui_visual_interaction_evidence_finalization",
        "run_id": run_id,
        "result": result,
        "generated_at": now_iso(),
        "capture_plan": repo_rel(args.capture_plan),
        "evidence_dir": repo_rel(evidence_dir),
        "expected_size": {"width": args.expected_width, "height": args.expected_height},
        "max_pixel_ratio": args.max_pixel_ratio,
        "visual_target_count": len(visual_results),
        "visual_passed_count": visual_passed,
        "interaction_route_count": len(interaction_results),
        "interaction_passed_count": interaction_passed,
        "metrics_generated_count": metrics_generated_count,
        "blockers": blockers,
        "visual_results": visual_results,
        "interaction_results": interaction_results,
    }
    write_json(args.output_json, summary)
    args.output_md.parent.mkdir(parents=True, exist_ok=True)
    args.output_md.write_text(render_markdown(summary), encoding="utf-8")
    print(
        "ui-visual-interaction-evidence-finalize "
        f"result={result} visual={visual_passed}/{len(visual_results)} "
        f"interaction={interaction_passed}/{len(interaction_results)} summary={repo_rel(args.output_json)}"
    )
    if result != "pass" and not args.allow_blockers:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
