#!/usr/bin/env python3
"""Resolve one Python declaration against a frozen candidate AST.

The receipt proves only an exact after-state locator.  It deliberately cannot
grant function-design, execution, test, merge, or production acceptance.
"""

from __future__ import annotations

import argparse
import ast
import copy
import hashlib
import json
from pathlib import Path
import subprocess
import sys
from datetime import datetime, timezone
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
SOURCE_REL = "scripts/alignment/python_ast_locator.py"


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def safe_repo_file(relative: str) -> Path:
    raw = ROOT / relative
    candidate = raw.resolve()
    if (
        Path(relative).is_absolute()
        or ".." in Path(relative).parts
        or not candidate.is_relative_to(ROOT)
        or not candidate.is_file()
    ):
        raise ValueError(f"unsafe or missing repository file: {relative}")
    cursor = ROOT
    for part in Path(relative).parts:
        cursor = cursor / part
        if cursor.is_symlink():
            raise ValueError(f"repository path contains a symlink: {relative}")
    return candidate


def load_manifest(path: str, expected_sha: str, commit: str) -> dict[str, Any]:
    manifest_path = safe_repo_file(path)
    raw = manifest_path.read_bytes()
    if digest(raw) != expected_sha:
        raise ValueError("candidate manifest SHA-256 mismatch")
    payload = json.loads(raw)
    if payload.get("implementation_candidate_commit") != commit:
        raise ValueError("candidate manifest commit mismatch")
    if not isinstance(payload.get("source_blob_sha256"), dict):
        raise ValueError("candidate manifest has no source blob map")
    return payload


def node_symbols(tree: ast.Module) -> list[tuple[str, ast.AST, str]]:
    result: list[tuple[str, ast.AST, str]] = []
    for node in tree.body:
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            kind = "ASYNC_FUNCTION" if isinstance(node, ast.AsyncFunctionDef) else "FUNCTION"
            result.append((node.name, node, kind))
        elif isinstance(node, ast.ClassDef):
            result.append((node.name, node, "CLASS"))
            for child in node.body:
                if isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef)):
                    kind = "ASYNC_METHOD" if isinstance(child, ast.AsyncFunctionDef) else "METHOD"
                    result.append((f"{node.name}.{child.name}", child, kind))
        elif isinstance(node, (ast.Assign, ast.AnnAssign)):
            targets = node.targets if isinstance(node, ast.Assign) else [node.target]
            for target in targets:
                if isinstance(target, ast.Name):
                    kind = "MODULE_CONSTANT" if target.id.isupper() else "MODULE_VARIABLE"
                    result.append((target.id, node, kind))
    return result


def function_signature(node: ast.FunctionDef | ast.AsyncFunctionDef) -> str:
    clone = copy.deepcopy(node)
    clone.body = [ast.Pass()]
    clone.decorator_list = []
    rendered = ast.unparse(ast.fix_missing_locations(clone))
    marker = ":\n    pass"
    if not rendered.endswith(marker):
        raise ValueError("cannot derive deterministic Python signature")
    return rendered[: -len(marker)] + ":"


def declaration_signature(node: ast.AST) -> str:
    if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
        return function_signature(node)
    if isinstance(node, ast.ClassDef):
        bases = ", ".join(ast.unparse(item) for item in node.bases)
        return f"class {node.name}({bases})" if bases else f"class {node.name}"
    if isinstance(node, (ast.Assign, ast.AnnAssign)):
        return ast.unparse(node)
    raise ValueError("unsupported Python declaration kind")


def byte_offsets(source: bytes, node: ast.AST) -> tuple[int, int]:
    lines = source.splitlines(keepends=True)
    start = sum(len(item) for item in lines[: node.lineno - 1]) + node.col_offset
    end = sum(len(item) for item in lines[: node.end_lineno - 1]) + node.end_col_offset
    if not 0 <= start < end <= len(source):
        raise ValueError("invalid Python AST byte span")
    return start, end


def call_name(node: ast.Call) -> str:
    try:
        return ast.unparse(node.func)
    except Exception as exc:  # pragma: no cover - ast.unparse is deterministic here
        raise ValueError("cannot render Python call expression") from exc


