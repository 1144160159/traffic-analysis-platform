# T-OS-004 告警 OpenSearch 投影债务、重建与对账

状态：`PARTIAL`。候选代码与默认关闭护栏已实现；live PostgreSQL 已存在 expand-only migration，候选 worker、V2 OpenSearch target、repair 和开关仍未部署或执行。

## 权威边界

- ClickHouse `traffic.alerts_latest` 是告警事实权威源；OpenSearch 仅为可重建搜索投影。
- ClickHouse 成功、OpenSearch 失败不是最终成功。只有 PG `alert_opensearch_projection_debts` 已提交时，Kafka 消费者才可把该批次归类为 `projection_pending` 并继续 offset；PG 记债失败必须返回错误。
- OpenSearch `_id=alert_id`，写入使用 `version_type=external_gte` 和源时间毫秒版本，旧重放不能覆盖新投影。
- 权威字段哈希排除 `attack_phase`、`arkime_link`、`evidence_count`；它们不是 ClickHouse 权威列。

## 前置检查

1. 固定候选 commit、镜像 digest、配置 hash 和本次 `run_id`。
2. 审核并应用 expand-only migration `202608041100_alert_opensearch_projection_reconciliation_v1.sql`。验证 `alignment_schema_migrations.version='202608041100'`。
3. 保持 `OPENSEARCH_ALERT_PROJECTION_RECONCILE_V1_ENABLED=false`，先验证记债提交屏障。
4. 只读 plan 可用 `OPENSEARCH_LEGACY_READ_TARGET=<exact-index>` 绑定冻结的 legacy 实际索引；动态 legacy 映射使用 `.keyword` 过滤和排序。repair 只允许在 `OPENSEARCH_ALERTS_V2_ENABLED=true` 且目标为已批准的精确 V2 write alias 时执行，禁止向发现阶段的 legacy 目标回填。禁止使用 `latest`。
5. 记录 Kafka consumer group lag、最后提交 offset/事件 ID、PG 债务基线、OS 文档数、active search contexts、CPU/heap/磁盘水位。

## 只读 plan

CLI 必须指定租户、操作者、精确索引版本和数量上限；时间或业务 ID 至少选一项用于首轮小范围验证。

```bash
go run ./cmd/alert-projection-reconcile \
  --mode plan \
  --tenant <tenant_id> \
  --requested-by <operator_id> \
  --target-index-version <approved_version> \
  --start <RFC3339> --end <RFC3339> \
  --max-documents 1000
```

保存 source/target count、missing/extra/stale ID、字段 SHA、trace 与 `alert_opensearch_reconcile_runs`。任一侧超过上限时结果必须为 `partial/bounded_scope_truncated`，不得继续 repair。

### 2026-08-05 有界 legacy shadow 结果

- 固定 `default` tenant、1 秒时间片、`max_documents=100`，独立 oracle 均为 14 条且 14 个唯一 `alert_id`。
- 首次结果 `source=0,target=14,extra=14` 已判无效：`SELECT` 中 `AS last_seen` 覆盖了 `WHERE` 使用的原始毫秒列。投影别名现固定为 `first_seen_time/last_seen_time`，并由负向测试禁止回归。
- 第二次结果 `source=14,target=14,stale=14` 用于诊断 legacy `_source` 命名差异：旧索引保存 `dedup_fingerprint`，canonical Go 对象使用 `fingerprint`。
- legacy 读取器现仅在旧索引模式下把 `dedup_fingerprint` 适配为 `fingerprint`；V2 canonical 字段优先且不会被覆盖。最终结果为 `source=14,target=14,missing=0,extra=0,stale=0,partial=false`。
- 三次均为 `mode=plan`，只新增 PG 对账审计行，`repaired_count=0`，没有 CH/OS 写入、索引或 alias 变更。
- 最终候选完整 G0：`doc/02_acceptance/runs/20260805-remediation-g0-full-v127/manifest.json#sha256=3605bd977fd6603344d458017aa99634cfcf04f344066b877a5fb3a321079db0`。
- 不可变 shadow 证据：`doc/02_acceptance/runs/20260805-remediation-opensearch-shadow-reconcile-v1/manifest.json#sha256=865de0be7f71460fb755bd42d49d017ee991ec82c69ab9f1be5c68b6d0cb2757`。

该结果只关闭一个 14 文档 bounded slice 的查询真实性问题，不代表 V2 expand、全量 backfill、repair、性能、切换、回滚或观察完成。

### 2026-08-05 只读部署前检查

