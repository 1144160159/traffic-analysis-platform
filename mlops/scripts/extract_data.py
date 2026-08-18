#!/usr/bin/env python3
"""
数据提取脚本：从 ClickHouse 提取特征 + 标注数据
严格遵守 Protobuf 契约和 ClickHouse 表结构
"""

import os
import sys
import pandas as pd
import numpy as np
from clickhouse_driver import Client
from sklearn.model_selection import train_test_split
import logging
import json
from typing import Dict, Any

from dataset_governance import (
    ExtractionScope,
    build_dataset_manifest,
    parse_utc,
    sha256_file,
    split_by_time_and_validate,
    validate_extracted_frame,
    write_json_exclusive,
)

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def escape_clickhouse_string(value: str) -> str:
    """Escape a string literal for the ClickHouse SQL snippets below."""
    return value.replace("\\", "\\\\").replace("'", "\\'")


def connect_clickhouse():
    """连接 ClickHouse"""
    host = os.getenv('CLICKHOUSE_HOST', 'clickhouse')
    port = int(os.getenv('CLICKHOUSE_PORT', '9000'))
    database = os.getenv('CLICKHOUSE_DB', 'traffic')
    user = os.getenv('CLICKHOUSE_USER', 'default')
    password = os.getenv('CLICKHOUSE_PASSWORD', '')
    
    logger.info(f"Connecting to ClickHouse: {host}:{port}/{database}")
    
    try:
        client = Client(
            host=host,
            port=port,
            database=database,
            user=user,
            password=password,
            settings={'use_numpy': True},
            connect_timeout=30,
            send_receive_timeout=300,
        )
        
        # 测试连接
        result = client.execute('SELECT 1')
        logger.info("ClickHouse connection successful")
        
        return client
        
    except Exception as e:
        logger.error(f"Failed to connect to ClickHouse: {e}")
        raise


