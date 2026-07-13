#!/usr/bin/env python3
"""Turn failed evidence checks into a concrete Codex repair plan.

The repair planner is the feedback-loop half of the automation engine. It does
not patch code; it converts evidence failures into the next design, implement,
verify, or human-gate step that Codex should execute.
"""

from __future__ import annotations

import argparse
import json
from datetime import datetime
from pathlib import Path
from typing import Any

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


DESIGN_CODES = {"GUIDANCE_BLOCKER", "CLOSE_WHEN_UNPROVEN"}
VERIFY_CODES = {"LOCAL_REPORT_MISSING", "LOCAL_NOT_EXECUTED", "LOCAL_FAILED", "LOCAL_RESULT_UNCLEAR"}
REVIEW_CODES = {"REVIEW_REPORT_MISSING", "REVIEW_DECISION_MISSING", "REVIEW_NOT_PASSED"}
EVIDENCE_CODES = {
    "MISSING_REQUIRED_EVIDENCE",
    "PREP_EVIDENCE_CANNOT_CLOSE",
    "EVIDENCE_TYPE_MISMATCH",
    "EVIDENCE_REPORT_MISSING",
    "RUN_STATUS_NOT_CLOSABLE",
    "LIVE_REPORT_MISSING",
    "DESTRUCTIVE_SQL_ARTIFACT",
}
IMPLEMENT_CODES = {
    "PATCH_SCOPE_INVALID",
    "PATCH_RUNNER_BLOCKER",
    "CODEX_OUTPUT_FIELD_MISSING",
}
RUNTIME_CODES = {
    "LIVE_REPORT_HAS_ERRORS",
    "BROWSER_REPORT_HAS_ERRORS",
}


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def classify(code: str) -> str:
    if code in DESIGN_CODES:
        return "design"
    if code in VERIFY_CODES:
        return "verify"
    if code in REVIEW_CODES:
        return "review"
    if code in EVIDENCE_CODES:
        return "evidence"
    if code in IMPLEMENT_CODES:
        return "implement"
    if code in RUNTIME_CODES:
        return "verify"
    return "triage"


def action_for(kind: str, finding: dict[str, str]) -> str:
    code = finding.get("code")
    if kind == "design":
        return "Regenerate or update the design package, then rerun guidance before implementation."
    if kind == "implement":
        return "Repair patch scope or contract declarations, then rerun implement.py with the corrected patch."
    if kind == "verify":
        return "Run the declared local verification and preserve command output with exit codes."
    if kind == "review":
        return "Complete the third-view reviewer decision and resolve any blocking findings."
    if kind == "evidence":
        return "Publish a concrete evidence report that maps close_when items to artifacts and the required evidence layer."
    return f"Triage `{code}` and decide whether the task should repair, iterate design, or enter human gate."


def next_status(findings: list[dict[str, str]]) -> str:
    blocker_codes = {item.get("code") for item in findings if item.get("level") == "blocker"}
    if not blocker_codes:
        return "READY_FOR_STATE_UPDATE"
    if "GUIDANCE_BLOCKER" in blocker_codes:
        return "DESIGN_ITERATING"
    if "REVIEW_NOT_PASSED" in blocker_codes:
        return "REPAIRING"
    return "REPAIRING"


