#!/usr/bin/env python3
"""Select a Codex model profile and command template for one patch request.

The model profile is a planning and audit layer. It decides which model,
sandbox, timeout and output schema should be used for an external Codex patch
attempt, but it does not execute Codex and does not apply patches.
"""

from __future__ import annotations

import argparse
import json
import shlex
from datetime import datetime
from pathlib import Path
from typing import Any

from codex_runner import DEFAULT_POLICY as DEFAULT_CODEX_POLICY
from codex_runner import load_policy as load_codex_policy
from codex_runner import validate_command_template
from lib import copy_task_snapshot, ensure_run_dir, list_of, load_yaml_subset, make_run_id, rel_path, repo_path, run_git, write_json, write_text


DEFAULT_MODEL_POLICY = "scripts/codex_loop/policies/model-profiles.yaml"
MODEL_PROFILE_OUTPUTS = [
    "model-profile/model-profile.json",
    "model-profile/model-profile.md",
    "model-profile/command-template.txt",
]


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def finding(level: str, code: str, message: str, target: str = "model-profile") -> dict[str, str]:
    return {"level": level, "code": code, "target": target, "message": message}


def profile_map(policy: dict[str, Any]) -> dict[str, dict[str, Any]]:
    profiles: dict[str, dict[str, Any]] = {}
    raw_profiles = policy.get("profiles") or {}
    if isinstance(raw_profiles, dict):
        for key, value in raw_profiles.items():
            if isinstance(value, dict):
                profile = dict(value)
                profile.setdefault("id", key)
                profiles[str(key)] = profile
        return profiles
    for item in list_of(raw_profiles):
        if isinstance(item, dict) and item.get("id"):
            profiles[str(item["id"])] = item
    return profiles


def path_matches_prefix(path: str, prefixes: list[str]) -> bool:
    normalized = path.lstrip("./")
    return any(normalized == prefix.rstrip("/") or normalized.startswith(prefix) for prefix in prefixes)


def task_has_contract_change(task: dict[str, Any]) -> bool:
    return any(bool(value) for value in (task.get("contracts") or {}).values())


def docs_only(task: dict[str, Any], prefixes: list[str]) -> bool:
    allowed = [str(item) for item in list_of((task.get("workspace") or {}).get("allowed_paths")) if item]
    if not allowed:
        return False
    return all(path_matches_prefix(path, prefixes) for path in allowed)


def select_profile_id(task: dict[str, Any], policy: dict[str, Any], override: str | None) -> tuple[str, list[str]]:
    if override:
        return override, ["explicit --profile-id override"]
    selection = policy.get("selection") or {}
    priority = str(task.get("priority") or "")
    risk_level = str((task.get("risk") or {}).get("level") or "")
    high_priorities = {str(item) for item in list_of(selection.get("high_risk_priorities"))}
    high_levels = {str(item) for item in list_of(selection.get("high_risk_levels"))}
    if priority in high_priorities:
        return str(selection.get("contract_profile") or policy.get("default_profile")), [f"priority {priority} requires high-risk profile"]
    if risk_level in high_levels:
        return str(selection.get("contract_profile") or policy.get("default_profile")), [f"risk level {risk_level} requires high-risk profile"]
    if task_has_contract_change(task):
        return str(selection.get("contract_profile") or policy.get("default_profile")), ["contract-impacting task requires high-risk profile"]
    docs_prefixes = [str(item) for item in list_of(selection.get("docs_prefixes"))]
    if docs_only(task, docs_prefixes):
        return str(selection.get("docs_profile") or policy.get("default_profile")), ["workspace allowed_paths are docs/loop-control scoped"]
    return str(policy.get("default_profile")), ["default profile"]


