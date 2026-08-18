#!/usr/bin/env python3
"""Fail-closed semantic tests for the P918 pure reconciliation function."""

from __future__ import annotations

import copy
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/alignment"))
SPEC = importlib.util.spec_from_file_location(
    "asset_live_reconcile", ROOT / "scripts/alignment/reconcile_asset_authority_live.py"
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


def main() -> int:
    ident = {
        "run_id": "run-selftest-0001",
        "candidate_manifest_sha256": "a" * 64,
        "profile_id": "profile-selftest",
        "environment_id": "env-selftest",
        "tenant_ref": "tenant-hash",
        "trace_id": "trace-1",
        "event_id": "event-1",
        "asset_id": "asset-1",
        "revision": "7",
    }
    required_targets = ["asset-projection"]
    run = {
        "artifact_kind": "ASSET_AUTHORITY_LIVE_RUN_MANIFEST",
        "schema_version": "1.0.0",
        "run_id": ident["run_id"],
        "candidate_manifest_sha256": ident["candidate_manifest_sha256"],
        "profile_id": ident["profile_id"],
        "environment_id": ident["environment_id"],
        "environment_class": "AUTHORIZED_REAL_DEPENDENCY",
        "time_window": "2026-08-12T00:00:00Z/2026-08-12T00:10:00Z",
        "production_applied": False,
        "execution_package_sha256": "b" * 64,
        "plan_kind": "EVIDENCE",
        "plan_id": "PLAN-P919-SELFTEST",
        "plan_sha256": "c" * 64,
        "bom_transition_sha256": None,
        "required_projection_targets": required_targets,
        "receipt_refs": {
            kind: {"path": f"doc/selftest-{kind.lower()}.json", "sha256": chr(100 + index) * 64}
            for index, kind in enumerate(MODULE.RECEIPT_KINDS)
        },
    }
    expected_final = hashlib.sha256(json.dumps({
        "asset_id": ident["asset_id"],
        "authority_result_sha256": "3" * 64,
        "event_id": ident["event_id"],
        "revision": ident["revision"],
        "tenant_ref": ident["tenant_ref"],
    }, ensure_ascii=False, separators=(",", ":"), sort_keys=True).encode()).hexdigest()
    common = {
        "artifact_kind": "ASSET_AUTHORITY_LIVE_RECEIPT",
        "schema_version": "1.0.0",
        **ident,
        "issued_at": "2026-08-12T00:05:00Z",
        "trusted_verifier_receipt": {"path": "doc/selftest-trust.json", "sha256": "f" * 64},
        "verification_status": "PASS",
    }
    receipts = {
        "AUTHORITY": {**common, "receipt_id": "receipt-authority", "receipt_kind": "AUTHORITY", "facts": {
            "durable_counts": {"assets": 1, "asset_events": 1, "audit_logs": 1, "asset_event_outbox": 1, "asset_upsert_requests": 1},
            "outbox_status": "published", "published_at": "2026-08-12T00:04:00Z",
            "payload_sha256": "1" * 64, "request_sha256": "2" * 64, "result_sha256": "3" * 64,
            "expected_projection_fact_sha256": expected_final,
            "required_projection_targets": required_targets,
        }},
        "BROKER": {**common, "receipt_id": "receipt-broker", "receipt_kind": "BROKER", "facts": {
            "topic": "asset.events.v2", "required_acks": "all", "async": False, "acked": True,
            "partition": 0, "offset": 9, "header_names": sorted(MODULE.REQUIRED_HEADERS), "payload_sha256": "1" * 64,
        }},
        "PROJECTION": {**common, "receipt_id": "receipt-projection", "receipt_kind": "PROJECTION", "facts": {
            "inbox_count": 1, "inbox_status": "applied", "partition": 0, "offset": 9,
            "watermark": "partition-0:9", "final_fact_sha256": expected_final,
            "required_targets": [{"name": "asset-projection", "required": True, "status": "converged"}],
        }},
    }
    result = MODULE.reconcile_receipts(run, receipts["AUTHORITY"], receipts["BROKER"], receipts["PROJECTION"])
    assert [item["gate_id"] for item in result["gate_results"]] == ["G2", "G3"]
    assert result["unexplained_differences"] == []

    with tempfile.TemporaryDirectory(dir=ROOT) as temporary:
        output = Path(temporary) / "test-result.json"
        paths = MODULE.write_reconciliation_outputs(
            run,
            {"run_manifest": "1" * 64, "authority": "2" * 64, "broker": "3" * 64, "projection": "4" * 64},
            result,
            output,
        )
        assert [path.name for path in paths] == ["test-result.json", "evidence-g2.json", "evidence-g3.json"]
        # Same bytes are an exact replay; changed semantic bytes must not overwrite.
        MODULE.write_reconciliation_outputs(
            run,
            {"run_manifest": "1" * 64, "authority": "2" * 64, "broker": "3" * 64, "projection": "4" * 64},
            result,
            output,
        )
        changed = copy.deepcopy(result)
        changed["authority_revision"] = "8"
        try:
            MODULE.write_reconciliation_outputs(
                run,
                {"run_manifest": "1" * 64, "authority": "2" * 64, "broker": "3" * 64, "projection": "4" * 64},
                changed,
                output,
            )
        except ValueError:
            pass
        else:
            raise AssertionError("immutable writer overwrote different reconciliation bytes")

    mutations = {
        "cross-candidate": lambda r, m: r["BROKER"].update(candidate_manifest_sha256="9" * 64),
        "duplicate-receipt": lambda r, m: r["BROKER"].update(receipt_id=r["AUTHORITY"]["receipt_id"]),
        "stale-receipt": lambda r, m: r["AUTHORITY"].update(issued_at="2026-08-11T23:59:59Z"),
        "missing-ledger": lambda r, m: r["AUTHORITY"]["facts"]["durable_counts"].update(asset_upsert_requests=0),
        "broker-not-acked": lambda r, m: r["BROKER"]["facts"].update(acked=False),
        "offset-mismatch": lambda r, m: r["PROJECTION"]["facts"].update(offset=10),
        "required-target-duplicate": lambda r, m: r["PROJECTION"]["facts"]["required_targets"].append(copy.deepcopy(r["PROJECTION"]["facts"]["required_targets"][0])),
        "final-fact-mismatch": lambda r, m: r["PROJECTION"]["facts"].update(final_fact_sha256="4" * 64),
        "authority-derived-hash-mismatch": lambda r, m: r["AUTHORITY"]["facts"].update(expected_projection_fact_sha256="5" * 64),
        "target-diverged": lambda r, m: r["PROJECTION"]["facts"]["required_targets"][0].update(status="diverged"),
    }
    for name, mutate in mutations.items():
        bad = copy.deepcopy(receipts)
        bad_run = copy.deepcopy(run)
        mutate(bad, bad_run)
        try:
            MODULE.reconcile_receipts(bad_run, bad["AUTHORITY"], bad["BROKER"], bad["PROJECTION"])
        except ValueError:
            continue
        raise AssertionError(f"negative reconciliation accepted: {name}")
    print("PASS P918 reconciliation: positive, immutable G2/G3 writer, plus 10 fail-closed semantic negatives")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
