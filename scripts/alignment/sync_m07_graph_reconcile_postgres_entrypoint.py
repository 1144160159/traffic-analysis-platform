#!/usr/bin/env python3
"""Synchronize the M07 graph reconcile migration into the K8s PostgreSQL init job."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MIGRATION = Path("deployments/postgres/migrations/202608142000_m07_graph_reconcile_v1.sql")
K8S_ENTRYPOINT = Path("deployments/kubernetes/init-jobs/02-postgres-schema.yaml")
K8S_KEY = "42-m07-graph-reconcile-v1.sql"
PREVIOUS_KEY = "41-m07-graph-projection-v1.sql"
BEGIN_MARKER = "  # BEGIN GENERATED T1-M07 GRAPH RECONCILE V1\n"
END_MARKER = "  # END GENERATED T1-M07 GRAPH RECONCILE V1"


def generated_block(root: Path = ROOT) -> str:
    migration = (root / MIGRATION).read_text(encoding="utf-8").rstrip()
    indented = "\n".join(f"    {line}" if line else "" for line in migration.splitlines())
    return f"{BEGIN_MARKER}  {K8S_KEY}: |\n{indented}\n{END_MARKER}\n"


def synchronize(source: str, block: str) -> str:
    start = source.find(BEGIN_MARKER)
    end = source.find(END_MARKER)
    if (start < 0) != (end < 0):
        raise ValueError("M07 graph reconcile Kubernetes generated block has only one marker")
    if start < 0:
        boundary = source.find("\n---\napiVersion: batch/v1")
        if boundary < 0:
            raise ValueError("Kubernetes init Job document boundary is missing")
        source = source[: boundary + 1] + block + source[boundary + 1 :]
    else:
        if end < start:
            raise ValueError("M07 graph reconcile Kubernetes block end precedes begin")
        end += len(END_MARKER)
        while end < len(source) and source[end] in "\r\n":
            end += 1
        source = source[:start] + block + source[end:]

    pattern = re.compile(r"(?m)^(\s*for f in )([^\n]+)(; do)$")
    matches = list(pattern.finditer(source))
    if len(matches) != 1:
        raise ValueError("Kubernetes init Job must contain exactly one migration runner")
    match = matches[0]
    names = match.group(2).split()
    if names.count(K8S_KEY) > 1:
        raise ValueError("M07 graph reconcile migration is duplicated in Kubernetes runner")
    if K8S_KEY not in names:
        if not names or names[-1] != PREVIOUS_KEY:
            raise ValueError(f"M07 graph reconcile migration must append after {PREVIOUS_KEY}")
        names.append(K8S_KEY)
    replacement = match.group(1) + " ".join(names) + match.group(3)
    return source[: match.start()] + replacement + source[match.end() :]


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--write", action="store_true")
    args = parser.parse_args()
    path = ROOT / K8S_ENTRYPOINT
    current = path.read_text(encoding="utf-8")
    try:
        expected = synchronize(current, generated_block())
    except ValueError as exc:
        print(f"{K8S_ENTRYPOINT}: {exc}", file=sys.stderr)
        return 1
    if args.check:
        if current != expected:
            print(f"{K8S_ENTRYPOINT}: generated M07 graph reconcile block is missing or stale", file=sys.stderr)
            return 1
        return 0
    if current != expected:
        path.write_text(expected, encoding="utf-8")
        print(K8S_ENTRYPOINT)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
