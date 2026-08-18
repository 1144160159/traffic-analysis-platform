#!/usr/bin/env python3
"""Shared helpers for the governed MLOps modules.

代码审查 H40 收敛项：`_load_json` 此前在 governed_evaluation / governed_explanation /
model_artifact_governance 三个模块中逐字重复。这里收敛为单一实现，各模块通过
`from governance_common import load_json_object as _load_json` 别名接入，保持
模块内调用点不变，避免后续加固时遗漏副本。
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


def load_json_object(path: str | Path, description: str) -> dict[str, Any]:
    """Load a JSON file and require a JSON object root."""
    target = Path(path)
    if not target.is_file():
        raise FileNotFoundError(f"{description} not found: {target}")
    with target.open("r", encoding="utf-8") as handle:
        value = json.load(handle)
    if not isinstance(value, dict):
        raise ValueError(f"{description} must be a JSON object")
    return value
