#!/usr/bin/env python3

from __future__ import annotations

import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

sys.path.insert(0, os.path.dirname(__file__))

from governed_evaluation import run_governed_evaluation
from governed_explanation import run_governed_explanation
from gnn_training import run_governed_gnn_training
from model_artifact_governance import (
    publish_export_package_to_minio,
    run_governed_model_export,
    validate_runtime_compatibility,
    verify_export_package,
)
import test_gnn_training as gnn_test_support
from train_model import run_governed_training


class GovernedModelExportTest(unittest.TestCase):
    class FakeS3Error(Exception):
        def __init__(self, code: str):
            super().__init__(code)
            self.code = code

    class FakeMinio:
        def __init__(self) -> None:
            self.objects: dict[tuple[str, str], dict] = {}

        def stat_object(self, bucket: str, name: str) -> SimpleNamespace:
            value = self.objects.get((bucket, name))
            if value is None:
                raise GovernedModelExportTest.FakeS3Error("NoSuchKey")
            return SimpleNamespace(size=len(value["body"]), metadata=value["metadata"])

        def _execute(self, method: str, bucket: str, name: str, *, body: bytes, headers: dict) -> None:
            if method != "PUT" or headers.get("If-None-Match") != "*":
                raise AssertionError("model package upload lost conditional create semantics")
            if (bucket, name) in self.objects:
                raise GovernedModelExportTest.FakeS3Error("PreconditionFailed")
            self.objects[(bucket, name)] = {
                "body": body,
                "metadata": {"x-amz-meta-sha256": headers["X-Amz-Meta-Sha256"]},
            }

    def _keys(self, root: Path) -> tuple[Path, Path]:
        from cryptography.hazmat.primitives import serialization
        from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

        private = Ed25519PrivateKey.generate()
        private_path, public_path = root / "private.pem", root / "public.pem"
        private_path.write_bytes(private.private_bytes(
            serialization.Encoding.PEM,
            serialization.PrivateFormat.PKCS8,
            serialization.NoEncryption(),
        ))
        public_path.write_bytes(private.public_key().public_bytes(
            serialization.Encoding.PEM,
            serialization.PublicFormat.SubjectPublicKeyInfo,
        ))
        return private_path, public_path

    def _pipeline(
        self, data: str, graph: str, baseline: str, gnn: str, evaluation: str, explanation: str,
    ) -> None:
        gnn_test_support.GovernedGNNTrainingTest().make_bundle(Path(data), Path(graph))
        with mock.patch.dict(os.environ, {
            "TRAIN_SEED": "7", "TRAIN_CPU_LIMIT": "1", "TRAIN_MEMORY_LIMIT": "1Gi",
            "TRAIN_GPU_LIMIT": "0", "TRAIN_RUN_ID": "11111111-1111-4111-8111-111111111111",
            "TRAINER_IMAGE_DIGEST": "sha256:" + "a" * 64,
            "TRAIN_MODEL_PARAMS_JSON": json.dumps({"n_estimators": 5, "max_depth": 2}),
        }, clear=False):
            run_governed_training("xgboost", data, baseline)
        with mock.patch.dict(os.environ, {
            "GNN_TRAIN_SEED": "23", "GNN_HIDDEN_SIZE": "4", "GNN_EPOCHS": "40",
            "GNN_LEARNING_RATE": "0.03", "GNN_L2": "0.0001", "GNN_PATIENCE": "8",
            "GNN_SOURCE_ABLATION_KINDS": "evidence",
            "GNN_TRAIN_RUN_ID": "44444444-4444-4444-8444-444444444444",
            "TRAINER_IMAGE_DIGEST": "sha256:" + "b" * 64,
            "TRAIN_CPU_LIMIT": "1", "TRAIN_MEMORY_LIMIT": "1Gi", "TRAIN_GPU_LIMIT": "0",
        }, clear=False):
            run_governed_gnn_training(data, graph, gnn)
        with mock.patch.dict(os.environ, {
            "EVALUATION_BOOTSTRAP_SEED": "19", "EVALUATION_BOOTSTRAP_ROUNDS": "100",
            "EVALUATION_KNOWN_RETENTION_TARGET": "0.95",
            "EVALUATOR_IMAGE_DIGEST": "sha256:" + "c" * 64,
            "GRAPH_ABLATION_PREDICTIONS_DIR": gnn,
        }, clear=False):
            run_governed_evaluation(data, baseline, evaluation)
        with mock.patch.dict(os.environ, {
            "EXPLAINER_IMAGE_DIGEST": "sha256:" + "d" * 64,
            "EXPLANATION_MAX_EVENTS": "4",
        }, clear=False):
            run_governed_explanation(data, baseline, graph, gnn, evaluation, explanation)

    def test_signed_package_rejects_hash_replacement_and_graph_incompatibility(self) -> None:
        with tempfile.TemporaryDirectory() as data, tempfile.TemporaryDirectory() as graph, \
                tempfile.TemporaryDirectory() as baseline, tempfile.TemporaryDirectory() as gnn, \
                tempfile.TemporaryDirectory() as evaluation, tempfile.TemporaryDirectory() as explanation, \
                tempfile.TemporaryDirectory() as output, tempfile.TemporaryDirectory() as keys:
            self._pipeline(data, graph, baseline, gnn, evaluation, explanation)
            private_path, public_path = self._keys(Path(keys))
            env = {
                "MODEL_EXPORT_TENANT_ID": "tenant-a",
                "MODEL_EXPORT_MODEL_ID": "behavior-attack",
                "MODEL_EXPORT_VERSION": "candidate-v1",
                "MODEL_RUNTIME_VERSION": "1.0.0",
                "MODEL_SIGNING_PRIVATE_KEY_FILE": str(private_path),
                "MODEL_SIGNING_PUBLIC_KEY_FILE": str(public_path),
                "MODEL_SIGNING_KEY_ID": "test/model-signing-key/v1",
            }

            def fake_onnx(_model: Path, _test: Path, _features: list[str], target: Path) -> dict:
                target.write_bytes(b"deterministic-test-onnx")
                return {"validated_rows": 12, "max_absolute_probability_error": 0.0, "opset": 15}

            with mock.patch.dict(os.environ, env, clear=False), \
                    mock.patch("model_artifact_governance.export_baseline_onnx", side_effect=fake_onnx):
                manifest = run_governed_model_export(
                    data, baseline, graph, gnn, evaluation, explanation, output,
                )
            verified = verify_export_package(output, str(public_path))
            self.assertEqual(verified["package_sha256"], manifest["package_sha256"])
            self.assertFalse(verified["activation_authorized"])
            self.assertEqual(len(verified["artifacts"]), 4)
            schema = json.loads(Path(
                __file__
            ).resolve().parents[2].joinpath("contracts/mlops/model-artifact-manifest.schema.json").read_text())
            self.assertEqual(schema["$schema"], "https://json-schema.org/draft/2020-12/schema")
            self.assertEqual(set(schema["required"]) - set(verified), set())
            supported = {
                "runtime_contract": "traffic.behavior.inference.v1",
                "runtime_version": "1.0.0",
                "feature_set_id": "feature-v1",
                "feature_schema_version": 2,
                "graph_schema_version": 1,
                "gnn_formats": ["numpy_npz_v1"],
                "graph_edge_fields": list(verified["compatibility"]["gnn"]["edge_fields"]),
            }
            validate_runtime_compatibility(verified, supported)
            incompatible = dict(supported, graph_schema_version=2)
            with self.assertRaisesRegex(ValueError, "graph_schema_version"):
                validate_runtime_compatibility(verified, incompatible)
            object_store = self.FakeMinio()
            receipt = publish_export_package_to_minio(
                output, str(public_path), object_store, "traffic-models",
            )
            self.assertEqual(receipt["state"], "stored")
            self.assertEqual(len(receipt["objects"]), 5)
            first = sorted(object_store.objects)[0]
            object_store.objects[first]["metadata"]["x-amz-meta-sha256"] = "0" * 64
            with self.assertRaisesRegex(ValueError, "object replacement"):
                publish_export_package_to_minio(
                    output, str(public_path), object_store, "traffic-models",
                )
            with open(Path(output) / "gnn-full-model.npz", "ab") as handle:
                handle.write(b"replacement")
            with self.assertRaisesRegex(ValueError, "artifact hash or size"):
                verify_export_package(output, str(public_path))
            with mock.patch.dict(os.environ, env, clear=False), \
                    mock.patch("model_artifact_governance.export_baseline_onnx", side_effect=fake_onnx):
                with self.assertRaisesRegex(FileExistsError, "output directory must be empty"):
                    run_governed_model_export(
                        data, baseline, graph, gnn, evaluation, explanation, output,
                    )


if __name__ == "__main__":
    unittest.main(verbosity=2)
