#!/usr/bin/env python3
"""
MLOps Pipeline Unit Tests
测试 extract_data, train_model, evaluate_model, register_model 的核心逻辑

Usage:
    cd mlops && python -m pytest scripts/test_mlops.py -v
    cd mlops && python scripts/test_mlops.py
"""

import os
import sys
import json
import tempfile
import unittest
import numpy as np
import pandas as pd

# Add scripts to path
sys.path.insert(0, os.path.dirname(__file__))

# Import functions under test (with graceful degradation if deps missing)
try:
    from extract_data import preprocess_data, split_and_save
    EXTRACT_OK = True
except ImportError as e:
    EXTRACT_OK = False
    EXTRACT_ERR = str(e)

try:
    from train_model import load_training_data, train_xgboost, save_model
    import xgboost as xgb
    TRAIN_OK = True
except ImportError as e:
    TRAIN_OK = False
    TRAIN_ERR = str(e)

try:
    from evaluate_model import evaluate_model, save_metrics, load_model as eval_load_model, load_test_data
    from sklearn.metrics import f1_score, precision_score, recall_score
    EVAL_OK = True
except ImportError as e:
    EVAL_OK = False
    EVAL_ERR = str(e)


class TestDataExtraction(unittest.TestCase):
    """Test extract_data.py functions"""

    @unittest.skipIf(not EXTRACT_OK, f"Dependencies missing: {EXTRACT_ERR if not EXTRACT_OK else ''}")
    def setUp(self):
        """Create a synthetic dataset that mimics the ClickHouse feature extraction output"""
        np.random.seed(42)
        n_samples = 200

        # Create features that match the extract_data schema
        data = {
            'tenant_id': ['campus-net'] * n_samples,
            'event_id': [f'evt_{i:04d}' for i in range(n_samples)],
            'probe_id': ['probe-01'] * n_samples,
            'run_id': ['realtime'] * n_samples,
            'feature_set_id': ['v1'] * n_samples,
            'community_id': [f'comm_{i:04d}' for i in range(n_samples)],
            'object_id': [f'obj_{i:04d}' for i in range(n_samples)],
            'object_type': ['session'] * n_samples,
            'ts': np.random.randint(1700000000, 1700100000, n_samples),
            'ingest_ts': np.random.randint(1700000000, 1700100000, n_samples),
            'protocol': np.random.choice([6, 17], n_samples),
            'duration_ms': np.random.exponential(5000, n_samples).astype(int),
            'pps': np.abs(np.random.normal(100, 50, n_samples)),
            'bps': np.abs(np.random.normal(10000, 5000, n_samples)),
            'up_down_ratio': np.abs(np.random.normal(1.0, 0.5, n_samples)),
            'pktlen_mean': np.abs(np.random.normal(500, 200, n_samples)),
            'pktlen_std': np.abs(np.random.normal(100, 50, n_samples)),
            'iat_mean_ms': np.abs(np.random.exponential(10, n_samples)),
            'iat_std_ms': np.abs(np.random.exponential(5, n_samples)),
            'active_mean_ms': np.abs(np.random.exponential(100, n_samples)),
            'idle_mean_ms': np.abs(np.random.exponential(50, n_samples)),
            'tcp_flag_syn_cnt': np.random.randint(0, 5, n_samples),
            'tcp_flag_ack_cnt': np.random.randint(0, 10, n_samples),
            'tcp_init_win_bytes_fwd': np.random.randint(0, 65535, n_samples),
            'tcp_init_win_bytes_bwd': np.random.randint(0, 65535, n_samples),
            'alert_type': ['scan'] * n_samples,
            'severity_str': ['medium'] * n_samples,
            'alert_score': np.random.uniform(0.5, 1.0, n_samples),
            'model_version': ['v1'] * n_samples,
            'rule_version': ['r1'] * n_samples,
            'labeled_by': ['user_001'] * n_samples,
        }
        # Label: ~40% positive (TP)
        data['label'] = np.random.choice([0, 1], n_samples, p=[0.6, 0.4])
        self.df = pd.DataFrame(data)

    def test_preprocess_handles_nan(self):
        """Test that preprocessing fills NaN values"""
        df = self.df.copy()
        df.loc[0, 'pps'] = np.nan
        df.loc[1, 'bps'] = np.nan
        result = preprocess_data(df)
        self.assertEqual(result['pps'].iloc[0], 0)
        self.assertEqual(result['bps'].iloc[1], 0)

    def test_preprocess_handles_inf(self):
        """Test that preprocessing replaces infinity"""
        df = self.df.copy()
        df.loc[0, 'pps'] = np.inf
        df.loc[1, 'bps'] = -np.inf
        result = preprocess_data(df)
        self.assertEqual(result['pps'].iloc[0], 0)
        self.assertEqual(result['bps'].iloc[1], 0)

    def test_preprocess_removes_duplicates(self):
        """Test deduplication by community_id"""
        df = self.df.copy()
        initial_len = len(df)
        # Add duplicate
        df = pd.concat([df, df.iloc[0:1]], ignore_index=True)
        result = preprocess_data(df)
        self.assertEqual(len(result), initial_len)

    def test_balanced_labels(self):
        """Should pass with both positive and negative labels"""
        result = preprocess_data(self.df)
        labels = result['label'].unique()
        self.assertIn(0, labels)
        self.assertIn(1, labels)

    def test_imbalanced_labels_raises(self):
        """Should raise ValueError if only one class present"""
        df = self.df.copy()
        df['label'] = 1  # All positive
        with self.assertRaises(ValueError):
            preprocess_data(df)

    def test_split_preserves_stratification(self):
        """Test stratified train/test split"""
        df = preprocess_data(self.df)
        with tempfile.TemporaryDirectory() as tmpdir:
            metadata = split_and_save(df, tmpdir, test_size=0.2)
            train = pd.read_parquet(os.path.join(tmpdir, 'train.parquet'))
            test = pd.read_parquet(os.path.join(tmpdir, 'test.parquet'))

            # Check split ratios
            total = len(df)
            self.assertAlmostEqual(len(test) / total, 0.2, delta=0.05)
            self.assertAlmostEqual(len(train) / total, 0.8, delta=0.05)

            # Check stratification
            train_pos_ratio = train['label'].mean()
            test_pos_ratio = test['label'].mean()
            self.assertAlmostEqual(train_pos_ratio, test_pos_ratio, delta=0.05)

    def test_metadata_output(self):
        """Test metadata contains required fields"""
        df = preprocess_data(self.df)
        with tempfile.TemporaryDirectory() as tmpdir:
            split_and_save(df, tmpdir)
            with open(os.path.join(tmpdir, 'metadata.json')) as f:
                metadata = json.load(f)
            self.assertIn('feature_columns', metadata)
            self.assertIn('train_samples', metadata)
            self.assertIn('test_samples', metadata)
            self.assertIn('total_features', metadata)
            self.assertTrue(len(metadata['feature_columns']) > 0)


