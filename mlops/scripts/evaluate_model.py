#!/usr/bin/env python3
"""
模型评估脚本：在测试集上评估模型性能
生成详细指标报告、PR曲线、ROC曲线、错误分析
严格遵守生产级评估标准
"""

import os
import sys
import json
import pandas as pd
import numpy as np
import xgboost as xgb
import lightgbm as lgb
from sklearn.metrics import (
    classification_report, 
    confusion_matrix,
    roc_auc_score,
    precision_recall_curve,
    roc_curve,
    f1_score,
    precision_score,
    recall_score,
    accuracy_score,
    average_precision_score
)
import logging
from typing import Tuple, Dict, Any
import warnings

warnings.filterwarnings('ignore')

# 配置日志
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def load_model(model_path: str, model_type: str):
    """
    加载训练好的模型
    
    支持：
    - XGBoost (.json)
    - LightGBM (.txt)
    """
    
    logger.info("=" * 80)
    logger.info(f"Loading {model_type} model...")
    logger.info("=" * 80)
    
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"Model file not found: {model_path}")
    
    logger.info(f"Model path: {model_path}")
    
    try:
        if model_type == 'xgboost':
            model = xgb.XGBClassifier()
            model.load_model(model_path)
            logger.info("  ✓ XGBoost model loaded successfully")
            
        elif model_type == 'lightgbm':
            model = lgb.Booster(model_file=model_path)
            logger.info("  ✓ LightGBM model loaded successfully")
            
        else:
            raise ValueError(f"Unsupported model type: {model_type}")
        
        logger.info("=" * 80)
        return model
        
    except Exception as e:
        logger.error(f"Failed to load model: {e}")
        raise


def load_test_data(data_path: str, metadata_path: str) -> Tuple[pd.DataFrame, pd.Series, list]:
    """
    加载测试数据
    
    验证：
    - Parquet 文件完整性
    - 元数据一致性
    - 特征列完整性
    """
    
    logger.info("=" * 80)
    logger.info("Loading test data...")
    logger.info("=" * 80)
    
    if not os.path.exists(data_path):
        raise FileNotFoundError(f"Test data not found: {data_path}")
    
    if not os.path.exists(metadata_path):
        raise FileNotFoundError(f"Metadata not found: {metadata_path}")
    
    # 加载测试集
    logger.info(f"Reading test data: {data_path}")
    df = pd.read_parquet(data_path, engine='pyarrow')
    logger.info(f"  ✓ Loaded {len(df)} test samples")
    
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
        raise ValueError(f"Missing feature columns in test data: {missing_cols}")
    
    # 验证标签列
    if 'label' not in df.columns:
        raise ValueError("Label column 'label' not found in test data!")
    
    # 提取特征和标签
    X = df[feature_cols].copy()
    y = df['label'].copy()
    
    # 数据质量检查
    logger.info("\nTest data validation:")
    logger.info(f"  - X shape: {X.shape}")
    logger.info(f"  - y shape: {y.shape}")
    logger.info(f"  - Positive samples: {y.sum()}")
    logger.info(f"  - Negative samples: {(y == 0).sum()}")
    
    # 检查 NaN
    nan_count = X.isnull().sum().sum()
    if nan_count > 0:
        logger.warning(f"  ⚠️  Found {nan_count} NaN values, filling with 0")
        X = X.fillna(0)
    
    # 检查无穷大
    inf_count = np.isinf(X.select_dtypes(include=[np.number])).sum().sum()
    if inf_count > 0:
        logger.warning(f"  ⚠️  Found {inf_count} infinity values, replacing with 0")
        X = X.replace([np.inf, -np.inf], 0)
    
    logger.info("=" * 80)
    
    return X, y, feature_cols


