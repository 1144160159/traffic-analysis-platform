#!/usr/bin/env python3
"""Create and validate Codex patch work orders for one loop task.

This is the bridge between the loop control plane and Codex as an implementer.
By default it only writes a patch request and a structured output contract.
When a patch or Codex output JSON is supplied, it validates the touched paths,
task contracts and guidance blockers before optionally applying the patch.
"""

from __future__ import annotations

import argparse
import json
from datetime import datetime
from pathlib import Path
from typing import Any

from implement import (
    finding,
    git_apply_check,
    git_apply_patch,
    guidance_blockers,
    parse_patch_paths,
    validate_patch_scope,
)
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


PATCH_RUNNER_OUTPUTS = [
    "patch-runner/patch-request.md",
    "patch-runner/patch-request.json",
    "patch-runner/codex-output-contract.json",
    "patch-runner/codex-output-schema.json",
    "patch-runner/patch-intake.json",
    "patch-runner/patch-runner-summary.json",
]


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def render_request(
    task: dict[str, Any],
    run_id: str,
    context_pack: Path | None,
    design_dir: Path | None,
    guidance_path: Path | None,
    repair_plan: Path | None,
) -> str:
    allowed = list_of((task.get("workspace") or {}).get("allowed_paths"))
    local_checks = list_of((task.get("verification") or {}).get("local"))
    lines = [
        f"# Codex Patch Request: {task.get('id')}",
        "",
        f"- run_id: `{run_id}`",
        f"- task: {task.get('title')}",
        f"- priority: `{task.get('priority')}`",
        f"- acceptance_type: `{task.get('acceptance_type')}`",
        "",
        "## Inputs",
    ]
    if context_pack:
        lines.append(f"- context_pack: `{rel_path(context_pack)}`")
    if design_dir:
        lines.append(f"- design_dir: `{rel_path(design_dir)}`")
    if guidance_path:
        lines.append(f"- guidance: `{rel_path(guidance_path)}`")
    if repair_plan:
        lines.append(f"- repair_plan: `{rel_path(repair_plan)}`")
    lines.extend(["", "## Allowed Paths"])
    lines.extend(f"- `{path}`" for path in allowed) if allowed else lines.append("- none")
    lines.extend(["", "## Close Conditions"])
    lines.extend(f"- {item}" for item in list_of(task.get("close_when")))
    lines.extend(["", "## Verification"])
    lines.extend(f"- `{cmd}`" for cmd in local_checks) if local_checks else lines.append("- none")
    lines.extend(
        [
            "",
            "## Required Codex Output",
            "- Provide a unified diff patch file.",
            "- Provide `codex-output.json` matching `patch-runner/codex-output-contract.json`.",
            "- Do not edit outside Allowed Paths.",
            "- Do not mark the task closed; evidence_check.py decides closure eligibility.",
            "",
        ]
    )
    return "\n".join(lines)


def output_contract(task: dict[str, Any]) -> dict[str, Any]:
    return {
        "schema": "codex_loop.codex_output.v1",
        "required": ["summary", "files", "tests", "evidence", "risks"],
        "properties": {
            "summary": "Short explanation of the implemented change.",
            "files": [{"path": "relative/path", "reason": "why touched"}],
            "tests": [{"command": "tests/run_tests.sh web", "status": "passed|failed|not_run", "evidence": "local-report.md"}],
            "evidence": [{"path": "evidence-report.md", "type": task.get("acceptance_type")}],
            "risks": [{"level": "low|medium|high", "message": "remaining risk"}],
            "patch_path": "Optional relative path to the unified diff patch.",
        },
    }


def output_json_schema(task: dict[str, Any]) -> dict[str, Any]:
    acceptance_type = str(task.get("acceptance_type") or "")
    return {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "title": "CodexLoopCodexOutput",
        "type": "object",
        "additionalProperties": False,
        "required": ["summary", "files", "tests", "evidence", "risks"],
        "properties": {
            "summary": {
                "type": "string",
                "minLength": 1,
            },
            "files": {
                "type": "array",
                "items": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["path", "reason"],
                    "properties": {
                        "path": {"type": "string", "minLength": 1},
                        "reason": {"type": "string", "minLength": 1},
                    },
                },
            },
            "tests": {
                "type": "array",
                "items": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["command", "status", "evidence"],
                    "properties": {
                        "command": {"type": "string", "minLength": 1},
                        "status": {"type": "string", "enum": ["passed", "failed", "not_run"]},
                        "evidence": {"type": "string"},
                    },
                },
            },
            "evidence": {
                "type": "array",
                "items": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["path", "type"],
                    "properties": {
                        "path": {"type": "string", "minLength": 1},
                        "type": {"type": "string", "enum": ["smoke", "regression", "acceptance", "third-party", acceptance_type]},
                    },
                },
            },
            "risks": {
                "type": "array",
                "items": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["level", "message"],
                    "properties": {
                        "level": {"type": "string", "enum": ["low", "medium", "high"]},
                        "message": {"type": "string", "minLength": 1},
                    },
                },
            },
            "patch_path": {"type": "string"},
        },
    }


