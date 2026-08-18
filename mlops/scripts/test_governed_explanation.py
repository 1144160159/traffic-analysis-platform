#!/usr/bin/env python3

from __future__ import annotations

import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, os.path.dirname(__file__))

from dataset_governance import canonical_json_sha256
from gnn_training import run_governed_gnn_training
from governed_evaluation import run_governed_evaluation
from governed_explanation import EXPLANATION_NAMESPACE, run_governed_explanation
import test_gnn_training as gnn_test_support
from train_model import run_governed_training


class GovernedExplanationTest(unittest.TestCase):
    def test_explanation_binds_baseline_gnn_graph_evaluation_and_limitations(self) -> None:
        with tempfile.TemporaryDirectory() as data, tempfile.TemporaryDirectory() as graph, \
                tempfile.TemporaryDirectory() as baseline, tempfile.TemporaryDirectory() as gnn, \
                tempfile.TemporaryDirectory() as evaluation, tempfile.TemporaryDirectory() as output:
            bundle = gnn_test_support.GovernedGNNTrainingTest().make_bundle(Path(data), Path(graph))
            baseline_env = {
                "TRAIN_SEED": "7", "TRAIN_CPU_LIMIT": "1", "TRAIN_MEMORY_LIMIT": "1Gi",
                "TRAIN_GPU_LIMIT": "0", "TRAIN_RUN_ID": "11111111-1111-4111-8111-111111111111",
                "TRAINER_IMAGE_DIGEST": "sha256:" + "a" * 64,
                "TRAIN_MODEL_PARAMS_JSON": json.dumps({"n_estimators": 5, "max_depth": 2}),
            }
            with mock.patch.dict(os.environ, baseline_env, clear=False):
                baseline_run = run_governed_training("xgboost", data, baseline)
            gnn_env = {
                "GNN_TRAIN_SEED": "23", "GNN_HIDDEN_SIZE": "4", "GNN_EPOCHS": "40",
                "GNN_LEARNING_RATE": "0.03", "GNN_L2": "0.0001", "GNN_PATIENCE": "8",
                "GNN_SOURCE_ABLATION_KINDS": "evidence",
                "GNN_TRAIN_RUN_ID": "44444444-4444-4444-8444-444444444444",
                "TRAINER_IMAGE_DIGEST": "sha256:" + "b" * 64,
                "TRAIN_CPU_LIMIT": "1", "TRAIN_MEMORY_LIMIT": "1Gi", "TRAIN_GPU_LIMIT": "0",
            }
            with mock.patch.dict(os.environ, gnn_env, clear=False):
                gnn_run = run_governed_gnn_training(data, graph, gnn)
            evaluation_env = {
                "EVALUATION_BOOTSTRAP_SEED": "19", "EVALUATION_BOOTSTRAP_ROUNDS": "100",
                "EVALUATION_KNOWN_RETENTION_TARGET": "0.95",
                "EVALUATOR_IMAGE_DIGEST": "sha256:" + "c" * 64,
                "GRAPH_ABLATION_PREDICTIONS_DIR": gnn,
            }
            with mock.patch.dict(os.environ, evaluation_env, clear=False):
                evaluation_run = run_governed_evaluation(data, baseline, evaluation)
            explanation_env = {
                "EXPLAINER_IMAGE_DIGEST": "sha256:" + "d" * 64,
                "EXPLANATION_MAX_EVENTS": "4",
            }
            with mock.patch.dict(os.environ, explanation_env, clear=False):
                explanation = run_governed_explanation(
                    data, baseline, graph, gnn, evaluation, output
                )
            self.assertEqual(explanation["dataset_id"], bundle["dataset"]["dataset_id"])
            self.assertEqual(explanation["baseline_training_run_sha256"], baseline_run["run_sha256"])
            self.assertEqual(explanation["gnn_training_run_sha256"], gnn_run["run_sha256"])
            self.assertEqual(explanation["evaluation_sha256"], evaluation_run["evaluation_sha256"])
            self.assertEqual(explanation["population"]["explained"], 4)
            self.assertFalse(explanation["activation_authorized"])
            self.assertGreaterEqual(len(explanation["limitations"]), 4)
            self.assertTrue((Path(output) / "event-explanations.parquet").is_file())
            self.assertTrue((Path(output) / "model-card.json").is_file())
            meaning = {
                key: value for key, value in explanation.items()
                if key not in {"schema_version", "explanation_id", "state", "explanation_sha256"}
            }
            expected_sha = canonical_json_sha256(meaning)
            self.assertEqual(explanation["explanation_sha256"], expected_sha)
            import uuid
            self.assertEqual(explanation["explanation_id"], str(uuid.uuid5(EXPLANATION_NAMESPACE, expected_sha)))
            with self.assertRaisesRegex(FileExistsError, "overwrite immutable explanation"):
                with mock.patch.dict(os.environ, explanation_env, clear=False):
                    run_governed_explanation(data, baseline, graph, gnn, evaluation, output)


if __name__ == "__main__":
    unittest.main(verbosity=2)