def evaluate_model(model, X: pd.DataFrame, y: pd.Series, model_type: str) -> Tuple[Dict[str, Any], np.ndarray]:
    """
    全面评估模型
    
    计算指标：
    - 准确率、精确率、召回率、F1
    - AUC-ROC、AUC-PR
    - 混淆矩阵
    - PR曲线、ROC曲线
    - 最佳阈值分析
    """
    
    logger.info("=" * 80)
    logger.info("Evaluating model on test set...")
    logger.info("=" * 80)
    
    # 预测
    logger.info("Making predictions...")
    
    if model_type == 'xgboost':
        y_pred = model.predict(X)
        y_pred_proba = model.predict_proba(X)[:, 1]
        
    elif model_type == 'lightgbm':
        y_pred_proba = model.predict(X)
        y_pred = (y_pred_proba > 0.5).astype(int)
    
    logger.info("  ✓ Predictions completed")
    
    # 基础指标
    logger.info("\nCalculating metrics...")
    
    has_both_classes = y.nunique() == 2
    accuracy = accuracy_score(y, y_pred)
    precision = precision_score(y, y_pred, zero_division=0)
    recall = recall_score(y, y_pred, zero_division=0)
    f1 = f1_score(y, y_pred, zero_division=0)
    auc_roc = roc_auc_score(y, y_pred_proba) if has_both_classes else 0.0
    auc_pr = average_precision_score(y, y_pred_proba) if has_both_classes else 0.0
    
    logger.info("\nClassification Metrics:")
    logger.info(f"  - Accuracy:  {accuracy:.4f}")
    logger.info(f"  - Precision: {precision:.4f}")
    logger.info(f"  - Recall:    {recall:.4f}")
    logger.info(f"  - F1 Score:  {f1:.4f}")
    logger.info(f"  - AUC-ROC:   {auc_roc:.4f}")
    logger.info(f"  - AUC-PR:    {auc_pr:.4f}")
    
    # 混淆矩阵
    cm = confusion_matrix(y, y_pred, labels=[0, 1])
    tn, fp, fn, tp = cm.ravel()
    
    logger.info("\nConfusion Matrix:")
    logger.info(f"  ┌─────────┬─────────┐")
    logger.info(f"  │ TN: {tn:4d} │ FP: {fp:4d} │")
    logger.info(f"  ├─────────┼─────────┤")
    logger.info(f"  │ FN: {fn:4d} │ TP: {tp:4d} │")
    logger.info(f"  └─────────┴─────────┘")
    
    # 分类报告
    report = classification_report(y, y_pred, labels=[0, 1], output_dict=True, zero_division=0)
    
    # PR 曲线和 ROC 曲线
    logger.info("\nCalculating PR and ROC curves...")
    if has_both_classes:
        precision_curve, recall_curve, pr_thresholds = precision_recall_curve(y, y_pred_proba)
        fpr, tpr, roc_thresholds = roc_curve(y, y_pred_proba)

        # 找到最佳阈值（F1 最大）
        f1_scores = 2 * (precision_curve * recall_curve) / (precision_curve + recall_curve + 1e-10)
        best_threshold_idx = np.argmax(f1_scores[:-1])  # 排除最后一个点
        best_threshold = pr_thresholds[best_threshold_idx]
    else:
        logger.warning("Only one class is present in the test set; curve metrics are set to defaults")
        precision_curve = np.array([precision])
        recall_curve = np.array([recall])
        pr_thresholds = np.array([0.5])
        fpr = np.array([0.0, 1.0])
        tpr = np.array([0.0, 1.0])
        roc_thresholds = np.array([1.0, 0.0])
        f1_scores = np.array([f1])
        best_threshold_idx = 0
        best_threshold = 0.5
    
    logger.info(f"\nBest Threshold Analysis (maximizing F1):")
    logger.info(f"  - Threshold:  {best_threshold:.4f}")
    logger.info(f"  - Precision:  {precision_curve[best_threshold_idx]:.4f}")
    logger.info(f"  - Recall:     {recall_curve[best_threshold_idx]:.4f}")
    logger.info(f"  - F1 Score:   {f1_scores[best_threshold_idx]:.4f}")
    
    # 构建完整指标字典
    metrics = {
        # 基础指标
        'accuracy': float(accuracy),
        'precision': float(precision),
        'recall': float(recall),
        'f1_score': float(f1),
        'auc_roc': float(auc_roc),
        'auc_pr': float(auc_pr),
        
        # 混淆矩阵
        'confusion_matrix': {
            'tn': int(tn),
            'fp': int(fp),
            'fn': int(fn),
            'tp': int(tp),
        },
        
        # 样本统计
        'test_samples': len(y),
        'positive_samples': int(y.sum()),
        'negative_samples': int((y == 0).sum()),
        
        # 分类报告
        'classification_report': report,
        
        # 最佳阈值
        'best_threshold': float(best_threshold),
        'best_threshold_metrics': {
            'precision': float(precision_curve[best_threshold_idx]),
            'recall': float(recall_curve[best_threshold_idx]),
            'f1_score': float(f1_scores[best_threshold_idx]),
        },
    }
    
    # PR 曲线数据（采样到 100 个点）
    n_pr_points = min(100, len(precision_curve))
    sample_indices = np.linspace(0, len(precision_curve) - 1, n_pr_points, dtype=int)

    # 阈值数组通常比 precision/recall 少一个元素
    n_thresh = len(pr_thresholds)
    thresh_indices = np.clip(sample_indices, 0, n_thresh - 1)

    metrics['pr_curve'] = {
        'precision': precision_curve[sample_indices].tolist(),
        'recall': recall_curve[sample_indices].tolist(),
        'thresholds': pr_thresholds[thresh_indices].tolist(),
    }
    
    # ROC 曲线数据（采样到 100 个点）
    sample_indices_roc = np.linspace(0, len(fpr) - 1, min(100, len(fpr)), dtype=int)
    
    metrics['roc_curve'] = {
        'fpr': fpr[sample_indices_roc].tolist(),
        'tpr': tpr[sample_indices_roc].tolist(),
        'thresholds': roc_thresholds[sample_indices_roc].tolist(),
    }
    
    logger.info("=" * 80)
    
    return metrics, y_pred_proba