def extract_features_with_labels(client, feature_set_id, lookback_days, tenant_id):
    """
    提取特征数据 + 标注
    
    数据源（严格遵守 ClickHouse 表结构）：
    1. traffic.feature_stat (FeatureStat from proto/traffic/v1/feature.proto)
    2. traffic.alert_feedback (AlertFeedback from proto/traffic/v1/alert.proto)
    3. traffic.alerts (Alert from proto/traffic/v1/alert.proto)
    
    契约字段映射：
    - FeatureStat.header.event_id -> event_id
    - FeatureStat.header.tenant_id -> tenant_id
    - FeatureStat.community_id -> community_id
    - AlertFeedback.label -> 'TP'/'FP'/'TN'
    """
    
    tenant_escaped = escape_clickhouse_string(tenant_id)
    feature_set_escaped = escape_clickhouse_string(feature_set_id)
    lookback_days = int(lookback_days)
    # The distributed feature table can contain hundreds of millions of rows.
    # Sorting the entire lookback window before LIMIT caused ClickHouse to hit
    # the cluster-wide 16 GiB memory ceiling. Training is order-independent, so
    # cap an unsorted sample and constrain read parallelism instead.
    max_extract_rows = int(os.getenv('MAX_EXTRACT_ROWS', '100000'))
    max_extract_rows = max(1000, min(max_extract_rows, 1000000))

    query = f"""
        WITH labeled_alerts AS (
            -- 从 AlertFeedback 提取标注（对应 proto AlertFeedback message）
            SELECT
                af.tenant_id,
                af.alert_id,
                af.label,
                af.created_at AS ts,
                af.user_id,
                af.alert_type,
                af.model_version,
                af.rule_version
            FROM traffic.alert_feedback AS af
            WHERE af.tenant_id = '{tenant_escaped}'
              AND af.label IN ('TP', 'FP')  -- 只取有效标注
              AND af.created_at >= now() - INTERVAL {lookback_days} DAY
        ),
        alert_session_mapping AS (
            -- 关联 Alert 到 Session（通过 community_id）
            SELECT
                a.tenant_id,
                a.alert_id,
                a.community_id,
                a.session_id,
                a.alert_type,
                toString(a.severity) AS severity_str,
                a.score,
                la.label AS label,
                la.user_id AS user_id,
                la.model_version AS model_version,
                la.rule_version AS rule_version
            FROM traffic.alerts AS a
            GLOBAL INNER JOIN labeled_alerts AS la 
                ON a.alert_id = la.alert_id 
                AND a.tenant_id = la.tenant_id
            WHERE a.tenant_id = '{tenant_escaped}'
        )
        SELECT
            -- EventHeader 字段（对应 proto EventHeader）
            fs.tenant_id,
            fs.event_id,
            '' AS probe_id,
            fs.run_id,
            fs.feature_set_id,
            
            -- 关联键
            fs.community_id,
            fs.object_id,
            fs.object_type,
            
            -- 时间戳
            toUnixTimestamp(fs.ts) AS ts,
            toUnixTimestamp(fs.ingest_ts) AS ingest_ts,
            
            -- L1 统计特征（对应 proto FeatureStat message）
            fs.protocol,
            fs.duration_ms,
            fs.pps,
            fs.bps,
            fs.up_down_ratio,
            fs.pktlen_mean,
            fs.pktlen_std,
            fs.iat_mean_ms,
            fs.iat_std_ms,
            fs.active_mean_ms,
            fs.idle_mean_ms,
            fs.tcp_flag_syn_cnt,
            fs.tcp_flag_ack_cnt,
            fs.tcp_init_win_bytes_fwd,
            fs.tcp_init_win_bytes_bwd,
            
            -- 标注信息
            CASE 
                WHEN asm.label = 'TP' THEN 1
                WHEN asm.label = 'FP' THEN 0
                ELSE NULL
            END AS label,
            asm.alert_type,
            asm.severity_str,
            asm.score AS alert_score,
            asm.model_version,
            asm.rule_version,
            asm.user_id AS labeled_by
            
        FROM traffic.feature_stat AS fs
        GLOBAL INNER JOIN alert_session_mapping AS asm 
            ON fs.community_id = asm.community_id 
            AND fs.tenant_id = asm.tenant_id
        WHERE fs.tenant_id = '{tenant_escaped}'
          AND fs.feature_set_id = '{feature_set_escaped}'
          AND fs.ts >= now() - INTERVAL {lookback_days} DAY
          AND asm.label IS NOT NULL
        LIMIT {max_extract_rows}
        SETTINGS max_threads = 1, max_block_size = 2048
    """
    
    logger.info(f"Executing feature extraction query:")
    logger.info(f"  - Tenant: {tenant_id}")
    logger.info(f"  - Feature Set: {feature_set_id}")
    logger.info(f"  - Lookback: {lookback_days} days")
    logger.info(f"  - Maximum Samples: {max_extract_rows}")
    
    try:
        df = client.query_dataframe(query)
        
        logger.info(f"Successfully extracted {len(df)} samples")
        logger.info(f"Columns: {df.columns.tolist()}")
        
        if len(df) == 0:
            logger.warning("No data returned from query!")
            logger.warning("Possible causes:")
            logger.warning("  1. No alerts with feedback in the time range")
            logger.warning("  2. No matching features for labeled alerts")
            logger.warning("  3. Incorrect tenant_id or feature_set_id")
        
        return df
        
    except Exception as e:
        logger.error(f"Query execution failed: {e}")
        logger.error(f"Query: {query}")
        raise