def build_repair_plan(task: dict[str, Any], check: dict[str, Any], run_dir: Path) -> dict[str, Any]:
    blockers = [item for item in check.get("findings", []) if item.get("level") == "blocker"]
    warnings = [item for item in check.get("findings", []) if item.get("level") == "warning"]
    steps: list[dict[str, str]] = []
    seen: set[tuple[str, str]] = set()
    for finding in blockers + warnings:
        code = str(finding.get("code"))
        kind = classify(code)
        key = (kind, code)
        if key in seen:
            continue
        seen.add(key)
        steps.append(
            {
                "kind": kind,
                "code": code,
                "target": str(finding.get("evidence")),
                "action": action_for(kind, finding),
                "source_message": str(finding.get("message")),
            }
        )
    if not steps:
        steps.append(
            {
                "kind": "state",
                "code": "NO_BLOCKERS",
                "target": "task_state",
                "action": "Run task_state.py and consider applying the status transition after human review.",
                "source_message": "Evidence checker found no blockers.",
            }
        )
    return {
        "run_kind": "repair_plan",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "checked_run": rel_path(run_dir),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "source_check_status": check.get("status"),
        "recommended_next_status": next_status(check.get("findings", [])),
        "blocker_count": len(blockers),
        "warning_count": len(warnings),
        "steps": steps,
        "guardrail": "Repair plan does not apply code or status changes by itself.",
    }


def render_report(plan: dict[str, Any]) -> str:
    lines = [
        f"# Repair Plan: {plan.get('task_id')}",
        "",
        f"- checked_run: `{plan['checked_run']}`",
        f"- source_check_status: `{plan.get('source_check_status')}`",
        f"- recommended_next_status: `{plan['recommended_next_status']}`",
        f"- blockers: `{plan['blocker_count']}`",
        f"- warnings: `{plan['warning_count']}`",
        "",
        "## Ordered Repair Steps",
    ]
    for index, step in enumerate(plan["steps"], start=1):
        lines.append(
            f"{index}. `{step['kind']}` `{step['code']}` `{step['target']}`: {step['action']} Source: {step['source_message']}"
        )
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Do not skip back to CLOSED from this plan.",
            "- After repair, rerun implementation/verification/review as appropriate, then rerun evidence_check.py.",
            "",
        ]
    )
    return "\n".join(lines)


def render_prompt(task: dict[str, Any], plan: dict[str, Any]) -> str:
    allowed = list_of((task.get("workspace") or {}).get("allowed_paths"))
    lines = [
        f"# Codex Repair Prompt: {task.get('id')}",
        "",
        f"Use `doc/02_acceptance/runs/{Path(plan['checked_run']).name}/repair/repair-plan.json` as the repair source.",
        "",
        "Repair objective:",
        f"- Move the task from `{plan['recommended_next_status']}` toward verified evidence, not direct closure.",
        "",
        "Allowed paths:",
    ]
    lines.extend(f"- `{path}`" for path in allowed) if allowed else lines.append("- none declared")
    lines.extend(["", "Required repair steps:"])
    for step in plan["steps"]:
        lines.append(f"- `{step['kind']}` `{step['code']}`: {step['action']}")
    lines.extend(
        [
            "",
            "After repairing, run the declared verification, complete reviewer output, and rerun `evidence_check.py`.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--run-dir", default=None)
    parser.add_argument("--evidence-check", default=None)
    parser.add_argument("--out-dir", default=None)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    if args.run_dir:
        run_dir = repo_path(args.run_dir)
    else:
        run_id = args.run_id or make_run_id(str(task.get("id", "repair")))
        run_dir = ensure_run_dir(run_id)
    copy_task_snapshot(task_path, run_dir)

    check_path = repo_path(args.evidence_check) if args.evidence_check else run_dir / "evidence-check" / "evidence-check.json"
    check = load_json(check_path)
    if not check:
        raise FileNotFoundError(f"Evidence check not found: {check_path}")

    out_dir = repo_path(args.out_dir) if args.out_dir else run_dir / "repair"
    out_dir.mkdir(parents=True, exist_ok=True)
    plan = build_repair_plan(task, check, run_dir)
    write_json(out_dir / "repair-plan.json", plan)
    write_text(out_dir / "repair-report.md", render_report(plan))
    write_text(out_dir / "codex-repair-prompt.md", render_prompt(task, plan))
    print(out_dir)
    print(f"next={plan['recommended_next_status']} blockers={plan['blocker_count']} steps={len(plan['steps'])}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
