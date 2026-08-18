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

import numpy as np
import pandas as pd

sys.path.insert(0, os.path.dirname(__file__))

from dataset_governance import ExtractionScope, build_dataset_manifest, sha256_file, write_json_exclusive
from gnn_training import load_graph_training_bundle, run_governed_gnn_training
from governed_evaluation import compare_graph_ablations
from graph_governance import build_graph_snapshot_manifest, validate_graph_snapshot_manifest


class GovernedGNNTrainingTest(unittest.TestCase):
    def make_bundle(self, data_root: Path, graph_root: Path, *, cross_split: bool = False) -> dict:
        graph_root.mkdir(parents=True, exist_ok=True)
        split_names = ["train"] * 12 + ["validation"] * 8 + ["test"] * 12 + ["open_set"] * 8
        rows, node_rows, edge_rows = [], [], []
        event_to_split: dict[str, str] = {}
        for index, split_name in enumerate(split_names):
            event_id, node_id = f"event-{index:03d}", f"node-{index:03d}"
            event_to_split[event_id] = split_name
            timestamp = int(datetime(
                2026, 8, 14, index // 10, index % 10, tzinfo=timezone.utc,
            ).timestamp())
            is_open = split_name == "open_set"
            label = 1 if is_open else index % 2
            rows.append({
                "tenant_id": "tenant-a", "feature_set_id": "feature-v1",
                "event_id": event_id, "object_id": f"object-{index:03d}",
                "community_id": f"community-{index:03d}", "entity_id": f"entity-{index:03d}",
                "site_id": f"site-{index:03d}", "pcap_id": f"pcap-{index:03d}",
                "attack_family": "unknown-holdout" if is_open else "known-family",
                "graph_node_ids": json.dumps([node_id]), "graph_edge_ids": json.dumps([]),
                "ts": timestamp, "ingest_ts": timestamp + 1, "label": label,
                "feature_a": float(label * 5 + index / 100), "_split": split_name,
            })
            node_rows.append({
                "node_id": node_id, "event_id": event_id,
                "graph_signal": float(label * 3 + (index % 3)),
                "graph_context": float((index * 5) % 7),
            })
        by_split: dict[str, list[int]] = {}
        for index, split_name in enumerate(split_names):
            by_split.setdefault(split_name, []).append(index)
        edge_index = 0
        for split_name, indices in by_split.items():
            for source, target in zip(indices[:-1], indices[1:]):
                edge_id = f"edge-{edge_index:03d}"
                edge_index += 1
                edge_rows.append({
                    "edge_id": edge_id,
                    "source_node_id": f"node-{source:03d}",
                    "target_node_id": f"node-{target:03d}",
                    "observed_at": "2026-08-14T04:00:00Z",
                    "source_kind": "evidence" if edge_index % 2 else "flow",
                    "evidence_id": f"evidence-{edge_index:03d}",
                })
                rows[source]["graph_edge_ids"] = json.dumps([edge_id])
        if cross_split:
            edge_rows[0]["target_node_id"] = "node-012"
        nodes, edges = pd.DataFrame(node_rows), pd.DataFrame(edge_rows)
        nodes_path, edges_path = graph_root / "nodes.parquet", graph_root / "edges.parquet"
        nodes.to_parquet(nodes_path, index=False, engine="pyarrow")
        edges.to_parquet(edges_path, index=False, engine="pyarrow")
        graph = build_graph_snapshot_manifest(
            tenant_id="tenant-a", as_of="2026-08-14T05:00:00Z",
            source_watermarks={"clickhouse": "2026-08-14T04:50:00Z", "nebula": "2026-08-14T04:55:00Z"},
            nodes_path=nodes_path, edges_path=edges_path,
            feature_columns=["graph_signal", "graph_context"],
            source_evidence_sha256="d" * 64,
        )
        graph_manifest_path = graph_root / "graph-snapshot-manifest.json"
        write_json_exclusive(graph_manifest_path, graph)

        scope = ExtractionScope.create(
            tenant_id="tenant-a", feature_set_id="feature-v1",
            window_from="2026-08-14T00:00:00Z", window_through="2026-08-14T05:00:00Z",
            source_watermark="2026-08-14T05:00:00Z", as_of="2026-08-14T05:05:00Z",
            max_rows=100,
        )
        source = pd.DataFrame(rows).drop(columns=["_split"])
        splits = {
            name: pd.DataFrame([row for row in rows if row["_split"] == name]).drop(columns=["_split"])
            for name in ("train", "validation", "test", "open_set")
        }
        artifacts = {}
        for name, frame in splits.items():
            path = data_root / f"{name}.parquet"
            frame.to_parquet(path, index=False, engine="pyarrow")
            artifacts[name] = path
        isolation = data_root / "isolation.json"
        isolation.write_text("[]\n", encoding="utf-8")
        metadata_path = data_root / "metadata.json"
        write_json_exclusive(metadata_path, {
            "schema_version": 2, "governed_dataset": True,
            "feature_columns": ["feature_a"], "total_features": 1,
        })
        artifacts.update({"metadata": metadata_path, "isolation_scope": isolation,
                          "graph_snapshot_manifest": graph_manifest_path})
        dataset = build_dataset_manifest(
            scope=scope, source_frame=source, splits=splits,
            feature_columns=["feature_a"], artifact_paths=artifacts,
            label_schema_version="labels-v1", label_revision_sha256="a" * 64,
            graph_snapshot={"snapshot_id": graph["snapshot_id"],
                            "manifest_sha256": sha256_file(graph_manifest_path)},
        )
        write_json_exclusive(data_root / "dataset-manifest.json", dataset)
        return {"dataset": dataset, "graph": graph}

    def test_graph_manifest_rehashes_artifacts_and_future_edges_fail(self) -> None:
        with tempfile.TemporaryDirectory() as data, tempfile.TemporaryDirectory() as graph:
            bundle = self.make_bundle(Path(data), Path(graph))
            nodes, edges, features = validate_graph_snapshot_manifest(
                bundle["graph"], Path(graph) / "nodes.parquet", Path(graph) / "edges.parquet"
            )
            self.assertEqual(len(nodes), 40)
            self.assertGreater(len(edges), 0)
            self.assertEqual(features, ["graph_context", "graph_signal"])
            with open(Path(graph) / "nodes.parquet", "ab") as handle:
                handle.write(b"drift")
            with self.assertRaisesRegex(ValueError, "nodes artifact hash"):
                validate_graph_snapshot_manifest(
                    bundle["graph"], Path(graph) / "nodes.parquet", Path(graph) / "edges.parquet"
                )

    def test_cross_split_graph_edge_is_rejected_before_training(self) -> None:
        with tempfile.TemporaryDirectory() as data, tempfile.TemporaryDirectory() as graph:
            self.make_bundle(Path(data), Path(graph), cross_split=True)
            with self.assertRaisesRegex(ValueError, "crosses governed"):
                load_graph_training_bundle(data, graph)

    def test_real_gcn_suite_emits_three_exact_population_variants(self) -> None:
        with tempfile.TemporaryDirectory() as data, tempfile.TemporaryDirectory() as graph, \
                tempfile.TemporaryDirectory() as output:
            bundle = self.make_bundle(Path(data), Path(graph))
            env = {
                "GNN_TRAIN_SEED": "23", "GNN_HIDDEN_SIZE": "4", "GNN_EPOCHS": "40",
                "GNN_LEARNING_RATE": "0.03", "GNN_L2": "0.0001", "GNN_PATIENCE": "8",
                "GNN_SOURCE_ABLATION_KINDS": "evidence",
                "GNN_TRAIN_RUN_ID": "44444444-4444-4444-8444-444444444444",
                "TRAINER_IMAGE_DIGEST": "sha256:" + "e" * 64,
                "TRAIN_CPU_LIMIT": "1", "TRAIN_MEMORY_LIMIT": "1Gi", "TRAIN_GPU_LIMIT": "0",
            }
            with mock.patch.dict(os.environ, env, clear=False):
                training = run_governed_gnn_training(data, graph, output)
            self.assertEqual(training["algorithm"], "gnn")
            self.assertEqual(training["graph_snapshot"]["snapshot_id"], bundle["graph"]["snapshot_id"])
            variants = {
                name: pd.read_parquet(Path(output) / f"{name}.parquet", engine="pyarrow")
                for name in ("gnn_full", "gnn_no_edges", "gnn_no_sources")
            }
            baseline = variants["gnn_no_edges"].assign(
                score=np.linspace(0.1, 0.9, len(variants["gnn_no_edges"]))
            )
            result = compare_graph_ablations({"non_graph_baseline": baseline, **variants})
            self.assertEqual(result["state"], "EVALUATED")
            self.assertTrue((Path(output) / "gnn-training-run-manifest.json").is_file())
            self.assertEqual(len(training["artifacts"]), 7)


if __name__ == "__main__":
    unittest.main(verbosity=2)
