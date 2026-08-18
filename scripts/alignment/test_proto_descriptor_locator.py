#!/usr/bin/env python3
"""Fail-closed integration tests for the Protobuf descriptor locator."""

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
TOOL = REPO / "scripts/alignment/proto_descriptor_locator"
RESOLVER_SOURCE = TOOL / "main.go"
SCHEMA = REPO / "contracts/alignment/proto-descriptor-locator-resolution-receipt.schema.json"
FIXED_TIME = "2026-08-13T12:00:00Z"
COMMON = """\
syntax = "proto3";
package traffic.v1;
message Common { string id = 1; }
"""
SOURCE = """\
syntax = "proto3";
package traffic.v1;
import "traffic/v1/common.proto";

message Sample {
  string receipt = 1;
  enum State {
    STATE_UNSPECIFIED = 0;
    READY = 1;
  }
  State state = 2;
  Common common = 3;
}

enum TopState {
  TOP_STATE_UNSPECIFIED = 0;
  TOP_READY = 1;
}

service SampleService {
  rpc Send(Sample) returns (Sample);
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


def build_binary(target: Path) -> Path:
    env = os.environ.copy()
    env["GOWORK"] = "off"
    env["GOBIN"] = str(target)
    run(["go", "install", "-mod=readonly", "."], TOOL, env=env)
    binary = target / "proto-descriptor-locator"
    if not binary.is_file():
        raise AssertionError("Protobuf locator binary was not built")
    return binary


class Fixture:
    def __init__(self, binary: Path) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="m02-proto-locator-")
        self.root = Path(self.temp.name)
        self.binary = binary
        self.source_rel = "proto/traffic/v1/sample.proto"
        self.common_rel = "proto/traffic/v1/common.proto"
        self.manifest_rel = "candidate/manifest.json"
        for relative, content in (
            (self.source_rel, SOURCE),
            (self.common_rel, COMMON),
        ):
            target = self.root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content, encoding="utf-8")
        resolver_target = self.root / "scripts/alignment/proto_descriptor_locator/main.go"
        resolver_target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(RESOLVER_SOURCE, resolver_target)
        run(["git", "init", "-q"], self.root)
        run(["git", "config", "user.email", "locator@example.invalid"], self.root)
        run(["git", "config", "user.name", "Locator Test"], self.root)
        run(["git", "add", "proto", "scripts"], self.root)
        run(["git", "commit", "-qm", "frozen candidate"], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()
        self.write_manifest(include_common=True)

    def write_manifest(self, *, include_common: bool) -> None:
        blobs = {
            self.source_rel: digest((self.root / self.source_rel).read_bytes()),
        }
        if include_common:
            blobs[self.common_rel] = digest((self.root / self.common_rel).read_bytes())
        manifest: dict[str, Any] = {
            "artifact_kind": "DESIGN_CANDIDATE_MANIFEST",
            "schema_version": "1.0.0",
            "candidate_id": "DESIGN-T1-M02-N001-PROTO-R1",
            "implementation_candidate_commit": self.commit,
            "scope": "Protobuf locator self-test only",
            "source_blob_sha256": blobs,
            "formal_execution_status": "BLOCKED_UNTIL_SIGNED_OVERLAY",
            "proof_ceiling": "DEVELOPMENT_READINESS_DESIGN_ONLY_NOT_EXECUTION_AUTHORIZATION",
        }
        path = self.root / self.manifest_rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
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
            str(self.binary),
            "--source", source or self.source_rel,
            "--symbol", symbol,
            "--locator-id", "LOC-PROTO-SELFTEST",
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
            raise AssertionError(
                f"{label}: expected {expected!r}, stderr={completed.stderr!r}"
            )


def main() -> int:
    with tempfile.TemporaryDirectory(prefix="m02-proto-locator-build-") as build_temp:
        fixture = Fixture(build_binary(Path(build_temp)))
        try:
            message = fixture.resolve("traffic.v1.Sample")
            if message["locator"]["declaration_kind"] != "MESSAGE":
                raise AssertionError("message FQN did not resolve")
            field = fixture.resolve("traffic.v1.Sample.receipt")
            if (
                field["locator"]["declaration_kind"] != "FIELD"
                or field["locator"]["signature"]
                != "optional string traffic.v1.Sample.receipt = 1"
            ):
                raise AssertionError("field descriptor identity is incomplete")
            nested_enum = fixture.resolve("traffic.v1.Sample.State")
            if nested_enum["locator"]["declaration_kind"] != "ENUM":
                raise AssertionError("nested enum FQN did not resolve")
            service = fixture.resolve("traffic.v1.SampleService")
            if service["locator"]["declaration_kind"] != "SERVICE":
                raise AssertionError("service FQN did not resolve")
            method = fixture.resolve("traffic.v1.SampleService.Send")
            if method["locator"]["declaration_kind"] != "METHOD":
                raise AssertionError("method FQN did not resolve")

            fixture.reject(
                "FQN drift",
                "traffic.v1.Sample.missing",
                'expected exactly one descriptor match for "traffic.v1.Sample.missing", got 0',
            )
            fixture.reject(
                "manifest hash drift",
                "traffic.v1.Sample",
                "candidate manifest SHA-256 mismatch",
                manifest_hash="0" * 64,
            )

            source_path = fixture.root / fixture.source_rel
            source_path.write_text(SOURCE + "\nmessage Drift {}\n", encoding="utf-8")
            fixture.reject(
                "candidate drift",
                "traffic.v1.Sample",
                "worktree source differs from frozen candidate",
            )
            source_path.write_text(SOURCE, encoding="utf-8")

            fixture.write_manifest(include_common=False)
            fixture.reject(
                "import manifest omission",
                "traffic.v1.Sample",
                "candidate manifest does not bind descriptor input: proto/traffic/v1/common.proto",
            )
            fixture.write_manifest(include_common=True)

            outside = fixture.root / "outside.proto"
            outside.write_text(SOURCE, encoding="utf-8")
            source_path.unlink()
            source_path.symlink_to(outside)
            fixture.reject(
                "source symlink",
                "traffic.v1.Sample",
                "repository path contains a symlink",
            )
            source_path.unlink()
            source_path.write_text(SOURCE, encoding="utf-8")

            fixture.reject(
                "path escape",
                "traffic.v1.Sample",
                "source must be a repository-relative proto/*.proto path",
                source="../outside.proto",
            )

            output = "receipts/sample.json"
            run(fixture.command("traffic.v1.Sample", output=output), fixture.root)
            run(fixture.command("traffic.v1.Sample", output=output), fixture.root)
            (fixture.root / output).write_text("{}\n", encoding="utf-8")
            fixture.reject(
                "immutable output overwrite",
                "traffic.v1.Sample",
                "immutable output already exists with different bytes",
                output=output,
            )
        finally:
            fixture.close()
    print(
        "PASS Protobuf descriptor locator: 5 positive declaration forms and 7 targeted "
        "FQN/manifest/candidate/import/symlink/path/output negative cases"
    )
    print(
        "PROOF_CEILING EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
        "OR_LOCATORS_RESOLVED"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
