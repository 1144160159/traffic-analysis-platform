# Flink Alert Generator Job

将检测结果（DetectionBehavior + DetectionBusiness）转换为告警（Alert）和证据（Evidence）的 Flink 流处理作业。

## 核心功能

- **告警生成与去重**：基于指纹的去重聚合，支持 State TTL 自动清理
- **证据提取与关联**：自动生成结构化证据，关联 Arkime 会话链接
- **多存储写入**：同时写入 ClickHouse (主查)、OpenSearch (检索)、Kafka (事件流)
- **幂等保障**：At-least-once + event_id 去重

## 快速开始

### 1. 本地开发环境

```bash
# 启动所有服务
make start

# 或使用脚本
./scripts/start.sh up

# 查看服务状态
make status

# 查看日志
make logs

# 发送测试数据
make test-data

# 停止服务
make stop

# 清理所有数据
make clean
```

### 2. 访问地址

| 服务 | 地址 | 说明 |
|------|------|------|
| Flink Web UI | http://localhost:8081 | 作业监控 |
| Grafana | http://localhost:3000 | 监控面板 (admin/admin) |
| Prometheus | http://localhost:9090 | 指标查询 |
| Kafka UI | http://localhost:8080 | 消息查看 |
| ClickHouse | http://localhost:8123 | SQL 查询 |
| OpenSearch | http://localhost:9200 | REST API |

### 3. 生产环境部署

```bash
# 构建镜像
make docker-build

# 推送镜像
DOCKER_REGISTRY=your-registry.com make docker-push
```

## 配置说明

### 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `FLINK_PARALLELISM` | 4 | 作业并行度 |
| `SINK_PARALLELISM` | 2 | Sink 算子并行度 |
| `KAFKA_BROKERS` | localhost:9092 | Kafka 地址 |
| `KAFKA_INPUT_TOPIC_BEHAVIOR` | detections.behavior.v1 | 行为检测输入 |
| `KAFKA_INPUT_TOPIC_BUSINESS` | detections.business.v1 | 业务检测输入 |
| `KAFKA_OUTPUT_TOPIC` | alerts.v1 | 告警输出 |
| `CLICKHOUSE_URL` | localhost:8123 | ClickHouse 地址 |
| `CLICKHOUSE_DB` | traffic | 数据库名 |
| `OPENSEARCH_URL` | http://localhost:9200 | OpenSearch 地址 |
| `ARKIME_URL` | http://localhost:8005 | Arkime 地址 |
| `FLINK_CHECKPOINT_INTERVAL` | 60000 | Checkpoint 间隔 (ms) |
| `DEDUP_WINDOW_MINUTES` | 10 | 去重窗口 (分钟) |
| `SEVERITY_CRITICAL` | 0.9 | Critical 阈值 |
| `SEVERITY_HIGH` | 0.7 | High 阈值 |
| `SEVERITY_MEDIUM` | 0.5 | Medium 阈值 |
| `SEVERITY_LOW` | 0.3 | Low 阈值 |

## 监控与告警

### Prometheus 指标

业务指标：
- `alerts_generated_total` - 生成的告警总数
- `alerts_deduplicated_total` - 去重命中次数
- `alerts_updated_total` - 告警更新次数
- `evidences_generated_total` - 生成的证据总数
- `dedup_state_count` - 去重状态数量

### Grafana Dashboard

导入 `docker/grafana/dashboards/alert-generator-overview.json`：

1. 作业概览：状态、运行时长、Checkpoint
2. 业务指标：告警生成率、去重率
3. 性能指标：吞吐量、背压
4. 资源使用：CPU、内存、GC
5. Kafka 消费：延迟、消费速率
6. Sink 状态：错误率、写入速率

### 告警规则

关键告警（Critical）：
- `FlinkJobManagerDown` - JobManager 宕机
- `FlinkJobRestartLoop` - 作业频繁重启
- `KafkaConnectionError` - Kafka 连接错误
- `ClickHouseWriteError` - ClickHouse 写入失败
- `OpenSearchWriteError` - OpenSearch 写入失败

警告告警（Warning）：
- `FlinkCheckpointFailing` - Checkpoint 失败
- `FlinkHighBackpressure` - 高背压
- `FlinkHighHeapMemoryUsage` - 堆内存使用率高
- `KafkaConsumerLagHigh` - Kafka 消费延迟

## 数据流

```
┌─────────────────────────────────────────────────────────────┐
│                      Kafka Topics                           │
├─────────────────────────────────────────────────────────────┤
│  detections.behavior.v1   │  detections.business.v1        │
└──────────────┬─────────────┴──────────────┬─────────────────┘
               │                            │
               ▼                            ▼
        ┌──────────────┐            ┌──────────────┐
        │ Behavior     │            │ Business     │
        │ Source       │            │ Source       │
        └──────┬───────┘            └──────┬───────┘
               │                            │
               ▼                            ▼
        ┌──────────────┐            ┌──────────────┐
        │ Alert        │            │ Business     │
        │ Generator    │            │ Alert Gen    │
        │ (State TTL)  │            │ (State TTL)  │
        └──────┬───────┘            └──────┬───────┘
               │                            │
               └────────────┬───────────────┘
                            │
                            ▼
                    ┌───────────────┐
                    │ Union         │
                    └───────┬───────┘
                            │
                ┌───────────┼───────────┐
                ▼           ▼           ▼
         ┌──────────┐ ┌──────────┐ ┌──────────┐
         │ClickHouse│ │OpenSearch│ │  Kafka   │
         │  Sink    │ │  Sink    │ │  Sink    │
         └──────────┘ └──────────┘ └──────────┘
```

## 故障排查

### 作业频繁重启

```bash
# 查看 Flink 日志
make logs

# 检查 Checkpoint 状态
curl http://localhost:8081/jobs/<job-id>/checkpoints

# 检查内存使用
curl http://localhost:9249 | grep heap
```

### Kafka 消费延迟高

```bash
# 查看消费延迟
curl http://localhost:9249 | grep lag

# 查看算子背压
curl http://localhost:8081/jobs/<job-id>/vertices/<vertex-id>/backpressure
```

### ClickHouse 写入失败

```bash
# 测试 ClickHouse 连接
docker exec alert-job-clickhouse clickhouse-client --query "SELECT 1"

# 检查表结构
docker exec alert-job-clickhouse clickhouse-client --query "DESCRIBE traffic.alerts_local"
```

## 目录结构

```
flink-alert-generator-job/
├── Dockerfile                    # 多阶段构建
├── docker-compose.yml            # 本地开发环境
├── Makefile                      # 常用命令
├── pom.xml                       # Maven 配置
├── README.md                     # 本文档
├── docker/
│   ├── entrypoint.sh             # 容器入口点
│   ├── clickhouse/
│   │   ├── init.sql              # ClickHouse 初始化
│   │   └── config.xml            # ClickHouse 配置
│   ├── prometheus/
│   │   ├── prometheus.yml        # Prometheus 配置
│   │   └── alert_rules.yml       # 告警规则
│   └── grafana/
│       ├── provisioning/         # 自动配置
│       └── dashboards/           # Dashboard JSON
├── scripts/
│   ├── start.sh                  # 启动脚本
│   └── test-producer.sh          # 测试数据生产者
└── src/
    └── main/
        ├── java/                 # Java 源码
        └── resources/            # 配置文件
```

## 许可证

Apache License 2.0
