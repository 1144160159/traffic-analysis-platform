# 面向园区网络的全流量采集分析系统 UI 设计套装

更新时间：2026-06-19

## 1. 设计目标

本 UI 套装面向课题一“园区网络流量智能检测与分析”的前端重设计，目标不是增加装饰性页面，而是把当前系统已有功能组织成一条清晰的安全运营闭环。以下是流程语言，不是左侧一级菜单命名：

```text
看见 -> 研判 -> 取证 -> 治理 -> 验收
```

设计依据：

- `doc/01_design/课题一产品与技术总体设计.md` 的产品定位、信息架构和 25 分钟演示主线。
- `doc/05_status/未开发项梳理-2026-06-19.md` 的 P0/P1 开发缺口。
- 当前前端 `web/ui` 的 React、Ant Design、ECharts、深色主题和 6 组菜单结构。
- 态势感知大屏概念图：`doc/04_assets/generated/situation_screen_concept_chatgpt_20260619.png`。

## 2. UI 产品原则

| 原则 | 设计要求 |
|---|---|
| 一屏讲清系统价值 | 首页和大屏必须让用户看到采集链路、检测结果、证据、处置和反馈学习，而不是只有指标堆叠 |
| 先闭环再炫技 | 页面优先表达“下一步做什么”和“证据是否充分”，动效只服务于状态变化和态势感知 |
| 统一工作台和大屏口径 | Dashboard、SituationalScreen、Alert、Evidence、Graph、Compliance 使用同一套指标解释和时间窗 |
| 面向真实运营 | 表格、筛选、批量操作、详情抽屉、状态机、审计轨迹必须清晰可用 |
| 面向验收交付 | P0/P1、证据包、专项测试、第三方评测、生产安全和 HA 状态要能直接追溯 |

## 3. 信息架构与导航命名

一级菜单必须使用标准业务域命名，不能直接使用闭环阶段词。闭环阶段只用于页面流程、演示脚本、状态机、验收 ID 和 `RouteManifest.workflow`，不作为左侧第一层菜单 label。

建议左侧一级菜单规范为 6 组主导航，并在前端 `MainLayout.tsx` 中同步落地：

| 一级菜单 | 承载闭环阶段 | 目标 | 页面 |
|---|---|---|---|
| 综合态势 | 态势感知 | 掌握当前风险态势、采集健康和流量基线 | Dashboard、SituationalScreen、TopicTunnel、TopicExfil、TopicAPT |
| 采集监测 | 采集接入、质量监测 | 证明系统采得全、处理得动、数据可信 | Probe、DataQuality |
| 威胁分析 | 告警分析、证据入口 | 把告警判成可解释、可处置、可复盘的事件，并进入 PCAP 证据 | Alerts、Campaigns、AttackChain、EncryptedTraffic、Forensics |
| 资产图谱 | 资产上下文、关联证据 | 形成资产、实体、融合数据和行为基准的上下文证据 | AssetInventory、Graph、Fusion、Baselines |
| 检测运营 | 检测策略、模型运营、响应处置 | 管理规则、模型、白名单、部署、剧本和反馈学习 | Rules、Whitelist、Deployments、Models、MLOps、Playbooks |
| 审计配置 | 合规审计、通知配置、系统配置 | 证明工程系统可交付、可追溯、可配置 | Compliance、AuditLog、Notifications、Settings |

## 4. 三套视觉方向与已选混合方向

本轮已基于 Product Design UI ideation 流程生成 3 套视觉方向。图像工具未返回可复制的本地生成路径，因此本文件记录方向定义和可复用提示词；用户已选择“1+2 混合视觉”，即以 Command Deck 的全局态势和指挥感作为骨架，叠加 Analyst Workbench 的研判效率、证据闭环和操作密度，形成后续前端改造的主视觉方向。

### 4.1 方向一：Command Deck 指挥舱

定位：最适合售前演示、领导汇报、SOC 大屏和 `/screen` 产品化落地。

设计特征：

- 主画面围绕园区数字孪生、采集链路、Kafka/Flink、告警研判、PCAP 证据、响应动作和反馈学习展开。
- 深色底、冷色网格、青蓝主线、绿色闭环流向、红橙风险点。
- 信息密度高，但用区域分组和动线表达系统闭环。

适合页面：

- `/screen` 态势感知大屏
- `/dashboard` 的指挥舱模式
- 售前演示首页

关键模块：

| 区域 | 内容 |
|---|---|
| 左侧 | 园区数字孪生、探针、接入交换机、外部链路 |
| 中央 | 采集、协议解析、归一化、Kafka、Flink、数据湖、检测分析 |
| 右侧 | 告警聚类、时间线、PCAP 证据、资产上下文、响应动作 |
| 顶部 | 反馈标注、模型训练、效果评估、模型发布 |
| 底部 | Top Talkers、实时流量、威胁地图、告警摘要 |

### 4.2 方向二：Analyst Workbench 研判工作台

定位：最适合安全分析员日常值守和告警处置，是当前多页面系统最应优先统一的工作台方向。

设计特征：

- 三栏布局：告警队列、研判时间线、证据和上下文。
- 弱化装饰，突出筛选、证据、状态、动作和反馈。
- 所有操作围绕一个 selected alert 展开，减少页面跳转。

适合页面：

- `/alerts`
- `/alerts/:alertId`
- `/forensics`
- `/graph`
- `/playbooks`

关键模块：

| 区域 | 内容 |
|---|---|
| 左栏 | 告警队列、风险筛选、状态筛选、资产筛选 |
| 中栏 | 告警摘要、攻击阶段、时间线、研判笔记 |
| 右栏 | PCAP、Session、日志、资产画像、图谱邻居、处置动作 |
| 底部 | TP/FP 反馈、白名单草案、规则复审、审计记录 |

### 4.3 方向三：Evidence & Acceptance Suite 验收治理套装

定位：最适合项目经理、测试验收、实施和第三方评测，解决“系统到底是否可交付”的表达问题。

设计特征：

- 更克制、更工程化，强调证据、门禁、状态和追溯。
- 用矩阵、状态条、证据包和详情抽屉组织信息。
- 与 `05_status`、`02_acceptance`、`03_review` 的文档结构直接对应。

适合页面：

- `/data-quality`
- `/compliance`
- `/audit-log`
- `/deployments`
- 验收证据包页面

关键模块：

| 区域 | 内容 |
|---|---|
| 顶部 | release baseline、租户/站点、时间窗、证据包版本 |
| 主区 | P0/P1 门禁矩阵、证据完整度、专项测试状态、生产安全状态 |
| 右侧 | 选中门禁详情、缺口、责任人、下一步动作 |
| 底部 | 审计轨迹、报告导出、第三方评测材料 |

