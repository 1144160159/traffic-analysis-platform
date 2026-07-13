# GPT 生图 UI 视觉套装 v1

本目录用于生成“园区网络全流量采集与分析系统”181 张工业级高保真 UI 套装。视觉基准采用：

- doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-generation-reference.png

像素规范：所有高保真 UI 图一律输出为 `1920x1080 px` PNG。

全局生图约束：后续所有生成、编辑或重生成的 UI 图片，无论是页面、浮层、组件、状态图还是响应式适配图，都必须严格遵循 foundations 的 UI 规范；不得以单张图、局部修图、业务差异或风格自由发挥为由绕过 foundations。

公共 AppShell 绝对一致性硬门禁：除 login.png、screen.png、登录/认证态和明确不展示 AppShell 的独立素材外，所有 UI 图的公共部分必须与态势大屏 screen.png 完全一致。公共部分包括顶部单栏、左侧单栏和底部单栏；三者的内容、图标、顺序、尺寸、间距、分隔线、状态色、字号密度、背景、圆角和激活态都不得按页面自行变化。顶部栏固定为 screen.png 的系统名称、站点/时间、风险态势、告警数、严重告警、采集健康、数据质量和快捷入口结构；快捷入口固定为 PCAP检索、资产检索、规则检索、脚本中心、帮助中心、更多应用。顶部单栏不得加入通知铃铛、用户头像、用户菜单、设置或电源动作组；顶部的告警数/严重告警只是运行指标，不是通知中心入口。左侧栏固定为 screen.png 的单栏展开式菜单，一级菜单图标和二级菜单图标必须遵循 doc/04_assets/ui_suite_gpt_v1/standards/APP_SHELL_ICON_STANDARD.md；不同页面只允许改变当前展开域、二级菜单文本和当前高亮项；用户身份、角色、在线状态和用户动作只归属左侧底部用户区，顶部不得重复展示。底部单栏固定为 screen.png 的数据延迟 / 系统运行 / 告警处理SLA / 数据质量合格率 / 存储使用 / 带宽使用 / 日志吞吐 / 右侧全局动作图标组；通知角标、设置、全局配置和电源只归属底部右侧全局动作区。修复既有 UI 图时，只允许修改顶部、左侧、底部公共区域，中部业务内容区必须保持原图不变，不得重绘、替换指标、调整业务面板或改变业务布局。

全局左侧菜单约束：除登录/认证页、态势大屏基准图和移动端导航抽屉等明确例外外，所有出现左侧菜单的 UI 图片都必须与态势大屏 screen.png 的左侧单栏完全一致；同一个侧栏内承载一级菜单和当前一级业务域下的二级菜单，禁止恢复“窄一级栏 + 独立二级栏”的双栏结构。菜单整体宽度、背景、图标尺寸、文字密度、分割线、底部用户区和激活蓝色样式必须复刻 screen.png；一级菜单固定为“综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置”，二级菜单只能显示当前业务域页面，二级菜单图标按 doc/04_assets/ui_suite_gpt_v1/standards/APP_SHELL_ICON_STANDARD.md 固定；禁止新增第三层菜单，禁止把二级菜单画成厚重卡片、巨大按钮、工具面板或专题目录，禁止把页面内部模块塞进左侧菜单。

## 范围

- 总图数：181
- 视觉基线与规范板：8
- 页面主图：27
- 业务浮层图：70
- 元件与组件板：48
- 通用状态图：16
- 响应式与大屏适配图：12

## 当前状态

- 已生成 181 个 GPT 生图 prompt：`prompts/*.prompt.txt`
- 已生成本地生成清单：`manifest.json`
- 图片目标目录：`screens/foundations`、`screens/pages`、`screens/overlays`、`screens/components`、`screens/states`、`screens/responsive`
- P7 扩容 18 张已于 2026-06-28 使用内置 `image_gen.imagegen` 逐张生成并提取落盘；manifest 目标文件 181/181 存在。
- `run_generation.sh` 仅用于未来单张重生、质量返工或 API 批量再生成；本轮交付图已完成。

