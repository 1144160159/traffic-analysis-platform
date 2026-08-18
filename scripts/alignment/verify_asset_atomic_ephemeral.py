#!/usr/bin/env python3
"""Run sentinel-guarded atomic command integrations in owned PostgreSQL."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import subprocess
import time
from pathlib import Path
from typing import Any

from run_exact_go_tests import run_exact_go_tests


ROOT = Path(__file__).resolve().parents[2]
IMAGE = "postgres:16"
ASSET_ORACLE_MARKERS = {
    "TestUpsertAtomicPostgresPreCommitFaultMatrix": {
        "ROLLBACK_AFTER_B09_ZERO_FIVE_TABLE_EFFECT",
        "ROLLBACK_AFTER_B11_ZERO_FIVE_TABLE_EFFECT",
        "ROLLBACK_AFTER_B12_ZERO_FIVE_TABLE_EFFECT",
        "ROLLBACK_AFTER_B13_ZERO_FIVE_TABLE_EFFECT",
        "ROLLBACK_AFTER_B14_ZERO_FIVE_TABLE_EFFECT",
    },
    "TestUpsertAtomicCommitUnknownSameKeyRecovery": {
        "COMMIT_UNKNOWN_SAME_KEY_EXACTLY_ONE",
    },
}


def require_candidate_sources(candidate_manifest: Path, sources: list[Path]) -> dict[str, str]:
    payload = json.loads(candidate_manifest.read_text(encoding="utf-8"))
    declared = payload.get("source_blob_sha256")
    if not isinstance(declared, dict):
        raise ValueError("candidate manifest lacks source_blob_sha256")
    actual: dict[str, str] = {}
    for source in sources:
        relative = source.relative_to(ROOT).as_posix()
        digest = hashlib.sha256(source.read_bytes()).hexdigest()
        if declared.get(relative) != digest:
            raise ValueError(f"candidate manifest does not bind current source: {relative}")
        actual[relative] = digest
    return actual


def collect_asset_oracle_markers(events: list[dict[str, Any]]) -> dict[str, list[str]]:
    observed = {test: [] for test in ASSET_ORACLE_MARKERS}
    prefix = "TOPIC1_ORACLE PASS "
    for event in events:
        test = event.get("Test")
        output = event.get("Output")
        if test not in observed or not isinstance(output, str) or prefix not in output:
            continue
        observed[test].append(output.split(prefix, 1)[1].strip())
    for test, expected in ASSET_ORACLE_MARKERS.items():
        markers = observed[test]
        if set(markers) != expected or len(markers) != len(expected):
            raise ValueError(
                f"asset atomic oracle exact-set mismatch for {test}: "
                f"expected={sorted(expected)} observed={markers}"
            )
    return {test: sorted(markers) for test, markers in observed.items()}


def run(
    command: list[str],
    *,
    input_bytes: bytes | None = None,
    env: dict[str, str] | None = None,
    check: bool = True,
) -> subprocess.CompletedProcess[bytes]:
    return subprocess.run(
        command,
        cwd=ROOT,
        input=input_bytes,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=check,
    )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--suite", choices=("full", "asset-upsert-only"), default="full")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--candidate-manifest", type=Path)
    parser.add_argument("--profile-id")
    parser.add_argument("--environment-id")
    args = parser.parse_args()

    suffix = hashlib.sha256(args.run_id.encode()).hexdigest()[:12]
    container = f"codex-asset-atomic-pg-{suffix}"
    if args.suite == "asset-upsert-only" and (
        args.candidate_manifest is None or not args.profile_id or not args.environment_id
    ):
        raise SystemExit(
            "asset-upsert-only requires --candidate-manifest, --profile-id and --environment-id"
        )
    candidate_manifest = (ROOT / args.candidate_manifest).resolve() if args.candidate_manifest else None
    if candidate_manifest is not None and (
        not candidate_manifest.is_relative_to(ROOT) or not candidate_manifest.is_file()
    ):
        raise SystemExit(f"unsafe or missing candidate manifest: {args.candidate_manifest}")
    bound_sources = [
        ROOT / "go/control-plane/internal/asset/repository/atomic_upsert.go",
        ROOT / "go/control-plane/internal/asset/repository/atomic_upsert_test.go",
        ROOT / "go/control-plane/internal/asset/repository/atomic_upsert_integration_test.go",
    ]
    source_hashes = (
        require_candidate_sources(candidate_manifest, bound_sources)
        if candidate_manifest is not None else {}
    )
    result: dict[str, object] = {
        "schema_version": 1,
        "artifact_kind": "ASSET_ATOMIC_EPHEMERAL_TEST_RESULT",
        "subject_pr_id": "T1-M06-P906-TST-PRE-n004-authority-transaction-fault-matrix",
        "run_id": args.run_id,
        "container": container,
        "image": IMAGE,
        "status": "FAIL",
        "candidate_manifest": ({
            "path": candidate_manifest.relative_to(ROOT).as_posix(),
            "sha256": hashlib.sha256(candidate_manifest.read_bytes()).hexdigest(),
        } if candidate_manifest is not None else None),
        "profile_id": args.profile_id,
        "environment_id": args.environment_id,
        "source_blob_sha256": source_hashes,
        "runner_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "exact_runner_sha256": hashlib.sha256(
            (ROOT / "scripts/alignment/run_exact_go_tests.py").read_bytes()
        ).hexdigest(),
        "schema_files": 0,
        "sentinel_verified": False,
        "shared_environment_touched": False,
        "persistent_volume_attached": False,
        "container_removed": False,
        "secrets_captured": False,
        "oracle_results": {},
        "errors": [],
        "proof_ceiling": "OWNED_EPHEMERAL_POSTGRES_G1_ONLY_NOT_PRODUCTION_ACCEPTANCE",
    }
    created = False
    try:
        if run(["docker", "container", "inspect", container], check=False).returncode == 0:
            raise RuntimeError(f"refusing to reuse existing container: {container}")
        run(["docker", "image", "inspect", IMAGE])
        run([
            "docker", "run", "--name", container,
            "-e", "POSTGRES_PASSWORD=codex-ephemeral-only",
            "-e", "POSTGRES_DB=traffic_platform",
            "-p", "127.0.0.1::5432", "-d", IMAGE,
        ])
        created = True

        for _ in range(30):
            ready = run(
                ["docker", "exec", container, "pg_isready", "-U", "postgres", "-d", "traffic_platform"],
                check=False,
            )
            if ready.returncode == 0:
                break
            time.sleep(1)
        else:
            raise RuntimeError("ephemeral PostgreSQL did not become ready")

        schema_files = sorted((ROOT / "common/sql/pg").glob("*.sql"))
        schema_files.extend([
            ROOT / "deployments/postgres/migrations/202607302045_alert_schema_runtime_ddl_exit.sql",
            ROOT / "deployments/postgres/migrations/202607302345_threat_intel_transactional_outbox.sql",
            ROOT / "deployments/postgres/migrations/202608031550_threat_intel_command_atomic.sql",
            ROOT / "deployments/postgres/migrations/202608031600_forensics_task_command_atomic.sql",
            ROOT / "deployments/postgres/migrations/202608031610_whitelist_governance_v2.sql",
            ROOT / "deployments/postgres/migrations/202608031620_dashboard_task_v2.sql",
            ROOT / "deployments/postgres/migrations/202608041930_dashboard_task_execution_pipeline_v1.sql",
        ])
        for path in schema_files:
            run([
                "docker", "exec", "-e", "PGOPTIONS=--client-min-messages=warning", "-i",
                container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "traffic_platform",
            ], input_bytes=path.read_bytes())
        result["schema_files"] = len(schema_files)

        run([
            "docker", "exec", container, "psql", "-v", "ON_ERROR_STOP=1", "-U", "postgres",
            "-d", "traffic_platform", "-c",
            "CREATE TABLE codex_ephemeral_asset_atomic_test_sentinel(marker text primary key); "
            "INSERT INTO codex_ephemeral_asset_atomic_test_sentinel(marker) VALUES ('ephemeral-only'); "
            "CREATE TABLE codex_ephemeral_campaign_aggregate_sentinel(marker text primary key); "
            "INSERT INTO codex_ephemeral_campaign_aggregate_sentinel(marker) VALUES ('ephemeral-only'); "
            "CREATE TABLE codex_ephemeral_probe_registry_sentinel(marker text primary key); "
            "INSERT INTO codex_ephemeral_probe_registry_sentinel(marker) VALUES ('ephemeral-only'); "
            "CREATE TABLE codex_ephemeral_threat_intel_command_sentinel(marker text primary key); "
            "INSERT INTO codex_ephemeral_threat_intel_command_sentinel(marker) VALUES ('ephemeral-only'); "
            "CREATE TABLE codex_ephemeral_forensics_task_sentinel(marker text primary key); "
            "INSERT INTO codex_ephemeral_forensics_task_sentinel(marker) VALUES ('ephemeral-only'); "
            "CREATE TABLE codex_ephemeral_whitelist_governance_sentinel(marker text primary key); "
            "INSERT INTO codex_ephemeral_whitelist_governance_sentinel(marker) VALUES ('ephemeral-only'); "
            "CREATE TABLE codex_ephemeral_dashboard_task_sentinel(marker text primary key); "
            "INSERT INTO codex_ephemeral_dashboard_task_sentinel(marker) VALUES ('ephemeral-only');",
        ])
        result["sentinel_verified"] = True

        port_output = run(["docker", "port", container, "5432/tcp"]).stdout.decode().strip()
        host_port = port_output.rsplit(":", 1)[-1]
        if not host_port.isdigit():
            raise RuntimeError("could not resolve ephemeral PostgreSQL loopback port")
        test_env = os.environ.copy()
        test_env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"] = (
            f"postgres://postgres:codex-ephemeral-only@127.0.0.1:{host_port}/"
            "traffic_platform?sslmode=disable"
        )
        test_env["AUTH_USER_ATOMIC_EPHEMERAL_PG_DSN"] = test_env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"]
        test_env["CAMPAIGN_AGGREGATE_EPHEMERAL_PG_DSN"] = test_env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"]
        test_env["PROBE_REGISTRY_EPHEMERAL_PG_DSN"] = test_env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"]
        test_env["THREAT_INTEL_COMMAND_EPHEMERAL_PG_DSN"] = test_env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"]
        test_env["FORENSICS_TASK_ATOMIC_EPHEMERAL_PG_DSN"] = test_env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"]
        test_env["WHITELIST_GOVERNANCE_EPHEMERAL_PG_DSN"] = test_env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"]
        test_env["DASHBOARD_TASK_EPHEMERAL_PG_DSN"] = test_env["ASSET_ATOMIC_EPHEMERAL_PG_DSN"]
        if args.suite == "asset-upsert-only":
            unit_tests = [
                "TestAssetUpsertIdentityV1GoldenAndBeginFailure",
                "TestUpsertAtomicSameKeyDifferentPayloadZeroWrite",
                "TestUpsertAtomicActionClassSourcePolicy",
                "TestUpsertAtomicCrashMatrix",
            ]
            integration_tests = [
                "TestAssetAtomicUpsertPostgresIntegration",
                "TestUpsertAtomicPostgresPreCommitFaultMatrix",
                "TestUpsertAtomicCommitUnknownSameKeyRecovery",
            ]
        else:
            unit_tests = [
                "TestAssetUpsertIdentityV1GoldenAndBeginFailure",
                "TestUpsertAtomicSameKeyDifferentPayloadZeroWrite",
                "TestUpsertAtomicActionClassSourcePolicy",
                "TestUpsertAtomicCrashMatrix",
            ]
            integration_tests = [
                "TestAssetAtomicUpsertPostgresIntegration",
                "TestDiscoveryResourceAtomicPostgresIntegration",
            ]
        unit_completed, _, unit_counts = run_exact_go_tests(
            go_root=ROOT / "go/control-plane",
            package="./internal/asset/repository",
            test_names=unit_tests,
            env=test_env,
        )
        print(unit_completed.stdout, end="")
        completed, integration_events, integration_counts = run_exact_go_tests(
            go_root=ROOT / "go/control-plane",
            package="./internal/asset/repository",
            test_names=integration_tests,
            env=test_env,
        )
        print(completed.stdout, end="")
        result["exact_test_events"] = {
            "unit": unit_counts,
            "integration": integration_counts,
        }
        if args.suite == "asset-upsert-only":
            result["oracle_results"] = collect_asset_oracle_markers(integration_events)
        if args.suite == "full":
            auth_completed = run([
                "go", "-C", "go/control-plane", "test", "./internal/auth/repository",
                "-run", "TestUserCommandAtomicPostgresIntegration",
                "-count=1", "-v",
            ], env=test_env, check=False)
            print(auth_completed.stdout.decode(), end="")
            if auth_completed.returncode != 0:
                raise RuntimeError(f"auth user atomic PostgreSQL integration test exited {auth_completed.returncode}")
            campaign_completed = run([
                "go", "-C", "go/control-plane", "test", "./internal/alert/api",
                "-run", "TestCampaignAggregateV2PostgresIntegration",
                "-count=1", "-v",
            ], env=test_env, check=False)
            print(campaign_completed.stdout.decode(), end="")
            if campaign_completed.returncode != 0:
                raise RuntimeError(f"campaign aggregate PostgreSQL integration test exited {campaign_completed.returncode}")
            probe_registry_completed = run([
                "go", "-C", "go/control-plane", "test", "./internal/ingest/server",
                "-run", "TestProbeRegistryAtomicPostgresIntegration",
                "-count=1", "-v",
            ], env=test_env, check=False)
            print(probe_registry_completed.stdout.decode(), end="")
            if probe_registry_completed.returncode != 0:
                raise RuntimeError(f"probe registry PostgreSQL integration test exited {probe_registry_completed.returncode}")
            threat_intel_completed = run([
                "go", "-C", "go/control-plane", "test", "./cmd/threat-intel-service",
                "-run", "TestThreatIntelCommandAtomicEphemeralPostgres",
                "-count=1", "-v",
            ], env=test_env, check=False)
            print(threat_intel_completed.stdout.decode(), end="")
            if threat_intel_completed.returncode != 0:
                raise RuntimeError(f"threat intel command PostgreSQL integration test exited {threat_intel_completed.returncode}")
            forensics_task_completed = run([
                "go", "-C", "go/control-plane", "test", "./internal/forensics/repository",
                "-run", "TestForensicsTaskCommandAtomicEphemeralPostgres",
                "-count=1", "-v",
            ], env=test_env, check=False)
            print(forensics_task_completed.stdout.decode(), end="")
            if forensics_task_completed.returncode != 0:
                raise RuntimeError(f"forensics task command PostgreSQL integration test exited {forensics_task_completed.returncode}")
            whitelist_completed = run([
                "go", "-C", "go/control-plane", "test", "./internal/alert/whitelist",
                "-run", "TestWhitelistGovernanceAtomicEphemeralPostgres",
                "-count=1", "-v",
            ], env=test_env, check=False)
            print(whitelist_completed.stdout.decode(), end="")
            if whitelist_completed.returncode != 0:
                raise RuntimeError(f"whitelist governance PostgreSQL integration test exited {whitelist_completed.returncode}")
            dashboard_task_completed = run([
                "go", "-C", "go/control-plane", "test", "./internal/alert/api",
                "-run", "TestDashboardTaskV2EphemeralPostgres",
                "-count=1", "-v",
            ], env=test_env, check=False)
            print(dashboard_task_completed.stdout.decode(), end="")
            if dashboard_task_completed.returncode != 0:
                raise RuntimeError(f"dashboard task PostgreSQL integration test exited {dashboard_task_completed.returncode}")
        result["status"] = "PASS"
    except Exception as exc:  # Fail closed while preserving cleanup evidence.
        result["errors"] = [str(exc)]
    finally:
        if created:
            run(["docker", "rm", "-f", container], check=False)
        result["container_removed"] = (
            run(["docker", "container", "inspect", container], check=False).returncode != 0
        )

    payload = json.dumps(result, ensure_ascii=False, indent=2) + "\n"
    if args.output:
        output = args.output.resolve()
        if output.exists():
            raise SystemExit(f"refusing to overwrite asset atomic evidence: {output}")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result["status"] == "PASS" and result["container_removed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
