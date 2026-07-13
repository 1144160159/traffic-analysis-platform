# 页面、浮层与基础组件缺口盘点

更新日期：2026-06-28

本文基于 `doc/` 产品文档、`manifest.json`、`CHAT_IMAGEGEN_INVENTORY.md`、`README.md`、`GENERATION_STATE.md` 和当前 `screens/` 落盘文件，梳理 UI 图套装还需要生成的页面浮层、基础组件、状态图和响应式图。

本文只做盘点，当前 `manifest.json` 已按用户确认从 163 张扩展为 181 张。

## 1. 数字口径

必须区分两个统计口径：

| 口径 | 含义 | 当前数量 |
|---|---|---:|
| Manifest 交付基线 | `manifest.json` 中正式计划交付的 foundation/page/overlay/component/state/responsive | 181 |
| Manifest 已落盘 | 181 张中目标文件已存在的图 | 181 |
| Manifest 还缺 | 181 张中目标文件尚未存在的图 | 0 |
| 当前实际最终 PNG | `screens/` 下所有非 `raw-imagegen`、非 `raw-deterministic`、非备份的最终 PNG，包括 Tab 变体、专题状态输入和 P7 扩容浮层 | 241 |
| Manifest 外扩展图 | 已生成但不属于 manifest 的最终 PNG | 60 |

Manifest 还缺 0 张的构成：

| 类型 | 计划 | 已落盘 | 还缺 |
|---|---:|---:|---:|
| foundation 视觉基线与规范板 | 8 | 8 | 0 |
| page 页面主图 | 27 | 27 | 0 |
| overlay 业务浮层 | 70 | 70 | 0 |
| component 元件与组件板 | 48 | 48 | 0 |
| state 通用状态图 | 16 | 16 | 0 |
| responsive 响应式与大屏适配图 | 12 | 12 | 0 |
| 合计 | 181 | 181 | 0 |

业务合理性复核补充（2026-06-28）：在 manifest 全部落盘后，已基于 `screens/` contact sheet 复核 pages、overlays、components、states、responsive。pages、overlays、components 作为业务语义参考保留；`screens/states/` 16 张和 `screens/responsive/` 12 张已完成返工，共 28 张，详见 `doc/04_assets/ui_suite_gpt_v1/BUSINESS_REASONABILITY_AUDIT.md`。

Manifest 外 60 张扩展图说明：

- `screens/pages/` 额外 58 张：Tab 变体、详情小 Tab、专题页内状态输入。
- `screens/foundations/` 额外 2 张：`foundation-generation-reference.png` 拼接参考板、`foundation-current-screen-reference.png` 当前态势大屏基准参考板，不计入 8 张 foundation。
- `/topics` 是现役前端菜单和路由，但不生成单张 `topics.png`；已有三张专题状态输入用于前端合并到 `/topics` 页内 Tab/Segmented。

## 2. 业务逻辑理解

系统不是通用后台 CRUD，而是园区网络全流量安全运营闭环：

1. 采集链路：Probe 采集全流量，经 Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO 形成可检索、可关联、可取证的数据底座。
2. 威胁研判：告警中心、战役列表、攻击链、加密流量、取证分析完成告警筛选、上下文聚合、证据定位、处置反馈。
3. 资产图谱：资产台账、实体图谱、数据融合、行为基准把 IP、MAC、主机、账号、服务、域名、告警、证据映射成可解释关系。
4. 检测运营：规则、部署、模型、MLOps、SOAR、白名单负责检测能力创建、测试、发布、回滚、反馈学习和例外治理。
5. 审计配置：合规、审计日志、通知、系统设置负责验收证据、关键动作留痕、通知升级和敏感配置治理。

浮层判断规则：

- Drawer：对象详情、日志、证据、实体、任务、审批、历史。
- Modal：创建、编辑、导出、配置、发布、回滚、反馈、危险动作确认。
- Dropdown：用户菜单、快速入口、行操作、批量操作。
- Popconfirm：删除、下载、停用、吊销、回滚等需要二次确认的轻量危险动作。