def build_governed_extraction_query() -> str:
    """Return the parameterized, stable-order M08 v1 extraction query."""
    return """
        WITH labeled_alerts AS (
            SELECT tenant_id, alert_id, label, created_at, user_id, alert_type,
                   model_version, rule_version
            FROM traffic.alert_feedback
            WHERE tenant_id = %(tenant_id)s
              AND label IN ('TP', 'FP')
              AND created_at <= %(as_of)s
        ),
        alert_session_mapping AS (
            SELECT a.tenant_id, a.alert_id, a.community_id, a.session_id,
                   a.alert_type, toString(a.severity) AS severity_str, a.score,
                   la.label, la.user_id, la.model_version, la.rule_version
            FROM traffic.alerts AS a
            GLOBAL INNER JOIN labeled_alerts AS la
              ON a.alert_id = la.alert_id AND a.tenant_id = la.tenant_id
            WHERE a.tenant_id = %(tenant_id)s
        )
        SELECT
            fs.tenant_id, fs.event_id, '' AS probe_id, fs.run_id,
            fs.feature_set_id, fs.community_id, fs.object_id, fs.object_type,
            toUnixTimestamp(fs.ts) AS ts, toUnixTimestamp(fs.ingest_ts) AS ingest_ts,
            fs.protocol, fs.duration_ms, fs.pps, fs.bps, fs.up_down_ratio,
            fs.pktlen_mean, fs.pktlen_std, fs.iat_mean_ms, fs.iat_std_ms,
            fs.active_mean_ms, fs.idle_mean_ms, fs.tcp_flag_syn_cnt,
            fs.tcp_flag_ack_cnt, fs.tcp_init_win_bytes_fwd,
            fs.tcp_init_win_bytes_bwd,
            CASE WHEN asm.label = 'TP' THEN 1 WHEN asm.label = 'FP' THEN 0 ELSE NULL END AS label,
            asm.alert_type, asm.severity_str, asm.score AS alert_score,
            asm.model_version, asm.rule_version, asm.user_id AS labeled_by
        FROM traffic.feature_stat AS fs
        GLOBAL INNER JOIN alert_session_mapping AS asm
          ON fs.community_id = asm.community_id AND fs.tenant_id = asm.tenant_id
        WHERE fs.tenant_id = %(tenant_id)s
          AND fs.feature_set_id = %(feature_set_id)s
          AND fs.ts >= %(window_from)s AND fs.ts < %(window_through)s
          AND fs.ts <= %(source_watermark)s
          AND fs.ingest_ts <= %(as_of)s
          AND asm.label IS NOT NULL
        ORDER BY fs.ts, fs.event_id
        LIMIT %(limit)s
        SETTINGS max_threads = 1, max_block_size = 2048
    """


def extract_governed_features_with_labels(client, scope: ExtractionScope) -> pd.DataFrame:
    """Extract a bounded snapshot; max_rows+1 detects rather than truncates."""
    params = {
        'tenant_id': scope.tenant_id,
        'feature_set_id': scope.feature_set_id,
        'window_from': scope.window_from,
        'window_through': scope.window_through,
        'source_watermark': scope.source_watermark,
        'as_of': scope.as_of,
        'limit': scope.max_rows + 1,
    }
    frame = client.query_dataframe(build_governed_extraction_query(), params=params)
    validate_extracted_frame(frame, scope)
    return frame


def load_isolation_scope(path: str, frame: pd.DataFrame) -> tuple[pd.DataFrame, str]:
    """Join a governed, one-row-per-event isolation sidecar.

    The current ClickHouse feature table does not own site, PCAP or graph
    identities.  Those identities therefore must come from an immutable
    upstream manifest instead of being guessed from run_id/evidence_ids.
    """
    sidecar_path = os.path.abspath(path)
    if not os.path.isfile(sidecar_path):
        raise FileNotFoundError(f"Isolation scope not found: {sidecar_path}")
    if sidecar_path.endswith('.parquet'):
        sidecar = pd.read_parquet(sidecar_path, engine='pyarrow')
    elif sidecar_path.endswith('.json'):
        sidecar = pd.read_json(sidecar_path)
    else:
        raise ValueError("ISOLATION_SCOPE_PATH must be .parquet or .json")
    required = {'event_id', 'entity_id', 'site_id', 'pcap_id', 'attack_family'}
    missing = sorted(required - set(sidecar.columns))
    if missing:
        raise ValueError(f"isolation scope is missing columns: {missing}")
    if sidecar['event_id'].astype(str).duplicated().any():
        raise ValueError("isolation scope contains duplicate event_id")
    optional = [column for column in ('graph_node_ids', 'graph_edge_ids') if column in sidecar.columns]
    joined = frame.merge(sidecar[sorted(required) + optional], on='event_id', how='left', validate='one_to_one')
    for column in required - {'event_id'}:
        if joined[column].isna().any() or joined[column].astype(str).str.strip().eq('').any():
            raise ValueError(f"isolation scope does not cover every extracted event: {column}")
    if len(joined) != len(frame):
        raise ValueError("isolation scope join changed the source row count")
    return joined, sha256_file(sidecar_path)


def ml_feature_columns(frame: pd.DataFrame) -> list[str]:
    exclude = {
        'label', 'tenant_id', 'event_id', 'community_id', 'ts', 'ingest_ts',
        'alert_type', 'severity_str', 'object_id', 'object_type', 'probe_id',
        'run_id', 'feature_set_id', 'alert_score', 'model_version',
        'rule_version', 'labeled_by', 'entity_id', 'site_id', 'pcap_id',
        'attack_family', 'graph_node_ids', 'graph_edge_ids',
    }
    return sorted(column for column in frame.columns if column not in exclude)


