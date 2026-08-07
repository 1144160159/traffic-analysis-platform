# F-ALERT-001 告警报告回滚

适用范围：`alert_report_jobs_v1` 候选的创建、状态查询、对象下载和后台生成器。

## 触发条件

- manifest 与 MinIO 对象的 size 或 SHA-256 不一致；
- 报告跨租户可见、快照 revision 不一致或缺失章节被伪造为完整；
- 队列持续增长、重试超过 5 次或资源预算越界；
- 候选 bundle 与服务端契约版本不匹配。

## 停止扩大

1. 将候选 tenant 停留在当前集合，不继续扩大灰度。
2. 将 `ALERT_REPORT_JOBS_V1_ENABLED=false`，发布同一候选镜像的配置回滚。
3. 确认新建报告端点返回 404，既有告警读取和处置路由不受影响。
4. 保留 `alert_report_jobs`、`alert_report_outbox` 和 `report-artifacts`；禁止删除失败对象或审计记录。

## 在途任务

- `accepted` 任务保留等待恢复；
- `running` 任务通过 5 分钟 lease 回收，最多重试 5 次；
- 已上传但未提交 manifest 的同名对象允许确定性覆盖；
- 已完成对象仍按 manifest 校验后下载，若功能开关关闭则暂时不可下载。

## 恢复与验证

重新开启前，至少验证 PG job/outbox/audit 原子性、MinIO size/SHA-256、跨租户负向用例、
一次 lease 过期恢复，以及 Windows Chrome 的创建—轮询—下载 HAR。回滚本身不回退 migration，
也不删除对象；恢复时只切换功能开关。
