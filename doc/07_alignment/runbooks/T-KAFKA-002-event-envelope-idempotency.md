# T-KAFKA-002 事件封装与幂等迁移

状态：`IMPLEMENTING`；`production_applied=false`；覆盖范围：`PARTIAL_DETECTION_VERTICAL_SLICE`。

本候选只证明 Session → Feature → Detection → Alert 纵向切片的追加合同和离线行为，不证明全部 canonical topic 已迁移，也不证明 live Kafka 重放、顺序、DLQ、offset 或跨存储对账通过。

## 不变量

- `EventHeader` 只追加字段，不删除或重编号旧字段。
- v1 envelope 必须携带 `event_id/event_type/schema_version/tenant_id/aggregate_type/aggregate_id/aggregate_version/occurred_at/produced_at/trace_id/causation_id/correlation_id/idempotency_key/producer`。
- Kafka key 使用需保序的业务实体；Detection 使用 `tenant_id:community_id`，不得使用随机 UUID 作为分区键。
- 同一源事件重放必须产生相同 behavior `event_id` 和 tenant-scoped `alert_id`。
- Probe Flow 根事件使用租户、探针、run、community、标准五元组和时间窗派生 UUIDv5；不得在同一流窗口重放时重新生成随机 flow/event ID。
- 五元组与 evidence 引用来自源 payload；禁止硬编码空地址、0 端口/协议或从空集合制造“完整告警”。
- envelope 或五元组不完整时 alert consumer 必须 fail closed，由 T-KAFKA-003 的可靠 DLQ/提交屏障处理。

## Expand 与切换

1. 发布追加 Proto，并用仓库脚本重建 Go、Java、Rust 客户端；执行 breaking/lint 门禁。
2. 先部署 Feature producer，再部署 Behavior producer；确认新消息带完整 envelope、tuple 和 flow/evidence 引用。
3. 冻结一组源 Session，记录源 event ID、community ID、tuple、flow IDs 和 Kafka offset。
4. 对同一集合至少重放两次，对比 behavior event ID、Kafka key、alert ID、tuple、evidence IDs；集合差异必须为零。
5. 排空或通过版本 topic 隔离旧消息后，再为内部 tenant 启用严格 Alert consumer。不得让旧消息在切换时静默丢失。
6. 注入重复、乱序、空 envelope、空 tuple、毒消息和 DLQ ACK 失败，证明失败时 offset 不越过未持久化消息。

## 观测与停止扩大

保存 topic/partition/offset、event ID、aggregate version、trace、alert ID、投影水位和审计引用。若出现同源多 alert ID、空网络身份、跨 tenant、offset 越过失败消息、DLQ 不可确认、lag/P99 或资源越界，立即停止扩大。

观察窗为 `T+0/T+1/T+3/T+7`。低频检测还需覆盖一个完整业务周期。

## 回滚

停止严格 consumer 的新 canary，恢复上一版 consumer 镜像；保留追加 Proto 字段和已提交原始事件。不得重写 event ID、删除 DLQ 或回退权威告警。待旧消息排空/隔离并修复后，以相同 event ID 重放，再进行 offset、alert、evidence 和审计对账。

## 尚未通过

全部 topic envelope inventory、批准的 registry 兼容策略、真实 Kafka 双次重放、重复/乱序/DLQ 故障、offset 与跨存储对账、容量/隐私预算、灰度回滚观察和独立签字仍为 G2—G7 门禁。
