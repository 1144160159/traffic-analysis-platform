# MLOps 训练流程

基于 Argo Workflows 的自动化模型训练流程，支持反馈回灌和热更新。

## 📋 目录结构

```
mlops/
├── requirements.txt              # Python 依赖
├── scripts/                      # 训练脚本
│   ├── extract_data.py          # 数据提取
│   ├── train_model.py           # 模型训练
│   ├── evaluate_model.py        # 模型评估
│   └── register_model.py        # 模型注册
├── workflows/                    # Argo Workflow 定义
│   ├── training-workflow.yaml   # 主训练流程
│   ├── cron-training-workflow.yaml  # 定时触发
│   ├── mlops-configmap.yaml     # 脚本 ConfigMap
│   └── mlops-secrets.yaml       # Secrets & RBAC
└── README.md
```

## 🎯 功能特性

### 1. **自动化数据提取**
- 从 ClickHouse 提取特征数据
- 关联用户反馈标注 (TP/FP)
- 自动分割训练集/测试集 (80/20)
- Parquet 格式存储到 S3

### 2. **模型训练**
- 支持 XGBoost / LightGBM
- 类别不平衡处理 (scale_pos_weight)
- 交叉验证
- 特征重要性分析

### 3. **模型评估**
- 多维度指标 (F1/Precision/Recall/AUC)
- 混淆矩阵分析
- PR 曲线和 ROC 曲线
- 最佳阈值计算
- 错误分析

### 4. **模型注册**
- 版本化管理
- MinIO 对象存储
- Model Registry API 集成
- Kafka/Nacos 热更新通知

### 5. **定时触发**
- CronWorkflow 定时执行
- 每周日凌晨 2:00
- 历史记录保留

## 🚀 快速开始

### 前置条件

1. 安装 Argo Workflows
```bash
kubectl create namespace argo
kubectl apply -n argo -f https://github.com/argoproj/argo-workflows/releases/download/v3.5.0/install.yaml
```

2. 创建命名空间
```bash
kubectl create namespace traffic-analysis
```

3. 部署 Secrets 和 RBAC
```bash
kubectl apply -f workflows/mlops-secrets.yaml
```

4. 部署 ConfigMap (包含脚本)
```bash
kubectl apply -f workflows/mlops-configmap.yaml
```

### 手动触发训练

1. 提交 Workflow
```bash
argo submit -n traffic-analysis workflows/training-workflow.yaml \
  --parameter model-type=xgboost \
  --parameter lookback-days=7
```

2. 查看日志
```bash
# 列出所有 Workflows
argo list -n traffic-analysis

# 查看详细日志
argo logs -n traffic-analysis <workflow-name>

# 实时跟踪
argo logs -n traffic-analysis <workflow-name> --follow
```

3. 查看状态
```bash
argo get -n traffic-analysis <workflow-name>
```

### 定时触发

1. 部署 CronWorkflow
```bash
kubectl apply -f workflows/cron-training-workflow.yaml
```

2. 查看定时任务
```bash
argo cron list -n traffic-analysis
```

3. 手动触发一次
```bash
argo cron trigger -n traffic-analysis weekly-model-training
```

4. 暂停/恢复
```bash
# 暂停
argo cron suspend -n traffic-analysis weekly-model-training

# 恢复
argo cron resume -n traffic-analysis weekly-model-training
```

## 📊 数据流程

```
┌─────────────────┐
│  ClickHouse     │  ← 特征数据 + 用户反馈
│  features_stat  │
│  alert_feedback │
└────────┬────────┘
         │ SQL 查询
         ↓
┌─────────────────┐
│  Data Extract   │  → train.parquet (80%)
│  Python Script  │  → test.parquet (20%)
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│  Model Training │  → XGBoost/LightGBM
│  Cross-Validate │  → Feature Importance
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│  Model Evaluate │  → F1/Precision/Recall
│  Test Set       │  → Confusion Matrix
└────────┬────────┘
         │
         ↓ (F1 > 0.85)
┌─────────────────┐
│  Model Register │  → MinIO S3
│  Version: vYYYY │  → Model Registry API
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│  Flink Reload   │  ← Kafka notification
│  Hot Update     │  ← Nacos config
└─────────────────┘
```

## 🔧 配置参数

### Workflow 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `model-type` | `xgboost` | 模型类型 (xgboost/lightgbm) |
| `model-id` | `behavior-classifier` | 模型标识符 |
| `feature-set-id` | `v1` | 特征集版本 |
| `tenant-id` | `campus-net` | 租户 ID |
| `lookback-days` | `7` | 回溯天数 |
| `test-size` | `0.2` | 测试集比例 |
| `min-f1-score` | `0.85` | 最小 F1 阈值 |

### 环境变量

**ClickHouse 连接**
- `CLICKHOUSE_HOST`: ClickHouse 主机
- `CLICKHOUSE_PORT`: 端口 (默认 9000)
- `CLICKHOUSE_DB`: 数据库名
- `CLICKHOUSE_USER`: 用户名
- `CLICKHOUSE_PASSWORD`: 密码

