#!/usr/bin/env python3
"""Fail-closed tests for JSON, YAML, TOML and Kubernetes config locators."""

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
RESOLVER = REPO / "scripts/alignment/structured_config_locator.py"
SCHEMA = REPO / "contracts/alignment/structured-config-locator-resolution-receipt.schema.json"
FIXED_TIME = "2026-08-13T12:00:00Z"
TOML_SOURCE = """\
[workspace]
members = ["one", "two"]

[dependencies]
xdp-abi = { path = "../xdp-abi" }
"""
YAML_SOURCE = """\
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ingest-gateway
  annotations: {self-test-purpose: "unique-token"}
spec:
  template:
    spec:
      containers:
      - name: ingest-gateway
        env:
        - {name: KAFKA_READY_TOPIC, value: "probe.group-readiness.v1"}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alert-service
spec:
  template:
    spec:
      containers:
      - name: alert-service
        env:
        - {name: KAFKA_READY_TOPIC, value: "probe.group-readiness.v1"}
"""
JSON_SOURCE = '{"service":{"topics":["one","two"]}}\n'


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
        self.temp = tempfile.TemporaryDirectory(prefix="m02-config-locator-")
        self.root = Path(self.temp.name)
        self.manifest_rel = "candidate/manifest.json"
        self.sources = {
            "config/workspace.toml": TOML_SOURCE,
            "config/services.yaml": YAML_SOURCE,
            "config/services.json": JSON_SOURCE,
        }
        for relative, content in self.sources.items():
            path = self.root / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8")
        resolver_target = self.root / "scripts/alignment/structured_config_locator.py"
        resolver_target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(RESOLVER, resolver_target)
        run(["git", "init", "-q"], self.root)
        run(["git", "config", "user.email", "locator@example.invalid"], self.root)
        run(["git", "config", "user.name", "Locator Test"], self.root)
        run(["git", "add", "config", "scripts"], self.root)
        run(["git", "commit", "-qm", "frozen candidate"], self.root)
        self.commit = run(["git", "rev-parse", "HEAD"], self.root).stdout.strip()
        self.write_manifest()

    def write_manifest(self) -> None:
        payload: dict[str, Any] = {
            "implementation_candidate_commit": self.commit,
            "source_blob_sha256": {
                relative: digest((self.root / relative).read_bytes())
                for relative in self.sources
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
        source: str,
        query: str,
        *,
        manifest_hash: str | None = None,
        output: str | None = None,
    ) -> list[str]:
        result = [
            "python3", "scripts/alignment/structured_config_locator.py",
            "--source", source,
            "--query", query,
            "--locator-id", "LOC-CONFIG-SELFTEST",
            "--candidate-commit", self.commit,
            "--candidate-manifest", self.manifest_rel,
            "--candidate-manifest-sha256", manifest_hash or self.manifest_hash,
            "--repo-root", str(self.root),
            "--resolved-at", FIXED_TIME,
        ]
        if output:
            result += ["--output", output]
        return result

    def resolve(self, source: str, query: str) -> dict[str, Any]:
        completed = run(self.command(source, query), self.root)
        payload = json.loads(completed.stdout)
        validate_against_schema(payload, SCHEMA)
        return payload

    def reject(self, label: str, source: str, query: str, expected: str, **kwargs: Any) -> None:
        completed = run(self.command(source, query, **kwargs), self.root, check=False)
        if completed.returncode == 0:
            raise AssertionError(f"{label}: resolver unexpectedly succeeded")
        if expected not in completed.stderr:
            raise AssertionError(
                f"{label}: expected {expected!r}, stderr={completed.stderr!r}"
            )


def main() -> int:
    fixture = Fixture()
    try:
        toml = fixture.resolve("config/workspace.toml", "workspace.members")
        if toml["locator"]["semantic_value"] != ["one", "two"]:
            raise AssertionError("TOML dotted path value drifted")
        json_result = fixture.resolve("config/services.json", "/service/topics/1")
        if json_result["locator"]["semantic_value"] != "two":
            raise AssertionError("JSON pointer value drifted")
        json_document = fixture.resolve("config/services.json", "$DOCUMENT")
        if (
            json_document["locator"]["match_strategy"] != "JSON_WHOLE_DOCUMENT"
            or json_document["locator"]["semantic_value"]
            != {"service": {"topics": ["one", "two"]}}
        ):
            raise AssertionError("whole JSON document did not resolve")
        yaml_scalar = fixture.resolve("config/services.yaml", "unique-token")
        if yaml_scalar["locator"]["match_strategy"] != "YAML_EXACT_SCALAR_VALUE":
            raise AssertionError("YAML scalar did not resolve")
        k8s = fixture.resolve(
            "config/services.yaml", "ingest-gateway.KAFKA_READY_TOPIC"
        )
        if k8s["locator"]["semantic_value"] != {
            "name": "KAFKA_READY_TOPIC",
            "value": "probe.group-readiness.v1",
        }:
            raise AssertionError("Kubernetes env locator drifted")

        fixture.reject(
            "YAML scalar ambiguity", "config/services.yaml", "probe.group-readiness.v1",
            "got 2",
        )
        fixture.reject(
            "Kubernetes workload drift", "config/services.yaml", "missing.KAFKA_READY_TOPIC",
            "got 0",
        )
        fixture.reject(
            "manifest hash drift", "config/workspace.toml", "workspace.members",
            "candidate manifest SHA-256 mismatch", manifest_hash="0" * 64,
        )

        source = fixture.root / "config/workspace.toml"
        source.write_text(TOML_SOURCE + "\n[drift]\nvalue = 1\n", encoding="utf-8")
        fixture.reject(
            "candidate drift", "config/workspace.toml", "workspace.members",
            "worktree source differs from frozen candidate",
        )
        source.write_text(TOML_SOURCE, encoding="utf-8")

        outside = fixture.root / "outside.toml"
        outside.write_text(TOML_SOURCE, encoding="utf-8")
        source.unlink()
        source.symlink_to(outside)
        fixture.reject(
            "source symlink", "config/workspace.toml", "workspace.members",
            "repository path contains a symlink",
        )
        source.unlink()
        source.write_text(TOML_SOURCE, encoding="utf-8")

        fixture.reject(
            "path escape", "../outside.toml", "workspace.members",
            "path contains an unsafe component",
        )

        output = "receipts/config.json"
        run(fixture.command("config/workspace.toml", "workspace.members", output=output), fixture.root)
        run(fixture.command("config/workspace.toml", "workspace.members", output=output), fixture.root)
        (fixture.root / output).write_text("{}\n", encoding="utf-8")
        fixture.reject(
            "immutable output overwrite", "config/workspace.toml", "workspace.members",
            "immutable output already exists with different bytes", output=output,
        )
        dangling = fixture.root / "receipts/dangling.json"
        dangling.symlink_to(fixture.root / "missing.json")
        fixture.reject(
            "output symlink", "config/workspace.toml", "workspace.members",
            "output path is not a regular file", output="receipts/dangling.json",
        )
    finally:
        fixture.close()
    print(
        "PASS structured config locator: 5 positive JSON-document/JSON-pointer/YAML/TOML/Kubernetes forms "
        "and 8 targeted ambiguity/query/manifest/candidate/symlink/path/output negative cases"
    )
    print(
        "PROOF_CEILING EXACT_LOCATOR_RESOLVER_IMPLEMENTED_NOT_M02_CANDIDATE_FROZEN_"
        "OR_LOCATORS_RESOLVED"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
