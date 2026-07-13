#!/usr/bin/env python3
"""Build and optionally apply task state transitions for Codex Loop tasks."""

from __future__ import annotations

import argparse
import json
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import (
    ensure_run_dir,
    list_of,
    load_yaml_subset,
    make_run_id,
    rel_path,
    repo_path,
    run_git,
    write_json,
    write_text,
    write_yaml,
)


OPEN_STATES = {
    "DISCOVERED",
    "RECOMMENDED_NEXT",
    "TRIAGED",
    "SPECED",
    "PLANNED",
    "APPROVED_FOR_AUTO",
    "IMPLEMENTING",
    "LOCAL_VERIFIED",
    "REVIEWING",
    "LIVE_VALIDATING",
    "EVIDENCING",
}

TERMINAL_STATES = {"CLOSED", "DEFERRED"}
EXCEPTION_STATES = {
    "NEEDS_HUMAN_GATE",
    "DESIGN_ITERATING",
    "REPAIRING",
    "QUARANTINED",
    "BLOCKED",
    "EVIDENCE_INCOMPLETE",
}
KNOWN_STATES = OPEN_STATES | TERMINAL_STATES | EXCEPTION_STATES

RUN_STATUS_TO_TASK_STATUS = {
    "PLANNED": "PLANNED",
    "DESIGN_ITERATING": "DESIGN_ITERATING",
    "DESIGN_READY": "SPECED",
    "CONTEXT_PACKED": "SPECED",
    "WORKFLOW_PREPARED": "PLANNED",
    "WORKFLOW_DRY_RUN": "PLANNED",
    "LOCAL_VERIFIED": "LOCAL_VERIFIED",
    "WORKFLOW_FAILED": "REPAIRING",
    "EVIDENCE_INCOMPLETE": "EVIDENCE_INCOMPLETE",
}


def load_json(path: Path | None) -> dict[str, Any]:
    if not path:
        return {}
    if not path.exists():
        raise FileNotFoundError(path)
    return json.loads(path.read_text(encoding="utf-8"))


def load_tasks(tasks_dir: Path) -> list[dict[str, Any]]:
    tasks: list[dict[str, Any]] = []
    for path in sorted(tasks_dir.glob("*.yaml")):
        task = load_yaml_subset(path)
        task["_path"] = rel_path(path)
        tasks.append(task)
    return tasks


def load_context(context_dir: Path | None) -> dict[str, Any]:
    if not context_dir:
        return {}
    evidence_path = context_dir / "evidence-ledger.json"
    gap_path = context_dir / "gap-index.json"
    return {
        "evidence": load_json(evidence_path) if evidence_path.exists() else {},
        "gaps": load_json(gap_path) if gap_path.exists() else {},
    }


def latest_task_runs(evidence: dict[str, Any]) -> dict[str, dict[str, Any]]:
    latest: dict[str, dict[str, Any]] = {}
    for run in evidence.get("runs", []):
        task_id = run.get("task_id")
        if not task_id:
            continue
        latest[str(task_id)] = run
    return latest


def findings_by_task(guidance: dict[str, Any]) -> dict[str, list[dict[str, Any]]]:
    result: dict[str, list[dict[str, Any]]] = {}
    for item in guidance.get("findings", []):
        target = str(item.get("target"))
        result.setdefault(target, []).append(item)
    return result


def suggestions_by_task(guidance: dict[str, Any]) -> dict[str, list[dict[str, Any]]]:
    result: dict[str, list[dict[str, Any]]] = {}
    for item in guidance.get("status_suggestions", []):
        target = str(item.get("target"))
        result.setdefault(target, []).append(item)
    return result


def recommendation_rank(guidance: dict[str, Any]) -> dict[str, int]:
    return {str(item.get("id")): index + 1 for index, item in enumerate(guidance.get("recommended_next", []))}


