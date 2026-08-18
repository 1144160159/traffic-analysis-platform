#!/usr/bin/env python3
"""
模型训练脚本：训练 XGBoost/LightGBM 模型
支持类别不平衡处理、超参数优化、早停机制
严格遵守生产级模型训练规范
"""

import os
import sys
import json
import platform
import re
import uuid
from importlib import metadata as importlib_metadata
import pandas as pd
import numpy as np
import xgboost as xgb
import lightgbm as lgb
from sklearn.model_selection import cross_val_score, StratifiedKFold
from sklearn.metrics import (
    classification_report, 
    roc_auc_score, 
    precision_recall_curve,
    confusion_matrix,
    f1_score
)
import logging
from typing import Tuple, Dict, Any, List
import joblib
import warnings

from dataset_governance import (
    canonical_json_sha256,
    sha256_file,
    validate_dataset_manifest_identity,
    validate_split_isolation,
    write_json_exclusive,
)

# 禁止模块级全局吞掉 warning（代码审查 H40 收敛项）：已知噪音在调用点用
# catch_warnings 局部抑制，避免掩盖数据/API 兼容问题。
_DEPRECATION_WARNINGS = (
    UserWarning,
    FutureWarning,
    DeprecationWarning,
)

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def class_counts(y: pd.Series) -> Dict[int, int]:
    counts = y.value_counts().to_dict()
    return {int(k): int(v) for k, v in counts.items()}


def load_training_data(data_path: str, metadata_path: str) -> Tuple[pd.DataFrame, pd.Series, List[str]]:
    """
    加载训练数据
    
    严格遵守数据契约：
    - 读取 Parquet 格式训练集
    - 验证元数据完整性
    - 检查特征列完整性
    """
    
    logger.info("=" * 80)
    logger.info("Loading training data...")
    logger.info("=" * 80)
    
    if not os.path.exists(data_path):
        raise FileNotFoundError(f"Training data not found: {data_path}")
    
    if not os.path.exists(metadata_path):
        raise FileNotFoundError(f"Metadata not found: {metadata_path}")
    
    # 加载数据
    logger.info(f"Reading Parquet file: {data_path}")
    df = pd.read_parquet(data_path, engine='pyarrow')
    logger.info(f"  ✓ Loaded {len(df)} training samples")
    
    # 加载元数据
    logger.info(f"Reading metadata: {metadata_path}")
    with open(metadata_path, 'r') as f:
        metadata = json.load(f)
    
    feature_cols = metadata.get('feature_columns', [])
    
    if not feature_cols:
        raise ValueError("No feature columns found in metadata!")
    
    logger.info(f"  ✓ Metadata loaded: {len(feature_cols)} features")
    
    # 验证特征列存在
    missing_cols = set(feature_cols) - set(df.columns)
    if missing_cols:
        raise ValueError(f"Missing feature columns in data: {missing_cols}")
    
    # 验证标签列存在
    if 'label' not in df.columns:
        raise ValueError("Label column 'label' not found in training data!")
    
    # 提取特征和标签
    X = df[feature_cols].copy()
    y = df['label'].copy()
    
    # 数据验证
    logger.info("\nData validation:")
    logger.info(f"  - X shape: {X.shape}")
    logger.info(f"  - y shape: {y.shape}")
    logger.info(f"  - Feature count: {len(feature_cols)}")
    
    # 标签分布
    label_dist = y.value_counts().to_dict()
    logger.info(f"  - Label distribution: {label_dist}")
    
    # 检查是否有 NaN
    nan_count = X.isnull().sum().sum()
    if nan_count > 0:
        logger.warning(f"  ⚠️  Found {nan_count} NaN values in features, filling with 0")
        X = X.fillna(0)
    
    # 检查是否有无穷大
    inf_count = np.isinf(X.select_dtypes(include=[np.number])).sum().sum()
    if inf_count > 0:
        logger.warning(f"  ⚠️  Found {inf_count} infinity values, replacing with 0")
        X = X.replace([np.inf, -np.inf], 0)
    
    logger.info("=" * 80)
    
    return X, y, feature_cols


