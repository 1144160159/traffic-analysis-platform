#!/usr/bin/env python3
"""Fail-closed tests for the PostgreSQL/ClickHouse DDL locator resolver."""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
from typing import Any

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
RESOLVER = REPO / "scripts/alignment/sql_ddl_locator.py"
SCHEMA = REPO / "contracts/alignment/sql-ddl-locator-resolution-receipt.schema.json"
FIXED_TIME = "2026-08-13T12:00:00Z"
PG_SOURCE = """\
-- The text CREATE TABLE fake must not become a declaration.
BEGIN;

CREATE TABLE IF NOT EXISTS pcap_metadata_receipts (
  receipt_id BIGSERIAL PRIMARY KEY,
  aggregate_version BIGINT NOT NULL,
  CONSTRAINT pcap_metadata_receipts_version_positive CHECK (aggregate_version > 0),
  note TEXT NOT NULL DEFAULT 'CONSTRAINT fake_name CHECK (false)'
);

CREATE TABLE IF NOT EXISTS pcap_metadata_outbox (
  outbox_id BIGSERIAL PRIMARY KEY,
  event_id UUID NOT NULL UNIQUE
);

ALTER TABLE probe_operation_outbox
  ADD CONSTRAINT probe_operation_event_projection_event_type_check
  CHECK (event_type IN ('OperationCreated', 'OperationExpired'));

COMMIT;
"""
CH_SOURCE = """\
-- One grouped logical ALTER target spans two safe additive statements.
ALTER TABLE traffic.pcap_index_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS manifest_version UInt16;
ALTER TABLE traffic.pcap_index_local ON CLUSTER traffic_cluster
  ADD COLUMN IF NOT EXISTS source_partition Int32;
"""


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def run(command: list[str], cwd: Path, *, check: bool = True) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(command, cwd=cwd, check=False, capture_output=True, text=True)
    if check and completed.returncode != 0:
        raise AssertionError(
            f"command failed rc={completed.returncode}: {command!r}\n"
            f"stdout={completed.stdout!r}\nstderr={completed.stderr!r}"
        )
    return completed


