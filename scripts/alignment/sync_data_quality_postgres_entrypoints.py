#!/usr/bin/env python3
"""Synchronize generated T-DQ-001 SQL mirrors from the versioned migration.

The versioned migration remains authoritative.  The common monolith and the
legacy Docker monolith are compatibility entrypoints, so their generated blocks
must be byte-for-byte copies instead of independently maintained DDL.
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MIGRATIONS = (
    Path("deployments/postgres/migrations/202608041400_data_quality_control_plane_v1.sql"),
    Path("deployments/postgres/migrations/202608041500_data_quality_governance_v1.sql"),
    Path("deployments/postgres/migrations/202608041600_data_quality_rule_evaluation_v1.sql"),
    Path("deployments/postgres/migrations/202608041700_data_quality_repair_lifecycle_v1.sql"),
    Path("deployments/postgres/migrations/202608041800_data_quality_replay_projection_v1.sql"),
)
ENTRYPOINTS = (
    Path("common/sql/pg/04-tasks-audit.sql"),
    Path("go/control-plane/deployments/docker/init/postgres_merged.sql"),
)
BEGIN_MARKER = (
    "-- BEGIN GENERATED T-DQ-001 DATA QUALITY CONTROL PLANE\n"
    "-- Source: deployments/postgres/migrations/202608041400_data_quality_control_plane_v1.sql\n"
    "-- Do not edit this block directly; run scripts/alignment/"
    "sync_data_quality_postgres_entrypoints.py --write.\n"
)
END_MARKER = "-- END GENERATED T-DQ-001 DATA QUALITY CONTROL PLANE"
K8S_ENTRYPOINT = Path("deployments/kubernetes/init-jobs/02-postgres-schema.yaml")
K8S_BEGIN_MARKER = "  # BEGIN GENERATED T-DQ-001 GOVERNANCE\n"
K8S_END_MARKER = "  # END GENERATED T-DQ-001 GOVERNANCE"
K8S_GENERATED = (
    ("22-data-quality-control-plane-v1.sql", MIGRATIONS[0]),
    ("23-data-quality-governance-v1.sql", MIGRATIONS[1]),
    ("24-data-quality-rule-evaluation-v1.sql", MIGRATIONS[2]),
    ("25-data-quality-repair-lifecycle-v1.sql", MIGRATIONS[3]),
    ("26-data-quality-replay-projection-v1.sql", MIGRATIONS[4]),
)
K8S_RUNNER_PREVIOUS = "22-data-quality-control-plane-v1.sql 23-data-quality-governance-v1.sql 24-data-quality-rule-evaluation-v1.sql 25-data-quality-repair-lifecycle-v1.sql 26-data-quality-replay-projection-v1.sql 27-dashboard-task-execution-pipeline-v1.sql 28-dashboard-task-compensation-v1.sql; do"
K8S_RUNNER_CURRENT = "22-data-quality-control-plane-v1.sql 23-data-quality-governance-v1.sql 24-data-quality-rule-evaluation-v1.sql 25-data-quality-repair-lifecycle-v1.sql 26-data-quality-replay-projection-v1.sql 27-dashboard-task-execution-pipeline-v1.sql 28-dashboard-task-compensation-v1.sql 29-dashboard-task-dlq-receipt-v1.sql; do"


def generated_block(root: Path = ROOT) -> str:
    migrations = "\n\n".join(
        (root / path).read_text(encoding="utf-8").rstrip() for path in MIGRATIONS
    )
    return f"{BEGIN_MARKER}{migrations}\n{END_MARKER}\n"


def generated_k8s_block(root: Path = ROOT) -> str:
    entries: list[str] = []
    for key, migration_path in K8S_GENERATED:
        migration = (root / migration_path).read_text(encoding="utf-8").rstrip()
        indented = "\n".join(f"    {line}" if line else "" for line in migration.splitlines())
        entries.append(f"  {key}: |\n{indented}")
    return f"{K8S_BEGIN_MARKER}{'\n'.join(entries)}\n{K8S_END_MARKER}\n"


def synchronize(source: str, block: str) -> str:
    start = source.find(BEGIN_MARKER)
    end = source.find(END_MARKER)
    if (start < 0) != (end < 0):
        raise ValueError("generated block has only one marker")
    if start < 0:
        return source.rstrip() + "\n\n" + block
    if end < start:
        raise ValueError("generated block end precedes begin")
    end += len(END_MARKER)
    while end < len(source) and source[end] in "\r\n":
        end += 1
    return source[:start] + block + source[end:]


def synchronize_k8s(source: str, block: str) -> str:
    start = source.find(K8S_BEGIN_MARKER)
    end = source.find(K8S_END_MARKER)
    if (start < 0) != (end < 0):
        raise ValueError("Kubernetes generated block has only one marker")
    if start < 0:
        document_boundary = source.find("\n---\napiVersion: batch/v1")
        if document_boundary < 0:
            raise ValueError("Kubernetes init Job document boundary is missing")
        source = source[: document_boundary + 1] + block + source[document_boundary + 1 :]
    else:
        if end < start:
            raise ValueError("Kubernetes generated block end precedes begin")
        legacy_control_plane = source.rfind("\n  22-data-quality-control-plane-v1.sql: |", 0, start)
        if legacy_control_plane >= 0:
            start = legacy_control_plane + 1
        end += len(K8S_END_MARKER)
        while end < len(source) and source[end] in "\r\n":
            end += 1
        source = source[:start] + block + source[end:]
    if K8S_RUNNER_CURRENT in source:
        return source
    if K8S_RUNNER_PREVIOUS not in source:
        raise ValueError("Kubernetes init Job runner does not end at the expected migration")
    return source.replace(K8S_RUNNER_PREVIOUS, K8S_RUNNER_CURRENT, 1)


def check(root: Path = ROOT) -> list[str]:
    expected = generated_block(root)
    errors: list[str] = []
    for relative in ENTRYPOINTS:
        source = (root / relative).read_text(encoding="utf-8")
        try:
            rendered = synchronize(source, expected)
        except ValueError as exc:
            errors.append(f"{relative}: {exc}")
            continue
        if rendered != source:
            errors.append(f"{relative}: generated T-DQ-001 block is missing or stale")
    relative = K8S_ENTRYPOINT
    source = (root / relative).read_text(encoding="utf-8")
    try:
        rendered = synchronize_k8s(source, generated_k8s_block(root))
    except ValueError as exc:
        errors.append(f"{relative}: {exc}")
    else:
        if rendered != source:
            errors.append(f"{relative}: generated T-DQ-001 governance block is missing or stale")
    return errors


def write(root: Path = ROOT) -> list[Path]:
    expected = generated_block(root)
    changed: list[Path] = []
    for relative in ENTRYPOINTS:
        path = root / relative
        source = path.read_text(encoding="utf-8")
        rendered = synchronize(source, expected)
        if rendered != source:
            path.write_text(rendered, encoding="utf-8")
            changed.append(relative)
    path = root / K8S_ENTRYPOINT
    source = path.read_text(encoding="utf-8")
    rendered = synchronize_k8s(source, generated_k8s_block(root))
    if rendered != source:
        path.write_text(rendered, encoding="utf-8")
        changed.append(K8S_ENTRYPOINT)
    return changed


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--write", action="store_true")
    args = parser.parse_args()
    if args.check:
        errors = check()
        for error in errors:
            print(error, file=sys.stderr)
        return 1 if errors else 0
    for path in write():
        print(path)
    return 0


if __name__ == "__main__":
    sys.exit(main())
