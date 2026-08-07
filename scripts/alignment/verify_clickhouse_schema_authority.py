#!/usr/bin/env python3
"""Verify T-CH-001 migration authority and enumerate legacy DDL drift."""

from __future__ import annotations

import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/clickhouse/schema-authority.v1.json")
MIGRATION_NAME = re.compile(r"^(\d{12})_([a-z0-9_]+)\.sql$")
DDL_OBJECT = re.compile(
    r"\bCREATE\s+(?:OR\s+REPLACE\s+)?"
    r"(MATERIALIZED\s+VIEW|TABLE|VIEW|DICTIONARY)\s+"
    r"(?:IF\s+NOT\s+EXISTS\s+)?"
    r"(?:(?:traffic|\$\{CH_DB\})\.)?([A-Za-z_][A-Za-z0-9_]*)",
    re.IGNORECASE,
)


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def executable_sql(source: str) -> str:
    source = re.sub(r"/\*.*?\*/", "", source, flags=re.DOTALL)
    return re.sub(r"--[^\n]*", "", source)


def ddl_objects(path: Path) -> list[str]:
    source = executable_sql(path.read_text(encoding="utf-8", errors="ignore"))
    return sorted({match.group(2).lower() for match in DDL_OBJECT.finditer(source)})


def discover_legacy_sources(root: Path) -> set[str]:
    candidates: set[Path] = set((root / "common/sql/ch").glob("*.sql"))
    candidates.update(
        (root / "go/control-plane/deployments/docker/init").glob("clickhouse*.sql")
    )
    java_deployments = root / "java/flink-jobs"
    if java_deployments.exists():
        for path in java_deployments.rglob("*.sql"):
            if "deployments" in path.parts and "clickhouse" in path.as_posix().lower():
                candidates.add(path)
    kubernetes = root / "deployments/kubernetes/init-jobs/03-clickhouse-schema.yaml"
    if kubernetes.exists():
        candidates.add(kubernetes)
    return {
        path.relative_to(root).as_posix()
        for path in candidates
        if DDL_OBJECT.search(executable_sql(path.read_text(encoding="utf-8", errors="ignore")))
    }


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    contract_path = root / CONTRACT
    if not contract_path.is_file():
        return {"status": "FAIL", "errors": [f"missing {CONTRACT.as_posix()}"]}
    contract = json.loads(contract_path.read_text(encoding="utf-8"))

    if contract.get("remediation_id") != "T-CH-001":
        errors.append("contract remediation_id must be T-CH-001")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("repository-only contract must not claim T-CH-001 closure")
    if contract.get("production_applied") is not False:
        errors.append("repository-only contract must not claim production apply")
    if contract.get("execution_model") != "direct_each_node":
        errors.append("ClickHouse execution_model must remain direct_each_node")

    migration_dir = root / str(contract.get("authoritative_migration_directory", ""))
    migrations = sorted(migration_dir.glob("*.sql")) if migration_dir.is_dir() else []
    migration_inventory: list[dict[str, Any]] = []
    versions: list[str] = []
    for path in migrations:
        match = MIGRATION_NAME.fullmatch(path.name)
        if not match:
            errors.append(f"invalid ClickHouse migration filename: {path.name}")
            continue
        version = match.group(1)
        versions.append(version)
        sql = executable_sql(path.read_text(encoding="utf-8", errors="ignore"))
        if re.search(r"\bON\s+CLUSTER\b", sql, re.IGNORECASE):
            errors.append(f"direct_each_node migration contains ON CLUSTER: {path.name}")
        migration_inventory.append(
            {
                "path": path.relative_to(root).as_posix(),
                "version": version,
                "sha256": sha256(path),
                "objects": ddl_objects(path),
            }
        )
    if not migrations:
        errors.append("authoritative ClickHouse migration directory is empty")
    if len(versions) != len(set(versions)):
        errors.append("ClickHouse migration versions must be unique")
    if versions != sorted(versions):
        errors.append("ClickHouse migration versions must sort lexically")
    if not migrations or migrations[0].name != "202607300000_schema_authority.sql":
        errors.append("schema authority bootstrap must be the first migration")

    runner = root / str(contract.get("migration_runner", ""))
    if not runner.is_file():
        errors.append("ClickHouse migration runner is missing")
    else:
        runner_source = runner.read_text(encoding="utf-8", errors="ignore")
        for token in (
            "sha256sum",
            "Checksum mismatch for applied migration",
            "traffic.alignment_schema_migrations_local",
            "--multiquery",
            "CLICKHOUSE_HOSTS",
        ):
            if token not in runner_source:
                errors.append(f"ClickHouse migration runner missing token: {token}")

    legacy_entries = contract.get("legacy_schema_sources", [])
    listed = {str(item.get("path")) for item in legacy_entries}
    discovered = discover_legacy_sources(root)
    if listed != discovered:
        errors.append(
            "legacy ClickHouse source inventory mismatch: "
            f"unlisted={sorted(discovered - listed)} missing={sorted(listed - discovered)}"
        )

    legacy_inventory: list[dict[str, Any]] = []
    object_sets: set[tuple[str, ...]] = set()
    for item in legacy_entries:
        relative = str(item.get("path", ""))
        path = root / relative
        if not path.is_file():
            errors.append(f"legacy ClickHouse source is missing: {relative}")
            continue
        actual_hash = sha256(path)
        if item.get("sha256") != actual_hash:
            errors.append(f"legacy ClickHouse source changed without inventory update: {relative}")
        objects = ddl_objects(path)
        object_sets.add(tuple(objects))
        legacy_inventory.append(
            {
                "path": relative,
                "sha256": actual_hash,
                "disposition": item.get("disposition"),
                "object_count": len(objects),
                "objects": objects,
            }
        )

    drift_detected = len(object_sets) > 1
    if not drift_detected:
        errors.append("expected legacy ClickHouse drift was not detected; review inventory")
    if not contract.get("closure_blockers"):
        errors.append("T-CH-001 closure blockers must remain explicit")

    return {
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": "PARTIAL_MIGRATION_AUTHORITY_AND_DRIFT_INVENTORY",
        "production_applied": False,
        "migration_count": len(migration_inventory),
        "migration_inventory": migration_inventory,
        "legacy_source_count": len(legacy_inventory),
        "legacy_drift_detected": drift_detected,
        "legacy_inventory": legacy_inventory,
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
