# Flink Rule Job 部署指南

## 硬件环境约束

### Node-8 (10.0.5.8) - Master节点
- **配置**: 2TB RAM, 1TB SSD, 120TB HDD
- **SSD 目录**: `/home/k8s-data/flink` (Checkpoint, RocksDB State)
- **角色**: JobManager + TaskManager

### Node-9 (10.0.5.9) - Worker节点  
- **配置**: 512GB RAM, 1TB SSD, 120TB HDD
- **SSD 目录**: `/home/k8s-data/flink`
- **角色**: TaskManager

## 前置条件

### 1. 软件依赖

```bash
# OpenEuler 22.03 LTS
cat /etc/os-release

# Java 11
java -version

# Maven 3.8+
mvn -version

# Docker
docker --version

# Kubernetes
kubectl version
```

### 2. 存储准备

```bash
# Node-8 & Node-9
sudo mkdir -p /home/k8s-data/flink/{checkpoints,savepoints,rocksdb}
sudo chown -R flink:flink /home/k8s-data/flink
sudo chmod -R 755 /home/k8s-data/flink
```

### 3. Kafka Topics

```bash
# 创建必需的 Kafka Topics
kafka-topics.sh --create \
  --bootstrap-server localhost:9092 \
  --topic feature.stat.v1 \
  --partitions 12 \
  --replication-factor 2 \
  --config retention.ms=259200000 \
  --config compression.type=lz4

kafka-topics.sh --create \
  --bootstrap-server localhost:9092 \
  --topic rule.updates \
  --partitions 1 \
  --replication-factor 2 \
  --config retention.ms=-1 \
  --config cleanup.policy=compact

kafka-topics.sh --create \
  --bootstrap-server localhost:9092 \
  --topic detections.v1 \
  --partitions 12 \
  --replication-factor 2 \
  --config retention.ms=604800000

kafka-topics.sh --create \
  --bootstrap-server localhost:9092 \
  --topic dlq.rule-job \
  --partitions 1 \
  --replication-factor 2 \
  --config retention.ms=2592000000
```

### 4. ClickHouse 表结构

```sql
-- 在 Node-8 & Node-9 上创建本地表
CREATE TABLE IF NOT EXISTS traffic.detections_behavior_local ON CLUSTER default
(
    tenant_id String,
    run_id String,
    feature_set_id String,
    model_version String,
    event_id String,
    
    community_id String,
    object_type String,
    object_id String,
    
    ts DateTime64(3),
    
    labels Array(String),
    scores Array(Float32),
    top_label String,
    top_score Float32,
    
    ingest_ts DateTime64(3)
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/detections_behavior_local', '{replica}')
PARTITION BY toYYYYMMDD(ts)
ORDER BY (tenant_id, ts, community_id)
TTL ts + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- 创建分布式表
CREATE TABLE IF NOT EXISTS traffic.detections_behavior ON CLUSTER default
AS traffic.detections_behavior_local
ENGINE = Distributed(default, traffic, detections_behavior_local, cityHash64(tenant_id));
```

## 构建步骤

### 1. 克隆代码

```bash
git clone https://github.com/1144160159/traffic-analysis-platform.git
cd traffic-analysis-platform/flink-jobs
```

### 2. 编译 Protobuf

```bash
# 假设 proto 定义在 proto/ 目录
cd ../proto
protoc --java_out=../flink-jobs/proto-java/src/main/java \
  traffic/v1/*.proto

cd ../flink-jobs
```

### 3. 编译打包

```bash
# 完整编译（包含 common 和 rule-job）
mvn clean package -DskipTests

# 编译产物
ls -lh flink-rule-job/target/flink-rule-job-*.jar
```

### 4. 构建 Docker 镜像

```bash
cd flink-rule-job

# 构建镜像
docker build -t traffic-analysis/flink-rule-job:1.0.0 .

# 推送到内部镜像仓库
docker tag traffic-analysis/flink-rule-job:1.0.0 \
  registry.local/traffic-analysis/flink-rule-job:1.0.0

docker push registry.local/traffic-analysis/flink-rule-job:1.0.0
```

## Kubernetes 部署

### 1. 创建命名空间

```bash
kubectl create namespace traffic-analysis
```

### 2. 创建 Secret（可选）

```bash
kubectl create secret generic clickhouse-secret \
  --from-literal=password='your-clickhouse-password' \
  -n traffic-analysis
```

### 3. 部署服务

```bash
# 应用配置
kubectl apply -f k8s/deployment.yaml

# 验证部署
kubectl get all -n traffic-analysis -l app=flink-rule-job
```

### 4. 检查部署状态

```bash
# 使用验证脚本
chmod +x scripts/verify-deployment.sh
./scripts/verify-deployment.sh
```

### 5. 访问 Flink UI

```bash
# 端口转发
kubectl port-forward -n traffic-analysis \
  svc/flink-rule-job-jobmanager-rest 8081:8081

# 浏览器访问
open http://localhost:8081
```

## 配置调优

### 1. 内存配置（基于 Node-8 2TB 内存）

```yaml
# JobManager (Node-8)
jobmanager.memory.process.size: 8g
jobmanager.memory.jvm-overhead.max: 2g

# TaskManager (Node-8, 分配 100GB)
taskmanager.memory.process.size: 100g
taskmanager.memory.managed.size: 50g
taskmanager.memory.network.min: 10g
taskmanager.memory.network.max: 20g

# RocksDB (SSD 加速)
state.backend.rocksdb.localdir: /home/k8s-data/flink/rocksdb
state.backend.rocksdb.memory.managed: true
state.backend.rocksdb.memory.write-buffer-ratio: 0.5
state.backend.rocksdb.memory.high-prio-pool-ratio: 0.1
```

### 2. Checkpoint 优化

