# Flink Session Job - 部署与运维文档

## 项目概述

**Flink Session Job** 是园区网全流量采集分析系统的核心组件之一，负责将 Flow 事件聚合为双向 Session 会话。

### 核心功能
- ✅ Flow → Session 双向聚合（基于 `community_id`）
- ✅ Idle Timeout：2 分钟无数据自动结束会话
- ✅ Active Timeout：30 分钟长流强制切分
- ✅ Late Data 侧流输出（延迟数据隔离）
- ✅ ClickHouse 异步写入 + DLQ 降级
- ✅ OpenSearch 双写（可选，用于检索）
- ✅ State TTL 防泄漏（30 分钟自动过期）
- ✅ Prometheus Metrics 全链路监控

### 技术栈
- **Flink**: 1.17.2
- **ClickHouse**: 23.8
- **Kafka**: 7.5.0
- **OpenSearch**: 2.11.0
- **Prometheus**: 2.47.0
- **Grafana**: 10.1.0

---

## 快速开始

### 1. 环境准备

#### 前置依赖
- Docker 20.10+
- Docker Compose 2.x
- Maven 3.8+
- JDK 11+

#### 硬件要求（生产环境）
- **JobManager**: 2 核 / 4GB 内存
- **TaskManager**: 4 核 / 8GB 内存（每个）
- **磁盘**: 500GB SSD（Checkpoint） + 2TB HDD（归档）

### 2. 本地开发部署

#### 2.1 编译项目

```bash
# 进入项目目录
cd flink-jobs/flink-session-job

# Maven 编译
mvn clean package -DskipTests

# 检查生成的 JAR
ls -lh target/flink-session-job-*.jar
```

#### 2.2 启动开发环境

```bash
# 启动所有服务（Flink + Kafka + ClickHouse + OpenSearch + Prometheus + Grafana）
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看 JobManager 日志
docker-compose logs -f jobmanager

# 查看 TaskManager 日志
docker-compose logs -f taskmanager
```

#### 2.3 提交作业

```bash
# 方式 1: 通过 Web UI 提交（推荐）
# 访问 http://localhost:8081
# 上传 target/flink-session-job-*.jar
# 设置 Entry Class: com.traffic.flink.session.SessionJob
# 设置并行度: 4

# 方式 2: 通过 CLI 提交
docker exec -it flink-session-jobmanager \
  flink run \
  -c com.traffic.flink.session.SessionJob \
  -p 4 \
  /opt/flink/usrlib/flink-session-job.jar \
  --session.mode process \
  --parallelism 12
```

#### 2.4 验证作业运行

```bash
# 查看作业状态
curl http://localhost:8081/jobs

# 查看 Metrics
curl http://localhost:9250/metrics

# 查看 Prometheus 指标
curl http://localhost:9090/api/v1/query?query=flink_jobmanager_job_uptime

# 查看 ClickHouse 数据
docker exec -it flink-session-clickhouse clickhouse-client
> SELECT count() FROM traffic.sessions_local;
```

---

## 生产环境部署

### 1. Kubernetes 部署（推荐）

#### 1.1 创建 ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: flink-session-job-config
  namespace: flink
data:
  session-job.properties: |
    session.mode=process
    kafka.brokers=kafka-0.kafka-headless.kafka.svc.cluster.local:9092
    input.topic=flow.events.v1
    output.topic=session.events.v1
    clickhouse.url=jdbc:clickhouse://clickhouse-svc.storage.svc.cluster.local:8123/traffic
    parallelism=12
```

#### 1.2 部署 Flink Cluster

```yaml
# 完整的 K8s YAML 见 deploy/kubernetes/flink-session-job-deployment.yaml
kubectl apply -f deploy/kubernetes/
```

#### 1.3 扩缩容

```bash
# 扩展 TaskManager
kubectl scale deployment flink-session-taskmanager --replicas=4

# 查看 Pod 状态
kubectl get pods -n flink -l app=flink-session
```

### 2. 裸金属部署（openEuler 22.03 LTS）

#### 2.1 安装 Flink

```bash
# 下载 Flink
cd /opt
wget https://archive.apache.org/dist/flink/flink-1.17.2/flink-1.17.2-bin-scala_2.12.tgz
tar -xzf flink-1.17.2-bin-scala_2.12.tgz
ln -s flink-1.17.2 flink