浮层生成新口径（2026-06-27）：后续 overlay 不需要绘制公共 AppShell 或宿主页面公共区，只需要绘制当前业务交互容器本体。画面可以是深色空画布上的 Modal/Drawer/Dropdown/Popconfirm，也可以是局部业务浮层裁切图；顶部栏、左侧菜单、底部栏和完整页面背景不再是必需内容。

## 3. `screens/pages` 当前图的层级

`screens/pages/` 当前有 85 张最终 PNG，分为三类：

| 类型 | 数量 | 说明 |
|---|---:|---|
| 路由页面主图 | 27 | manifest 内 page，已全部完成 |
| 页内 Tab / 小 Tab / 详情状态图 | 55 | 不新增路由，不当作浮层，用于补齐同一路由内部状态 |
| 专题页内状态输入 | 3 | `topics-encrypted-tunnel`、`topics-data-exfiltration`、`topics-apt-campaign`，用于 `/topics` 页内合并 |

已经完成的 Tab / 小 Tab / 详情状态图：

| 来源页面 | 已有扩展图 | 说明 |
|---|---|---|
| `/data-quality` | 7 | Topic 健康、Flink 质量、字段质量、存储质量、重放对账、质量报告、质量设置 |
| `/encrypted-traffic` | 4 | 指纹分析、隧道检测、外联画像、证据中心 |
| `/assets` | 9 | 服务器、网络设备、业务系统、未知资产，另有资产详情基础信息、网络接口、开放服务、归属信息、历史变更 |
| `/baselines` | 4 | 账号基线、端口基线、协议基线、时间段基线 |
| `/graph` | 3 | 攻击路径、通信路径、账号访问路径 |
| `/rules` | 4 | 测试验证、依赖引用、Session 样本、日志样本 |
| `/models` | 4 | 规则贡献、异常解释、样本示例、审计门禁 |
| `/whitelist` | 8 | IP、资产、账号、规则、模型、过期未处理、长期生效、未归属责任角色 |
| `/alerts/:alertId` | 5 | PCAP、Session、日志、图谱路径、文件 |
| `/audit-log` | 2 | 操作上下文、关联链路 |
| `/campaigns/:campaignId` | 5 | 账号、服务、部门、园区、业务系统 |

注意：

- 以上 55 张不替代 overlay。比如 `assets-detail-basic.png` 是资产详情组件小 Tab，不等于 `drawer-asset-detail` 浮层。
- 以上 55 张不新增左侧菜单或独立业务路由。
- 大 Tab、页面主图、详情小 Tab、浮层图必须分开生成和验收。
- `drawer-asset-detail` 是 manifest 内 overlay，定位为资产详情抽屉父容器：只展示选中资产轻摘要、风险概览、详情小 Tab 导航、当前 Tab 轻摘要、关键入口、关联状态、图谱预览、操作门禁和审计留痕。它不是 `assets-detail-basic`、`assets-detail-network-interface`、`assets-detail-open-services`、`assets-detail-ownership`、`assets-detail-history` 任一小 Tab 的完整内容页，也不因这些小 Tab 已生成而从 overlay 计数中删除。

## 4. 按页面梳理还缺哪些浮层

### 4.1 Manifest 内已规划但未生成的浮层

