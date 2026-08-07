#!/usr/bin/env python3
"""Verify the default-off T-OS-004 durable projection reconciliation candidate."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = Path("contracts/opensearch/projection-reconciliation.v1.json")
FEATURE = Path("contracts/alignment/features/F-SEARCH-001.json")
DUAL = Path("go/control-plane/internal/alert/persistence/dual_writer.go")
DEBT = Path("go/control-plane/internal/alert/persistence/projection_debt.go")
OS_WRITER = Path("go/control-plane/internal/alert/persistence/opensearch.go")
CH_REPOSITORY = Path("go/control-plane/internal/alert/repository/clickhouse.go")
CONSUMER = Path("go/control-plane/internal/alert/consumer/kafka_consumer.go")
COMMON_CONSUMER = Path("go/control-plane/internal/common/kafka/consumer.go")
WORKER = Path("go/control-plane/internal/alert/projection/worker.go")
RECONCILE = Path("go/control-plane/internal/alert/projection/reconcile.go")
CLI = Path("go/control-plane/cmd/alert-projection-reconcile/main.go")
CONFIG = Path("go/control-plane/internal/alert/config/config.go")
MAIN = Path("go/control-plane/cmd/alert-service/main.go")
MIGRATION = Path("deployments/postgres/migrations/202608041100_alert_opensearch_projection_reconciliation_v1.sql")
DOCKER_SQL = Path("go/control-plane/deployments/docker/init/postgres_merged.sql")
K8S_SQL = Path("deployments/kubernetes/init-jobs/02-postgres-schema.yaml")
DEPLOYMENT = Path("deployments/kubernetes/applications/go-services.yaml")
STANDALONE_DEPLOYMENT = Path("go/control-plane/deployments/kubernetes/alert-service.yaml")
CAPTURE = Path("scripts/alignment/capture_opensearch_projection_reconciliation.py")
RUNBOOK = Path("doc/07_alignment/runbooks/T-OS-004-alert-projection-rebuild-reconcile.md")
TESTS = (
    Path("go/control-plane/internal/alert/persistence/dual_writer_projection_test.go"),
    Path("go/control-plane/internal/alert/persistence/opensearch_external_version_test.go"),
    Path("go/control-plane/internal/alert/persistence/projection_debt_test.go"),
    Path("go/control-plane/internal/alert/persistence/projection_debt_integration_test.go"),
    Path("go/control-plane/internal/alert/repository/clickhouse_projection_query_test.go"),
    Path("go/control-plane/internal/alert/projection/worker_test.go"),
    Path("go/control-plane/internal/alert/projection/reconcile_test.go"),
    Path("go/control-plane/internal/alert/projection/reconcile_integration_test.go"),
    Path("go/control-plane/internal/alert/consumer/projection_receipt_real_kafka_integration_test.go"),
    Path("go/control-plane/internal/common/kafka/consumer_commit_observer_test.go"),
)


def load_json(root: Path, relative: Path) -> dict[str, Any]:
    return json.loads((root / relative).read_text(encoding="utf-8"))


def verify(root: Path = ROOT) -> dict[str, Any]:
    errors: list[str] = []
    required = (CONTRACT, FEATURE, DUAL, DEBT, OS_WRITER, CH_REPOSITORY, CONSUMER, COMMON_CONSUMER, WORKER, RECONCILE,
                CLI, CONFIG, MAIN, MIGRATION, DOCKER_SQL, K8S_SQL, DEPLOYMENT, STANDALONE_DEPLOYMENT, CAPTURE, RUNBOOK, *TESTS)
    missing = [str(path) for path in required if not (root / path).is_file()]
    if missing:
        return {"status": "FAIL", "errors": [f"missing required files: {missing}"]}

    contract = load_json(root, CONTRACT)
    if contract.get("remediation_id") != "T-OS-004" or contract.get("feature_id") != "F-SEARCH-001":
        errors.append("contract must bind T-OS-004 to canonical F-SEARCH-001")
    if contract.get("status") in {"closed", "complete", "pass"}:
        errors.append("partial candidate must not claim T-OS-004 closure")
    if contract.get("coverage_status") != "PARTIAL" or contract.get("production_applied") is not False:
        errors.append("candidate must remain PARTIAL with production_applied=false")
    if not contract.get("closure_blockers"):
        errors.append("real services, performance, canary and observation blockers must remain explicit")
    boundary = contract.get("write_boundary", {})
    if boundary.get("clickhouse_success_opensearch_failure") != "projection_pending_only_after_durable_debt":
        errors.append("CH success plus OS failure must require durable projection debt")
    if boundary.get("debt_persistence_failure") != "return_error_and_block_offset_commit":
        errors.append("debt persistence failure must block Kafka offset advancement")
    if boundary.get("opensearch_success_with_receipt_store") != "record_applied_watermarks_before_offset_commit":
        errors.append("successful OpenSearch writes must record applied watermarks before Kafka advances")
    if boundary.get("precise_partial_bulk") != "atomically_record_applied_watermarks_and_failed_debts_before_offset_commit":
        errors.append("precise partial bulk outcomes must have one atomic PostgreSQL receipt")
    if boundary.get("receipt_persistence_failure") != "return_error_and_block_offset_commit":
        errors.append("applied receipt persistence failure must block Kafka offset advancement")
    if boundary.get("version_type") != "external_gte" or boundary.get("projection_id") != "alert_id":
        errors.append("deterministic alert_id and external_gte guards drifted")
    scope = contract.get("rebuild_scope", {})
    if scope.get("maximum_documents") != 10000 or scope.get("automatic_delete_extra") is not False:
        errors.append("bounded rebuild or no-auto-delete guard drifted")
    if scope.get("automatic_repairs") != ["missing", "stale"]:
        errors.append("automatic repair must be limited to missing and stale")
    if scope.get("repair_terminal_receipt") != "refresh_exact_write_alias_then_requery_same_scope":
        errors.append("repair terminal receipt must refresh and requery the same bounded scope")
    if scope.get("repair_converged") != "remaining_missing_stale_and_watermark_mismatches_are_zero_and_error_count_is_zero":
        errors.append("repair convergence definition drifted")
    if scope.get("watermark_terminal_receipt") != "compare_source_version_and_authoritative_sha256_in_postgresql_repair_missing_receipts_then_requery":
        errors.append("PostgreSQL watermark terminal receipt guard drifted")
    if scope.get("cross_service_terminal_receipt") != "same_owned_run_real_opensearch_requery_and_real_postgresql_watermark_requery":
        errors.append("cross-service terminal receipt guard drifted")
    if scope.get("authoritative_three_store_receipt") != "same_owned_run_real_clickhouse_authority_real_opensearch_target_real_postgresql_receipt_equal_hash_and_version":
        errors.append("authoritative three-store receipt guard drifted")
    if scope.get("kafka_five_store_receipt") != "same_owned_run_real_kafka_commit_after_redis_dedup_clickhouse_authority_opensearch_target_and_postgresql_applied_receipt":
        errors.append("Kafka five-store receipt guard drifted")
    if scope.get("kafka_receipt_failure_recovery") != "postgresql_receipt_failure_retains_offset_same_group_restart_redelivers_and_reconciles":
        errors.append("Kafka PostgreSQL receipt failure recovery guard drifted")
    runtime = contract.get("runtime_guards", {})
    if runtime.get("feature_flag_default_enabled") is not False or runtime.get("production_mutations_in_repository_candidate") != []:
        errors.append("candidate must remain default-off with no production mutation claim")
    if runtime.get("source_or_target_truncation_is_partial") is not True:
        errors.append("truncated reconciliation must remain partial")
    if contract.get("canonical_hash", {}).get("runtime_fields_excluded") != ["attack_phase", "arkime_link", "evidence_count"]:
        errors.append("canonical hash runtime-field exclusion drifted")
    if contract.get("canonical_hash", {}).get("timestamp_precision") != "utc_milliseconds_before_storage_writes_and_hashing":
        errors.append("canonical alert timestamp precision drifted")
    observability = contract.get("observability", {})
    if observability.get("metrics_service_port") != 9093 or observability.get("metrics_target_port") != 8082:
        errors.append("alert metrics service-to-listener mapping drifted")
    if observability.get("live_capture_transport") != "kubernetes_service_proxy_without_container_shell":
        errors.append("distroless-safe live metrics capture transport drifted")
    compatibility = contract.get("read_compatibility", {})
    if compatibility.get("legacy_only_field_alias") != {"dedup_fingerprint": "fingerprint"}:
        errors.append("legacy projection field alias drifted")
    if compatibility.get("v2_canonical_write_field") != "fingerprint" or compatibility.get("legacy_alias_must_not_override_non_empty_canonical_field") is not True:
        errors.append("canonical fingerprint precedence guard drifted")
    if compatibility.get("projection_alias_must_not_shadow_raw_filter_column") is not True:
        errors.append("ClickHouse projection alias shadow guard drifted")

    feature = load_json(root, FEATURE)
    if feature.get("feature_id") != "F-SEARCH-001" or feature.get("data", {}).get("authoritative_store") != "clickhouse":
        errors.append("canonical F-SEARCH-001 authority contract drifted")

    tokens = {
        DUAL: ("WriteBatchWithOutcome", "ProjectionPendingError", "RecordProjectionOutcome", "projectionDebtAlerts", "projectionAppliedAlerts", "ReceiptRecorded"),
        CONSUMER: ("outcome.ClickHouseCommitted && outcome.DebtRecorded", "WriteBatchWithOutcome",
                   "alert_consumer_last_committed_offset", "alert_consumer_last_committed_event_info", "SetCommitObserver"),
        COMMON_CONSUMER: ("SetCommitObserver", "notifyCommitObserver(messages)", "CommitsSucceeded"),
        DEBT: ("FOR UPDATE SKIP LOCKED", "alert_opensearch_projection_debts", "alert_opensearch_projection_watermarks", "AlertProjectionSHA256", "ListProjectionWatermarkMismatches", "RecordProjectionOutcome", "jsonb_to_recordset"),
        OS_WRITER: ('VersionType: "external_gte"', '"version_type": "external_gte"', '"search_after"', "scope.TargetIndexVersion != w.TargetVersion()", "RefreshProjectionTarget",
                    'DedupFingerprint string `json:"dedup_fingerprint"`', "canonicalAlert(w.legacyReadKeywordFields)"),
        CH_REPOSITORY: ("AS first_seen_time", "AS last_seen_time"),
        WORKER: ("TargetIndexVersion != w.target.TargetVersion()", "RetryProjectionDebt", "ResolveProjectionDebt"),
        RECONCILE: ('request.Mode != "plan" && request.Mode != "repair"', '"bounded_scope_truncated"', "result.MissingIDs", "result.StaleIDs", "result.ExtraIDs", "post_repair_differences_remain", "RepairConverged"),
        CLI: ('"confirm-repair"', '"target-index-version"', '"max-documents"'),
        CONFIG: ('env:"OPENSEARCH_ALERT_PROJECTION_RECONCILE_V1_ENABLED" envDefault:"false"', 'env:"OPENSEARCH_ALERT_PROJECTION_REBUILD_MAX_DOCUMENTS" envDefault:"10000"'),
        MAIN: ("SetProjectionDebtRecorder", "projectionWorker.Run(ctx)"),
        CAPTURE: ('"alert_consumer_lag"', '"alert_consumer_last_committed"', "if line.startswith(prefixes)"),
    }
    for path, required_tokens in tokens.items():
        text = (root / path).read_text(encoding="utf-8")
        for token in required_tokens:
            if token not in text:
                errors.append(f"implementation guard missing in {path}: {token}")

    for path in (MIGRATION, DOCKER_SQL, K8S_SQL):
        text = (root / path).read_text(encoding="utf-8")
        for token in ("202608041100", "alert_opensearch_projection_debts", "alert_opensearch_projection_watermarks", "alert_opensearch_reconcile_runs"):
            if token not in text:
                errors.append(f"migration entrypoint drift in {path}: {token}")
    deployment = (root / DEPLOYMENT).read_text(encoding="utf-8")
    if 'OPENSEARCH_ALERT_PROJECTION_RECONCILE_V1_ENABLED, value: "false"' not in deployment:
        errors.append("Kubernetes candidate must explicitly keep automatic reconcile disabled")
    for token in ('name: metrics, port: 9093, targetPort: 8082', 'prometheus.io/port: "8082"'):
        if token not in deployment:
            errors.append(f"alert metrics deployment mapping drifted: {token}")
    standalone = (root / STANDALONE_DEPLOYMENT).read_text(encoding="utf-8")
    for token in ("name: metrics", "port: 9093", "targetPort: 8082", 'prometheus.io/port: "8082"'):
        if token not in standalone:
            errors.append(f"standalone alert metrics mapping drifted: {token}")

    test_text = "\n".join((root / path).read_text(encoding="utf-8") for path in TESTS)
    for token in ("DebtPersistenceFailureBlocksCommit", "RecordsOnlyAcknowledgedBulkFailures", "ExternalGTEVersioning",
                  "ProjectionDebtBatchCommitsAtomically", "CommitObserverReceivesIsolatedCommittedMessages",
                  "DoesNotWriteWrongIndexGeneration", "ClassifiesMissingExtraAndStale", "StopsOnTruncationBeforeRepair", "StopsAtRepairErrorThreshold", "FailsReceiptWhenRefreshFails",
                  "DoesNotConvergeWhenAcknowledgedWriteIsNotVisible", "DoesNotConvergeWhenWatermarkWriteFails",
                  "RetriesMissingWatermarkAfterTargetAlreadyConverged", "ProjectionWatermarkMismatchQueryIsBoundedAndVersioned",
                  "AlertProjectionRepairTerminalReceiptRealOpenSearch", "AlertProjectionRepairRealPostgresAndOpenSearch",
                  "AlertProjectionRepairRealClickHousePostgresAndOpenSearch", "WriteBatchAppliedReceiptFailureBlocksCommit",
                  "ProjectionOutcomeCommitsAppliedAndPendingAtomically", "AlertProjectionReceiptRealKafka"):
        if token not in test_text:
            errors.append(f"negative or reconciliation test missing: {token}")
    for token in ("DoNotShadowMillisecondFilterColumns", "legacy-fingerprint"):
        if token not in test_text:
            errors.append(f"legacy projection compatibility test missing: {token}")

    return {
        "status": "PASS" if not errors else "FAIL",
        "feature_id": contract.get("feature_id"),
        "remediation_id": contract.get("remediation_id"),
        "coverage_status": contract.get("coverage_status"),
        "production_applied": contract.get("production_applied"),
        "feature_flag_default_enabled": runtime.get("feature_flag_default_enabled"),
        "maximum_documents": scope.get("maximum_documents"),
        "closure_blockers": contract.get("closure_blockers", []),
        "errors": errors,
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
