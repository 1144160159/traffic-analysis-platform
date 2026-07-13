# Overlay / Component Imagegen Progress

更新日期：2026-06-28

本文记录 Tab 补图完成后的 manifest 队列继续生成进度，覆盖 overlay、component、state、responsive。完整清单仍以 `manifest.json` 和 `PAGE_OVERLAY_COMPONENT_GAP_INVENTORY.md` 为准。

## 生成口径

- 浮层、弹窗、抽屉、下拉、确认框不再绘制公共 AppShell、宿主页面背景、顶部栏、左侧菜单或底部栏。
- 每张 overlay 只画当前业务交互容器本体，公共区域只作为颜色、字号、密度、状态语义和图标风格参考。
- 默认每张生成后立即运行 `extract_latest_imagegen.py <targetFile>`，保存最终 `1920x1080` PNG 和 `*.raw-imagegen.png`；严格 AppShell 组件可使用 `screen.png` 确定性裁切重组，并保留 `*.raw-deterministic.png`。
- 每张生成后同步 `GENERATION_STATE.md`、`CONTEXT_HANDOFF.md` 和 `PAGE_OVERLAY_COMPONENT_GAP_INVENTORY.md`。

## 当前统计

| 范围 | 计划 | 已完成 | 还缺 |
|---|---:|---:|---:|
| overlay | 52 | 52 | 0 |
| component | 48 | 48 | 0 |
| state | 16 | 16 | 0 |
| responsive | 12 | 12 | 0 |
| P7 overlay 扩容 | 18 | 18 | 0 |
| 合计 | 146 | 146 | 0 |

## 业务合理性返工

- 2026-06-28：基于 `screens/` contact sheet 复核业务语义，已返工 `screens/states/` 16 张和 `screens/responsive/` 12 张，共 28 张。
- 状态图修复重点：加载、空态、API 错误、网络错误、401、403、部分降级、探针离线、流处理背压、任务运行/失败/受理成功不再复用同一套泛化错误模板；401 与 403 的主动作已明确分离。
- 响应式图修复重点：12 张断点图已绑定具体页面与业务优先级，不再只是空白线框；每张说明核心业务保留、次要区域折叠、危险动作位置和上下文传递方式。
- 返工前文件保留为 `*.before-business-fix.png`，返工后追溯文件保留为 `*.raw-deterministic.png`；完整审计记录见 `doc/04_assets/ui_suite_gpt_v1/BUSINESS_REASONABILITY_AUDIT.md`。

## 已完成