def split_and_save_governed(
    frame: pd.DataFrame,
    output_dir: str,
    *,
    scope: ExtractionScope,
    train_through: str,
    validation_through: str,
    holdout_attack_families: list[str],
    label_schema_version: str,
    label_revision_sha256: str,
    isolation_scope_path: str,
    isolation_scope_sha256: str,
    graph_snapshot_manifest_path: str = '',
    preprocessing_notes: Dict[str, Any] | None = None,
) -> Dict[str, Any]:
    splits = split_by_time_and_validate(
        frame,
        train_through=train_through,
        validation_through=validation_through,
        holdout_attack_families=holdout_attack_families,
    )
    output = os.path.abspath(output_dir)
    os.makedirs(output, exist_ok=True)
    split_paths = {name: os.path.join(output, f'{name}.parquet') for name in splits}
    metadata_path = os.path.join(output, 'metadata.json')
    manifest_path = os.path.join(output, 'dataset-manifest.json')
    for path in [*split_paths.values(), metadata_path, manifest_path]:
        if os.path.exists(path):
            raise FileExistsError(f"refusing to overwrite immutable dataset artifact: {path}")
    for name, split in splits.items():
        split.to_parquet(split_paths[name], index=False, compression='snappy', engine='pyarrow')

    feature_columns = ml_feature_columns(frame)
    metadata = {
        'schema_version': 2,
        'governed_dataset': True,
        'tenant_id': scope.tenant_id,
        'feature_set_id': scope.feature_set_id,
        'feature_columns': feature_columns,
        'total_features': len(feature_columns),
        'split_samples': {name: len(split) for name, split in splits.items()},
        'train_samples': len(splits['train']),
        'test_samples': len(splits['test']),
        'label_schema_version': label_schema_version,
        'label_revision_sha256': label_revision_sha256,
        'isolation_scope_sha256': isolation_scope_sha256,
        'scope': scope.manifest_scope(),
        # 插补/清洗规则可追溯（代码审查 H40 收敛项）：preprocess 的决策写入
        # metadata，禁止静默 fillna 后不落账。
        'preprocessing': dict(preprocessing_notes) if preprocessing_notes else None,
    }
    write_json_exclusive(metadata_path, metadata)
    artifacts = {**split_paths, 'metadata': metadata_path, 'isolation_scope': isolation_scope_path}

    graph_snapshot = None
    if graph_snapshot_manifest_path:
        with open(graph_snapshot_manifest_path, 'r', encoding='utf-8') as handle:
            graph = json.load(handle)
        graph_as_of = parse_utc(graph.get('as_of', ''), 'graph_snapshot.as_of')
        if graph_as_of > scope.as_of:
            raise ValueError("graph snapshot reads future edges after dataset as_of")
        graph_snapshot = {
            'snapshot_id': str(graph.get('snapshot_id', '')).strip(),
            'manifest_sha256': sha256_file(graph_snapshot_manifest_path),
        }
        if not graph_snapshot['snapshot_id']:
            raise ValueError("graph snapshot_id is required")
        artifacts['graph_snapshot_manifest'] = graph_snapshot_manifest_path

    manifest = build_dataset_manifest(
        scope=scope,
        source_frame=frame,
        splits=splits,
        feature_columns=feature_columns,
        artifact_paths=artifacts,
        label_schema_version=label_schema_version,
        label_revision_sha256=label_revision_sha256,
        graph_snapshot=graph_snapshot,
    )
    write_json_exclusive(manifest_path, manifest)
    return manifest


def strict_env_flag(name: str, default: bool = False) -> bool:
    value = os.getenv(name)
    if value is None or value.strip() == '':
        return default
    normalized = value.strip().lower()
    if normalized not in {'true', 'false'}:
        raise ValueError(f"{name} must be explicitly true or false")
    return normalized == 'true'


def required_env(name: str) -> str:
    value = os.getenv(name, '').strip()
    if not value:
        raise ValueError(f"{name} is required when MLOPS_DATASET_GOVERNANCE_V1_ENABLED=true")
    return value