## 生成命令

```bash
cd /home/wangwt/phase_2/code/traffic-analysis-platform
QUALITY=medium SIZE=1920x1080 bash doc/04_assets/ui_suite_gpt_v1/run_generation.sh
```

可选参数：

- `ONLY_ID=alerts`：只生成指定条目
- `LIMIT=3`：只生成前 3 张
- `START_AT=rules`：从指定条目开始恢复
- `DRY_RUN=1`：只打印计划，不调用 API

## 视觉基线与规范板

| # | ID | 名称 | 目标文件 |
| - | - | - | - |
| 1 | foundation-visual-reference | 最终视觉基准板 | doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-visual-reference.png |
| 2 | foundation-layout-grid | 布局与栅格规范 | doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-layout-grid.png |
| 3 | foundation-color-status | 色彩与状态语义 | doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-color-status.png |
| 4 | foundation-typography-density | 字体与密度规范 | doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-typography-density.png |
| 5 | foundation-icons-actions | 图标与动作语义 | doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-icons-actions.png |
| 6 | foundation-data-viz | 数据可视化规范 | doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-data-viz.png |
| 7 | foundation-table-form | 表格与表单密度规范 | doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-table-form.png |
| 8 | foundation-responsive | 响应式适配原则 | doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-responsive.png |

## 页面清单

| # | ID | 页面 | 路由 | 目标文件 |
| - | - | - | - | - |
| 1 | login | 登录 | /login | doc/04_assets/ui_suite_gpt_v1/screens/pages/login.png |
| 2 | screen | 态势大屏 | /screen | doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png |
| 3 | dashboard | 仪表盘 | /dashboard | doc/04_assets/ui_suite_gpt_v1/screens/pages/dashboard.png |
| 4 | alerts | 告警中心 | /alerts | doc/04_assets/ui_suite_gpt_v1/screens/pages/alerts.png |
| 5 | alert-detail | 告警详情 | /alerts/:alertId | doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail.png |
| 6 | campaigns | 战役列表 | /campaigns | doc/04_assets/ui_suite_gpt_v1/screens/pages/campaigns.png |
| 7 | campaign-detail | 战役详情 | /campaigns/:campaignId | doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail.png |
| 8 | attack-chains | 攻击链分析 | /attack-chains | doc/04_assets/ui_suite_gpt_v1/screens/pages/attack-chains.png |
| 9 | encrypted-traffic | 加密流量 | /encrypted-traffic | doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic.png |
| 10 | forensics | 取证分析 | /forensics | doc/04_assets/ui_suite_gpt_v1/screens/pages/forensics.png |
| 11 | assets | 资产台账 | /assets | doc/04_assets/ui_suite_gpt_v1/screens/pages/assets.png |
| 12 | graph | 实体图谱 | /graph | doc/04_assets/ui_suite_gpt_v1/screens/pages/graph.png |
| 13 | fusion | 数据融合 | /fusion | doc/04_assets/ui_suite_gpt_v1/screens/pages/fusion.png |
| 14 | baselines | 行为基准 | /baselines | doc/04_assets/ui_suite_gpt_v1/screens/pages/baselines.png |
| 15 | probes | 探针管理 | /probes | doc/04_assets/ui_suite_gpt_v1/screens/pages/probes.png |
| 16 | rules | 规则管理 | /rules | doc/04_assets/ui_suite_gpt_v1/screens/pages/rules.png |
| 17 | deployments | 部署管理 | /deployments | doc/04_assets/ui_suite_gpt_v1/screens/pages/deployments.png |
| 18 | models | 模型管理 | /models | doc/04_assets/ui_suite_gpt_v1/screens/pages/models.png |
| 19 | mlops | MLOps 编排 | /mlops | doc/04_assets/ui_suite_gpt_v1/screens/pages/mlops.png |
| 20 | data-quality | 数据质量 | /data-quality | doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality.png |
| 21 | playbooks | SOAR 剧本 | /playbooks | doc/04_assets/ui_suite_gpt_v1/screens/pages/playbooks.png |
| 22 | whitelist | 白名单 | /whitelist | doc/04_assets/ui_suite_gpt_v1/screens/pages/whitelist.png |
| 23 | compliance | 合规审计 | /compliance | doc/04_assets/ui_suite_gpt_v1/screens/pages/compliance.png |
| 24 | audit-log | 审计日志 | /audit-log | doc/04_assets/ui_suite_gpt_v1/screens/pages/audit-log.png |
| 25 | notifications | 通知配置 | /notifications | doc/04_assets/ui_suite_gpt_v1/screens/pages/notifications.png |
| 26 | settings | 系统设置 | /settings | doc/04_assets/ui_suite_gpt_v1/screens/pages/settings.png |
| 27 | not-found | 404 异常页 | * | doc/04_assets/ui_suite_gpt_v1/screens/pages/not-found.png |

