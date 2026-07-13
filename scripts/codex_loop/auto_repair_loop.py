#!/usr/bin/env python3
"""Plan the next automated loop action from a repair plan."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import SCRIPT_ROOT, copy_task_snapshot, ensure_run_dir, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text


NEXT_STAGE_BY_KIND = {
    "design": "prepare",
    "implement": "prepare",
    "verify": "execute-local",
    "review": "prepare",
    "evidence": "prepare",
    "triage": "prepare",
}


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def choose_stage(repair: dict[str, Any]) -> str:
    for step in repair.get("steps", []):
        kind = str(step.get("kind"))
        if kind in NEXT_STAGE_BY_KIND:
            return NEXT_STAGE_BY_KIND[kind]
    return "prepare"


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Auto Repair Loop",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- next_stage: `{summary['next_stage']}`",
        f"- executed: `{summary['executed']}`",
        "",
        "## Rule",
        "- By default this script plans the next repair loop only.",
        "- Use --execute to run workflow.py for the selected next stage.",
        "",
    ]
    if summary.get("child_run"):
        lines.append(f"- child_run: `{summary['child_run'].get('run_id')}` exit `{summary['child_run'].get('exit_code')}`")
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--repair-plan", required=True)
    parser.add_argument("--context-dir", required=True)
    parser.add_argument("--guidance", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--execute", action="store_true")
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    repair_path = repo_path(args.repair_plan)
    repair = load_json(repair_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "auto-repair")))
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "auto-repair"
    out_dir.mkdir(parents=True, exist_ok=True)
    copy_task_snapshot(task_path, run_dir)
    next_stage = choose_stage(repair)
    child_run: dict[str, Any] | None = None
    status = "AUTO_REPAIR_PLANNED"
    if args.execute:
        child_run_id = f"{run_id}-repair"
        command = [
            sys.executable,
            "-B",
            str(SCRIPT_ROOT / "workflow.py"),
            "--task",
            str(task.get("id")),
            "--run-id",
            child_run_id,
            "--context-dir",
            rel_path(repo_path(args.context_dir)),
            "--guidance",
            rel_path(repo_path(args.guidance)),
            "--stage",
            next_stage,
        ]
        proc = subprocess.run(command, cwd=repo_path("."), text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)
        child_run = {"run_id": child_run_id, "exit_code": proc.returncode, "output_tail": proc.stdout[-4000:], "command": command}
        status = "AUTO_REPAIR_EXECUTED" if proc.returncode == 0 else "AUTO_REPAIR_FAILED"
    summary = {
        "run_id": run_id,
        "run_kind": "auto_repair_loop",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "repair_plan": rel_path(repair_path),
        "next_stage": next_stage,
        "executed": args.execute,
        "child_run": child_run,
        "outputs": ["auto-repair/auto-repair-summary.json", "auto-repair/auto-repair-report.md"],
    }
    write_json(out_dir / "auto-repair-summary.json", summary)
    write_text(out_dir / "auto-repair-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} next_stage={next_stage}")
    return 1 if status == "AUTO_REPAIR_FAILED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
