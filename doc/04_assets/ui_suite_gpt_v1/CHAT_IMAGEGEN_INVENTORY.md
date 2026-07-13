# GPT 聊天窗口生图 UI 清单 v2

更新时间：2026-06-28

## 1. 结论

由于当前 `ui_suite_gpt_v1` 的 API 自动生图链路不可用，后续改为在 GPT 聊天窗口中逐张生成 UI 图片，并将结果人工保存到本地文件系统。

按工业级 UI 设计交付口径，第一版完整 UI 视觉套装不应只包含页面和弹窗，还必须包含设计系统级的元件、组件、状态、响应式和数据可视化规范图。

像素规范：所有高保真 UI 图一律生成 `1920x1080 px` PNG。响应式与大屏适配图可以表达不同视口下的信息布局策略，但交付图片本身仍必须是 `1920x1080 px`，不得输出 1440、2K、4K、平板或移动端设备原始像素尺寸。

建议生成总数：

```text
181 张
```

其中：

| 类型 | 数量 | 说明 |
|---|---:|---|
| 视觉基线与规范板 | 8 | 统一视觉语言、布局、色彩、字体、图标、图表和响应式规则 |
| 页面主图 | 27 | 覆盖登录、404、现役业务页面和详情路由；`/topics` 不生成单张页面主图，按专题 Tab 设计输入在前端合并实现 |
| 业务浮层图 | 70 | 覆盖 Modal、Drawer、Dropdown、Popconfirm、右侧闭环栏关键状态；含 P7 专题、合规、审计、通知和设置扩容浮层 |
| 元件与组件板 | 48 | 覆盖 AppShell、导航、表格、表单、状态、图表、业务组件等可复用组件 |
| 通用状态图 | 16 | 覆盖 loading、empty、error、unauthorized、offline、degraded、success 等状态 |
| 响应式与大屏适配图 | 12 | 在统一 1920x1080 画布内表达桌面、平板、移动端和大屏适配策略 |

当前 `manifest.json` 已覆盖完整 `181` 张提示词范围：

```text
8 张 foundations + 27 张页面图 + 70 张浮层图 + 48 张组件板 + 16 张状态图 + 12 张响应式图 = 181 张
```

这些提示词已经统一绑定 foundations 拼接参考板：

```text
doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-generation-reference.png
```

生成图必须严格遵守 foundations 的 AppShell、栅格、色彩 token、状态语义、字号密度、圆角、表格行高、ECharts 深色样式和响应式策略；如果只是“风格相似”但偏离规范，应判定为不合格并重新生成。

全局 AppShell 专项门禁：除登录/认证页、态势大屏基准图和移动端导航抽屉等明确例外外，所有出现 AppShell 的 UI 图片都必须与 `screens/pages/screen.png` 的公共区域完全一致。公共区域包括顶部单栏、左侧单栏和底部单栏；三者的内容、图标、顺序、尺寸、间距、分隔线、状态色、字号密度、背景、圆角和激活态都不得按页面自行变化。左侧菜单固定为态势大屏同款单栏展开式菜单，一级菜单固定为“综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置”，二级只显示当前业务域页面，二级菜单图标必须遵循 `doc/04_assets/ui_suite_gpt_v1/standards/APP_SHELL_ICON_STANDARD.md`；禁止第三层菜单、双栏导航、卡片式二级菜单、工具面板式侧栏、专题目录式侧栏，禁止把页面内部模块塞进左侧菜单。修复既有 page 图片时，只允许修改顶部、左侧、底部公共区域，中部业务内容区必须保持原图不变。

Tab 拆分设计合并门禁：`/topics` 是现役专题菜单和前端路由，但不生成单张 `topics.png`；加密隧道、数据外传、APT 战役的拆分设计输入必须在前端合并到同一个 `/topics` 页面内作为页内 Tab/Segmented 状态。后续任何页面如果 UI 设计图按 Tab 拆多张，也必须合并为一个路由页面内的状态，不能拆成多个左侧菜单或独立业务路由，除非产品文档明确要求新增导航入口。