def load_governed_training_data(
    data_dir: str,
) -> Tuple[pd.DataFrame, pd.Series, pd.DataFrame, pd.Series, List[str], Dict[str, Any]]:
    """Load and re-hash the immutable dataset before training."""
    root = os.path.abspath(data_dir)
    manifest_path = os.path.join(root, 'dataset-manifest.json')
    metadata_path = os.path.join(root, 'metadata.json')
    for path in (manifest_path, metadata_path):
        if not os.path.isfile(path):
            raise FileNotFoundError(f"Governed dataset artifact not found: {path}")
    with open(manifest_path, 'r', encoding='utf-8') as handle:
        manifest = json.load(handle)
    with open(metadata_path, 'r', encoding='utf-8') as handle:
        metadata = json.load(handle)
    validate_dataset_manifest_identity(manifest)
    if not metadata.get('governed_dataset') or metadata.get('schema_version') != 2:
        raise ValueError("metadata is not a governed dataset v2 artifact")
    if manifest['artifacts'].get('metadata') != sha256_file(metadata_path):
        raise ValueError("metadata artifact hash does not match dataset manifest")
    feature_cols = list(manifest['features']['columns'])
    if metadata.get('feature_columns') != feature_cols:
        raise ValueError("metadata feature columns drift from dataset manifest")
    if canonical_json_sha256(feature_cols) != manifest['features']['columns_sha256']:
        raise ValueError("feature column hash does not match dataset manifest")

    frames: dict[str, pd.DataFrame] = {}
    for name, split in manifest['splits'].items():
        path = os.path.join(root, f'{name}.parquet')
        if not os.path.isfile(path) or manifest['artifacts'].get(name) != sha256_file(path):
            raise ValueError(f"{name} artifact hash does not match dataset manifest")
        frame = pd.read_parquet(path, engine='pyarrow')
        if len(frame) != split['row_count']:
            raise ValueError(f"{name} row count does not match dataset manifest")
        event_ids_sha = canonical_json_sha256(sorted(frame['event_id'].astype(str)))
        if event_ids_sha != split['event_ids_sha256']:
            raise ValueError(f"{name} event identity set does not match dataset manifest")
        missing = sorted(set(feature_cols + ['label']) - set(frame.columns))
        if missing:
            raise ValueError(f"{name} is missing governed training columns: {missing}")
        frames[name] = frame
    if 'train' not in frames or 'validation' not in frames:
        raise ValueError("governed training requires pre-isolated train and validation splits")
    holdout_families = (
        set(frames['open_set']['attack_family'].astype(str))
        if 'open_set' in frames else set()
    )
    validate_split_isolation(frames, holdout_attack_families=holdout_families)

    from dataset_governance import canonical_rows_sha256
    reconstructed = pd.concat([frames[name] for name in sorted(frames)], ignore_index=True)
    if len(reconstructed) != manifest['source']['row_count'] or \
            canonical_rows_sha256(reconstructed) != manifest['source']['canonical_rows_sha256']:
        raise ValueError("split artifacts do not reconstruct the immutable source dataset")
    train, validation = frames['train'], frames['validation']
    if train['label'].nunique() < 2 or validation['label'].nunique() < 2:
        raise ValueError("train and validation must each contain at least two labels")
    return (
        train[feature_cols].copy(), train['label'].copy(),
        validation[feature_cols].copy(), validation['label'].copy(),
        feature_cols, manifest,
    )


