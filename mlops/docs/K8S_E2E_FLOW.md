# MLOps K8s 环境端到端流程

## 一、全流程总览

```
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│                     MLOps End-to-End Flow — K8s 环境 (traffic-analysis-platform)           │
├──────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                            │
│  ┌─────────────────────────────────── 数据采集层 ───────────────────────────────────┐    │
│  │                                                                                    │    │
│  │  Probe Agent (Rust)         Fluent Bit (K8s)      Keycloak + APISIX               │    │
│  │  AF_XDP 抓包                UDP 514 Syslog        SSO/OIDC 登录事件               │    │
│  │  ↓ gRPC + mTLS              ↓ Kafka                ↓ Kafka                         │    │
│  └────────────────────────────────────────────────────────────────────────────────────┘    │
│                                          │                                                 │
│  ┌─────────────────────────── Kafka 数据总线 (middleware/) ──────────────────────────┐    │
│  │  flow.events.v1 │ feature.stat.v1 │ detections.v1 │ alerts.v1 │ alert.feedback.v1 │    │
│  │  kafka.middleware.svc:9092  (NodePort 30092)                                        │    │
│  └────────────────────────────────────────────────────────────────────────────────────┘    │
│                                          │                                                 │
│  ┌──────────────────────────── Flink 流计算 (flink/) ───────────────────────────────┐    │
│  │  Session → Feature → Rule → CEP → Behavior → Alert Generator                      │    │
│  │  BehaviorDetectorFunction: XGBoost4j 模型推理 (热更新)                             │    │
│  └────────────────────────────────────────────────────────────────────────────────────┘    │
│                                          │                                                 │
│  ┌────────────────────────────── 存储层 ────────────────────────────────────────────┐    │
│  │  ClickHouse (middleware/)    PostgreSQL (databases/)    MinIO (minio/)            │    │
│  │  features_stat                models / model_versions     traffic-models bucket   │    │
│  │  alert_feedback               deployments                model.json               │    │
│  │  flows_raw                    feature_sets               feature_columns.json     │    │
│  └────────────────────────────────────────────────────────────────────────────────────┘    │
│                                          │                                                 │
│  ┌──────────────────────────── MLOps 训练层 ────────────────────────────────────────┐    │
│  │                                                                                    │    │
│  │  Argo Workflows (argo/ namespace, NodePort 30046)                                  │    │
│  │  ┌─────────────────────────────────────────────────────────────────────────┐     │    │
│  │  │  1. Preflight Check    → ClickHouse 数据就绪检查                          │     │    │
│  │  │  2. Data Extraction    → SQL → Parquet (80/20 split) → MinIO artifacts   │     │    │
│  │  │  3. Model Training     → XGBoost/LightGBM + cross-validation + early-stop│     │    │
│  │  │  4. Model Evaluation   → F1/AUC/PR/ROC + confusion matrix + error analysis│    │    │
│  │  │  5. Drift Detection    → PSI × 7 特征维度 (新增)                          │     │    │
│  │  │  6. Model Registration → MinIO upload + Go API + Kafka notify             │     │    │
│  │  │  7. Auto-Activate      → Go Model Registry API (条件: F1 > 0.85)         │     │    │
│  │  └─────────────────────────────────────────────────────────────────────────┘     │    │
│  └────────────────────────────────────────────────────────────────────────────────────┘    │
│                                          │                                                 │
│  ┌──────────────────────── Go 控制面 (traffic-analysis/) ──────────────────────────┐    │
│  │                                                                                    │    │
│  │  Rule Manager Service (NodePort via APISIX 30180)                                  │    │
│  │  ┌────────────────────────────────────────────────────────────────────────┐      │    │
│  │  │  Model Registry API (14 端点)                                            │      │    │
│  │  │  MLOps Orchestrator (5 触发条件, 每小时检查)                             │      │    │
│  │  │  Model Service (Kafka PublishModelUpdate → Flink 热更新)                 │      │    │
│  │  └────────────────────────────────────────────────────────────────────────┘      │    │
│  └────────────────────────────────────────────────────────────────────────────────────┘    │
│                                          │                                                 │
│  ┌──────────────────────── 自编排闭环 ────────────────────────────────────────────┐    │
│  │                                                                                    │    │
│  │  Flink 推理 → Alert → 运营标注 (TP/FP) → ClickHouse alert_feedback               │    │
│  │      ↑                                                              ↓             │    │
│  │      │                                               MLOps Orchestrator 检查:     │    │
│  │      │                                               - feedback ≥ 500 条          │    │
│  │      │                                               - FP rate > 15%              │    │
│  │      └────────── 自动提交 Argo Workflow ←────────── - PSI drift > 0.25            │    │
│  │                                                                                    │    │
│  └────────────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                            │
└──────────────────────────────────────────────────────────────────────────────────────────┘
```