def save_metrics(metrics: Dict[str, Any], output_dir: str):
    """
    保存评估指标
    
    生成文件：
    1. metrics.json - 完整指标
    2. summary.json - 简化摘要
    3. f1_score.txt - F1值（供 Argo Workflow 使用）
    """
    
    logger.info("=" * 80)
    logger.info("Saving evaluation metrics...")
    logger.info("=" * 80)
    
    os.makedirs(output_dir, exist_ok=True)
    
    # 1. 完整指标
    metrics_path = os.path.join(output_dir, 'metrics.json')
    with open(metrics_path, 'w') as f:
        json.dump(metrics, f, indent=2)
    
    logger.info(f"  ✓ Saved metrics: {metrics_path}")
    
    # 2. F1 Score（单独文件，供 Argo Workflow 条件判断）
    f1_path = os.path.join(output_dir, 'f1_score.txt')
    with open(f1_path, 'w') as f:
        f.write(str(metrics['f1_score']))
    
    logger.info(f"  ✓ Saved F1 score: {f1_path}")
    
    # 3. 简化摘要
    summary = {
        'f1_score': metrics['f1_score'],
        'precision': metrics['precision'],
        'recall': metrics['recall'],
        'auc_roc': metrics['auc_roc'],
        'auc_pr': metrics['auc_pr'],
        'accuracy': metrics['accuracy'],
        'best_threshold': metrics['best_threshold'],
        'test_samples': metrics['test_samples'],
        'confusion_matrix': metrics['confusion_matrix'],
    }
    
    summary_path = os.path.join(output_dir, 'summary.json')
    with open(summary_path, 'w') as f:
        json.dump(summary, f, indent=2)
    
    logger.info(f"  ✓ Saved summary: {summary_path}")
    logger.info("=" * 80)


