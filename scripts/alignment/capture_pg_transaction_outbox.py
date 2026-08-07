#!/usr/bin/env python3
"""Capture immutable evidence for implemented T-PG-002 transaction slices."""

from __future__ import annotations

import argparse
import hashlib
import json
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
COMMANDS = (
    ("pg-transaction-contract", ["python3", "scripts/alignment/verify_pg_transaction_outbox.py"]),
    ("pg-transaction-tests", ["python3", "-m", "unittest", "tests.alignment.test_pg_transaction_outbox", "-v"]),
    ("transaction-go-tests", ["go", "-C", "go/control-plane", "test", "./internal/alert/api", "./internal/alert/config", "./internal/alert/whitelist", "./cmd/alert-service", "./cmd/threat-intel-service", "./internal/alert/threatintel", "./internal/alert/consumer", "./internal/rules/config", "./internal/rules/consumer", "./cmd/rule-manager", "./internal/auth/repository", "./internal/auth/service", "./internal/auth/api", "./internal/auth/config", "./cmd/auth-service", "./internal/forensics/...", "-count=1"]),
    ("asset-transaction-go-tests", ["go", "-C", "go/control-plane", "test", "./internal/asset/...", "./cmd/asset-service", "-count=1"]),
    ("asset-transaction-ephemeral-pg", ["python3", "scripts/alignment/verify_asset_atomic_ephemeral.py", "--run-id", "pg-transaction-outbox-asset-v1"]),
    ("transaction-ui-tests", ["npm", "--prefix", "web/ui", "test", "--", "--run", "src/services/notificationGovernanceApi.test.ts", "src/services/pageApiPlans.test.ts", "src/services/whitelistGovernanceApi.test.ts"]),
    ("schema-entrypoints", ["python3", "scripts/alignment/verify_pg_schema_entrypoints_ephemeral.py", "--run-id", "pg-transaction-outbox-whitelist-v1"]),
    ("whitelist-event-pipeline", ["python3", "scripts/alignment/verify_whitelist_event_pipeline_ephemeral.py", "--run-id", "pg-transaction-outbox-whitelist-pipeline-v1"]),
    ("event-catalog", ["python3", "scripts/alignment/check_event_catalog.py"]),
    ("kafka-acl-tests", ["python3", "-m", "unittest", "tests.alignment.test_kafka_acl_catalog", "-v"]),
    ("migration-guard", ["python3", "scripts/alignment/check_migrations.py"]),
)
SOURCE_ARTIFACTS = (
    "contracts/postgres/transaction-outbox.v1.json",
    "contracts/events/kafka-json-events-v1.schema.json",
    "contracts/events/kafka-topic-catalog.v1.json",
    "contracts/events/kafka-acl-catalog.v1.json",
    "common/kafka/create-topics.sh",
    "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
    "deployments/kubernetes/init-jobs/01-kafka-topics.yaml",
    "deployments/postgres/migrations/202608031100_alert_saved_view_transaction_v2.sql",
    "deployments/postgres/migrations/202608031300_notification_rule_transaction_v2.sql",
    "deployments/postgres/migrations/202608031500_user_settings_transaction_v2.sql",
    "deployments/postgres/migrations/202608031530_user_commands_atomic.sql",
    "deployments/postgres/migrations/202608010700_campaign_aggregate_v2.sql",
    "deployments/postgres/migrations/202608031540_probe_registry_atomic.sql",
    "deployments/postgres/migrations/202608031550_threat_intel_command_atomic.sql",
    "deployments/postgres/migrations/202608031600_forensics_task_command_atomic.sql",
    "deployments/postgres/migrations/202608031610_whitelist_governance_v2.sql",
    "deployments/postgres/migrations/202608071930_whitelist_rule_projection_v1.sql",
    "deployments/postgres/migrations/202608031620_dashboard_task_v2.sql",
    "deployments/postgres/migrations/202608041930_dashboard_task_execution_pipeline_v1.sql",
    "deployments/postgres/migrations/202607310015_asset_atomic_upsert_v2.sql",
    "deployments/postgres/migrations/202607310030_asset_projection_inbox.sql",
    "deployments/postgres/migrations/202608031510_asset_legacy_mutation_atomic.sql",
    "deployments/postgres/migrations/202608031520_asset_discovery_resource_atomic.sql",
    "go/control-plane/internal/alert/api/handler_alert_actions.go",
    "go/control-plane/internal/alert/api/handler_alert_saved_view_outbox.go",
    "go/control-plane/internal/alert/api/handler_alert_saved_view_outbox_test.go",
    "go/control-plane/internal/alert/api/notification_governance_transaction.go",
    "go/control-plane/internal/alert/api/notification_governance_transaction_test.go",
    "go/control-plane/internal/alert/api/notification_governance_outbox.go",
    "go/control-plane/internal/alert/api/notification_governance_outbox_test.go",
    "go/control-plane/internal/alert/api/notification_template_transaction.go",
    "go/control-plane/internal/alert/api/notification_template_transaction_test.go",
    "go/control-plane/internal/alert/api/notification_escalation_transaction.go",
    "go/control-plane/internal/alert/api/notification_escalation_transaction_test.go",
    "go/control-plane/internal/alert/api/notification_silence_transaction.go",
    "go/control-plane/internal/alert/api/notification_silence_transaction_test.go",
    "go/control-plane/internal/alert/api/notification_settings_transaction.go",
    "go/control-plane/internal/alert/api/notification_settings_transaction_test.go",
    "go/control-plane/internal/alert/api/handler_system.go",
    "go/control-plane/internal/alert/api/handler_campaign_actions_test.go",
    "go/control-plane/internal/alert/api/campaign_action_jobs.go",
    "go/control-plane/internal/alert/api/campaign_action_jobs_test.go",
    "go/control-plane/internal/alert/api/campaign_workbench_state.go",
    "go/control-plane/internal/alert/api/campaign_workbench_state_test.go",
    "go/control-plane/internal/alert/api/campaign_aggregate_v2.go",
    "go/control-plane/internal/alert/api/campaign_aggregate_v2_test.go",
    "go/control-plane/internal/alert/api/campaign_aggregate_v2_integration_test.go",
    "go/control-plane/internal/ingest/server/probe_registry.go",
    "go/control-plane/internal/ingest/server/probe_registry_test.go",
    "go/control-plane/internal/ingest/server/probe_registry_integration_test.go",
    "go/control-plane/cmd/threat-intel-service/main.go",
    "go/control-plane/cmd/threat-intel-service/command_atomic.go",
    "go/control-plane/cmd/threat-intel-service/transaction_test.go",
    "go/control-plane/cmd/threat-intel-service/command_atomic_integration_test.go",
    "go/control-plane/internal/forensics/repository/task_repository.go",
    "go/control-plane/internal/forensics/repository/task_command_atomic.go",
    "go/control-plane/internal/forensics/repository/task_command_atomic_integration_test.go",
    "go/control-plane/internal/forensics/task/async_cutter.go",
    "go/control-plane/internal/forensics/api/handler.go",
    "common/sql/pg/09-forensics-task-atomic.sql",
    "common/sql/pg/10-whitelist-governance-v2.sql",
    "common/sql/pg/11-dashboard-task-v2.sql",
    "common/sql/pg/12-dashboard-task-execution-pipeline-v1.sql",
    "go/control-plane/internal/alert/whitelist/whitelist.go",
    "go/control-plane/internal/alert/whitelist/command_atomic.go",
    "go/control-plane/internal/alert/whitelist/command_atomic_integration_test.go",
    "go/control-plane/internal/alert/whitelist/outbox_dispatcher.go",
    "go/control-plane/internal/alert/whitelist/outbox_dispatcher_test.go",
    "go/control-plane/internal/alert/whitelist/handler.go",
    "go/control-plane/internal/alert/consumer/whitelist_projection_test.go",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/internal/alert/config/config_test.go",
    "go/control-plane/internal/rules/config/config.go",
    "go/control-plane/internal/rules/config/config_test.go",
    "go/control-plane/internal/rules/consumer/whitelist_rule_effect_consumer.go",
    "go/control-plane/internal/rules/consumer/whitelist_rule_effect_consumer_test.go",
    "go/control-plane/cmd/rule-manager/main.go",
    "go/control-plane/deployments/kubernetes/alert-service.yaml",
    "go/control-plane/deployments/kubernetes/rule-manager.yaml",
    "go/control-plane/internal/alert/api/dashboard_task_v2.go",
    "go/control-plane/internal/alert/api/dashboard_task_v2_integration_test.go",
    "go/control-plane/internal/alert/api/dashboard_task_pipeline.go",
    "go/control-plane/internal/alert/api/dashboard_task_http_provider.go",
    "go/control-plane/internal/alert/api/dashboard_task_http_provider_test.go",
    "go/control-plane/internal/alert/api/handler_feedback_transaction.go",
    "go/control-plane/internal/alert/threatintel/threat_intel.go",
    "go/control-plane/internal/alert/consumer/threat_intel_event_consumer.go",
    "go/control-plane/cmd/alert-service/main.go",
    "go/control-plane/internal/auth/api/handler.go",
    "go/control-plane/internal/auth/config/config.go",
    "go/control-plane/internal/auth/model/user.go",
    "go/control-plane/internal/auth/repository/user_repository.go",
    "go/control-plane/internal/auth/repository/user_command_atomic.go",
    "go/control-plane/internal/auth/repository/user_command_atomic_integration_test.go",
    "go/control-plane/internal/auth/repository/user_command_outbox.go",
    "go/control-plane/internal/auth/repository/user_settings_repository.go",
    "go/control-plane/internal/auth/repository/user_settings_transaction.go",
    "go/control-plane/internal/auth/repository/user_settings_transaction_test.go",
    "go/control-plane/internal/auth/repository/user_settings_outbox.go",
    "go/control-plane/internal/auth/repository/user_settings_outbox_test.go",
    "go/control-plane/internal/auth/service/auth_service.go",
    "go/control-plane/internal/auth/service/user_settings_service_test.go",
    "go/control-plane/cmd/auth-service/main.go",
    "contracts/alignment/features/F-AUTH-001.json",
    "contracts/alignment/features/F-CAMPAIGN-001.json",
    "contracts/alignment/features/F-PROBE-001.json",
    "contracts/alignment/features/F-WHITELIST-001.json",
    "contracts/alignment/features/F-DASHBOARD-002.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "common/sql/pg/00-init.sql",
    "common/sql/pg/04-tasks-audit.sql",
    "go/control-plane/deployments/docker/init/postgres_merged.sql",
    "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
    "deployments/kubernetes/applications/go-services.yaml",
    "go/control-plane/internal/asset/repository/atomic_upsert.go",
    "go/control-plane/internal/asset/repository/atomic_upsert_test.go",
    "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go",
    "go/control-plane/internal/asset/repository/inactive_atomic.go",
    "go/control-plane/internal/asset/repository/inactive_atomic_test.go",
    "go/control-plane/internal/asset/repository/outbox_dispatcher.go",
    "go/control-plane/internal/asset/repository/outbox_dispatcher_test.go",
    "go/control-plane/internal/asset/repository/discovery_jobs.go",
    "go/control-plane/internal/asset/repository/discovery_legacy_atomic.go",
    "go/control-plane/internal/asset/repository/discovery_resource_atomic.go",
    "go/control-plane/internal/asset/repository/discovery_resource_atomic_integration_test.go",
    "go/control-plane/internal/asset/repository/discovery_outbox_dispatcher.go",
    "go/control-plane/internal/asset/repository/discovery_outbox_dispatcher_test.go",
    "web/ui/src/services/alertTriageApi.ts",
    "web/ui/src/services/notificationGovernanceApi.ts",
    "web/ui/src/services/notificationGovernanceApi.test.ts",
    "web/ui/src/services/pageApiPlans.ts",
    "web/ui/src/services/pageApiPlans.test.ts",
    "web/ui/src/services/whitelistGovernanceApi.ts",
    "web/ui/src/services/whitelistGovernanceApi.test.ts",
    "web/ui/src/pages/WhitelistGovernancePage.tsx",
    "web/ui/src/services/dashboardTaskApi.ts",
    "web/ui/src/services/dashboardTaskApi.test.ts",
    "web/ui/src/pages/DashboardOperationsPage.tsx",
    "scripts/alignment/verify_pg_transaction_outbox.py",
    "scripts/alignment/verify_asset_atomic_ephemeral.py",
    "scripts/alignment/verify_pg_schema_entrypoints_ephemeral.py",
    "scripts/alignment/verify_whitelist_event_pipeline_ephemeral.py",
    "scripts/alignment/verify_playbook_schema_entrypoints.py",
    "tests/alignment/test_pg_transaction_outbox.py",
    "tests/alignment/test_whitelist_event_pipeline.py",
    "doc/07_alignment/runbooks/T-PG-002-saved-view-transaction-outbox.md",
    "doc/07_alignment/runbooks/T-PG-002-notification-rule-transaction-outbox.md",
    "doc/07_alignment/runbooks/T-PG-002-user-settings-transaction-outbox.md",
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[pg-transaction-outbox] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(command, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT, check=False)
    finished = datetime.now(timezone.utc)
    result = {
        "name": name,
        "command": command,
        "exit_code": completed.returncode,
        "status": "PASS" if completed.returncode == 0 else "FAIL",
        "started_at": started.isoformat(),
        "finished_at": finished.isoformat(),
        "duration_seconds": round((finished - started).total_seconds(), 3),
        "artifact": log_path.name,
        "sha256": sha256(log_path),
        "size_bytes": log_path.stat().st_size,
    }
    print(f"[pg-transaction-outbox] {name}: {result['status']}", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    g0_path = args.g0_manifest.resolve()
    if not g0_path.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_path}")
    g0 = json.loads(g0_path.read_text(encoding="utf-8"))
    if g0.get("gate") != "G0" or g0.get("status") != "PASS":
        raise SystemExit("referenced G0 manifest is not PASS")

    output = args.output_root.resolve() / args.run_id
    if output.exists():
        raise SystemExit(f"refusing to overwrite immutable evidence directory: {output}")
    output.mkdir(parents=True)
    results: list[dict[str, Any]] = []
    for name, command in COMMANDS:
        result = run_command(name, list(command), output)
        results.append(result)
        if result["status"] != "PASS":
            break
    scoped_status = "PASS" if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results) else "FAIL"

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    candidate = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    candidate_matches = candidate.get("content_sha256") == g0_hash
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-PG-002",
        "related_ids": ["F-ALERT-006", "F-NOTIFICATION-001", "F-PREF-001", "F-ASSET-002", "F-AUTH-001", "F-CAMPAIGN-001", "F-PROBE-001", "F-FORENSICS-001", "F-WHITELIST-001", "F-DASHBOARD-002", "T-KAFKA-001", "T-SCHEMA-001"],
        "status": "PARTIAL" if scoped_status == "PASS" else "FAIL",
        "coverage_status": "PARTIAL",
        "scoped_evidence_status": scoped_status,
        "candidate_source": candidate,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_path.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_path),
            "status": g0.get("status"),
            "candidate_content_sha256": g0_hash,
            "candidate_matches": candidate_matches,
        },
        "gate_status": {
            "G0": "PASS" if candidate_matches else "STALE_REQUIRES_REFRESH",
            "G1": "PASS_FOR_SAVED_VIEW_NOTIFICATION_GOVERNANCE_USER_SETTINGS_USER_COMMAND_ASSET_CAMPAIGN_PROBE_REGISTRATION_THREAT_INTEL_FORENSICS_TASK_WHITELIST_AND_DASHBOARD_TASK_TRANSACTION_IDEMPOTENCY_PERSISTENCE_WITH_DISPATCHER_GAPS_RETAINED" if scoped_status == "PASS" else "FAIL",
            "G2": "OPEN_FOR_RELEASE_CANDIDATE_POSTGRES_AND_KAFKA",
            "G3": "OPEN_FOR_KAFKA_CONSUMER_AUDIT_AND_REVISION_RECONCILIATION",
            "G4": "OPEN_FOR_LOCK_CONTENTION_OUTBOX_LAG_AND_THROUGHPUT_BUDGETS",
            "G5": "OPEN",
            "G6": "HOLD_FOR_EXPAND_CANARY_ROLLBACK_AND_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "SaveAlertView commits business state, immutable history, minimal audit, outbox and idempotency registry in one serializable transaction",
            "same tenant and idempotency key replays the committed result while a changed payload is rejected",
            "the worker claims with SKIP LOCKED, reclaims expired leases, retries with bounded backoff and transitions poison rows to dead",
            "the outbox row is marked published only after Kafka acknowledgement; ACK-before-mark remains an explicit at-least-once duplicate boundary",
            "the dedicated producer-only topic, JSON schema, literal ACL and Kubernetes creation catalog are synchronized",
            "notification rule template escalation silence and settings commands use revision conflicts and atomically commit rich audit history outbox and request registration",
            "user settings commands reject pseudo-success, enforce optimistic revision and atomically commit state history compatible USER_UPDATE audit UserEvent outbox and request registration",
            "the auth-service user settings worker publishes protobuf UserEvent only before marking the leased outbox row published",
            "authenticated profile password login OIDC and legacy user commands bind tenant state revision history audit outbox and idempotency results in one transaction without credential material",
            "the auth-service user command worker publishes protobuf UserEvent only before marking the leased outbox row published, and OIDC role-set hashing is order independent",
            "asset upsert and inactive sweep atomically commit business revision history audit outbox and durable request replay state",
            "the asset outbox leases with SKIP LOCKED and marks published only after the publisher ACK; poison events transition to dead after bounded attempts",
            "the sentinel-guarded asset integration test exercises create update inactive exact replay outbox dispatch and idempotent projection against owned ephemeral PostgreSQL 16",
            "mutating campaign workbench commands fail closed while aggregate v2 is disabled and cannot fall back to raw business state mutation",
            "aggregate v2 compatibility commands normalize missing legacy metadata, then atomically commit campaign revision history audit outbox durable job and idempotency result in one serializable transaction",
            "compatibility mode is preserved in the request hash, durable job result, history payload, event payload and audit detail for rollout observability",
            "authenticated probe registration atomically commits probe revision history audit outbox and durable replay state while exact retries do not create a revision and cross-tenant rebinds fail closed",
            "probe heartbeat is explicitly retained as a tenant-bound liveness projection instead of creating an audit and outbox record every 30 seconds",
            "threat-intel entry import feed configuration and feed execution atomically commit revisions history audit outbox and durable request replay in one serializable transaction",
            "threat-intel exact retries do not create revisions, changed payloads collide, strict updates require revision and entry/feed tenant mismatches fail closed",
            "the owned PostgreSQL 16 sentinel reconciles threat-intel history requests outbox audit entry and feed revisions and removes the container without touching shared state",
            "forensics task create lease progress complete fail cancel recover and archive commands atomically commit revision history audit outbox and durable request replay state",
            "forensics task exact retries preserve event identity, changed payloads collide, stale revisions fail, tenant-bound cancellation does not reveal another tenant and retention uses soft archive",
            "the owned PostgreSQL 16 sentinel reconciles thirteen forensics task command revisions across four tasks and proves history outbox request and audit counts stay equal",
            "whitelist create update approval disable expiry and archive commands atomically commit revision history audit outbox durable replay state and rule-effect intent while optimistic conflicts and cross-tenant access fail closed",
            "whitelist exact retries preserve entry and event identity, changed payloads collide, approval remains distinct from effective rule ACK, and archive retains the authoritative row",
            "the whitelist outbox dispatcher leases with SKIP LOCKED, validates authoritative envelope fields and marks published only after broker acknowledgement",
            "the rule-manager consumer validates topic key headers and payload, then atomically commits a deterministic rule projection and matching rule-effect ACK",
            "alert ingestion consults the current authoritative whitelist version plus projection before dedup storage evidence and notification side effects, while lookup failures fail open",
            "the owned PostgreSQL 16 sentinel proves create approve dispatch project ACK match disable revoke duplicate and older replay behavior without touching shared state; broker transport is in-process for this gate",
            "the production UI bundle sends canonical action and idempotency metadata and distinguishes committed pending rule ACK from final effective state",
            "dashboard task creation binds the authenticated tenant and atomically commits accepted task history audit outbox and durable idempotency receipt while the UI distinguishes accepted from final completion",
            "the owned PostgreSQL 16 sentinel proves dashboard exact replay conflict viewer denial cross-tenant invisibility and full rollback when audit insertion fails",
            "common Docker merged and Kubernetes schema entrypoints replay twice with equal hashes for the tracked transaction tables in ephemeral PostgreSQL 16",
        ],
        "open": [
            "review and remediate the remaining P1 and P2 mutable PostgreSQL command inventory outside the completed atomic slices; P0 review is empty",
            "require explicit idempotency keys revisions and reasons from strict campaign clients before compatibility metadata normalization can be retired",
            "require explicit idempotency keys revisions and reasons from strict threat-intel clients before compatibility metadata normalization can be retired",
            "require explicit idempotency keys revisions and reasons from strict forensics clients before compatibility metadata normalization can be retired",
            "implement the probe registration outbox dispatcher and prove ACK-before-published ordering against release-candidate Kafka",
            "implement the forensics task outbox dispatcher and idempotent consumer before claiming Kafka or projection completion",
            "operate the default-off whitelist dispatcher and rule-manager consumer against release-candidate Kafka and reconcile PG outbox projection ACK partition and offset watermarks",
            "implement the dashboard task outbox dispatcher and final-effect consumer before claiming completed task execution",
            "apply the additive migrations through 202608071930 and topic/ACL expansion to an approved candidate environment",
            "implement and prove an independently idempotent consumer keyed by event_id and aggregate_version",
            "run live commit/publish/ACK-before-mark/worker-kill fault injection and reconcile PG, Kafka and audit",
            "measure lock contention, outbox lag, retry/dead alerts, rollback and T+0/T+1/T+3/T+7 observation",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": manifest["status"],
        "scoped_evidence_status": scoped_status,
        "g0_status": manifest["gate_status"]["G0"],
        "manifest": str(manifest_path),
        "manifest_sha256": sha256(manifest_path),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if scoped_status == "PASS" and candidate_matches else 1


if __name__ == "__main__":
    raise SystemExit(main())