def resolve(args: argparse.Namespace) -> dict[str, Any]:
    if not args.source.startswith(("scripts/", "tests/", "ml/", "mlops/")) or not args.source.endswith(".py"):
        raise ValueError("source must be an approved repository-relative Python path")
    if not args.locator_id.startswith("LOC-"):
        raise ValueError("locator ID must start with LOC-")
    manifest = load_manifest(args.candidate_manifest, args.candidate_manifest_sha256, args.candidate_commit)
    source_path = safe_repo_file(args.source)
    current = source_path.read_bytes()
    frozen = subprocess.run(
        ["git", "show", f"{args.candidate_commit}:{args.source}"],
        cwd=ROOT, check=True, capture_output=True,
    ).stdout
    declared = manifest["source_blob_sha256"].get(args.source)
    if current != frozen or declared != digest(frozen):
        raise ValueError("worktree/source manifest differs from frozen candidate")
    tree = ast.parse(frozen, filename=args.source, type_comments=True)
    matches = [(symbol, node, kind) for symbol, node, kind in node_symbols(tree) if symbol == args.symbol]
    if len(matches) != 1:
        raise ValueError(f"expected one exact Python AST match, got {len(matches)}")
    symbol, node, kind = matches[0]
    start, end = byte_offsets(frozen, node)
    calls = sorted(
        ({"expression": call_name(child), "line": child.lineno} for child in ast.walk(node) if isinstance(child, ast.Call)),
        key=lambda item: (item["line"], item["expression"]),
    )
    resolved_at = args.resolved_at or datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    parsed_time = datetime.fromisoformat(resolved_at.replace("Z", "+00:00"))
    if parsed_time.utcoffset() != timezone.utc.utcoffset(parsed_time) or not resolved_at.endswith("Z"):
        raise ValueError("resolved-at must be RFC3339 UTC ending in Z")
    module = args.source[:-3].replace("/", ".")
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "PYTHON_LOCATOR_RESOLUTION_RECEIPT",
        "status": "RESOLVED",
        "proof_level": "LANGUAGE_AST",
        "candidate": {
            "commit": args.candidate_commit,
            "manifest_path": args.candidate_manifest,
            "manifest_sha256": args.candidate_manifest_sha256,
        },
        "resolver": {
            "resolver_id": "traffic-python-ast-locator@1",
            "engine": "python/ast",
            "engine_version": sys.version.split()[0],
            "source_path": SOURCE_REL,
            "source_sha256": digest(safe_repo_file(SOURCE_REL).read_bytes()),
        },
        "locator": {
            "locator_id": args.locator_id,
            "language": "python",
            "path": args.source,
            "module": module,
            "qualified_symbol": symbol,
            "declaration_kind": kind,
            "query": args.symbol,
            "match_strategy": "EXACT_PYTHON_MODULE_OR_CLASS_DECLARATION",
            "signature": declaration_signature(node),
            "candidate_blob_sha256": digest(frozen),
            "source_span_sha256": digest(frozen[start:end]),
            "normalized_ast_sha256": digest(ast.dump(node, include_attributes=False).encode()),
            "start": {"byte_offset": start, "line": node.lineno, "column": node.col_offset + 1},
            "end": {"byte_offset": end, "line": node.end_lineno, "column": node.end_col_offset + 1},
            "calls": calls,
        },
        "ambiguity_count": 1,
        "resolved_at": resolved_at,
        "proof_ceiling": "EXACT_LOCATOR_ONLY_NOT_FUNCTION_DESIGN_OR_EXECUTION_AUTHORIZATION",
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True)
    parser.add_argument("--symbol", required=True)
    parser.add_argument("--locator-id", required=True)
    parser.add_argument("--candidate-commit", required=True)
    parser.add_argument("--candidate-manifest", required=True)
    parser.add_argument("--candidate-manifest-sha256", required=True)
    parser.add_argument("--resolved-at")
    parser.add_argument("--output")
    args = parser.parse_args()
    payload = resolve(args)
    encoded = (json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()
    if not args.output:
        sys.stdout.buffer.write(encoded)
        return 0
    raw_output = ROOT / args.output
    output = raw_output.resolve()
    if Path(args.output).is_absolute() or ".." in Path(args.output).parts or not output.is_relative_to(ROOT):
        raise ValueError("output must be repository-relative")
    output.parent.mkdir(parents=True, exist_ok=True)
    cursor = ROOT
    for part in Path(args.output).parts:
        cursor = cursor / part
        if cursor.is_symlink():
            raise ValueError("output path contains a symlink")
    if output.exists() and output.read_bytes() != encoded:
        raise ValueError("immutable Python locator receipt exists with different bytes")
    if not output.exists():
        output.write_bytes(encoded)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
