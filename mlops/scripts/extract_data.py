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
        ORDER BY fs.ts DESC
        LIMIT 1000000
    """
    
    logger.info(f"Executing feature extraction query:")
    logger.info(f"  - Tenant: {tenant_id}")
    logger.info(f"  - Feature Set: {feature_set_id}")
    logger.info(f"  - Lookback: {lookback_days} days")
    
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


def preprocess_data(df: pd.DataFrame) -> pd.DataFrame:
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
    df[numeric_cols] = df[numeric_cols].fillna(0)
    
    # 字符串列填充空字符串
    string_cols = df.select_dtypes(include=['object']).columns
    df[string_cols] = df[string_cols].fillna('')
    
    # 3. 处理无穷大值
    logger.info("Replacing infinity values with 0...")
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
                     'model_version', 'rule_version', 'labeled_by']]
    
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
    logger.info("")
    
    try:
        # 1. 连接 ClickHouse
        logger.info("Step 1: Connecting to ClickHouse...")
        client = connect_clickhouse()
        
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