Tab 拆分设计合并口径：`/topics` 是当前唯一现役专题页面和左侧菜单项；加密隧道、数据外传、APT 战役已有拆分 Tab 设计输入，前端开发时必须合并到同一个 `/topics` 页面内作为页内 Tab/Segmented 状态实现，不再生成单张 `topics.png`。旧 `/topics/tunnel`、`/topics/exfil`、`/topics/apt` 只作为兼容深链或 API 语义来源，不进入页面主图清单。后续任何页面如果 UI 设计图按 Tab 拆多张，前端也必须合并为一个路由页面内的 Tab 状态，不能拆成多个左侧菜单或独立业务路由。

## 浮层清单

| # | ID | 浮层 | 基准页面 | 目标文件 |
| - | - | - | - | - |
| 1 | dropdown-user-menu | 用户下拉菜单 | settings | doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-user-menu.png |
| 2 | drawer-mobile-navigation | 移动端侧滑菜单 | dashboard | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-mobile-navigation.png |
| 3 | drawer-notification-center | 通知中心抽屉 | notifications | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-notification-center.png |
| 4 | modal-global-search | 全局搜索弹窗 | dashboard | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-global-search.png |
| 5 | dropdown-quick-entry | 快速入口下拉 | dashboard | doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-quick-entry.png |
| 6 | modal-login-error-captcha | 登录异常与验证码状态 | login | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-login-error-captcha.png |
| 7 | drawer-dashboard-kpi-detail | 仪表盘 KPI 详情 | dashboard | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-kpi-detail.png |
| 8 | drawer-dashboard-task-detail | 待办任务详情 | dashboard | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-task-detail.png |
| 9 | modal-screen-readonly-token | 态势大屏只读令牌/脱敏配置 | screen | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-screen-readonly-token.png |
| 10 | drawer-probe-detail | 探针详情 | probes | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-detail.png |
| 11 | modal-probe-config | 探针配置下发 | probes | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-config.png |
| 12 | modal-probe-batch-upgrade | 探针批量升级确认 | probes | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-batch-upgrade.png |
| 13 | modal-probe-cert-rotate | 证书轮换确认 | probes | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-cert-rotate.png |
| 14 | drawer-probe-log | 探针日志抽屉 | probes | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-log.png |
| 15 | drawer-dlq-sample | DLQ 样例详情 | data-quality | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dlq-sample.png |
| 16 | modal-data-replay-task | 数据重放任务 | data-quality | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-data-replay-task.png |
| 17 | drawer-field-quality-sample | 字段质量样例 | data-quality | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-field-quality-sample.png |
| 18 | modal-alert-batch | 告警批量操作确认 | alerts | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-batch.png |
| 19 | dropdown-alert-batch-actions | 告警批量操作下拉 | alerts | doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-batch-actions.png |
| 20 | dropdown-alert-row-actions | 告警行操作下拉 | alerts | doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-row-actions.png |
| 21 | modal-alert-status | 更新告警状态 | alert-detail | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-status.png |
| 22 | modal-alert-feedback | 提交告警反馈 | alert-detail | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-feedback.png |
| 23 | modal-evidence-detail | 证据详情 | alert-detail | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-evidence-detail.png |
| 24 | modal-forensics-task | 取证任务详情 | forensics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-forensics-task.png |
| 25 | drawer-campaign-detail | 战役详情抽屉 | campaigns | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-campaign-detail.png |
| 26 | drawer-attack-chain-detail | 攻击链详情抽屉 | attack-chains | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-attack-chain-detail.png |
| 27 | drawer-encrypted-fingerprint | 加密指纹详情 | encrypted-traffic | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-encrypted-fingerprint.png |
| 28 | drawer-certificate-detail | 证书详情 | encrypted-traffic | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-certificate-detail.png |
| 29 | modal-playbook-trigger | 从告警触发剧本 | alert-detail | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-playbook-trigger.png |
| 30 | modal-whitelist-draft-from-alert | 从告警生成白名单草案 | alert-detail | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-whitelist-draft-from-alert.png |
| 31 | popconfirm-pcap-download | PCAP 下载确认 | forensics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-pcap-download.png |
| 32 | drawer-session-replay | 会话复放抽屉 | forensics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-session-replay.png |
| 33 | drawer-asset-detail | 资产详情 | assets | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-asset-detail.png |
| 34 | modal-asset-edit | 编辑资产 | assets | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-asset-edit.png |
| 35 | drawer-asset-history | 资产历史 | assets | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-asset-history.png |
| 36 | drawer-graph-entity | 图谱实体详情 | graph | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-graph-entity.png |
| 37 | drawer-graph-path-analysis | 图谱路径分析 | graph | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-graph-path-analysis.png |
| 38 | drawer-fusion-conflict | 数据融合冲突处理 | fusion | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-fusion-conflict.png |
| 39 | modal-fusion-rule-edit | 融合规则编辑 | fusion | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-fusion-rule-edit.png |
| 40 | modal-baseline-threshold | 基线阈值编辑 | baselines | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-baseline-threshold.png |
| 41 | modal-rule-edit | 新建/编辑规则 | rules | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-rule-edit.png |
| 42 | drawer-rule-detail | 规则详情 | rules | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-rule-detail.png |
| 43 | popconfirm-delete | 规则删除确认 | rules | doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-delete.png |
| 44 | modal-rule-publish | 规则发布确认 | rules | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-rule-publish.png |
| 45 | modal-deployment-create | 新建部署 | deployments | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-deployment-create.png |
| 46 | modal-deployment-rollback | 回滚部署确认 | deployments | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-deployment-rollback.png |
| 47 | drawer-model-detail | 模型详情 | models | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-model-detail.png |
| 48 | drawer-mlops-task-detail | MLOps 任务详情 | mlops | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-mlops-task-detail.png |
| 49 | modal-playbook-edit | 剧本编辑 | playbooks | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-playbook-edit.png |
| 50 | modal-whitelist-add | 添加白名单 | whitelist | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-whitelist-add.png |
| 51 | drawer-whitelist-approval | 白名单审批详情 | whitelist | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-whitelist-approval.png |
| 52 | modal-settings-token | 创建 API 令牌 | settings | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-settings-token.png |
| 53 | modal-topic-save-view | 专题保存视图 | topics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-topic-save-view.png |
| 54 | drawer-topic-scope-edit | 专题范围编辑 | topics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-topic-scope-edit.png |
| 55 | modal-topic-report-export | 专题报告导出 | topics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-topic-report-export.png |
| 56 | modal-topic-evidence-package-export | 专题证据包导出 | topics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-topic-evidence-package-export.png |
| 57 | drawer-topic-subscription | 专题订阅配置 | topics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-topic-subscription.png |
| 58 | dropdown-topic-share-favorite | 专题分享收藏菜单 | topics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-topic-share-favorite.png |
| 59 | modal-campaign-report-export | 战役报告导出 | campaign-detail | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-campaign-report-export.png |
| 60 | modal-forensics-evidence-export | 取证证据导出 | forensics | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-forensics-evidence-export.png |
| 61 | drawer-compliance-gate-detail | 合规门禁详情 | compliance | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-compliance-gate-detail.png |
| 62 | modal-compliance-evidence-package-export | 合规证据包导出 | compliance | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-compliance-evidence-package-export.png |
| 63 | modal-compliance-report-export | 合规运行报告导出 | compliance | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-compliance-report-export.png |
| 64 | drawer-audit-operation-detail | 审计操作详情 | audit-log | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-audit-operation-detail.png |
| 65 | modal-audit-export | 审计材料导出 | audit-log | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-audit-export.png |
| 66 | modal-notification-channel-edit | 通知渠道编辑 | notifications | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-notification-channel-edit.png |
| 67 | modal-notification-template-preview-test | 通知模板预览测试 | notifications | doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-notification-template-preview-test.png |
| 68 | drawer-notification-silence-rule | 通知静默规则 | notifications | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-notification-silence-rule.png |
| 69 | popconfirm-settings-token-revoke | API 令牌吊销确认 | settings | doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-settings-token-revoke.png |
| 70 | drawer-settings-rbac-edit | RBAC 权限编辑 | settings | doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-settings-rbac-edit.png |

