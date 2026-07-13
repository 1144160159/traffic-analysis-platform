# AppShell Global Chrome Standard

更新日期：2026-06-23

适用范围：`doc/04_assets/ui_suite_gpt_v1/screens/pages/` 下除 `login.png`、`screen.png`、登录/认证态和明确不展示 AppShell 的独立页面外的所有 UI 图。`dashboard.png`、`not-found.png` 和所有业务 page 都必须修复到本文标准。

视觉基准：态势大屏 `screen.png`。`dashboard.png` 不再作为公共 AppShell 基准，只作为需要修复到 `screen.png` 公共壳标准的页面。

## 最高优先级

1. 公共 AppShell 只认一套基准：当前 `screen.png` 的顶部单栏、左侧单栏、底部单栏。
2. 顶部、左侧、底部三块公共区域的内容、图标、顺序、尺寸、间距、分隔线、状态色、字号密度、背景、圆角和激活态必须与 `screen.png` 完全一致。
3. 不同页面只允许改变主内容区业务内容，以及左侧当前展开域、当前二级菜单文本和当前高亮项；不得改变公共区的图标体系、布局层级和状态栏项目。
4. 修复既有 page 图片时，只允许修改顶部、左侧和底部公共 AppShell 区域；中部业务内容区必须保持原图不变，不得重绘、替换指标、调整业务面板或改变业务布局。
5. 如本文文字描述与当前 `screen.png` 的实际公共区域出现冲突，以当前 `screen.png` 的实际视觉结果为最高优先级。

## 顶部单栏标准

顶部单栏是全局 AppShell，不属于页面业务内容。所有适用页面必须固定以下结构、顺序、图标风格、字号密度和分隔线，并以 `screen.png` 的实际顶部栏为准：

| 顺序 | 区域 | 固定内容 | 固定图标 ID | 图标语义 |
|---:|---|---|---|---|
| 1 | 品牌区 | `园区网络全流量采集与分析系统` | `top-brand-shield` | 盾牌 / 系统徽标 |
| 2 | 站点选择 | `站点` + 当前站点 | `top-site-selector` | 站点定位 / 下拉 |
| 3 | 时间 | 当前日期时间 | `top-time-clock` | 时钟 |
| 4 | 风险态势 | `整体风险` 或 `风险态势` + 风险等级/分值 | `top-risk-shield` | 风险盾牌 |
| 5 | 告警数量 | `告警数` 或 `告警总数` + 数量 | `top-alert-bell` | 告警铃 |
| 6 | 严重告警 | `严重告警` 或 `关键告警` + 数量 | `top-critical-alert` | 高危告警 |
| 7 | 采集健康 | `采集健康` 或 `采集健康度` + 百分比 | `top-collection-pulse` | 健康脉冲 |
| 8 | 数据质量 | `数据质量` + 百分比/合格率 | `top-data-quality-db` | 数据库 / 质量 |
| 9 | 快捷入口 | `PCAP检索` | `top-quick-pcap` | 包检索 |
| 10 | 快捷入口 | `资产检索` | `top-quick-asset` | 资产搜索 |
| 11 | 快捷入口 | `规则检索` | `top-quick-rule` | 规则文档搜索 |
| 12 | 快捷入口 | `脚本中心` | `top-quick-script` | 脚本终端 |
| 13 | 快捷入口 | `帮助中心` | `top-quick-help` | 帮助问号 |
| 14 | 快捷入口 | `更多应用` | `top-quick-more` | 更多应用九宫格 |

允许页面改变具体数值、站点名称和时间，但不得改变顶部单栏模块顺序、快捷入口集合、图标风格、字号密度、分隔方式和整体高度。

职责边界：

- 顶部单栏不得承载通知铃铛、用户头像、用户菜单、设置或电源动作组。
- `告警数量` 和 `严重告警` 是态势运行指标，不是通知中心入口。
- 通知角标、设置、全局配置和电源属于底部单栏右侧全局动作区。
- 用户身份、角色、在线状态和用户动作属于左侧单栏底部用户区，顶部不得重复展示。

