#!/usr/bin/env python3
"""Isolated Kubernetes canary for the M08 governed dataset/training seam."""

from __future__ import annotations

import json
import hashlib
import os
import re
import shutil
import tempfile
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path

import pandas as pd
from jsonschema import Draft202012Validator, FormatChecker

from dataset_governance import (
    ExtractionScope,
    build_dataset_manifest,
    canonical_json_sha256,
    split_by_time_and_validate,
    sha256_file,
    validate_dataset_manifest_identity,
    write_json_exclusive,
)
from gnn_training import run_governed_gnn_training
from governed_evaluation import run_governed_evaluation
from governed_explanation import run_governed_explanation
from graph_governance import build_graph_snapshot_manifest
from model_artifact_governance import run_governed_model_export, verify_export_package
from register_model import build_governed_registration_payload, write_registration_receipt
from train_model import run_governed_training


UTC = timezone.utc


def required(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise ValueError(f"{name} is required")
    return value


def synthetic_frame(run_id: str) -> pd.DataFrame:
    base = datetime(2026, 8, 14, 0, 0, tzinfo=UTC)
    runtime_feature_profile = os.getenv("M08_CANARY_RUNTIME_FEATURE_PROFILE", "").strip()
    if runtime_feature_profile not in ("", "feature_stat_v1"):
        raise ValueError("unsupported M08_CANARY_RUNTIME_FEATURE_PROFILE")
    rows = []
    for index in range(80):
        if index < 40:
            timestamp = base + timedelta(minutes=index)
            family = "known-scan"
        elif index < 56:
            timestamp = base + timedelta(hours=1, minutes=index - 40)
            family = "known-scan"
        elif index < 72:
            timestamp = base + timedelta(hours=2, minutes=index - 56)
            family = "known-scan"
        else:
            timestamp = base + timedelta(hours=3, minutes=index - 72)
            family = "unknown-holdout"
        label = 1 if index >= 72 else index % 2
        row = {
            "tenant_id": "canary-m08-" + run_id.replace("-", "")[:12],
            "feature_set_id": "m08-canary-v1",
            "event_id": f"{run_id}:event:{index:03d}",
            "object_id": f"object-{index:03d}",
            "community_id": f"community-{index:03d}",
            "entity_id": f"entity-{index:03d}",
            "site_id": f"site-{index:03d}",
            "pcap_id": f"pcap-{index:03d}",
            "attack_family": family,
            "graph_node_ids": json.dumps([f"node-{index:03d}"]),
            "graph_edge_ids": json.dumps([f"edge-{index:03d}"]),
            "ts": int(timestamp.timestamp()),
            "ingest_ts": int((timestamp + timedelta(seconds=10)).timestamp()),
            "label": label,
            "feature_signal": float(label * 10 + index / 1000),
            "feature_noise": float((index * 17) % 11),
        }
        if runtime_feature_profile == "feature_stat_v1":
            row.update({
                "pps": float(20 + label * 80 + index / 10),
                "bps": float(2048 + label * 8192 + index * 16),
            })
        rows.append(row)
    return pd.DataFrame(rows)


def create_dataset(root: Path, graph_root: Path, run_id: str) -> tuple[dict, dict]:
    frame = synthetic_frame(run_id)
    runtime_feature_profile = os.getenv("M08_CANARY_RUNTIME_FEATURE_PROFILE", "").strip()
    training_feature_columns = (
        ["bps", "pps"] if runtime_feature_profile == "feature_stat_v1"
        else ["feature_noise", "feature_signal"]
    )
    manifest_feature_columns = (
        ["bps", "pps"] if runtime_feature_profile == "feature_stat_v1"
        else ["feature_signal", "feature_noise"]
    )
    tenant = str(frame.iloc[0]["tenant_id"])
    scope = ExtractionScope.create(
        tenant_id=tenant,
        feature_set_id="m08-canary-v1",
        window_from="2026-08-14T00:00:00Z",
        window_through="2026-08-14T04:00:00Z",
        source_watermark="2026-08-14T04:00:00Z",
        as_of="2026-08-14T04:05:00Z",
        max_rows=100,
    )
    splits = split_by_time_and_validate(
        frame,
        train_through="2026-08-14T01:00:00Z",
        validation_through="2026-08-14T02:00:00Z",
        holdout_attack_families=["unknown-holdout"],
    )
    if {name: len(split) for name, split in splits.items()} != {
        "train": 40, "validation": 16, "test": 16, "open_set": 8,
    }:
        raise ValueError("synthetic split counts drifted")
    graph_root.mkdir()
    nodes = pd.DataFrame([{
        "node_id": f"node-{index:03d}",
        "event_id": row.event_id,
        "graph_signal": float(row.label * 3 + index / 100),
        "graph_context": float((index * 5) % 7),
    } for index, row in enumerate(frame.itertuples(index=False))])
    edges = []
    split_ranges = ((0, 40), (40, 56), (56, 72), (72, 80))
    for start, through in split_ranges:
        for index in range(start, through):
            target = start if index + 1 == through else index + 1
            edges.append({
                "edge_id": f"edge-{index:03d}",
                "source_node_id": f"node-{index:03d}",
                "target_node_id": f"node-{target:03d}",
                "observed_at": "2026-08-14T03:55:00Z",
                "source_kind": "evidence" if index % 2 else "flow",
                "evidence_id": f"evidence-{index:03d}",
            })
    nodes_path, edges_path = graph_root / "nodes.parquet", graph_root / "edges.parquet"
    nodes.to_parquet(nodes_path, index=False, engine="pyarrow")
    pd.DataFrame(edges).to_parquet(edges_path, index=False, engine="pyarrow")
    graph = build_graph_snapshot_manifest(
        tenant_id=tenant,
        as_of="2026-08-14T04:00:00Z",
        source_watermarks={
            "clickhouse": "2026-08-14T03:58:00Z",
            "nebula": "2026-08-14T03:59:00Z",
        },
        nodes_path=nodes_path,
        edges_path=edges_path,
        feature_columns=["graph_signal", "graph_context"],
        source_evidence_sha256="d" * 64,
    )
    graph_manifest_path = graph_root / "graph-snapshot-manifest.json"
    write_json_exclusive(graph_manifest_path, graph)
    artifacts = {}
    for name, split in splits.items():
        path = root / f"{name}.parquet"
        split.to_parquet(path, index=False, engine="pyarrow")
        artifacts[name] = path
    isolation = root / "isolation-scope.json"
    isolation.write_text(frame[[
        "event_id", "entity_id", "site_id", "pcap_id", "attack_family",
        "graph_node_ids", "graph_edge_ids",
    ]].to_json(orient="records"), encoding="utf-8")
    metadata_path = root / "metadata.json"
    metadata = {
        "schema_version": 2,
        "governed_dataset": True,
        "tenant_id": tenant,
        "feature_set_id": "m08-canary-v1",
        "feature_columns": training_feature_columns,
        "total_features": 2,
    }
    write_json_exclusive(metadata_path, metadata)
    artifacts.update({
        "metadata": metadata_path,
        "isolation_scope": isolation,
        "graph_snapshot_manifest": graph_manifest_path,
    })
    manifest = build_dataset_manifest(
        scope=scope,
        source_frame=frame,
        splits=splits,
        feature_columns=manifest_feature_columns,
        artifact_paths=artifacts,
        label_schema_version="m08-canary-labels-v1",
        label_revision_sha256="a" * 64,
        graph_snapshot={
            "snapshot_id": graph["snapshot_id"],
            "manifest_sha256": sha256_file(graph_manifest_path),
        },
    )
    write_json_exclusive(root / "dataset-manifest.json", manifest)
    return manifest, graph


def validate_schema(contract_dir: Path, name: str, value: dict) -> None:
    with open(contract_dir / name, "r", encoding="utf-8") as handle:
        schema = json.load(handle)
    Draft202012Validator.check_schema(schema)
    Draft202012Validator(schema, format_checker=FormatChecker()).validate(value)


def main() -> None:
    run_id = required("M08_CANARY_RUN_ID")
    candidate = required("M08_CANARY_CANDIDATE_SHA256")
    if not re.fullmatch(r"[0-9a-f]{64}", candidate):
        raise ValueError("M08_CANARY_CANDIDATE_SHA256 must be lowercase SHA-256")
    contract_dir = Path(required("M08_CANARY_CONTRACT_DIR"))
    with tempfile.TemporaryDirectory(prefix="m08-governed-canary-") as directory:
        root = Path(directory)
        data_dir, graph_dir = root / "data", root / "graph"
        output_dir, gnn_dir = root / "model", root / "gnn"
        evaluation_dir, explanation_dir = root / "evaluation", root / "explanation"
        export_dir, key_dir, registration_dir = root / "export", root / "signing", root / "registration"
        data_dir.mkdir()
        dataset, graph = create_dataset(data_dir, graph_dir, run_id)
        validate_dataset_manifest_identity(dataset)
        validate_schema(contract_dir, "dataset-manifest.schema.json", dataset)
        validate_schema(contract_dir, "graph-snapshot-manifest.schema.json", graph)
        env = {
            "TRAIN_SEED": "42",
            "TRAIN_CPU_LIMIT": "1",
            "TRAIN_MEMORY_LIMIT": "1Gi",
            "TRAIN_GPU_LIMIT": "0",
            "TRAIN_RUN_ID": run_id,
            "TRAINER_IMAGE_DIGEST": "sha256:" + candidate,
            "TRAIN_MODEL_PARAMS_JSON": json.dumps({"n_estimators": 20, "max_depth": 3}),
        }
        os.environ.update(env)
        training = run_governed_training("xgboost", str(data_dir), str(output_dir))
        validate_schema(contract_dir, "training-run-manifest.schema.json", training)
        if training["dataset_id"] != dataset["dataset_id"] or training["dataset_sha256"] != dataset["dataset_sha256"]:
            raise ValueError("training run lost immutable dataset lineage")
        gnn_run_id = str(uuid.uuid5(uuid.UUID(run_id), "gnn-ablation-suite-v1"))
        os.environ.update({
            "GNN_TRAIN_SEED": "43",
            "GNN_HIDDEN_SIZE": "8",
            "GNN_EPOCHS": "80",
            "GNN_LEARNING_RATE": "0.03",
            "GNN_L2": "0.0001",
            "GNN_PATIENCE": "12",
            "GNN_SOURCE_ABLATION_KINDS": "evidence",
            "GNN_TRAIN_RUN_ID": gnn_run_id,
        })
        gnn_training = run_governed_gnn_training(str(data_dir), str(graph_dir), str(gnn_dir))
        validate_schema(contract_dir, "training-run-manifest.schema.json", gnn_training)
        if gnn_training["graph_snapshot"] != dataset["graph_snapshot"]:
            raise ValueError("GNN training lost immutable graph-snapshot lineage")
        os.environ.update({
            "EVALUATION_BOOTSTRAP_SEED": "314159",
            "EVALUATION_BOOTSTRAP_ROUNDS": "100",
            "EVALUATION_KNOWN_RETENTION_TARGET": "0.95",
            "EVALUATOR_IMAGE_DIGEST": "sha256:" + candidate,
            "GRAPH_ABLATION_PREDICTIONS_DIR": str(gnn_dir),
        })
        evaluation = run_governed_evaluation(str(data_dir), str(output_dir), str(evaluation_dir))
        validate_schema(contract_dir, "evaluation-manifest.schema.json", evaluation)
        if evaluation["training_run_sha256"] != training["run_sha256"]:
            raise ValueError("evaluation lost immutable training-run lineage")
        if evaluation["activation_authorized"] or evaluation["graph_ablations"]["state"] != "EVALUATED":
            raise ValueError("baseline canary overstated activation or graph-ablation state")
        if evaluation["graph_training_run"]["run_sha256"] != gnn_training["run_sha256"]:
            raise ValueError("evaluation lost immutable GNN-training lineage")
        os.environ.update({
            "EXPLAINER_IMAGE_DIGEST": "sha256:" + candidate,
            "EXPLANATION_MAX_EVENTS": "8",
        })
        explanation = run_governed_explanation(
            str(data_dir), str(output_dir), str(graph_dir), str(gnn_dir),
            str(evaluation_dir), str(explanation_dir),
        )
        validate_schema(contract_dir, "explanation-manifest.schema.json", explanation)
        if explanation["evaluation_sha256"] != evaluation["evaluation_sha256"]:
            raise ValueError("explanation lost immutable evaluation lineage")
        if explanation["activation_authorized"]:
            raise ValueError("explanation canary overstated activation state")
        from cryptography.hazmat.primitives import serialization
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
        key_dir.mkdir()
        seed = hashlib.sha256(f"{run_id}:{candidate}:m08-export-signing-v1".encode()).digest()
        signing_key = Ed25519PrivateKey.from_private_bytes(seed)
        private_key_path, public_key_path = key_dir / "private.pem", key_dir / "public.pem"
        private_key_path.write_bytes(signing_key.private_bytes(
            serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8,
            serialization.NoEncryption(),
        ))
        public_key_path.write_bytes(signing_key.public_key().public_bytes(
            serialization.Encoding.PEM, serialization.PublicFormat.SubjectPublicKeyInfo,
        ))
        os.environ.update({
            "MODEL_EXPORT_TENANT_ID": dataset["tenant_id"],
            "MODEL_EXPORT_MODEL_ID": "m08-governed-canary",
            "MODEL_EXPORT_VERSION": run_id,
            "MODEL_RUNTIME_VERSION": "1.0.0",
            "MODEL_SIGNING_PRIVATE_KEY_FILE": str(private_key_path),
            "MODEL_SIGNING_PUBLIC_KEY_FILE": str(public_key_path),
            "MODEL_SIGNING_KEY_ID": "ephemeral-canary/m08/" + run_id,
        })
        model_package = run_governed_model_export(
            str(data_dir), str(output_dir), str(graph_dir), str(gnn_dir),
            str(evaluation_dir), str(explanation_dir), str(export_dir),
        )
        validate_schema(contract_dir, "model-artifact-manifest.schema.json", model_package)
        verify_export_package(str(export_dir), str(public_key_path))
        if model_package["activation_authorized"]:
            raise ValueError("model export canary overstated activation state")
        prefix = f"synthetic-canary/{run_id}/{model_package['package_sha256']}"
        stored_objects = {
            name: {
                "uri": f"s3://traffic-models/{prefix}/{name}",
                "sha256": artifact["sha256"],
                "size_bytes": artifact["size_bytes"],
                "disposition": "SYNTHETIC_CANARY_ONLY",
            }
            for name, artifact in model_package["artifacts"].items()
        }
        manifest_path = export_dir / "model-artifact-manifest.json"
        stored_objects[manifest_path.name] = {
            "uri": f"s3://traffic-models/{prefix}/{manifest_path.name}",
            "sha256": sha256_file(manifest_path),
            "size_bytes": manifest_path.stat().st_size,
            "disposition": "SYNTHETIC_CANARY_ONLY",
        }
        write_json_exclusive(export_dir / "model-package-storage-receipt.json", {
            "schema_version": 1, "state": "stored",
            "package_id": model_package["package_id"],
            "package_sha256": model_package["package_sha256"],
            "bucket": "traffic-models", "object_prefix": prefix,
            "objects": stored_objects, "activation_authorized": False,
        })
        registration_payload = build_governed_registration_payload(
            str(export_dir), evaluation["metrics"], str(public_key_path),
        )
        registration_request_sha = canonical_json_sha256(registration_payload)
        registration_dir.mkdir()
        registration_path = registration_dir / "model-registration-receipt.json"
        registration_receipt = write_registration_receipt(
            str(registration_path), registration_payload, {
                "success": True,
                "data": {
                    "status": "registered", "revision": 1,
                    "model_id": str(uuid.uuid5(uuid.UUID(run_id), "registry-model-v1")),
                    "model_version": model_package["model_version"],
                    "registration_request_sha256": registration_request_sha,
                },
            },
        )
        validate_schema(contract_dir, "model-registration-receipt.schema.json", registration_receipt)
        if registration_receipt["activation_event_created"] or registration_receipt["activation_authorized"]:
            raise ValueError("metadata registration canary crossed the activation boundary")
        preserve_dir = os.getenv("M08_CANARY_PRESERVE_DIR", "").strip()
        if preserve_dir:
            preserved = Path(preserve_dir).resolve()
            if preserved.exists():
                raise FileExistsError(f"refusing to overwrite preserved canary directory: {preserved}")
            shutil.copytree(root, preserved)
            preserved_manifest = preserved / "export" / "model-artifact-manifest.json"
            shadow_event = {
                "event_id": str(uuid.uuid5(uuid.UUID(run_id), "shadow-load-v1")),
                "schema_version": 2,
                "tenant_id": model_package["tenant_id"],
                "model_id": model_package["model_id"],
                "model_name": "m08-governed-canary",
                "model_type": "governed-baseline-gnn",
                "version": model_package["model_version"],
                "artifact_uri": preserved_manifest.as_uri(),
                "artifact_manifest_uri": preserved_manifest.as_uri(),
                "artifact_manifest_sha256": sha256_file(preserved_manifest),
                "package_id": model_package["package_id"],
                "package_sha256": model_package["package_sha256"],
                "evaluation_sha256": model_package["evaluation_sha256"],
                "explanation_sha256": model_package["explanation_sha256"],
                "graph_snapshot_id": model_package["graph_snapshot"]["snapshot_id"],
                "graph_snapshot_sha256": model_package["graph_snapshot"]["manifest_sha256"],
                "aggregate_revision": 1,
                "compatibility": model_package["compatibility"],
                "action": "shadow-load",
                "metrics": {},
                "expected_applied_parallelism": 1,
                "timestamp": "2026-08-15T00:00:00Z",
            }
            write_json_exclusive(preserved / "shadow-load-event.json", shadow_event)
        print(json.dumps({
            "status": "PASS",
            "infrastructure": "kubernetes",
            "production_applied": False,
            "run_id": run_id,
            "candidate_sha256": candidate,
            "dataset_id": dataset["dataset_id"],
            "dataset_sha256": dataset["dataset_sha256"],
            "training_run_sha256": training["run_sha256"],
            "gnn_training_run_sha256": gnn_training["run_sha256"],
            "gnn_artifact_count": len(gnn_training["artifacts"]),
            "graph_snapshot_id": graph["snapshot_id"],
            "graph_snapshot_sha256": graph["graph_snapshot_sha256"],
            "evaluation_id": evaluation["evaluation_id"],
            "evaluation_sha256": evaluation["evaluation_sha256"],
            "evaluation_artifact_count": 2,
            "explanation_id": explanation["explanation_id"],
            "explanation_sha256": explanation["explanation_sha256"],
            "explanation_artifact_count": len(explanation["artifacts"]) + 1,
            "explanation_activation_authorized": explanation["activation_authorized"],
            "model_package_id": model_package["package_id"],
            "model_package_sha256": model_package["package_sha256"],
            "model_export_artifact_count": len(model_package["artifacts"]) + 1,
            "model_export_activation_authorized": model_package["activation_authorized"],
            "model_signing_key_id": model_package["signature"]["key_id"],
            "model_registration_receipt_id": registration_receipt["receipt_id"],
            "model_registration_receipt_sha256": sha256_file(registration_path),
            "model_registration_request_sha256": registration_receipt["registration_request_sha256"],
            "model_registration_status": registration_receipt["status"],
            "model_registration_revision": registration_receipt["revision"],
            "model_registration_activation_event_created": registration_receipt["activation_event_created"],
            "model_registration_storage_mode": "synthetic_canary_receipt",
            "activation_authorized": evaluation["activation_authorized"],
            "graph_ablation_state": evaluation["graph_ablations"]["state"],
            "known_attack_recall": evaluation["metrics"]["known_attack_recall"],
            "normal_false_positive_rate": evaluation["metrics"]["normal_false_positive_rate"],
            "unknown_recall": evaluation["metrics"]["unknown_recall"],
            "split_counts": dataset["splits"],
            "artifact_count": len(training["artifacts"]),
            "temporary_storage_removed_on_exit": True,
            "preserved_fixture": bool(preserve_dir),
        }, sort_keys=True))


if __name__ == "__main__":
    main()