def detect_label_conflicts(frame: pd.DataFrame) -> None:
    """Detect community-level label conflicts caused by alert→feature fan-out.

    标注粒度说明（代码审查 H40 收敛项）：本提取以 community_id 级联 alert 标注
    到 feature 行。当同一 community_id 命中多条 TP/FP 反馈且标签不一致时，同一
    feature 事件会被放大为多行甚至冲突标签。governed 路径随后要求 event_id
    唯一（validate_extracted_frame），这里先给出精确的冲突诊断而非笼统报错。
    更优的 1:1 口径（alert_id/event_id 一对一标注）应在提取层保证，见
    run_governed_extraction 注释。
    """
    if frame.empty or 'community_id' not in frame.columns or 'label' not in frame.columns:
        return
    grouped = frame.groupby('community_id', dropna=False)['label']
    conflict_groups = []
    for community, labels in grouped:
        unique = {int(value) for value in labels.unique() if pd.notna(value)}
        if len(unique) > 1:
            conflict_groups.append((str(community), sorted(unique)))
    if conflict_groups:
        sample = conflict_groups[:20]
        raise ValueError(
            f"community_id label conflict detected for {len(conflict_groups)} communities "
            f"(e.g. {sample}); feature rows are amplified by the alert join and receive "
            f"conflicting labels. Fix the annotation granularity to alert/event 1:1 "
            f"before extracting a governed dataset"
        )


def run_governed_extraction(client, feature_set_id: str, tenant_id: str, output_dir: str) -> Dict[str, Any]:
    holdouts = [
        item.strip()
        for item in required_env('OPEN_SET_ATTACK_FAMILIES').split(',')
        if item.strip()
    ]
    if not holdouts:
        raise ValueError(
            'OPEN_SET_ATTACK_FAMILIES must contain at least one held-out family '
            'when MLOPS_DATASET_GOVERNANCE_V1_ENABLED=true'
        )
    scope = ExtractionScope.create(
        tenant_id=tenant_id,
        feature_set_id=feature_set_id,
        window_from=required_env('DATASET_WINDOW_FROM'),
        window_through=required_env('DATASET_WINDOW_THROUGH'),
        source_watermark=required_env('DATASET_SOURCE_WATERMARK'),
        as_of=required_env('DATASET_AS_OF'),
        max_rows=int(required_env('MAX_EXTRACT_ROWS')),
    )
    extracted = extract_governed_features_with_labels(client, scope)
    # 标注粒度：先做 community 级冲突诊断，再合并隔离 sidecar；冲突会使
    # event_id 唯一性校验失败，这里先给出精确报错。
    detect_label_conflicts(extracted)
    frame, isolation_sha = load_isolation_scope(required_env('ISOLATION_SCOPE_PATH'), extracted)
    validate_extracted_frame(frame, scope)
    frame, preprocessing_notes = preprocess_data(frame)
    return split_and_save_governed(
        frame,
        output_dir,
        scope=scope,
        train_through=required_env('DATASET_TRAIN_THROUGH'),
        validation_through=required_env('DATASET_VALIDATION_THROUGH'),
        holdout_attack_families=holdouts,
        label_schema_version=required_env('LABEL_SCHEMA_VERSION'),
        label_revision_sha256=required_env('LABEL_REVISION_SHA256'),
        isolation_scope_path=required_env('ISOLATION_SCOPE_PATH'),
        isolation_scope_sha256=isolation_sha,
        graph_snapshot_manifest_path=os.getenv('GRAPH_SNAPSHOT_MANIFEST', '').strip(),
        preprocessing_notes=preprocessing_notes,
    )


def preprocess_data(df: pd.DataFrame) -> pd.DataFrame:
    """Legacy-compatible preprocessing entry point (returns the frame only)."""
    return _preprocess(df, None)


def preprocess_data_with_notes(df: pd.DataFrame) -> tuple[pd.DataFrame, Dict[str, Any]]:
    """Governed preprocessing: returns the frame plus an auditable record of the
    imputation/cleaning decisions applied (代码审查 H40 收敛项：插补规则必须可
    追溯，禁止静默 fillna 后不落账)。"""
    notes: Dict[str, Any] = {}
    return _preprocess(df, notes), notes


