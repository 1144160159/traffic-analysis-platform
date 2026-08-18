#!/usr/bin/env python3
"""P918/P919 exact authority -> broker -> projection reconciliation.

Three top-level functions form the complete production code-unit set:

* reconcile_receipts: pure, deterministic G2/G3 fact comparison;
* write_reconciliation_outputs: immutable result and evidence manifests;
* main: safe CLI loading, trusted-receipt verification, and orchestration.

The module never grants requirement, milestone, release, or production
acceptance.  Formal mode remains blocked until the protected M01 signature
verifier is installed and validates each exact receipt artifact.
"""

from __future__ import annotations

import argparse
from datetime import datetime
import hashlib
import json
import os
from pathlib import Path
from typing import Any

from build_topic1_task_registry import (
    canonical_json,
    load_hashed_json_artifact,
    require_trusted_signature_verifier,
    validate_against_schema,
)


ROOT = Path(__file__).resolve().parents[2]
RUN_SCHEMA = ROOT / "contracts/alignment/asset-authority-live-run-manifest.schema.json"
RECEIPT_SCHEMA = ROOT / "contracts/alignment/asset-authority-live-receipt.schema.json"
RESULT_SCHEMA = ROOT / "contracts/alignment/asset-authority-live-reconcile-result.schema.json"
EVIDENCE_SCHEMA = ROOT / "contracts/alignment/evidence-run-manifest.schema.json"
CANDIDATE_SCHEMA = ROOT / "contracts/alignment/implementation-candidate.schema.json"
RECEIPT_KINDS = ("AUTHORITY", "BROKER", "PROJECTION")
IDENTITY_KEYS = ("tenant_ref", "trace_id", "event_id", "asset_id", "revision")
REQUIRED_HEADERS = {
    "event_id", "event_type", "schema_version", "aggregate_version",
    "tenant_id", "asset_id", "trace_id",
}


def reconcile_receipts(
    run_manifest: dict[str, Any],
    authority: dict[str, Any],
    broker: dict[str, Any],
    projection: dict[str, Any],
) -> dict[str, Any]:
    """Return PASS facts only for one exact, zero-difference identity chain.

    Preconditions: all arguments are in-memory decoded objects.  This function
    performs no I/O, signature verification, clock reads, or output writes.
    It raises ValueError at the first stable invariant violation.
    """
    validate_against_schema(run_manifest, RUN_SCHEMA)
    receipts = {"AUTHORITY": authority, "BROKER": broker, "PROJECTION": projection}
    window_start_raw, window_end_raw = run_manifest["time_window"].split("/", 1)
    window_start = datetime.fromisoformat(window_start_raw.replace("Z", "+00:00"))
    window_end = datetime.fromisoformat(window_end_raw.replace("Z", "+00:00"))
    if window_start >= window_end:
        raise ValueError("run time window is empty or reversed")

    identities: dict[str, dict[str, str]] = {}
    for kind, receipt in receipts.items():
        validate_against_schema(receipt, RECEIPT_SCHEMA)
        if receipt["receipt_kind"] != kind:
            raise ValueError(f"receipt kind mismatch: expected {kind}")
        for field in ("run_id", "candidate_manifest_sha256", "profile_id", "environment_id"):
            if receipt[field] != run_manifest[field]:
                raise ValueError(f"{kind} crosses {field}")
        issued = datetime.fromisoformat(receipt["issued_at"].replace("Z", "+00:00"))
        if issued < window_start or issued > window_end:
            raise ValueError(f"{kind} receipt is stale or outside the authorized window")
        identities[kind] = {key: receipt[key] for key in IDENTITY_KEYS}
    if len({canonical_json(value) for value in identities.values()}) != 1:
        raise ValueError("authority, broker and projection identity exact-sets differ")
    if len({receipt["receipt_id"] for receipt in receipts.values()}) != 3:
        raise ValueError("duplicate receipt identity cannot represent three independent facts")

    af = authority["facts"]
    expected_counts = {
        "assets": 1, "asset_events": 1, "audit_logs": 1,
        "asset_event_outbox": 1, "asset_upsert_requests": 1,
    }
    if af.get("durable_counts") != expected_counts:
        raise ValueError("authority does not reconcile exactly one of five durable effects")
    if af.get("outbox_status") != "published" or af.get("published_at") in {None, ""}:
        raise ValueError("authority outbox is not ACK-qualified published")
    hash_fields = ("payload_sha256", "request_sha256", "result_sha256", "expected_projection_fact_sha256")
    if any(not isinstance(af.get(name), str) or len(af[name]) != 64 for name in hash_fields):
        raise ValueError("authority receipt lacks stable request/result/payload/final hashes")
    required_names = run_manifest["required_projection_targets"]
    if af.get("required_projection_targets") != required_names:
        raise ValueError("authority required projection target set differs from the run contract")
    expected_final_fact = hashlib.sha256(json.dumps({
        "tenant_ref": authority["tenant_ref"],
        "asset_id": authority["asset_id"],
        "revision": authority["revision"],
        "event_id": authority["event_id"],
        "authority_result_sha256": af["result_sha256"],
    }, ensure_ascii=False, separators=(",", ":"), sort_keys=True).encode("utf-8")).hexdigest()
    if af["expected_projection_fact_sha256"] != expected_final_fact:
        raise ValueError("authority expected final fact is not derived from its signed result")

    bf = broker["facts"]
    if (
        bf.get("topic") != "asset.events.v2"
        or bf.get("required_acks") != "all"
        or bf.get("async") is not False
        or bf.get("acked") is not True
        or not isinstance(bf.get("partition"), int) or bf["partition"] < 0
        or not isinstance(bf.get("offset"), int) or bf["offset"] < 0
        or set(bf.get("header_names", [])) != REQUIRED_HEADERS
        or len(bf.get("header_names", [])) != len(REQUIRED_HEADERS)
        or bf.get("payload_sha256") != af["payload_sha256"]
    ):
        raise ValueError("broker ACK/header/payload facts do not match the outbox fact")

    pf = projection["facts"]
    target_rows = pf.get("required_targets")
    if not isinstance(target_rows, list):
        raise ValueError("projection required targets are absent")
    target_names = [item.get("name") for item in target_rows]
    if target_names != required_names or len(target_names) != len(set(target_names)):
        raise ValueError("projection required target exact-set/order/uniqueness differs from the run contract")
    if (
        pf.get("inbox_count") != 1
        or pf.get("inbox_status") != "applied"
        or pf.get("partition") != bf["partition"]
        or pf.get("offset") != bf["offset"]
        or not isinstance(pf.get("watermark"), str) or not pf["watermark"]
        or pf.get("final_fact_sha256") != expected_final_fact
        or any(item.get("required") is not True or item.get("status") != "converged" for item in target_rows)
    ):
        raise ValueError("projection inbox/offset/watermark/final fact has not converged")

    result = {
        "identity": identities["AUTHORITY"],
        "gate_results": [
            {"gate_id": "G2", "result": "PASS", "oracle_ids": ["AUTHORITY-FIVE-EFFECTS", "BROKER-ACK-HEADERS-PAYLOAD", "DURABLE-INBOX-OFFSET"]},
            {"gate_id": "G3", "result": "PASS", "oracle_ids": ["ONE-LOGICAL-PROJECTION", "REQUIRED-TARGETS-EXACT-CONVERGED", "FINAL-FACT-HASH-EQUAL", "ZERO-UNEXPLAINED-DIFF"]},
        ],
        "unexplained_differences": [],
    }
    if [item["gate_id"] for item in result["gate_results"]] != ["G2", "G3"]:
        raise ValueError("reconciliation result must contain exactly ordered G2 and G3")
    return result


