#!/usr/bin/env python3
"""Generate correction and next-step guidance from Context Scout ledgers."""

from __future__ import annotations

import argparse
import json
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import ensure_run_dir, list_of, load_yaml_subset, rel_path, repo_path, run_git, write_json, write_text
from scout import main as scout_main  # noqa: F401  # Imported to keep scout.py discoverable for py_compile.


VALID_EVIDENCE_TYPES = {
    "smoke",
    "regression",
    "acceptance",
    "acceptance-prep",
    "security",
    "third-party",
    "third-party-prep",
}

PRIORITY_WEIGHT = {"P0": 1000, "P1": 500, "P2": 100}
MODE_WEIGHT = {"local": 35, "backup": 30, "review": 25, "plan": 15}


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def load_context(context_dir: Path) -> dict[str, dict[str, Any]]:
    required = {
        "context": "context.snapshot.json",
        "gaps": "gap-index.json",
        "deps": "dependency-map.json",
        "evidence": "evidence-ledger.json",
    }
    missing = [name for name in required.values() if not (context_dir / name).exists()]
    if missing:
        raise FileNotFoundError(f"Missing Context Scout files in {context_dir}: {', '.join(missing)}")
    return {key: load_json(context_dir / filename) for key, filename in required.items()}


def load_tasks(tasks_dir: Path) -> list[dict[str, Any]]:
    tasks: list[dict[str, Any]] = []
    for path in sorted(tasks_dir.glob("*.yaml")):
        task = load_yaml_subset(path)
        task["_path"] = str(path.relative_to(repo_path(".")))
        tasks.append(task)
    return tasks


def severity(level: str, code: str, message: str, target: str, suggestion: str) -> dict[str, str]:
    return {
        "level": level,
        "code": code,
        "target": target,
        "message": message,
        "suggestion": suggestion,
    }


