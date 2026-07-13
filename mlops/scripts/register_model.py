#!/usr/bin/env python3
"""
模型注册脚本：将训练好的模型注册到 Model Registry
上传到 MinIO，并通知 Flink 作业热加载
"""

import os
import sys
import json
import argparse
import requests
from datetime import datetime
from minio import Minio
from minio.error import S3Error
import logging
from urllib.parse import quote

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


def auth_headers():
    headers = {
        'Content-Type': 'application/json',
    }
    api_token = os.getenv('API_TOKEN', '')
    if api_token:
        headers['Authorization'] = f'Bearer {api_token}'
    return headers


def upload_to_minio(model_path, model_type, version):
    """上传模型到 MinIO"""
    
    # MinIO 配置
    endpoint = os.getenv('MINIO_ENDPOINT', 'minio:9000')
    access_key = os.getenv('MINIO_ACCESS_KEY', 'minioadmin')
    secret_key = os.getenv('MINIO_SECRET_KEY', 'minioadmin')
    bucket_name = os.getenv('MINIO_BUCKET', 'traffic-models')
    secure = os.getenv('MINIO_SECURE', 'false').lower() == 'true'
    
    logger.info(f"Connecting to MinIO: {endpoint}")
    
    # 创建 MinIO 客户端
    client = Minio(
        endpoint,
        access_key=access_key,
        secret_key=secret_key,
        secure=secure
    )
    
    # 确保 bucket 存在
    if not client.bucket_exists(bucket_name):
        logger.info(f"Creating bucket: {bucket_name}")
        client.make_bucket(bucket_name)
    
    # 构建对象键
    if model_type == 'xgboost':
        file_ext = 'json'
    elif model_type == 'lightgbm':
        file_ext = 'txt'
    else:
        file_ext = 'model'
    
    object_name = f"models/{version}/model.{file_ext}"
    
    # 上传模型
    logger.info(f"Uploading model to s3://{bucket_name}/{object_name}")
    
    client.fput_object(
        bucket_name,
        object_name,
        model_path,
        content_type='application/octet-stream'
    )
    
    logger.info(f"Model uploaded successfully")
    
    # 构建 S3 URI
    s3_uri = f"s3://{bucket_name}/{object_name}"
    
    return s3_uri


def upload_artifacts(model_dir, version):
    """上传模型相关的所有文件"""
    
    endpoint = os.getenv('MINIO_ENDPOINT', 'minio:9000')
    access_key = os.getenv('MINIO_ACCESS_KEY', 'minioadmin')
    secret_key = os.getenv('MINIO_SECRET_KEY', 'minioadmin')
    bucket_name = os.getenv('MINIO_BUCKET', 'traffic-models')
    secure = os.getenv('MINIO_SECURE', 'false').lower() == 'true'
    
    client = Minio(
        endpoint,
        access_key=access_key,
        secret_key=secret_key,
        secure=secure
    )
    
    # 上传特征列表
    feature_cols_path = os.path.join(model_dir, 'feature_columns.json')
    if os.path.exists(feature_cols_path):
        object_name = f"models/{version}/feature_columns.json"
        client.fput_object(bucket_name, object_name, feature_cols_path)
        logger.info(f"Uploaded feature_columns.json")
    
    # 上传特征重要性
    importance_path = os.path.join(model_dir, 'feature_importance.json')
    if os.path.exists(importance_path):
        object_name = f"models/{version}/feature_importance.json"
        client.fput_object(bucket_name, object_name, importance_path)
        logger.info(f"Uploaded feature_importance.json")
    
    # 上传训练指标
    train_metrics_path = os.path.join(model_dir, 'train_metrics.json')
    if os.path.exists(train_metrics_path):
        object_name = f"models/{version}/train_metrics.json"
        client.fput_object(bucket_name, object_name, train_metrics_path)
        logger.info(f"Uploaded train_metrics.json")


