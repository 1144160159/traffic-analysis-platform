# UI Suite Context Handoff

更新日期：2026-06-28

本文档用于 Codex 重连、上下文压缩或中断后继续处理 GPT ImageGen 高保真 UI 套装。它只记录最新可执行上下文；完整清单仍以 `manifest.json`、`CHAT_IMAGEGEN_INVENTORY.md` 和 `GENERATION_STATE.md` 为准。

当前生成口径：pages 公共 AppShell 返工已完成；后续以最终交付图、manifest、prompt 和状态文档为准，不再维护修复过程图目录。2026-06-28 用户已确认处理全部待生成 UI 图，P7 18 张候选浮层已扩展进 manifest，当前总计划为 181 张。

P7 扩容进度（2026-06-28）：已完成 batch-01 `modal-topic-save-view`、`drawer-topic-scope-edit`，batch-02 `modal-topic-report-export`、`modal-topic-evidence-package-export`，batch-03 `drawer-topic-subscription`、`dropdown-topic-share-favorite`，batch-04 `modal-campaign-report-export`、`modal-forensics-evidence-export`，batch-05 `drawer-compliance-gate-detail`、`modal-compliance-evidence-package-export`，batch-06 `modal-compliance-report-export`、`drawer-audit-operation-detail`，batch-07 `modal-audit-export`、`modal-notification-channel-edit`，batch-08 `modal-notification-template-preview-test`、`drawer-notification-silence-rule`，batch-09 `popconfirm-settings-token-revoke`、`drawer-settings-rbac-edit`，均为 overlay 业务容器本体，最终 PNG 为 `1920x1080`，并保留 `*.raw-imagegen.png`。当前 P7 进度 18/18；manifest 交付基线完成 181/181，无下一张常规生成项。

文档与 prompt 源头修正（2026-06-28）：已把旧设计文档中的分栏尺寸口径同步为当前 `screen.png` 的 `166px` 单栏展开式 AppShell；`doc/04_assets/ui_suite_gpt_v1/build_prompt_manifest.mjs` 已补入 `state-unauthorized` 与 `state-forbidden` 的 401/403 语义硬门禁，避免后续重建 prompts 时把“未登录”和“无权限”重新混成泛化锁定状态。

业务合理性复核（2026-06-28）：已基于 `screens/` 图片内容完成一轮业务审计，记录见 `doc/04_assets/ui_suite_gpt_v1/BUSINESS_REASONABILITY_AUDIT.md`。本轮发现并修复的系统性问题集中在状态图和响应式图：`screens/states/` 16 张已从泛化错误模板改为加载、空态、API/网络错误、401、403、降级、探针离线、流处理背压、任务运行/失败/受理成功的独立业务语义；`screens/responsive/` 12 张已从空白断点策略板改为 dashboard、告警、取证、图谱、合规、平板和移动端的页面级折叠策略。每张返工图均保留 `*.before-business-fix.png` 和新的 `*.raw-deterministic.png`。

AppShell 职责边界修正（2026-06-28）：当前 `screen.png` 中顶部头部不承载通知/用户动作组。顶部只保留产品标题、站点/时间、风险态势、告警总数、关键告警、采集健康、数据质量和 `PCAP检索 / 资产检索 / 规则检索 / 脚本中心 / 帮助中心 / 更多应用` 六个快捷入口；告警总数/关键告警是运行指标，不是通知中心入口。用户身份、角色、在线状态和用户动作只归属左侧底部用户区；通知角标、设置、全局配置和电源只归属底部状态栏右侧全局动作区。`component-app-header`、`component-primary-sidebar` 已按此边界从 `screen.png` 裁切重组成确定性规格板，并覆盖旧 imagegen 终稿；旧版备份为 `*.before-duplication-fix.png`。

Tab 变体补图补充（2026-06-26）：用户要求对 page 中存在 Tab 但没有对应 UI 页面图的状态逐张补图，并且每次生成 1 张后压缩上下文成为文档。当前专用进度文档为 `doc/04_assets/ui_suite_gpt_v1/TAB_IMAGEGEN_PROGRESS.md`。最新口径：小 Tab 独立图只输出局部 Tab 业务内容，不包含公共 AppShell、宿主页面公共区或业务公共区；具体 Tab 形态必须跟随宿主页已有 UI，不能统一画成左侧竖向 Tab。`graph-*` 路径分析按路径结果框顶部 Tab 生成；`rules-editor-test-validation` 和 `rules-editor-dependencies` 已根据 `rules.png` 实测规则编辑局部框重生，使用顶部横向 `规则定义 / 测试验证 / 依赖引用` Tab，规则编辑局部框约 `x=834 y=211 w=598 h=456`，顶部 Tab 行约 `x=846 y=247 w=250 h=39`。`rules-sample-session` 和 `rules-sample-logs` 已根据用户提供的 `样本回放验证（近 7 天）` 小组件重生，参考图尺寸 `504x392`、可见外框约 `501x368`，同组件在 `rules.png` 中约 `x=205 y=672 w=360 h=264`，使用顶部横向 `PCAP 样本 32 / Session 样本 128 / 日志样本 256` Tab。`models-feature-rule-contribution`、`models-feature-anomaly-explanation`、`models-feature-sample-examples` 已根据用户提供的 `解释与特征` 小组件重生，参考图尺寸 `516x371`、可见外框约 `512x360`，同组件在 `models.png` 中约 `x=842 y=521 w=580 h=388`，使用顶部横向 `重要特征 / 规则贡献 / 异常解释 / 样本示例` Tab。`models-activation-audit-gate` 已根据用户提供的 `激活与回滚` 小组件重生，参考图尺寸 `755x423`、可见外框约 `748x404`，同组件在 `models.png` 中约 `x=1280 y=633 w=600 h=309`，使用顶部横向 `激活流程 / 审计门禁` Tab。`whitelist-condition-ip`、`whitelist-condition-asset`、`whitelist-condition-account`、`whitelist-condition-rule`、`whitelist-condition-model` 已根据用户提供的 `B. 条件构造器 / 新增白名单草案` 小组件生成，参考图尺寸 `504x450`、可见外框约 `491x441`，同组件在 `whitelist.png` 中约 `x=835 y=215 w=520 h=455`，使用顶部横向 `IP / 资产 / 账号 / 域名 / 规则 / 模型` Tab。`whitelist-expiry-expired-unhandled`、`whitelist-expiry-long-lived`、`whitelist-expiry-unassigned-owner` 已根据用户提供的 `E. 到期治理` 小组件生成，参考图尺寸 `588x410`、可见外框约 `588x390`，使用顶部横向 `即将到期（7天内） / 过期未处理 / 长期生效（>180天） / 未归属责任角色` Tab。`alert-detail-evidence-pcap`、`alert-detail-evidence-session`、`alert-detail-evidence-logs`、`alert-detail-evidence-graph-path`、`alert-detail-evidence-files` 已根据用户提供的 `证据链（6）` 小组件生成，参考图尺寸 `1338x369`、可见外框约 `1336x363`，使用顶部横向 `全部 6 / PCAP 1 / Session 2 / 日志 1 / 图谱路径 1 / 文件 1` Tab。`audit-log-operation-context` 和 `audit-log-related-chain` 已根据用户提供的 `操作详情 / Diff 视图` 小组件生成，参考图尺寸 `707x456`、可见外框约 `705x449`，使用顶部横向 `字段变更对比 / 操作上下文 / 关联链路` Tab。`campaign-detail-impact-account`、`campaign-detail-impact-service`、`campaign-detail-impact-department`、`campaign-detail-impact-campus`、`campaign-detail-impact-business-system` 已根据用户提供的 `影响范围` 小组件生成，参考图尺寸 `450x591`、可见外框约 `438x557`，使用顶部横向 `资产 / 账号 / 服务 / 部门 / 园区 / 业务系统` Tab。显式小 Tab 独立图队列已完成，当前断点为 `无`。