### 4.4 已选方向：Command Workbench 指挥研判台

定位：作为当前前端重设计的主视觉基线，覆盖态势感知、威胁分析、证据上下文三类主场景；检测运营和审计配置仍使用 Evidence & Acceptance Suite 的工程化表达。

设计组合：

- 继承 Command Deck 的园区拓扑、采集链路、流处理状态、数据湖、检测分析和反馈学习闭环。
- 吸收 Analyst Workbench 的三栏研判效率：告警队列、选中事件、证据上下文、响应动作都围绕同一个 selected alert 展开。
- 大屏不再只是展示墙，而是可进入工作流的只读或脱敏指挥视图；工作台不再是孤立表格，而是保留全局态势感的操作界面。

主布局：

| 区域 | 内容 | 设计目的 |
|---|---|---|
| 左侧导航 | 综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置 | 一级菜单使用业务域命名，闭环阶段在页面内流程和状态机中表达 |
| 顶部状态条 | 站点、时间窗、风险级别、采集健康、证据完整度 | 统一 Dashboard、Screen、Alert 的指标口径 |
| 中央上区 | 园区拓扑、探针覆盖、Kafka/Flink/存储链路 | 让用户先判断系统是否看得见、采得全、处理得动 |
| 中央中区 | 告警聚类、选中告警时间线、攻击阶段、流量趋势 | 把态势切换为可解释事件 |
| 中央下区 | PCAP、Session、日志、图谱邻居、资产画像 | 让研判结论有证据可追溯 |
| 右侧闭环栏 | 指派、关闭、白名单、剧本、反馈标注、模型重训、验收证据 | 保证每个事件都能收口到处置、学习和审计 |

优先落地路由：

- `/screen`：Command Workbench 的只读大屏视图。
- `/dashboard`：Command Workbench 的值守首页。
- `/alerts`、`/alerts/:alertId`：Command Workbench 的主操作面。
- `/forensics`、`/graph`、`/assets`：Command Workbench 的证据和上下文扩展面。

不可变规则：

1. 不能把 `/screen` 做成孤立炫屏，必须能跳转到告警、探针、仪表盘和验收缺口。
2. 不能把 `/alerts` 做成普通 CRUD 表，必须围绕选中告警完成研判、取证、处置、反馈和审计。
3. 不能让图谱、PCAP、资产、模型反馈分别成为孤岛，所有入口都必须保留上下文返回路径。
4. 同一指标在 `/screen`、`/dashboard`、`/alerts` 中只允许一套解释口径和时间窗语义。

### 4.5 第一版全量 UI 图套装（已清退）

第一版图套装曾按第三张 UI 视觉和当时 `doc/` 业务内容生成，用于早期前端重构、评审沟通和设计验收。该旧版目录已于 2026-06-23 从 `doc/04_assets/ui_suite_v1/` 清退，不再作为交付物、生成输入或后续缺口统计依据。

当前 UI 图套装以 `doc/04_assets/ui_suite_gpt_v1/` 为主线；历史第一版只保留本节口径说明，避免继续引用已删除的页面图、浮层图、manifest 和浏览入口。

### 4.6 GPT 生图版全量 UI 套装

在用户确认最终视觉参考图后，已将同一业务范围整理为 GPT 生图版输入资产，落盘到 `doc/04_assets/ui_suite_gpt_v1/`。该目录用于通过 GPT 图像模型生成首版全量 UI 视觉效果图，要求每一张页面图、浮层图、组件板和状态图都严格遵守 foundations UI 规范，并一律输出为 `1920x1080 px`。

GPT 生图范围在 4.5 的页面/浮层基础上补齐 foundations、组件、状态和响应式规范图：

| 类型 | 数量 | 覆盖范围 |
|---|---:|---|
| 视觉基线与规范板 | 8 | 最终视觉、布局栅格、色彩状态、字体密度、图标动作、数据可视化、表格表单、响应式 |
| 页面图 | 27 | `/login`、`/screen`、现役页面主图、详情路由、404；`/topics` 按 Tab 设计输入合并实现，不生成单张页面图 |
| 浮层图 | 52 | Modal、Drawer、Dropdown、Popconfirm、移动端侧滑菜单、用户菜单 |
| 元件与组件板 | 48 | AppShell、导航、按钮、表格、表单、图表、告警、证据、动作栏等 |
| 通用状态图 | 16 | loading、empty、error、unauthorized、offline、degraded、success 等 |
| 响应式与大屏适配图 | 12 | 1440、1920、4K、平板和移动端策略 |
| 总计 | 163 | 覆盖当前前端业务页面、关键交互状态和 UI foundations |

生图硬门禁：

1. 全局约束：后续所有生成、编辑或重生成的 UI 图片，无论是页面、浮层、组件、状态图还是响应式适配图，都必须严格遵循 foundations 的 UI 规范；不得以单张图、局部修图、业务差异或风格自由发挥为由绕过 foundations。
2. 默认参考图为 `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-generation-reference.png`，它由最终视觉基线和 8 张 foundations 规范板拼接而成。
3. 生成图必须严格遵守 foundations 中的 AppShell 结构、12 栅格、8px 面板间距、深色 token、状态色、字号密度、圆角、表格行高、ECharts 深色样式和响应式策略。
4. 左侧菜单必须严格遵守当前 `screen.png` 的 AppShell 单栏展开式导航：左侧整体实测约 `166px`，同一个侧栏内承载一级业务域和当前业务域二级页面；一级固定为“综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置”，二级只显示当前业务域页面；禁止恢复“窄一级栏 + 独立二级栏”的双栏结构，禁止第三层菜单、卡片式二级菜单、工具面板式侧栏、专题目录式侧栏，禁止把页面内部模块塞进左侧菜单。若文字描述与当前 `screen.png` 实际公共 AppShell 冲突，以 `screen.png` 为最高优先级。
5. 不得只做“风格相似”的自由发挥；如果布局、色彩、字号、圆角、表格密度、图表样式、状态语义或左侧菜单形态偏离 foundations、态势大屏或仪表盘，应视为不合格并重新生成。
6. 对 `gpt-image-2`，脚本会以 `1920x1088` 请求生成并自动裁剪回 `1920x1080`，最终归档图仍必须是 `1920x1080 px`。

本地资产：