## 2. 计数原则

| 原则 | 说明 |
|---|---|
| 一张图等于一次聊天窗口生图输出 | 便于逐张生成、逐张归档、逐张审核 |
| 页面图按业务页面计数 | 每个路由至少一张完整页面主图 |
| 弹窗/抽屉/下拉按关键交互计数 | 只生成会影响业务闭环、权限、安全、验收的浮层 |
| 组件按组件板计数 | 一张组件板可以包含同一组件的正常、悬停、选中、禁用、错误、加载等状态 |
| 状态按模式计数 | 不为每个页面重复生成 loading/empty/error，而是生成可复用状态规范板 |
| 响应式按关键场景计数 | 只生成主工作流在不同断点下的代表图，不重复 28 个页面的所有断点 |

## 3. 目录规划

聊天窗口生成后，图片建议保存到：

| 类型 | 本地目录 |
|---|---|
| 视觉基线与规范板 | `doc/04_assets/ui_suite_gpt_v1/screens/foundations/` |
| 页面主图 | `doc/04_assets/ui_suite_gpt_v1/screens/pages/` |
| 业务浮层图 | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/` |
| 元件与组件板 | `doc/04_assets/ui_suite_gpt_v1/screens/components/` |
| 通用状态图 | `doc/04_assets/ui_suite_gpt_v1/screens/states/` |
| 响应式与大屏适配图 | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/` |

每张图片必须在 `doc/04_assets/generated/IMAGEGEN_ARCHIVE.md` 或后续专门台账中记录：图片 ID、生成时间、使用提示词、保存路径、是否采纳、驳回原因。

## 4. 视觉基线与规范板：8 张

| # | 图片 ID | 名称 | 主要内容 |
|---:|---|---|---|
| 1 | `foundation-visual-reference` | 最终视觉基准板 | 以最终选定深色安全运营台为基准，说明导航、顶部状态条、主工作区、右侧闭环栏、底部状态栏 |
| 2 | `foundation-layout-grid` | 布局与栅格规范 | 统一 1920x1080 画布下的 AppShell、单栏展开式导航、内容网格、面板间距，并标注不同视口的适配原则 |
| 3 | `foundation-color-status` | 色彩与状态语义 | 背景、面板、边框、主色、成功、警告、高危、禁用、审计等语义色 |
| 4 | `foundation-typography-density` | 字体与密度规范 | 页面标题、面板标题、KPI 数字、表格正文、字段标签、辅助说明字号 |
| 5 | `foundation-icons-actions` | 图标与动作语义 | 导航图标、告警、资产、证据、处置、审计、配置等图标使用规则 |
| 6 | `foundation-data-viz` | 数据可视化规范 | 折线、面积、环图、桑基、拓扑、图谱、时间线、矩阵、热力图样式 |
| 7 | `foundation-table-form` | 表格与表单密度规范 | 高密度表格、筛选区、表单项、按钮组、批量操作、分页 |
| 8 | `foundation-responsive` | 响应式适配原则 | 桌面、平板、移动端抽屉导航、右侧栏收起、图表压缩策略 |

## 5. 页面主图：27 张

页面主图沿用当前 `manifest.json` 范围，但必须按最新 6 个一级菜单和业务页面重审内容。`/topics` 是唯一现役专题菜单页，但不作为单张页面主图生成；加密隧道、数据外传、APT 战役只作为该页内业务切换状态和前端合并实现输入。

