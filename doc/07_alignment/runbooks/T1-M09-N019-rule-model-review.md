# T1-M09-N019 规则/模型发布复核

## 当前结论

实现状态为 `PARTIAL`。部署预检查、独立审批、灰度启动和全量激活现在可以通过 `DEPLOYMENT_RUNTIME_ACK_GATE_V1_ENABLED` 连接到现有规则 ACK、模型 ACK 和部署事件投影权威表。该开关默认关闭；关闭时保持旧接口与旧状态机行为，不能据此宣告运行时已经完成。

最新 Kubernetes 验收 run 为 `384d9a95-68dd-4eee-a9f5-81fc3a9c2d32`，证据位于 `doc/02_acceptance/topic1/tasks/t1-m09-n019/k8s-rule-model-review-latest.json`。该 run 在节点 `8-2tb` 上创建临时 PostgreSQL 16，并使用统一 Web 候选 `traffic/web-ui:m09-n022-alert-detail-css-20260816-r2`，依次证明部分 ACK 阻断、精确 ACK 放行、审批后 event 漂移要求重审、缺少灰度投影阻断全量激活、投影 ACK 到达后放行以及旧部署版本仍可查询；Go 集成、Go 单元和 Web bundle 三个 Job 全部成功，数据库清理 oracle 全零，run-scoped 资源已删除。

## 代码执行链

1. `DeploymentService.UpdateDeploymentWorkflow` 的 deploy precheck 从 deployment 的不可变 `rule_version` / `model_version` 反查最新 outbox 事件和逐 subtask ACK，不接受浏览器指定 event ID。
2. 完整收据被写入 workflow configuration 并参与审批快照 hash；申请人和审批人仍由原有两人审批合同隔离。
3. `StartGrayDeployment` 在持有部署事务锁后重新读取运行收据，只有 live event 与审批 event 相同且精确 ACK 集完整时才进入 gray。
4. `ActivateDeployment` 禁止启用门禁后的 planned→active 直达，并额外要求 `gray_started` outbox 已 broker published 且 `deployment_event_projection` 具有精确 Kafka partition/offset。
5. `GetDeploymentWorkbench` 和 `ExportDeploymentEvidence` 返回同一 `runtime_gate`；页面展示规则、模型、灰度投影的 event、ACK 数和 Kafka 位点，并在灰度态阻断“继续灰度”。

## 候选启用顺序

1. 保持 `DEPLOYMENT_RUNTIME_ACK_GATE_V1_ENABLED=false`，先完成规则更新 consumer、模型更新 consumer 和 deployment projection consumer 的候选部署。
2. 对待发布 rule/model 版本核对 outbox 的 broker published 状态、期望 parallelism、subtask `0..N-1` 精确 ACK 以及失败/陈旧收据；不能手工补 ready 结论。
3. 在授权的单租户、单 release line canary 中将门禁改为 `true`，重新运行 deploy precheck 并由不同身份审批。
4. 启动灰度后等待 `gray_started` 的 broker projection receipt，再允许扩大至全量；任何 event ID 变化都必须重新预检查和审批。
5. 导出证据并对照 deployment history、audit、outbox、规则/模型 ACK 和 Kafka 位点，完成观察窗口后才能讨论扩大范围。

## 停止与回滚

任一规则或模型收据为 `missing`、`pending`、`partial`、`failed`，subtask 范围不完整，consumer parallelism 不符，审批后 event 漂移，或灰度投影缺失时立即停止扩展。关闭门禁只恢复兼容行为，不会删除历史、审批快照、outbox 或 ACK，也不能把未完成目标解释为已恢复。

部署回滚目标仍通过同租户、同 release line 和可恢复状态校验并保留旧 `rule_version` / `model_version`。但正式回滚必须先调用各自专用的规则/模型回滚命令，等待新 rollback event 的完整 ACK，再改变 deployment active pointer；当前 DeploymentService 尚未编排这两个专用服务，因此该项仍是明确的未关闭边界，禁止仅凭 PostgreSQL 状态切换宣告运行时回滚完成。

## 未关闭项

生产开关授权、真实 broker 生成的规则/模型 ACK 联合演练、跨服务新鲜回滚 ACK 编排、规模性能、Kafka/PG 故障注入、正式回滚演练、Windows Chrome 验收以及生产观察窗口尚未完成。本证据只支持 N019 的 run-scoped K8s ACK 扩展门候选，不支持 M09 或项目整体完成。
