#!/usr/bin/env python3
"""Create a diff-aware third-view reviewer report for one task."""

from __future__ import annotations

import argparse
import json
import re
from datetime import datetime
from pathlib import Path
from typing import Any

from implement import CONTRACT_PATH_RULES, parse_patch_paths, path_is_safe, path_within
from lib import (
    copy_task_snapshot,
    ensure_run_dir,
    list_of,
    load_yaml_subset,
    make_run_id,
    rel_path,
    repo_path,
    run_git,
    write_json,
    write_text,
)


PASS_DECISION = "pass"
PENDING_DECISION = "pending"
REPAIR_DECISION = "repair_required"
DESIGN_DECISION = "design_update_required"
HUMAN_DECISION = "human_gate_required"


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def finding(level: str, code: str, target: str, message: str, suggestion: str) -> dict[str, str]:
    return {
        "level": level,
        "code": code,
        "target": target,
        "message": message,
        "suggestion": suggestion,
    }


def read_optional(path: Path) -> str:
    return path.read_text(encoding="utf-8") if path.exists() else ""


def changed_paths_from_run(run_dir: Path) -> list[str]:
    scope = load_json(run_dir / "implementation" / "patch-scope.json")
    touched = [str(item) for item in list_of(scope.get("touched_paths")) if item]
    if scope:
        return sorted(set(touched))
    patch_runner = load_json(run_dir / "patch-runner" / "patch-intake.json")
    touched = [str(item) for item in list_of(patch_runner.get("touched_paths")) if item]
    if patch_runner:
        return sorted(set(touched))
    changed = read_optional(run_dir / "changed-files.txt")
    if changed.strip():
        return sorted(set(line.strip() for line in changed.splitlines() if line.strip()))
    return sorted(set(line.strip() for line in run_git(["diff", "--name-only"]).splitlines() if line.strip()))


def local_report_status(run_dir: Path) -> tuple[str, str]:
    text = read_optional(run_dir / "local-report.md")
    if not text:
        return "missing", "local-report.md is missing"
    if "Not executed" in text:
        return "not_run", "local verification was not executed"
    codes = [int(match.group(1)) for match in re.finditer(r"exit_code:\s*(-?\d+)", text)]
    if codes and all(code == 0 for code in codes):
        return "passed", "all recorded local commands exited 0"
    if codes:
        return "failed", "at least one local command exited non-zero"
    return "unclear", "local report has no explicit exit_code markers"


def guidance_findings(task: dict[str, Any], guidance: dict[str, Any]) -> list[dict[str, str]]:
    task_id = str(task.get("id"))
    results: list[dict[str, str]] = []
    for item in guidance.get("findings", []):
        if str(item.get("target")) == task_id:
            results.append(
                finding(
                    str(item.get("level", "info")),
                    str(item.get("code")),
                    "guidance/guidance.json",
                    str(item.get("message")),
                    str(item.get("suggestion")),
                )
            )
    return results


def review_scope(task: dict[str, Any], changed_paths: list[str]) -> list[dict[str, str]]:
    allowed = [str(item).lstrip("./") for item in list_of((task.get("workspace") or {}).get("allowed_paths"))]
    contracts = task.get("contracts") or {}
    findings: list[dict[str, str]] = []
    for path in changed_paths:
        safe, reason = path_is_safe(path)
        if not safe:
            findings.append(finding("blocker", "UNSAFE_PATH", path, reason, "Remove unsafe path from the patch."))
            continue
        if allowed and not any(path_within(path, item) for item in allowed):
            findings.append(
                finding(
                    "blocker",
                    "PATH_OUT_OF_SCOPE",
                    path,
                    f"path is outside workspace.allowed_paths: {', '.join(allowed)}",
                    "Keep the patch inside task workspace boundaries or update the task through design review.",
                )
            )
        for prefix, contract_key in CONTRACT_PATH_RULES:
            if path_within(path, prefix) and not contracts.get(contract_key):
                findings.append(
                    finding(
                        "blocker",
                        "CONTRACT_NOT_DECLARED",
                        path,
                        f"path requires contracts.{contract_key}=true",
                        "Declare the contract and expand verification to consumers before closing.",
                    )
                )
    return findings


