#!/usr/bin/env python3
"""Verify default-off and approval-bound asset expand controls."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/alignment/asset-expand-control.v1.json")
CONFIG = Path("go/control-plane/internal/asset/config/config.go")
CANONICAL_DEPLOYMENT = Path("deployments/kubernetes/applications/go-services.yaml")
COMPATIBILITY_DEPLOYMENT = Path("go/control-plane/deployments/kubernetes/asset-service.yaml")
RENDERER = Path("scripts/alignment/render_asset_postgres_expand.py")
EPHEMERAL_G1 = Path("scripts/alignment/verify_asset_expand_ephemeral.py")
RUNBOOK = Path("doc/07_alignment/runbooks/F-ASSET-002-rollback.md")
MIGRATION_DIR = Path("deployments/postgres/migrations")


def read(root: Path, relative: Path) -> str:
    return (root / relative).read_text(encoding="utf-8")


def deployment_env(root: Path, relative: Path) -> dict[str, str | None]:
    for document in yaml.safe_load_all(read(root, relative)):
        if not isinstance(document, dict):
            continue
        if document.get("kind") != "Deployment":
            continue
        if document.get("metadata", {}).get("name") != "asset-service":
            continue
        containers = document["spec"]["template"]["spec"]["containers"]
        container = next(item for item in containers if item.get("name") == "asset-service")
        return {item["name"]: item.get("value") for item in container.get("env", [])}
    raise ValueError(f"asset-service Deployment not found in {relative}")


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    contract = json.loads(read(root, CONTRACT))
    expected_ids = {f"F-ASSET-{index:03d}" for index in range(1, 7)}
    if set(contract.get("remediation_ids", [])) != expected_ids:
        errors.append("asset expand contract must cover F-ASSET-001 through F-ASSET-006")
    if contract.get("status") != "candidate_default_off":
        errors.append("asset expand contract status must remain candidate_default_off")
    if contract.get("production_applied") is not False:
        errors.append("asset expand contract must retain production_applied=false")

    migrations = contract.get("migration_versions", [])
    expected_migration_names = []
    for version in migrations:
        matches = sorted((root / MIGRATION_DIR).glob(f"{version}_*.sql"))
        if len(matches) != 1:
            errors.append(f"asset migration version {version} must resolve to exactly one file")
        else:
            expected_migration_names.append(matches[0].name)
            if f"'{version}'" not in matches[0].read_text(encoding="utf-8"):
                errors.append(f"asset migration {matches[0].name} does not record version {version}")

    renderer_source = read(root, RENDERER)
    for name in expected_migration_names:
        if f'"{name}"' not in renderer_source:
            errors.append(f"asset expand renderer does not bind migration {name}")
    for token in (
        "build_snapshot",
        "postgres_system_identifier",
        "expected_migration_state",
        "independent approver",
        "MAX_WINDOW_SECONDS = 14_400",
        '"suspend": True',
        '"production_mutations": []',
        "actual_bundle_sha",
        'sha256sum "/migrations/$file"',
        "refusing to overwrite rendered migration",
    ):
        if token not in renderer_source:
            errors.append(f"asset expand renderer missing guard: {token}")

    if contract.get("authority", {}).get("ephemeral_g1_verifier") != EPHEMERAL_G1.as_posix():
        errors.append("asset expand contract must bind the isolated G1 verifier")
    ephemeral_source = read(root, EPHEMERAL_G1)
    for token in (
        "POSTGRES_IMAGE",
        "codex_ephemeral_asset_expand_sentinel",
        "ephemeral-only",
        '"persistent_volume_attached": False',
        '"shared_environment_touched": False',
        "for replay in range(1, 3)",
        "schema_fingerprint",
        "refusing to overwrite asset expand G1 evidence",
        'docker", "rm", "-f", container',
    ):
        if token not in ephemeral_source:
            errors.append(f"asset isolated G1 verifier missing guard: {token}")

    default_off_flags = contract.get("default_off_flags", [])
    config_source = read(root, CONFIG)
    for flag in default_off_flags:
        pattern = rf'env:"{re.escape(flag)}"[^`]*envDefault:"false"'
        if not re.search(pattern, config_source):
            errors.append(f"runtime config {flag} must default false")
    if not re.search(r'env:"ASSET_KAFKA_ENABLED"[^`]*envDefault:"true"', config_source):
        errors.append("legacy ASSET_KAFKA_ENABLED must remain true")

    try:
        canonical = deployment_env(root, CANONICAL_DEPLOYMENT)
        compatibility = deployment_env(root, COMPATIBILITY_DEPLOYMENT)
    except (KeyError, StopIteration, ValueError, yaml.YAMLError) as exc:
        errors.append(str(exc))
        canonical = {}
        compatibility = {}

    for flag in default_off_flags:
        if canonical.get(flag) != "false":
            errors.append(f"canonical asset deployment {flag} must be explicit false")
    compatibility_flags = {
        "ASSET_EVENT_OUTBOX_ENABLED",
        "ASSET_DISCOVERY_OUTBOX_ENABLED",
        "ASSET_PROJECTION_ENABLED",
        "ASSET_CURSOR_V2_ENABLED",
        "ASSET_DETAIL_SNAPSHOT_V1_ENABLED",
        "ASSET_DETAIL_CLICKHOUSE_ENABLED",
        "ASSET_DETAIL_NEBULA_ENABLED",
        "ASSET_DETAIL_EVIDENCE_ENABLED",
        "ASSET_GOVERNANCE_V1_ENABLED",
        "ASSET_EXPORT_JOBS_V1_ENABLED",
        "ASSET_EXPORT_WORKER_ENABLED",
        "ASSET_EXPORT_OUTBOX_ENABLED",
    }
    for flag in compatibility_flags:
        if compatibility.get(flag) != "false":
            errors.append(f"compatibility asset deployment {flag} must be explicit false")
    for label, env in (("canonical", canonical), ("compatibility", compatibility)):
        if env.get("ASSET_KAFKA_ENABLED") != "true":
            errors.append(f"{label} asset deployment must preserve ASSET_KAFKA_ENABLED=true")

    if not (root / RUNBOOK).is_file():
        errors.append("F-ASSET-002 rollback runbook is missing")
    else:
        runbook = read(root, RUNBOOK)
        for token in (
            "expand",
            "ASSET_PROJECTION_ENABLED=false",
            "ASSET_EVENT_OUTBOX_ENABLED=false",
            "asset_projection_inbox",
            "T+0",
            "production_applied=false",
        ):
            if token not in runbook:
                errors.append(f"asset rollback runbook missing token: {token}")

    return {
        "status": "PASS" if not errors else "FAIL",
        "errors": errors,
        "remediation_ids": sorted(expected_ids),
        "migration_versions": migrations,
        "default_off_flags": default_off_flags,
        "production_applied": contract.get("production_applied"),
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, indent=2, ensure_ascii=False))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
