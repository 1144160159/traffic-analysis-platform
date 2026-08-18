#!/usr/bin/env python3
"""Validate the M02 frozen offline PCAP source without scanning siblings."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
MANIFEST = ROOT / "contracts/capture/offline-pcap-input.v1.json"
EXPECTED_FIELDS = {
    "schema_version", "artifact_kind", "dataset_id", "run_id", "base_dir",
    "approval_status", "entries",
}
ENTRY_FIELDS = {
    "entry_id", "relative_path", "sha256", "size_bytes", "byte_order",
    "timestamp_precision", "link_type", "packet_count",
}


def sha256_path(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def safe_relative(value: str, field: str) -> PurePosixPath:
    path = PurePosixPath(value)
    if not value or path.is_absolute() or any(part in {"", ".", ".."} for part in path.parts):
        raise ValueError(f"{field} must be a normalized relative path")
    return path


def validate_manifest(manifest: dict[str, Any]) -> None:
    if set(manifest) != EXPECTED_FIELDS:
        raise ValueError("offline source top-level exact-set drifted")
    if manifest["schema_version"] != "1.0.0" or manifest["artifact_kind"] != "M02_OFFLINE_PCAP_INPUT_MANIFEST":
        raise ValueError("offline source identity drifted")
    if manifest["approval_status"] != "ENGINEERING_REGRESSION_ONLY_PENDING_N015_PROFILE_SIGNATURE":
        raise ValueError("offline source overstated its approval status")
    safe_relative(manifest["base_dir"], "base_dir")
    entries = manifest.get("entries")
    if not isinstance(entries, list) or not entries:
        raise ValueError("offline source entries are required")
    identities: set[str] = set()
    paths: set[str] = set()
    for entry in entries:
        if set(entry) != ENTRY_FIELDS:
            raise ValueError("offline source entry exact-set drifted")
        if not entry["entry_id"] or entry["entry_id"] in identities:
            raise ValueError("offline source entry identity is empty or duplicated")
        identities.add(entry["entry_id"])
        safe_relative(entry["relative_path"], "entry relative_path")
        if entry["relative_path"] in paths:
            raise ValueError("offline source relative_path is duplicated")
        paths.add(entry["relative_path"])
        digest = entry["sha256"]
        if len(digest) != 64 or digest.lower() != digest or any(character not in "0123456789abcdef" for character in digest):
            raise ValueError("offline source sha256 is not lowercase hex")
        if entry["size_bytes"] <= 24 or entry["packet_count"] <= 0:
            raise ValueError("offline source size or packet count is invalid")
        if entry["byte_order"] not in {"little_endian", "big_endian"}:
            raise ValueError("offline source byte order is invalid")
        if entry["timestamp_precision"] not in {"microsecond", "nanosecond"}:
            raise ValueError("offline source timestamp precision is invalid")
        if entry["link_type"] != 1:
            raise ValueError("offline source link type is outside the approved Ethernet fixture")


def verify_fixture_root(manifest: dict[str, Any], fixture_root: Path) -> list[dict[str, Any]]:
    root = fixture_root.resolve(strict=True)
    base = (root / Path(*PurePosixPath(manifest["base_dir"]).parts)).resolve(strict=True)
    if not base.is_dir() or not base.is_relative_to(root):
        raise ValueError("offline source base directory escapes fixture root")
    receipts: list[dict[str, Any]] = []
    for entry in manifest["entries"]:
        candidate = (base / Path(*PurePosixPath(entry["relative_path"]).parts)).resolve(strict=True)
        if not candidate.is_file() or not candidate.is_relative_to(base):
            raise ValueError(f"offline source path escapes base: {entry['entry_id']}")
        if candidate.stat().st_size != entry["size_bytes"]:
            raise ValueError(f"offline source size mismatch: {entry['entry_id']}")
        digest = sha256_path(candidate)
        if digest != entry["sha256"]:
            raise ValueError(f"offline source hash mismatch: {entry['entry_id']}")
        receipts.append({
            "entry_id": entry["entry_id"],
            "path": str(candidate),
            "sha256": digest,
            "size_bytes": candidate.stat().st_size,
        })
    return receipts


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixture-root", type=Path)
    args = parser.parse_args()
    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    validate_manifest(manifest)
    result: dict[str, Any] = {
        "status": "PASS",
        "manifest_sha256": sha256_path(MANIFEST),
        "entry_count": len(manifest["entries"]),
        "runtime_fixture_verified": args.fixture_root is not None,
    }
    if args.fixture_root:
        result["receipts"] = verify_fixture_root(manifest, args.fixture_root)
    print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
