#!/usr/bin/env python3
"""Validate the action-class asset field-precedence contract fail closed.

This checker proves only design-contract exactness.  Domain approval and code
implementation remain separate authority boundaries.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any

from build_topic1_task_registry import validate_against_schema


ROOT = Path(__file__).resolve().parents[2]
SCHEMA = ROOT / "contracts/alignment/asset-upsert-source-precedence.schema.json"
RESULT_SCHEMA = ROOT / "contracts/alignment/asset-upsert-source-precedence-test-result.schema.json"
MANIFEST_SCHEMA = ROOT / "contracts/alignment/design-candidate-manifest.schema.json"
INSTANCE = ROOT / "contracts/alignment/asset-upsert-source-precedence.v1.json"
ASSET_RECORD = ROOT / "go/control-plane/internal/asset/config/config.go"
VALIDATOR = Path(__file__).resolve()
EXPECTED_ACTIONS = {"asset-upsert", "asset-observation-upsert"}
EXPECTED_CLASSES = {"IDENTITY", "GOVERNANCE", "OBSERVATION", "SERVER_MANAGED"}


def load(path: Path) -> dict[str, Any]:
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise ValueError(f"expected JSON object: {path}")
    return payload


def asset_record_fields() -> set[str]:
    text = ASSET_RECORD.read_text(encoding="utf-8")
    body_match = re.search(r"type AssetRecord struct \{(?P<body>.*?)\n\}", text, re.S)
    if body_match is None:
        raise ValueError("AssetRecord declaration is missing")
    fields = set()
    for line in body_match.group("body").splitlines():
        match = re.match(r"\s*([A-Z][A-Za-z0-9_]*)\s+", line)
        if match:
            fields.add(match.group(1))
    return fields


def validate_contract(payload: dict[str, Any]) -> None:
    validate_against_schema(payload, SCHEMA)
    if set(payload["actions"]) != EXPECTED_ACTIONS:
        raise ValueError("source-precedence action exact-set mismatch")
    rules = payload["field_rules"]
    fields = [item["field"] for item in rules]
    if len(fields) != len(set(fields)):
        raise ValueError("source-precedence contract contains duplicate fields")
    source_fields = asset_record_fields()
    if set(fields) != source_fields:
        raise ValueError(
            "source-precedence field exact-set differs from AssetRecord: "
            f"missing={sorted(source_fields - set(fields))} extra={sorted(set(fields) - source_fields)}"
        )
    if {item["class"] for item in rules} != EXPECTED_CLASSES:
        raise ValueError("source-precedence field classes are incomplete")
    for item in rules:
        if item["class"] in {"IDENTITY", "GOVERNANCE"} and "preserve" not in item["stale_observation"]:
            raise ValueError(f"stale observation does not preserve {item['field']}")
        if not item["oracle_ids"]:
            raise ValueError(f"field rule lacks oracle: {item['field']}")
    if payload["status"] == "APPROVED" and payload["approval_blockers"]:
        raise ValueError("APPROVED source-precedence contract still has blockers")


def bind_candidate_sources(candidate_manifest: Path, sources: list[Path]) -> dict[str, str]:
    manifest = load(candidate_manifest)
    validate_against_schema(manifest, MANIFEST_SCHEMA)
    declared = manifest.get("source_blob_sha256")
    if not isinstance(declared, dict):
        raise ValueError("candidate manifest lacks source_blob_sha256")
    actual: dict[str, str] = {}
    for source in sources:
        relative = source.relative_to(ROOT).as_posix()
        source_sha = hashlib.sha256(source.read_bytes()).hexdigest()
        if declared.get(relative) != source_sha:
            raise ValueError(f"candidate manifest does not bind current source: {relative}")
        actual[relative] = source_sha
    return actual


def require_contract_candidate(contract: dict[str, Any], candidate_manifest: Path) -> None:
    manifest_sha = hashlib.sha256(candidate_manifest.read_bytes()).hexdigest()
    if contract.get("candidate_manifest_sha256") != manifest_sha:
        raise ValueError("source-precedence contract crosses candidate manifest identity")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--instance", type=Path, default=INSTANCE)
    parser.add_argument("--self-test", action="store_true")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--candidate-manifest", type=Path)
    parser.add_argument("--profile-id")
    parser.add_argument("--environment-id")
    parser.add_argument("--run-id")
    args = parser.parse_args()
    payload = load(args.instance.resolve())
    validate_contract(payload)
    case_results: list[dict[str, str]] = []
    if args.self_test:
        missing = json.loads(json.dumps(payload))
        missing["field_rules"] = missing["field_rules"][:-1]
        duplicate = json.loads(json.dumps(payload))
        duplicate["field_rules"].append(duplicate["field_rules"][0])
        approved_with_blocker = json.loads(json.dumps(payload))
        approved_with_blocker["status"] = "APPROVED"
        for name, bad in (
            ("missing-field", missing),
            ("duplicate-field", duplicate),
            ("approved-with-blocker", approved_with_blocker),
        ):
            try:
                validate_contract(bad)
            except ValueError:
                print(f"PASS negative {name}")
                case_results.append({"case_id": name, "expected": "REJECT", "actual": "REJECT", "status": "PASS"})
            else:
                raise ValueError(f"negative source-precedence case accepted: {name}")
        crossed = json.loads(json.dumps(payload))
        crossed["candidate_manifest_sha256"] = "f" * 64
        try:
            require_contract_candidate(crossed, ROOT / "doc/02_acceptance/topic1/tasks/t1-m06-n004/design/candidate-manifest.json")
        except ValueError:
            print("PASS negative cross-candidate-contract")
            case_results.append({"case_id": "cross-candidate-contract", "expected": "REJECT", "actual": "REJECT", "status": "PASS"})
        else:
            raise ValueError("negative source-precedence cross-candidate contract accepted")
    case_results.insert(0, {"case_id": "current-contract", "expected": "PASS", "actual": "PASS", "status": "PASS"})
    if args.output is not None:
        if (
            args.candidate_manifest is None or not args.profile_id
            or not args.environment_id or not args.run_id
        ):
            parser.error(
                "--output requires --candidate-manifest, --profile-id, --environment-id and --run-id"
            )
        candidate_manifest = (ROOT / args.candidate_manifest).resolve()
        if not candidate_manifest.is_relative_to(ROOT) or not candidate_manifest.is_file():
            raise ValueError("candidate manifest must be a repository-relative existing file")
        require_contract_candidate(payload, candidate_manifest)
        # Candidate manifests bind mutable implementation sources only.  The
        # contract already binds this manifest, so adding the contract (or
        # this validator) to source_blob_sha256 would create a manifest ->
        # contract -> manifest hash cycle.  Their hashes are carried as
        # independent downstream artifact identities in this result.
        source_hashes = bind_candidate_sources(candidate_manifest, [ASSET_RECORD])
        output = args.output.resolve()
        if not output.is_relative_to(ROOT):
            raise ValueError("output must be repository-relative")
        instance_path = args.instance.resolve()
        result = {
            "artifact_kind": "ASSET_SOURCE_PRECEDENCE_TEST_RESULT",
            "schema_version": "1.0.0",
            "run_id": args.run_id,
            "subject_pr_id": "T1-M06-P916-TST-PRE-n004-source-precedence-verification",
            "candidate_manifest": {
                "path": str(candidate_manifest.relative_to(ROOT)),
                "sha256": hashlib.sha256(candidate_manifest.read_bytes()).hexdigest(),
            },
            "candidate_manifest_sha256": hashlib.sha256(candidate_manifest.read_bytes()).hexdigest(),
            "profile_id": args.profile_id,
            "environment_id": args.environment_id,
            "contract_path": str(instance_path.relative_to(ROOT)),
            "contract_sha256": hashlib.sha256(instance_path.read_bytes()).hexdigest(),
            "asset_record_path": str(ASSET_RECORD.relative_to(ROOT)),
            "asset_record_sha256": hashlib.sha256(ASSET_RECORD.read_bytes()).hexdigest(),
            "validator_path": str(VALIDATOR.relative_to(ROOT)),
            "validator_sha256": hashlib.sha256(VALIDATOR.read_bytes()).hexdigest(),
            "source_blob_sha256": source_hashes,
            "self_test": args.self_test,
            "cases": case_results,
            "result": "PASS",
            "proof_ceiling": "SOURCE_PRECEDENCE_DESIGN_VALIDATION_ONLY_NOT_DOMAIN_APPROVAL_OR_EXECUTION_AUTHORIZATION",
        }
        validate_against_schema(result, RESULT_SCHEMA)
        encoded = (json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode("utf-8")
        output.parent.mkdir(parents=True, exist_ok=True)
        if output.exists() and output.read_bytes() != encoded:
            raise ValueError(f"immutable output exists with different bytes: {output.relative_to(ROOT)}")
        if not output.exists():
            output.write_bytes(encoded)
    print("PASS asset source-precedence actions, AssetRecord field exact-set, classes, oracles and approval ceiling")
    print("PROOF_CEILING PROPOSED_DOMAIN_CONTRACT_ONLY_NOT_IMPLEMENTATION_OR_EXECUTION_AUTHORIZATION")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
