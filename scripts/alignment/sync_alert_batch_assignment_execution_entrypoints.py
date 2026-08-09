#!/usr/bin/env python3
"""Synchronize F-ALERT-004 execution migration compatibility entrypoints."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MIGRATION = Path(
    "deployments/postgres/migrations/202608092130_alert_batch_assignment_execution_v1.sql"
)
DOCKER_ENTRYPOINT = Path("go/control-plane/deployments/docker/init/postgres_merged.sql")
K8S_ENTRYPOINT = Path("deployments/kubernetes/init-jobs/02-postgres-schema.yaml")
BEGIN_MARKER = "-- BEGIN GENERATED F-ALERT-004 ASSIGNMENT EXECUTION V1\n"
END_MARKER = "-- END GENERATED F-ALERT-004 ASSIGNMENT EXECUTION V1"
K8S_BEGIN_MARKER = "  # BEGIN GENERATED F-ALERT-004 ASSIGNMENT EXECUTION V1\n"
K8S_END_MARKER = "  # END GENERATED F-ALERT-004 ASSIGNMENT EXECUTION V1"
K8S_KEY = "35-alert-batch-assignment-execution-v1.sql"
RUNNER_SUFFIX = (
    "34-alert-batch-assignment-v1.sql "
    "35-alert-batch-assignment-execution-v1.sql "
    "36-alert-batch-assignment-compensation-v1.sql; do"
)


def migration_source(root: Path = ROOT) -> str:
    return (root / MIGRATION).read_text(encoding="utf-8").rstrip()


def docker_block(root: Path = ROOT) -> str:
    return f"{BEGIN_MARKER}{migration_source(root)}\n{END_MARKER}\n"


def k8s_block(root: Path = ROOT) -> str:
    indented = "\n".join(
        f"    {line}" if line else "" for line in migration_source(root).splitlines()
    )
    return f"{K8S_BEGIN_MARKER}  {K8S_KEY}: |\n{indented}\n{K8S_END_MARKER}\n"


def synchronize(source: str, begin: str, end_marker: str, block: str, insertion: int) -> str:
    start = source.find(begin)
    end = source.find(end_marker)
    if (start < 0) != (end < 0):
        raise ValueError("generated block has only one marker")
    if start < 0:
        return source[:insertion] + block + source[insertion:]
    if end < start:
        raise ValueError("generated block end precedes begin")
    end += len(end_marker)
    while end < len(source) and source[end] in "\r\n":
        end += 1
    return source[:start] + block + source[end:]


def render_docker(source: str, root: Path = ROOT) -> str:
    if BEGIN_MARKER in source:
        return synchronize(
            source,
            BEGIN_MARKER,
            END_MARKER,
            docker_block(root),
            0,
        )
    insertion = len(source.rstrip())
    prefix = source[:insertion]
    separator = "\n\n" if prefix else ""
    return prefix + separator + docker_block(root) + source[insertion:]


def render_k8s(source: str, root: Path = ROOT) -> str:
    boundary = source.find("\n---\napiVersion: batch/v1")
    if boundary < 0:
        raise ValueError("Kubernetes init Job document boundary is missing")
    rendered = synchronize(
        source,
        K8S_BEGIN_MARKER,
        K8S_END_MARKER,
        k8s_block(root),
        boundary + 1,
    )
    if RUNNER_SUFFIX not in rendered:
        raise ValueError("Kubernetes init Job runner does not include migrations 35 and 36")
    return rendered


def check(root: Path = ROOT) -> list[str]:
    errors: list[str] = []
    for relative, renderer in (
        (DOCKER_ENTRYPOINT, render_docker),
        (K8S_ENTRYPOINT, render_k8s),
    ):
        source = (root / relative).read_text(encoding="utf-8")
        try:
            rendered = renderer(source, root)
        except ValueError as exc:
            errors.append(f"{relative}: {exc}")
            continue
        if rendered != source:
            errors.append(f"{relative}: generated F-ALERT-004 execution block is missing or stale")
    return errors


def write(root: Path = ROOT) -> list[Path]:
    changed: list[Path] = []
    for relative, renderer in (
        (DOCKER_ENTRYPOINT, render_docker),
        (K8S_ENTRYPOINT, render_k8s),
    ):
        path = root / relative
        source = path.read_text(encoding="utf-8")
        rendered = renderer(source, root)
        if rendered != source:
            path.write_text(rendered, encoding="utf-8")
            changed.append(relative)
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
