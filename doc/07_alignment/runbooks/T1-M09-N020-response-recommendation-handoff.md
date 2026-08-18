# T1-M09-N020 响应建议与外部交接

## 当前结论

实现状态为 `PARTIAL`。告警详情现在能够创建响应建议或 dry-run 任务，并通过既有独立审批、Kafka consumer 和外部 provider adapter 执行显式交接；权威查询接口及页面会显示 PostgreSQL 中的 provider receipt，而不是把 HTTP 受理或无回执状态解释为成功。

最新 Kubernetes 验收 run 为 `54207a03-d01d-4a26-8391-e3235d2dad7f`，证据位于 `doc/02_acceptance/topic1/tasks/t1-m09-n020/k8s-response-handoff-latest.json`。该 run 在节点 `8-2tb` 上创建临时 PostgreSQL 16，并以四个 Job 验证 API 工作流、consumer dry-run/默认阻断/真实 provider、失败语义单测和 Web 候选 bundle。数据库 oracle 记录 6 个 action、4 个 receipt、1 个模拟回执、1 个无 executor 阻断回执及 2 个 provider-confirmed 外部效果；资源已按 run-id 删除。

## 代码执行链

1. `Handler.CreateAlertResponseAction` 将 dry-run 直接写入 outbox；真实动作只写 `pending_approval`，不会在请求线程执行网络变更。
2. `Handler.DecideAlertResponseAction` 要求不同于申请人的审批身份和精确 revision；审批通过后才创建可发布 outbox。
3. `PostgresAlertResponseProjection.resolveExecutionOutcome` 对 dry-run 生成 `internal-simulation` 回执；无 executor 或审批权限不完整时写 `blocked_external_executor`，外部效果始终为 false。
4. 外部 adapter 使用 event 派生的幂等键；只有通过字段校验的 provider receipt 才能进入 completed。连接结果不确定时保持 partial/unknown，并进入权威查询流程。
5. `Handler.getAlertResponseExecutionReceipt` 以 tenant、alert、job 三元组读取回执，解析 result、effect IDs 和 authority lookup；损坏 JSON 使请求失败，不降级成伪成功。
6. 告警详情按 job 轮询 GET，显示 provider、provider receipt ID、effect state、effect IDs、Kafka 位点、摘要和执行时间；无回执时明确显示“等待执行回执”。

## K8s 候选验证

候选 Go 镜像为 `traffic/alert-response-handoff-test:m09-n020-20260816-r1`，Docker image ID 为 `sha256:cc290816679d418db1dff78d5601d99cf3d0a4541405e2cf0db7271d59bdacc9`。候选 Web 镜像为 `traffic/web-ui:m09-n022-alert-detail-css-20260816-r2`，Docker image ID 为 `sha256:6ec59641d1b271198c4a842218c79ff4c575d7c3f191375a4278d58dd0496b13`。两者均以 `imagePullPolicy=Never` 在本地 K8s 节点执行，证据保存 Pod UID、Job UID、container ID 和完成时间。

runner 使用 server-side apply，避免大 schema ConfigMap 写入 last-applied annotation；无论创建阶段是否部分失败，finally 都按 `traffic.analysis/canary-run` 删除 Job、Pod、Service、ConfigMap 和 Secret。

## 启用、停止与回滚

四个开关 `ALERT_RESPONSE_EXECUTION_V1_ENABLED`、`ALERT_RESPONSE_EXTERNAL_EXECUTOR_V1_ENABLED`、`ALERT_RESPONSE_UNKNOWN_EFFECT_RECONCILIATION_V1_ENABLED` 和 `ALERT_RESPONSE_COMPENSATION_EXECUTOR_V1_ENABLED` 在 K8s manifest 中均保持 false。生产启用必须先部署 additive schema 和 consumer，再配置候选 provider、authority lookup 与 compensation，最后只在获批 canary 中逐项打开。

出现缺少独立审批、provider 未配置、provider receipt 字段不完整、effect identity 冲突、Kafka 身份冲突或 authority lookup 不确定时立即停止扩展。关闭开关不会删除 action、approval、outbox、receipt、audit 或 compensation 证据；不能直接修改数据库终态来“修复”回执。

课题一只拥有检测派生的响应建议、dry-run、审计交接任务和 provider receipt 可见性，不拥有生产流量清洗、黑洞路由、攻击源直接阻断或 provider 网络策略实现。外部 provider 的真实效果仍需独立授权与其自身回滚合同。

## 未关闭项

生产 provider 授权、候选 executor 配置、真实故障注入与补偿演练、规模性能、正式回滚、Windows Chrome 验收和生产观察窗口尚未完成。本证据不支持启用生产开关，也不支持 N020、M09 或项目整体的生产完成声明。
