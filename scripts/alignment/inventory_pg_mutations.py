#!/usr/bin/env python3
"""Build and verify the T-PG-002 mutable PostgreSQL command inventory.

The inventory scans SQL inside Go string literals rather than comments or log
messages. It records source-level transaction signals for review; those signals
are deliberately not presented as statement-level proof of atomicity.
"""

from __future__ import annotations

import argparse
import ast
import hashlib
import json
import re
import sys
from collections import Counter
from pathlib import Path
from typing import Any, Iterable


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/postgres/mutable-command-inventory.v1.json"
SCAN_ROOTS = ("go/control-plane/internal", "go/control-plane/cmd")

RAW_STRING = re.compile(r"`([^`]*)`", re.DOTALL)
QUOTED_STRING = re.compile(r'"(?:\\.|[^"\\])*"', re.DOTALL)
MUTATION = re.compile(
    r"\b(INSERT\s+INTO|DELETE\s+FROM|UPDATE)\s+"
    r"([A-Za-z_][A-Za-z0-9_.]*|%s)\b",
    re.IGNORECASE,
)
SQL_KEYWORDS = {"of", "set", "skip"}

# Dynamic table identifiers must be reviewed rather than inferred. Each source
# is tied to its validated runtime allow-list in the corresponding Go code.
DYNAMIC_BACKEND_OVERRIDES: dict[str, dict[str, Any]] = {
    "go/control-plane/internal/alert/api/campaign_projection_targets.go": {
        "backend": "clickhouse",
        "resolved_tables": ["<validated_clickhouse_database>.<validated_clickhouse_table>"],
        "review_basis": "parseCampaignClickHouseTable validates a qualified ClickHouse identifier",
    },
    "go/control-plane/internal/alert/api/handler_campaign_outbox.go": {
        "backend": "postgresql",
        "resolved_tables": ["campaign_aggregate_outbox", "campaign_alert_link_outbox"],
        "review_basis": "campaignOutboxTable returns one of two literal PostgreSQL outbox tables",
    },
}

# Explicit semantic roles are intentionally narrow. They may only be used for
# reviewed, rebuildable derived state whose table name cannot express its role.
TABLE_ROLE_OVERRIDES: dict[str, dict[str, str]] = {
    "dashboard_task_dlq_receipts": {
        "role": "inbox_projection",
        "review_basis": "idempotent source topic partition offset receipt materialized only after the canonical DLQ broker acknowledgement and before source offset commit",
    },
    "graph_hot_ips": {
        "role": "inbox_projection",
        "review_basis": "derived query-frequency cache used only to select graph warmup targets; rebuildable from query telemetry",
    },
}


def go_strings(source: str) -> Iterable[tuple[int, str]]:
    """Yield (source offset, decoded value) for Go raw/interpreted strings."""
    values: list[tuple[int, str]] = []
    for match in RAW_STRING.finditer(source):
        values.append((match.start(), match.group(1)))
    for match in QUOTED_STRING.finditer(source):
        try:
            value = ast.literal_eval(match.group(0))
        except (SyntaxError, ValueError):
            continue
        if isinstance(value, str):
            values.append((match.start(), value))
    yield from sorted(values)


def normalize_verb(value: str) -> str:
    return " ".join(value.upper().split())


def mutation_role(table: str) -> tuple[str, str | None]:
    name = table.rsplit(".", 1)[-1].lower()
    override = TABLE_ROLE_OVERRIDES.get(name)
    if override:
        return override["role"], override["review_basis"]
    if "outbox" in name:
        return "outbox", None
    if name == "audit_logs" or name.startswith("audit_"):
        return "audit", None
    if any(token in name for token in ("_history", "_versions", "_manifest_entries")):
        return "history", None
    if any(token in name for token in ("projection", "_inbox", "_watermarks", "_deliveries")):
        return "inbox_projection", None
    if any(
        token in name
        for token in ("_requests", "_commands", "_control_requests", "_applied_acks")
    ):
        return "control_idempotency", None
    return "business", None


def workload_class(source: str) -> str:
    name = Path(source).name.lower()
    lowered = source.lower()
    if "projection" in name or "/consumer/" in lowered:
        return "projection_consumer"
    if "outbox" in name:
        return "outbox_dispatcher"
    if any(token in name for token in ("scheduler", "worker", "backfill", "async_")):
        return "background_worker"
    return "command_path"