## 二、K8s 架构与服务拓扑

### 命名空间与核心服务

```
K8s Cluster (2 nodes: Node-8 / Node-9)

┌─ middleware ──────────────────────────────────────────────────────┐
│  Kafka            kafka.middleware.svc:9092          NP 30092     │
│  ClickHouse-1     clickhouse-1.middleware.svc:8123   NP 30023     │
│  ClickHouse-2     clickhouse-2.middleware.svc:8123   NP 30123     │
│  OpenSearch       opensearch.middleware.svc:9200     NP 30020     │
│  NebulaGraph      nebula-graph.middleware.svc:9669   NP 30069     │
│  NebulaGraph HTTP nebula-graph.middleware.svc:19669  NP 31179     │
└───────────────────────────────────────────────────────────────────┘
┌─ databases ───────────────────────────────────────────────────────┐
│  PostgreSQL       postgres-primary.databases.svc:5432  NP 30032  │
│  Redis            redis-master.databases.svc:6379      NP 30079  │
│  Redis Sentinel   redis-sentinel.databases.svc:26379   NP 30279  │
└───────────────────────────────────────────────────────────────────┘
┌─ minio ───────────────────────────────────────────────────────────┐
│  MinIO            minio.minio.svc:9000                 NP 30000  │
│  MinIO Console    minio.minio.svc:9001                 NP 31728  │
└───────────────────────────────────────────────────────────────────┘
┌─ flink ───────────────────────────────────────────────────────────┐
│  Flink JM         flink-jobmanager.flink.svc:8081      NP 30082  │
│  Flink TM ×2      (TaskManager per node)                          │
│  StreamPark       streampark.streampark.svc:8080       NP 30100  │
└───────────────────────────────────────────────────────────────────┘
┌─ argo ────────────────────────────────────────────────────────────┐
│  Argo Server      argo-server.argo.svc:2746           NP 30046  │
│  Workflow Controller                                             │
└───────────────────────────────────────────────────────────────────┘
┌─ traffic-analysis ────────────────────────────────────────────────┐
│  rule-manager     rule-manager.traffic-analysis.svc:8080         │
│  alert-service    alert-service.traffic-analysis.svc:8080        │
│  ingest-gateway   ingest-gateway.traffic-analysis.svc:8080       │
│  ... (7 Go microservices)                                        │
└───────────────────────────────────────────────────────────────────┘
┌─ gateway ─────────────────────────────────────────────────────────┐
│  APISIX           apisix-gateway.gateway.svc:9080     NP 30180  │
│  (L7 路由: /api/v1/* → rule-manager, /grafana → grafana, ...)    │
└───────────────────────────────────────────────────────────────────┘
```

### 外部访问地址

| 组件 | 外部 URL |
|------|---------|
| **Argo Server** | `http://10.0.5.8:30046` |
| **StreamPark** | `http://10.0.5.8:30174` |
| **Flink Web UI** | `http://10.0.5.8:30172` |
| **MinIO Console** | `http://10.0.5.8:31728` |
| **Grafana** | `http://10.0.5.8:30300` |

## 三、全流程 Step-by-Step

### Step 1: 数据采集 → 存储

```
Probe Agent (Rust, AF_XDP)
  │  每 100ms / 100 条批量上报
  ▼  gRPC UploadFlows (mTLS 双向认证)
Ingest Gateway (Go)
  │  Auth → 限流 → 去重 → 分区
  ▼  Kafka flow.events.v1
Flink Session Job
  │  community_id 会话聚合
  ▼  ClickHouse sessions_agg
Flink Feature Job
  │  L1/L2/L3 统计特征提取
  ▼  ClickHouse features_stat + Kafka feature.stat.v1
```

### Step 2: 模型推理（实时）