def write_reconciliation_outputs(
    run_manifest: dict[str, Any],
    input_sha256: dict[str, str],
    reconciled: dict[str, Any],
    output: Path,
) -> tuple[Path, Path, Path]:
    """Atomically materialize one immutable result and exact G2/G3 manifests."""
    def write_immutable(path: Path, data: bytes) -> None:
        if path.exists():
            if path.read_bytes() != data:
                raise ValueError(f"immutable output exists with different bytes: {path.name}")
            return
        temporary = path.with_name(f".{path.name}.tmp")
        if temporary.exists():
            raise ValueError(f"stale temporary output blocks publication: {temporary.name}")
        try:
            with temporary.open("xb") as handle:
                handle.write(data)
                handle.flush()
                os.fsync(handle.fileno())
            os.replace(temporary, path)
            directory_fd = os.open(path.parent, os.O_RDONLY)
            try:
                os.fsync(directory_fd)
            finally:
                os.close(directory_fd)
        finally:
            if temporary.exists():
                temporary.unlink()

    result = {
        "artifact_kind": "ASSET_AUTHORITY_LIVE_RECONCILE_RESULT",
        "schema_version": "1.0.0",
        "subject_pr_id": "T1-M06-P919-TST-POST-n004-asset-authority-live-reconcile",
        "run_id": run_manifest["run_id"],
        "candidate_manifest_sha256": run_manifest["candidate_manifest_sha256"],
        "profile_id": run_manifest["profile_id"],
        "environment_id": run_manifest["environment_id"],
        "input_sha256": input_sha256,
        **reconciled,
        "result": "PASS",
        "proof_ceiling": "G3_RECONCILIATION_ONLY_NOT_REQUIREMENT_MILESTONE_OR_PRODUCTION_ACCEPTANCE",
    }
    validate_against_schema(result, RESULT_SCHEMA)
    if [item["gate_id"] for item in result["gate_results"]] != ["G2", "G3"]:
        raise ValueError("result gate exact-set is not [G2,G3]")
    encoded = (json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()
    output.parent.mkdir(parents=True, exist_ok=True)
    write_immutable(output, encoded)
    result_sha = hashlib.sha256(encoded).hexdigest()
    manifest_paths: list[Path] = []
    for gate in ("G2", "G3"):
        manifest = {
            "schema_version": "1.0.0",
            "run_id": f"{run_manifest['run_id']}-{gate.lower()}",
            "subject_pr_id": "T1-M06-P919-TST-POST-n004-asset-authority-live-reconcile",
            "subject_work_id": "T1-M06-N004",
            "subject_milestone_id": "T1-M06",
            "execution_package_sha256": run_manifest["execution_package_sha256"],
            "plan_kind": "EVIDENCE",
            "plan_id": run_manifest["plan_id"],
            "plan_sha256": run_manifest["plan_sha256"],
            "bom_transition_sha256": run_manifest["bom_transition_sha256"],
            "candidate_manifest_sha256": run_manifest["candidate_manifest_sha256"],
            "profile_id": run_manifest["profile_id"],
            "environment_id": run_manifest["environment_id"],
            "time_window": run_manifest["time_window"],
            "run_purpose": "RECONCILIATION",
            "gate_id": gate,
            "result": "PASS",
            "artifacts": [{
                "direction": "OUTPUT",
                "artifact_id": f"P919-{gate}-RECONCILE",
                "path": output.relative_to(ROOT).as_posix(),
                "sha256": result_sha,
                "schema_ref": "contracts/alignment/asset-authority-live-reconcile-result.schema.json",
            }],
            "production_applied": run_manifest["production_applied"],
            "exclusions": ["requirement satisfaction", "milestone completion", "production acceptance"],
        }
        validate_against_schema(manifest, EVIDENCE_SCHEMA)
        path = output.with_name(f"evidence-{gate.lower()}.json")
        data = (json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n").encode()
        write_immutable(path, data)
        manifest_paths.append(path)
    return output, manifest_paths[0], manifest_paths[1]


def main() -> int:
    """Validate formal inputs, verify exact receipts, reconcile, and emit outputs."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate-manifest", required=True)
    parser.add_argument("--profile-id", required=True)
    parser.add_argument("--environment-id", required=True)
    parser.add_argument("--run-manifest", required=True)
    parser.add_argument("--authority-receipt", required=True)
    parser.add_argument("--broker-receipt", required=True)
    parser.add_argument("--projection-receipt", required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    candidate_path = (ROOT / args.candidate_manifest).resolve()
    if not candidate_path.is_relative_to(ROOT):
        raise ValueError("candidate manifest path escapes repository")
    candidate_raw = candidate_path.read_bytes()
    candidate = json.loads(candidate_raw)
    validate_against_schema(candidate, CANDIDATE_SCHEMA)
    run_path = (ROOT / args.run_manifest).resolve()
    run_raw = run_path.read_bytes()
    run = json.loads(run_raw)
    validate_against_schema(run, RUN_SCHEMA)
    candidate_sha = hashlib.sha256(candidate_raw).hexdigest()
    if (
        candidate_sha != run["candidate_manifest_sha256"]
        or args.profile_id != run["profile_id"]
        or args.environment_id != run["environment_id"]
        or candidate["environment_id"] != args.environment_id
        or run["plan_kind"] != "EVIDENCE"
    ):
        raise ValueError("run manifest crosses candidate/profile/environment/evidence plan")

    cli_paths = {
        "AUTHORITY": args.authority_receipt,
        "BROKER": args.broker_receipt,
        "PROJECTION": args.projection_receipt,
    }
    receipts: dict[str, dict[str, Any]] = {}
    receipt_hashes: dict[str, str] = {}
    for kind, relative in cli_paths.items():
        ref = run["receipt_refs"][kind]
        if relative != ref["path"]:
            raise ValueError(f"{kind} CLI path differs from the signed run manifest")
        receipt = load_hashed_json_artifact(ref["path"], ref["sha256"], RECEIPT_SCHEMA)
        trust = receipt["trusted_verifier_receipt"]
        load_hashed_json_artifact(trust["path"], trust["sha256"], None)
        require_trusted_signature_verifier(
            f"P919 {kind} receipt path={ref['path']} sha256={ref['sha256']} purpose=ASSET_AUTHORITY_RECONCILIATION"
        )
        receipts[kind] = receipt
        receipt_hashes[kind.lower()] = ref["sha256"]

    reconciled = reconcile_receipts(run, receipts["AUTHORITY"], receipts["BROKER"], receipts["PROJECTION"])
    output = (ROOT / args.output).resolve()
    if not output.is_relative_to(ROOT):
        raise ValueError("output must be repository-relative")
    write_reconciliation_outputs(
        run,
        {"run_manifest": hashlib.sha256(run_raw).hexdigest(), **receipt_hashes},
        reconciled,
        output,
    )
    print("PASS P919 exact G2/G3 authority-broker-projection reconciliation")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
