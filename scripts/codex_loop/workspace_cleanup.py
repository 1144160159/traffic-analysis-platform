#!/usr/bin/env python3
"""Plan or execute gated cleanup of Codex Loop per-task workspaces."""

from __future__ import annotations

import argparse
import json
import shutil
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import ensure_run_dir, make_run_id, rel_path, repo_path, run_git, write_json, write_text
from workspace_isolation import DEFAULT_POLICY, env_gate, load_policy, path_under_allowed_roots


def finding(level: str, code: str, message: str) -> dict[str, str]:
    return {"level": level, "code": code, "message": message}


def load_json(path: str | Path) -> dict[str, Any]:
    target = repo_path(path)
    if not target.exists():
        return {"missing": True, "path": rel_path(target)}
    return json.loads(target.read_text(encoding="utf-8"))


def run_command(command: list[str], cwd: Path | None = None, timeout: int = 120) -> dict[str, Any]:
    proc = subprocess.run(
        command,
        cwd=cwd or repo_path("."),
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        timeout=timeout,
        check=False,
    )
    return {"command": command, "exit_code": proc.returncode, "output_tail": proc.stdout[-4000:]}


def git_worktree_paths() -> set[str]:
    paths: set[str] = set()
    current: str | None = None
    for line in run_git(["worktree", "list", "--porcelain"]).splitlines():
        if line.startswith("worktree "):
            current = line.removeprefix("worktree ").strip()
            paths.add(str(Path(current).resolve()))
    return paths


def workspace_specs(source: dict[str, Any]) -> list[dict[str, Any]]:
    return list(source.get("workspaces") or [])


def workspace_path(spec: dict[str, Any]) -> Path:
    raw = spec.get("absolute_workspace_path") or spec.get("workspace_path")
    return repo_path(str(raw))


def workspace_is_git(path: Path) -> bool:
    if not path.exists():
        return False
    result = run_command(["git", "-C", str(path), "rev-parse", "--is-inside-work-tree"])
    return result.get("exit_code") == 0 and result.get("output_tail", "").strip().endswith("true")


def workspace_dirty(path: Path, should_check: bool) -> bool:
    if not should_check or not path.exists() or not workspace_is_git(path):
        return False
    return bool(run_command(["git", "-C", str(path), "status", "--porcelain"]).get("output_tail", "").strip())


def planned_items(source: dict[str, Any], policy: dict[str, Any], force: bool) -> tuple[list[dict[str, Any]], list[dict[str, str]]]:
    registered_paths = git_worktree_paths()
    findings: list[dict[str, str]] = []
    items: list[dict[str, Any]] = []
    for spec in workspace_specs(source):
        path = workspace_path(spec)
        resolved = str(path.resolve())
        backend = str(spec.get("backend") or source.get("workspace_backend") or "git-worktree")
        registered = resolved in registered_paths
        exists = path.exists()
        allowed = path_under_allowed_roots(path, policy)
        git_workspace = workspace_is_git(path)
        dirty = workspace_dirty(path, registered or backend == "local-clone")
        cleanup_method = "git-worktree-remove" if backend == "git-worktree" else "directory-remove"
        item = {
            "task_id": spec.get("task_id"),
            "backend": backend,
            "workspace_path": rel_path(path),
            "absolute_workspace_path": resolved,
            "exists": exists,
            "registered": registered,
            "git_workspace": git_workspace,
            "path_allowed": allowed,
            "dirty": dirty,
            "cleanup_method": cleanup_method,
            "remove_command": ["git", "worktree", "remove", "--force", str(path)] if force else ["git", "worktree", "remove", str(path)],
        }
        items.append(item)
        if not allowed:
            findings.append(finding("blocker", "WORKSPACE_CLEANUP_PATH_OUTSIDE_ALLOWED_ROOT", f"Workspace path `{rel_path(path)}` is outside allowed roots."))
        if exists and backend == "local-clone" and not git_workspace:
            findings.append(finding("blocker", "WORKSPACE_CLEANUP_NOT_GIT_WORKSPACE", f"Workspace `{rel_path(path)}` is not a git workspace; refusing directory cleanup."))
        if dirty and not force:
            findings.append(finding("blocker", "WORKSPACE_CLEANUP_DIRTY_WORKTREE", f"Workspace `{rel_path(path)}` has uncommitted changes; rerun with --force only after review."))
    return items, findings


def status_for(findings: list[dict[str, str]], items: list[dict[str, Any]], execute: bool, results: list[dict[str, Any]]) -> str:
    if any(item.get("level") == "blocker" for item in findings):
        return "WORKSPACE_CLEANUP_BLOCKED"
    if not items:
        return "WORKSPACE_CLEANUP_EMPTY"
    if execute:
        if all(result.get("removed") or result.get("reason") in {"missing", "not registered"} for result in results):
            return "WORKSPACE_CLEANUP_COMPLETED"
        return "WORKSPACE_CLEANUP_BLOCKED"
    return "WORKSPACE_CLEANUP_PLANNED"