## 元件与组件板

| # | ID | 名称 | 目标文件 |
| - | - | - | - |
| 1 | component-app-header | 顶部状态栏 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-app-header.png |
| 2 | component-primary-sidebar | 左侧一级导航 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-primary-sidebar.png |
| 3 | component-secondary-menu | 二级菜单 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-secondary-menu.png |
| 4 | component-bottom-status-bar | 底部状态栏 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-bottom-status-bar.png |
| 5 | component-breadcrumb-context | 面包屑与上下文条 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-breadcrumb-context.png |
| 6 | component-site-time-selector | 站点与时间选择 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-site-time-selector.png |
| 7 | component-quick-entry | 快速入口 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-quick-entry.png |
| 8 | component-user-menu | 用户菜单 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-user-menu.png |
| 9 | component-button | 按钮 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-button.png |
| 10 | component-icon-button | 图标按钮 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-icon-button.png |
| 11 | component-status-chip | 标签与 Badge | doc/04_assets/ui_suite_gpt_v1/screens/components/component-status-chip.png |
| 12 | component-tooltip | Tooltip | doc/04_assets/ui_suite_gpt_v1/screens/components/component-tooltip.png |
| 13 | component-tabs | Tabs | doc/04_assets/ui_suite_gpt_v1/screens/components/component-tabs.png |
| 14 | component-segmented | Segmented | doc/04_assets/ui_suite_gpt_v1/screens/components/component-segmented.png |
| 15 | component-dropdown | Dropdown | doc/04_assets/ui_suite_gpt_v1/screens/components/component-dropdown.png |
| 16 | component-pagination | Pagination | doc/04_assets/ui_suite_gpt_v1/screens/components/component-pagination.png |
| 17 | component-input | 输入框 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-input.png |
| 18 | component-search | 搜索框 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-search.png |
| 19 | component-select | 选择器 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-select.png |
| 20 | component-date-range | 日期时间窗 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-date-range.png |
| 21 | component-switch-checkbox-radio | 开关/复选/单选 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-switch-checkbox-radio.png |
| 22 | component-condition-builder | 条件构造器 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-condition-builder.png |
| 23 | component-batch-action-bar | 批量操作栏 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-batch-action-bar.png |
| 24 | component-data-table | 高密度表格 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-data-table.png |
| 25 | component-description-list | 详情描述列表 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-description-list.png |
| 26 | component-kpi-tile | KPI 卡 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-kpi-tile.png |
| 27 | component-health-card | 状态卡 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-health-card.png |
| 28 | component-ranking-list | 排行列表 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-ranking-list.png |
| 29 | component-log-list | 日志列表 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-log-list.png |
| 30 | component-evidence-file-card | 证据文件卡 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-evidence-file-card.png |
| 31 | component-empty-card | 空状态卡 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-empty-card.png |
| 32 | component-permission-card | 权限提示卡 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-permission-card.png |
| 33 | component-line-area-chart | 折线/面积图 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-line-area-chart.png |
| 34 | component-donut-chart | 环图 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-donut-chart.png |
| 35 | component-bar-ranking-chart | 柱状/排行图 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-bar-ranking-chart.png |
| 36 | component-sankey-flow | 桑基流向图 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-sankey-flow.png |
| 37 | component-radar-quality | 雷达/质量评分 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-radar-quality.png |
| 38 | component-heatmap | 热力图 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-heatmap.png |
| 39 | component-topology-graph | 拓扑/图谱 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-topology-graph.png |
| 40 | component-timeline-state-machine | 时间线/状态机 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-timeline-state-machine.png |
| 41 | component-alert-queue | 告警队列 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-alert-queue.png |
| 42 | component-risk-score | 风险评分 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-risk-score.png |
| 43 | component-alert-timeline | 告警时间线 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-alert-timeline.png |
| 44 | component-evidence-drawer | 证据抽屉 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-evidence-drawer.png |
| 45 | component-asset-context | 资产上下文 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-asset-context.png |
| 46 | component-action-rail | 响应动作栏 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-action-rail.png |
| 47 | component-feedback-block | 反馈学习块 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-feedback-block.png |
| 48 | component-acceptance-gate-matrix | 验收门禁矩阵 | doc/04_assets/ui_suite_gpt_v1/screens/components/component-acceptance-gate-matrix.png |

