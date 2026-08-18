#!/usr/bin/env python3

from __future__ import annotations

import json
import os
import sys
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path

import pandas as pd

sys.path.insert(0, os.path.dirname(__file__))

from dataset_governance import (
    ExtractionScope,
    build_dataset_manifest,
    canonical_rows_sha256,
    split_by_time_and_validate,
    validate_extracted_frame,
    write_json_exclusive,
)


UTC = timezone.utc


class DatasetGovernanceTest(unittest.TestCase):
    def setUp(self) -> None:
        self.scope = ExtractionScope.create(
            tenant_id="tenant-a",
            feature_set_id="feature-v1",
            window_from="2026-08-14T00:00:00Z",
            window_through="2026-08-14T04:00:00Z",
            source_watermark="2026-08-14T04:00:00Z",
            as_of="2026-08-14T04:05:00Z",
            max_rows=100,
        )
        timestamps = [
            "2026-08-14T00:10:00Z", "2026-08-14T00:20:00Z",
            "2026-08-14T01:10:00Z", "2026-08-14T01:20:00Z",
            "2026-08-14T02:10:00Z", "2026-08-14T02:20:00Z",
            "2026-08-14T03:10:00Z", "2026-08-14T03:20:00Z",
        ]
        rows = []
        for index, timestamp in enumerate(timestamps):
            seconds = int(datetime.fromisoformat(timestamp.replace("Z", "+00:00")).timestamp())
            rows.append({
                "tenant_id": "tenant-a",
                "feature_set_id": "feature-v1",
                "event_id": f"event-{index}",
                "object_id": f"object-{index}",
                "community_id": f"community-{index}",
                "entity_id": f"entity-{index}",
                "site_id": f"site-{index}",
                "pcap_id": f"pcap-{index}",
                "attack_family": "holdout-x" if index == 7 else f"known-{index % 2}",
                "graph_node_ids": json.dumps([f"node-{index}"]),
                "graph_edge_ids": json.dumps([f"edge-{index}"]),
                "ts": seconds,
                "ingest_ts": seconds + 30,
                "label": index % 2,
                "feature_a": float(index),
            })
        self.frame = pd.DataFrame(rows)

    def split(self, frame: pd.DataFrame | None = None):
        return split_by_time_and_validate(
            self.frame if frame is None else frame,
            train_through="2026-08-14T01:00:00Z",
            validation_through="2026-08-14T02:00:00Z",
            holdout_attack_families=["holdout-x"],
        )

    def test_scope_rejects_future_watermark(self) -> None:
        with self.assertRaisesRegex(ValueError, "watermark exceeds as_of"):
            ExtractionScope.create(
                tenant_id="tenant-a", feature_set_id="feature-v1",
                window_from="2026-08-14T00:00:00Z", window_through="2026-08-14T01:00:00Z",
                source_watermark="2026-08-14T02:00:00Z", as_of="2026-08-14T01:30:00Z", max_rows=100,
            )

    def test_frame_rejects_future_ingest_and_budget_truncation(self) -> None:
        future = self.frame.copy()
        future.loc[0, "ingest_ts"] = int(datetime(2026, 8, 14, 5, tzinfo=UTC).timestamp())
        with self.assertRaisesRegex(ValueError, "ingested after as_of"):
            validate_extracted_frame(future, self.scope)
        tight = ExtractionScope.create(
            tenant_id="tenant-a", feature_set_id="feature-v1",
            window_from=self.scope.window_from, window_through=self.scope.window_through,
            source_watermark=self.scope.source_watermark, as_of=self.scope.as_of, max_rows=2,
        )
        with self.assertRaisesRegex(ValueError, "row budget exceeded"):
            validate_extracted_frame(self.frame, tight)

    def test_time_site_entity_pcap_family_and_graph_split_isolation(self) -> None:
        splits = self.split()
        self.assertEqual(set(splits), {"train", "validation", "test", "open_set"})
        self.assertEqual(set(splits["open_set"]["attack_family"]), {"holdout-x"})

        leaked = self.frame.copy()
        leaked.loc[4, "object_id"] = leaked.loc[0, "object_id"]
        with self.assertRaisesRegex(ValueError, "leakage: object_id"):
            self.split(leaked)

        graph_leaked = self.frame.copy()
        graph_leaked.loc[4, "graph_node_ids"] = graph_leaked.loc[0, "graph_node_ids"]
        with self.assertRaisesRegex(ValueError, "graph leakage"):
            self.split(graph_leaked)

    def test_manifest_is_deterministic_and_exclusive(self) -> None:
        splits = self.split()
        validate_extracted_frame(self.frame, self.scope)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            artifacts = {}
            for name, split in splits.items():
                path = root / f"{name}.json"
                path.write_text(split.to_json(orient="records"), encoding="utf-8")
                artifacts[name] = path
            label_revision = "a" * 64
            first = build_dataset_manifest(
                scope=self.scope, source_frame=self.frame, splits=splits,
                feature_columns=["feature_a"], artifact_paths=artifacts,
                label_schema_version="labels-v1", label_revision_sha256=label_revision,
            )
            second = build_dataset_manifest(
                scope=self.scope, source_frame=self.frame.sample(frac=1, random_state=42), splits=splits,
                feature_columns=["feature_a"], artifact_paths=artifacts,
                label_schema_version="labels-v1", label_revision_sha256=label_revision,
            )
            self.assertEqual(first["dataset_id"], second["dataset_id"])
            self.assertEqual(first["dataset_sha256"], second["dataset_sha256"])
            self.assertEqual(canonical_rows_sha256(self.frame), canonical_rows_sha256(self.frame.iloc[::-1]))

            manifest_path = root / "dataset-manifest.json"
            write_json_exclusive(manifest_path, first)
            with self.assertRaises(FileExistsError):
                write_json_exclusive(manifest_path, second)


if __name__ == "__main__":
    unittest.main(verbosity=2)
