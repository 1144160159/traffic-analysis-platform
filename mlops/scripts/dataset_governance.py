#!/usr/bin/env python3
"""Deterministic dataset scope, split-isolation and manifest primitives.

This module is the M08 v1 path.  It is intentionally separate from the legacy
random stratified split so existing callers keep working until the governed
workflow is explicitly enabled.
"""

from __future__ import annotations

import hashlib
import json
import math
import os
import re
import uuid
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable, Mapping, Sequence

import numpy as np
import pandas as pd


UTC = timezone.utc
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
DEFAULT_ISOLATION_COLUMNS = ("object_id", "community_id", "site_id", "pcap_id")
DEFAULT_ENTITY_COLUMNS = ("entity_id",)
DEFAULT_GRAPH_SET_COLUMNS = ("graph_node_ids", "graph_edge_ids")
MANIFEST_NAMESPACE = uuid.UUID("8261edfd-3354-51b9-9f1e-a97fa582115c")


def parse_utc(value: str | datetime, field: str) -> datetime:
    if isinstance(value, datetime):
        parsed = value
    else:
        text = str(value).strip()
        if text.endswith("Z"):
            text = text[:-1] + "+00:00"
        try:
            parsed = datetime.fromisoformat(text)
        except ValueError as exc:
            raise ValueError(f"{field} must be RFC3339") from exc
    if parsed.tzinfo is None:
        raise ValueError(f"{field} must include a timezone")
    return parsed.astimezone(UTC)


def rfc3339(value: datetime) -> str:
    return value.astimezone(UTC).isoformat(timespec="milliseconds").replace("+00:00", "Z")


@dataclass(frozen=True)
class ExtractionScope:
    tenant_id: str
    feature_set_id: str
    window_from: datetime
    window_through: datetime
    as_of: datetime
    source_watermark: datetime
    max_rows: int

    @classmethod
    def create(
        cls,
        *,
        tenant_id: str,
        feature_set_id: str,
        window_from: str | datetime,
        window_through: str | datetime,
        as_of: str | datetime,
        source_watermark: str | datetime,
        max_rows: int,
    ) -> "ExtractionScope":
        scope = cls(
            tenant_id=str(tenant_id).strip(),
            feature_set_id=str(feature_set_id).strip(),
            window_from=parse_utc(window_from, "window_from"),
            window_through=parse_utc(window_through, "window_through"),
            as_of=parse_utc(as_of, "as_of"),
            source_watermark=parse_utc(source_watermark, "source_watermark"),
            max_rows=int(max_rows),
        )
        scope.validate()
        return scope

    def validate(self) -> None:
        if not self.tenant_id or self.tenant_id.lower() == "unknown":
            raise ValueError("tenant_id is required and cannot be unknown")
        if not self.feature_set_id:
            raise ValueError("feature_set_id is required")
        if not self.window_from < self.window_through:
            raise ValueError("window_from must be before window_through")
        if self.window_through > self.source_watermark:
            raise ValueError("window_through exceeds the source watermark")
        if self.source_watermark > self.as_of:
            raise ValueError("source watermark exceeds as_of")
        if self.max_rows < 1 or self.max_rows > 1_000_000:
            raise ValueError("max_rows must be between 1 and 1000000")

    def manifest_scope(self) -> dict[str, Any]:
        return {
            "window_from": rfc3339(self.window_from),
            "window_through": rfc3339(self.window_through),
            "as_of": rfc3339(self.as_of),
            "source_watermark": rfc3339(self.source_watermark),
            "max_rows": self.max_rows,
        }


def _normalise_scalar(value: Any) -> Any:
    if value is None:
        return None
    if isinstance(value, np.generic):
        value = value.item()
    if isinstance(value, (pd.Timestamp, datetime)):
        timestamp = value.to_pydatetime() if isinstance(value, pd.Timestamp) else value
        if timestamp.tzinfo is None:
            timestamp = timestamp.replace(tzinfo=UTC)
        return rfc3339(timestamp)
    if isinstance(value, float):
        if math.isnan(value) or math.isinf(value):
            raise ValueError("canonical dataset rows cannot contain NaN or infinity")
        return format(value, ".17g")
    if isinstance(value, (str, int, bool)):
        return value
    if isinstance(value, (list, tuple, set)):
        return sorted((_normalise_scalar(item) for item in value), key=lambda item: json.dumps(item, sort_keys=True))
    if pd.isna(value):
        return None
    return str(value)


def sha256_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def sha256_file(path: str | os.PathLike[str]) -> str:
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def canonical_json_sha256(value: Any) -> str:
    payload = json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False,
                         allow_nan=False).encode("utf-8")
    return sha256_bytes(payload)