| # | 图片 ID | 页面 | 路由 | 归属 |
| ---: | --- | --- | --- | --- |
| 1 | `login` | 登录 | `/login` | 全局 |
| 2 | `screen` | 态势大屏 | `/screen` | 综合态势 |
| 3 | `dashboard` | 仪表盘 | `/dashboard` | 综合态势 |
| 4 | `alerts` | 告警中心 | `/alerts` | 威胁分析 |
| 5 | `alert-detail` | 告警详情 | `/alerts/:alertId` | 威胁分析 |
| 6 | `campaigns` | 战役列表 | `/campaigns` | 威胁分析 |
| 7 | `campaign-detail` | 战役详情 | `/campaigns/:campaignId` | 威胁分析 |
| 8 | `attack-chains` | 攻击链分析 | `/attack-chains` | 威胁分析 |
| 9 | `encrypted-traffic` | 加密流量 | `/encrypted-traffic` | 威胁分析 |
| 10 | `forensics` | 取证分析 | `/forensics` | 威胁分析 |
| 11 | `assets` | 资产台账 | `/assets` | 资产图谱 |
| 12 | `graph` | 实体图谱 | `/graph` | 资产图谱 |
| 13 | `fusion` | 数据融合 | `/fusion` | 资产图谱 |
| 14 | `baselines` | 行为基准 | `/baselines` | 资产图谱 |
| 15 | `probes` | 探针管理 | `/probes` | 采集监测 |
| 16 | `rules` | 规则管理 | `/rules` | 检测运营 |
| 17 | `deployments` | 部署管理 | `/deployments` | 检测运营 |
| 18 | `models` | 模型管理 | `/models` | 检测运营 |
| 19 | `mlops` | MLOps 编排 | `/mlops` | 检测运营 |
| 20 | `data-quality` | 数据质量 | `/data-quality` | 采集监测 |
| 21 | `playbooks` | SOAR 剧本 | `/playbooks` | 检测运营 |
| 22 | `whitelist` | 白名单 | `/whitelist` | 检测运营 |
| 23 | `compliance` | 合规审计 | `/compliance` | 审计配置 |
| 24 | `audit-log` | 审计日志 | `/audit-log` | 审计配置 |
| 25 | `notifications` | 通知配置 | `/notifications` | 审计配置 |
| 26 | `settings` | 系统设置 | `/settings` | 审计配置 |
| 27 | `not-found` | 404 异常页 | `*` | 全局 |

## 6. 业务浮层图：70 张

当前 `manifest.json` 已覆盖 70 张业务浮层。前 52 张为原交付基线，53-70 为 2026-06-28 用户确认“全部处理生成 UI 图”后追加的 P7 扩容浮层；当前 70/70 均已落盘。

| 分组 | 数量 | 应覆盖内容 |
|---|---:|---|
| 全局与登录 | 6 | 用户菜单、快速入口、通知中心、全局搜索、移动端导航、登录异常/验证码 |
| 综合态势 | 9 | KPI 详情、待办详情、态势大屏脱敏/只读令牌、专题保存视图、专题范围编辑、专题报告导出、专题证据包导出、专题订阅、专题分享收藏 |
| 采集监测 | 8 | 探针详情、探针配置、批量升级、证书轮换、探针日志、DLQ 样例、重放任务、字段质量样例 |
| 威胁分析 | 15 | 告警批量操作、状态更新、反馈、证据详情、取证任务、战役详情、攻击链详情、加密指纹详情、证书详情、剧本触发、白名单草案、PCAP 下载确认、会话复放、战役报告导出、取证证据导出 |
| 资产图谱 | 8 | 资产详情、资产编辑、资产历史、图谱实体、路径分析、融合冲突、融合规则编辑、基线阈值编辑 |
| 检测运营 | 9 | 规则编辑、规则详情、发布确认、新建部署、回滚部署、模型详情、训练任务、剧本编辑、白名单新增/审批 |
| 审计配置 | 15 | 合规门禁详情、合规证据包导出、合规运行报告导出、审计详情、审计材料导出、通知渠道编辑、通知模板预览测试、通知静默规则、API 令牌创建/撤销、RBAC 权限编辑 |

浮层应优先生成以下 70 个 ID：

