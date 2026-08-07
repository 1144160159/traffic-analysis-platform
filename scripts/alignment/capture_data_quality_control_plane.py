#!/usr/bin/env python3
"""Capture immutable T-DQ-001 repository and read-only pre-rollout evidence."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib.parse import urlencode

from candidate_snapshot import build_snapshot


ROOT = Path(__file__).resolve().parents[2]
DEFAULT_OUTPUT_ROOT = ROOT / "doc/02_acceptance/runs"
EXPECTED_TABLES = (
    "data_quality_datasets",
    "data_quality_rules",
    "data_quality_baselines",
    "data_quality_watermarks",
    "data_quality_events",
    "data_quality_repairs",
    "data_quality_outbox",
    "data_quality_dataset_history",
    "data_quality_rule_history",
    "data_quality_command_requests",
    "data_quality_rule_evaluations",
    "data_quality_repair_history",
    "data_quality_repair_requests",
    "data_quality_flow_replay_projection",
    "data_quality_replay_projection_receipts",
)
EXPECTED_MIGRATIONS = ("202608041400", "202608041500", "202608041600", "202608041700", "202608041800")
COMMANDS = (
    ("data-quality-contract", ["python3", "scripts/alignment/verify_data_quality_control_plane.py"]),
    (
        "data-quality-negative-tests",
        ["python3", "-m", "unittest", "tests.alignment.test_data_quality_control_plane", "-v"],
    ),
    (
        "data-quality-go-tests",
        [
            "bash",
            "-lc",
            "cd go/control-plane && go test ./internal/common/kafka ./internal/common/dataquality ./internal/alert/config ./internal/alert/api ./cmd/alert-service -count=1",
        ],
    ),
    ("migration-contract", ["python3", "scripts/alignment/check_migrations.py"]),
    ("openapi-contract", ["python3", "scripts/alignment/check_openapi.py"]),
    ("strict-registry", ["python3", "scripts/alignment/validate.py", "--strict-w1"]),
    (
        "postgres-init-client-dry-run",
        [
            "kubectl",
            "apply",
            "--dry-run=client",
            "--validate=false",
            "-f",
            "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
            "-o",
            "name",
        ],
    ),
)
SOURCE_ARTIFACTS = (
    "contracts/alignment/features/F-DATAQUALITY-001.json",
    "contracts/data-quality/control-plane.v1.json",
    "contracts/data-quality/dataset-signals.v1.json",
    "contracts/openapi/alignment-v1.openapi.json",
    "deployments/postgres/migrations/202608041400_data_quality_control_plane_v1.sql",
    "deployments/postgres/migrations/202608041500_data_quality_governance_v1.sql",
    "deployments/postgres/migrations/202608041600_data_quality_rule_evaluation_v1.sql",
    "deployments/postgres/migrations/202608041700_data_quality_repair_lifecycle_v1.sql",
    "deployments/postgres/migrations/202608041800_data_quality_replay_projection_v1.sql",
    "deployments/kubernetes/init-jobs/02-postgres-schema.yaml",
    "go/control-plane/internal/common/dataquality/monitor.go",
    "go/control-plane/internal/common/dataquality/monitor_test.go",
    "go/control-plane/internal/common/dataquality/monitor_persistence_test.go",
    "go/control-plane/internal/common/dataquality/handoff_signals.go",
    "go/control-plane/internal/common/dataquality/handoff_repository.go",
    "go/control-plane/internal/common/dataquality/handoff_signals_test.go",
    "go/control-plane/internal/common/dataquality/governance.go",
    "go/control-plane/internal/common/dataquality/governance_integration_test.go",
    "go/control-plane/internal/common/dataquality/evaluation.go",
    "go/control-plane/internal/common/dataquality/evaluation_test.go",
    "go/control-plane/internal/common/dataquality/repair.go",
    "go/control-plane/internal/common/dataquality/repair_test.go",
    "go/control-plane/internal/common/dataquality/repair_evidence.go",
    "go/control-plane/internal/common/dataquality/repair_evidence_test.go",
    "go/control-plane/internal/common/dataquality/repair_executor.go",
    "go/control-plane/internal/common/dataquality/repair_replay_driver.go",
    "go/control-plane/internal/common/dataquality/repair_replay_driver_test.go",
    "go/control-plane/internal/common/dataquality/repair_projection_consumer.go",
    "go/control-plane/internal/common/dataquality/repair_projection_consumer_test.go",
    "go/control-plane/internal/common/kafka/security.go",
    "go/control-plane/internal/alert/api/handler_advanced.go",
    "go/control-plane/internal/alert/api/handler_data_quality_governance.go",
    "go/control-plane/internal/alert/api/handler_data_quality_governance_integration_test.go",
    "go/control-plane/internal/alert/config/config.go",
    "go/control-plane/internal/alert/config/config_test.go",
    "go/control-plane/cmd/alert-service/main.go",
    "scripts/alignment/verify_data_quality_control_plane.py",
    "scripts/alignment/capture_data_quality_control_plane.py",
    "scripts/alignment/render_data_quality_postgres_expand.py",
    "tests/alignment/test_data_quality_control_plane.py",
    "tests/alignment/test_data_quality_expand_renderer.py",
    "deployments/postgres/migrations/README.md",
    "web/ui/src/generated/alignmentClient.ts",
    "doc/07_alignment/runbooks/T-DQ-001-persistent-quality-control-plane.md",
    "Makefile",
)

SECRET_ASSIGNMENT = re.compile(
    rb"(?:KAFKA_CLIENT_PASSWORD|KAFKA_TLS_TRUSTSTORE_PASSWORD|CLICKHOUSE_PASSWORD)\s*=\s*(?![\"']?\$\{)[^\r\n]+"
)
PRIVATE_KEY_MARKER = b"-----BEGIN PRIVATE KEY-----"
FLINK_WATERMARK_NAME = "currentOutputWatermark"
FLINK_WATERMARK_METRIC = re.compile(
    rf"^\d+\.Assign_FlowEvent_Watermarks\.{re.escape(FLINK_WATERMARK_NAME)}$"
)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def artifact(path: Path) -> dict[str, Any]:
    return {"path": path.name, "sha256": sha256(path), "size_bytes": path.stat().st_size}


def evidence_path(path: Path) -> str:
    """Keep repository evidence paths relative and external output paths explicit."""
    resolved = path.resolve()
    try:
        return resolved.relative_to(ROOT.resolve()).as_posix()
    except ValueError:
        return resolved.as_posix()


def scan_evidence_secrets(output: Path) -> list[str]:
    hits: list[str] = []
    for path in sorted(item for item in output.iterdir() if item.is_file()):
        payload = path.read_bytes()
        if SECRET_ASSIGNMENT.search(payload) or PRIVATE_KEY_MARKER in payload:
            hits.append(path.name)
    return hits


def write_json(path: Path, payload: Any) -> dict[str, Any]:
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return artifact(path)


def run_command(name: str, command: list[str], output: Path) -> dict[str, Any]:
    log_path = output / f"{name}.log"
    started = datetime.now(timezone.utc)
    print(f"[data-quality] starting {name}: {' '.join(command)}", flush=True)
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
    print(f"[data-quality] {name}: {result['status']}", flush=True)
    return result


def kubectl(args: list[str]) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        ["kubectl", "--request-timeout=25s", *args],
        cwd=ROOT,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def save_result(output: Path, name: str, result: subprocess.CompletedProcess[bytes]) -> list[dict[str, Any]]:
    stdout = output / f"live-{name}.stdout"
    stderr = output / f"live-{name}.stderr.log"
    stdout.write_bytes(result.stdout)
    stderr.write_bytes(result.stderr)
    return [artifact(stdout), artifact(stderr)]


def save_stderr(output: Path, name: str, result: subprocess.CompletedProcess[bytes]) -> dict[str, Any]:
    stderr = output / f"live-{name}.stderr.log"
    stderr.write_bytes(result.stderr)
    return artifact(stderr)


def parse_json(result: subprocess.CompletedProcess[bytes], scope: str, errors: list[dict[str, Any]]) -> Any:
    if result.returncode:
        errors.append({"scope": scope, "exit_code": result.returncode})
        return None
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        errors.append({"scope": scope, "error": f"JSON decode failed: {exc}"})
        return None


def finite_flink_watermarks(payload: Any) -> tuple[list[dict[str, Any]], int | None]:
    finite_values: list[dict[str, Any]] = []
    for item in payload if isinstance(payload, list) else []:
        try:
            value = int(str(item.get("value", "")))
        except (AttributeError, ValueError):
            continue
        if value == -(1 << 63):
            continue
        finite_values.append({"metric_id": str(item.get("id", "")), "value": value})
    if not finite_values:
        return [], None
    return finite_values, min(item["value"] for item in finite_values)


def select_flink_watermark_metric_ids(payload: Any) -> list[str]:
    """Select only the assigned-watermark operator metrics for every subtask."""
    return sorted(
        metric_id
        for item in (payload if isinstance(payload, list) else [])
        if isinstance(item, dict)
        if FLINK_WATERMARK_METRIC.fullmatch(metric_id := str(item.get("id", "")))
    )


def postgres_schema_query() -> str:
    values = ",".join(f"('{table}')" for table in EXPECTED_TABLES)
    migration_values = ",".join(f"('{version}')" for version in EXPECTED_MIGRATIONS)
    return f"""
