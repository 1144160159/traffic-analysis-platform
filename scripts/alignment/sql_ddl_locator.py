#!/usr/bin/env python3
"""Resolve one supported PostgreSQL/ClickHouse DDL declaration in a frozen candidate.

This is deliberately a small fail-closed parser for CREATE TABLE, ALTER TABLE,
and named CONSTRAINT declarations. It does not claim migration safety or full
SQL semantic validation.
"""

from __future__ import annotations

import argparse
from dataclasses import dataclass
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
from typing import Any


SOURCE_REL = "scripts/alignment/sql_ddl_locator.py"
HEX40 = re.compile(r"^[0-9a-f]{40}$")
HEX64 = re.compile(r"^[0-9a-f]{64}$")
IDENTIFIER = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


@dataclass(frozen=True)
class Token:
    text: str
    upper: str
    kind: str
    start: int
    end: int


@dataclass(frozen=True)
class Statement:
    ordinal: int
    tokens: tuple[Token, ...]
    start: int
    end: int
    kind: str | None
    qualified_object: str | None
    constraints: tuple[tuple[str, int, int], ...]


@dataclass(frozen=True)
class Match:
    declaration_kind: str
    qualified_object: str
    statements: tuple[Statement, ...]
    start: int
    end: int


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def canonical(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def safe_relative(value: str) -> PurePosixPath:
    path = PurePosixPath(value)
    if not value or path.is_absolute() or any(part in {"", ".", ".."} for part in path.parts):
        raise ValueError(f"path contains an unsafe component: {value}")
    return path


def safe_regular(root: Path, relative: str) -> Path:
    current = root
    for part in safe_relative(relative).parts:
        current /= part
        try:
            current.lstat()
        except FileNotFoundError as exc:
            raise ValueError(f"repository path is missing: {relative}") from exc
        if current.is_symlink():
            raise ValueError(f"repository path contains a symlink: {relative}")
    if not current.is_file():
        raise ValueError(f"repository path is not a regular file: {relative}")
    return current


def consume_quoted(text: str, start: int, quote: str) -> int:
    index = start + 1
    while index < len(text):
        if text[index] == quote:
            if index + 1 < len(text) and text[index + 1] == quote:
                index += 2
                continue
            return index + 1
        index += 1
    raise ValueError(f"unterminated SQL quoted token at byte {start}")


def consume_dollar_quote(text: str, start: int) -> int | None:
    match = re.match(r"\$[A-Za-z_0-9]*\$", text[start:])
    if match is None:
        return None
    delimiter = match.group(0)
    end = text.find(delimiter, start + len(delimiter))
    if end < 0:
        raise ValueError(f"unterminated SQL dollar quote at byte {start}")
    return end + len(delimiter)


def tokenize(source: bytes) -> list[Token]:
    try:
        text = source.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise ValueError("SQL source must be UTF-8") from exc
    tokens: list[Token] = []
    index = 0
    depth_comment = 0
    punct = set("(),;.")
    while index < len(text):
        char = text[index]
        if char.isspace():
            index += 1
            continue
        if text.startswith("--", index):
            newline = text.find("\n", index + 2)
            index = len(text) if newline < 0 else newline + 1
            continue
        if text.startswith("/*", index):
            depth_comment = 1
            cursor = index + 2
            while cursor < len(text) and depth_comment:
                if text.startswith("/*", cursor):
                    depth_comment += 1; cursor += 2
                elif text.startswith("*/", cursor):
                    depth_comment -= 1; cursor += 2
                else:
                    cursor += 1
            if depth_comment:
                raise ValueError(f"unterminated SQL block comment at byte {index}")
            index = cursor
            continue
        if char == "'":
            end = consume_quoted(text, index, "'")
            tokens.append(Token(text[index:end], text[index:end], "STRING", index, end))
            index = end
            continue
        if char in {'"', '`'}:
            end = consume_quoted(text, index, char)
            value = text[index + 1:end - 1].replace(char * 2, char)
            tokens.append(Token(value, value, "IDENT", index, end))
            index = end
            continue
        if char == "$":
            end = consume_dollar_quote(text, index)
            if end is not None:
                tokens.append(Token(text[index:end], text[index:end], "STRING", index, end))
                index = end
                continue
        if char in punct:
            tokens.append(Token(char, char, "PUNCT", index, index + 1))
            index += 1
            continue
        start = index
        while index < len(text):
            if text[index].isspace() or text[index] in punct or text[index] in {'"', "'", '`'}:
                break
            if text.startswith("--", index) or text.startswith("/*", index):
                break
            index += 1
        if index == start:
            raise ValueError(f"unsupported SQL token at byte {index}")
        value = text[start:index]
        tokens.append(Token(value, value.upper(), "WORD", start, index))
    return tokens


def identifier(tokens: tuple[Token, ...], index: int) -> tuple[str, int]:
    parts: list[str] = []
    while index < len(tokens):
        token = tokens[index]
        if token.kind not in {"WORD", "IDENT"}:
            break
        parts.append(token.text.lower() if token.kind == "WORD" else token.text)
        index += 1
        if index >= len(tokens) or tokens[index].text != ".":
            break
        index += 1
    if not parts:
        raise ValueError("DDL declaration lacks an identifier")
    return ".".join(parts), index


def parse_statement(ordinal: int, tokens: tuple[Token, ...], start: int, end: int) -> Statement:
    kind: str | None = None
    qualified: str | None = None
    cursor = 0
    if len(tokens) >= 2 and tokens[0].upper == "CREATE" and tokens[1].upper == "TABLE":
        cursor = 2
        if len(tokens) >= cursor + 3 and [item.upper for item in tokens[cursor:cursor + 3]] == ["IF", "NOT", "EXISTS"]:
            cursor += 3
        qualified, cursor = identifier(tokens, cursor)
        kind = "CREATE_TABLE"
    elif len(tokens) >= 2 and tokens[0].upper == "ALTER" and tokens[1].upper == "TABLE":
        cursor = 2
        if len(tokens) >= cursor + 2 and [item.upper for item in tokens[cursor:cursor + 2]] == ["IF", "EXISTS"]:
            cursor += 2
        qualified, cursor = identifier(tokens, cursor)
        kind = "ALTER_TABLE"
    constraints: list[tuple[str, int, int]] = []
    for index, token in enumerate(tokens[:-1]):
        if token.upper != "CONSTRAINT":
            continue
        candidate = tokens[index + 1]
        if candidate.kind not in {"WORD", "IDENT"}:
            raise ValueError(f"CONSTRAINT lacks a name in statement {ordinal}")
        constraints.append((candidate.text.lower() if candidate.kind == "WORD" else candidate.text, token.start, candidate.end))
    return Statement(ordinal, tokens, start, end, kind, qualified, tuple(constraints))


def parse_statements(source: bytes) -> list[Statement]:
    tokens = tokenize(source)
    statements: list[Statement] = []
    current: list[Token] = []
    depth = 0
    start = 0
    ordinal = 0
    for token in tokens:
        if token.text == "(": depth += 1
        elif token.text == ")":
            depth -= 1
            if depth < 0: raise ValueError(f"unbalanced SQL parenthesis at byte {token.start}")
        if token.text == ";" and depth == 0:
            if current:
                ordinal += 1
                statements.append(parse_statement(ordinal, tuple(current), start, token.end))
                current = []
            continue
        if not current: start = token.start
        current.append(token)
    if depth != 0: raise ValueError("unbalanced SQL parentheses at end of file")
    if current:
        raise ValueError("SQL migration statement must end with a semicolon")
    return statements


def unqualified(value: str) -> str:
    return value.rsplit(".", 1)[-1]


def resolve_match(statements: list[Statement], query: str, dialect: str) -> Match:
    if not IDENTIFIER.fullmatch(query):
        raise ValueError("SQL locator query must be one unquoted identifier or ALTER_TABLE_<identifier>")
    if dialect == "clickhouse" and not query.startswith("ALTER_TABLE_"):
        raise ValueError("clickhouse locator requires an ALTER_TABLE_ grouped query")
    if query.startswith("ALTER_TABLE_"):
        if dialect != "clickhouse":
            raise ValueError("ALTER_TABLE_ grouped locators are supported only for clickhouse")
        target = query.removeprefix("ALTER_TABLE_").lower()
        candidates = [item for item in statements if item.kind == "ALTER_TABLE" and item.qualified_object and unqualified(item.qualified_object) == target]
        groups: dict[str, list[Statement]] = {}
        for item in candidates: groups.setdefault(item.qualified_object or "", []).append(item)
        if len(groups) != 1:
            raise ValueError(f"expected exactly one SQL DDL declaration for {query!r}, got {len(groups)}")
        qualified, items = next(iter(groups.items()))
        return Match("ALTER_TABLE_GROUP", qualified, tuple(items), items[0].start, items[-1].end)

    declarations: list[Match] = []
    for statement in statements:
        if statement.kind == "CREATE_TABLE" and statement.qualified_object and unqualified(statement.qualified_object) == query.lower():
            declarations.append(Match("TABLE", statement.qualified_object, (statement,), statement.start, statement.end))
        for constraint, start, end in statement.constraints:
            if constraint == query.lower():
                owner = statement.qualified_object or "<unknown-table>"
                declarations.append(Match("CONSTRAINT", owner + "." + constraint, (statement,), start, end))
    if len(declarations) != 1:
        raise ValueError(f"expected exactly one SQL DDL declaration for {query!r}, got {len(declarations)}")
    match = declarations[0]
    return match


def byte_offset(text: str, char_offset: int) -> int:
    return len(text[:char_offset].encode())


def position(text: str, char_offset: int) -> dict[str, int]:
    prefix = text[:char_offset]
    return {
        "byte_offset": len(prefix.encode()),
        "line": prefix.count("\n") + 1,
        "column": len(prefix.rsplit("\n", 1)[-1]) + 1,
    }


def normalized_statement(statement: Statement) -> list[str]:
    return [token.upper if token.kind == "WORD" else token.text for token in statement.tokens]


def resolve(args: argparse.Namespace) -> dict[str, Any]:
    if not HEX40.fullmatch(args.candidate_commit): raise ValueError("candidate commit must be 40 lowercase hex characters")
    if not HEX64.fullmatch(args.candidate_manifest_sha256): raise ValueError("candidate manifest SHA-256 must be 64 lowercase hex characters")
    expected_prefix = f"deployments/{args.dialect}/migrations/"
    if not args.source.startswith(expected_prefix) or not args.source.endswith(".sql"):
        raise ValueError(f"source must be under {expected_prefix} and end in .sql")
    root = Path(args.repo_root).resolve()
    source_path = safe_regular(root, args.source)
    manifest_path = safe_regular(root, args.candidate_manifest)
    resolver_path = safe_regular(root, SOURCE_REL)
    manifest_bytes = manifest_path.read_bytes()
    if digest(manifest_bytes) != args.candidate_manifest_sha256: raise ValueError("candidate manifest SHA-256 mismatch")
    manifest = json.loads(manifest_bytes)
    if manifest.get("implementation_candidate_commit") != args.candidate_commit: raise ValueError("candidate manifest commit mismatch")
    frozen = subprocess.run(["git", "show", f"{args.candidate_commit}:{args.source}"], cwd=root, check=True, capture_output=True).stdout
    if source_path.read_bytes() != frozen: raise ValueError("worktree source differs from frozen candidate")
    if manifest.get("source_blob_sha256", {}).get(args.source) != digest(frozen): raise ValueError("candidate manifest does not bind exact source blob")
    statements = parse_statements(frozen)
    match = resolve_match(statements, args.query, args.dialect)
    text = frozen.decode()
    statement_rows = []
    for statement in match.statements:
        statement_bytes = text[statement.start:statement.end].encode()
        statement_rows.append({
            "ordinal": statement.ordinal,
            "statement_kind": statement.kind,
            "qualified_object": statement.qualified_object,
            "normalized_statement_sha256": digest(canonical(normalized_statement(statement))),
            "source_span_sha256": digest(statement_bytes),
            "start": position(text, statement.start),
            "end": position(text, statement.end),
        })
    resolved = datetime.fromisoformat(args.resolved_at.replace("Z", "+00:00"))
    if not args.resolved_at.endswith("Z") or resolved.utcoffset() != timezone.utc.utcoffset(resolved):
        raise ValueError("resolved-at must be RFC3339 UTC ending in Z")
    parse_tree = {
        "dialect": args.dialect,
        "declaration_kind": match.declaration_kind,
        "qualified_object": match.qualified_object,
        "statements": [normalized_statement(item) for item in match.statements],
    }
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "SQL_DDL_LOCATOR_RESOLUTION_RECEIPT",
        "status": "RESOLVED",
        "proof_level": "DIALECT_DDL_PARSE_TREE",
        "candidate": {"commit": args.candidate_commit, "manifest_path": args.candidate_manifest, "manifest_sha256": args.candidate_manifest_sha256},
        "resolver": {
            "resolver_id": "traffic-sql-ddl-locator@1",
            "engine": "traffic-stdlib-ddl-token-parser",
            "engine_version": "postgres-clickhouse-ddl-v1",
            "source_path": SOURCE_REL,
            "source_sha256": digest(resolver_path.read_bytes()),
        },
        "locator": {
            "locator_id": args.locator_id,
            "language": "sql",
            "dialect": args.dialect,
            "path": args.source,
            "query": args.query,
            "match_strategy": "EXACT_DIALECT_DDL_DECLARATION_OR_ALTER_TABLE_GROUP",
            "declaration_kind": match.declaration_kind,
            "qualified_object": match.qualified_object,
            "statement_count": len(match.statements),
            "statements": statement_rows,
            "candidate_blob_sha256": digest(frozen),
            "source_span_sha256": digest(text[match.start:match.end].encode()),
            "normalized_parse_tree_sha256": digest(canonical(parse_tree)),
            "start": position(text, match.start),
            "end": position(text, match.end),
        },
        "ambiguity_count": 1,
        "resolved_at": args.resolved_at,
        "proof_ceiling": "EXACT_DDL_LOCATOR_ONLY_NOT_MIGRATION_SAFETY_DATABASE_COMPATIBILITY_DEPLOYMENT_OR_EXECUTION_AUTHORIZATION",
    }


