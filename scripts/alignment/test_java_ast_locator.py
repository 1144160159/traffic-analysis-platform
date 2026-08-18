#!/usr/bin/env python3
"""Fail-closed integration tests for the JDK javac Java locator resolver."""

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
RESOLVER_SOURCE = REPO / "scripts/alignment/java_ast_locator/JavaAstLocator.java"
SCHEMA = REPO / "contracts/alignment/java-locator-resolution-receipt.schema.json"
FIXED_TIME = "2026-08-13T12:00:00Z"
SOURCE = """\
package com.example;

public class Sample {
    private final String value;

    public Sample(String value) {
        this.value = value;
    }

    public int process(String input, int count) {
        String normalized = input.trim();
        return helper(normalized, count);
    }

    private int helper(String input, int count) {
        return input.length() + count;
    }

    public static Sample build(String value) {
        return new Sample(value);
    }

    public static class Nested {}
}

interface Handler {
    int process(String input, int count);
}

record Receipt(String id) {}

enum Mode { READY, STOPPED }

class Other {
    int process(String input, int count) { return count; }
}
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


def build_classes(target: Path) -> Path:
    run(["javac", "-encoding", "UTF-8", "-d", str(target), str(RESOLVER_SOURCE)], REPO)
    if not (target / "JavaAstLocator.class").is_file():
        raise AssertionError("Java AST locator class was not built")
    return target


class Fixture:
    def __init__(self, classes: Path) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m02-java-locator-")
        self.root = Path(self.temp.name)
        self.classes = classes
        self.source_rel = "java/sample/src/main/java/com/example/Sample.java"
        self.manifest_rel = "candidate/manifest.json"
        source = self.root / self.source_rel
        source.parent.mkdir(parents=True)
        source.write_text(SOURCE, encoding="utf-8")
        resolver = self.root / "scripts/alignment/java_ast_locator/JavaAstLocator.java"
        resolver.parent.mkdir(parents=True)
        shutil.copy2(RESOLVER_SOURCE, resolver)
        run(["git", "init", "-q"], self.root)
        run(["git", "config", "user.email", "locator@example.invalid"], self.root)
        run(["git", "config", "user.name", "Locator Test"], self.root)
        run(["git", "add", "java", "scripts"], self.root)
        run(["git", "commit", "-qm", "frozen Java candidate"], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()
        self.write_manifest(bind_source=True)

    def write_manifest(self, *, bind_source: bool) -> None:
        blobs = {}
        if bind_source:
            blobs[self.source_rel] = digest((self.root / self.source_rel).read_bytes())
        payload: dict[str, Any] = {
            "artifact_kind": "DESIGN_CANDIDATE_MANIFEST",
            "schema_version": "1.0.0",
            "candidate_id": "DESIGN-T1-M02-JAVA-R1",
            "implementation_candidate_commit": self.commit,
            "scope": "Java AST locator self-test only",
            "source_blob_sha256": blobs,
            "formal_execution_status": "BLOCKED_UNTIL_SIGNED_OVERLAY",
            "proof_ceiling": "DEVELOPMENT_READINESS_DESIGN_ONLY_NOT_EXECUTION_AUTHORIZATION",
        }
        path = self.root / self.manifest_rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        self.manifest_hash = digest(path.read_bytes())

    def close(self) -> None:
        self.temp.cleanup()

    def command(
        self,
        symbol: str,
        *,
        source: str | None = None,
        manifest_hash: str | None = None,
        output: str | None = None,
    ) -> list[str]:
        command = [
            "java", "-cp", str(self.classes), "JavaAstLocator",
            "--source", source or self.source_rel,
            "--symbol", symbol,
            "--locator-id", "LOC-JAVA-SELFTEST",
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
        completed = run(self.command(symbol), self.root)
        payload = json.loads(completed.stdout)
        validate_against_schema(payload, SCHEMA)
        return payload

    def reject(self, label: str, symbol: str, expected: str, **kwargs: Any) -> None:
        completed = run(self.command(symbol, **kwargs), self.root, check=False)
        if completed.returncode == 0:
            raise AssertionError(f"{label}: resolver unexpectedly succeeded")
        if expected not in completed.stderr:
            raise AssertionError(f"{label}: expected {expected!r}, stderr={completed.stderr!r}")


def main() -> int:
    with tempfile.TemporaryDirectory(prefix="m02-java-locator-build-") as build_temp:
        fixture = Fixture(build_classes(Path(build_temp)))
        try:
            top = fixture.resolve("Sample")
            if top["locator"]["declaration_kind"] != "CLASS" or top["locator"]["qualified_symbol"] != "com.example.Sample":
                raise AssertionError("top-level class identity is incomplete")

            method = fixture.resolve("Sample.process(String,int)")
            if (
                method["locator"]["declaration_kind"] != "METHOD"
                or method["locator"]["qualified_symbol"] != "com.example.Sample.process"
                or not any(item["expression"] == "input.trim" for item in method["locator"]["calls"])
                or not any(item["expression"] == "helper" for item in method["locator"]["calls"])
            ):
                raise AssertionError("parameter-qualified method receipt is incomplete")

            constructor = fixture.resolve("Sample.<init>(String)")
            if constructor["locator"]["declaration_kind"] != "CONSTRUCTOR":
                raise AssertionError("constructor did not resolve")

            nested = fixture.resolve("Sample.Nested")
            if nested["locator"]["qualified_symbol"] != "com.example.Sample.Nested":
                raise AssertionError("nested class identity drifted")

            record = fixture.resolve("Receipt")
            if record["locator"]["declaration_kind"] != "RECORD":
                raise AssertionError("record declaration did not resolve")

            fixture.reject(
                "same-name ambiguity", "process(String,int)",
                'expected exactly one Java AST match for "process(String,int)", got 3',
            )
            fixture.reject(
                "parameter signature drift", "Sample.process(String,long)",
                'expected exactly one Java AST match for "Sample.process(String,long)", got 0',
            )
            fixture.reject(
                "manifest hash drift", "Sample",
                "candidate manifest SHA-256 mismatch",
                manifest_hash="0" * 64,
            )

            source = fixture.root / fixture.source_rel
            source.write_text(SOURCE + "\nclass Drift {}\n", encoding="utf-8")
            fixture.reject("candidate drift", "Sample", "worktree source differs from frozen candidate")
            source.write_text(SOURCE, encoding="utf-8")

            fixture.write_manifest(bind_source=False)
            fixture.reject("manifest source omission", "Sample", "candidate manifest does not bind source")
            fixture.write_manifest(bind_source=True)

            outside = fixture.root / "outside.java"
            outside.write_text(SOURCE, encoding="utf-8")
            source.unlink()
            source.symlink_to(outside)
            fixture.reject("source symlink", "Sample", "repository path contains a symlink")
            source.unlink()
            source.write_text(SOURCE, encoding="utf-8")

            fixture.reject(
                "path escape", "Sample", "source must be a repository-relative java/*.java path",
                source="../outside.java",
            )

            source.write_text("package com.example; class Broken {", encoding="utf-8")
            run(["git", "add", fixture.source_rel], fixture.root)
            run(["git", "commit", "-qm", "broken syntax candidate"], fixture.root)
            fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
            fixture.write_manifest(bind_source=True)
            fixture.reject("invalid syntax", "Broken", "javac syntax parse failed")

            run(["git", "reset", "--hard", "HEAD^", "-q"], fixture.root)
            fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
            fixture.write_manifest(bind_source=True)
            output = "receipts/sample.json"
            run(fixture.command("Sample", output=output), fixture.root)
            run(fixture.command("Sample", output=output), fixture.root)
            (fixture.root / output).write_text("{}\n", encoding="utf-8")
            fixture.reject(
                "immutable output overwrite", "Sample",
                "immutable output already exists with different bytes", output=output,
            )
        finally:
            fixture.close()
    print(
        "PASS Java AST locator: 5 positive type/member forms and 8 targeted "
        "ambiguity/signature/candidate/manifest/syntax/symlink/path/output negative cases"
    )
    print(
        "PROOF_CEILING EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
        "OR_LOCATORS_RESOLVED"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
