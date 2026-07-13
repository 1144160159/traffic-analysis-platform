#!/usr/bin/env python3
"""Guarded task runner for the Codex Loop MVP.

By default this script only writes a plan and evidence skeleton. It executes
local verification commands only when --execute-local is supplied.
"""

from __future__ import annotations

import argparse
import subprocess

from lib import copy_task_snapshot, ensure_run_dir, list_of, load_yaml_subset, make_run_id, repo_path, write_text
from plan import render_plan


DENY_SUBSTRINGS = [
    "git reset",
    "git checkout",
    "kubectl delete",
    "helm uninstall",
    "rm -rf",
    "docker push",
    "truncate ",
    "DROP DATABASE",
    "DROP TABLE",
]


def command_allowed(command: str) -> tuple[bool, str]:
    lowered = command.lower()
    for item in DENY_SUBSTRINGS:
        if item.lower() in lowered:
            return False, f"denied command fragment: {item}"
    return True, "ok"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--mode", default=None)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--execute-local", action="store_true")
    args = parser.parse_args()

    task = load_yaml_subset(args.task)
    run_id = args.run_id or make_run_id(str(task.get("id", "task")))
    run_dir = ensure_run_dir(run_id)
    copy_task_snapshot(args.task, run_dir)
    write_text(run_dir / "plan.md", render_plan(task, run_id))

    requested_mode = args.mode or (task.get("execution") or {}).get("mode")
    declared_mode = (task.get("execution") or {}).get("mode")
    if requested_mode != declared_mode:
        write_text(
            run_dir / "gate-request.md",
            f"# Gate Request\n\nRequested mode `{requested_mode}` does not match task mode `{declared_mode}`.\n",
        )
        print(run_dir / "gate-request.md")
        return 2

    commands = list_of((task.get("verification") or {}).get("local"))
    if not args.execute_local:
        write_text(
            run_dir / "local-report.md",
            "# Local Report\n\nNot executed. Re-run with `--execute-local` after reviewing `plan.md`.\n",
        )
        print(run_dir)
        return 0

    report = ["# Local Report", ""]
    for command in commands:
        allowed, reason = command_allowed(command)
        report.append(f"## `{command}`")
        if not allowed:
            report.append(f"blocked: {reason}")
            write_text(run_dir / "local-report.md", "\n".join(report) + "\n")
            print(run_dir / "local-report.md")
            return 3
        proc = subprocess.run(
            command,
            cwd=repo_path("."),
            shell=True,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        )
        report.append(f"exit_code: {proc.returncode}")
        report.append("")
        report.append("```text")
        report.append(proc.stdout[-12000:])
        report.append("```")
        report.append("")
        if proc.returncode != 0:
            write_text(run_dir / "local-report.md", "\n".join(report) + "\n")
            print(run_dir / "local-report.md")
            return proc.returncode
    write_text(run_dir / "local-report.md", "\n".join(report) + "\n")
    print(run_dir)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