def source_facts(source: str) -> dict[str, bool]:
    return {
        "has_transaction_begin": bool(
            re.search(r"\.(?:BeginTx|BeginTxx|Begin)\s*\(|\bBeginFunc\s*\(", source)
        ),
        "uses_transaction_handle": bool(
            re.search(r"\btx\.(?:Exec|ExecContext|Query|QueryContext|QueryRow|QueryRowContext)\s*\(", source)
        ),
        "has_direct_database_call": bool(
            re.search(
                r"\b(?:db|pgDB|pool|client|clickhouse)\.(?:Exec|ExecContext|Query|QueryContext|QueryRow|QueryRowContext)\s*\(",
                source,
            )
        ),
        "has_audit_signal": bool(
            re.search(
                r"audit_logs|AuditTx|auditTrail|auditWriter|actionAudit|recordWithExecutor|insertAuditWithRunner|insertThreatIntelAudit|insertDashboardTaskPipelineAudit",
                source,
                re.I,
            )
        ),
        "has_history_signal": bool(re.search(r"_history|_versions|HistoryTx", source, re.I)),
        "has_outbox_signal": bool(re.search(r"_outbox|OutboxTx|outbox", source, re.I)),
        "has_idempotency_signal": bool(
            re.search(r"idempotenc|IdempotencyKey|_requests|request_key|operation_id", source, re.I)
        ),
    }


def classify_backend(
    source: str,
    table: str,
    dynamic_overrides: dict[str, dict[str, Any]],
) -> tuple[str, list[str], str | None]:
    if table == "%s":
        override = dynamic_overrides.get(source)
        if not override:
            return "unclassified", [], "dynamic table has no reviewed backend override"
        return (
            str(override["backend"]),
            list(override["resolved_tables"]),
            str(override["review_basis"]),
        )
    if "." not in table:
        return "postgresql", [table], None
    schema = table.split(".", 1)[0].lower()
    if schema == "traffic":
        return "clickhouse", [table], None
    if schema == "public":
        return "postgresql", [table], None
    return "unclassified", [table], f"qualified schema {schema!r} has no backend policy"


def source_paths(root: Path) -> Iterable[Path]:
    paths: set[Path] = set()
    for relative in SCAN_ROOTS:
        base = root / relative
        if base.is_dir():
            paths.update(path for path in base.rglob("*.go") if not path.name.endswith("_test.go"))
    yield from sorted(paths)


def scan_root(
    root: Path = ROOT,
    dynamic_overrides: dict[str, dict[str, Any]] | None = None,
) -> dict[str, Any]:
    overrides = DYNAMIC_BACKEND_OVERRIDES if dynamic_overrides is None else dynamic_overrides
    records: list[dict[str, Any]] = []
    unclassified: list[dict[str, Any]] = []
    mutation_count = 0

    for path in source_paths(root):
        relative = path.relative_to(root).as_posix()
        source = path.read_text(encoding="utf-8")
        mutations: list[dict[str, Any]] = []
        ordinal = 0
        for string_offset, value in go_strings(source):
            for match in MUTATION.finditer(value):
                verb = normalize_verb(match.group(1))
                table = match.group(2)
                if table.lower() in SQL_KEYWORDS:
                    continue
                # PostgreSQL UPDATE requires SET. This rejects error messages
                # such as "failed to update token" while preserving real SQL.
                if verb == "UPDATE" and not re.search(r"\bSET\b", value[match.end() :], re.I):
                    continue
                ordinal += 1
                mutation_count += 1
                line = source.count("\n", 0, string_offset) + value.count("\n", 0, match.start()) + 1
                backend, resolved_tables, review_basis = classify_backend(relative, table, overrides)
                role, role_review_basis = mutation_role(
                    resolved_tables[0] if len(resolved_tables) == 1 else table
                )
                identity = f"{relative}:{line}:{verb}:{table}:{ordinal}"
                item = {
                    "mutation_id": "sql-" + hashlib.sha256(identity.encode("utf-8")).hexdigest()[:16],
                    "line": line,
                    "verb": verb,
                    "table_expression": table,
                    "resolved_tables": resolved_tables,
                    "backend": backend,
                    "role": role,
                    "dynamic_identifier": table == "%s",
                }
                if review_basis:
                    item["backend_review_basis"] = review_basis
                if role_review_basis:
                    item["role_review_basis"] = role_review_basis
                mutations.append(item)
                if backend == "unclassified":
                    unclassified.append(
                        {
                            "source": relative,
                            "line": line,
                            "table_expression": table,
                            "reason": review_basis,
                        }
                    )
        if mutations:
            facts = source_facts(source)
            records.append(
                {
                    "source": relative,
                    "workload_class": workload_class(relative),
                    "source_facts": facts,
                    "mutations": mutations,
                }
            )

    review_queue: list[dict[str, Any]] = []
    for record in records:
        pg_business = [
            item
            for item in record["mutations"]
            if item["backend"] == "postgresql" and item["role"] == "business"
        ]
        if not pg_business:
            continue
        facts = record["source_facts"]
        reasons = []
        if not (facts["has_transaction_begin"] and facts["uses_transaction_handle"]):
            reasons.append("transaction_scope_unproven")
        if not facts["has_audit_signal"]:
            reasons.append("audit_boundary_unproven")
        if not facts["has_outbox_signal"]:
            reasons.append("outbox_boundary_unproven")
        if not facts["has_idempotency_signal"]:
            reasons.append("idempotency_boundary_unproven")
        if not reasons:
            continue
        command_path = record["workload_class"] == "command_path"
        if command_path and "transaction_scope_unproven" in reasons and len(reasons) >= 3:
            priority = "P0_REVIEW"
        elif command_path:
            priority = "P1_REVIEW"
        else:
            priority = "P2_REVIEW"
        review_queue.append(
            {
                "priority": priority,
                "source": record["source"],
                "workload_class": record["workload_class"],
                "business_tables": sorted(
                    {table for item in pg_business for table in item["resolved_tables"]}
                ),
                "business_mutation_count": len(pg_business),
                "reasons": reasons,
                "evidence_limit": "source-level signals only; inspect each command boundary before remediation",
            }
        )

    priority_order = {"P0_REVIEW": 0, "P1_REVIEW": 1, "P2_REVIEW": 2}
    review_queue.sort(key=lambda item: (priority_order[item["priority"]], item["source"]))
    backend_counts = Counter(
        item["backend"] for record in records for item in record["mutations"]
    )
    role_counts = Counter(
        item["role"]
        for record in records
        for item in record["mutations"]
        if item["backend"] == "postgresql"
    )
    priority_counts = Counter(item["priority"] for item in review_queue)

    return {
        "schema_version": 1,
        "contract_id": "postgres.mutable-command-inventory.v1",
        "remediation_id": "T-PG-002",
        "coverage_status": "REPOSITORY_STATIC_INVENTORY",
        "scope": {
            "roots": list(SCAN_ROOTS),
            "language": "Go",
            "excludes": ["*_test.go", "comments", "non-SQL string messages"],
            "classification_rule": "traffic schema is ClickHouse; public or unqualified tables are PostgreSQL; dynamic and other qualified tables require an explicit reviewed override",
            "evidence_limit": "source-level static facts do not prove statement-level transaction atomicity or runtime execution",
        },
        "summary": {
            "source_files": len(records),
            "mutation_statements": mutation_count,
            "backend_counts": dict(sorted(backend_counts.items())),
            "postgres_role_counts": dict(sorted(role_counts.items())),
            "review_queue": len(review_queue),
            "review_priority_counts": dict(sorted(priority_counts.items())),
            "unclassified": len(unclassified),
        },
        "dynamic_backend_overrides": overrides,
        "table_role_overrides": TABLE_ROLE_OVERRIDES,
        "records": records,
        "review_queue": review_queue,
        "unclassified": unclassified,
    }