def _preprocess(df: pd.DataFrame, notes: Dict[str, Any] | None) -> pd.DataFrame:
    """
    数据预处理（严格数据质量控制）
    """
    
    logger.info("=" * 80)
    logger.info("Starting data preprocessing...")
    logger.info("=" * 80)
    
    initial_count = len(df)
    
    # 1. 检查必需字段
    required_cols = ['tenant_id', 'event_id', 'community_id', 'label']
    missing_cols = set(required_cols) - set(df.columns)
    if missing_cols:
        raise ValueError(f"Missing required columns: {missing_cols}")
    
    # 2. 处理缺失值
    logger.info(f"Missing values before imputation:")
    missing_summary = df.isnull().sum()
    missing_summary = missing_summary[missing_summary > 0]
    if len(missing_summary) > 0:
        logger.info(f"\n{missing_summary}")
    else:
        logger.info("  No missing values found")
    
    # 数值列填充 0
    numeric_cols = df.select_dtypes(include=[np.number]).columns
    imputed_numeric = sorted(str(column) for column in missing_summary.index if column in numeric_cols)
    df[numeric_cols] = df[numeric_cols].fillna(0)
    
    # 字符串列填充空字符串
    string_cols = df.select_dtypes(include=['object', 'str']).columns
    imputed_string = sorted(str(column) for column in missing_summary.index if column in string_cols)
    df[string_cols] = df[string_cols].fillna('')
    
    # 3. 处理无穷大值
    logger.info("Replacing infinity values with 0...")
    before_inf = df.select_dtypes(include=[np.number])
    inf_cols = sorted(str(column) for column in before_inf.columns if np.isinf(before_inf[column]).any())
    df = df.replace([np.inf, -np.inf], 0)
    
    # 4. 移除重复（基于 event_id）
    before_dedup = len(df)
    df = df.drop_duplicates(subset=['event_id'], keep='first')
    dedup_count = before_dedup - len(df)
    logger.info(f"Removed {dedup_count} duplicate event_ids")
    
    # 5. 标签分布检查
    label_counts = df['label'].value_counts()
    logger.info(f"\nLabel distribution:")
    logger.info(f"  Positive (TP): {label_counts.get(1, 0)}")
    logger.info(f"  Negative (FP): {label_counts.get(0, 0)}")
    
    if len(label_counts) < 2:
        raise ValueError("Dataset must contain both positive and negative samples!")
    
    # 6. 类别不平衡检测
    imbalance_ratio = label_counts.max() / label_counts.min()
    logger.info(f"Class imbalance ratio: {imbalance_ratio:.2f}")
    
    if imbalance_ratio > 10:
        logger.warning("⚠️  High class imbalance detected!")
        logger.warning("    Consider using:")
        logger.warning("    - SMOTE (Synthetic Minority Over-sampling)")
        logger.warning("    - Class weights in XGBoost (scale_pos_weight)")
        logger.warning("    - Stratified sampling")
    
    # 7. 异常值检测（基于 IQR）
    logger.info("\nDetecting outliers using IQR method...")
    feature_cols = [col for col in df.columns if col not in 
                    ['tenant_id', 'event_id', 'community_id', 'label', 'ts', 
                     'ingest_ts', 'alert_type', 'severity_str', 'object_id', 
                     'object_type', 'probe_id', 'run_id', 'feature_set_id',
                     'model_version', 'rule_version', 'labeled_by', 'entity_id',
                     'site_id', 'pcap_id', 'attack_family', 'graph_node_ids',
                     'graph_edge_ids']]
    
    outlier_summary = {}
    for col in feature_cols:
        if df[col].dtype in [np.float64, np.float32, np.int64, np.int32]:
            Q1 = df[col].quantile(0.25)
            Q3 = df[col].quantile(0.75)
            IQR = Q3 - Q1
            lower_bound = Q1 - 3 * IQR
            upper_bound = Q3 + 3 * IQR
            
            outliers = ((df[col] < lower_bound) | (df[col] > upper_bound)).sum()
            if outliers > 0:
                outlier_summary[col] = outliers
    
    if outlier_summary:
        logger.info(f"Outliers detected in {len(outlier_summary)} features:")
        for col, count in sorted(outlier_summary.items(), key=lambda x: x[1], reverse=True)[:10]:
            logger.info(f"  - {col}: {count} outliers")
    else:
        logger.info("No significant outliers detected")
    
    # 8. 特征统计摘要
    logger.info(f"\nFeature statistics summary:")
    logger.info(f"  Total features: {len(feature_cols)}")
    logger.info(f"  Mean values range: [{df[feature_cols].mean().min():.2f}, {df[feature_cols].mean().max():.2f}]")
    logger.info(f"  Std values range: [{df[feature_cols].std().min():.2f}, {df[feature_cols].std().max():.2f}]")
    
    # 9. 数据质量报告
    final_count = len(df)
    logger.info("=" * 80)
    logger.info("Data preprocessing completed!")
    logger.info(f"  Initial samples: {initial_count}")
    logger.info(f"  Final samples: {final_count}")
    logger.info(f"  Removed: {initial_count - final_count}")
    logger.info("=" * 80)

    if notes is not None:
        notes.update({
            'na_strategy': 'zero_for_numeric_empty_string_for_string',
            'inf_strategy': 'zero',
            'dedup_by': 'event_id_keep_first',
            'initial_rows': int(initial_count),
            'final_rows': int(final_count),
            'removed_duplicate_rows': int(dedup_count),
            'imputed_numeric_columns': imputed_numeric,
            'imputed_string_columns': imputed_string,
            'inf_replaced_columns': inf_cols,
        })
    
    return df


