#!/usr/bin/env python3
"""Fail-closed checks for the T1-M06-N004 development-readiness pilot.

This validator checks design integrity only. It cannot produce READY_BINDING or
formal execution authorization.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

from build_topic1_task_registry import validate_against_schema
from validate_function_design_contracts import validate_code_unit_contract


REPO = Path(__file__).resolve().parents[2]
ROOT = REPO / "doc/02_acceptance/topic1/tasks/t1-m06-n004/design"
LEAF = ROOT / "t1-m06-p007-wrt-n004-s1"
MANIFEST = ROOT / "candidate-manifest.json"
CODE_UNIT = LEAF / "code-unit-contract.v2.json"
MANIFEST_SCHEMA = REPO / "contracts/alignment/design-candidate-manifest.schema.json"
LOCATOR_SCHEMA = REPO / "contracts/alignment/locator-resolution-receipt.schema.json"
DELTA = LEAF / "implementation-delta.v1.json"
DELTA_SCHEMA = REPO / "contracts/alignment/function-implementation-delta.schema.json"
EXECUTION = LEAF / "atomic-pr-execution-package.draft.json"
EXECUTION_SCHEMA = REPO / "contracts/alignment/atomic-pr-execution-package.schema.json"
SOURCE_POLICY = REPO / "contracts/alignment/asset-upsert-source-precedence.v1.json"
SOURCE_POLICY_SCHEMA = REPO / "contracts/alignment/asset-upsert-source-precedence.schema.json"
CLAIM_CATALOG = REPO / "contracts/alignment/developer-claim-package-catalog.v1.json"
TASK_REGISTRY = REPO / "contracts/alignment/task-registry.v1.json"
SECONDARY_FUNCTION_DESIGNS = {
    "T1-M06-P902-WRT-n004-http-commit-unknown-mapping": (
        ROOT / "t1-m06-p902-wrt-n004-http-commit-unknown-mapping",
        "api.(*HTTPHandler).upsertAsset",
    ),
    "T1-M06-P903-WRT-n004-grpc-commit-unknown-mapping": (
        ROOT / "t1-m06-p903-wrt-n004-grpc-commit-unknown-mapping",
        "api.(*AssetHandler).UpsertAsset",
    ),
    "T1-M06-P904-WRT-n004-asset-event-topic-rail": (
        ROOT / "t1-m06-p904-wrt-n004-asset-event-topic-rail",
        "config.(*Config).validate",
    ),
}
SECONDARY_CONTEXT_SYMBOLS = {
    "T1-M06-P902-WRT-n004-http-commit-unknown-mapping": {
        "api.(*HTTPHandler).upsertAsset",
        "api.(*HTTPHandler).ServeHTTP",
        "api.(*HTTPHandler).requireAssetDiscoveryWrite",
        "service.(*AssetService).UpsertAssetAtomic",
        "api.writeAssetCommandError",
        "api.auditActor",
        "api.clientIP",
        "api.writeJSON",
    },
    "T1-M06-P903-WRT-n004-grpc-commit-unknown-mapping": {
        "api.(*AssetHandler).UpsertAsset",
        "trafficv1._AssetService_UpsertAsset_Handler",
        "api.(*AssetHandler).assetUpsertCommandFromGRPC",
        "api.protoToRecord",
        "service.(*AssetService).UpsertAssetAtomic",
        "api.(*AssetHandler).logError",
    },
    "T1-M06-P904-WRT-n004-asset-event-topic-rail": {
        "config.(*Config).validate",
        "config.Load",
    },
}
SECONDARY_PRIMARY_INTERNAL_CALLS = {
    "T1-M06-P902-WRT-n004-http-commit-unknown-mapping": {
        "h.requireAssetDiscoveryWrite", "h.svc.UpsertAssetAtomic",
        "writeAssetCommandError", "auditActor", "clientIP", "writeJSON",
    },
    "T1-M06-P903-WRT-n004-grpc-commit-unknown-mapping": {
        "h.assetUpsertCommandFromGRPC", "protoToRecord",
        "h.svc.UpsertAssetAtomic", "h.logError",
    },
}
PLANNED_TEST_SYMBOLS = {
    "T1-M06-P909-REF-n004-http-commit-unknown-test-fixture": {
        "api.TestAtomicAssetUpsertCommitUnknownReturnsSafePending",
    },
    "T1-M06-P911-REF-n004-grpc-commit-unknown-test-fixture": {
        "api.TestAssetHandlerCommitUnknownReturnsUnavailableSafeMessage",
    },
    "T1-M06-P913-REF-n004-asset-event-topic-rail-test-fixture": {
        "config.TestAssetEventTopicRailFailsClosed",
    },
    "T1-M06-P905-REF-n004-authority-transaction-test-fixture": {
        "repository.TestAssetUpsertIdentityV1GoldenAndBeginFailure",
        "repository.TestUpsertAtomicSameKeyDifferentPayloadZeroWrite",
        "repository.TestUpsertAtomicActionClassSourcePolicy",
        "repository.TestUpsertAtomicCrashMatrix",
        "repository_test.TestUpsertAtomicCommitUnknownSameKeyRecovery",
        "repository_test.TestUpsertAtomicPostgresPreCommitFaultMatrix",
    },
    "T1-M06-P907-REF-n004-asset-event-real-broker-fixture": {
        "consumer.TestAssetProjectionRealKafkaDurableInbox",
        "consumer.TestAssetProjectionKafkaPublishFailureKeepsOutboxPending",
    },
}


def load(path: Path) -> dict:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"expected JSON object: {path}")
    return value


def sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def assert_hashed_ref(ref: dict) -> Path:
    path = (REPO / ref["path"]).resolve()
    if not path.is_relative_to(REPO) or not path.is_file():
        raise ValueError(f"missing or unsafe ref: {ref['path']}")
    if sha(path) != ref["sha256"]:
        raise ValueError(f"stale ref: {ref['path']}")
    return path


def main() -> int:
    manifest = load(MANIFEST)
    validate_against_schema(manifest, MANIFEST_SCHEMA)
    commit = manifest["implementation_candidate_commit"]
    for relative, expected_sha in manifest["source_blob_sha256"].items():
        path = REPO / relative
        if not path.is_file() or sha(path) != expected_sha:
            raise ValueError(f"worktree/candidate source drift: {relative}")

    locator_paths = sorted(LEAF.glob("*locator*.json")) + sorted(LEAF.glob("caller-*.json")) + sorted(LEAF.glob("callee-*.json"))
    if len(locator_paths) != 7:
        raise ValueError(f"expected 7 primary/context locator receipts, got {len(locator_paths)}")
    locator_by_id: dict[str, dict] = {}
    for path in locator_paths:
        receipt = load(path)
        validate_against_schema(receipt, LOCATOR_SCHEMA)
        if receipt["candidate"]["commit"] != commit or receipt["candidate"]["manifest_sha256"] != sha(MANIFEST):
            raise ValueError(f"cross-candidate locator receipt: {path}")
        if receipt["resolver"]["source_sha256"] != sha(REPO / receipt["resolver"]["source_path"]):
            raise ValueError(f"resolver source drift: {path}")
        identity = receipt["locator"]["locator_id"]
        if identity in locator_by_id:
            raise ValueError(f"duplicate locator id: {identity}")
        locator_by_id[identity] = receipt

    contract = load(CODE_UNIT)
    validate_code_unit_contract(contract)
    if contract["candidate"]["manifest_sha256"] != sha(MANIFEST):
        raise ValueError("code-unit/candidate manifest mismatch")
    all_locators = [*contract["context_locators"], *(unit["locator"] for unit in contract["code_units"])]
    if {item["locator_id"] for item in all_locators} != set(locator_by_id):
        raise ValueError("code-unit locator exact-set differs from the seven AST receipts")
    for locator in all_locators:
        receipt_path = assert_hashed_ref(locator["resolver_receipt_ref"])
        receipt = load(receipt_path)
        if receipt["locator"]["locator_id"] != locator["locator_id"]:
            raise ValueError(f"locator ref identity mismatch: {locator['locator_id']}")
        if receipt["locator"]["qualified_symbol"] != locator["qualified_symbol"]:
            raise ValueError(f"locator symbol mismatch: {locator['locator_id']}")
        if receipt["locator"]["normalized_ast_sha256"] != locator["ast_node_sha256"]:
            raise ValueError(f"locator AST mismatch: {locator['locator_id']}")

    unit = contract["code_units"][0]
    primary_receipt = locator_by_id[unit["locator"]["locator_id"]]
    declared_callee_ids = {item["locator_id"] for item in unit["callees"]}
    callee_name_by_id = {
        locator_id: locator_by_id[locator_id]["locator"]["qualified_symbol"].rsplit(".", 1)[-1]
        for locator_id in declared_callee_ids
    }
    actual_local_calls = {
        call["expression"]
        for call in primary_receipt["locator"]["calls"]
        if call["expression"] in set(callee_name_by_id.values())
    }
    if actual_local_calls != set(callee_name_by_id.values()):
        raise ValueError("primary AST receipt local-call exact-set differs from declared callees")
    caller_receipt = locator_by_id[next(item["locator_id"] for item in unit["callers"])]
    if not any(call["expression"].endswith(".UpsertAtomic") for call in caller_receipt["locator"]["calls"]):
        raise ValueError("declared direct caller AST does not invoke UpsertAtomic")

    for name, ref in contract["plan_refs"].items():
        if ref is None:
            raise ValueError(f"missing plan ref: {name}")
        assert_hashed_ref(ref)

    step_ids = {step["step_id"] for step in unit["body_steps"]}
    if step_ids != {f"B{index:02d}" for index in range(1, 17)}:
        raise ValueError("body-step exact-set is not B01-B16")
    effect_ids = {effect["effect_id"] for effect in unit["side_effects"]}
    if effect_ids != {"EF-ASSET", "EF-HISTORY", "EF-AUDIT", "EF-OUTBOX", "EF-LEDGER"}:
        raise ValueError("durable-effect exact-set mismatch")
    caller_ids = {item["locator_id"] for item in unit["callers"]}
    callee_ids = {item["locator_id"] for item in unit["callees"]}
    before_only_callee_ids = {
        item["locator_id"] for item in unit["callees"]
        if item["condition"].startswith("before-state only:")
    }
    after_callee_ids = callee_ids - before_only_callee_ids
    expected_context = set(locator_by_id) - {unit["locator"]["locator_id"]}
    if caller_ids | callee_ids != expected_context or caller_ids & callee_ids:
        raise ValueError("caller/callee exact-set mismatch")
    declared_invokes = {
        value for step in unit["body_steps"] for value in step["invokes"] if value.startswith("LOC-")
    }
    if declared_invokes != after_callee_ids:
        raise ValueError("body-step after-state repository callee exact-set mismatch")
    flow_context = {
        edge["to_locator_or_effect_id"] for edge in contract["call_flow"]["edges"]
        if edge["to_locator_or_effect_id"].startswith("LOC-")
    }
    if flow_context != after_callee_ids:
        raise ValueError("call-flow after-state repository callee exact-set mismatch")
    for step in unit["body_steps"]:
        for invocation in step["invokes"]:
            if invocation.startswith("LOC-") and invocation not in after_callee_ids:
                raise ValueError(f"body step invokes undeclared repository locator: {invocation}")

    delta = load(DELTA)
    validate_against_schema(delta, DELTA_SCHEMA)
    if delta["candidate_manifest_sha256"] != sha(MANIFEST):
        raise ValueError("implementation delta crosses candidate identity")
    delta_steps = {step for item in delta["deltas"] for step in item["body_step_ids"]}
    if not delta_steps.issubset(step_ids):
        raise ValueError("implementation delta references an unknown body step")
    source_policy = load(SOURCE_POLICY)
    validate_against_schema(source_policy, SOURCE_POLICY_SCHEMA)
    if source_policy["candidate_manifest_sha256"] != sha(MANIFEST):
        raise ValueError("source precedence contract crosses candidate identity")
    execution = load(EXECUTION)
    validate_against_schema(execution, EXECUTION_SCHEMA)
    if execution["artifact_status"] != "DRAFT_BINDING" or execution["readiness"]["status"] != "DRAFT":
        raise ValueError("pilot execution package must remain a DRAFT binding")
    design_files = {
        "code_unit_contract": (CODE_UNIT, "CODE_UNIT_CONTRACT"),
        "implementation_delta": (DELTA, "FUNCTION_IMPLEMENTATION_DELTA"),
        "source_precedence_contract": (SOURCE_POLICY, "ASSET_UPSERT_SOURCE_PRECEDENCE"),
    }
    for key, (expected_path, expected_kind) in design_files.items():
        ref = execution["design_refs"][key]
        if ref["artifact_kind"] != expected_kind or assert_hashed_ref(ref) != expected_path.resolve():
            raise ValueError(f"execution package {key} typed design ref mismatch")

    registry = load(TASK_REGISTRY)
    task = next(item for item in registry["tasks"] if item["task_id"] == "T1-M06-N004")
    expected_train = [
        "T1-M06-P901-CTR-n004-source-precedence-contract",
        "T1-M06-P915-REF-n004-source-precedence-validator",
        "T1-M06-P916-TST-PRE-n004-source-precedence-verification",
        "T1-M06-P917-IDX-n004-source-precedence-approval",
        "T1-M06-P007-WRT-n004-s1",
        "T1-M06-P902-WRT-n004-http-commit-unknown-mapping",
        "T1-M06-P909-REF-n004-http-commit-unknown-test-fixture",
        "T1-M06-P910-TST-PRE-n004-http-commit-unknown-verification",
        "T1-M06-P903-WRT-n004-grpc-commit-unknown-mapping",
        "T1-M06-P911-REF-n004-grpc-commit-unknown-test-fixture",
        "T1-M06-P912-TST-PRE-n004-grpc-commit-unknown-verification",
        "T1-M06-P904-WRT-n004-asset-event-topic-rail",
        "T1-M06-P913-REF-n004-asset-event-topic-rail-test-fixture",
        "T1-M06-P914-TST-PRE-n004-asset-event-topic-rail-verification",
        "T1-M06-P905-REF-n004-authority-transaction-test-fixture",
        "T1-M06-P906-TST-PRE-n004-authority-transaction-fault-matrix",
        "T1-M06-P907-REF-n004-asset-event-real-broker-fixture",
        "T1-M06-P908-TST-PRE-n004-asset-event-real-broker-ack",
        "T1-M06-P918-REF-n004-asset-authority-live-reconcile-runner",
        "T1-M06-P919-TST-POST-n004-asset-authority-live-reconcile",
        "T1-M06-P008-IDX-n004-task-completion",
    ]
    if [item["pr_id"] for item in task["pr_sequence"]] != expected_train:
        raise ValueError("M06-N004 registered PR train differs from the reviewed non-reusing exact sequence")
    claim_catalog = load(CLAIM_CATALOG)
    claim = next(item for item in claim_catalog["packages"] if item["atomic_pr_id"] == contract["atomic_pr_id"])
    if (
        claim["claim_mode"] != "DIRECT_TARGET_BOUND"
        or not claim["direct_target_bound"]
        or claim["formal_execution_status"] != "BLOCKED_UNTIL_SIGNED_OVERLAY"
        or claim["change_targets"][0]["symbol_or_pointer"] != unit["locator"]["qualified_symbol"]
    ):
        raise ValueError("developer claim and function contract disagree on exact target or authority ceiling")
    packages_by_id = {item["atomic_pr_id"]: item for item in claim_catalog["packages"]}
    for pr_id, (design_root, expected_symbol) in SECONDARY_FUNCTION_DESIGNS.items():
        candidate = load(design_root / "candidate-manifest.json")
        validate_against_schema(candidate, MANIFEST_SCHEMA)
        for relative, expected_sha in candidate["source_blob_sha256"].items():
            source = REPO / relative
            if not source.is_file() or sha(source) != expected_sha:
                raise ValueError(f"secondary function candidate source drift: {pr_id} {relative}")
        receipt_paths = [design_root / "locator-resolver-receipt.json"]
        receipt_paths.extend(sorted(design_root.glob("caller-*.json")))
        receipt_paths.extend(sorted(design_root.glob("callee-*.json")))
        receipts = [load(path) for path in receipt_paths]
        for path, resolved in zip(receipt_paths, receipts):
            validate_against_schema(resolved, LOCATOR_SCHEMA)
            if (
                resolved["candidate"]["commit"] != candidate["implementation_candidate_commit"]
                or resolved["candidate"]["manifest_sha256"] != sha(design_root / "candidate-manifest.json")
                or resolved["resolver"]["source_sha256"]
                != sha(REPO / resolved["resolver"]["source_path"])
            ):
                raise ValueError(f"secondary locator crosses candidate or resolver: {path}")
        receipt = receipts[0]
        actual_symbols = {item["locator"]["qualified_symbol"] for item in receipts}
        if (
            actual_symbols != SECONDARY_CONTEXT_SYMBOLS[pr_id]
            or receipt["locator"]["qualified_symbol"] != expected_symbol
            or not (design_root / "function-design.md").is_file()
        ):
            raise ValueError(f"secondary function design is not candidate-bound: {pr_id}")
        primary_calls = {item["expression"] for item in receipt["locator"]["calls"]}
        if pr_id in SECONDARY_PRIMARY_INTERNAL_CALLS:
            expected_calls = SECONDARY_PRIMARY_INTERNAL_CALLS[pr_id]
            # Compare the reviewed in-repository collaboration exact-set, not
            # a permissive subset. Standard-library calls remain outside this
            # domain-call set and cannot mask a dropped or extra local edge.
            local_call_tokens = {
                "requireAssetDiscoveryWrite", "UpsertAssetAtomic",
                "writeAssetCommandError", "auditActor", "clientIP", "writeJSON",
                "assetUpsertCommandFromGRPC", "protoToRecord", "logError",
            }
            actual_internal = {
                expression for expression in primary_calls
                if expression.rsplit(".", 1)[-1] in local_call_tokens
            }
            if actual_internal != expected_calls:
                raise ValueError(f"{pr_id} primary in-repository call exact-set differs")
            caller_symbol = (
                "api.(*HTTPHandler).ServeHTTP"
                if pr_id.endswith("http-commit-unknown-mapping")
                else "trafficv1._AssetService_UpsertAsset_Handler"
            )
            caller = next(item for item in receipts if item["locator"]["qualified_symbol"] == caller_symbol)
            expected_edge = (
                "h.upsertAsset"
                if pr_id.endswith("http-commit-unknown-mapping")
                else "srv.(AssetServiceServer).UpsertAsset"
            )
            if not any(call["expression"] == expected_edge for call in caller["locator"]["calls"]):
                raise ValueError(f"{pr_id} declared caller does not invoke the primary function")
        if pr_id.endswith("asset-event-topic-rail"):
            load_receipt = next(
                item for item in receipts
                if item["locator"]["qualified_symbol"] == "config.Load"
            )
            if not any(call["expression"] == "cfg.validate" for call in load_receipt["locator"]["calls"]):
                raise ValueError("P904 declared direct caller does not invoke Config.validate")
        package = packages_by_id[pr_id]
        target = package["change_targets"][0]
        if (
            package["pr_type"] != "WRT"
            or target["symbol_state"] != "EXISTING"
            or target["symbol_or_pointer"] != expected_symbol
            or target["signature_before"] != receipt["locator"]["signature"]
            or target["candidate_blob_sha256"] != receipt["locator"]["candidate_blob_sha256"]
            or len(package["implementation_steps"]) < 4
        ):
            raise ValueError(f"secondary WRT claim lacks exact function design: {pr_id}")
    for pr_id, expected_symbols in PLANNED_TEST_SYMBOLS.items():
        package = packages_by_id[pr_id]
        targets = package["change_targets"]
        planned_symbols = {
            "consumer.TestAssetProjectionKafkaPublishFailureKeepsOutboxPending"
        } if pr_id == "T1-M06-P907-REF-n004-asset-event-real-broker-fixture" else expected_symbols
        if (
            {item["symbol_or_pointer"] for item in targets} != expected_symbols
            or any(
                item["symbol_state"] != "PLANNED"
                or item["signature_before"] is not None
                or item["signature_after"] is None
                or item["candidate_blob_sha256"] is None
                for item in targets if item["symbol_or_pointer"] in planned_symbols
            )
            or any(
                item["symbol_state"] != "EXISTING"
                or item["signature_before"] is None
                or item["candidate_blob_sha256"] is None
                for item in targets if item["symbol_or_pointer"] not in planned_symbols
            )
            or len(package["implementation_steps"]) < 3
        ):
            raise ValueError(f"planned test exact-set is incomplete: {pr_id}")
    exact_commands = {
        "T1-M06-P916-TST-PRE-n004-source-precedence-verification": "--output doc/02_acceptance/topic1/work-orders/t1-m06-p916",
        "T1-M06-P910-TST-PRE-n004-http-commit-unknown-verification": "TestAtomicAssetUpsertCommitUnknownReturnsSafePending",
        "T1-M06-P912-TST-PRE-n004-grpc-commit-unknown-verification": "TestAssetHandlerCommitUnknownReturnsUnavailableSafeMessage",
        "T1-M06-P914-TST-PRE-n004-asset-event-topic-rail-verification": "TestAssetEventTopicRailFailsClosed",
        "T1-M06-P906-TST-PRE-n004-authority-transaction-fault-matrix": "--suite asset-upsert-only",
        "T1-M06-P908-TST-PRE-n004-asset-event-real-broker-ack": "verify_asset_projection_kafka_ephemeral.py",
        "T1-M06-P919-TST-POST-n004-asset-authority-live-reconcile": "reconcile_asset_authority_live.py",
    }
    for pr_id, command_fragment in exact_commands.items():
        commands = [item["command"] for item in packages_by_id[pr_id]["verification_checks"]]
        if not any(command_fragment in command for command in commands):
            raise ValueError(f"evidence leaf lacks its exact no-skip command: {pr_id}")
    if contract["artifact_status"] != "DRAFT" or contract["readiness"]["status"] != "DRAFT":
        raise ValueError("pilot must remain DRAFT until all blockers are closed")
    forbidden = set(contract["claims"]["forbidden"])
    if not {"READY_BINDING", "EXECUTION_AUTHORIZED", "IMPLEMENTATION_COMPLETE", "PRODUCTION_ACCEPTED"}.issubset(forbidden):
        raise ValueError("claim ceiling is incomplete")

    print("PASS T1-M06-N004 pilot: 4 production function designs, 23 AST receipts, B01-B16, 5 effects, planned test exact-sets, typed refs, P901-P919 train and claim ceiling")
    print("PROOF_CEILING DEVELOPMENT_READINESS_DESIGN_ONLY_NOT_EXECUTION_AUTHORIZATION")
    return 0


def run_negative_self_tests() -> None:
    contract = load(CODE_UNIT)
    bad_callee = json.loads(json.dumps(contract))
    bad_callee["code_units"][0]["callees"] = bad_callee["code_units"][0]["callees"][:-1]
    primary = load(LEAF / "locator-resolver-receipt.json")
    expected = {
        item["locator_id"] for item in contract["code_units"][0]["callees"]
    }
    actual = {
        item["locator_id"] for item in bad_callee["code_units"][0]["callees"]
    }
    if actual == expected or not any(
        call["expression"] == "jsonObject" for call in primary["locator"]["calls"]
    ):
        raise ValueError("negative call exact-set mutation did not create the expected mismatch")
    execution = load(EXECUTION)
    stale = json.loads(json.dumps(execution))
    stale["design_refs"]["code_unit_contract"]["sha256"] = "0" * 64
    try:
        assert_hashed_ref(stale["design_refs"]["code_unit_contract"])
    except ValueError:
        pass
    else:
        raise ValueError("stale design hash negative was accepted")
    crossed = json.loads(json.dumps(execution))
    crossed["candidate_manifest_sha256"] = "f" * 64
    if crossed["candidate_manifest_sha256"] == contract["candidate"]["manifest_sha256"]:
        raise ValueError("cross-candidate negative was not mutated")
    print("PASS T1-M06-N004 fail-closed negatives: dropped callee, stale design hash, cross-candidate identity")


if __name__ == "__main__":
    result = main()
    run_negative_self_tests()
    raise SystemExit(result)