def review_diff_content(diff_text: str) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    suspicious = [
        (r"\bSKIP_AUTH\b|\bbypassAuth\b|auth\s*=\s*false", "AUTH_BYPASS_SUSPECT", "auth bypass-looking code appears in diff"),
        (r"password\s*[:=]\s*['\"][^'\"]+", "PLAINTEXT_SECRET_SUSPECT", "plaintext secret-looking assignment appears in diff"),
        (r"\bunwrap\(\)", "RUST_UNWRAP_SUSPECT", "Rust unwrap appears in diff"),
        (r"DROP\s+TABLE|DROP\s+DATABASE|TRUNCATE\s+", "DESTRUCTIVE_SQL_SUSPECT", "destructive SQL appears in diff"),
        (r"fetch\(", "DIRECT_FETCH_SUSPECT", "direct fetch appears in diff; Web UI should usually use services/api.ts"),
    ]
    additions = "\n".join(line[1:] for line in diff_text.splitlines() if line.startswith("+") and not line.startswith("+++"))
    for pattern, code, message in suspicious:
        if re.search(pattern, additions, flags=re.IGNORECASE):
            level = "blocker" if code in {"AUTH_BYPASS_SUSPECT", "PLAINTEXT_SECRET_SUSPECT", "DESTRUCTIVE_SQL_SUSPECT"} else "warning"
            findings.append(finding(level, code, "git diff", message, "Inspect the diff and either remove the risky construct or document why it is safe."))
    return findings


def load_diff(patch_path: Path | None, changed_paths: list[str]) -> str:
    if patch_path and patch_path.exists():
        return patch_path.read_text(encoding="utf-8")
    if not changed_paths:
        return ""
    return run_git(["diff", "--", *changed_paths])[-20000:]


def decide(findings: list[dict[str, str]], changed_paths: list[str], local_status: str) -> str:
    blockers = [item for item in findings if item["level"] == "blocker"]
    if any(item["code"] == "GUIDANCE_BLOCKER" or item["code"] == "SCREEN_AUTH_BOUNDARY" for item in blockers):
        return DESIGN_DECISION
    if any(item["code"] in {"PLAINTEXT_SECRET_SUSPECT", "DESTRUCTIVE_SQL_SUSPECT"} for item in blockers):
        return HUMAN_DECISION
    if blockers:
        return REPAIR_DECISION
    if not changed_paths:
        return PENDING_DECISION
    if local_status in {"missing", "not_run", "failed"}:
        return REPAIR_DECISION
    return PASS_DECISION


def perspective_status(decision: str, perspective: str, findings: list[dict[str, str]]) -> str:
    if decision == PASS_DECISION:
        return "pass"
    if perspective == "acceptance_evidence" and any(item["code"].startswith("LOCAL_") for item in findings):
        return "repair_required"
    if perspective in {"technical_design", "product_logic"} and decision == DESIGN_DECISION:
        return "design_update_required"
    if perspective == "code_correctness" and decision == REPAIR_DECISION:
        return "repair_required"
    return "pending" if decision == PENDING_DECISION else decision


def build_review(task: dict[str, Any], run_id: str, run_dir: Path, patch_path: Path | None, guidance: dict[str, Any]) -> dict[str, Any]:
    changed_paths = parse_patch_paths(patch_path) if patch_path else changed_paths_from_run(run_dir)
    local_status, local_message = local_report_status(run_dir)
    findings: list[dict[str, str]] = []
    findings.extend(guidance_findings(task, guidance))
    findings.extend(review_scope(task, changed_paths))
    findings.extend(review_diff_content(load_diff(patch_path, changed_paths)))
    if local_status == "failed":
        findings.append(finding("blocker", "LOCAL_FAILED", "local-report.md", local_message, "Fix the failure and rerun local verification."))
    elif local_status in {"missing", "not_run"} and changed_paths:
        findings.append(finding("blocker", "LOCAL_NOT_VERIFIED", "local-report.md", local_message, "Run the task's local verification before review pass."))
    elif local_status == "unclear":
        findings.append(finding("warning", "LOCAL_RESULT_UNCLEAR", "local-report.md", local_message, "Record exit_code markers in local-report.md."))
    if not changed_paths:
        findings.append(finding("warning", "NO_DIFF_TO_REVIEW", "git diff", "No patch or changed paths were available for diff-aware review.", "Generate or apply a patch before expecting reviewer pass."))

    decision = decide(findings, changed_paths, local_status)
    perspectives = {
        perspective: perspective_status(decision, str(perspective), findings)
        for perspective in list_of((task.get("review") or {}).get("perspectives"))
    }
    blockers = [item for item in findings if item["level"] == "blocker"]
    warnings = [item for item in findings if item["level"] == "warning"]
    return {
        "run_id": run_id,
        "run_kind": "diff_aware_review",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "decision": decision,
        "acceptance_type": task.get("acceptance_type"),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "changed_paths": changed_paths,
        "local_status": local_status,
        "perspectives": perspectives,
        "blocking_findings": blockers,
        "non_blocking_findings": warnings + [item for item in findings if item["level"] == "info"],
        "findings": findings,
    }


