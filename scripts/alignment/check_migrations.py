#!/usr/bin/env python3
"""Prevent new runtime DDL and validate remediation migration ordering."""

from __future__ import annotations

import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
BASELINE = ROOT / "contracts/alignment/runtime-ddl-baseline.json"
MIGRATIONS = ROOT / "deployments/postgres/migrations"
CLICKHOUSE_MIGRATIONS = ROOT / "deployments/clickhouse/migrations"
DDL_PATTERN = re.compile(
    r"\bCREATE\s+TABLE\b"
    r"|\bDROP\s+TABLE\b"
    r"|\bALTER\s+TABLE\s+(?![A-Za-z0-9_.\"]+\s+DELETE\b)",
    re.IGNORECASE,
)
INIT_SCHEMA_CALL_PATTERN = re.compile(r"\.InitSchema\s*\(")


def main() -> int:
    baseline = json.loads(BASELINE.read_text(encoding="utf-8"))
    allowed = baseline["allowed_runtime_ddl"]
    runtime_ddl: dict[str, int] = {}
    for path in (ROOT / "go/control-plane").rglob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        relative = path.relative_to(ROOT).as_posix()
        count = len(DDL_PATTERN.findall(path.read_text(encoding="utf-8", errors="ignore")))
        if count:
            runtime_ddl[relative] = count
    new_runtime_ddl = {
        path: count
        for path, count in runtime_ddl.items()
        if count > int(allowed.get(path, 0))
    }
    migration_files = sorted(MIGRATIONS.glob("*.sql")) if MIGRATIONS.exists() else []
    versions = [path.name.split("_", 1)[0] for path in migration_files]
    clickhouse_migration_files = (
        sorted(CLICKHOUSE_MIGRATIONS.glob("*.sql"))
        if CLICKHOUSE_MIGRATIONS.exists()
        else []
    )
    clickhouse_versions = [
        path.name.split("_", 1)[0] for path in clickhouse_migration_files
    ]
    startup_schema_calls: list[str] = []
    for path in (ROOT / "go/control-plane/cmd").rglob("*.go"):
        if path.name.endswith("_test.go"):
            continue
        relative = path.relative_to(ROOT).as_posix()
        for line_number, line in enumerate(
            path.read_text(encoding="utf-8", errors="ignore").splitlines(), start=1
        ):
            if INIT_SCHEMA_CALL_PATTERN.search(line):
                startup_schema_calls.append(f"{relative}:{line_number}")
    errors: list[str] = []
    if len(versions) != len(set(versions)):
        errors.append("migration versions must be unique")
    if versions != sorted(versions):
        errors.append("migration versions must sort lexically")
    if len(clickhouse_versions) != len(set(clickhouse_versions)):
        errors.append("ClickHouse migration versions must be unique")
    if clickhouse_versions != sorted(clickhouse_versions):
        errors.append("ClickHouse migration versions must sort lexically")
    if not clickhouse_migration_files:
        errors.append("ClickHouse migration directory must not be empty")
    else:
        bootstrap = clickhouse_migration_files[0]
        if bootstrap.name != "202607300000_schema_authority.sql":
            errors.append(
                "ClickHouse schema authority bootstrap must be the first migration"
            )
        bootstrap_sql = bootstrap.read_text(encoding="utf-8", errors="ignore")
        for token in (
            "traffic.alignment_schema_migrations_local",
            "checksum      FixedString(64)",
            "ReplicatedReplacingMergeTree",
        ):
            if token not in bootstrap_sql:
                errors.append(
                    f"ClickHouse bootstrap is missing required token: {token}"
                )
        on_cluster_files = []
        for path in clickhouse_migration_files:
            sql = path.read_text(encoding="utf-8", errors="ignore")
            executable_sql = re.sub(r"--[^\n]*", "", sql)
            if re.search(r"\bON\s+CLUSTER\b", executable_sql, re.IGNORECASE):
                on_cluster_files.append(path.relative_to(ROOT).as_posix())
        if on_cluster_files:
            errors.append(
                "ClickHouse migrations are applied directly to each node and must "
                f"not contain ON CLUSTER: {on_cluster_files}"
            )
    if new_runtime_ddl:
        errors.append(f"new runtime DDL is forbidden: {new_runtime_ddl}")
    if startup_schema_calls:
        errors.append(
            "service startup InitSchema calls are forbidden: "
            f"{startup_schema_calls}"
        )
    print(json.dumps({
        "result": "pass" if not errors else "blocked",
        "migration_count": len(migration_files),
        "clickhouse_migration_count": len(clickhouse_migration_files),
        "startup_schema_calls": startup_schema_calls,
        "existing_runtime_ddl_debt": {
            path: count for path, count in sorted(runtime_ddl.items()) if path in allowed
        },
        "errors": errors,
    }, indent=2))
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