```
Flink BehaviorDetectorFunction
  │  消费 Kafka feature.stat.v1
  │
  ├─ [规则模型] ScanDetectionModel / TunnelDetectionModel / ... (10 个硬编码规则)
  │     → DetectionBehavior
  │
  └─ [ML 模型] XGBoostModelWrapper (热加载)
        │  MinioModelLoader 已缓存: /opt/flink/models/{hash}/model.json
        │  XGBoost4j Booster.loadModel()
        │  buildFeatureVector(featureStat) → float[]
        │  booster.predict(DMatrix(features))
        │
        ▼  DetectionBehavior (model_version, top_label, top_score, labels, scores)
```

### Step 3: 告警生成与反馈收集

```
Flink Alert Generator Job
  │  去重 + 证据生成
  ▼  ClickHouse alerts + Kafka alert.events.v1
Go Alert Service
  │  威胁情报富化 + 白名单过滤
  ▼  Web UI 告警列表
运营人员
  │  标记 TP (真正) / FP (误报)
  ▼  POST /api/v1/alerts/{id}/feedback
Go Feedback Handler
  │  Kafka alert.feedback.v1 + ClickHouse alert_feedback
  ▼  反馈积累 ← MLOps Orchestrator 监控此表
```

### Step 4: 自编排触发 → 模型重训

```
MLOps Orchestrator (每 1 小时)
  │
  ├─ Check 1: ClickHouse alert_feedback 24h 新增 ≥ 500 条 ────→ Trigger: feedback
  ├─ Check 2: FP rate > 15% (样本 ≥ 100) ──────────────────────→ Trigger: fp_rate
  ├─ Check 3: PSI drift > 0.25 (7 特征维度) ───────────────────→ Trigger: drift
  ├─ Check 4: CronWorkflow 每周日 02:00 ────────────────────────→ Trigger: scheduled
  └─ Check 5: API POST /api/v1/mlops/retrain ──────────────────→ Trigger: manual
         │
         ▼ 条件满足
  argo submit --from workflowtemplate/mlops-training-template
         │
         ▼
  Argo Workflow 7 步流水线 (见 Step 5)
```

### Step 5: Argo Workflow 训练流水线

```
argo submit -n traffic-analysis
  --from workflowtemplate/mlops-training-template
  -p model-type=xgboost -p lookback-days=7

┌─ Step 0: preflight-check ─────────────────────────────────────┐
│  python:3.11-slim                                              │
│  → ClickHouse 查询: feedback ≥ 100? features ≥ 1000?          │
│  → 输出: data-ready = true/false                               │
│  → 条件不满足 → 跳过后续步骤                                    │
└────────────────────────────────────────────────────────────────┘
         │ data-ready = true
         ▼
┌─ Step 1: extract-data ────────────────────────────────────────┐
│  python:3.11-slim + requirements.txt                           │
│  → SQL JOIN: features_stat ⋈ alert_feedback (community_id)    │
│  → Preprocessing: NaN→0, Inf→0, dedup, IQR outlier detection  │
│  → Stratified split: 80% train / 20% test                     │
│  → Output: train.parquet, test.parquet, metadata.json → S3    │
└────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─ Step 2: train-model ─────────────────────────────────────────┐
│  python:3.11-slim + xgboost + lightgbm + scikit-learn          │
│  → XGBoost: scale_pos_weight, early_stop(20), cross-val(5)   │
│  → LightGBM: scale_pos_weight, early_stop(20), cross-val(5)  │
│  → Output: model.json, feature_importance.json, metrics.json  │
└────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─ Step 3: evaluate-model ──────────────────────────────────────┐
│  python:3.11-slim + xgboost + scikit-learn                     │
│  → Metrics: F1, Precision, Recall, AUC-ROC, AUC-PR            │
│  → Confusion Matrix: TN, FP, FN, TP                            │
│  → Best Threshold: F1-maximizing                               │
│  → Error Analysis: FP/FN 特征分布, 置信度分析                  │
│  → Output: f1_score.txt, metrics.json, summary.json           │
└────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─ Step 4: drift-check (新增) ──────────────────────────────────┐
│  python:3.11-slim + scipy + numpy                              │
│  → PSI (Population Stability Index) × 7 features              │
│  → Baseline: 30d-7d vs Recent: 24h                             │
│  → PSI > 0.25 → 标记漂移                                       │
│  → Output: drift_report.json, has_drift.txt                   │
└────────────────────────────────────────────────────────────────┘
         │
         ▼  F1 > 0.85 ?
┌─ Step 5: register-model ──────────────────────────────────────┐
│  python:3.11-slim + minio + requests                           │
│  → 上传 MinIO: model.json, feature_columns.json, metrics      │
│  → 调用 Go API: POST /api/v1/models/{id}/versions             │
│  → Kafka 通知: PublishModelUpdate (rule.updates topic)         │
└────────────────────────────────────────────────────────────────┘
         │
         ▼  auto-activate = true ?
┌─ Step 6: auto-activate (新增) ────────────────────────────────┐
│  curlimages/curl                                               │
│  → POST /api/v1/models/{id}/versions/{v}/activate             │
│  → Go: PublishDeploymentEvent + PublishModelUpdate             │
└────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─ Step 7: send-notification ───────────────────────────────────┐
│  curlimages/curl                                               │
│  → 打印汇总: Trigger, F1 Score, Workflow Name                  │
│  → 可选: Slack Webhook / Email                                 │
└────────────────────────────────────────────────────────────────┘
```

