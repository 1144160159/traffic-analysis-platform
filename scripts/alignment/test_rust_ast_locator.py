#!/usr/bin/env python3
"""Fail-closed integration tests for the Rust syn locator resolver."""

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
MANIFEST = REPO / "scripts/alignment/rust_ast_locator/Cargo.toml"
RESOLVER_SOURCE = REPO / "scripts/alignment/rust_ast_locator/src/main.rs"
SCHEMA = REPO / "contracts/alignment/rust-locator-resolution-receipt.schema.json"
FIXED_TIME = "2026-08-13T12:00:00Z"
SOURCE = """\
pub trait Runner {
    fn run(&self, value: u64) -> Result<u64, String>;
}

pub struct Worker;

impl Worker {
    pub async fn work(
        &self,
        value: u64,
    ) -> Result<u64, String> {
        helper(value).await
    }
}

impl Runner for Worker {
    fn run(&self, value: u64) -> Result<u64, String> {
        Ok(value)
    }
}

pub async fn helper(value: u64) -> Result<u64, String> {
    Ok(value)
}

mod first {
    pub fn collide() {}
}

mod second {
    pub fn collide() {}
}
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
        command,
        cwd=cwd,
        check=False,
        capture_output=True,
        text=True,
        env=env,
    )
    if check and completed.returncode != 0:
        raise AssertionError(
            f"command failed rc={completed.returncode}: {command!r}\n"
            f"stdout={completed.stdout!r}\nstderr={completed.stderr!r}"
        )
    return completed


def build_binary(target_dir: Path) -> Path:
    env = os.environ.copy()
    env["CARGO_TARGET_DIR"] = str(target_dir)
    run(
        [
            "cargo", "build", "--manifest-path", str(MANIFEST),
            "--locked", "--offline",
        ],
        REPO,
        env=env,
    )
    binary = target_dir / "debug/traffic-rust-ast-locator"
    if not binary.is_file():
        raise AssertionError("Rust locator binary was not built")
    return binary


class Fixture:
    def __init__(self, binary: Path) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m02-rust-locator-")
        self.root = Path(self.temp.name)
        self.binary = binary
        (self.root / "rust/sample/src").mkdir(parents=True)
        (self.root / "scripts/alignment/rust_ast_locator/src").mkdir(parents=True)
        self.source_rel = "rust/sample/src/lib.rs"
        self.manifest_rel = "candidate/manifest.json"
        (self.root / self.source_rel).write_text(SOURCE, encoding="utf-8")
        shutil.copy2(
            RESOLVER_SOURCE,
            self.root / "scripts/alignment/rust_ast_locator/src/main.rs",
        )
        run(["git", "init", "-q"], self.root)
        run(["git", "config", "user.email", "locator@example.invalid"], self.root)
        run(["git", "config", "user.name", "Locator Test"], self.root)
        run(["git", "add", "rust", "scripts"], self.root)
        run(["git", "commit", "-qm", "frozen candidate"], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()
        source_hash = digest((self.root / self.source_rel).read_bytes())
        manifest: dict[str, Any] = {
            "artifact_kind": "DESIGN_CANDIDATE_MANIFEST",
            "schema_version": "1.0.0",
            "candidate_id": "DESIGN-T1-M02-N001-R1",
            "implementation_candidate_commit": self.commit,
            "scope": "Rust locator self-test only",
            "source_blob_sha256": {self.source_rel: source_hash},
            "formal_execution_status": "BLOCKED_UNTIL_SIGNED_OVERLAY",
            "proof_ceiling": "DEVELOPMENT_READINESS_DESIGN_ONLY_NOT_EXECUTION_AUTHORIZATION",
        }
        manifest_path = self.root / self.manifest_rel
        manifest_path.parent.mkdir(parents=True)
        manifest_path.write_text(
            json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        self.manifest_hash = digest(manifest_path.read_bytes())

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
            str(self.binary),
            "--source", source or self.source_rel,
            "--symbol", symbol,
            "--locator-id", "LOC-RUST-SELFTEST",
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

    def reject(
        self,
        label: str,
        symbol: str,
        expected: str,
        **kwargs: Any,
    ) -> None:
        completed = run(self.command(symbol, **kwargs), self.root, check=False)
        if completed.returncode == 0:
            raise AssertionError(f"{label}: resolver unexpectedly succeeded")
        if expected not in completed.stderr:
            raise AssertionError(
                f"{label}: expected {expected!r}, stderr={completed.stderr!r}"
            )


def main() -> int:
    with tempfile.TemporaryDirectory(prefix="m02-rust-locator-build-") as build_temp:
        binary = build_binary(Path(build_temp))
        fixture = Fixture(binary)
        try:
            work = fixture.resolve(
                "Worker::work(&self, value: u64) -> Result<u64, String>"
            )
            if (
                work["locator"]["declaration_kind"] != "ASYNC_INHERENT_METHOD"
                or work["locator"]["qualified_symbol"] != "crate::Worker::work"
                or work["locator"]["start"]["line"] != 8
                or not any(
                    item["expression"] == "helper"
                    for item in work["locator"]["calls"]
                )
            ):
                raise AssertionError("multiline inherent method receipt is incomplete")

            trait_method = fixture.resolve("impl Runner for Worker::run")
            if trait_method["locator"]["declaration_kind"] != "TRAIT_METHOD":
                raise AssertionError("trait method did not resolve as TRAIT_METHOD")

            impl_block = fixture.resolve("impl Runner for Worker")
            if impl_block["locator"]["declaration_kind"] != "TRAIT_IMPL":
                raise AssertionError("trait impl block did not resolve as TRAIT_IMPL")

            struct = fixture.resolve("Worker")
            if struct["locator"]["declaration_kind"] != "STRUCT":
                raise AssertionError("struct did not resolve as STRUCT")

            nested = fixture.resolve("first::collide")
            if nested["locator"]["qualified_symbol"] != "crate::first::collide":
                raise AssertionError("nested module function identity drifted")

            fixture.reject(
                "same-name ambiguity",
                "collide",
                'expected exactly one Rust AST match for "collide", got 2',
            )
            fixture.reject(
                "signature drift",
                "Worker::work(&self, value: u32) -> Result<u64, String>",
                "got 0",
            )
            fixture.reject(
                "manifest hash drift",
                "Worker",
                "candidate manifest SHA-256 mismatch",
                manifest_hash="0" * 64,
            )

            source_path = fixture.root / fixture.source_rel
            source_path.write_text(SOURCE + "\npub fn drift() {}\n", encoding="utf-8")
            fixture.reject(
                "candidate drift",
                "Worker",
                "worktree source differs from frozen candidate",
            )
            source_path.write_text(SOURCE, encoding="utf-8")

            outside = fixture.root / "outside.rs"
            outside.write_text(SOURCE, encoding="utf-8")
            source_path.unlink()
            source_path.symlink_to(outside)
            fixture.reject(
                "source symlink", "Worker", "repository path contains a symlink"
            )
            source_path.unlink()
            source_path.write_text(SOURCE, encoding="utf-8")

            fixture.reject(
                "path escape",
                "Worker",
                "path contains an unsafe component",
                source="../outside.rs",
            )

            output = "receipts/worker.json"
            run(fixture.command("Worker", output=output), fixture.root)
            run(fixture.command("Worker", output=output), fixture.root)
            (fixture.root / output).write_text("{}\n", encoding="utf-8")
            fixture.reject(
                "immutable output overwrite",
                "Worker",
                "immutable output already exists with different bytes",
                output=output,
            )
        finally:
            fixture.close()
    print(
        "PASS Rust AST locator: 5 positive code-unit forms and 7 targeted "
        "ambiguity/signature/candidate/symlink/path/output negative cases"
    )
    print(
        "PROOF_CEILING EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
        "OR_LOCATORS_RESOLVED"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