def render_review(review: dict[str, Any]) -> str:
    lines = [
        f"# Third-view Review: {review.get('task_id')}",
        "",
        f"- run_id: `{review['run_id']}`",
        f"- decision: `{review['decision']}`",
        f"- acceptance_type: `{review.get('acceptance_type')}`",
        f"- changed_paths: `{len(review['changed_paths'])}`",
        f"- local_status: `{review['local_status']}`",
        "",
        "## Perspectives",
    ]
    for key, value in review["perspectives"].items():
        lines.append(f"- {key}: {value}")
    lines.extend(["", "## Changed Paths"])
    lines.extend(f"- `{path}`" for path in review["changed_paths"]) if review["changed_paths"] else lines.append("- none")
    lines.extend(["", "## Blocking Findings"])
    if review["blocking_findings"]:
        for item in review["blocking_findings"]:
            lines.append(f"- `{item['code']}` `{item['target']}`: {item['message']} Suggestion: {item['suggestion']}")
    else:
        lines.append("- none")
    lines.extend(["", "## Non-blocking Findings"])
    if review["non_blocking_findings"]:
        for item in review["non_blocking_findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}` `{item['target']}`: {item['message']} Suggestion: {item['suggestion']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Product Logic Result",
            review["perspectives"].get("product_logic", "pending"),
            "",
            "## Technical Design Result",
            review["perspectives"].get("technical_design", "pending"),
            "",
            "## Evidence Result",
            review["perspectives"].get("acceptance_evidence", "pending"),
            "",
            "## Reviewer Rules",
            "- Do not pass a task by weakening product or technical design.",
            "- Do not mark regression evidence as acceptance or third-party evidence.",
            "- P0/P1 blockers must send the task to REPAIRING or DESIGN_ITERATING.",
            "",
        ]
    )
    return "\n".join(lines)


def render_design_delta(task: dict[str, Any], review: dict[str, Any]) -> str:
    needs_design = review["decision"] == DESIGN_DECISION
    lines = [
        f"# Design Delta: {task.get('id')}",
        "",
        f"- run_id: `{review['run_id']}`",
        f"- task: {task.get('title')}",
        f"- decision: `{review['decision']}`",
        "",
        "## Design Updates Needed",
    ]
    if needs_design:
        for item in review["blocking_findings"]:
            lines.append(f"- `{item['code']}`: {item['message']}")
    else:
        lines.append("- none recorded")
    lines.extend(["", "## Rationale"])
    lines.append("- Diff-aware reviewer found design blockers." if needs_design else "- No design update required by this reviewer run.")
    lines.extend(["", "## Target Documents"])
    lines.extend(f"- `{path}`" for path in list_of((task.get("review") or {}).get("design_update_allowed")))
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--patch", default=None)
    parser.add_argument("--guidance", default=None)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "review")))
    run_dir = ensure_run_dir(run_id)
    copy_task_snapshot(task_path, run_dir)
    patch_path = repo_path(args.patch) if args.patch else None
    guidance = load_json(repo_path(args.guidance) if args.guidance else run_dir / "guidance" / "guidance.json")
    review = build_review(task, run_id, run_dir, patch_path, guidance)
    review_dir = run_dir / "review"
    review_dir.mkdir(parents=True, exist_ok=True)
    write_json(review_dir / "review-summary.json", review)
    review_path = write_text(run_dir / "review-report.md", render_review(review))
    delta_path = write_text(run_dir / "design-delta.md", render_design_delta(task, review))
    print(review_path)
    print(delta_path)
    print(f"decision={review['decision']} blockers={len(review['blocking_findings'])} changed={len(review['changed_paths'])}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