| 页面 / 范围 | 路由 | 当前 `pages/` 状态 | Manifest 内还缺浮层 |
|---|---|---|---|
| 全局 AppShell | 全局 | 无页面主图，已有 `dropdown-user-menu`、`drawer-mobile-navigation`、`drawer-notification-center`、`modal-global-search`、`dropdown-quick-entry` | 无 |
| 登录 | `/login` | 主图已完成，已有 `modal-login-error-captcha` | Manifest 内无单独未生成浮层 |
| 仪表盘 | `/dashboard` | 主图已完成，已有 `drawer-dashboard-kpi-detail`、`drawer-dashboard-task-detail` | Manifest 内无单独未生成浮层 |
| 态势大屏 | `/screen` | 主图已完成，已有 `modal-screen-readonly-token` | Manifest 内无单独未生成浮层 |
| 探针管理 | `/probes` | 主图已完成，已有 `drawer-probe-detail`、`modal-probe-config`、`modal-probe-batch-upgrade`、`modal-probe-cert-rotate`、`drawer-probe-log` | Manifest 内无单独未生成浮层 |
| 数据质量 | `/data-quality` | 主图 + 7 张 Tab 图已完成，已有 `drawer-dlq-sample`、`modal-data-replay-task`、`drawer-field-quality-sample` | Manifest 内无单独未生成浮层 |
| 告警中心 | `/alerts` | 主图已完成，已有 `modal-alert-batch`、`dropdown-alert-batch-actions`、`dropdown-alert-row-actions` | Manifest 内无单独未生成浮层 |
| 告警详情 | `/alerts/:alertId` | 主图 + 5 张证据 Tab 图已完成，已有 `modal-alert-status`、`modal-alert-feedback`、`modal-evidence-detail`、`modal-playbook-trigger`、`modal-whitelist-draft-from-alert` | Manifest 内无单独浮层 |
| 战役列表 | `/campaigns` | 主图已完成，已有 `drawer-campaign-detail` 战役详情抽屉 | Manifest 内无单独未生成浮层 |
| 战役详情 | `/campaigns/:campaignId` | 主图 + 5 张影响范围 Tab 图已完成 | Manifest 内无单独浮层 |
| 攻击链分析 | `/attack-chains` | 主图已完成，已有 `drawer-attack-chain-detail` 攻击链详情抽屉 | Manifest 内无单独未生成浮层 |
| 加密流量 | `/encrypted-traffic` | 主图 + 4 张 Tab 图已完成，已有 `drawer-encrypted-fingerprint` 加密指纹详情抽屉、`drawer-certificate-detail` 证书详情抽屉 | Manifest 内无单独未生成浮层 |
| 取证分析 | `/forensics` | 主图已完成，已有 `popconfirm-pcap-download`、`drawer-session-replay`、`modal-forensics-task` 取证任务详情弹窗 | Manifest 内无单独未生成浮层 |
| 资产台账 | `/assets` | 主图 + 4 张资产类型大 Tab + 5 张资产详情小 Tab 已完成，已有 `drawer-asset-detail` 父容器图、`modal-asset-edit` 编辑弹窗和 `drawer-asset-history` 历史抽屉 | Manifest 内无单独未生成浮层 |
| 实体图谱 | `/graph` | 主图 + 3 张路径分析小 Tab 已完成，已有 `drawer-graph-entity` 实体详情抽屉和 `drawer-graph-path-analysis` 路径分析抽屉 | Manifest 内无单独未生成浮层 |
| 数据融合 | `/fusion` | 主图已完成，已有 `drawer-fusion-conflict` 冲突处理抽屉、`modal-fusion-rule-edit` 融合规则编辑弹窗 | Manifest 内无单独未生成浮层 |
| 行为基准 | `/baselines` | 主图 + 4 张 Tab 图已完成，已有 `modal-baseline-threshold` 基线阈值编辑弹窗 | Manifest 内无单独未生成浮层 |
| 规则管理 | `/rules` | 主图 + 4 张局部小 Tab 已完成，已有 `modal-rule-edit` 规则编辑弹窗、`drawer-rule-detail` 规则详情抽屉、`popconfirm-delete` 删除确认、`modal-rule-publish` 发布确认弹窗 | Manifest 内无单独未生成浮层 |
| 部署管理 | `/deployments` | 主图已完成，已有 `modal-deployment-create` 创建部署弹窗、`modal-deployment-rollback` 回滚确认弹窗 | Manifest 内无单独未生成浮层 |
| 模型管理 | `/models` | 主图 + 4 张局部小 Tab 已完成，已有 `drawer-model-detail` 模型详情抽屉 | Manifest 内无单独未生成浮层 |
| MLOps 编排 | `/mlops` | 主图已完成，已有 `drawer-mlops-task-detail` MLOps 任务详情抽屉 | Manifest 内无单独未生成浮层 |
| SOAR 剧本 | `/playbooks` | 主图已完成，已有 `modal-playbook-edit` SOAR 剧本编辑弹窗 | Manifest 内无单独未生成浮层 |
| 白名单 | `/whitelist` | 主图 + 8 张局部小 Tab 已完成，已有 `modal-whitelist-add` 新增白名单弹窗、`drawer-whitelist-approval` 白名单审批详情抽屉 | Manifest 内无单独未生成浮层 |
| 合规审计 | `/compliance` | 主图已完成 | Manifest 内无单独浮层 |
| 审计日志 | `/audit-log` | 主图 + 2 张操作详情小 Tab 已完成 | Manifest 内无单独浮层 |
| 通知配置 | `/notifications` | 主图已完成 | Manifest 内只有全局 `drawer-notification-center`，无通知配置页专属编辑浮层 |
| 系统设置 | `/settings` | 主图已完成，已有 `modal-settings-token` API 令牌管理弹窗 | Manifest 内无单独未生成浮层 |
| 404 | `*` | 主图已完成 | 无需业务浮层 |