| # | 图片 ID | 名称 |
| ---: | --- | --- |
| 1 | `dropdown-user-menu` | 用户下拉菜单 |
| 2 | `drawer-mobile-navigation` | 移动端侧滑菜单 |
| 3 | `drawer-notification-center` | 通知中心抽屉 |
| 4 | `modal-global-search` | 全局搜索弹窗 |
| 5 | `dropdown-quick-entry` | 快速入口下拉 |
| 6 | `modal-login-error-captcha` | 登录异常与验证码状态 |
| 7 | `drawer-dashboard-kpi-detail` | 仪表盘 KPI 详情 |
| 8 | `drawer-dashboard-task-detail` | 待办任务详情 |
| 9 | `modal-screen-readonly-token` | 态势大屏只读令牌/脱敏配置 |
| 10 | `drawer-probe-detail` | 探针详情 |
| 11 | `modal-probe-config` | 探针配置下发 |
| 12 | `modal-probe-batch-upgrade` | 探针批量升级确认 |
| 13 | `modal-probe-cert-rotate` | 证书轮换确认 |
| 14 | `drawer-probe-log` | 探针日志抽屉 |
| 15 | `drawer-dlq-sample` | DLQ 样例详情 |
| 16 | `modal-data-replay-task` | 数据重放任务 |
| 17 | `drawer-field-quality-sample` | 字段质量样例 |
| 18 | `modal-alert-batch` | 告警批量操作确认 |
| 19 | `dropdown-alert-batch-actions` | 告警批量操作下拉 |
| 20 | `dropdown-alert-row-actions` | 告警行操作下拉 |
| 21 | `modal-alert-status` | 更新告警状态 |
| 22 | `modal-alert-feedback` | 提交告警反馈 |
| 23 | `modal-evidence-detail` | 证据详情 |
| 24 | `modal-forensics-task` | 取证任务详情 |
| 25 | `drawer-campaign-detail` | 战役详情抽屉 |
| 26 | `drawer-attack-chain-detail` | 攻击链详情抽屉 |
| 27 | `drawer-encrypted-fingerprint` | 加密指纹详情 |
| 28 | `drawer-certificate-detail` | 证书详情 |
| 29 | `modal-playbook-trigger` | 从告警触发剧本 |
| 30 | `modal-whitelist-draft-from-alert` | 从告警生成白名单草案 |
| 31 | `popconfirm-pcap-download` | PCAP 下载确认 |
| 32 | `drawer-session-replay` | 会话复放抽屉 |
| 33 | `drawer-asset-detail` | 资产详情 |
| 34 | `modal-asset-edit` | 编辑资产 |
| 35 | `drawer-asset-history` | 资产历史 |
| 36 | `drawer-graph-entity` | 图谱实体详情 |
| 37 | `drawer-graph-path-analysis` | 图谱路径分析 |
| 38 | `drawer-fusion-conflict` | 数据融合冲突处理 |
| 39 | `modal-fusion-rule-edit` | 融合规则编辑 |
| 40 | `modal-baseline-threshold` | 基线阈值编辑 |
| 41 | `modal-rule-edit` | 新建/编辑规则 |
| 42 | `drawer-rule-detail` | 规则详情 |
| 43 | `popconfirm-delete` | 规则删除确认 |
| 44 | `modal-rule-publish` | 规则发布确认 |
| 45 | `modal-deployment-create` | 新建部署 |
| 46 | `modal-deployment-rollback` | 回滚部署确认 |
| 47 | `drawer-model-detail` | 模型详情 |
| 48 | `drawer-mlops-task-detail` | MLOps 任务详情 |
| 49 | `modal-playbook-edit` | 剧本编辑 |
| 50 | `modal-whitelist-add` | 添加白名单 |
| 51 | `drawer-whitelist-approval` | 白名单审批详情 |
| 52 | `modal-settings-token` | 创建 API 令牌 |
| 53 | `modal-topic-save-view` | 专题保存视图 |
| 54 | `drawer-topic-scope-edit` | 专题范围编辑 |
| 55 | `modal-topic-report-export` | 专题报告导出 |
| 56 | `modal-topic-evidence-package-export` | 专题证据包导出 |
| 57 | `drawer-topic-subscription` | 专题订阅配置 |
| 58 | `dropdown-topic-share-favorite` | 专题分享收藏菜单 |
| 59 | `modal-campaign-report-export` | 战役报告导出 |
| 60 | `modal-forensics-evidence-export` | 取证证据导出 |
| 61 | `drawer-compliance-gate-detail` | 合规门禁详情 |
| 62 | `modal-compliance-evidence-package-export` | 合规证据包导出 |
| 63 | `modal-compliance-report-export` | 合规运行报告导出 |
| 64 | `drawer-audit-operation-detail` | 审计操作详情 |
| 65 | `modal-audit-export` | 审计材料导出 |
| 66 | `modal-notification-channel-edit` | 通知渠道编辑 |
| 67 | `modal-notification-template-preview-test` | 通知模板预览测试 |
| 68 | `drawer-notification-silence-rule` | 通知静默规则 |
| 69 | `popconfirm-settings-token-revoke` | API 令牌吊销确认 |
| 70 | `drawer-settings-rbac-edit` | RBAC 权限编辑 |