def validate_codex_output(codex_output: dict[str, Any]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    for key in ["summary", "files", "tests", "evidence", "risks"]:
        if key not in codex_output:
            findings.append(finding("blocker", "CODEX_OUTPUT_FIELD_MISSING", key, f"`{key}` is required in codex-output.json"))
    if "files" in codex_output and not isinstance(codex_output["files"], list):
        findings.append(finding("blocker", "CODEX_OUTPUT_FILES_INVALID", "files", "`files` must be a list"))
    if "tests" in codex_output and not isinstance(codex_output["tests"], list):
        findings.append(finding("blocker", "CODEX_OUTPUT_TESTS_INVALID", "tests", "`tests` must be a list"))
    return findings


def render_summary(summary: dict[str, Any]) -> str:
    lines = [
        f"# Patch Runner Summary: {summary.get('task_id')}",
        "",
        f"- status: `{summary['status']}`",
        f"- patch_path: `{summary.get('patch_path') or 'none'}`",
        f"- codex_output: `{summary.get('codex_output_path') or 'none'}`",
        f"- touched_paths: `{len(summary.get('touched_paths') or [])}`",
        f"- findings: `{len(summary.get('findings') or [])}`",
        "",
        "## Findings",
    ]
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}` `{item['path']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- This runner may validate and optionally apply a patch, but task closure still requires verification, review and evidence_check.py.",
            "",
        ]
    )
    return "\n".join(lines)


def status_for(validation: dict[str, Any], patch_path: Path | None, apply_patch: bool, applied: tuple[bool, str] | None) -> str:
    if not validation.get("valid"):
        if not patch_path:
            return "PATCH_BLOCKED"
        return "PATCH_REJECTED"
    if patch_path and apply_patch and applied and applied[0]:
        return "PATCH_APPLIED"
    if patch_path:
        return "PATCH_VALIDATED"
    return "PATCH_REQUESTED"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--context-pack", default=None)
    parser.add_argument("--design-dir", default=None)
    parser.add_argument("--guidance", default=None)
    parser.add_argument("--repair-plan", default=None)
    parser.add_argument("--patch", default=None)
    parser.add_argument("--codex-output", default=None)
    parser.add_argument("--apply-patch", action="store_true")
    parser.add_argument("--allow-blocker-implementation", action="store_true")
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--out-dir", default=None)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "patch-runner")))
    run_dir = ensure_run_dir(run_id)
    out_dir = repo_path(args.out_dir) if args.out_dir else run_dir / "patch-runner"
    out_dir.mkdir(parents=True, exist_ok=True)
    copy_task_snapshot(task_path, run_dir)

    context_pack = repo_path(args.context_pack) if args.context_pack else None
    design_dir = repo_path(args.design_dir) if args.design_dir else None
    guidance_path = repo_path(args.guidance) if args.guidance else run_dir / "guidance" / "guidance.json"
    repair_plan = repo_path(args.repair_plan) if args.repair_plan else None
    patch_path = repo_path(args.patch) if args.patch else None
    codex_output_path = repo_path(args.codex_output) if args.codex_output else None
    guidance = load_json(guidance_path)
    codex_output = load_json(codex_output_path)

    write_text(out_dir / "patch-request.md", render_request(task, run_id, context_pack, design_dir, guidance_path, repair_plan))
    write_json(
        out_dir / "patch-request.json",
        {
            "run_id": run_id,
            "task_id": task.get("id"),
            "task_title": task.get("title"),
            "context_pack": rel_path(context_pack) if context_pack else None,
            "design_dir": rel_path(design_dir) if design_dir else None,
            "guidance": rel_path(guidance_path) if guidance_path else None,
            "repair_plan": rel_path(repair_plan) if repair_plan else None,
            "allowed_paths": list_of((task.get("workspace") or {}).get("allowed_paths")),
            "close_when": list_of(task.get("close_when")),
        },
    )
    write_json(out_dir / "codex-output-contract.json", output_contract(task))
    write_json(out_dir / "codex-output-schema.json", output_json_schema(task))

    touched_paths = parse_patch_paths(patch_path)
    blockers = guidance_blockers(task, guidance)
    validation = validate_patch_scope(task, touched_paths, blockers, args.allow_blocker_implementation)
    if codex_output_path:
        validation["findings"].extend(validate_codex_output(codex_output))
    if any(item["level"] == "blocker" for item in validation["findings"]):
        validation["valid"] = False

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

    status = status_for(validation, patch_path, args.apply_patch, git_apply)
    intake = {
        "patch_path": rel_path(patch_path) if patch_path else None,
        "codex_output_path": rel_path(codex_output_path) if codex_output_path else None,
        "touched_paths": touched_paths,
        "validation": validation,
        "git_apply_check": {"passed": git_check[0], "output_tail": git_check[1][-4000:]} if git_check else None,
        "git_apply": {"applied": git_apply[0], "output_tail": git_apply[1][-4000:]} if git_apply else None,
    }
    write_json(out_dir / "patch-intake.json", intake)

    summary = {
        "run_id": run_id,
        "run_kind": "patch_runner",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "patch_path": rel_path(patch_path) if patch_path else None,
        "codex_output_path": rel_path(codex_output_path) if codex_output_path else None,
        "touched_paths": touched_paths,
        "findings": validation["findings"],
        "patch_valid": validation["valid"],
        "outputs": PATCH_RUNNER_OUTPUTS,
        "warning": "Patch runner output is implementation orchestration evidence, not task closure evidence.",
    }
    write_json(out_dir / "patch-runner-summary.json", summary)
    write_text(out_dir / "patch-runner-report.md", render_summary(summary))
    print(out_dir)
    print(f"status={status} patch_valid={validation['valid']} touched={len(touched_paths)} findings={len(validation['findings'])}")
    return 2 if status == "PATCH_REJECTED" and patch_path else 0


if __name__ == "__main__":
    raise SystemExit(main())
