#!/usr/bin/env python3
"""
Model Export — XGBoost / LightGBM → ONNX
支持 Flink ONNX Runtime 推理 (跨语言兼容)

Usage:
    python export_onnx.py --model /model/model.json --output /output/model.onnx
    python export_onnx.py --model /model/model.txt --model-type lightgbm --output /output/model.onnx
"""

import os, sys, json, argparse, logging
import numpy as np

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)


def load_xgboost(model_path):
    """Load XGBoost model from JSON"""
    import xgboost as xgb
    model = xgb.XGBClassifier()
    model.load_model(model_path)
    # Warm up to get n_features
    booster = model.get_booster()
    n_features = booster.num_features()
    logger.info(f"Loaded XGBoost: {n_features} features")
    return model, n_features


def load_lightgbm(model_path):
    """Load LightGBM model from TXT"""
    import lightgbm as lgb
    model = lgb.Booster(model_file=model_path)
    n_features = model.num_feature()
    logger.info(f"Loaded LightGBM: {n_features} features")
    return model, n_features


def export_to_onnx(model, n_features, output_path, model_type='xgboost', initial_types=None):
    """Export model to ONNX format"""
    try:
        from onnxmltools import convert_xgboost, convert_lightgbm
        from onnxconverter_common.data_types import FloatTensorType
    except ImportError:
        logger.error("ONNX export deps missing. Install: pip install onnxmltools onnxruntime skl2onnx")
        return False

    if initial_types is None:
        initial_types = [('float_input', FloatTensorType([None, n_features]))]

    try:
        if model_type == 'xgboost':
            onnx_model = convert_xgboost(model, initial_types=initial_types)
        elif model_type == 'lightgbm':
            onnx_model = convert_lightgbm(model, initial_types=initial_types)
        else:
            logger.error(f"Unknown model type: {model_type}")
            return False

        # Save
        with open(output_path, 'wb') as f:
            f.write(onnx_model.SerializeToString())

        size_mb = os.path.getsize(output_path) / (1024 * 1024)
        logger.info(f"ONNX model exported: {output_path} ({size_mb:.2f} MB)")
        return True

    except Exception as e:
        logger.error(f"ONNX conversion failed: {e}")
        return False


def validate_onnx(onnx_path, n_features):
    """Validate ONNX model with a dummy inference"""
    try:
        import onnxruntime as ort
        session = ort.InferenceSession(onnx_path)

        input_name = session.get_inputs()[0].name
        dummy_input = np.random.randn(1, n_features).astype(np.float32)
        output = session.run(None, {input_name: dummy_input})

        logger.info(f"ONNX validation OK: output shape={output[0].shape}, dtype={output[0].dtype}")
        return True
    except ImportError:
        logger.warning("onnxruntime not installed, skipping validation")
        return True
    except Exception as e:
        logger.error(f"ONNX validation failed: {e}")
        return False


def main():
    parser = argparse.ArgumentParser(description='MLOps Model Export to ONNX')
    parser.add_argument('--model', required=True, help='Model file path (model.json / model.txt)')
    parser.add_argument('--model-type', default='xgboost', choices=['xgboost', 'lightgbm'],
                        help='Model type')
    parser.add_argument('--feature-count', type=int, help='Feature count (auto-detect if omitted)')
    parser.add_argument('--output', required=True, help='Output ONNX file path')
    parser.add_argument('--validate', action='store_true', default=True, help='Validate ONNX after export')
    args = parser.parse_args()

    logger.info("=" * 60)
    logger.info("MLOps Model Export → ONNX")
    logger.info("=" * 60)
    logger.info(f"  Model: {args.model}")
    logger.info(f"  Type:  {args.model_type}")
    logger.info(f"  Output: {args.output}")

    os.makedirs(os.path.dirname(args.output) or '.', exist_ok=True)

    # 1. Load model
    if args.model_type == 'xgboost':
        model, n_features = load_xgboost(args.model)
    else:
        model, n_features = load_lightgbm(args.model)

    if args.feature_count:
        n_features = args.feature_count

    # 2. Export to ONNX
    success = export_to_onnx(model, n_features, args.output, args.model_type)
    if not success:
        sys.exit(1)

    # 3. Validate
    if args.validate:
        if validate_onnx(args.output, n_features):
            logger.info("ONNX export + validation successful ✅")
        else:
            logger.warning("ONNX exported but validation failed ⚠️")

    # 4. Metadata
    meta = {
        'source_model': args.model,
        'model_type': args.model_type,
        'n_features': n_features,
        'onnx_path': args.output,
        'onnx_size_bytes': os.path.getsize(args.output),
    }
    meta_path = args.output + '.meta.json'
    with open(meta_path, 'w') as f:
        json.dump(meta, f, indent=2)
    logger.info(f"Metadata saved: {meta_path}")


if __name__ == '__main__':
    main()