- 完整 G0 v130 在同一候选哈希 `6f00d4dc327e8af5832bdc75847797c7c1e41344f008573ddee171972a204b79` 上通过 alignment、full 和 Python 三个独立阶段：`doc/02_acceptance/runs/20260805-remediation-g0-full-v130/manifest.json#sha256=aac4955fcf62771805f09c951e7fec97886a3765e5c047f1f1bef0c6429c98e0`。
- v2 采集暴露 live alert-service 为 distroless 镜像，容器内没有 `sh`；同时 Service 的 `9093` 错误指向未监听的 Pod `9093`。失败包保留，不作为通过证据。
- 候选清单已把 metrics Service `9093` 映射到应用真实监听的 `8082`，Prometheus 注解也改为 `8082`，并移除 distroless 不可执行的 shell preStop。live Service 同步了端口映射且未重启 Pod；采集改走 Kubernetes Service Proxy，不依赖容器 shell。
- v4 只读证据确认 PG migration `202608041100` 和三张投影表存在；OpenSearch `alerts` 约 7,287 万文档、green、1 primary/1 replica；旧部署没有投影开关，因此新 post-commit 指标为空且 `candidate_applied=false`。指标采集只保留 `alert_consumer_lag*` 和 `alert_consumer_last_committed*`，13 个专项测试包含过滤防回归。
- 不可变证据：`doc/02_acceptance/runs/20260805-remediation-opensearch-projection-reconciliation-v4/manifest.json#sha256=75b826eab7c8a67e877f220841ceb28183f31d0ee21acfb00e454dad77443eee`；状态为 `PARTIAL`、`scoped_evidence_status=PASS`、`production_applied=false`。

该检查证明 migration 与 metrics 读取链路当前可见，不证明候选 worker 已部署，也不授权 V2 template、alias、backfill 或 repair。

## 受控 repair

1. 在内部 tenant 先执行 plan；核验无跨租户、无重复 ID、目标版本一致。
2. repair 需要额外 `--confirm-repair`。执行器只写 missing/stale，不自动删除 extra。
3. 默认最大 10,000 文档、100 docs/s、25 个错误即停止；实际值只能更保守，放宽需新批准。
4. 每个成功写入必须推进 PG watermark；失败保持 debt 或 run error，不得记为 repaired。
5. CLI 在受控写入后必须刷新精确 V2 write alias，并用相同 scope 回读。run manifest 同时保存修复前 `missing/extra/stale` 和修复后 remaining 清单；只有 remaining missing/stale 为零且 watermark error 为零才返回 `repair_converged=true`。remaining extra 继续人工裁决，绝不自动删除。
6. 对目标内容已经一致的记录，repair 仍批量比较 PG watermark 的 `source_version + source_sha256`。缺失或不一致的 receipt 必须补写并再次查询；因此“OS 已写入但首次 PG watermark 失败”的后续运行能够恢复，不能因本轮没有 missing/stale 就跳过水位并伪报收敛。
7. G1至少保留一次同一自有运行内的真实OpenSearch回读与真实PostgreSQL watermark回读。两个分离容器运行的PASS只能作为单组件诊断，不能替代这一跨服务终态回执；该G1仍不替代真实ClickHouse/Kafka或生产G3。

```bash
go run ./cmd/alert-projection-reconcile \
  --mode repair --confirm-repair \
  --tenant <tenant_id> --requested-by <operator_id> \
  --target-index-version <approved_version> \
  --alert-ids <id1,id2> --max-documents 100
```

repair 后用相同 scope 再跑 plan，要求 missing/stale 为零；extra 进入人工裁决清单。CLI 的 read target 与 write target 相互独立：legacy plan 读取冻结的精确索引，V2 repair 读取 read alias 并写 write alias；不得通过把两个字符串强行设为同一未批准索引来绕过迁移门禁。

## 债务 worker 灰度

- 仅在 migration、真实 CH 读取、OS alias、外部版本写、PG lease/retry/dead 测试通过后，把单个 canary deployment 的开关设为 true。
- worker 使用 `FOR UPDATE SKIP LOCKED`、45 秒 lease、指数退避和 8 次上限。目标版本不一致、CH 版本落后、租户/ID 不一致时停止该项并重试/转 dead。
- 监控 pending/processing/dead 债务、最老债务年龄、修复成功率、consumer lag、最后 event_id、水位差及 OS 429/5xx。

## 立即停止条件

- 跨租户记录、ID 重复或权威源与请求租户不一致；
- source/target 截断、目标索引版本不一致或 manifest 不完整；
- 数据丢失、missing/stale 持续扩大、silent loss 非零；
- 错误数达到阈值，或 P99、CPU、heap、磁盘、Kafka lag 超过批准预算；
- PG 债务/水位提交失败、OS partial bulk 无法归因、worker lease 无法收敛。

## 回滚

1. 将 reconcile 开关恢复为 false，停止新 claim；保留 PG debt/run/watermark，不删除证据。
2. 停止 repair CLI；外部版本写保证已成功的新投影可安全保留。
3. 如 candidate consumer 不稳定，回滚镜像；旧消费者在 OS 失败时没有安全记债能力，因此必须同时确保 OS 健康或暂停消费，不能把旧行为当作安全降级。
4. 用相同 scope 运行只读 plan，确认未引入新的 missing/stale；记录回滚候选、在途任务与 offset。

## 关闭证据

G2—G6 至少需要：真实 CH/PG/OS 记债与修复、Kafka offset/lag/最后 event_id、源/索引 count 与采样 ID/SHA、missing/extra/stale manifest、修复率与停止阈值、固定规模 P50/P95/P99、canary 回滚，以及 T+0/T+1/T+3/T+7 观察。当前代码测试和 HTTP 2xx 不能单独关闭 T-OS-004。
