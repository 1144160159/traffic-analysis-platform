#!/usr/bin/env python3
"""Upload model artifacts and register immutable metadata without activation."""

import os
import sys
import json
import argparse
import hashlib
import requests
from dataclasses import dataclass
from datetime import datetime
import logging
import ssl
import urllib3
import uuid
import time
from urllib.parse import quote

try:
    from minio import Minio
except ImportError:  # Configuration validation remains testable in minimal CI images.
    Minio = None

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class MinioRuntimeConfig:
    endpoint: str
    access_key: str
    secret_key: str
    bucket: str
    secure: bool
    ca_file: str | None


def _required_env(name):
    value = os.getenv(name, '').strip()
    if not value:
        raise RuntimeError(f'{name} is required; model artifact storage fails closed')
    return value


def load_minio_config():
    """Load explicit MinIO settings without development credential fallbacks."""
    endpoint = _required_env('MINIO_ENDPOINT')
    access_key = _required_env('MINIO_ACCESS_KEY')
    secret_key = _required_env('MINIO_SECRET_KEY')
    bucket = _required_env('MINIO_BUCKET')
    secure_value = _required_env('MINIO_SECURE').lower()
    if secure_value not in {'true', 'false'}:
        raise RuntimeError('MINIO_SECURE must be explicitly true or false')
    if '://' in endpoint:
        raise RuntimeError('MINIO_ENDPOINT must be host:port; select TLS with MINIO_SECURE')
    if access_key == 'minioadmin' or secret_key == 'minioadmin':
        raise RuntimeError('default MinIO administrator credentials are prohibited')
    secure = secure_value == 'true'
    ca_file = os.getenv('MINIO_CA_FILE', '').strip()
    if secure and not ca_file:
        raise RuntimeError('MINIO_CA_FILE is required when MINIO_SECURE=true')
    if not secure and ca_file:
        raise RuntimeError('MINIO_CA_FILE cannot be set when MINIO_SECURE=false')
    if ca_file and not os.path.isfile(ca_file):
        raise RuntimeError('MINIO_CA_FILE must reference a readable CA certificate file')
    return MinioRuntimeConfig(
        endpoint=endpoint,
        access_key=access_key,
        secret_key=secret_key,
        bucket=bucket,
        secure=secure,
        ca_file=ca_file or None,
    )


def build_minio_client(config=None):
    config = config or load_minio_config()
    if Minio is None:
        raise RuntimeError('the minio package is required for model artifact upload')
    kwargs = {}
    if config.secure:
        try:
            ssl.create_default_context(cafile=config.ca_file)
        except (OSError, ssl.SSLError) as exc:
            raise RuntimeError(f'MINIO_CA_FILE is not a valid CA bundle: {exc}') from exc
        kwargs['http_client'] = urllib3.PoolManager(
            cert_reqs='CERT_REQUIRED',
            ca_certs=config.ca_file,
        )
    return Minio(
        config.endpoint,
        access_key=config.access_key,
        secret_key=config.secret_key,
        secure=config.secure,
        **kwargs,
    )


def require_model_bucket(client, bucket):
    """Require bootstrap-owned bucket creation; application credentials never create it."""
    if not client.bucket_exists(bucket):
        raise RuntimeError(
            f'MinIO bucket {bucket!r} is absent; create it through the governed bootstrap path'
        )


def auth_headers(required: bool = False) -> dict:
    """Build the registry Authorization header.

    Governed registration and shadow-activation calls fail closed when the
    API token is missing; a request without credentials must never silently
    reach the registry (code-review H40 收敛项).
    """
    headers = {
        'Content-Type': 'application/json',
    }
    api_token = os.getenv('API_TOKEN', '')
    if api_token:
        headers['Authorization'] = f'Bearer {api_token}'
    elif required:
        raise RuntimeError('API_TOKEN is required for governed model registry calls; failing closed')
    return headers


def artifact_prefix(tenant_id, model_id, version, artifact_sha256):
    """Build a tenant/model scoped, content-addressed object prefix."""
    segments = [tenant_id, model_id, version, artifact_sha256]
    encoded = [quote(str(segment), safe='') for segment in segments]
    return f"tenants/{encoded[0]}/models/{encoded[1]}/versions/{encoded[2]}/{encoded[3]}"


