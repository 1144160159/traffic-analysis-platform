# UI Suite Business Reasonability Audit

更新日期：2026-06-28

本文记录基于 `screens/` 已落盘图片的业务合理性复核、发现的不合理点和本轮修复结果。复核依据为 `agent.md`、`doc/01_design/面向园区网络的全流量采集分析系统-UI前端规范.md`、`doc/01_design/面向园区网络的全流量采集分析系统-二级菜单功能点与表现形式矩阵.md`、`manifest.json`、`UI_IMAGEGEN_METHOD.md` 和 `CONTEXT_HANDOFF.md`。

## 复核范围

- 已生成 contact sheet：pages、overlays、components、states、responsive。
- 逐类检查重点：业务对象是否明确、异常状态是否给出正确动作、权限和审计是否符合安全平台语义、响应式图是否能指导前端实现。
- 本轮实际返工：`screens/states/` 16 张、`screens/responsive/` 12 张，共 28 张。
- 2026-06-28 重新生成候选复核：追加检查 P7 18 张 overlay contact sheet，未发现需要重新生成的业务错误；当前重新生成清单为 0 张。

## 发现与修复

| 编号 | 范围 | 不合理性 | 修复结果 |
|---|---|---|---|
| R1 | `screens/states/*.png` | 加载、空态、错误、未登录、无权限、降级、任务状态被画成接近同一套通用错误模板，容易把等待、无数据、权限不足、系统故障混成同一业务含义。 | 已重绘 16 张状态图。每张拆出触发场景、影响范围、复用规则、可执行动作和审计标识；加载态不再出现业务重试，空态区分筛选/时间窗/权限，错误态区分 API 与网络。 |
| R2 | `state-unauthorized` / `state-forbidden` | 401 与 403 原先容易被理解成同一“锁定”状态；未登录和无权限的用户动作边界不清，可能误导用户用普通重试解决权限问题。 | 已重绘并补充二次修正：401 只指向重新认证、复制返回链接、登录帮助；403 只指向申请权限、返回上一页、查看审计、联系管理员。中心视觉改为 `401` / `403` 状态码。 |
| R3 | `state-stream-backpressure`、`state-offline-probe`、`state-partial-degraded` | 平台运行状态若只提示刷新/重试，会掩盖 Kafka/Flink/Probe/ClickHouse 等采集链路治理动作。 | 已重绘为运行治理语义：背压展示 lag、水位、checkpoint 与限流/扩容动作；探针离线展示心跳、链路、证书和恢复动作；部分降级展示可用能力与影响范围。 |
| R4 | `screens/responsive/*.png` | 12 张响应式图过于像空白布局策略板，页面差异弱，无法指导 dashboard、告警、取证、图谱、合规、移动端等不同业务的折叠优先级。 | 已重绘 12 张响应式图。每张都绑定具体页面和断点，明确保留模块、折叠区域、抽屉/侧栏位置、危险动作入口和上下文传递方式。 |
| R5 | pages / overlays / components | 页面、浮层和组件 contact sheet 中业务内容整体可用；此前已处理 AppShell 重复职责、资产详情父容器、专题页内 Tab 合并等问题。 | 本轮不做批量重绘，仅将它们作为业务语义参考。后续若确认 P7 扩容，再单独进入新增浮层队列。 |
| R6 | P7 overlay 扩容 18 张 | 用户确认追加后生成的专题、战役/取证导出、合规、审计、通知和设置浮层需要补做业务复核，避免只完成尺寸和落盘校验。 | 已基于 contact sheet 复核，18 张均匹配各自交互类型和业务动作；权限、影响范围、审计留痕、导出/吊销/静默/RBAC 等闭环语义可用。本轮无需重新生成。 |

## 已修复图片

状态图：

- `state-page-loading`、`state-table-loading`、`state-chart-loading`
- `state-empty-page`、`state-empty-table`、`state-empty-chart`
- `state-api-error`、`state-network-error`、`state-unauthorized`、`state-forbidden`
- `state-partial-degraded`、`state-offline-probe`、`state-stream-backpressure`
- `state-task-running`、`state-task-failed`、`state-success-accepted`

响应式图：

- `responsive-dashboard-1440`、`responsive-dashboard-1920`、`responsive-screen-4k`
- `responsive-alerts-1440`、`responsive-alerts-1920`
- `responsive-forensics-1440`、`responsive-graph-1440`、`responsive-compliance-1440`
- `responsive-tablet-dashboard`、`responsive-tablet-alert-detail`
- `responsive-mobile-navigation`、`responsive-mobile-alert-list`

P7 复核通过且无需重生的浮层：

- `modal-topic-save-view`、`drawer-topic-scope-edit`
- `modal-topic-report-export`、`modal-topic-evidence-package-export`
- `drawer-topic-subscription`、`dropdown-topic-share-favorite`
- `modal-campaign-report-export`、`modal-forensics-evidence-export`
- `drawer-compliance-gate-detail`、`modal-compliance-evidence-package-export`
- `modal-compliance-report-export`、`drawer-audit-operation-detail`
- `modal-audit-export`、`modal-notification-channel-edit`
- `modal-notification-template-preview-test`、`drawer-notification-silence-rule`
- `popconfirm-settings-token-revoke`、`drawer-settings-rbac-edit`

## 追溯文件

- 返工前备份：每张被修复图旁保留 `*.before-business-fix.png`。
- 返工后 raw：每张被修复图旁保留 `*.raw-deterministic.png`。
- 最终 PNG：仍覆盖 manifest 指向的 `screens/states/` 和 `screens/responsive/` 目标文件。

## 后续规则

- 状态图不得复用单一“错误/重试”模板；必须先区分加载、空态、API 错误、网络错误、认证、授权、降级、运行背压和任务状态。
- 401/403 不得出现同一主按钮；401 的主动作是重新认证，403 的主动作是申请权限或查看审计。
- 响应式图必须绑定页面与断点，说明保留的核心业务、折叠的次要区域、危险动作位置和上下文传递方式。
- 弹窗、抽屉、下拉和确认框仍可只带业务区域，不强制携带公共 AppShell。
