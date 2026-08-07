#!/usr/bin/env python3
"""Build a deterministic content manifest for an uncommitted remediation candidate."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
from typing import Any, Iterable


ROOT = Path(__file__).resolve().parents[2]
SOURCE_ROOTS = (
    ".github/workflows/alignment-contracts.yml",
    "Makefile",
    "common/kafka",
    "common/sql/pg",
    "contracts",
    "deployments/clickhouse",
    "deployments/kubernetes",
    "deployments/postgres",
    "go/control-plane",
    "java/flink-jobs",
    "mlops",
    "proto",
    "rust/probe-agent",
    "scripts/alignment",
    "scripts/clickhouse",
    "tests",
    "web/ui",
)
EXCLUDED_PARTS = {
    ".git",
    ".pytest_cache",
    "__pycache__",
    "bin",
    "coverage",
    "dist",
    "node_modules",
    "target",
    "test-results",
}
EXCLUDED_SUFFIXES = {".log", ".pyc", ".pyo"}
EXCLUDED_PATHS = {
    "contracts/alignment/evidence-index.json",
    "contracts/alignment/progress-overrides.json",
    "contracts/alignment/remediation-ledger.json",
}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _included(path: Path) -> bool:
    relative = path.relative_to(ROOT)
    return (
        relative.as_posix() not in EXCLUDED_PATHS
        and not any(part in EXCLUDED_PARTS for part in relative.parts)
        and path.suffix not in EXCLUDED_SUFFIXES
        and path.is_file()
    )


def source_files() -> Iterable[Path]:
    seen: set[Path] = set()
    for item in SOURCE_ROOTS:
        path = ROOT / item
        candidates = [path] if path.is_file() else path.rglob("*") if path.is_dir() else []
        for candidate in candidates:
            if _included(candidate):
                resolved = candidate.resolve()
                if resolved not in seen:
                    seen.add(resolved)
                    yield candidate


def build_snapshot() -> dict[str, Any]:
    entries = []
    aggregate = hashlib.sha256()
    for path in sorted(source_files(), key=lambda item: item.relative_to(ROOT).as_posix()):
        relative = path.relative_to(ROOT).as_posix()
        item = {
            "path": relative,
            "sha256": sha256(path),
            "size_bytes": path.stat().st_size,
        }
        entries.append(item)
        aggregate.update(relative.encode("utf-8"))
        aggregate.update(b"\0")
        aggregate.update(item["sha256"].encode("ascii"))
        aggregate.update(b"\0")
        aggregate.update(str(item["size_bytes"]).encode("ascii"))
        aggregate.update(b"\n")
    return {
        "schema_version": 1,
        "algorithm": "sha256(path NUL sha256 NUL size LF)",
        "source_roots": list(SOURCE_ROOTS),
        "excluded_path_parts": sorted(EXCLUDED_PARTS),
        "excluded_paths": sorted(EXCLUDED_PATHS),
        "file_count": len(entries),
        "content_sha256": aggregate.hexdigest(),
        "files": entries,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    payload = build_snapshot()
    text = json.dumps(payload, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(text, encoding="utf-8")
    else:
        print(text, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
