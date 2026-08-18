#!/usr/bin/env python3

from __future__ import annotations

import json
import os
import sys
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path
from unittest import mock

import pandas as pd

sys.path.insert(0, os.path.dirname(__file__))

from dataset_governance import ExtractionScope, build_dataset_manifest, sha256_file, write_json_exclusive
from governed_evaluation import (
    METHOD_CONTRACT_SHA256,
    compare_graph_ablations,
    run_governed_evaluation,
    select_abstain_threshold,
)
from train_model import run_governed_training


class GovernedEvaluationTest(unittest.TestCase):
    def make_dataset(self, root: Path, *, family_leak: bool = False) -> dict:
        scope = ExtractionScope.create(
            tenant_id="tenant-a", feature_set_id="feature-v1",
            window_from="2026-08-14T00:00:00Z", window_through="2026-08-14T05:00:00Z",
            source_watermark="2026-08-14T05:00:00Z", as_of="2026-08-14T05:05:00Z",
            max_rows=100,
        )
        split_names = ["train"] * 12 + ["validation"] * 8 + ["test"] * 12 + ["open_set"] * 8
        rows = []
        for index, split_name in enumerate(split_names):
            timestamp = int(datetime(
                2026, 8, 14, index // 10, index % 10, tzinfo=timezone.utc,
            ).timestamp())
            is_open = split_name == "open_set"
            label = 1 if is_open else index % 2
            family = "unknown-holdout" if is_open else "known-family"
            if family_leak and index == 0:
                family = "unknown-holdout"
            rows.append({
                "tenant_id": "tenant-a", "feature_set_id": "feature-v1",
                "event_id": f"event-{index:03d}", "object_id": f"object-{index:03d}",
                "community_id": f"community-{index:03d}", "entity_id": f"entity-{index:03d}",
                "site_id": f"site-{index:03d}", "pcap_id": f"pcap-{index:03d}",
                "attack_family": family,
                "graph_node_ids": json.dumps([f"node-{index:03d}"]),
                "graph_edge_ids": json.dumps([f"edge-{index:03d}"]),
                "ts": timestamp, "ingest_ts": timestamp + 1, "label": label,
                "feature_a": float(label * 10 + index / 100),
                "feature_b": float((index * 7) % 5), "_split": split_name,
            })
        source = pd.DataFrame(rows).drop(columns=["_split"])
        splits = {
            name: pd.DataFrame([row for row in rows if row["_split"] == name]).drop(columns=["_split"])
            for name in ("train", "validation", "test", "open_set")
        }
        artifacts = {}
        for name, frame in splits.items():
            path = root / f"{name}.parquet"
            frame.to_parquet(path, index=False, engine="pyarrow")
            artifacts[name] = path
        isolation = root / "isolation.json"
        isolation.write_text("[]\n", encoding="utf-8")
        metadata_path = root / "metadata.json"
        write_json_exclusive(metadata_path, {
            "schema_version": 2, "governed_dataset": True,
            "feature_columns": ["feature_a", "feature_b"], "total_features": 2,
        })
        artifacts.update({"metadata": metadata_path, "isolation_scope": isolation})
        manifest = build_dataset_manifest(
            scope=scope, source_frame=source, splits=splits,
            feature_columns=["feature_a", "feature_b"], artifact_paths=artifacts,
            label_schema_version="labels-v1", label_revision_sha256="a" * 64,
        )
        write_json_exclusive(root / "dataset-manifest.json", manifest)
        return manifest

    def train(self, data_root: Path, model_root: Path) -> dict:
        env = {
            "TRAIN_SEED": "7", "TRAIN_CPU_LIMIT": "1", "TRAIN_MEMORY_LIMIT": "1Gi",
            "TRAIN_GPU_LIMIT": "0", "TRAIN_RUN_ID": "11111111-1111-4111-8111-111111111111",
            "TRAINER_IMAGE_DIGEST": "sha256:" + "b" * 64,
            "TRAIN_MODEL_PARAMS_JSON": json.dumps({"n_estimators": 5, "max_depth": 2}),
        }
        with mock.patch.dict(os.environ, env, clear=False):
            return run_governed_training("xgboost", str(data_root), str(model_root))

    def test_method_hash_and_validation_only_threshold_are_stable(self) -> None:
        contract = Path(__file__).resolve().parents[2] / "contracts/mlops/governed-evaluation-method.v1.json"
        self.assertEqual(sha256_file(contract), METHOD_CONTRACT_SHA256)
        first = select_abstain_threshold(pd.Series([0.51, 0.9, 0.2, 0.7]).to_numpy(), 0.75)
        second = select_abstain_threshold(pd.Series([0.51, 0.9, 0.2, 0.7]).to_numpy(), 0.75)
        self.assertEqual(first, second)
        self.assertGreaterEqual(first, 0.5)

    def test_full_governed_evaluation_binds_lineage_and_never_authorizes_activation(self) -> None:
        with tempfile.TemporaryDirectory() as data, tempfile.TemporaryDirectory() as model, \
                tempfile.TemporaryDirectory() as output:
            data_root, model_root, output_root = Path(data), Path(model), Path(output)
            dataset = self.make_dataset(data_root)
            training = self.train(data_root, model_root)
            env = {
                "EVALUATION_BOOTSTRAP_SEED": "19",
                "EVALUATION_BOOTSTRAP_ROUNDS": "100",
                "EVALUATION_KNOWN_RETENTION_TARGET": "0.95",
                "EVALUATOR_IMAGE_DIGEST": "sha256:" + "c" * 64,
            }
            with mock.patch.dict(os.environ, env, clear=False):
                evaluation = run_governed_evaluation(str(data_root), str(model_root), str(output_root))
            self.assertEqual(evaluation["dataset_id"], dataset["dataset_id"])
            self.assertEqual(evaluation["training_run_sha256"], training["run_sha256"])
            self.assertFalse(evaluation["activation_authorized"])
            self.assertEqual(evaluation["graph_ablations"]["state"], "NOT_EXECUTED")
            self.assertEqual(set(evaluation["confidence_intervals"]), {
                "known_attack_recall", "normal_false_positive_rate", "unknown_recall",
                "unknown_detection_rate", "known_retention", "auc_roc",
            })
            self.assertIn("metric_definitions", evaluation["metrics"])
            self.assertIn("unknown_detection_rate", evaluation["metrics"])
            self.assertTrue((output_root / "governed-predictions.parquet").is_file())
            self.assertTrue((output_root / "evaluation-manifest.json").is_file())
            with self.assertRaisesRegex(FileExistsError, "overwrite immutable evaluation"):
                with mock.patch.dict(os.environ, env, clear=False):
                    run_governed_evaluation(str(data_root), str(model_root), str(output_root))

    def test_training_rejects_open_set_family_leak(self) -> None:
        with tempfile.TemporaryDirectory() as data, tempfile.TemporaryDirectory() as model:
            data_root = Path(data)
            self.make_dataset(data_root, family_leak=True)
            with self.assertRaisesRegex(ValueError, "holdout attack family"):
                self.train(data_root, Path(model))

    def test_graph_ablation_requires_exact_population_and_preserves_negative_gain(self) -> None:
        base = pd.DataFrame({
            "event_id": ["a", "b", "c", "d"], "label": [0, 0, 1, 1],
            "score": [0.1, 0.2, 0.8, 0.9],
        })
        worse = base.assign(score=[0.4, 0.6, 0.4, 0.6])
        result = compare_graph_ablations({
            "non_graph_baseline": base,
            "gnn_full": worse,
            "gnn_no_edges": base,
            "gnn_no_sources": base,
        })
        self.assertEqual(result["state"], "EVALUATED")
        self.assertLess(result["deltas"]["gnn_full_minus_non_graph"]["f1"], 0)
        mismatched = base.copy()
        mismatched.loc[0, "event_id"] = "different"
        with self.assertRaisesRegex(ValueError, "exact reference population"):
            compare_graph_ablations({
                "non_graph_baseline": base,
                "gnn_full": mismatched,
                "gnn_no_edges": base,
                "gnn_no_sources": base,
            })


if __name__ == "__main__":
    unittest.main(verbosity=2)