def upload_to_minio(model_path, model_type, tenant_id, model_id, version, artifact_sha256):
    """上传模型到 MinIO"""
    config = load_minio_config()
    logger.info(
        "Connecting to MinIO endpoint=%s bucket=%s secure=%s",
        config.endpoint,
        config.bucket,
        config.secure,
    )
    client = build_minio_client(config)
    require_model_bucket(client, config.bucket)
    
    # 构建对象键
    if model_type == 'xgboost':
        file_ext = 'json'
    elif model_type == 'lightgbm':
        file_ext = 'txt'
    else:
        file_ext = 'model'
    
    object_name = f"{artifact_prefix(tenant_id, model_id, version, artifact_sha256)}/model.{file_ext}"
    
    # 上传模型
    logger.info(f"Uploading model to s3://{config.bucket}/{object_name}")
    
    client.fput_object(
        config.bucket,
        object_name,
        model_path,
        content_type='application/octet-stream'
    )
    
    logger.info(f"Model uploaded successfully")
    
    # 构建 S3 URI
    s3_uri = f"s3://{config.bucket}/{object_name}"
    
    return s3_uri


def upload_artifacts(model_dir, tenant_id, model_id, version, artifact_sha256):
    """上传模型相关的所有文件（side artifacts）并返回内容寻址绑定。

    每个侧产物均计算 sha256、写入 x-amz-meta-sha256 元数据并在返回字典中
    记录，防止对象被篡改而不被发现（代码审查 H40 收敛项）。同时补齐此前
    漏传的 train_config.json 与 training-run-manifest.json。
    """
    config = load_minio_config()
    client = build_minio_client(config)
    require_model_bucket(client, config.bucket)

    uploaded = {}
    candidates = [
        'feature_columns.json',
        'feature_importance.json',
        'train_metrics.json',
        'train_config.json',
        'training-run-manifest.json',
    ]
    for filename in candidates:
        path = os.path.join(model_dir, filename)
        if not os.path.exists(path):
            logger.info(f"Skipping absent side artifact: {filename}")
            continue
        file_sha256 = hashlib.sha256()
        with open(path, 'rb') as handle:
            for chunk in iter(lambda: handle.read(1024 * 1024), b''):
                file_sha256.update(chunk)
        file_sha256 = file_sha256.hexdigest()
        object_name = f"{artifact_prefix(tenant_id, model_id, version, artifact_sha256)}/{filename}"
        client.fput_object(
            config.bucket,
            object_name,
            path,
            metadata={"X-Amz-Meta-Sha256": file_sha256},
            content_type='application/json',
        )
        uploaded[filename] = {
            'uri': f"s3://{config.bucket}/{object_name}",
            'sha256': file_sha256,
        }
        logger.info(f"Uploaded {filename} (sha256={file_sha256[:12]}...)")
    return uploaded


def register_model_to_registry(model_metadata, idempotency_key=None):
    """Register metadata only; activation and runtime notification are forbidden here."""
    
    registry_url = os.getenv('MODEL_REGISTRY_URL', 'http://rule-manager:8080')
    model_id = quote(model_metadata['model_id'], safe='')
    endpoint = f"{registry_url}/api/v1/models/{model_id}/versions"
    headers = auth_headers(required=True)
    headers['Idempotency-Key'] = idempotency_key or _required_env(
        'MODEL_REGISTRATION_IDEMPOTENCY_KEY'
    )
    
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


def prepare_model_shadow_activation(registry_model_id, version, expected_revision,
                                    requested_by, approval_reason, idempotency_key):
    """Ask the governed Go authority to create a shadow-load outbox fact.

    The authenticated token belongs to the independent approver.  This call
    does not activate serving and is unreachable unless the explicit WRT flag
    is enabled in both this pipeline and the rule-manager.
    """
    registry_url = os.getenv('MODEL_REGISTRY_URL', 'http://rule-manager:8080')
    model_id = quote(registry_model_id, safe='')
    model_version = quote(version, safe='')
    endpoint = (
        f"{registry_url}/api/v1/models/{model_id}/versions/"
        f"{model_version}/shadow-activation"
    )
    headers = auth_headers(required=True)
    headers['Idempotency-Key'] = idempotency_key
    response = requests.post(
        endpoint,
        json={
            'expected_revision': expected_revision,
            'requested_by': requested_by,
            'approval_reason': approval_reason,
        },
        headers=headers,
        timeout=30,
    )
    try:
        response.raise_for_status()
    except requests.exceptions.RequestException:
        logger.error(
            "Shadow activation preparation failed: status=%s body=%s",
            response.status_code, response.text,
        )
        raise
    result = response.json()
    data = result.get('data', {})
    if data.get('state') not in {'outbox_pending', 'outbox_processing', 'published', 'shadow_ready'}:
        raise ValueError('model registry returned an invalid shadow activation state')
    if data.get('serving_activated') is not False:
        raise ValueError('shadow activation receipt must not claim serving activation')
    if int(data.get('aggregate_revision', 0)) != expected_revision + 1:
        raise ValueError('shadow activation aggregate revision is inconsistent')
    return result