def train_xgboost_governed(
    X_train: pd.DataFrame,
    y_train: pd.Series,
    X_validation: pd.DataFrame,
    y_validation: pd.Series,
    *,
    seed: int,
    cpu_limit: int,
    params: Dict[str, Any] | None = None,
) -> xgb.XGBClassifier:
    """Train only on pre-isolated splits; never create a random validation."""
    if seed < 0 or cpu_limit < 1:
        raise ValueError("seed and cpu_limit must be explicit positive training controls")
    if y_train.nunique() != 2 or y_validation.nunique() != 2:
        raise ValueError("governed XGBoost requires two labels in train and validation")
    negative, positive = int((y_train == 0).sum()), int((y_train == 1).sum())
    if positive == 0:
        raise ValueError("governed XGBoost has no positive training samples")
    model_params: Dict[str, Any] = {
        'max_depth': 6,
        'learning_rate': 0.1,
        'n_estimators': 200,
        'objective': 'binary:logistic',
        'eval_metric': 'logloss',
        'scale_pos_weight': negative / positive,
        'subsample': 0.8,
        'colsample_bytree': 0.8,
        'min_child_weight': 1,
        'gamma': 0,
        'reg_alpha': 0,
        'reg_lambda': 1,
        'random_state': seed,
        'n_jobs': cpu_limit,
        'tree_method': 'hist',
        'grow_policy': 'depthwise',
        'max_bin': 256,
    }
    if params:
        forbidden = {'random_state', 'n_jobs', 'objective'} & set(params)
        if forbidden:
            raise ValueError(f"governed parameters cannot override {sorted(forbidden)}")
        model_params.update(params)
    model = xgb.XGBClassifier(**model_params)
    major = int(xgb.__version__.split('.')[0])
    if len(X_validation) >= 10:
        if major >= 3:
            model.set_params(early_stopping_rounds=20)
            model.fit(X_train, y_train, eval_set=[(X_validation, y_validation)], verbose=False)
        else:
            model.fit(X_train, y_train, eval_set=[(X_validation, y_validation)],
                      early_stopping_rounds=20, verbose=False)
    else:
        model.fit(X_train, y_train, eval_set=[(X_validation, y_validation)], verbose=False)
    return model


def build_training_run_manifest(
    *,
    run_id: str,
    dataset_manifest: Dict[str, Any],
    algorithm: str,
    seed: int,
    model: Any,
    trainer_image_digest: str,
    cpu_limit: str,
    memory_limit: str,
    gpu_limit: int,
    artifact_paths: Dict[str, str],
    code_path: str = __file__,
) -> Dict[str, Any]:
    parsed_run_id = uuid.UUID(run_id)
    if str(parsed_run_id) != run_id:
        raise ValueError("TRAIN_RUN_ID must be a canonical lowercase UUID")
    if not re.fullmatch(r'sha256:[0-9a-f]{64}', trainer_image_digest):
        raise ValueError("TRAINER_IMAGE_DIGEST must be sha256:<lowercase digest>")
    if not cpu_limit.strip() or not memory_limit.strip() or gpu_limit < 0:
        raise ValueError("training resource limits must be explicit")
    artifacts = {name: sha256_file(path) for name, path in sorted(artifact_paths.items())}
    dependencies = {
        'python': platform.python_version(),
        'numpy': np.__version__,
        'pandas': pd.__version__,
        'scikit_learn': importlib_metadata.version('scikit-learn'),
        'xgboost': xgb.__version__,
        'lightgbm': lgb.__version__,
    }
    def safe_parameter(value: Any) -> Any:
        if isinstance(value, np.generic):
            value = value.item()
        if isinstance(value, float) and (np.isnan(value) or np.isinf(value)):
            return 'NaN' if np.isnan(value) else ('Infinity' if value > 0 else '-Infinity')
        if isinstance(value, dict):
            return {str(key): safe_parameter(item) for key, item in value.items()}
        if isinstance(value, (list, tuple)):
            return [safe_parameter(item) for item in value]
        if value is None or isinstance(value, (str, int, float, bool)):
            return value
        return str(value)

    meaning = {
        'run_id': run_id,
        'dataset_id': dataset_manifest['dataset_id'],
        'dataset_sha256': dataset_manifest['dataset_sha256'],
        'graph_snapshot': dataset_manifest.get('graph_snapshot'),
        'algorithm': algorithm,
        'seed': seed,
        'parameters': safe_parameter(model.get_params()),
        'code_sha256': sha256_file(code_path),
        'trainer_image_digest': trainer_image_digest,
        'dependencies': dependencies,
        'resources': {'cpu_limit': cpu_limit, 'memory_limit': memory_limit, 'gpu_limit': gpu_limit},
        'artifacts': artifacts,
    }
    return {'schema_version': 1, 'state': 'trained', **meaning, 'run_sha256': canonical_json_sha256(meaning)}