class TestModelTraining(unittest.TestCase):
    """Test train_model.py functions"""

    @unittest.skipIf(not TRAIN_OK, f"Dependencies missing: {TRAIN_ERR if not TRAIN_OK else ''}")
    def setUp(self):
        np.random.seed(42)
        n_samples = 200
        n_features = 10

        # Create features
        X = np.random.randn(n_samples, n_features)
        # Create labels with some signal
        y = (X[:, 0] + X[:, 1] > 0).astype(int)

        feature_cols = [f'feat_{i}' for i in range(n_features)]
        self.X = pd.DataFrame(X, columns=feature_cols)
        self.y = pd.Series(y, name='label')
        self.feature_cols = feature_cols

        # Create temp files
        self.tmpdir = tempfile.mkdtemp()
        df = self.X.copy()
        df['label'] = self.y
        df.to_parquet(os.path.join(self.tmpdir, 'train.parquet'), index=False)

        metadata = {'feature_columns': feature_cols}
        with open(os.path.join(self.tmpdir, 'metadata.json'), 'w') as f:
            json.dump(metadata, f)

    def tearDown(self):
        import shutil
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_xgboost_training(self):
        """Test XGBoost model training pipeline"""
        model = train_xgboost(self.X, self.y)
        self.assertIsNotNone(model)
        # Check it can predict
        preds = model.predict(self.X)
        self.assertEqual(len(preds), len(self.y))

    def test_xgboost_save_and_load(self):
        """Test model save/load roundtrip"""
        model = train_xgboost(self.X, self.y)
        output_dir = tempfile.mkdtemp()
        try:
            model_path = save_model(model, output_dir, 'xgboost', self.feature_cols)
            self.assertTrue(os.path.exists(model_path))

            # Load back
            loaded = xgb.XGBClassifier()
            loaded.load_model(model_path)
            preds_orig = model.predict(self.X)
            preds_loaded = loaded.predict(self.X)
            np.testing.assert_array_equal(preds_orig, preds_loaded)
        finally:
            import shutil
            shutil.rmtree(output_dir, ignore_errors=True)

    def test_feature_importance_saved(self):
        """Test feature importance is saved"""
        model = train_xgboost(self.X, self.y)
        output_dir = tempfile.mkdtemp()
        try:
            save_model(model, output_dir, 'xgboost', self.feature_cols)
            with open(os.path.join(output_dir, 'feature_importance.json')) as f:
                importance = json.load(f)
            self.assertTrue(len(importance) > 0)
        finally:
            import shutil
            shutil.rmtree(output_dir, ignore_errors=True)

    def test_class_imbalance_handling(self):
        """Test training with imbalanced data"""
        # Create moderately imbalanced data (enough for validation split)
        y_imb = pd.Series([0] * 180 + [1] * 20)
        X_imb = pd.DataFrame(np.random.randn(200, 5))

        model = train_xgboost(X_imb, y_imb)
        self.assertIsNotNone(model)