# 配置环境变量
echo 'export FLINK_HOME=/opt/flink' >> ~/.bashrc
echo 'export PATH=$FLINK_HOME/bin:$PATH' >> ~/.bashrc
source ~/.bashrc
```

#### 2.2 配置 Flink

编辑 `/opt/flink/conf/flink-conf.yaml`：

```yaml
# JobManager 配置
jobmanager.rpc.address: 10.0.5.8
jobmanager.rpc.port: 6123
jobmanager.memory.process.size: 4096m

# TaskManager 配置
taskmanager.numberOfTaskSlots: 4
taskmanager.memory.process.size: 16384m
taskmanager.memory.managed.fraction: 0.4
taskmanager.memory.network.fraction: 0.1

# 状态后端配置
state.backend: rocksdb
state.backend.incremental: true
state.checkpoints.dir: file:///home/wangwt/task/flink-checkpoints
state.savepoints.dir: file:///home/wangwt/task/flink-savepoints

# Checkpoint 配置
execution.checkpointing.interval: 60000
execution.checkpointing.mode: EXACTLY_ONCE
execution.checkpointing.timeout: 600000

# Metrics 配置
metrics.reporters: prom
metrics.reporter.prom.factory.class: org.apache.flink.metrics.prometheus.PrometheusReporterFactory
metrics.reporter.prom.port: 9250-9260
```

#### 2.3 启动集群

```bash
# 启动 JobManager
/opt/flink/bin/jobmanager.sh start

# 启动 TaskManager（在每个 Worker 节点）
/opt/flink/bin/taskmanager.sh start

# 查看进程
jps | grep -E 'JobManager|TaskManager'

# 提交作业
/opt/flink/bin/flink run \
  -c com.traffic.flink.session.SessionJob \
  -p 12 \
  /path/to/flink-session-job.jar \
  --kafka.brokers 10.0.5.8:9092,10.0.5.9:9092 \
  --clickhouse.url jdbc:clickhouse://10.0.5.8:8123/traffic \
  --parallelism 12
```

---

## 配置参数说明

### 核心参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `session.mode` | `process` | 聚合模式：`window` 或 `process` |
| `session.gap.ms` | `120000` | Idle Timeout（毫秒） |
| `active.timeout.ms` | `1800000` | Active Timeout（毫秒） |
| `parallelism` | `12` | 并行度 |
| `watermark.delay.ms` | `10000` | Watermark 延迟（毫秒） |

### Kafka 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `kafka.brokers` | `10.0.5.8:9092` | Kafka 集群地址 |
| `input.topic` | `flow.events.v1` | 输入 Topic |
| `output.topic` | `session.events.v1` | 输出 Topic |
| `consumer.group` | `flink-session-job` | 消费者分组 ID |
| `kafka.max.poll.records` | `500` | 单次拉取记录数 |

### ClickHouse 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `clickhouse.url` | `jdbc:clickhouse://10.0.5.8:8123/traffic` | JDBC URL |
| `clickhouse.table` | `sessions_local` | 目标表名 |
| `clickhouse.batch.size` | `10000` | 批量大小 |
| `clickhouse.batch.interval.ms` | `5000` | 批量间隔（毫秒） |
| `clickhouse.max.retries` | `3` | 最大重试次数 |
| `clickhouse.timeout.ms` | `30000` | 超时时间（毫秒） |

### OpenSearch 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `opensearch.enabled` | `false` | 是否启用 OpenSearch |
| `opensearch.hosts` | `10.0.5.8` | OpenSearch 节点地址 |
| `opensearch.index` | `sessions_v1` | 索引名称 |
| `opensearch.batch.size` | `1000` | 批量大小 |

---

## 监控与告警

### 1. Prometheus Metrics

作业暴露以下关键指标：

#### 作业健康指标
- `flink_jobmanager_job_uptime` - 作业运行时长
- `flink_jobmanager_job_numRestarts` - 作业重启次数
- `flink_jobmanager_job_numberOfCompletedCheckpoints` - 成功的 Checkpoint 数量
- `flink_jobmanager_job_numberOfFailedCheckpoints` - 失败的 Checkpoint 数量
- `flink_jobmanager_job_lastCheckpointDuration` - 最近一次 Checkpoint 耗时

