#!/usr/bin/env python3
"""Static and evidence verifier for T1-M09-N018 whitelist governance."""

from __future__ import annotations

import hashlib
import json
import subprocess
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/alignment/whitelist-governance.v2.json")
FEATURE = Path("contracts/alignment/features/F-WHITELIST-001.json")
EVIDENCE = Path("doc/02_acceptance/topic1/tasks/t1-m09-n018/k8s-whitelist-governance-latest.json")
TOPICS = Path("contracts/events/kafka-topic-catalog.v1.json")
SCHEMA = Path("contracts/events/kafka-json-events-v1.schema.json")
OPENAPI = Path("contracts/openapi/alignment-v1.openapi.json")
TEXT_PATHS = (
    "go/control-plane/internal/alert/whitelist/command_atomic.go",
    "go/control-plane/internal/alert/whitelist/producer_readiness.go",
    "go/control-plane/internal/alert/whitelist/handler.go",
    "go/control-plane/internal/alert/whitelist/whitelist.go",
    "go/control-plane/internal/rules/consumer/whitelist_rule_effect_consumer.go",
    "go/control-plane/cmd/alert-service/main.go",
    "go/control-plane/cmd/rule-manager/main.go",
    "web/ui/src/services/whitelistGovernanceApi.ts",
    "web/ui/src/pages/WhitelistGovernancePage.tsx",
    "deployments/postgres/migrations/202608161100_m09_whitelist_consumer_readiness_v2.sql",
    "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
    "go/control-plane/deployments/docker/init/postgres_merged.sql",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "go/control-plane/deployments/kubernetes/rule-manager.yaml",
)


def load_json(path: Path) -> dict[str, Any]:
    return json.loads((ROOT / path).read_text(encoding="utf-8"))


def load_texts() -> dict[str, str]:
    return {relative: (ROOT / relative).read_text(encoding="utf-8") for relative in TEXT_PATHS}


def canonical_definition_sha256(schema: dict[str, Any]) -> str:
    payload = json.dumps(schema["$defs"]["WhitelistLifecycleV2Json"], sort_keys=True, separators=(",", ":")) + "\n"
    return hashlib.sha256(payload.encode()).hexdigest()


