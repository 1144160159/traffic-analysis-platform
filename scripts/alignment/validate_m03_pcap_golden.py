#!/usr/bin/env python3
"""Validate the M03 PCAP golden corpus and external inventory boundaries."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
from pathlib import Path


REPO = Path(__file__).resolve().parents[2]
CORPUS = REPO / "tests/fixtures/pcap/m03"
MANIFEST = CORPUS / "manifest.v1.json"
SCHEMA = CORPUS / "manifest.v1.schema.json"
GENERATOR = REPO / "scripts/testdata/generate_m03_pcap_golden.py"
INVENTORY = REPO / "doc/02_acceptance/02-regression/pcap-dataset-inventory-latest.json"
EXPECTED_COVERAGE = {
    "normal",
    "attack_structure",
    "ipv6",
    "tls",
    "quic",
    "truncated",
    "large_flow",
    "empty",
}


def fail(message: str) -> None:
    raise ValueError(message)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def load_json(path: Path) -> dict:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"M03_GOLDEN_INVALID_JSON path={path}: {exc}")
    if not isinstance(value, dict):
        fail(f"M03_GOLDEN_ROOT_NOT_OBJECT path={path}")
    return value


def require_keys(value: dict, expected: set[str], context: str) -> None:
    actual = set(value)
    if actual != expected:
        fail(f"M03_GOLDEN_KEYS_MISMATCH context={context} expected={sorted(expected)} actual={sorted(actual)}")


def validate_manifest() -> None:
    manifest = load_json(MANIFEST)
    require_keys(
        manifest,
        {"schema_version", "corpus_id", "generator", "source", "blind_label_policy", "coverage", "fixtures"},
        "manifest",
    )
    if manifest["schema_version"] != "1.0.0" or manifest["corpus_id"] != "m03-pcap-golden-v1":
        fail("M03_GOLDEN_IDENTITY_DRIFT")
    if set(manifest["coverage"]) != EXPECTED_COVERAGE or len(manifest["coverage"]) != 8:
        fail("M03_GOLDEN_COVERAGE_NOT_EXACT")
    if "forbidden" not in manifest["blind_label_policy"]:
        fail("M03_GOLDEN_BLIND_LABEL_POLICY_MISSING")
    if manifest["generator"] != {
        "path": "scripts/testdata/generate_m03_pcap_golden.py",
        "sha256": sha256(GENERATOR),
    }:
        fail("M03_GOLDEN_GENERATOR_HASH_DRIFT")
    if manifest["source"] != {
        "kind": "deterministic_synthetic",
        "license": "project_generated_test_fixture",
        "custodian_role": "traffic-parser-feature-owner",
    }:
        fail("M03_GOLDEN_SOURCE_CONTRACT_DRIFT")

    fixtures = manifest["fixtures"]
    if not isinstance(fixtures, list) or len(fixtures) != 8:
        fail("M03_GOLDEN_FIXTURE_COUNT_NOT_EXACT")
    ids: set[str] = set()
    paths: set[str] = set()
    categories: set[str] = set()
    for row in fixtures:
        require_keys(
            row,
            {"fixture_id", "relative_path", "category", "sha256", "size_bytes", "packet_count", "expected", "semantic_label"},
            "fixture",
        )
        if row["fixture_id"] in ids or row["relative_path"] in paths:
            fail("M03_GOLDEN_DUPLICATE_FIXTURE_ID_OR_PATH")
        ids.add(row["fixture_id"])
        paths.add(row["relative_path"])
        categories.add(row["category"])
        relative = Path(row["relative_path"])
        if relative.is_absolute() or len(relative.parts) != 1 or relative.suffix != ".pcap":
            fail(f"M03_GOLDEN_PATH_ESCAPE path={relative}")
        if not re.fullmatch(r"[a-z0-9-]+", row["fixture_id"]):
            fail(f"M03_GOLDEN_INVALID_FIXTURE_ID id={row['fixture_id']}")
        path = CORPUS / relative
        if not path.is_file() or path.stat().st_size != row["size_bytes"] or sha256(path) != row["sha256"]:
            fail(f"M03_GOLDEN_BODY_DRIFT fixture={row['fixture_id']}")
        if row["semantic_label"] != {
            "status": "synthetic_structure_not_ground_truth",
            "blind_evaluation_eligible": False,
        }:
            fail(f"M03_GOLDEN_LABEL_LEAK fixture={row['fixture_id']}")
    if categories != EXPECTED_COVERAGE:
        fail("M03_GOLDEN_CATEGORY_SET_NOT_EXACT")
    actual_pcaps = {path.name for path in CORPUS.glob("*.pcap")}
    if actual_pcaps != paths:
        fail("M03_GOLDEN_PCAP_FILE_SET_DRIFT")


def validate_external_inventory() -> None:
    inventory = load_json(INVENTORY)
    policy = inventory.get("candidate_policy", {})
    if policy.get("ground_truth") != "never inferred from file or directory names":
        fail("M03_EXTERNAL_GROUND_TRUTH_POLICY_MISSING")
    candidates = inventory.get("candidates")
    if not isinstance(candidates, list):
        fail("M03_EXTERNAL_CANDIDATES_NOT_ARRAY")
    for row in candidates:
        if "label" in row:
            fail("M03_EXTERNAL_INFERRED_LABEL_LEAK")
        if row.get("ground_truth_status") != "unverified_not_for_blind_evaluation":
            fail("M03_EXTERNAL_GROUND_TRUTH_STATUS_DRIFT")
        if row.get("license_status") != "requires_dataset_custodian_confirmation":
            fail("M03_EXTERNAL_LICENSE_STATUS_DRIFT")
        if not re.fullmatch(r"[0-9a-f]{64}", str(row.get("sha256", ""))):
            fail("M03_EXTERNAL_HASH_INVALID")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--skip-generator-check", action="store_true")
    args = parser.parse_args()
    if not SCHEMA.is_file():
        fail("M03_GOLDEN_SCHEMA_MISSING")
    if not args.skip_generator_check:
        completed = subprocess.run(
            [sys.executable, str(GENERATOR), "--check"],
            cwd=REPO,
            check=False,
            capture_output=True,
            text=True,
        )
        if completed.returncode != 0:
            fail("M03_GOLDEN_GENERATOR_CHECK_FAILED: " + (completed.stderr or completed.stdout).strip())
    validate_manifest()
    validate_external_inventory()
    print(json.dumps({"result": "pass", "fixtures": 8, "external_candidates": len(load_json(INVENTORY)["candidates"])}))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ValueError as exc:
        print(str(exc), file=sys.stderr)
        raise SystemExit(1)