def register_model_to_registry(model_metadata):
    """调用 Rule Manager API 注册模型"""
    
    registry_url = os.getenv('MODEL_REGISTRY_URL', 'http://rule-manager:8080')
    model_id = quote(model_metadata['model_id'], safe='')
    endpoint = f"{registry_url}/api/v1/models/{model_id}/versions"
    headers = auth_headers()
    
    logger.info(f"Registering model to {endpoint}")
    logger.debug(f"Metadata: {json.dumps(model_metadata, indent=2)}")
    
    try:
        response = requests.post(
            endpoint,
            json=model_metadata,
            headers=headers,
            timeout=30
        )
        
        response.raise_for_status()
        
        result = response.json()
        logger.info(f"Model registered successfully: {result}")
        
        return result
        
    except requests.exceptions.RequestException as e:
        logger.error(f"Failed to register model: {e}")
        
        if hasattr(e, 'response') and e.response is not None:
            logger.error(f"Response status: {e.response.status_code}")
            logger.error(f"Response body: {e.response.text}")
        
        raise


def activate_model_version(model_id, version):
    """调用 Rule Manager API 激活模型版本"""

    registry_url = os.getenv('MODEL_REGISTRY_URL', 'http://rule-manager:8080')
    endpoint = (
        f"{registry_url}/api/v1/models/"
        f"{quote(model_id, safe='')}/versions/{quote(version, safe='')}/activate"
    )

    logger.info(f"Activating model version via {endpoint}")
    response = requests.post(endpoint, headers=auth_headers(), timeout=30)
    try:
        response.raise_for_status()
    except requests.exceptions.RequestException as e:
        logger.error(f"Failed to activate model version: {e}")
        logger.error(f"Response status: {response.status_code}")
        logger.error(f"Response body: {response.text}")
        raise
    logger.info("Model version activated successfully")
    return response.json()


def notify_flink_reload(model_id, version):
    """通知 Flink 作业重新加载模型（通过 Nacos 或 Kafka）"""
    
    notification_method = os.getenv('NOTIFICATION_METHOD', 'kafka')
    
    if notification_method == 'kafka':
        notify_via_kafka(model_id, version)
    elif notification_method == 'nacos':
        notify_via_nacos(model_id, version)
    else:
        logger.warning(f"Unknown notification method: {notification_method}")


def notify_via_kafka(model_id, version):
    """通过 Kafka 通知模型更新"""
    
    from kafka import KafkaProducer
    
    bootstrap_servers = os.getenv('KAFKA_BOOTSTRAP_SERVERS', 'kafka:9092')
    topic = os.getenv('MODEL_UPDATE_TOPIC', 'model-updates')
    
    logger.info(f"Sending model update notification to Kafka: {topic}")
    
    producer = KafkaProducer(
        bootstrap_servers=bootstrap_servers.split(','),
        value_serializer=lambda v: json.dumps(v).encode('utf-8')
    )
    
    message = {
        'model_id': model_id,
        'version': version,
        'timestamp': datetime.now().isoformat(),
        'action': 'reload',
    }
    
    producer.send(topic, value=message)
    producer.flush()
    
    logger.info(f"Notification sent to Kafka topic: {topic}")


def notify_via_nacos(model_id, version):
    """通过 Nacos 通知模型更新"""
    
    nacos_server = os.getenv('NACOS_SERVER', 'nacos:8848')
    namespace = os.getenv('NACOS_NAMESPACE', 'traffic-analysis')
    group = os.getenv('NACOS_GROUP', 'DEFAULT_GROUP')
    data_id = f"model-{model_id}"
    
    logger.info(f"Updating Nacos config: {data_id}")
    
    # 构建配置内容
    config_content = json.dumps({
        'model_id': model_id,
        'version': version,
        'updated_at': datetime.now().isoformat(),
    })
    
    # 调用 Nacos API
    url = f"http://{nacos_server}/nacos/v1/cs/configs"
    
    params = {
        'dataId': data_id,
        'group': group,
        'tenant': namespace,
        'content': config_content,
    }
    
    response = requests.post(url, data=params, timeout=10)
    
    if response.status_code == 200 and response.text == 'true':
        logger.info(f"Nacos config updated successfully")
    else:
        logger.error(f"Failed to update Nacos config: {response.text}")


