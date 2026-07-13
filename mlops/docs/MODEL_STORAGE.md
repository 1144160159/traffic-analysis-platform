# MLOps 模型存储与调用架构（K8s 环境）

## 架构总览

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                          K8s 环境 — 模型存储与调用                                  │
├──────────────────────────────────────────────────────────────────────────────────┤
│                                                                                    │
│  模型训练 (Argo Workflow)               模型存储 (MinIO)                            │
│  ┌──────────────────────┐              ┌─────────────────────────┐                │
│  │ train_model.py       │── upload ──→│ traffic-models bucket    │                │
│  │  XGBoost/LightGBM    │              │                         │                │
│  │  → model.json        │              │ models/                 │                │
│  │  → feature_importance│              │   v20240614_120000/     │                │
│  │  → metrics.json      │              │     model.json          │                │
│  └──────┬───────────────┘              │     feature_importance  │                │
│         │                              │     metrics.json        │                │
│         ▼                              │   v20240615_020000/     │                │
│  ┌──────────────────────┐              │     model.json          │                │
│  │ register_model.py    │── API ──→ ┌──┤     ...                │                │
│  │  → Go Model Registry │           │  └─────────────────────────┘                │
│  │  → Kafka notify      │           │                                              │
│  └──────────────────────┘           │  模型调用 (Flink)                             │
│                                      │  ┌─────────────────────────────────────┐   │
│  Go Model Registry API              │  │ MinioModelLoader                     │   │
│  ┌──────────────────────┐           │  │  1. 接收 Kafka model-update 事件     │   │
│  │ POST /api/v1/models/  │           ├──│  2. 解析 artifact_uri                │   │
│  │   {id}/versions       │           │  │     s3://traffic-models/models/     │   │
│  │   {v}/activate        │── Kafka ─→│  │     v20240614.../model.json         │   │
│  └──────────────────────┘           │  │  3. MinIO SDK 下载 → /tmp/models/   │   │
│                                      │  │  4. XGBoost4j 加载 model.json       │   │
│  Kafka: model-updates               │  │  5. 更新 ModelRegistry (热替换)      │   │
│  ┌──────────────────────┐           │  └─────────────────────────────────────┘   │
│  │ { model_id, version, │           │                                              │
│  │   artifact_uri,       │           │  本地缓存: /opt/flink/models/               │
│  │   action: activated } │           │  ┌─────────────────────────────────────┐   │
│  └──────────────────────┘           │  │ SHA256(artifact_uri) → model.json    │   │
│                                      │  │ 避免重复下载 + 版本隔离              │   │
│                                      │  └─────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────────────────────┘
```

## 一、模型存储（MinIO）

### Bucket 结构

```
s3://traffic-models/
├── models/
│   ├── v20240614_120000/           ← 版本号 = Argo workflow name
│   │   ├── model.json              ← XGBoost 原生格式 (Flink 可直接加载)
│   │   ├── model.txt               ← LightGBM 格式 (可选)
│   │   ├── feature_columns.json    ← 特征列列表
│   │   ├── feature_importance.json ← 特征重要性
│   │   └── metrics.json            ← 评估指标
│   ├── v20240615_020000/
│   │   └── ...
│   └── latest → v20240615_020000/  ← 最新版本符号链接/标记
│
├── checkpoints/
│   └── behavior-job/               ← Flink checkpoint (已有的 s3://flink-checkpoints)
│
└── artifacts/
    └── argo/                       ← Argo Workflow 中间产物 (parquet, metadata)
```

### 模型版本命名

```
v{YYYYMMDD}_{HHMMSS}
例如: v20240614_120000  (2026年6月14日 12:00:00 UTC)

来源: Go Model Registry 中的 model_version 字段
      Python register_model.py: version = datetime.now().strftime('v%Y%m%d_%H%M%S')
```

### MinIO 连接信息（K8s 集群内）

| 参数 | 值 |
|------|-----|
| 内部 Service | `minio.minio.svc:9000` |
| 外部 NodePort | `10.0.5.8:30000` |
| Bucket | `traffic-models` |
| Access Key | `minioadmin` (开发) / K8s Secret `minio-secret` |
| Secret Key | K8s Secret `minio-secret` |

## 二、模型格式与兼容性

### 训练侧（Python）

| 模型类型 | 输出格式 | 文件 |
|---------|---------|------|
| XGBoost | Native JSON | `model.json` (XGBClassifier.save_model) |
| LightGBM | Native TXT | `model.txt` (LGBMClassifier.booster_.save_model) |

### 推理侧（Flink Java）

| 格式 | 加载方式 | Maven 依赖 |
|------|---------|-----------|
| XGBoost JSON | `ml.dmlc:xgboost4j_2.12` → `XGBoost.loadModel(path)` | `xgboost4j_2.12:2.0.3` |
| LightGBM TXT | `com.microsoft.ml.lightgbm:lightgbm4j` | `lightgbm4j:4.3.0` |
| PMML | `org.jpmml:pmml-evaluator` | 需额外导出步骤 |
| ONNX | `com.microsoft.onnxruntime:onnxruntime` | 需额外导出步骤 |

**推荐路径**: XGBoost JSON ← 原生兼容，无需格式转换。

### 特征列对齐

Python 训练时 `feature_columns.json` 定义了特征顺序。Flink 推理时必须按相同顺序提供特征向量：

```json
// feature_columns.json (训练时生成)
["pps", "bps", "pktlen_mean", "pktlen_std", "iat_mean_ms",
 "iat_std_ms", "active_mean_ms", "idle_mean_ms", "duration_ms",
 "up_down_ratio", "tcp_flag_syn_cnt", "tcp_flag_ack_cnt",
 "tcp_init_win_bytes_fwd", "tcp_init_win_bytes_bwd"]
```

```java
// Flink 侧推理 (必须按相同顺序构建特征向量)
float[] features = new float[] {
    feature.getPps(), feature.getBps(), feature.getPktlenMean(),
    // ... 严格对齐 feature_columns.json 顺序
};
Booster booster = modelRegistry.getBooster("behavior-classifier");
float[][] predictions = booster.predict(new DMatrix(features, 1, features.length, Float.NaN));
```

## 三、MinIO 下载流程（Flink 侧）

```
ModelUpdateBroadcastHandler 接收 Kafka 事件
  │  { model_id: "behavior-classifier", version: "v20240614_120000",
  │    artifact_uri: "s3://traffic-models/models/v20240614_120000/model.json",
  │    action: "activated" }
  ▼
MinioModelLoader.download(artifactUri)
  │  1. 解析 s3://bucket/path
  │  2. 检查本地缓存: SHA256(artifactUri) → /opt/flink/models/{hash}
  │  3. 缓存命中 → 直接返回本地路径
  │  4. 缓存未命中 → MinIO Java SDK 下载
  │  5. 写入 /opt/flink/models/{hash}/model.json
  │  6. 同时下载 feature_columns.json
  ▼
XGBoost4jModel.load(localModelPath)
  │  1. XGBoost.loadModel(localModelPath/model.json)
  │  2. 验证特征列完整性
  │  3. 预热（跑一次空推理验证模型可用）
  ▼
ModelRegistry.hotSwap(modelId, newBooster)
  │  models.put("behavior-classifier", newBooster)
  │  旧 booster 引用释放 → GC
  ▼
BehaviorDetectorFunction.asyncInvoke(feature)
  │  Booster booster = registry.getBooster("behavior-classifier")
  │  float[] feats = buildFeatureVector(feature, featureColumns)
  │  float[][] pred = booster.predict(new DMatrix(feats, ...))
  │  输出 DetectionBehavior
```

## 四、Go Model Registry API 调用

### 注册模型（Argo 训练完成时）

```bash
curl -X POST http://rule-manager.traffic-analysis.svc:8080/api/v1/models/behavior-classifier/versions \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: campus-net" \
  -H "X-User-ID: mlops-pipeline" \
  -d '{
    "model_id": "behavior-classifier",
    "model_type": "xgboost",
    "version": "v20240614_120000",
    "artifact_uri": "s3://traffic-models/models/v20240614_120000/model.json",
    "feature_set_id": "v1",
    "tenant_id": "campus-net",
    "metrics": {"f1_score": 0.92, "auc_roc": 0.95},
    "status": "registered"
  }'
