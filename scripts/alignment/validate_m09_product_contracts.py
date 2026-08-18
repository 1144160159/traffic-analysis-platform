#!/usr/bin/env python3
"""Validate M09 product contracts against canonical ownership and registry hashes."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any

import jsonschema


ROOT = Path(__file__).resolve().parents[2]
SCHEMA = ROOT / "contracts/alignment/feature-contract.schema.json"
CANONICAL = ROOT / "contracts/alignment/canonical-registry.json"
PACKAGES = ROOT / "contracts/alignment/work-packages.json"
REGISTRY = ROOT / "contracts/alignment/feature-contract-registry.v1.json"
FEATURES = {
    "F-ENCRYPTED-001": {
        "owner": "encrypted-traffic-domain-owner",
        "rollback": "doc/07_alignment/runbooks/F-ENCRYPTED-001-rollback.md",
    },
    "F-ENCRYPTED-002": {
        "owner": "encrypted-traffic-domain-owner",
        "rollback": "doc/07_alignment/runbooks/F-ENCRYPTED-002-rollback.md",
    },
    "F-FORENSICS-001": {
        "owner": "forensics-domain-owner",
        "rollback": "doc/07_alignment/runbooks/F-FORENSICS-001-rollback.md",
    },
}


def load(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def validate() -> dict[str, Any]:
    schema = load(SCHEMA)
    canonical = {item["id"]: item for item in load(CANONICAL)["items"]}
    packages = {item["id"]: item for item in load(PACKAGES)["packages"]}
    registry = load(REGISTRY)
    registry_features = {item["feature_id"]: item for item in registry["features"]}
    observed_hashes: dict[str, str] = {}
    for feature_id, expected in FEATURES.items():
        path = ROOT / f"contracts/alignment/features/{feature_id}.json"
        contract = load(path)
        jsonschema.Draft202012Validator(schema).validate(contract)
        if contract["feature_id"] != feature_id or contract["status"] != "draft":
            raise ValueError(f"{feature_id}: identity or draft status drifted")
        canonical_item = canonical.get(feature_id)
        if not canonical_item:
            raise ValueError(f"{feature_id}: canonical ID missing")
        package = packages[canonical_item["work_package"]]
        if package["accountable"] != expected["owner"]:
            raise ValueError(f"{feature_id}: canonical owner drifted")
        entry = registry_features.get(feature_id) or {}
        formal = entry.get("formal_contract") or {}
        digest = sha256(path)
        if (
            entry.get("accountable") != expected["owner"]
            or entry.get("formal_contract_present") is not True
            or formal.get("path") != f"contracts/alignment/features/{feature_id}.json"
            or formal.get("sha256") != digest
            or formal.get("validation_errors") != []
        ):
            raise ValueError(f"{feature_id}: registry binding drifted")
        if contract["rollout"].get("default") is not False:
            raise ValueError(f"{feature_id}: rollout must remain default-off")
        if contract["rollout"]["rollback_runbook"] != expected["rollback"]:
            raise ValueError(f"{feature_id}: rollback binding drifted")
        if not (ROOT / expected["rollback"]).is_file():
            raise ValueError(f"{feature_id}: rollback runbook missing")
        observed_hashes[feature_id] = digest
    missing_backlog = set(registry["coverage"]["missing_backlog_contracts"])
    if missing_backlog & set(FEATURES):
        raise ValueError("M09 product contracts remain listed as missing backlog")
    if registry["coverage"]["formal_contracts"] != 44:
        raise ValueError("formal contract count is not 44")
    return {
        "schema_version": 1,
        "artifact_kind": "M09_PRODUCT_CONTRACT_K8S_VALIDATION",
        "task_id": "T1-M09-N001",
        "contract_count": len(observed_hashes),
        "contract_sha256": observed_hashes,
        "feature_registry_catalog_sha256": registry["catalog_sha256"],
        "canonical_owner_unique": True,
        "rollouts_default_off": True,
        "production_applied": False,
        "status": "PASS",
    }


def main() -> None:
    print(json.dumps(validate(), sort_keys=True))


if __name__ == "__main__":
    main()