def wait_for_model_shadow_ready(registry_model_id, version, request_id,
                                timeout_seconds, poll_interval_seconds=2.0):
    """Poll the durable receipt until exact subtask quorum is ready or fails."""
    if timeout_seconds <= 0 or poll_interval_seconds <= 0:
        raise ValueError('shadow readiness timeout and poll interval must be positive')
    registry_url = os.getenv('MODEL_REGISTRY_URL', 'http://rule-manager:8080')
    endpoint = (
        f"{registry_url}/api/v1/models/{quote(registry_model_id, safe='')}/versions/"
        f"{quote(version, safe='')}/shadow-activation/{quote(request_id, safe='')}"
    )
    deadline = time.monotonic() + timeout_seconds
    while True:
        response = requests.get(endpoint, headers=auth_headers(required=True), timeout=30)
        response.raise_for_status()
        result = response.json()
        data = result.get('data', {})
        if data.get('serving_activated') is not False:
            raise ValueError('shadow status must never claim serving activation')
        state = data.get('state')
        if state == 'shadow_ready':
            if not data.get('shadow_ready_expires_at'):
                raise ValueError('shadow-ready receipt is missing its expiry')
            return result
        if state == 'failed':
            raise RuntimeError(f'model shadow loading failed for request {request_id}')
        if state not in {'outbox_pending', 'outbox_processing', 'published'}:
            raise ValueError(f'unknown model shadow activation state: {state}')
        if time.monotonic() >= deadline:
            raise TimeoutError(f'model shadow loading timed out for request {request_id}')
        time.sleep(min(poll_interval_seconds, max(0.0, deadline - time.monotonic())))


def _load_json_object(path, description):
    with open(path, 'r', encoding='utf-8') as handle:
        value = json.load(handle)
    if not isinstance(value, dict):
        raise ValueError(f'{description} must be a JSON object')
    return value


def validate_registration_evaluation_gate(evaluation_manifest_path):
    """Fail-closed second gate: the metrics bound to this registration must
    satisfy the task-book 95%/5% intervals (预警准确率/误报率/unknown recall).

    This is a second line of defense behind the workflow quality-gate step:
    even if a workflow skips or misconfigures the gate, registration refuses
    to proceed with metrics that miss the frozen bounds.  The manifest is
    identity-verified first so a stale/tampered manifest cannot pass.
    """
    from governed_evaluation import validate_evaluation_manifest_identity
    from evaluate_quality_gate import evaluate_manifest_gate

    manifest = _load_json_object(evaluation_manifest_path, 'evaluation manifest')
    validate_evaluation_manifest_identity(manifest)
    min_kar = float(os.getenv('MODEL_REGISTRATION_MIN_KNOWN_ATTACK_RECALL', '0.95'))
    max_fpr = float(os.getenv('MODEL_REGISTRATION_MAX_FALSE_POSITIVE_RATE', '0.05'))
    min_uk = float(os.getenv('MODEL_REGISTRATION_MIN_UNKNOWN_RECALL', '0.80'))
    passed, checks = evaluate_manifest_gate(
        manifest,
        min_known_attack_recall=min_kar,
        max_false_positive_rate=max_fpr,
        min_unknown_recall=min_uk,
    )
    if not passed:
        failed = [check['name'] for check in checks if not check['passed']]
        raise RuntimeError(
            f'model registration refused: evaluation quality gate failed for '
            f'{failed}; metrics do not meet the frozen task-book bounds'
        )
    return manifest


