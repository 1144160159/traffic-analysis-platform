#!/usr/bin/env python3
"""Fail-closed integration tests for the Go AST declaration locator."""

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
RESOLVER = REPO / "scripts/alignment/go_ast_locator/main.go"
SCHEMA = REPO / "contracts/alignment/locator-resolution-receipt.schema.json"
FIXED_TIME = "2026-08-13T12:00:00Z"
SOURCE = """\
package sample

const EventTopic = "events.v1"
var Ready = true

type Config struct {
    Topic string
}

func Build() *Config { return &Config{Topic: EventTopic} }

func (c *Config) Load(value string) string { return value + c.Topic }
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
        self.temp = tempfile.TemporaryDirectory(prefix="m02-go-locator-")
        self.root = Path(self.temp.name)
        self.source_rel = "go/sample/sample.go"
        self.manifest_rel = "candidate/manifest.json"
        source = self.root / self.source_rel
        source.parent.mkdir(parents=True)
        source.write_text(SOURCE, encoding="utf-8")
        resolver = self.root / "scripts/alignment/go_ast_locator/main.go"
        resolver.parent.mkdir(parents=True)
        shutil.copy2(RESOLVER, resolver)
        run(["git", "init", "-q"], self.root)
        run(["git", "config", "user.email", "locator@example.invalid"], self.root)
        run(["git", "config", "user.name", "Locator Test"], self.root)
        run(["git", "add", "go", "scripts"], self.root)
        run(["git", "commit", "-qm", "frozen Go candidate"], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()
        self.write_manifest(bind_source=True)

    def write_manifest(self, *, bind_source: bool) -> None:
        blobs = {self.source_rel: digest((self.root / self.source_rel).read_bytes())} if bind_source else {}
        payload: dict[str, Any] = {
            "artifact_kind": "DESIGN_CANDIDATE_MANIFEST",
            "schema_version": "1.0.0",
            "candidate_id": "DESIGN-T1-M02-GO-R1",
            "implementation_candidate_commit": self.commit,
            "scope": "Go AST locator self-test only",
            "source_blob_sha256": blobs,
            "formal_execution_status": "BLOCKED_UNTIL_SIGNED_OVERLAY",
            "proof_ceiling": "DEVELOPMENT_READINESS_DESIGN_ONLY_NOT_EXECUTION_AUTHORIZATION",
        }
        path = self.root / self.manifest_rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
        self.manifest_hash = digest(path.read_bytes())

    def command(self, symbol: str, *, source: str | None = None, manifest_hash: str | None = None, output: str | None = None) -> list[str]:
        command = [
            "go", "run", "./scripts/alignment/go_ast_locator/main.go",
            "--source", source or self.source_rel,
            "--symbol", symbol,
            "--locator-id", "LOC-GO-SELFTEST",
            "--candidate-commit", self.commit,
            "--candidate-manifest", self.manifest_rel,
            "--candidate-manifest-sha256", manifest_hash or self.manifest_hash,
            "--repo-root", str(self.root),
            "--resolved-at", FIXED_TIME,
        ]
        if output is not None:
            command += ["--output", output]
        return command

    def resolve(self, symbol: str) -> dict[str, Any]:
        payload = json.loads(run(self.command(symbol), self.root).stdout)
        validate_against_schema(payload, SCHEMA)
        return payload

    def reject(self, label: str, symbol: str, expected: str, **kwargs: Any) -> None:
        completed = run(self.command(symbol, **kwargs), self.root, check=False)
        if completed.returncode == 0:
            raise AssertionError(f"{label}: resolver unexpectedly succeeded")
        if expected not in completed.stderr:
            raise AssertionError(f"{label}: expected {expected!r}, stderr={completed.stderr!r}")

    def close(self) -> None:
        self.temp.cleanup()


def main() -> int:
    fixture = Fixture()
    try:
        expected = {
            "Build": "FUNCTION",
            "Config.Load": "METHOD",
            "Config": "TYPE",
            "Config.Topic": "STRUCT_FIELD",
            "EventTopic": "CONST",
            "Ready": "VAR",
        }
        for symbol, kind in expected.items():
            receipt = fixture.resolve(symbol)
            if receipt["locator"]["declaration_kind"] != kind:
                raise AssertionError(f"{symbol} resolved as {receipt['locator']['declaration_kind']}, expected {kind}")
        if fixture.resolve("(*Config).Load")["locator"]["qualified_symbol"] != "sample.Config.Load":
            raise AssertionError("pointer receiver compatibility query lost canonical identity")

        fixture.reject("unknown query", "Missing", "expected exactly one Go AST declaration match")
        fixture.reject("manifest hash drift", "Build", "candidate manifest sha256 mismatch", manifest_hash="0" * 64)
        source = fixture.root / fixture.source_rel
        source.write_text(SOURCE + "\nvar Drift = true\n", encoding="utf-8")
        fixture.reject("candidate drift", "Build", "worktree source differs from frozen candidate")
        source.write_text(SOURCE, encoding="utf-8")
        fixture.write_manifest(bind_source=False)
        fixture.reject("manifest source omission", "Build", "candidate manifest does not bind source")
        fixture.write_manifest(bind_source=True)

        outside = fixture.root / "outside.go"
        outside.write_text(SOURCE, encoding="utf-8")
        source.unlink()
        source.symlink_to(outside)
        fixture.reject("source symlink", "Build", "repository path contains a symlink")
        source.unlink()
        source.write_text(SOURCE, encoding="utf-8")
        fixture.reject("path escape", "Build", "canonical and repository-relative", source="go/../outside.go")

        source.write_text("package sample\nfunc Broken( {\n", encoding="utf-8")
        run(["git", "add", fixture.source_rel], fixture.root)
        run(["git", "commit", "-qm", "broken syntax candidate"], fixture.root)
        fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
        fixture.write_manifest(bind_source=True)
        fixture.reject("invalid syntax", "Broken", "expected ')'")

        run(["git", "reset", "--hard", "HEAD^", "-q"], fixture.root)
        fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
        fixture.write_manifest(bind_source=True)
        output = "receipts/go.json"
        run(fixture.command("Build", output=output), fixture.root)
        run(fixture.command("Build", output=output), fixture.root)
        (fixture.root / output).write_text("{}\n", encoding="utf-8")
        fixture.reject("immutable output overwrite", "Build", "immutable output already exists", output=output)
    finally:
        fixture.close()
    print(
        "PASS Go AST locator: 6 positive declaration forms and 8 targeted "
        "query/candidate/manifest/syntax/symlink/path/output negative cases"
    )
    print("PROOF_CEILING EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_OR_LOCATORS_RESOLVED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