| 交付物 | 路径 | 说明 |
|---|---|---|
| GPT 版生成清单 | `doc/04_assets/ui_suite_gpt_v1/manifest.json` | 记录 163 张图的 ID、路由、目标文件、prompt 文件和 foundations 参考图 |
| GPT 逐图提示词 | `doc/04_assets/ui_suite_gpt_v1/prompts/` | 每张页面/浮层/组件/状态图独立 prompt，绑定 foundations 参考图和硬门禁 |
| GPT 图片目标目录 | `doc/04_assets/ui_suite_gpt_v1/screens/` | API 成功后输出 PNG 到 `foundations/`、`pages/`、`overlays/`、`components/`、`states/`、`responsive/` |
| 生成脚本 | `doc/04_assets/ui_suite_gpt_v1/run_generation.sh` | 支持 `ONLY_ID`、`START_AT`、`LIMIT`、`DRY_RUN` |

当前生成状态：本地 prompt、manifest 和执行脚本已准备完成；API 冒烟调用已到达 OpenAI，但当前本地 `OPENAI_API_KEY` 不可用，因此需切换可用项目 key 后继续生成：

```bash
cd /home/wangwt/phase_2/code/traffic-analysis-platform
QUALITY=medium SIZE=1920x1080 bash doc/04_assets/ui_suite_gpt_v1/run_generation.sh
```

## 5. 统一设计系统

### 5.1 色彩

| Token | 建议值 | 用途 |
|---|---|---|
| `--surface-0` | `#07111f` | 全局背景 |
| `--surface-1` | `#0b1b2e` | 主工作区 |
| `--surface-2` | `#102842` | 面板底色 |
| `--line-subtle` | `rgba(115, 201, 255, 0.16)` | 分隔线 |
| `--accent-cyan` | `#19c8ff` | 采集、链路、主按钮 |
| `--accent-green` | `#45d483` | 正常、闭环、已验证 |
| `--accent-amber` | `#f5b84b` | 中风险、待确认 |
| `--accent-red` | `#ff5d5d` | 高危、失败、阻断 |
| `--accent-violet` | `#8b7cf6` | 模型、MLOps、AI 分析 |

要求：主界面不再只依赖蓝色系，必须用语义色区分风险、证据、闭环和模型状态。

### 5.2 字体与密度

| 层级 | 用途 | 建议 |
|---|---|---|
| Product Title | 产品标题 | 24px，700，单行显示“园区网络全流量采集与分析系统” |
| Page Title | 页面标题 | 20-22px，700，工作台内慎用大标题 |
| Section Title | 区域标题、面板标题 | 15-16px，600，统一中文标题 |
| Nav Primary | 一级菜单 | 16px，500，图标和文字垂直对齐 |
| Nav Secondary | 二级菜单 | 15px，500，选中态可加左侧高亮线和背景 |
| Body | 表格、字段、说明 | 12-13px，400，行高 1.45-1.6 |
| Meta | 标签、时间、辅助信息 | 11-12px，400，不承担主信息 |
| Number | 指标数字、风险评分 | tabular nums，20-28px；大屏可放大到 32px，但同类指标必须一致 |

工作台页面优先使用 12-13px 的高密度信息结构；大屏可以按距离适度放大，但不得牺牲可读性。

字体统一规则：

1. 同类组件必须使用同一字号和字重，例如所有面板标题统一为 15-16px，所有表格正文统一为 12-13px。
2. 禁止在同一页面内随机混用大号面板标题、超小字段和值班屏风格数字；指标数字可以放大，但必须按指标卡、风险评分、表格数值三个等级管理。
3. 所有中文标题使用简体中文，不使用英文副标题，不使用“中文 / English”并列标题。
4. 技术专名和协议名允许保留英文或缩写，例如 Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO、PCAP、TLS、IP、DNS、JA3。
5. 图表标题、表格标题、右侧详情栏标题必须去英文化：`Alert Queue` 改为“告警队列”，`Risk Score` 改为“风险评分”，`Response Actions` 改为“响应处置”。

### 5.3 通用组件

| 组件 | 说明 |
|---|---|
| `MetricTile` | 统一指标卡，支持口径说明、趋势、阈值和异常状态 |
| `StatusChip` | 统一状态标签，覆盖 new、triage、assigned、closed、verified、blocked |
| `EvidenceDrawer` | PCAP、Session、日志、hash、审计记录的右侧详情抽屉 |
| `RiskTimeline` | 告警、证据、处置、反馈的时间线 |
| `TopologyPanel` | 资产、探针、链路、图谱关系的可复用面板 |
| `GateMatrix` | P0/P1 验收门禁和证据完整度矩阵 |
| `ActionRail` | 固定响应动作：指派、关闭、白名单、取证、剧本、导出 |
| `RouteManifest` | 路由、权限、验收点、菜单和面包屑的单一来源 |

### 5.4 标题与术语规范

后续整套 UI 设计必须统一中文标题体系，避免同一页面出现中英混排标题。技术词保留英文时，必须是业务上无法自然翻译或行业惯用的专名。

| 原标题模式 | 规范标题 | 说明 |
|---|---|---|
| `选中告警详情 / Selected Alert` | 选中告警详情 | 右侧事件上下文标题 |
| `资产上下文 / Asset Context` | 资产上下文 | 告警关联资产信息 |
| `风险评分 / Risk Score` | 风险评分 | 风险评分卡 |
| `响应处置 / Response Actions` | 响应处置 | 处置动作区 |
| `反馈与学习 / Feedback` | 反馈与学习 | 反馈标注和模型学习入口 |
| `模型状态 / Model Status` | 模型状态 | 模型版本、训练、检测状态 |
| `验收证据 / Acceptance Evidence` | 验收证据状态 | 面向验收闭环 |
| `告警时间线 / Alert Timeline` | 告警时间线 | 告警研判过程 |
| `关联告警簇 / Correlated Cluster` | 关联告警簇 | 关联图和聚类摘要 |
| `流量趋势 / Traffic Trend` | 流量趋势 | 趋势图标题 |
| `流量拓扑 / Traffic Topology` | 流量拓扑 | 流向拓扑图标题 |
| `证据列表 / Evidence List` | 证据列表 | PCAP、会话、日志证据 |

ImageGen 后续提示词必须包含以下约束：

```text
All headings and panel titles must be Chinese-only. Do not render bilingual titles like “中文 / English”.
Keep technical identifiers such as Kafka, Flink, ClickHouse, OpenSearch, NebulaGraph, MinIO, PCAP, TLS, IP, DNS, JA3.
Use one consistent Chinese enterprise UI typography scale: product title 24px, panel titles 15-16px, body/table 12-13px, metric numbers 20-28px.
Chinese panel titles that previously had “中文 / English” headings must keep the same visual pixel size as “园区拓扑总览（数字孪生）”; remove only the slash and English subtitle.
```

既有截图修订的硬性规则：

