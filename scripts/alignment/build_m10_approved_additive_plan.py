#!/usr/bin/env python3
"""Build the fail-closed T1-M10-N005 additive change proposal."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
OUTPUT_RELATIVE = Path("deployments/releases/topic1/m10-approved-additive-plan.v1.json")
CANDIDATE_RELATIVE = Path("deployments/releases/topic1/m10-deployable-candidate-closure.v1.json")
PREFLIGHT_RELATIVE = Path("doc/02_acceptance/topic1/tasks/t1-m10-n004/k8s-site-preflight-latest.json")
G1_RECEIPT_RELATIVE = Path("contracts/releases/topic1/approvals/t1-m10-n005-g1-receipt.json")
APPROVAL_RECEIPT_RELATIVE = Path("contracts/releases/topic1/approvals/t1-m10-n005-release-approval.json")

PROPOSED_ARTIFACTS: tuple[tuple[str, str, str], ...] = (
    ("postgres_migration", "deployments/postgres/migrations/202608160030_m09_alert_evidence_links_v1.sql", "T1-M09-N012"),
    ("clickhouse_migration", "deployments/clickhouse/migrations/202608160040_m09_alert_evidence_link_projection_v1.sql", "T1-M09-N012"),
    ("postgres_migration", "deployments/postgres/migrations/202608161100_m09_whitelist_consumer_readiness_v2.sql", "T1-M09-N018"),
    ("kafka_catalog", "contracts/events/kafka-topic-catalog.v1.json", "T1-M09"),
    ("kafka_topic_plan", "deployments/kubernetes/init-jobs/01-kafka-topics.yaml", "T1-M09"),
    ("kafka_acl_plan", "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml", "T1-M09"),
)

REPLAY_POLICY = {
    "mode": "IDEMPOTENT_EXACT_HASH",
    "on_already_applied": "VERIFY_HASH_AND_EFFECT_THEN_SKIP",
    "on_hash_mismatch": "STOP_NO_MUTATION",
}
HALF_FAILURE_POLICY = {
    "mode": "STOP_VERIFY_RESUME_FIRST_UNVERIFIED",
    "checkpoint_after_each_artifact": True,
    "automatic_destructive_rollback": False,
}
COMPATIBILITY_POLICY = {
    "legacy_read": "PRESERVE",
    "legacy_write": "PRESERVE",
    "new_writers": "DEFAULT_OFF_UNTIL_POST_APPLY_VERIFICATION",
    "rollback": "DISABLE_NEW_WRITERS_PRESERVE_FACTS_NO_DROP",
}


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def sha256_file(path: Path) -> str:
    return sha256_bytes(path.read_bytes())


def canonical_sha256(value: Any) -> str:
    return sha256_bytes(json.dumps(value, sort_keys=True, separators=(",", ":")).encode())


def load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError(f"JSON object required: {path}")
    return value


def probe(root: Path, relative: Path) -> dict[str, Any]:
    path = root / relative
    return {
        "path": str(relative),
        "exists": path.is_file(),
        "sha256": sha256_file(path) if path.is_file() else None,
    }


def _strip_sql_comments_and_literals(source: str) -> str:
    """Return SQL tokens while ignoring comments and quoted function/string bodies."""
    pattern = re.compile(
        r"--[^\n]*|/\*.*?\*/|'(?:''|[^'])*'|\$(?P<tag>[A-Za-z_][A-Za-z0-9_]*)?\$.*?\$(?P=tag)\$",
        re.DOTALL,
    )
    return pattern.sub(" ", source)


def destructive_findings(kind: str, source: str) -> list[str]:
    if not kind.endswith("migration"):
        return []
    sql = _strip_sql_comments_and_literals(source)
    checks = (
        ("DROP_STATEMENT", r"\bDROP\s+(?:TABLE|VIEW|MATERIALIZED\s+VIEW|COLUMN|INDEX|TRIGGER|FUNCTION|DATABASE)\b"),
        ("TRUNCATE_STATEMENT", r"\bTRUNCATE\b"),
        ("DELETE_STATEMENT", r"\bDELETE\s+FROM\b"),
        ("ALTER_DROP_OR_RENAME", r"\bALTER\b[^;]*(?:\bDROP\b|\bRENAME\b)"),
    )
    return [code for code, expression in checks if re.search(expression, sql, re.IGNORECASE)]


def candidate_state(root: Path) -> dict[str, Any]:
    result = probe(root, CANDIDATE_RELATIVE)
    if result["exists"]:
        candidate = load_json(root / CANDIDATE_RELATIVE)
        result.update({"status": candidate.get("status"), "candidate_id": candidate.get("candidate_id")})
    else:
        result.update({"status": None, "candidate_id": None})
    return result


def preflight_state(root: Path) -> dict[str, Any]:
    result = probe(root, PREFLIGHT_RELATIVE)
    if result["exists"]:
        preflight = load_json(root / PREFLIGHT_RELATIVE)
        result.update(
            {
                "run_id": preflight.get("run_id"),
                "acceptance_status": preflight.get("acceptance_status"),
                "g6": preflight.get("required_gates", {}).get("G6"),
            }
        )
    else:
        result.update({"run_id": None, "acceptance_status": None, "g6": None})
    return result


def build(root: Path = ROOT) -> dict[str, Any]:
    artifacts: list[dict[str, Any]] = []
    for order, (kind, relative, milestone) in enumerate(PROPOSED_ARTIFACTS, 1):
        path = root / relative
        exists = path.is_file()
        findings = destructive_findings(kind, path.read_text(encoding="utf-8")) if exists else []
        artifacts.append(
            {
                "apply_order": order,
                "kind": kind,
                "path": relative,
                "exists": exists,
                "sha256": sha256_file(path) if exists else None,
                "responsible_milestone": milestone,
                "safety_class": "ADDITIVE_STATIC" if exists and not findings else ("NON_ADDITIVE_BLOCKED" if findings else "MISSING"),
                "destructive_findings": findings,
                "approval_status": "PENDING_CURRENT_CANDIDATE_RECEIPTS",
            }
        )

    candidate = candidate_state(root)
    preflight = preflight_state(root)
    g1 = probe(root, G1_RECEIPT_RELATIVE)
    approval = probe(root, APPROVAL_RECEIPT_RELATIVE)
    blockers: list[str] = []
    if candidate.get("status") != "FROZEN" or not candidate.get("candidate_id"):
        blockers.append("DEPLOYABLE_CANDIDATE_REQUIRED")
    if preflight.get("acceptance_status") != "PASS" or preflight.get("g6") != "PASS":
        blockers.append("N004_PREFLIGHT_G6_PASS_REQUIRED")
    if not g1["exists"]:
        blockers.append("CURRENT_CANDIDATE_G1_RECEIPT_REQUIRED")
    if not approval["exists"]:
        blockers.append("INDEPENDENT_RELEASE_APPROVAL_REQUIRED")
    if any(not item["exists"] for item in artifacts):
        blockers.append("PROPOSED_ARTIFACT_MISSING")
    if any(item["destructive_findings"] for item in artifacts):
        blockers.append("NON_ADDITIVE_ARTIFACT_PRESENT")

    basis = {
        "candidate": candidate,
        "preflight": preflight,
        "receipts": {"g1": g1, "release_approval": approval},
        "artifacts": artifacts,
        "replay_policy": REPLAY_POLICY,
        "half_failure_policy": HALF_FAILURE_POLICY,
        "compatibility_policy": COMPATIBILITY_POLICY,
    }
    blocking_codes = sorted(set(blockers))
    return {
        "schema_version": 1,
        "artifact_kind": "M10_APPROVED_ADDITIVE_CHANGE_PLAN",
        "task_id": "T1-M10-N005",
        "atomic_pr_id": "T1-M10-P011-OPS-n005-s1",
        "status": "AUTHORIZED" if not blocking_codes else "BLOCKED_UNAPPROVED",
        "environment_kind": "KUBERNETES",
        "candidate_id": candidate.get("candidate_id"),
        "plan_basis_sha256": canonical_sha256(basis),
        "candidate": candidate,
        "preflight": preflight,
        "receipts": {"g1": g1, "release_approval": approval},
        "artifacts": artifacts,
        "replay_policy": REPLAY_POLICY,
        "half_failure_policy": HALF_FAILURE_POLICY,
        "compatibility_policy": COMPATIBILITY_POLICY,
        "blocking_codes": blocking_codes,
        "apply_allowed": not blocking_codes,
        "default_runtime_state": "off",
        "shared_infrastructure_touched": False,
        "production_applied": False,
        "allowed_claims": [
            "The proposed M09 database and Kafka artifacts were inventoried with exact hashes",
            "The N005 apply authorization was deterministically evaluated and failed closed",
        ],
        "forbidden_claims": [
            "Any proposal artifact is approved for application",
            "G1 or G6 passed for one deployable candidate",
            "Shared Kubernetes, database, or Kafka infrastructure was changed",
        ],
    }


def render(value: dict[str, Any]) -> str:
    return json.dumps(value, indent=2, sort_keys=True) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / OUTPUT_RELATIVE)
    parser.add_argument("--check", action="store_true")
    args = parser.parse_args()
    allowed = {(ROOT / OUTPUT_RELATIVE).resolve(strict=False), Path("/tmp/m10-approved-additive-plan.json")}
    output = args.output.resolve(strict=False)
    if output not in allowed:
        raise SystemExit("output must be the repository plan or documented /tmp path")
    rendered = render(build(ROOT))
    if args.check:
        if not output.is_file() or output.read_text(encoding="utf-8") != rendered:
            print("FAIL: M10 approved additive plan is stale")
            return 1
        print("PASS: M10 approved additive plan is deterministic and current")
        return 0
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(rendered, encoding="utf-8")
    print(rendered, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