def validate_snapshot(
    texts: dict[str, str], contract: dict[str, Any], feature: dict[str, Any],
    evidence: dict[str, Any], topics: dict[str, Any], schema: dict[str, Any],
    openapi: dict[str, Any],
) -> list[str]:
    errors: list[str] = []

    expected_hash = contract.get("event", {}).get("canonical_definition_sha256")
    if canonical_definition_sha256(schema) != expected_hash:
        errors.append("whitelist event canonical definition hash drifted")

    rails = contract.get("runtime_rails", {})
    expected_flags = {
        rails.get("consumer"), rails.get("producer"), rails.get("detection_matcher")
    }
    expected_flags.discard(None)
    if expected_flags != {
        "WHITELIST_EVENT_CONSUMER_V2_ENABLED", "WHITELIST_EVENT_PRODUCER_V2_ENABLED",
        "WHITELIST_DETECTION_MATCHER_V2_ENABLED",
    } or rails.get("default") is not False:
        errors.append("whitelist runtime rail contract is not split and default-off")

    alert_main = texts["go/control-plane/cmd/alert-service/main.go"]
    rule_main = texts["go/control-plane/cmd/rule-manager/main.go"]
    if ".WhitelistEventPipelineEnabled" in alert_main or ".WhitelistEventPipelineEnabled" in rule_main:
        errors.append("deprecated combined whitelist flag still grants runtime authority")
    for token in ("WhitelistEventProducerEnabled", "WhitelistDetectionMatcherEnabled", "VerifyProducerReadiness"):
        if token not in alert_main:
            errors.append(f"alert-service whitelist admission missing {token}")
    for token in ("WhitelistEventConsumerEnabled", "NewPostgresWhitelistRuleProjectionWithReadiness"):
        if token not in rule_main:
            errors.append(f"rule-manager consumer-first startup missing {token}")

    readiness = texts["go/control-plane/internal/alert/whitelist/producer_readiness.go"]
    for token in ("whitelist_consumer_readiness_receipt", "JOIN whitelist_rule_projection", "JOIN whitelist_rule_effects", "effect.status='applied'", "CandidateSHA256"):
        if token not in readiness:
            errors.append(f"producer readiness broker projection join missing {token}")

    projection = texts["go/control-plane/internal/rules/consumer/whitelist_rule_effect_consumer.go"]
    receipt_at = projection.find("INSERT INTO whitelist_consumer_readiness_receipt")
    commit_at = projection.find("tx.Commit()", receipt_at)
    if receipt_at < 0 or commit_at < 0:
        errors.append("consumer readiness receipt is not committed with projection ACK")

    command = texts["go/control-plane/internal/alert/whitelist/command_atomic.go"]
    handler = texts["go/control-plane/internal/alert/whitelist/handler.go"]
    for token in ("validateWhitelistUpdateShape", "one whitelist command cannot combine lifecycle, expiry and assignment changes"):
        if token not in command:
            errors.append(f"whitelist command shape guard missing {token}")
    for token in ("WHITELIST_EXPIRY_NOT_FUTURE", "CommandReason"):
        if token not in handler:
            errors.append(f"whitelist transition guard missing {token}")

    ui_service = texts["web/ui/src/services/whitelistGovernanceApi.ts"]
    ui_page = texts["web/ui/src/pages/WhitelistGovernancePage.tsx"]
    for token in ("command_reason", "rule_ack_event_id", "rule_kafka_partition", "rule_kafka_offset"):
        if token not in ui_service:
            errors.append(f"whitelist UI client missing {token}")
    for token in ("审计与规则投影 ACK", "rule_ack_event_id", "rule_revision", "rule_last_error"):
        if token not in ui_page:
            errors.append(f"whitelist UI ACK view missing {token}")

    for relative in (
        "deployments/postgres/migrations/202608161100_m09_whitelist_consumer_readiness_v2.sql",
        "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
        "go/control-plane/deployments/docker/init/postgres_merged.sql",
    ):
        if "whitelist_consumer_readiness_receipt" not in texts[relative] or "202608161100" not in texts[relative]:
            errors.append(f"whitelist readiness migration missing from {relative}")

    for relative in (
        "deployments/kubernetes/applications/go-services.yaml",
        "go/control-plane/deployments/kubernetes/alert-service.yaml",
        "go/control-plane/deployments/kubernetes/rule-manager.yaml",
    ):
        manifest = texts[relative]
        if "WHITELIST_CONSUMER_CANDIDATE_SHA256" not in manifest or expected_hash not in manifest:
            errors.append(f"whitelist candidate/contract binding missing from {relative}")
        for flag in expected_flags.intersection(set(flag for flag in expected_flags if flag in manifest)):
            window = manifest[manifest.find(flag):manifest.find(flag) + 120]
            if 'false' not in window:
                errors.append(f"{flag} is not default-off in {relative}")

    entry_schema = openapi.get("components", {}).get("schemas", {}).get("WhitelistEntry", {}).get("properties", {})
    transition = openapi.get("components", {}).get("schemas", {}).get("WhitelistTransition", {})
    for field in contract.get("ack_contract", {}).get("ui_fields", []):
        if field not in entry_schema:
            errors.append(f"OpenAPI WhitelistEntry missing {field}")
    if "command_reason" not in transition.get("properties", {}):
        errors.append("OpenAPI WhitelistTransition missing command_reason")

    topic = next((item for item in topics.get("topics", []) if item.get("name") == "whitelist.events.v2"), {})
    if topic.get("readiness") != "producer_candidate_default_off" or topic.get("key_contract") != "tenant_id+entry_id":
        errors.append("whitelist Kafka catalog readiness or key contract is wrong")

    if feature.get("contract_version") != 3 or feature.get("domain", {}).get("network_effect") != "none; this feature changes detection suppression only":
        errors.append("F-WHITELIST-001 does not bind N018 detection-only governance")
    if set(feature.get("rollout", {}).get("feature_flags", [])) != expected_flags:
        errors.append("F-WHITELIST-001 rollout rails differ from N018 contract")

    latest = contract.get("latest_evidence", {})
    if evidence.get("task_id") != "T1-M09-N018" or evidence.get("status") != "PASS" or evidence.get("run_id") != latest.get("run_id"):
        errors.append("N018 Kubernetes evidence identity/status mismatch")
    for field in ("draft_from_fp_feedback", "two_person_approval", "expiry_sweeper", "broker_projection_ack", "deterministic_rule_revision", "producer_readiness_join", "run_scoped_resources_removed"):
        if evidence.get(field) is not True:
            errors.append(f"N018 Kubernetes evidence missing {field}=true")
    for field in ("producer_default_enabled", "consumer_default_enabled", "detection_matcher_default_enabled", "real_network_blocking_executed", "mock_enabled", "shared_postgres_touched", "shared_kafka_touched", "production_applied"):
        if evidence.get(field) is not False:
            errors.append(f"N018 Kubernetes evidence must keep {field}=false")
    if len(evidence.get("kubernetes_jobs", [])) != 4:
        errors.append("N018 Kubernetes evidence does not contain four successful test jobs")
    oracle = evidence.get("postgres_cleanup_oracle", {})
    for field in ("tenants", "whitelist", "history", "outbox", "effects", "projection", "readiness", "audit"):
        if oracle.get(field) != 0:
            errors.append(f"N018 PostgreSQL cleanup oracle {field} is not zero")

    code_only = "\n".join(texts[path] for path in TEXT_PATHS if path.endswith((".go", ".ts", ".tsx")))
    for forbidden in ("exec.Command(\"iptables", "exec.Command(\"nft", "NetworkPolicy", "network blocking executor"):
        if forbidden in code_only:
            errors.append(f"whitelist governance introduced forbidden network action: {forbidden}")
    return errors


def main() -> int:
    contract, feature, evidence = load_json(CONTRACT), load_json(FEATURE), load_json(EVIDENCE)
    topics, schema, openapi = load_json(TOPICS), load_json(SCHEMA), load_json(OPENAPI)
    texts = load_texts()
    errors = validate_snapshot(texts, contract, feature, evidence, topics, schema, openapi)

    for relative, expected in evidence.get("inputs", {}).get("source_sha256", {}).items():
        path = ROOT / relative
        actual = hashlib.sha256(path.read_bytes()).hexdigest() if path.is_file() else "missing"
        if actual != expected:
            errors.append(f"Kubernetes evidence source hash drifted: {relative}")
    sync = subprocess.run(
        [sys.executable, str(ROOT / "scripts/alignment/sync_m09_whitelist_readiness_postgres_entrypoints.py"), "--check"],
        cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False,
    )
    if sync.returncode != 0:
        errors.append("whitelist readiness PG entrypoints are stale: " + sync.stdout.strip())

    if errors:
        for error in errors:
            print(f"FAIL: {error}")
        return 1
    print("PASS: T1-M09-N018 whitelist governance contract and Kubernetes evidence are current")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