1. 如果需求是“其余不变”“不允许改变内容”“只替换某个标题/菜单/局部区域”，GPT 全图重绘结果不能直接作为最终稿。
2. GPT 只能用于生成候选视觉或局部风格参考；最终采用稿必须通过遮罩/局部编辑方式保证业务数据、图表、拓扑、表格、面板位置和数值不漂移。
3. 一旦 GPT 输出改变了非目标区域，例如告警名称、IP、时间、风险分、图表曲线、拓扑建筑、菜单结构或右侧详情，必须在存档台账标记为 `rejected`。
4. 后续全套 UI 图生成时，若是从零生成页面，可以使用 GPT 生成；若是修订已确认截图，只允许局部编辑，不允许全图重绘。

当前最终视觉效果图：

- 文件：`../04_assets/generated/campus_full_traffic_system_visual_reference_20260620_business_corrected.png`
- 处理方式：基于用户提供并确认的新业务口径截图作为最终视觉参考基线，当前页固定为“综合态势 / 态势大屏”，保留深色园区安全运营台、六组一级导航、综合态势二级菜单、采集链路、告警研判、证据取证、响应处置、反馈学习和验收证据闭环。
- 前端规范：`面向园区网络的全流量采集分析系统-UI前端规范.md`
- 保留原则：后续实现时以业务语义、信息结构、导航层级、状态颜色和组件密度为准；GPT 图中的小字如出现个别 OCR 偏差，不作为真实字段名和接口字段依据。

## 6. 关键页面规格

### 6.0 `/dashboard`、`/screen` 与专题面板的去重原则

`/dashboard`、`/screen` 和 `/topics` 共享同一套全局指标口径、视觉 token、站点/时间窗和真实 API 数据源，但必须承担不同业务职责：

| 页面 | 业务角色 | 中心对象 | 页面应该让用户完成 |
|---|---|---|---|
| `/dashboard` 仪表盘 | 安全值班员、研判员、平台管理员 | 待处理事项 | 快速判断今天先处理哪个告警、哪个采集问题、哪个取证/反馈/审计缺口 |
| `/screen` 态势大屏 | 领导、客户、评审、值班大厅 | 系统闭环叙事 | 一眼理解系统如何从全流量采集走到告警研判、PCAP 取证、响应反馈和验收证据 |
| `/topics` 专题面板 | 分析员、实施、售前、项目经理、验收人员 | 固化的攻击场景与证据范围 | 在同一页面持续观察加密隧道、数据外传或 APT 战役，并形成证据和报告 |

内容约束：

1. 仪表盘不得做成缩小版态势大屏；态势大屏不得做成放大版仪表盘。
2. 仪表盘保留脱敏运营 KPI、优先级待办队列、采集与数据健康门禁、告警处置阶段工作篮、证据/反馈质量摘要、验收缺口和建议下一步动作类型。
3. 态势大屏保留园区拓扑、采集管道、威胁态势、证据取证、响应反馈、运行底座和脱敏展示状态。
4. 需要筛选、批量操作、编辑、审批、下载、复核的内容放仪表盘或业务页；涉及人员归属、指派、值班表和交接名单的内容不放仪表盘主区，只能进入具备权限控制和审计记录的业务详情页。
5. 专题面板保留范围标签、保存视图、局部影响面、专题分析、关联证据、报告预览、订阅和导出。
6. UI suite 不再生成单张 `/topics` 专题面板页面图；加密隧道、数据外传和 APT 战役的拆分设计输入通过页内切换与专题背景承载，前端开发时必须合并到同一个 `/topics` 页面，不能拆成三个左侧菜单，也不能复用 `/dashboard` 或 `/screen` 的主工作区构图。
7. 后续任何页面如果 UI 设计图按 Tab 拆多张，前端都必须合并为一个路由页面内的 Tab/Segmented 状态；除非产品文档明确要求新增导航入口，否则不得新增左侧菜单、独立业务路由或额外 AppShell 入口。

### 6.1 `/screen` 态势感知大屏

目标：基于 Command Workbench，将概念图落成可进入工作流的只读或脱敏态势大屏。它的中心对象是系统闭环叙事，展示“看得见、处理得动、判得出、查得到、改得动、验得过”的完整路径。

必须包含：

- 园区拓扑总览：楼宇覆盖率、园区在线覆盖、核心/汇聚链路状态、异常链路位置、探针覆盖地图。
- 采集流处理管道：探针采集、协议解析、归一化、Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO。
- 威胁态势：攻击阶段热度、战役簇密度、风险区域密度、异常链路影响面、外联流向强度。
- 证据取证：PCAP 覆盖率、Session 还原率、日志关联率、对象存储归档率、hash 校验和签名 URL 状态。
- 响应与反馈：隔离、阻断、封禁、下发脚本、反馈标注、模型学习批次。
- 运行底座：大屏刷新间隔、拓扑渲染延迟、链路带宽水位、流向动画帧率、脱敏展示状态。

验收：

- 1920x1080、2560x1440、3840x2160 无遮挡。
- 无 4xx/5xx、无 `requestfailed`、无 runtime error。
- 支持只读 token 或脱敏演示模式。
- 指标口径与 Dashboard 共用全局定义，但主区不复用 Dashboard 的待办、SLA、证据缺口和复核队列。

### 6.2 `/dashboard` 运营工作台

目标：从“指标堆叠”升级为“今天需要关注什么”。它的中心对象是脱敏的待处理事项和运营工作量，展示异常原因、风险优先级、处理阶段、证据/反馈质量和下一步动作类型，不展示人员归属。

布局：

- 顶部：脱敏运营 KPI，包含超时 SLA、临近超时数、高危未处理、待取证、待反馈、待复核、队列积压量和今日闭环进度。
- 左中：优先级待办队列和告警处置阶段工作篮，按今日必处理、处理中、待反馈、待取证、待复核、需审计留痕分组；表格列只展示事件 ID、风险级别、资产组、业务系统、处置阶段、剩余时间和证据状态。
- 中部：采集与数据健康门禁矩阵，展示 Probe、Kafka、Flink、ClickHouse、OpenSearch、NebulaGraph、MinIO、PostgreSQL 的状态、失败原因、影响范围和最近更新时间。
- 右侧：验收缺口看板，展示待补证据、待回流样本、审计留痕缺口、工单逾期、合规门禁缺口和建议下一步动作类型。
- 底部：证据与反馈质量摘要，展示 PCAP/Session/日志证据完整度、反馈覆盖率、误报回流量、复核完成率和样本回流缺口。

禁止在仪表盘主区放置大尺寸园区拓扑、横向全链路管道、攻击阶段热力图、外联流向动画和领导汇报式闭环图；这些内容属于 `/screen`。