```

### 激活模型（部署到生产）

```bash
curl -X POST http://rule-manager.traffic-analysis.svc:8080/api/v1/models/behavior-classifier/versions/v20240614_120000/activate \
  -H "X-Tenant-ID: campus-net" \
  -H "X-User-ID: mlops-orchestrator"
```

→ Go 自动发送 Kafka `model-updates` 消息 → Flink 热加载

## 五、存储容量规划

| 项目 | 估算 |
|------|------|
| 单个 XGBoost 模型 (JSON) | ~50KB–5MB |
| 单次训练产物 (含 metrics/importance/pcap) | ~50MB |
| 每月新增 (每周训练 × 4) | ~200MB |
| 保留策略 | 最近 5 个激活版本 + 3 个候选版本 |
| MinIO bucket 配额 | 建议 10GB |

## 六、本地开发测试

```bash
# 本地启动 MinIO (Docker)
docker run -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  quay.io/minio/minio server /data --console-address ":9001"

# 创建 bucket
mc alias set local http://localhost:9000 minioadmin minioadmin
mc mb local/traffic-models

# 模拟模型上传
mc cp model.json local/traffic-models/models/v20240614_120000/model.json

# Go API: 注册模型
curl -X POST http://localhost:8080/api/v1/models/behavior-classifier/versions \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: campus-net" \
  -H "X-User-ID: dev" \
  -d '{"model_type":"xgboost","version":"v20240614_120000","artifact_uri":"s3://traffic-models/models/v20240614_120000/model.json","feature_set_id":"v1","tenant_id":"campus-net"}'
```