def transition_reason(
    task: dict[str, Any],
    task_findings: list[dict[str, Any]],
    task_suggestions: list[dict[str, Any]],
    latest_run: dict[str, Any] | None,
    rank: int | None,
    recommend_limit: int,
) -> tuple[str, str, str]:
    current = str(task.get("status", "DISCOVERED"))
    blockers = [item for item in task_findings if item.get("level") == "blocker"]
    if blockers:
        codes = ", ".join(str(item.get("code")) for item in blockers)
        return "DESIGN_ITERATING", "BLOCKER_PRESENT", f"Blocker findings require design iteration before execution: {codes}."

    if latest_run:
        mapped = RUN_STATUS_TO_TASK_STATUS.get(str(latest_run.get("status")))
        if mapped and mapped != current:
            return mapped, "LATEST_RUN_STATUS", f"Latest run `{latest_run.get('run_id')}` has status `{latest_run.get('status')}`."

    if task_suggestions:
        suggestion = task_suggestions[0]
        target_status = str(suggestion.get("to"))
        if target_status and target_status != current:
            return target_status, "GUIDANCE_STATUS_SUGGESTION", str(suggestion.get("reason"))

    if rank and rank <= recommend_limit and current == "DISCOVERED":
        return "RECOMMENDED_NEXT", "GUIDANCE_RECOMMENDATION", f"Ranked #{rank} in guidance recommended_next."

    return current, "NO_CHANGE", "No stronger transition signal was found."


def valid_transition(current: str, proposed: str, apply_terminal: bool = False) -> tuple[bool, str]:
    if proposed not in KNOWN_STATES:
        return False, f"unknown proposed state `{proposed}`"
    if current == proposed:
        return True, "no-op"
    if current in TERMINAL_STATES and not apply_terminal:
        return False, f"terminal current state `{current}` requires --allow-terminal-update"
    if proposed == "CLOSED":
        return False, "`CLOSED` cannot be applied by task_state.py; use evidence/review closure workflow"
    return True, "ok"


def build_board(tasks: list[dict[str, Any]], transitions: list[dict[str, Any]], guidance: dict[str, Any]) -> str:
    transition_by_id = {item["task_id"]: item for item in transitions}
    lines = [
        "# Codex Loop Task Board",
        "",
        f"- generated_at: `{datetime.now().isoformat(timespec='seconds')}`",
        f"- task_count: `{len(tasks)}`",
        f"- guidance_recommendations: `{len(guidance.get('recommended_next', []))}`",
        "",
        "| Task | Current | Proposed | Reason | Rank | Latest Run |",
        "|---|---|---|---|---:|---|",
    ]
    for task in tasks:
        task_id = str(task.get("id"))
        item = transition_by_id[task_id]
        latest = item.get("latest_run") or {}
        latest_label = latest.get("run_id") or ""
        if latest_label:
            latest_label = f"{latest_label} / {latest.get('status')}"
        lines.append(
            f"| `{task_id}` | `{item['current_status']}` | `{item['proposed_status']}` | {item['reason_code']} | {item.get('rank') or ''} | {latest_label} |"
        )
    lines.extend(["", "## Guardrail", "- This board is task-state evidence only; it does not close a task.", ""])
    return "\n".join(lines)


def build_transitions(
    tasks: list[dict[str, Any]],
    context: dict[str, Any],
    guidance: dict[str, Any],
    allow_terminal_update: bool,
    recommend_limit: int,
) -> list[dict[str, Any]]:
    latest = latest_task_runs(context.get("evidence") or {})
    by_findings = findings_by_task(guidance)
    by_suggestions = suggestions_by_task(guidance)
    ranks = recommendation_rank(guidance)
    transitions: list[dict[str, Any]] = []
    for task in tasks:
        task_id = str(task.get("id"))
        current = str(task.get("status", "DISCOVERED"))
        proposed, reason_code, reason = transition_reason(
            task=task,
            task_findings=by_findings.get(task_id, []),
            task_suggestions=by_suggestions.get(task_id, []),
            latest_run=latest.get(task_id),
            rank=ranks.get(task_id),
            recommend_limit=recommend_limit,
        )
        valid, validation = valid_transition(current, proposed, apply_terminal=allow_terminal_update)
        transitions.append(
            {
                "task_id": task_id,
                "title": task.get("title"),
                "path": task.get("_path"),
                "current_status": current,
                "proposed_status": proposed,
                "changed": current != proposed,
                "valid": valid,
                "validation": validation,
                "reason_code": reason_code,
                "reason": reason,
                "rank": ranks.get(task_id),
                "findings": by_findings.get(task_id, []),
                "status_suggestions": by_suggestions.get(task_id, []),
                "latest_run": latest.get(task_id),
            }
        )
    return transitions


def apply_transitions(transitions: list[dict[str, Any]]) -> list[dict[str, Any]]:
    applied: list[dict[str, Any]] = []
    now = datetime.now().isoformat(timespec="seconds")
    for item in transitions:
        if not item["changed"] or not item["valid"]:
            continue
        path = repo_path(item["path"])
        task = load_yaml_subset(path)
        old_status = task.get("status")
        task["status"] = item["proposed_status"]
        write_yaml(path, task)
        applied.append(
            {
                "ts": now,
                "task_id": item["task_id"],
                "path": item["path"],
                "from": old_status,
                "to": item["proposed_status"],
                "reason_code": item["reason_code"],
                "reason": item["reason"],
            }
        )
    return applied