def _normalise_column_values(series: pd.Series) -> list[Any]:
    """Pre-normalize a whole column for canonical hashing.

    Fast paths for common dtypes produce exactly the same values as the
    per-cell ``_normalise_scalar``; object/mixed columns fall back to it.
    This keeps ``canonical_rows_sha256`` byte-identical to the reference
    implementation while removing per-cell dispatch for numeric columns
    (代码审查 H40 收敛项：向量化哈希避免百万行级逐格 Python 开销).
    """
    dtype = series.dtype
    if pd.api.types.is_datetime64_any_dtype(dtype) or isinstance(dtype, pd.DatetimeTZDtype):
        out: list[Any] = []
        for value in series:
            if pd.isna(value):
                out.append(None)
                continue
            timestamp = value.to_pydatetime() if hasattr(value, "to_pydatetime") else value
            if timestamp.tzinfo is None:
                timestamp = timestamp.replace(tzinfo=UTC)
            out.append(rfc3339(timestamp))
        return out
    if pd.api.types.is_bool_dtype(dtype):
        return [bool(value) if not pd.isna(value) else None for value in series]
    if pd.api.types.is_integer_dtype(dtype):
        return [int(value) if not pd.isna(value) else None for value in series]
    if pd.api.types.is_float_dtype(dtype):
        out = []
        for value in series:
            if pd.isna(value):
                out.append(None)
                continue
            scalar = value.item() if isinstance(value, np.generic) else value
            if math.isnan(scalar) or math.isinf(scalar):
                raise ValueError("canonical dataset rows cannot contain NaN or infinity")
            out.append(format(scalar, ".17g"))
        return out
    return [_normalise_scalar(value) for value in series]


