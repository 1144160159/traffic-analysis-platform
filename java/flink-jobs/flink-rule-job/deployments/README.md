# Flink Rule Engine Job

## 概述

动态规则引擎，使用 Flink Broadcast State Pattern 实现规则热更新。

### 核心功能

- **规则类型**
  - `THRESHOLD`: 阈值规则（PPS/BPS/比率等）
  - `BLACKLIST`: IP 黑名单（使用 BloomFilter 优化）
  - `PORT_SCAN`: 端口扫描检测
  - `BRUTE_FORCE`: 暴力破解检测
  - `DATA_EXFIL`: 数据外泄检测
  - `ANOMALY`: 统计异常检测

- **架构特性**
  - 规则优先级排序
  - 规则版本控制（乐观锁）
  - 规则命中统计（按规则维度）
  - 规则更新审计日志
  - DLQ 容错（规则解析失败）

### 数据流

```
┌──────────────────┐      ┌──────────────────┐
│ feature.stat.v1  │      │  rule.updates    │
│    (Kafka)       │      │    (Kafka)       │
└────────┬─────────┘      └────────┬─────────┘
         │                         │
         │  ┌──────────────────────┘
         │  │ (Broadcast)
         ▼  ▼
    ┌─────────────────┐
    │ RuleBroadcast   │
    │ ProcessFunction │
    └────────┬────────┘
             │
     ┌───────┴───────┐
     ▼               ▼
┌──────────┐   ┌──────────┐
│detections│   │ClickHouse│
│   .v1    │   │          │
│ (Kafka)  │   │          │
└──────────┘   └──────────┘
```

## 构建与部署

### 本地构建

```bash
cd flink-jobs
mvn clean package -pl flink-rule-job -am
```

### Docker 构建

```bash
cd flink-jobs/flink-rule-job
docker build -t traffic-analysis/flink-rule-job:1.0.0 .
```

### Kubernetes 部署

```bash
# 创建命名空间
kubectl create namespace traffic-analysis

# 部署
kubectl apply -f k8s/deployment.yaml

# 查看状态
kubectl get pods -n traffic-analysis -l app=flink-rule-job

# 查看日志
kubectl logs -n traffic-analysis -l app=flink-rule-job,component=jobmanager -f

# 访问 Flink UI
kubectl port-forward -n traffic-analysis svc/flink-rule-job-jobmanager-rest 8081:8081
# 浏览器访问 http://localhost:8081
```

## 配置说明

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `KAFKA_BROKERS` | Kafka 地址 | `localhost:9092` |
| `CLICKHOUSE_PASSWORD` | ClickHouse 密码 | - |
| `PARALLELISM` | 并行度 | `4` |

### 核心配置项

```properties
# Kafka
kafka.brokers=localhost:9092
kafka.feature.topic=feature.stat.v1
kafka.rule.topic=rule.updates
kafka.output.topic=detections.v1
kafka.group.id=flink-rule-job

# ClickHouse
clickhouse.url=localhost:8123
clickhouse.database=traffic
clickhouse.table=detections_behavior_local

# Checkpoint
checkpoint.path=s3://flink-checkpoints/checkpoints/rule-job
checkpoint.interval.ms=60000
```

## 规则管理

### 规则 JSON 格式

```json
{
  "rule_id": "rule-001",
  "tenant_id": "tenant-1",
  "name": "High PPS Detection",
  "description": "Detect high packet rate",
  "type": "threshold",
  "enabled": true,
  "severity": "high",
  "priority": 80,
  "conditions": {
    "feature": "pps",
    "operator": ">",
    "value": 100000
  },
  "labels": ["ddos", "anomaly"],
  "version": 1,
  "action": "update"
}
```

### 规则操作

#### 创建/更新规则

```bash
# 发送规则到 Kafka
kafka-console-producer --bootstrap-server localhost:9092 \
  --topic rule.updates < sample-rules/threshold-rule.json
```

#### 删除规则

```json
{
  "rule_id": "rule-001",
  "tenant_id": "tenant-1",
  "action": "delete"
}
```

#### 禁用规则

```json
{
  "rule_id": "rule-001",
  "tenant_id": "tenant-1",
  "action": "disable"
}
```

## 规则类型详解

### 1. THRESHOLD（阈值规则）

```json
{
  "type": "threshold",
  "conditions": {
    "feature": "pps",
    "operator": ">",
    "value": 100000
  }
}
```

