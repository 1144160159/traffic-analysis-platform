#!/usr/bin/env python3
"""Validate Kubernetes image references against an explicit evidence lock."""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path


IMAGE_LINE_RE = re.compile(r"^\s*-?\s*image:\s*(?P<value>.+?)\s*(?:#.*)?$")
TEMPLATE_RE = re.compile(r"^\{\{\s*workflow\.parameters\.([A-Za-z0-9_-]+)\s*\}\}$")
SHA_RE = re.compile(r"sha256:[0-9a-fA-F]{64}")


def clean_image_value(value: str) -> str:
    return value.strip().strip('"').strip("'")


def normalize_image_ref(ref: str) -> str:
    if ref.startswith("{{"):
        return ref
    if "@sha256:" in ref:
        ref = ref.split("@sha256:", 1)[0]
    first = ref.split("/", 1)[0]
    has_registry = "." in first or ":" in first or first == "localhost"
    if "/" not in ref:
        return f"docker.io/library/{ref}"
    if not has_registry:
        return f"docker.io/{ref}"
    return ref


def image_tag(ref: str) -> str:
    if "@sha256:" in ref:
        return ""
    leaf = ref.rsplit("/", 1)[-1]
    if ":" not in leaf:
        return ""
    return leaf.rsplit(":", 1)[-1]


def is_mutable_tag(ref: str) -> bool:
    return image_tag(ref) == "latest"


def has_digest_pin(ref: str) -> bool:
    return "@sha256:" in ref


def extract_image_refs(root: Path) -> list[dict]:
    records: list[dict] = []
    for path in sorted(root.rglob("*.yaml")) + sorted(root.rglob("*.yml")):
        if ".archive" in path.parts:
            continue
        for line_no, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            match = IMAGE_LINE_RE.match(line)
            if not match:
                continue
            image = clean_image_value(match.group("value"))
            records.append(
                {
                    "path": str(path),
                    "line": line_no,
                    "image": image,
                    "normalized": normalize_image_ref(image),
                }
            )
    return records


def load_lock(path: Path) -> tuple[dict, dict[str, dict]]:
    lock = json.loads(path.read_text(encoding="utf-8"))
    lookup: dict[str, dict] = {}
    for entry in lock.get("images", []):
        keys = [
            entry.get("reference", ""),
            entry.get("normalized", ""),
            normalize_image_ref(entry.get("reference", "")),
        ]
        keys.extend(entry.get("aliases", []))
        for key in keys:
            if key:
                lookup[key] = entry
                lookup[normalize_image_ref(key)] = entry
    return lock, lookup


def entry_has_evidence(entry: dict | None) -> bool:
    if not entry:
        return False
    repo_digest = entry.get("repo_digest") or ""
    image_id = entry.get("image_id") or ""
    return bool(SHA_RE.search(repo_digest) or SHA_RE.search(image_id))


def write_text_lines(path: Path, lines: list[str]) -> None:
    path.write_text(("".join(f"{line}\n" for line in lines)), encoding="utf-8")


def validate(args: argparse.Namespace) -> int:
    lock, lookup = load_lock(args.lock)
    workflow_parameters = lock.get("workflow_parameters", {})
    inventory = []
    missing = []
    mutable = []

    for record in extract_image_refs(args.root):
        image = record["image"]
        template_match = TEMPLATE_RE.match(image)
        effective_ref = image
        template_parameter = ""
        if template_match:
            template_parameter = template_match.group(1)
            effective_ref = workflow_parameters.get(template_parameter, "")

        status = "digest_pinned"
        evidence = {}
        reason = ""
        if has_digest_pin(image):
            evidence = {"type": "manifest_digest"}
        elif not effective_ref:
            status = "missing_lock"
            reason = f"workflow parameter {template_parameter} has no lock default"
        else:
            entry = lookup.get(effective_ref) or lookup.get(normalize_image_ref(effective_ref))
            if entry_has_evidence(entry):
                status = "lock_covered"
                evidence = {
                    "type": "repo_digest" if entry.get("repo_digest") else "image_id",
                    "lock_reference": entry.get("reference", ""),
                    "repo_digest": entry.get("repo_digest", ""),
                    "image_id": entry.get("image_id", ""),
                    "source": entry.get("source", ""),
                }
            else:
                status = "missing_lock"
                reason = f"{effective_ref} has no repo_digest or image_id in {args.lock}"

        item = dict(record)
        item.update(
            {
                "status": status,
                "effective_reference": effective_ref,
                "effective_normalized": normalize_image_ref(effective_ref) if effective_ref else "",
                "template_parameter": template_parameter,
                "mutable_tag": bool(effective_ref and is_mutable_tag(effective_ref)),
                "evidence": evidence,
                "reason": reason,
            }
        )
        inventory.append(item)

        location = f"{record['path']}:{record['line']}: {image}"
        if status == "missing_lock":
            missing.append(f"{location} -> {reason}")
        if item["mutable_tag"]:
            mutable.append(f"{location} -> covered by lock but tag remains mutable")

    missing_files = sorted({line.split(":", 1)[0] for line in missing})
    summary = {
        "lock_file": str(args.lock),
        "root": str(args.root),
        "image_lines": len(inventory),
        "missing_lock_lines": len(missing),
        "missing_lock_files": len(missing_files),
        "mutable_tag_lines": len(mutable),
        "digest_pinned_lines": sum(1 for item in inventory if item["status"] == "digest_pinned"),
        "lock_covered_lines": sum(1 for item in inventory if item["status"] == "lock_covered"),
    }

    if args.out_inventory:
        args.out_inventory.write_text(json.dumps(inventory, indent=2, ensure_ascii=True), encoding="utf-8")
    if args.out_missing:
        write_text_lines(args.out_missing, missing)
    if args.out_missing_files:
        write_text_lines(args.out_missing_files, missing_files)
    if args.out_mutable:
        write_text_lines(args.out_mutable, mutable)

    print(json.dumps(summary, indent=2, ensure_ascii=True))
    return 1 if missing else 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    validate_parser = subparsers.add_parser("validate")
    validate_parser.add_argument("--root", type=Path, required=True)
    validate_parser.add_argument("--lock", type=Path, required=True)
    validate_parser.add_argument("--out-inventory", type=Path)
    validate_parser.add_argument("--out-missing", type=Path)
    validate_parser.add_argument("--out-missing-files", type=Path)
    validate_parser.add_argument("--out-mutable", type=Path)
    validate_parser.set_defaults(func=validate)

    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