def split_and_save(df: pd.DataFrame, output_dir: str, test_size: float = 0.2, random_state: int = 42) -> Dict[str, Any]:
    """
    分割训练集和测试集，并保存为 Parquet
    """
    
    logger.info("=" * 80)
    logger.info("Splitting dataset...")
    logger.info("=" * 80)
    logger.info(f"Configuration:")
    logger.info(f"  - Test size: {test_size}")
    logger.info(f"  - Random state: {random_state}")
    logger.info(f"  - Stratify by: label")
    
    label_counts = df['label'].value_counts()
    min_class_count = int(label_counts.min())
    if min_class_count < 2:
        raise ValueError("Each class must contain at least 2 samples for train/test split")

    test_count = max(int(round(len(df) * test_size)), len(label_counts))
    test_count = min(test_count, len(df) - len(label_counts))
    effective_test_size = test_count / len(df)
    if abs(effective_test_size - test_size) > 1e-9:
        logger.warning(
            "Adjusted test size from %.3f to %.3f to keep both classes in train/test",
            test_size,
            effective_test_size,
        )

    # 分层采样保持标签分布
    train_df, test_df = train_test_split(
        df,
        test_size=effective_test_size,
        random_state=random_state,
        stratify=df['label']
    )
    
    logger.info(f"\nTrain set:")
    logger.info(f"  - Total samples: {len(train_df)}")
    logger.info(f"  - Positive: {train_df['label'].sum()}")
    logger.info(f"  - Negative: {(train_df['label'] == 0).sum()}")
    
    logger.info(f"\nTest set:")
    logger.info(f"  - Total samples: {len(test_df)}")
    logger.info(f"  - Positive: {test_df['label'].sum()}")
    logger.info(f"  - Negative: {(test_df['label'] == 0).sum()}")
    
    # 创建输出目录
    os.makedirs(output_dir, exist_ok=True)
    
    # 保存为 Parquet（高效压缩）
    train_path = os.path.join(output_dir, 'train.parquet')
    test_path = os.path.join(output_dir, 'test.parquet')
    
    train_df.to_parquet(train_path, index=False, compression='snappy', engine='pyarrow')
    test_df.to_parquet(test_path, index=False, compression='snappy', engine='pyarrow')
    
    logger.info(f"\n✓ Saved train.parquet: {train_path}")
    logger.info(f"✓ Saved test.parquet: {test_path}")
    
    # 提取特征列（排除元数据字段）
    exclude_cols = ['label', 'tenant_id', 'event_id', 'community_id', 'ts', 
                    'ingest_ts', 'alert_type', 'severity_str', 'object_id', 
                    'object_type', 'probe_id', 'run_id', 'feature_set_id',
                    'alert_score', 'model_version', 'rule_version', 'labeled_by']
    
    feature_columns = [col for col in df.columns if col not in exclude_cols]
    
    # 保存元数据
    metadata = {
        'train_samples': len(train_df),
        'test_samples': len(test_df),
        'train_positive': int(train_df['label'].sum()),
        'train_negative': int((train_df['label'] == 0).sum()),
        'test_positive': int(test_df['label'].sum()),
        'test_negative': int((test_df['label'] == 0).sum()),
        'feature_columns': feature_columns,
        'total_features': len(feature_columns),
        'test_size': test_size,
        'random_state': random_state,
        'tenant_id': df['tenant_id'].iloc[0] if len(df) > 0 else '',
        'feature_set_id': df['feature_set_id'].iloc[0] if 'feature_set_id' in df.columns and len(df) > 0 else '',
    }
    
    metadata_path = os.path.join(output_dir, 'metadata.json')
    with open(metadata_path, 'w') as f:
        json.dump(metadata, f, indent=2)
    
    logger.info(f"✓ Saved metadata: {metadata_path}")
    logger.info("=" * 80)
    
    return metadata