资产台账层级改造（2026-06-27）：`assets`、`assets-server`、`assets-network-device`、`assets-business-system`、`assets-unknown` 已重生为 `/assets` 页面级资产分类大 Tab，分别对应 `终端 / 服务器 / 网络设备 / 业务系统 / 未知资产`，右侧只保留选中对象摘要。`assets-detail-basic`、`assets-detail-network-interface`、`assets-detail-open-services`、`assets-detail-ownership`、`assets-detail-history` 已重生为资产详情局部小 Tab，分别对应 `基础信息 / 网络接口 / 开放服务 / 归属信息 / 历史变更`，不含 AppShell、资产台账列表、页面级资产类型 Tab、筛选区或统计区。`drawer-asset-detail` 是 manifest 内的资产详情抽屉父容器图，只展示选中资产轻摘要、详情小 Tab 导航、当前 Tab 轻摘要、关键入口、关联状态、操作门禁和审计留痕；它不是 `assets-detail-*` 小 Tab 内容页，也不展开任一小 Tab 的完整表格或详情内容。后续开发时资产台账大 Tab 负责分类视图与列表筛选，资产详情小 Tab 只在选中资产后的详情组件内使用，`drawer-asset-detail` 负责把这些小 Tab 作为抽屉父容器入口承载起来。

页面、浮层与基础组件缺口盘点（2026-06-28）：已重写并持续同步 `doc/04_assets/ui_suite_gpt_v1/PAGE_OVERLAY_COMPONENT_GAP_INVENTORY.md`，明确区分两个统计口径：`manifest.json` 交付基线已从 163 张扩展为 181 张，当前已落盘 181 张、还缺 0 张；P7 扩容前已有的 Tab 变体、详情小 Tab、专题页内状态输入和 2 张 foundation 拼接/基准参考板仍作为 manifest 外扩展图保留。当前 manifest 缺口为 0；foundation、27 张页面主图、70 张 overlay、48 张 component、16 张 state 和 12 张 responsive 均已完成。

浮层生成新口径（2026-06-27）：用户明确后续弹窗可不需要公共区域，只带业务区域即可。因此从 `drawer-mobile-navigation` 开始，overlay 类图只输出当前交互容器本体：Modal、Drawer、Dropdown、Popconfirm 或局部业务浮层；不要求携带完整顶部栏、左侧菜单、底部栏或宿主页面背景。公共 AppShell 规范只作为颜色、字号、密度、状态语义和图标风格参考。

生产浮层尺寸硬门禁（2026-07-10）：桌面端 Modal 必须是小尺寸业务弹窗，不得铺满或遮住整个浏览器业务区域；业务详情、证据和日志优先从右侧以窄 Drawer 滑出，宿主页面上下文必须保持可见。内容较少时次选小 Modal，不允许把详情实现为全屏覆盖层。逐图验收使用的专用 focus 路由只用于确定性截图，不代表生产浮层形态。

前端实现备注（2026-06-26）：用户已纠正专题产品口径，三个专题需要合并到一个页面，菜单才是 `专题面板`。当前 Web 前端以 `/topics` 作为唯一现役专题菜单页，加密隧道、数据外传、APT 战役是页面内业务切换状态，并分别读取 `/v1/topics/tunnel`、`/v1/topics/exfil`、`/v1/topics/apt`；旧 `/topics/tunnel`、`/topics/exfil`、`/topics/apt` 只作为兼容深链映射到 `/topics?topic=...`。不再生成单张 `topics.png`；后续任何按 Tab 拆分的设计图都按同样方式合并为单一路由页面内的 Tab/Segmented 状态。

专题面板 UI 图恢复记录（2026-06-26）：曾误用本地运行态截图恢复 `screens/pages/topic-tunnel.png`、`screens/pages/topic-exfil.png`、`screens/pages/topic-apt.png`，用户确认它们不是原始 UI 图，已撤下。随后已从 Codex imagegen 会话历史恢复原始三张专题面板 UI 图：`screens/pages/topics-encrypted-tunnel.png`、`screens/pages/topics-data-exfiltration.png`、`screens/pages/topics-apt-campaign.png`，并保留对应 `*.raw-imagegen.png`。三张图仅作为 `/topics` 页内状态视觉对照，不代表恢复独立左侧菜单或独立业务路由，也不计入 `manifest.json` 的 163 张主生图清单。

## 当前目标

