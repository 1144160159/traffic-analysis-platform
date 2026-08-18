#!/usr/bin/env python3
"""Immutable baseline/GNN export, compatibility, signing and object-store guards."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import re
import shutil
import uuid
from pathlib import Path
from typing import Any, Mapping
from urllib.parse import quote

import numpy as np
import pandas as pd
import xgboost as xgb

from dataset_governance import (
    canonical_json_sha256,
    sha256_file,
    validate_dataset_manifest_identity,
    write_json_exclusive,
)
from governed_evaluation import validate_evaluation_manifest_identity
from governed_explanation import validate_explanation_manifest_identity


PACKAGE_NAMESPACE = uuid.UUID("612ba5a8-cdb8-5cf2-9de8-273eab2c24d2")
RUNTIME_CONTRACT = "traffic.behavior.inference.v1"
EDGE_FIELDS = (
    "edge_id", "source_node_id", "target_node_id", "source_kind", "evidence_id", "observed_at",
)


def canonical_json_bytes(value: Any) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")


def required_env(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise ValueError(f"{name} is required for governed model export")
    return value


from governance_common import load_json_object as _load_json  # noqa: F401  (DRY 收敛)


def _validate_training_manifest(manifest: Mapping[str, Any], algorithm: str) -> None:
    meaning = {
        key: value for key, value in manifest.items()
        if key not in {"schema_version", "state", "run_sha256"}
    }
    if manifest.get("schema_version") != 1 or manifest.get("state") != "trained" or \
            manifest.get("algorithm") != algorithm or \
            canonical_json_sha256(meaning) != manifest.get("run_sha256"):
        raise ValueError(f"{algorithm} training manifest version, state or identity is invalid")


def _write_bytes_exclusive(path: Path, payload: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        raise FileExistsError(f"refusing to overwrite immutable export artifact: {path}")
    temporary = path.with_name(f".{path.name}.{uuid.uuid4().hex}.tmp")
    try:
        with temporary.open("xb") as handle:
            handle.write(payload)
            handle.flush()
            os.fsync(handle.fileno())
        os.link(temporary, path)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass


def _copy_exclusive(source: Path, target: Path) -> None:
    if not source.is_file():
        raise FileNotFoundError(f"model source artifact not found: {source}")
    target.parent.mkdir(parents=True, exist_ok=True)
    if target.exists():
        raise FileExistsError(f"refusing to overwrite immutable export artifact: {target}")
    temporary = target.with_name(f".{target.name}.{uuid.uuid4().hex}.tmp")
    try:
        with source.open("rb") as reader, temporary.open("xb") as writer:
            shutil.copyfileobj(reader, writer, length=1024 * 1024)
            writer.flush()
            os.fsync(writer.fileno())
        os.link(temporary, target)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass


def _positive_probability(outputs: list[Any], row_count: int) -> np.ndarray:
    for value in outputs:
        if isinstance(value, list) and len(value) == row_count and all(isinstance(item, dict) for item in value):
            return np.asarray([float(item.get(1, item.get("1"))) for item in value], dtype=float)
        array = np.asarray(value)
        if array.ndim == 2 and array.shape == (row_count, 2) and np.issubdtype(array.dtype, np.number):
            return array[:, 1].astype(float)
        if array.ndim == 1 and array.shape == (row_count,) and np.issubdtype(array.dtype, np.floating):
            if np.isfinite(array).all() and ((array >= 0) & (array <= 1)).all():
                return array.astype(float)
    raise ValueError("ONNX output does not expose an aligned binary attack probability")


def export_baseline_onnx(
    model_path: Path, test_path: Path, feature_columns: list[str], output_path: Path,
) -> dict[str, Any]:
    """Convert the immutable XGBoost baseline and compare runtime scores."""
    try:
        import onnx
        import onnxruntime as ort
        from onnxmltools import convert_xgboost
        from onnxmltools.convert.common.data_types import FloatTensorType
    except ImportError as exc:
        raise RuntimeError("governed ONNX dependencies are required and must be image-pinned") from exc
    model = xgb.XGBClassifier()
    model.load_model(model_path)
    if model.get_booster().num_features() != len(feature_columns):
        raise ValueError("baseline feature count does not match immutable feature metadata")
    # onnxmltools consumes XGBoost's positional f0..fN tree split IDs.  The
    # governed feature order remains explicit in compatibility metadata and is
    # parity-checked below, so removing display names cannot reorder inputs.
    model.get_booster().feature_names = None
    model.get_booster().feature_types = None
    converted = convert_xgboost(
        model,
        initial_types=[("float_input", FloatTensorType([None, len(feature_columns)]))],
        target_opset=15,
    )
    onnx.checker.check_model(converted)
    _write_bytes_exclusive(output_path, converted.SerializeToString())
    test = pd.read_parquet(test_path, engine="pyarrow").sort_values("event_id", kind="mergesort")
    sample = test[feature_columns].head(32).to_numpy(dtype=np.float32, copy=True)
    expected = np.asarray(model.predict_proba(sample)[:, 1], dtype=float)
    session = ort.InferenceSession(str(output_path), providers=["CPUExecutionProvider"])
    if len(session.get_inputs()) != 1 or session.get_inputs()[0].name != "float_input":
        raise ValueError("ONNX input contract drifted from float_input")
    actual = _positive_probability(session.run(None, {"float_input": sample}), len(sample))
    max_error = float(np.max(np.abs(actual - expected)))
    if max_error > 1e-5:
        raise ValueError(f"ONNX/XGBoost probability parity exceeded tolerance: {max_error}")
    return {"validated_rows": len(sample), "max_absolute_probability_error": max_error, "opset": 15}


def _artifact_entry(path: Path, media_type: str, role: str) -> dict[str, Any]:
    size = path.stat().st_size
    if size <= 0:
        raise ValueError(f"export artifact is empty: {path.name}")
    return {"sha256": sha256_file(path), "size_bytes": size, "media_type": media_type, "role": role}


def _package_meaning(manifest: Mapping[str, Any]) -> dict[str, Any]:
    return {
        key: value for key, value in manifest.items()
        if key not in {"schema_version", "package_id", "state", "package_sha256", "signature"}
    }


def _signed_payload(manifest: Mapping[str, Any]) -> dict[str, Any]:
    return {key: value for key, value in manifest.items() if key != "signature"}


def _load_private_key(path: Path) -> Any:
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

    key = serialization.load_pem_private_key(path.read_bytes(), password=None)
    if not isinstance(key, Ed25519PrivateKey):
        raise ValueError("MODEL_SIGNING_PRIVATE_KEY_FILE must contain an Ed25519 PEM private key")
    return key


def _load_public_key(path: Path) -> Any:
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey

    key = serialization.load_pem_public_key(path.read_bytes())
    if not isinstance(key, Ed25519PublicKey):
        raise ValueError("trusted model signing key must be an Ed25519 PEM public key")
    return key


def _public_key_der(key: Any) -> bytes:
    from cryptography.hazmat.primitives import serialization
    return key.public_bytes(serialization.Encoding.DER, serialization.PublicFormat.SubjectPublicKeyInfo)


def validate_runtime_compatibility(manifest: Mapping[str, Any], supported: Mapping[str, Any]) -> None:
    compatibility = manifest.get("compatibility", {})
    exact = ("runtime_contract", "runtime_version", "feature_set_id", "feature_schema_version", "graph_schema_version")
    for field in exact:
        if compatibility.get(field) != supported.get(field):
            raise ValueError(f"model package is incompatible with runtime field {field}")
    gnn = compatibility.get("gnn", {})
    if gnn.get("format") not in set(supported.get("gnn_formats", [])):
        raise ValueError("model package GNN format is unsupported")
    if not set(gnn.get("edge_fields", [])).issubset(set(supported.get("graph_edge_fields", []))):
        raise ValueError("model package graph edge schema is unsupported")


def verify_export_package(output_dir: str, public_key_path: str) -> dict[str, Any]:
    root = Path(output_dir).resolve()
    manifest = _load_json(root / "model-artifact-manifest.json", "model artifact manifest")
    meaning_sha = canonical_json_sha256(_package_meaning(manifest))
    if meaning_sha != manifest.get("package_sha256"):
        raise ValueError("model package meaning hash mismatch")
    if str(uuid.uuid5(PACKAGE_NAMESPACE, meaning_sha)) != manifest.get("package_id"):
        raise ValueError("model package ID does not match package hash")
    for name, expected in manifest.get("artifacts", {}).items():
        if Path(name).name != name:
            raise ValueError("model package artifact name must be a basename")
        path = root / name
        if not path.is_file() or path.stat().st_size != expected.get("size_bytes") or \
                sha256_file(path) != expected.get("sha256"):
            raise ValueError(f"model package artifact hash or size mismatch: {name}")
    signature = manifest.get("signature", {})
    payload = canonical_json_bytes(_signed_payload(manifest))
    payload_sha = hashlib.sha256(payload).hexdigest()
    if signature.get("algorithm") != "ed25519" or signature.get("signed_payload_sha256") != payload_sha:
        raise ValueError("model package signed payload identity mismatch")
    public_key = _load_public_key(Path(public_key_path).resolve())
    if hashlib.sha256(_public_key_der(public_key)).hexdigest() != signature.get("public_key_sha256"):
        raise ValueError("model package signing public key fingerprint mismatch")
    try:
        public_key.verify(base64.b64decode(signature.get("value_base64", ""), validate=True), payload)
    except Exception as exc:
        raise ValueError("model package Ed25519 signature verification failed") from exc
    if manifest.get("activation_authorized") is not False:
        raise ValueError("model export package must never authorize activation")
    return manifest


def _object_metadata_sha256(stat: Any) -> str:
    metadata = getattr(stat, "metadata", {}) or {}
    lowered = {str(key).lower(): str(value) for key, value in metadata.items()}
    return lowered.get("x-amz-meta-sha256", lowered.get("sha256", ""))


def _stat_or_none(client: Any, bucket: str, object_name: str) -> Any | None:
    try:
        return client.stat_object(bucket, object_name)
    except Exception as exc:
        if getattr(exc, "code", "") in {"NoSuchKey", "NoSuchObject", "NoSuchBucket"}:
            if getattr(exc, "code", "") == "NoSuchBucket":
                raise RuntimeError(f"model package bucket is absent: {bucket}") from exc
            return None
        raise


def _require_stored_identity(stat: Any, expected_sha256: str, expected_size: int, object_name: str) -> None:
    if int(getattr(stat, "size", -1)) != expected_size or \
            _object_metadata_sha256(stat) != expected_sha256:
        raise ValueError(f"immutable model object replacement detected: {object_name}")


def _conditional_put_small_object(
    client: Any, bucket: str, object_name: str, payload: bytes, media_type: str, expected_sha256: str,
) -> str:
    existing = _stat_or_none(client, bucket, object_name)
    if existing is not None:
        _require_stored_identity(existing, expected_sha256, len(payload), object_name)
        return "REUSED_IDENTICAL"
    headers = {
        "Content-Length": str(len(payload)),
        "Content-Type": media_type,
        "X-Amz-Meta-Sha256": expected_sha256,
        "If-None-Match": "*",
    }
    disposition = "WRITTEN"
    try:
        _conditional_put_object(client, bucket, object_name, payload, headers)
    except Exception as exc:
        if getattr(exc, "code", "") not in {"PreconditionFailed", "ConditionalRequestConflict"}:
            raise
        disposition = "REUSED_IDENTICAL"
    stored = _stat_or_none(client, bucket, object_name)
    if stored is None:
        raise RuntimeError(f"conditional model object write produced no durable object: {object_name}")
    _require_stored_identity(stored, expected_sha256, len(payload), object_name)
    return disposition


def _conditional_put_object(
    client: Any, bucket: str, object_name: str, payload: bytes, headers: dict[str, str],
) -> None:
    """Atomic If-None-Match object write.

    代码审查 H40 收敛项：minio-py 7.2.x 的公开 ``put_object`` 不接受条件请求头，
    而私有 ``client._execute("PUT", ...)`` 是当前受支持版本下唯一能同时保留
    MinIO 签名认证与 If-None-Match 原子的路径。此处将其封装为独立函数并做
    版本守卫：任何未在受控版本范围（7.2.x）内的 minio 版本都会显式拒绝，防止
    私有 API 漂移静默破坏原子性。升级 minio-py 至公开支持条件写/etag 的版本后，
    应改走公开 API 并删除本封装。
    """
    try:
        from importlib import metadata as _metadata

        _minio_version = _metadata.version("minio")
    except Exception:
        # minio 未安装：真实路径（build_minio_client）会在到达这里之前失败；
        # 单元测试使用 FakeMinio 时跳过版本守卫。
        _minio_version = None
    if _minio_version is not None and not str(_minio_version).startswith("7.2."):
        raise RuntimeError(
            f"minio-py version {_minio_version} is outside the pinned 7.2.x range "
            "supported by the conditional-write shim; upgrade path requires a "
            "public If-None-Match/etag API or a controlled review"
        )
    client._execute("PUT", bucket, object_name, body=payload, headers=headers)


def publish_export_package_to_minio(
    output_dir: str, public_key_path: str, client: Any, bucket: str,
) -> dict[str, Any]:
    """Publish a verified package under a content-addressed, non-overwriting prefix."""
    root = Path(output_dir).resolve()
    manifest = verify_export_package(str(root), public_key_path)
    encoded = [
        quote(str(manifest[field]), safe="")
        for field in ("tenant_id", "model_id", "model_version", "package_sha256")
    ]
    prefix = f"tenants/{encoded[0]}/models/{encoded[1]}/versions/{encoded[2]}/{encoded[3]}"
    objects: dict[str, dict[str, Any]] = {}
    sources = {
        name: (root / name, value["media_type"], value["sha256"])
        for name, value in manifest["artifacts"].items()
    }
    manifest_path = root / "model-artifact-manifest.json"
    sources[manifest_path.name] = (manifest_path, "application/json", sha256_file(manifest_path))
    for name, (path, media_type, expected_sha) in sorted(sources.items()):
        payload = path.read_bytes()
        if hashlib.sha256(payload).hexdigest() != expected_sha:
            raise ValueError(f"model package artifact changed before object write: {name}")
        object_name = f"{prefix}/{name}"
        disposition = _conditional_put_small_object(
            client, bucket, object_name, payload, media_type, expected_sha,
        )
        objects[name] = {
            "uri": f"s3://{bucket}/{object_name}", "sha256": expected_sha,
            "size_bytes": len(payload), "disposition": disposition,
        }
    receipt = {
        "schema_version": 1,
        "state": "stored",
        "package_id": manifest["package_id"],
        "package_sha256": manifest["package_sha256"],
        "bucket": bucket,
        "object_prefix": prefix,
        "objects": objects,
        "activation_authorized": False,
    }
    write_json_exclusive(root / "model-package-storage-receipt.json", receipt)
    return receipt


def publish_export_package_from_environment(output_dir: str) -> dict[str, Any] | None:
    enabled = os.getenv("MLOPS_MODEL_PACKAGE_MINIO_WRITE_ENABLED", "false").strip().lower()
    if enabled not in {"true", "false"}:
        raise ValueError("MLOPS_MODEL_PACKAGE_MINIO_WRITE_ENABLED must be explicitly true or false")
    if enabled != "true":
        return None
    from register_model import build_minio_client, load_minio_config, require_model_bucket

    config = load_minio_config()
    client = build_minio_client(config)
    require_model_bucket(client, config.bucket)
    return publish_export_package_to_minio(
        output_dir, required_env("MODEL_SIGNING_PUBLIC_KEY_FILE"), client, config.bucket,
    )


def run_governed_model_export(
    data_dir: str,
    baseline_model_dir: str,
    graph_dir: str,
    gnn_dir: str,
    evaluation_dir: str,
    explanation_dir: str,
    output_dir: str,
) -> dict[str, Any]:
    roots = {
        name: Path(value).resolve() for name, value in {
            "data": data_dir, "baseline": baseline_model_dir, "graph": graph_dir, "gnn": gnn_dir,
            "evaluation": evaluation_dir, "explanation": explanation_dir, "output": output_dir,
        }.items()
    }
    dataset = _load_json(roots["data"] / "dataset-manifest.json", "dataset manifest")
    validate_dataset_manifest_identity(dataset)
    metadata = _load_json(roots["data"] / "metadata.json", "feature metadata")
    baseline = _load_json(roots["baseline"] / "training-run-manifest.json", "baseline training manifest")
    gnn = _load_json(roots["gnn"] / "gnn-training-run-manifest.json", "GNN training manifest")
    evaluation = _load_json(roots["evaluation"] / "evaluation-manifest.json", "evaluation manifest")
    explanation = _load_json(roots["explanation"] / "explanation-manifest.json", "explanation manifest")
    graph = _load_json(roots["graph"] / "graph-snapshot-manifest.json", "graph snapshot manifest")
    _validate_training_manifest(baseline, "xgboost")
    _validate_training_manifest(gnn, "gnn")
    validate_evaluation_manifest_identity(evaluation)
    validate_explanation_manifest_identity(explanation)
    if baseline.get("dataset_sha256") != dataset["dataset_sha256"] or \
            gnn.get("dataset_sha256") != dataset["dataset_sha256"] or \
            evaluation.get("dataset_sha256") != dataset["dataset_sha256"] or \
            explanation.get("dataset_sha256") != dataset["dataset_sha256"]:
        raise ValueError("model export inputs do not bind the same immutable dataset")
    if evaluation.get("training_run_sha256") != baseline["run_sha256"] or \
            evaluation.get("graph_training_run", {}).get("run_sha256") != gnn["run_sha256"] or \
            explanation.get("evaluation_sha256") != evaluation["evaluation_sha256"]:
        raise ValueError("model export evaluation/explanation lineage is inconsistent")
    graph_path = roots["graph"] / "graph-snapshot-manifest.json"
    if dataset.get("graph_snapshot", {}).get("manifest_sha256") != sha256_file(graph_path) or \
            dataset.get("graph_snapshot", {}).get("snapshot_id") != graph.get("snapshot_id"):
        raise ValueError("model export graph snapshot artifact does not match dataset lineage")
    model_path = roots["baseline"] / "model.json"
    gnn_path = roots["gnn"] / "gnn_full-model.npz"
    if baseline.get("artifacts", {}).get("model") != sha256_file(model_path) or \
            gnn.get("artifacts", {}).get("gnn_full_model") != sha256_file(gnn_path):
        raise ValueError("model export source model hash mismatch")

    tenant_id = required_env("MODEL_EXPORT_TENANT_ID")
    if tenant_id != dataset.get("tenant_id"):
        raise ValueError("MODEL_EXPORT_TENANT_ID does not match dataset tenant")
    model_id, model_version = required_env("MODEL_EXPORT_MODEL_ID"), required_env("MODEL_EXPORT_VERSION")
    runtime_version = required_env("MODEL_RUNTIME_VERSION")
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", runtime_version):
        raise ValueError("MODEL_RUNTIME_VERSION must be semantic major.minor.patch")
    feature_columns = metadata.get("feature_columns", [])
    if not isinstance(feature_columns, list) or not feature_columns:
        raise ValueError("feature metadata must contain a non-empty ordered feature_columns list")
    graph_schema_version = int(graph.get("schema_version", 0))
    if graph_schema_version < 1:
        raise ValueError("graph snapshot schema version is invalid")
    output = roots["output"]
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        raise FileExistsError("governed model export output directory must be empty")

    onnx_path, exported_gnn = output / "baseline-model.onnx", output / "gnn-full-model.npz"
    parity = export_baseline_onnx(model_path, roots["data"] / "test.parquet", feature_columns, onnx_path)
    _copy_exclusive(gnn_path, exported_gnn)
    inference_schema = {
        "schema_version": 1,
        "runtime_contract": RUNTIME_CONTRACT,
        "graph_snapshot_schema_version": graph_schema_version,
        "node_id_field": "node_id",
        "event_id_field": "event_id",
        "node_feature_columns": list(graph.get("node_schema", {}).get("feature_columns", [])),
        "required_edge_fields": list(EDGE_FIELDS),
        "normalization": "symmetric_adjacency_with_self_loops",
        "self_loops_required": True,
        "directed_input_edges": True,
    }
    if not inference_schema["node_feature_columns"]:
        raise ValueError("graph snapshot does not declare node feature columns")
    inference_schema_path = output / "inference-graph-schema.json"
    write_json_exclusive(inference_schema_path, inference_schema)
    compatibility = {
        "runtime_contract": RUNTIME_CONTRACT,
        "runtime_version": runtime_version,
        "feature_set_id": dataset["feature_set_id"],
        "feature_schema_version": int(metadata.get("schema_version", 0)),
        "graph_schema_version": graph_schema_version,
        "baseline": {
            "format": "onnx", "input_name": "float_input", "input_dtype": "float32",
            "input_shape": [None, len(feature_columns)], "feature_columns": feature_columns,
            "feature_columns_sha256": canonical_json_sha256(feature_columns),
            "output": "binary_attack_probability",
        },
        "gnn": {
            "format": "numpy_npz_v1", "architecture": "two_layer_sparse_gcn",
            "node_feature_columns": inference_schema["node_feature_columns"],
            "edge_fields": list(EDGE_FIELDS),
            "normalization": "symmetric_adjacency_with_self_loops",
            "output": "binary_attack_probability_per_node",
        },
    }
    compatibility_path = output / "compatibility-metadata.json"
    write_json_exclusive(compatibility_path, {
        **compatibility,
        "onnx_parity": parity,
        "activation_authorized": False,
    })
    artifacts = {
        onnx_path.name: _artifact_entry(onnx_path, "application/onnx", "baseline_model"),
        exported_gnn.name: _artifact_entry(exported_gnn, "application/x-numpy-npz", "gnn_model"),
        inference_schema_path.name: _artifact_entry(inference_schema_path, "application/schema+json", "inference_graph_schema"),
        compatibility_path.name: _artifact_entry(compatibility_path, "application/json", "compatibility_metadata"),
    }
    meaning = {
        "tenant_id": tenant_id, "model_id": model_id, "model_version": model_version,
        "dataset_id": dataset["dataset_id"], "dataset_sha256": dataset["dataset_sha256"],
        "baseline_training_run_sha256": baseline["run_sha256"],
        "gnn_training_run_sha256": gnn["run_sha256"],
        "evaluation_sha256": evaluation["evaluation_sha256"],
        "explanation_sha256": explanation["explanation_sha256"],
        "graph_snapshot": {
            "snapshot_id": graph["snapshot_id"],
            "manifest_sha256": sha256_file(graph_path),
            "schema_version": graph_schema_version,
        },
        "compatibility": compatibility, "artifacts": artifacts, "activation_authorized": False,
    }
    package_sha = canonical_json_sha256(meaning)
    manifest: dict[str, Any] = {
        "schema_version": 1,
        "package_id": str(uuid.uuid5(PACKAGE_NAMESPACE, package_sha)),
        "state": "exported",
        **meaning,
        "package_sha256": package_sha,
    }
    private_key = _load_private_key(Path(required_env("MODEL_SIGNING_PRIVATE_KEY_FILE")).resolve())
    payload = canonical_json_bytes(manifest)
    public_key = private_key.public_key()
    manifest["signature"] = {
        "algorithm": "ed25519",
        "key_id": required_env("MODEL_SIGNING_KEY_ID"),
        "public_key_sha256": hashlib.sha256(_public_key_der(public_key)).hexdigest(),
        "signed_payload_sha256": hashlib.sha256(payload).hexdigest(),
        "value_base64": base64.b64encode(private_key.sign(payload)).decode("ascii"),
    }
    manifest_path = output / "model-artifact-manifest.json"
    write_json_exclusive(manifest_path, manifest)
    verify_export_package(str(output), required_env("MODEL_SIGNING_PUBLIC_KEY_FILE"))
    return manifest