#### 处理性能指标
- `flink_taskmanager_job_task_operator_numRecordsIn` - 输入记录数
- `flink_taskmanager_job_task_operator_numRecordsOut` - 输出记录数
- `flink_taskmanager_job_task_operator_currentInputWatermark` - 当前输入 Watermark
- `flink_taskmanager_job_task_operator_currentOutputWatermark` - 当前输出 Watermark

#### 业务指标
- `flink_taskmanager_job_task_operator_session_job_session_emitted_total` - Session 输出总数
- `flink_taskmanager_job_task_operator_session_job_session_bytes_total` - Session 总字节数
- `flink_taskmanager_job_task_operator_session_job_late_flow_total` - Late Data 数量
- `flink_taskmanager_job_task_operator_session_job_idle_timeout_total` - Idle Timeout 次数
- `flink_taskmanager_job_task_operator_session_job_active_timeout_total` - Active Timeout 次数

#### ClickHouse Sink 指标
- `flink_taskmanager_job_task_operator_clickhouse_sink_insert_success_total` - 成功写入数
- `flink_taskmanager_job_task_operator_clickhouse_sink_insert_fail_total` - 失败写入数
- `flink_taskmanager_job_task_operator_clickhouse_sink_insert_retry_total` - 重试次数
- `flink_taskmanager_job_task_operator_clickhouse_sink_batch_flush_total` - Batch Flush 次数

### 2. Grafana Dashboard

访问 http://localhost:3000（默认用户名/密码：admin/admin）

导入的 Dashboard 包含以下面板：
- **作业健康状态**: 运行时长、重启次数、TaskManager 数量
- **Checkpoint 监控**: 耗时、大小、成功/失败率
- **处理性能**: 数据处理速率、Watermark Lag
- **业务指标**: Session 输出速率、Timeout 统计
- **Sink 性能**: ClickHouse/OpenSearch 写入速率与失败率

### 3. 告警规则

已配置的关键告警（见 `deploy/prometheus/alerts.yml`）：

| 告警名称 | 触发条件 | 级别 |
|----------|----------|------|
| `FlinkJobNotRunning` | 作业停止超过 2 分钟 | Critical |
| `FlinkCheckpointFailureRateHigh` | Checkpoint 失败率 > 10% | Warning |
| `FlinkTaskManagerMemoryHigh` | 堆内存使用率 > 85% | Warning |
| `SessionProcessingLagHigh` | Watermark Lag > 60 秒 | Warning |
| `ClickHouseInsertFailureRateHigh` | 写入失败率 > 5% | Critical |
| `KafkaConsumerLagHigh` | Consumer Lag > 100 万 | Warning |

---

## 故障排查

### 1. 作业频繁重启

**症状**: `flink_jobmanager_job_numRestarts` 持续增长

**可能原因**:
1. ClickHouse 连接超时
2. Kafka Consumer Lag 过高导致 OOM
3. State 恢复失败

**排查步骤**:
```bash
# 查看 TaskManager 日志
docker-compose logs -f taskmanager | grep -i error

# 检查 ClickHouse 连接
docker exec -it flink-session-clickhouse clickhouse-client --query "SELECT 1"

# 检查 Kafka Lag
docker exec -it flink-session-kafka kafka-consumer-groups \
  --bootstrap-server localhost:9092 \
  --group flink-session-job \
  --describe
```

**解决方案**:
- 增加 `clickhouse.timeout.ms` 到 60000
- 增加 TaskManager 内存到 16GB
- 减少 `kafka.max.poll.records` 到 200

### 2. ClickHouse 写入失败率高

**症状**: `clickhouse_sink_insert_fail_total` 增长快

**可能原因**:
1. ClickHouse 磁盘满
2. 网络抖动
3. 批量大小过大导致超时

**排查步骤**:
```bash
# 检查 ClickHouse 磁盘空间
docker exec -it flink-session-clickhouse df -h

# 检查 ClickHouse 日志
docker-compose logs -f clickhouse | grep -i error

# 查看 DLQ Topic 数据
docker exec -it flink-session-kafka kafka-console-consumer \
  --bootstrap-server localhost:9092 \
  --topic dlq.session.ch.v1 \
  --from-beginning \
  --max-messages 10
```

**解决方案**:
- 减少 `clickhouse.batch.size` 到 5000
- 增加 `clickhouse.max.retries` 到 5
- 清理 ClickHouse 过期数据

### 3. Watermark Lag 过高

**症状**: 处理延迟持续超过 60 秒