禁止：

- 把顶部单栏改成页面业务 KPI。
- 删除或改名快捷入口。
- 在顶部单栏加入页面专属大按钮、搜索表单或营销文案。
- 在顶部单栏加入用户头像、用户菜单、通知中心入口、设置或电源动作组。
- 使用与 `screen.png` 不同的图标体系、圆角、颜色或密度。

## 左侧单栏一级菜单标准

左侧导航必须是 `screen.png` 同款单栏展开式菜单：同一个侧栏内承载一级菜单和当前一级业务域下的二级菜单。不得恢复为“窄一级栏 + 独立二级栏”的双栏结构。

左侧底部用户区是 AppShell 唯一常驻用户身份区域，用于承载当前账号、角色、在线状态和用户动作；顶部单栏不得重复绘制用户头像或用户菜单。

| 一级菜单 | 固定图标 ID | 图标语义 | 视觉要求 |
|---|---|---|---|
| 综合态势 | `primary-overview-grid` | 四宫格 / 总览网格 | 4 个小方块组成的总览图标，蓝色激活态，白灰默认态 |
| 采集监测 | `primary-collection-crosshair` | 采集准星 / 探针定位 | 十字准星或采集探针语义，线性描边 |
| 威胁分析 | `primary-threat-shield` | 威胁盾牌 / 攻击雷达 | 盾牌或雷达类安全威胁语义，线性描边 |
| 资产图谱 | `primary-asset-graph` | 关系节点 / 图谱网络 | 节点连接或资产关系图语义，线性描边 |
| 检测运营 | `primary-detection-hex` | 六边形检测 / 运营节点 | 检测节点或六边形运营语义，线性描边 |
| 审计配置 | `primary-audit-clipboard` | 审计剪贴板 / 配置记录 | 剪贴板、清单或审计记录语义，线性描边 |

## 菜单图标硬锁规则

所有通过 imagegen 生成、重生成或修复的页面，左侧一级菜单和二级菜单图标必须按本文件的固定图标 ID 执行，不允许只用自然语言近似描述，也不允许由 imagegen 自行替换成相似通用图标。

修复过程贴片目录已清理。后续生成、验收和必要的后处理校正统一以本文件固定图标 ID、图标语义和 `screen.png` 公共 AppShell 视觉基准为准。

一级菜单贴片命名约定：

| 一级菜单 | 固定图标 ID | 页面贴片命名 |
|---|---|---|
| 综合态势 | `primary-overview-grid` | `primary-overview-<page>.png` |
| 采集监测 | `primary-collection-crosshair` | `primary-collection-<page>.png` |
| 威胁分析 | `primary-threat-shield` | `primary-threat-<page>.png` |
| 资产图谱 | `primary-asset-graph` | `primary-asset-<page>.png` |
| 检测运营 | `primary-detection-hex` | `primary-ops-<page>.png` |
| 审计配置 | `primary-audit-clipboard` | `primary-audit-<page>.png` |

二级菜单贴片命名约定：`<固定图标 ID>-<page>.png`。例如 `/data-quality` 页面必须使用 `secondary-probe-crosshair-data-quality.png` 表示 `探针管理`，使用 `secondary-data-wave-check-data-quality.png` 表示 `数据质量`。两个二级图标不得互换，不得共用同一准星、加号、方框或列表类替代图标。

图标与菜单文字的中心线必须水平对齐；图标和文字之间不得出现阴影、遮挡、模糊边、额外竖线或裁切。一级菜单之间、展开二级菜单与下一组一级菜单之间的垂直间距必须保持 `screen.png` 的紧凑节奏，不得出现异常大空隙。

## 二级菜单固定图标清单

二级菜单图标是固定资产标准。所有 page、overlay、component、state 和 responsive 图中，只要出现对应二级菜单，都必须使用同一个图标 ID、同一个图标语义、同一线宽、尺寸、颜色状态和文本密度。

