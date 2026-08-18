#!/usr/bin/env python3
"""Fail-closed integration tests for the Python AST declaration locator."""

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
RESOLVER = REPO / "scripts/alignment/python_ast_locator.py"
SCHEMA = REPO / "contracts/alignment/python-locator-resolution-receipt.schema.json"
FIXED_TIME = "2026-08-13T12:00:00Z"
SOURCE = """\
SETTING = {"enabled": False}
value = 1

def run(value: str) -> str:
    return value.strip()

async def run_async() -> None:
    return None

class Worker:
    def handle(self, value: str) -> str:
        return value.lower()
"""


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def run(command: list[str], cwd: Path, *, check: bool = True) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(command, cwd=cwd, check=False, capture_output=True, text=True)
    if check and completed.returncode != 0:
        raise AssertionError(f"command failed: {command!r}\nstdout={completed.stdout!r}\nstderr={completed.stderr!r}")
    return completed


class Fixture:
    def __init__(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m02-python-locator-")
        self.root = Path(self.temp.name)
        self.source_rel = "scripts/sample.py"
        self.manifest_rel = "candidate/manifest.json"
        source = self.root / self.source_rel
        source.parent.mkdir(parents=True)
        source.write_text(SOURCE, encoding="utf-8")
        resolver = self.root / "scripts/alignment/python_ast_locator.py"
        resolver.parent.mkdir(parents=True)
        shutil.copy2(RESOLVER, resolver)
        run(["git", "init", "-q"], self.root)
        run(["git", "config", "user.email", "locator@example.invalid"], self.root)
        run(["git", "config", "user.name", "Locator Test"], self.root)
        run(["git", "add", "scripts"], self.root)
        run(["git", "commit", "-qm", "frozen Python candidate"], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()
        self.write_manifest(True)

    def write_manifest(self, bind_source: bool) -> None:
        blobs = {self.source_rel: digest((self.root / self.source_rel).read_bytes())} if bind_source else {}
        payload = {
            "artifact_kind": "DESIGN_CANDIDATE_MANIFEST", "schema_version": "1.0.0",
            "candidate_id": "DESIGN-T1-M02-PY-R1", "implementation_candidate_commit": self.commit,
            "scope": "Python AST locator self-test only", "source_blob_sha256": blobs,
            "formal_execution_status": "BLOCKED_UNTIL_SIGNED_OVERLAY",
            "proof_ceiling": "DEVELOPMENT_READINESS_DESIGN_ONLY_NOT_EXECUTION_AUTHORIZATION",
        }
        path = self.root / self.manifest_rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
        self.manifest_hash = digest(path.read_bytes())

    def command(self, symbol: str, *, source: str | None = None, manifest_hash: str | None = None, output: str | None = None) -> list[str]:
        command = [
            "python3", "scripts/alignment/python_ast_locator.py", "--source", source or self.source_rel,
            "--symbol", symbol, "--locator-id", "LOC-PY-SELFTEST", "--candidate-commit", self.commit,
            "--candidate-manifest", self.manifest_rel, "--candidate-manifest-sha256", manifest_hash or self.manifest_hash,
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
        if completed.returncode == 0 or expected not in completed.stderr:
            raise AssertionError(f"{label}: expected {expected!r}, rc={completed.returncode}, stderr={completed.stderr!r}")

    def close(self) -> None:
        self.temp.cleanup()


def main() -> int:
    fixture = Fixture()
    try:
        expected = {
            "run": "FUNCTION", "run_async": "ASYNC_FUNCTION", "Worker": "CLASS",
            "Worker.handle": "METHOD", "SETTING": "MODULE_CONSTANT", "value": "MODULE_VARIABLE",
        }
        for symbol, kind in expected.items():
            if fixture.resolve(symbol)["locator"]["declaration_kind"] != kind:
                raise AssertionError(f"Python declaration kind drifted for {symbol}")
        fixture.reject("unknown query", "missing", "expected one exact Python AST match")
        fixture.reject("manifest hash drift", "run", "candidate manifest SHA-256 mismatch", manifest_hash="0" * 64)
        source = fixture.root / fixture.source_rel
        source.write_text(SOURCE + "\ndrift = 2\n", encoding="utf-8")
        fixture.reject("candidate drift", "run", "worktree/source manifest differs from frozen candidate")
        source.write_text(SOURCE, encoding="utf-8")
        fixture.write_manifest(False)
        fixture.reject("manifest omission", "run", "worktree/source manifest differs from frozen candidate")
        fixture.write_manifest(True)
        outside = fixture.root / "outside.py"
        outside.write_text(SOURCE, encoding="utf-8")
        source.unlink(); source.symlink_to(outside)
        fixture.reject("source symlink", "run", "repository path contains a symlink")
        source.unlink(); source.write_text(SOURCE, encoding="utf-8")
        fixture.reject("path escape", "run", "unsafe or missing repository file", source="scripts/../outside.py")
        source.write_text("def broken(:\n", encoding="utf-8")
        run(["git", "add", fixture.source_rel], fixture.root); run(["git", "commit", "-qm", "broken syntax"], fixture.root)
        fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip(); fixture.write_manifest(True)
        fixture.reject("invalid syntax", "broken", "SyntaxError")
        run(["git", "reset", "--hard", "HEAD^", "-q"], fixture.root)
        fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip(); fixture.write_manifest(True)
        output = "receipts/python.json"
        run(fixture.command("run", output=output), fixture.root); run(fixture.command("run", output=output), fixture.root)
        (fixture.root / output).write_text("{}\n", encoding="utf-8")
        fixture.reject("immutable output", "run", "immutable Python locator receipt exists", output=output)
    finally:
        fixture.close()
    print(
        "PASS Python AST locator: 6 positive declaration forms and 8 targeted "
        "query/candidate/manifest/syntax/symlink/path/output negative cases"
    )
    print("PROOF_CEILING EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_OR_LOCATORS_RESOLVED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
