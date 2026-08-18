# T1-M09-N016 告警报告异步任务

本任务把已有告警报告链路收敛到可执行的冻结快照、异步任务、版本化对象 manifest 和下载到期合同。候选运行开关 `ALERT_REPORT_JOBS_V1_ENABLED` 默认关闭，`ALERT_REPORT_ARTIFACT_TTL` 默认 `24h`，只接受 `5m` 至 `720h`。关闭候选时不影响告警读取、研判和处置，也不删除已存在的任务、history、outbox 或对象。

## 代码所有权

- `Handler.CreateAlertReport`：认证 tenant、`alert:export`、`Idempotency-Key`、alert revision 和冻结模型；在同一 PG 事务提交 job、history、outbox、audit 后返回 202。
- `defaultAlertReportBuilder.Build`：冻结 alert/evidence/assets/response/audit，记录 rule/model/state/update 水位；不可用章节进入 `missing_sections`，禁止补模拟数据。
- `Handler.processNextAlertReport`：以 `FOR UPDATE SKIP LOCKED` 领取 5 分钟 lease，最多重试 5 次；从已持久化 snapshot 生成确定性 JSON/PDF/DOCX。
- `minioAlertReportObjectStore.Put`：只写 `{tenant}/alerts/{alert}/{job}.{format}`；重试覆盖同一逻辑对象，不生成第二个 key。
- `Handler.writeAlertReportJobResponse`：返回 `manifest_version=1`、`object_format_version=1`、snapshot/artifact SHA-256、size/MIME、到期时间和 artifact 状态。
- `Handler.DownloadAlertReport`：到期前重新校验 size/SHA-256；到期后返回 `410 REPORT_EXPIRED`，PG 任务、manifest 和水位继续可查。
- `Handler.cleanupAlertReportObject`：取消只删除该任务 manifest 绑定的精确 key；删除失败进入可查询 retry/partial/compensation 状态，不宣告成功。
- `AlertDetailPage` / `alertDetailActionApi`：轮询权威任务状态，展示 manifest/水位/到期，不返回伪下载，不把过期等同于任务失败。

## K8s 证据

2026-08-16 的 run `346f8306-abef-46cd-9de2-84bd49c5ff37` 使用包含 N016/N017 测试的不可变镜像 `traffic/model-feedback-test:m09-n017-20260816-r5` 和同时包含 N016–N022 前端功能的 Web 镜像 `traffic/web-ui:m09-n022-alert-detail-css-20260816-r2`，在现有 `postgres-primary.databases.svc` 与 `minio.minio.svc:9000` 上创建一个 run-scoped tenant 和 bucket。四个 Job 分别验证真实集成、清理 oracle、目标单元测试和不可变 Web bundle，全部 PASS；tenant/job/history/outbox/audit 行、对象和 bucket 均已清除。

证据文件：`doc/02_acceptance/topic1/tasks/t1-m09-n016/k8s-alert-report-latest.json`。

该证据不证明生产发布、大报告队列性能、Kafka outbox 发布 ACK、Windows Chrome 交互、长期观察或全局里程碑完成。因此 `contracts/reporting/alert-report-job.v1.json` 的 closure 保持 `PARTIAL`，生产开关保持关闭。

## 回滚

按 `F-ALERT-001-rollback.md` 将 `ALERT_REPORT_JOBS_V1_ENABLED=false`。不得回退 migration、删除审计事实或批量删除对象。若只是 TTL 配置错误，先恢复上一个已验证 TTL，再核对存量任务 `artifact_expires_at` 与 410 行为；TTL 变更不是对象生命周期清理工具。