def render_apply_report(transitions: list[dict[str, Any]], applied: list[dict[str, Any]], apply: bool) -> str:
    invalid = [item for item in transitions if item["changed"] and not item["valid"]]
    planned = [item for item in transitions if item["changed"] and item["valid"]]
    lines = [
        "# Task State Apply Report",
        "",
        f"- apply: `{apply}`",
        f"- planned_changes: `{len(planned)}`",
        f"- applied_changes: `{len(applied)}`",
        f"- invalid_changes: `{len(invalid)}`",
        "",
        "## Planned Changes",
    ]
    if planned:
        for item in planned:
            lines.append(f"- `{item['task_id']}`: `{item['current_status']}` -> `{item['proposed_status']}` ({item['reason_code']})")
    else:
        lines.append("- none")
    lines.extend(["", "## Invalid Changes"])
    if invalid:
        for item in invalid:
            lines.append(f"- `{item['task_id']}`: `{item['current_status']}` -> `{item['proposed_status']}` blocked: {item['validation']}")
    else:
        lines.append("- none")
    lines.append("")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tasks-dir", default="scripts/codex_loop/tasks")
    parser.add_argument("--context-dir", default=None)
    parser.add_argument("--guidance", default=None)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--out-dir", default=None)
    parser.add_argument("--apply", action="store_true")
    parser.add_argument("--allow-terminal-update", action="store_true")
    parser.add_argument("--recommend-limit", type=int, default=3)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("task-state")
    run_dir = ensure_run_dir(run_id)
    out_dir = repo_path(args.out_dir) if args.out_dir else run_dir / "task-state"
    out_dir.mkdir(parents=True, exist_ok=True)

    tasks = load_tasks(repo_path(args.tasks_dir))
    context = load_context(repo_path(args.context_dir) if args.context_dir else None)
    guidance = load_json(repo_path(args.guidance) if args.guidance else None)
    transitions = build_transitions(tasks, context, guidance, args.allow_terminal_update, args.recommend_limit)
    applied = apply_transitions(transitions) if args.apply else []

    state = {
        "run_id": run_id,
        "run_kind": "task_state",
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "tasks_dir": rel_path(repo_path(args.tasks_dir)),
        "context_dir": rel_path(repo_path(args.context_dir)) if args.context_dir else None,
        "guidance_path": rel_path(repo_path(args.guidance)) if args.guidance else None,
        "apply": args.apply,
        "recommend_limit": args.recommend_limit,
        "summary": {
            "tasks": len(tasks),
            "planned_changes": sum(1 for item in transitions if item["changed"] and item["valid"]),
            "invalid_changes": sum(1 for item in transitions if item["changed"] and not item["valid"]),
            "applied_changes": len(applied),
        },
        "transitions": transitions,
        "applied": applied,
    }
    write_json(out_dir / "task-state.json", state)
    write_json(out_dir / "transition-plan.json", {"run_id": run_id, "transitions": transitions})
    write_text(out_dir / "task-board.md", build_board(tasks, transitions, guidance))
    write_text(out_dir / "apply-report.md", render_apply_report(transitions, applied, args.apply))
    write_text(out_dir / "transition-log.jsonl", "".join(json.dumps(item, ensure_ascii=False) + "\n" for item in applied))

    summary = {
        "run_id": run_id,
        "run_kind": "task_state",
        "status": "TASK_STATE_APPLIED" if args.apply else "TASK_STATE_PLANNED",
        "evidence_type": "acceptance-prep",
        "created_at": state["created_at"],
        "commit": state["commit"],
        "tasks_dir": state["tasks_dir"],
        "context_dir": state["context_dir"],
        "guidance_path": state["guidance_path"],
        "summary": state["summary"],
        "outputs": [
            "task-state/task-state.json",
            "task-state/task-board.md",
            "task-state/transition-plan.json",
            "task-state/apply-report.md",
            "task-state/transition-log.jsonl",
        ],
        "warning": "Task state evidence is a transition plan. It does not close a task or prove implementation.",
    }
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(
        f"status={summary['status']} planned={state['summary']['planned_changes']} "
        f"applied={state['summary']['applied_changes']} invalid={state['summary']['invalid_changes']}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