class TestModelEvaluation(unittest.TestCase):
    """Test evaluate_model.py functions"""

    @unittest.skipIf(not EVAL_OK, f"Dependencies missing: {EVAL_ERR if not EVAL_OK else ''}")
    def setUp(self):
        np.random.seed(42)
        n_samples = 200
        n_features = 10

        X = np.random.randn(n_samples, n_features)
        y = (X[:, 0] + X[:, 1] > 0).astype(int)

        feature_cols = [f'feat_{i}' for i in range(n_features)]
        self.X = pd.DataFrame(X, columns=feature_cols)
        self.y = pd.Series(y, name='label')
        self.feature_cols = feature_cols

        # Train a model
        self.model = xgb.XGBClassifier(n_estimators=50, max_depth=3, random_state=42)
        self.model.fit(self.X, self.y)

    def test_evaluate_metrics(self):
        """Test evaluation produces all required metrics"""
        metrics, y_pred_proba = evaluate_model(self.model, self.X, self.y, 'xgboost')
        self.assertIn('f1_score', metrics)
        self.assertIn('precision', metrics)
        self.assertIn('recall', metrics)
        self.assertIn('auc_roc', metrics)
        self.assertIn('confusion_matrix', metrics)
        self.assertTrue(0 <= metrics['f1_score'] <= 1)
        self.assertTrue(0 <= metrics['auc_roc'] <= 1)

    def test_confusion_matrix_sum(self):
        """Test confusion matrix TN+FP+FN+TP = total samples"""
        metrics, _ = evaluate_model(self.model, self.X, self.y, 'xgboost')
        cm = metrics['confusion_matrix']
        total = cm['tn'] + cm['fp'] + cm['fn'] + cm['tp']
        self.assertEqual(total, len(self.y))

    def test_save_metrics(self):
        """Test metrics are saved correctly"""
        metrics, _ = evaluate_model(self.model, self.X, self.y, 'xgboost')
        with tempfile.TemporaryDirectory() as tmpdir:
            save_metrics(metrics, tmpdir)
            self.assertTrue(os.path.exists(os.path.join(tmpdir, 'metrics.json')))
            self.assertTrue(os.path.exists(os.path.join(tmpdir, 'f1_score.txt')))
            self.assertTrue(os.path.exists(os.path.join(tmpdir, 'summary.json')))

    def test_f1_score_txt_format(self):
        """Test f1_score.txt contains a parseable float"""
        metrics, _ = evaluate_model(self.model, self.X, self.y, 'xgboost')
        with tempfile.TemporaryDirectory() as tmpdir:
            save_metrics(metrics, tmpdir)
            with open(os.path.join(tmpdir, 'f1_score.txt')) as f:
                f1_str = f.read().strip()
            f1 = float(f1_str)
            self.assertAlmostEqual(f1, metrics['f1_score'], places=4)

    def test_best_threshold(self):
        """Test best threshold is between 0 and 1"""
        metrics, _ = evaluate_model(self.model, self.X, self.y, 'xgboost')
        self.assertIn('best_threshold', metrics)
        self.assertTrue(0 <= metrics['best_threshold'] <= 1)