def train_xgboost(X: pd.DataFrame, y: pd.Series, params: Dict[str, Any] = None) -> xgb.XGBClassifier:
    """
    训练 XGBoost 模型
    
    特性：
    - 类别不平衡处理（scale_pos_weight）
    - 早停机制（early_stopping_rounds）
    - 交叉验证（StratifiedKFold）
    - GPU 加速支持
    """
    
    logger.info("=" * 80)
    logger.info("Training XGBoost model...")
    logger.info("=" * 80)
    
    # 计算类别权重（处理不平衡）
    n_negative = (y == 0).sum()
    n_positive = (y == 1).sum()
    scale_pos_weight = n_negative / n_positive if n_positive > 0 else 1.0
    
    logger.info(f"Class distribution:")
    logger.info(f"  - Negative samples: {n_negative}")
    logger.info(f"  - Positive samples: {n_positive}")
    logger.info(f"  - Scale pos weight: {scale_pos_weight:.2f}")
    
    # 默认超参数
    default_params = {
        'max_depth': 6,
        'learning_rate': 0.1,
        'n_estimators': 200,
        'objective': 'binary:logistic',
        'eval_metric': 'logloss',
        'scale_pos_weight': scale_pos_weight,
        'subsample': 0.8,
        'colsample_bytree': 0.8,
        'min_child_weight': 1,
        'gamma': 0,
        'reg_alpha': 0,
        'reg_lambda': 1,
        'random_state': 42,
        'n_jobs': -1,
        'tree_method': 'hist',  # 快速训练
        'grow_policy': 'depthwise',
        'max_bin': 256,
    }
    
    # 合并自定义参数
    if params:
        default_params.update(params)
    
    logger.info("\nModel hyperparameters:")
    for k, v in sorted(default_params.items()):
        logger.info(f"  - {k}: {v}")
    
    # 创建模型
    model = xgb.XGBClassifier(**default_params)
    
    # 训练（带早停）
    logger.info("\nTraining with early stopping...")

    # 分割验证集用于早停
    from sklearn.model_selection import train_test_split
    counts = class_counts(y)
    min_class_count = min(counts.values()) if counts else 0
    can_split_validation = len(counts) == 2 and min_class_count >= 2 and len(X) >= 4

    if can_split_validation:
        X_train, X_val, y_train, y_val = train_test_split(
            X, y, test_size=0.2, random_state=42, stratify=y
        )
    else:
        logger.info("  Dataset too small for a validation split, training on all samples")
        X_train, X_val, y_train, y_val = X, None, y, None

    # XGBoost early stopping: xgboost>=3.x uses constructor param; <3.x uses fit() param
    import xgboost as xgb_mod
    xgb_major = int(xgb_mod.__version__.split('.')[0])
    use_early_stopping = X_val is not None and len(X_val) >= 10 and y_val.nunique() == 2

    if use_early_stopping:
        if xgb_major >= 3:
            # XGBoost 3.x: early_stopping_rounds in constructor
            model.set_params(early_stopping_rounds=20)
            model.fit(X_train, y_train, eval_set=[(X_val, y_val)], verbose=10)
        else:
            # XGBoost 2.x: early_stopping_rounds in fit()
            model.fit(X_train, y_train, eval_set=[(X_val, y_val)],
                     early_stopping_rounds=20, verbose=10)
    else:
        logger.info("  Validation set too small for early stopping, training without")
        model.set_params(early_stopping_rounds=None)
        model.fit(X_train, y_train, verbose=10)

    if hasattr(model, 'best_iteration') and model.best_iteration is not None:
        logger.info(f"  ✓ Best iteration: {model.best_iteration}")
    if hasattr(model, 'best_score') and model.best_score is not None:
        logger.info(f"  ✓ Best score: {model.best_score:.4f}")

    # 交叉验证前关闭早停（cross_val_score 不传 eval_set）
    model.set_params(early_stopping_rounds=None)

    # 交叉验证
    n_splits = min(5, min_class_count)
    if len(counts) == 2 and n_splits >= 2:
        logger.info(f"\nPerforming {n_splits}-fold stratified cross-validation...")
        skf = StratifiedKFold(n_splits=n_splits, shuffle=True, random_state=42)

        cv_scores_f1 = cross_val_score(model, X, y, cv=skf, scoring='f1', n_jobs=-1)
        cv_scores_auc = cross_val_score(model, X, y, cv=skf, scoring='roc_auc', n_jobs=-1)

        logger.info("Cross-validation results:")
        logger.info(f"  - F1 scores: {[f'{s:.4f}' for s in cv_scores_f1]}")
        logger.info(f"  - Mean F1: {cv_scores_f1.mean():.4f} ± {cv_scores_f1.std():.4f}")
        logger.info(f"  - AUC scores: {[f'{s:.4f}' for s in cv_scores_auc]}")
        logger.info(f"  - Mean AUC: {cv_scores_auc.mean():.4f} ± {cv_scores_auc.std():.4f}")
    else:
        logger.info("\nSkipping cross-validation: each class needs at least 2 samples")
    
    logger.info("=" * 80)
    
    return model