Manifest 内业务浮层、组件板、通用状态图和响应式适配图已全部完成。AppShell 与上下文前八张组件板已按当前 `screen.png` 和 foundations token 返工/生成完成：顶部不承载通知/用户动作组，顶部快捷入口只包含 PCAP/资产/规则/脚本/帮助/更多应用，左侧底部用户区是唯一常驻用户身份区，二级菜单同处 166px 左侧单栏，底部右侧承载通知/设置/全局配置/电源，业务面包屑不进入公共 AppShell，站点/时间模块不混入业务筛选。基础控件、表单筛选、数据展示、图表组件、安全业务组件、通用状态图和响应式适配图均已完成；下一张常规生成项为无。

### 4.2 产品逻辑建议追加但未进入 manifest 的浮层

这些项来自产品矩阵中的闭环动作。2026-06-28 用户确认“全部处理生成 UI 图”后，P7 已从候选转为扩容队列并写入 `manifest.json`；总计划从 163 张扩展为 181 张。当前 P7 已完成 18/18，还缺 0 张：

| 建议 ID | 来源页面 | 类型 | 业务原因 |
|---|---|---|---|
| `modal-topic-save-view` | `/topics` | Modal | 已完成；固化专题筛选范围和保存视图 |
| `drawer-topic-scope-edit` | `/topics` | Drawer | 已完成；编辑专题范围、名称、刷新周期 |
| `modal-topic-report-export` | `/topics` | Modal | 已完成；导出专题报告 PDF/Word |
| `modal-topic-evidence-package-export` | `/topics` | Modal | 已完成；导出专题证据包或试点周报 |
| `drawer-topic-subscription` | `/topics` | Drawer | 已完成；专题订阅、静默和免打扰配置 |
| `dropdown-topic-share-favorite` | `/topics` | Dropdown | 已完成；分享、收藏、复制专题链接 |
| `modal-campaign-report-export` | `/campaigns/:campaignId` | Modal | 已完成；战役详情生成战役报告 |
| `modal-forensics-evidence-export` | `/forensics` | Modal | 已完成；取证页导出 PCAP、CSV、日志、图谱路径材料 |
| `drawer-compliance-gate-detail` | `/compliance` | Drawer | 已完成；验收门禁未通过项下钻 |
| `modal-compliance-evidence-package-export` | `/compliance` | Modal | 已完成；导出合规证据包 |
| `modal-compliance-report-export` | `/compliance` | Modal | 已完成；导出运行报告 PDF/Word |
| `drawer-audit-operation-detail` | `/audit-log` | Drawer | 已完成；审计操作详情抽屉，承载 Diff、请求上下文和关联链路 |
| `modal-audit-export` | `/audit-log` | Modal | 已完成；导出审计取证材料并做权限确认 |
| `modal-notification-channel-edit` | `/notifications` | Modal | 已完成；新增或编辑通知渠道 |
| `modal-notification-template-preview-test` | `/notifications` | Modal | 已完成；通知模板预览和测试发送 |
| `drawer-notification-silence-rule` | `/notifications` | Drawer | 已完成；抑制、静默、维护窗口规则 |
| `popconfirm-settings-token-revoke` | `/settings` | Popconfirm | 已完成；API 令牌吊销二次确认 |
| `drawer-settings-rbac-edit` | `/settings` | Drawer | 已完成；RBAC 权限矩阵编辑和影响范围提示 |