class TestIntegration(unittest.TestCase):
    """End-to-end integration test of MLOps pipeline"""

    @unittest.skipIf(not (EXTRACT_OK and TRAIN_OK and EVAL_OK), "Dependencies missing")
    def test_full_pipeline(self):
        """Test the complete pipeline: extract → train → evaluate"""
        np.random.seed(42)

        # 1. Create synthetic data
        n_samples = 300
        data = {
            'tenant_id': ['campus-net'] * n_samples,
            'event_id': [f'evt_{i:04d}' for i in range(n_samples)],
            'probe_id': ['probe-01'] * n_samples,
            'run_id': ['realtime'] * n_samples,
            'feature_set_id': ['v1'] * n_samples,
            'community_id': [f'comm_{i:04d}' for i in range(n_samples)],
            'object_id': [f'obj_{i:04d}' for i in range(n_samples)],
            'object_type': ['session'] * n_samples,
            'ts': np.random.randint(1700000000, 1700100000, n_samples),
            'ingest_ts': np.random.randint(1700000000, 1700100000, n_samples),
            'protocol': np.random.choice([6, 17], n_samples),
            'duration_ms': np.random.exponential(5000, n_samples).astype(int),
            'pps': np.abs(np.random.normal(100, 50, n_samples)),
            'bps': np.abs(np.random.normal(10000, 5000, n_samples)),
            'up_down_ratio': np.abs(np.random.normal(1.0, 0.5, n_samples)),
            'pktlen_mean': np.abs(np.random.normal(500, 200, n_samples)),
            'pktlen_std': np.abs(np.random.normal(100, 50, n_samples)),
            'iat_mean_ms': np.abs(np.random.exponential(10, n_samples)),
            'iat_std_ms': np.abs(np.random.exponential(5, n_samples)),
            'active_mean_ms': np.abs(np.random.exponential(100, n_samples)),
            'idle_mean_ms': np.abs(np.random.exponential(50, n_samples)),
            'tcp_flag_syn_cnt': np.random.randint(0, 5, n_samples),
            'tcp_flag_ack_cnt': np.random.randint(0, 10, n_samples),
            'tcp_init_win_bytes_fwd': np.random.randint(0, 65535, n_samples),
            'tcp_init_win_bytes_bwd': np.random.randint(0, 65535, n_samples),
            'alert_type': ['scan'] * n_samples,
            'severity_str': ['medium'] * n_samples,
            'alert_score': np.random.uniform(0.5, 1.0, n_samples),
            'model_version': ['v1'] * n_samples,
            'rule_version': ['r1'] * n_samples,
            'labeled_by': ['user_001'] * n_samples,
        }
        data['label'] = np.random.choice([0, 1], n_samples, p=[0.6, 0.4])
        df = pd.DataFrame(data)

        with tempfile.TemporaryDirectory() as tmpdir:
            # 2. Preprocess & split
            df = preprocess_data(df)
            metadata = split_and_save(df, tmpdir)

            # 3. Train
            X, y, feature_cols = load_training_data(
                os.path.join(tmpdir, 'train.parquet'),
                os.path.join(tmpdir, 'metadata.json')
            )
            model = train_xgboost(X, y)
            model_dir = os.path.join(tmpdir, 'model')
            save_model(model, model_dir, 'xgboost', feature_cols)

            # 4. Evaluate
            X_test, y_test, _ = load_test_data(
                os.path.join(tmpdir, 'test.parquet'),
                os.path.join(tmpdir, 'metadata.json')
            )
            metrics, _ = evaluate_model(model, X_test, y_test, 'xgboost')

            # 5. Assertions
            self.assertGreater(metrics['auc_roc'], 0.5,
                             f"AUC-ROC {metrics['auc_roc']} should be > 0.5 (better than random)")
            self.assertTrue(0 <= metrics['f1_score'] <= 1)
            self.assertIn('confusion_matrix', metrics)


if __name__ == '__main__':
    print("=" * 70)
    print("MLOps Pipeline Unit Tests")
    print("=" * 70)
    print(f"  extract_data.py: {'READY' if EXTRACT_OK else 'SKIP (' + EXTRACT_ERR + ')'}")
    print(f"  train_model.py:  {'READY' if TRAIN_OK else 'SKIP (' + TRAIN_ERR + ')'}")
    print(f"  evaluate_model.py: {'READY' if EVAL_OK else 'SKIP (' + EVAL_ERR + ')'}")
    print("=" * 70)
    print()

    unittest.main(verbosity=2)
