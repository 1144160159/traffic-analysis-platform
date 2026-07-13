#!/usr/bin/env python3
"""Plan or intake an external LLM reviewer pass for one Codex Loop task.

This is a stronger review layer on top of diff-aware and semantic reviewers.
By default it only writes a review request, JSON schema, model command template
and audit summary. If an LLM output JSON is supplied, it validates the decision
and turns non-pass decisions into explicit loop evidence.
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


DEFAULT_LLM_REVIEW_POLICY = "scripts/codex_loop/policies/llm-review.yaml"
PASS_DECISIONS = {"pass", "passed", "approved"}
NON_PASS_STATUSES = {
    "repair_required": "LLM_REVIEW_REPAIR_REQUIRED",
    "design_update_required": "LLM_REVIEW_DESIGN_REQUIRED",
    "human_gate_required": "LLM_REVIEW_HUMAN_GATE_REQUIRED",
    "pending": "LLM_REVIEW_PENDING",
}
LLM_REVIEW_OUTPUTS = [
    "llm-review/llm-review-request.md",
    "llm-review/llm-review-schema.json",
    "llm-review/llm-review-profile.json",
    "llm-review/command-template.txt",
    "llm-review/llm-review-summary.json",
    "llm-review/llm-review-report.md",
]


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def read_excerpt(path: Path | None, limit: int = 12000) -> str:
    if not path or not path.exists():
        return ""
    text = path.read_text(encoding="utf-8")
    if len(text) <= limit:
        return text
    return text[:limit] + "\n[TRUNCATED]\n"


def finding(level: str, code: str, target: str, message: str) -> dict[str, str]:
    return {"level": level, "code": code, "target": target, "message": message}


def profile_map(policy: dict[str, Any]) -> dict[str, dict[str, Any]]:
    raw = policy.get("profiles") or {}
    if not isinstance(raw, dict):
        return {}
    profiles: dict[str, dict[str, Any]] = {}
    for key, value in raw.items():
        if isinstance(value, dict):
            profile = dict(value)
            profile.setdefault("id", key)
            profiles[str(key)] = profile
    return profiles


def task_has_contract_change(task: dict[str, Any]) -> bool:
    return any(bool(value) for value in (task.get("contracts") or {}).values())


def select_profile_id(task: dict[str, Any], policy: dict[str, Any], override: str | None) -> tuple[str, list[str]]:
    if override:
        return override, ["explicit --profile-id override"]
    selection = policy.get("selection") or {}
    priority = str(task.get("priority") or "")
    risk_level = str((task.get("risk") or {}).get("level") or "")
    high_priorities = {str(item) for item in list_of(selection.get("high_risk_priorities"))}
    high_levels = {str(item) for item in list_of(selection.get("high_risk_levels"))}
    if priority in high_priorities:
        return str(selection.get("contract_profile") or policy.get("default_profile")), [f"priority {priority} requires high-risk LLM reviewer"]
    if risk_level in high_levels:
        return str(selection.get("contract_profile") or policy.get("default_profile")), [f"risk level {risk_level} requires high-risk LLM reviewer"]
    if task_has_contract_change(task):
        return str(selection.get("contract_profile") or policy.get("default_profile")), ["contract-impacting task requires high-risk LLM reviewer"]
    return str(policy.get("default_profile")), ["default LLM reviewer profile"]


def render_schema() -> dict[str, Any]:
    return {
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "title": "CodexLoopLlmReview",
        "type": "object",
        "additionalProperties": False,
        "required": ["decision", "perspectives", "findings", "evidence_gaps", "required_next_action", "confidence"],
        "properties": {
            "decision": {
                "type": "string",
                "enum": ["pass", "repair_required", "design_update_required", "human_gate_required", "pending"],
            },
            "perspectives": {
                "type": "object",
                "additionalProperties": {"type": "string"},
            },
            "findings": {
                "type": "array",
                "items": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["level", "code", "target", "message", "suggestion"],
                    "properties": {
                        "level": {"type": "string", "enum": ["blocker", "warning", "info"]},
                        "code": {"type": "string", "minLength": 1},
                        "target": {"type": "string", "minLength": 1},
                        "message": {"type": "string", "minLength": 1},
                        "suggestion": {"type": "string", "minLength": 1},
                    },
                },
            },
            "evidence_gaps": {
                "type": "array",
                "items": {"type": "string"},
            },
            "required_next_action": {"type": "string", "minLength": 1},
            "confidence": {"type": "string", "enum": ["low", "medium", "high"]},
            "reviewer_notes": {"type": "string"},
        },
    }


def render_request(
    task: dict[str, Any],
    run_id: str,
    review: dict[str, Any],
    semantic: dict[str, Any],
    context_pack: Path | None,
    design_dir: Path | None,
    patch_request: Path | None,
    patch_intake: dict[str, Any],
) -> str:
    lines = [
        f"# LLM Reviewer Request: {task.get('id')}",
        "",
        "## Task",
        f"- run_id: `{run_id}`",
        f"- title: {task.get('title')}",
        f"- priority: `{task.get('priority')}`",
        f"- acceptance_type: `{task.get('acceptance_type')}`",
        f"- execution_mode: `{(task.get('execution') or {}).get('mode')}`",
        "",
        "## Required Decision",
        "- Return JSON only.",
        "- Use decision `pass` only when product logic, technical design, code scope and evidence are all sufficient.",
        "- Use `repair_required` for implementation or verification gaps.",
        "- Use `design_update_required` for product, architecture, API, data, or visual-design mismatch.",
        "- Use `human_gate_required` for secrets, destructive data action, auth boundary risk, production safety risk, or unclear ownership.",
        "",
        "## Static Reviewer",
        f"- decision: `{review.get('decision', 'missing')}`",
        f"- local_status: `{review.get('local_status', 'missing')}`",
        f"- changed_paths: `{len(review.get('changed_paths') or [])}`",
    ]
    for item in list_of(review.get("blocking_findings"))[:20]:
        lines.append(f"- blocker `{item.get('code')}` `{item.get('target')}`: {item.get('message')}")
    for item in list_of(review.get("non_blocking_findings"))[:20]:
        lines.append(f"- non_blocking `{item.get('code')}` `{item.get('target')}`: {item.get('message')}")
    lines.extend(
        [
            "",
            "## Semantic Reviewer",
            f"- status: `{semantic.get('status', 'missing')}`",
            f"- review_decision: `{semantic.get('review_decision', 'missing')}`",
        ]
    )
    for item in list_of(semantic.get("findings"))[:20]:
        lines.append(f"- semantic `{item.get('level')}` `{item.get('code')}`: {item.get('message')}")
    lines.extend(["", "## Patch Intake"])
    if patch_intake:
        validation = patch_intake.get("validation") or {}
        lines.append(f"- patch_path: `{patch_intake.get('patch_path') or 'none'}`")
        lines.append(f"- touched_paths: `{len(patch_intake.get('touched_paths') or [])}`")
        lines.append(f"- patch_valid: `{validation.get('valid')}`")
        for item in list_of(validation.get("findings"))[:20]:
            lines.append(f"- patch finding `{item.get('level')}` `{item.get('code')}` `{item.get('path')}`: {item.get('message')}")
    else:
        lines.append("- no patch intake was available")
    lines.extend(["", "## Close Conditions"])
    lines.extend(f"- {item}" for item in list_of(task.get("close_when")))
    lines.extend(["", "## Context Pack Excerpt", "```text", read_excerpt(context_pack, 10000), "```"])
    if design_dir:
        for name in ["feature-spec.md", "architecture-evolution.md", "visual-correction.md", "acceptance-cases.md"]:
            lines.extend(["", f"## Design {name}", "```text", read_excerpt(design_dir / name, 5000), "```"])
    if patch_request:
        lines.extend(["", "## Patch Request Excerpt", "```text", read_excerpt(patch_request, 8000), "```"])
    lines.append("")
    return "\n".join(lines)


def render_command_template(profile: dict[str, Any], request_path: Path, schema_path: Path, model_override: str | None) -> str:
    model = model_override or str(profile.get("model") or "")
    sandbox = str(profile.get("sandbox") or "read-only")
    prompt = str(profile.get("prompt") or "Review the Codex Loop reviewer request at {prompt}. Return only JSON matching the provided schema.")
    parts = ["codex", "exec"]
    if model:
        parts.extend(["--model", model])
    if sandbox:
        parts.extend(["--sandbox", sandbox])
    parts.extend(["--output-schema", rel_path(schema_path)])
    parts.append(prompt)
    return " ".join(shlex.quote(part) for part in parts)


def validate_llm_output(path: Path | None) -> tuple[dict[str, Any], list[dict[str, str]]]:
    if not path:
        return {}, []
    findings: list[dict[str, str]] = []
    if not path.exists():
        return {}, [finding("blocker", "LLM_OUTPUT_MISSING", rel_path(path), "LLM output JSON was not found.")]
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return {}, [finding("blocker", "LLM_OUTPUT_PARSE_FAILED", rel_path(path), str(exc))]
    for key in ["decision", "perspectives", "findings", "evidence_gaps", "required_next_action", "confidence"]:
        if key not in data:
            findings.append(finding("blocker", "LLM_OUTPUT_FIELD_MISSING", rel_path(path), f"`{key}` is required."))
    decision = str(data.get("decision") or "")
    if decision not in PASS_DECISIONS and decision not in NON_PASS_STATUSES:
        findings.append(finding("blocker", "LLM_OUTPUT_DECISION_INVALID", rel_path(path), f"Invalid decision `{decision}`."))
    if "findings" in data and not isinstance(data["findings"], list):
        findings.append(finding("blocker", "LLM_OUTPUT_FINDINGS_INVALID", rel_path(path), "`findings` must be a list."))
    return data, findings


def status_for(findings: list[dict[str, str]], llm_output: dict[str, Any], llm_output_path: Path | None) -> str:
    if any(item["level"] == "blocker" for item in findings):
        return "LLM_REVIEW_BLOCKED"
    if not llm_output_path:
        return "LLM_REVIEW_PLANNED"
    decision = str(llm_output.get("decision") or "").lower()
    if decision in PASS_DECISIONS:
        return "LLM_REVIEW_PASSED"
    return NON_PASS_STATUSES.get(decision, "LLM_REVIEW_BLOCKED")


def render_report(summary: dict[str, Any]) -> str:
    lines = [
        f"# LLM Review: {summary.get('task_id')}",
        "",
        f"- status: `{summary['status']}`",
        f"- selected_profile: `{summary.get('selected_profile')}`",
        f"- model: `{summary.get('model')}`",
        f"- sandbox: `{summary.get('sandbox')}`",
        f"- timeout_seconds: `{summary.get('timeout_seconds')}`",
        f"- decision: `{summary.get('decision') or 'none'}`",
        f"- review_request: `{summary.get('review_request')}`",
        f"- output_schema: `{summary.get('output_schema')}`",
        f"- llm_output: `{summary.get('llm_output_path') or 'none'}`",
        f"- findings: `{len(summary.get('findings') or [])}`",
        "",
        "## Selection Reasons",
    ]
    for reason in summary.get("selection_reasons") or []:
        lines.append(f"- {reason}")
    lines.extend(["", "## Command Template", "", "```bash", summary.get("command_template") or "", "```", "", "## Findings"])
    if summary.get("findings"):
        for item in summary["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}` `{item.get('target')}`: {item['message']}")
    else:
        lines.append("- none")
    if summary.get("llm_output"):
        lines.extend(["", "## LLM Decision"])
        lines.append(f"- decision: `{summary['llm_output'].get('decision')}`")
        lines.append(f"- required_next_action: {summary['llm_output'].get('required_next_action')}")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- Planned LLM review evidence does not close a task.",
            "- Actual LLM output is advisory unless `evidence_check.py` and the required reviewer gates also pass.",
            "- Non-pass LLM decisions must become repair, design iteration, or human gate evidence.",
            "",
        ]
    )
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--run-id", default=None)
    parser.add_argument("--review-summary", default=None)
    parser.add_argument("--semantic-review", default=None)
    parser.add_argument("--context-pack", default=None)
    parser.add_argument("--design-dir", default=None)
    parser.add_argument("--patch-request", default=None)
    parser.add_argument("--patch-intake", default=None)
    parser.add_argument("--llm-output", default=None)
    parser.add_argument("--llm-policy", default=DEFAULT_LLM_REVIEW_POLICY)
    parser.add_argument("--codex-policy", default=DEFAULT_CODEX_POLICY)
    parser.add_argument("--profile-id", default=None)
    parser.add_argument("--model", default=None)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_id = args.run_id or make_run_id(str(task.get("id", "llm-review")))
    run_dir = ensure_run_dir(run_id)
    out_dir = run_dir / "llm-review"
    out_dir.mkdir(parents=True, exist_ok=True)
    copy_task_snapshot(task_path, run_dir)

    review_path = repo_path(args.review_summary) if args.review_summary else run_dir / "review" / "review-summary.json"
    semantic_path = repo_path(args.semantic_review) if args.semantic_review else run_dir / "semantic-review" / "semantic-review.json"
    context_pack = repo_path(args.context_pack) if args.context_pack else run_dir / "context-pack" / "task-context-pack.md"
    design_dir = repo_path(args.design_dir) if args.design_dir else run_dir / "design"
    patch_request = repo_path(args.patch_request) if args.patch_request else run_dir / "patch-runner" / "patch-request.md"
    patch_intake_path = repo_path(args.patch_intake) if args.patch_intake else run_dir / "patch-runner" / "patch-intake.json"
    llm_output_path = repo_path(args.llm_output) if args.llm_output else None
    llm_policy_path = repo_path(args.llm_policy)
    codex_policy_path = repo_path(args.codex_policy)

    review = load_json(review_path)
    semantic = load_json(semantic_path)
    patch_intake = load_json(patch_intake_path)
    request_path = out_dir / "llm-review-request.md"
    schema_path = out_dir / "llm-review-schema.json"
    write_text(request_path, render_request(task, run_id, review, semantic, context_pack, design_dir, patch_request, patch_intake))
    write_json(schema_path, render_schema())

    policy = load_yaml_subset(llm_policy_path)
    profiles = profile_map(policy)
    selected_profile_id, reasons = select_profile_id(task, policy, args.profile_id)
    profile = profiles.get(selected_profile_id, {})
    findings: list[dict[str, str]] = []
    if not profile:
        findings.append(finding("blocker", "LLM_REVIEW_PROFILE_NOT_FOUND", rel_path(llm_policy_path), f"Profile not found: {selected_profile_id}"))
    if not review:
        findings.append(finding("warning", "STATIC_REVIEW_MISSING", rel_path(review_path), "Diff-aware review summary is missing."))
    if not semantic:
        findings.append(finding("warning", "SEMANTIC_REVIEW_MISSING", rel_path(semantic_path), "Semantic review summary is missing."))
    command_template = render_command_template(profile, request_path, schema_path, args.model) if profile else ""
    codex_policy = load_codex_policy(codex_policy_path)
    _, command_findings = validate_command_template(command_template, codex_policy)
    findings.extend(
        finding(str(item.get("level", "blocker")), str(item.get("code")), rel_path(codex_policy_path), str(item.get("message")))
        for item in command_findings
    )
    llm_output, output_findings = validate_llm_output(llm_output_path)
    findings.extend(output_findings)
    status = status_for(findings, llm_output, llm_output_path)
    summary = {
        "run_id": run_id,
        "run_kind": "llm_review",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "status": status,
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "llm_policy": rel_path(llm_policy_path),
        "codex_policy": rel_path(codex_policy_path),
        "selected_profile": selected_profile_id,
        "selection_reasons": reasons,
        "model": args.model or profile.get("model"),
        "sandbox": profile.get("sandbox"),
        "timeout_seconds": profile.get("timeout_seconds"),
        "review_request": rel_path(request_path),
        "output_schema": rel_path(schema_path),
        "command_template": command_template,
        "llm_output_path": rel_path(llm_output_path) if llm_output_path else None,
        "decision": llm_output.get("decision"),
        "llm_output": llm_output,
        "findings": findings,
        "outputs": LLM_REVIEW_OUTPUTS,
        "warning": "LLM reviewer evidence is an additional review layer. It does not replace diff-aware review, semantic review, local verification, or evidence_check.py.",
    }
    write_json(out_dir / "llm-review-profile.json", {
        "selected_profile": selected_profile_id,
        "selection_reasons": reasons,
        "model": summary["model"],
        "sandbox": summary["sandbox"],
        "timeout_seconds": summary["timeout_seconds"],
        "command_template": command_template,
        "codex_policy": summary["codex_policy"],
        "findings": findings,
    })
    write_text(out_dir / "command-template.txt", command_template + "\n")
    write_json(out_dir / "llm-review-summary.json", summary)
    write_text(out_dir / "llm-review-report.md", render_report(summary))
    write_json(run_dir / "run-summary.json", summary)
    print(out_dir)
    print(f"status={status} profile={selected_profile_id} findings={len(findings)} decision={summary.get('decision')}")
    return 1 if status == "LLM_REVIEW_BLOCKED" else 0


if __name__ == "__main__":
    raise SystemExit(main())
