#!/usr/bin/env python3
"""Prepare and guard implementation work for one Codex Loop task.

This script is the safe implementation boundary for the loop. By default it
does not edit business code. It emits an implementation brief and validates an
optional unified-diff patch against task scope, contract declarations and
guidance blockers. A patch is applied only when --apply-patch is supplied.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import (
    REPO_ROOT,
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


CONTRACT_PATH_RULES = [
    ("proto/traffic/v1/", "proto"),
    ("common/kafka/", "kafka_topics"),
    ("common/sql/", "database_schema"),
    ("deployments/kubernetes/init-jobs/", "database_schema"),
    ("deployments/kubernetes/configmaps/apisix-routes.yaml", "apisix_routes"),
]

DENY_PATH_PARTS = {
    ".git",
    "node_modules",
    "__pycache__",
}

IMPLEMENTATION_OUTPUTS = [
    "implementation/implementation-brief.md",
    "implementation/codex-implementation-prompt.md",
    "implementation/patch-scope.json",
    "implementation/patch-validation.json",
    "implementation/apply-report.md",
]


def load_json(path: Path | None) -> dict[str, Any]:
    if not path:
        return {}
    if not path.exists():
        raise FileNotFoundError(path)
    return json.loads(path.read_text(encoding="utf-8"))


def normalize_patch_path(raw: str) -> str | None:
    path = raw.strip().split("\t", 1)[0]
    if path in {"/dev/null", "dev/null"}:
        return None
    if path.startswith("a/") or path.startswith("b/"):
        path = path[2:]
    path = path.lstrip("./")
    if not path:
        return None
    return path


def parse_patch_paths(patch_path: Path | None) -> list[str]:
    if not patch_path:
        return []
    text = patch_path.read_text(encoding="utf-8")
    paths: set[str] = set()
    for line in text.splitlines():
        if line.startswith("diff --git "):
            parts = line.split()
            for raw in parts[2:4]:
                path = normalize_patch_path(raw)
                if path:
                    paths.add(path)
        elif line.startswith("--- ") or line.startswith("+++ "):
            path = normalize_patch_path(line[4:])
            if path:
                paths.add(path)
        elif line.startswith("rename from ") or line.startswith("rename to "):
            path = normalize_patch_path(line.split(" ", 2)[2])
            if path:
                paths.add(path)
    return sorted(paths)


def path_within(path: str, allowed: str) -> bool:
    allowed = allowed.strip().lstrip("./")
    if allowed.endswith("/"):
        return path.startswith(allowed)
    return path == allowed or path.startswith(f"{allowed}/")


def path_is_safe(path: str) -> tuple[bool, str]:
    candidate = Path(path)
    if candidate.is_absolute():
        return False, "absolute path is not allowed"
    if ".." in candidate.parts:
        return False, "parent traversal is not allowed"
    if any(part in DENY_PATH_PARTS for part in candidate.parts):
        return False, "denied path component"
    return True, "ok"


def guidance_blockers(task: dict[str, Any], guidance: dict[str, Any]) -> list[dict[str, Any]]:
    task_id = str(task.get("id"))
    return [
        item
        for item in guidance.get("findings", [])
        if str(item.get("target")) == task_id and item.get("level") == "blocker"
    ]


def finding(level: str, code: str, path: str, message: str) -> dict[str, str]:
    return {"level": level, "code": code, "path": path, "message": message}


def validate_patch_scope(
    task: dict[str, Any],
    touched_paths: list[str],
    blockers: list[dict[str, Any]],
    allow_blocker_implementation: bool,
) -> dict[str, Any]:
    allowed_paths = [str(item).lstrip("./") for item in list_of((task.get("workspace") or {}).get("allowed_paths"))]
    contracts = task.get("contracts") or {}
    findings: list[dict[str, str]] = []

    if blockers and not allow_blocker_implementation:
        for item in blockers:
            findings.append(
                finding(
                    "blocker",
                    "GUIDANCE_BLOCKER",
                    str(task.get("id")),
                    f"{item.get('code')}: {item.get('message')}",
                )
            )

    for path in touched_paths:
        safe, reason = path_is_safe(path)
        if not safe:
            findings.append(finding("blocker", "UNSAFE_PATH", path, reason))
            continue
        if allowed_paths and not any(path_within(path, allowed) for allowed in allowed_paths):
            findings.append(
                finding(
                    "blocker",
                    "PATH_OUT_OF_SCOPE",
                    path,
                    f"path is outside workspace.allowed_paths: {', '.join(allowed_paths)}",
                )
            )
        for prefix, contract_key in CONTRACT_PATH_RULES:
            if path_within(path, prefix) and not contracts.get(contract_key):
                findings.append(
                    finding(
                        "blocker",
                        "CONTRACT_NOT_DECLARED",
                        path,
                        f"path requires contracts.{contract_key}=true",
                    )
                )

    has_blocker = any(item["level"] == "blocker" for item in findings)
    return {
        "allowed_paths": allowed_paths,
        "contracts": contracts,
        "touched_paths": touched_paths,
        "findings": findings,
        "valid": not has_blocker,
    }


def git_apply_check(patch_path: Path) -> tuple[bool, str]:
    proc = subprocess.run(
        ["git", "apply", "--check", str(patch_path)],
        cwd=REPO_ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    return proc.returncode == 0, proc.stdout


def git_apply_patch(patch_path: Path) -> tuple[bool, str]:
    proc = subprocess.run(
        ["git", "apply", str(patch_path)],
        cwd=REPO_ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    return proc.returncode == 0, proc.stdout


def render_brief(
    task: dict[str, Any],
    run_id: str,
    context_pack: Path | None,
    design_dir: Path | None,
    plan_path: Path | None,
    validation: dict[str, Any],
) -> str:
    verification = task.get("verification") or {}
    lines = [
        f"# Implementation Brief: {task.get('id')}",
        "",
        f"- run_id: `{run_id}`",
        f"- task: {task.get('title')}",
        f"- priority: `{task.get('priority')}`",
        f"- current_status: `{task.get('status')}`",
        f"- execution_mode: `{(task.get('execution') or {}).get('mode')}`",
        f"- patch_valid: `{validation.get('valid')}`",
        "",
        "## Inputs",
    ]
    if context_pack:
        lines.append(f"- context_pack: `{rel_path(context_pack)}`")
    if design_dir:
        lines.append(f"- design_dir: `{rel_path(design_dir)}`")
    if plan_path:
        lines.append(f"- plan: `{rel_path(plan_path)}`")
    if not any([context_pack, design_dir, plan_path]):
        lines.append("- none")
    lines.extend(["", "## Allowed Paths"])
    allowed = validation.get("allowed_paths") or []
    lines.extend(f"- `{path}`" for path in allowed) if allowed else lines.append("- none")
    lines.extend(["", "## Close Conditions"])
    lines.extend(f"- {item}" for item in list_of(task.get("close_when")))
    lines.extend(["", "## Local Verification"])
    local_checks = list_of(verification.get("local"))
    lines.extend(f"- `{cmd}`" for cmd in local_checks) if local_checks else lines.append("- none")
    lines.extend(
        [
            "",
            "## Implementation Rules",
            "- Refresh `git status --short` before editing.",
            "- Read the context pack and exact source refs before relying on summarized text.",
            "- Keep changes inside workspace.allowed_paths unless a design delta expands scope.",
            "- If touching Proto, Kafka, DB schema or APISIX routes, the task contract must declare it and consumers must be checked.",
            "- Do not treat this brief, patch validation or context evidence as task closure.",
            "- After patching, run the task's smallest relevant verification and third-view review.",
            "",
            "## Patch Scope",
        ]
    )
    touched = validation.get("touched_paths") or []
    lines.extend(f"- `{path}`" for path in touched) if touched else lines.append("- no patch supplied")
    lines.extend(["", "## Blocking Findings"])
    blockers = [item for item in validation.get("findings", []) if item.get("level") == "blocker"]
    if blockers:
        for item in blockers:
            lines.append(f"- `{item['code']}` `{item['path']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.append("")
    return "\n".join(lines)


def render_prompt(task: dict[str, Any], run_id: str, brief_path: Path, validation: dict[str, Any]) -> str:
    return "\n".join(
        [
            f"# Codex Implementation Prompt: {task.get('id')}",
            "",
            f"Use `{rel_path(brief_path)}` as the implementation boundary for run `{run_id}`.",
            "",
            "You are allowed to edit only the paths listed in the brief.",
            "Open exact source files before patching; do not rely only on summarized context.",
            "Use repository-native patterns and the smallest relevant verification command.",
            "If any blocker is listed in the brief, stop at design/repair planning unless a human gate explicitly overrides it.",
            "",
            "Current patch validation:",
            f"- valid: `{validation.get('valid')}`",
            f"- touched_paths: `{len(validation.get('touched_paths') or [])}`",
            f"- findings: `{len(validation.get('findings') or [])}`",
            "",
        ]
    )


def render_apply_report(
    task: dict[str, Any],
    run_id: str,
    patch_path: Path | None,
    validation: dict[str, Any],
    apply_patch: bool,
    git_check: tuple[bool, str] | None,
    git_apply: tuple[bool, str] | None,
) -> str:
    lines = [
        f"# Implementation Apply Report: {task.get('id')}",
        "",
        f"- run_id: `{run_id}`",
        f"- patch: `{rel_path(patch_path) if patch_path else 'none'}`",
        f"- apply_patch: `{apply_patch}`",
        f"- scope_valid: `{validation.get('valid')}`",
        "",
        "## Scope Findings",
    ]
    if validation.get("findings"):
        for item in validation["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}` `{item['path']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(["", "## Git Apply Check"])
    if git_check:
        lines.append(f"- passed: `{git_check[0]}`")
        if git_check[1].strip():
            lines.extend(["", "```text", git_check[1][-4000:], "```"])
    else:
        lines.append("- not run")
    lines.extend(["", "## Git Apply"])
    if git_apply:
        lines.append(f"- applied: `{git_apply[0]}`")
        if git_apply[1].strip():
            lines.extend(["", "```text", git_apply[1][-4000:], "```"])
    else:
        lines.append("- not run")
    lines.append("")
    return "\n".join(lines)


def status_for(validation: dict[str, Any], patch_path: Path | None, apply_patch: bool, git_apply: tuple[bool, str] | None) -> str:
    if not validation.get("valid"):
        return "IMPLEMENTATION_BLOCKED"
    if patch_path and apply_patch and git_apply and git_apply[0]:
        return "PATCH_APPLIED"
    if patch_path:
        return "PATCH_VALIDATED"
    return "IMPLEMENTATION_READY"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--context-pack", default=None)
    parser.add_argument("--design-dir", default=None)
    parser.add_argument("--plan", default=None)
    parser.add_argument("--guidance", default=None)
    parser.add_argument("--patch", default=None, help="Optional unified diff patch to validate.")
    parser.add_argument("--apply-patch", action="store_true", help="Apply --patch after scope and git apply checks pass.")
    parser.add_argument("--allow-blocker-implementation", action="store_true")
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--out-dir", default=None)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "implement")))
    run_dir = ensure_run_dir(run_id)
    out_dir = repo_path(args.out_dir) if args.out_dir else run_dir / "implementation"
    out_dir.mkdir(parents=True, exist_ok=True)
    copy_task_snapshot(task_path, run_dir)

    context_pack = repo_path(args.context_pack) if args.context_pack else None
    design_dir = repo_path(args.design_dir) if args.design_dir else None
    plan_path = repo_path(args.plan) if args.plan else None
    guidance = load_json(repo_path(args.guidance) if args.guidance else None)
    patch_path = repo_path(args.patch) if args.patch else None
    touched_paths = parse_patch_paths(patch_path)
    blockers = guidance_blockers(task, guidance)
    validation = validate_patch_scope(task, touched_paths, blockers, args.allow_blocker_implementation)

    git_check: tuple[bool, str] | None = None
    git_apply: tuple[bool, str] | None = None
    if patch_path and validation["valid"]:
        git_check = git_apply_check(patch_path)
        if not git_check[0]:
            validation["valid"] = False
            validation["findings"].append(finding("blocker", "GIT_APPLY_CHECK_FAILED", rel_path(patch_path), "git apply --check failed"))
        elif args.apply_patch:
            git_apply = git_apply_patch(patch_path)
            if not git_apply[0]:
                validation["valid"] = False
                validation["findings"].append(finding("blocker", "GIT_APPLY_FAILED", rel_path(patch_path), "git apply failed"))

    brief_path = out_dir / "implementation-brief.md"
    write_text(brief_path, render_brief(task, run_id, context_pack, design_dir, plan_path, validation))
    write_text(out_dir / "codex-implementation-prompt.md", render_prompt(task, run_id, brief_path, validation))
    write_json(
        out_dir / "patch-scope.json",
        {
            "run_id": run_id,
            "task_id": task.get("id"),
            "patch_path": rel_path(patch_path) if patch_path else None,
            "touched_paths": touched_paths,
            "allowed_paths": validation["allowed_paths"],
            "contracts": validation["contracts"],
        },
    )
    write_json(out_dir / "patch-validation.json", validation)
    write_text(out_dir / "apply-report.md", render_apply_report(task, run_id, patch_path, validation, args.apply_patch, git_check, git_apply))

    status = status_for(validation, patch_path, args.apply_patch, git_apply)
    summary = {
        "run_id": run_id,
        "run_kind": "implementation_guard",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "status": status,
        "evidence_type": "acceptance-prep",
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "context_pack": rel_path(context_pack) if context_pack else None,
        "design_dir": rel_path(design_dir) if design_dir else None,
        "plan": rel_path(plan_path) if plan_path else None,
        "patch_path": rel_path(patch_path) if patch_path else None,
        "patch_valid": validation["valid"],
        "findings": validation["findings"],
        "outputs": IMPLEMENTATION_OUTPUTS,
        "warning": "Implementation guard evidence does not close a task. It only scopes and optionally applies a patch.",
    }
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} patch_valid={validation['valid']} touched={len(touched_paths)} findings={len(validation['findings'])}")
    return 2 if args.apply_patch and status in {"IMPLEMENTATION_BLOCKED"} else 0


if __name__ == "__main__":
    raise SystemExit(main())
