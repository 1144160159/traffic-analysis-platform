#!/usr/bin/env python3
"""Validate the additive M03 SessionEvent/FeatureStat ClickHouse seam."""

from __future__ import annotations

import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MIGRATION = ROOT / "common/sql/ch/03-m03-session-feature-contract-v1.sql"
BASE_SCHEMA = ROOT / "common/sql/ch/00-all-tables.sql"
K8S_SCHEMA = ROOT / "deployments/kubernetes/init-jobs/03-clickhouse-schema.yaml"
JAVA_ROOTS = (
    ROOT / "java/flink-jobs/flink-session-job/src/main",
    ROOT / "java/flink-jobs/flink-feature-job/src/main",
)

SESSION_COLUMNS = {
    "event_schema_version",
    "aggregate_version",
    "identity_version",
    "session_version",
    "event_time_start_ms",
    "event_time_end_ms",
    "source_watermark_ms",
    "source_event_ids",
    "evidence_ids",
    "completeness",
    "is_partial",
    "missing_fields",
}
FEATURE_COLUMNS = {
    "event_schema_version",
    "aggregate_version",
    "event_time_start_ms",
    "event_time_end_ms",
    "source_watermark_ms",
    "source_event_ids",
    "evidence_ids",
    "feature_category",
    "availability",
    "algorithm_version",
    "window_id",
    "value_unit",
    "is_partial",
    "missing_fields",
    "missing_reason",
}


def fail(message: str) -> None:
    raise SystemExit(f"M03 ClickHouse contract validation failed: {message}")


def alter_body(sql: str, table: str) -> str:
    pattern = re.compile(
        rf"ALTER\s+TABLE\s+traffic\.{re.escape(table)}\s+ON\s+CLUSTER\s+traffic_cluster\s+(.*?);",
        re.IGNORECASE | re.DOTALL,
    )
    matches = pattern.findall(sql)
    if len(matches) != 1:
        fail(f"expected exactly one ALTER statement for traffic.{table}, got {len(matches)}")
    return matches[0]


def added_columns(body: str) -> set[str]:
    if re.search(r"\bADD\s+COLUMN\s+(?!IF\s+NOT\s+EXISTS\b)", body, re.IGNORECASE):
        fail("every expansion must use ADD COLUMN IF NOT EXISTS")
    return set(
        re.findall(r"\bADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\s+([a-z_][a-z0-9_]*)", body, re.IGNORECASE)
    )


def main() -> None:
    migration = MIGRATION.read_text(encoding="utf-8")
    uncommented = "\n".join(line for line in migration.splitlines() if not line.lstrip().startswith("--"))
    forbidden = re.search(
        r"\b(DROP|DELETE|UPDATE|TRUNCATE|RENAME|REPLACE|MODIFY|MATERIALIZE|CREATE)\b",
        uncommented,
        re.IGNORECASE,
    )
    if forbidden:
        fail(f"expand migration contains forbidden operation {forbidden.group(1).upper()}")

    expected = {
        "sessions_local": SESSION_COLUMNS,
        "sessions": SESSION_COLUMNS,
        "feature_stat_local": FEATURE_COLUMNS,
        "feature_stat": FEATURE_COLUMNS,
    }
    for table, columns in expected.items():
        actual = added_columns(alter_body(migration, table))
        if actual != columns:
            fail(f"traffic.{table} columns differ: missing={sorted(columns-actual)}, extra={sorted(actual-columns)}")

    if migration.count("Nullable(Int64) DEFAULT NULL") != 4:
        fail("all four tables must preserve unknown source watermark as Nullable(Int64) DEFAULT NULL")

    base = BASE_SCHEMA.read_text(encoding="utf-8")
    k8s = K8S_SCHEMA.read_text(encoding="utf-8")
    for column in sorted(SESSION_COLUMNS | FEATURE_COLUMNS):
        if column not in base:
            fail(f"base schema omits {column}")
        if column not in k8s:
            fail(f"Kubernetes schema omits {column}")

    java_ddl = []
    for root in JAVA_ROOTS:
        for path in root.rglob("*.java"):
            text = path.read_text(encoding="utf-8")
            if re.search(r"(?:ALTER|CREATE|DROP)\s+TABLE\s+traffic\.", text, re.IGNORECASE):
                java_ddl.append(str(path.relative_to(ROOT)))
    if java_ddl:
        fail(f"Flink runtime DDL is forbidden: {java_ddl}")

    print(
        '{"result":"pass","migration":"common/sql/ch/03-m03-session-feature-contract-v1.sql",'
        '"tables":4,"session_columns":12,"feature_columns":15,"runtime_ddl":0}'
    )


if __name__ == "__main__":
    main()