- 2026-06-28 当前范围修正：`/topics` 专题面板是现役 Web 左侧菜单和前端路由，但不进入 UI suite 单页生图范围；三张专题 Tab 设计输入只作为 `/topics` 页面内模式视觉资产。P7 已确认追加到 overlay 队列，现役 manifest 为 181 张；仍不生成单张 `topics.png`。
- 项目：园区网络全流量采集与分析系统高保真 UI 套装。
- 规模：181 张工业级口径。
- 当前已落地：181 张。
- 下一张常规生成项：无，manifest 交付基线已完成。
- 当前优先断点：无；不再补 `doc/04_assets/ui_suite_gpt_v1/screens/pages/topics.png`，P7 18/18 已完成，后续只处理质量返工或用户新增范围。
- 最新业务复核：`screens/states/` 16 张、`screens/responsive/` 12 张已于 2026-06-28 完成业务语义返工；当前没有下一张常规生成项。
- 最近完成输出：已按两张一批完成 P7 18 张 overlay 扩容，均使用内置 `image_gen.imagegen` 逐张生成并提取到 `screens/overlays/`，最终 PNG 统一 `1920x1080`，并保存 `*.raw-imagegen.png`。`drawer-asset-detail` 仍按父容器规则处理，不替代 `assets-detail-*` 小 Tab 内容页；`topics/专题面板` 已确认为前端路由与页内 Tab 合并实现对象，当前 prompt 和 manifest 不包含单张 `topics` 页面主图。
- 最新质量门禁：pages 公共 AppShell 返工已完成；过程审计图已清理，当前以 `doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png`、最终 page 图和 `doc/04_assets/ui_suite_gpt_v1/standards/APP_SHELL_ICON_STANDARD.md` 为准。所有 page 公共区域只以态势大屏 `screen.png` 为基准，顶部单栏、左侧单栏、底部单栏的内容和图标必须完全一致，二级菜单图标按固定 iconId 清单执行；若未来重新修复，中部业务内容区不得变化。
- 当前 AppShell 返工进度：既有 pages 公共区返工已完成；专题面板不再补齐单张 `/topics` 页面主图。常规 overlay 已完成，当前进入 component 队列；显式 Tab 补图队列已按 `TAB_IMAGEGEN_PROGRESS.md` 完成，当前 Tab 断点为 `无`。
- 最新 component 批次（2026-06-28）：已完成 `component-secondary-menu`、`component-bottom-status-bar`。前者明确二级菜单只在 166px 左侧单栏内展开，禁止独立二级栏、第三层菜单和业务模块塞入菜单；后者明确底部单栏固定 y=997 / h=83，固定七个运行状态项和右侧全局动作区，通知/设置/全局配置/电源不得移到顶部。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-breadcrumb-context`、`component-site-time-selector`。
- 最新 component 批次（2026-06-28）：已完成 `component-breadcrumb-context`、`component-site-time-selector`。前者只作为业务内容区顶部上下文导航，不进入公共 AppShell；后者只作为顶部 80px 状态栏中的站点/时间模块，不混入通知、用户、设置、电源或页面业务筛选。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-quick-entry`、`component-user-menu`。
- 最新 component 批次（2026-06-28）：已完成 `component-quick-entry`、`component-user-menu`。前者只覆盖顶部六个业务快捷入口，禁止混入通知、用户、设置、全局配置或电源；后者只从左侧底部用户卡触发，顶部不得重复用户头像、用户名、用户组或个人菜单。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-button`、`component-icon-button`。
- 最新 component 批次（2026-06-28）：已完成 `component-button`、`component-icon-button`。两张都是基础控件组件板，不绘制完整 AppShell；按钮板覆盖类型、尺寸、状态、危险动作和审计门禁，图标按钮板覆盖 tooltip、状态矩阵、表格操作列和全局动作归属边界。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-status-chip`、`component-tooltip`。
- 最新 component 批次（2026-06-28）：已完成 `component-status-chip`、`component-tooltip`。两张都是基础控件组件板，不绘制完整 AppShell；标签板覆盖状态语义、风险 Badge、计数徽标、业务对象 Tag、状态矩阵和 Ant Design 映射，提示板覆盖四向 placement、字段/图标解释、权限/风险/校验说明、Popover 边界和危险确认归属。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-tabs`、`component-segmented`。
- 最新 component 批次（2026-06-28）：已完成 `component-tabs`、`component-segmented`。两张都是基础控件组件板，不绘制完整 AppShell；Tabs 板覆盖横向/卡片/业务小 Tab、Badge、状态矩阵、滚动和内容区稳定高度，Segmented 板覆盖专题模式、视图密度、粒度、风险级别、状态矩阵和与 Tabs/Menu 的边界。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-dropdown`、`component-pagination`。
- 最新 component 批次（2026-06-28）：已完成 `component-dropdown`、`component-pagination`。两张都是基础控件组件板，不绘制完整 AppShell；Dropdown 板覆盖局部动作菜单、分组/二级菜单、批量危险动作边界和状态矩阵，Pagination 板覆盖基础分页、表格底部分页、服务端/游标分页、状态矩阵和性能提示。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-input`、`component-search`。
- 最新 component 批次（2026-06-28）：已完成 `component-input`、`component-search`。两张都是基础控件组件板，不绘制完整 AppShell；Input 板覆盖 Input/InputNumber/Password/TextArea、校验、前后缀、脱敏、单位和危险变更审计边界，Search 板覆盖本地/服务端/实体/审计/PCAP 搜索、建议层、筛选 chip、查询状态和脱敏结果。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-select`、`component-date-range`。
- 最新 component 批次（2026-06-28）：已完成 `component-select`、`component-date-range`。两张都是基础控件组件板，不绘制完整 AppShell；Select 板覆盖单选、多选、分组选项、远程搜索、长列表、受限选项和状态矩阵，DateRange 板覆盖绝对/相对时间、快捷项、双月面板、时区/延迟提示、高成本查询和审计门禁。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-switch-checkbox-radio`、`component-condition-builder`。
- 最新 component 批次（2026-06-28）：已完成 `component-switch-checkbox-radio`、`component-condition-builder`。两张都是基础表单组件板，不绘制完整 AppShell；Switch/Checkbox/Radio 板覆盖布尔开关、多选范围、半选树、互斥策略、危险选项、状态矩阵和审计边界，ConditionBuilder 板覆盖条件组、字段 schema、操作符、值输入、嵌套、拖拽、命中预估、权限锁定和保存前门禁。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-batch-action-bar`、`component-data-table`。
- 最新 component 批次（2026-06-28）：已完成 `component-batch-action-bar`、`component-data-table`。两张都是基础数据操作组件板，不绘制完整 AppShell；BatchActionBar 板覆盖已选对象、跨页范围、动作分组、危险确认、权限影响和审计追踪，DataTable 板覆盖高密度表格、固定列、排序筛选、行选择、展开、虚拟滚动、服务端分页和状态矩阵。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-description-list`、`component-kpi-tile`。
- 最新 component 批次（2026-06-28）：已完成 `component-description-list`、`component-kpi-tile`。两张都是基础数据展示组件板，不绘制完整 AppShell；DescriptionList 板覆盖详情键值、分组、脱敏、复制、权限锁定、字段状态和 React/AntD 映射，KpiTile 板覆盖指标结构、趋势、阈值、状态矩阵、数据新鲜度和下钻审计边界。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-health-card`、`component-ranking-list`。
- 最新 component 批次（2026-06-28）：已完成 `component-health-card`、`component-ranking-list`。两张都是基础数据展示组件板，不绘制完整 AppShell；HealthCard 板覆盖健康结构、Probe/Kafka/Flink/存储/数据质量/模型部署样例、状态矩阵、依赖子检查和修复审计边界，RankingList 板覆盖 TopN 行结构、高风险资产/异常链路/外联/Kafka lag/规则/证据/模型/慢查询样例、状态矩阵、排序阈值和行级审计边界。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-log-list`、`component-evidence-file-card`。
- 最新 component 批次（2026-06-28）：已完成 `component-log-list`、`component-evidence-file-card`。两张都是基础数据展示组件板，不绘制完整 AppShell；LogList 板覆盖时间戳、日志级别、来源组件、对象 ID、trace_id、message 摘要、上下文标签、展开详情、复制、定位来源、筛选高亮、审计留痕、脱敏提示和状态矩阵，EvidenceFileCard 板覆盖文件类型图标、文件名、证据类型、关联对象、大小、时间窗、hash 校验、签名 URL、保留期、权限范围、下载/预览/复制 hash/关联告警动作和审计提示。两张均保存 `*.raw-deterministic.png` 作为确定性追溯文件。下一批为 `component-empty-card`、`component-permission-card`。
- 最新完成批次（2026-06-28）：已按两张一组完成 `component-empty-card` 至 `responsive-mobile-alert-list` 的最后 46 张 manifest 缺口。该收口批次覆盖数据展示、图表、安全业务组件、通用加载/空态/错误/权限/任务状态和 1440/1920/4K/平板/移动端适配策略；所有图均为 `1920x1080`，均保存 `*.raw-deterministic.png`。至此 manifest 交付基线完成 163/163，无下一张常规生成项。

## 必须参考

主设计入口：

- `doc/01_design/面向园区网络的全流量采集分析系统-UI设计套装.md`

最终视觉基线：

- `doc/04_assets/generated/campus_full_traffic_system_visual_reference_20260620_business_corrected.png`

前端视觉规范：

- `doc/01_design/面向园区网络的全流量采集分析系统-UI前端规范.md`

页面内容与菜单依据：

- `doc/01_design/面向园区网络的全流量采集分析系统-左侧菜单信息架构.md`
- `doc/01_design/面向园区网络的全流量采集分析系统-二级菜单功能点与表现形式矩阵.md`

生成清单与规模控制：

- `doc/04_assets/ui_suite_gpt_v1/CHAT_IMAGEGEN_INVENTORY.md`
- `doc/04_assets/ui_suite_gpt_v1/manifest.json`

Foundation 规范板：

- `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-visual-reference.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-layout-grid.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-color-status.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-typography-density.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-icons-actions.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-data-viz.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-table-form.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-responsive.png`

## 硬约束

1. 使用 GPT 内置 `image_gen.imagegen` 逐张生成或编辑，不使用 CLI/API fallback，除非用户明确改口。
2. 所有最终 PNG 必须保存到 `doc/04_assets/ui_suite_gpt_v1/screens/...`。
3. 所有高保真 UI 图最终尺寸必须为 `1920x1080 px`。
4. 视觉风格不改变基线：深色安全运营台、态势大屏同款单栏展开式左侧菜单、右侧闭环栏、单层底部状态栏。
5. 后续所有生成、编辑或重生成的 UI 图片必须符合 foundations 下的 UI 规范：AppShell、12 栅格、8px 面板间距、深色 token、状态色、字号密度、圆角、表格行高、ECharts 深色样式和响应式策略；不得以单张图、局部修图、业务差异或风格自由发挥为由绕过 foundations。
6. 独立页面之间不能相似，不能只替换标题、菜单或数字。
7. 独立页面的主工作区、右侧栏、表格和图表指标名称不能重叠；系统固定顶部状态条和底部状态栏除外。
8. 公共 AppShell 必须与态势大屏 `screen.png` 完全一致，公共部分包括顶部单栏、左侧单栏和底部单栏；`dashboard.png` 不作为公共壳基准。
9. 顶部单栏必须保持 `screen.png` 的系统名称、站点/时间、风险态势、告警数、严重告警、采集健康、数据质量和快捷入口结构；快捷入口固定为 `PCAP检索 / 资产检索 / 规则检索 / 脚本中心 / 帮助中心 / 更多应用`，内容、图标、顺序、尺寸、间距、分隔线、状态色、字号密度、背景、圆角和激活态都不得按页面改写。
10. 底部单栏只能是 `screen.png` 同款单层 AppShell Statusbar，内容必须统一为：`数据延迟 / 系统运行 / 告警处理SLA / 数据质量合格率 / 存储使用 / 带宽使用 / 日志吞吐 / 右侧全局动作图标组`；顺序、图标语义、分隔线、状态色和字号不得按页面改写。
11. 左侧单栏必须使用 `screen.png` 同款单栏展开式菜单：六个一级菜单图标语义和视觉风格必须逐项一致；每个二级菜单图标按 `APP_SHELL_ICON_STANDARD.md` 的固定 iconId 清单执行。不同页面只允许改变当前展开域、二级菜单文本和高亮项，不允许更换图标体系、双栏化或新增第三层。
12. 修复既有 page 图片时，只允许修改顶部、左侧、底部公共区域，中部业务内容区必须保持原图不变，不得重绘、替换指标、调整业务面板或改变业务布局。

## 当前图片状态

当前关键文件：

- `doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/login.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/login.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/dashboard.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/dashboard.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/alerts.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/alerts.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/probes.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/probes.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/campaigns.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/campaigns.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/attack-chains.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/attack-chains.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/forensics.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/forensics.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/assets.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/assets.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/graph.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/graph.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/fusion.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/fusion.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/baselines.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/baselines.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/rules.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/rules.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/deployments.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/deployments.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/models.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/models.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/mlops.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/mlops.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/playbooks.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/playbooks.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/whitelist.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/whitelist.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/compliance.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/compliance.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/audit-log.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/audit-log.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/notifications.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/notifications.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/settings.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/settings.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/not-found.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/pages/not-found.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-user-menu.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-user-menu.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-mobile-navigation.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-mobile-navigation.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-notification-center.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-notification-center.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-global-search.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-global-search.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-quick-entry.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-quick-entry.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-login-error-captcha.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-login-error-captcha.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-kpi-detail.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-kpi-detail.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-task-detail.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dashboard-task-detail.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-screen-readonly-token.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-screen-readonly-token.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-detail.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-detail.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-config.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-config.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-batch-upgrade.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-batch-upgrade.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-cert-rotate.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-probe-cert-rotate.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-log.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-probe-log.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dlq-sample.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-dlq-sample.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-data-replay-task.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-data-replay-task.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-field-quality-sample.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-field-quality-sample.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-batch.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-batch.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-batch-actions.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-batch-actions.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-row-actions.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/dropdown-alert-row-actions.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-status.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-status.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-feedback.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-alert-feedback.raw-imagegen.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-evidence-detail.png`
- `doc/04_assets/ui_suite_gpt_v1/screens/overlays/modal-evidence-detail.raw-imagegen.png`

已确认：

- `screen.png` 是 `1920x1080`。
- `login.png` 是 `1920x1080`。
- `dashboard.png` 是 `1920x1080`。
- `alerts.png` 是 `1920x1080`。
- `probes.png` 是 `1920x1080`。
- `data-quality.png` 是 `1920x1080`。
- `alert-detail.png` 是 `1920x1080`。
- `campaigns.png` 是 `1920x1080`。
- `campaign-detail.png` 是 `1920x1080`。
- `attack-chains.png` 是 `1920x1080`。
- `encrypted-traffic.png` 是 `1920x1080`。
- `forensics.png` 是 `1920x1080`。
- `assets.png` 是 `1920x1080`。
- `graph.png` 是 `1920x1080`。
- `fusion.png` 是 `1920x1080`。
- `baselines.png` 是 `1920x1080`。
- `rules.png` 是 `1920x1080`。
- `deployments.png` 是 `1920x1080`。
- `models.png` 是 `1920x1080`。
- `mlops.png` 是 `1920x1080`。
- `playbooks.png` 是 `1920x1080`。
- `whitelist.png` 是 `1920x1080`。
- `compliance.png` 是 `1920x1080`。
- `audit-log.png` 是 `1920x1080`。
- `notifications.png` 是 `1920x1080`。
- `settings.png` 是 `1920x1080`。
- `not-found.png` 是 `1920x1080`。
- `dropdown-user-menu.png` 是 `1920x1080`。
- `drawer-mobile-navigation.png` 是 `1920x1080`。
- `drawer-notification-center.png` 是 `1920x1080`。
- `modal-global-search.png` 是 `1920x1080`。
- `dropdown-quick-entry.png` 是 `1920x1080`。
- `modal-login-error-captcha.png` 是 `1920x1080`。
- `drawer-dashboard-kpi-detail.png` 是 `1920x1080`。
- `drawer-dashboard-task-detail.png` 是 `1920x1080`。
- `modal-screen-readonly-token.png` 是 `1920x1080`。
- `drawer-probe-detail.png` 是 `1920x1080`。
- `modal-probe-config.png` 是 `1920x1080`。
- `modal-probe-batch-upgrade.png` 是 `1920x1080`。
- `modal-probe-cert-rotate.png` 是 `1920x1080`。
- `drawer-probe-log.png` 是 `1920x1080`。
- `drawer-dlq-sample.png` 是 `1920x1080`。
- `modal-data-replay-task.png` 是 `1920x1080`。
- `drawer-field-quality-sample.png` 是 `1920x1080`。
- `modal-alert-batch.png` 是 `1920x1080`。
- `dropdown-alert-batch-actions.png` 是 `1920x1080`。
- `dropdown-alert-row-actions.png` 是 `1920x1080`。
- `modal-alert-status.png` 是 `1920x1080`。
- `modal-alert-feedback.png` 是 `1920x1080`。
- `modal-evidence-detail.png` 是 `1920x1080`。
- `dashboard` 已按脱敏运营工作台口径重新生成：主视觉是脱敏 KPI、优先级待办、健康门禁、验收缺口、处置阶段工作篮、证据/反馈质量和 Top Talkers 风险贡献；不得展示责任人、负责人、头像、班组、值班表、个人账号、联系方式、待指派、未认领责任、交接名单或个人任务归属。右上快捷入口必须与态势大屏 `screen.png` 一致：PCAP检索、资产检索、规则检索、脚本中心、帮助中心、更多应用。
- `login` 已按低暴露认证入口重新生成：不得展示业务导航、顶部 KPI、底部运维状态栏、系统能力摘要、拓扑、链路、内部组件名、指标、版本号、时间戳、追踪 ID、默认账号或只读演示入口。
- `screen` 已按用户最新附件 `codex-clipboard-1315c1d0-c327-48b5-9f45-6415a9fe94e0.png` 重新走 imagegen，目标是保持业务内容不变，仅让右侧“威胁态势总览”和“运行底座（大屏性能与渲染）”边框分开且闭合。
- `screen` 底部栏必须保持单层，不允许恢复为两栏底部栏。
- `alerts` 已完成：左侧菜单已按用户要求与态势大屏 `screen.png` 保持一致，采用单栏展开式导航，在同一侧栏内展开 `威胁分析` 并高亮子项 `告警中心`，不再使用“窄一级栏 + 独立二级栏”的双栏结构；主结构为告警筛选、告警队列表格、右侧选中告警详情、研判时间线、关联告警簇和处理反馈表单；底部为单层 AppShell Statusbar，右上快捷入口必须与态势大屏 `screen.png` 一致。
- `probes` 已完成：左侧菜单与态势大屏 `screen.png` 一致，采用单栏展开式导航，在同一侧栏内展开 `采集监测` 并高亮子项 `探针管理`；主结构为探针部署拓扑、探针状态矩阵、采集吞吐/丢包趋势、配置与运维动作和心跳日志。
- `data-quality` 已完成：左侧菜单与态势大屏 `screen.png` 一致，采用单栏展开式导航，在同一侧栏内展开 `采集监测` 并高亮子项 `数据质量`；主结构为质量门禁评分、Kafka/Flink/字段/存储质量矩阵、DLQ 重放与对账任务、质量异常定位和验收报告。
- 已对 `doc/04_assets/ui_suite_gpt_v1/prompts/*.prompt.txt` 做全局机械修正：旧双栏导航约束已替换为与态势大屏 `screen.png` 一致的单栏展开式左侧菜单。
- `alert-detail` 已完成：单条告警调查工作台，包含研判摘要、资产上下文、事件时间线、证据链、响应动作和反馈学习。
- `campaigns` 已完成：战役列表工作台，包含战役聚合表、ATT&CK 阶段时间线、影响矩阵、证据完整度和报告动作。
- `campaign-detail` 已完成：战役故事板，包含战役画像、横向攻击时间轴、影响范围、证据包和复盘结论。
- `attack-chains` 已完成：攻击链分析画布，包含阶段泳道、路径分析、证据锚点、ATT&CK 矩阵和处置建议。
- `encrypted-traffic` 已完成：加密流量解释分析台，包含 TLS/QUIC/JA3/SNI/证书/隧道检测和外联画像。
- `forensics` 已完成：证据取证工作台，包含取证任务、PCAP 索引、Session 复放、hash 校验、签名 URL、证据导出和审计日志。
- `assets` 已完成并已在 2026-06-27 重生：资产台账工作台包含 `终端 / 服务器 / 网络设备 / 业务系统 / 未知资产` 页面级资产分类大 Tab；每个分类视图包含对应资产清单、风险画像、流量画像、协议/服务/依赖信息和右侧选中对象摘要，不展示资产详情内部小 Tab；左侧菜单在同一侧栏内展开 `资产图谱` 并高亮 `资产台账`。
- `graph` 已完成：实体图谱工作台，包含实体搜索、关系筛选、抽象实体关系图谱画布、路径分析结果、图查询治理、右侧实体详情、邻居统计和关联时间线；左侧菜单在同一侧栏内展开 `资产图谱` 并高亮 `实体图谱`。
- `fusion` 已完成：数据融合工作台，包含数据源状态、多源融合编排、融合规则管理、融合收益对比、冲突队列、融合事件审计、右侧冲突处理抽屉和融合质量看板；左侧菜单在同一侧栏内展开 `资产图谱` 并高亮 `数据融合`。
- `baselines` 已完成：行为基准工作台，包含基线范围筛选、基线状态机、行为分布分析、偏离列表、基线版本管理、右侧偏离解释与治理操作；左侧菜单在同一侧栏内展开 `资产图谱` 并高亮 `行为基准`。
- `rules` 已完成：规则管理工作台，包含规则列表、规则状态、命中趋势、规则编排与规则详情配置；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `规则管理`。
- `deployments` 已完成：部署管理工作台，包含发布清单、灰度策略、发布健康、版本对比、回滚管理和发布证据链；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `部署管理`。
- `models` 已完成：模型管理工作台，包含模型列表、模型指标、Champion/Challenger 状态机、数据集与样本、解释与特征、激活与回滚；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `模型管理`。
- `mlops` 已完成：MLOps 编排工作台，包含闭环编排 DAG、反馈样本池、训练任务队列、评估与门禁、注册与发布和效果回流；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `MLOps 编排`。
- `playbooks` 已完成：SOAR 剧本工作台，包含剧本列表、剧本编排流程画布、节点配置/触发策略、风险控制、执行历史、处置效果和审计证据；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `SOAR 剧本`。
- `whitelist` 已完成：白名单工作台，包含白名单列表、条件构造器、审批流程状态机、命中监控、到期治理、反馈关联和影响矩阵；左侧菜单在同一侧栏内展开 `检测运营` 并高亮 `白名单`。
- `compliance` 已完成：合规审计工作台，包含验收门禁矩阵、指标映射追踪表、证据包完整度、运行报告预览、缺口治理和第三方评测批次；左侧菜单在同一侧栏内展开 `审计配置` 并高亮 `合规审计`。
- `audit-log` 已完成：审计日志工作台，包含日志检索、高密度审计表、操作详情 Diff、高风险审计、关联链路、留存状态和导出取证；左侧菜单在同一侧栏内展开 `审计配置` 并高亮 `审计日志`。
- `notifications` 已完成：通知配置工作台，包含通知渠道健康、订阅规则、条件构造器、升级策略流程、模板管理、发送历史、抑制与静默；左侧菜单在同一侧栏内展开 `审计配置` 并高亮 `通知配置`。
- `settings` 已完成：系统设置工作台，包含租户与站点树表、RBAC 权限矩阵、API 令牌、数据留存策略、集成配置健康、安全策略和系统参数；左侧菜单在同一侧栏内展开 `审计配置` 并高亮 `系统设置`。
- `not-found` 已完成：404 异常页，包含工程化错误摘要、返回入口、安全提示、辅助动作、最近可用入口和相关系统状态；不展示敏感路径、堆栈、接口细节或凭据。

## 最新三张批次压缩

- 批次时间：2026-06-21。
- 批次输出：`models`、`mlops`、`playbooks`。
- 生成方式：逐张调用 GPT 内置 `image_gen.imagegen`，每张生成后立即运行 `extract_latest_imagegen.py <targetFile>`，避免后续 imagegen 覆盖“最新图片”选择。
- 抽检结果：三张最终 PNG 均为 `1920x1080`；侧栏均为单栏展开式导航，分别高亮 `模型管理`、`MLOps 编排`、`SOAR 剧本`；底部均保持单层 AppShell Statusbar。
- 差异化结论：`models` 以模型指标、数据集、特征解释和 Champion/Challenger 状态机为主；`mlops` 以反馈样本到效果回流的编排 DAG 为主；`playbooks` 以 SOAR 流程画布、授权边界、执行历史和审计证据为主。
- 历史后续项：`whitelist`，prompt 为 `doc/04_assets/ui_suite_gpt_v1/prompts/whitelist.prompt.txt`，目标为 `doc/04_assets/ui_suite_gpt_v1/screens/pages/whitelist.png`；该项现已完成。

## 最新三张批次压缩

- 批次时间：2026-06-21。
- 批次输出：`whitelist`、`compliance`、`audit-log`。
- 生成方式：逐张调用 GPT 内置 `image_gen.imagegen`，每张生成后立即运行 `extract_latest_imagegen.py <targetFile>`，避免后续 imagegen 覆盖“最新图片”选择。
- 抽检结果：三张最终 PNG 均为 `1920x1080`；`whitelist` 在 `检测运营` 下高亮 `白名单`，`compliance` 和 `audit-log` 在 `审计配置` 下分别高亮 `合规审计`、`审计日志`。
- 差异化结论：`whitelist` 以例外治理、审批、命中风险和到期治理为主；`compliance` 以验收门禁矩阵、证据包和缺口治理为主；`audit-log` 以日志检索、操作详情 Diff、高风险审计和关联链路为主。
- 质量注意：本批产物尺寸和页面语义通过，但 pages 全局 AppShell 底栏和菜单图标仍存在跨批次漂移；后续生成必须显式锁定统一底栏内容与统一左侧图标基准。
- 历史后续项：`notifications`，prompt 为 `doc/04_assets/ui_suite_gpt_v1/prompts/notifications.prompt.txt`，目标为 `doc/04_assets/ui_suite_gpt_v1/screens/pages/notifications.png`；该项现已完成。

## 最新三张批次压缩

- 批次时间：2026-06-21。
- 批次输出：`notifications`、`settings`、`not-found`。
- 生成方式：逐张调用 GPT 内置 `image_gen.imagegen`，每张生成后立即运行 `extract_latest_imagegen.py <targetFile>`。
- 抽检结果：三张最终 PNG 均为 `1920x1080`；`notifications` 和 `settings` 在 `审计配置` 下分别高亮 `通知配置`、`系统设置`；`not-found` 保留产品 AppShell，不展示敏感路径、堆栈、接口细节或凭据。
- 质量注意：本批生成 prompt 已显式加入顶部单栏、左侧单栏、底部单栏完全对齐 `screen.png` 的硬约束。后续 overlay、component、state、responsive 生成只要出现 AppShell，也必须遵循 `APP_SHELL_ICON_STANDARD.md` 中的公共部分标准。
- 历史下一批起点曾更新为：`component-app-header`，prompt 为 `doc/04_assets/ui_suite_gpt_v1/prompts/component-app-header.prompt.txt`，目标为 `doc/04_assets/ui_suite_gpt_v1/screens/components/component-app-header.png`。该项现已完成，overlay/component/state/responsive 队列均已清零。

## 最新 overlay 单张压缩

- 批次时间：2026-06-27。
- 批次输出：`drawer-mobile-navigation`。
- 生成方式：先按用户新口径修正 prompt，明确 overlay 不带公共 AppShell、宿主页面背景、手机外壳或浏览器框；再调用 GPT 内置 `image_gen.imagegen` 生成，并运行 `extract_latest_imagegen.py doc/04_assets/ui_suite_gpt_v1/screens/overlays/drawer-mobile-navigation.png` 落盘。
- 抽检结果：最终 PNG 为 `1920x1080`；画面只包含居中的移动端导航抽屉业务容器，内容覆盖站点/时间/在线状态、菜单搜索、一级菜单、展开的 `综合态势` 二级项、快捷入口、运行状态和底部用户动作。
- 同批补充输出：`drawer-notification-center` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含通知中心抽屉业务容器，内容覆盖筛选、通知分组、未读/高危/待处理/归档状态、快捷动作和审计留痕。
- 同批补充输出：`modal-global-search` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含全局搜索弹窗业务容器，内容覆盖跨对象筛选、搜索结果、命令建议、最近访问、受限结果和审计提示。
- 同批补充输出：`dropdown-quick-entry` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含快速入口下拉业务容器，内容覆盖快捷入口、最近动作、受限入口和审计提示。
- 同批补充输出：`modal-login-error-captcha` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含登录验证弹窗业务容器，内容保持低暴露，不展示内部系统能力或完整审计追踪信息。
- 同批补充输出：`drawer-dashboard-kpi-detail` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含 KPI 详情抽屉业务容器，内容覆盖 SLA 指标解释、趋势、分解、影响范围、下钻动作和审计留痕。
- 同批补充输出：`drawer-dashboard-task-detail` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含待办任务详情抽屉业务容器，内容覆盖任务摘要、处置阶段、关联对象、证据缺口、操作建议、风险影响和审计留痕。
- 同批补充输出：`modal-screen-readonly-token` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含态势大屏只读访问令牌弹窗业务容器，内容覆盖访问范围、脱敏配置、有效期、权限边界、脱敏令牌预览、状态提示和审计留痕。
- 同批补充输出：`drawer-probe-detail` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含探针详情抽屉业务容器，内容覆盖基础信息、心跳与链路、采集吞吐、接口与丢包、证书与配置、最近日志、操作与审计。
- 同批补充输出：`modal-probe-config` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含探针配置下发弹窗业务容器，内容覆盖目标范围、配置变更对比、下发策略、预检查、影响范围、回滚与审计。
- 同批补充输出：`modal-probe-batch-upgrade` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含探针批量升级确认弹窗业务容器，内容覆盖升级范围、版本变更、分批策略、预检查结果、影响范围、回滚与审计。
- 同批补充输出：`modal-probe-cert-rotate` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含探针证书轮换弹窗业务容器，内容覆盖证书范围、当前证书、新证书、预检查、轮换策略、影响范围、回滚与审计，且不展示私钥、完整证书或完整指纹。
- 同批补充输出：`drawer-probe-log` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含探针日志抽屉业务容器，内容覆盖探针状态、筛选、脱敏日志表、事件解释、建议动作和审计留痕。
- 同批补充输出：`drawer-dlq-sample` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含 DLQ 样例详情抽屉业务容器，内容覆盖核心摘要、诊断链路、脱敏样本对比、影响评估、当前解释、重放门禁动作和审计留痕。
- 同批补充输出：`modal-data-replay-task` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含数据重放任务弹窗业务容器，内容覆盖重放范围、门禁校验、重放策略、影响评估、样本预览、审计与权限。
- 同批补充输出：`drawer-field-quality-sample` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含字段质量样例抽屉业务容器，内容覆盖字段画像、异常分布、样本对比、下游影响、修复建议和审计留痕。
- 同批补充输出：`modal-alert-batch` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含告警批量操作确认弹窗业务容器，内容覆盖操作类型、状态变更预览、选择范围、影响提示、选中告警预览、权限审计、备注和影响检查门禁。
- 同批补充输出：`dropdown-alert-batch-actions` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含告警批量操作下拉菜单业务容器，内容覆盖状态处理、证据与联动、例外与忽略、风险标签和审计提示。
- 同批补充输出：`dropdown-alert-row-actions` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含告警行操作下拉菜单业务容器，内容覆盖查看与证据、处置动作、例外与反馈、风险标签和审计提示。
- 同批补充输出：`modal-alert-status` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含更新告警状态弹窗业务容器，内容覆盖状态机、证据检查、处置备注、影响联动、权限审计和状态检查门禁。
- 同批补充输出：`modal-alert-feedback` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含提交告警反馈弹窗业务容器，内容覆盖反馈标签、证据引用、回流范围、样本标注、说明输入、门禁审计和反馈校验。
- 同批补充输出：`modal-evidence-detail` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含证据详情弹窗业务容器，内容覆盖证据属性、脱敏预览、链路完整性、操作区、权限审计和下载授权提示。
- 同批补充输出：`modal-playbook-trigger` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含触发 SOAR 剧本弹窗业务容器，内容覆盖剧本选择、节点预览、参数映射、影响与回滚、门禁检查、审计和审批前置灰执行动作。
- 同批补充输出：`modal-whitelist-draft-from-alert` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含从告警生成白名单草案弹窗业务容器，内容覆盖告警来源、条件提取、生效范围、例外原因、影响评估、审批门禁、状态解释和审计留痕。
- 同批补充输出：`popconfirm-pcap-download` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含 PCAP 下载确认浮层业务容器，内容覆盖下载对象、下载前检查、影响与风险、授权选项、审计留痕和签名 URL 生成前置灰确认下载。
- 同批补充输出：`drawer-session-replay` 已按相同口径完成，最终 PNG 为 `1920x1080`；画面只包含会话复放抽屉业务容器，内容覆盖会话摘要、复放控制、会话时间线、协议解码、载荷摘要、风险定位、证据关联和审计留痕。
- 同批补充输出：`drawer-asset-detail` 已按用户反馈重生为资产详情抽屉父容器图，最终 PNG 为 `1920x1080`；画面只包含选中资产摘要、风险概览、详情小 Tab 导航、当前 Tab 轻摘要、关键入口、关联状态、图谱预览、操作门禁和审计留痕，不含完整资产台账页面、资产分类大 Tab，也不展开基础信息、网络接口、开放服务、归属信息或历史变更小 Tab 的完整内容。
- 同批补充输出：`modal-asset-edit` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含资产编辑弹窗业务容器，内容覆盖资产摘要、基础/归属/标签/业务系统/园区网段/维护窗口表单、变更 Diff、影响范围、变更门禁、审计信息和底部取消/保存草稿/提交审批/确认更新动作，确认更新在门禁未完成时锁定。
- 同批补充输出：`drawer-asset-history` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含资产历史抽屉业务容器，内容覆盖资产摘要、变更时间线、字段 Diff、来源证据、影响范围、关联告警、图谱邻居、可回滚项、审批门禁和审计留痕，不画完整资产台账页面或 `assets-detail-history` 小 Tab 内容页。
- 同批补充输出：`drawer-graph-entity` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含图谱实体详情抽屉业务容器，内容覆盖实体画像、局部关系预览、关联边、最近会话、命中规则、关联告警、攻击路径入口、影响范围、权限门禁和审计记录，不画完整实体图谱页面或 `graph-*` 小 Tab 结果页。
- 同批补充输出：`drawer-graph-path-analysis` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含图谱路径分析抽屉业务容器，内容覆盖路径候选、路径链路预览、边证据详情、影响范围、关联告警、导出证据、权限门禁和下一步建议，不画完整实体图谱页面或图谱路径小 Tab 结果页。
- 同批补充输出：`drawer-fusion-conflict` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含数据融合冲突处理抽屉业务容器，内容覆盖冲突摘要、多源可信度、冲突处理流程、字段候选对比、关联证据、决策建议、影响范围、审计策略和审批门禁，不画完整数据融合页面或融合规则编辑弹窗。
- 同批补充输出：`modal-fusion-rule-edit` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含融合规则编辑 Modal 业务容器，内容覆盖规则配置、主键优先级、数据源可信度、冲突处理策略、权限门禁、影响范围、状态解释、下一步动作和审计留痕。
- 同批补充输出：`modal-baseline-threshold` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含基线阈值编辑 Modal 业务容器，内容覆盖阈值参数、学习窗口、回放评估、曲线预览、权限门禁、影响范围、状态解释和审计留痕。
- 同批补充输出：`modal-forensics-task` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含取证任务详情 Modal 业务容器，内容覆盖切片范围、证据类型、输出文件、存储位置、处理阶段、失败重试、权限门禁和审计留痕。
- 同批补充输出：`drawer-campaign-detail` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含战役详情 Drawer 业务容器，内容覆盖聚类原因、攻击阶段、证据表、图谱链路、影响范围、处置建议和审批门禁。
- 同批补充输出：`drawer-attack-chain-detail` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含攻击链详情 Drawer 业务容器，内容覆盖节点上下文、证据、命中规则、关联资产、链路预览、下一步调查和权限审计。
- 同批补充输出：`drawer-encrypted-fingerprint` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含加密指纹详情 Drawer 业务容器，内容覆盖 TLS/JA3/JA4、SNI、证书链、相似样本、趋势、风险解释、影响范围和审计留痕。
- 同批补充输出：`drawer-certificate-detail` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含证书详情 Drawer 业务容器，内容覆盖证书主体、证书链、风险检查、相似样本、趋势、影响范围、动作和审计留痕。
- 同批补充输出：`modal-rule-edit` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含规则编辑 Modal 业务容器，内容覆盖规则定义、DSL 编辑器、测试门禁、依赖引用、影响范围、版本 Diff、审批和审计。
- 同批补充输出：`drawer-rule-detail` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含规则详情 Drawer 业务容器，内容覆盖规则生命周期、版本历史、命中趋势、发布记录、关联模型、影响范围和审计留痕。
- 同批补充输出：`popconfirm-delete` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含规则删除确认 Popconfirm 业务容器，内容覆盖规则名、影响范围、软删除说明、权限提示、删除原因输入和审计编号。
- 同批补充输出：`modal-rule-publish` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含规则发布确认 Modal 业务容器，内容覆盖发布类型、灰度范围、发布前检查矩阵、影响范围、审批链、回滚策略和审计留痕。
- 同批补充输出：`modal-deployment-create` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含创建部署 Modal 业务容器，内容覆盖能力包版本、目标部署集、灰度策略、发布编排预览、预检查矩阵、影响范围和审计留痕。
- 同批补充输出：`modal-deployment-rollback` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含部署回滚 Modal 业务容器，内容覆盖回滚目标版本、影响范围、回滚前检查、观测窗口、审批链、回滚原因和审计留痕。
- 同批补充输出：`drawer-model-detail` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含模型详情 Drawer 业务容器，内容覆盖模型版本、评估指标、特征解释、Champion/Challenger、影响范围、回滚动作和审计留痕。
- 同批补充输出：`drawer-mlops-task-detail` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含 MLOps 任务详情 Drawer 业务容器，内容覆盖任务 DAG、阶段状态、指标、日志、产物、失败重试、发布门禁和审计留痕。
- 同批补充输出：`modal-playbook-edit` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含 SOAR 剧本编辑 Modal 业务容器，内容覆盖节点编排、参数映射、风险控制、测试验证、影响范围、审批和审计留痕。
- 同批补充输出：`modal-whitelist-add` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含新增白名单 Modal 业务容器，内容覆盖条件构造、生效策略、风险评估、影响范围、审批链和审计留痕。
- 同批补充输出：`drawer-whitelist-approval` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含白名单审批详情 Drawer 业务容器，内容覆盖审批流程、命中证据、风险解释、到期治理、审批动作和审计留痕。
- 单张收尾输出：`modal-settings-token` 已按新口径完成，最终 PNG 为 `1920x1080`；画面只包含 API 令牌管理 Modal 业务容器，内容覆盖令牌配置、脱敏 token、权限 scope、有效期、IP 白名单、轮换/吊销风险和审计留痕，未展示真实可用密钥。
- 下一张起点：无；manifest 交付基线已完成 163/163，overlay/component/state/responsive 队列均已清零。

## 清退模块

- `topics/专题面板` 不恢复单张 `topics.png` 生图入口；已恢复的 `topics-encrypted-tunnel.png`、`topics-data-exfiltration.png`、`topics-apt-campaign.png` 是同一 `/topics` 页面的页内状态图。旧 `/topics/tunnel`、`/topics/exfil`、`/topics/apt` 只作为兼容深链或 API 语义来源。
- `modal-topic-create`、`modal-topic-report-export` 仍保持清退；如需恢复专题创建或报告导出浮层，需要重新加入 manifest 和 prompt。

## `login` 低暴露认证入口记录

用户反馈：

- “因为登录页面暴露太多系统信息，需要利用gpt的imagegen重新生成登录页面。”

已执行处理：

- 更新 `doc/04_assets/ui_suite_gpt_v1/prompts/login.prompt.txt`，删除旧口径中正向要求的能力摘要、只读演示入口、追踪 ID、详细失败原因、暗色园区拓扑/链路背景等内容。
- 使用内置 `image_gen.imagegen` 重新生成 `login`，并提取为 `doc/04_assets/ui_suite_gpt_v1/screens/pages/login.png`。
- `login.raw-imagegen.png` 为本轮 imagegen 原始输出，尺寸 `1672x941`；`login.png` 已标准化为 `1920x1080`。
- 目视抽检：新登录页为深色统一身份认证入口，仅包含品牌、租户/站点、账号、密码、验证码、OIDC/SSO、记住登录、忘记密码、帮助中心、隐私声明、登录按钮和通用低暴露安全提示。

后续注意：

- 登录页是外部可见入口，禁止出现系统能力摘要、运行状态、版本号、时间戳、追踪 ID、网络拓扑、组件清单、业务指标、资产信息、告警信息、取证信息、数据质量信息、模型信息、内部服务名、内部导航菜单、默认账号示例或只读演示入口。
- 如重新生成 `login`，必须继续使用低暴露认证入口口径，而不是复用态势大屏、仪表盘或系统能力展示布局。

## `screen` 右侧边框处理记录

用户反馈：

- “右侧竖线还是连在一起的。”
- “基于该图使用imagegen重新生成态势大屏，所有的业务内容不变，只要求运行底座和威胁态势总览的边框是分开的且是闭合的。”

已确认问题：

- `screen` 右侧列中，上方面板“威胁态势总览”和下方面板“运行底座（大屏性能与渲染）”应为两个同级独立面板。
- 原图里两者之间能看到边框关系不清晰，视觉上像一个右侧总外框或共享边框。

当前已执行处理：

- 按用户明确要求使用内置 `image_gen.imagegen`，以最新附件 `codex-clipboard-1315c1d0-c327-48b5-9f45-6415a9fe94e0.png` 作为编辑目标重新生成 `screen`。
- 第一版 imagegen 改动了业务内容，未落盘。
- 第二版 imagegen 用更强的“局部截图修补、业务内容保持不变”提示生成，已提取到 `doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png`。
- `screen.raw-imagegen.png` 为本轮 imagegen 原始输出，尺寸 `1672x941`；`screen.png` 已标准化为 `1920x1080`。
- 抽检右侧栏后，`威胁态势总览` 与 `运行底座（大屏性能与渲染）` 已呈现为两个分离的闭合面板，中间有独立间距，不再作为一个共享大外框。

后续注意：

- 如继续修 `screen`，必须以用户最新附件和当前 `screen.png` 为基线，不允许生成内容明显不同的态势大屏。
- 不改产品名、顶部栏、左侧菜单、主拓扑区、底部状态栏和整体视觉基线。
- 如果仍需微调，优先使用“业务内容逐字逐项保持，只修右侧两块闭合边框”的局部 imagegen 编辑提示；若 imagegen 再次漂移业务内容，不要落盘。

本轮可复用提示要点：

```text
把可见附件作为截图修补目标，而不是重新设计目标。
业务内容、文字、数字、图表、拓扑、菜单、顶部栏、底部栏全部保持不变。
只修右侧“威胁态势总览”和“运行底座（大屏性能与渲染）”两块面板边框。
两块必须是同级独立闭合圆角矩形面板，中间有深色背景间距，不共享外框、不嵌套、不连线。
```

## 已删除的临时规则

不要恢复下面这条规则：

```text
右侧只允许 5 个威胁态势内容，禁止额外塞运行底座
```

当前正确口径：

- 右侧可以同时包含“威胁态势总览”和“运行底座（大屏性能与渲染）”。
- 两者必须是两个分离的同级面板，谁也不包含谁。

## 继续生成流程

1. 读取 `doc/04_assets/ui_suite_gpt_v1/manifest.json`。
2. 找到第一条 `targetFile` 不存在的 item。
3. 读取该 item 的 `promptFile`。
4. 调用内置 `image_gen.imagegen` 生成一张图。
5. 提取最新聊天生图结果：

```bash
python3 doc/04_assets/ui_suite_gpt_v1/extract_latest_imagegen.py <targetFile>
```

6. 使用 `view_image` 抽检输出。
7. 抽检通过后更新 `GENERATION_STATE.md` 的进度表。
8. 每累计 2 张新图后，同步本文件的当前断点，形成可重连的压缩上下文。

## 快速验证命令

检查关键图片尺寸：

```bash
python3 - <<'PY'
from pathlib import Path
from PIL import Image
for p in [
    Path('doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png'),
    Path('doc/04_assets/ui_suite_gpt_v1/screens/pages/dashboard.png'),
]:
    im = Image.open(p)
    print(f'{p}\t{im.size[0]}x{im.size[1]}')
PY
```

检查临时规则是否已删除：

```bash
rg -n "右侧栏内部只能包含|只允许 5|禁止额外塞|禁止额外加入运行底座|运行底座.*右侧|右侧.*运行底座|5 个内容必须|不能只拉伸最后" doc/04_assets/ui_suite_gpt_v1
```

裁剪右侧区域用于人工检查：

```bash
python3 - <<'PY'
from pathlib import Path
from PIL import Image
p = Path('doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png')
im = Image.open(p)
for name, box in {
    'screen-right-column': (1420, 80, 1910, 990),
    'screen-right-divider': (1420, 730, 1910, 990),
}.items():
    out = Path('/tmp') / f'{name}.png'
    im.crop(box).save(out)
    print(out)
PY
```
