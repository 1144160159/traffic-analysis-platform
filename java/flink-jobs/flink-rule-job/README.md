# Flink Rule Job - 动态规则引擎（增强版）

## 概述

Flink Rule Job 是园区网全流量采集分析系统的核心检测组件，使用 **Broadcast State Pattern** 实现规则热更新，支持多种检测规则类型。

## 新增功能（v2.0）

### 1. ✅ IP 字段提取
- **问题**：原版本 MatchContext 中 `srcIp`/`dstIp` 未填充，导致黑名单检测失效
- **解决方案**：
  - 新增 `CommunityIdParser` 工具类
  - 从 `FeatureStat.objectId` 解析五元组（格式：`192.168.1.1:443-10.0.0.1:52345`）
  - 解析失败时记录 `ip_extraction_failed` 指标

### 2. ✅ BruteForceMatcher（暴力破解检测）
- **检测特征**：
  - 高频连接尝试（高 PPS）
  - 大量 SYN 包
  - 短持续时间（快速尝试）
  - 特定端口（SSH/RDP/FTP/SMTP/POP3/IMAP/MSSQL/MySQL/PostgreSQL）
- **配置示例**：见 `sample-rules/brute-force-rule.json`

### 3. ✅ 规则优先级排序
- **功能**：规则按 `priority` 字段降序排序后执行
- **用途**：控制多条规则同时匹配时的执行顺序

### 4. ✅ 规则命中统计（按规则维度）
- **指标**：`rule.<rule_id>.hit_count`
- **用途**：评估规则质量，发现高频误报规则

### 5. ✅ 规则更新审计日志
- **格式**：`[RULE_AUDIT] Rule updated: ruleId=..., tenantId=..., version=...→..., updatedBy=...`
- **用途**：安全审计与合规

