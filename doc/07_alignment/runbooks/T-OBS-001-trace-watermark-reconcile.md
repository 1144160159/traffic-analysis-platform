# T-OBS-001 统一 trace、水位与跨存储对账运行手册

## 1. 当前结论

本候选只形成 `PARTIAL` 的仓库侧纵切，不代表生产已应用或 T-OBS-001 已关闭。候选已统一 HTTP 与 Kafka 的 W3C trace 传播，并把告警 `trace_id` 写入 Proto、Go/Flink 告警模型、ClickHouse 和 OpenSearch 严格映射。资产域另有一个固定摘要、无持久卷、仅回环端口的 G1 七源样例：生产资产路径覆盖 PostgreSQL/outbox/Kafka/durable inbox/OpenSearch/NebulaGraph/审计，ClickHouse 使用生产告警 writer 绑定同一身份，MinIO 为明确标注的适配写入。该样例不包含 Flink，也不证明同哈希候选部署、生产 G3、性能、浏览器、回滚或观察。

仓库候选不得执行生产修复。`scripts/alignment/cross_store_reconcile.py` 仅生成确定性只读差异和修复计划；不会更新、删除、回放或隔离任何在线记录。

## 2. 不变量

- `traceparent` 中的合法W3C trace优先于 `X-Trace-ID`；非法输入必须重新生成。
- trace ID固定为32位小写十六进制；采集器不可用不得阻断传播。
- Kafka显式 `trace_id` 与上下文冲突时必须在发布前失败，不能形成分裂链路。
- 每次对账必须显式给出单一 `tenant_id`、单一数据域、权威源和最多10000条输入；禁止 `*`、`all`、`any`、`_all`。
- 额外投影只能进入 `quarantine_review_no_delete`，不得自动删除。
- 缺失、旧版本、哈希或trace不一致只形成待批准的有界重投影计划。
- 任一无法解析的记录或水位使结果保持 `PARTIAL` 并停止修复建议执行。

## 3. 标准输入

输入为一个JSON对象。每个source提供同一租户、同一数据域下的规范化记录与采集水位：

```json
{
  "schema_version": 1,
  "tenant_id": "tenant-a",
  "data_domain": "alerts",
  "authoritative_source": "postgresql",
  "sources": [{
    "source": "postgresql",
    "watermark": {
      "position_kind": "aggregate_version",
      "position": "7",
      "observed_at": "2026-08-04T13:00:00Z",
      "trace_id": "0123456789abcdef0123456789abcdef",
      "state": "complete"
    },
    "records": [{
      "record_id": "alert-a",
      "version": 7,
      "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "trace_id": "0123456789abcdef0123456789abcdef"
    }]
  }]
}
```

生产证据采集器必须从权威查询生成 `sha256`，不可由页面字段、mock或截图反推。PG需绑定业务revision/outbox状态，Kafka需绑定event ID和已提交offset，CH/OS/Nebula需绑定确定性投影ID/version，MinIO需绑定PG manifest和对象metadata/hash。

## 4. 只读计划

```bash
python3 scripts/alignment/cross_store_reconcile.py \
  --input /approved/read-only/normalized-alerts.json \
  --output /immutable/run/reconcile-report.json \
  --max-records 10000 \
  --mode plan
```

退出码：`0` 表示七源无差异；`3` 表示成功完成对账但存在差异或无法解析项；`2` 表示输入、范围或文件本身不合法。所有结果都必须保存输入hash、输出 `report_sha256`、查询时间窗、候选manifest和实际source watermarks。

## 5. 差异裁决

| 分类 | 含义 | 默认动作 |
|---|---|---|
| `missing` | 权威源存在，目标源缺失 | 审批后按ID有界重投影 |
| `extra` | 目标源存在，权威源不存在 | 隔离审查；禁止自动删除 |
| `stale_version` | 目标version与权威源不同 | 检查乱序/回放后有界重投影 |
| `hash_mismatch` | 同version内容hash不同 | 停止扩大并查Schema/序列化漂移 |
| `unparseable` | 记录或水位不符合合同 | 保持partial，修复采集合同 |
| `trace_mismatch` | 同一业务记录跨源trace不同 | 按完整性失败处理，禁止关闭 |

## 6. 迁移和灰度

1. 先在所有ClickHouse节点执行expand migration `202608041300_alert_trace_correlation_v1.sql`，核对四张alerts表均出现 `trace_id`。
2. 以hash固定候选启动内部tenant灰度，确认Go和Flink写入的Proto、CH、OS具有同一trace。
3. 对同一用户动作采集HTTP、PG/outbox、Kafka header/offset、Flink、CH、OS、Nebula、MinIO和审计证据。
4. 页面快照必须返回真实 `source_watermarks`，并能区分 `complete_empty` 与 `lagging/unavailable`。
5. 先运行plan；任何差异由独立QA/SRE裁决。修复适配器在本候选中尚不存在，不得手工把plan解释为已修复。

## 7. 停止与回滚

出现跨租户、hash/trace冲突扩大、无法解析、源水位倒退、Kafka/Flink不可恢复、CH/OS写入失败、MinIO manifest不一致或资源越界时立即停止灰度。回滚应用写路径至旧兼容版本；保留expand字段和已确认trace数据，不执行DROP。回滚后继续只读对账在途事件，直至水位稳定。

## 8. 关闭证据

关闭T-OBS-001至少需要：生产候选同一用户动作的十阶段同trace证据、所有关键页面真实水位与partial语义、七源全量/抽样对账、故障注入、固定规模P50/P95/P99、指定Windows Chrome、灰度/回滚，以及T+0、T+1、T+3、T+7观察。仓库测试或HTTP 2xx不能单独作为关闭依据。
