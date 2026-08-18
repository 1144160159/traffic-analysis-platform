#!/usr/bin/env python3

from __future__ import annotations

import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import pandas as pd

sys.path.insert(0, os.path.dirname(__file__))

from dataset_governance import ExtractionScope
from extract_data import (
    build_governed_extraction_query,
    extract_governed_features_with_labels,
    load_isolation_scope,
    run_governed_extraction,
    strict_env_flag,
)


class GovernedExtractDataTest(unittest.TestCase):
    def setUp(self) -> None:
        self.scope = ExtractionScope.create(
            tenant_id="tenant-a", feature_set_id="feature-v1",
            window_from="2026-08-14T00:00:00Z", window_through="2026-08-14T01:00:00Z",
            source_watermark="2026-08-14T01:00:00Z", as_of="2026-08-14T01:05:00Z", max_rows=10,
        )
        self.frame = pd.DataFrame([{
            "tenant_id": "tenant-a", "event_id": "event-1", "feature_set_id": "feature-v1",
            "ts": 1786667400, "ingest_ts": 1786667430, "label": 1,
            "object_id": "object-1", "community_id": "community-1",
        }])

    def test_query_is_parameterized_stable_and_budget_probe_is_max_plus_one(self) -> None:
        query = build_governed_extraction_query()
        self.assertIn("fs.tenant_id = %(tenant_id)s", query)
        self.assertIn("ORDER BY fs.ts, fs.event_id", query)
        self.assertNotIn("now() - INTERVAL", query)
        client = mock.Mock()
        client.query_dataframe.return_value = self.frame
        extracted = extract_governed_features_with_labels(client, self.scope)
        self.assertEqual(len(extracted), 1)
        _, kwargs = client.query_dataframe.call_args
        self.assertEqual(kwargs["params"]["limit"], 11)
        self.assertEqual(kwargs["params"]["tenant_id"], "tenant-a")

    def test_isolation_scope_is_one_to_one_and_never_guessed(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "scope.json"
            path.write_text(json.dumps([{
                "event_id": "event-1", "entity_id": "entity-1", "site_id": "site-1",
                "pcap_id": "pcap-1", "attack_family": "scan",
                "graph_node_ids": ["node-1"], "graph_edge_ids": ["edge-1"],
            }]), encoding="utf-8")
            joined, digest = load_isolation_scope(str(path), self.frame)
            self.assertEqual(joined.loc[0, "site_id"], "site-1")
            self.assertEqual(len(digest), 64)

            path.write_text(json.dumps([{
                "event_id": "different", "entity_id": "entity-1", "site_id": "site-1",
                "pcap_id": "pcap-1", "attack_family": "scan",
            }]), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "does not cover every extracted event"):
                load_isolation_scope(str(path), self.frame)

    def test_governance_flag_rejects_ambiguous_values(self) -> None:
        with mock.patch.dict(os.environ, {"MLOPS_DATASET_GOVERNANCE_V1_ENABLED": "yes"}):
            with self.assertRaisesRegex(ValueError, "explicitly true or false"):
                strict_env_flag("MLOPS_DATASET_GOVERNANCE_V1_ENABLED")

    def test_governed_extraction_requires_an_explicit_open_set_family(self) -> None:
        with mock.patch.dict(os.environ, {"OPEN_SET_ATTACK_FAMILIES": "  "}, clear=True):
            with self.assertRaisesRegex(ValueError, "OPEN_SET_ATTACK_FAMILIES is required"):
                run_governed_extraction(mock.Mock(), "feature-v1", "tenant-a", "/unused")


if __name__ == "__main__":
    unittest.main(verbosity=2)
