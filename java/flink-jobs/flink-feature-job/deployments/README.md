# Flink Feature Extraction Job

## 概述

Flink Feature Extraction Job 是流量分析平台的核心组件，负责从 Session 事件中提取 L1 统计特征，支持实时特征计算、动态配置热更新和多租户隔离。

## 架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Flink Feature Job v3                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────┐    ┌──────────────────┐    ┌──────────────────┐   │
│  │   Kafka     │───▶│  FeatureProcess  │───▶│   ClickHouse     │   │
│  │   Source    │    │   Function V3    │    │   Sink           │   │
│  └─────────────┘    └──────────────────┘    └──────────────────┘   │
│         │                    │                        │              │
│         │           ┌────────┴────────┐              │              │
│         │           │                 │              │              │
│         │    ┌──────▼──────┐   ┌──────▼──────┐      │              │
│         │    │  L2 Trigger │   │    DLQ      │      │              │
│         │    │  Side Output│   │  Side Output│      │              │
│         │    └─────────────┘   └─────────────┘      │              │
│         │                                            │              │
│  ┌──────▼──────────────────────────────────────────▼──────────┐    │
│  │                    BroadcastState                           │    │
│  │  ┌─────────────────┐       ┌─────────────────┐             │    │
│  │  │ FeatureSetConfig│       │  TenantConfig   │             │    │
│  │  └─────────────────┘       └─────────────────┘             │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                          │                                          │
│                   ┌──────▼──────┐                                   │
│                   │  PostgreSQL │                                   │
│                   │  (Config)   │                                   │
│                   └─────────────┘                                   │
└─────────────────────────────────────────────────────────────────────┘
```

## 功能特性

### 核心功能
- ✅ L1 统计特征计算（v2.0 Schema，20 个扩展字段）
- ✅ 动态配置热更新（BroadcastState）
- ✅ 多租户隔离与优先级管理
- ✅ L2 候选触发机制
- ✅ Backpressure 检测与自动降级
- ✅ 完整的 Metrics 暴露

### 输入输出
| 类型 | Topic/表 | 格式 |
|------|----------|------|
| 输入 | `session.events.v1` | Protobuf |
| 输出 | `feature.stat.v1` | Protobuf |
| 输出 | `feature_stat_local` | ClickHouse |
| DLQ | `dlq.feature-job` | JSON |
| L2 触发 | `l2.trigger.v1` | Protobuf |

## 快速开始

### 前置条件
- Docker & Docker Compose
- JDK 11+
- Maven 3.6+

### 本地开发

```bash
# 1. 构建项目
make build

# 2. 启动本地环境（包含 Kafka, PostgreSQL, ClickHouse, Prometheus, Grafana）
make up

# 3. 查看日志
make logs-flink

# 4. 访问 UI
make flink-ui      # Flink Web UI: http://localhost:8081
make grafana       # Grafana: http://localhost:3000 (admin/admin)
make prometheus    # Prometheus: http://localhost:9090
```

### 提交作业

```bash
# 提交到集群
make submit

# 查看状态
make status

# 创建 Savepoint
make savepoint JOB_ID=<job-id>

# 停止作业
make stop JOB_ID=<job-id>
```

## 配置说明

### 核心配置 (`feature-job.properties`)

```properties
# Kafka 配置
kafka.brokers=localhost:9092
kafka.input.topic=session.events.v1
kafka.output.topic=feature.stat.v1
kafka.group.id=flink-feature-job

# PostgreSQL（配置源）
postgres.url=jdbc:postgresql://localhost:5432/traffic
postgres.user=postgres
config.poll.interval.ms=30000

# ClickHouse
clickhouse.url=localhost:8123
clickhouse.database=traffic
clickhouse.table=feature_stat_local

# Checkpoint
checkpoint.path=file:///data/flink/checkpoints
checkpoint.interval.ms=60000

