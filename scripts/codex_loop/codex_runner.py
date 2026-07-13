#!/usr/bin/env python3
"""Run external Codex commands through a policy gate and audit log.

This runner is stricter than codex_adapter.py. It defaults to plan-only mode,
uses argv execution instead of a shell, only forwards allowlisted environment
variables, redacts process output, and requires an explicit environment gate
before any external Codex command can run.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import shutil
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any

from lib import copy_task_snapshot, ensure_run_dir, list_of, load_yaml_subset, rel_path, repo_path, run_git, write_json, write_text


DEFAULT_POLICY = "scripts/codex_loop/policies/codex-execution.yaml"
EXECUTE_TRUE_VALUES = {"1", "true", "yes", "allow", "allowed"}


def load_policy(path: Path) -> dict[str, Any]:
    policy = load_yaml_subset(path)
    policy.setdefault("allow_execute_env", "CODEX_LOOP_ALLOW_EXTERNAL_CODEX")
    policy.setdefault("required_prompt_placeholder", "{prompt}")
    policy.setdefault("allowed_binaries", ["codex"])
    policy.setdefault("allowed_command_prefixes", ["codex exec"])
    policy.setdefault("env_allowlist", ["PATH", "HOME", "CODEX_HOME", "OPENAI_API_KEY"])
    policy.setdefault("secret_name_patterns", ["KEY", "TOKEN", "SECRET", "PASSWORD"])
    policy.setdefault("denied_fragments", ["&&", ";", "|", "`", "$(", "rm -rf"])
    policy.setdefault("default_timeout_seconds", 1800)
    policy.setdefault("max_prompt_bytes", 262144)
    policy.setdefault("max_output_chars", 120000)
    return policy


def load_json(path: Path | None) -> dict[str, Any]:
    if not path or not path.exists():
        return {}
    return json.loads(path.read_text(encoding="utf-8"))


def is_secret_name(name: str, patterns: list[str]) -> bool:
    upper = name.upper()
    return any(pattern.upper() in upper for pattern in patterns)


def build_env(policy: dict[str, Any]) -> tuple[dict[str, str], dict[str, Any]]:
    patterns = [str(item) for item in list_of(policy.get("secret_name_patterns"))]
    env: dict[str, str] = {}
    audit: dict[str, Any] = {}
    for name in [str(item) for item in list_of(policy.get("env_allowlist"))]:
        if name not in os.environ:
            continue
        value = os.environ[name]
        env[name] = value
        audit[name] = {
            "present": True,
            "secret_like": is_secret_name(name, patterns),
            "value": "[redacted]",
        }
    return env, audit


def redact_text(text: str, env: dict[str, str]) -> str:
    redacted = text
    for name, value in env.items():
        if value and len(value) >= 4:
            redacted = redacted.replace(value, f"[REDACTED:{name}]")
    redactions = [
        (re.compile(r"sk-[A-Za-z0-9_-]{20,}"), "[REDACTED:OPENAI_KEY]"),
        (re.compile(r"(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{12,}"), r"\1[REDACTED:TOKEN]"),
        (
            re.compile(r"(?i)((?:api[_-]?key|token|secret|password|authorization)\s*[:=]\s*)[\"']?[^\"'\s]+"),
            r"\1[REDACTED]",
        ),
    ]
    for pattern, replacement in redactions:
        redacted = pattern.sub(replacement, redacted)
    return redacted


def truncate_text(text: str, limit: int) -> tuple[str, bool]:
    if len(text) <= limit:
        return text, False
    return text[:limit] + "\n[TRUNCATED]\n", True


def validate_prompt_path(path: Path, policy: dict[str, Any]) -> list[dict[str, str]]:
    findings: list[dict[str, str]] = []
    if not path.exists():
        findings.append({"level": "blocker", "code": "PROMPT_NOT_FOUND", "message": f"Patch request not found: {rel_path(path)}"})
        return findings
    max_bytes = int(policy.get("max_prompt_bytes") or 262144)
    size = path.stat().st_size
    if size > max_bytes:
        findings.append({"level": "blocker", "code": "PROMPT_TOO_LARGE", "message": f"Patch request is {size} bytes, limit is {max_bytes}"})
    return findings


def validate_command_template(template: str, policy: dict[str, Any]) -> tuple[list[str], list[dict[str, str]]]:
    findings: list[dict[str, str]] = []
    placeholder = str(policy.get("required_prompt_placeholder") or "{prompt}")
    if placeholder not in template:
        findings.append({"level": "blocker", "code": "PROMPT_PLACEHOLDER_MISSING", "message": f"Command template must include {placeholder}"})

    for fragment in [str(item) for item in list_of(policy.get("denied_fragments"))]:
        if fragment and fragment in template:
            findings.append({"level": "blocker", "code": "COMMAND_FRAGMENT_DENIED", "message": f"Denied command fragment: {fragment}"})

    try:
        argv = shlex.split(template)
    except ValueError as exc:
        return [], findings + [{"level": "blocker", "code": "COMMAND_PARSE_FAILED", "message": str(exc)}]

    if not argv:
        findings.append({"level": "blocker", "code": "COMMAND_EMPTY", "message": "Command template parsed to an empty argv"})
        return argv, findings

    allowed_binaries = {str(item) for item in list_of(policy.get("allowed_binaries"))}
    if argv[0] not in allowed_binaries:
        findings.append({"level": "blocker", "code": "COMMAND_BINARY_NOT_ALLOWED", "message": f"Binary is not allowlisted: {argv[0]}"})

    prefixes = [shlex.split(str(item)) for item in list_of(policy.get("allowed_command_prefixes"))]
    if prefixes and not any(argv[: len(prefix)] == prefix for prefix in prefixes):
        findings.append({"level": "blocker", "code": "COMMAND_PREFIX_NOT_ALLOWED", "message": "Command prefix does not match policy"})

    return argv, findings


def render_argv(template: str, prompt_path: Path, policy: dict[str, Any]) -> tuple[list[str], list[dict[str, str]]]:
    argv, findings = validate_command_template(template, policy)
    if findings:
        return argv, findings
    placeholder = str(policy.get("required_prompt_placeholder") or "{prompt}")
    rendered = [item.replace(placeholder, str(prompt_path)) for item in argv]
    return rendered, []


def execute_allowed(policy: dict[str, Any]) -> tuple[bool, dict[str, Any]]:
    gate_name = str(policy.get("allow_execute_env") or "CODEX_LOOP_ALLOW_EXTERNAL_CODEX")
    value = os.environ.get(gate_name, "")
    allowed = value.strip().lower() in EXECUTE_TRUE_VALUES
    return allowed, {"name": gate_name, "present": gate_name in os.environ, "accepted": allowed, "value": "[redacted]" if gate_name in os.environ else None}


def render_report(invocation: dict[str, Any]) -> str:
    lines = [
        f"# Codex Runner Report: {invocation.get('task_id')}",
        "",
        f"- status: `{invocation['status']}`",
        f"- execute_requested: `{invocation['execute_requested']}`",
        f"- command_template: `{invocation['command_template']}`",
        f"- model_profile: `{invocation.get('model_profile') or 'none'}`",
        f"- selected_model: `{invocation.get('selected_model') or 'none'}`",
        f"- patch_request: `{invocation['patch_request']}`",
        f"- policy: `{invocation['policy']}`",
        f"- env_gate: `{invocation['execute_gate']['name']}` accepted `{invocation['execute_gate']['accepted']}`",
        f"- exit_code: `{invocation.get('exit_code', 'none')}`",
        f"- findings: `{len(invocation.get('findings') or [])}`",
        "",
        "## Findings",
    ]
    if invocation.get("findings"):
        for item in invocation["findings"]:
            lines.append(f"- `{item['level']}` `{item['code']}`: {item['message']}")
    else:
        lines.append("- none")
    lines.extend(
        [
            "",
            "## Guardrail",
            "- This runner never executes through a shell.",
            "- Only policy-allowlisted environment variables are forwarded, and all recorded values are redacted.",
            "- External output is redacted before being written as evidence.",
            "- Patch application and task closure remain delegated to patch_runner.py and evidence_check.py.",
            "",
        ]
    )
    return "\n".join(lines)


def status_for(execute: bool, findings: list[dict[str, str]], exit_code: int | None, timed_out: bool) -> str:
    if timed_out:
        return "CODEX_RUNNER_TIMEOUT"
    if findings:
        return "CODEX_RUNNER_BLOCKED"
    if not execute:
        return "CODEX_RUNNER_PLANNED"
    if exit_code == 0:
        return "CODEX_RUNNER_COMPLETED"
    return "CODEX_RUNNER_FAILED"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--task", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--patch-request", required=True)
    parser.add_argument("--command-template", default=None, help="External command argv template. Must include {prompt}.")
    parser.add_argument("--model-profile", default=None, help="model-profile/model-profile.json. Used when --command-template is omitted.")
    parser.add_argument("--policy", default=DEFAULT_POLICY)
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--timeout-seconds", type=int, default=None)
    args = parser.parse_args()

    task_path = repo_path(args.task)
    task = load_yaml_subset(task_path)
    run_dir = ensure_run_dir(args.run_id)
    out_dir = run_dir / "codex-runner"
    out_dir.mkdir(parents=True, exist_ok=True)
    copy_task_snapshot(task_path, run_dir)

    policy_path = repo_path(args.policy)
    policy = load_policy(policy_path)
    prompt_path = repo_path(args.patch_request)
    model_profile_path = repo_path(args.model_profile) if args.model_profile else None
    model_profile = load_json(model_profile_path)
    command_template = args.command_template or str(model_profile.get("command_template") or "")
    env, env_audit = build_env(policy)
    execute_gate_allowed, gate_audit = execute_allowed(policy)
    timeout = int(args.timeout_seconds or model_profile.get("timeout_seconds") or policy.get("default_timeout_seconds") or 1800)
    max_output_chars = int(policy.get("max_output_chars") or 120000)

    findings = validate_prompt_path(prompt_path, policy)
    if args.model_profile:
        if not model_profile_path or not model_profile_path.exists():
            findings.append({"level": "blocker", "code": "MODEL_PROFILE_MISSING", "message": f"Model profile not found: {args.model_profile}"})
        elif model_profile.get("status") != "MODEL_PROFILE_SELECTED":
            findings.append({"level": "blocker", "code": "MODEL_PROFILE_NOT_SELECTED", "message": f"Model profile status is {model_profile.get('status')}"})
    if not command_template:
        findings.append({"level": "blocker", "code": "COMMAND_TEMPLATE_MISSING", "message": "Provide --command-template or a selected --model-profile"})
    argv, command_findings = render_argv(command_template, prompt_path, policy)
    findings.extend(command_findings)
    binary_found = shutil.which(argv[0], path=env.get("PATH")) if argv else None
    if args.execute and not execute_gate_allowed:
        findings.append({"level": "blocker", "code": "EXECUTE_GATE_NOT_SET", "message": f"Set {gate_audit['name']}=1 to allow external Codex execution"})
    if args.execute and argv and not binary_found:
        findings.append({"level": "blocker", "code": "COMMAND_BINARY_NOT_FOUND", "message": f"Binary not found on PATH: {argv[0]}"})

    stdout = ""
    stderr = ""
    exit_code: int | None = None
    timed_out = False
    if args.execute and not findings:
        try:
            proc = subprocess.run(
                argv,
                cwd=repo_path("."),
                env=env,
                text=True,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                timeout=timeout,
                check=False,
            )
            exit_code = proc.returncode
            stdout = proc.stdout
            stderr = proc.stderr
        except subprocess.TimeoutExpired as exc:
            timed_out = True
            stdout = exc.stdout if isinstance(exc.stdout, str) else (exc.stdout or b"").decode("utf-8", errors="replace")
            stderr = exc.stderr if isinstance(exc.stderr, str) else (exc.stderr or b"").decode("utf-8", errors="replace")
            findings.append({"level": "blocker", "code": "COMMAND_TIMEOUT", "message": f"Command exceeded timeout {timeout}s"})

    stdout_redacted, stdout_truncated = truncate_text(redact_text(stdout, env), max_output_chars)
    stderr_redacted, stderr_truncated = truncate_text(redact_text(stderr, env), max_output_chars)
    write_text(out_dir / "stdout.txt", stdout_redacted)
    write_text(out_dir / "stderr.txt", stderr_redacted)

    invocation = {
        "run_id": args.run_id,
        "run_kind": "codex_runner",
        "task_id": task.get("id"),
        "task_title": task.get("title"),
        "created_at": datetime.now().isoformat(timespec="seconds"),
        "commit": run_git(["rev-parse", "HEAD"]).strip(),
        "policy": rel_path(policy_path),
        "model_profile": rel_path(model_profile_path) if model_profile_path else None,
        "selected_profile": model_profile.get("selected_profile"),
        "selected_model": model_profile.get("model"),
        "patch_request": rel_path(prompt_path),
        "command_template": command_template,
        "argv": argv,
        "execute_requested": args.execute,
        "execute_gate": gate_audit,
        "timeout_seconds": timeout,
        "binary_found": bool(binary_found),
        "env": env_audit,
        "findings": findings,
        "exit_code": exit_code,
        "stdout_truncated": stdout_truncated,
        "stderr_truncated": stderr_truncated,
        "outputs": [
            "codex-runner/invocation.json",
            "codex-runner/codex-runner-report.md",
            "codex-runner/stdout.txt",
            "codex-runner/stderr.txt",
        ],
    }
    invocation["status"] = status_for(args.execute, findings, exit_code, timed_out)
    write_json(out_dir / "invocation.json", invocation)
    write_text(out_dir / "codex-runner-report.md", render_report(invocation))
    write_json(run_dir / "run-summary.json", invocation)

    print(out_dir)
    print(f"status={invocation['status']} execute={args.execute} findings={len(findings)} exit_code={exit_code}")
    return 0 if invocation["status"] in {"CODEX_RUNNER_PLANNED", "CODEX_RUNNER_COMPLETED"} else 2


if __name__ == "__main__":
    raise SystemExit(main())
