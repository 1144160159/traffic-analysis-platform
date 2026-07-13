#!/usr/bin/env python3
"""Render a guarded execution plan for one Codex Loop task."""

from __future__ import annotations

import argparse
from pathlib import Path
from typing import Any

from lib import (
    bool_contract,
    copy_task_snapshot,
    ensure_run_dir,
    list_of,
    load_yaml_subset,
    make_run_id,
    repo_path,
    write_text,
)


def required_gates(task: dict[str, Any]) -> list[str]:
    gates = [
        "G0 Intake: agent.md, doc inputs and git status must be captured",
        "G1 Scope: all changes must stay inside workspace.allowed_paths",
    ]
    if any(bool_contract(task, key) for key in ["proto", "kafka_topics", "database_schema", "apisix_routes"]):
        gates.append("G2 Contract: changed contracts require producer/consumer/deploy checks")
    risk_text = " ".join(list_of((task.get("risk") or {}).get("reasons"))).lower()
    if (task.get("risk") or {}).get("level") == "high" or any(word in risk_text for word in ["auth", "tenant", "security", "screen", "pcap"]):
        gates.append("G3 Security: auth, tenant, audit and secret boundaries must be explicit")
    data_plan = task.get("data_plan") or {}
    if data_plan.get("mode") == "live_generated":
        gates.append("G4 Data: live writes require run_id, tenant and cleanup")
    gates.extend(
        [
            "G5 Local: smallest relevant local verification must pass or be explained",
            "G6 Reviewer: third-view review must have no P0/P1 blockers",
        ]
    )
    verification = task.get("verification") or {}
    if verification.get("live_readonly") or verification.get("live_generated"):
        gates.append("G7 Live: live evidence must match its declared evidence layer")
    gates.append("G8 Evidence: run-summary and required reports must exist")
    if "release" in str(task.get("title", "")).lower() or "BASELINE" in str(task.get("id", "")):
        gates.append("G9 Release: release baseline must include commit, images, manifests, DDL and topics")
    return gates


def render_plan(task: dict[str, Any], run_id: str) -> str:
    lane = task.get("lane") or {}
    verification = task.get("verification") or {}
    workspace = task.get("workspace") or {}
    evidence = task.get("evidence") or {}
    lines = [
        f"# Codex Loop Plan: {task.get('id')}",
        "",
        f"- run_id: `{run_id}`",
        f"- title: {task.get('title')}",
        f"- priority: `{task.get('priority')}`",
        f"- status: `{task.get('status')}`",
        f"- primary lane: `{lane.get('primary')}`",
        f"- dependent lanes: {', '.join(list_of(lane.get('dependent'))) or 'none'}",
        f"- acceptance type: `{task.get('acceptance_type')}`",
        f"- execution mode: `{(task.get('execution') or {}).get('mode')}`",
        "",
        "## Source",
    ]
    lines.extend(f"- {source}" for source in list_of(task.get("source")))
    lines.extend(["", "## Scope"])
    lines.extend(f"- `{path}`" for path in list_of(workspace.get("allowed_paths")))
    lines.extend(["", "## Required Gates"])
    lines.extend(f"- {gate}" for gate in required_gates(task))
    lines.extend(["", "## Local Verification"])
    local_checks = list_of(verification.get("local"))
    lines.extend(f"- `{cmd}`" for cmd in local_checks) if local_checks else lines.append("- none")
    lines.extend(["", "## Live Readonly Verification"])
    live_checks = list_of(verification.get("live_readonly"))
    lines.extend(f"- `{cmd}`" for cmd in live_checks) if live_checks else lines.append("- none")
    lines.extend(["", "## Evidence"])
    lines.append(f"- run directory: `{(evidence.get('run_dir') or '').replace('${run_id}', run_id)}`")
    lines.extend(f"- required: `{item}`" for item in list_of(evidence.get("required")))
    lines.extend(["", "## Close Conditions"])
    lines.extend(f"- {item}" for item in list_of(task.get("close_when")))
    lines.extend(
        [
            "",
            "## Safety Notes",
            "- This plan does not grant permission for destructive live operations.",
            "- Smoke/regression/acceptance/third-party evidence must remain separate.",
            "- Any policy failure should produce a gate request instead of continuing execution.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True, help="Task YAML path.")
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--write", action="store_true", help="Write plan.md under doc/02_acceptance/runs/<run_id>.")
    parser.add_argument("--out", default=None, help="Optional explicit output path.")
    args = parser.parse_args()

    task = load_yaml_subset(args.task)
    run_id = args.run_id or make_run_id(str(task.get("id", "codex-loop")))
    content = render_plan(task, run_id)

    if args.write:
        run_dir = ensure_run_dir(run_id)
        copy_task_snapshot(args.task, run_dir)
        path = write_text(run_dir / "plan.md", content)
        print(path)
    elif args.out:
        path = write_text(Path(args.out), content)
        print(path)
    else:
        print(content)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
