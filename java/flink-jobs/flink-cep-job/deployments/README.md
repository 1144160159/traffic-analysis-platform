# Flink CEP Job - Docker 部署指南

## 目录结构

```
docker/
├── Dockerfile                          # 生产镜像构建文件
├── docker-compose.yaml                 # 本地开发环境
├── docker-entrypoint.sh               # 容器入口脚本
├── flink-conf.yaml                    # Flink 配置文件
├── clickhouse-init/                   # ClickHouse 初始化脚本
│   └── 01-init-campaigns.sql
├── prometheus/                        # Prometheus 配置
│   ├── prometheus.yml
│   └── rules/
│       └── flink-cep-alerts.yml       # 告警规则
└── grafana/                           # Grafana 配置
    ├── provisioning/
    │   ├── datasources/
    │   │   └── datasources.yml
    │   └── dashboards/
    │       └── dashboards.yml
    └── dashboards/
        └── flink-cep-job-overview.json
```

## 快速开始

### 1. 构建镜像

```bash
# 在项目根目录执行
cd flink-jobs/flink-cep-job/docker
docker-compose build
```

### 2. 启动本地开发环境

```bash
# 启动所有服务
docker-compose up -d

# 查看日志
docker-compose logs -f flink-cep-job

# 查看各服务状态
docker-compose ps
```

### 3. 访问服务

| 服务 | 地址 | 说明 |
|------|------|------|
| Flink Web UI | http://localhost:8081 | 作业管理界面 |
| Grafana | http://localhost:3000 | 监控面板 (admin/admin) |
| Prometheus | http://localhost:9090 | 指标查询 |
| Kafka UI | http://localhost:8080 | Kafka 管理 |
| ClickHouse | localhost:8123 | HTTP 接口 |

### 4. 停止环境

```bash
# 停止并保留数据
docker-compose down

# 停止并清理数据
docker-compose down -v
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `FLINK_MODE` | standalone | 启动模式 (jobmanager/taskmanager/submit/standalone) |
| `JOB_MANAGER_RPC_ADDRESS` | jobmanager | JobManager 地址 |
| `JOB_MANAGER_MEMORY` | 2048m | JobManager 内存 |
| `TASK_MANAGER_MEMORY` | 4096m | TaskManager 内存 |
| `TASK_SLOTS` | 4 | 每个 TaskManager 的 Slot 数量 |
| `PARALLELISM` | 4 | 作业并行度 |
| `KAFKA_BROKERS` | kafka:9092 | Kafka 集群地址 |
| `KAFKA_INPUT_TOPIC` | alerts.v1 | 输入 Topic |
| `KAFKA_OUTPUT_TOPIC` | campaigns.v1 | 输出 Topic |
| `CLICKHOUSE_URL` | clickhouse:8123 | ClickHouse 地址 |
| `CLICKHOUSE_DATABASE` | traffic | 数据库名 |
| `CHECKPOINT_PATH` | file:///opt/flink/checkpoints | Checkpoint 路径 |

## 生产部署

### Kubernetes 部署

参考 `k8s/` 目录下的 Kubernetes 部署文件。

### 手动提交作业

```bash
# 进入 JobManager 容器
docker exec -it cep-flink-jobmanager bash

# 提交作业
/opt/flink/bin/flink run \
  -d \
  -c com.traffic.flink.cep.CepJob \
  /opt/flink/usrlib/flink-cep-job.jar \
  --kafka.brokers kafka:9092 \
  --kafka.input.topic alerts.v1 \
  --kafka.output.topic campaigns.v1 \
  --clickhouse.url clickhouse:8123 \
  --clickhouse.database traffic
```

### 创建 Savepoint

```bash
# 获取 Job ID
JOB_ID=$(curl -s http://localhost:8081/jobs | jq -r '.jobs[0].id')

# 创建 Savepoint
curl -X POST "http://localhost:8081/jobs/${JOB_ID}/savepoints" \
  -H "Content-Type: application/json" \
  -d '{"target-directory": "/opt/flink/savepoints", "cancel-job": false}'
```

### 从 Savepoint 恢复

```bash
/opt/flink/bin/flink run \
  -d \
  -s /opt/flink/savepoints/savepoint-xxx \
  -c com.traffic.flink.cep.CepJob \
  /opt/flink/usrlib/flink-cep-job.jar \
  --kafka.brokers kafka:9092
```

## 监控告警

### Prometheus 告警规则

告警规则定义在 `prometheus/rules/flink-cep-alerts.yml`，包括：

- **集群健康**: JobManager/TaskManager 可用性
- **作业状态**: 重启、失败检测
- **Checkpoint**: 失败、超时告警
- **背压与延迟**: Kafka 消费延迟、背压告警
- **资源使用**: JVM 内存、GC 告警
- **CEP 模式**: 匹配率、DLQ 增长告警

### Grafana Dashboard

Dashboard 提供以下面板：

1. **作业概览**: 状态、TaskManager 数量、Slots、重启次数
2. **吞吐量**: Records/s、Bytes/s
3. **CEP 模式匹配**: 各类型 Campaign 生成速率
4. **Checkpoint**: 耗时、大小趋势
5. **资源使用**: JVM 堆内存、GC 频率
6. **Kafka 延迟**: 消费 Lag

## 故障排查

### 查看作业日志

```bash
# JobManager 日志
docker logs cep-flink-jobmanager

# TaskManager 日志
docker logs cep-flink-taskmanager

# CEP Job 提交日志
docker logs cep-flink-job
```

### 检查 Kafka Topics

```bash
# 查看 Topic 列表
docker exec cep-kafka kafka-topics --list --bootstrap-server localhost:9092

# 查看 alerts.v1 消费情况
docker exec cep-kafka kafka-consumer-groups \
  --bootstrap-server localhost:9092 \
  --group flink-cep-job \
  --describe

# 查看 campaigns.v1 消息
docker exec cep-kafka kafka-console-consumer \
  --bootstrap-server localhost:9092 \
  --topic campaigns.v1 \
  --from-beginning \
  --max-messages 10
```

### 检查 ClickHouse 数据

```bash
# 连接 ClickHouse
docker exec -it cep-clickhouse clickhouse-client

# 查询 Campaign 数据
SELECT * FROM traffic.campaigns_local ORDER BY ts_end DESC LIMIT 10;

# 统计各类型 Campaign
SELECT campaign_type, count() FROM traffic.campaigns_local GROUP BY campaign_type;
```

## 性能调优

### 增加并行度

```yaml
# docker-compose.yaml
environment:
  PARALLELISM: 8
  TASK_SLOTS: 8
```

### 增加 TaskManager 内存

```yaml
# docker-compose.yaml
environment:
  TASK_MANAGER_MEMORY: 8192m
```

### 扩展 TaskManager 数量

```bash
docker-compose up -d --scale flink-taskmanager=3
```

### RocksDB 调优

编辑 `flink-conf.yaml`:

```yaml
state.backend.rocksdb.memory.fixed-per-slot: 1024mb
state.backend.rocksdb.checkpoint.transfer.thread.num: 8
```