def execute_cleanup(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    results: list[dict[str, Any]] = []
    for item in items:
        if not item.get("exists"):
            results.append({"task_id": item.get("task_id"), "workspace_path": item.get("workspace_path"), "removed": False, "reason": "missing"})
            continue
        if item.get("backend") == "git-worktree" and not item.get("registered"):
            results.append({"task_id": item.get("task_id"), "workspace_path": item.get("workspace_path"), "removed": False, "reason": "not registered"})
            continue
        if item.get("backend") == "local-clone":
            try:
                shutil.rmtree(str(item["absolute_workspace_path"]))
                results.append({"task_id": item.get("task_id"), "workspace_path": item.get("workspace_path"), "removed": True, "method": item.get("cleanup_method"), "pruned_empty_parent": prune_empty_parent(item)})
            except OSError as exc:
                results.append({"task_id": item.get("task_id"), "workspace_path": item.get("workspace_path"), "removed": False, "method": item.get("cleanup_method"), "error": str(exc)})
            continue
        result = run_command([str(part) for part in item["remove_command"]])
        removed = result.get("exit_code") == 0
        results.append(
            {
                "task_id": item.get("task_id"),
                "workspace_path": item.get("workspace_path"),
                "removed": removed,
                "method": item.get("cleanup_method"),
                "pruned_empty_parent": prune_empty_parent(item) if removed else False,
                "result": result,
            }
        )
    return results


def prune_empty_parent(item: dict[str, Any]) -> bool:
    parent = Path(str(item["absolute_workspace_path"])).parent
    try:
        parent.rmdir()
        return True
    except OSError:
        return False


def build_cleanup(source_path: str | Path, policy_path: str | Path | None = None, execute: bool = False, force: bool = False) -> dict[str, Any]:
    source = load_json(source_path)
    policy = load_policy(policy_path)
    findings: list[dict[str, str]] = []
    if source.get("missing"):
        findings.append(finding("blocker", "WORKSPACE_CLEANUP_SOURCE_MISSING", f"Cleanup source is missing: {source_path}."))
    items, item_findings = planned_items(source, policy, force) if not source.get("missing") else ([], [])
    findings.extend(item_findings)
    cleanup_gate = env_gate(str(policy.get("allow_cleanup_env") or "CODEX_LOOP_ALLOW_WORKTREE_CLEANUP"))
    if execute and not cleanup_gate.get("accepted"):
        findings.append(finding("blocker", "WORKTREE_CLEANUP_GATE_NOT_SET", f"Set {cleanup_gate['name']}=1 to remove isolated workspaces."))
    results = execute_cleanup(items) if execute and not any(item.get("level") == "blocker" for item in findings) else []
    return {
        "run_kind": "workspace_cleanup",
        "status": status_for(findings, items, execute, results),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "source_path": rel_path(repo_path(source_path)),
        "policy_path": policy.get("_path"),
        "execute_requested": execute,
        "force": force,
        "cleanup_gate": cleanup_gate,
        "workspaces": items,
        "cleanup_results": results,
        "findings": findings,
    }


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Workspace Cleanup",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- source: `{summary['source_path']}`",
        f"- execute_requested: `{summary['execute_requested']}`",
        f"- force: `{summary['force']}`",
        f"- workspaces: `{len(summary.get('workspaces') or [])}`",
        "",
        "## Workspaces",
    ]
    if summary.get("workspaces"):
        for item in summary["workspaces"]:
            lines.append(f"- `{item.get('task_id')}` -> `{item.get('workspace_path')}` backend `{item.get('backend')}` exists `{item.get('exists')}` registered `{item.get('registered')}` dirty `{item.get('dirty')}`")
    else:
        lines.append("- none")
    lines.extend(["", "## Cleanup Results"])
    if summary.get("cleanup_results"):
        for item in summary["cleanup_results"]:
            lines.append(f"- `{item.get('task_id')}` -> removed `{item.get('removed')}` method `{item.get('method') or ''}` reason `{item.get('reason') or ''}`")
    else:
        lines.append("- not executed")
    lines.extend(["", "## Findings"])
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Default mode only plans cleanup; it does not remove workspaces.",
            "- Cleanup execution requires CODEX_LOOP_ALLOW_WORKTREE_CLEANUP=1.",
            "- Cleanup is limited to policy allowed roots and known workspace backends.",
            "- Dirty workspaces are blocked unless --force is explicitly provided.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--workspace-isolation", required=True)
    parser.add_argument("--policy", default=str(DEFAULT_POLICY))
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--force", action="store_true")
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("workspace-cleanup")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "workspace-cleanup"
    out_dir.mkdir(parents=True, exist_ok=True)
    summary = {
        "run_id": run_id,
        **build_cleanup(args.workspace_isolation, args.policy, args.execute, args.force),
        "outputs": ["workspace-cleanup/cleanup-plan.json", "workspace-cleanup/cleanup-report.md"],
    }
    write_json(out_dir / "cleanup-plan.json", summary)
    write_text(out_dir / "cleanup-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} workspaces={len(summary.get('workspaces') or [])} findings={len(summary.get('findings') or [])}")
    return 2 if summary["status"] == "WORKSPACE_CLEANUP_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
