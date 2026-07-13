#!/usr/bin/env python3
"""Build a bounded task context pack for long-running Codex Loop work."""

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


PACK_OUTPUTS = [
    "task-context-pack.md",
    "task-context-pack.json",
    "context-budget.json",
    "decision-log.jsonl",
    "handoff.md",
]


def load_json(path: Path | None) -> dict[str, Any]:
    if not path:
        return {}
    if not path.exists():
        raise FileNotFoundError(path)
    return json.loads(path.read_text(encoding="utf-8"))


def load_context(context_dir: Path | None) -> dict[str, Any]:
    if not context_dir:
        return {}
    files = {
        "context": "context.snapshot.json",
        "gaps": "gap-index.json",
        "deps": "dependency-map.json",
        "evidence": "evidence-ledger.json",
    }
    loaded: dict[str, Any] = {}
    for key, filename in files.items():
        path = context_dir / filename
        if path.exists():
            loaded[key] = load_json(path)
    return loaded


def load_design(design_dir: Path | None) -> dict[str, Any]:
    if not design_dir:
        return {}
    summary_path = design_dir / "design-summary.json"
    summary = load_json(summary_path) if summary_path.exists() else {}
    docs: dict[str, str] = {}
    for name in [
        "product-iteration.md",
        "feature-spec.md",
        "visual-correction.md",
        "architecture-evolution.md",
        "acceptance-cases.md",
        "implementation-plan.md",
    ]:
        path = design_dir / name
        if path.exists():
            docs[name] = rel_path(path)
    return {"summary": summary, "docs": docs}


def md_list(items: list[Any], fallback: str = "none") -> list[str]:
    values = [str(item) for item in items if item not in (None, "")]
    return [f"- {value}" for value in values] if values else [f"- {fallback}"]


def md_code_list(items: list[Any], fallback: str = "none") -> list[str]:
    values = [str(item) for item in items if item not in (None, "")]
    return [f"- `{value}`" for value in values] if values else [f"- {fallback}"]


def task_id(task: dict[str, Any]) -> str:
    return str(task.get("id", "UNKNOWN-TASK"))


def task_title(task: dict[str, Any]) -> str:
    return str(task.get("title", "Untitled task"))


def task_findings(task: dict[str, Any], guidance: dict[str, Any]) -> list[dict[str, Any]]:
    current = task_id(task)
    return [item for item in guidance.get("findings", []) if str(item.get("target")) == current]


def task_recommendations(task: dict[str, Any], guidance: dict[str, Any]) -> list[dict[str, Any]]:
    current = task_id(task)
    return [item for item in guidance.get("recommended_next", []) if str(item.get("id")) == current]


def task_status_suggestions(task: dict[str, Any], guidance: dict[str, Any]) -> list[dict[str, Any]]:
    current = task_id(task)
    return [item for item in guidance.get("status_suggestions", []) if str(item.get("target")) == current]


def route_signals(task: dict[str, Any], context: dict[str, Any]) -> list[dict[str, Any]]:
    title = f"{task_id(task)} {task_title(task)}".lower()
    routes = ((context.get("deps") or {}).get("routes") or {}).get("routes", [])
    if "/screen" in title or "screen" in title:
        return [route for route in routes if route.get("path") == "/screen"]
    allowed = " ".join(str(path) for path in list_of((task.get("workspace") or {}).get("allowed_paths")))
    if "web/ui" in allowed:
        return routes[:30]
    return []


def contract_signals(task: dict[str, Any], context: dict[str, Any]) -> list[dict[str, Any]]:
    deps = context.get("deps") or {}
    contract_to_tasks = deps.get("contract_to_tasks") or {}
    current = task_id(task)
    signals: list[dict[str, Any]] = []
    declared = task.get("contracts") or {}
    for contract, tasks in sorted(contract_to_tasks.items()):
        if current in tasks or declared.get(contract):
            signals.append({"contract": contract, "impacted_tasks": tasks})
    return signals


def evidence_signals(task: dict[str, Any], context: dict[str, Any]) -> list[dict[str, Any]]:
    evidence = context.get("evidence") or {}
    current = task_id(task)
    rows: list[dict[str, Any]] = []
    for run in evidence.get("runs", []):
        if run.get("task_id") == current or run.get("run_kind") in {"context_scout", "design_package", "context_pack"}:
            rows.append(
                {
                    "run_id": run.get("run_id"),
                    "run_kind": run.get("run_kind"),
                    "status": run.get("status"),
                    "evidence_type": run.get("evidence_type"),
                    "missing_core_files": run.get("missing_core_files"),
                }
            )
    return rows[-12:]


