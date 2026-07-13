#!/usr/bin/env python3
"""Prepare or execute an external Codex implementation command.

The adapter is intentionally opt-in. By default it writes an invocation plan
that points to the patch-runner request. It executes an external command only
when --execute and --command are both provided.
"""

from __future__ import annotations

import argparse
import json
import shlex
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import copy_task_snapshot, ensure_run_dir, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def render_plan(task: dict[str, Any], request_path: Path, command: str | None, execute: bool) -> str:
    lines = [
        f"# Codex Adapter Invocation: {task.get('id')}",
        "",
        f"- task: {task.get('title')}",
        f"- patch_request: `{rel_path(request_path)}`",
        f"- execute: `{execute}`",
        f"- command: `{command or 'none'}`",
        "",
        "## Contract",
        "- External Codex must read the patch request.",
        "- External Codex should produce a unified diff and `codex-output.json` matching patch-runner/codex-output-contract.json.",
        "- The adapter does not trust model output; patch_runner.py must validate it before apply.",
        "",
    ]
    if command and "{prompt}" not in command:
        lines.extend(
            [
                "## Warning",
                "- Command has no `{prompt}` placeholder. The patch request path will be appended as the final argument during execution.",
                "",
            ]
        )
    return "\n".join(lines)


def build_command(command: str, request_path: Path) -> list[str]:
    request = str(request_path)
    if "{prompt}" in command:
        command = command.replace("{prompt}", shlex.quote(request))
        return ["bash", "-lc", command]
    return ["bash", "-lc", f"{command} {shlex.quote(request)}"]


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--patch-request", default=None)
    parser.add_argument("--command", default=None, help="External command. Use {prompt} for the patch request path.")
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--timeout", type=int, default=1800)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "codex-adapter")))
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "codex-adapter"
    out_dir.mkdir(parents=True, exist_ok=True)
    copy_task_snapshot(task_path, run_dir)

    request_path = repo_path(args.patch_request) if args.patch_request else run_dir / "patch-runner" / "patch-request.md"
    if not request_path.exists():
        raise FileNotFoundError(f"Patch request not found: {request_path}")

    write_text(out_dir / "invocation-plan.md", render_plan(task, request_path, args.command, args.execute))
    invocation: dict[str, Any] = {
        "run_id": run_id,
        "run_kind": "codex_adapter",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "patch_request": rel_path(request_path),
        "execute": args.execute,
        "command": args.command,
        "status": "CODEX_INVOCATION_PLANNED",
        "outputs": ["codex-adapter/invocation-plan.md", "codex-adapter/invocation.json", "codex-adapter/stdout.txt", "codex-adapter/stderr.txt"],
    }

    if args.execute:
        if not args.command:
            invocation["status"] = "CODEX_INVOCATION_BLOCKED"
            invocation["error"] = "--execute requires --command"
        else:
            proc = subprocess.run(
                build_command(args.command, request_path),
                cwd=repo_path("."),
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                timeout=args.timeout,
                check=False,
            )
            write_text(out_dir / "stdout.txt", proc.stdout)
            write_text(out_dir / "stderr.txt", proc.stderr)
            invocation["exit_code"] = proc.returncode
            invocation["status"] = "CODEX_INVOCATION_SUCCEEDED" if proc.returncode == 0 else "CODEX_INVOCATION_FAILED"
    else:
        write_text(out_dir / "stdout.txt", "")
        write_text(out_dir / "stderr.txt", "")

    write_json(out_dir / "invocation.json", invocation)
    write_json(run_dir / "run-summary.json", invocation)
    print(out_dir)
    print(f"status={invocation['status']}")
    return 2 if invocation["status"] in {"CODEX_INVOCATION_BLOCKED", "CODEX_INVOCATION_FAILED"} else 0


if __name__ == "__main__":
    raise SystemExit(main())