class Fixture:
    def __init__(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m02-sql-locator-")
        self.root = Path(self.temp.name)
        self.manifest_rel = "candidate/manifest.json"
        self.pg_rel = "deployments/postgres/migrations/202608131000_selftest.sql"
        self.ch_rel = "deployments/clickhouse/migrations/202608131000_selftest.sql"
        self.sources = {self.pg_rel: PG_SOURCE, self.ch_rel: CH_SOURCE}
        for relative, content in self.sources.items():
            path = self.root / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8")
        resolver = self.root / "scripts/alignment/sql_ddl_locator.py"
        resolver.parent.mkdir(parents=True)
        shutil.copy2(RESOLVER, resolver)
        run(["git", "init", "-q"], self.root)
        run(["git", "config", "user.email", "locator@example.invalid"], self.root)
        run(["git", "config", "user.name", "Locator Test"], self.root)
        run(["git", "add", "deployments", "scripts"], self.root)
        run(["git", "commit", "-qm", "frozen SQL candidate"], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()
        self.write_manifest(omit=None)

    def write_manifest(self, *, omit: str | None) -> None:
        blobs = {
            relative: digest((self.root / relative).read_bytes())
            for relative in self.sources
            if relative != omit
        }
        payload: dict[str, Any] = {
            "implementation_candidate_commit": self.commit,
            "source_blob_sha256": blobs,
        }
        path = self.root / self.manifest_rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
        self.manifest_hash = digest(path.read_bytes())

    def close(self) -> None:
        self.temp.cleanup()

    def command(
        self,
        source: str,
        query: str,
        dialect: str,
        *,
        manifest_hash: str | None = None,
        output: str | None = None,
    ) -> list[str]:
        command = [
            "python3", "scripts/alignment/sql_ddl_locator.py",
            "--source", source,
            "--query", query,
            "--dialect", dialect,
            "--locator-id", "LOC-SQL-SELFTEST",
            "--candidate-commit", self.commit,
            "--candidate-manifest", self.manifest_rel,
            "--candidate-manifest-sha256", manifest_hash or self.manifest_hash,
            "--repo-root", str(self.root),
            "--resolved-at", FIXED_TIME,
        ]
        if output is not None:
            command += ["--output", output]
        return command

    def resolve(self, source: str, query: str, dialect: str) -> dict[str, Any]:
        completed = run(self.command(source, query, dialect), self.root)
        payload = json.loads(completed.stdout)
        validate_against_schema(payload, SCHEMA)
        return payload

    def reject(self, label: str, source: str, query: str, dialect: str, expected: str, **kwargs: Any) -> None:
        completed = run(self.command(source, query, dialect, **kwargs), self.root, check=False)
        if completed.returncode == 0:
            raise AssertionError(f"{label}: resolver unexpectedly succeeded")
        if expected not in completed.stderr:
            raise AssertionError(f"{label}: expected {expected!r}, stderr={completed.stderr!r}")

    def commit_source(self, relative: str, content: str, message: str) -> None:
        (self.root / relative).write_text(content, encoding="utf-8")
        self.sources[relative] = content
        run(["git", "add", relative], self.root)
        run(["git", "commit", "-qm", message], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()
        self.write_manifest(omit=None)


def main() -> int:
    fixture = Fixture()
    try:
        table = fixture.resolve(fixture.pg_rel, "pcap_metadata_receipts", "postgres")
        if table["locator"]["declaration_kind"] != "TABLE" or table["locator"]["qualified_object"] != "pcap_metadata_receipts":
            raise AssertionError("PostgreSQL table declaration receipt is incomplete")

        outbox = fixture.resolve(fixture.pg_rel, "pcap_metadata_outbox", "postgres")
        if outbox["locator"]["statement_count"] != 1:
            raise AssertionError("second PostgreSQL table did not resolve exactly")

        constraint = fixture.resolve(
            fixture.pg_rel, "probe_operation_event_projection_event_type_check", "postgres"
        )
        if constraint["locator"]["declaration_kind"] != "CONSTRAINT":
            raise AssertionError("named PostgreSQL constraint did not resolve")

        alter = fixture.resolve(fixture.ch_rel, "ALTER_TABLE_pcap_index_local", "clickhouse")
        if (
            alter["locator"]["declaration_kind"] != "ALTER_TABLE_GROUP"
            or alter["locator"]["statement_count"] != 2
            or alter["locator"]["qualified_object"] != "traffic.pcap_index_local"
        ):
            raise AssertionError("ClickHouse ALTER TABLE group receipt is incomplete")

        ambiguous = PG_SOURCE + "\nCREATE TABLE audit.pcap_metadata_receipts (id BIGINT);\n"
        fixture.commit_source(fixture.pg_rel, ambiguous, "ambiguous SQL candidate")
        fixture.reject(
            "cross-schema table ambiguity", fixture.pg_rel, "pcap_metadata_receipts", "postgres",
            "got 2",
        )

        run(["git", "reset", "--hard", "HEAD^", "-q"], fixture.root)
        fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
        fixture.sources[fixture.pg_rel] = PG_SOURCE
        fixture.write_manifest(omit=None)
        fixture.reject(
            "comment and string false positive", fixture.pg_rel, "fake_name", "postgres", "got 0"
        )
        fixture.reject(
            "dialect misuse", fixture.pg_rel, "ALTER_TABLE_probe_operation_outbox", "postgres",
            "supported only for clickhouse",
        )
        fixture.reject(
            "clickhouse non-group query", fixture.ch_rel, "pcap_index_local", "clickhouse",
            "requires an ALTER_TABLE_ grouped query",
        )
        fixture.reject(
            "manifest hash drift", fixture.pg_rel, "pcap_metadata_receipts", "postgres",
            "candidate manifest SHA-256 mismatch", manifest_hash="0" * 64,
        )

        source = fixture.root / fixture.pg_rel
        source.write_text(PG_SOURCE + "\nCREATE TABLE drift (id BIGINT);\n", encoding="utf-8")
        fixture.reject(
            "candidate drift", fixture.pg_rel, "pcap_metadata_receipts", "postgres",
            "worktree source differs from frozen candidate",
        )
        source.write_text(PG_SOURCE, encoding="utf-8")

        fixture.write_manifest(omit=fixture.pg_rel)
        fixture.reject(
            "manifest source omission", fixture.pg_rel, "pcap_metadata_receipts", "postgres",
            "candidate manifest does not bind exact source blob",
        )
        fixture.write_manifest(omit=None)

        outside = fixture.root / "outside.sql"
        outside.write_text(PG_SOURCE, encoding="utf-8")
        source.unlink()
        source.symlink_to(outside)
        fixture.reject(
            "source symlink", fixture.pg_rel, "pcap_metadata_receipts", "postgres",
            "repository path contains a symlink",
        )
        source.unlink()
        source.write_text(PG_SOURCE, encoding="utf-8")

        fixture.reject(
            "path escape", "../outside.sql", "pcap_metadata_receipts", "postgres",
            "source must be under deployments/postgres/migrations/",
        )

        broken = PG_SOURCE.replace("COMMIT;", "CREATE TABLE broken (id BIGINT;\nCOMMIT;")
        fixture.commit_source(fixture.pg_rel, broken, "broken SQL candidate")
        fixture.reject(
            "unbalanced syntax", fixture.pg_rel, "pcap_metadata_receipts", "postgres",
            "unbalanced SQL parentheses at end of file",
        )

        run(["git", "reset", "--hard", "HEAD^", "-q"], fixture.root)
        fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
        fixture.sources[fixture.pg_rel] = PG_SOURCE
        fixture.write_manifest(omit=None)
        output = "receipts/table.json"
        run(fixture.command(fixture.pg_rel, "pcap_metadata_receipts", "postgres", output=output), fixture.root)
        run(fixture.command(fixture.pg_rel, "pcap_metadata_receipts", "postgres", output=output), fixture.root)
        (fixture.root / output).write_text("{}\n", encoding="utf-8")
        fixture.reject(
            "immutable output overwrite", fixture.pg_rel, "pcap_metadata_receipts", "postgres",
            "immutable output already exists with different bytes", output=output,
        )
    finally:
        fixture.close()
    print(
        "PASS SQL DDL locator: 4 positive PostgreSQL/ClickHouse declaration forms and 10 targeted "
        "ambiguity/false-positive/dialect/candidate/manifest/syntax/symlink/path/output negative cases"
    )
    print(
        "PROOF_CEILING EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
        "OR_LOCATORS_RESOLVED"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
