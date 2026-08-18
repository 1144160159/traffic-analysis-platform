#!/usr/bin/env python3
"""Render one guarded Flink Native Kubernetes Application Cluster migration."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from pathlib import Path
from typing import Any

import yaml


ROOT = Path(__file__).resolve().parents[2]
CONTRACT_PATH = ROOT / "contracts/flink/application-cluster-migration.v1.json"
ACL_PATH = ROOT / "contracts/events/kafka-acl-catalog.v1.json"
IMAGE_RE = re.compile(r"^[a-z0-9][a-z0-9._/:~-]*@sha256:[0-9a-f]{64}$")
SHA_RE = re.compile(r"^[0-9a-f]{64}$")
JOB_ID_RE = re.compile(r"^[0-9a-f]{32}$")
SAVEPOINT_PREFIX = "s3://flink-checkpoints/savepoints/"
M03_CONSUMER_GROUP_ARGUMENT = {
    "flink-session-job": "--consumer.group",
    "flink-feature-job": "--kafka.group.id",
}


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def canonical_json_digest(value: Any) -> str:
    payload = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def validate_contract(contract: dict[str, Any], acl: dict[str, Any]) -> dict[str, Any]:
    errors: list[str] = []
    jobs = contract.get("jobs") or []
    if contract.get("schema_version") != 1 or contract.get("phase") != "expand":
        errors.append("migration contract must be schema v1 in expand phase")
    if contract.get("namespace") != "flink":
        errors.append("application clusters must run in flink namespace")
    migration = contract.get("migration") or {}
    if migration.get("max_parallel_applications_during_migration") != 1:
        errors.append("migration must permit exactly one application at a time")
    state = contract.get("state") or {}
    if state.get("allow_non_restored_state") is not False:
        errors.append("allow_non_restored_state must be false")
    for key in ("checkpoint_root", "savepoint_root", "ha_root", "job_result_root"):
        value = str(state.get(key, ""))
        if not value.startswith("s3://flink-checkpoints/") or "${" in value:
            errors.append(f"state {key} must use the literal flink-checkpoints S3 bucket")
    if len(jobs) != 9:
        errors.append(f"expected 9 Flink jobs, found {len(jobs)}")

    orders = [job.get("migration_order") for job in jobs]
    if sorted(orders) != list(range(1, 10)):
        errors.append("migration_order must be unique and contiguous 1..9")
    for field in ("id", "cluster_id", "principal_id", "job_name", "jar_uri"):
        values = [job.get(field) for job in jobs]
        if len(values) != len(set(values)):
            errors.append(f"job field {field} must be unique")

    principals = {item.get("id"): item for item in acl.get("principals") or []}
    resources = contract.get("common_resources") or {}
    tm = resources.get("taskmanager") or {}
    slots = tm.get("slots")
    if not isinstance(slots, int) or slots <= 0:
        errors.append("taskmanager slots must be a positive integer")
        slots = 1
    max_cpu = 0
    max_memory = 0
    jm = resources.get("jobmanager") or {}
    jm_replicas = jm.get("replicas")
    if not isinstance(jm_replicas, int) or jm_replicas < 2:
        errors.append("jobmanager replicas must be at least 2 for Kubernetes HA")
        jm_replicas = 1
    for job in jobs:
        job_id = str(job.get("id", ""))
        principal = principals.get(job.get("principal_id")) or {}
        credential = principal.get("credential") or {}
        if principal.get("kind") != "flink_job" or principal.get("rollout_state") != "expand":
            errors.append(f"{job_id}: principal must be an expanded flink_job")
        if credential.get("namespace") != "flink" or credential.get("workload") != job_id:
            errors.append(f"{job_id}: principal credential does not bind this Flink workload")
        uri = str(job.get("jar_uri", ""))
        if uri != f"local:///opt/flink/usrlib/{job_id}.jar":
            errors.append(f"{job_id}: Application Mode JAR URI is not canonical")
        parallelism = job.get("parallelism")
        max_parallelism = job.get("max_parallelism")
        replicas = job.get("taskmanager_replicas_ceiling")
        if not isinstance(parallelism, int) or parallelism <= 0:
            errors.append(f"{job_id}: parallelism must be positive")
            continue
        if (
            not isinstance(max_parallelism, int)
            or max_parallelism < parallelism
            or max_parallelism > 32768
        ):
            errors.append(
                f"{job_id}: max_parallelism must be between parallelism and 32768"
            )
        expected_replicas = (parallelism + slots - 1) // slots
        if replicas != expected_replicas:
            errors.append(
                f"{job_id}: taskmanager replica ceiling {replicas} != {expected_replicas}"
            )
        max_cpu = max(max_cpu, jm_replicas * int(jm.get("cpu", 0)) + expected_replicas * int(tm.get("cpu", 0)))
        memory_mib = jm_replicas * int(str(jm.get("memory_process", "0m")).removesuffix("m"))
        memory_mib += expected_replicas * int(str(tm.get("memory_process", "0m")).removesuffix("m"))
        max_memory = max(max_memory, (memory_mib + 1023) // 1024)

    budget = contract.get("online_expand_budget") or {}
    if max_cpu > int(budget.get("max_additional_cpu_requests", 0)):
        errors.append("largest serial application exceeds CPU expand budget")
    if max_memory > int(budget.get("max_additional_memory_requests_gib", 0)):
        errors.append("largest serial application exceeds memory expand budget")
    if budget.get("automatic_pause_on_exceed") is not True:
        errors.append("expand budget must automatically pause on exceed")
    return {
        "result": "pass" if not errors else "blocked",
        "errors": errors,
        "jobs": len(jobs),
        "expected_tasks": sum(int(job.get("expected_tasks", 0)) for job in jobs),
        "max_serial_cpu_request": max_cpu,
        "max_serial_memory_request_gib": max_memory,
    }


def _savepoint_for(job_id: str, manifest: dict[str, Any], contract: dict[str, Any]) -> dict[str, str]:
    if manifest.get("schema_version") != 1:
        raise ValueError("savepoint manifest must use schema_version 1")
    if manifest.get("source_cluster_id") != contract.get("source_session_cluster_id"):
        raise ValueError("savepoint manifest source cluster does not match migration contract")
    entry = (manifest.get("savepoints") or {}).get(job_id)
    if not isinstance(entry, dict):
        raise ValueError(f"savepoint manifest has no entry for {job_id}")
    uri = str(entry.get("uri", ""))
    digest = str(entry.get("sha256", ""))
    source_job_id = str(entry.get("source_job_id", ""))
    if not uri.startswith(SAVEPOINT_PREFIX) or "${" in uri or ".." in uri:
        raise ValueError(f"{job_id}: savepoint URI must be an immutable MinIO savepoint path")
    if not SHA_RE.fullmatch(digest):
        raise ValueError(f"{job_id}: savepoint sha256 is invalid")
    if not JOB_ID_RE.fullmatch(source_job_id):
        raise ValueError(f"{job_id}: source Flink job id is invalid")
    return {"uri": uri, "sha256": digest, "source_job_id": source_job_id}


def _activation_arguments(
    job_id: str,
    activation_mode: str,
    candidate_sha256: str,
) -> tuple[list[str], dict[str, str]]:
    mode = activation_mode.strip().lower()
    candidate = candidate_sha256.strip().lower()
    if mode not in {"legacy", "shadow", "production"}:
        raise ValueError("activation mode must be legacy, shadow or production")
    annotations = {"traffic.openai.com/deployment-activation": mode}
    if mode == "legacy":
        if candidate:
            raise ValueError("legacy activation must not carry a candidate sha256")
        return [], annotations
    group_argument = M03_CONSUMER_GROUP_ARGUMENT.get(job_id)
    if group_argument is None:
        raise ValueError("shadow and production activation are limited to the M03 Session/Feature jobs")
    if not SHA_RE.fullmatch(candidate):
        raise ValueError("shadow and production activation require a lowercase candidate sha256")
    consumer_group = job_id
    if mode == "shadow":
        consumer_group = f"{job_id}-shadow-{candidate[:12]}"
    annotations.update({
        "traffic.openai.com/candidate-sha256": candidate,
        "traffic.openai.com/consumer-group": consumer_group,
    })
    return [
        "--deployment.activation.mode", mode,
        "--deployment.candidate.sha256", candidate,
        group_argument, consumer_group,
    ], annotations


def _pod_template(job: dict[str, Any], credential: dict[str, Any], contract: dict[str, Any]) -> str:
    empty_dir_size = contract["common_resources"]["local_rocksdb_empty_dir_size"]
    env = [
        {"name": "ENABLE_BUILT_IN_PLUGINS", "value": "flink-s3-fs-presto-1.18.1.jar"},
        {"name": "KAFKA_SECURITY_PROTOCOL", "value": "SASL_SSL"},
        {"name": "KAFKA_SASL_MECHANISM", "value": "SCRAM-SHA-512"},
        {"name": "KAFKA_SASL_USERNAME", "valueFrom": {"secretKeyRef": {"name": credential["secret_name"], "key": "username"}}},
        {"name": "KAFKA_SASL_PASSWORD", "valueFrom": {"secretKeyRef": {"name": credential["secret_name"], "key": "password"}}},
        {"name": "KAFKA_SSL_TRUSTSTORE_LOCATION", "value": "/etc/kafka/tls/kafka.truststore.p12"},
        {"name": "KAFKA_SSL_TRUSTSTORE_TYPE", "value": "PKCS12"},
        {"name": "KAFKA_SSL_TRUSTSTORE_PASSWORD", "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": "KAFKA_TLS_TRUSTSTORE_PASSWORD"}}},
        {"name": "MINIO_ACCESS_KEY", "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": "MINIO_ACCESS_KEY"}}},
        {"name": "MINIO_SECRET_KEY", "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": "MINIO_SECRET_KEY"}}},
        {"name": "AWS_ACCESS_KEY_ID", "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": "MINIO_ACCESS_KEY"}}},
        {"name": "AWS_SECRET_ACCESS_KEY", "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": "MINIO_SECRET_KEY"}}},
        {"name": "CLICKHOUSE_PASSWORD", "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": "CLICKHOUSE_PASSWORD", "optional": True}}},
        {"name": "OPENSEARCH_PASSWORD", "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": "OPENSEARCH_ADMIN_PASSWORD", "optional": True}}},
        {"name": "POSTGRES_PASSWORD", "valueFrom": {"secretKeyRef": {"name": "traffic-credentials", "key": "PG_PASSWORD", "optional": True}}},
    ]
    pod = {
        "apiVersion": "v1",
        "kind": "Pod",
        "metadata": {"name": f"{job['cluster_id']}-pod-template", "labels": {"traffic.openai.com/flink-job-id": job["id"]}},
        "spec": {
            "serviceAccountName": f"{job['cluster_id']}-runtime",
            "containers": [{
                "name": "flink-main-container",
                "env": env,
                "volumeMounts": [
                    {"name": "kafka-client-tls", "mountPath": "/etc/kafka/tls", "readOnly": True},
                    {"name": "rocksdb-local", "mountPath": "/opt/flink/rocksdb"},
                ],
            }],
            "volumes": [
                {"name": "kafka-client-tls", "secret": {"secretName": "kafka-client-tls", "optional": False}},
                {"name": "rocksdb-local", "emptyDir": {"sizeLimit": empty_dir_size}},
            ],
        },
    }
    return yaml.safe_dump(pod, sort_keys=False)


def render(
    job_id: str,
    image: str,
    savepoint_manifest: dict[str, Any],
    activation_mode: str = "legacy",
    candidate_sha256: str = "",
    rollback_image: str | None = None,
) -> str:
    if not IMAGE_RE.fullmatch(image) or ":latest" in image or "${" in image:
        raise ValueError("application image must be a lowercase repository@sha256 digest")
    contract = load_json(CONTRACT_PATH)
    acl = load_json(ACL_PATH)
    validation = validate_contract(contract, acl)
    if validation["result"] != "pass":
        raise ValueError("invalid Flink migration contract: " + "; ".join(validation["errors"]))
    jobs = {job["id"]: job for job in contract["jobs"]}
    if job_id not in jobs:
        raise ValueError(f"unknown Flink job id: {job_id}")
    job = jobs[job_id]
    principals = {item["id"]: item for item in acl["principals"]}
    credential = principals[job["principal_id"]]["credential"]
    savepoint = _savepoint_for(job_id, savepoint_manifest, contract)
    activation_arguments, activation_annotations = _activation_arguments(
        job_id, activation_mode, candidate_sha256
    )
    previous_image = rollback_image or image
    if not IMAGE_RE.fullmatch(previous_image) or ":latest" in previous_image or "${" in previous_image:
        raise ValueError("rollback image must be a lowercase repository@sha256 digest")
    activation = activation_annotations["traffic.openai.com/deployment-activation"]
    resource_revision = "v1"
    if activation != "legacy":
        resource_revision = f"{activation}-{candidate_sha256[:12]}"
    runtime_cluster_id = job["cluster_id"]
    state_instance = job_id
    if activation != "legacy":
        runtime_cluster_id = f"{job['cluster_id']}-{resource_revision}"
        state_instance = f"{job_id}/{resource_revision}"
    contract_digest = canonical_json_digest(contract)
    manifest_digest = canonical_json_digest(savepoint_manifest)
    annotations = {
        "traffic.openai.com/contract-sha256": contract_digest,
        "traffic.openai.com/savepoint-manifest-sha256": manifest_digest,
        "traffic.openai.com/savepoint-sha256": savepoint["sha256"],
        "traffic.openai.com/source-job-id": savepoint["source_job_id"],
        "traffic.openai.com/migration-order": str(job["migration_order"]),
        **activation_annotations,
    }
    sa_name = f"{job['cluster_id']}-runtime"
    role_name = "flink-application-cluster-runtime-v1"
    pod_config_name = f"{job['cluster_id']}-pod-template-{resource_revision}"
    jm = contract["common_resources"]["jobmanager"]
    tm = contract["common_resources"]["taskmanager"]
    state = contract["state"]
    checkpoint_path = f"{state['checkpoint_root']}/{state_instance}"
    savepoint_path = f"{state['savepoint_root']}/{state_instance}"
    ha_path = f"{state['ha_root']}/{runtime_cluster_id}"
    job_result_path = f"{state['job_result_root']}/{runtime_cluster_id}"
    command = [
        "/opt/flink/bin/flink", "run-application", "--target", "kubernetes-application",
        f"-Dkubernetes.cluster-id={runtime_cluster_id}",
        "-Dkubernetes.namespace=flink",
        f"-Dkubernetes.container.image.ref={image}",
        f"-Dkubernetes.service-account={sa_name}",
        "-Dkubernetes.pod-template-file.default=/opt/traffic/pod-template/pod-template.yaml",
        "-Dkubernetes.rest-service.exposed.type=ClusterIP",
        "-Dhigh-availability.type=kubernetes",
        f"-Dhigh-availability.cluster-id={runtime_cluster_id}",
        f"-Dhigh-availability.storageDir={ha_path}",
        f"-Dkubernetes.jobmanager.replicas={jm['replicas']}",
        f"-Djob-result-store.storage-path={job_result_path}",
        "-Djob-result-store.delete-on-commit=false",
        f"-Dstate.checkpoints.dir={checkpoint_path}",
        f"-Dstate.savepoints.dir={savepoint_path}",
        "-Dstate.backend=rocksdb",
        "-Dstate.backend.rocksdb.localdir=/opt/flink/rocksdb",
        "-Ds3.endpoint=http://minio.minio.svc:9000",
        "-Ds3.path.style.access=true",
        "-Dexecution.checkpointing.externalized-checkpoint-retention=RETAIN_ON_CANCELLATION",
        f"-Dkubernetes.jobmanager.cpu={jm['cpu']}",
        f"-Djobmanager.memory.process.size={jm['memory_process']}",
        f"-Dkubernetes.taskmanager.cpu={tm['cpu']}",
        f"-Dtaskmanager.memory.process.size={tm['memory_process']}",
        f"-Dtaskmanager.numberOfTaskSlots={tm['slots']}",
        f"-Dpipeline.max-parallelism={job['max_parallelism']}",
        "-c", job["main_class"], "-p", str(job["parallelism"]),
        "-s", savepoint["uri"], job["jar_uri"],
        *activation_arguments,
    ]
    docs: list[dict[str, Any]] = [
        {"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": sa_name, "namespace": "flink", "annotations": annotations}},
        {
            "apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
            "metadata": {"name": role_name, "namespace": "flink", "annotations": {"traffic.openai.com/contract-sha256": contract_digest}},
            "rules": [
                {"apiGroups": [""], "resources": ["pods", "pods/log", "services"], "verbs": ["get", "list", "watch", "create", "delete"]},
                {"apiGroups": [""], "resources": ["configmaps"], "verbs": ["get", "list", "watch", "create", "update", "patch", "delete"]},
                {"apiGroups": ["apps"], "resources": ["deployments"], "verbs": ["get", "list", "watch", "create", "update", "patch", "delete"]},
            ],
        },
        {
            "apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
            "metadata": {"name": f"{sa_name}-v1", "namespace": "flink", "annotations": annotations},
            "subjects": [{"kind": "ServiceAccount", "name": sa_name, "namespace": "flink"}],
            "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": role_name},
        },
        {
            "apiVersion": "v1", "kind": "ConfigMap",
            "metadata": {"name": pod_config_name, "namespace": "flink", "annotations": annotations},
            "data": {"pod-template.yaml": _pod_template(job, credential, contract)},
        },
        {
            "apiVersion": "v1", "kind": "ConfigMap",
            "metadata": {"name": f"{job['cluster_id']}-rollback-{resource_revision}", "namespace": "flink", "annotations": annotations},
            "immutable": True,
            "data": {
                "source-cluster-id": contract["source_session_cluster_id"],
                "source-job-id": savepoint["source_job_id"],
                "rollback-savepoint-uri": savepoint["uri"],
                "rollback-savepoint-sha256": savepoint["sha256"],
                "allow-non-restored-state": "false",
            },
        },
        {
            "apiVersion": "batch/v1", "kind": "Job",
            "metadata": {"name": f"migrate-{job_id}-{resource_revision}", "namespace": "flink", "annotations": annotations},
            "spec": {
                "backoffLimit": 0,
                "ttlSecondsAfterFinished": 86400,
                "template": {
                    "metadata": {"labels": {"app": "flink-application-migrator", "traffic.openai.com/flink-job-id": job_id}},
                    "spec": {
                        "serviceAccountName": sa_name,
                        "restartPolicy": "Never",
                        "containers": [{
                            "name": "launch-application-cluster",
                            "image": image,
                            "imagePullPolicy": "IfNotPresent",
                            "command": command,
                            "volumeMounts": [{"name": "pod-template", "mountPath": "/opt/traffic/pod-template", "readOnly": True}],
                            "resources": {"requests": {"cpu": "100m", "memory": "256Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                        }],
                        "volumes": [{"name": "pod-template", "configMap": {"name": pod_config_name}}],
                    },
                },
            },
        },
        {
            "apiVersion": "batch/v1", "kind": "Job",
            "metadata": {"name": f"rollback-{job_id}-{resource_revision}", "namespace": "flink", "annotations": annotations},
            "spec": {
                "suspend": True,
                "backoffLimit": 0,
                "ttlSecondsAfterFinished": 86400,
                "template": {
                    "metadata": {"labels": {"app": "flink-session-rollback", "traffic.openai.com/flink-job-id": job_id}},
                    "spec": {
                        "serviceAccountName": sa_name,
                        "restartPolicy": "Never",
                        "containers": [{
                            "name": "restore-session-cluster-job",
                            "image": previous_image,
                            "imagePullPolicy": "IfNotPresent",
                            "command": [
                                "/opt/flink/bin/flink", "run", "--target", "kubernetes-session",
                                "-Dkubernetes.cluster-id=flink-traffic", "-Dkubernetes.namespace=flink",
                                f"-Dpipeline.max-parallelism={job['max_parallelism']}",
                                "-c", job["main_class"], "-p", str(job["parallelism"]),
                                "-s", savepoint["uri"], job["jar_uri"],
                            ],
                            "resources": {"requests": {"cpu": "100m", "memory": "256Mi"}, "limits": {"cpu": "1", "memory": "1Gi"}},
                        }],
                    },
                },
            },
        },
    ]
    return "# Generated canary/expand manifest. Apply only after the recorded stop-with-savepoint gate.\n" + yaml.safe_dump_all(docs, sort_keys=False)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--job-id")
    parser.add_argument("--image")
    parser.add_argument("--savepoint-manifest", type=Path)
    parser.add_argument(
        "--activation-mode",
        choices=("legacy", "shadow", "production"),
        default="legacy",
    )
    parser.add_argument("--candidate-sha256", default="")
    parser.add_argument("--rollback-image")
    parser.add_argument("--output", type=Path)
    parser.add_argument("--check-contract", action="store_true")
    args = parser.parse_args()
    if args.check_contract:
        result = validate_contract(load_json(CONTRACT_PATH), load_json(ACL_PATH))
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0 if result["result"] == "pass" else 1
    if not args.job_id or not args.image or not args.savepoint_manifest:
        parser.error("--job-id, --image and --savepoint-manifest are required for rendering")
    payload = render(
        args.job_id,
        args.image,
        load_json(args.savepoint_manifest),
        args.activation_mode,
        args.candidate_sha256,
        args.rollback_image,
    )
    if args.output:
        args.output.write_text(payload, encoding="utf-8")
    else:
        print(payload, end="")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