def build_governed_registration_payload(package_dir, metrics, public_key_path):
    """Verify a stored signed package and map it to the metadata-only API contract."""
    from model_artifact_governance import verify_export_package

    manifest = verify_export_package(package_dir, public_key_path)
    receipt_path = os.path.join(package_dir, 'model-package-storage-receipt.json')
    receipt = _load_json_object(receipt_path, 'model package storage receipt')
    if receipt.get('schema_version') != 1 or receipt.get('state') != 'stored':
        raise ValueError('model package storage receipt is not in stored state')
    if receipt.get('package_id') != manifest['package_id'] or \
            receipt.get('package_sha256') != manifest['package_sha256']:
        raise ValueError('model package storage receipt identity mismatch')
    if receipt.get('activation_authorized') is not False:
        raise ValueError('model package storage receipt must not authorize activation')
    objects = receipt.get('objects')
    if not isinstance(objects, dict):
        raise ValueError('model package storage receipt objects are required')
    required_objects = set(manifest['artifacts']) | {'model-artifact-manifest.json'}
    if set(objects) != required_objects:
        raise ValueError('model package storage receipt object set mismatch')
    for name, artifact in manifest['artifacts'].items():
        if objects[name].get('sha256') != artifact['sha256']:
            raise ValueError(f'model package storage receipt hash mismatch: {name}')
    manifest_object = objects['model-artifact-manifest.json']
    with open(os.path.join(package_dir, 'model-artifact-manifest.json'), 'rb') as handle:
        manifest_sha256 = hashlib.sha256(handle.read()).hexdigest()
    if manifest_object.get('sha256') != manifest_sha256:
        raise ValueError('stored model manifest hash does not match local signed manifest')
    baseline = objects.get('baseline-model.onnx')
    if not baseline or not str(baseline.get('uri', '')).startswith('s3://'):
        raise ValueError('stored baseline ONNX URI is required')
    graph = manifest['graph_snapshot']
    return {
        'model_id': manifest['model_id'],
        'version': manifest['model_version'],
        'model_type': 'onnx',
        'artifact_uri': baseline['uri'],
        'artifact_manifest_uri': manifest_object['uri'],
        'package_id': manifest['package_id'],
        'package_sha256': manifest['package_sha256'],
        'artifact_manifest_sha256': manifest_sha256,
        'evaluation_sha256': manifest['evaluation_sha256'],
        'explanation_sha256': manifest['explanation_sha256'],
        'graph_snapshot_id': graph['snapshot_id'],
        'graph_snapshot_sha256': graph['manifest_sha256'],
        'signing_key_id': manifest['signature']['key_id'],
        'compatibility': manifest['compatibility'],
        'governance_version': 'model-registration.v1',
        'expected_revision': 0,
        'feature_set_id': manifest['compatibility']['feature_set_id'],
        'tenant_id': manifest['tenant_id'],
        'metrics': metrics,
        'status': 'registered',
        'description': 'Governed signed baseline/GNN model package',
    }


def _enabled(name):
    value = os.getenv(name, 'false').strip().lower()
    if value not in {'true', 'false'}:
        raise RuntimeError(f'{name} must be explicitly true or false')
    return value == 'true'


def write_registration_receipt(path, model_metadata, result):
    data = result.get('data', {}) if isinstance(result, dict) else {}
    if data.get('status') != 'registered' or int(data.get('revision', 0)) != 1:
        raise ValueError('model registry did not return an immutable registered revision')
    request_sha = data.get('registration_request_sha256', '')
    if len(request_sha) != 64:
        raise ValueError('model registry response is missing registration_request_sha256')
    receipt = {
        'schema_version': 1,
        'receipt_id': str(uuid.uuid5(
            uuid.UUID('6b97701a-8b8d-5c52-b4e1-a65e8225e149'), request_sha,
        )),
        'state': 'metadata_registered',
        'tenant_id': model_metadata['tenant_id'],
        'package_model_id': model_metadata['model_id'],
        'registry_model_id': data.get('model_id', ''),
        'model_version': data.get('model_version', model_metadata['version']),
        'package_id': model_metadata['package_id'],
        'package_sha256': model_metadata['package_sha256'],
        'registration_request_sha256': request_sha,
        'revision': 1,
        'status': 'registered',
        'activation_event_created': False,
        'activation_authorized': False,
    }
    target = os.path.abspath(path)
    os.makedirs(os.path.dirname(target), exist_ok=True)
    descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(descriptor, 'w', encoding='utf-8') as handle:
            json.dump(receipt, handle, sort_keys=True, separators=(',', ':'))
            handle.write('\n')
            handle.flush()
            os.fsync(handle.fileno())
    except Exception:
        try:
            os.unlink(target)
        except FileNotFoundError:
            pass
        raise
    return receipt