**可能原因**:
1. TaskManager 资源不足
2. 下游 Sink 写入慢
3. Late Data 过多

**排查步骤**:
```bash
# 查看 CPU 使用率
docker stats --no-stream

# 查看 Late Data 数量
curl -s http://localhost:9250/metrics | grep late_flow_total

# 查看 Checkpoint 耗时
curl -s http://localhost:9090/api/v1/query?query=flink_jobmanager_job_lastCheckpointDuration
```

**解决方案**:
- 增加并行度到 16
- 增加 TaskManager 副本到 4
- 调整 `watermark.delay.ms` 到 30000

---

## 性能优化

### 1. 吞吐量优化

**目标**: 达到 10 万 Flow/s → 5 万 Session/s

**优化方案**:
1. **并行度调优**
   ```properties
   parallelism=16
   max.parallelism=128
   ```

2. **Kafka 消费优化**
   ```properties
   kafka.fetch.min.bytes=524288
   kafka.max.poll.records=1000
   kafka.max.partition.fetch.bytes=2097152
   ```

3. **ClickHouse 批量优化**
   ```properties
   clickhouse.batch.size=20000
   clickhouse.batch.interval.ms=3000
   clickhouse.thread.pool.size=8
   ```

### 2. 内存优化

**问题**: TaskManager OOM

**优化方案**:
1. **增加 TaskManager 内存**
   ```yaml
   taskmanager.memory.process.size: 16384m
   taskmanager.memory.managed.fraction: 0.5
   ```

2. **启用 RocksDB 增量 Checkpoint**
   ```yaml
   state.backend.incremental: true
   state.backend.rocksdb.ttl.compaction.filter.enabled: true
   ```

3. **减少状态 TTL**
   ```properties
   state.ttl.ms=900000  # 15 分钟
   ```

### 3. Checkpoint 优化

**问题**: Checkpoint 耗时过长（> 5 分钟）

**优化方案**:
1. **启用 Unaligned Checkpoint**
   ```yaml
   execution.checkpointing.unaligned: true
   ```

2. **增加 Checkpoint 间隔**
   ```yaml
   execution.checkpointing.interval: 120000  # 2 分钟
   ```

3. **使用本地恢复**
   ```yaml
   state.backend.local-recovery: true
   ```

---

## 运维脚本

### 1. 作业重启脚本

```bash
#!/bin/bash
# restart-job.sh

JOB_ID=$(curl -s http://localhost:8081/jobs | jq -r '.jobs[0].id')

echo "Stopping job: $JOB_ID"
curl -X PATCH http://localhost:8081/jobs/$JOB_ID?mode=cancel

sleep 10

echo "Restarting job from latest checkpoint"
docker exec -it flink-session-jobmanager \
  flink run -s /opt/flink/checkpoints/latest \
  -c com.traffic.flink.session.SessionJob \
  /opt/flink/usrlib/flink-session-job.jar
```

### 2. 健康检查脚本

```bash
#!/bin/bash
# health-check.sh

JOBMANAGER_URL="http://localhost:8081"
UPTIME=$(curl -s $JOBMANAGER_URL/jobs | jq -r '.jobs[0].duration')

if [ "$UPTIME" -gt 0 ]; then
  echo "✅ Job is running (uptime: ${UPTIME}ms)"
  exit 0
else
  echo "❌ Job is not running"
  exit 1
fi
```

### 3. Metrics 导出脚本

```bash
#!/bin/bash
# export-metrics.sh

PROMETHEUS_URL="http://localhost:9090"
START_TIME=$(date -u -d '1 hour ago' +%s)
END_TIME=$(date -u +%s)

curl -G "$PROMETHEUS_URL/api/v1/query_range" \
  --data-urlencode "query=flink_taskmanager_job_task_operator_session_job_session_emitted_total" \
  --data-urlencode "start=$START_TIME" \
  --data-urlencode "end=$END_TIME" \
  --data-urlencode "step=60" \
  | jq . > metrics_$(date +%Y%m%d_%H%M%S).json
```

---

## 升级与回滚

### 升级流程

1. **构建新版本 JAR**
   ```bash
   mvn clean package -DskipTests
   ```

2. **创建 Savepoint**
   ```bash
   JOB_ID=$(curl -s http://localhost:8081/jobs | jq -r '.jobs[0].id')
   curl -X POST http://localhost:8081/jobs/$JOB_ID/savepoints \
     -H 'Content-Type: application/json' \
     -d '{"target-directory": "/opt/flink/savepoints", "cancel-job": false}'
   ```