敏感信息边界：仪表盘主区不得出现责任人、负责人、头像、班组、值班表、个人账号、联系方式、待指派、未认领责任、交接名单或个人任务归属。确需查看人员或组织归属时，只能跳转到具备权限控制和审计记录的业务详情页。

### 6.3 `/topics` 专题面板

目标：把加密隧道、数据外传和 APT 战役三类攻击场景固化为可持续观察、可下钻研判、可导出材料的专题视角。`/topics` 是唯一现役二级菜单和前端路由，但不是单张 UI 生图对象；旧 `/topics/tunnel`、`/topics/exfil`、`/topics/apt` 只作为兼容深链，进入后映射到 `/topics?topic=...`。

必须包含：

- 专题入口分工：左侧菜单只有 `/topics` 专题面板；页内切换 `加密隧道专题`、`数据外传专题`、`APT 战役专题`。
- 专题范围定义：站点、部门、资产组、业务系统、IP 段、账号、协议、规则、模型、攻击阶段、时间窗和来源页面。
- 专题专属 KPI：专题资产数、活跃探针数、专题流量、关联告警数、高危资产数、关联战役数、证据完整度、报告就绪度、未闭环风险数。
- 局部影响面：专题范围内资产、探针、链路、外联目的地、账号、服务、告警和战役关系。
- 专题分析：隧道协议和高频源、数据外传源和路径、APT 战役阶段、园区/部门/业务系统风险、验收门禁差距。
- 关联事件与证据：告警、战役、PCAP、Session、日志、图谱路径、审计记录。
- 专题报告与交付：风险摘要、趋势、处置进度、证据清单、模型/规则版本、试点周报、验收材料导出。

禁止把专题页做成小号仪表盘或小号态势大屏。它可以承接 `/alerts` 的保存筛选视图、`/assets` 的资产组、`/encrypted-traffic` 的隧道/外传分析、`/campaigns` 的战役视角和 `/compliance` 的验收主题，但主页面必须围绕“专题范围”组织。

交互边界：

- 专题专属浮层：创建专题、保存视图、编辑专题范围/模板/名称/负责人/刷新周期、报告导出、证据包导出、试点周报导出、订阅、静默、分享和收藏。
- 业务详情跳转：告警详情、战役详情、PCAP/Session/证据、资产详情、实体图谱、加密流量、合规审计和审计日志。
- 上下文传递：所有跳转必须携带 `topicId`、时间窗、资产组、攻击阶段和筛选条件，业务页必须能返回原专题。
- 危险动作边界：专题浮层不执行隔离、阻断、封禁、模型发布、规则发布等业务动作；这些动作回到告警、SOAR、MLOps、规则或部署页面完成。

### 6.4 `/alerts` 与 `/alerts/:alertId`

目标：把告警列表和详情统一成 Command Workbench 的主操作面，并保留 Analyst Workbench 的高效三栏研判结构。

布局：

- 左侧告警队列：筛选、排序、批量操作。
- 中央研判：摘要、时间线、攻击阶段、原因码。
- 右侧证据：PCAP、Session、日志、资产、图谱。
- 底部动作：指派、状态更新、白名单、反馈、剧本。

### 6.5 `/forensics` 与 `/graph`

目标：让取证不是孤立下载，而是形成证据链。

要求：

- PCAP 下载必须展示 hash、租户、过期时间和审计状态。
- 图谱路径要能从告警、资产、IP、用户跳转进入。
- 支持证据包导出和审计留痕。

### 6.6 `/data-quality`、`/compliance`、`/audit-log`

目标：构成 Evidence & Acceptance Suite。

要求：

- 以 P0/P1 门禁、证据完整度、专项测试状态组织页面。
- 支持一键查看证据来源、责任人、状态和下一步动作。
- 和 `02_acceptance` 的证据包目录保持一致。

## 7. 开发落地顺序

| 顺序 | 任务 | 优先级 |
|---:|---|---|
| 1 | 建立 `RouteManifest`：菜单、路由、权限、验收点、页面分组统一 | P0 |
| 2 | 修复 `/screen` 安全边界：只读 token、脱敏模式、WebSocket 授权后连接 | P0 |
| 3 | 落地 Command Workbench 到 `/screen`、`/dashboard`、`/alerts` 主链路 | P1 |
| 4 | 将 Command Workbench 扩展到 `/alerts/:alertId`、`/forensics`、`/graph`、`/assets` | P1 |
| 5 | 落地 Evidence & Acceptance Suite 到 `/data-quality`、`/compliance`、`/audit-log` | P1 |
| 6 | 增加 Playwright 多分辨率设计巡检 | P1 |
| 7 | 同步 PRD/SDD DOCX 页面规格矩阵 | P2 |

## 8. 可复用 ImageGen 提示词

### 8.1 Command Deck

```text
Create a realistic, production-quality desktop UI mockup, 1440 x 1024, for a campus network full-traffic collection and analysis system. Direction: Command Deck. Dark enterprise SOC command center, campus digital twin, collection pipeline, Kafka/Flink stream status, data lake, detection analytics, alert triage, PCAP evidence, asset context, response actions, feedback learning. Communicate the operational loop from collection to investigation, evidence, response learning, and acceptance evidence. The left first-level navigation must use business-domain labels: 综合态势, 威胁分析, 资产图谱, 检测运营, 审计配置. Use React + Ant Design + ECharts design language, restrained cyan/blue highlights with green/amber/red semantic accents, dense but clear layout, no marketing hero, no browser chrome, no decorative orbs, no cards inside cards.
```

### 8.2 Analyst Workbench

```text
Create a realistic, production-quality desktop UI mockup, 1440 x 1024, for a campus network security analyst workbench. Direction: Analyst Workbench. Three-column investigation layout with alert queue, selected alert timeline, PCAP/session/log evidence, asset topology, response actions and feedback. Chinese labels, dark professional product UI, Ant Design/ECharts compatible, task-focused, readable, dense tables with strong hierarchy, no excessive glow, no browser chrome, no stock imagery.
```

### 8.3 Evidence & Acceptance Suite

```text
Create a realistic, production-quality desktop UI mockup, 1440 x 1024, for an engineering-grade evidence and acceptance console. Direction: Evidence & Acceptance Suite. Show release baseline, P0/P1 gate matrix, evidence package status, live chain health, detection quality, production security readiness, open gaps, audit trail, selected gate details drawer. Dark charcoal enterprise style with cyan/green/amber/red semantic indicators, structured and calm, no marketing hero, no browser chrome, no cards inside cards.
```

### 8.4 园区网络全流量采集与分析系统

