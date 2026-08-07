# T-FLINK-004 Sink 协议、重放与对账运行手册

## 1. 范围和判定边界

本手册覆盖九个 canonical Flink 作业的外部 sink ACK、稳定键、重放标记和逐键对账。仓库静态验证通过只证明 G0/G1 护栏；没有真实 source offset、外部回执和逐键差异时，不得把 HTTP 2xx、总行数相同或 checkpoint 成功写成 G2/G3 通过。

版本化合同为 `contracts/flink/sink-reconciliation.v1.json`。合同逐作业列出当前 sink、对账键和未完成项；只要任一 `gaps` 非空，T-FLINK-004 保持 `IMPLEMENTING`。

## 2. 已落地的止血项

- PCAP JDBC statement builder 只绑定参数，不再把绑定成功计为 ClickHouse 插入成功；上游处理算子也不再暴露无法由真实 ACK 驱动的 ClickHouse success/failure 指标。
- 告警和设备日志 OpenSearch sink 使用稳定文档 ID，bulk item 在有界退避耗尽后抛错，使 checkpoint 不能越过缺失投影。
- Loki、Session raw-flow、Session OpenSearch 和用户异常 ClickHouse sink 均保留 checkpoint buffer，并在外部回执之后清空。
- 用户异常投影写入 `traffic.user_anomalies_v2`，以 `anomaly_id` 为稳定业务键、`event_version` 为替换版本、`replay_id` 区分回放；Schema 仅由 migration/初始化制品管理，作业启动不执行 DDL。

## 3. Expand 与灰度顺序

1. 对候选运行 `python3 scripts/alignment/check_migrations.py`、`python3 scripts/alignment/verify_flink_sink_reconciliation.py` 和对应 Maven 测试。
2. 在批准窗口应用 `deployments/clickhouse/migrations/202608031000_user_anomalies_v2.sql`，查询 `system.tables` 确认 local/distributed 两表及 engine。
3. 冻结旧作业 source offset、checkpoint/savepoint、镜像 digest 和配置 hash。禁止在没有 savepoint 的情况下直接替换带状态 UID。
4. 先启动隔离 replay/canary，显式传入唯一 `--replay.id <run_id>`；在线流量必须保持空 replay ID。
5. 按 `anomaly_id,event_version` 比较冻结输入、Kafka alert、ClickHouse `FINAL` 行和 DLQ；通过后才允许单作业灰度。
6. 回滚时停止候选、从冻结 savepoint 恢复旧镜像/配置，并保留 v2 表及 replay 行作为不可变证据，禁止删除已 ACK 数据来伪造一致。

## 4. 逐键对账

用户异常 ClickHouse 示例：

```sql
SELECT tenant_id, anomaly_id, event_version, replay_id, count() AS physical_rows
FROM traffic.user_anomalies_v2
WHERE detected_at >= {range_start:DateTime64(3)}
  AND detected_at < {range_end:DateTime64(3)}
GROUP BY tenant_id, anomaly_id, event_version, replay_id
HAVING physical_rows > 1;
```

对账制品至少包含：冻结 topic/partition/start/end offset，输入 event ID 或派生键清单，sink ACK/失败/DLQ 清单，ClickHouse/OS/Loki/MinIO 对应键与版本，`replay_id`，checkpoint/savepoint 路径和 hash。只比较 count 不合格。

## 5. 故障场景

必须分别注入 JDBC 整批失败、JDBC item receipt 不完整、OpenSearch bulk 部分失败、HTTP 非 2xx/超时、Kafka 重复与乱序、TaskManager 在 ACK 前后退出、checkpoint 恢复及回滚中在途批次。预期结果是未 ACK 数据保留或进入 durable DLQ；允许的重复必须由稳定键收敛，禁止丢失和伪成功。

## 6. 仍开放的门禁

- 九作业统一 `input/accepted/dropped/late/failed/DLQ/sink_success/last_watermark` 指标尚未全部实现。
- 所有可重放投影和 HTTP 请求尚未全部携带 `replay_id` 或等价幂等键。
- Feature、Rule、CEP、Behavior、Alert 等遗留 JDBC sink 尚未全部具备 checkpoint ACK buffer。
- 真实 Kafka offset、CH/OS/Loki/MinIO 逐键对账、性能、故障、灰度、回滚和观察窗尚未执行。

因此当前只允许记录“仓库侧止血和合同护栏通过，整体覆盖 PARTIAL”，不得提升为 G7/G8 完成。