def safe_write(root: Path, relative: str, encoded: bytes) -> None:
    parts = safe_relative(relative).parts
    current = root
    for part in parts[:-1]:
        current /= part
        if current.exists() and current.is_symlink(): raise ValueError("output path contains a symlink")
        current.mkdir(exist_ok=True)
    output = current / parts[-1]
    try: output.lstat()
    except FileNotFoundError: output.open("xb").write(encoded)
    else:
        if output.is_symlink() or not output.is_file(): raise ValueError("output path is not a regular file")
        if output.read_bytes() != encoded: raise ValueError("immutable output already exists with different bytes")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True)
    parser.add_argument("--query", required=True)
    parser.add_argument("--dialect", required=True, choices=["postgres", "clickhouse"])
    parser.add_argument("--locator-id", required=True)
    parser.add_argument("--candidate-commit", required=True)
    parser.add_argument("--candidate-manifest", required=True)
    parser.add_argument("--candidate-manifest-sha256", required=True)
    parser.add_argument("--repo-root", default=".")
    parser.add_argument("--resolved-at", required=True)
    parser.add_argument("--output")
    args = parser.parse_args()
    payload = resolve(args)
    encoded = (json.dumps(payload, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()
    if args.output: safe_write(Path(args.repo_root).resolve(), args.output, encoded)
    else: sys.stdout.buffer.write(encoded)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, subprocess.CalledProcessError, json.JSONDecodeError) as error:
        print(error, file=sys.stderr)
        raise SystemExit(1)
