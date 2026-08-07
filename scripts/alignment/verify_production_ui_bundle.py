#!/usr/bin/env python3
"""Verify that a deployable Web UI bundle cannot load runtime fixtures or MSW."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_DIST = ROOT / "web/ui/dist"
TEXT_SUFFIXES = {".html", ".js", ".css", ".json"}
FORBIDDEN_PATHS = (
    re.compile(r"(^|/)mockServiceWorker\.js$", re.IGNORECASE),
    re.compile(r"(^|/)browser-[^/]*\.js$", re.IGNORECASE),
    re.compile(r"(^|/)vendor-msw(?:js)?(?:-[^/]*)?\.js$", re.IGNORECASE),
)
FORBIDDEN_CONTENT = (
    "Mock Service Worker",
    "mockServiceWorker.js",
    "msw/passthrough",
    "__msw-cookie-store__",
    "buildMockAlertDetailSnapshot",
    "buildMockCampaignDetailSnapshot",
)
FORBIDDEN_MANIFEST_SOURCE = (
    "/src/mocks/",
    "src/mocks/",
    "/src/services/mockData.ts",
    "src/services/mockData.ts",
    "/node_modules/msw/",
    "/node_modules/@mswjs/",
)


def _sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _tree_sha256(files: list[dict[str, Any]]) -> str:
    digest = hashlib.sha256()
    for item in files:
        digest.update(f"{item['path']}\0{item['sha256']}\0{item['size_bytes']}\n".encode())
    return digest.hexdigest()


def _manifest_strings(value: Any) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, list):
        return [item for entry in value for item in _manifest_strings(entry)]
    if isinstance(value, dict):
        return [
            item
            for key, entry in value.items()
            for item in [str(key), *_manifest_strings(entry)]
        ]
    return []


def verify_bundle(dist: Path = DEFAULT_DIST) -> dict[str, Any]:
    dist = dist.resolve()
    errors: list[str] = []
    if not dist.is_dir():
        return {"status": "FAIL", "dist": str(dist), "errors": ["bundle directory does not exist"]}

    index = dist / "index.html"
    manifest_path = dist / ".vite/manifest.json"
    if not index.is_file():
        errors.append("index.html is missing")
    if not manifest_path.is_file():
        errors.append("Vite production manifest is missing")

    files: list[dict[str, Any]] = []
    forbidden_paths: list[str] = []
    forbidden_tokens: list[dict[str, str]] = []
    for path in sorted(item for item in dist.rglob("*") if item.is_file()):
        relative = path.relative_to(dist).as_posix()
        record = {"path": relative, "sha256": _sha256(path), "size_bytes": path.stat().st_size}
        files.append(record)
        if any(pattern.search(relative) for pattern in FORBIDDEN_PATHS):
            forbidden_paths.append(relative)
        if path.suffix in TEXT_SUFFIXES:
            source = path.read_text(encoding="utf-8", errors="replace")
            for token in FORBIDDEN_CONTENT:
                if token in source:
                    forbidden_tokens.append({"path": relative, "token": token})

    manifest_sources: list[str] = []
    if manifest_path.is_file():
        try:
            manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
            manifest_sources = sorted(
                {
                    value
                    for value in _manifest_strings(manifest)
                    if any(marker in value for marker in FORBIDDEN_MANIFEST_SOURCE)
                }
            )
        except (OSError, json.JSONDecodeError) as error:
            errors.append(f"Vite production manifest is invalid: {error}")

    if forbidden_paths:
        errors.append(f"forbidden mock bundle paths: {forbidden_paths}")
    if forbidden_tokens:
        errors.append(f"forbidden mock runtime markers: {forbidden_tokens}")
    if manifest_sources:
        errors.append(f"forbidden mock source graph: {manifest_sources}")

    index_assets: list[str] = []
    missing_index_assets: list[str] = []
    if index.is_file():
        index_source = index.read_text(encoding="utf-8", errors="replace")
        index_assets = sorted(set(re.findall(r'(?:src|href)="/([^"?#]+)', index_source)))
        missing_index_assets = [asset for asset in index_assets if not (dist / asset).is_file()]
        if missing_index_assets:
            errors.append(f"index references missing assets: {missing_index_assets}")

    return {
        "status": "PASS" if not errors else "FAIL",
        "dist": str(dist),
        "bundle_sha256": _tree_sha256(files),
        "file_count": len(files),
        "size_bytes": sum(int(item["size_bytes"]) for item in files),
        "index_assets": index_assets,
        "missing_index_assets": missing_index_assets,
        "forbidden_paths": forbidden_paths,
        "forbidden_tokens": forbidden_tokens,
        "forbidden_manifest_sources": manifest_sources,
        "files": files,
        "errors": errors,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--dist", type=Path, default=DEFAULT_DIST)
    parser.add_argument("--summary-only", action="store_true")
    args = parser.parse_args()
    result = verify_bundle(args.dist)
    if args.summary_only:
        result = {key: value for key, value in result.items() if key != "files"}
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