def source_refs(
    task: dict[str, Any],
    context_dir: Path | None,
    guidance_path: Path | None,
    design_dir: Path | None,
    design: dict[str, Any],
) -> list[dict[str, str]]:
    refs: list[dict[str, str]] = []
    for source in list_of(task.get("source")):
        refs.append({"kind": "task_source", "path": str(source), "why": "task provenance"})
    if context_dir:
        refs.extend(
            [
                {"kind": "context", "path": rel_path(context_dir / "context.snapshot.json"), "why": "bounded global snapshot"},
                {"kind": "context", "path": rel_path(context_dir / "dependency-map.json"), "why": "route and contract signals"},
                {"kind": "context", "path": rel_path(context_dir / "evidence-ledger.json"), "why": "prior run evidence state"},
            ]
        )
    if guidance_path:
        refs.append({"kind": "guidance", "path": rel_path(guidance_path), "why": "blockers and scheduling signal"})
    if design_dir:
        refs.append({"kind": "design", "path": rel_path(design_dir / "design-summary.json"), "why": "product and architecture strategy"})
        for name, path in sorted((design.get("docs") or {}).items()):
            refs.append({"kind": "design_doc", "path": path, "why": name})
    return refs


def compact_design_summary(design: dict[str, Any]) -> dict[str, Any]:
    summary = design.get("summary") or {}
    return {
        "status": summary.get("status"),
        "decision": summary.get("decision"),
        "recommended_strategy": summary.get("recommended_strategy"),
        "route_signal": summary.get("route_signal"),
        "blockers": summary.get("blockers", []),
        "warnings": summary.get("warnings", []),
        "outputs": summary.get("outputs", []),
    }


def build_pack_data(
    task: dict[str, Any],
    run_id: str,
    context_dir: Path | None,
    guidance_path: Path | None,
    design_dir: Path | None,
    context: dict[str, Any],
    guidance: dict[str, Any],
    design: dict[str, Any],
    max_chars: int,
) -> dict[str, Any]:
    lane = task.get("lane") or {}
    execution = task.get("execution") or {}
    workspace = task.get("workspace") or {}
    verification = task.get("verification") or {}
    context_snapshot = context.get("context") or {}
    findings = task_findings(task, guidance)
    return {
        "run_id": run_id,
        "run_kind": "context_pack",
        "task": {
            "id": task_id(task),
            "title": task_title(task),
            "priority": task.get("priority"),
            "status": task.get("status"),
            "primary_lane": lane.get("primary"),
            "dependent_lanes": list_of(lane.get("dependent")),
            "acceptance_type": task.get("acceptance_type"),
            "subsystems": list_of(task.get("subsystems")),
        },
        "budget": {
            "max_chars": max_chars,
            "policy": "Keep this pack small; fetch original files by source_refs when exact detail is needed.",
        },
        "repo_state": {
            "commit": run_git(["rev-parse", "HEAD"]).strip(),
            "branch": run_git(["branch", "--show-current"]).strip(),
            "snapshot_commit": context_snapshot.get("commit"),
            "dirty_items_seen": ((context_snapshot.get("git_status") or {}).get("total")),
        },
        "scope": {
            "execution_mode": execution.get("mode"),
            "allow_live_write": execution.get("allow_live_write"),
            "allowed_paths": list_of(workspace.get("allowed_paths")),
            "contracts": task.get("contracts") or {},
            "data_plan": task.get("data_plan") or {},
        },
        "must_keep": {
            "close_when": list_of(task.get("close_when")),
            "local_verification": list_of(verification.get("local")),
            "live_readonly_verification": list_of(verification.get("live_readonly")),
            "review_perspectives": list_of((task.get("review") or {}).get("perspectives")),
        },
        "signals": {
            "guidance_findings": findings,
            "guidance_recommendations": task_recommendations(task, guidance),
            "status_suggestions": task_status_suggestions(task, guidance),
            "route_signals": route_signals(task, context),
            "contract_signals": contract_signals(task, context),
            "evidence_signals": evidence_signals(task, context),
            "design_summary": compact_design_summary(design),
        },
        "source_refs": source_refs(task, context_dir, guidance_path, design_dir, design),
        "working_rules": [
            "Use this pack as the active context, not the whole repository history.",
            "When a source detail matters, open the referenced file instead of trusting memory.",
            "If this pack conflicts with current code, current code and fresh evidence win.",
            "Do not close a task from context-pack evidence alone.",
            "Keep smoke, regression, acceptance and third-party labels separate.",
        ],
    }


