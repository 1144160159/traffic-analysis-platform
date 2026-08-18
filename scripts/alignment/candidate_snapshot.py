#!/usr/bin/env python3
"""Build a deterministic content manifest for an uncommitted remediation candidate."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
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
    "common/config/credentials.yaml",
    "contracts/alignment/evidence-index.json",
    "contracts/alignment/progress-overrides.json",
    "contracts/alignment/remediation-ledger.json",
    "deployments/kubernetes/configmaps/credentials-secret.yaml",
    "go/control-plane/alert-service",
    "go/control-plane/deployments/docker/alert-service",
    "mlops/workflows/mlops-secrets.yaml",
    "rust/probe-agent/probe-agent/config.yaml",
    "web/ui/.env",
    "web/ui/.env.development.local",
    "web/ui/.env.local",
    "web/ui/.env.production.local",
    "web/ui/.env.test.local",
}
PROVENANCE_INDEX = Path("deployments/releases/topic1/m10-artifact-provenance.v1.json")
ACTIVE_MANIFESTS = (
    Path("deployments/kubernetes/applications/go-services.yaml"),
    Path("deployments/kubernetes/applications/probe-agent.yaml"),
    Path("deployments/kubernetes/applications/web-ui.yaml"),
)
BUILD_SELECTORS = (
    Path("Makefile"),
    Path(".github/workflows/mlops-cd.yml"),
)
PREBUILT_RECIPES = {
    "go/control-plane/alert-service": (
        "go/control-plane/deployments/docker/Dockerfile.alert-service.overlay",
    ),
    "go/control-plane/deployments/docker/alert-service": (
        "go/control-plane/deployments/docker/Dockerfile.alert-service.prebuilt.overlay",
    ),
}
IMAGE_PATTERN = re.compile(r"^\s*image:\s*[\"']?([^\s\"']+)", re.MULTILINE)


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


def _probe(root: Path, relative: str) -> dict[str, Any]:
    path = root / relative
    return {
        "path": relative,
        "exists": path.is_file(),
        "sha256": sha256(path) if path.is_file() else None,
    }


def _load_provenance_index(root: Path) -> tuple[dict[str, dict[str, Any]], dict[str, Any]]:
    path = root / PROVENANCE_INDEX
    probe = _probe(root, str(PROVENANCE_INDEX))
    if not path.is_file():
        return {}, probe
    try:
        body = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        probe["parse_status"] = "INVALID"
        return {}, probe
    registrations = body.get("registrations") if isinstance(body, dict) else None
    if not isinstance(registrations, list):
        probe["parse_status"] = "INVALID"
        return {}, probe
    by_ref: dict[str, dict[str, Any]] = {}
    for item in registrations:
        if isinstance(item, dict) and isinstance(item.get("artifact_ref"), str):
            if item["artifact_ref"] in by_ref:
                probe["parse_status"] = "DUPLICATE"
            by_ref[item["artifact_ref"]] = item
    probe.setdefault("parse_status", "PASS")
    return by_ref, probe


def _active_first_party_images(root: Path) -> list[dict[str, str]]:
    found: set[tuple[str, str]] = set()
    for relative in ACTIVE_MANIFESTS:
        path = root / relative
        if not path.is_file():
            continue
        for image in IMAGE_PATTERN.findall(path.read_text(encoding="utf-8")):
            if image.startswith(("traffic/", "docker.io/traffic/", "registry.example.com/traffic/")):
                found.add((str(relative), image))
    return [
        {"manifest_path": manifest, "image_ref": image}
        for manifest, image in sorted(found)
    ]


def _registration_errors(
    root: Path, artifact_ref: str, item: dict[str, Any] | None
) -> list[str]:
    if item is None:
        return ["PROVENANCE_REGISTRATION_REQUIRED"]
    required_strings = (
        "binary_path", "binary_sha256", "source_or_builder_sha", "recipe_or_toolchain",
        "sbom_path", "sbom_sha256", "attestation_path", "attestation_sha256",
        "image_digest", "image_internal_binary_sha256",
    )
    errors = [
        f"REGISTRATION_FIELD_REQUIRED:{field}"
        for field in required_strings
        if not isinstance(item.get(field), str) or not item[field]
    ]
    binary_path = item.get("binary_path")
    binary = root / binary_path if isinstance(binary_path, str) else None
    if binary is None or not binary.is_file():
        errors.append("REGISTERED_BINARY_MISSING")
    elif sha256(binary) != item.get("binary_sha256"):
        errors.append("REGISTERED_BINARY_HASH_MISMATCH")
    for path_field, hash_field in (
        ("sbom_path", "sbom_sha256"),
        ("attestation_path", "attestation_sha256"),
    ):
        relative = item.get(path_field)
        expected = item.get(hash_field)
        path = root / relative if isinstance(relative, str) else None
        if path is None or not path.is_file():
            errors.append(f"REGISTERED_ARTIFACT_MISSING:{path_field}")
        elif sha256(path) != expected:
            errors.append(f"REGISTERED_ARTIFACT_HASH_MISMATCH:{hash_field}")
    if item.get("binary_sha256") != item.get("image_internal_binary_sha256"):
        errors.append("IMAGE_INTERNAL_BINARY_SHA_MISMATCH")
    image_digest = item.get("image_digest")
    if not isinstance(image_digest, str) or not re.fullmatch(r"[^\s]+@sha256:[0-9a-f]{64}", image_digest):
        errors.append("IMMUTABLE_IMAGE_DIGEST_REQUIRED")
    if "@sha256:" in artifact_ref and image_digest != artifact_ref:
        errors.append("DEPLOYED_IMAGE_DIGEST_MISMATCH")
    if not re.fullmatch(r"(?:[0-9a-f]{40}|[0-9a-f]{64})", str(item.get("source_or_builder_sha", ""))):
        errors.append("SOURCE_OR_BUILDER_SHA_INVALID")
    return sorted(set(errors))


def scan_candidate_artifact_provenance(root: Path = ROOT) -> dict[str, Any]:
    """Scan active manifests and excluded prebuilt binaries for exact provenance."""
    registrations, index_probe = _load_provenance_index(root)
    active_images = []
    blockers: set[str] = set()
    for found in _active_first_party_images(root):
        errors = _registration_errors(
            root, found["image_ref"], registrations.get(found["image_ref"])
        )
        active_images.append({**found, "registration_status": "PASS" if not errors else "BLOCKED", "errors": errors})
        blockers.update(errors)

    selector_text = "\n".join(
        (root / path).read_text(encoding="utf-8")
        for path in BUILD_SELECTORS
        if (root / path).is_file()
    )
    excluded_prebuilt = []
    for artifact_path, recipes in PREBUILT_RECIPES.items():
        recipe_refs = [_probe(root, recipe) for recipe in recipes]
        selected = [
            recipe for recipe in recipes
            if recipe in selector_text or Path(recipe).name in selector_text
        ]
        errors = _registration_errors(
            root, artifact_path, registrations.get(artifact_path)
        )
        if not selected:
            errors.append("EXCLUDED_PREBUILT_ACTIVE_SELECTOR_UNPROVEN")
        if not recipes:
            errors.append("EXCLUDED_PREBUILT_RECIPE_REQUIRED")
        errors = sorted(set(errors))
        excluded_prebuilt.append({
            "artifact": _probe(root, artifact_path),
            "recipes": recipe_refs,
            "active_selectors": sorted(selected),
            "registration_status": "PASS" if not errors else "BLOCKED",
            "errors": errors,
        })
        blockers.update(errors)
    if index_probe.get("parse_status") not in (None, "PASS"):
        blockers.add("PROVENANCE_INDEX_INVALID")
    return {
        "schema_version": 1,
        "status": "PASS" if not blockers else "BLOCKED",
        "provenance_index": index_probe,
        "active_manifest_paths": [str(path) for path in ACTIVE_MANIFESTS],
        "build_selector_paths": [str(path) for path in BUILD_SELECTORS],
        "active_first_party_images": active_images,
        "excluded_prebuilt_artifacts": excluded_prebuilt,
        "blocking_codes": sorted(blockers),
    }


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
        "artifact_provenance": scan_candidate_artifact_provenance(ROOT),
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
