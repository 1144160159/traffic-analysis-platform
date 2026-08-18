#!/usr/bin/env python3
"""Immutable graph-training snapshot construction and validation."""

from __future__ import annotations

import json
import math
import uuid
from pathlib import Path
from typing import Any, Mapping, Sequence

import numpy as np
import pandas as pd

from dataset_governance import canonical_json_sha256, parse_utc, rfc3339, sha256_file


GRAPH_NAMESPACE = uuid.UUID("a1fe5d2f-84d1-5cf0-8418-c672a5e1489a")
NODE_REQUIRED = {"node_id", "event_id"}
EDGE_REQUIRED = {
    "edge_id", "source_node_id", "target_node_id", "observed_at", "source_kind", "evidence_id",
}


def _node_schema(feature_columns: Sequence[str]) -> dict[str, Any]:
    features = sorted(set(str(item).strip() for item in feature_columns if str(item).strip()))
    if not features or NODE_REQUIRED & set(features):
        raise ValueError("graph node feature columns must be non-empty and cannot reuse identity columns")
    return {
        "id_column": "node_id",
        "event_id_column": "event_id",
        "feature_columns": features,
    }


def _edge_schema() -> dict[str, str]:
    return {
        "id_column": "edge_id",
        "source_column": "source_node_id",
        "target_column": "target_node_id",
        "observed_at_column": "observed_at",
        "source_kind_column": "source_kind",
        "evidence_id_column": "evidence_id",
    }


def validate_graph_frames(
    nodes: pd.DataFrame,
    edges: pd.DataFrame,
    *,
    feature_columns: Sequence[str],
    as_of: str,
) -> None:
    features = _node_schema(feature_columns)["feature_columns"]
    missing_nodes = sorted((NODE_REQUIRED | set(features)) - set(nodes.columns))
    missing_edges = sorted(EDGE_REQUIRED - set(edges.columns))
    if missing_nodes or missing_edges:
        raise ValueError(f"graph snapshot columns missing: nodes={missing_nodes}, edges={missing_edges}")
    if nodes.empty:
        raise ValueError("graph snapshot must contain at least one node")
    for column in ("node_id", "event_id"):
        values = nodes[column].astype(str).str.strip()
        if values.eq("").any() or values.duplicated().any():
            raise ValueError(f"graph {column} must be non-empty and unique")
    numeric = nodes[features].apply(pd.to_numeric, errors="coerce")
    if numeric.isnull().any().any() or not np.isfinite(numeric.to_numpy(dtype=float)).all():
        raise ValueError("graph node features must be finite numeric values")
    if not edges.empty:
        for column in ("edge_id", "source_node_id", "target_node_id", "source_kind", "evidence_id"):
            values = edges[column].astype(str).str.strip()
            if values.eq("").any():
                raise ValueError(f"graph edge {column} cannot be empty")
        if edges["edge_id"].astype(str).duplicated().any():
            raise ValueError("graph edge_id must be unique")
        node_ids = set(nodes["node_id"].astype(str))
        endpoints = set(edges["source_node_id"].astype(str)) | set(edges["target_node_id"].astype(str))
        if endpoints - node_ids:
            raise ValueError("graph edge references a node outside the immutable snapshot")
        edge_times = pd.to_datetime(edges["observed_at"], utc=True, errors="coerce")
        if edge_times.isnull().any() or (edge_times > parse_utc(as_of, "graph.as_of")).any():
            raise ValueError("graph contains an edge observed after as_of")