```text
Create one realistic, production-quality widescreen UI mockup at exactly 1920x1080 px, 16:9 composition, for a campus network full-traffic collection and analysis system. The main screen title must be 园区网络全流量采集与分析系统. Do not use 指挥研判平台 as the product title. Command Workbench is only an internal visual direction.

The left sidebar must have two visible levels. Level 1: 综合态势, 采集监测, 威胁分析, 资产图谱, 检测运营, 审计配置. Level 2: when 威胁分析 is active, show 告警中心, 战役列表, 攻击链分析, 加密流量, 取证分析, and highlight 告警中心 as the current page. Communicate the closed loop from full-traffic collection to alert investigation, PCAP evidence, response action, feedback learning, and acceptance evidence, but do not render 看见, 研判, 取证, 治理, 验收 as left navigation labels.

Use a full-screen enterprise app shell. Top: title, site/time/risk/collection health bar. Main center: campus topology and collection pipeline, selected alert timeline, correlated alert cluster, traffic flow chart, PCAP/session/log evidence table. Right fixed action rail: asset context, response actions, feedback labeling, model status and acceptance evidence. Dark enterprise cybersecurity SaaS UI, Ant Design and ECharts compatible, readable Chinese labels, deep charcoal background, cyan data flow, green healthy/closed-loop, amber medium risk, red high risk, 8px or smaller radius, no browser chrome, no marketing hero, no decorative orbs, no cards inside cards, no unreadably tiny text.
```

### 8.5 GPT 生成图存档规则

每次调用 GPT 画图功能后，必须完成以下动作：

1. 从会话日志或图像工具默认输出中提取最终 PNG。
2. 将 PNG 保存到 `doc/04_assets/generated/`，使用语义化文件名和日期后缀。
3. 同目录保存 `.prompt.txt`，记录本次最终提示词。
4. 更新 `doc/04_assets/generated/IMAGEGEN_ARCHIVE.md`，标注采用、废弃或被后续版本替代。
5. 不允许只把图片留在聊天窗口中作为唯一来源。

## 9. 决策建议

已确认主路线选择：

1. 综合态势、威胁分析、资产图谱中的态势、告警和证据主链路采用 Command Workbench 指挥研判台，作为 1+2 混合视觉的统一主线。
2. `/screen` 保留 Command Deck 的全局态势表达，但必须服务于业务入口，不做孤立大屏。
3. `/alerts`、`/alerts/:alertId`、`/forensics`、`/graph` 保留 Analyst Workbench 的操作效率和证据密度。
4. 检测运营、审计配置中的合规、审计、项目交付采用 Evidence & Acceptance Suite。

这不是三套互斥风格，而是一套产品系统里的三种工作模式：指挥研判、检测运营、审计交付。视觉 token、导航、状态标签和指标口径必须统一。

## 10. 当前前端页面覆盖映射矩阵

本矩阵用于确认已选混合方向和支撑方向是否覆盖当前前端业务场景，并作为后续 `RouteManifest`、页面重构、设计评审和 Playwright 验收的输入。

当前 GPT 生图版图套装已按本矩阵推进视觉覆盖：每个路由至少有一张页面图；源码中已确认的关键弹窗、抽屉、下拉和删除确认状态均应在 `doc/04_assets/ui_suite_gpt_v1/manifest.json` 与对应 prompt 中维护。

| 当前路由 | 当前菜单/页面 | 主 UI 方向 | 辅助方向 | 核心组件 | 设计验收点 |
|---|---|---|---|---|---|
| `/login` | 登录 | Evidence & Acceptance Suite | Analyst Workbench | 登录表单、SSO/OIDC 入口、租户/站点提示、错误态 | 支持认证失败、过期跳转、回跳原页面；视觉与深色产品系统一致 |
| `/screen` | 态势大屏 | Command Workbench | Evidence & Acceptance Suite | 园区数字孪生、采集链路、告警态势、PCAP 证据、响应动作、反馈学习 | 1080p/2K/4K 无遮挡；只读 token 或脱敏模式；指标口径与 Dashboard 一致 |
| `/dashboard` | 仪表盘 | Command Workbench | Evidence & Acceptance Suite | 风险总览、采集健康、待办队列、Top 资产、证据完整度 | 让值班员 30 秒内知道当前最该处理的问题；支持时间窗和站点切换 |
| `/topics` | 专题面板 | Command Workbench | Analyst Workbench | 页内切换加密隧道、数据外传、APT 战役；4K 专题背景、专题信号、证据门禁、报告动作 | 与加密流量、取证分析、实体图谱、资产台账、行为基准、战役列表、攻击链分析和审计日志联动；旧 `/topics/tunnel`、`/topics/exfil`、`/topics/apt` 只作为兼容深链 |
| `/probes` | 探针管理 | Command Workbench | Evidence & Acceptance Suite | 探针列表、采集健康、版本、心跳、吞吐、丢包 | 支持离线/异常/升级状态；可追溯到采集覆盖和性能验收 |
| `/alerts` | 告警中心 | Command Workbench | Analyst Workbench | 告警队列、筛选器、风险排序、批量动作、状态标签 | 高危优先、可批量处置、状态值与后端状态机一致 |
| `/alerts/:alertId` | 告警详情 | Command Workbench | Analyst Workbench | 研判摘要、时间线、证据抽屉、资产上下文、反馈动作 | 从告警到 PCAP、图谱、反馈、审计形成闭环 |
| `/campaigns`、`/campaigns/:campaignId` | 战役列表 | Command Workbench | Analyst Workbench | 战役列表、攻击阶段、关联告警、影响资产 | 能把多告警聚合成战役，并跳转到证据链 |
| `/attack-chains` | 攻击链分析 | Command Workbench | Analyst Workbench | 攻击链图、阶段时间线、实体关联、处置建议 | 攻击路径、阶段、证据和下一步动作清晰可读 |
| `/encrypted-traffic` | 加密流量 | Command Workbench | Analyst Workbench | JA3/JA3S、隧道检测、外联行为、异常连接 | 解释为何可疑，并能跳转到相关告警/资产/PCAP |
| `/forensics` | 取证分析 | Command Workbench | Analyst Workbench | PCAP 索引、下载、hash、签名 URL、审计状态 | hash、租户、过期时间、下载审计可见；跨租户拒绝可验 |
| `/graph` | 实体图谱 | Command Workbench | Analyst Workbench | 实体搜索、邻居图、路径分析、关系筛选 | 从告警/资产/IP/用户进入图谱时上下文不丢失 |
| `/assets` | 资产台账 | Command Workbench | Analyst Workbench | 资产列表、风险画像、开放端口、归属、最近告警 | 支持资产到告警、取证、图谱的闭环跳转 |
| `/fusion` | 数据融合 | Command Workbench | Analyst Workbench | 数据源状态、融合规则、实体对齐、质量雷达 | 展示融合增益和字段一致性，而不仅是数据源列表 |
| `/baselines` | 行为基准 | Command Workbench | Analyst Workbench | 基线分布、异常偏离、用户/资产画像、阈值说明 | 能解释异常原因，并沉淀到检测/告警策略 |
| `/rules` | 规则管理 | Evidence & Acceptance Suite | Analyst Workbench | 规则列表、版本、命中率、灰度、回滚 | 规则生命周期清晰，变更可审计，能关联误报反馈 |
| `/whitelist` | 白名单 | Evidence & Acceptance Suite | Analyst Workbench | 白名单草案、审批、命中、过期、影响范围 | 白名单来源、有效期、责任人和审计记录完整 |
| `/deployments` | 部署管理 | Evidence & Acceptance Suite | Command Deck | 服务状态、版本、发布批次、回滚、健康检查 | release baseline、镜像、配置、状态和回滚路径可追溯 |
| `/models` | 模型管理 | Evidence & Acceptance Suite | Analyst Workbench | 模型版本、指标、解释、激活、回滚 | champion/challenger、阈值、样本集和质量指标可追溯 |
| `/mlops` | MLOps 编排 | Evidence & Acceptance Suite | Analyst Workbench | 训练任务、评估、注册、发布、反馈样本 | 反馈 -> 训练 -> 评估 -> 发布形成闭环；失败态可恢复 |
| `/data-quality` | 数据质量 | Evidence & Acceptance Suite | Command Deck | 完整性、延迟、重复、字段质量、Topic 健康 | 与验收证据包关联，支持按数据源/Topic/时间窗追溯 |
| `/playbooks` | SOAR 剧本 | Evidence & Acceptance Suite | Analyst Workbench | 剧本列表、触发条件、执行历史、人工确认 | 剧本动作不越权；执行结果进入审计和告警闭环 |
| `/compliance` | 合规审计 | Evidence & Acceptance Suite | Command Deck | 合规报告、门禁矩阵、证据包、导出 | 区分 smoke、regression、acceptance、third-party 证据 |
| `/audit-log` | 审计日志 | Evidence & Acceptance Suite | Analyst Workbench | 操作日志、筛选、关联对象、导出、留存状态 | 所有关键动作可按用户、租户、对象、时间追溯 |
| `/notifications` | 通知配置 | Evidence & Acceptance Suite | Analyst Workbench | 通知渠道、告警订阅、升级策略、测试发送 | 通知策略与严重级别、责任人、值班表匹配 |
| `/settings` | 系统设置 | Evidence & Acceptance Suite | Command Deck | 租户、权限、集成、凭据、系统参数 | 敏感配置有权限、遮蔽、审计和变更确认 |
| `*` | 404/异常页 | Evidence & Acceptance Suite | - | 错误说明、返回入口、追踪码 | 不能暴露敏感信息；提供回到工作流的明确路径 |