def main():
    """主函数"""
    
    # 读取环境变量
    feature_set_id = os.getenv('FEATURE_SET_ID', 'v1')
    lookback_days = int(os.getenv('LOOKBACK_DAYS', '7'))
    tenant_id = os.getenv('TENANT_ID', 'campus-net')
    output_dir = os.getenv('OUTPUT_DIR', '/output')
    test_size = float(os.getenv('TEST_SIZE', '0.2'))
    governed_v1 = strict_env_flag('MLOPS_DATASET_GOVERNANCE_V1_ENABLED', False)
    
    logger.info("")
    logger.info("=" * 80)
    logger.info("🚀 MLOps Data Extraction Pipeline")
    logger.info("=" * 80)
    logger.info("")
    logger.info("Configuration:")
    logger.info(f"  - Feature Set ID: {feature_set_id}")
    logger.info(f"  - Lookback Days: {lookback_days}")
    logger.info(f"  - Tenant ID: {tenant_id}")
    logger.info(f"  - Output Directory: {output_dir}")
    logger.info(f"  - Test Size: {test_size}")
    logger.info(f"  - Dataset Governance v1: {governed_v1}")
    logger.info("")
    
    try:
        # 1. 连接 ClickHouse
        logger.info("Step 1: Connecting to ClickHouse...")
        client = connect_clickhouse()

        if governed_v1:
            logger.info("\nRunning immutable M08 dataset-governance v1 path...")
            manifest = run_governed_extraction(client, feature_set_id, tenant_id, output_dir)
            logger.info("Governed dataset snapshot complete: dataset_id=%s sha256=%s",
                        manifest['dataset_id'], manifest['dataset_sha256'])
            return
        
        # 2. 提取数据
        logger.info("\nStep 2: Extracting features and labels...")
        df = extract_features_with_labels(client, feature_set_id, lookback_days, tenant_id)
        
        if len(df) == 0:
            logger.error("❌ No data extracted!")
            logger.error("Please check:")
            logger.error("  1. alert_feedback table has TP/FP labels")
            logger.error("  2. feature_stat table has matching community_ids")
            logger.error("  3. Correct tenant_id and feature_set_id")
            sys.exit(1)
        
        # 3. 预处理
        logger.info("\nStep 3: Preprocessing data...")
        df = preprocess_data(df)
        
        # 4. 分割并保存
        logger.info("\nStep 4: Splitting and saving dataset...")
        metadata = split_and_save(df, output_dir, test_size=test_size)
        
        # 5. 最终总结
        logger.info("")
        logger.info("=" * 80)
        logger.info("✅ Data extraction completed successfully!")
        logger.info("=" * 80)
        logger.info("")
        logger.info("Summary:")
        logger.info(f"  📊 Total samples: {metadata['train_samples'] + metadata['test_samples']}")
        logger.info(f"  🎓 Train samples: {metadata['train_samples']} (TP: {metadata['train_positive']}, FP: {metadata['train_negative']})")
        logger.info(f"  🧪 Test samples: {metadata['test_samples']} (TP: {metadata['test_positive']}, FP: {metadata['test_negative']})")
        logger.info(f"  🔢 Feature columns: {metadata['total_features']}")
        logger.info("")
        logger.info("Output files:")
        logger.info(f"  ✓ {os.path.join(output_dir, 'train.parquet')}")
        logger.info(f"  ✓ {os.path.join(output_dir, 'test.parquet')}")
        logger.info(f"  ✓ {os.path.join(output_dir, 'metadata.json')}")
        logger.info("=" * 80)
        logger.info("")
        
    except Exception as e:
        logger.error("")
        logger.error("=" * 80)
        logger.error("❌ Data extraction failed!")
        logger.error("=" * 80)
        logger.error(f"Error: {e}", exc_info=True)
        logger.error("=" * 80)
        sys.exit(1)


if __name__ == '__main__':
    main()
