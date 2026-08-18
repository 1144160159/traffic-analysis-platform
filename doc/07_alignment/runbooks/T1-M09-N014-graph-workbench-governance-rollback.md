# T1-M09-N014 图实体/路径查询治理回滚

适用范围：`GRAPH_WORKBENCH_V2_ENABLED` 控制的有界 NebulaGraph 工作台、opaque continuation、字段脱敏，以及 Web 图谱页的 continuation/服务端保存视图适配。

## 停止条件

- 任一响应超过租户节点、边或每跳邻居预算；
- continuation 可跨 tenant、跨筛选条件复用，或过期/篡改 token 被接受；
- 没有 `evidence:read` 的身份收到 evidence ID、对象键或原始载荷；
- 超级节点返回未显式 `truncated`，路径在循环中重复节点；
- 保存视图退回浏览器 `localStorage`，或页面自行补点、补线；
- 查询超时、NebulaGraph 错误或服务端保存失败被 UI 表示成完整成功。

## 回滚步骤

1. 保持或恢复 `GRAPH_WORKBENCH_V2_ENABLED=false`。该开关默认即为关闭，不需要删除 Secret。
2. 回滚 Graph Service 与 Web UI 到候选前镜像；不要回滚 M07 图投影、攻击链快照或其他 M09 API。
3. 保留 `GRAPH_WORKBENCH_CONTINUATION_SECRET`。旧 token 在旧实现中不可消费；重新启用时可轮换该 Secret，使全部候选 token 失效。
4. 本任务没有数据库迁移。不要删除 `alert_saved_views`、图投影表、NebulaGraph space、tag、edge type 或生产实体。
5. 若仅 Web 保存视图适配异常，可先回滚 Web；已提交的保存视图、history、audit 和 outbox 是业务证据，必须保留。

## 验证

- Graph Service readiness 恢复；旧 `/v1/graph/workbench` 与 `/v1/graph/workbench/path` 兼容行为可用；
- 配置摘要显示 `workbench_v2_enabled=false`，日志不包含 continuation Secret；
- 没有继续签发 governed continuation；
- 回滚没有删除任何图投影或服务端保存视图记录；
- K8s canary 资源按 `traffic.analysis/canary-run=<run-id>` 清理，N014 临时 tenant 的节点和边均为零。

## 证据边界

N014 K8s 证据只证明对应候选镜像在该次 run-scoped K8s/NebulaGraph 环境中的有界查询、负例和 UI bundle。它不授权生产切换，不证明 Windows Chrome 视觉验收，也不代表 M09 或整个项目完成。