WITH expected(table_name) AS (VALUES {values}),
expected_migrations(version) AS (VALUES {migration_values})
SELECT json_build_object(
  'migration_records',(SELECT json_object_agg(e.version,(
      SELECT count(*) FROM alignment_schema_migrations m WHERE m.version=e.version
    )) FROM expected_migrations e),
  'tables',(SELECT COALESCE(json_agg(e.table_name ORDER BY e.table_name),'[]'::json)
            FROM expected e JOIN pg_tables p ON p.schemaname='public' AND p.tablename=e.table_name),
  'missing_tables',(SELECT COALESCE(json_agg(e.table_name ORDER BY e.table_name),'[]'::json)
                    FROM expected e WHERE NOT EXISTS (
                      SELECT 1 FROM pg_tables p WHERE p.schemaname='public' AND p.tablename=e.table_name
                    ))
)::text
""".strip()


def capture_live(output: Path) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    artifacts: list[dict[str, Any]] = []
    errors: list[dict[str, Any]] = []

    postgres = kubectl(
        [
            "-n",
            "databases",
            "exec",
            "postgres-primary-0",
            "--",
            "psql",
            "-U",
            "postgres",
            "-d",
            "traffic_platform",
            "-Atqc",
            postgres_schema_query(),
        ]
    )
    artifacts.extend(save_result(output, "postgres-data-quality-schema", postgres))
    postgres_state = parse_json(postgres, "postgres-schema", errors) or {
        "migration_records": {version: 0 for version in EXPECTED_MIGRATIONS},
        "tables": [],
        "missing_tables": list(EXPECTED_TABLES),
    }

    table_counts: dict[str, int] | None = None
    if not postgres_state.get("missing_tables"):
        count_fields = ",".join(
            f"'{table}',(SELECT count(*) FROM {table})" for table in EXPECTED_TABLES
        )
        counts = kubectl(
            [
                "-n",
                "databases",
                "exec",
                "postgres-primary-0",
                "--",
                "psql",
                "-U",
                "postgres",
                "-d",
                "traffic_platform",
                "-Atqc",
                f"SELECT json_build_object({count_fields})::text",
            ]
        )
        artifacts.extend(save_result(output, "postgres-data-quality-counts", counts))
        table_counts = parse_json(counts, "postgres-counts", errors)

    configmap = kubectl(["-n", "databases", "get", "configmap", "postgres-init-sql", "-o", "json"])
    artifacts.append(save_stderr(output, "postgres-init-configmap", configmap))
    configmap_summary: dict[str, Any] = {"found": False, "all_migration_keys_present": False}
    configmap_payload = parse_json(configmap, "postgres-init-configmap", errors)
    if isinstance(configmap_payload, dict):
        data = configmap_payload.get("data") or {}
        keys = (
            "22-data-quality-control-plane-v1.sql",
            "23-data-quality-governance-v1.sql",
            "24-data-quality-rule-evaluation-v1.sql",
            "25-data-quality-repair-lifecycle-v1.sql",
            "26-data-quality-replay-projection-v1.sql",
        )
        migrations = {
            key: {
                "present": isinstance(data.get(key), str),
                "size_bytes": len(data[key].encode("utf-8")) if isinstance(data.get(key), str) else 0,
                "sha256": hashlib.sha256(data[key].encode("utf-8")).hexdigest()
                if isinstance(data.get(key), str)
                else None,
            }
            for key in keys
        }
        configmap_summary = {
            "found": True,
            "resource_version": configmap_payload.get("metadata", {}).get("resourceVersion"),
            "migration_keys": migrations,
            "all_migration_keys_present": all(item["present"] for item in migrations.values()),
        }
    artifacts.append(write_json(output / "live-postgres-init-configmap-summary.json", configmap_summary))

    deployment = kubectl(["-n", "traffic-analysis", "get", "deployment", "alert-service", "-o", "json"])
    artifacts.append(save_stderr(output, "alert-service-deployment", deployment))
    deployment_summary: dict[str, Any] = {"found": False}
    deployment_payload = parse_json(deployment, "alert-service-deployment", errors)
    if isinstance(deployment_payload, dict):
        spec = deployment_payload.get("spec", {})
        status = deployment_payload.get("status", {})
        containers = spec.get("template", {}).get("spec", {}).get("containers", [])
        container = containers[0] if containers else {}
        dq_env = []
        for item in container.get("env", []):
            name = str(item.get("name", ""))
            if name.startswith("DATA_QUALITY"):
                dq_env.append({"name": name, "value": item.get("value"), "uses_value_from": "valueFrom" in item})
        deployment_summary = {
            "found": True,
            "image": container.get("image"),
            "generation": deployment_payload.get("metadata", {}).get("generation"),
            "observed_generation": status.get("observedGeneration"),
            "replicas": status.get("replicas", 0),
            "ready_replicas": status.get("readyReplicas", 0),
            "available_replicas": status.get("availableReplicas", 0),
            "data_quality_environment": dq_env,
        }
    artifacts.append(write_json(output / "live-alert-service-deployment-summary.json", deployment_summary))

    pods = kubectl(["-n", "middleware", "get", "pods", "-o", "json"])
    artifacts.append(save_stderr(output, "middleware-pods", pods))
    pods_payload = parse_json(pods, "middleware-pods", errors)
    running_pods: list[str] = []
    if isinstance(pods_payload, dict):
        running_pods = [
            str(item.get("metadata", {}).get("name", ""))
            for item in pods_payload.get("items", [])
            if item.get("status", {}).get("phase") == "Running"
        ]
    kafka_pod = next((name for name in running_pods if name.startswith("kafka-") and name[6:].isdigit()), "")
    clickhouse_pod = next((name for name in running_pods if name.startswith("clickhouse-1-")), "")

    kafka_signal: dict[str, Any] = {
        "status": "error",
        "source_id": "flow.events.v1/flink-session-job",
        "measurement_error": "running Kafka broker pod not found",
    }
    if kafka_pod:
        kafka_command = r'''
/opt/kafka/bin/kafka-consumer-groups.sh \
  --bootstrap-server kafka-bootstrap.middleware.svc:9092 \
  --group flink-session-job --describe \
  --command-config <(printf "%s\n" \
    "security.protocol=SASL_SSL" \
    "sasl.mechanism=SCRAM-SHA-512" \
    "sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username=\"${KAFKA_CLIENT_USERNAME}\" password=\"${KAFKA_CLIENT_PASSWORD}\";" \
    "ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12" \
    "ssl.truststore.password=${KAFKA_TLS_TRUSTSTORE_PASSWORD}" \
    "ssl.truststore.type=PKCS12") 2>/dev/null | \
awk '$1=="flink-session-job" && $2=="flow.events.v1" {lag+=$6; partitions++; active+=($7!="-")} END {
  if (partitions>0) printf "{\"status\":\"measured\",\"source_id\":\"flow.events.v1/flink-session-job\",\"partition_count\":%d,\"watermark_value\":\"%d\",\"active_consumers\":%d}\n",partitions,lag,active;
  else exit 3
}'
'''.strip()
        kafka = kubectl(["-n", "middleware", "exec", kafka_pod, "--", "bash", "-lc", kafka_command])
        artifacts.extend(save_result(output, "kafka-flow-session-lag", kafka))
        parsed_kafka = parse_json(kafka, "kafka-flow-session-lag", errors)
        if isinstance(parsed_kafka, dict):
            kafka_signal = parsed_kafka

    flink_signal: dict[str, Any] = {
        "status": "error",
        "source_id": "Session Aggregation Job V2/Assign FlowEvent Watermarks",
        "measurement_error": "Flink JobManager REST unavailable",
    }
    flink = kubectl(["-n", "flink", "exec", "flink-jobmanager-0", "--", "curl", "-fsS", "http://127.0.0.1:8081/jobs/overview"])
    artifacts.extend(save_result(output, "flink-jobs-overview", flink))
    flink_payload = parse_json(flink, "flink-jobs-overview", errors)
    if isinstance(flink_payload, dict):
        running_jobs = sorted(
            str(item.get("name", ""))
            for item in flink_payload.get("jobs", [])
            if item.get("state") == "RUNNING"
        )
        expected_jobs = [
            item
            for item in flink_payload.get("jobs", [])
            if item.get("state") == "RUNNING" and item.get("name") == "Session Aggregation Job V2"
        ]
        flink_signal = {
            "status": "unknown" if len(expected_jobs) == 1 else "error",
            "source_id": "Session Aggregation Job V2/Assign FlowEvent Watermarks",
            "expected_job_running": len(expected_jobs) == 1,
            "running_job_count": len(running_jobs),
            "running_jobs": running_jobs,
            "measurement_error": (
                None
                if len(expected_jobs) == 1
                else f"expected exactly one RUNNING Session Aggregation Job V2, found {len(expected_jobs)}"
            ),
            "watermark_value": None,
        }
        if len(expected_jobs) == 1:
            job_id = str(expected_jobs[0].get("jid", ""))
            detail = kubectl(
                [
                    "-n",
                    "flink",
                    "exec",
                    "flink-jobmanager-0",
                    "--",
                    "curl",
                    "-fsS",
                    f"http://127.0.0.1:8081/jobs/{job_id}",
                ]
            )
            artifacts.extend(save_result(output, "flink-session-job-detail", detail))
            detail_payload = parse_json(detail, "flink-session-job-detail", errors)
            vertices = []
            if isinstance(detail_payload, dict):
                vertices = [
                    item
                    for item in detail_payload.get("vertices", [])
                    if item.get("status") == "RUNNING"
                    and "Assign FlowEvent Watermarks" in str(item.get("name", ""))
                ]
            if len(vertices) != 1:
                flink_signal.update(
                    status="error",
                    measurement_error=(
                        "expected exactly one RUNNING vertex containing "
                        f"Assign FlowEvent Watermarks, found {len(vertices)}"
                    ),
                )
            else:
                vertex_id = str(vertices[0].get("id", ""))
                metrics_url = f"http://127.0.0.1:8081/jobs/{job_id}/vertices/{vertex_id}/metrics"
                catalog = kubectl(
                    ["-n", "flink", "exec", "flink-jobmanager-0", "--", "curl", "-fsS", metrics_url]
                )
                artifacts.extend(save_result(output, "flink-session-watermark-metric-catalog", catalog))
                catalog_payload = parse_json(catalog, "flink-session-watermark-metric-catalog", errors)
                metric_ids = select_flink_watermark_metric_ids(catalog_payload)
                if not metric_ids:
                    flink_signal.update(
                        status="error",
                        measurement_error="currentOutputWatermark metric not found on the expected vertex",
                    )
                else:
                    values = kubectl(
                        [
                            "-n",
                            "flink",
                            "exec",
                            "flink-jobmanager-0",
                            "--",
                            "curl",
                            "-fsS",
                            f"{metrics_url}?{urlencode({'get': ','.join(metric_ids)})}",
                        ]
                    )
                    artifacts.extend(save_result(output, "flink-session-watermark-values", values))
                    values_payload = parse_json(values, "flink-session-watermark-values", errors)
                    finite_values, watermark = finite_flink_watermarks(values_payload)
                    if watermark is None:
                        flink_signal.update(
                            status="error",
                            measurement_error="currentOutputWatermark has no finite subtask values",
                        )
                    else:
                        flink_signal = {
                            "status": "measured",
                            "source_id": "Session Aggregation Job V2/Assign FlowEvent Watermarks",
                            "expected_job_running": True,
                            "job_id": job_id,
                            "vertex_id": vertex_id,
                            "watermark_value": str(watermark),
                            "subtask_metrics": finite_values,
                            "measurement_error": None,
                        }

    sink_signal: dict[str, Any] = {
        "status": "error",
        "source_id": "clickhouse.traffic.flows_raw.max_ingest_ts",
        "measurement_error": "running ClickHouse pod not found",
        "tenants": [],
    }
    if clickhouse_pod:
        clickhouse_command = (
            'clickhouse-client --user default --password "$CLICKHOUSE_PASSWORD" '
            '--query "SELECT tenant_id,count() AS rows,max(ingest_ts) AS sink_commit_ms '
            'FROM traffic.flows_raw GROUP BY tenant_id ORDER BY tenant_id FORMAT JSONEachRow"'
        )
        clickhouse = kubectl(["-n", "middleware", "exec", clickhouse_pod, "-c", "clickhouse", "--", "sh", "-lc", clickhouse_command])
        artifacts.extend(save_result(output, "clickhouse-flows-raw-sink-commit", clickhouse))
        tenant_commits: list[dict[str, Any]] = []
        if clickhouse.returncode == 0:
            try:
                tenant_commits = [json.loads(line) for line in clickhouse.stdout.decode().splitlines() if line.strip()]
            except json.JSONDecodeError as exc:
                errors.append({"scope": "clickhouse-flows-raw-sink-commit", "error": f"JSON decode failed: {exc}"})
        else:
            errors.append({"scope": "clickhouse-flows-raw-sink-commit", "exit_code": clickhouse.returncode})
        if tenant_commits:
            sink_signal = {
                "status": "measured",
                "source_id": "clickhouse.traffic.flows_raw.max_ingest_ts",
                "tenants": tenant_commits,
                "measurement_error": None,
            }
        elif clickhouse.returncode == 0:
            sink_signal = {
                "status": "unknown",
                "source_id": "clickhouse.traffic.flows_raw.max_ingest_ts",
                "tenants": [],
                "measurement_error": None,
            }

    live_schema_present = (
        all(
            postgres_state.get("migration_records", {}).get(version) == 1
            for version in EXPECTED_MIGRATIONS
        )
        and set(postgres_state.get("tables", [])) == set(EXPECTED_TABLES)
        and not postgres_state.get("missing_tables")
    )
    live = {
        "read_only": True,
        "postgres": postgres_state,
        "table_counts": table_counts,
        "postgres_init_configmap": configmap_summary,
        "alert_service": deployment_summary,
        "versioned_schema_present": live_schema_present,
        "candidate_source_bound_to_deployed_image": False,
        "candidate_applied": False,
        "real_handoff_signals": {
            "kafka_offset": kafka_signal,
            "flink_watermark": flink_signal,
            "sink_commit": sink_signal,
            "business_version": {
                "status": "not_applicable",
                "source_id": "flows_raw.immutable_event",
                "reason": "immutable flow facts do not have an aggregate revision",
            },
            "object_manifest": {
                "status": "not_applicable",
                "source_id": "flows_raw.no_object_payload",
                "reason": "flow facts are not object-backed artifacts",
            },
        },
        "repair_replay_reconcile_manifest": None,
        "production_mutations": [],
        "errors": errors,
    }
    return live, artifacts


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
    before = build_snapshot()
    g0_hash = (g0.get("candidate_source") or {}).get("content_sha256")
    if not g0_hash or before["content_sha256"] != g0_hash:
        raise SystemExit("current candidate does not match referenced G0 manifest")

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
    repository_pass = len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results)
    live, live_artifacts = capture_live(output) if repository_pass else (
        {"errors": [{"error": "skipped after repository failure"}]},
        [],
    )
    secret_hits = scan_evidence_secrets(output)
    if secret_hits:
        live.setdefault("errors", []).append(
            {"scope": "secret-scan", "artifact_count": len(secret_hits), "artifacts": secret_hits}
        )
    after = build_snapshot()
    stable = before["content_sha256"] == after["content_sha256"]
    scoped = "PASS" if repository_pass and stable and not live.get("errors") else "FAIL"

    sources: list[dict[str, Any]] = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    contract = json.loads((ROOT / "contracts/data-quality/control-plane.v1.json").read_text(encoding="utf-8"))
    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-DQ-001",
        "related_ids": ["F-DATAQUALITY-001", "T-OBS-001", "T-KAFKA-004", "T-MINIO-001"],
        "status": "PARTIAL" if scoped == "PASS" else "FAIL",
        "coverage_status": "PARTIAL_PERSISTENT_BASELINE_GOVERNANCE_AND_FLOWS_RAW_REAL_SIGNAL_COLLECTORS_PRE_ROLLOUT",
        "scoped_evidence_status": scoped,
        "candidate_source": before,
        "candidate_source_stable": stable,
        "production_applied": False,
        "read_only_live_capture": True,
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": evidence_path(g0_path),
            "manifest_sha256": sha256(g0_path),
            "candidate_source_sha256": g0_hash,
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS_FOR_BASELINE_GOVERNANCE_SCHEMA_GUARD_ATOMIC_HISTORY_OUTBOX_AUDIT_RECEIPTS_AND_FIVE_SIGNAL_PERSISTENCE",
            "G2": "PARTIAL_READ_ONLY_KAFKA_OFFSETS_FLINK_ABSENCE_CLICKHOUSE_SINK_AND_EXPLICIT_NOT_APPLICABLE_SIGNALS_OPEN_FOR_CANDIDATE",
            "G3": "OPEN_FOR_REPAIR_DRY_RUN_APPROVAL_REPLAY_AND_CROSS_STORE_RECONCILIATION",
            "G4": "OPEN_FOR_FIXED_SCALE_QUALITY_SLO_AND_RESOURCE_BUDGETS",
            "G5": "OPEN_FOR_WINDOWS_CHROME_MOCK_OFF_UNKNOWN_PARTIAL_AND_REPAIR_LIFECYCLE",
            "G6": "HOLD_FOR_EXPAND_SHADOW_CANARY_ROLLBACK_AND_T_PLUS_OBSERVATION",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "commands": results,
        "live_observation": live,
        "source_artifacts": sources,
        "live_artifacts": live_artifacts,
        "closure_blockers": contract["closure_blockers"],
        "secret_scan": {"status": "PASS" if not secret_hits else "FAIL", "artifact_hits": secret_hits},
        "secrets_captured": bool(secret_hits),
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(
        json.dumps(
            {
                "status": manifest["status"],
                "scoped_evidence_status": scoped,
                "manifest": evidence_path(manifest_path),
                "manifest_sha256": sha256(manifest_path),
                "candidate_source_sha256": before["content_sha256"],
                "live_observation": live,
            },
            ensure_ascii=False,
            indent=2,
        ),
        flush=True,
    )
    return 0 if scoped == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