def generate_error_analysis(y_true: pd.Series, y_pred: np.ndarray, y_pred_proba: np.ndarray, 
                            X: pd.DataFrame, feature_cols: list, output_dir: str):
    """
    生成错误分析报告
    
    分析内容：
    - 假阳性（FP）样本分析
    - 假阴性（FN）样本分析
    - 错误样本的特征分布
    - 置信度分布
    """
    
    logger.info("=" * 80)
    logger.info("Generating error analysis...")
    logger.info("=" * 80)
    
    # 找到误分类样本
    false_positives = (y_true == 0) & (y_pred == 1)
    false_negatives = (y_true == 1) & (y_pred == 0)
    
    fp_count = false_positives.sum()
    fn_count = false_negatives.sum()
    
    logger.info(f"Error Analysis Summary:")
    logger.info(f"  - False Positives: {fp_count} ({fp_count/len(y_true)*100:.2f}%)")
    logger.info(f"  - False Negatives: {fn_count} ({fn_count/len(y_true)*100:.2f}%)")
    
    error_analysis = {
        'false_positives': {
            'count': int(fp_count),
            'percentage': float(fp_count / len(y_true) * 100),
            'sample_indices': np.where(false_positives)[0][:10].tolist(),
        },
        'false_negatives': {
            'count': int(fn_count),
            'percentage': float(fn_count / len(y_true) * 100),
            'sample_indices': np.where(false_negatives)[0][:10].tolist(),
        },
    }
    
    # 分析 FP 的特征分布
    if fp_count > 0:
        fp_features = X[false_positives].mean()
        fp_proba = y_pred_proba[false_positives]
        
        error_analysis['false_positives']['mean_features'] = fp_features.to_dict()
        error_analysis['false_positives']['mean_confidence'] = float(fp_proba.mean())
        error_analysis['false_positives']['min_confidence'] = float(fp_proba.min())
        error_analysis['false_positives']['max_confidence'] = float(fp_proba.max())
        
        logger.info(f"\nFalse Positive Analysis:")
        logger.info(f"  - Mean confidence: {fp_proba.mean():.4f}")
        logger.info(f"  - Confidence range: [{fp_proba.min():.4f}, {fp_proba.max():.4f}]")
    
    # 分析 FN 的特征分布
    if fn_count > 0:
        fn_features = X[false_negatives].mean()
        fn_proba = y_pred_proba[false_negatives]
        
        error_analysis['false_negatives']['mean_features'] = fn_features.to_dict()
        error_analysis['false_negatives']['mean_confidence'] = float(fn_proba.mean())
        error_analysis['false_negatives']['min_confidence'] = float(fn_proba.min())
        error_analysis['false_negatives']['max_confidence'] = float(fn_proba.max())
        
        logger.info(f"\nFalse Negative Analysis:")
        logger.info(f"  - Mean confidence: {fn_proba.mean():.4f}")
        logger.info(f"  - Confidence range: [{fn_proba.min():.4f}, {fn_proba.max():.4f}]")
    
    # 保存错误分析
    error_path = os.path.join(output_dir, 'error_analysis.json')
    with open(error_path, 'w') as f:
        json.dump(error_analysis, f, indent=2)
    
    logger.info(f"\n  ✓ Saved error analysis: {error_path}")
    logger.info("=" * 80)