# 并行度
parallelism=4
```

### 环境变量覆盖

| 环境变量 | 说明 | 默认值 |
|----------|------|--------|
| `KAFKA_BROKERS` | Kafka 地址 | localhost:9092 |
| `POSTGRES_URL` | PostgreSQL JDBC URL | - |
| `CLICKHOUSE_URL` | ClickHouse 地址 | localhost:8123 |
| `PARALLELISM` | 作业并行度 | 4 |
| `CHECKPOINT_PATH` | Checkpoint 存储路径 | file:///data/flink/checkpoints |

## 特征说明

### L1 统计特征 (FeatureStat)

| 字段 | 类型 | 说明 |
|------|------|------|
| `pps` | float | 包速率 (packets/sec) |
| `bps` | float | 比特率 (bits/sec) |
| `up_down_ratio` | float | 上下行比 |
| `pktlen_mean` | float | 平均包长 |
| `pktlen_std` | float | 包长标准差 |
| `iat_mean_ms` | float | 平均到达间隔 |
| `iat_std_ms` | float | 到达间隔标准差 |
| `active_mean_ms` | float | 平均活跃时间 |
| `idle_mean_ms` | float | 平均空闲时间 |

### Extra 字段映射（v2.0）

| 索引 | 字段名 | 说明 |
|------|--------|------|
| 0 | dns_pkt_ratio | DNS 包比例 |
| 1 | tcp_pkt_ratio | TCP 包比例 |
| 2 | udp_pkt_ratio | UDP 包比例 |
| 3 | icmp_pkt_ratio | ICMP 包比例 |
| 4 | std_payload | 载荷标准差 |
| 5 | min_payload | 最小包长 |
| 6 | max_payload | 最大包长 |
| 7 | avg_payload | 平均载荷 |
| 8 | min_iat_ms | 最小 IAT |
| 9 | max_iat_ms | 最大 IAT |
| 10 | iat_range_ms | IAT 范围 |
| 11 | is_established | TCP 是否建立 |
| 12 | end_reason_code | 会话结束原因 |
| 13 | evidence_count | 证据数量 |
| 14-19 | TCP Flags | FIN/PSH/RST 等 |

## 监控

### Prometheus Metrics

| Metric | 类型 | 说明 |
|--------|------|------|
| `feature_processed_total` | Counter | 处理成功数 |
| `feature_error_total` | Counter | 错误数 |
| `feature_skipped_total` | Counter | 跳过数 |
| `feature_l2_triggered_total` | Counter | L2 触发数 |
| `e2e_latency_ms` | Histogram | 端到端延迟 |
| `clickhouse_write_success_total` | Counter | CH 写入成功数 |

### Grafana Dashboard

预配置的 Dashboard 包含：
- 作业概览（状态、重启次数、Uptime）
- 处理性能（吞吐率、延迟分布）
- 业务指标（高 PPS、加密流量、L2 触发）
- Sink 状态（ClickHouse/Kafka 写入）
- Checkpoint 状态
- JVM 资源（内存、GC）

### 告警规则

| 告警 | 严重程度 | 条件 |
|------|----------|------|
| FlinkFeatureJobNotRunning | Critical | 作业未运行 > 2分钟 |
| FlinkCheckpointFailing | Warning | 10分钟内 > 3 次失败 |
| FlinkE2ELatencyHigh | Warning | P95 > 30秒 |
| FlinkBackpressureCritical | Critical | Backpressure > 800ms/s |
| FlinkClickHouseWriteFailure | Warning | 5分钟内 > 10 次失败 |

## 运维操作

### 扩缩容

```bash
# 修改 TaskManager 副本数
docker-compose up -d --scale taskmanager=4
```

### 从 Savepoint 恢复

```bash
# 设置 Savepoint 路径
export SAVEPOINT_PATH=/data/flink/savepoints/savepoint-xxx

# 重新提交
make submit
```

### 查看 DLQ

```bash
make consume-dlq
```

## 目录结构

```
flink-feature-job/
├── docker/
│   ├── Dockerfile              # 生产镜像
│   ├── flink-conf.yaml         # Flink 配置
│   ├── entrypoint.sh           # 启动脚本
│   ├── submit-job.sh           # 作业管理脚本
│   ├── init-postgres.sql       # PostgreSQL 初始化
│   └── init-clickhouse.sql     # ClickHouse 初始化
├── monitoring/
│   ├── prometheus/
│   │   ├── prometheus.yml      # Prometheus 配置
│   │   └── alerts/             # 告警规则
│   └── grafana/
│       ├── provisioning/       # 数据源配置
│       └── dashboards/         # Dashboard JSON
├── src/main/java/
│   └── com/traffic/flink/feature/
│       ├── FeatureJob.java             # 主入口
│       ├── calculator/                  # 特征计算
│       ├── config/                      # 配置类
│       ├── metrics/                     # Metrics
│       ├── processor/                   # 处理函数
│       ├── sink/                        # Sink 工厂
│       └── source/                      # 配置源
├── src/main/resources/
│   ├── feature-job.properties  # 配置文件
│   └── log4j2.xml              # 日志配置
├── docker-compose.yml          # 本地开发环境
├── Makefile                    # 构建脚本
└── README.md
```

## 版本历史

| 版本 | 日期 | 变更 |
|------|------|------|
| v3.0 | 2024-01 | 动态配置、L2 触发、多租户 |
| v2.0 | 2023-12 | Extra 字段扩展到 20 个 |
| v1.0 | 2023-11 | 初始版本 |

## 许可证

Traffic Analysis Platform - Internal Use Only