def main():
    """主函数"""
    
    parser = argparse.ArgumentParser(description='Register trained model to Model Registry')
    parser.add_argument('--metrics', required=True, help='Evaluation metrics JSON string')
    parser.add_argument('--model-id', default='behavior-classifier', help='Model ID')
    parser.add_argument('--model-type', default='xgboost', help='Model type (xgboost/lightgbm)')
    args = parser.parse_args()
    
    # 解析指标
    try:
        metrics = json.loads(args.metrics)
    except json.JSONDecodeError as e:
        logger.error(f"Failed to parse metrics JSON: {e}")
        sys.exit(1)
    
    # 读取环境变量
    model_dir = os.getenv('MODEL_DIR', '/model')
    feature_set_id = os.getenv('FEATURE_SET_ID', 'v1')
    tenant_id = os.getenv('TENANT_ID', 'campus-net')
    
    # 生成版本号
    version = os.getenv('MODEL_VERSION')
    if not version:
        version = datetime.now().strftime('v%Y%m%d_%H%M%S')
    
    logger.info("=" * 80)
    logger.info("Starting model registration pipeline")
    logger.info("=" * 80)
    logger.info(f"Configuration:")
    logger.info(f"  - Model ID: {args.model_id}")
    logger.info(f"  - Model Type: {args.model_type}")
    logger.info(f"  - Version: {version}")
    logger.info(f"  - Feature Set: {feature_set_id}")
    logger.info(f"  - Tenant: {tenant_id}")
    
    try:
        # 1. 上传模型到 MinIO
        if args.model_type == 'xgboost':
            model_path = os.path.join(model_dir, 'model.json')
        elif args.model_type == 'lightgbm':
            model_path = os.path.join(model_dir, 'model.txt')
        else:
            raise ValueError(f"Unsupported model type: {args.model_type}")
        
        if not os.path.exists(model_path):
            raise FileNotFoundError(f"Model file not found: {model_path}")
        
        s3_uri = upload_to_minio(model_path, args.model_type, version)
        
        # 2. 上传其他文件
        upload_artifacts(model_dir, version)
        
        # 3. 构建模型元数据
        model_metadata = {
            'model_id': args.model_id,
            'version': version,
            'model_type': args.model_type,
            'artifact_uri': s3_uri,
            'feature_set_id': feature_set_id,
            'tenant_id': tenant_id,
            'metrics': {
                'f1_score': metrics.get('f1_score'),
                'precision': metrics.get('precision'),
                'recall': metrics.get('recall'),
                'auc': metrics.get('auc_roc', metrics.get('auc')),
                'auc_roc': metrics.get('auc_roc'),
                'auc_pr': metrics.get('auc_pr'),
                'accuracy': metrics.get('accuracy'),
            },
            'status': 'registered',
            'created_at': datetime.now().isoformat(),
            'description': f"Auto-trained model from MLOps pipeline",
        }
        
        # 4. 注册到 Model Registry
        result = register_model_to_registry(model_metadata)

        # 4.1 可选激活
        if os.getenv('AUTO_ACTIVATE', 'false').lower() == 'true':
            activate_model_version(args.model_id, version)
        
        # 5. 通知 Flink 重新加载
        notify_flink_reload(args.model_id, version)
        
        logger.info("=" * 80)
        logger.info("Model registration completed successfully!")
        logger.info(f"Model ID: {args.model_id}")
        logger.info(f"Version: {version}")
        logger.info(f"S3 URI: {s3_uri}")
        logger.info(f"F1 Score: {metrics.get('f1_score', 'N/A')}")
        logger.info("=" * 80)
        
    except Exception as e:
        logger.error(f"Model registration failed: {e}", exc_info=True)
        sys.exit(1)


if __name__ == '__main__':
    main()
