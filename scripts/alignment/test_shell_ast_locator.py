#!/usr/bin/env python3
"""Fail-closed integration tests for the shell AST locator."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
from typing import Any

from build_topic1_task_registry import validate_against_schema


REPO = Path(__file__).resolve().parents[2]
TOOL = REPO / "scripts/alignment/shell_ast_locator"
SOURCE_SCHEMA = REPO / "contracts/alignment/shell-ast-locator-resolution-receipt.schema.json"
RESOLVER_SOURCE = TOOL / "main.go"
FIXED_TIME = "2026-08-13T12:00:00Z"
SOURCE = """\
#!/usr/bin/env bash
set -euo pipefail
for entry in \\
  "probe.control.v2:6:100" \\
  "probe.group-readiness.v1:6:200"; do
  name=$(echo "$entry" | cut -d: -f1)
  printf '%s\\n' "$name"
done
"""


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def run(
    command: list[str],
    cwd: Path,
    *,
    check: bool = True,
    env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(
        command, cwd=cwd, check=False, capture_output=True, text=True, env=env
    )
    if check and completed.returncode != 0:
        raise AssertionError(
            f"command failed rc={completed.returncode}: {command!r}\n"
            f"stdout={completed.stdout!r}\nstderr={completed.stderr!r}"
        )
    return completed


def build_binary(target: Path) -> Path:
    env = os.environ.copy()
    env["GOWORK"] = "off"
    env["GOBIN"] = str(target)
    run(["go", "install", "-mod=readonly", "."], TOOL, env=env)
    binary = target / "shell-ast-locator"
    if not binary.is_file():
        raise AssertionError("shell AST locator binary was not built")
    return binary


class Fixture:
    def __init__(self, binary: Path) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m02-shell-locator-")
        self.root = Path(self.temp.name)
        self.binary = binary
        self.source_rel = "scripts/topics.sh"
        self.manifest_rel = "candidate/manifest.json"
        source_path = self.root / self.source_rel
        source_path.parent.mkdir(parents=True)
        source_path.write_text(SOURCE, encoding="utf-8")
        resolver_target = self.root / "scripts/alignment/shell_ast_locator/main.go"
        resolver_target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(RESOLVER_SOURCE, resolver_target)
        run(["git", "init", "-q"], self.root)
        run(["git", "config", "user.email", "locator@example.invalid"], self.root)
        run(["git", "config", "user.name", "Locator Test"], self.root)
        run(["git", "add", "scripts"], self.root)
        run(["git", "commit", "-qm", "frozen candidate"], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()
        self.write_manifest()

    def write_manifest(self) -> None:
        payload: dict[str, Any] = {
            "implementation_candidate_commit": self.commit,
            "source_blob_sha256": {
                self.source_rel: digest((self.root / self.source_rel).read_bytes())
            },
        }
        path = self.root / self.manifest_rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
        self.manifest_hash = digest(path.read_bytes())

    def close(self) -> None:
        self.temp.cleanup()

    def command(
        self,
        query: str,
        *,
        source: str | None = None,
        manifest_hash: str | None = None,
        output: str | None = None,
    ) -> list[str]:
        command = [
            str(self.binary),
            "--source", source or self.source_rel,
            "--query", query,
            "--locator-id", "LOC-SHELL-SELFTEST",
            "--candidate-commit", self.commit,
            "--candidate-manifest", self.manifest_rel,
            "--candidate-manifest-sha256", manifest_hash or self.manifest_hash,
            "--repo-root", str(self.root),
            "--resolved-at", FIXED_TIME,
        ]
        if output:
            command += ["--output", output]
        return command

    def resolve(self, query: str) -> dict[str, Any]:
        completed = run(self.command(query), self.root)
        payload = json.loads(completed.stdout)
        validate_against_schema(payload, SOURCE_SCHEMA)
        return payload

    def reject(self, label: str, query: str, expected: str, **kwargs: Any) -> None:
        completed = run(self.command(query, **kwargs), self.root, check=False)
        if completed.returncode == 0:
            raise AssertionError(f"{label}: resolver unexpectedly succeeded")
        if expected not in completed.stderr:
            raise AssertionError(
                f"{label}: expected {expected!r}, stderr={completed.stderr!r}"
            )


def main() -> int:
    with tempfile.TemporaryDirectory(prefix="m02-shell-locator-build-") as build_temp:
        fixture = Fixture(build_binary(Path(build_temp)))
        try:
            receipt = fixture.resolve("probe.group-readiness.v1")
            locator = receipt["locator"]
            if (
                locator["literal_value"] != "probe.group-readiness.v1:6:200"
                or not locator["rendered_word"].startswith('"probe.group-readiness.v1')
            ):
                raise AssertionError("shell topic word identity is incomplete")

            fixture.reject("query drift", "missing.topic", "got 0")

            dynamic = SOURCE.replace(
                '"probe.group-readiness.v1:6:200"',
                '"${DYNAMIC_PREFIX:-probe.group-readiness.v1}:6:200"',
            )
            run(["git", "checkout", "-qb", "dynamic"], fixture.root)
            source_path = fixture.root / fixture.source_rel
            source_path.write_text(dynamic, encoding="utf-8")
            run(["git", "add", fixture.source_rel], fixture.root)
            run(["git", "commit", "-qm", "dynamic candidate"], fixture.root)
            fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
            fixture.write_manifest()
            fixture.reject("dynamic word", "probe.group-readiness.v1", "got 0")
            run(["git", "checkout", "-q", "HEAD~1"], fixture.root)
            fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
            source_path.write_text(SOURCE, encoding="utf-8")
            fixture.write_manifest()

            duplicated = SOURCE.replace(
                '"probe.group-readiness.v1:6:200"',
                '"probe.group-readiness.v1:6:200" \\\n+  "probe.group-readiness.v1:8:300"',
            )
            run(["git", "checkout", "-qb", "ambiguous"], fixture.root)
            source_path.write_text(duplicated, encoding="utf-8")
            run(["git", "add", fixture.source_rel], fixture.root)
            run(["git", "commit", "-qm", "ambiguous candidate"], fixture.root)
            fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
            fixture.write_manifest()
            fixture.reject("same-topic ambiguity", "probe.group-readiness.v1", "got 2")

            run(["git", "checkout", "-q", "HEAD~1"], fixture.root)
            fixture.commit = run(["git", "rev-parse", "HEAD"], fixture.root).stdout.strip()
            source_path.write_text(SOURCE, encoding="utf-8")
            fixture.write_manifest()
            fixture.reject(
                "manifest hash drift", "probe.group-readiness.v1",
                "candidate manifest SHA-256 mismatch", manifest_hash="0" * 64,
            )

            source_path.write_text(SOURCE + "\necho drift\n", encoding="utf-8")
            fixture.reject(
                "candidate drift", "probe.group-readiness.v1",
                "worktree source differs from frozen candidate",
            )
            source_path.write_text(SOURCE, encoding="utf-8")

            outside = fixture.root / "outside.sh"
            outside.write_text(SOURCE, encoding="utf-8")
            source_path.unlink()
            source_path.symlink_to(outside)
            fixture.reject(
                "source symlink", "probe.group-readiness.v1",
                "repository path contains a symlink",
            )
            source_path.unlink()
            source_path.write_text(SOURCE, encoding="utf-8")

            fixture.reject(
                "path escape", "probe.group-readiness.v1",
                "path contains an unsafe component",
                source="../outside.sh",
            )

            output = "receipts/topic.json"
            run(fixture.command("probe.group-readiness.v1", output=output), fixture.root)
            run(fixture.command("probe.group-readiness.v1", output=output), fixture.root)
            (fixture.root / output).write_text("{}\n", encoding="utf-8")
            fixture.reject(
                "immutable output overwrite", "probe.group-readiness.v1",
                "immutable output already exists with different bytes", output=output,
            )
        finally:
            fixture.close()
    print(
        "PASS shell AST locator: 1 positive literal topic form and 8 targeted "
        "query/dynamic/ambiguity/manifest/candidate/symlink/path/output negative cases"
    )
    print(
        "PROOF_CEILING EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
        "OR_LOCATORS_RESOLVED"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