### Step 6: 模型热加载 → 推理生效

```
Argo Step 6 (auto-activate) 或手动 POST .../activate
  │
  ▼
Go Model Registry API
  │  model_versions.status = 'active'
  │  deprecateOtherVersions()
  │  PublishModelUpdate() → Kafka (rule.updates topic)
  ▼
Kafka: rule.updates
  │  Headers: { event_type: "model_update", action: "activated" }
  │  Value: { model_id, version, artifact_uri, action: "activated" }
  ▼
Flink ModelUpdateBroadcastHandler (BroadcastProcessFunction)
  │  processBroadcastElement():
  │    ctx.getBroadcastState(MODEL_UPDATE_STATE)
  │    state.put(modelName, ModelUpdateState(modelType, version, artifactUri, ...))
  ▼
Flink BehaviorDetectorFunction (每个并行 TaskManager 实例)
  │  processElement():
  │    ReadOnlyBroadcastState → 检测到新版本
  │    ↓
  │  MinioModelLoader.download(artifactUri)
  │    ↓ SHA256 cache key → /opt/flink/models/{hash}/model.json
  │    ↓ MinIO SDK getObject()
  │  XGBoostModelWrapper.initialize()
  │    ↓ Booster.loadModel(localPath)
  │  ModelRegistry.hotSwap(modelName, newWrapper)
  │    ↓ models.put("behavior-classifier", newBooster)
  │    ↓ oldBooster.close()
  │
  ▼  下一次 asyncInvoke(featureStat):
  XGBoostModelWrapper.infer(featureStat)
  │  features = buildFeatureVector(featureStat)
  │  score = booster.predict(DMatrix(features))[0][0]
  │  → ModelInferenceResult(topLabel, topScore, detected)
  │  → DetectionBehavior → Kafka detections.behavior.v1
```

### Step 7: 监控观测

```
Prometheus Metrics (由各组件暴露):
  ┌─────────────────────────────────────────────────────────────┐
  │  Argo Workflow:                                              │
  │    workflows_completed_total{status="Succeeded|Failed"}     │
  │    workflow_duration_seconds                                 │
  │                                                              │
  │  Go Model Registry:                                          │
  │    model_versions_total{status="active|registered|..."}     │
  │    model_register_duration_seconds                           │
  │                                                              │
  │  Go Orchestrator:                                            │
  │    mlops_checks_total{trigger="feedback|fp_rate|drift"}     │
  │    mlops_retrain_interval_seconds                            │
  │                                                              │
  │  Flink BehaviorDetectorFunction:                             │
  │    model_inference_duration_seconds                          │
  │    model_inference_total{model="behavior-classifier"}       │
  │    model_load_success_total                                  │
  │                                                              │
  │  MinIO:                                                      │
  │    minio_bucket_objects_total{bucket="traffic-models"}      │
  │    minio_bucket_usage_bytes                                  │
  └─────────────────────────────────────────────────────────────┘

Grafana Dashboard:
  http://10.0.5.8:30300/grafana → MLOps Dashboard
  ┌─────────────────────────────────────────────────────────────┐
  │  📊 模型性能趋势: F1/AUC 时序图                              │
  │  📊 特征漂移热力图: PSI × 7 特征                              │
  │  📊 反馈标注率: TP/FP 比例趋势                                │
  │  📊 训练流水线: 成功率 / 耗时分布                              │
  │  📊 模型版本: 激活版本 / 候选版本 / F1 对比                    │
  └─────────────────────────────────────────────────────────────┘
```