**MinIO 连接**
- `MINIO_ENDPOINT`: MinIO 端点
- `MINIO_ACCESS_KEY`: Access Key
- `MINIO_SECRET_KEY`: Secret Key
- `MINIO_BUCKET`: Bucket 名称

**Model Registry**
- `MODEL_REGISTRY_URL`: Rule Manager API 地址
- `API_TOKEN`: API 认证 Token

**通知配置**
- `NOTIFICATION_METHOD`: 通知方式 (kafka/nacos)
- `KAFKA_BOOTSTRAP_SERVERS`: Kafka 地址
- `MODEL_UPDATE_TOPIC`: Kafka Topic

## 📈 监控与调试

### 1. 查看 Artifact

```bash
# 列出所有 Artifacts
argo get -n traffic-analysis <workflow-name> -o wide

# 下载 Artifact
argo artifact get -n traffic-analysis <workflow-name> <artifact-name>
```

### 2. 查看输出参数

```bash
argo get -n traffic-analysis <workflow-name> -o json | jq '.status.outputs.parameters'
```

### 3. 重新运行失败的步骤

```bash
argo resubmit -n traffic-analysis <workflow-name>
```

### 4. 删除旧 Workflow

```bash
# 删除单个
argo delete -n traffic-analysis <workflow-name>

# 批量删除
argo delete -n traffic-analysis --older 7d
```

## 🔍 故障排查

### 常见问题

#### 1. 数据提取失败

```bash
# 查看 extract-data 步骤日志
argo logs -n traffic-analysis <workflow-name> -c extract-data

# 常见原因：
# - ClickHouse 连接失败 → 检查 Secret
# - SQL 查询超时 → 增加 lookback-days
# - 无标注数据 → 检查 alert_feedback 表
```

#### 2. 模型训练失败

```bash
# 查看 train-model 步骤日志
argo logs -n traffic-analysis <workflow-name> -c train-model

# 常见原因：
# - 内存不足 → 增加 resources.limits.memory
# - 数据质量问题 → 检查 NaN/Inf 值
# - 特征列缺失 → 检查 metadata.json
```

#### 3. 评估不通过 (F1 < 0.85)

```bash
# 查看评估指标
argo artifact get -n traffic-analysis <workflow-name> metrics -o metrics.json
cat metrics.json | jq '.classification_report'

# 优化建议：
# - 增加训练数据 (lookback-days)
# - 调整模型超参数
# - 特征工程改进
# - 处理类别不平衡
```

#### 4. 模型注册失败

```bash
# 查看注册日志
argo logs -n traffic-analysis <workflow-name> -c register-model

# 常见原因：
# - MinIO 连接失败 → 检查 minio-secret
# - Model Registry API 不可用 → 检查 rule-manager 服务
# - Kafka 通知失败 → 检查 Kafka 连接
```

## 🎓 最佳实践

### 1. 数据质量
- 确保至少有 1000+ 标注样本
- TP/FP 比例不要过于失衡 (建议 1:3 ~ 3:1)
- 定期清理异常标注

### 2. 模型版本管理
- 使用语义化版本号 (v20240101_120000)
- 保留最近 5 个版本
- 记录每个版本的训练参数和指标

### 3. 资源限制
- 根据数据量调整内存/CPU 限制
- 使用 `nodeSelector` 指定训练节点
- 避免高峰期运行训练

### 4. 监控告警
- 监控 Workflow 成功率
- F1 Score 低于阈值告警
- 训练时长异常告警

## 📦 扩展功能

### 1. 多模型训练

```yaml
# 并行训练多个模型
- - name: train-xgboost
    template: train-model-step
    arguments:
      parameters:
      - name: model-type
        value: "xgboost"
  - name: train-lightgbm
    template: train-model-step
    arguments:
      parameters:
      - name: model-type
        value: "lightgbm"
```

### 2. 超参数调优

```python
# train_model.py 中集成 Optuna
import optuna

def objective(trial):
    params = {
        'max_depth': trial.suggest_int('max_depth', 3, 10),
        'learning_rate': trial.suggest_float('learning_rate', 0.01, 0.3),
        # ...
    }
    # 训练并返回 F1 Score
```

### 3. A/B 测试

```yaml
# 注册多个版本，逐步切量
- name: register-model-v1
  template: register-model-step
  arguments:
    parameters:
    - name: traffic-percentage
      value: "10"  # 10% 灰度流量
```

## 📚 参考文档

- [Argo Workflows](https://argoproj.github.io/argo-workflows/)
- [XGBoost Documentation](https://xgboost.readthedocs.io/)
- [LightGBM Documentation](https://lightgbm.readthedocs.io/)
- [ClickHouse Python Driver](https://clickhouse-driver.readthedocs.io/)
- [MinIO Python SDK](https://min.io/docs/minio/linux/developers/python/minio-py.html)

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！

## 📄 许可证

MIT License