def render_pack_md(pack: dict[str, Any]) -> str:
    task = pack["task"]
    scope = pack["scope"]
    must_keep = pack["must_keep"]
    signals = pack["signals"]
    lines = [
        f"# Task Context Pack: {task['id']}",
        "",
        f"- run_id: `{pack['run_id']}`",
        f"- task: {task['title']}",
        f"- priority: `{task['priority']}`",
        f"- task_status: `{task['status']}`",
        f"- lane: `{task['primary_lane']}`",
        f"- acceptance_type: `{task['acceptance_type']}`",
        f"- budget: `{pack['budget']['max_chars']}` chars",
        "",
        "## Current Objective",
        f"- Continue only the bounded task `{task['id']}` unless a design delta explicitly expands scope.",
        "- Use original source refs for exact details; this pack is a working brief, not a source of truth.",
        "",
        "## Scope",
        f"- execution_mode: `{scope['execution_mode']}`",
        f"- allow_live_write: `{scope['allow_live_write']}`",
        "- allowed_paths:",
    ]
    lines.extend(md_code_list(scope["allowed_paths"]))
    lines.extend(["", "## Close Conditions"])
    lines.extend(md_list(must_keep["close_when"]))
    lines.extend(["", "## Verification To Preserve"])
    lines.append("- local:")
    lines.extend(md_code_list(must_keep["local_verification"]))
    lines.append("- live_readonly:")
    lines.extend(md_code_list(must_keep["live_readonly_verification"]))
    lines.extend(["", "## Guidance Findings"])
    findings = signals["guidance_findings"]
    if findings:
        for item in findings:
            lines.append(f"- `{item.get('level')}` `{item.get('code')}`: {item.get('message')} Suggestion: {item.get('suggestion')}")
    else:
        lines.append("- none")
    lines.extend(["", "## Design Signal"])
    design = signals["design_summary"]
    if design.get("recommended_strategy"):
        lines.append(f"- status: `{design.get('status')}`")
        lines.append(f"- decision: `{design.get('decision')}`")
        lines.append(f"- strategy: {design.get('recommended_strategy')}")
        if design.get("route_signal"):
            lines.append(f"- route_signal: {design.get('route_signal')}")
    else:
        lines.append("- no design package was provided")
    lines.extend(["", "## Route Signals"])
    if signals["route_signals"]:
        for route in signals["route_signals"]:
            lines.append(f"- `{route.get('path')}` line `{route.get('line')}` protected=`{route.get('protected_shell')}` note={route.get('note') or 'none'}")
    else:
        lines.append("- none")
    lines.extend(["", "## Contract Signals"])
    if signals["contract_signals"]:
        for item in signals["contract_signals"]:
            lines.append(f"- `{item['contract']}` impacts: {', '.join(item['impacted_tasks'])}")
    else:
        lines.append("- none")
    lines.extend(["", "## Evidence Signals"])
    if signals["evidence_signals"]:
        for run in signals["evidence_signals"]:
            missing = run.get("missing_core_files") or []
            lines.append(f"- `{run.get('run_id')}` [{run.get('run_kind')}/{run.get('evidence_type')}]: status `{run.get('status')}`, missing `{len(missing)}`")
    else:
        lines.append("- none")
    lines.extend(["", "## Source Refs"])
    for ref in pack["source_refs"]:
        lines.append(f"- `{ref['kind']}` `{ref['path']}`: {ref['why']}")
    lines.extend(["", "## Working Rules"])
    lines.extend(md_list(pack["working_rules"]))
    lines.append("")
    return "\n".join(lines)


def render_handoff(pack: dict[str, Any]) -> str:
    task = pack["task"]
    signals = pack["signals"]
    findings = signals["guidance_findings"]
    blockers = [item for item in findings if item.get("level") == "blocker"]
    lines = [
        f"# Handoff: {task['id']}",
        "",
        f"- Current task: {task['title']}",
        f"- Current context pack: `doc/02_acceptance/runs/{pack['run_id']}/context-pack/task-context-pack.md`",
        f"- Status to assume: `{task['status']}` with `{len(blockers)}` blocker(s) from guidance.",
        "",
        "## Resume Steps",
        "- Read this handoff and `task-context-pack.md` first.",
        "- Refresh `git status --short` before any edit.",
        "- If implementing, open exact source refs before relying on summarized text.",
        "- Preserve the declared evidence layer and close_when conditions.",
        "",
        "## Current Stop Conditions",
    ]
    if blockers:
        for item in blockers:
            lines.append(f"- `{item.get('code')}`: {item.get('message')}")
    else:
        lines.append("- none")
    lines.extend(["", "## Next Likely Action"])
    design = signals["design_summary"]
    if design.get("recommended_strategy"):
        lines.append(f"- Continue from design strategy: {design.get('recommended_strategy')}")
    else:
        lines.append("- Generate or review a design package before implementation.")
    lines.append("")
    return "\n".join(lines)


