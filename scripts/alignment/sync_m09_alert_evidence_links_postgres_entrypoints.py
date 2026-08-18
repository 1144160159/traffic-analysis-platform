#!/usr/bin/env python3
"""Synchronize the M09 alert-evidence-link migration into PG entrypoints."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
MIGRATION = Path("deployments/postgres/migrations/202608160030_m09_alert_evidence_links_v1.sql")
DOCKER_ENTRYPOINT = Path("go/control-plane/deployments/docker/init/postgres_merged.sql")
K8S_ENTRYPOINT = Path("deployments/kubernetes/init-jobs/02-postgres-schema.yaml")
K8S_KEY = "48-m09-alert-evidence-links-v1.sql"
PREVIOUS_KEY = "47-m08-model-rollback-v2.sql"
BEGIN = "-- BEGIN GENERATED T1-M09 ALERT EVIDENCE LINKS V1\n"
END = "-- END GENERATED T1-M09 ALERT EVIDENCE LINKS V1"
K8S_BEGIN = "  # BEGIN GENERATED T1-M09 ALERT EVIDENCE LINKS V1\n"
K8S_END = "  # END GENERATED T1-M09 ALERT EVIDENCE LINKS V1"


def migration(root: Path = ROOT) -> str:
    return (root / MIGRATION).read_text(encoding="utf-8").rstrip()


def replace_block(source: str, begin: str, end: str, block: str, insertion: int) -> str:
    start, finish = source.find(begin), source.find(end)
    if (start < 0) != (finish < 0):
        raise ValueError("generated block has only one marker")
    if start < 0:
        return source[:insertion] + block + source[insertion:]
    if finish < start:
        raise ValueError("generated block end precedes begin")
    finish += len(end)
    while finish < len(source) and source[finish] in "\r\n":
        finish += 1
    return source[:start] + block + source[finish:]


def render_docker(source: str, root: Path = ROOT) -> str:
    block = f"{BEGIN}{migration(root)}\n{END}\n\n"
    if BEGIN in source:
        return replace_block(source, BEGIN, END, block, 0)
    insertion = len(source.rstrip())
    return source[:insertion] + "\n\n" + block + source[insertion:]


def render_k8s(source: str, root: Path = ROOT) -> str:
    boundary = source.find("\n---\napiVersion: batch/v1")
    if boundary < 0:
        raise ValueError("Kubernetes init Job document boundary is missing")
    body = "\n".join(f"    {line}" if line else "" for line in migration(root).splitlines())
    block = f"{K8S_BEGIN}  {K8S_KEY}: |\n{body}\n{K8S_END}\n"
    rendered = replace_block(source, K8S_BEGIN, K8S_END, block, boundary + 1)
    pattern = re.compile(r"(?m)^(\s*for f in )([^\n]+)(; do)$")
    matches = list(pattern.finditer(rendered))
    if len(matches) != 1:
        raise ValueError("Kubernetes init Job must contain exactly one migration runner")
    match = matches[0]
    names = match.group(2).split()
    if names.count(K8S_KEY) > 1:
        raise ValueError("M09 alert evidence link migration is duplicated")
    if K8S_KEY not in names:
        if not names or names[-1] != PREVIOUS_KEY:
            raise ValueError(f"migration must append after {PREVIOUS_KEY}")
        names.append(K8S_KEY)
    replacement = match.group(1) + " ".join(names) + match.group(3)
    return rendered[: match.start()] + replacement + rendered[match.end() :]


def expected(relative: Path, root: Path = ROOT) -> str:
    source = (root / relative).read_text(encoding="utf-8")
    return render_docker(source, root) if relative == DOCKER_ENTRYPOINT else render_k8s(source, root)


def main() -> int:
    parser = argparse.ArgumentParser()
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--write", action="store_true")
    args = parser.parse_args()
    errors: list[str] = []
    for relative in (DOCKER_ENTRYPOINT, K8S_ENTRYPOINT):
        path = ROOT / relative
        current = path.read_text(encoding="utf-8")
        try:
            rendered = expected(relative)
        except ValueError as exc:
            errors.append(f"{relative}: {exc}")
            continue
        if args.check:
            if current != rendered:
                errors.append(f"{relative}: generated M09 alert evidence link block is missing or stale")
        elif current != rendered:
            path.write_text(rendered, encoding="utf-8")
            print(relative)
    for error in errors:
        print(error, file=sys.stderr)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