**支持的特征**:
- `pps`: 包速率
- `bps`: 比特率
- `up_down_ratio`: 上下行比
- `pktlen_mean`: 平均包长
- `duration_ms`: 持续时间

**支持的操作符**: `>`, `<`, `>=`, `<=`, `==`, `!=`

### 2. BLACKLIST（IP 黑名单）

```json
{
  "type": "blacklist",
  "conditions": {
    "ip_list": ["192.168.1.100", "10.0.0.50"],
    "direction": "both"
  }
}
```

**优化**: 使用 BloomFilter 快速过滤 + HashSet 精确匹配

### 3. PORT_SCAN（端口扫描）

```json
{
  "type": "port_scan",
  "conditions": {
    "min_pps": 100,
    "max_pkt_len": 100,
    "min_syn_cnt": 10,
    "max_duration_ms": 60000,
    "min_conditions": 3
  }
}
```

**检测特征**:
- 高 PPS
- 小包长
- 大量 SYN 包
- 短持续时间

### 4. BRUTE_FORCE（暴力破解）

```json
{
  "type": "brute_force",
  "conditions": {
    "min_pps": 50,
    "min_syn_cnt": 20,
    "target_ports": "22,3389,21",
    "max_duration_ms": 60000
  }
}
```

**检测端口**: SSH(22), RDP(3389), FTP(21), MySQL(3306), MSSQL(1433), PostgreSQL(5432)

### 5. DATA_EXFIL（数据外泄）

```json
{
  "type": "data_exfil",
  "conditions": {
    "min_bps": 10000000,
    "min_up_down_ratio": 10.0,
    "max_duration_ms": 300000
  }
}
```

**检测特征**:
- 高带宽
- 上传远大于下载
- 短时间大量传输

### 6. ANOMALY（统计异常）

```json
{
  "type": "anomaly",
  "conditions": {
    "pktlen_std_threshold": 500.0,
    "iat_std_threshold": 100.0,
    "extreme_up_down_ratio": 100.0,
    "min_anomalies": 2
  }
}
```

## 监控与指标

### Prometheus 指标

- `flink_rule_job_features_processed_total`: 处理的特征总数
- `flink_rule_job_rules_matched_total`: 规则匹配总数
- `flink_rule_job_rules_updated_total`: 规则更新总数
- `flink_rule_job_rule_{rule_id}_hit_count`: 单个规则命中数
- `flink_rule_job_active_rule_count`: 活跃规则数
- `flink_rule_job_last_match_time_ms`: 最后匹配耗时

### 审计日志

```
[RULE_AUDIT] Rule updated: ruleId=rule-001, tenantId=tenant-1, 
  version=1→2, type=THRESHOLD, enabled=true, updatedBy=admin
```

## 故障排查

### 规则未生效

1. 检查规则 JSON 格式是否正确
2. 查看 DLQ topic (`dlq.rule-job`)
3. 检查规则版本号（旧版本会被忽略）
4. 确认 `enabled=true`

### 性能问题

1. 检查规则数量（建议 < 1000）
2. 优化 BloomFilter 大小
3. 调整并行度（`parallelism`）
4. 检查 Checkpoint 间隔

### 内存溢出

1. 增加 TaskManager 内存（`taskmanager.memory.process.size`）
2. 调整 RocksDB 配置（`state.backend.rocksdb.memory.managed`）
3. 减少批次大小（`kafka.consumer.fetch.min.bytes`）

## 测试

### 单元测试

```bash
mvn test -pl flink-rule-job
```

### 集成测试

```bash
# 启动本地 Flink 集群
docker-compose up -d

# 运行 Job
flink run -c com.traffic.flink.rule.RuleJob \
  target/flink-rule-job-1.0.0.jar \
  --kafka.brokers localhost:9092
```

### 性能测试

```bash
# 使用 Kafka 生成测试数据
kafka-producer-perf-test --topic feature.stat.v1 \
  --num-records 1000000 \
  --record-size 1024 \
  --throughput 10000 \
  --producer-props bootstrap.servers=localhost:9092
```

## 最佳实践

1. **规则优先级**: 高优先级规则先执行（黑名单 > 暴力破解 > 端口扫描）
2. **版本控制**: 每次更新规则递增版本号
3. **灰度发布**: 先在测试租户测试规则
4. **监控告警**: 配置规则命中率告警
5. **定期清理**: 删除过期的规则

## 许可证

Internal Use Only