def build_graph_snapshot_manifest(
    *,
    tenant_id: str,
    as_of: str,
    source_watermarks: Mapping[str, str],
    nodes_path: str | Path,
    edges_path: str | Path,
    feature_columns: Sequence[str],
    source_evidence_sha256: str,
) -> dict[str, Any]:
    tenant = str(tenant_id).strip()
    if not tenant or tenant.lower() == "unknown":
        raise ValueError("graph tenant_id is required and cannot be unknown")
    if len(source_evidence_sha256) != 64 or any(c not in "0123456789abcdef" for c in source_evidence_sha256):
        raise ValueError("source_evidence_sha256 must be lowercase SHA-256")
    graph_as_of = parse_utc(as_of, "graph.as_of")
    watermarks = {
        str(name).strip(): rfc3339(parse_utc(value, f"source_watermarks.{name}"))
        for name, value in sorted(source_watermarks.items())
        if str(name).strip()
    }
    if not watermarks or any(parse_utc(value, "source watermark") > graph_as_of for value in watermarks.values()):
        raise ValueError("graph source watermarks must be non-empty and at or before as_of")
    nodes_file, edges_file = Path(nodes_path), Path(edges_path)
    nodes = pd.read_parquet(nodes_file, engine="pyarrow")
    edges = pd.read_parquet(edges_file, engine="pyarrow")
    node_schema, edge_schema = _node_schema(feature_columns), _edge_schema()
    validate_graph_frames(
        nodes, edges, feature_columns=node_schema["feature_columns"], as_of=rfc3339(graph_as_of)
    )
    nodes_sha, edges_sha = sha256_file(nodes_file), sha256_file(edges_file)
    meaning = {
        "tenant_id": tenant,
        "as_of": rfc3339(graph_as_of),
        "source_watermarks": watermarks,
        "node_schema": node_schema,
        "edge_schema": edge_schema,
        "node_schema_sha256": canonical_json_sha256(node_schema),
        "edge_schema_sha256": canonical_json_sha256(edge_schema),
        "node_count": len(nodes),
        "edge_count": len(edges),
        "nodes_sha256": nodes_sha,
        "edges_sha256": edges_sha,
        "source_evidence_sha256": source_evidence_sha256,
        "artifacts": {"nodes": nodes_sha, "edges": edges_sha},
    }
    graph_sha = canonical_json_sha256(meaning)
    return {
        "schema_version": 1,
        "snapshot_id": str(uuid.uuid5(GRAPH_NAMESPACE, graph_sha)),
        **meaning,
        "graph_snapshot_sha256": graph_sha,
    }


def validate_graph_snapshot_manifest(
    manifest: Mapping[str, Any], nodes_path: str | Path, edges_path: str | Path,
) -> tuple[pd.DataFrame, pd.DataFrame, list[str]]:
    try:
        meaning = {
            key: manifest[key]
            for key in (
                "tenant_id", "as_of", "source_watermarks", "node_schema", "edge_schema",
                "node_schema_sha256", "edge_schema_sha256", "node_count", "edge_count",
                "nodes_sha256", "edges_sha256", "source_evidence_sha256", "artifacts",
            )
        }
    except KeyError as exc:
        raise ValueError("graph snapshot manifest is incomplete") from exc
    graph_sha = canonical_json_sha256(meaning)
    expected_id = str(uuid.uuid5(GRAPH_NAMESPACE, graph_sha))
    if manifest.get("schema_version") != 1 or manifest.get("graph_snapshot_sha256") != graph_sha:
        raise ValueError("graph snapshot meaning hash mismatch")
    if manifest.get("snapshot_id") != expected_id:
        raise ValueError("graph snapshot deterministic identity mismatch")
    node_schema, edge_schema = manifest["node_schema"], manifest["edge_schema"]
    if canonical_json_sha256(node_schema) != manifest["node_schema_sha256"] or \
            canonical_json_sha256(edge_schema) != manifest["edge_schema_sha256"]:
        raise ValueError("graph node or edge schema hash mismatch")
    nodes_file, edges_file = Path(nodes_path), Path(edges_path)
    for name, path in (("nodes", nodes_file), ("edges", edges_file)):
        actual = sha256_file(path)
        if actual != manifest["artifacts"][name] or actual != manifest[f"{name}_sha256"]:
            raise ValueError(f"graph {name} artifact hash mismatch")
    nodes = pd.read_parquet(nodes_file, engine="pyarrow")
    edges = pd.read_parquet(edges_file, engine="pyarrow")
    features = list(node_schema["feature_columns"])
    validate_graph_frames(nodes, edges, feature_columns=features, as_of=manifest["as_of"])
    if len(nodes) != manifest["node_count"] or len(edges) != manifest["edge_count"]:
        raise ValueError("graph node or edge count mismatch")
    return nodes, edges, features