def compare_snapshot(actual: dict[str, Any], contract_path: Path = CONTRACT) -> dict[str, Any]:
    if not contract_path.is_file():
        return {
            "status": "FAIL",
            "errors": [f"inventory snapshot is missing: {contract_path}"],
            "summary": actual["summary"],
        }
    expected = json.loads(contract_path.read_text(encoding="utf-8"))
    errors: list[str] = []
    if actual["unclassified"]:
        errors.append(f"unclassified SQL mutations: {actual['unclassified']}")
    if actual != expected:
        errors.append("repository mutable-command inventory differs from versioned snapshot")
    return {
        "status": "PASS" if not errors else "FAIL",
        "contract_id": actual["contract_id"],
        "remediation_id": actual["remediation_id"],
        "coverage_status": actual["coverage_status"],
        "summary": actual["summary"],
        "errors": errors,
    }


def write_snapshot(actual: dict[str, Any], contract_path: Path = CONTRACT) -> None:
    if actual["unclassified"]:
        raise ValueError(f"refusing to snapshot unclassified SQL mutations: {actual['unclassified']}")
    contract_path.parent.mkdir(parents=True, exist_ok=True)
    contract_path.write_text(
        json.dumps(actual, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    action = parser.add_mutually_exclusive_group()
    action.add_argument("--check", action="store_true", help="compare the repository with the snapshot")
    action.add_argument("--write-snapshot", action="store_true", help="replace the versioned snapshot")
    parser.add_argument("--contract", type=Path, default=CONTRACT)
    args = parser.parse_args()

    actual = scan_root()
    if args.write_snapshot:
        try:
            write_snapshot(actual, args.contract)
        except ValueError as error:
            print(json.dumps({"status": "FAIL", "error": str(error)}, ensure_ascii=False, indent=2))
            return 1
        print(json.dumps({"status": "WROTE", "path": str(args.contract), "summary": actual["summary"]}, ensure_ascii=False, indent=2))
        return 0
    if args.check:
        result = compare_snapshot(actual, args.contract)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0 if result["status"] == "PASS" else 1
    print(json.dumps(actual, ensure_ascii=False, indent=2, sort_keys=True))
    return 0 if not actual["unclassified"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
