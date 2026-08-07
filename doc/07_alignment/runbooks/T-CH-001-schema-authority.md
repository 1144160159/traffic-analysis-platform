# T-CH-001 ClickHouse Schema Authority

## 当前结论

状态为 `IMPLEMENTING`。仓库已经建立有序 migration、逐节点执行器和 checksum 防篡改版本表，但 Kubernetes、Docker、common SQL 与各 Flink 组件仍存在 12 个遗留 DDL 入口。静态检查或只读 live inventory 均不得写成整改关闭。

## 唯一迁移入口

- 权威目录：`deployments/clickhouse/migrations/`
- 执行器：`scripts/clickhouse/run-migrations.sh`
- 执行模式：对每个 shard/replica 节点逐一执行，不使用 `ON CLUSTER`
- 版本事实：`traffic.alignment_schema_migrations_local`
- 不变量：同一 `version` 已登记后，候选文件 SHA-256 变化必须停止执行

执行器要求显式提供 `CLICKHOUSE_PASSWORD`，通过 `CLICKHOUSE_HOSTS` 传入节点清单。不得把密码写入日志、证据或 migration 文件。

## 只读 live baseline

```bash
python3 scripts/alignment/capture_clickhouse_live_schema.py \
  --run-id <immutable-run-id>
```

采集器只查询 `system.tables`、`system.columns`、`system.replicas` 与 `system.distributed_ddl_queue`，对每个运行中的 ClickHouse Pod 独立留存 JSONL、stderr、对象签名及 hash manifest。它不会执行 DDL、INSERT、mutation 或 migration。

## 切换顺序

1. 以 live `system.tables/system.columns` 为生产事实，按表登记 local/Distributed/MV、列、引擎、分区、排序、TTL、codec、索引、存储策略、写入者和读取者。
2. 对不兼容表建立 V2/兼容视图，执行 expand、受限 backfill、shadow read/write 和键级对账。
3. 依次切换 writer、reader，再将 Kubernetes、Docker 和组件 fixture 改为同一 migration runner。
4. 观察 T+0/T+1/T+3/T+7；任何数据丢失、副本只读、DDL 队列异常扩大、资源越界或键级差异扩大均停止扩容并回滚流量。
5. 只有独立验收完成分片/副本/MV、性能、故障与回滚证据后，才可申请关闭 T-CH-001。

## 当前未关闭项

- 遗留 12 个 DDL 入口尚未替换。
- 现有大表尚未形成以生产事实为基线的完整 migration。
- 未执行 migration、backfill、shadow、cutover 或 rollback。
- 未完成跨分片/副本数据和物化视图对账。