def canonical_rows_sha256(df: pd.DataFrame) -> str:
    if "event_id" not in df.columns:
        raise ValueError("event_id is required for canonical row identity")
    if df["event_id"].astype(str).duplicated().any():
        raise ValueError("duplicate event_id in canonical dataset")
    columns = sorted(str(column) for column in df.columns)
    ordered = df.assign(event_id=df["event_id"].astype(str)).sort_values("event_id", kind="mergesort")
    # Vectorized per-column normalization; the per-row canonical JSON encoding
    # itself is preserved exactly (length-prefixed, sorted keys, separators).
    normalized = {column: _normalise_column_values(ordered[column]) for column in columns}
    digest = hashlib.sha256()
    for row in zip(*(normalized[column] for column in columns)):
        canonical = dict(zip(columns, row))
        encoded = json.dumps(canonical, sort_keys=True, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
        digest.update(len(encoded).to_bytes(8, "big"))
        digest.update(encoded)
    return digest.hexdigest()


def validate_extracted_frame(df: pd.DataFrame, scope: ExtractionScope) -> None:
    required = {"tenant_id", "event_id", "feature_set_id", "ts", "ingest_ts", "label"}
    missing = sorted(required - set(df.columns))
    if missing:
        raise ValueError(f"governed dataset is missing columns: {missing}")
    if df.empty:
        raise ValueError("governed dataset cannot be empty")
    if len(df) > scope.max_rows:
        raise ValueError(f"extraction row budget exceeded: {len(df)} > {scope.max_rows}")
    tenants = set(df["tenant_id"].astype(str).str.strip())
    if tenants != {scope.tenant_id}:
        raise ValueError(f"dataset tenant drift: {sorted(tenants)}")
    feature_sets = set(df["feature_set_id"].astype(str).str.strip())
    if feature_sets != {scope.feature_set_id}:
        raise ValueError(f"dataset feature-set drift: {sorted(feature_sets)}")
    if df["event_id"].astype(str).str.strip().eq("").any() or df["event_id"].astype(str).duplicated().any():
        raise ValueError("event_id must be non-empty and unique")
    event_time = pd.to_datetime(df["ts"], unit="s", utc=True)
    ingest_time = pd.to_datetime(df["ingest_ts"], unit="s", utc=True)
    if (event_time < scope.window_from).any() or (event_time >= scope.window_through).any():
        raise ValueError("dataset contains event time outside the immutable window")
    if (event_time > scope.source_watermark).any():
        raise ValueError("dataset contains event time after the source watermark")
    if (ingest_time > scope.as_of).any():
        raise ValueError("dataset contains a row ingested after as_of")


def _tokens(value: Any) -> set[str]:
    if value is None or (isinstance(value, float) and math.isnan(value)):
        return set()
    if isinstance(value, str):
        text = value.strip()
        if not text:
            return set()
        if text.startswith("["):
            try:
                value = json.loads(text)
            except json.JSONDecodeError as exc:
                raise ValueError("graph identity set must be valid JSON") from exc
        else:
            return {text}
    if isinstance(value, (list, tuple, set, np.ndarray)):
        return {str(item).strip() for item in value if str(item).strip()}
    return {str(value).strip()}


def _column_tokens(series: pd.Series, *, require_non_empty: bool = True) -> set[str]:
    """Token set of one isolation/entity column with an empty-token guard.

    Fast path for plain scalar strings; falls back to per-row ``_tokens`` for
    JSON list columns (graph identities) so semantics are unchanged
    (代码审查 H40 收敛项：隔离校验向量化，避免逐行 iterrows 开销).
    ``require_non_empty=False`` preserves the legacy graph-column behavior of
    silently skipping empty token sets (empty graph adjacency lists are valid).
    """
    values = series.tolist()
    if any(isinstance(value, str) and value.strip().startswith("[") for value in values):
        tokens: set[str] = set()
        for value in values:
            row_tokens = _tokens(value)
            if not row_tokens:
                if require_non_empty:
                    raise ValueError("isolation column contains an empty token")
                continue
            tokens |= row_tokens
        return tokens
    if require_non_empty:
        for value in values:
            if value is None or (isinstance(value, float) and math.isnan(value)):
                raise ValueError("isolation column contains an empty token")
            if not str(value).strip():
                raise ValueError("isolation column contains an empty token")
    return {str(value).strip() for value in values if str(value).strip()}


def validate_split_isolation(
    splits: Mapping[str, pd.DataFrame],
    *,
    isolation_columns: Sequence[str] = DEFAULT_ISOLATION_COLUMNS,
    entity_columns: Sequence[str] = DEFAULT_ENTITY_COLUMNS,
    graph_set_columns: Sequence[str] = DEFAULT_GRAPH_SET_COLUMNS,
    holdout_attack_families: Iterable[str] = (),
) -> None:
    if len(splits) < 2 or any(frame.empty for frame in splits.values()):
        raise ValueError("at least two non-empty splits are required")
    all_columns = set.intersection(*(set(frame.columns) for frame in splits.values()))
    required = set(isolation_columns) | {"event_id", "attack_family"}
    if not set(entity_columns) & all_columns:
        required.add(next(iter(entity_columns), "entity_id"))
    missing = sorted(required - all_columns)
    if missing:
        raise ValueError(f"leakage isolation columns are missing: {missing}")

    observed: dict[tuple[str, str], str] = {}
    token_columns = tuple(graph_set_columns)
    for split_name, frame in splits.items():
        if frame["event_id"].astype(str).duplicated().any():
            raise ValueError(f"duplicate event_id inside split {split_name}")
        for column in tuple(isolation_columns) + tuple(column for column in entity_columns if column in all_columns):
            values = _column_tokens(frame[column])
            if not values:
                raise ValueError(f"{column} is empty in governed split {split_name}")
            for value in values:
                key = (column, value)
                owner = observed.setdefault(key, split_name)
                if owner != split_name:
                    raise ValueError(f"leakage: {column}={value} appears in {owner} and {split_name}")
            for column in token_columns:
                if column not in all_columns:
                    continue
                for value in _column_tokens(frame[column], require_non_empty=False):
                    key = (column, value)
                    owner = observed.setdefault(key, split_name)
                    if owner != split_name:
                        raise ValueError(f"graph leakage: {column}={value} appears in {owner} and {split_name}")

    holdouts = {str(value).strip() for value in holdout_attack_families if str(value).strip()}
    for family in holdouts:
        owners = {name for name, frame in splits.items() if family in set(frame["attack_family"].astype(str))}
        if owners != {"open_set"}:
            raise ValueError(f"holdout attack family {family} must appear only in open_set, got {sorted(owners)}")


def split_by_time_and_validate(
    df: pd.DataFrame,
    *,
    train_through: str | datetime,
    validation_through: str | datetime,
    holdout_attack_families: Iterable[str] = (),
) -> dict[str, pd.DataFrame]:
    train_cutoff = parse_utc(train_through, "train_through")
    validation_cutoff = parse_utc(validation_through, "validation_through")
    if not train_cutoff < validation_cutoff:
        raise ValueError("train_through must be before validation_through")
    if "ts" not in df.columns or "attack_family" not in df.columns:
        raise ValueError("ts and attack_family are required for governed splitting")
    timestamps = pd.to_datetime(df["ts"], unit="s", utc=True)
    holdouts = {str(value).strip() for value in holdout_attack_families if str(value).strip()}
    is_open = df["attack_family"].astype(str).isin(holdouts)
    splits = {
        "train": df[(timestamps < train_cutoff) & ~is_open].copy(),
        "validation": df[(timestamps >= train_cutoff) & (timestamps < validation_cutoff) & ~is_open].copy(),
        "test": df[(timestamps >= validation_cutoff) & ~is_open].copy(),
    }
    if is_open.any():
        splits["open_set"] = df[is_open].copy()
    validate_split_isolation(splits, holdout_attack_families=holdouts)
    return {name: frame.sort_values(["ts", "event_id"], kind="mergesort").reset_index(drop=True)
            for name, frame in splits.items()}


def build_dataset_manifest(
    *,
    scope: ExtractionScope,
    source_frame: pd.DataFrame,
    splits: Mapping[str, pd.DataFrame],
    feature_columns: Sequence[str],
    artifact_paths: Mapping[str, str | os.PathLike[str]],
    label_schema_version: str,
    label_revision_sha256: str,
    graph_snapshot: Mapping[str, str] | None = None,
) -> dict[str, Any]:
    if not SHA256_RE.fullmatch(label_revision_sha256):
        raise ValueError("label_revision_sha256 must be lowercase SHA-256")
    validate_extracted_frame(source_frame, scope)
    validate_split_isolation(splits)
    source_sha = canonical_rows_sha256(source_frame)
    ordered_features = sorted(set(str(column) for column in feature_columns))
    if not ordered_features or set(ordered_features) - set(source_frame.columns):
        raise ValueError("feature columns are empty or absent from the dataset")
    artifacts = {name: sha256_file(path) for name, path in sorted(artifact_paths.items())}
    split_manifest = {
        name: {
            "row_count": len(frame),
            "event_ids_sha256": canonical_json_sha256(sorted(frame["event_id"].astype(str))),
        }
        for name, frame in sorted(splits.items())
    }
    meaning = {
        "tenant_id": scope.tenant_id,
        "feature_set_id": scope.feature_set_id,
        "scope": scope.manifest_scope(),
        "source_rows_sha256": source_sha,
        "splits": split_manifest,
        "features_sha256": canonical_json_sha256(ordered_features),
        "label_revision_sha256": label_revision_sha256,
        "graph_snapshot": graph_snapshot,
    }
    dataset_sha = canonical_json_sha256(meaning)
    dataset_id = str(uuid.uuid5(MANIFEST_NAMESPACE, dataset_sha))
    return {
        "schema_version": 1,
        "dataset_id": dataset_id,
        "tenant_id": scope.tenant_id,
        "feature_set_id": scope.feature_set_id,
        "scope": scope.manifest_scope(),
        "source": {
            "tables": ["traffic.feature_stat", "traffic.alerts", "traffic.alert_feedback"],
            "row_count": len(source_frame),
            "canonical_rows_sha256": source_sha,
        },
        "labels": {
            "schema_version": label_schema_version,
            "revision_sha256": label_revision_sha256,
            "classes": sorted(_normalise_scalar(value) for value in source_frame["label"].unique()),
        },
        "features": {
            "columns": ordered_features,
            "columns_sha256": canonical_json_sha256(ordered_features),
        },
        "graph_snapshot": dict(graph_snapshot) if graph_snapshot else None,
        "splits": split_manifest,
        "artifacts": artifacts,
        "dataset_sha256": dataset_sha,
    }


def validate_dataset_manifest_identity(manifest: Mapping[str, Any]) -> None:
    try:
        meaning = {
            "tenant_id": manifest["tenant_id"],
            "feature_set_id": manifest["feature_set_id"],
            "scope": manifest["scope"],
            "source_rows_sha256": manifest["source"]["canonical_rows_sha256"],
            "splits": manifest["splits"],
            "features_sha256": manifest["features"]["columns_sha256"],
            "label_revision_sha256": manifest["labels"]["revision_sha256"],
            "graph_snapshot": manifest.get("graph_snapshot"),
        }
        expected_sha = canonical_json_sha256(meaning)
        expected_id = str(uuid.uuid5(MANIFEST_NAMESPACE, expected_sha))
    except (KeyError, TypeError) as exc:
        raise ValueError("dataset manifest is structurally incomplete") from exc
    if manifest.get("schema_version") != 1 or manifest.get("dataset_sha256") != expected_sha:
        raise ValueError("dataset manifest meaning hash mismatch")
    if manifest.get("dataset_id") != expected_id:
        raise ValueError("dataset manifest deterministic identity mismatch")
    if not SHA256_RE.fullmatch(str(manifest["source"]["canonical_rows_sha256"])):
        raise ValueError("dataset manifest source hash is invalid")
    if not SHA256_RE.fullmatch(str(manifest["features"]["columns_sha256"])):
        raise ValueError("dataset manifest feature hash is invalid")


def write_json_exclusive(path: str | os.PathLike[str], value: Any) -> None:
    target = Path(path)
    target.parent.mkdir(parents=True, exist_ok=True)
    payload = (json.dumps(value, indent=2, sort_keys=True, ensure_ascii=False) + "\n").encode("utf-8")
    descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o640)
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(payload)
            handle.flush()
            os.fsync(handle.fileno())
    except Exception:
        try:
            target.unlink()
        except FileNotFoundError:
            pass
        raise
