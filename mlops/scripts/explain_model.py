#!/usr/bin/env python3
"""
Model Explainability — SHAP 特征重要性分析
缺失业务逻辑 #5: 模型可解释性 (为什么模型做出这个决策?)

Usage:
    python explain_model.py --model /model/model.json --data /data/test.parquet \
        --metadata /data/metadata.json --output /output/explain/
"""

import os, sys, json, argparse, logging
import numpy as np
import pandas as pd

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)


def load_model_and_data(model_path, data_path, metadata_path):
    """Load XGBoost model + test data + feature columns"""
    import xgboost as xgb

    logger.info(f"Loading model: {model_path}")
    model = xgb.XGBClassifier()
    model.load_model(model_path)

    logger.info(f"Loading data: {data_path}")
    df = pd.read_parquet(data_path, engine='pyarrow')

    with open(metadata_path) as f:
        metadata = json.load(f)
    feature_cols = metadata.get('feature_columns', [])
    X = df[feature_cols].fillna(0).replace([np.inf, -np.inf], 0)
    y = df['label'] if 'label' in df.columns else None

    logger.info(f"Loaded: X={X.shape}, features={len(feature_cols)}")
    return model, X, y, feature_cols


def compute_shap(model, X, feature_cols, output_dir, sample_size=200):
    """Compute SHAP values for feature importance explanation"""
    try:
        import shap
        logger.info("Computing SHAP values (TreeExplainer)...")

        # Sample data for efficiency
        if len(X) > sample_size:
            X_sample = X.sample(sample_size, random_state=42)
        else:
            X_sample = X

        # TreeExplainer (fast for XGBoost/LightGBM)
        explainer = shap.TreeExplainer(model.get_booster())
        shap_values = explainer.shap_values(X_sample)

        # === 1. Summary Plot (bar) ===
        shap.summary_plot(
            shap_values, X_sample, feature_names=feature_cols,
            plot_type="bar", show=False, max_display=15
        )
        import matplotlib.pyplot as plt
        plt.tight_layout()
        plt.savefig(os.path.join(output_dir, 'shap_summary_bar.png'), dpi=150)
        plt.close()
        logger.info("Saved shap_summary_bar.png")

        # === 2. Beeswarm Plot ===
        shap.summary_plot(
            shap_values, X_sample, feature_names=feature_cols,
            show=False, max_display=15
        )
        plt.tight_layout()
        plt.savefig(os.path.join(output_dir, 'shap_beeswarm.png'), dpi=150)
        plt.close()
        logger.info("Saved shap_beeswarm.png")

        # === 3. Feature Importance JSON ===
        shap_importance = []
        mean_abs_shap = np.abs(shap_values).mean(axis=0)
        for i, col in enumerate(feature_cols):
            if i < len(mean_abs_shap):
                shap_importance.append({
                    'feature': col,
                    'shap_importance': float(mean_abs_shap[i]),
                    'rank': i + 1
                })

        # Sort by importance
        shap_importance.sort(key=lambda x: x['shap_importance'], reverse=True)
        for i, item in enumerate(shap_importance):
            item['rank'] = i + 1

        with open(os.path.join(output_dir, 'shap_importance.json'), 'w') as f:
            json.dump(shap_importance, f, indent=2)
        logger.info(f"Saved shap_importance.json ({len(shap_importance)} features)")

        # === 4. Top 10 Text Report ===
        logger.info("\nTop 10 SHAP Features:")
        for item in shap_importance[:10]:
            logger.info(f"  {item['rank']:2d}. {item['feature']:30s} → {item['shap_importance']:.4f}")

        # === 5. Waterfall for a single prediction ===
        shap.waterfall_plot(
            shap.Explanation(
                values=shap_values[0],
                base_values=explainer.expected_value,
                data=X_sample.iloc[0].values,
                feature_names=feature_cols
            ),
            show=False, max_display=10
        )
        plt.tight_layout()
        plt.savefig(os.path.join(output_dir, 'shap_waterfall_single.png'), dpi=150)
        plt.close()
        logger.info("Saved shap_waterfall_single.png")

        return shap_importance

    except ImportError:
        logger.warning("SHAP not installed. Install: pip install shap matplotlib")
        # Fallback: use built-in XGBoost feature importance
        importance = model.get_booster().get_score(importance_type='weight')
        result = []
        for i, col in enumerate(feature_cols):
            key = f'f{i}'
            result.append({
                'feature': col,
                'xgboost_weight': float(importance.get(key, 0)),
                'rank': i + 1
            })
        result.sort(key=lambda x: x['xgboost_weight'], reverse=True)
        with open(os.path.join(output_dir, 'shap_importance.json'), 'w') as f:
            json.dump(result, f, indent=2)
        return result


def compute_permutation_importance(model, X, y, feature_cols, output_dir):
    """Complementary: Permutation Importance (model-agnostic)"""
    from sklearn.inspection import permutation_importance

    logger.info("Computing permutation importance...")
    result = permutation_importance(model, X, y, n_repeats=5, random_state=42, n_jobs=-1)

    perm_data = []
    for i, col in enumerate(feature_cols):
        perm_data.append({
            'feature': col,
            'importance_mean': float(result.importances_mean[i]),
            'importance_std': float(result.importances_std[i]),
        })
    perm_data.sort(key=lambda x: x['importance_mean'], reverse=True)

    with open(os.path.join(output_dir, 'permutation_importance.json'), 'w') as f:
        json.dump(perm_data, f, indent=2)
    logger.info(f"Saved permutation_importance.json")
    return perm_data


def main():
    parser = argparse.ArgumentParser(description='MLOps Model Explainability')
    parser.add_argument('--model', default='/model/model.json', help='Model file path')
    parser.add_argument('--data', default='/data/test.parquet', help='Test data path')
    parser.add_argument('--metadata', default='/data/metadata.json', help='Metadata path')
    parser.add_argument('--output', default='/output/explain', help='Output directory')
    parser.add_argument('--sample-size', type=int, default=200, help='Sample size for SHAP')
    args = parser.parse_args()

    os.makedirs(args.output, exist_ok=True)

    logger.info("=" * 60)
    logger.info("MLOps Model Explainability Report")
    logger.info("=" * 60)

    # 1. Load model + data
    model, X, y, feature_cols = load_model_and_data(args.model, args.data, args.metadata)

    # 2. SHAP analysis
    shap_result = compute_shap(model, X, feature_cols, args.output, args.sample_size)

    # 3. Permutation importance (if labels available)
    if y is not None:
        compute_permutation_importance(model, X, y, feature_cols, args.output)

    # 4. Generate explainability report
    report = {
        'model_path': args.model,
        'feature_count': len(feature_cols),
        'shap_analysis': 'completed' if shap_result else 'fallback',
        'top_features': shap_result[:5] if shap_result else [],
        'output_files': [f for f in os.listdir(args.output) if os.path.isfile(os.path.join(args.output, f))],
    }
    with open(os.path.join(args.output, 'explainability_report.json'), 'w') as f:
        json.dump(report, f, indent=2)

    logger.info("=" * 60)
    logger.info("Explainability report generated: %s", args.output)
    logger.info("=" * 60)


if __name__ == '__main__':
    main()
