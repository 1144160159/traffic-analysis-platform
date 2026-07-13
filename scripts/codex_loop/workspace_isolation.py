#!/usr/bin/env python3
"""Plan optional per-task workspace isolation for Codex Loop execution pools."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import SCRIPT_ROOT, ensure_run_dir, list_of, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text


DEFAULT_POLICY = SCRIPT_ROOT / "policies" / "workspace-isolation.yaml"
TRUE_VALUES = {"1", "true", "yes", "allow", "allowed"}
SUPPORTED_BACKENDS = {"git-worktree", "local-clone"}


def finding(level: str, code: str, message: str) -> dict[str, str]:
    return {"level": level, "code": code, "message": message}


def safe_slug(value: str) -> str:
    text = re.sub(r"[^a-zA-Z0-9]+", "-", str(value).lower()).strip("-")
    return text or "task"


def load_json(path: str | Path | None) -> dict[str, Any]:
    if not path:
        return {}
    target = repo_path(path)
    if not target.exists():
        return {}
    return json.loads(target.read_text(encoding="utf-8"))


def load_policy(path: str | Path | None = None) -> dict[str, Any]:
    target = repo_path(path) if path else DEFAULT_POLICY
    policy = load_yaml_subset(target)
    policy.setdefault("enabled", True)
    policy.setdefault("mode", "worktree-plan")
    policy.setdefault("backend", "git-worktree")
    policy.setdefault("root", "doc/02_acceptance/runs/.loop/worktrees")
    policy.setdefault("allow_create_env", "CODEX_LOOP_ALLOW_WORKTREE_CREATE")
    policy.setdefault("allow_cleanup_env", "CODEX_LOOP_ALLOW_WORKTREE_CLEANUP")
    policy.setdefault("max_parallel_workspaces", 4)
    policy.setdefault("require_task_path", True)
    policy.setdefault("require_allowed_paths", True)
    policy.setdefault("allow_dirty_source_worktree", False)
    policy.setdefault("allowed_roots", ["doc/02_acceptance/runs/.loop/worktrees"])
    policy["_path"] = rel_path(target)
    return policy


def normalize_backend(value: str | None) -> str:
    backend = str(value or "git-worktree")
    if backend not in SUPPORTED_BACKENDS:
        raise ValueError(f"Unsupported workspace backend `{backend}`. Expected one of: {', '.join(sorted(SUPPORTED_BACKENDS))}.")
    return backend


def env_gate(name: str) -> dict[str, Any]:
    value = os.environ.get(name, "")
    accepted = value.strip().lower() in TRUE_VALUES
    return {"name": name, "present": name in os.environ, "accepted": accepted, "value": "[redacted]" if name in os.environ else None}


def git_status_dirty() -> bool:
    return bool(run_git(["status", "--short"]).strip())


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


def path_under_allowed_roots(path: Path, policy: dict[str, Any]) -> bool:
    target = path.resolve()
    for raw in list_of(policy.get("allowed_roots")):
        root = repo_path(str(raw)).resolve()
        if target == root or root in target.parents:
            return True
    return False


def item_task_id(item: dict[str, Any]) -> str:
    return str(item.get("id") or item.get("task_id") or "unknown-task")


def selected_from_scheduler_plan(path: str | Path) -> list[dict[str, Any]]:
    plan = load_json(path)
    return list(plan.get("queue", {}).get("selected", []))


def normalize_item(item: dict[str, Any]) -> dict[str, Any]:
    normalized = dict(item)
    normalized["id"] = item_task_id(item)
    normalized["task_path"] = item.get("path") or item.get("task_path")
    normalized["allowed_paths"] = list_of(item.get("allowed_paths") or ((item.get("workspace") or {}).get("allowed_paths")))
    return normalized


def validate_items(items: list[dict[str, Any]], policy: dict[str, Any]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    max_parallel = int(policy.get("max_parallel_workspaces") or 1)
    if len(items) > max_parallel:
        findings.append(finding("blocker", "WORKSPACE_ISOLATION_POOL_TOO_LARGE", f"Selected {len(items)} tasks; policy allows {max_parallel} parallel workspaces."))
    if policy.get("allow_dirty_source_worktree") is not True and git_status_dirty():
        findings.append(finding("warning", "SOURCE_WORKTREE_DIRTY", "Source worktree is dirty; generated workspaces are based on HEAD and will not include uncommitted changes."))
    for item in items:
        task_id = item_task_id(item)
        if policy.get("require_task_path") is True and not item.get("task_path"):
            findings.append(finding("blocker", "TASK_PATH_MISSING", f"Task `{task_id}` has no task path."))
        if policy.get("require_allowed_paths") is True and not item.get("allowed_paths"):
            findings.append(finding("warning", "ALLOWED_PATHS_MISSING", f"Task `{task_id}` has no allowed_paths; isolation cannot narrow review scope."))
    return findings


def create_commands_for_backend(backend: str, path: Path, commit: str) -> list[list[str]]:
    if backend == "local-clone":
        return [
            ["git", "clone", "--local", "--no-hardlinks", "--no-checkout", str(repo_path(".")), str(path)],
            ["git", "-C", str(path), "checkout", "--detach", commit],
        ]
    return [["git", "worktree", "add", "--detach", str(path), commit]]


def build_workspace_specs(run_id: str, items: list[dict[str, Any]], policy: dict[str, Any], backend: str) -> list[dict[str, Any]]:
    root = repo_path(str(policy.get("root")))
    commit = run_git(["rev-parse", "HEAD"]).strip()
    specs: list[dict[str, Any]] = []
    for item in items:
        task_id = item_task_id(item)
        slug = safe_slug(task_id)
        path = root / run_id / slug
        commands = create_commands_for_backend(backend, path, commit)
        specs.append(
            {
                "task_id": task_id,
                "backend": backend,
                "workspace_path": rel_path(path),
                "absolute_workspace_path": str(path),
                "base_commit": commit,
                "task_path": item.get("task_path"),
                "allowed_paths": item.get("allowed_paths") or [],
                "create_command": commands[0],
                "create_commands": commands,
                "exists": path.exists(),
                "path_allowed": path_under_allowed_roots(path, policy),
            }
        )
    return specs


def create_workspaces(specs: list[dict[str, Any]], gate: dict[str, Any]) -> list[dict[str, Any]]:
    results: list[dict[str, Any]] = []
    if not gate.get("accepted"):
        return results
    for spec in specs:
        path = repo_path(spec["workspace_path"])
        if path.exists():
            results.append({"task_id": spec["task_id"], "created": False, "reason": "already exists", "workspace_path": spec["workspace_path"]})
            continue
        path.parent.mkdir(parents=True, exist_ok=True)
        command_results = []
        ok = True
        for command in spec.get("create_commands") or [spec["create_command"]]:
            result = run_command([str(item) for item in command])
            command_results.append(result)
            if result.get("exit_code") != 0:
                ok = False
                break
        results.append({"task_id": spec["task_id"], "backend": spec.get("backend"), "created": ok, "workspace_path": spec["workspace_path"], "results": command_results, "result": command_results[-1] if command_results else None})
    return results


def status_for(findings: list[dict[str, str]], create_requested: bool, create_results: list[dict[str, Any]]) -> str:
    if any(item.get("level") == "blocker" for item in findings):
        return "WORKSPACE_ISOLATION_BLOCKED"
    if create_requested:
        if create_results and all(item.get("created") or item.get("reason") == "already exists" for item in create_results):
            return "WORKSPACE_ISOLATION_READY"
        return "WORKSPACE_ISOLATION_BLOCKED"
    if findings:
        return "WORKSPACE_ISOLATION_DEGRADED"
    return "WORKSPACE_ISOLATION_PLANNED"


def build_isolation(
    run_id: str,
    raw_items: list[dict[str, Any]],
    policy_path: str | Path | None = None,
    create_worktree: bool = False,
    workspace_backend: str | None = None,
) -> dict[str, Any]:
    policy = load_policy(policy_path)
    backend = normalize_backend(workspace_backend or str(policy.get("backend") or "git-worktree"))
    items = [normalize_item(item) for item in raw_items]
    findings = validate_items(items, policy)
    specs = build_workspace_specs(run_id, items, policy, backend)
    for spec in specs:
        if not spec.get("path_allowed"):
            findings.append(finding("blocker", "WORKSPACE_PATH_OUTSIDE_ALLOWED_ROOT", f"Workspace path `{spec['workspace_path']}` is outside allowed roots."))
    create_gate = env_gate(str(policy.get("allow_create_env") or "CODEX_LOOP_ALLOW_WORKTREE_CREATE"))
    if create_worktree and not create_gate.get("accepted"):
        findings.append(finding("blocker", "WORKSPACE_CREATE_GATE_NOT_SET", f"Set {create_gate['name']}=1 to create isolated workspaces."))
    create_results = create_workspaces(specs, create_gate) if create_worktree and not any(item.get("level") == "blocker" for item in findings) else []
    for spec in specs:
        spec["exists_after"] = repo_path(spec["workspace_path"]).exists()
    return {
        "run_kind": "workspace_isolation",
        "status": status_for(findings, create_worktree, create_results),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "policy_path": policy.get("_path"),
        "mode": f"{backend}-create" if create_worktree else f"{backend}-plan",
        "workspace_backend": backend,
        "create_requested": create_worktree,
        "create_gate": create_gate,
        "source_dirty": git_status_dirty(),
        "workspace_root": rel_path(repo_path(str(policy.get("root")))),
        "workspaces": specs,
        "create_results": create_results,
        "findings": findings,
    }


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        "# Codex Loop Workspace Isolation",
        "",
        f"- run_id: `{summary['run_id']}`",
        f"- status: `{summary['status']}`",
        f"- mode: `{summary['mode']}`",
        f"- workspace_backend: `{summary.get('workspace_backend', 'git-worktree')}`",
        f"- workspace_root: `{summary['workspace_root']}`",
        f"- workspaces: `{len(summary.get('workspaces') or [])}`",
        f"- source_dirty: `{summary.get('source_dirty')}`",
        "",
        "## Workspaces",
    ]
    for item in summary.get("workspaces") or []:
        lines.append(f"- `{item['task_id']}` -> `{item['workspace_path']}` backend `{item.get('backend', 'git-worktree')}` base `{item['base_commit']}`")
    if not summary.get("workspaces"):
        lines.append("- none")
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
            "- Default mode only plans per-task workspaces; it does not create or delete workspaces.",
            "- Workspace creation requires the explicit CODEX_LOOP_ALLOW_WORKTREE_CREATE gate.",
            "- git-worktree writes source .git metadata; local-clone writes only under the workspace root.",
            "- Workspaces are based on HEAD and do not include uncommitted source changes.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--scheduler-plan", required=True)
    parser.add_argument("--policy", default=str(DEFAULT_POLICY))
    parser.add_argument("--max-items", type=int, default=4)
    parser.add_argument("--create-worktrees", action="store_true")
    parser.add_argument("--workspace-backend", choices=sorted(SUPPORTED_BACKENDS), default=None)
    args = parser.parse_args()

    run_id = args.run_id or make_run_id("workspace-isolation")
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "workspace-isolation"
    out_dir.mkdir(parents=True, exist_ok=True)
    raw_items = selected_from_scheduler_plan(args.scheduler_plan)[: args.max_items]
    summary = {
        "run_id": run_id,
        "scheduler_plan": rel_path(repo_path(args.scheduler_plan)),
        **build_isolation(run_id, raw_items, args.policy, args.create_worktrees, args.workspace_backend),
        "outputs": ["workspace-isolation/isolation-plan.json", "workspace-isolation/isolation-report.md"],
    }
    write_json(out_dir / "isolation-plan.json", summary)
    write_text(out_dir / "isolation-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={summary['status']} workspaces={len(summary.get('workspaces') or [])} findings={len(summary.get('findings') or [])}")
    return 2 if summary["status"] == "WORKSPACE_ISOLATION_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