P7 已确认追加；当前总计划 181 张，P7 还缺 0 张。

### 4.3 暂不需要单独浮层的页面状态

| 页面状态 | 处理方式 |
|---|---|
| Tab 变体图 | 已作为 `screens/pages/` 扩展图生成，不再转成 Modal/Drawer |
| `not-found` | 404 页面只保留页面主图和通用状态图，不需要业务浮层 |
| `/topics` 三张专题状态输入 | 前端合并进 `/topics` 页内 Tab/Segmented，不作为独立路由或左侧菜单 |

## 5. 基础组件与基础图片还缺多少

### 5.1 Foundation 基础图片

已完成 8/8，不需要继续补 foundation：

`foundation-visual-reference`、`foundation-layout-grid`、`foundation-color-status`、`foundation-typography-density`、`foundation-icons-actions`、`foundation-data-viz`、`foundation-table-form`、`foundation-responsive`。

### 5.2 Component 元件与组件板

当前已生成 48/48，还缺 0/48。

| 分组 | 还缺数量 | 图片 ID |
|---|---:|---|
| AppShell 与导航 | 0 | 已完成：`component-app-header`、`component-primary-sidebar`、`component-secondary-menu`、`component-bottom-status-bar`、`component-breadcrumb-context`、`component-site-time-selector`、`component-quick-entry`、`component-user-menu` |
| 基础控件 | 0 | 已完成：`component-button`、`component-icon-button`、`component-status-chip`、`component-tooltip`、`component-tabs`、`component-segmented`、`component-dropdown`、`component-pagination`、`component-input`、`component-search` |
| 表单与筛选 | 0 | 已完成：`component-select`、`component-date-range`、`component-switch-checkbox-radio`、`component-condition-builder`、`component-batch-action-bar` |
| 数据展示 | 0 | 已完成：`component-empty-card`、`component-permission-card` |
| 图表组件 | 0 | 已完成：`component-line-area-chart`、`component-donut-chart`、`component-bar-ranking-chart`、`component-sankey-flow`、`component-radar-quality`、`component-heatmap`、`component-topology-graph`、`component-timeline-state-machine` |
| 安全业务组件 | 0 | 已完成：`component-alert-queue`、`component-risk-score`、`component-alert-timeline`、`component-evidence-drawer`、`component-asset-context`、`component-action-rail`、`component-feedback-block`、`component-acceptance-gate-matrix` |

### 5.3 State 通用状态图

当前已生成 16/16，还缺 0/16：

`state-page-loading`、`state-table-loading`、`state-chart-loading`、`state-empty-page`、`state-empty-table`、`state-empty-chart`、`state-api-error`、`state-network-error`、`state-unauthorized`、`state-forbidden`、`state-partial-degraded`、`state-offline-probe`、`state-stream-backpressure`、`state-task-running`、`state-task-failed`、`state-success-accepted`。

### 5.4 Responsive 响应式与大屏适配图

当前已生成 12/12，还缺 0/12：

`responsive-dashboard-1440`、`responsive-dashboard-1920`、`responsive-screen-4k`、`responsive-alerts-1440`、`responsive-alerts-1920`、`responsive-forensics-1440`、`responsive-graph-1440`、`responsive-compliance-1440`、`responsive-tablet-dashboard`、`responsive-tablet-alert-detail`、`responsive-mobile-navigation`、`responsive-mobile-alert-list`。

## 6. 推荐生成顺序

1. manifest 交付基线已完成 181/181。
2. component、state、responsive 均已补齐；当前无下一张常规生成项。
3. P7 18 张专题、合规、审计、通知和设置浮层已确认追加并完成 18/18；当前无待生成 UI 图。

## 7. 验收注意事项

- 浮层图必须是真实交互容器，不能用已有 Tab 状态图替代。
- 危险动作必须出现影响范围、权限提示、审计留痕和确认/取消动作。
- 详情 Drawer 必须保留来源上下文，例如 `alertId`、`assetId`、`evidenceId`、时间窗、租户和跳回入口。
- 组件板不是页面截图，必须展示正常、悬停、选中、禁用、错误、加载等状态。
- 所有最终 PNG 仍统一输出 `1920x1080`。