def main():
    """主函数"""
    
    # 读取环境变量
    model_type = os.getenv('MODEL_TYPE', 'xgboost').lower()
    model_dir = os.getenv('MODEL_DIR', '/model')
    data_dir = os.getenv('DATA_DIR', '/data')
    output_dir = os.getenv('OUTPUT_DIR', '/output')
    min_f1_score = float(os.getenv('MIN_F1_SCORE', '0.85'))
    
    logger.info("")
    logger.info("=" * 80)
    logger.info("🚀 MLOps Model Evaluation Pipeline")
    logger.info("=" * 80)
    logger.info("")
    logger.info("Configuration:")
    logger.info(f"  - Model Type: {model_type}")
    logger.info(f"  - Model Directory: {model_dir}")
    logger.info(f"  - Data Directory: {data_dir}")
    logger.info(f"  - Output Directory: {output_dir}")
    logger.info(f"  - Min F1 Threshold: {min_f1_score}")
    logger.info("")
    
    try:
        # 1. 加载模型
        logger.info("Step 1: Loading trained model...")
        
        if model_type == 'xgboost':
            model_path = os.path.join(model_dir, 'model.json')
        elif model_type == 'lightgbm':
            model_path = os.path.join(model_dir, 'model.txt')
        else:
            raise ValueError(f"Unsupported model type: {model_type}")
        
        model = load_model(model_path, model_type)
        
        # 2. 加载测试数据
        logger.info("\nStep 2: Loading test data...")
        test_path = os.path.join(data_dir, 'test.parquet')
        metadata_path = os.path.join(data_dir, 'metadata.json')
        
        X, y, feature_cols = load_test_data(test_path, metadata_path)
        
        # 3. 评估模型
        logger.info("\nStep 3: Evaluating model...")
        metrics, y_pred_proba = evaluate_model(model, X, y, model_type)
        
        # 4. 保存指标
        logger.info("\nStep 4: Saving metrics...")
        save_metrics(metrics, output_dir)
        
        # 5. 错误分析
        logger.info("\nStep 5: Performing error analysis...")
        y_pred = (y_pred_proba > 0.5).astype(int)
        generate_error_analysis(y, y_pred, y_pred_proba, X, feature_cols, output_dir)
        
        # 6. 最终总结
        logger.info("")
        logger.info("=" * 80)
        logger.info("✅ Model evaluation completed successfully!")
        logger.info("=" * 80)
        logger.info("")
        logger.info("Evaluation Summary:")
        logger.info(f"  📊 Test Samples: {metrics['test_samples']}")
        logger.info(f"  📈 F1 Score: {metrics['f1_score']:.4f}")
        logger.info(f"  📈 Precision: {metrics['precision']:.4f}")
        logger.info(f"  📈 Recall: {metrics['recall']:.4f}")
        logger.info(f"  📈 AUC-ROC: {metrics['auc_roc']:.4f}")
        logger.info(f"  📈 AUC-PR: {metrics['auc_pr']:.4f}")
        logger.info(f"  🎯 Best Threshold: {metrics['best_threshold']:.4f}")
        logger.info("")
        
        # 检查部署条件
        if metrics['f1_score'] >= min_f1_score:
            logger.info(f"✅ Model MEETS deployment criteria (F1 {metrics['f1_score']:.4f} >= {min_f1_score})")
            logger.info("   → Model will be registered and deployed")
        else:
            logger.warning(f"⚠️  Model does NOT meet deployment criteria (F1 {metrics['f1_score']:.4f} < {min_f1_score})")
            logger.warning("   → Model will NOT be registered")
        
        logger.info("")
        logger.info("Output files:")
        logger.info(f"  ✓ {os.path.join(output_dir, 'metrics.json')}")
        logger.info(f"  ✓ {os.path.join(output_dir, 'summary.json')}")
        logger.info(f"  ✓ {os.path.join(output_dir, 'f1_score.txt')}")
        logger.info(f"  ✓ {os.path.join(output_dir, 'error_analysis.json')}")
        logger.info("=" * 80)
        logger.info("")
        
    except Exception as e:
        logger.error("")
        logger.error("=" * 80)
        logger.error("❌ Model evaluation failed!")
        logger.error("=" * 80)
        logger.error(f"Error: {e}", exc_info=True)
        logger.error("=" * 80)
        sys.exit(1)


if __name__ == '__main__':
    main()
