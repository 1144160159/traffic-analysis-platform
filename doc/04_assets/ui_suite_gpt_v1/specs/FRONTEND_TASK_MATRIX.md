# 前端任务矩阵

本文件把 UI 图契约整理成前端可派工的批次、页面、路由、API 和浮层依赖。前端开发以批次推进，不按 PNG 零散推进。

## 批次顺序

- `00-foundation` 公共骨架与组件底座：先统一 AppShell、token、Ant Design 组件封装、状态页和响应式规则，避免每个页面重复返工。 页面=0，浮层=0 foundation=8, component=48, state=16, responsive=12
- `01-core-entry` 入口、总览与专题统一页：打通登录、仪表盘、大屏、专题三态，形成所有页面复用的导航和闭环栏样板。 页面=4，浮层=13
- `02-threat-forensics` 告警、战役、攻击链、加密流量与取证闭环：完成核心安全分析闭环：告警研判、详情证据、战役聚类、攻击路径、取证导出和审计留痕。 页面=7，浮层=17
- `03-collection-asset` 采集质量、资产图谱与融合基线：证明数据来源可信、资产画像可追踪、图谱/融合/基线能支撑告警解释。 页面=6，浮层=16
- `04-detection-ops` 规则、部署、模型、MLOps、剧本和白名单：把检测能力从规则编辑推进到发布、模型激活、剧本执行和白名单治理。 页面=6，浮层=11
- `05-audit-config` 合规、审计、通知、系统设置与异常页：补齐验收证据、审计查询、通知升级、租户配置、权限与 API token 管理。 页面=5，浮层=13

## 页面矩阵

| 批次 | 页面 | 路由 | React 页面 | API | 浮层 | 契约 |
| --- | --- | --- | --- | ---: | ---: | --- |
| 01-core-entry | 登录 | `/login` | `LoginPage` | 0 | 1 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/login.md` |
| 01-core-entry | 态势大屏 | `/screen` | `SituationalScreen` | 3 | 1 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/screen.md` |
| 01-core-entry | 仪表盘 | `/dashboard` | `DashboardOperationsPage` | 3 | 5 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/dashboard.md` |
| 02-threat-forensics | 告警中心 | `/alerts` | `AlertTriagePage` | 1 | 3 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/alerts.md` |
| 02-threat-forensics | 告警详情 | `/alerts/:alertId` | `AlertDetailPage` | 3 | 5 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/alert-detail.md` |
| 02-threat-forensics | 战役列表 | `/campaigns` | `CampaignWorkbenchPage` | 1 | 1 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/campaigns.md` |
| 02-threat-forensics | 战役详情 | `/campaigns/:campaignId` | `CampaignDetailPage` | 1 | 1 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/campaign-detail.md` |
| 02-threat-forensics | 攻击链分析 | `/attack-chains` | `AttackChainAnalysisPage` | 1 | 1 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/attack-chains.md` |
| 02-threat-forensics | 加密流量 | `/encrypted-traffic` | `EncryptedTrafficPage` | 5 | 2 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/encrypted-traffic.md` |
| 02-threat-forensics | 取证分析 | `/forensics` | `ForensicsWorkbenchPage` | 2 | 4 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/forensics.md` |
| 03-collection-asset | 资产台账 | `/assets` | `AssetInventoryPage` | 1 | 3 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/assets.md` |
| 03-collection-asset | 实体图谱 | `/graph` | `GraphEntityPage` | 1 | 2 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/graph.md` |
| 03-collection-asset | 数据融合 | `/fusion` | `FusionWorkbenchPage` | 2 | 2 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/fusion.md` |
| 03-collection-asset | 行为基准 | `/baselines` | `BaselineWorkbenchPage` | 1 | 1 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/baselines.md` |
| 03-collection-asset | 探针管理 | `/probes` | `ProbesManagementPage` | 1 | 5 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/probes.md` |
| 04-detection-ops | 规则管理 | `/rules` | `RuleManagementPage` | 1 | 4 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/rules.md` |
| 04-detection-ops | 部署管理 | `/deployments` | `DeploymentManagementPage` | 1 | 2 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/deployments.md` |
| 04-detection-ops | 模型管理 | `/models` | `ModelManagementPage` | 1 | 1 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/models.md` |
| 04-detection-ops | MLOps 编排 | `/mlops` | `MlopsOrchestrationPage` | 2 | 1 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/mlops.md` |
| 03-collection-asset | 数据质量 | `/data-quality` | `DataQualityPage` | 1 | 3 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/data-quality.md` |
| 04-detection-ops | SOAR 剧本 | `/playbooks` | `PlaybookAutomationPage` | 2 | 1 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/playbooks.md` |
| 04-detection-ops | 白名单 | `/whitelist` | `WhitelistGovernancePage` | 1 | 2 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/whitelist.md` |
| 05-audit-config | 合规审计 | `/compliance` | `ComplianceAuditPage` | 2 | 3 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/compliance.md` |
| 05-audit-config | 审计日志 | `/audit-log` | `AuditLogPage` | 1 | 2 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/audit-log.md` |
| 05-audit-config | 通知配置 | `/notifications` | `NotificationConfigPage` | 1 | 4 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/notifications.md` |
| 05-audit-config | 系统设置 | `/settings` | `SettingsGovernancePage` | 3 | 4 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/settings.md` |
| 05-audit-config | 404 异常页 | `*` | `NotFoundPage` | 0 | 0 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/not-found.md` |
| 01-core-entry | 专题面板 | `/topics` | `TopicWorkbenchPage` | 3 | 6 | `doc/04_assets/ui_suite_gpt_v1/specs/page-contracts/topics.md` |

## 浮层实现规则

- Modal：承载少量表单、导出、发布、确认前预览等交互；桌面端必须使用小尺寸弹窗，不得铺满或遮住整个浏览器业务区域。
- Drawer：优先承载详情、证据、日志、路径分析等上下文下钻；从侧面滑出并保留宿主业务上下文可见，不得做成全屏覆盖层。
- Dropdown/Menu：承载行操作、快捷入口和分享收藏等轻量操作。
- Popconfirm：只用于删除、撤销、下载等需要二次确认的短动作。
- 业务详情内容较少时优先使用窄 Drawer，其次使用小 Modal；只有独立页面级工作流才允许占据完整业务区，验收专用 focus 截图状态不等同于生产弹层。

## 完成定义

- 当前批次所有页面契约已实现，相关浮层均可触发。
- 页面使用 `services/api.ts` 或现有 service，不直接 `fetch`。
- 每个页面覆盖 loading、error、empty、403/401 中的适用状态。
- 运行 `node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs` 无 error。
- 运行 `cd web/ui && npm run build`，页面级变更再补 Playwright 截图证据。