## 通用状态图

| # | ID | 名称 | 目标文件 |
| - | - | - | - |
| 1 | state-page-loading | 页面加载中 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-page-loading.png |
| 2 | state-table-loading | 表格加载中 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-table-loading.png |
| 3 | state-chart-loading | 图表加载中 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-chart-loading.png |
| 4 | state-empty-page | 页面空数据 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-empty-page.png |
| 5 | state-empty-table | 表格空数据 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-empty-table.png |
| 6 | state-empty-chart | 图表空数据 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-empty-chart.png |
| 7 | state-api-error | API 错误 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-api-error.png |
| 8 | state-network-error | 网络异常 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-network-error.png |
| 9 | state-unauthorized | 未登录 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-unauthorized.png |
| 10 | state-forbidden | 无权限 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-forbidden.png |
| 11 | state-partial-degraded | 部分降级 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-partial-degraded.png |
| 12 | state-offline-probe | 探针离线 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-offline-probe.png |
| 13 | state-stream-backpressure | 流处理背压 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-stream-backpressure.png |
| 14 | state-task-running | 任务运行中 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-task-running.png |
| 15 | state-task-failed | 任务失败可重试 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-task-failed.png |
| 16 | state-success-accepted | 验收通过/操作成功 | doc/04_assets/ui_suite_gpt_v1/screens/states/state-success-accepted.png |

