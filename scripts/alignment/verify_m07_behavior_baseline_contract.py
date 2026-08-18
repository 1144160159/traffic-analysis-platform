#!/usr/bin/env python3
"""Verify repository-side M07 static/dynamic behavior-baseline authority."""

from __future__ import annotations

import hashlib
import json
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def read(relative: str) -> str:
    return (ROOT / relative).read_text(encoding="utf-8")


def load(relative: str) -> dict:
    return json.loads(read(relative))


def require(condition: bool, message: str, errors: list[str]) -> None:
    if not condition:
        errors.append(message)


def main() -> int:
    errors: list[str] = []
    migration_path = "deployments/postgres/migrations/202608141800_m07_behavior_baseline_v1.sql"
    migration = read(migration_path)
    required_tables = {
        "behavior_baseline_definitions_v1",
        "behavior_baseline_sample_snapshots_v1",
        "behavior_baseline_versions_v1",
        "behavior_baseline_build_jobs_v1",
        "behavior_baseline_approval_requests_v1",
        "behavior_baseline_activation_targets_v1",
        "behavior_baseline_ack_readiness_history_v1",
        "behavior_baseline_ack_readiness_current_v1",
        "behavior_baseline_lifecycle_history_v1",
        "behavior_baseline_lifecycle_outbox_v1",
        "behavior_baseline_command_receipts_v1",
        "behavior_baseline_drift_evaluations_v1",
    }
    created_tables = set(re.findall(r"CREATE TABLE IF NOT EXISTS\s+([a-z0-9_]+)", migration))
    require(required_tables <= created_tables, f"missing baseline tables: {sorted(required_tables-created_tables)}", errors)
    require(
        not re.search(r"(?im)^\s*(DROP\s+(TABLE|SCHEMA)|TRUNCATE|DELETE)\b", migration),
        "behavior baseline expand migration contains destructive data/schema SQL",
        errors,
    )
    for fragment in (
        "baseline_kind IN ('static','dynamic')",
        "lifecycle_state IN ('learning','frozen','active','retired','failed')",
        "eligible_row_count>=minimum_eligible_rows",
        "decided_by IS NULL OR decided_by<>requested_by",
        "publish_state IN ('PENDING','OUTCOME_UNKNOWN','KAFKA_ACKED')",
        "protect_behavior_baseline_version_v1",
        "source_watermark",
        "snapshot_sha256",
        "candidate_sha256",
        "outbox_sequence   BIGSERIAL NOT NULL UNIQUE",
    ):
        require(fragment in migration, f"migration is missing invariant {fragment}", errors)

    k8s = read("deployments/kubernetes/init-jobs/02-postgres-schema.yaml")
    marker = "# BEGIN GENERATED T1-M07 BEHAVIOR BASELINE V1"
    require(k8s.count(marker) == 1, "Kubernetes PostgreSQL entrypoint lacks one baseline generated block", errors)
    require(
        "39-m07-fusion-snapshots-v1.sql 40-m07-behavior-baseline-v1.sql; do" in k8s,
        "behavior baseline migration does not follow fusion migration in the K8s runner",
        errors,
    )
    migration_lines = "\n".join(f"    {line}" if line else "" for line in migration.rstrip().splitlines())
    require(migration_lines in k8s, "Kubernetes behavior baseline migration bytes differ from source", errors)

    contract = load("contracts/alignment/features/F-BASELINE-001.json")
    require(contract.get("status") == "draft", "baseline contract must remain draft before live evidence", errors)
    require(contract.get("rollout", {}).get("default") is False, "behavior baseline authority must remain default off", errors)
    require(set(contract.get("data", {}).get("persistent_objects", [])) == required_tables,
            "baseline contract persistent object set differs from migration authority", errors)

    repository = read("go/control-plane/internal/alert/baseline/repository.go")
    completion = read("go/control-plane/internal/alert/baseline/build_completion.go")
    static_completion = read("go/control-plane/internal/alert/baseline/static_completion.go")
    approval = read("go/control-plane/internal/alert/baseline/approval.go")
    activation = read("go/control-plane/internal/alert/baseline/activation.go")
    evaluator = read("go/control-plane/internal/alert/baseline/evaluator.go")
    dispatcher = read("go/control-plane/internal/alert/baseline/lifecycle_outbox.go")
    ack_consumer = read("go/control-plane/internal/alert/consumer/baseline_activation_ack_consumer.go")
    flink_job = read("java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/UserBehaviorJob.java")
    flink_lifecycle = read("java/flink-jobs/flink-user-behavior-job/src/main/java/com/traffic/flink/behavior/user/baseline/BaselineLifecycleProcessFunction.java")
    reader = read("go/control-plane/internal/alert/baseline/clickhouse_sample_reader.go")
    worker = read("go/control-plane/internal/alert/baseline/worker.go")
    handler = read("go/control-plane/internal/alert/api/handler_baseline_v1.go")
    main_go = read("go/control-plane/cmd/alert-service/main.go")
    for text, fragment, label in (
        (repository, "func (repository *Repository) RequestBuildTx(", "atomic build request"),
        (repository, "behavior_baseline_command_receipts_v1", "idempotency receipt"),
        (completion, "func (repository *Repository) CompleteDynamicBuildTx(", "dynamic build completion"),
        (completion, "sample contains future data", "future-data rejection"),
        (static_completion, "func (repository *Repository) CompleteStaticBuildTx(", "static build completion"),
        (approval, "func (repository *Repository) DecideApprovalTx(", "independent approval decision"),
        (approval, "build requester cannot approve activation", "self-approval rejection"),
        (approval, '"threshold_spec": json.RawMessage(thresholdSpecJSON)', "immutable threshold dispatch"),
        (activation, "func (repository *Repository) RecordActivationAckTx(", "all-target activation ACK transaction"),
        (activation, "func (repository *Repository) RequestRollbackTx(", "immutable rollback version"),
        (activation, '"baseline.version.retired.v1"', "previous-version retirement event"),
        (evaluator, "func (repository *Repository) EvaluateTx(", "version-bound drift evaluation"),
        (dispatcher, "func (dispatcher *LifecycleOutboxDispatcher) Drain(", "lifecycle outbox claim"),
        (dispatcher, "PublishOutcomeUnknownError", "ambiguous Kafka outcome handling"),
        (ack_consumer, "func (consumer *BaselineActivationAckConsumer) Handle(", "strict ACK consumer"),
        (flink_job, 'ConfigUtils.getBoolean(\n                params, "baseline.lifecycle.enabled", false)', "default-off Flink baseline wiring"),
        (flink_lifecycle, "STAGED_BASELINES", "checkpointed staged baseline state"),
        (flink_lifecycle, "ACTIVE_BASELINES", "checkpointed active baseline state"),
        (reader, '"source_completeness_unavailable"', "legacy account quality fail-visible"),
        (reader, "countIf(is_partial=0)", "eligible session filter"),
        (worker, "FOR UPDATE OF j SKIP LOCKED", "durable build claim"),
        (handler, "insertFusionAuditTx", "transactional API audit"),
        (handler, "func (h *SystemHandler) RequestBehaviorBaselineRollbackV1(", "rollback API"),
        (handler, "func (h *SystemHandler) EvaluateBehaviorBaselineV1(", "evaluation API"),
        (main_go, 'BEHAVIOR_BASELINE_BUILD_WORKER_V1_ENABLED', "default-off worker wiring"),
    ):
        require(fragment in text, f"missing {label}", errors)

    for manifest_path in (
        "deployments/kubernetes/applications/go-services.yaml",
        "go/control-plane/deployments/kubernetes/alert-service.yaml",
    ):
        manifest = read(manifest_path)
        for variable in (
            "BEHAVIOR_BASELINE_V1_ENABLED",
            "BEHAVIOR_BASELINE_BUILD_WORKER_V1_ENABLED",
            "BEHAVIOR_BASELINE_ACK_CONSUMER_V1_ENABLED",
            "BEHAVIOR_BASELINE_LIFECYCLE_DISPATCHER_V1_ENABLED",
            "BEHAVIOR_BASELINE_CANDIDATE_SHA256",
        ):
            require(manifest.count(variable) == 1, f"{manifest_path} {variable} is missing or duplicated", errors)

    tests = [
        "go/control-plane/internal/alert/baseline/types_test.go",
        "go/control-plane/internal/alert/baseline/repository_test.go",
        "go/control-plane/internal/alert/baseline/clickhouse_sample_reader_test.go",
        "go/control-plane/internal/alert/baseline/activation_test.go",
        "go/control-plane/internal/alert/baseline/evaluator_test.go",
        "go/control-plane/internal/alert/baseline/lifecycle_outbox_test.go",
        "go/control-plane/internal/alert/consumer/baseline_activation_ack_consumer_test.go",
        "java/flink-jobs/flink-user-behavior-job/src/test/java/com/traffic/flink/behavior/user/baseline/BaselineLifecycleContractTest.java",
        "scripts/alignment/test_sync_m07_baseline_postgres_entrypoint.py",
    ]
    require(all((ROOT / path).is_file() for path in tests), "behavior baseline test inventory is incomplete", errors)

    result = {
        "schema_version": 1,
        "milestone": "T1-M07",
        "feature_id": "F-BASELINE-001",
        "status": "PASS" if not errors else "FAIL",
        "scope": "REPOSITORY_IMPLEMENTED_N008_N009_N010_N011_LIVE_BLOCKED",
        "production_applied": False,
        "kubernetes_live_evidence": False,
        "migration_sha256": hashlib.sha256(migration.encode()).hexdigest(),
        "tables": sorted(required_tables),
        "default_flags": {
            "authority_writer": False,
            "build_worker": False,
            "ack_consumer": False,
            "lifecycle_dispatcher": False,
            "flink_baseline": False,
        },
        "closure_blockers": [
            "no real closed-window ClickHouse sample has been accepted",
            "no digest-bound alert-service or Flink candidate images and savepoint restore evidence exist",
            "no live Kafka lifecycle/ACK offsets, PostgreSQL exact-set activation reconciliation, rollback or observation window exists",
        ],
        "errors": errors,
    }
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main())