详情路由不新增二级菜单：`/alerts/:alertId` 继承 `告警中心` 的菜单高亮与图标；`/campaigns/:campaignId` 继承 `战役列表` 的菜单高亮与图标。

### 综合态势

| 二级菜单 | 路由 | 固定图标 ID | 图标语义 |
|---|---|---|---|
| 仪表盘 | `/dashboard` | `secondary-home-dashboard` | 房屋 / 首页仪表盘 |
| 态势大屏 | `/screen` | `secondary-situation-radar` | 圆形态势盘 / 地球雷达 |

### 采集监测

| 二级菜单 | 路由 | 固定图标 ID | 图标语义 |
|---|---|---|---|
| 探针管理 | `/probes` | `secondary-probe-crosshair` | 探针准星 / 采集节点 |
| 数据质量 | `/data-quality` | `secondary-data-wave-check` | 波形质检 / 数据校验 |

### 威胁分析

| 二级菜单 | 路由 | 固定图标 ID | 图标语义 |
|---|---|---|---|
| 告警中心 | `/alerts` | `secondary-alert-list` | 告警单 / 警报列表 |
| 战役列表 | `/campaigns` | `secondary-campaign-flag` | 战役旗帜 / 事件列表 |
| 攻击链分析 | `/attack-chains` | `secondary-attack-chain` | 链路路径 / 攻击链节点 |
| 加密流量 | `/encrypted-traffic` | `secondary-tls-lock` | 加密锁 / TLS 盾牌 |
| 取证分析 | `/forensics` | `secondary-evidence-search` | 证据文件 / 放大镜 |

### 资产图谱

| 二级菜单 | 路由 | 固定图标 ID | 图标语义 |
|---|---|---|---|
| 资产台账 | `/assets` | `secondary-asset-inventory` | 资产清单 / 设备列表 |
| 实体图谱 | `/graph` | `secondary-entity-graph` | 节点网络 / 关系图谱 |
| 数据融合 | `/fusion` | `secondary-fusion-merge` | 多源汇聚 / 合并节点 |
| 行为基准 | `/baselines` | `secondary-behavior-baseline` | 基线层 / 行为轮廓 |

### 检测运营

| 二级菜单 | 路由 | 固定图标 ID | 图标语义 |
|---|---|---|---|
| 规则管理 | `/rules` | `secondary-rule-doc` | 规则文档 / 检测条目 |
| 部署管理 | `/deployments` | `secondary-deploy-box` | 发布盒 / 部署节点 |
| 模型管理 | `/models` | `secondary-model-cube` | 模型立方体 / 算法块 |
| MLOps 编排 | `/mlops` | `secondary-mlops-dag` | 流水线节点 / 编排 DAG |
| SOAR 剧本 | `/playbooks` | `secondary-playbook-flow` | 剧本清单 / 自动化流程 |
| 白名单 | `/whitelist` | `secondary-whitelist-shield` | 盾牌勾选 / 例外放行 |

### 审计配置

| 二级菜单 | 路由 | 固定图标 ID | 图标语义 |
|---|---|---|---|
| 合规审计 | `/compliance` | `secondary-compliance-badge` | 合规证章 / 审计门禁 |
| 审计日志 | `/audit-log` | `secondary-audit-log` | 日志清单 / 时间记录 |
| 通知配置 | `/notifications` | `secondary-notification-bell` | 通知铃 / 消息通道 |
| 系统设置 | `/settings` | `secondary-settings-gear` | 齿轮 / 系统参数 |

## 底部单栏标准

底部单栏为全局 AppShell 状态，不属于页面业务内容。所有适用页面必须与当前 `screen.png` 的底部单栏保持一致，使用同一组运行状态项、同一顺序、同一图标语义和右侧全局动作区。