## 7. 元件与组件板：48 张

组件板不是页面截图，而是用于后续 Figma / 前端实现的设计系统图。每张图应展示组件结构、常用状态、尺寸、密度和语义色。

| 分组 | 数量 | 组件板 |
|---|---:|---|
| AppShell 与导航 | 8 | 顶部状态栏、左侧一级导航、二级菜单、底部状态栏、面包屑、站点/时间选择、快速入口、用户菜单 |
| 基础控件 | 8 | 按钮、图标按钮、标签/Badge、Tooltip、Tabs、Segmented、Dropdown、Pagination |
| 表单与筛选 | 7 | 输入框、搜索框、选择器、日期时间窗、开关/复选/单选、条件构造器、批量操作栏 |
| 数据展示 | 9 | 高密度表格、详情描述列表、KPI 卡、状态卡、排行列表、日志列表、证据文件卡、空状态卡、权限提示卡 |
| 图表组件 | 8 | 折线/面积、环图、柱状/排行、桑基流向、雷达/质量评分、热力图、拓扑/图谱、时间线/状态机 |
| 安全业务组件 | 8 | 告警队列、风险评分、告警时间线、证据抽屉、资产上下文、响应动作栏、反馈学习块、验收门禁矩阵 |

建议组件板 ID：

```text
component-app-header
component-primary-sidebar
component-secondary-menu
component-bottom-status-bar
component-breadcrumb-context
component-site-time-selector
component-quick-entry
component-user-menu
component-button
component-icon-button
component-status-chip
component-tooltip
component-tabs
component-segmented
component-dropdown
component-pagination
component-input
component-search
component-select
component-date-range
component-switch-checkbox-radio
component-condition-builder
component-batch-action-bar
component-data-table
component-description-list
component-kpi-tile
component-health-card
component-ranking-list
component-log-list
component-evidence-file-card
component-empty-card
component-permission-card
component-line-area-chart
component-donut-chart
component-bar-ranking-chart
component-sankey-flow
component-radar-quality
component-heatmap
component-topology-graph
component-timeline-state-machine
component-alert-queue
component-risk-score
component-alert-timeline
component-evidence-drawer
component-asset-context
component-action-rail
component-feedback-block
component-acceptance-gate-matrix
```

## 8. 通用状态图：16 张

| # | 图片 ID | 状态 |
|---:|---|---|
| 1 | `state-page-loading` | 页面加载中 |
| 2 | `state-table-loading` | 表格加载中 |
| 3 | `state-chart-loading` | 图表加载中 |
| 4 | `state-empty-page` | 页面空数据 |
| 5 | `state-empty-table` | 表格空数据 |
| 6 | `state-empty-chart` | 图表空数据 |
| 7 | `state-api-error` | API 错误 |
| 8 | `state-network-error` | 网络异常 |
| 9 | `state-unauthorized` | 未登录 / 401 重新认证 |
| 10 | `state-forbidden` | 无权限 / 403 申请权限 |
| 11 | `state-partial-degraded` | 部分降级 |
| 12 | `state-offline-probe` | 探针离线 |
| 13 | `state-stream-backpressure` | 流处理背压 |
| 14 | `state-task-running` | 任务运行中 |
| 15 | `state-task-failed` | 任务失败可重试 |
| 16 | `state-success-accepted` | 验收通过/操作成功 |

