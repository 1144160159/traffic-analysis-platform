#!/usr/bin/env python3
"""Lineage-bound baseline and GCN explanation artifacts."""

from __future__ import annotations

import json
import os
import re
import uuid
from pathlib import Path
from typing import Any, Mapping

import numpy as np
import pandas as pd
import xgboost as xgb
from scipy import sparse

from dataset_governance import canonical_json_sha256, sha256_file, write_json_exclusive
from gnn_training import GCNModel, GraphSplit, _sigmoid, _variant_edges, load_graph_training_bundle
from governed_evaluation import (
    load_governed_evaluation_inputs,
    validate_evaluation_manifest_identity,
)


EXPLANATION_NAMESPACE = uuid.UUID("d8e68ea1-65cc-5875-8b30-752eadf18458")
LIMITATIONS = [
    "Feature and edge occlusion quantify local model sensitivity, not causality.",
    "GNN path explanations are ranked one-hop evidence edges and do not prove an end-to-end attack path.",
    "Open-set abstention quality is bounded by the held-out families and sites in the immutable evaluation dataset.",
    "Explanations cannot authorize model activation, deployment, analyst disposition or CNAS claims.",
]


def required_env(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise ValueError(f"{name} is required when MLOPS_GOVERNED_EXPLANATION_V1_ENABLED=true")
    return value


from governance_common import load_json_object as _load_json  # noqa: F401  (DRY 收敛)


def _validate_training_manifest(manifest: Mapping[str, Any]) -> None:
    meaning = {
        key: value for key, value in manifest.items()
        if key not in {"schema_version", "state", "run_sha256"}
    }
    if manifest.get("schema_version") != 1 or manifest.get("state") != "trained" or \
            canonical_json_sha256(meaning) != manifest.get("run_sha256"):
        raise ValueError("training run manifest version, state or meaning hash is invalid")


def validate_explanation_manifest_identity(manifest: Mapping[str, Any]) -> None:
    meaning = {
        key: value for key, value in manifest.items()
        if key not in {"schema_version", "explanation_id", "state", "explanation_sha256"}
    }
    explanation_sha = canonical_json_sha256(meaning)
    if manifest.get("schema_version") != 1 or manifest.get("state") != "explained" or \
            manifest.get("explanation_sha256") != explanation_sha or \
            manifest.get("explanation_id") != str(uuid.uuid5(EXPLANATION_NAMESPACE, explanation_sha)):
        raise ValueError("explanation manifest version, state or identity is invalid")
    if manifest.get("activation_authorized") is not False:
        raise ValueError("explanation manifest must never authorize activation")


def _load_gcn_model(path: Path) -> GCNModel:
    with np.load(path, allow_pickle=False) as values:
        required = {"feature_mean", "feature_std", "w0", "b0", "w1", "b1", "best_epoch", "validation_loss"}
        if required - set(values.files):
            raise ValueError("GNN model artifact is incomplete")
        return GCNModel(
            feature_mean=values["feature_mean"], feature_std=values["feature_std"],
            w0=values["w0"], b0=values["b0"], w1=values["w1"], b1=values["b1"],
            best_epoch=int(values["best_epoch"][0]),
            validation_loss=float(values["validation_loss"][0]),
        )


def _write_parquet_exclusive(path: Path, frame: pd.DataFrame) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        raise FileExistsError(f"refusing to overwrite immutable explanation artifact: {path}")
    temporary = path.with_name(f".{path.name}.{uuid.uuid4().hex}.tmp")
    try:
        frame.to_parquet(temporary, index=False, engine="pyarrow")
        os.link(temporary, path)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass


def _baseline_contributions(model: Any, features: pd.DataFrame, columns: list[str]) -> tuple[np.ndarray, np.ndarray]:
    matrix = xgb.DMatrix(features, feature_names=columns)
    contributions = np.asarray(model.get_booster().predict(matrix, pred_contribs=True), dtype=float)
    if contributions.shape != (len(features), len(columns) + 1):
        raise ValueError("XGBoost contribution tensor shape drifted")
    scores = np.asarray(model.predict_proba(features)[:, 1], dtype=float)
    return scores, contributions


def _precompute_adjacency(
    split: GraphSplit, edges: pd.DataFrame,
) -> tuple[dict[str, int], sparse.csr_matrix, np.ndarray, dict[str, tuple[int, int]]]:
    """Build the base sparse adjacency once for reuse in edge-occlusion.

    代码审查 H40 收敛项：原实现对每条 incident 边都做 pandas 子集过滤并重建
    归一化邻接（O(E²)）。此处预计算节点索引、基邻接与度数，逐边移除只更新
    两个端点的度数并做一次 O(nnz) 稀疏过滤。
    """
    ordered = list(split.node_ids)
    if not ordered:
        raise ValueError("cannot build adjacency for an empty node set")
    index_map = {node_id: offset for offset, node_id in enumerate(ordered)}
    rows: list[int] = list(range(len(ordered)))
    cols: list[int] = list(range(len(ordered)))
    edge_index: dict[str, tuple[int, int]] = {}
    for row in edges.itertuples(index=False):
        source, target = str(row.source_node_id), str(row.target_node_id)
        if source not in index_map or target not in index_map:
            raise ValueError("split adjacency contains a cross-split or unknown endpoint")
        if source == target:
            continue
        i, j = index_map[source], index_map[target]
        edge_index[str(row.edge_id)] = (i, j)
        rows.extend((i, j))
        cols.extend((j, i))
    weights = [1.0] * len(rows)
    coo = sparse.coo_matrix((weights, (rows, cols)), shape=(len(ordered), len(ordered))).tocsr()
    coo.sum_duplicates()
    coo.data[:] = 1.0
    degree = np.asarray(coo.sum(axis=1)).reshape(-1)
    return index_map, coo, degree, edge_index


def _renormalize(coo: sparse.csr_matrix, degree: np.ndarray) -> sparse.csr_matrix:
    inverse = np.zeros_like(degree, dtype=float)
    np.power(degree, -0.5, out=inverse, where=degree > 0)
    scale = sparse.diags(inverse)
    return (scale @ coo @ scale).tocsr()


def _coo_without_edge(base_coo: sparse.csr_matrix, i: int, j: int) -> sparse.csr_matrix:
    coo = base_coo.tocoo()
    keep = ~(((coo.row == i) & (coo.col == j)) | ((coo.row == j) & (coo.col == i)))
    return sparse.coo_matrix(
        (coo.data[keep], (coo.row[keep], coo.col[keep])), shape=base_coo.shape,
    ).tocsr()


def _gnn_occlusion_explanations(
    model: GCNModel,
    split: GraphSplit,
    feature_columns: tuple[str, ...],
    selected_events: list[str],
) -> tuple[dict[str, float], dict[str, list[dict[str, Any]]], dict[str, list[dict[str, Any]]]]:
    full_edges = _variant_edges(split.edges, "gnn_full", set())
    index_map, base_coo, base_degree, edge_index = _precompute_adjacency(split, full_edges)
    base_x = (split.features - model.feature_mean) / model.feature_std

    def _forward(adjacency: sparse.csr_matrix, x_matrix: np.ndarray) -> np.ndarray:
        hidden = np.maximum(adjacency @ x_matrix @ model.w0 + model.b0, 0.0)
        logits = adjacency @ hidden @ model.w1 + model.b1
        return _sigmoid(np.asarray(logits).reshape(-1))

    base_adj = _renormalize(base_coo, base_degree)
    full_scores = _forward(base_adj, base_x)
    event_index = {event_id: index for index, event_id in enumerate(split.event_ids)}
    node_for_event = {event_id: split.node_ids[index] for event_id, index in event_index.items()}
    score_by_event = {event_id: float(full_scores[index]) for event_id, index in event_index.items()}
    feature_explanations: dict[str, list[dict[str, Any]]] = {event_id: [] for event_id in selected_events}
    for feature_index, feature_name in enumerate(feature_columns):
        occluded_features = split.features.copy()
        occluded_features[:, feature_index] = model.feature_mean[feature_index]
        occluded_x = (occluded_features - model.feature_mean) / model.feature_std
        occluded_scores = _forward(base_adj, occluded_x)
        for event_id in selected_events:
            index = event_index[event_id]
            feature_explanations[event_id].append({
                "feature": feature_name,
                "score_delta_when_occluded": float(full_scores[index] - occluded_scores[index]),
            })
    for values in feature_explanations.values():
        values.sort(key=lambda item: (-abs(item["score_delta_when_occluded"]), item["feature"]))

    edge_explanations: dict[str, list[dict[str, Any]]] = {event_id: [] for event_id in selected_events}
    for event_id in selected_events:
        node_id, index = node_for_event[event_id], event_index[event_id]
        incident = full_edges[
            (full_edges["source_node_id"].astype(str) == node_id) |
            (full_edges["target_node_id"].astype(str) == node_id)
        ]
        for edge in incident.itertuples(index=False):
            i, j = edge_index[str(edge.edge_id)]
            reduced_degree = base_degree.copy()
            if i != j:
                reduced_degree[i] = max(0.0, reduced_degree[i] - 1.0)
                reduced_degree[j] = max(0.0, reduced_degree[j] - 1.0)
            reduced_score = _forward(
                _renormalize(_coo_without_edge(base_coo, i, j), reduced_degree), base_x,
            )[index]
            edge_explanations[event_id].append({
                "edge_id": str(edge.edge_id),
                "source_node_id": str(edge.source_node_id),
                "target_node_id": str(edge.target_node_id),
                "source_kind": str(edge.source_kind),
                "evidence_id": str(edge.evidence_id),
                "score_delta_when_removed": float(full_scores[index] - reduced_score),
            })
        edge_explanations[event_id].sort(
            key=lambda item: (-abs(item["score_delta_when_removed"]), item["edge_id"])
        )
    paths = {
        event_id: [
            {
                "rank": rank,
                "kind": "one_hop_evidence_edge",
                **edge,
            }
            for rank, edge in enumerate(edge_explanations[event_id][:3], start=1)
        ]
        for event_id in selected_events
    }
    return score_by_event, feature_explanations, paths


def run_governed_explanation(
    data_dir: str,
    baseline_model_dir: str,
    graph_dir: str,
    gnn_dir: str,
    evaluation_dir: str,
    output_dir: str,
) -> dict[str, Any]:
    image_digest = required_env("EXPLAINER_IMAGE_DIGEST")
    if not re.fullmatch(r"sha256:[0-9a-f]{64}", image_digest):
        raise ValueError("EXPLAINER_IMAGE_DIGEST must be sha256:<lowercase digest>")
    max_events = int(required_env("EXPLANATION_MAX_EVENTS"))
    if max_events < 1 or max_events > 1000:
        raise ValueError("EXPLANATION_MAX_EVENTS must be between 1 and 1000")

    baseline_model, frames, baseline_features, dataset, baseline_training = \
        load_governed_evaluation_inputs(data_dir, baseline_model_dir)
    graph_bundle = load_graph_training_bundle(data_dir, graph_dir)
    gnn_root, evaluation_root = Path(gnn_dir).resolve(), Path(evaluation_dir).resolve()
    gnn_training = _load_json(gnn_root / "gnn-training-run-manifest.json", "GNN training run manifest")
    _validate_training_manifest(gnn_training)
    if gnn_training.get("dataset_sha256") != dataset["dataset_sha256"] or \
            gnn_training.get("graph_snapshot") != dataset.get("graph_snapshot"):
        raise ValueError("GNN training lineage does not match the explanation dataset")
    gnn_model_path = gnn_root / "gnn_full-model.npz"
    if gnn_training.get("artifacts", {}).get("gnn_full_model") != sha256_file(gnn_model_path):
        raise ValueError("full GNN model hash does not match training manifest")
    gnn_predictions_path = gnn_root / "gnn_full.parquet"
    if gnn_training.get("artifacts", {}).get("gnn_full_predictions") != sha256_file(gnn_predictions_path):
        raise ValueError("full GNN prediction hash does not match training manifest")

    evaluation = _load_json(evaluation_root / "evaluation-manifest.json", "evaluation manifest")
    validate_evaluation_manifest_identity(evaluation)
    if evaluation.get("dataset_sha256") != dataset["dataset_sha256"] or \
            evaluation.get("training_run_sha256") != baseline_training["run_sha256"] or \
            evaluation.get("graph_training_run", {}).get("run_sha256") != gnn_training["run_sha256"]:
        raise ValueError("evaluation lineage does not match explanation inputs")
    governed_predictions_path = evaluation_root / "governed-predictions.parquet"
    if evaluation.get("prediction_artifact_sha256") != sha256_file(governed_predictions_path):
        raise ValueError("governed prediction hash does not match evaluation manifest")

    test = frames["test"].sort_values("event_id", kind="mergesort").reset_index(drop=True)
    selected_events = test["event_id"].astype(str).tolist()[:max_events]
    selected = test[test["event_id"].astype(str).isin(selected_events)].copy()
    baseline_scores, contributions = _baseline_contributions(
        baseline_model, selected[baseline_features], baseline_features
    )
    gnn_model = _load_gcn_model(gnn_model_path)
    gnn_scores, gnn_features, gnn_paths = _gnn_occlusion_explanations(
        gnn_model, graph_bundle.splits["test"], graph_bundle.feature_columns, selected_events
    )
    expected_gnn = pd.read_parquet(gnn_predictions_path, engine="pyarrow").set_index("event_id")["score"]
    for event_id in selected_events:
        if event_id not in expected_gnn.index or not np.isclose(
            gnn_scores[event_id], float(expected_gnn[event_id]), rtol=1e-10, atol=1e-12
        ):
            raise ValueError("explainer GNN inference does not match immutable prediction artifact")

    rows = []
    for row_index, row in selected.reset_index(drop=True).iterrows():
        event_id = str(row["event_id"])
        baseline_values = [
            {"feature": feature, "contribution": float(contributions[row_index, index])}
            for index, feature in enumerate(baseline_features)
        ]
        baseline_values.sort(key=lambda item: (-abs(item["contribution"]), item["feature"]))
        rows.append({
            "event_id": event_id,
            "label": int(row["label"]),
            "attack_family": str(row["attack_family"]),
            "baseline_score": float(baseline_scores[row_index]),
            "gnn_score": gnn_scores[event_id],
            "baseline_feature_contributions": json.dumps(baseline_values, sort_keys=True),
            "baseline_bias": float(contributions[row_index, -1]),
            "gnn_feature_occlusion": json.dumps(gnn_features[event_id], sort_keys=True),
            "gnn_ranked_one_hop_paths": json.dumps(gnn_paths[event_id], sort_keys=True),
        })
    output_root = Path(output_dir).resolve()
    explanations_path = output_root / "event-explanations.parquet"
    card_path = output_root / "model-card.json"
    manifest_path = output_root / "explanation-manifest.json"
    if any(path.exists() for path in (explanations_path, card_path, manifest_path)):
        raise FileExistsError("refusing to overwrite immutable explanation artifact")
    _write_parquet_exclusive(explanations_path, pd.DataFrame(rows))
    model_card = {
        "schema_version": 1,
        "state": "engineering_candidate_not_activated",
        "intended_use": "tenant-scoped traffic attack candidate scoring with analyst review",
        "prohibited_use": [
            "automatic blocking or legal attribution without independent review",
            "cross-tenant inference",
            "CNAS or production quality claims from internal fixtures",
        ],
        "dataset_id": dataset["dataset_id"],
        "baseline_training_run_sha256": baseline_training["run_sha256"],
        "gnn_training_run_sha256": gnn_training["run_sha256"],
        "evaluation_sha256": evaluation["evaluation_sha256"],
        "graph_snapshot": dataset["graph_snapshot"],
        "calibration": evaluation["calibration"],
        "observed_internal_metrics": {
            "known_attack_recall": evaluation["metrics"]["known_attack_recall"],
            "normal_false_positive_rate": evaluation["metrics"]["normal_false_positive_rate"],
            "unknown_recall": evaluation["metrics"]["unknown_recall"],
            "graph_ablation_state": evaluation["graph_ablations"]["state"],
        },
        "limitations": LIMITATIONS,
        "activation_authorized": False,
    }
    write_json_exclusive(card_path, model_card)
    meaning = {
        "dataset_id": dataset["dataset_id"],
        "dataset_sha256": dataset["dataset_sha256"],
        "baseline_training_run_sha256": baseline_training["run_sha256"],
        "gnn_training_run_sha256": gnn_training["run_sha256"],
        "evaluation_id": evaluation["evaluation_id"],
        "evaluation_sha256": evaluation["evaluation_sha256"],
        "graph_snapshot": dataset["graph_snapshot"],
        "baseline_model_sha256": baseline_training["artifacts"]["model"],
        "gnn_model_sha256": gnn_training["artifacts"]["gnn_full_model"],
        "explainer_code_sha256": sha256_file(__file__),
        "explainer_image_digest": image_digest,
        "population": {
            "split": "test",
            "available": len(test),
            "explained": len(selected_events),
            "event_ids_sha256": canonical_json_sha256(selected_events),
        },
        "methods": {
            "baseline": "xgboost_pred_contribs",
            "gnn_node": "feature_mean_occlusion",
            "gnn_edge": "single_edge_occlusion",
            "gnn_path": "ranked_one_hop_evidence_edges",
        },
        "calibration": evaluation["calibration"],
        "limitations": LIMITATIONS,
        "artifacts": {
            "event_explanations": sha256_file(explanations_path),
            "model_card": sha256_file(card_path),
        },
        "activation_authorized": False,
    }
    explanation_sha = canonical_json_sha256(meaning)
    manifest = {
        "schema_version": 1,
        "explanation_id": str(uuid.uuid5(EXPLANATION_NAMESPACE, explanation_sha)),
        "state": "explained",
        **meaning,
        "explanation_sha256": explanation_sha,
    }
    write_json_exclusive(manifest_path, manifest)
    return manifest
