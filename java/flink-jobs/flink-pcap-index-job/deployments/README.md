# Flink PCAP Index Job - 部署与运维指南

## 📋 目录

- [快速开始](#快速开始)
- [目录结构](#目录结构)
- [本地开发](#本地开发)
- [生产部署](#生产部署)
- [监控与告警](#监控与告警)
- [故障排查](#故障排查)

## 🚀 快速开始

### 前置条件

- Docker 20.10+
- Docker Compose 2.0+
- Make（可选）

### 一键启动

```bash
# 进入 docker 目录
cd flink-jobs/flink-pcap-index-job/docker

# 启动所有服务
make up

# 或使用 docker-compose
docker-compose up -d
```

### 访问地址

| 服务 | 地址 | 说明 |
|------|------|------|
| Flink Web UI | http://localhost:8081 | 作业监控 |
| Grafana | http://localhost:3000 | 监控面板 (admin/admin) |
| Prometheus | http://localhost:9090 | 指标查询 |
| Kafka UI | http://localhost:8080 | Kafka 管理 |
| ClickHouse | http://localhost:8123 | 数据查询 |

## 📁 目录结构

```
docker/
├── Dockerfile                      # 多阶段构建镜像
├── docker-compose.yml              # 本地开发编排
├── Makefile                        # 常用命令
├── README.md                       # 本文档
├── init-scripts/
│   └── clickhouse/
│       └── 01-create-tables.sql    # ClickHouse 表初始化
├── monitoring/
│   ├── prometheus/
│   │   ├── prometheus.yml          # Prometheus 配置
│   │   └── rules/
│   │       └── pcap-index-alerts.yml  # 告警规则
│   ├── alertmanager/
│   │   └── alertmanager.yml        # Alertmanager 配置
│   └── grafana/
│       ├── provisioning/
│       │   ├── datasources/        # 数据源配置
│       │   └── dashboards/         # Dashboard 配置
│       └── dashboards/
│           └── pcap-index-overview.json  # Grafana Dashboard
└── scripts/
    ├── healthcheck.sh              # 健康检查脚本
    └── submit-job.sh               # 作业提交脚本
```

## 💻 本地开发

### 常用命令

```bash
# 构建镜像
make build

# 启动服务
make up

# 提交作业
make submit

# 查看日志
make logs-flink

# 健康检查
make health

# 查看服务状态
make status

# 停止服务
make down

# 清理数据
make clean
```

### 提交作业

```bash
# 使用 Makefile
make submit

# 或使用脚本
./scripts/submit-job.sh local
```

### 发送测试数据

```bash
# 发送测试消息
make test-produce

# 查看 DLQ 消息
make test-consume
```

### 调试

```bash
# 进入 Flink 容器
make shell-flink

# 进入 ClickHouse 容器
make shell-clickhouse

# 查看 Metrics
make metrics
```

## 🏭 生产部署

### 构建生产镜像

```bash
# 构建镜像
docker build -t traffic-platform/flink-pcap-index-job:v1.0.0 \
  -f docker/Dockerfile \
  ../../../..

# 推送到镜像仓库
docker push your-registry/flink-pcap-index-job:v1.0.0
```

### Kubernetes 部署

参考 `k8s/` 目录下的 Kubernetes 部署文件（如需要可单独生成）。

### 环境配置

| 环境 | Kafka | ClickHouse | 并行度 |
|------|-------|------------|--------|
| local | kafka:29092 | clickhouse:8123 | 2 |
| dev | 10.0.5.8:9092 | 10.0.5.8:8123 | 4 |
| staging | kafka-staging:9092 | ch-staging:8123 | 4 |
| prod | kafka-cluster:9092 | ch-cluster:8123 | 8 |

### 提交生产作业

```bash
./scripts/submit-job.sh prod
```

## 📊 监控与告警

### Grafana Dashboard

Dashboard 包含以下面板：

1. **作业概览**
   - 作业状态
   - 已处理总数
   - 无效/错误/DLQ 计数
   - 已索引字节数

2. **处理吞吐量**
   - 处理速率 (ops/s)
   - 索引字节速率 (bytes/s)

3. **数据质量**
   - 缺失字段速率
   - 数据有效率
   - 大文件/截断统计

4. **ClickHouse 写入**
   - 写入成功/失败速率
   - 写入成功率
   - DLQ 写入速率

5. **Flink 资源**
   - 内存使用率
   - GC 时间

6. **Checkpoint**
   - Checkpoint 耗时
   - 成功/失败统计

### 告警规则

| 告警名称 | 级别 | 触发条件 | 说明 |
|----------|------|----------|------|
| PcapIndexJobNotRunning | Critical | 作业停止 2 分钟 | 作业未运行 |
| PcapIndexJobFrequentRestarts | Warning | 30 分钟内重启 > 3 次 | 频繁重启 |
| PcapIndexHighInvalidRate | Warning | 无效率 > 5% | 数据质量问题 |
| PcapIndexDLQSpike | Warning | DLQ 速率 > 10/s | 大量无效数据 |
| PcapIndexClickHouseWriteFailed | Critical | 写入失败 | 存储问题 |
| PcapIndexHighErrorRate | Critical | 错误率 > 1% | 需立即排查 |

### 查看告警

```bash
# 查看当前告警
make alerts

# 或直接访问 Prometheus
curl http://localhost:9090/api/v1/alerts | jq
```

## 🔧 故障排查

### 常见问题

#### 1. 作业无法启动

```bash
# 检查 Flink 日志
make logs-flink

# 检查 TaskManager 可用 Slots
curl http://localhost:8081/overview | jq '.["slots-available"]'
```

#### 2. Kafka 连接失败

```bash
# 检查 Kafka 状态
make health

# 检查 Topic 是否存在
docker-compose exec kafka kafka-topics --bootstrap-server localhost:9092 --list
```

#### 3. ClickHouse 写入失败

```bash
# 检查 ClickHouse 连接
make shell-clickhouse

# 检查表是否存在
SELECT * FROM traffic.pcap_index_local LIMIT 1;
```

#### 4. 数据质量问题

```bash
# 查看 DLQ 消息
make test-consume

# 检查 Metrics
make metrics | grep -E "invalid|missing|error"
```

### 日志位置

| 组件 | 容器内路径 | 说明 |
|------|-----------|------|
| Flink | /var/log/flink/ | 作业日志 |
| ClickHouse | /var/log/clickhouse-server/ | 数据库日志 |
| Kafka | 标准输出 | docker logs 查看 |

### 性能调优

1. **增加并行度**
   ```bash
   --parallelism 4
   ```

2. **调整 Checkpoint 间隔**
   ```bash
   --checkpoint.interval.ms 60000
   ```

3. **增加 TaskManager 内存**
   ```yaml
   # docker-compose.yml
   environment:
     - FLINK_PROPERTIES=
       taskmanager.memory.process.size: 4096m
   ```

## 📞 联系方式

- 项目负责人: traffic-analysis-platform
- 问题反馈: GitHub Issues
