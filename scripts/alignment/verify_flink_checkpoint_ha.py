#!/usr/bin/env python3
"""Verify the repository-side T-FLINK-003 checkpoint, HA and upgrade contract."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / "contracts/flink/checkpoint-ha-upgrade.v1.json"
APPLICATION = ROOT / "contracts/flink/application-cluster-migration.v1.json"


def _load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def _container(document: dict[str, Any]) -> dict[str, Any]:
    containers = document["spec"]["template"]["spec"].get("containers") or []
    if len(containers) != 1:
        raise ValueError(f"{document['metadata']['name']}: expected one Flink container")
    return containers[0]


def verify(root: Path = ROOT) -> dict[str, Any]:
    contract = _load_json(root / CONTRACT.relative_to(ROOT))
    app = _load_json(root / APPLICATION.relative_to(ROOT))
    errors: list[str] = []

    if contract.get("schema_version") != 1 or contract.get("remediation_id") != "T-FLINK-003":
        errors.append("checkpoint HA contract must be schema v1 for T-FLINK-003")
    if contract.get("scope", {}).get("canonical_jobs") != 9 or len(app.get("jobs") or []) != 9:
        errors.append("checkpoint HA contract and application migration must cover nine jobs")

    ha = contract.get("ha") or {}
    session_path = root / contract["scope"]["session_cluster_manifest"]
    session_docs = [doc for doc in yaml.safe_load_all(session_path.read_text(encoding="utf-8")) if doc]
    stateful = {
        doc.get("metadata", {}).get("name"): doc
        for doc in session_docs
        if doc.get("kind") == "StatefulSet"
    }
    for name in ("flink-jobmanager", "flink-taskmanager"):
        if name not in stateful:
            errors.append(f"session cluster is missing {name}")
    jm = stateful.get("flink-jobmanager")
    if jm:
        replicas = jm.get("spec", {}).get("replicas")
        if not isinstance(replicas, int) or replicas < ha.get("minimum_jobmanager_replicas", 2):
            errors.append("session JobManager must have at least two replicas")

    required_properties = {
        "high-availability.type: kubernetes",
        "state.checkpoints.dir: s3://flink-checkpoints/checkpoints/",
        "state.savepoints.dir: s3://flink-checkpoints/savepoints/",
        "high-availability.storageDir: s3://flink-checkpoints/ha/",
        "job-result-store.storage-path: s3://flink-checkpoints/job-result-store/",
        "job-result-store.delete-on-commit: false",
    }
    for name in ("flink-jobmanager", "flink-taskmanager"):
        document = stateful.get(name)
        if not document:
            continue
        container = _container(document)
        env = {item.get("name"): item for item in container.get("env") or []}
        for required_env in ha.get("credential_environment") or []:
            item = env.get(required_env) or {}
            if "secretKeyRef" not in (item.get("valueFrom") or {}):
                errors.append(f"{name}: {required_env} must come from a Secret")
        flink_properties = str((env.get("FLINK_PROPERTIES") or {}).get("value", ""))
        for token in required_properties:
            if token not in flink_properties:
                errors.append(f"{name}: FLINK_PROPERTIES missing {token}")
        for token in ha.get("forbidden_flink_properties_tokens") or []:
            if token in flink_properties:
                errors.append(f"{name}: FLINK_PROPERTIES contains forbidden {token}")

    app_state = app.get("state") or {}
    for key in ("checkpoint_root", "savepoint_root", "ha_root", "job_result_root"):
        if not str(app_state.get(key, "")).startswith("s3://flink-checkpoints/"):
            errors.append(f"application contract {key} is not durable S3 storage")
    app_jm = (app.get("common_resources") or {}).get("jobmanager") or {}
    if int(app_jm.get("replicas", 0)) < ha.get("minimum_jobmanager_replicas", 2):
        errors.append("application clusters must declare standby JobManagers")
    renderer = (root / "scripts/alignment/render_flink_application_cluster.py").read_text(encoding="utf-8")
    for token in (
        "-Dkubernetes.jobmanager.replicas=",
        "-Djob-result-store.storage-path=",
        "-Djob-result-store.delete-on-commit=false",
        "-Dhigh-availability.storageDir=",
    ):
        if token not in renderer:
            errors.append(f"application renderer missing {token}")

    source_policy = contract.get("source_start_policy") or {}
    if source_policy.get("default") != "committed-or-earliest":
        errors.append("Kafka source default must be committed-or-earliest")
    common_policy = (root / "java/flink-jobs/flink-common/src/main/java/com/traffic/flink/common/KafkaStartingOffsets.java").read_text(encoding="utf-8")
    if 'DEFAULT_MODE = "committed-or-earliest"' not in common_policy:
        errors.append("shared Kafka startup policy has an unsafe default")
    latest_hits: list[str] = []
    java_root = root / "java/flink-jobs"
    for path in java_root.glob("flink-*-job/src/main/java/**/*.java"):
        source = path.read_text(encoding="utf-8")
        if "OffsetsInitializer.latest()" in source:
            latest_hits.append(path.relative_to(root).as_posix())
    if latest_hits:
        errors.append(f"production jobs still hard-code latest offsets: {latest_hits}")

    app_jobs = {job["id"]: job for job in app.get("jobs") or []}
    for job_id in source_policy.get("data_sources_requiring_committed_or_earliest") or []:
        job = app_jobs.get(job_id)
        if not job:
            errors.append(f"source policy references unknown job {job_id}")
            continue
        module = root / "java/flink-jobs" / job["module"]
        java_sources = "\n".join(
            path.read_text(encoding="utf-8")
            for path in module.glob("src/main/java/**/*.java")
        )
        if "KafkaStartingOffsets.from(params)" not in java_sources:
            errors.append(f"{job_id}: source does not use shared startup policy")
        properties = list(module.glob("src/main/resources/*.properties"))
        if not properties or not any(
            "kafka.starting.offsets=committed-or-earliest" in path.read_text(encoding="utf-8")
            for path in properties
        ):
            errors.append(f"{job_id}: production properties do not declare safe startup mode")

    for job in app_jobs.values():
        module = root / "java/flink-jobs" / job["module"]
        java_sources = "\n".join(
            path.read_text(encoding="utf-8")
            for path in module.glob("src/main/java/**/*.java")
        )
        if "enableCheckpointing" not in java_sources:
            errors.append(f"{job['id']}: checkpointing is not enabled")

    slo_path = root / contract["scope"]["checkpoint_slo_manifest"]
    slo_docs = list(yaml.safe_load_all(slo_path.read_text(encoding="utf-8")))
    if len(slo_docs) != 1 or slo_docs[0].get("kind") != "PrometheusRule":
        errors.append("checkpoint SLO manifest must contain one PrometheusRule")
        alert_names: set[str] = set()
        serialized_slo = ""
    else:
        rules = slo_docs[0].get("spec", {}).get("groups", [{}])[0].get("rules") or []
        alert_names = {rule.get("alert") for rule in rules if rule.get("alert")}
        serialized_slo = json.dumps(slo_docs[0], ensure_ascii=False)
    for alert in (
        "FlinkCheckpointSuccessRatioBelowSLO",
        "FlinkCheckpointDurationExceedsHalfInterval",
        "FlinkJobRecoveryExceedsRTO",
    ):
        if alert not in alert_names:
            errors.append(f"checkpoint SLO manifest missing {alert}")
    for threshold in ("0.999", "30000", "15000", "300000"):
        if threshold not in serialized_slo:
            errors.append(f"checkpoint SLO manifest missing threshold {threshold}")

    runbook = root / "doc/07_alignment/runbooks/T-FLINK-003-checkpoint-ha-upgrade.md"
    if not runbook.exists() or "allowNonRestoredState=false" not in runbook.read_text(encoding="utf-8"):
        errors.append("checkpoint HA upgrade rollback runbook is missing or unsafe")

    return {
        "schema_version": 1,
        "contract_id": contract.get("contract_id"),
        "remediation_id": "T-FLINK-003",
        "status": "PASS" if not errors else "FAIL",
        "canonical_jobs": len(app_jobs),
        "session_jobmanager_replicas": jm.get("spec", {}).get("replicas") if jm else None,
        "application_jobmanager_replicas": app_jm.get("replicas"),
        "hard_coded_latest_sources": latest_hits,
        "safe_default_sources": len(source_policy.get("data_sources_requiring_committed_or_earliest") or []),
        "slo_alerts": sorted(alert_names),
        "errors": errors,
        "remaining_gates": list((contract.get("remaining_gates") or {}).values()),
    }


def main() -> int:
    result = verify()
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0 if result["status"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