| 顺序 | 固定文案 | 固定图标 ID | 图标语义 | 示例内容 |
|---:|---|---|---|---|
| 1 | 数据延迟 | `bottom-latency-dot` | 健康圆点 / 延迟状态 | 1.23 s |
| 2 | 系统运行 | `bottom-uptime-bolt` | 闪电 / 运行状态 | 23 天 14 小时 |
| 3 | 告警处理SLA | `bottom-sla-diamond` | 菱形门禁 / SLA 状态 | 98.2% |
| 4 | 数据质量合格率 | `bottom-quality-pulse` | 脉冲 / 质量状态 | 99.1% |
| 5 | 存储使用 | `bottom-storage-box` | 存储盒 / 容量状态 | 68.7 / 120 TB (57%) |
| 6 | 带宽使用 | `bottom-bandwidth-link` | 圆形链路 / 带宽状态 | 42.7 / 100 Gbps (43%) |
| 7 | 日志吞吐 | `bottom-log-dot` | 日志圆点 / 吞吐状态 | 12.6 K EPS |
| 8 | 全局动作区 | `bottom-global-actions` | 通知角标 / 设置 / 全局配置 / 电源 | 与 `screen.png` 右下角图标组一致 |

禁止把底栏改为另一套组件状态栏。探针、Kafka、Flink、PostgreSQL/ClickHouse、模型服务、证书/密钥、最近同步等运行底座信息只能出现在页面主内容区或运行底座面板，不得替换当前 `screen.png` 的全局底部单栏。

## 修复和验收规则

1. 修复对象：`pages/` 下除 `login.png` 和 `screen.png` 外的所有 UI 图。
2. 修复边界：只允许处理顶部单栏、左侧单栏和底部单栏；中部业务内容区必须保持原图不变。
3. 左侧验收：一级菜单、二级菜单、图标、展开域、高亮项、用户区、背景、分割线、行高和激活样式必须与 `screen.png` 一致。
4. 顶部验收：品牌、站点/时间、风险、告警、采集健康、数据质量、快捷入口及其图标必须与 `screen.png` 一致。
5. 底部验收：固定八组状态/动作项必须与 `screen.png` 一致，不得出现页面专属底栏。
6. 内容验收：主工作区、右侧业务栏、表格、图表、业务指标和业务布局不得因公共区修复发生变化。

## 生图 Prompt 必须包含

后续所有 page prompt 必须显式加入：

```text
公共 AppShell 绝对一致性硬门禁：除 login.png、screen.png、登录/认证态和明确不展示 AppShell 的独立素材外，所有 UI 图的公共部分必须与态势大屏 screen.png 完全一致。公共部分包括顶部单栏、左侧单栏和底部单栏；三者的内容、图标、顺序、尺寸、间距、分隔线、状态色、字号密度、背景、圆角和激活态都不得按页面自行变化。顶部栏固定为 screen.png 的系统名称、站点/时间、风险态势、告警数、严重告警、采集健康、数据质量和快捷入口结构；快捷入口固定为 PCAP检索、资产检索、规则检索、脚本中心、帮助中心、更多应用。顶部单栏不得加入通知铃铛、用户头像、用户菜单、设置或电源动作组；顶部的告警数/严重告警只是运行指标，不是通知中心入口。左侧栏固定为 screen.png 的单栏展开式菜单，一级菜单图标和二级菜单图标必须遵循 doc/04_assets/ui_suite_gpt_v1/standards/APP_SHELL_ICON_STANDARD.md；不同页面只允许改变当前展开域、二级菜单文本和当前高亮项；用户身份、角色、在线状态和用户动作只归属左侧底部用户区，顶部不得重复展示。底部单栏固定为 screen.png 的数据延迟 / 系统运行 / 告警处理SLA / 数据质量合格率 / 存储使用 / 带宽使用 / 日志吞吐 / 右侧全局动作图标组；通知角标、设置、全局配置和电源只归属底部右侧全局动作区。修复既有 UI 图时，只允许修改顶部、左侧、底部公共区域，中部业务内容区必须保持原图不变，不得重绘、替换指标、调整业务面板或改变业务布局。
```
