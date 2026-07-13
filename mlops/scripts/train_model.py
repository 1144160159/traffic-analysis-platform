#!/usr/bin/env python3
"""
模型训练脚本：训练 XGBoost/LightGBM 模型
支持类别不平衡处理、超参数优化、早停机制
严格遵守生产级模型训练规范
"""

import os
import sys
import json
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

warnings.filterwarnings('ignore')

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


def main():
    """主函数"""
    
    # 读取环境变量
    model_type = os.getenv('MODEL_TYPE', 'xgboost').lower()
    data_dir = os.getenv('DATA_DIR', '/data')
    output_dir = os.getenv('OUTPUT_DIR', '/output')
    
    logger.info("")
    logger.info("=" * 80)
    logger.info("🚀 MLOps Model Training Pipeline")
    logger.info("=" * 80)
    logger.info("")
    logger.info("Configuration:")
    logger.info(f"  - Model Type: {model_type}")
    logger.info(f"  - Data Directory: {data_dir}")
    logger.info(f"  - Output Directory: {output_dir}")
    logger.info("")
    
    try:
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