## 11. 覆盖结论

已选混合方向能够覆盖当前前端业务场景，但落地时必须遵守以下边界：

1. Command Workbench 覆盖综合态势、威胁分析、资产图谱中的主操作场景，是后续主前端的统一视觉基线。
2. Command Deck 和 Analyst Workbench 分别作为 Command Workbench 的视觉来源，而不是独立落地成两套割裂界面。
3. Evidence & Acceptance Suite 覆盖检测运营、审计配置、合规交付，是项目经理、测试、实施和第三方评审的主工作面。
4. 三套方向共享同一套设计 token、状态标签、时间窗、指标解释、RouteManifest 和权限边界。
5. 后续前端重构不能按页面孤立改样式，应按业务导航批次推进：Command Workbench 主链、证据上下文扩展、检测运营与审计交付三个批次最合理。

## 12. RouteManifest 设计草案

`RouteManifest` 是后续前端 UI 重构的单一入口配置。它不只是菜单数组，而是把路由、权限、页面工作流、UI 方向、核心组件、数据依赖和验收点放到同一个结构里，解决当前菜单、路由、权限、验收和设计口径分散的问题。

### 12.1 字段模型

```ts
type RouteGroupId =
  | 'overview'
  | 'threat-analysis'
  | 'asset-graph'
  | 'detection-ops'
  | 'audit-config';
type RouteWorkflow = 'observe' | 'triage' | 'evidence' | 'governance' | 'acceptance';
type UIDirection =
  | 'command-workbench'
  | 'command-deck'
  | 'analyst-workbench'
  | 'evidence-acceptance-suite';
type AuthMode = 'public' | 'private' | 'readonly-token' | 'masked-demo';

interface RouteManifestItem {
  routeId: string;
  path: string;
  label: string;
  groupId: RouteGroupId;
  workflow: RouteWorkflow;
  primaryDirection: UIDirection;
  secondaryDirection?: UIDirection;
  authMode: AuthMode;
  permission: string;
  pageComponent: string;
  keyComponents: string[];
  dataDomains: string[];
  acceptanceIds: string[];
  entryFrom: string[];
  exitsTo: string[];
}
```

### 12.2 分组定义

| groupId | 一级菜单名称 | 覆盖 workflow | UI 主方向 | 业务目标 |
|---|---|---|---|---|
| `overview` | 综合态势 | observe | Command Workbench | 让用户快速掌握风险态势、采集健康和关键异常 |
| `collection-monitoring` | 采集监测 | observe / acceptance | Command Workbench | 展示探针覆盖、采集完整性、链路延迟、数据质量和存储健康 |
| `threat-analysis` | 威胁分析 | triage / evidence | Command Workbench | 把告警、战役、攻击链、加密流量和 PCAP 入口组织成可解释事件 |
| `asset-graph` | 资产图谱 | evidence / observe | Command Workbench | 从资产、实体图谱、融合数据和行为基准建立上下文 |
| `detection-ops` | 检测运营 | governance / acceptance | Evidence & Acceptance Suite | 管理规则、白名单、部署、模型、MLOps 和剧本 |
| `audit-config` | 审计配置 | acceptance / governance | Evidence & Acceptance Suite | 承接合规审计、审计日志、通知配置、系统设置和交付证据 |

### 12.3 路由草案