def render_command_template(profile: dict[str, Any], patch_request: Path, output_schema: Path | None, model_override: str | None) -> str:
    model = model_override or str(profile.get("model") or "")
    sandbox = str(profile.get("sandbox") or "read-only")
    include_schema = bool(profile.get("include_output_schema"))
    prompt = str(profile.get("prompt") or "Read and follow the Codex patch request at {prompt}.")
    parts = ["codex", "exec"]
    if model:
        parts.extend(["--model", model])
    if sandbox:
        parts.extend(["--sandbox", sandbox])
    if include_schema and output_schema:
        parts.extend(["--output-schema", rel_path(output_schema)])
    parts.append(prompt)
    return " ".join(shlex.quote(part) for part in parts)


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        f"# Model Profile: {summary.get('task_id')}",
        "",
        f"- status: `{summary['status']}`",
        f"- selected_profile: `{summary.get('selected_profile')}`",
        f"- model: `{summary.get('model')}`",
        f"- sandbox: `{summary.get('sandbox')}`",
        f"- timeout_seconds: `{summary.get('timeout_seconds')}`",
        f"- patch_request: `{summary.get('patch_request')}`",
        f"- output_schema: `{summary.get('output_schema') or 'none'}`",
        f"- codex_policy: `{summary.get('codex_policy')}`",
        f"- findings: `{len(summary.get('findings') or [])}`",
        "",
        "## Selection Reasons",
    ]
    for reason in summary.get("selection_reasons") or []:
        lines.append(f"- {reason}")
    lines.extend(["", "## Command Template", "", "```bash", summary["command_template"], "```", "", "## Findings"])
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}` `{item.get('target', 'model-profile')}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- This profile only selects a model command template; it does not call Codex.",
            "- `codex_runner.py` must still validate policy, environment gate, redaction and execution status.",
            "- Generated patches must still return through `patch_runner.py`, `review.py` and `evidence_check.py`.",
            "",
        ]
    )
    return "\n".join(lines)


def status_for(findings: list[dict[str, str]]) -> str:
    return "MODEL_PROFILE_BLOCKED" if any(item["level"] == "blocker" for item in findings) else "MODEL_PROFILE_SELECTED"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--patch-request", default=None)
    parser.add_argument("--output-schema", default=None)
    parser.add_argument("--model-policy", default=DEFAULT_MODEL_POLICY)
    parser.add_argument("--codex-policy", default=DEFAULT_CODEX_POLICY)
    parser.add_argument("--profile-id", default=None)
    parser.add_argument("--model", default=None, help="Optional model override recorded in evidence.")
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "model-profile")))
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "model-profile"
    out_dir.mkdir(parents=True, exist_ok=True)
    copy_task_snapshot(task_path, run_dir)

    patch_request = repo_path(args.patch_request) if args.patch_request else run_dir / "patch-runner" / "patch-request.md"
    output_schema = repo_path(args.output_schema) if args.output_schema else run_dir / "patch-runner" / "codex-output-schema.json"
    policy_path = repo_path(args.model_policy)
    codex_policy_path = repo_path(args.codex_policy)
    policy = load_yaml_subset(policy_path)
    profiles = profile_map(policy)
    selected_profile_id, reasons = select_profile_id(task, policy, args.profile_id)
    selected = profiles.get(selected_profile_id, {})
    findings: list[dict[str, str]] = []

    if not selected:
        findings.append(finding("blocker", "MODEL_PROFILE_NOT_FOUND", f"Profile not found: {selected_profile_id}", rel_path(policy_path)))
    if not patch_request.exists():
        findings.append(finding("blocker", "PATCH_REQUEST_MISSING", f"Patch request not found: {rel_path(patch_request)}", rel_path(patch_request)))
    if selected.get("include_output_schema") and not output_schema.exists():
        findings.append(finding("blocker", "OUTPUT_SCHEMA_MISSING", f"Output schema not found: {rel_path(output_schema)}", rel_path(output_schema)))

    command_template = render_command_template(selected, patch_request, output_schema if output_schema.exists() else None, args.model) if selected else ""
    codex_policy = load_codex_policy(codex_policy_path)
    _, command_findings = validate_command_template(command_template, codex_policy)
    findings.extend(
        finding(str(item.get("level", "blocker")), str(item.get("code")), str(item.get("message")), rel_path(codex_policy_path))
        for item in command_findings
    )

    status = status_for(findings)
    summary = {
        "run_id": run_id,
        "run_kind": "model_profile",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "model_policy": rel_path(policy_path),
        "codex_policy": rel_path(codex_policy_path),
        "selected_profile": selected_profile_id,
        "selection_reasons": reasons,
        "model": args.model or selected.get("model"),
        "sandbox": selected.get("sandbox"),
        "timeout_seconds": selected.get("timeout_seconds"),
        "reviewer": selected.get("reviewer"),
        "execute_default": selected.get("execute_default"),
        "patch_request": rel_path(patch_request),
        "output_schema": rel_path(output_schema) if output_schema.exists() else None,
        "command_template": command_template,
        "findings": findings,
        "outputs": MODEL_PROFILE_OUTPUTS,
        "warning": "Model profile evidence selects an external Codex command; it does not execute Codex or trust generated patches.",
    }
    write_json(out_dir / "model-profile.json", summary)
    write_text(out_dir / "model-profile.md", render_report(summary))
    write_text(out_dir / "command-template.txt", command_template + "\n")
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} profile={selected_profile_id} findings={len(findings)}")
    return 1 if status == "MODEL_PROFILE_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