def train_lightgbm(X: pd.DataFrame, y: pd.Series, params: Dict[str, Any] = None) -> lgb.LGBMClassifier:
    """
    训练 LightGBM 模型
    
    特性：
    - 类别不平衡处理（scale_pos_weight）
    - 早停机制（early_stopping_rounds）
    - 交叉验证（StratifiedKFold）
    """
    
    logger.info("=" * 80)
    logger.info("Training LightGBM model...")
    logger.info("=" * 80)
    
    # 计算类别权重
    n_negative = (y == 0).sum()
    n_positive = (y == 1).sum()
    scale_pos_weight = n_negative / n_positive if n_positive > 0 else 1.0
    
    logger.info(f"Class distribution:")
    logger.info(f"  - Negative samples: {n_negative}")
    logger.info(f"  - Positive samples: {n_positive}")
    logger.info(f"  - Scale pos weight: {scale_pos_weight:.2f}")
    
    # 默认超参数
    default_params = {
        'max_depth': 6,
        'learning_rate': 0.1,
        'n_estimators': 200,
        'objective': 'binary',
        'metric': 'binary_logloss',
        'scale_pos_weight': scale_pos_weight,
        'subsample': 0.8,
        'subsample_freq': 1,
        'colsample_bytree': 0.8,
        'min_child_weight': 1,
        'min_child_samples': 20,
        'reg_alpha': 0,
        'reg_lambda': 1,
        'random_state': 42,
        'n_jobs': -1,
        'boosting_type': 'gbdt',
        'num_leaves': 31,
        'max_bin': 255,
    }
    
    # 合并自定义参数
    if params:
        default_params.update(params)
    
    logger.info("\nModel hyperparameters:")
    for k, v in sorted(default_params.items()):
        logger.info(f"  - {k}: {v}")
    
    # 创建模型
    model = lgb.LGBMClassifier(**default_params)
    
    # 训练（带早停）
    logger.info("\nTraining with early stopping...")
    
    from sklearn.model_selection import train_test_split
    counts = class_counts(y)
    min_class_count = min(counts.values()) if counts else 0
    can_split_validation = len(counts) == 2 and min_class_count >= 2 and len(X) >= 4

    if can_split_validation:
        X_train, X_val, y_train, y_val = train_test_split(
            X, y, test_size=0.2, random_state=42, stratify=y
        )

        model.fit(
            X_train, y_train,
            eval_set=[(X_val, y_val)],
            eval_metric='binary_logloss',
            callbacks=[
                lgb.early_stopping(stopping_rounds=20, verbose=True),
                lgb.log_evaluation(period=10)
            ]
        )

        logger.info(f"  ✓ Best iteration: {model.best_iteration_}")
        logger.info(f"  ✓ Best score: {model.best_score_['valid_0']['binary_logloss']:.4f}")
    else:
        logger.info("  Dataset too small for a validation split, training on all samples")
        model.fit(X, y)

    # 交叉验证
    n_splits = min(5, min_class_count)
    if len(counts) == 2 and n_splits >= 2:
        logger.info(f"\nPerforming {n_splits}-fold stratified cross-validation...")
        skf = StratifiedKFold(n_splits=n_splits, shuffle=True, random_state=42)

        cv_scores_f1 = cross_val_score(model, X, y, cv=skf, scoring='f1', n_jobs=-1)
        cv_scores_auc = cross_val_score(model, X, y, cv=skf, scoring='roc_auc', n_jobs=-1)

        logger.info("Cross-validation results:")
        logger.info(f"  - F1 scores: {[f'{s:.4f}' for s in cv_scores_f1]}")
        logger.info(f"  - Mean F1: {cv_scores_f1.mean():.4f} ± {cv_scores_f1.std():.4f}")
        logger.info(f"  - AUC scores: {[f'{s:.4f}' for s in cv_scores_auc]}")
        logger.info(f"  - Mean AUC: {cv_scores_auc.mean():.4f} ± {cv_scores_auc.std():.4f}")
    else:
        logger.info("\nSkipping cross-validation: each class needs at least 2 samples")
    
    logger.info("=" * 80)
    
    return model