| routeId | path | groupId | UI 方向 | authMode | permission | acceptanceIds |
|---|---|---|---|---|---|---|
| `login` | `/login` | `audit-config` | Evidence & Acceptance Suite | public | `auth.login` | `AUTH-LOGIN-01` |
| `screen` | `/screen` | `overview` | Command Workbench | readonly-token / masked-demo | `screen.view` | `UI-SCREEN-01`、`SEC-SCREEN-01`、`RESP-4K-01` |
| `dashboard` | `/dashboard` | `overview` | Command Workbench | private | `dashboard.view` | `OBS-DASH-01` |
| `topics` | `/topics` | `overview` | Command Workbench | private | `topics.view` | `OBS-TOPIC-PANEL-01` |
| `probes` | `/probes` | `collection-monitoring` | Command Workbench | private | `probes.view` | `OBS-PROBE-01`、`PERF-CAPTURE-01` |
| `alerts` | `/alerts` | `threat-analysis` | Command Workbench | private | `alerts.view` | `TRIAGE-LIST-01`、`STATE-ALERT-01` |
| `alert-detail` | `/alerts/:alertId` | `threat-analysis` | Command Workbench | private | `alerts.detail` | `TRIAGE-DETAIL-01`、`EVI-LINK-01` |
| `campaigns` | `/campaigns` | `threat-analysis` | Command Workbench | private | `campaigns.view` | `TRIAGE-CAMPAIGN-01` |
| `campaign-detail` | `/campaigns/:campaignId` | `threat-analysis` | Command Workbench | private | `campaigns.detail` | `TRIAGE-CAMPAIGN-02` |
| `attack-chains` | `/attack-chains` | `threat-analysis` | Command Workbench | private | `attack_chains.view` | `TRIAGE-CHAIN-01` |
| `encrypted-traffic` | `/encrypted-traffic` | `threat-analysis` | Command Workbench | private | `encrypted_traffic.view` | `TRIAGE-ENC-01` |
| `forensics` | `/forensics` | `threat-analysis` | Command Workbench | private | `forensics.view` | `PCAP-HASH-01`、`PCAP-TENANT-01` |
| `graph` | `/graph` | `asset-graph` | Command Workbench | private | `graph.view` | `GRAPH-CONTEXT-01` |
| `assets` | `/assets` | `asset-graph` | Command Workbench | private | `assets.view` | `ASSET-CONTEXT-01` |
| `fusion` | `/fusion` | `asset-graph` | Command Workbench | private | `fusion.view` | `FUSION-GAIN-01` |
| `baselines` | `/baselines` | `asset-graph` | Command Workbench | private | `baselines.view` | `BASELINE-EXPLAIN-01` |
| `rules` | `/rules` | `detection-ops` | Evidence & Acceptance Suite | private | `rules.manage` | `RULE-LIFECYCLE-01` |
| `whitelist` | `/whitelist` | `detection-ops` | Evidence & Acceptance Suite | private | `whitelist.manage` | `WL-AUDIT-01` |
| `deployments` | `/deployments` | `detection-ops` | Evidence & Acceptance Suite | private | `deployments.manage` | `REL-BASELINE-01` |
| `models` | `/models` | `detection-ops` | Evidence & Acceptance Suite | private | `models.manage` | `MODEL-GOV-01` |
| `mlops` | `/mlops` | `detection-ops` | Evidence & Acceptance Suite | private | `mlops.manage` | `MLOPS-LOOP-01` |
| `playbooks` | `/playbooks` | `detection-ops` | Evidence & Acceptance Suite | private | `playbooks.manage` | `SOAR-AUDIT-01` |
| `data-quality` | `/data-quality` | `collection-monitoring` | Evidence & Acceptance Suite | private | `data_quality.view` | `DATA-QUALITY-01` |
| `compliance` | `/compliance` | `audit-config` | Evidence & Acceptance Suite | private | `compliance.view` | `ACCEPT-GATE-01` |
| `audit-log` | `/audit-log` | `audit-config` | Evidence & Acceptance Suite | private | `audit.view` | `AUDIT-TRACE-01` |
| `notifications` | `/notifications` | `audit-config` | Evidence & Acceptance Suite | private | `notifications.manage` | `NOTIFY-ESC-01` |
| `settings` | `/settings` | `audit-config` | Evidence & Acceptance Suite | private | `settings.manage` | `CONFIG-AUDIT-01` |
| `not-found` | `*` | `audit-config` | Evidence & Acceptance Suite | public | `system.not_found` | `ERROR-SAFE-01` |

### 12.4 导航与页面联动规则

二级菜单内的功能点、数据内容、表现形式和闭环动作以 `doc/01_design/面向园区网络的全流量采集分析系统-二级菜单功能点与表现形式矩阵.md` 为准；本节只保留跨页面联动原则。

| 起点 | 推荐出口 | 设计目的 |
|---|---|---|
| `/screen` | `/dashboard`、`/alerts`、`/probes` | 大屏只负责态势和演示，深入操作回到工作台 |
| `/dashboard` | `/alerts`、`/forensics`、`/data-quality` | 从总览进入待办、证据和验收缺口 |
| `/alerts` | `/alerts/:alertId`、`/forensics`、`/graph`、`/playbooks` | 告警处置必须进入证据、上下文和响应动作 |
| `/forensics` | `/alerts/:alertId`、`/graph`、`/audit-log` | 取证结果必须能回到事件和审计 |
| `/models`、`/mlops` | `/alerts`、`/data-quality`、`/audit-log` | 模型治理必须能关联反馈样本、质量和审计 |
| `/compliance` | `/data-quality`、`/deployments`、`/audit-log` | 验收门禁必须能追到证据来源 |

### 12.5 设计验收 ID 说明

| 前缀 | 含义 | 示例 |
|---|---|---|
| `OBS` | 态势类验收 | `OBS-DASH-01`：Dashboard 指标口径和时间窗统一 |
| `TRIAGE` | 告警分析类验收 | `TRIAGE-DETAIL-01`：告警详情形成时间线和证据链 |
| `EVI` / `PCAP` | 证据类验收 | `PCAP-HASH-01`：PCAP hash、租户和审计可见 |
| `RULE` / `MODEL` / `MLOPS` | 检测运营类验收 | `MLOPS-LOOP-01`：反馈到训练、评估、发布闭环 |
| `ACCEPT` / `REL` | 合规交付类验收 | `REL-BASELINE-01`：发布基线可追溯 |
| `SEC` | 安全边界验收 | `SEC-SCREEN-01`：大屏未授权访问边界明确 |
| `RESP` | 响应式设计验收 | `RESP-4K-01`：1080p/2K/4K 无遮挡 |

### 12.6 后续落地建议

1. 先把 `RouteManifest` 做成前端只读配置，不急于替换所有页面。
2. 用它生成侧边栏、面包屑、页面标题、权限声明和验收点说明。
3. 把 `/screen` 的 `authMode` 从公开页面改为 `readonly-token` 或 `masked-demo`。
4. Playwright 巡检按 `acceptanceIds` 组织，而不是只按 URL 成功加载组织。
5. 后端权限可先映射到字符串 permission，后续再接 RBAC/ABAC 策略。