```properties
# 高频 Checkpoint（基于 SSD）
execution.checkpointing.interval=30000
execution.checkpointing.min-pause=15000
execution.checkpointing.timeout=180000

# 增量 Checkpoint
state.backend.incremental=true
state.backend.local-recovery=true
```

### 3. 并行度配置

```properties
# 基于 12 个 Kafka 分区
parallelism.default=12
taskmanager.numberOfTaskSlots=6

# TaskManager 实例数 = 12 / 6 = 2
# 每个实例 50GB 内存 * 2 = 100GB
```

### 4. Kafka 消费优化

```properties
# 大批次拉取（充分利用 2TB 内存）
kafka.consumer.fetch.min.bytes=10485760
kafka.consumer.fetch.max.wait.ms=500
kafka.consumer.max.poll.records=50000

# 高吞吐生产
kafka.producer.batch.size=131072
kafka.producer.linger.ms=10
kafka.producer.compression.type=lz4
```

## 规则管理

### 1. 提交示例规则

```bash
chmod +x scripts/submit-sample-rules.sh
export KAFKA_BROKERS="kafka-headless.traffic-analysis.svc.cluster.local:9092"
./scripts/submit-sample-rules.sh
```

### 2. 动态更新规则

```bash
# 创建新规则
cat > custom-rule.json <<EOF
{
  "rule_id": "rule-custom-001",
  "tenant_id": "tenant-1",
  "name": "Custom High BPS Detection",
  "type": "threshold",
  "enabled": true,
  "severity": "critical",
  "priority": 90,
  "conditions": {
    "feature": "bps",
    "operator": ">",
    "value": 1000000000
  },
  "labels": ["custom", "bandwidth"],
  "version": 1,
  "action": "update"
}
EOF

# 提交到 Kafka
kafka-console-producer.sh \
  --bootstrap-server $KAFKA_BROKERS \
  --topic rule.updates < custom-rule.json

# 验证规则已加载（查看日志）
kubectl logs -n traffic-analysis -l app=flink-rule-job,component=jobmanager \
  | grep "RULE_AUDIT"
```

### 3. 删除规则

```bash
cat > delete-rule.json <<EOF
{
  "rule_id": "rule-custom-001",
  "tenant_id": "tenant-1",
  "action": "delete"
}
EOF

kafka-console-producer.sh \
  --bootstrap-server $KAFKA_BROKERS \
  --topic rule.updates < delete-rule.json
```

## 监控与告警

### 1. Prometheus Metrics

```bash
# 端口转发
kubectl port-forward -n traffic-analysis \
  $(kubectl get pod -n traffic-analysis -l component=jobmanager -o name) \
  9250:9250

# 查看指标
curl http://localhost:9250/metrics | grep flink_rule
```

### 2. 关键指标

- `flink_rule_job_features_processed_total`: 处理特征数
- `flink_rule_job_rules_matched_total`: 规则命中数
- `flink_rule_job_active_rule_count`: 活跃规则数
- `flink_taskmanager_job_task_numRecordsInPerSecond`: 吞吐量
- `flink_taskmanager_Status_JVM_Memory_Heap_Used`: 堆内存使用

### 3. Grafana Dashboard

导入预定义 Dashboard:
```bash
kubectl apply -f k8s/grafana-dashboard.json
```

## 故障恢复

### 1. 从 Checkpoint 恢复

```bash
# 查看最新 Checkpoint
ls -lh /home/k8s-data/flink/checkpoints/rule-job/

# 取消当前 Job
kubectl delete deployment flink-rule-job-jobmanager -n traffic-analysis

# 从 Checkpoint 重启
# 编辑 deployment.yaml，添加：
# args: ["--fromSavepoint", "/home/k8s-data/flink/checkpoints/rule-job/chk-123"]

kubectl apply -f k8s/deployment.yaml
```

### 2. 手动 Savepoint

```bash
# 触发 Savepoint
JOB_ID=$(kubectl exec -n traffic-analysis $(kubectl get pod -n traffic-analysis -l component=jobmanager -o name) -- \
  curl -s http://localhost:8081/jobs | jq -r '.jobs[0].id')

kubectl exec -n traffic-analysis $(kubectl get pod -n traffic-analysis -l component=jobmanager -o name) -- \
  curl -X POST http://localhost:8081/jobs/$JOB_ID/savepoints \
  -H "Content-Type: application/json" \
  -d '{"target-directory": "/home/k8s-data/flink/savepoints/rule-job"}'
```

## 性能基准

### 预期性能（Node-8 + Node-9 集群）

| 指标 | 目标值 | 备注 |
|------|--------|------|
| 吞吐量 | 500K events/s | feature.stat.v1 消费速率 |
| 延迟（P99） | < 200ms | 特征 → 检测结果 |
| Checkpoint 时间 | < 30s | 基于 SSD 的增量 Checkpoint |
| 规则数量 | 1000+ | 支持大规模规则集 |
| 状态大小 | < 500GB | RocksDB State |

### 压测命令

```bash
# 生成测试流量（feature.stat.v1）
flink run -c com.traffic.test.FeatureGenerator \
  test-data-generator.jar \
  --kafka.brokers localhost:9092 \
  --rate 500000 \
  --duration 3600
```

## 常见问题

### Q1: 规则未生效？
检查：
1. 规则 JSON 格式
2. DLQ topic (`dlq.rule-job`)
3. 规则版本号
4. `enabled=true`

### Q2: 内存溢出？
调优：
1. 增加 TaskManager 内存
2. 减少批次大小
3. 启用 RocksDB 增量 Checkpoint

### Q3: Checkpoint 失败？
排查：
1. SSD 空间是否充足
2. 文件系统权限
3. Checkpoint 超时设置

## 联系方式

技术支持: traffic-analysis@example.com
