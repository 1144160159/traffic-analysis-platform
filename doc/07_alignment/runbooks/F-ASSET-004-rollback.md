# F-ASSET-004 资产导出与列偏好回滚

## 适用范围

用于资产导出 API、后台 worker、MinIO 制品或列偏好出现跨租户、清单不一致、资源越界、持续失败或错误 bundle 时的停止扩大与回滚。该操作只回退流量和执行能力，不删除 PostgreSQL 记录、审计、outbox 或 MinIO 对象。

## 立即停止条件

- 任意跨租户 job、偏好或对象访问。
- 对象 size/sha256 与 PostgreSQL manifest 不一致。
- 行数、对象大小、worker CPU/内存或数据库 P99 超过批准预算。
- completed 状态缺少对象，或下载未形成审计。
- `asset.exports.v1` 持续积压、重复扩大、租约无法回收或 `dead` 记录超过批准阈值。
- 浏览器 bundle 与候选 manifest 不一致。

## 回滚步骤

1. 将 `ASSET_EXPORT_JOBS_V1_ENABLED=false`，停止新增请求和偏好写入。
2. 将 `ASSET_EXPORT_WORKER_ENABLED=false`，等待当前租约到期，不强制终止正在上传的对象。
3. 将 `ASSET_EXPORT_OUTBOX_ENABLED=false`，停止新的 Kafka 投递；保留已经 broker ACK 的 `published` 记录，不回退 offset，不删除 `pending/processing/dead` 记录。
4. 保留 `asset_export_jobs`、`asset_export_outbox`、`asset_column_preferences`、`audit_logs` 与 `report-artifacts` 对象；禁止执行 destructive migration。
5. 按 `tenant_id/job_id` 对账 running/completed job、manifest、对象及 `asset.exports.v1` 事件；确定性对象键允许修复后重试覆盖同一对象，事件消费者必须按 event_id 幂等。
6. 前端 bundle 回退到上一批准候选，但不得恢复浏览器内存生成 CSV；功能关闭时显示服务端不可用状态。
7. 记录候选 hash、开关变更、未完成租约、outbox 积压/dead、孤儿对象、回滚验证与批准人。

## 恢复条件

- 在隔离租户完成 idempotency、跨租户、worker lease、outbox lease/retry/dead、真实 broker ACK/消费、manifest、下载审计和 preference revision 回归。
- 孤儿对象和未完成 job 已 reconcile，且差异为零或均有已批准处置。
- G0—G3 重新通过；扩大流量前重新执行 G4—G6。