### 6. ✅ 规则解析失败 DLQ
- **功能**：规则 JSON 解析失败时，将原始消息与错误信息写入 `dlq.rule-job` Topic
- **DLQ 记录格式**：
  ```json
  {
    "error_code": "RULE_PARSE_FAILED",
    "error_message": "...",
    "raw_event": "...",
    "timestamp": 1234567890
  }
7. ✅ Watermark 配置
规则流：使用 forMonotonousTimestamps()（规则按时间顺序到达）
特征流：使用 forBoundedOutOfOrderness(10s)（容忍 10 秒乱序）
架构设计
数据流
text

Feature Stream (Kafka: feature.stat.v1)
    ↓
Filter Invalid Features
    ↓
KeyBy(tenant_id) ───┐
                    ├→ RuleBroadcastProcessFunction
Rule Updates ───────┘       ↓
(Kafka: rule.updates)   Detection Results
                            ↓
                    ┌───────┴───────┐
                    ↓               ↓
            ClickHouse Sink    Kafka Sink
         (detections_behavior) (detections.v1)
支持的规则类型
Rule Type	Matcher	检测逻辑	状态
THRESHOLD	ThresholdMatcher	PPS/BPS 阈值检测	✅
BLACKLIST	BlacklistMatcher	IP 黑名单（BloomFilter 优化）	✅
PORT_SCAN	PortScanMatcher	端口扫描（高 PPS + 小包 + 大量 SYN）	✅
BRUTE_FORCE	BruteForceMatcher	暴力破解（高频认证尝试）	✅ 新增
DATA_EXFIL	DataExfilMatcher	数据外泄（高上行流量）	✅
ANOMALY	AnomalyMatcher	异常流量（统计偏差）	✅
DGA	-	DGA 域名检测	⏳ 待实现（应由 flink-behavior-job 负责）
TUNNEL	-	隐蔽隧道检测	⏳ 待实现（应由 flink-behavior-job 负责）
C2	-	C2 通信检测	⏳ 待实现（应由 flink-behavior-job 负责）
配置参数
rule-job.properties
properties

# Kafka 配置
kafka.brokers=localhost:9092
kafka.feature.topic=feature.stat.v1
kafka.rule.topic=rule.updates
kafka.output.topic=detections.v1
kafka.dlq.topic=dlq.rule-job
kafka.group.id=flink-rule-job

# ClickHouse 配置
clickhouse.url=localhost:8123
clickhouse.database=traffic
clickhouse.table=detections_behavior_local
clickhouse.user=default
clickhouse.password=

# Checkpoint 配置
checkpoint.path=file:///tmp/flink-checkpoints/rule-job
checkpoint.interval.ms=60000

# 并行度
parallelism=4

# 调试模式
debug.print=false
规则示例
Threshold Rule（阈值检测）
JSON

{
  "rule_id": "rule-threshold-001",
  "tenant_id": "tenant-1",
  "name": "High PPS Detection",
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
Blacklist Rule（黑名单）
JSON

{
  "rule_id": "rule-blacklist-001",
  "tenant_id": "tenant-1",
  "name": "Known Malicious IPs",
  "type": "blacklist",
  "enabled": true,
  "severity": "critical",
  "priority": 100,
  "conditions": {
    "ip_list": ["192.168.100.100", "10.0.0.99"],
    "direction": "both"
  },
  "labels": ["malware", "c2"],
  "version": 1,
  "action": "update"
}
Brute Force Rule（暴力破解）
JSON

{
  "rule_id": "rule-bruteforce-001",
  "tenant_id": "tenant-1",
  "name": "SSH Brute Force Detection",
  "type": "brute_force",
  "enabled": true,
  "severity": "critical",
  "priority": 95,
  "conditions": {
    "min_pps": 50,
    "min_syn_cnt": 20,
    "max_duration_ms": 60000,
    "target_ports": "22,3389,21",
    "min_conditions": 3
  },
  "labels": ["brute-force", "ssh"],
  "version": 1,
  "action": "update"
}
Metrics 指标
基础指标
features_processed_total - 处理的特征总数
rules_matched_total - 触发的规则总数
rules_updated_total - 更新的规则总数
rules_deleted_total - 删除的规则总数
ip_extraction_failed_total - IP 提取失败次数
active_rule_count - 当前活跃规则数
last_match_time_ms - 最近一次匹配耗时（毫秒）
规则维度指标
rule.<rule_id>.hit_count - 每条规则的命中次数
运维指南
查看 DLQ 中的失败规则
Bash

kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 \
  --topic dlq.rule-job \
  --from-beginning
监控规则命中率
Bash

# 查询 ClickHouse
SELECT 
    rule_id,
    count() AS hit_count,
    uniq(community_id) AS unique_sessions
FROM detections_behavior_local
WHERE ts > now() - INTERVAL 1 HOUR
GROUP BY rule_id
ORDER BY hit_count DESC
LIMIT 10;
故障排查
问题 1：黑名单检测不生效
原因：objectId 解析失败，无法提取 IP
排查：

Bash

# 查看 ip_extraction_failed 指标
curl http://flink-jobmanager:8081/jobs/<job-id>/metrics?get=ip_extraction_failed_total
解决：检查 FeatureStat.objectId 格式是否为 srcIP:srcPort-dstIP:dstPort

问题 2：规则更新未生效
原因：规则版本号冲突或解析失败
排查：

Bash

# 查看审计日志
grep "RULE_AUDIT" flink-taskmanager.log

# 查看 DLQ
kafka-console-consumer.sh --topic dlq.rule-job
问题 3：规则匹配延迟高
原因：规则数量过多或优先级排序开销大
排查：

Bash

# 查看 last_match_time_ms 指标
curl http://flink-jobmanager:8081/jobs/<job-id>/metrics?get=last_match_time_ms
优化：

减少低优先级规则数量
禁用不必要的规则
提高并行度
开发指南
添加新 Matcher
创建 Matcher 类（实现 RuleMatcher 接口）
在 MatcherFactory.initialize() 中注册
添加单元测试
更新 README
示例：

Java

public class MyCustomMatcher implements RuleMatcher {
    @Override
    public Optional<DetectionResult> match(FeatureStat feature, Rule rule, MatchContext context) {
        // 检测逻辑
        return Optional.empty();
    }
}
运行测试
Bash

mvn test -pl flink-rule-job
本地调试
Bash

# 启动 Flink 本地集群
./bin/start-cluster.sh

# 提交作业
./bin/flink run \
  -c com.traffic.flink.rule.RuleJob \
  flink-rule-job/target/flink-rule-job-1.0-SNAPSHOT.jar
已知限制
IP 提取依赖 objectId 格式

若 objectId 不包含五元组，黑名单检测将失效
推荐解决方案：在 Flink Session Job 中将五元组写入 objectId，或使用 Redis 缓存映射
DGA/Tunnel/C2 检测未实现

这些检测依赖 L2/L3 特征（序列特征、指纹特征）
建议由 flink-behavior-job 负责
规则优先级仅影响执行顺序

不支持"高优先级规则触发后跳过低优先级规则"
若需要此功能，需在 RuleBroadcastProcessFunction 中添加 break 逻辑
License
Apache License 2.0