def save_model(model, output_dir: str, model_type: str, feature_cols: List[str]) -> str:
    """
    保存模型及相关文件
    
    保存内容：
    1. 模型文件（.json/.txt）
    2. 特征重要性（feature_importance.json）
    3. 特征列表（feature_columns.json）
    4. 训练配置（train_config.json）
    """
    
    logger.info("=" * 80)
    logger.info("Saving model artifacts...")
    logger.info("=" * 80)
    
    os.makedirs(output_dir, exist_ok=True)
    
    # 1. 保存模型文件
    if model_type == 'xgboost':
        model_path = os.path.join(output_dir, 'model.json')
        model.save_model(model_path)
        logger.info(f"  ✓ Saved XGBoost model: {model_path}")
        
        # 获取特征重要性
        importance = model.get_booster().get_score(importance_type='weight')
        # 映射回特征名
        feature_importance = {}
        for i, col in enumerate(feature_cols):
            feature_importance[col] = importance.get(f'f{i}', 0)
        
    elif model_type == 'lightgbm':
        model_path = os.path.join(output_dir, 'model.txt')
        model.booster_.save_model(model_path)
        logger.info(f"  ✓ Saved LightGBM model: {model_path}")
        
        # 获取特征重要性
        feature_importance = dict(zip(feature_cols, model.feature_importances_.tolist()))
    
    else:
        raise ValueError(f"Unsupported model type: {model_type}")
    
    # 2. 保存特征重要性
    sorted_importance = dict(sorted(feature_importance.items(), key=lambda x: x[1], reverse=True))
    
    importance_path = os.path.join(output_dir, 'feature_importance.json')
    with open(importance_path, 'w') as f:
        json.dump(sorted_importance, f, indent=2)
    
    logger.info(f"  ✓ Saved feature importance: {importance_path}")
    
    # 打印 Top 10 特征
    logger.info("\nTop 10 important features:")
    for i, (feat, score) in enumerate(list(sorted_importance.items())[:10], 1):
        logger.info(f"  {i:2d}. {feat:30s} : {score:.2f}")
    
    # 3. 保存特征列表
    feature_list_path = os.path.join(output_dir, 'feature_columns.json')
    with open(feature_list_path, 'w') as f:
        json.dump(feature_cols, f, indent=2)
    
    logger.info(f"  ✓ Saved feature columns: {feature_list_path}")
    
    # 4. 保存训练配置
    train_config = {
        'model_type': model_type,
        'feature_count': len(feature_cols),
        'feature_columns': feature_cols,
    }
    
    if model_type == 'xgboost':
        train_config['hyperparameters'] = model.get_params()
        train_config['best_iteration'] = int(model.best_iteration) if hasattr(model, 'best_iteration') else None
        train_config['best_score'] = float(model.best_score) if hasattr(model, 'best_score') else None
    elif model_type == 'lightgbm':
        train_config['hyperparameters'] = model.get_params()
        train_config['best_iteration'] = int(model.best_iteration_) if hasattr(model, 'best_iteration_') else None
    
    config_path = os.path.join(output_dir, 'train_config.json')
    with open(config_path, 'w') as f:
        json.dump(train_config, f, indent=2)
    
    logger.info(f"  ✓ Saved training config: {config_path}")
    logger.info("=" * 80)
    
    return model_path


