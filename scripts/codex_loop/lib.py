#!/usr/bin/env python3
"""Shared helpers for the Codex Loop Engineering MVP scripts."""

from __future__ import annotations

import json
import os
import re
import hashlib
import subprocess
from datetime import datetime
from pathlib import Path
from typing import Any


DEFAULT_REPO_ROOT = Path(__file__).resolve().parents[2]
SCRIPT_ROOT = Path(os.environ.get("CODEX_LOOP_SCRIPT_ROOT") or Path(__file__).resolve().parent).resolve()
REPO_ROOT = Path(os.environ.get("CODEX_LOOP_REPO_ROOT") or DEFAULT_REPO_ROOT).resolve()
_RUNS_ROOT_RAW = os.environ.get("CODEX_LOOP_RUNS_ROOT")
if _RUNS_ROOT_RAW:
    _runs_root_path = Path(_RUNS_ROOT_RAW)
    RUNS_ROOT = (_runs_root_path if _runs_root_path.is_absolute() else REPO_ROOT / _runs_root_path).resolve()
else:
    RUNS_ROOT = REPO_ROOT / "doc" / "02_acceptance" / "runs"


def repo_path(path: str | Path) -> Path:
    path = Path(path)
    return path if path.is_absolute() else REPO_ROOT / path


def read_text(path: str | Path) -> str:
    return repo_path(path).read_text(encoding="utf-8")


def write_text(path: str | Path, content: str) -> Path:
    target = repo_path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")
    return target


def write_json(path: str | Path, data: dict[str, Any]) -> Path:
    return write_text(path, json.dumps(data, ensure_ascii=False, indent=2) + "\n")


def sha256_file(path: str | Path) -> str:
    target = repo_path(path)
    digest = hashlib.sha256()
    with target.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def rel_path(path: str | Path) -> str:
    target = repo_path(path)
    try:
        return str(target.relative_to(REPO_ROOT))
    except ValueError:
        return str(target)


def run_git(args: list[str]) -> str:
    try:
        proc = subprocess.run(
            ["git", *args],
            cwd=REPO_ROOT,
            check=False,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        )
        return proc.stdout
    except FileNotFoundError:
        return "git-unavailable\n"


def run_command(args: list[str]) -> str:
    proc = subprocess.run(
        args,
        cwd=REPO_ROOT,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
    )
    return proc.stdout


def make_run_id(task_id: str | None = None) -> str:
    stamp = datetime.now().strftime("%Y%m%d-%H%M%S")
    if not task_id:
        return stamp
    slug = re.sub(r"[^a-zA-Z0-9]+", "-", task_id.lower()).strip("-")
    return f"{stamp}-{slug}"


def scalar_to_yaml(value: Any) -> str:
    if value is True:
        return "true"
    if value is False:
        return "false"
    if value is None:
        return "null"
    if isinstance(value, (int, float)):
        return str(value)
    text = str(value)
    if text == "":
        return '""'
    if re.search(r"[:#\[\]\{\},]|^\s|\s$", text):
        return json.dumps(text, ensure_ascii=False)
    return text


def to_yaml(data: Any, indent: int = 0) -> str:
    pad = " " * indent
    lines: list[str] = []
    if isinstance(data, dict):
        for key, value in data.items():
            if isinstance(value, (dict, list)):
                lines.append(f"{pad}{key}:")
                lines.append(to_yaml(value, indent + 2))
            else:
                lines.append(f"{pad}{key}: {scalar_to_yaml(value)}")
    elif isinstance(data, list):
        for value in data:
            if isinstance(value, (dict, list)):
                lines.append(f"{pad}-")
                lines.append(to_yaml(value, indent + 2))
            else:
                lines.append(f"{pad}- {scalar_to_yaml(value)}")
    else:
        lines.append(f"{pad}{scalar_to_yaml(data)}")
    return "\n".join(lines)


def write_yaml(path: str | Path, data: dict[str, Any]) -> Path:
    return write_text(path, to_yaml(data) + "\n")


def _strip_comments(content: str) -> list[tuple[int, str]]:
    rows: list[tuple[int, str]] = []
    for raw in content.splitlines():
        if not raw.strip() or raw.lstrip().startswith("#"):
            continue
        indent = len(raw) - len(raw.lstrip(" "))
        rows.append((indent, raw.strip()))
    return rows


def _parse_scalar(value: str) -> Any:
    value = value.strip()
    if value in {"true", "false"}:
        return value == "true"
    if value == "null":
        return None
    if value == "[]":
        return []
    if value.startswith('"') and value.endswith('"'):
        return json.loads(value)
    if re.fullmatch(r"-?\d+", value):
        return int(value)
    return value


def _parse_block(rows: list[tuple[int, str]], index: int, indent: int) -> tuple[Any, int]:
    if index >= len(rows):
        return {}, index
    is_list = rows[index][1].startswith("- ")
    if is_list:
        result: list[Any] = []
        while index < len(rows) and rows[index][0] == indent and rows[index][1].startswith("- "):
            item = rows[index][1][2:].strip()
            index += 1
            if item:
                result.append(_parse_scalar(item))
            elif index < len(rows) and rows[index][0] > indent:
                nested, index = _parse_block(rows, index, rows[index][0])
                result.append(nested)
            else:
                result.append(None)
        return result, index

    result: dict[str, Any] = {}
    while index < len(rows) and rows[index][0] == indent and not rows[index][1].startswith("- "):
        key, sep, value = rows[index][1].partition(":")
        if not sep:
            raise ValueError(f"Invalid YAML subset line: {rows[index][1]}")
        index += 1
        key = key.strip()
        value = value.strip()
        if value:
            result[key] = _parse_scalar(value)
        elif index < len(rows) and rows[index][0] > indent:
            nested, index = _parse_block(rows, index, rows[index][0])
            result[key] = nested
        else:
            result[key] = None
    return result, index


def load_yaml_subset(path: str | Path) -> dict[str, Any]:
    rows = _strip_comments(read_text(path))
    if not rows:
        return {}
    data, index = _parse_block(rows, 0, rows[0][0])
    if index != len(rows):
        raise ValueError(f"Could not parse all rows from {path}")
    if not isinstance(data, dict):
        raise ValueError(f"Expected mapping at root in {path}")
    return data


def copy_task_snapshot(task_path: str | Path, run_dir: Path) -> Path:
    target = run_dir / "task.yaml"
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(read_text(task_path), encoding="utf-8")
    return target


def ensure_run_dir(run_id: str) -> Path:
    run_dir = RUNS_ROOT / run_id
    for child in ["logs", "screenshots", "sql", "artifacts", "context"]:
        (run_dir / child).mkdir(parents=True, exist_ok=True)
    return run_dir


def bool_contract(task: dict[str, Any], key: str) -> bool:
    contracts = task.get("contracts") or {}
    return bool(contracts.get(key))


def list_of(value: Any) -> list[Any]:
    if value is None:
        return []
    return value if isinstance(value, list) else [value]
