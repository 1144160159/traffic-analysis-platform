# T1-M09-N012 告警证据关联写入与投影回滚

适用对象：`RB-T1-M09-P026-WRT-n012-s1` 的 link/unlink 权威写入与 outbox，以及 `RB-T1-M09-P027-PRJ-n012-s2` 的 Kafka consumer 和 PostgreSQL/ClickHouse 投影。该能力是 additive expand，部署默认值仍为：

- `ALERT_EVIDENCE_LINK_CONSUMER_V1_ENABLED=false`
- `ALERT_EVIDENCE_LINK_DISPATCHER_V1_ENABLED=false`
- `ALERT_EVIDENCE_LINK_WRITER_V1_ENABLED=false`
- `ALERT_EVIDENCE_LINK_CANDIDATE_SHA256` 为空

## 停止条件

出现跨租户读写、manifest 与 relation 的对象 bucket/key/version/SHA-256 不一致仍被接受、旧 relation 或 manifest revision 被接受、同一幂等键得到不同结果、同一 event ID 对应不同 payload、Kafka ACK 前 outbox 被标记 published、投影坐标与实际 topic/partition/offset 不同，或 consumer readiness 丢失后 writer 仍接单时，立即停止扩大流量。

## 写入侧回滚

1. 首先设置 `ALERT_EVIDENCE_LINK_WRITER_V1_ENABLED=false`，停止新的 link/unlink 命令。路由继续存在并返回明确的 not-ready，不得退回伪成功。
2. 保持 dispatcher 与 consumer 可观察，等待已经提交的 pending/processing outbox 获得真实 broker ACK；不能手工把记录改成 published，也不能跳过 dead/retry 审计。
3. writer 关闭后再设置 `ALERT_EVIDENCE_LINK_DISPATCHER_V1_ENABLED=false`。保存每条未完成 outbox 的 tenant、event ID、relation revision、request SHA、payload SHA 和最后错误。
4. 不删除或覆盖 `alert_evidence_links`、history、commands、outbox、manifest/history 和 audit。unlink 是追加 revision，不是证据对象删除权限。

## 投影侧回滚

1. writer 与 dispatcher 已关闭且 outbox 状态已核对后，设置 `ALERT_EVIDENCE_LINK_CONSUMER_V1_ENABLED=false`。
2. 保存 consumer group 的最后已确认 offset，以及 PostgreSQL inbox/delivery/watermark 和 ClickHouse 行的同一 event/revision/topic/partition/offset/payload SHA 对账结果。
3. 不回退 Kafka offset，不删除 inbox 或历史行来制造“干净”状态；未完成投影的事件在相同 candidate SHA 下重放。相同 event+payload 复用，event 相同但 payload 不同必须继续 fail closed。
4. ClickHouse 投影是派生事实；回滚期间读路径不得把它提升为 PostgreSQL relation/manifest 权威，也不得依据投影删除 MinIO 或其他证据对象。

## 迁移与重新启用

PostgreSQL `202608160030` 和 ClickHouse `202608160040` 均为 expand-only。回滚不执行 DROP，不降低 revision，不改写 immutable object identity，也不移动 Kafka 已确认坐标。

重新启用必须使用同一批准 candidate SHA，并严格按 consumer → dispatcher → writer 顺序：先证明 schema、topic/group assignment 和投影可用，再证明 outbox 真实 ACK，最后才开放写命令。至少重跑同对象同摘要幂等、不同摘要冲突、旧 relation/manifest revision 拒绝、重复 event 精确复用、header/key/坐标不一致拒绝和隔离 K8s 全链路。任一阶段失败都按上述逆序关闭；本回滚手册不授权生产发布、共享数据迁移或证据对象删除。
