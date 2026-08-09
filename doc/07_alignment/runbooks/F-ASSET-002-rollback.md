# F-ASSET-002 资产原子写入与投影回滚手册

## 适用边界

本手册覆盖资产 PostgreSQL expand、`asset.events.v2` outbox 发布，以及 OpenSearch/NebulaGraph 投影的灰度与回滚。仓库候选始终记录 `production_applied=false`；渲染成功、G0 PASS 或只读 canary PASS 都不表示已执行生产变更，也不满足 canonical 关单条件。

## expand 前置条件

1. 使用同一候选源码生成 PASS 的 G0 manifest，并记录其 candidate SHA-256 与 manifest SHA-256。
2. 由请求人与独立审批人确认 PostgreSQL `system_identifier`、十个 migration 的当前逐项状态、四小时内的执行窗口和回滚负责人。
3. 运行 `scripts/alignment/render_asset_postgres_expand.py` 生成不可变 ConfigMap、审批 Secret 和 `suspend: true` Job。禁止编辑已渲染文件，变化后必须使用新的 run ID 重新渲染。
4. 在解除 suspend 前复核 Job 注解、migration bundle hash、目标集群身份和当前 migration 状态。expand 仅增加对象，不删除旧表、字段、路由、游标、offset 或兼容 consumer。

## 默认关闭与逐项启用

基线必须保留 `ASSET_EVENT_OUTBOX_ENABLED=false`、`ASSET_DISCOVERY_OUTBOX_ENABLED=false`、`ASSET_PROJECTION_ENABLED=false`，以及 cursor、detail、discovery job、export 和 governance 的默认关闭状态。`ASSET_KAFKA_ENABLED=true` 只保留既有 `asset.bindings.v1` 兼容 consumer，不得把它当作新投影已切换的证据。

expand 完成后先执行 Kafka、OpenSearch alias 与 Nebula space 的只读 readiness canary，再按以下顺序逐项灰度：

1. `ASSET_EVENT_OUTBOX_ENABLED=true`，验证 PG outbox 到 Kafka broker ACK、积压和重放；
2. `ASSET_PROJECTION_ENABLED=true`，仅在专用 canary 实例和批准的 consumer group 上启用；
3. 验证 `asset_projection_inbox`、`asset_projection_watermarks`、OpenSearch external version 与 Nebula revision 对账；
4. 独立启用 discovery/export 等其余 worker，禁止一次性打开全部开关。

## 停止与回滚触发器

遇到 schema 校验失败、broker ACK 失败、offset 在 inbox 持久化前推进、投影版本回退、跨租户数据、alias 指向错误索引、Nebula VID 不确定、积压超过批准预算或错误率越界时立即停止切换。

1. 将 canary 或正式工作负载恢复为 `ASSET_PROJECTION_ENABLED=false`。
2. 将 outbox dispatcher 恢复为 `ASSET_EVENT_OUTBOX_ENABLED=false`；若 discovery dispatcher 已启用，同时恢复 `ASSET_DISCOVERY_OUTBOX_ENABLED=false`。
3. 保留 `asset_event_outbox`、`asset_projection_inbox`、`asset_projection_watermarks` 和失败记录，禁止删除或手工伪造完成水位。
4. OpenSearch 通过 write alias 回切到前一已批准索引；Nebula 停止 projector 后从 PostgreSQL/outbox 权威事实有界重放。
5. 若已发布 Kafka 事件，不撤销 offset；使用稳定 event identity、aggregate version 和投影水位执行幂等重放。

## 复核与观察

回滚或切换后保存同一 `tenant_id/trace_id/event_id/revision/snapshot_id/watermark` 的 PG、Kafka、OpenSearch、Nebula 与审计证据，并分别记录 T+0、T+1、T+3、T+7。只有独立 QA、SRE、安全和领域负责人完成 G2—G6 裁决后，相关 canonical 项才可进入观察或关闭；本手册本身不改变账本状态。