| 序号 | ID | 类型 | 文件 | 备注 |
|---:|---|---|---|---|
| 1 | `dropdown-user-menu` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-user-menu.png` | 历史已完成项 |
| 2 | `drawer-mobile-navigation` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-mobile-navigation.png` | 2026-06-27 按新口径生成，仅包含移动端侧滑导航抽屉本体 |
| 3 | `drawer-notification-center` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-notification-center.png` | 2026-06-27 按新口径生成，仅包含通知中心抽屉本体 |
| 4 | `modal-global-search` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-global-search.png` | 2026-06-27 按新口径生成，仅包含全局搜索弹窗本体 |
| 5 | `dropdown-quick-entry` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-quick-entry.png` | 2026-06-27 按新口径生成，仅包含快速入口下拉本体 |
| 6 | `modal-login-error-captcha` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-login-error-captcha.png` | 2026-06-27 按新口径生成，仅包含登录验证弹窗本体，低暴露 |
| 7 | `drawer-dashboard-kpi-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-kpi-detail.png` | 2026-06-27 按新口径生成，仅包含 KPI 详情抽屉本体 |
| 8 | `drawer-dashboard-task-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-task-detail.png` | 2026-06-27 按新口径生成，仅包含待办任务详情抽屉本体 |
| 9 | `modal-screen-readonly-token` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-screen-readonly-token.png` | 2026-06-27 按新口径生成，仅包含只读访问令牌弹窗本体 |
| 10 | `drawer-probe-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-detail.png` | 2026-06-27 按新口径生成，仅包含探针详情抽屉本体 |
| 11 | `modal-probe-config` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-config.png` | 2026-06-27 按新口径生成，仅包含探针配置下发弹窗本体 |
| 12 | `modal-probe-batch-upgrade` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-batch-upgrade.png` | 2026-06-27 按新口径生成，仅包含探针批量升级确认弹窗本体 |
| 13 | `modal-probe-cert-rotate` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-cert-rotate.png` | 2026-06-27 按新口径生成，仅包含探针证书轮换弹窗本体 |
| 14 | `drawer-probe-log` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-log.png` | 2026-06-27 按新口径生成，仅包含探针日志抽屉本体 |
| 15 | `drawer-dlq-sample` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dlq-sample.png` | 2026-06-27 按新口径生成，仅包含 DLQ 样例详情抽屉本体 |
| 16 | `modal-data-replay-task` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-data-replay-task.png` | 2026-06-27 按新口径生成，仅包含数据重放任务弹窗本体 |
| 17 | `drawer-field-quality-sample` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-field-quality-sample.png` | 2026-06-27 按新口径生成，仅包含字段质量样例抽屉本体 |
| 18 | `modal-alert-batch` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-batch.png` | 2026-06-27 按新口径生成，仅包含告警批量操作确认弹窗本体 |
| 19 | `dropdown-alert-batch-actions` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-batch-actions.png` | 2026-06-27 按新口径生成，仅包含告警批量操作下拉本体 |
| 20 | `dropdown-alert-row-actions` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-row-actions.png` | 2026-06-27 按新口径生成，仅包含告警行操作下拉本体 |
| 21 | `modal-alert-status` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-status.png` | 2026-06-27 按新口径生成，仅包含更新告警状态弹窗本体 |
| 22 | `modal-alert-feedback` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-feedback.png` | 2026-06-27 按新口径生成，仅包含提交告警反馈弹窗本体 |
| 23 | `modal-evidence-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-evidence-detail.png` | 2026-06-27 按新口径生成，仅包含证据详情弹窗本体 |
| 24 | `modal-playbook-trigger` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-playbook-trigger.png` | 2026-06-27 按新口径生成，仅包含触发 SOAR 剧本弹窗本体 |
| 25 | `modal-whitelist-draft-from-alert` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-whitelist-draft-from-alert.png` | 2026-06-27 按新口径生成，仅包含从告警生成白名单草案弹窗本体 |
| 26 | `popconfirm-pcap-download` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-pcap-download.png` | 2026-06-27 按新口径生成，仅包含 PCAP 下载确认浮层本体 |
| 27 | `drawer-session-replay` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-session-replay.png` | 2026-06-27 按新口径生成，仅包含会话复放抽屉本体 |
| 28 | `drawer-asset-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-asset-detail.png` | 2026-06-27 按用户反馈重生，仅包含资产详情父容器、小 Tab 导航和轻摘要，不展开小 Tab 完整内容 |
| 29 | `modal-asset-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-asset-edit.png` | 2026-06-27 按新口径生成，仅包含资产编辑弹窗本体、变更 Diff、影响门禁和审计动作 |
| 30 | `drawer-asset-history` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-asset-history.png` | 2026-06-27 按新口径生成，仅包含资产历史抽屉本体、变更时间线、字段 Diff、影响范围和审批门禁 |
| 31 | `drawer-graph-entity` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-graph-entity.png` | 2026-06-27 按新口径生成，仅包含图谱实体详情抽屉本体、局部关系预览、关联边、最近会话和权限门禁 |
| 32 | `drawer-graph-path-analysis` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-graph-path-analysis.png` | 2026-06-27 按新口径生成，仅包含图谱路径分析抽屉本体、路径候选、边证据、影响范围和审计授权 |
| 33 | `drawer-fusion-conflict` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-fusion-conflict.png` | 2026-06-27 按新口径生成，仅包含数据融合冲突处理抽屉本体、多源可信度、字段候选、决策建议和审批门禁 |
| 34 | `modal-fusion-rule-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-fusion-rule-edit.png` | 2026-06-27 按新口径生成，仅包含融合规则编辑 Modal 本体、权限门禁、影响范围、状态解释和审计留痕 |
| 35 | `modal-baseline-threshold` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-baseline-threshold.png` | 2026-06-27 按新口径生成，仅包含基线阈值编辑 Modal 本体、阈值参数、回放评估、影响范围和审计留痕 |
| 36 | `modal-forensics-task` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-forensics-task.png` | 2026-06-27 按新口径生成，仅包含取证任务详情 Modal 本体、切片范围、证据输出、处理状态和审计留痕 |
| 37 | `drawer-campaign-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-campaign-detail.png` | 2026-06-27 按新口径生成，仅包含战役详情 Drawer 本体、聚类原因、攻击阶段、证据、影响范围和处置建议 |
| 38 | `drawer-attack-chain-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-attack-chain-detail.png` | 2026-06-27 按新口径生成，仅包含攻击链详情 Drawer 本体、节点证据、命中规则、关联资产和下一步调查 |
| 39 | `drawer-encrypted-fingerprint` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-encrypted-fingerprint.png` | 2026-06-27 按新口径生成，仅包含加密指纹详情 Drawer 本体、TLS/JA3/JA4、证书链、相似样本和风险解释 |
| 40 | `drawer-certificate-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-certificate-detail.png` | 2026-06-27 按新口径生成，仅包含证书详情 Drawer 本体、证书链、风险检查、相似样本和影响范围 |
| 41 | `modal-rule-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-rule-edit.png` | 2026-06-27 按新口径生成，仅包含规则编辑 Modal 本体、规则 DSL、测试门禁、版本 Diff 和审批审计 |
| 42 | `drawer-rule-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-rule-detail.png` | 2026-06-28 按新口径生成，仅包含规则详情 Drawer 本体、生命周期、版本历史、命中趋势和关联模型 |
| 43 | `popconfirm-delete` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-delete.png` | 2026-06-28 按新口径生成，仅包含规则删除确认 Popconfirm 本体、影响范围、权限提示和审计原因 |
| 44 | `modal-rule-publish` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-rule-publish.png` | 2026-06-28 按新口径生成，仅包含规则发布 Modal 本体、发布前检查、影响预估、审批链和回滚策略 |
| 45 | `modal-deployment-create` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-deployment-create.png` | 2026-06-28 按新口径生成，仅包含创建部署 Modal 本体、能力包版本、目标部署集、灰度策略和预检查 |
| 46 | `modal-deployment-rollback` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-deployment-rollback.png` | 2026-06-28 按新口径生成，仅包含部署回滚 Modal 本体、目标版本、回滚检查、观测窗口和审批审计 |
| 47 | `drawer-model-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-model-detail.png` | 2026-06-28 按新口径生成，仅包含模型详情 Drawer 本体、评估指标、特征解释、激活状态和回滚审计 |
| 48 | `drawer-mlops-task-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-mlops-task-detail.png` | 2026-06-28 按新口径生成，仅包含 MLOps 任务详情 Drawer 本体、任务 DAG、指标、日志、产物和发布门禁 |
| 49 | `modal-playbook-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-playbook-edit.png` | 2026-06-28 按新口径生成，仅包含 SOAR 剧本编辑 Modal 本体、节点编排、参数映射、风险控制和测试审计 |
| 50 | `modal-whitelist-add` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-whitelist-add.png` | 2026-06-28 按新口径生成，仅包含新增白名单 Modal 本体、条件构造、生效策略、风险评估和审批审计 |
| 51 | `drawer-whitelist-approval` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-whitelist-approval.png` | 2026-06-28 按新口径生成，仅包含白名单审批详情 Drawer 本体、审批流程、命中证据、风险解释和到期治理 |
| 52 | `modal-settings-token` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-settings-token.png` | 2026-06-28 按新口径生成，仅包含 API 令牌管理 Modal 本体、权限 scope、脱敏 token、轮换吊销风险和审计留痕 |
| 53 | `component-app-header` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-app-header.png` | 2026-06-28 已按当前 `screen.png` 返工为确定性规格板；顶部只展示产品标题、站点/时间、风险/告警/健康/质量指标和六个快捷入口，不包含通知铃铛、用户头像、用户菜单、设置或电源动作组 |
| 54 | `component-primary-sidebar` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-primary-sidebar.png` | 2026-06-28 已按当前 `screen.png` 返工为确定性规格板；左侧 166px 单栏保留底部用户区，并明确其为唯一常驻用户身份/角色/在线状态区域，顶部不得重复用户/通知动作组 |
| 55 | `component-secondary-menu` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-secondary-menu.png` | 2026-06-28 按当前 `screen.png` 确定性裁切重组，展示二级菜单在同一 166px 左侧单栏内展开、六个业务域固定二级项、状态/尺寸和禁止独立二级栏/第三层菜单的验收口径 |
| 56 | `component-bottom-status-bar` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-bottom-status-bar.png` | 2026-06-28 按当前 `screen.png` 确定性裁切重组，展示底部状态栏 y=997 / h=83、七个运行状态项和底部右侧全局动作区，明确通知/设置/电源不得上移到顶部 |
| 57 | `component-breadcrumb-context` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-breadcrumb-context.png` | 2026-06-28 按当前 `screen.png` 与 foundations token 确定性绘制，展示业务内容区面包屑/上下文条、对象 ID、上下文 Chip、状态和职责边界，明确不得进入顶部状态栏或替代左侧菜单 |
| 58 | `component-site-time-selector` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-site-time-selector.png` | 2026-06-28 按当前 `screen.png` 顶部站点/时间裁切确定性绘制，展示顶部 80px 内站点选择、时间窗、刷新/NTP 状态和职责边界，明确不得混入通知、用户、设置、电源或页面业务筛选 |
| 59 | `component-quick-entry` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-quick-entry.png` | 2026-06-28 按当前 `screen.png` 顶部右侧六个快捷入口确定性绘制，明确 quick-entry 只覆盖 PCAP/资产/规则/脚本/帮助/更多应用，不包含通知、用户、设置、全局配置或电源 |
| 60 | `component-user-menu` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-user-menu.png` | 2026-06-28 按当前 `screen.png` 左侧底部用户卡确定性绘制，明确用户菜单只从左下用户区触发，顶部不得重复用户头像、用户名、用户组或个人菜单 |
| 61 | `component-button` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-button.png` | 2026-06-28 按 foundations token 确定性绘制，展示按钮类型、尺寸、状态、危险动作和审计门禁，不绘制完整 AppShell |
| 62 | `component-icon-button` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-icon-button.png` | 2026-06-28 按 foundations token 确定性绘制，展示图标按钮集合、tooltip、状态矩阵、表格操作列和全局动作归属边界 |
| 63 | `component-status-chip` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-status-chip.png` | 2026-06-28 按 foundations token 确定性绘制，展示状态标签、风险 Badge、计数徽标、业务对象 Tag、交互状态、颜色语义锁定和 Ant Design 映射 |
| 64 | `component-tooltip` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-tooltip.png` | 2026-06-28 按 foundations token 确定性绘制，展示 Tooltip placement、字段解释、权限/风险/校验提示、Popover 边界、disabled wrapper 和危险确认边界 |
| 65 | `component-tabs` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-tabs.png` | 2026-06-28 按 foundations token 确定性绘制，展示横向 Tabs、Card Tabs、业务小 Tab、Badge、状态矩阵、稳定内容区和不新增路由/菜单边界 |
| 66 | `component-segmented` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-segmented.png` | 2026-06-28 按 foundations token 确定性绘制，展示专题模式、视图密度、时间粒度、风险级别、状态矩阵和局部轻量互斥切换边界 |
| 67 | `component-dropdown` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-dropdown.png` | 2026-06-28 按 foundations token 确定性绘制，展示行操作、批量操作、快速入口、分组/二级菜单、状态矩阵、局部菜单边界和危险动作确认规则 |
| 68 | `component-pagination` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-pagination.png` | 2026-06-28 按 foundations token 确定性绘制，展示基础分页、表格底部分页、pageSize、跳页、服务端分页、游标分页、状态矩阵和性能提示 |
| 69 | `component-input` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-input.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示字段输入、状态矩阵、校验帮助、前后缀、脱敏、单位、React 映射和危险变更审计边界 |
| 70 | `component-search` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-search.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示本地/服务端/实体/审计/PCAP 搜索、建议层、筛选 chip、查询状态矩阵、脱敏命中结果和 React 映射 |
| 71 | `component-select` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-select.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示单选、多选、分组选项、远程搜索、长列表、受限选项、状态矩阵和高影响选择审计边界 |
| 72 | `component-date-range` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-date-range.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示绝对/相对时间、快捷窗口、双月面板、时区/延迟、业务查询时间窗、高成本查询预估和权限审计门禁 |
| 73 | `component-switch-checkbox-radio` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-switch-checkbox-radio.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示 Switch、Checkbox、Radio、半选树、互斥策略、状态矩阵和高影响开关审计边界 |
| 74 | `component-condition-builder` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-condition-builder.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示条件组、AND/OR 逻辑、字段类型、嵌套条件、拖拽编辑、命中预估、状态校验和权限审计门禁 |
| 75 | `component-batch-action-bar` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-batch-action-bar.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示选中计数、跨页全选、筛选范围快照、动作分组、危险批量动作确认、权限影响和审计追踪 |
| 76 | `component-data-table` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-data-table.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示高密度表格、固定表头/列、排序筛选、行选择、行状态、虚拟滚动、服务端分页和状态矩阵 |
| 77 | `component-description-list` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-description-list.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示详情键值、分组、脱敏、复制、权限锁定、字段状态和 React/AntD 映射边界 |
| 78 | `component-kpi-tile` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-kpi-tile.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示指标结构、主数值、趋势 sparkline、阈值、状态矩阵、数据新鲜度和下钻审计边界 |
| 79 | `component-health-card` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-health-card.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示健康卡结构、业务健康样例、状态矩阵、依赖子检查、修复门禁和 React/AntD/ECharts 映射边界 |
| 80 | `component-ranking-list` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-ranking-list.png` | 2026-06-28 按 foundations token 和 Noto Sans CJK 字体确定性绘制，展示 TopN 行结构、业务排行样例、状态矩阵、排序阈值、行级动作和 React/AntD/ECharts 映射边界 |
| 81 | `component-log-list` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-log-list.png` | 2026-06-28 按 foundations token、DroidSansFallback 中文字体和 DejaVu Sans 英文字体确定性绘制，展示实时日志列表、级别语义、trace 高亮、展开详情、定位来源、复制、脱敏、权限状态和 React/AntD 映射边界 |
| 82 | `component-evidence-file-card` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-evidence-file-card.png` | 2026-06-28 按 foundations token、DroidSansFallback 中文字体和 DejaVu Sans 英文字体确定性绘制，展示证据文件卡、文件类型图标、hash、签名 URL、保留期、权限范围、下载/预览/复制 hash/关联告警和审计门禁 |
| 83 | `component-empty-card` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-empty-card.png` | 2026-06-28 最终补齐批次，确定性绘制，展示空态卡片、下一步动作、权限边界和复用规则 |
| 84 | `component-permission-card` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-permission-card.png` | 2026-06-28 最终补齐批次，确定性绘制，展示权限卡片、范围、门禁、审计和锁定态 |
| 85 | `component-line-area-chart` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-line-area-chart.png` | 2026-06-28 最终补齐批次，确定性绘制，展示折线/面积图结构、状态矩阵和 ECharts 映射 |
| 86 | `component-donut-chart` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-donut-chart.png` | 2026-06-28 最终补齐批次，确定性绘制，展示环图、图例、占比、阈值和下钻动作 |
| 87 | `component-bar-ranking-chart` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-bar-ranking-chart.png` | 2026-06-28 最终补齐批次，确定性绘制，展示柱状排行、排序阈值、风险语义和行级动作 |
| 88 | `component-sankey-flow` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-sankey-flow.png` | 2026-06-28 最终补齐批次，确定性绘制，展示桑基流向、节点/边状态和数据血缘边界 |
| 89 | `component-radar-quality` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-radar-quality.png` | 2026-06-28 最终补齐批次，确定性绘制，展示质量雷达、维度评分、阈值和解释动作 |
| 90 | `component-heatmap` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-heatmap.png` | 2026-06-28 最终补齐批次，确定性绘制，展示热力图、时间/对象矩阵、异常强度和图例 |
| 91 | `component-topology-graph` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-topology-graph.png` | 2026-06-28 最终补齐批次，确定性绘制，展示拓扑图节点、连线、状态、缩放和审计边界 |
| 92 | `component-timeline-state-machine` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-timeline-state-machine.png` | 2026-06-28 最终补齐批次，确定性绘制，展示时间线、状态机、事件证据和回放边界 |
| 93 | `component-alert-queue` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-alert-queue.png` | 2026-06-28 最终补齐批次，确定性绘制，展示告警队列、优先级、批量动作和权限门禁 |
| 94 | `component-risk-score` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-risk-score.png` | 2026-06-28 最终补齐批次，确定性绘制，展示风险分、因子解释、阈值和下钻动作 |
| 95 | `component-alert-timeline` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-alert-timeline.png` | 2026-06-28 最终补齐批次，确定性绘制，展示告警时间线、处置阶段、证据和审计 |
| 96 | `component-evidence-drawer` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-evidence-drawer.png` | 2026-06-28 最终补齐批次，确定性绘制，展示证据抽屉结构、下载/预览权限和链路完整性 |
| 97 | `component-asset-context` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-asset-context.png` | 2026-06-28 最终补齐批次，确定性绘制，展示资产上下文、画像、关联对象和风险摘要 |
| 98 | `component-action-rail` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-action-rail.png` | 2026-06-28 最终补齐批次，确定性绘制，展示动作栏、危险动作、权限锁和审计提示 |
| 99 | `component-feedback-block` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-feedback-block.png` | 2026-06-28 最终补齐批次，确定性绘制，展示反馈闭环、标签、置信度和模型回流边界 |
| 100 | `component-acceptance-gate-matrix` | component | `doc/04_assets/ui_suite_gpt_v1/screens/components/component-acceptance-gate-matrix.png` | 2026-06-28 最终补齐批次，确定性绘制，展示验收门禁矩阵、证据状态和治理动作 |
| 101 | `state-page-loading` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-page-loading.png` | 2026-06-28 最终补齐批次，确定性绘制，展示页面加载骨架与状态规则 |
| 102 | `state-table-loading` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-table-loading.png` | 2026-06-28 最终补齐批次，确定性绘制，展示表格加载骨架与分页边界 |
| 103 | `state-chart-loading` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-chart-loading.png` | 2026-06-28 最终补齐批次，确定性绘制，展示图表加载骨架与数据延迟提示 |
| 104 | `state-empty-page` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-empty-page.png` | 2026-06-28 最终补齐批次，确定性绘制，展示页面空态、原因解释和下一步动作 |
| 105 | `state-empty-table` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-empty-table.png` | 2026-06-28 最终补齐批次，确定性绘制，展示表格空态、筛选重置和权限说明 |
| 106 | `state-empty-chart` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-empty-chart.png` | 2026-06-28 最终补齐批次，确定性绘制，展示图表空态、采集窗口和数据新鲜度 |
| 107 | `state-api-error` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-api-error.png` | 2026-06-28 最终补齐批次，确定性绘制，展示 API 错误诊断、影响范围和重试动作 |
| 108 | `state-network-error` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-network-error.png` | 2026-06-28 最终补齐批次，确定性绘制，展示网络错误、连通性检查和恢复动作 |
| 109 | `state-unauthorized` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-unauthorized.png` | 2026-06-28 最终补齐批次，确定性绘制，展示未登录/会话过期状态和重新认证动作 |
| 110 | `state-forbidden` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-forbidden.png` | 2026-06-28 最终补齐批次，确定性绘制，展示无权限状态、申请权限和审计解释 |
| 111 | `state-partial-degraded` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-partial-degraded.png` | 2026-06-28 最终补齐批次，确定性绘制，展示部分降级、可用能力和影响范围 |
| 112 | `state-offline-probe` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-offline-probe.png` | 2026-06-28 最终补齐批次，确定性绘制，展示探针离线、心跳、链路和恢复动作 |
| 113 | `state-stream-backpressure` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-stream-backpressure.png` | 2026-06-28 最终补齐批次，确定性绘制，展示流式背压、队列水位和治理动作 |
| 114 | `state-task-running` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-task-running.png` | 2026-06-28 最终补齐批次，确定性绘制，展示任务运行、阶段进度和取消/查看日志动作 |
| 115 | `state-task-failed` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-task-failed.png` | 2026-06-28 最终补齐批次，确定性绘制，展示任务失败、错误原因、重试和审计 |
| 116 | `state-success-accepted` | state | `doc/04_assets/ui_suite_gpt_v1/screens/states/state-success-accepted.png` | 2026-06-28 最终补齐批次，确定性绘制，展示提交成功、已受理、下一步和证据留痕 |
| 117 | `responsive-dashboard-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-dashboard-1440.png` | 2026-06-28 最终补齐批次，确定性绘制，展示 dashboard 在 1440 断点的信息优先级 |
| 118 | `responsive-dashboard-1920` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-dashboard-1920.png` | 2026-06-28 最终补齐批次，确定性绘制，展示 dashboard 在 1920 断点的布局密度 |
| 119 | `responsive-screen-4k` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-screen-4k.png` | 2026-06-28 最终补齐批次，确定性绘制，展示态势大屏在 4K 断点的放大策略 |
| 120 | `responsive-alerts-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-alerts-1440.png` | 2026-06-28 最终补齐批次，确定性绘制，展示告警中心在 1440 断点的表格/详情折叠 |
| 121 | `responsive-alerts-1920` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-alerts-1920.png` | 2026-06-28 最终补齐批次，确定性绘制，展示告警中心在 1920 断点的完整工作区 |
| 122 | `responsive-forensics-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-forensics-1440.png` | 2026-06-28 最终补齐批次，确定性绘制，展示取证页在 1440 断点的主次面板策略 |
| 123 | `responsive-graph-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-graph-1440.png` | 2026-06-28 最终补齐批次，确定性绘制，展示图谱页在 1440 断点的画布/侧栏策略 |
| 124 | `responsive-compliance-1440` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-compliance-1440.png` | 2026-06-28 最终补齐批次，确定性绘制，展示合规页在 1440 断点的门禁矩阵策略 |
| 125 | `responsive-tablet-dashboard` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-tablet-dashboard.png` | 2026-06-28 最终补齐批次，确定性绘制，展示平板 dashboard 信息折叠策略 |
| 126 | `responsive-tablet-alert-detail` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-tablet-alert-detail.png` | 2026-06-28 最终补齐批次，确定性绘制，展示平板告警详情证据区折叠策略 |
| 127 | `responsive-mobile-navigation` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-mobile-navigation.png` | 2026-06-28 最终补齐批次，确定性绘制，展示移动端导航抽屉适配策略 |
| 128 | `responsive-mobile-alert-list` | responsive | `doc/04_assets/ui_suite_gpt_v1/screens/responsive/responsive-mobile-alert-list.png` | 2026-06-28 最终补齐批次，确定性绘制，展示移动端告警列表适配策略 |
| 129 | `modal-topic-save-view` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-topic-save-view.png` | 2026-06-28 P7 扩容 batch-01，专题保存视图 Modal，最终 1920x1080，保留 raw-imagegen |
| 130 | `drawer-topic-scope-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-topic-scope-edit.png` | 2026-06-28 P7 扩容 batch-01，专题范围编辑 Drawer，最终 1920x1080，保留 raw-imagegen |
| 131 | `modal-topic-report-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-topic-report-export.png` | 2026-06-28 P7 扩容 batch-02，专题报告导出 Modal，最终 1920x1080，保留 raw-imagegen |
| 132 | `modal-topic-evidence-package-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-topic-evidence-package-export.png` | 2026-06-28 P7 扩容 batch-02，专题证据包导出 Modal，最终 1920x1080，保留 raw-imagegen |
| 133 | `drawer-topic-subscription` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-topic-subscription.png` | 2026-06-28 P7 扩容 batch-03，专题订阅配置 Drawer，最终 1920x1080，保留 raw-imagegen |
| 134 | `dropdown-topic-share-favorite` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-topic-share-favorite.png` | 2026-06-28 P7 扩容 batch-03，专题分享收藏 Dropdown，最终 1920x1080，保留 raw-imagegen |
| 135 | `modal-campaign-report-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-campaign-report-export.png` | 2026-06-28 P7 扩容 batch-04，战役报告导出 Modal，最终 1920x1080，保留 raw-imagegen |
| 136 | `modal-forensics-evidence-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-forensics-evidence-export.png` | 2026-06-28 P7 扩容 batch-04，取证证据导出 Modal，最终 1920x1080，保留 raw-imagegen |
| 137 | `drawer-compliance-gate-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-compliance-gate-detail.png` | 2026-06-28 P7 扩容 batch-05，合规门禁详情 Drawer，最终 1920x1080，保留 raw-imagegen |
| 138 | `modal-compliance-evidence-package-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-compliance-evidence-package-export.png` | 2026-06-28 P7 扩容 batch-05，合规证据包导出 Modal，最终 1920x1080，保留 raw-imagegen |
| 139 | `modal-compliance-report-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-compliance-report-export.png` | 2026-06-28 P7 扩容 batch-06，合规运行报告导出 Modal，最终 1920x1080，保留 raw-imagegen |
| 140 | `drawer-audit-operation-detail` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-audit-operation-detail.png` | 2026-06-28 P7 扩容 batch-06，审计操作详情 Drawer，最终 1920x1080，保留 raw-imagegen |
| 141 | `modal-audit-export` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-audit-export.png` | 2026-06-28 P7 扩容 batch-07，审计材料导出 Modal，最终 1920x1080，保留 raw-imagegen |
| 142 | `modal-notification-channel-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-notification-channel-edit.png` | 2026-06-28 P7 扩容 batch-07，通知渠道编辑 Modal，最终 1920x1080，保留 raw-imagegen |
| 143 | `modal-notification-template-preview-test` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-notification-template-preview-test.png` | 2026-06-28 P7 扩容 batch-08，通知模板预览测试 Modal，最终 1920x1080，保留 raw-imagegen |
| 144 | `drawer-notification-silence-rule` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-notification-silence-rule.png` | 2026-06-28 P7 扩容 batch-08，通知静默规则 Drawer，最终 1920x1080，保留 raw-imagegen |
| 145 | `popconfirm-settings-token-revoke` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/popconfirm-settings-token-revoke.png` | 2026-06-28 P7 扩容 batch-09，API 令牌吊销确认 Popconfirm，最终 1920x1080，保留 raw-imagegen |
| 146 | `drawer-settings-rbac-edit` | overlay | `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-settings-rbac-edit.png` | 2026-06-28 P7 扩容 batch-09，RBAC 权限编辑 Drawer，最终 1920x1080，保留 raw-imagegen |

## 下一张

- ID：无
- Prompt：无
- Target：无
- 生成前处理：P7 扩容 18/18 已完成，manifest 交付基线完成 181/181。