def enforce_budget(pack_md: str, max_chars: int) -> tuple[str, list[str]]:
    if len(pack_md) <= max_chars:
        return pack_md, []
    lines = pack_md.splitlines()
    keep: list[str] = []
    omitted: list[str] = []
    current_section = "preamble"
    hard_keep_sections = {
        "preamble",
        "## Current Objective",
        "## Scope",
        "## Close Conditions",
        "## Guidance Findings",
        "## Design Signal",
        "## Working Rules",
    }
    for line in lines:
        if line.startswith("## "):
            current_section = line
        candidate = "\n".join(keep + [line]) + "\n"
        if len(candidate) <= max_chars or current_section in hard_keep_sections:
            keep.append(line)
        else:
            if current_section not in omitted:
                omitted.append(current_section)
    compact = "\n".join(keep)
    if omitted:
        compact += "\n\n## Budget Omissions\n"
        compact += "\n".join(f"- {section}" for section in omitted)
        compact += "\n"
    return compact, omitted


def build_decision_log(pack: dict[str, Any], omitted_sections: list[str]) -> str:
    now = datetime.now().isoformat(timespec="seconds")
    rows = [
        {
            "ts": now,
            "event": "context_pack_created",
            "task_id": pack["task"]["id"],
            "run_id": pack["run_id"],
            "max_chars": pack["budget"]["max_chars"],
        },
        {
            "ts": now,
            "event": "active_scope_selected",
            "allowed_paths": pack["scope"]["allowed_paths"],
            "execution_mode": pack["scope"]["execution_mode"],
        },
    ]
    design = pack["signals"]["design_summary"]
    if design.get("recommended_strategy"):
        rows.append(
            {
                "ts": now,
                "event": "design_strategy_carried_forward",
                "status": design.get("status"),
                "strategy": design.get("recommended_strategy"),
            }
        )
    if omitted_sections:
        rows.append({"ts": now, "event": "budget_omitted_sections", "sections": omitted_sections})
    return "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows)


def build_budget(pack_md: str, pack_json: dict[str, Any], max_chars: int, omitted_sections: list[str]) -> dict[str, Any]:
    return {
        "max_chars": max_chars,
        "actual_chars": len(pack_md),
        "actual_lines": len(pack_md.splitlines()),
        "source_ref_count": len(pack_json.get("source_refs", [])),
        "omitted_sections": omitted_sections,
        "policy": "If a section is omitted, fetch it from source_refs or the original context/design files.",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True, help="Task YAML path.")
    parser.add_argument("--context-dir", default=None, help="Context Scout output directory.")
    parser.add_argument("--guidance", default=None, help="guidance.json path.")
    parser.add_argument("--design-dir", default=None, help="design.py output directory.")
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--out-dir", default=None)
    parser.add_argument("--max-chars", type=int, default=12000)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "context-pack")))
    run_dir = ensure_run_dir(run_id)
    out_dir = repo_path(args.out_dir) if args.out_dir else run_dir / "context-pack"
    out_dir.mkdir(parents=True, exist_ok=True)

    context_dir = repo_path(args.context_dir) if args.context_dir else None
    guidance_path = repo_path(args.guidance) if args.guidance else None
    design_dir = repo_path(args.design_dir) if args.design_dir else None

    context = load_context(context_dir)
    guidance = load_json(guidance_path)
    design = load_design(design_dir)
    copy_task_snapshot(task_path, run_dir)

    pack = build_pack_data(task, run_id, context_dir, guidance_path, design_dir, context, guidance, design, args.max_chars)
    pack_md, omitted_sections = enforce_budget(render_pack_md(pack), args.max_chars)
    budget = build_budget(pack_md, pack, args.max_chars, omitted_sections)

    write_text(out_dir / "task-context-pack.md", pack_md)
    write_json(out_dir / "task-context-pack.json", pack)
    write_json(out_dir / "context-budget.json", budget)
    write_text(out_dir / "decision-log.jsonl", build_decision_log(pack, omitted_sections))
    write_text(out_dir / "handoff.md", render_handoff(pack))

    summary = {
        "run_id": run_id,
        "run_kind": "context_pack",
        "task_id": task_id(task),
        "task_title": task_title(task),
        "status": "CONTEXT_PACKED",
        "evidence_type": "acceptance-prep",
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "context_dir": rel_path(context_dir) if context_dir else None,
        "guidance_path": rel_path(guidance_path) if guidance_path else None,
        "design_dir": rel_path(design_dir) if design_dir else None,
        "context_pack_dir": rel_path(out_dir),
        "budget": budget,
        "outputs": [f"context-pack/{name}" for name in PACK_OUTPUTS],
        "warning": "Context pack is a bounded working brief; it is not implementation, regression, acceptance or third-party evidence.",
    }
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status=CONTEXT_PACKED chars={budget['actual_chars']}/{args.max_chars} sources={budget['source_ref_count']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