## 响应式与大屏适配图

| # | ID | 名称 | 目标文件 |
| - | - | - | - |
| 1 | responsive-dashboard-1440 | 仪表盘 1440 视口适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-dashboard-1440.png |
| 2 | responsive-dashboard-1920 | 仪表盘 1920x1080 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-dashboard-1920.png |
| 3 | responsive-screen-4k | 态势大屏 4K 视口适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-screen-4k.png |
| 4 | responsive-alerts-1440 | 告警中心 1440 视口适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-alerts-1440.png |
| 5 | responsive-alerts-1920 | 告警中心 1920x1080 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-alerts-1920.png |
| 6 | responsive-forensics-1440 | 取证分析 1440 视口适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-forensics-1440.png |
| 7 | responsive-graph-1440 | 实体图谱 1440 视口适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-graph-1440.png |
| 8 | responsive-compliance-1440 | 合规审计 1440 视口适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-compliance-1440.png |
| 9 | responsive-tablet-dashboard | 平板仪表盘适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-tablet-dashboard.png |
| 10 | responsive-tablet-alert-detail | 平板告警详情适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-tablet-alert-detail.png |
| 11 | responsive-mobile-navigation | 移动端导航抽屉适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-mobile-navigation.png |
| 12 | responsive-mobile-alert-list | 移动端告警列表适配策略 | doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-mobile-alert-list.png |
