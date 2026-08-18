#!/usr/bin/env python3
"""Fail-closed integration tests for the M02 design-candidate freezer."""

from __future__ import annotations

import json
from pathlib import Path
import shutil
import subprocess
import tempfile


REPO = Path(__file__).resolve().parents[2]
FREEZER = REPO / "scripts/alignment/freeze_m02_design_candidate.py"
COVERAGE = REPO / "contracts/alignment/m02-code-direct-locator-coverage.v1.json"
DESIGN_SCHEMA = REPO / "contracts/alignment/design-candidate-manifest.schema.json"
LOCATOR_SCHEMA = REPO / "contracts/alignment/m02-code-direct-locator-coverage.schema.json"
OUTPUT = "doc/02_acceptance/topic1/m02/candidates/m02-code-direct-v4/design-candidate-manifest.json"


def run(command: list[str], cwd: Path, *, check: bool = True) -> subprocess.CompletedProcess[str]:
    completed = subprocess.run(command, cwd=cwd, check=False, capture_output=True, text=True)
    if check and completed.returncode != 0:
        raise AssertionError(
            f"command failed rc={completed.returncode}: {command!r}\n"
            f"stdout={completed.stdout!r}\nstderr={completed.stderr!r}"
        )
    return completed


class Fixture:
    def __init__(self, *, omit_source: bool = False, unsafe_source: bool = False) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m02-design-freeze-")
        self.root = Path(self.temp.name)
        coverage = json.loads(COVERAGE.read_text(encoding="utf-8"))
        if unsafe_source:
            coverage["locator_occurrences"][0]["path"] = "../outside.py"
        coverage_target = self.root / "contracts/alignment/m02-code-direct-locator-coverage.v1.json"
        coverage_target.parent.mkdir(parents=True)
        coverage_target.write_text(json.dumps(coverage, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        shutil.copy2(DESIGN_SCHEMA, self.root / "contracts/alignment/design-candidate-manifest.schema.json")
        shutil.copy2(LOCATOR_SCHEMA, self.root / "contracts/alignment/m02-code-direct-locator-coverage.schema.json")
        sources = sorted({item["path"] for item in coverage["locator_occurrences"]})
        if unsafe_source:
            sources = [item for item in sources if item != "../outside.py"]
        if omit_source:
            sources = sources[1:]
        self.sources = sources
        for relative in sources:
            target = self.root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(f"fixture:{relative}\n", encoding="utf-8")
        run(["git", "init", "-q"], self.root)
        run(["git", "config", "user.email", "candidate@example.invalid"], self.root)
        run(["git", "config", "user.name", "Candidate Test"], self.root)
        run(["git", "add", "."], self.root)
        run(["git", "commit", "-qm", "clean M02 candidate"], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()

    def command(self, *, commit: str | None = None, mode: str = "--write") -> list[str]:
        return [
            "python3", str(FREEZER), "--repo-root", str(self.root),
            "--candidate-commit", commit or self.commit, mode,
        ]

    def reject(self, label: str, expected: str, *, commit: str | None = None) -> None:
        completed = run(self.command(commit=commit), self.root, check=False)
        if completed.returncode == 0:
            raise AssertionError(f"{label}: freezer unexpectedly succeeded")
        if expected not in completed.stderr:
            raise AssertionError(f"{label}: expected {expected!r}, stderr={completed.stderr!r}")

    def close(self) -> None:
        self.temp.cleanup()


def main() -> int:
    fixture = Fixture()
    try:
        first = run(fixture.command(), fixture.root)
        if "source_count=163" not in first.stdout:
            raise AssertionError("positive freeze did not bind the 163-source exact-set")
        run(fixture.command(mode="--check"), fixture.root)

        output = fixture.root / OUTPUT
        original_output = output.read_bytes()
        output.write_text("{}\n", encoding="utf-8")
        fixture.reject("immutable output drift", "immutable design candidate output already exists")
        output.write_bytes(original_output)

        rogue = fixture.root / "rogue.txt"
        rogue.write_text("untracked\n", encoding="utf-8")
        fixture.reject("dirty worktree", "worktree contains uncommitted tracked or untracked changes")
        rogue.unlink()

        source = fixture.root / fixture.sources[0]
        original_source = source.read_bytes()
        source.write_bytes(original_source + b"drift\n")
        fixture.reject("source drift", "worktree source differs from frozen Git blob")
        source.write_bytes(original_source)

        outside = fixture.root / "outside-source"
        outside.write_bytes(original_source)
        source.unlink()
        source.symlink_to(outside)
        fixture.reject("source symlink", "source path contains a symlink")
        source.unlink()
        source.write_bytes(original_source)

        old_commit = fixture.commit
        unrelated = fixture.root / "committed-extra.txt"
        unrelated.write_text("next head\n", encoding="utf-8")
        run(["git", "add", "committed-extra.txt"], fixture.root)
        run(["git", "commit", "-qm", "different head"], fixture.root)
        fixture.reject("cross HEAD", "candidate commit must equal the worktree HEAD commit", commit=old_commit)
    finally:
        fixture.close()

    missing = Fixture(omit_source=True)
    try:
        missing.reject("missing candidate source", "candidate commit does not contain exactly one source blob")
    finally:
        missing.close()

    unsafe = Fixture(unsafe_source=True)
    try:
        unsafe.reject("path escape", "candidate path contains an unsafe component")
    finally:
        unsafe.close()

    output_symlink = Fixture()
    try:
        outside_dir = output_symlink.root / "outside-output"
        outside_dir.mkdir()
        doc = output_symlink.root / "doc"
        doc.symlink_to(outside_dir, target_is_directory=True)
        output_symlink.reject("output parent symlink", "output parent contains a symlink")
    finally:
        output_symlink.close()

    print(
        "PASS M02 design-candidate freezer: 2 positive immutable write/check cases and "
        "8 targeted output/dirty/source/symlink/HEAD/missing/path negative cases"
    )
    print("PROOF_CEILING CLEAN_DESIGN_CANDIDATE_FREEZE_ONLY_NOT_IMPLEMENTATION_SUPPLY_CHAIN_OR_EXECUTION_AUTHORIZATION")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