3. **停止旧作业**
   ```bash
   curl -X PATCH http://localhost:8081/jobs/$JOB_ID?mode=cancel
   ```

4. **启动新作业（从 Savepoint 恢复）**
   ```bash
   docker exec -it flink-session-jobmanager \
     flink run -s /opt/flink/savepoints/savepoint-xxx \
     -c com.traffic.flink.session.SessionJob \
     /opt/flink/usrlib/flink-session-job-v2.jar
   ```

### 回滚流程

```bash
# 停止当前作业
JOB_ID=$(curl -s http://localhost:8081/jobs | jq -r '.jobs[0].id')
curl -X PATCH http://localhost:8081/jobs/$JOB_ID?mode=cancel

# 从最近的 Savepoint 恢复旧版本
docker exec -it flink-session-jobmanager \
  flink run -s /opt/flink/savepoints/savepoint-before-upgrade \
  -c com.traffic.flink.session.SessionJob \
  /opt/flink/usrlib/flink-session-job-v1.jar
```

---

## 常见问题 (FAQ)

### Q1: 为什么选择 `process` 模式而不是 `window` 模式？

**A**: `process` 模式支持 Active Timeout（30 分钟强制切分），而 `window` 模式只支持 Idle Timeout。对于长连接场景（如 SSH、数据库连接），`process` 模式可以避免单个 Session 无限膨胀。

### Q2: Late Data 会丢失吗？

**A**: 不会。Late Data 会输出到 `session.late.v1` Topic，可以通过批处理作业重新处理。

### Q3: ClickHouse 写入失败的数据如何恢复？

**A**: 失败的数据会写入 `dlq.session.ch.v1` Topic，可以通过以下脚本重新导入：

```bash
# 消费 DLQ 数据并重新写入 ClickHouse
flink run -c com.traffic.flink.session.DlqReprocessJob \
  /opt/flink/usrlib/flink-session-dlq-reprocess.jar \
  --input.topic dlq.session.ch.v1
```

### Q4: 如何调整 Session Gap（Idle Timeout）？

**A**: 修改配置参数：
```properties
session.gap.ms=180000  # 改为 3 分钟
```

重启作业后生效。

### Q5: 如何查看某个 IP 的 Session 详情？

**A**: 通过 ClickHouse 查询：
```sql
SELECT 
    session_id,
    ts_start,
    ts_end,
    duration_ms,
    packets_total,
    bytes_total,
    end_reason
FROM traffic.sessions_dist
WHERE client_ip = '192.168.1.100'
  AND ts_start >= now() - INTERVAL 1 HOUR
ORDER BY ts_start DESC
LIMIT 100;
```

---

## 附录

### A. 目录结构

```
flink-session-job/
├── src/
│   ├── main/
│   │   ├── java/com/traffic/flink/session/
│   │   │   ├── SessionJob.java
│   │   │   ├── SessionJobConfig.java
│   │   │   ├── aggregator/
│   │   │   ├── processor/
│   │   │   ├── sink/
│   │   │   └── state/
│   │   └── resources/
│   │       ├── session-job.properties
│   │       └── log4j2.xml
│   └── test/
├── deploy/
│   ├── clickhouse/
│   │   └── init.sql
│   ├── prometheus/
│   │   ├── prometheus.yml
│   │   └── alerts.yml
│   ├── alertmanager/
│   │   └── alertmanager.yml
│   └── grafana/
│       ├── provisioning/
│       └── dashboards/
├── docker/
│   └── healthcheck.sh
├── Dockerfile
├── docker-compose.yml
├── pom.xml
└── README.md
```

### B. 相关文档

- [Flink 官方文档](https://nightlies.apache.org/flink/flink-docs-release-1.17/)
- [ClickHouse 文档](https://clickhouse.com/docs/)
- [Kafka 文档](https://kafka.apache.org/documentation/)
- [Prometheus 告警规则最佳实践](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/)

### C. 技术支持

- **作者**: Traffic Analysis Platform Team
- **邮箱**: support@traffic-platform.local
- **项目地址**: https://github.com/1144160159/traffic-analysis-platform

---

**最后更新**: 2024-01-15
**版本**: v1.0.0