## 四、K8s 部署命令速查

### 首次部署

```bash
# 1. 部署 Argo WorkflowTemplate
kubectl apply -f mlops/workflows/mlops-workflow-template.yaml
kubectl apply -f mlops/workflows/cron-training-workflow.yaml

# 2. 创建 MLOps Secrets
kubectl apply -f mlops/workflows/mlops-secrets.yaml

# 3. 更新 MLOps ConfigMap (Python 脚本)
kubectl create configmap mlops-scripts \
  --namespace=traffic-analysis \
  --from-file=mlops/scripts/ \
  --dry-run=client -o yaml | kubectl apply -f -

# 4. 重新部署 rule-manager (含 Model Registry + Orchestrator)
kubectl rollout restart deployment/rule-manager -n traffic-analysis
```

### 触发训练

```bash
# 方式 1: CLI 直接提交
argo submit -n traffic-analysis \
  --from workflowtemplate/mlops-training-template \
  --generate-name mlops-manual- \
  -p model-type=xgboost \
  -p lookback-days=7 \
  -p auto-activate=false

# 方式 2: API 触发
curl -X POST http://10.0.5.8:30180/api/v1/mlops/retrain \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: campus-net" \
  -H "X-User-ID: admin" \
  -d '{"model_type":"xgboost","lookback_days":7}'

# 方式 3: 查看状态
curl http://10.0.5.8:30180/api/v1/mlops/status
```

### 查看模型

```bash
# 查看已注册模型
curl http://10.0.5.8:30180/api/v1/models?tenant_id=campus-net \
  -H "X-Tenant-ID: campus-net" -H "X-User-ID: admin" | jq .data

# 查看模型版本
curl http://10.0.5.8:30180/api/v1/models/behavior-classifier/versions \
  -H "X-Tenant-ID: campus-net" -H "X-User-ID: admin" | jq .data

# 查看激活版本
curl http://10.0.5.8:30180/api/v1/models/behavior-classifier/versions/active \
  -H "X-Tenant-ID: campus-net" -H "X-User-ID: admin" | jq .data
```

### 监控流水线

```bash
# 查看最近 10 个 workflow
argo list -n traffic-analysis | head -10

# 查看 workflow 详情
argo get -n traffic-analysis <workflow-name>

# 查看 step 日志
argo logs -n traffic-analysis <workflow-name> -c extract-data
argo logs -n traffic-analysis <workflow-name> -c train-model
argo logs -n traffic-analysis <workflow-name> -c evaluate-model

# 下载产物
argo artifact get -n traffic-analysis <workflow-name> metrics -o /tmp/metrics.json
cat /tmp/metrics.json | jq '.f1_score'
```

### 查看 MinIO 存储

```bash
# Port-forward MinIO Console
kubectl port-forward -n minio svc/minio 9001:9001 &

# 浏览器访问: http://localhost:9001
# Login: minioadmin / minioadmin123
# Bucket: traffic-models → models/ → 查看各版本模型文件
```

## 五、关键时间节点

| 步骤 | 预期耗时 | 瓶颈 |
|------|:--:|------|
| Probe → Kafka → ClickHouse | < 60s (P95) | 网络 + Flink 处理 |
| FeatureStat → DetectionBehavior | < 100ms | Flink 推理 (async) |
| Alert → 运营标注 (TP/FP) | 人工 | 反馈闭环关键路径 |
| Orchestrator 检查周期 | 1h (可配) | ClickHouse 查询 |
| Argo Workflow (全流程) | 10–30min | 模型训练 (CPU) |
| 模型热加载 (Kafka → Flink) | < 30s | Broadcast State 传播 |
| 端到端 (反馈 → 新模型上线) | 12h–24h | 最小重训间隔 |

## 六、相关文档

| 文档 | 内容 |
|------|------|
| [`MODEL_STORAGE.md`](MODEL_STORAGE.md) | MinIO 存储布局、格式兼容、下载流程 |
| [`CICD.md`](CICD.md) | GitHub Actions CI/CD 流水线 |
| [`agent.md`](../../agent.md) | 项目开发规范、子系统边界 |
