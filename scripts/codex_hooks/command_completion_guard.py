#!/usr/bin/env python3
"""Codex PostToolUse guard for terminal command completion.

The guard is intentionally fail-open. It never reruns a command and never starts
another Codex process. A Codex ``PostToolUse`` event for canonical tool ``Bash``
is the terminal signal; unified-exec polling does not emit that event until the
original command finishes. For every such event the guard:

1. appends a metadata-only JSONL event under a runtime directory; and
2. injects a short continuation reminder into the next model request.

Raw commands and tool output are never persisted by this script.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable


SCHEMA_VERSION = "command-completion-guard.v1"
COMMAND_TOOL_PATTERN = re.compile(r"^Bash$", re.IGNORECASE)
EXIT_TEXT_PATTERNS = (
    re.compile(r'"exit_code"\s*:\s*(-?\d+)'),
    re.compile(r"\b(?:process|command) exited with (?:exit )?code\s+(-?\d+)\b", re.IGNORECASE),
    re.compile(r"\bexit code\s*[:=]\s*(-?\d+)\b", re.IGNORECASE),
)
EXIT_KEYS = {"exit_code", "exitCode", "returncode", "return_code"}


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def safe_component(value: Any) -> str:
    text = str(value or "unknown")
    return re.sub(r"[^A-Za-z0-9_.-]", "_", text)[:160] or "unknown"


def state_root() -> Path:
    configured = os.environ.get("CODEX_COMMAND_COMPLETION_GUARD_DIR")
    if configured:
        return Path(configured)
    runtime_dir = os.environ.get("XDG_RUNTIME_DIR")
    base = Path(runtime_dir) if runtime_dir else Path("/tmp")
    return base / "codex-command-completion-guard"


def walk_values(value: Any) -> Iterable[tuple[str | None, Any]]:
    if isinstance(value, dict):
        for key, child in value.items():
            yield str(key), child
            yield from walk_values(child)
    elif isinstance(value, list):
        for child in value:
            yield None, child
            yield from walk_values(child)


def response_text(value: Any) -> str:
    if isinstance(value, str):
        return value
    try:
        return json.dumps(value, ensure_ascii=False, separators=(",", ":"))
    except (TypeError, ValueError):
        return str(value)


def classify_response(tool_name: str, response: Any) -> tuple[str, int | None]:
    if not COMMAND_TOOL_PATTERN.fullmatch(tool_name):
        return "ignored", None

    exit_codes: list[int] = []
    for key, value in walk_values(response):
        if key in EXIT_KEYS and isinstance(value, int) and not isinstance(value, bool):
            exit_codes.append(value)

    text = response_text(response)
    for pattern in EXIT_TEXT_PATTERNS:
        exit_codes.extend(int(match.group(1)) for match in pattern.finditer(text))

    # PostToolUse(Bash) is the completion signal. The response shape is
    # tool-specific, so the exit status is useful metadata but not a prerequisite
    # for injecting the continuation reminder.
    return "completed", exit_codes[-1] if exit_codes else None


def append_event(payload: dict[str, Any], outcome: str, exit_code: int | None) -> Path:
    root = state_root()
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    try:
        root.chmod(0o700)
    except OSError:
        pass

    session_id = safe_component(payload.get("session_id"))
    target = root / f"{session_id}.jsonl"
    event = {
        "schema_version": SCHEMA_VERSION,
        "event": "command_completed",
        "recorded_at": utc_now(),
        "session_id": str(payload.get("session_id") or ""),
        "turn_id": str(payload.get("turn_id") or ""),
        "tool_use_id": str(payload.get("tool_use_id") or ""),
        "tool_name": str(payload.get("tool_name") or ""),
        "outcome": outcome,
        "exit_code": exit_code,
    }
    encoded = (json.dumps(event, ensure_ascii=False, sort_keys=True) + "\n").encode("utf-8")
    descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        os.write(descriptor, encoded)
    finally:
        os.close(descriptor)
    return target


def hook_output(additional_context: str | None = None, warning: str | None = None) -> dict[str, Any]:
    output: dict[str, Any] = {"continue": True}
    if additional_context:
        output["hookSpecificOutput"] = {
            "hookEventName": "PostToolUse",
            "additionalContext": additional_context,
        }
    if warning:
        output["systemMessage"] = warning
    return output


def post_tool(payload: dict[str, Any]) -> dict[str, Any]:
    tool_name = str(payload.get("tool_name") or "")
    outcome, exit_code = classify_response(tool_name, payload.get("tool_response"))
    if outcome != "completed":
        return hook_output()

    warning = None
    try:
        append_event(payload, outcome, exit_code)
    except OSError as error:
        warning = f"Command completion guard could not write its metadata journal: {error}"

    exit_label = "unknown" if exit_code is None else str(exit_code)
    context = (
        "COMMAND_COMPLETION_GUARD: 命令工具已明确结束"
        f"（exit_code={exit_label}）。立即解释本次结果、向用户报告成功或失败，并继续当前任务；"
        "不要停在工具结果后，也不要为了确认而重跑已经完成的命令。"
    )
    return hook_output(context, warning)


def read_payload() -> dict[str, Any]:
    value = json.load(sys.stdin)
    if not isinstance(value, dict):
        raise ValueError("hook input must be a JSON object")
    return value


def status(session_id: str | None) -> int:
    root = state_root()
    paths = [root / f"{safe_component(session_id)}.jsonl"] if session_id else sorted(root.glob("*.jsonl"))
    events: list[dict[str, Any]] = []
    for path in paths:
        if not path.is_file():
            continue
        for line in path.read_text(encoding="utf-8").splitlines():
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(event, dict):
                events.append(event)
    print(json.dumps({"schema_version": SCHEMA_VERSION, "events": events}, ensure_ascii=False, indent=2))
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("post-tool", help="Process one PostToolUse hook JSON object from stdin")
    status_parser = subparsers.add_parser("status", help="Print metadata-only completion events")
    status_parser.add_argument("--session-id")
    args = parser.parse_args()

    if args.command == "status":
        return status(args.session_id)

    try:
        payload = read_payload()
        output = post_tool(payload)
    except (json.JSONDecodeError, OSError, ValueError) as error:
        # Hooks must never block the user's task because the guard itself failed.
        output = hook_output(warning=f"Command completion guard failed open: {error}")
    print(json.dumps(output, ensure_ascii=False, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
