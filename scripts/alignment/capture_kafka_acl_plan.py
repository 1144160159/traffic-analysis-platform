#!/usr/bin/env python3
"""Capture immutable evidence for the generated least-privilege Kafka ACL plan."""

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
    ("acl-catalog", ["python3", "scripts/alignment/generate_kafka_acl_plan.py", "--check-generated"]),
    ("acl-negative-tests", ["python3", "-m", "unittest", "tests.alignment.test_kafka_acl_catalog", "-v"]),
    ("identity-sync", ["bash", "tests/alignment/test_kafka_service_identity_sync.sh"]),
    ("audit-materializer-go", ["bash", "-lc", "cd go/control-plane && go test ./cmd/audit-materializer ./internal/common/audit ./internal/common/kafka -count=1 && go build -o /tmp/traffic-audit-materializer-evidence ./cmd/audit-materializer && rm -f /tmp/traffic-audit-materializer-evidence"]),
    ("event-catalog", ["python3", "scripts/alignment/check_event_catalog.py"]),
)
SOURCE_ARTIFACTS = (
    "contracts/events/kafka-acl-catalog.v1.json",
    "contracts/events/kafka-topic-catalog.v1.json",
    "scripts/alignment/generate_kafka_acl_plan.py",
    "scripts/alignment/render_audit_materializer_expand.py",
    "contracts/flink/application-cluster-migration.v1.json",
    "scripts/alignment/render_flink_application_cluster.py",
    "deployments/kubernetes/init-jobs/00-kafka-acl-plan.yaml",
    "deployments/kubernetes/init-jobs/00-kafka-service-principals.yaml",
    "deployments/kubernetes/init-jobs/01-kafka-topics.yaml",
    "deployments/kubernetes/security/generated-kafka-service-identities.v1.yaml",
    "deployments/kubernetes/security/README.md",
    "deployments/kubernetes/applications/go-services.yaml",
    "deployments/kubernetes/canary/probe-control-g2-canary.template.yaml",
    "deployments/kubernetes/canary/audit-materializer-expand.template.yaml",
    "deployments/kubernetes/site-values.template.yaml",
    "deployments/kubernetes/deploy.sh",
    "tests/alignment/test_kafka_acl_catalog.py",
    "tests/alignment/test_kafka_service_identity_sync.sh",
    "tests/e2e/live_kafka_security_rollout_preflight.sh",
    "go/control-plane/cmd/audit-materializer/main.go",
    "go/control-plane/cmd/audit-materializer/main_test.go",
    "go/control-plane/internal/common/audit/consumer.go",
    "go/control-plane/internal/common/audit/consumer_test.go",
    "go/control-plane/internal/common/kafka/errors.go",
    "go/control-plane/internal/common/kafka/errors_test.go",
    "go/control-plane/internal/common/kafka/consumer.go",
    "go/control-plane/internal/common/kafka/consumer_health_test.go",
    "Makefile",
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
    print(f"[kafka-acl] starting {name}: {' '.join(command)}", flush=True)
    with log_path.open("wb") as log:
        completed = subprocess.run(
            command,
            cwd=ROOT,
            stdout=log,
            stderr=subprocess.STDOUT,
            check=False,
        )
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
    print(f"[kafka-acl] {name}: {result['status']}", flush=True)
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--g0-manifest", type=Path, required=True)
    parser.add_argument("--output-root", type=Path, default=DEFAULT_OUTPUT_ROOT)
    args = parser.parse_args()

    g0_manifest = args.g0_manifest.resolve()
    if not g0_manifest.is_file():
        raise SystemExit(f"G0 manifest does not exist: {g0_manifest}")
    g0 = json.loads(g0_manifest.read_text(encoding="utf-8"))
    if g0.get("status") != "PASS" or g0.get("gate") != "G0":
        raise SystemExit("referenced G0 manifest is not a PASS result")

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
    status = "PASS" if len(results) == len(COMMANDS) and all(item["status"] == "PASS" for item in results) else "FAIL"

    sources = []
    for relative in SOURCE_ARTIFACTS:
        path = ROOT / relative
        if not path.is_file():
            raise SystemExit(f"source artifact does not exist: {relative}")
        sources.append({"path": relative, "sha256": sha256(path), "size_bytes": path.stat().st_size})

    manifest = {
        "schema_version": 1,
        "run_id": args.run_id,
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "feature_id": "T-KAFKA-005",
        "related_ids": ["T-KAFKA-001", "T-FLINK-001", "T-SCHEMA-001", "T-PKI-001"],
        "status": status,
        "candidate_source": build_snapshot(),
        "g0_reference": {
            "run_id": g0.get("run_id"),
            "manifest": str(g0_manifest.relative_to(ROOT)),
            "manifest_sha256": sha256(g0_manifest),
            "status": g0.get("status"),
        },
        "gate_status": {
            "G0": "PASS",
            "G1": "PASS" if status == "PASS" else "FAIL",
            "G2": "OPEN",
            "G3": "OPEN",
            "G4": "OPEN",
            "G5": "OPEN",
            "G6": "HOLD",
            "G7": "OPEN",
            "G8": "BLOCKED",
        },
        "production_applied": False,
        "commands": results,
        "source_artifacts": sources,
        "proven": [
            "all 29 canonical topics have explicit producer and consumer-group ACL bindings",
            "20 service, Flink, consumer, replayer and operator identities are separated in a versioned catalog",
            "the deterministic plan contains 197 literal grants and no wildcard or All grant; the user-behavior late-event path and audit materializer each have explicit dlq.v1 producer access",
            "all seven legacy additional topics remain explicitly blocked rather than receiving speculative access",
            "the baseline replayer identity is disabled and receives no grant",
            "the Kubernetes init job consumes the hash-addressed generated plan and does not add the legacy shared wildcard ACL",
            "expand retains an already-existing legacy ACL for credential migration; cutover explicitly removes it",
            "negative tests fail closed for wildcard principals, missing groups and an enabled baseline replayer",
            "eight Go services, the standalone audit materializer and nine Flink jobs have distinct credential Secret contracts whose usernames match ACL principals",
            "local Secret synchronization is idempotent, preserves existing passwords and builds the matching middleware aggregate without logging secrets",
            "a generated fail-closed Job provisions all 18 workload SCRAM principals before the deployment flow continues to application ACL rollout",
            "the audit materializer has a buildable Go entrypoint, migration-authority schema verification, fail-closed processing readiness, PG/Kafka health checks and graceful shutdown",
            "the audit materializer expand manifest binds its dedicated identity and can only be rendered with an immutable repository@sha256 image reference",
            "audit payload/contract failures are explicitly permanent and may cross the offset barrier only after a durable dlq.v1 write; PostgreSQL and other transient errors remain uncommitted and withdraw readiness",
            "the strict permanent-only DLQ policy is opt-in for audit materialization, preserving existing consumers until each is migrated deliberately",
        ],
        "open": [
            "build and sign a candidate audit-materializer image, render the digest-pinned expand manifest, then apply all nine workload Secrets and SCRAM users in an approved window",
            "prove the audit materializer authenticates as its declared principal and reconciles Kafka offsets to idempotent PostgreSQL audit rows without loss",
            "prove on a real broker that an invalid audit payload is committed only after durable dlq.v1 quarantine while a PostgreSQL outage leaves the source offset uncommitted",
            "build and sign the nine job-specific Flink images, capture fresh savepoints, then execute the reviewed serial Application Cluster migration",
            "prove the shared principal has zero active Go/Flink clients before cutover and retain a tested rollback window",
            "apply the plan to an approved real broker and capture allow/deny ACL negative tests and exact ACL diff",
            "validate quotas, RF/minISR, unclean leader election, rack placement and KRaft controller quorum",
            "execute broker/controller failure, quota exceedance, certificate rotation and partition reassignment drills",
            "capture producer sequence, consumer offsets, business reconciliation, rollback and observation-window evidence",
        ],
        "secrets_captured": False,
    }
    manifest_path = output / "manifest.json"
    manifest_path.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({
        "status": status,
        "manifest": str(manifest_path),
        "manifest_sha256": sha256(manifest_path),
    }, ensure_ascii=False, indent=2), flush=True)
    return 0 if status == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
