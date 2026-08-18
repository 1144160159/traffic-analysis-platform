#!/usr/bin/env python3
"""Verify the repository-only T-DQ-001 persistent data-quality slice."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any

from sync_data_quality_postgres_entrypoints import check as check_legacy_entrypoint_mirrors


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/data-quality/control-plane.v1.json")
DATASET_SIGNAL_CONTRACT = Path("contracts/data-quality/dataset-signals.v1.json")
FEATURE_CONTRACT = Path("contracts/alignment/features/F-DATAQUALITY-001.json")
OPENAPI = Path("contracts/openapi/alignment-v1.openapi.json")
MIGRATION = Path("deployments/postgres/migrations/202608041400_data_quality_control_plane_v1.sql")
GOVERNANCE_MIGRATION = Path("deployments/postgres/migrations/202608041500_data_quality_governance_v1.sql")
EVALUATION_MIGRATION = Path("deployments/postgres/migrations/202608041600_data_quality_rule_evaluation_v1.sql")
REPAIR_MIGRATION = Path("deployments/postgres/migrations/202608041700_data_quality_repair_lifecycle_v1.sql")
REPLAY_PROJECTION_MIGRATION = Path("deployments/postgres/migrations/202608041800_data_quality_replay_projection_v1.sql")
MONITOR = Path("go/control-plane/internal/common/dataquality/monitor.go")
HANDOFF_SIGNALS = Path("go/control-plane/internal/common/dataquality/handoff_signals.go")
HANDOFF_REPOSITORY = Path("go/control-plane/internal/common/dataquality/handoff_repository.go")
HANDOFF_TEST = Path("go/control-plane/internal/common/dataquality/handoff_signals_test.go")
MONITOR_PERSISTENCE_TEST = Path("go/control-plane/internal/common/dataquality/monitor_persistence_test.go")
HANDLER = Path("go/control-plane/internal/alert/api/handler_advanced.go")
GOVERNANCE_HANDLER = Path("go/control-plane/internal/alert/api/handler_data_quality_governance.go")
GOVERNANCE = Path("go/control-plane/internal/common/dataquality/governance.go")
GOVERNANCE_TEST = Path("go/control-plane/internal/common/dataquality/governance_integration_test.go")
GOVERNANCE_HTTP_TEST = Path("go/control-plane/internal/alert/api/handler_data_quality_governance_integration_test.go")
EVALUATION = Path("go/control-plane/internal/common/dataquality/evaluation.go")
EVALUATION_TEST = Path("go/control-plane/internal/common/dataquality/evaluation_test.go")
REPAIR = Path("go/control-plane/internal/common/dataquality/repair.go")
REPAIR_TEST = Path("go/control-plane/internal/common/dataquality/repair_test.go")
REPAIR_EVIDENCE = Path("go/control-plane/internal/common/dataquality/repair_evidence.go")
REPAIR_EVIDENCE_TEST = Path("go/control-plane/internal/common/dataquality/repair_evidence_test.go")
REPAIR_EXECUTOR = Path("go/control-plane/internal/common/dataquality/repair_executor.go")
REPAIR_REPLAY_DRIVER = Path("go/control-plane/internal/common/dataquality/repair_replay_driver.go")
REPAIR_REPLAY_DRIVER_TEST = Path("go/control-plane/internal/common/dataquality/repair_replay_driver_test.go")
REPAIR_PROJECTION_CONSUMER = Path("go/control-plane/internal/common/dataquality/repair_projection_consumer.go")
REPAIR_PROJECTION_CONSUMER_TEST = Path("go/control-plane/internal/common/dataquality/repair_projection_consumer_test.go")
MAIN = Path("go/control-plane/cmd/alert-service/main.go")
K8S_SCHEMA = Path("deployments/kubernetes/init-jobs/02-postgres-schema.yaml")
ALERT_DEPLOYMENT = Path("deployments/kubernetes/applications/go-services.yaml")
COMMON_SCHEMA = Path("common/sql/pg/04-tasks-audit.sql")
DOCKER_SCHEMA = Path("go/control-plane/deployments/docker/init/postgres_merged.sql")
RUNBOOK = Path("doc/07_alignment/runbooks/T-DQ-001-persistent-quality-control-plane.md")
CAPTURE = Path("scripts/alignment/capture_data_quality_control_plane.py")
EXPAND_RENDERER = Path("scripts/alignment/render_data_quality_postgres_expand.py")
SCHEMA_CAPTURE = Path("scripts/alignment/capture_data_quality_schema_entrypoints.py")
GOVERNANCE_CAPTURE = Path("scripts/alignment/capture_data_quality_governance.py")

EXPECTED_TABLES = {
    "data_quality_datasets",
    "data_quality_rules",
    "data_quality_baselines",
    "data_quality_watermarks",
    "data_quality_events",
    "data_quality_repairs",
    "data_quality_outbox",
}
EXPECTED_GOVERNANCE_TABLES = {
    "data_quality_dataset_history",
    "data_quality_rule_history",
    "data_quality_command_requests",
}
EXPECTED_EVALUATION_TABLES = {"data_quality_rule_evaluations"}
EXPECTED_REPAIR_TABLES = {"data_quality_repair_history", "data_quality_repair_requests"}
EXPECTED_REPLAY_PROJECTION_TABLES = {"data_quality_flow_replay_projection", "data_quality_replay_projection_receipts"}
EXPECTED_DIMENSIONS = {
    "completeness",
    "uniqueness",
    "timeliness",
    "validity",
    "referential_integrity",
    "ordering",
    "duplicate",
    "lateness",
    "tenant_ownership",
    "object_availability",
}
EXPECTED_SIGNALS = {
    "kafka_offset",
    "flink_watermark",
    "sink_commit",
    "business_version",
    "object_manifest",
}
EXPECTED_MEASUREMENT_STATES = {"measured", "unknown", "not_applicable", "error"}
EXPECTED_SEMANTIC_AXES = {
    "availability_status": {"arrived", "not_arrived", "unavailable", "not_applicable"},
    "freshness_status": {"fresh", "stale", "unknown", "not_applicable"},
    "completeness_status": {"complete", "partial", "unknown", "not_applicable"},
    "value_status": {"zero", "nonzero", "none"},
}


def read(root: Path, relative: Path) -> str:
    return (root / relative).read_text(encoding="utf-8")


def require_tokens(errors: list[str], label: str, source: str, tokens: list[str]) -> None:
    for token in tokens:
        if token not in source:
            errors.append(f"{label} missing token: {token}")


def require_semantic_axes(errors: list[str], label: str, source: dict[str, Any]) -> None:
    axes = source.get("semantic_axes")
    if not isinstance(axes, dict) or set(axes) != set(EXPECTED_SEMANTIC_AXES):
        errors.append(f"{label} semantic axis inventory is incomplete")
        return
    for axis, expected in EXPECTED_SEMANTIC_AXES.items():
        values = axes.get(axis)
        if not isinstance(values, list) or set(values) != expected or len(values) != len(expected):
            errors.append(f"{label} {axis} values are incomplete or duplicated")


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    contract = json.loads(read(root, CONTRACT))
    if contract.get("remediation_id") != "T-DQ-001":
        errors.append("data-quality contract remediation_id must be T-DQ-001")
    if contract.get("status") in {"closed", "complete"}:
        errors.append("repository-only T-DQ-001 contract must not claim closure")
    if contract.get("production_applied") is not False:
        errors.append("repository-only T-DQ-001 contract must not claim production apply")
    expected_persistent_objects = EXPECTED_TABLES | EXPECTED_GOVERNANCE_TABLES | EXPECTED_EVALUATION_TABLES | EXPECTED_REPAIR_TABLES | EXPECTED_REPLAY_PROJECTION_TABLES
    if set(contract.get("persistent_objects", [])) != expected_persistent_objects:
        errors.append("persistent data-quality object inventory is incomplete")
    if set(contract.get("quality_dimensions", [])) != EXPECTED_DIMENSIONS:
        errors.append("quality dimension inventory is incomplete")
    if set(contract.get("real_handoff_signals", [])) != EXPECTED_SIGNALS:
        errors.append("real handoff signal inventory is incomplete")
    if set(contract.get("measurement_states", [])) != EXPECTED_MEASUREMENT_STATES:
        errors.append("data-quality measurement state inventory is incomplete")
    require_semantic_axes(errors, "data-quality control-plane contract", contract)
    if "executed" not in contract.get("repair_states", []):
        errors.append("repair state inventory must distinguish executing from executed")
    authority = contract.get("authority", {})
    if authority.get("migration") != MIGRATION.as_posix():
        errors.append("data-quality authority must bind the versioned PostgreSQL migration")
    if authority.get("governance_migration") != GOVERNANCE_MIGRATION.as_posix():
        errors.append("data-quality authority must bind the versioned governance migration")
    if authority.get("evaluation_migration") != EVALUATION_MIGRATION.as_posix():
        errors.append("data-quality authority must bind the versioned evaluation migration")
    if authority.get("repair_migration") != REPAIR_MIGRATION.as_posix():
        errors.append("data-quality authority must bind the versioned repair migration")
    if authority.get("replay_projection_migration") != REPLAY_PROJECTION_MIGRATION.as_posix():
        errors.append("data-quality authority must bind the replay projection migration")
    if authority.get("expand_renderer") != EXPAND_RENDERER.as_posix():
        errors.append("data-quality authority must bind the approval-gated expand renderer")
    if set(authority.get("generated_compatibility_entrypoints", [])) != {
        COMMON_SCHEMA.as_posix(),
        DOCKER_SCHEMA.as_posix(),
        K8S_SCHEMA.as_posix(),
    }:
        errors.append("data-quality authority must inventory both generated compatibility entrypoints")
    if authority.get("entrypoint_sync") != "scripts/alignment/sync_data_quality_postgres_entrypoints.py --check":
        errors.append("data-quality authority must bind the fail-closed entrypoint sync check")
    if contract.get("dataset_signal_contract") != DATASET_SIGNAL_CONTRACT.as_posix():
        errors.append("control-plane contract must bind the versioned dataset signal contract")
    safety = contract.get("safety", {})
    if safety.get("missing_measurement_state") != "unknown":
        errors.append("missing measurements must remain unknown")
    if safety.get("release_blocking_default") is not False:
        errors.append("release blocking rules must remain default-off")
    if safety.get("separate_requester_and_approver") is not True:
        errors.append("repair approval must separate requester and approver")
    if not contract.get("closure_blockers"):
        errors.append("T-DQ-001 closure blockers must remain explicit")
    expand = contract.get("expand_execution", {})
    if expand.get("renderer") != EXPAND_RENDERER.as_posix():
        errors.append("data-quality expand execution must bind the repository renderer")
    if expand.get("artifact") != "immutable_configmap_secret_and_suspended_job":
        errors.append("data-quality expand artifact must remain immutable and suspended")
    if expand.get("approval_window_max_seconds") != 14400:
        errors.append("data-quality expand approval window must remain bounded to four hours")
    expected_expand_safeguards = {
        "independent_approval_identity",
        "immutable_approval_nonce",
        "bounded_not_before_and_expiry",
        "postgres_system_identifier_precondition",
        "expected_migration_ledger_precondition",
        "g0_candidate_and_manifest_hash_binding",
        "migration_bundle_hash_binding",
        "suspended_by_default",
        "no_retry",
        "psql_stop_on_error",
        "final_migration_ledger_and_table_inventory_verification",
        "refuse_overwrite",
    }
    if set(expand.get("safeguards", [])) != expected_expand_safeguards:
        errors.append("data-quality expand safeguard inventory is incomplete")

    dataset_contract = json.loads(read(root, DATASET_SIGNAL_CONTRACT))
    if dataset_contract.get("contract_id") != "data-quality-dataset-signals-v1":
        errors.append("dataset signal contract_id must be data-quality-dataset-signals-v1")
    if set(dataset_contract.get("measurement_states", [])) != EXPECTED_MEASUREMENT_STATES:
        errors.append("dataset signal contract measurement states are incomplete")
    require_semantic_axes(errors, "dataset signal contract", dataset_contract)
    datasets = dataset_contract.get("datasets", [])
    if len(datasets) != 1 or datasets[0].get("dataset_id") != "flows_raw":
        errors.append("candidate dataset signal contract must define the flows_raw slice")
    else:
        signals = datasets[0].get("signals", [])
        if {item.get("kind") for item in signals} != EXPECTED_SIGNALS:
            errors.append("flows_raw dataset signal kinds are incomplete")
        by_kind = {item.get("kind"): item for item in signals}
        for kind in {"kafka_offset", "flink_watermark", "sink_commit"}:
            if by_kind.get(kind, {}).get("required") is not True or by_kind.get(kind, {}).get("applicability") != "required":
                errors.append(f"flows_raw {kind} must remain required")
        for kind in {"business_version", "object_manifest"}:
            signal = by_kind.get(kind, {})
            if signal.get("required") is not False or signal.get("applicability") != "not_applicable" or not signal.get("reason"):
                errors.append(f"flows_raw {kind} must explicitly remain not_applicable with a reason")
    runtime = dataset_contract.get("runtime", {})
    if runtime.get("default_enabled") is not False or runtime.get("deployment_enabled_after_migration") is not True:
        errors.append("signal collection must be default-off in code and enabled only by candidate deployment after migration")

    feature = json.loads(read(root, FEATURE_CONTRACT))
    if feature.get("feature_id") != "F-DATAQUALITY-001":
        errors.append("Feature Contract must bind F-DATAQUALITY-001")
    if feature.get("status") in {"closed", "observing"}:
        errors.append("partial F-DATAQUALITY-001 must not claim observing or closed")
    compatibility = feature.get("compatibility", {})
    if compatibility.get("preserved_routes") != ["/data-quality"]:
        errors.append("Feature Contract must preserve the /data-quality route")
    if set(compatibility.get("preserved_api_operations", [])) != {
        "GET /v1/data-quality",
        "POST /v1/data-quality/actions",
    }:
        errors.append("Feature Contract must preserve existing data-quality API operations")
    if set(compatibility.get("added_api_operations", [])) != {
        "GET /v1/data-quality/datasets",
        "PUT /v1/data-quality/datasets/{dataset_id}",
        "GET /v1/data-quality/rules",
        "POST /v1/data-quality/rules",
        "POST /v1/data-quality/rules/{rule_id}/transitions",
        "POST /v1/data-quality/events/{quality_event_id}/repairs",
        "POST /v1/data-quality/repairs/{repair_id}/transitions",
    }:
        errors.append("Feature Contract must inventory the additive data-quality governance operations")
    if feature.get("rollout", {}).get("default") is not False:
        errors.append("data-quality release blocking must remain default-off")
    if feature.get("data", {}).get("authoritative_store") != "postgresql":
        errors.append("Feature Contract must keep data-quality control state authoritative in PostgreSQL")
    if feature.get("data", {}).get("dataset_signal_contract") != DATASET_SIGNAL_CONTRACT.as_posix():
        errors.append("Feature Contract must bind the versioned dataset signal contract")
    if set(feature.get("data", {}).get("measurement_states", [])) != EXPECTED_MEASUREMENT_STATES:
        errors.append("Feature Contract measurement states are incomplete")
    require_semantic_axes(errors, "Feature Contract data", feature.get("data", {}))
    feature_api = feature.get("api", {})
    if feature_api.get("side_effects") != "none" or "last PostgreSQL-persisted collection" not in feature_api.get("read_semantics", ""):
        errors.append("Feature Contract must declare GET as a persisted, side-effect-free read")

    openapi = json.loads(read(root, OPENAPI))
    expected_operations = {
        "/v1/data-quality": "getDataQualityControlPlaneSnapshot",
        "/v1/data-quality/baseline": "activateDataQualityBaseline",
        "/v1/data-quality/actions": "createDataQualityContextAction",
    }
    for path, operation_id in expected_operations.items():
        operation = openapi.get("paths", {}).get(path, {}).get("get" if path == "/v1/data-quality" else "post", {})
        if operation.get("operationId") != operation_id:
            errors.append(f"OpenAPI missing data-quality operation: {operation_id}")
        if operation.get("x-feature-id") != "F-DATAQUALITY-001":
            errors.append(f"OpenAPI operation {operation_id} must bind F-DATAQUALITY-001")
    governance_operations = {
        ("/v1/data-quality/datasets", "get"): "listDataQualityDatasets",
        ("/v1/data-quality/datasets/{dataset_id}", "put"): "upsertDataQualityDataset",
        ("/v1/data-quality/rules", "get"): "listDataQualityRules",
        ("/v1/data-quality/rules", "post"): "createDataQualityRuleDraft",
        ("/v1/data-quality/rules/{rule_id}/transitions", "post"): "transitionDataQualityRule",
        ("/v1/data-quality/events/{quality_event_id}/repairs", "post"): "createDataQualityRepair",
        ("/v1/data-quality/repairs/{repair_id}/transitions", "post"): "transitionDataQualityRepair",
    }
    for (path, method), operation_id in governance_operations.items():
        operation = openapi.get("paths", {}).get(path, {}).get(method, {})
        if operation.get("operationId") != operation_id:
            errors.append(f"OpenAPI missing data-quality governance operation: {operation_id}")
        if operation.get("x-feature-id") != "F-DATAQUALITY-001":
            errors.append(f"OpenAPI operation {operation_id} must bind F-DATAQUALITY-001")
        if operation.get("x-required-scope") not in {"data-quality:read", "data-quality:write"}:
            errors.append(f"OpenAPI operation {operation_id} must bind a data-quality scope")

    migration = read(root, MIGRATION)
    for table in sorted(EXPECTED_TABLES):
        if f"CREATE TABLE IF NOT EXISTS {table}" not in migration:
            errors.append(f"authoritative migration missing table: {table}")
    require_tokens(
        errors,
        "authoritative migration",
        migration,
        [
            "schema_sha256",
            "source_watermarks",
            "kafka_offset",
            "flink_watermark",
            "sink_commit",
            "object_manifest",
            "measurement_status",
            "not_applicable",
            "measurement_error",
            "signal_contract_version",
            "collected_at",
            "approved_by <> requested_by",
            "202608041400",
        ],
    )

    evaluation_migration = read(root, EVALUATION_MIGRATION)
    for table in sorted(EXPECTED_EVALUATION_TABLES):
        if f"CREATE TABLE IF NOT EXISTS {table}" not in evaluation_migration:
            errors.append(f"authoritative evaluation migration missing table: {table}")
    require_tokens(
        errors,
        "authoritative evaluation migration",
        evaluation_migration,
        ["window_start", "window_end", "quality_event_id", "source_watermarks", "202608041600"],
    )

    repair_migration = read(root, REPAIR_MIGRATION)
    for table in sorted(EXPECTED_REPAIR_TABLES):
        if f"CREATE TABLE IF NOT EXISTS {table}" not in repair_migration:
            errors.append(f"authoritative repair migration missing table: {table}")
    require_tokens(
        errors,
        "authoritative repair migration",
        repair_migration,
        ["idempotency_key", "request_sha256", "dry_run_completed", "execution_started", "reconciled", "202608041700"],
    )

    replay_projection_migration = read(root, REPLAY_PROJECTION_MIGRATION)
    for table in sorted(EXPECTED_REPLAY_PROJECTION_TABLES):
        if f"CREATE TABLE IF NOT EXISTS {table}" not in replay_projection_migration:
            errors.append(f"authoritative replay projection migration missing table: {table}")
    require_tokens(
        errors,
        "authoritative replay projection migration",
        replay_projection_migration,
        ["flow-replay-pg-v1", "flow.projection-replay.v1", "source_event_sha256", "target_payload_sha256", "202608041800"],
    )

    governance_migration = read(root, GOVERNANCE_MIGRATION)
    for table in sorted(EXPECTED_GOVERNANCE_TABLES):
        if f"CREATE TABLE IF NOT EXISTS {table}" not in governance_migration:
            errors.append(f"authoritative governance migration missing table: {table}")
    require_tokens(
        errors,
        "authoritative governance migration",
        governance_migration,
        [
            "idempotency_key",
            "request_sha256",
            "resulting_revision",
            "response_payload",
            "approved",
            "rejected",
            "202608041500",
        ],
    )

    legacy_entrypoint_errors = check_legacy_entrypoint_mirrors(root)
    errors.extend(legacy_entrypoint_errors)

    monitor = read(root, MONITOR)
    if "kafka_lag_proxy" in monitor or "insert_rate_per_min" in monitor:
        errors.append("ClickHouse insert-rate proxy reintroduced as Kafka lag")
    require_tokens(
        errors,
        "data-quality monitor",
        monitor,
        [
            "SetControlDB",
            "PostgreSQL data quality control plane not available",
            "applyHandoffSignals",
            "currentFlowSchema",
            "data_quality_baselines",
            "data_quality_outbox",
            "DATA_QUALITY_BASELINE_ACTIVATED",
            "pg_advisory_xact_lock",
        ],
    )
    handoff_signals = read(root, HANDOFF_SIGNALS)
    require_tokens(
        errors,
        "data-quality real signal collectors",
        handoff_signals,
        [
            "NewKafkaBrokerOffsetReader",
            "OffsetFetch",
            "ListOffsets",
            "NewFlinkRESTWatermarkReader",
            "currentOutputWatermark",
            "ClickHouseSinkCommitReader",
            "max(ingest_ts)",
            "SignalStatusNotApplicable",
            "SignalAvailabilityNotArrived",
            "SignalAvailabilityUnavailable",
            "SignalCompletenessPartial",
            "SignalValueZero",
            "deriveSignalSemantics",
            "DefaultFlowDatasetContract",
        ],
    )
    handoff_repository = read(root, HANDOFF_REPOSITORY)
    require_tokens(
        errors,
        "data-quality signal persistence",
        handoff_repository,
        [
            "CollectAndPersistHandoffSignals",
            "INSERT INTO data_quality_watermarks",
            "measurement_status=EXCLUDED.measurement_status",
            "tx.Commit()",
            'Status: "unknown"',
            "report.SourceWatermarks",
            "appendUnavailableHandoffCheck",
            "appendNotArrivedHandoffCheck",
            "qualityCheckFromSignal",
        ],
    )
    for function_name, availability in {
        "appendUnavailableHandoffCheck": "SignalAvailabilityUnavailable",
        "appendNotArrivedHandoffCheck": "SignalAvailabilityNotArrived",
    }.items():
        helper = handoff_repository.split(f"func {function_name}", 1)
        body = helper[1].split("\n}", 1)[0] if len(helper) == 2 else ""
        if 'Status: "unknown"' not in body or availability not in body:
            errors.append(f"{function_name} must append typed unknown checks, never pass")
    if "INSERT INTO traffic.flows_raw" in handoff_repository or "INSERT INTO traffic.flows_raw" in handoff_signals:
        errors.append("signal collectors must remain read-only against source systems")
    positions = [
        monitor.find("INSERT INTO data_quality_datasets"),
        monitor.find("INSERT INTO data_quality_baselines"),
        monitor.find("INSERT INTO data_quality_outbox"),
        monitor.find("INSERT INTO audit_logs"),
        monitor.find("tx.Commit()"),
    ]
    if any(position < 0 for position in positions) or positions != sorted(positions):
        errors.append("persistent baseline transaction must order dataset baseline outbox audit commit")

    handler = read(root, HANDLER)
    require_tokens(
        errors,
        "data-quality baseline handler",
        handler,
        [
            "persistent baseline activated",
            '"contract_version":  "data-quality-control-plane-v1"',
            '"snapshot_id":',
            '"as_of":',
            '"missing_sections":',
            '"source_watermarks": baseline.SourceWatermarks',
            '"source_watermarks": report.SourceWatermarks',
            '"error": nil',
        ],
    )
    if "dqMonitor.SetControlDB(db)" not in read(root, MAIN):
        errors.append("alert-service does not bind PostgreSQL to the data-quality monitor")
    require_tokens(
        errors,
        "alert-service signal wiring",
        read(root, MAIN),
        [
            "NewKafkaBrokerOffsetReader",
            "NewFlinkRESTWatermarkReader",
            "RunHandoffSignalCollectionLoop",
            "CollectionEnabled",
        ],
    )

    governance = read(root, GOVERNANCE)
    require_tokens(
        errors,
        "data-quality governance transaction",
        governance,
        [
            "sql.LevelSerializable",
            "pg_advisory_xact_lock",
            "data_quality_dataset_history",
            "data_quality_rule_history",
            "data_quality_command_requests",
            "data_quality_outbox",
            "audit_logs",
            "ErrIdempotencyConflict",
            "ErrSelfApproval",
            "tx.Commit()",
        ],
    )
    governance_handler = read(root, GOVERNANCE_HANDLER)
    require_tokens(
        errors,
        "data-quality governance HTTP handlers",
        governance_handler,
        [
            "ListDataQualityDatasets",
            "UpsertDataQualityDataset",
            "ListDataQualityRules",
            "CreateDataQualityRule",
            "TransitionDataQualityRule",
            "CreateDataQualityRepair",
            "TransitionDataQualityRepair",
            "DataQualityRepairExecutor",
            "DataQualityRepairEvidenceProvider",
            "dataQualityRepairEvidence.DryRun",
            "dataQualityRepairEvidence.Reconcile",
            "DATA_QUALITY_REPAIR_EVIDENCE_UNAVAILABLE",
            "dataQualityRepairExecutor.Ready",
            "DATA_QUALITY_REPAIR_EXECUTOR_UNAVAILABLE",
            "DATA_QUALITY_REPAIR_EXECUTION_DISABLED",
            'r.Header.Get("Idempotency-Key")',
            "SEPARATE_APPROVER_REQUIRED",
        ],
    )

    evaluation = read(root, EVALUATION)
    require_tokens(
        errors,
        "data-quality active-rule evaluator",
        evaluation,
        [
            "WHERE tenant_id=$1 AND status='active'",
            "safeFlowPredicate",
            "tenant_id = ? AND ingest_ts >= ? AND ingest_ts < ?",
            "data_quality_rule_evaluations",
            "data_quality_events",
            "DATA_QUALITY_RULE_EVALUATED",
            "DATA_QUALITY_EVENT_DETECTED",
            "sql.LevelSerializable",
            "pg_advisory_xact_lock",
            "RunRuleEvaluationLoop",
        ],
    )
    require_tokens(
        errors,
        "data-quality active-rule evaluator tests",
        read(root, EVALUATION_TEST),
        [
            "TestSafeFlowPredicateRejectsUnknownInput",
            "TestClickHouseRuleReaderUsesBoundedTenantWindow",
            "TestEvaluateActiveRuleEmptyWindowIsUnknown",
            "TestEvaluateActiveRuleFailurePersistsEventAndTwoOutboxes",
            "TestEvaluateActiveRuleRollsBackWhenOutboxFails",
            "TestEvaluateActiveRulesQueriesOnlyActiveDefinitions",
        ],
    )
    require_tokens(
        errors,
        "data-quality governance integration test",
        read(root, GOVERNANCE_TEST),
        [
            "TestDataQualityGovernanceEphemeralPostgres",
            "ErrIdempotencyConflict",
            "ErrSelfApproval",
            "cross-tenant dataset leak",
            "data_quality_command_requests",
        ],
    )
    require_tokens(
        errors,
        "data-quality governance HTTP integration test",
        read(root, GOVERNANCE_HTTP_TEST),
        [
            "TestDataQualityGovernanceHTTPEphemeralPostgres",
            "write scope status",
            "dataset replay",
            "self approval status",
            "independent approval",
            "cross-tenant dataset leak",
            "cross-tenant repair status",
            "dq-http-repair-execute-001",
            "dq-http-repair-no-executor",
            "dq-http-repair-no-evidence",
            "client dry-run summary was trusted",
        ],
    )

    repair = read(root, REPAIR)
    require_tokens(
        errors,
        "data-quality bounded repair transaction",
        repair,
        [
            "sql.LevelSerializable",
            "pg_advisory_xact_lock",
            'command.OperationID != "flow_replay_window_v1"',
            "end.Sub(start) > time.Hour",
            "data_quality_repair_history",
            "data_quality_repair_requests",
            "ErrRepairApprovalSeparation",
            "ErrRepairExecutionDisabled",
            'command.Action == "reconcile"',
            'case "record_executed":',
            'current.Status == "executed"',
            "tx.Commit()",
        ],
    )
    require_tokens(
        errors,
        "data-quality bounded repair tests",
        read(root, REPAIR_TEST),
        [
            "TestRepairScopeRejectsCrossTenantAndOversizedBudget",
            "TestRepairExecutionIsDefaultOffAndApprovalIsIndependent",
            "TestRepairReconcileRequiresZeroDifference",
            "requester reconciliation must fail",
        ],
    )
    require_tokens(
        errors,
        "data-quality server-derived repair evidence",
        read(root, REPAIR_EVIDENCE),
        [
            "ClickHouseRepairEvidenceProvider",
            "SELECT operation_id,status,input_scope,resource_budget",
            "validateRepairScope",
            "uniqExact(event_id)",
            "groupBitXor(cityHash64(event_id))",
            "WHERE tenant_id = ? AND ingest_ts >= ? AND ingest_ts < ?",
            "data_quality_flow_replay_projection",
            "data_quality_replay_projection_receipts",
            "differenceRepairEventIDs",
        ],
    )
    require_tokens(
        errors,
        "data-quality server-derived repair evidence tests",
        read(root, REPAIR_EVIDENCE_TEST),
        [
            "TestRepairDryRunUsesPersistedTenantScopeAndBoundedClickHouseQuery",
            "TestRepairReconcileFailsClosedUntilProjectionOracleExists",
            "TestRepairReconcileComparesClickHouseTargetAndReceipts",
        ],
    )
    require_tokens(
        errors,
        "data-quality durable repair execution worker",
        read(root, REPAIR_EXECUTOR),
        [
            "WHERE status='executing'",
            "pg_try_advisory_lock",
            "pg_advisory_unlock",
            "loadRepairReplayRequest",
            "validateRepairScope",
            'action := "record_executed"',
            'action = "record_failed"',
            "system:data-quality-repair-executor",
        ],
    )
    require_tokens(
        errors,
        "data-quality authoritative replay projection consumer",
        read(root, REPAIR_PROJECTION_CONSUMER),
        [
            "flow.projection-replay.v1",
            "data_quality_flow_replay_projection",
            "data_quality_replay_projection_receipts",
            "BeginTx",
            "ON CONFLICT DO NOTHING",
            "tx.Commit()",
            "header/body mismatch",
            "commonkafka.Permanent",
        ],
    )
    require_tokens(
        errors,
        "data-quality replay projection consumer tests",
        read(root, REPAIR_PROJECTION_CONSUMER_TEST),
        [
            "TestDecodeFlowReplayProjectionMessageRequiresMatchingBodyHeadersAndKey",
            "TestPostgresFlowReplayProjectionCommitsTargetAndReceiptAtomically",
        ],
    )
    require_tokens(
        errors,
        "data-quality bounded flow replay driver",
        read(root, REPAIR_REPLAY_DRIVER),
        [
            'p.topic == "flow.events.v1"',
            "target consumer readiness is not configured",
            "SELECT count()",
            "source row count is outside approved budget",
            "WHERE tenant_id = ? AND ingest_ts >= ? AND ingest_ts < ?",
            "ORDER BY ingest_ts,event_id",
            'IdempotencyKey: request.RepairID + ":" + r.eventID',
            'Producer: "data-quality-repair-executor"',
        ],
    )
    require_tokens(
        errors,
        "data-quality bounded flow replay driver tests",
        read(root, REPAIR_REPLAY_DRIVER_TEST),
        [
            "TestClickHouseFlowReplayDriverRechecksBudgetDeduplicatesAndPreservesStableIdentity",
            "TestClickHouseFlowReplayDriverFailsBeforePublishingWhenBudgetDrifts",
            "TestKafkaFlowReplayPublisherRefusesRawIngestAndTopicMismatch",
        ],
    )

    k8s = read(root, K8S_SCHEMA)
    for table in sorted(expected_persistent_objects):
        if f"CREATE TABLE IF NOT EXISTS {table}" not in k8s:
            errors.append(f"Kubernetes PostgreSQL schema missing table: {table}")
    require_tokens(
        errors,
        "Kubernetes PostgreSQL migration runner",
        k8s,
        [
            "22-data-quality-control-plane-v1.sql: |",
            "23-data-quality-governance-v1.sql: |",
            "24-data-quality-rule-evaluation-v1.sql: |",
            "25-data-quality-repair-lifecycle-v1.sql: |",
            "26-data-quality-replay-projection-v1.sql: |",
            "22-data-quality-control-plane-v1.sql 23-data-quality-governance-v1.sql 24-data-quality-rule-evaluation-v1.sql 25-data-quality-repair-lifecycle-v1.sql 26-data-quality-replay-projection-v1.sql 27-dashboard-task-execution-pipeline-v1.sql 28-dashboard-task-compensation-v1.sql 29-dashboard-task-dlq-receipt-v1.sql 30-alert-response-external-executor-v1.sql 31-alert-response-dlq-receipt-v1.sql 32-alert-response-reconciliation-compensation-v1.sql 33-alert-evidence-manifest-v1.sql 34-alert-batch-assignment-v1.sql 35-alert-batch-assignment-execution-v1.sql 36-alert-batch-assignment-compensation-v1.sql 37-rule-update-applied-ack-v1.sql 38-rule-version-rollback-v1.sql 39-m07-fusion-snapshots-v1.sql; do",
        ],
    )
    deployment = read(root, ALERT_DEPLOYMENT)
    require_tokens(
        errors,
        "candidate data-quality signal deployment",
        deployment,
        [
            "DATA_QUALITY_SIGNAL_COLLECTION_ENABLED",
            "DATA_QUALITY_SIGNAL_COLLECTION_INTERVAL",
            "DATA_QUALITY_KAFKA_GROUP_ID",
            "DATA_QUALITY_FLINK_REST_URL",
            "DQ_MAX_SIGNAL_AGE",
            "DATA_QUALITY_RULE_EVALUATION_ENABLED",
            "DATA_QUALITY_REPAIR_PROJECTION_ENABLED",
            "DATA_QUALITY_REPAIR_PROJECTION_TOPIC",
            "DATA_QUALITY_REPAIR_EXECUTION_ENABLED",
            "DATA_QUALITY_REPAIR_EVIDENCE_TIMEOUT",
            'value: "false"',
        ],
    )
    require_tokens(
        errors,
        "alert-service repair fail-closed wiring",
        read(root, MAIN),
        [
            "SetDataQualityRepairExecutionFeatureFlag",
            "NewClickHouseRepairEvidenceProvider",
            "NewFlowReplayProjectionConsumer",
            "NewKafkaFlowReplayPublisher",
            "NewRepairExecutionWorker",
            "SetDataQualityRepairExecutor",
            "repairProjectionConsumer.Ready",
            "repairTopicReader.ReadLag",
            "executor_registered",
            "evidence_provider_registered",
            "Data quality repair execution requires enabled PostgreSQL projection",
        ],
    )

    persistence_test = read(root, MONITOR_PERSISTENCE_TEST)
    require_tokens(
        errors,
        "data-quality atomic transaction tests",
        persistence_test,
        [
            "TestUpdateBaselineCommitsDatasetBaselineOutboxAndAuditAtomically",
            "TestUpdateBaselineRollsBackWhenOutboxOrAuditFails",
            '[]string{"outbox", "audit"}',
            "ExpectRollback()",
            "ExpectCommit()",
        ],
    )
    handoff_test = read(root, HANDOFF_TEST)
    require_tokens(
        errors,
        "data-quality hand-off signal tests",
        handoff_test,
        [
            "TestCompositeSignalCollectorKeepsMeasuredUnknownAndNotApplicableDistinct",
            "TestCompositeSignalCollectorIsolatesSourceFailureWithoutFabricatingValue",
            "TestFlinkRESTWatermarkReaderUsesMinimumFiniteSubtaskValue",
            "TestPersistHandoffSignalsCommitsAllFiveStatesAtomically",
            "TestPersistHandoffSignalsRollsBackOnAnyWatermarkFailure",
        ],
    )

    runbook = read(root, RUNBOOK)
    require_tokens(
        errors,
        "data-quality runbook",
        runbook,
        [
            "DATA_QUALITY_RELEASE_BLOCKING_ENABLED=false",
            "unknown/partial",
            "申请人与审批人必须分离",
            "不执行 `DROP`",
            "T+0/T+1/T+3/T+7",
        ],
    )

    capture = read(root, CAPTURE)
    require_tokens(
        errors,
        "data-quality read-only capture",
        capture,
        [
            '"production_applied": False',
            '"candidate_applied": False',
            '"production_mutations": []',
            '"read_only_live_capture": True',
            '"repair_replay_reconcile_manifest": None',
            "202608041500_data_quality_governance_v1.sql",
            "202608041600_data_quality_rule_evaluation_v1.sql",
            "202608041700_data_quality_repair_lifecycle_v1.sql",
            "202608041800_data_quality_replay_projection_v1.sql",
            "EXPECTED_MIGRATIONS",
            '"all_migration_keys_present"',
            '"flink-session-watermark-metric-catalog"',
            '"currentOutputWatermark"',
            '"subtask_metrics"',
            "scan_evidence_secrets",
            '"secrets_captured": bool(secret_hits)',
            "./internal/common/kafka",
            "./internal/alert/config",
            "PARTIAL_PERSISTENT_BASELINE_GOVERNANCE_AND_FLOWS_RAW_REAL_SIGNAL_COLLECTORS_PRE_ROLLOUT",
            "PASS_FOR_BASELINE_GOVERNANCE_SCHEMA_GUARD_ATOMIC_HISTORY_OUTBOX_AUDIT_RECEIPTS_AND_FIVE_SIGNAL_PERSISTENCE",
        ],
    )
    if '"candidate_applied": True' in capture or '"production_applied": True' in capture:
        errors.append("pre-rollout data-quality capture must not claim candidate or production apply")

    require_tokens(
        errors,
        "data-quality PostgreSQL expand renderer",
        read(root, EXPAND_RENDERER),
        [
            "build_snapshot",
            '"immutable": True',
            '"suspend": True',
            '"backoffLimit": 0',
            "APPROVED_CHANGE_ID",
            "EXPECTED_CHANGE_ID",
            "APPROVED_BY",
            "EXPECTED_APPROVER",
            "APPROVED_POSTGRES_SYSTEM_IDENTIFIER",
            "APPROVED_EXPECTED_MIGRATION_STATE",
            "EXPECTED_G0_CANDIDATE_SHA256",
            "EXPECTED_G0_MANIFEST_SHA256",
            "EXPECTED_MIGRATION_BUNDLE_SHA256",
            "CHANGE_WINDOW_NOT_BEFORE_EPOCH",
            "CHANGE_WINDOW_EXPIRES_AT_EPOCH",
            "ON_ERROR_STOP=1",
            'test "$final_state" = "1,1,1,1,1"',
            "refusing to overwrite rendered migration",
        ],
    )

    require_tokens(
        errors,
        "data-quality schema entrypoint capture",
        read(root, SCHEMA_CAPTURE),
        [
            "verify_pg_schema_entrypoints_ephemeral.py",
            '"all_three_entrypoints_replayed_twice"',
            '"schema_hashes_equal"',
            '"versioned_migrations_registered"',
            '"production_applied": False',
            '"production_mutations": []',
        ],
    )
    require_tokens(
        errors,
        "data-quality governance evidence capture",
        read(root, GOVERNANCE_CAPTURE),
        [
            "verify_data_quality_governance_ephemeral.py",
            "candidate_source_stable",
            '"production_applied": False',
            '"production_mutations": []',
            "HTTP_AUTHORIZATION_RULE_APPROVAL_EVALUATION_AND_REPAIR_LIFECYCLE",
            "refusing to overwrite immutable evidence directory",
        ],
    )

    return {
        "schema_version": 1,
        "contract_id": contract.get("contract_id"),
        "feature_id": feature.get("feature_id"),
        "remediation_id": contract.get("remediation_id"),
        "status": "PASS" if not errors else "FAIL",
        "coverage_status": "PARTIAL_PERSISTENT_CONTROL_PLANE_BASELINE_REAL_SIGNALS_DEFAULT_OFF_RULE_EVALUATION_AND_REPAIR_CONTROL_LIFECYCLE",
        "production_applied": False,
        "persistent_objects": sorted(expected_persistent_objects),
        "quality_dimensions": sorted(EXPECTED_DIMENSIONS),
        "real_handoff_signals": sorted(EXPECTED_SIGNALS),
        "implemented_slice": contract.get("implemented_slice"),
        "legacy_entrypoints_with_control_plane": {
            COMMON_SCHEMA.as_posix(): not any(
                error.startswith(f"{COMMON_SCHEMA}:") for error in legacy_entrypoint_errors
            ),
            DOCKER_SCHEMA.as_posix(): not any(
                error.startswith(f"{DOCKER_SCHEMA}:") for error in legacy_entrypoint_errors
            ),
        },
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify(ROOT)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    sys.exit(main())