def write_shadow_activation_receipt(path, result):
    data = result.get('data', {}) if isinstance(result, dict) else {}
    required = {
        'request_id', 'event_id', 'tenant_id', 'model_id', 'model_version',
        'package_id', 'package_sha256', 'aggregate_revision', 'request_sha256',
        'state', 'serving_activated',
    }
    missing = sorted(
        name for name in required if name not in data or data[name] in (None, '')
    )
    if missing:
        raise ValueError(f'shadow activation receipt is missing: {missing}')
    if data.get('serving_activated') is not False:
        raise ValueError('shadow activation receipt cannot mark serving active')
    target = os.path.abspath(path)
    os.makedirs(os.path.dirname(target), exist_ok=True)
    descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(descriptor, 'w', encoding='utf-8') as handle:
            json.dump(data, handle, sort_keys=True, separators=(',', ':'))
            handle.write('\n')
            handle.flush()
            os.fsync(handle.fileno())
    except Exception:
        try:
            os.unlink(target)
        except FileNotFoundError:
            pass
        raise
    return data


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
    
    # 生成版本号：必须由 workflow/权威方显式注入，禁止 wall-clock 兜底
    # （可复现性要求，代码审查 H40 收敛项）。
    version = os.getenv('MODEL_VERSION')
    if not version:
        raise RuntimeError(
            'MODEL_VERSION is required and must be injected by the workflow '
            '(e.g. workflow uid); refusing to fall back to wall-clock versioning'
        )
    
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
        if _enabled('AUTO_ACTIVATE'):
            raise RuntimeError(
                'register_model.py cannot activate or notify runtimes; use the governed activation authority'
            )
        idempotency_key = _required_env('MODEL_REGISTRATION_IDEMPOTENCY_KEY')
        if not 16 <= len(idempotency_key) <= 200:
            raise RuntimeError('MODEL_REGISTRATION_IDEMPOTENCY_KEY must contain 16 to 200 characters')
        if _enabled('MLOPS_GOVERNED_MODEL_REGISTRATION_V1_ENABLED'):
            package_dir = _required_env('MODEL_PACKAGE_DIR')
            public_key_path = _required_env('MODEL_SIGNING_PUBLIC_KEY_FILE')
            # Fail-closed second gate on the frozen 95%/5% intervals (see
            # validate_registration_evaluation_gate).  Registration refuses to
            # proceed when the bound metrics miss the task-book thresholds.
            evaluation_manifest_path = os.getenv('MODEL_REGISTRATION_EVALUATION_MANIFEST', '')
            if evaluation_manifest_path:
                validate_registration_evaluation_gate(evaluation_manifest_path)
            else:
                logger.warning(
                    'MODEL_REGISTRATION_EVALUATION_MANIFEST is not set; the '
                    'governed workflow must provide it for gate enforcement'
                )
            model_metadata = build_governed_registration_payload(
                package_dir, metrics, public_key_path,
            )
            result = register_model_to_registry(model_metadata, idempotency_key)
            receipt_path = _required_env('MODEL_REGISTRATION_RECEIPT_PATH')
            receipt = write_registration_receipt(receipt_path, model_metadata, result)
            logger.info(
                "Governed model metadata registered without activation: receipt_id=%s package_id=%s version=%s status=%s",
                receipt['receipt_id'],
                model_metadata['package_id'], model_metadata['version'],
                result.get('data', {}).get('status', 'registered'),
            )

            if _enabled('MODEL_SHADOW_ACTIVATION_REQUEST_V1_ENABLED'):
                registry_model_id = result.get('data', {}).get('model_id', '')
                if not registry_model_id:
                    raise ValueError('metadata registration response is missing registry model_id')
                shadow_idempotency_key = _required_env(
                    'MODEL_SHADOW_ACTIVATION_IDEMPOTENCY_KEY'
                )
                if not 16 <= len(shadow_idempotency_key) <= 200:
                    raise RuntimeError(
                        'MODEL_SHADOW_ACTIVATION_IDEMPOTENCY_KEY must contain 16 to 200 characters'
                    )
                expected_shadow_revision = int(_required_env(
                    'MODEL_SHADOW_ACTIVATION_EXPECTED_REVISION'
                ))
                if expected_shadow_revision < 0:
                    raise RuntimeError(
                        'MODEL_SHADOW_ACTIVATION_EXPECTED_REVISION must be non-negative'
                    )
                shadow_result = prepare_model_shadow_activation(
                    registry_model_id=registry_model_id,
                    version=model_metadata['version'],
                    expected_revision=expected_shadow_revision,
                    requested_by=_required_env('MODEL_SHADOW_ACTIVATION_REQUESTED_BY'),
                    approval_reason=_required_env('MODEL_SHADOW_ACTIVATION_APPROVAL_REASON'),
                    idempotency_key=shadow_idempotency_key,
                )
                shadow_receipt = write_shadow_activation_receipt(
                    _required_env('MODEL_SHADOW_ACTIVATION_RECEIPT_PATH'),
                    shadow_result,
                )
                logger.info(
                    "Schema-v2 shadow-load accepted without serving activation: request_id=%s event_id=%s revision=%s state=%s",
                    shadow_receipt['request_id'], shadow_receipt['event_id'],
                    shadow_receipt['aggregate_revision'], shadow_receipt['state'],
                )
                if _enabled('MODEL_SHADOW_ACTIVATION_WAIT_FOR_READY'):
                    ready_result = wait_for_model_shadow_ready(
                        registry_model_id=registry_model_id,
                        version=model_metadata['version'],
                        request_id=shadow_receipt['request_id'],
                        timeout_seconds=float(_required_env(
                            'MODEL_SHADOW_ACTIVATION_READY_TIMEOUT_SECONDS'
                        )),
                        poll_interval_seconds=float(os.getenv(
                            'MODEL_SHADOW_ACTIVATION_READY_POLL_SECONDS', '2'
                        )),
                    )
                    ready_receipt = write_shadow_activation_receipt(
                        _required_env('MODEL_SHADOW_READY_RECEIPT_PATH'), ready_result,
                    )
                    logger.info(
                        "Exact shadow consumer quorum ready: request_id=%s event_id=%s expires_at=%s serving_activated=false",
                        ready_receipt['request_id'], ready_receipt['event_id'],
                        ready_receipt['shadow_ready_expires_at'],
                    )
            return

        if _enabled('MODEL_SHADOW_ACTIVATION_REQUEST_V1_ENABLED'):
            raise RuntimeError(
                'shadow activation requests require MLOPS_GOVERNED_MODEL_REGISTRATION_V1_ENABLED=true'
            )

        # 1. 上传模型到 MinIO
        if args.model_type == 'xgboost':
            model_path = os.path.join(model_dir, 'model.json')
        elif args.model_type == 'lightgbm':
            model_path = os.path.join(model_dir, 'model.txt')
        else:
            raise ValueError(f"Unsupported model type: {args.model_type}")
        
        if not os.path.exists(model_path):
            raise FileNotFoundError(f"Model file not found: {model_path}")

        artifact_sha256 = hashlib.sha256()
        with open(model_path, 'rb') as artifact_file:
            for chunk in iter(lambda: artifact_file.read(1024 * 1024), b''):
                artifact_sha256.update(chunk)
        artifact_sha256 = artifact_sha256.hexdigest()
        
        s3_uri = upload_to_minio(model_path, args.model_type, tenant_id, args.model_id, version, artifact_sha256)
        
        # 2. 上传其他文件（返回含 sha256 的内容寻址绑定）
        uploaded_artifacts = upload_artifacts(model_dir, tenant_id, args.model_id, version, artifact_sha256)
        
        # 3. 构建模型元数据
        model_metadata = {
            'model_id': args.model_id,
            'version': version,
            'model_type': args.model_type,
            'artifact_uri': s3_uri,
            'feature_set_id': feature_set_id,
            'tenant_id': tenant_id,
            'side_artifacts': uploaded_artifacts,
            'metrics': {
                'f1_score': metrics.get('f1_score'),
                'precision': metrics.get('precision'),
                'recall': metrics.get('recall'),
                'auc': metrics.get('auc_roc', metrics.get('auc')),
                'auc_roc': metrics.get('auc_roc'),
                'auc_pr': metrics.get('auc_pr'),
                'accuracy': metrics.get('accuracy'),
                'threshold': metrics.get('threshold', 0.5),
                'artifact_sha256': artifact_sha256,
            },
            'status': 'registered',
            'expected_revision': 0,
            'created_at': datetime.now().isoformat(),
            'description': f"Auto-trained model from MLOps pipeline",
        }
        
        # 4. 注册到 Model Registry
        result = register_model_to_registry(model_metadata, idempotency_key)
        
        logger.info("=" * 80)
        logger.info("Model metadata registration completed successfully; activation not requested")
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