def evaluate_on_training_set(model, X: pd.DataFrame, y: pd.Series, output_dir: str):
    """
    在训练集上评估模型性能（用于诊断）
    """
    
    logger.info("=" * 80)
    logger.info("Evaluating model on training set...")
    logger.info("=" * 80)
    
    # 预测
    y_pred = model.predict(X)
    y_pred_proba = model.predict_proba(X)[:, 1]
    
    # 分类报告
    report = classification_report(y, y_pred, labels=[0, 1], output_dict=True, zero_division=0)
    
    # AUC
    auc = roc_auc_score(y, y_pred_proba) if y.nunique() == 2 else 0.0
    
    # 混淆矩阵
    cm = confusion_matrix(y, y_pred, labels=[0, 1])
    tn, fp, fn, tp = cm.ravel()
    
    logger.info("Classification Report:")
    logger.info(f"  - Accuracy:  {report['accuracy']:.4f}")
    logger.info(f"  - Precision: {report['1']['precision']:.4f}")
    logger.info(f"  - Recall:    {report['1']['recall']:.4f}")
    logger.info(f"  - F1 Score:  {report['1']['f1-score']:.4f}")
    logger.info(f"  - AUC:       {auc:.4f}")
    
    logger.info("\nConfusion Matrix:")
    logger.info(f"  TN: {tn:6d}  |  FP: {fp:6d}")
    logger.info(f"  FN: {fn:6d}  |  TP: {tp:6d}")
    
    # 保存训练指标
    train_metrics = {
        'model_evaluation': 'training_set',
        'train_samples': len(X),
        'accuracy': float(report['accuracy']),
        'precision': float(report['1']['precision']),
        'recall': float(report['1']['recall']),
        'f1_score': float(report['1']['f1-score']),
        'auc': float(auc),
        'confusion_matrix': {
            'tn': int(tn),
            'fp': int(fp),
            'fn': int(fn),
            'tp': int(tp),
        },
        'classification_report': report,
    }
    
    metrics_path = os.path.join(output_dir, 'train_metrics.json')
    with open(metrics_path, 'w') as f:
        json.dump(train_metrics, f, indent=2)
    
    logger.info(f"\n  ✓ Saved training metrics: {metrics_path}")
    logger.info("=" * 80)
    
    return train_metrics


def strict_training_flag(name: str, default: bool = False) -> bool:
    value = os.getenv(name)
    if value is None or value.strip() == '':
        return default
    normalized = value.strip().lower()
    if normalized not in {'true', 'false'}:
        raise ValueError(f"{name} must be explicitly true or false")
    return normalized == 'true'


def required_training_env(name: str) -> str:
    value = os.getenv(name, '').strip()
    if not value:
        raise ValueError(f"{name} is required for governed training")
    return value


def run_governed_training(model_type: str, data_dir: str, output_dir: str) -> Dict[str, Any]:
    if model_type != 'xgboost':
        raise ValueError("governed v1 currently requires the approved non-graph XGBoost baseline")
    seed = int(required_training_env('TRAIN_SEED'))
    cpu_limit_text = required_training_env('TRAIN_CPU_LIMIT')
    cpu_limit = int(cpu_limit_text)
    memory_limit = required_training_env('TRAIN_MEMORY_LIMIT')
    gpu_limit = int(required_training_env('TRAIN_GPU_LIMIT'))
    custom_params_text = os.getenv('TRAIN_MODEL_PARAMS_JSON', '').strip()
    custom_params = json.loads(custom_params_text) if custom_params_text else None
    if custom_params is not None and not isinstance(custom_params, dict):
        raise ValueError("TRAIN_MODEL_PARAMS_JSON must be a JSON object")

    X_train, y_train, X_validation, y_validation, feature_cols, dataset_manifest = \
        load_governed_training_data(data_dir)
    model = train_xgboost_governed(
        X_train, y_train, X_validation, y_validation,
        seed=seed, cpu_limit=cpu_limit, params=custom_params,
    )
    targets = [
        os.path.join(output_dir, name) for name in
        ('model.json', 'feature_importance.json', 'feature_columns.json', 'train_config.json',
         'train_metrics.json', 'training-run-manifest.json')
    ]
    for target in targets:
        if os.path.exists(target):
            raise FileExistsError(f"refusing to overwrite immutable training artifact: {target}")
    model_path = save_model(model, output_dir, model_type, feature_cols)
    evaluate_on_training_set(model, X_train, y_train, output_dir)
    artifact_paths = {
        'model': model_path,
        'feature_importance': os.path.join(output_dir, 'feature_importance.json'),
        'feature_columns': os.path.join(output_dir, 'feature_columns.json'),
        'train_config': os.path.join(output_dir, 'train_config.json'),
        'train_metrics': os.path.join(output_dir, 'train_metrics.json'),
    }
    manifest = build_training_run_manifest(
        run_id=required_training_env('TRAIN_RUN_ID'),
        dataset_manifest=dataset_manifest,
        algorithm=model_type,
        seed=seed,
        model=model,
        trainer_image_digest=required_training_env('TRAINER_IMAGE_DIGEST'),
        cpu_limit=cpu_limit_text,
        memory_limit=memory_limit,
        gpu_limit=gpu_limit,
        artifact_paths=artifact_paths,
    )
    write_json_exclusive(os.path.join(output_dir, 'training-run-manifest.json'), manifest)
    return manifest


