#!/usr/bin/env python3
"""Resolve one structured config locator against a frozen Git candidate."""

from __future__ import annotations

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import subprocess
import sys
import tomllib
from typing import Any

import yaml
from yaml.nodes import MappingNode, Node, ScalarNode, SequenceNode


SOURCE_REL = "scripts/alignment/structured_config_locator.py"
EXPECTED_PYYAML_VERSION = "6.0.3"


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def safe_relative(value: str) -> PurePosixPath:
    path = PurePosixPath(value)
    if not value or path.is_absolute() or any(part in {"", ".", ".."} for part in path.parts):
        raise ValueError(f"path contains an unsafe component: {value}")
    return path


def safe_regular(root: Path, relative: str) -> Path:
    parts = safe_relative(relative).parts
    current = root
    for part in parts:
        current /= part
        try:
            info = current.lstat()
        except FileNotFoundError as exc:
            raise ValueError(f"repository path is missing: {relative}") from exc
        if os.path.islink(current):
            raise ValueError(f"repository path contains a symlink: {relative}")
    if not current.is_file():
        raise ValueError(f"repository path is not a regular file: {relative}")
    return current


def canonical(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def dotted_parts(query: str) -> list[str]:
    parts = query.split(".")
    if not parts or any(not part for part in parts):
        raise ValueError("dotted query contains an empty component")
    return parts


def exact_mapping_value(node: MappingNode, key: str) -> Node | None:
    matches = [value for candidate, value in node.value if isinstance(candidate, ScalarNode) and candidate.value == key]
    if len(matches) > 1:
        raise ValueError(f"YAML mapping repeats key {key!r}")
    return matches[0] if matches else None


def walk_yaml(node: Node) -> list[Node]:
    result = [node]
    if isinstance(node, MappingNode):
        for key, value in node.value:
            result.extend(walk_yaml(key))
            result.extend(walk_yaml(value))
    elif isinstance(node, SequenceNode):
        for child in node.value:
            result.extend(walk_yaml(child))
    return result


def yaml_plain(node: Node) -> Any:
    if isinstance(node, ScalarNode):
        return node.value
    if isinstance(node, SequenceNode):
        return [yaml_plain(child) for child in node.value]
    if isinstance(node, MappingNode):
        return {str(yaml_plain(key)): yaml_plain(value) for key, value in node.value}
    raise ValueError(f"unsupported YAML node: {type(node).__name__}")


def resolve_yaml(source: bytes, query: str) -> tuple[str, str, Any, int, int]:
    decoded = source.decode("utf-8")
    documents = list(yaml.compose_all(decoded))
    nodes = [child for document in documents if document is not None for child in walk_yaml(document)]
    parts = query.split(".", 1)
    matches: list[Node] = []
    strategy = "YAML_EXACT_SCALAR_VALUE"
    if len(parts) == 2 and parts[1].startswith("KAFKA_"):
        workload, env_name = parts
        strategy = "K8S_EXACT_METADATA_NAME_AND_ENV_NAME"
        for node in nodes:
            if not isinstance(node, MappingNode):
                continue
            metadata = exact_mapping_value(node, "metadata")
            if not isinstance(metadata, MappingNode):
                continue
            name = exact_mapping_value(metadata, "name")
            if not isinstance(name, ScalarNode) or name.value != workload:
                continue
            for candidate in walk_yaml(node):
                if not isinstance(candidate, MappingNode):
                    continue
                env_key = exact_mapping_value(candidate, "name")
                if isinstance(env_key, ScalarNode) and env_key.value == env_name:
                    matches.append(candidate)
    else:
        matches = [node for node in nodes if isinstance(node, ScalarNode) and node.value == query]
    if len(matches) != 1:
        raise ValueError(f"expected exactly one structured config match for {query!r}, got {len(matches)}")
    match = matches[0]
    start = len(decoded[: match.start_mark.index].encode("utf-8"))
    end = len(decoded[: match.end_mark.index].encode("utf-8"))
    return strategy, type(match).__name__, yaml_plain(match), start, end


def resolve_toml(source: bytes, query: str) -> tuple[str, str, Any, None, None]:
    current: Any = tomllib.loads(source.decode("utf-8"))
    for part in dotted_parts(query):
        if not isinstance(current, dict) or part not in current:
            raise ValueError(f"expected exactly one structured config match for {query!r}, got 0")
        current = current[part]
    return "TOML_EXACT_DOTTED_PATH", type(current).__name__, current, None, None


def resolve_json(source: bytes, query: str) -> tuple[str, str, Any, None, None]:
    if query == "$DOCUMENT":
        current = json.loads(source)
        return "JSON_WHOLE_DOCUMENT", type(current).__name__, current, None, None
    if not query.startswith("/"):
        raise ValueError("JSON query must be $DOCUMENT or an RFC6901 pointer")
    current: Any = json.loads(source)
    for raw in query.split("/")[1:]:
        part = raw.replace("~1", "/").replace("~0", "~")
        try:
            current = current[int(part)] if isinstance(current, list) else current[part]
        except (KeyError, IndexError, ValueError, TypeError) as exc:
            raise ValueError(f"expected exactly one structured config match for {query!r}, got 0") from exc
    return "JSON_EXACT_RFC6901_POINTER", type(current).__name__, current, None, None


def resolve(args: argparse.Namespace) -> dict[str, Any]:
    if yaml.__version__ != EXPECTED_PYYAML_VERSION:
        raise ValueError(
            f"trusted resolver requires PyYAML {EXPECTED_PYYAML_VERSION}, got {yaml.__version__}"
        )
    root = Path(args.repo_root).resolve()
    source_path = safe_regular(root, args.source)
    manifest_path = safe_regular(root, args.candidate_manifest)
    resolver_path = safe_regular(root, SOURCE_REL)
    manifest_bytes = manifest_path.read_bytes()
    if digest(manifest_bytes) != args.candidate_manifest_sha256:
        raise ValueError("candidate manifest SHA-256 mismatch")
    manifest = json.loads(manifest_bytes)
    if manifest.get("implementation_candidate_commit") != args.candidate_commit:
        raise ValueError("candidate manifest commit mismatch")
    frozen = subprocess.run(
        ["git", "show", f"{args.candidate_commit}:{args.source}"],
        cwd=root,
        check=True,
        capture_output=True,
    ).stdout
    if source_path.read_bytes() != frozen:
        raise ValueError("worktree source differs from frozen candidate")
    if manifest.get("source_blob_sha256", {}).get(args.source) != digest(frozen):
        raise ValueError("candidate source SHA-256 differs from manifest")
    suffix = Path(args.source).suffix.lower()
    if suffix in {".yaml", ".yml"}:
        strategy, kind, value, start, end = resolve_yaml(frozen, args.query)
        config_format = "yaml"
    elif suffix == ".toml":
        strategy, kind, value, start, end = resolve_toml(frozen, args.query)
        config_format = "toml"
    elif suffix == ".json":
        strategy, kind, value, start, end = resolve_json(frozen, args.query)
        config_format = "json"
    else:
        raise ValueError("source must be JSON, YAML, or TOML")
    resolved_at = datetime.fromisoformat(args.resolved_at.replace("Z", "+00:00"))
    if not args.resolved_at.endswith("Z") or resolved_at.utcoffset() != timezone.utc.utcoffset(resolved_at):
        raise ValueError("resolved-at must be RFC3339 UTC ending in Z")
    semantic = canonical(value)
    locator: dict[str, Any] = {
        "locator_id": args.locator_id,
        "format": config_format,
        "path": args.source,
        "query": args.query,
        "match_strategy": strategy,
        "node_kind": kind,
        "candidate_blob_sha256": digest(frozen),
        "semantic_value_sha256": digest(semantic),
        "semantic_value": value,
        "start": None,
        "end": None,
        "source_span_sha256": None,
    }
    if start is not None and end is not None:
        locator["start"] = {"byte_offset": start}
        locator["end"] = {"byte_offset": end}
        locator["source_span_sha256"] = digest(frozen[start:end])
    return {
        "schema_version": "1.0.0",
        "artifact_kind": "STRUCTURED_CONFIG_LOCATOR_RESOLUTION_RECEIPT",
        "status": "RESOLVED",
        "proof_level": "STRUCTURED_PARSE_TREE",
        "candidate": {
            "commit": args.candidate_commit,
            "manifest_path": args.candidate_manifest,
            "manifest_sha256": args.candidate_manifest_sha256,
        },
        "resolver": {
            "resolver_id": "traffic-structured-config-locator@1",
            "engine": "tomllib/json/PyYAML",
            "engine_version": f"python-{sys.version.split()[0]}; PyYAML-{yaml.__version__}",
            "source_path": SOURCE_REL,
            "source_sha256": digest(resolver_path.read_bytes()),
        },
        "locator": locator,
        "ambiguity_count": 1,
        "resolved_at": args.resolved_at,
        "proof_ceiling": "EXACT_LOCATOR_ONLY_NOT_CONFIG_DESIGN_DEPLOYMENT_OR_EXECUTION_AUTHORIZATION",
    }


def safe_write(root: Path, relative: str, encoded: bytes) -> None:
    parts = safe_relative(relative).parts
    current = root
    for part in parts[:-1]:
        current /= part
        if current.exists() and current.is_symlink():
            raise ValueError("output path contains a symlink")
        current.mkdir(exist_ok=True)
    output = current / parts[-1]
    try:
        output.lstat()
    except FileNotFoundError:
        output.open("xb").write(encoded)
        return
    else:
        if output.is_symlink() or not output.is_file():
            raise ValueError("output path is not a regular file")
        if output.read_bytes() != encoded:
            raise ValueError("immutable output already exists with different bytes")
        return


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True)
    parser.add_argument("--query", required=True)
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
    if args.output:
        safe_write(Path(args.repo_root).resolve(), args.output, encoded)
    else:
        sys.stdout.buffer.write(encoded)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
