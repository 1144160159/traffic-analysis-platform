# T1-M09-N009 取证任务命令回滚

适用对象：`F-FORENSICS-001` 的 PCAP 任务创建、取消和重试命令。该候选没有修改现网 Deployment，默认值仍为：

- `FORENSICS_PIPELINE_V1_ENABLED=false`
- `FORENSICS_WORKER_COMPATIBLE_READY=false`

## 停止条件

出现跨租户可见性、同一幂等键产生两个任务、任务与 history/audit/outbox 数量不一致、旧 revision 被接受、重试改写冻结请求，或兼容 worker readiness 丢失时，停止扩大流量。

## 回滚步骤

1. 先将 `FORENSICS_PIPELINE_V1_ENABLED=false`，停止新的创建、取消和重试写入；不得只关闭 worker 而继续接单。
2. 保持 `FORENSICS_WORKER_COMPATIBLE_READY` 的实际值和 worker 运行状态可观察，不删除已经接受的任务。
3. 对已接受任务按其冻结的五元组、时间窗、probe/alert/case、权限快照、用途、保留策略和合同版本完成、失败或取消。
4. 核对每个已接受 revision 在 `forensics_task_history`、`forensics_task_outbox`、`forensics_task_requests` 和 `audit_logs` 中的同一 `event_id` 事实；未发布 outbox 保留待恢复，不手工标记 published。
5. API 路由继续存在并返回明确的 `FORENSICS_PIPELINE_NOT_READY`；不得退回伪成功或创建不带冻结请求的兼容任务。

## 数据处理

迁移 `202608031600` 是 expand-only。回滚不删除 `tasks`、history、outbox、request replay 或 audit 数据，不降低 revision，也不覆盖冻结的 `params`。只有在全部引用、保留策略和法务保全条件满足后，才可由后续治理任务执行归档。

## 重新启用

只有兼容 worker 的相同候选 readiness 已在 K8s 重新证明，且隔离 PostgreSQL 原子性、幂等 replay、跨租户拒绝、revision 冲突和 retry 请求不变性全部通过后，才能同时满足两个 admission 条件并进行受控 canary。