def validate_tasks(tasks: list[dict[str, Any]]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    required = ["id", "title", "priority", "status", "lane", "acceptance_type", "subsystems", "execution", "workspace", "verification", "review", "evidence", "close_when"]
    for task in tasks:
        task_id = str(task.get("id", task.get("_path")))
        for key in required:
            if key not in task or task.get(key) in (None, "", []):
                findings.append(severity("blocker", "TASK_SCHEMA_MISSING", f"Task missing `{key}`.", task_id, "Fill the required task field before planning or execution."))
        evidence_type = str(task.get("acceptance_type", ""))
        if evidence_type not in VALID_EVIDENCE_TYPES:
            findings.append(severity("blocker", "UNKNOWN_EVIDENCE_TYPE", f"Unknown acceptance_type `{evidence_type}`.", task_id, "Use smoke, regression, acceptance-prep, acceptance, security, third-party-prep or third-party."))
        execution = task.get("execution") or {}
        if execution.get("allow_live_write") is True and (task.get("data_plan") or {}).get("cleanup") in (None, "none"):
            findings.append(severity("blocker", "LIVE_WRITE_NO_CLEANUP", "Live write is enabled without cleanup.", task_id, "Add run_id, tenant, cleanup and human gate before live-generated execution."))
        if task.get("priority") == "P0" and not (task.get("review") or {}).get("required"):
            findings.append(severity("blocker", "P0_REVIEW_DISABLED", "P0 task has review.required disabled.", task_id, "Enable Third-view Reviewer Gate."))
        if (task.get("risk") or {}).get("level") == "high" and execution.get("mode") == "local":
            findings.append(severity("warning", "HIGH_RISK_LOCAL", "High-risk task is allowed to enter local mode.", task_id, "Keep security and reviewer gates mandatory; consider planning before implementation."))
    return findings


def validate_evidence(evidence: dict[str, Any]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    for run in evidence.get("runs", []):
        run_id = str(run.get("run_id"))
        missing = list_of(run.get("missing_core_files"))
        status = str(run.get("status"))
        evidence_type = str(run.get("evidence_type") or "unknown")
        if missing:
            findings.append(severity("warning", "RUN_EVIDENCE_INCOMPLETE", f"Run is missing core files: {', '.join(missing)}.", run_id, "Do not use this run to close a task until core evidence exists."))
        if evidence_type == "third-party" and not run.get("external_report"):
            findings.append(severity("blocker", "THIRD_PARTY_NO_REPORT", "Third-party evidence has no external report reference.", run_id, "Attach a third-party report before using this evidence layer."))
        if evidence_type == "acceptance" and status not in {"ACCEPTANCE_READY", "CLOSED", "PASSED"}:
            findings.append(severity("warning", "ACCEPTANCE_STATUS_MISMATCH", f"Acceptance evidence has status `{status}`.", run_id, "Confirm acceptance gates before using this as acceptance evidence."))
    return findings


def validate_dependencies(deps: dict[str, Any]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    routes = deps.get("routes", {}).get("routes", [])
    for route in routes:
        if route.get("path") == "/screen" and route.get("note"):
            findings.append(severity("blocker", "SCREEN_AUTH_BOUNDARY", "/screen is outside ProtectedLayout.", "CLE-P0-SCREEN-001", "Resolve the /screen public/protected/readonly strategy before claiming UI auth boundary closure."))
    for contract, tasks in (deps.get("contract_to_tasks") or {}).items():
        if contract in {"proto", "database_schema", "kafka_topics"} and tasks:
            findings.append(severity("info", "CONTRACT_IMPACT", f"`{contract}` changes affect {', '.join(tasks)}.", contract, "Expand verification to all listed dependent tasks before closing a contract-impacting change."))
    return findings


def score_task(task: dict[str, Any], deps: dict[str, Any], findings: list[dict[str, str]]) -> int:
    task_id = str(task.get("id"))
    score = 0
    score += PRIORITY_WEIGHT.get(str(task.get("priority")), 0)
    score += MODE_WEIGHT.get(str((task.get("execution") or {}).get("mode")), 0)
    score += 80 if (task.get("risk") or {}).get("level") == "high" else 20
    for contract_tasks in (deps.get("contract_to_tasks") or {}).values():
        if task_id in contract_tasks:
            score += 25
    for finding in findings:
        if finding.get("target") == task_id and finding.get("level") == "blocker":
            if finding.get("code") in {"TASK_SCHEMA_MISSING", "UNKNOWN_EVIDENCE_TYPE", "LIVE_WRITE_NO_CLEANUP", "P0_REVIEW_DISABLED"}:
                score -= 500
            else:
                score += 150
    return score


def recommended_tasks(tasks: list[dict[str, Any]], deps: dict[str, Any], findings: list[dict[str, str]]) -> list[dict[str, Any]]:
    ranked = []
    for task in tasks:
        ranked.append(
            {
                "id": task.get("id"),
                "title": task.get("title"),
                "priority": task.get("priority"),
                "status": task.get("status"),
                "lane": (task.get("lane") or {}).get("primary"),
                "mode": (task.get("execution") or {}).get("mode"),
                "acceptance_type": task.get("acceptance_type"),
                "score": score_task(task, deps, findings),
                "path": task.get("_path"),
            }
        )
    return sorted(ranked, key=lambda item: item["score"], reverse=True)


def status_suggestions(tasks: list[dict[str, Any]], evidence: dict[str, Any], recommendations: list[dict[str, Any]]) -> list[dict[str, str]]:
    suggestions: list[dict[str, str]] = []
    task_by_id = {str(task.get("id")): task for task in tasks}
    for item in recommendations[:3]:
        task = task_by_id.get(str(item.get("id")))
        if not task:
            continue
        task_id = str(task.get("id"))
        status = str(task.get("status"))
        if status == "DISCOVERED":
            suggestions.append({"target": task_id, "from": status, "to": "RECOMMENDED_NEXT", "reason": "Highest current priority after guidance scoring."})
    for task in tasks:
        task_id = str(task.get("id"))
        status = str(task.get("status"))
        if status == "CLOSED":
            matching_runs = [run for run in evidence.get("runs", []) if run.get("task_id") == task_id]
            if not matching_runs or any(run.get("missing_core_files") for run in matching_runs):
                suggestions.append({"target": task_id, "from": status, "to": "EVIDENCE_INCOMPLETE", "reason": "Closed task lacks complete core evidence."})
    for run in evidence.get("runs", []):
        if run.get("missing_core_files") and run.get("status") not in {"NO_SUMMARY", "CONTEXT_SCOUTED"}:
            suggestions.append({"target": str(run.get("run_id")), "from": str(run.get("status")), "to": "EVIDENCE_INCOMPLETE", "reason": "Run has missing core files."})
    return suggestions


def render_report(guidance: dict[str, Any]) -> str:
    findings = guidance["findings"]
    recommendations = guidance["recommended_next"]
    suggestions = guidance["status_suggestions"]
    lines = [
        "# Codex Loop Guidance",
        "",
        f"- generated_from: `{guidance['context_dir']}`",
        f"- blockers: `{sum(1 for item in findings if item['level'] == 'blocker')}`",
        f"- warnings: `{sum(1 for item in findings if item['level'] == 'warning')}`",
        f"- recommendations: `{len(recommendations)}`",
        "",
        "## Correction Findings",
    ]
    if findings:
        for item in findings:
            lines.append(f"- `{item['level']}` `{item['code']}` `{item['target']}`: {item['message']} Suggestion: {item['suggestion']}")
    else:
        lines.append("- none")
    lines.extend(["", "## Recommended Next"])
    for item in recommendations[:8]:
        lines.append(f"- score `{item['score']}` `{item['id']}` [{item['priority']}/{item['mode']}/{item['acceptance_type']}]: {item['title']}")
    lines.extend(["", "## Status Suggestions"])
    if suggestions:
        for item in suggestions:
            lines.append(f"- `{item['target']}`: `{item['from']}` -> `{item['to']}` because {item['reason']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- This guidance does not modify task status by itself.",
            "- A blocker means do not close the affected task or evidence run until the issue is resolved.",
            "- A recommendation is a scheduling hint, not permission to bypass task gates.",
            "",
        ]
    )
    return "\n".join(lines)


def build_guidance(context_dir: Path, tasks_dir: Path) -> dict[str, Any]:
    ledgers = load_context(context_dir)
    tasks = load_tasks(tasks_dir)
    findings: list[dict[str, str]] = []
    findings.extend(validate_tasks(tasks))
    findings.extend(validate_evidence(ledgers["evidence"]))
    findings.extend(validate_dependencies(ledgers["deps"]))
    recommendations = recommended_tasks(tasks, ledgers["deps"], findings)
    suggestions = status_suggestions(tasks, ledgers["evidence"], recommendations)
    return {
        "context_dir": str(context_dir),
        "summary": {
            "blockers": sum(1 for item in findings if item["level"] == "blocker"),
            "warnings": sum(1 for item in findings if item["level"] == "warning"),
            "infos": sum(1 for item in findings if item["level"] == "info"),
            "recommended_count": len(recommendations),
        },
        "findings": findings,
        "recommended_next": recommendations,
        "status_suggestions": suggestions,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--context-dir", default=None, help="Context Scout output directory.")
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--tasks-dir", default="scripts/codex_loop/tasks")
    parser.add_argument("--out-dir", default=None)
    args = parser.parse_args()

    if args.context_dir:
        context_dir = repo_path(args.context_dir)
        if args.run_id:
            run_dir = ensure_run_dir(args.run_id)
        else:
            run_dir = context_dir.parent if context_dir.name == "context" else ensure_run_dir("guidance")
    else:
        run_dir = ensure_run_dir(args.run_id)
        context_dir = run_dir / "context"
        if not (context_dir / "context.snapshot.json").exists():
            raise FileNotFoundError("Context files do not exist. Run scout.py first or pass --context-dir.")

    out_dir = repo_path(args.out_dir) if args.out_dir else run_dir / "guidance"
    out_dir.mkdir(parents=True, exist_ok=True)
    guidance = build_guidance(context_dir, repo_path(args.tasks_dir))
    write_json(out_dir / "guidance.json", guidance)
    write_text(out_dir / "guidance-report.md", render_report(guidance))
    summary = {
        "run_id": args.run_id or run_dir.name,
        "run_kind": "guidance",
        "status": "GUIDANCE_GENERATED",
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "context_dir": rel_path(context_dir),
        "summary": guidance["summary"],
        "outputs": [
            "guidance/guidance.json",
            "guidance/guidance-report.md",
        ],
    }
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"blockers={guidance['summary']['blockers']} warnings={guidance['summary']['warnings']} recommendations={guidance['summary']['recommended_count']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
