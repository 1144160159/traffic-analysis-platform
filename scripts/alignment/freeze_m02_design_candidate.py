#!/usr/bin/env python3
"""Freeze the M02 v4 design candidate from one clean Git HEAD.

This tool creates only the design-candidate source-blob manifest.  It does not
create or validate the implementation candidate's images, SBOMs, attestations,
delivery artifacts, signatures, execution authorization, or registry switch.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
from typing import Any

from build_topic1_task_registry import validate_against_schema


SOURCE_REPO = Path(__file__).resolve().parents[2]
SCHEMA_REL = "contracts/alignment/design-candidate-manifest.schema.json"
LOCATOR_COVERAGE_SCHEMA_REL = "contracts/alignment/m02-code-direct-locator-coverage.schema.json"
LOCATOR_COVERAGE = "contracts/alignment/m02-code-direct-locator-coverage.v1.json"
OUTPUT = "doc/02_acceptance/topic1/m02/candidates/m02-code-direct-v4/design-candidate-manifest.json"
CANDIDATE_ID = "DESIGN-T1-M02-R4"
REQUIRED_SOURCE_COUNT = 163
REQUIRED_SOURCE_EXACT_SET_SHA256 = "5c0716163cf1767ac7c4231499d5a45deaa88a1199c1dff17cf73e9ba4313f26"
HEX40 = re.compile(r"^[0-9a-f]{40}$")


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def semantic_sha256(value: Any) -> str:
    return sha256_bytes(
        json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    )


def run_git(root: Path, *args: str, check: bool = True) -> subprocess.CompletedProcess[bytes]:
    completed = subprocess.run(
        ["git", *args], cwd=root, check=False, stdout=subprocess.PIPE, stderr=subprocess.PIPE
    )
    if check and completed.returncode != 0:
        raise ValueError(
            f"git {' '.join(args)} failed: "
            f"{completed.stderr.decode('utf-8', errors='replace').strip()}"
        )
    return completed


def safe_relative(value: str) -> PurePosixPath:
    path = PurePosixPath(value)
    if (
        not value
        or path.is_absolute()
        or any(part in {"", ".", ".."} for part in path.parts)
        or "\\" in value
    ):
        raise ValueError(f"candidate path contains an unsafe component: {value}")
    return path


def safe_existing_file(root: Path, relative: str) -> Path:
    parts = safe_relative(relative).parts
    current = root
    for part in parts:
        current /= part
        try:
            current.lstat()
        except FileNotFoundError as exc:
            raise ValueError(f"candidate worktree source is missing: {relative}") from exc
        if current.is_symlink():
            raise ValueError(f"candidate worktree source path contains a symlink: {relative}")
    if not current.is_file():
        raise ValueError(f"candidate worktree source is not a regular file: {relative}")
    return current


def required_sources(root: Path) -> list[str]:
    coverage_path = safe_existing_file(root, LOCATOR_COVERAGE)
    coverage = json.loads(coverage_path.read_text(encoding="utf-8"))
    validate_against_schema(coverage, safe_existing_file(root, LOCATOR_COVERAGE_SCHEMA_REL))
    sources = sorted({item["path"] for item in coverage["locator_occurrences"]})
    for source in sources:
        safe_relative(source)
    if len(sources) != REQUIRED_SOURCE_COUNT:
        raise ValueError(
            f"M02 required source count drifted: expected {REQUIRED_SOURCE_COUNT}, got {len(sources)}"
        )
    if semantic_sha256(sources) != REQUIRED_SOURCE_EXACT_SET_SHA256:
        raise ValueError("M02 required source path exact-set hash drifted")
    return sources


def exact_head(root: Path, candidate_commit: str) -> None:
    if not HEX40.fullmatch(candidate_commit):
        raise ValueError("candidate commit must be exactly 40 lowercase hexadecimal characters")
    exists = run_git(root, "cat-file", "-e", f"{candidate_commit}^{{commit}}", check=False)
    if exists.returncode != 0:
        raise ValueError("candidate commit does not exist")
    head = run_git(root, "rev-parse", "--verify", "HEAD^{commit}").stdout.decode("ascii").strip()
    if head != candidate_commit:
        raise ValueError("candidate commit must equal the worktree HEAD commit")


def candidate_blob(root: Path, commit: str, relative: str) -> bytes:
    tree = run_git(root, "ls-tree", "-z", commit, "--", relative)
    records = [item for item in tree.stdout.split(b"\0") if item]
    if len(records) != 1:
        raise ValueError(f"candidate commit does not contain exactly one source blob: {relative}")
    metadata, raw_path = records[0].split(b"\t", 1)
    mode, object_type, _object_id = metadata.decode("ascii").split(" ")
    if raw_path.decode("utf-8") != relative or object_type != "blob" or mode not in {"100644", "100755"}:
        raise ValueError(f"candidate source is not a regular Git blob: {relative}")
    return run_git(root, "cat-file", "blob", f"{commit}:{relative}").stdout


def ensure_clean(root: Path) -> None:
    completed = run_git(
        root,
        "status", "--porcelain=v1", "--untracked-files=all", "--", ".", f":(exclude){OUTPUT}",
    )
    if completed.stdout:
        rows = completed.stdout.decode("utf-8", errors="replace").splitlines()
        preview = ", ".join(rows[:5])
        raise ValueError(
            f"candidate worktree contains uncommitted tracked or untracked changes outside the manifest: {preview}"
        )


def output_target(root: Path, *, create_parents: bool) -> Path:
    parts = safe_relative(OUTPUT).parts
    current = root
    for part in parts[:-1]:
        current /= part
        if current.exists():
            if current.is_symlink() or not current.is_dir():
                raise ValueError("design candidate output parent contains a symlink or non-directory")
        elif create_parents:
            current.mkdir()
        else:
            return root.joinpath(*parts)
    target = current / parts[-1]
    if target.exists() and (target.is_symlink() or not target.is_file()):
        raise ValueError("design candidate output is a symlink or non-regular file")
    if target.is_symlink():
        raise ValueError("design candidate output is a symlink or non-regular file")
    return target


def build(root: Path, candidate_commit: str) -> dict[str, Any]:
    exact_head(root, candidate_commit)
    sources = required_sources(root)
    blobs: dict[str, str] = {}
    for relative in sources:
        frozen = candidate_blob(root, candidate_commit, relative)
        current = safe_existing_file(root, relative).read_bytes()
        if current != frozen:
            raise ValueError(f"candidate worktree source differs from frozen Git blob: {relative}")
        blobs[relative] = sha256_bytes(frozen)
    output_target(root, create_parents=False)
    ensure_clean(root)
    payload = {
        "artifact_kind": "DESIGN_CANDIDATE_MANIFEST",
        "schema_version": "1.0.0",
        "candidate_id": CANDIDATE_ID,
        "implementation_candidate_commit": candidate_commit,
        "scope": "T1-M02 code-direct v4 exact 163-source design candidate",
        "source_blob_sha256": blobs,
        "formal_execution_status": "BLOCKED_UNTIL_SIGNED_OVERLAY",
        "proof_ceiling": "DEVELOPMENT_READINESS_DESIGN_ONLY_NOT_EXECUTION_AUTHORIZATION",
    }
    validate_against_schema(payload, safe_existing_file(root, SCHEMA_REL))
    return payload


def encoded(payload: dict[str, Any]) -> bytes:
    return (json.dumps(payload, ensure_ascii=False, indent=2) + "\n").encode()


def write_immutable(target: Path, body: bytes) -> None:
    try:
        descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        if target.is_symlink() or not target.is_file():
            raise ValueError("design candidate output is a symlink or non-regular file")
        if target.read_bytes() != body:
            raise ValueError("immutable design candidate output already exists with different bytes")
        return
    with os.fdopen(descriptor, "wb") as stream:
        stream.write(body)
        stream.flush()
        os.fsync(stream.fileno())


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo-root", default=".")
    parser.add_argument("--candidate-commit", required=True)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true")
    mode.add_argument("--check", action="store_true")
    args = parser.parse_args()

    root = Path(args.repo_root).resolve()
    if not root.is_dir() or not (root / ".git").exists():
        raise ValueError("repo-root is not a Git worktree root")
    payload = build(root, args.candidate_commit)
    body = encoded(payload)
    target = output_target(root, create_parents=args.write)
    if args.write:
        write_immutable(target, body)
        print(f"WROTE {OUTPUT}")
    else:
        if not target.is_file() or target.is_symlink() or target.read_bytes() != body:
            raise ValueError("M02 design candidate manifest is missing or differs from clean Git HEAD")
        print(f"PASS {OUTPUT}")
    print(f"candidate_commit={args.candidate_commit} source_count={len(payload['source_blob_sha256'])}")
    print(f"PROOF_CEILING {payload['proof_ceiling']}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (ValueError, json.JSONDecodeError, OSError) as error:
        print(error, file=sys.stderr)
        raise SystemExit(1)