## 9. 响应式与大屏适配图：12 张

| # | 图片 ID | 场景 |
|---:|---|---|
| 1 | `responsive-dashboard-1440` | 仪表盘 1440 视口适配策略，输出 1920x1080 |
| 2 | `responsive-dashboard-1920` | 仪表盘 1920x1080 |
| 3 | `responsive-screen-4k` | 态势大屏 4K 视口适配策略，输出 1920x1080 |
| 4 | `responsive-alerts-1440` | 告警中心 1440 视口适配策略，输出 1920x1080 |
| 5 | `responsive-alerts-1920` | 告警中心 1920x1080 |
| 6 | `responsive-forensics-1440` | 取证分析 1440 视口适配策略，输出 1920x1080 |
| 7 | `responsive-graph-1440` | 实体图谱 1440 视口适配策略，输出 1920x1080 |
| 8 | `responsive-compliance-1440` | 合规审计 1440 视口适配策略，输出 1920x1080 |
| 9 | `responsive-tablet-dashboard` | 平板仪表盘适配策略，输出 1920x1080 |
| 10 | `responsive-tablet-alert-detail` | 平板告警详情适配策略，输出 1920x1080 |
| 11 | `responsive-mobile-navigation` | 移动端导航抽屉适配策略，输出 1920x1080 |
| 12 | `responsive-mobile-alert-list` | 移动端告警列表适配策略，输出 1920x1080 |

## 10. 推荐生成批次

| 批次 | 数量 | 内容 | 目的 |
|---|---:|---|---|
| P0 | 8 | 视觉基线与规范板 | 先固定风格、字号、色彩、布局，避免后续图漂移 |
| P1 | 27 | 页面主图 | 覆盖现役页面主图；`/topics` 按 Tab 设计输入合并实现 |
| P2 | 22 | 当前已列入 manifest 的基础浮层 | 先复用现有提示词资产 |
| P3 | 30 | 新增业务浮层 | 补齐真实业务闭环 |
| P4 | 48 | 元件与组件板 | 支撑 Figma 组件库和前端组件实现 |
| P5 | 16 | 通用状态图 | 补齐加载、空、错、权限、降级、成功状态 |
| P6 | 12 | 响应式与大屏适配图 | 在统一 1920x1080 输出中支撑桌面、平板、移动端和大屏适配验收 |
| P7 | 18 | 用户确认追加浮层 | 专题、战役/取证导出、合规、审计、通知、设置闭环补齐 |

## 11. 与 manifest 的关系

当前 `manifest.json` 已经是完整 181 张口径，不再停留在早期基础清单或上一版交付基线。后续聊天窗口生图、API 生图、缺口统计和验收均以当前 manifest 为准。

| manifest 类型 | 当前数量 | 归属 |
|---|---:|---|
| foundation | 8 | P0 视觉基线与规范板 |
| page | 27 | P1 页面主图 |
| overlay | 70 | P2/P3/P7 业务浮层 |
| component | 48 | P4 元件与组件板 |
| state | 16 | P5 通用状态图 |
| responsive | 12 | P6 响应式与大屏适配图 |

最终合计：

```text
8 + 27 + 70 + 48 + 16 + 12 = 181
```

## 12. 生成前检查

每张图生成前必须确认：

1. 页面标题、面板标题只使用中文，不再保留英文副标题。
2. 一级菜单固定为：综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置。
3. UI 风格固定为最终参考图的深色安全运营台，不生成营销页、宣传页或泛用后台。
4. 业务数据必须贴合全流量采集分析系统：Probe、Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO、PCAP、MLOps、审计。
5. 危险动作必须出现确认、权限、影响范围和审计提示。
6. 所有组件要可用于 React + Ant Design + ECharts 落地。
7. 生成结果如果改变菜单结构、业务对象、关键字段、风险数值或闭环逻辑，应标记为 rejected，重新生成。
