#!/usr/bin/env python3
"""Deterministic two-layer graph convolution training and ablation suite."""

from __future__ import annotations

import json
import os
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable

import numpy as np
import pandas as pd
from scipy import sparse

from dataset_governance import parse_utc, sha256_file, write_json_exclusive
from graph_governance import validate_graph_snapshot_manifest
from train_model import build_training_run_manifest, load_governed_training_data


VARIANTS = ("gnn_full", "gnn_no_edges", "gnn_no_sources")


def required_env(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise ValueError(f"{name} is required when MLOPS_GNN_TRAINING_V1_ENABLED=true")
    return value


def _tokens(value: Any) -> set[str]:
    if value is None or (isinstance(value, float) and np.isnan(value)):
        return set()
    if isinstance(value, str):
        text = value.strip()
        if not text:
            return set()
        if text.startswith("["):
            parsed = json.loads(text)
            return {str(item).strip() for item in parsed if str(item).strip()}
        return {text}
    if isinstance(value, (list, tuple, set, np.ndarray)):
        return {str(item).strip() for item in value if str(item).strip()}
    return {str(value).strip()}


@dataclass(frozen=True)
class GraphSplit:
    name: str
    node_ids: tuple[str, ...]
    event_ids: tuple[str, ...]
    features: np.ndarray
    labels: np.ndarray
    edges: pd.DataFrame


@dataclass(frozen=True)
class GraphTrainingBundle:
    splits: dict[str, GraphSplit]
    feature_columns: tuple[str, ...]
    dataset_manifest: dict[str, Any]
    graph_manifest: dict[str, Any]


@dataclass
class GCNModel:
    feature_mean: np.ndarray
    feature_std: np.ndarray
    w0: np.ndarray
    b0: np.ndarray
    w1: np.ndarray
    b1: np.ndarray
    best_epoch: int
    validation_loss: float

    def predict_scores(self, split: GraphSplit, edges: pd.DataFrame) -> np.ndarray:
        x = (split.features - self.feature_mean) / self.feature_std
        adjacency = normalized_adjacency(split.node_ids, edges)
        hidden = np.maximum(adjacency @ x @ self.w0 + self.b0, 0.0)
        logits = adjacency @ hidden @ self.w1 + self.b1
        return _sigmoid(np.asarray(logits).reshape(-1))

    def save_exclusive(self, path: Path) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        if path.exists():
            raise FileExistsError(f"refusing to overwrite immutable GNN artifact: {path}")
        temporary = path.with_name(f".{path.name}.{uuid.uuid4().hex}.npz")
        try:
            np.savez(
                temporary,
                feature_mean=self.feature_mean,
                feature_std=self.feature_std,
                w0=self.w0,
                b0=self.b0,
                w1=self.w1,
                b1=self.b1,
                best_epoch=np.asarray([self.best_epoch], dtype=np.int64),
                validation_loss=np.asarray([self.validation_loss], dtype=np.float64),
            )
            os.link(temporary, path)
        finally:
            try:
                temporary.unlink()
            except FileNotFoundError:
                pass


class SuiteDescriptor:
    def __init__(self, parameters: dict[str, Any]):
        self.parameters = parameters

    def get_params(self) -> dict[str, Any]:
        return dict(self.parameters)


def _sigmoid(values: np.ndarray) -> np.ndarray:
    clipped = np.clip(values, -40.0, 40.0)
    return 1.0 / (1.0 + np.exp(-clipped))


def normalized_adjacency(node_ids: Iterable[str], edges: pd.DataFrame) -> sparse.csr_matrix:
    ordered = tuple(node_ids)
    if not ordered:
        raise ValueError("cannot build adjacency for an empty node set")
    index = {node_id: offset for offset, node_id in enumerate(ordered)}
    rows, columns, weights = list(range(len(ordered))), list(range(len(ordered))), [1.0] * len(ordered)
    for row in edges.itertuples(index=False):
        source, target = str(row.source_node_id), str(row.target_node_id)
        if source not in index or target not in index:
            raise ValueError("split adjacency contains a cross-split or unknown endpoint")
        if source == target:
            continue
        rows.extend((index[source], index[target]))
        columns.extend((index[target], index[source]))
        weights.extend((1.0, 1.0))
    adjacency = sparse.coo_matrix((weights, (rows, columns)), shape=(len(ordered), len(ordered))).tocsr()
    adjacency.sum_duplicates()
    adjacency.data[:] = 1.0
    degree = np.asarray(adjacency.sum(axis=1)).reshape(-1)
    inverse = np.zeros_like(degree, dtype=float)
    np.power(degree, -0.5, out=inverse, where=degree > 0)
    scale = sparse.diags(inverse)
    return (scale @ adjacency @ scale).tocsr()


def _variant_edges(edges: pd.DataFrame, variant: str, removed_sources: set[str]) -> pd.DataFrame:
    if variant == "gnn_full":
        return edges
    if variant == "gnn_no_edges":
        return edges.iloc[0:0].copy()
    if variant == "gnn_no_sources":
        return edges[~edges["source_kind"].astype(str).isin(removed_sources)].copy()
    raise ValueError(f"unknown GNN variant: {variant}")


def load_graph_training_bundle(data_dir: str, graph_dir: str) -> GraphTrainingBundle:
    data_root, graph_root = Path(data_dir).resolve(), Path(graph_dir).resolve()
    _, _, _, _, _, dataset = load_governed_training_data(str(data_root))
    graph_binding = dataset.get("graph_snapshot")
    if not isinstance(graph_binding, dict):
        raise ValueError("GNN training requires a graph snapshot bound by the dataset manifest")
    manifest_path = graph_root / "graph-snapshot-manifest.json"
    with manifest_path.open("r", encoding="utf-8") as handle:
        graph_manifest = json.load(handle)
    if graph_binding.get("snapshot_id") != graph_manifest.get("snapshot_id") or \
            graph_binding.get("manifest_sha256") != sha256_file(manifest_path):
        raise ValueError("dataset graph binding does not match the graph snapshot artifact")
    if graph_manifest.get("tenant_id") != dataset.get("tenant_id"):
        raise ValueError("graph snapshot tenant does not match dataset tenant")
    if parse_utc(graph_manifest.get("as_of", ""), "graph.as_of") > \
            parse_utc(dataset["scope"]["as_of"], "dataset.as_of"):
        raise ValueError("graph snapshot reads future data after dataset as_of")
    nodes, edges, graph_features = validate_graph_snapshot_manifest(
        graph_manifest, graph_root / "nodes.parquet", graph_root / "edges.parquet"
    )
    frames = {
        name: pd.read_parquet(data_root / f"{name}.parquet", engine="pyarrow")
        for name in dataset["splits"]
    }
    event_owner: dict[str, str] = {}
    event_label: dict[str, int] = {}
    dataset_node_refs: dict[str, set[str]] = {}
    dataset_edge_refs: dict[str, set[str]] = {}
    for split_name, frame in frames.items():
        for row in frame.itertuples(index=False):
            event_id = str(row.event_id)
            if event_id in event_owner:
                raise ValueError("event identity appears in more than one graph split")
            event_owner[event_id] = split_name
            event_label[event_id] = int(row.label)
            dataset_node_refs[event_id] = _tokens(getattr(row, "graph_node_ids", None))
            dataset_edge_refs[event_id] = _tokens(getattr(row, "graph_edge_ids", None))
    if set(nodes["event_id"].astype(str)) != set(event_owner):
        raise ValueError("graph nodes must cover exactly the governed dataset event identities")
    node_for_event = dict(zip(nodes["event_id"].astype(str), nodes["node_id"].astype(str)))
    for event_id, node_id in node_for_event.items():
        if dataset_node_refs[event_id] != {node_id}:
            raise ValueError("dataset graph_node_ids do not bind the exact training node")
    node_owner = {
        str(row.node_id): event_owner[str(row.event_id)] for row in nodes.itertuples(index=False)
    }
    edge_owner: dict[str, str] = {}
    for row in edges.itertuples(index=False):
        source_owner = node_owner[str(row.source_node_id)]
        target_owner = node_owner[str(row.target_node_id)]
        if source_owner != target_owner:
            raise ValueError("graph edge crosses governed train/validation/test/open_set splits")
        edge_owner[str(row.edge_id)] = source_owner
    referenced_edges: dict[str, str] = {}
    for event_id, edge_ids in dataset_edge_refs.items():
        for edge_id in edge_ids:
            if edge_id not in edge_owner or edge_owner[edge_id] != event_owner[event_id]:
                raise ValueError("dataset graph_edge_ids reference an absent or cross-split edge")
            previous = referenced_edges.setdefault(edge_id, event_id)
            if previous != event_id:
                raise ValueError("graph edge identity is assigned to multiple dataset events")
    if set(edge_owner) != set(referenced_edges):
        raise ValueError("graph snapshot contains an edge absent from dataset graph_edge_ids")

    graph_splits: dict[str, GraphSplit] = {}
    for split_name in frames:
        split_nodes = nodes[nodes["event_id"].astype(str).map(event_owner) == split_name].copy()
        split_nodes = split_nodes.sort_values("node_id", kind="mergesort").reset_index(drop=True)
        node_ids = tuple(split_nodes["node_id"].astype(str))
        event_ids = tuple(split_nodes["event_id"].astype(str))
        split_edges = edges[edges["edge_id"].astype(str).map(edge_owner) == split_name].copy()
        graph_splits[split_name] = GraphSplit(
            name=split_name,
            node_ids=node_ids,
            event_ids=event_ids,
            features=split_nodes[graph_features].to_numpy(dtype=float),
            labels=np.asarray([event_label[event_id] for event_id in event_ids], dtype=float),
            edges=split_edges,
        )
    required = {"train", "validation", "test", "open_set"}
    if required - set(graph_splits):
        raise ValueError("graph training bundle is missing a governed split")
    if set(np.unique(graph_splits["train"].labels)) != {0.0, 1.0} or \
            set(np.unique(graph_splits["validation"].labels)) != {0.0, 1.0}:
        raise ValueError("GNN train and validation splits require both labels")
    return GraphTrainingBundle(
        splits=graph_splits,
        feature_columns=tuple(graph_features),
        dataset_manifest=dataset,
        graph_manifest=graph_manifest,
    )


def _loss(labels: np.ndarray, scores: np.ndarray, positive_weight: float) -> float:
    p = np.clip(scores, 1e-8, 1.0 - 1e-8)
    weights = np.where(labels == 1.0, positive_weight, 1.0)
    return float(np.mean(weights * (-(labels * np.log(p) + (1.0 - labels) * np.log(1.0 - p)))))


def train_gcn_variant(
    train: GraphSplit,
    validation: GraphSplit,
    *,
    variant: str,
    removed_sources: set[str],
    seed: int,
    hidden_size: int,
    epochs: int,
    learning_rate: float,
    l2: float,
    patience: int,
) -> GCNModel:
    if seed < 0 or hidden_size < 2 or epochs < 1 or learning_rate <= 0 or l2 < 0 or patience < 1:
        raise ValueError("invalid governed GCN hyperparameters")
    feature_mean = train.features.mean(axis=0)
    feature_std = train.features.std(axis=0)
    feature_std = np.where(feature_std < 1e-8, 1.0, feature_std)
    train_x = (train.features - feature_mean) / feature_std
    validation_x = (validation.features - feature_mean) / feature_std
    train_adj = normalized_adjacency(train.node_ids, _variant_edges(train.edges, variant, removed_sources))
    validation_edges = _variant_edges(validation.edges, variant, removed_sources)
    validation_adj = normalized_adjacency(validation.node_ids, validation_edges)
    rng = np.random.default_rng(seed)
    input_size = train_x.shape[1]
    params = {
        "w0": rng.normal(0.0, np.sqrt(2.0 / max(1, input_size)), (input_size, hidden_size)),
        "b0": np.zeros(hidden_size),
        "w1": rng.normal(0.0, np.sqrt(2.0 / hidden_size), (hidden_size, 1)),
        "b1": np.zeros(1),
    }
    first = {key: np.zeros_like(value) for key, value in params.items()}
    second = {key: np.zeros_like(value) for key, value in params.items()}
    positive = float((train.labels == 1.0).sum())
    positive_weight = float((train.labels == 0.0).sum() / positive) if positive else 1.0
    best_loss, best_epoch, stale = float("inf"), -1, 0
    best = {key: value.copy() for key, value in params.items()}
    train_projected = train_adj @ train_x
    validation_projected = validation_adj @ validation_x
    for epoch in range(epochs):
        hidden_pre = train_projected @ params["w0"] + params["b0"]
        hidden = np.maximum(hidden_pre, 0.0)
        aggregated = train_adj @ hidden
        logits = np.asarray(aggregated @ params["w1"] + params["b1"]).reshape(-1)
        scores = _sigmoid(logits)
        sample_weights = np.where(train.labels == 1.0, positive_weight, 1.0)
        grad_logits = sample_weights * (scores - train.labels) / len(train.labels)
        gradients = {
            "w1": np.asarray(aggregated).T @ grad_logits[:, None] + l2 * params["w1"],
            "b1": np.asarray([grad_logits.sum()]),
        }
        grad_hidden = train_adj.T @ (grad_logits[:, None] @ params["w1"].T)
        grad_pre = np.asarray(grad_hidden) * (hidden_pre > 0.0)
        gradients["w0"] = np.asarray(train_projected).T @ grad_pre + l2 * params["w0"]
        gradients["b0"] = grad_pre.sum(axis=0)
        step = epoch + 1
        for key in params:
            first[key] = 0.9 * first[key] + 0.1 * gradients[key]
            second[key] = 0.999 * second[key] + 0.001 * gradients[key] ** 2
            first_hat = first[key] / (1.0 - 0.9 ** step)
            second_hat = second[key] / (1.0 - 0.999 ** step)
            params[key] -= learning_rate * first_hat / (np.sqrt(second_hat) + 1e-8)

        val_hidden = np.maximum(validation_projected @ params["w0"] + params["b0"], 0.0)
        val_scores = _sigmoid(np.asarray(validation_adj @ val_hidden @ params["w1"] + params["b1"]).reshape(-1))
        val_loss = _loss(validation.labels, val_scores, positive_weight)
        if val_loss < best_loss - 1e-10:
            best_loss, best_epoch, stale = val_loss, epoch, 0
            best = {key: value.copy() for key, value in params.items()}
        else:
            stale += 1
            if stale >= patience:
                break
    return GCNModel(
        feature_mean=feature_mean,
        feature_std=feature_std,
        w0=best["w0"], b0=best["b0"], w1=best["w1"], b1=best["b1"],
        best_epoch=best_epoch, validation_loss=float(best_loss),
    )


def _write_predictions_exclusive(path: Path, split: GraphSplit, scores: np.ndarray) -> None:
    if path.exists():
        raise FileExistsError(f"refusing to overwrite immutable GNN prediction artifact: {path}")
    temporary = path.with_name(f".{path.name}.{uuid.uuid4().hex}.tmp")
    try:
        pd.DataFrame({
            "event_id": split.event_ids,
            "label": split.labels.astype(int),
            "score": scores,
        }).to_parquet(temporary, index=False, engine="pyarrow")
        os.link(temporary, path)
    finally:
        try:
            temporary.unlink()
        except FileNotFoundError:
            pass


def run_governed_gnn_training(data_dir: str, graph_dir: str, output_dir: str) -> dict[str, Any]:
    bundle = load_graph_training_bundle(data_dir, graph_dir)
    seed = int(required_env("GNN_TRAIN_SEED"))
    hidden_size = int(required_env("GNN_HIDDEN_SIZE"))
    epochs = int(required_env("GNN_EPOCHS"))
    learning_rate = float(required_env("GNN_LEARNING_RATE"))
    l2 = float(required_env("GNN_L2"))
    patience = int(required_env("GNN_PATIENCE"))
    removed_sources = {
        item.strip() for item in required_env("GNN_SOURCE_ABLATION_KINDS").split(",") if item.strip()
    }
    if not removed_sources:
        raise ValueError("GNN_SOURCE_ABLATION_KINDS must contain at least one source kind")
    observed_sources = set(pd.concat([
        split.edges["source_kind"] for split in bundle.splits.values() if not split.edges.empty
    ]).astype(str))
    if not removed_sources.issubset(observed_sources):
        raise ValueError("GNN source ablation kind is absent from the graph snapshot")
    output = Path(output_dir).resolve()
    output.mkdir(parents=True, exist_ok=True)
    manifest_path = output / "gnn-training-run-manifest.json"
    if manifest_path.exists():
        raise FileExistsError(f"refusing to overwrite immutable GNN artifact: {manifest_path}")
    artifacts: dict[str, str] = {}
    metrics: dict[str, Any] = {}
    models: dict[str, GCNModel] = {}
    for offset, variant in enumerate(VARIANTS):
        model = train_gcn_variant(
            bundle.splits["train"], bundle.splits["validation"],
            variant=variant, removed_sources=removed_sources,
            seed=seed + offset, hidden_size=hidden_size, epochs=epochs,
            learning_rate=learning_rate, l2=l2, patience=patience,
        )
        models[variant] = model
        model_path = output / f"{variant}-model.npz"
        prediction_path = output / f"{variant}.parquet"
        model.save_exclusive(model_path)
        scores = model.predict_scores(
            bundle.splits["test"],
            _variant_edges(bundle.splits["test"].edges, variant, removed_sources),
        )
        _write_predictions_exclusive(prediction_path, bundle.splits["test"], scores)
        artifacts[f"{variant}_model"] = str(model_path)
        artifacts[f"{variant}_predictions"] = str(prediction_path)
        metrics[variant] = {
            "best_epoch": model.best_epoch,
            "validation_loss": model.validation_loss,
        }
    suite_config = {
        "schema_version": 1,
        "graph_snapshot_id": bundle.graph_manifest["snapshot_id"],
        "graph_snapshot_sha256": bundle.graph_manifest["graph_snapshot_sha256"],
        "feature_columns": list(bundle.feature_columns),
        "variants": list(VARIANTS),
        "removed_source_kinds": sorted(removed_sources),
        "seed": seed,
        "hidden_size": hidden_size,
        "epochs": epochs,
        "learning_rate": learning_rate,
        "l2": l2,
        "patience": patience,
        "variant_metrics": metrics,
    }
    config_path = output / "gnn-suite-config.json"
    write_json_exclusive(config_path, suite_config)
    artifacts["gnn_suite_config"] = str(config_path)
    descriptor = SuiteDescriptor({
        key: suite_config[key] for key in (
            "graph_snapshot_id", "graph_snapshot_sha256", "feature_columns", "variants",
            "removed_source_kinds", "seed", "hidden_size", "epochs", "learning_rate", "l2", "patience",
        )
    })
    manifest = build_training_run_manifest(
        run_id=required_env("GNN_TRAIN_RUN_ID"),
        dataset_manifest=bundle.dataset_manifest,
        algorithm="gnn",
        seed=seed,
        model=descriptor,
        trainer_image_digest=required_env("TRAINER_IMAGE_DIGEST"),
        cpu_limit=required_env("TRAIN_CPU_LIMIT"),
        memory_limit=required_env("TRAIN_MEMORY_LIMIT"),
        gpu_limit=int(required_env("TRAIN_GPU_LIMIT")),
        artifact_paths=artifacts,
        code_path=__file__,
    )
    write_json_exclusive(manifest_path, manifest)
    return manifest


def main() -> None:
    enabled = os.getenv("MLOPS_GNN_TRAINING_V1_ENABLED", "").strip().lower()
    if enabled not in {"true", "false"}:
        raise ValueError("MLOPS_GNN_TRAINING_V1_ENABLED must be explicitly true or false")
    if enabled != "true":
        raise ValueError("GNN training entrypoint is default-off and requires explicit enablement")
    manifest = run_governed_gnn_training(
        required_env("DATA_DIR"), required_env("GRAPH_DIR"), required_env("OUTPUT_DIR")
    )
    print(json.dumps({
        "status": "PASS",
        "run_id": manifest["run_id"],
        "run_sha256": manifest["run_sha256"],
        "graph_snapshot": manifest["graph_snapshot"],
        "activation_authorized": False,
    }, sort_keys=True))


if __name__ == "__main__":
    main()
