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

from dataset_governance import (
    ExtractionScope,
    build_dataset_manifest,
    canonical_json_sha256,
    write_json_exclusive,
)
from train_model import load_governed_training_data, run_governed_training


class GovernedTrainModelTest(unittest.TestCase):
    def make_dataset(self, root: Path) -> dict:
        scope = ExtractionScope.create(
            tenant_id="tenant-a", feature_set_id="feature-v1",
            window_from="2026-08-14T00:00:00Z", window_through="2026-08-14T04:00:00Z",
            source_watermark="2026-08-14T04:00:00Z", as_of="2026-08-14T04:05:00Z", max_rows=100,
        )
        split_names = ["train"] * 8 + ["validation"] * 4 + ["test"] * 4
        rows = []
        for index, split_name in enumerate(split_names):
            timestamp = int(datetime(2026, 8, 14, index // 4, index % 4, tzinfo=timezone.utc).timestamp())
            rows.append({
                "tenant_id": "tenant-a", "feature_set_id": "feature-v1",
                "event_id": f"event-{index}", "object_id": f"object-{index}",
                "community_id": f"community-{index}", "entity_id": f"entity-{index}",
                "site_id": f"site-{index}", "pcap_id": f"pcap-{index}",
                "attack_family": "known", "ts": timestamp, "ingest_ts": timestamp + 1,
                "label": index % 2, "feature_a": float(index), "feature_b": float(index % 3),
                "_split": split_name,
            })
        source = pd.DataFrame(rows).drop(columns=["_split"])
        splits = {
            name: pd.DataFrame([row for row in rows if row["_split"] == name]).drop(columns=["_split"])
            for name in ("train", "validation", "test")
        }
        artifacts = {}
        for name, frame in splits.items():
            path = root / f"{name}.parquet"
            frame.to_parquet(path, index=False, engine="pyarrow")
            artifacts[name] = path
        isolation = root / "isolation.json"
        isolation.write_text("[]\n", encoding="utf-8")
        metadata = {
            "schema_version": 2, "governed_dataset": True,
            "feature_columns": ["feature_a", "feature_b"], "total_features": 2,
        }
        metadata_path = root / "metadata.json"
        write_json_exclusive(metadata_path, metadata)
        artifacts.update({"metadata": metadata_path, "isolation_scope": isolation})
        manifest = build_dataset_manifest(
            scope=scope, source_frame=source, splits=splits,
            feature_columns=["feature_a", "feature_b"], artifact_paths=artifacts,
            label_schema_version="labels-v1", label_revision_sha256="a" * 64,
        )
        write_json_exclusive(root / "dataset-manifest.json", manifest)
        return manifest

    def test_loader_rehashes_every_split_and_training_emits_bound_run(self) -> None:
        with tempfile.TemporaryDirectory() as data_directory, tempfile.TemporaryDirectory() as output_directory:
            data_root, output_root = Path(data_directory), Path(output_directory)
            dataset = self.make_dataset(data_root)
            X_train, y_train, X_validation, y_validation, features, loaded = \
                load_governed_training_data(str(data_root))
            self.assertEqual((len(X_train), len(X_validation)), (8, 4))
            self.assertEqual(features, ["feature_a", "feature_b"])
            self.assertEqual(loaded["dataset_sha256"], dataset["dataset_sha256"])

            env = {
                "TRAIN_SEED": "7",
                "TRAIN_CPU_LIMIT": "1",
                "TRAIN_MEMORY_LIMIT": "1Gi",
                "TRAIN_GPU_LIMIT": "0",
                "TRAIN_RUN_ID": "11111111-1111-4111-8111-111111111111",
                "TRAINER_IMAGE_DIGEST": "sha256:" + "b" * 64,
                "TRAIN_MODEL_PARAMS_JSON": json.dumps({"n_estimators": 3, "max_depth": 2}),
            }
            with mock.patch.dict(os.environ, env, clear=False):
                run = run_governed_training("xgboost", str(data_root), str(output_root))
            self.assertEqual(run["dataset_id"], dataset["dataset_id"])
            self.assertEqual(run["seed"], 7)
            self.assertEqual(run["resources"], {"cpu_limit": "1", "memory_limit": "1Gi", "gpu_limit": 0})
            self.assertEqual(run["run_sha256"], canonical_json_sha256({
                key: value for key, value in run.items() if key not in {"schema_version", "state", "run_sha256"}
            }))
            self.assertTrue((output_root / "training-run-manifest.json").is_file())

    def test_loader_rejects_parquet_drift_before_training(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.make_dataset(root)
            with open(root / "train.parquet", "ab") as handle:
                handle.write(b"drift")
            with self.assertRaisesRegex(ValueError, "train artifact hash"):
                load_governed_training_data(str(root))


if __name__ == "__main__":
    unittest.main(verbosity=2)