def main():
    """主函数"""
    
    # 读取环境变量
    model_type = os.getenv('MODEL_TYPE', 'xgboost').lower()
    data_dir = os.getenv('DATA_DIR', '/data')
    output_dir = os.getenv('OUTPUT_DIR', '/output')
    governed_v1 = strict_training_flag('MLOPS_DATASET_GOVERNANCE_V1_ENABLED', False)
    
    logger.info("")
    logger.info("=" * 80)
    logger.info("🚀 MLOps Model Training Pipeline")
    logger.info("=" * 80)
    logger.info("")
    logger.info("Configuration:")
    logger.info(f"  - Model Type: {model_type}")
    logger.info(f"  - Data Directory: {data_dir}")
    logger.info(f"  - Output Directory: {output_dir}")
    logger.info(f"  - Dataset Governance v1: {governed_v1}")
    logger.info("")
    
    try:
        if governed_v1:
            manifest = run_governed_training(model_type, data_dir, output_dir)
            logger.info("Governed training complete: run_id=%s run_sha256=%s",
                        manifest['run_id'], manifest['run_sha256'])
            return
        # 1. 加载数据
        logger.info("Step 1: Loading training data...")
        train_path = os.path.join(data_dir, 'train.parquet')
        metadata_path = os.path.join(data_dir, 'metadata.json')
        
        X, y, feature_cols = load_training_data(train_path, metadata_path)
        
        # 2. 训练模型
        logger.info("\nStep 2: Training model...")
        if model_type == 'xgboost':
            model = train_xgboost(X, y)
        elif model_type == 'lightgbm':
            model = train_lightgbm(X, y)
        else:
            raise ValueError(f"Unsupported model type: {model_type}. Choose 'xgboost' or 'lightgbm'.")
        
        # 3. 保存模型
        logger.info("\nStep 3: Saving model artifacts...")
        model_path = save_model(model, output_dir, model_type, feature_cols)
        
        # 4. 训练集性能评估
        logger.info("\nStep 4: Evaluating on training set...")
        train_metrics = evaluate_on_training_set(model, X, y, output_dir)
        
        # 5. 最终总结
        logger.info("")
        logger.info("=" * 80)
        logger.info("✅ Model training completed successfully!")
        logger.info("=" * 80)
        logger.info("")
        logger.info("Summary:")
        logger.info(f"  🤖 Model Type: {model_type}")
        logger.info(f"  📊 Training Samples: {len(X)}")
        logger.info(f"  🔢 Features: {len(feature_cols)}")
        logger.info(f"  📈 F1 Score: {train_metrics['f1_score']:.4f}")
        logger.info(f"  📈 AUC: {train_metrics['auc']:.4f}")
        logger.info("")
        logger.info("Output files:")
        logger.info(f"  ✓ {model_path}")
        logger.info(f"  ✓ {os.path.join(output_dir, 'feature_importance.json')}")
        logger.info(f"  ✓ {os.path.join(output_dir, 'feature_columns.json')}")
        logger.info(f"  ✓ {os.path.join(output_dir, 'train_config.json')}")
        logger.info(f"  ✓ {os.path.join(output_dir, 'train_metrics.json')}")
        logger.info("=" * 80)
        logger.info("")
        
    except Exception as e:
        logger.error("")
        logger.error("=" * 80)
        logger.error("❌ Model training failed!")
        logger.error("=" * 80)
        logger.error(f"Error: {e}", exc_info=True)
        logger.error("=" * 80)
        sys.exit(1)


if __name__ == '__main__':
    main()
