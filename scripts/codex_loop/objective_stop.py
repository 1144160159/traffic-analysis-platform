#!/usr/bin/env python3
"""Evaluate project-level objective stop conditions for the Codex Loop."""

from __future__ import annotations

import argparse
import json
from collections import Counter
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import (
    RUNS_ROOT,
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


DEFAULT_POLICY = "scripts/codex_loop/policies/objective-stop.yaml"
READY = "OBJECTIVE_STOP_READY"
CONTINUE = "OBJECTIVE_STOP_CONTINUE"
BLOCKED = "OBJECTIVE_STOP_BLOCKED"
HUMAN_GATE = "OBJECTIVE_STOP_HUMAN_GATE"


def load_json(path: Path | None) -> dict[str, Any]:
    if not path:
        return {}
    if not path.exists():
        return {"missing": True, "path": rel_path(path)}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return {"parse_error": str(exc), "path": rel_path(path)}


def load_tasks(tasks_dir: Path) -> list[dict[str, Any]]:
    tasks: list[dict[str, Any]] = []
    if not tasks_dir.exists():
        return tasks
    for path in sorted(tasks_dir.glob("*.yaml")):
        task = load_yaml_subset(path)
        task["_path"] = rel_path(path)
        tasks.append(task)
    return tasks


def latest_context_dir() -> Path | None:
    candidates: list[tuple[str, Path]] = []
    if not RUNS_ROOT.exists():
        return None
    for run_dir in RUNS_ROOT.iterdir():
        summary = load_json(run_dir / "run-summary.json")
        context_dir = run_dir / "context"
        if summary.get("run_kind") == "context_scout" and (context_dir / "evidence-ledger.json").exists():
            candidates.append((str(summary.get("created_at") or run_dir.name), context_dir))
    if not candidates:
        return None
    return sorted(candidates, key=lambda item: item[0])[-1][1]


def latest_runs_by_kind(evidence: dict[str, Any]) -> dict[str, dict[str, Any]]:
    latest: dict[str, dict[str, Any]] = {}
    for run in evidence.get("runs", []):
        kind = str(run.get("run_kind") or "task")
        current = latest.get(kind)
        if not current:
            latest[kind] = run
            continue
        current_key = (str(current.get("created_at") or ""), str(current.get("run_id") or ""))
        candidate_key = (str(run.get("created_at") or ""), str(run.get("run_id") or ""))
        if candidate_key >= current_key:
            latest[kind] = run
    return latest


def add_finding(findings: list[dict[str, Any]], level: str, code: str, message: str, evidence: str, recommendation: str) -> None:
    findings.append(
        {
            "level": level,
            "code": code,
            "message": message,
            "evidence": evidence,
            "recommendation": recommendation,
        }
    )


def status_for(findings: list[dict[str, Any]]) -> str:
    if any(item["level"] == "blocker" for item in findings):
        return BLOCKED
    if any(item["level"] == "human_gate" for item in findings):
        return HUMAN_GATE
    if any(item["level"] == "pending" for item in findings):
        return CONTINUE
    return READY


def stop_recommendation(status: str) -> str:
    if status == READY:
        return "stop_success"
    if status == BLOCKED:
        return "stop_for_repair"
    if status == HUMAN_GATE:
        return "stop_for_human_gate"
    return "continue_loop"


def build_stop_conditions(
    status: str,
    findings: list[dict[str, Any]],
    task_summary: dict[str, Any],
    evidence_summary: dict[str, Any],
    policy: dict[str, Any],
) -> dict[str, Any]:
    levels = Counter(str(item.get("level")) for item in findings)
    codes = Counter(str(item.get("code")) for item in findings)
    open_required = int(task_summary.get("open_required") or 0)
    missing_required_kinds = list_of(evidence_summary.get("missing_required_kinds"))
    release_status = evidence_summary.get("release_status")
    checks = [
        {
            "name": "required_tasks_terminal",
            "passed": open_required == 0,
            "actual": open_required,
            "expected": "0 open required P0/P1 tasks",
        },
        {
            "name": "no_blocker_findings",
            "passed": levels.get("blocker", 0) == 0,
            "actual": levels.get("blocker", 0),
            "expected": 0,
        },
        {
            "name": "no_human_gate_findings",
            "passed": levels.get("human_gate", 0) == 0,
            "actual": levels.get("human_gate", 0),
            "expected": 0,
        },
        {
            "name": "no_pending_findings",
            "passed": levels.get("pending", 0) == 0,
            "actual": levels.get("pending", 0),
            "expected": 0,
        },
        {
            "name": "required_run_kinds_present",
            "passed": not missing_required_kinds,
            "actual": missing_required_kinds,
            "expected": list_of(policy.get("ready_required_run_kinds")),
        },
        {
            "name": "latest_blocker_runs_clear",
            "passed": codes.get("LATEST_REQUIRED_RUN_BLOCKED", 0) == 0,
            "actual": codes.get("LATEST_REQUIRED_RUN_BLOCKED", 0),
            "expected": 0,
        },
    ]
    if bool(policy.get("require_release_frozen")):
        checks.append(
            {
                "name": "release_frozen",
                "passed": release_status == "RELEASE_FROZEN",
                "actual": release_status,
                "expected": "RELEASE_FROZEN",
            }
        )
    return {
        "active_status": status,
        "active_recommendation": stop_recommendation(status),
        "success_stop": {
            "status": READY,
            "stop": True,
            "objective_complete": True,
            "when": "all condition checks pass and there are no blocker, human_gate, or pending findings",
        },
        "continue_loop": {
            "status": CONTINUE,
            "stop": False,
            "objective_complete": False,
            "when": "no blocker or human gate exists, but required tasks or evidence are still pending",
        },
        "repair_stop": {
            "status": BLOCKED,
            "stop": True,
            "objective_complete": False,
            "when": "any blocker finding exists, including failed latest required runtime, release, or evidence run",
        },
        "human_gate_stop": {
            "status": HUMAN_GATE,
            "stop": True,
            "objective_complete": False,
            "when": "a required task or policy decision needs explicit human approval",
        },
        "checks": checks,
        "policy_limits": {
            "required_priorities": list_of(policy.get("required_priorities")),
            "terminal_task_statuses": list_of(policy.get("terminal_task_statuses")),
            "max_open_p0": policy.get("max_open_p0"),
            "max_open_p1": policy.get("max_open_p1"),
            "allow_deferred_p0": bool(policy.get("allow_deferred_p0")),
            "require_release_frozen": bool(policy.get("require_release_frozen")),
        },
    }


def task_findings(tasks: list[dict[str, Any]], policy: dict[str, Any]) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    findings: list[dict[str, Any]] = []
    required_priorities = set(str(item) for item in list_of(policy.get("required_priorities")))
    terminal_statuses = set(str(item) for item in list_of(policy.get("terminal_task_statuses")))
    acceptable_terminal = set(str(item) for item in list_of(policy.get("acceptable_terminal_task_statuses")))
    blocker_statuses = set(str(item) for item in list_of(policy.get("blocker_task_statuses")))
    human_gate_statuses = set(str(item) for item in list_of(policy.get("human_gate_task_statuses")))
    required = [task for task in tasks if str(task.get("priority")) in required_priorities]
    open_required = [task for task in required if str(task.get("status")) not in terminal_statuses]
    deferred_p0 = [task for task in required if task.get("priority") == "P0" and task.get("status") == "DEFERRED"]
    by_priority = Counter(str(task.get("priority")) for task in tasks)
    by_status = Counter(str(task.get("status")) for task in tasks)

    for priority in sorted(required_priorities):
        max_key = f"max_open_{priority.lower()}"
        max_open = int(policy.get(max_key) if policy.get(max_key) is not None else 0)
        current_open = [task for task in open_required if str(task.get("priority")) == priority]
        if len(current_open) > max_open:
            add_finding(
                findings,
                "pending",
                "REQUIRED_TASKS_OPEN",
                f"{len(current_open)} required {priority} tasks are not terminal.",
                "scripts/codex_loop/tasks",
                "Continue task execution until each required task has closure evidence.",
            )

    for task in required:
        status = str(task.get("status"))
        task_id = str(task.get("id"))
        if status in blocker_statuses:
            add_finding(findings, "blocker", "TASK_STATUS_BLOCKED", f"{task_id} is `{status}`.", str(task.get("_path")), "Repair or redesign the task before objective stop.")
        elif status in human_gate_statuses:
            add_finding(findings, "human_gate", "TASK_NEEDS_HUMAN_GATE", f"{task_id} is `{status}`.", str(task.get("_path")), "Resolve the human gate before objective stop.")
        elif status in acceptable_terminal and status not in terminal_statuses:
            add_finding(findings, "warning", "TASK_TERMINAL_BUT_NOT_CLOSED", f"{task_id} is `{status}`, not CLOSED.", str(task.get("_path")), "Confirm this deferral is acceptable for the objective.")

    if deferred_p0 and not bool(policy.get("allow_deferred_p0")):
        add_finding(
            findings,
            "human_gate",
            "P0_DEFERRED_REQUIRES_HUMAN_GATE",
            f"{len(deferred_p0)} P0 tasks are deferred.",
            "scripts/codex_loop/tasks",
            "A human owner must explicitly accept P0 deferral before objective stop.",
        )

    return findings, {
        "total": len(tasks),
        "required": len(required),
        "open_required": len(open_required),
        "by_priority": dict(by_priority),
        "by_status": dict(by_status),
        "open_required_tasks": [
            {"id": task.get("id"), "priority": task.get("priority"), "status": task.get("status"), "path": task.get("_path")}
            for task in open_required
        ],
    }


def guidance_findings(guidance: dict[str, Any], tasks: list[dict[str, Any]], policy: dict[str, Any]) -> list[dict[str, Any]]:
    findings: list[dict[str, Any]] = []
    task_by_id = {str(task.get("id")): task for task in tasks}
    terminal_statuses = set(str(item) for item in list_of(policy.get("terminal_task_statuses")))
    blockers = [item for item in guidance.get("findings", []) if item.get("level") == "blocker"]
    global_blockers = []
    open_task_blockers = []
    terminal_task_blockers = []
    for item in blockers:
        target = str(item.get("target") or "")
        task = task_by_id.get(target)
        if not task:
            global_blockers.append(item)
            continue
        if str(task.get("status")) in terminal_statuses:
            terminal_task_blockers.append(item)
        else:
            open_task_blockers.append(item)
    if global_blockers:
        add_finding(
            findings,
            "blocker",
            "GUIDANCE_HAS_GLOBAL_BLOCKERS",
            f"Guidance contains {len(global_blockers)} global blocker findings.",
            "guidance/guidance.json",
            "Resolve guidance blockers before objective stop.",
        )
    if terminal_task_blockers:
        add_finding(
            findings,
            "blocker",
            "GUIDANCE_BLOCKS_TERMINAL_TASK",
            f"Guidance contradicts {len(terminal_task_blockers)} terminal tasks.",
            "guidance/guidance.json",
            "Reopen or repair the affected task before objective stop.",
        )
    if open_task_blockers:
        add_finding(
            findings,
            "pending",
            "GUIDANCE_BLOCKS_OPEN_TASK",
            f"Guidance contains {len(open_task_blockers)} blocker findings on open tasks.",
            "guidance/guidance.json",
            "Continue the loop on the affected task; do not close it until the guidance blocker is resolved.",
        )
    return findings


def evidence_findings(evidence: dict[str, Any], release: dict[str, Any], policy: dict[str, Any]) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    findings: list[dict[str, Any]] = []
    latest = latest_runs_by_kind(evidence)
    blocker_kinds = set(str(item) for item in list_of(policy.get("latest_blocker_run_kinds")))
    blocker_fragments = [str(item) for item in list_of(policy.get("blocker_status_fragments"))]
    ignore_run_fragments = [str(item) for item in list_of(policy.get("ignore_blocker_run_id_fragments"))]
    required_kinds = set(str(item) for item in list_of(policy.get("ready_required_run_kinds")))
    missing_required_kinds = sorted(kind for kind in required_kinds if kind not in latest)

    if evidence.get("missing"):
        add_finding(
            findings,
            "blocker",
            "CONTEXT_EVIDENCE_MISSING",
            f"Context evidence is missing: {evidence.get('path')}.",
            str(evidence.get("path")),
            "Regenerate context scout evidence before objective stop.",
        )
    elif evidence.get("parse_error"):
        add_finding(
            findings,
            "blocker",
            "CONTEXT_EVIDENCE_PARSE_FAILED",
            f"Context evidence could not be parsed: {evidence.get('parse_error')}.",
            str(evidence.get("path")),
            "Repair or regenerate context scout evidence before objective stop.",
        )

    if release.get("missing"):
        add_finding(
            findings,
            "blocker",
            "RELEASE_EVIDENCE_MISSING",
            f"Release evidence is missing: {release.get('path')}.",
            str(release.get("path")),
            "Regenerate release evidence before objective stop.",
        )
    elif release.get("parse_error"):
        add_finding(
            findings,
            "blocker",
            "RELEASE_EVIDENCE_PARSE_FAILED",
            f"Release evidence could not be parsed: {release.get('parse_error')}.",
            str(release.get("path")),
            "Repair or regenerate release evidence before objective stop.",
        )

    for kind in sorted(blocker_kinds):
        run = latest.get(kind)
        if not run:
            continue
        status = str(run.get("status") or "")
        run_id = str(run.get("run_id") or "")
        if any(fragment and fragment in run_id for fragment in ignore_run_fragments):
            continue
        if any(fragment in status for fragment in blocker_fragments):
            add_finding(
                findings,
                "blocker",
                "LATEST_REQUIRED_RUN_BLOCKED",
                f"Latest `{kind}` run `{run_id}` has status `{status}`.",
                str(run.get("path") or run.get("run_id")),
                "Repair this run kind before objective stop.",
            )

    if missing_required_kinds:
        add_finding(
            findings,
            "pending",
            "READY_REQUIRED_RUN_KIND_MISSING",
            f"Missing required run kinds for objective stop: {', '.join(missing_required_kinds)}.",
            "context/evidence-ledger.json",
            "Generate the required evidence run kinds before objective stop.",
        )

    release_status = str((release or latest.get("release_freeze") or {}).get("status") or "")
    if bool(policy.get("require_release_frozen")):
        if not release_status:
            add_finding(findings, "pending", "RELEASE_FREEZE_MISSING", "No release freeze evidence was provided or found.", "release/release-manifest.json", "Freeze a release manifest before objective stop.")
        elif release_status != "RELEASE_FROZEN":
            add_finding(findings, "blocker", "RELEASE_NOT_FROZEN", f"Release status is `{release_status}`.", "release/release-manifest.json", "Resolve release blockers and regenerate release evidence.")

    return findings, {
        "latest_by_kind": {
            kind: {"run_id": run.get("run_id"), "status": run.get("status"), "path": run.get("path")}
            for kind, run in sorted(latest.items())
        },
        "missing_required_kinds": missing_required_kinds,
        "release_status": release_status or None,
    }


def render_report(summary: dict[str, Any]) -> str:
    tasks = summary.get("tasks") or {}
    stop_conditions = summary.get("stop_conditions") or {}
    lines = [
        "# Codex Loop Objective Stop",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- recommendation: `{summary['stop_recommendation']}`",
        f"- objective: `{summary['objective']}`",
        f"- required_tasks: `{tasks.get('required')}`",
        f"- open_required_tasks: `{tasks.get('open_required')}`",
        f"- release_status: `{(summary.get('evidence') or {}).get('release_status')}`",
        "",
        "## Findings",
    ]
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}` `{item['evidence']}`: {item['message']} Recommendation: {item['recommendation']}")
    else:
        lines.append("- none")
    lines.extend(["", "## Open Required Tasks"])
    for task in tasks.get("open_required_tasks") or []:
        lines.append(f"- `{task['id']}` `{task['priority']}` `{task['status']}`")
    if not tasks.get("open_required_tasks"):
        lines.append("- none")
    lines.extend(["", "## Stop Conditions"])
    for key in ["success_stop", "continue_loop", "repair_stop", "human_gate_stop"]:
        condition = stop_conditions.get(key) or {}
        lines.append(
            f"- `{condition.get('status')}` stop=`{condition.get('stop')}` complete=`{condition.get('objective_complete')}`: {condition.get('when')}"
        )
    lines.extend(["", "## Condition Checks"])
    for check in stop_conditions.get("checks") or []:
        lines.append(f"- `{check['name']}` passed=`{check['passed']}` actual=`{check['actual']}` expected=`{check['expected']}`")
    lines.extend(
        [
            "",
            "## Rule",
            "- READY is the only success stop condition.",
            "- CONTINUE means the loop should keep working within normal budgets.",
            "- BLOCKED and HUMAN_GATE are stop conditions for repair or human decision, not success.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--objective", default=None, help="Human-readable objective text to record in stop evidence.")
    parser.add_argument("--tasks-dir", default="scripts/codex_loop/tasks")
    parser.add_argument("--context-dir", default=None)
    parser.add_argument("--context-run-id", default=None, help="Use doc/02_acceptance/runs/<id>/context as context-dir.")
    parser.add_argument("--guidance", default=None)
    parser.add_argument("--guidance-run-id", default=None, help="Use doc/02_acceptance/runs/<id>/guidance/guidance.json as guidance.")
    parser.add_argument("--release", default=None)
    parser.add_argument("--release-run-id", default=None, help="Use doc/02_acceptance/runs/<id>/release/release-manifest.json as release evidence.")
    parser.add_argument("--policy", default=DEFAULT_POLICY)
    parser.add_argument("--no-write", action="store_true", help="Evaluate and print the result without writing evidence files.")
    parser.add_argument("--fail-on-blocker", action="store_true")
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("objective-stop")
    run_dir = RUNS_ROOT / run_id if args.no_write else ensure_run_dir(run_id)
    out_dir = run_dir / "objective-stop"
    if not args.no_write:
        out_dir.mkdir(parents=True, exist_ok=True)
    policy_doc = load_yaml_subset(args.policy)
    policy = policy_doc.get("objective") or policy_doc
    context_dir = None
    if args.context_run_id:
        context_dir = RUNS_ROOT / args.context_run_id / "context"
    elif args.context_dir:
        context_dir = repo_path(args.context_dir)
    else:
        context_dir = latest_context_dir()
    guidance_path = None
    if args.guidance_run_id:
        guidance_path = RUNS_ROOT / args.guidance_run_id / "guidance" / "guidance.json"
    elif args.guidance:
        guidance_path = repo_path(args.guidance)
    release_path = None
    if args.release_run_id:
        release_path = RUNS_ROOT / args.release_run_id / "release" / "release-manifest.json"
    elif args.release:
        release_path = repo_path(args.release)
    evidence = load_json(context_dir / "evidence-ledger.json") if context_dir else {}
    guidance = load_json(guidance_path) if guidance_path else {}
    release = load_json(release_path) if release_path else {}
    tasks = load_tasks(repo_path(args.tasks_dir))

    findings: list[dict[str, Any]] = []
    task_results, task_summary = task_findings(tasks, policy)
    evidence_results, evidence_summary = evidence_findings(evidence, release, policy)
    findings.extend(task_results)
    findings.extend(guidance_findings(guidance, tasks, policy))
    findings.extend(evidence_results)
    status = status_for(findings)
    outputs = [] if args.no_write else [
        "objective-stop/stop-summary.json",
        "objective-stop/stop-report.md",
        "objective-stop/stop-policy.json",
    ]
    objective_text = args.objective or policy.get("description") or policy.get("name") or "complete_project_development"
    summary = {
        "run_id": run_id,
        "run_kind": "objective_stop",
        "status": status,
        "stop_recommendation": stop_recommendation(status),
        "objective": objective_text,
        "objective_policy_name": policy.get("name") or "complete_project_development",
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "policy_path": rel_path(repo_path(args.policy)),
        "context_dir": rel_path(context_dir) if context_dir else None,
        "guidance_path": rel_path(guidance_path) if guidance_path else None,
        "release_path": rel_path(release_path) if release_path else None,
        "tasks": task_summary,
        "evidence": evidence_summary,
        "findings": findings,
        "stop_conditions": build_stop_conditions(status, findings, task_summary, evidence_summary, policy),
        "outputs": outputs,
    }
    if not args.no_write:
        write_json(out_dir / "stop-policy.json", policy)
        write_json(out_dir / "stop-summary.json", summary)
        write_text(out_dir / "stop-report.md", render_report(summary))
        write_json(run_dir / "run-summary.json", summary)
    print(out_dir if not args.no_write else "no-write")
    print(f"status={status} recommendation={summary['stop_recommendation']} findings={len(findings)}")
    return 2 if args.fail_on_blocker and status in {BLOCKED, HUMAN_GATE} else 0


if __name__ == "__main__":
    raise SystemExit(main())
