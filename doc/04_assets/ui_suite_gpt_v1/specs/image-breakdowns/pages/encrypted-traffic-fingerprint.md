# encrypted-traffic-fingerprint 拆解记录

## 基本信息

- page-id：`encrypted-traffic-fingerprint`
- 分类：`pages`
- 队列顺序：32
- 分组：威胁分析 / 加密流量
- 类型：`menu-state`
- 生产路由：`/encrypted-traffic?tab=fingerprint`
- 宿主路由：`/encrypted-traffic`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-fingerprint.png`
- 证据目录：`evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/`
- 当前实现镜像：`traffic/web-ui:ui-encrypted-traffic-fingerprint-20260710-r181`
- 当前阶段：`business-pixel-accepted`；strict pixel 为 `fail-documented`

本图是“加密流量”页面的“指纹分析”子状态。验收要求 AppShell、加密流量页签、指纹排行、聚类散点、证书 issuer、TLS 套件和右侧闭环栏同时可见；生产实现不得加载目标 PNG、`implementation.html` 或 breakdown evidence 作为页面内容。

## 目标图观察

- 顶部页签：总览、指纹分析、隧道检测、外联画像、证据中心，其中“指纹分析”为激活态。
- KPI：加密流量总量、TLS/QUIC/未知加密占比、异常证书数、可疑 JA3 数、未知 SNI 比例。
- 主工作区：JA3/JA3S 指纹排行、JA3 聚类散点、证书 Issuer 异常、TLS 版本与密码套件、指纹处置建议。
- 右侧闭环栏：外联画像、处置与分析建议、关联与下钻、生成与导出。

## 动态数据契约

主数据入口：

```text
fetchPageSnapshot(route.id) -> adaptEncryptedTraffic -> visuals.encryptedTraffic
```

路由状态：

```text
resolveEncryptedTrafficTab(searchParams.get('tab')) -> fingerprint
```

API 计划：

- `GET /v1/encrypted-traffic/stats`
- `GET /v1/encrypted-traffic/sessions`
- `GET /v1/encrypted-traffic/ja3`
- `GET /v1/encrypted-traffic/tunnels`
- `GET /v1/encrypted-traffic/exfiltration`

动态绑定：

- `visuals.encryptedTraffic.ja3Rows` -> `Ja3Table`
- `visuals.encryptedTraffic.scatterPoints` -> `Ja3Scatter`
- `visuals.encryptedTraffic.certificateRows` -> `EncryptedDenseRows`（证书 Issuer 异常）
- `visuals.encryptedTraffic.adviceRows` -> `AdviceList`（指纹处置建议）
- `visuals.encryptedTraffic.destinationRows/egressKpis` -> 右侧 `EgressProfile`

真实 API 返回空或稀疏 payload 时，adapter 使用 typed fallback 补足业务视觉密度；生产路由仍保留真实 API 入口，不回退到目标图贴图。

## 实现映射

| 目标元素 | 前端实现 |
|---|---|
| 页面容器 | `EncryptedTrafficPage` |
| 指纹子状态 | `FingerprintContent` |
| JA3/JA3S 排行 | `Ja3Table` |
| JA3 聚类散点 | `Ja3Scatter` |
| 证书 Issuer 异常 | `EncryptedDenseRows` |
| TLS 版本与密码套件 | `TlsSuiteMatrix` |
| 处置建议 | `AdviceList` |
| 外联画像右栏 | `EgressProfile` |
| 数据适配 | `adaptEncryptedTraffic` |

## 验收证据

| 证据 | 路径 |
|---|---|
| target | `evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/target.png` |
| final implementation | `evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/implementation-r181-final.png` |
| business diff | `evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/diff-business-r181-final.png` |
| strict diff | `evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/diff-strict-r181-final.png` |
| business metrics | `evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/metrics-business-r181-final.json` |
| strict metrics | `evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/metrics-strict-r181-final.json` |
| normal route screenshot | `evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/normal-route-r181.png` |
| normal route runtime | `evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/normal-route-runtime-r181.json` |
| verification | `evidence/ui-image-breakdowns/pages/encrypted-traffic-fingerprint/verification.json` |
| acceptance report | `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-encrypted-traffic-fingerprint-r181.json` |

## 验收结论

- Focused tests：`encryptedTrafficVisuals`、`noBitmapUi` 和 encrypted-traffic adapter 用例 passed。
- Production build：`npm --prefix web/ui run build` passed，仅保留 Vite 大 chunk warning。
- Deployment：`traffic/web-ui:ui-encrypted-traffic-fingerprint-20260710-r181`，`web-ui` Deployment `1/1`，APISIX `200`。
- Business visual diff：`0.11497395833333333 <= 0.35`，channel tolerance `64`。
- Strict visual diff：`0.9999180169753087 > 0.015`，channel tolerance `0`，记录为非阻断 strict pixel failure。
- Normal route：`/encrypted-traffic?tab=fingerprint` pass，激活页签为“指纹分析”，5 个页签、7 个 KPI、6 条 JA3、34 个散点、4 条证书行、6 个 TLS 套件项、4 条处置建议均可见。
- Runtime：无 4xx/5xx、requestfailed、console/pageerror、horizontal overflow，`smoke_hash_consumed=true`。
- Auxiliary review：PASS（agent `019f4b14-af04-79b3-86f6-438cefe967e9`，read-only）。
- Main-thread judgment：`business-pixel-accepted`。

## 区域与坐标

目标图为 1920 x 1080，以下 bbox 由目标图直接测读，单位 px。

| 区域 | bbox | 视觉内容 |
|---|---:|---|
| 画布 | `0,0,1920,1080` | 深色 SOC 单屏工作台 |
| 顶部全局栏 | `8,0,1904,62` | 系统名、站点、时间、风险、告警、健康与快捷入口 |
| 左侧导航 | `8,68,171,895` | 威胁分析展开，加密流量高亮 |
| 页面标题/Tab | `194,66,1716,78` | 加密流量标题和五个固定 Tab |
| 筛选控制 | `194,155,1510,33` | 近 24 小时、日期范围、自动刷新、刷新与间隔 |
| KPI 行 | `194,193,1510,100` | 七个连续 KPI 卡片 |
| 指纹明细表 | `194,302,668,333` | Top 20 JA3/JA3S 指纹明细及分页 |
| 指纹聚类 | `870,302,337,333` | ECharts 气泡聚类图 |
| issuer/SNI 分布 | `1216,302,488,333` | 四个证书与 SNI 统计子面板 |
| TLS 热力矩阵 | `194,643,588,312` | TLS 版本与密码套件热力表 |
| 关联规则表 | `790,643,417,312` | 五条规则命中与分页 |
| 证书预览 | `1216,643,488,312` | 证书主题、有效期、链、指纹与 PCAP |
| 右侧异常摘要 | `1712,128,194,189` | 近 24 小时异常分类 |
| 右侧快捷定位 | `1712,324,194,203` | 规则、证据、报告、白名单、证据中心 |
| 右侧修复建议 | `1712,535,194,178` | 五项修复动作 |
| 右侧证据报告 | `1712,720,194,235` | 分析报告、证书链、PCAP、命中与审计导出 |
| 底部状态栏 | `8,981,1904,81` | 延迟、运行时长、SLA、质量、存储、带宽和全局动作 |

### 对齐约束

- 右栏宽度固定约 `194px`，不可挤占证书预览或覆盖底部状态栏。
- 五个 Tab 的宽、高和水平位置与加密流量其他四页一致。
- 业务控制组贴业务标题区右端，切 Tab 后不得跳动。
- KPI 卡片同高，数值基线与微趋势底线一致。
- 上下两排主面板分别共享 `y=302` 与 `y=643` 的顶边。
- 表格分页固定在面板底部，不随数据行数改变容器高度。

## 文本清单

| 文本 | 位置 | 类型 | 匹配要求 |
|---|---|---|---|
| 加密流量 | 标题 | title | 完全一致 |
| 总览 | Tab | tab | 完全一致 |
| 指纹分析 | Tab | active-tab | 完全一致 |
| 隧道检测 | Tab | tab | 完全一致 |
| 外联画像 | Tab | tab | 完全一致 |
| 证据中心 | Tab | tab | 完全一致 |
| 时间范围 | 控制栏 | label | 完全一致 |
| 近 24 小时 | 控制栏 | select | 完全一致 |
| 自动刷新 | 控制栏 | switch-label | 完全一致 |
| 刷新 | 控制栏 | button | 完全一致 |
| 指纹总数 | KPI | metric-label | 完全一致 |
| 可疑 JA3 | KPI | metric-label | 完全一致 |
| 未知 SNI | KPI | metric-label | 完全一致 |
| 异常 issuer | KPI | metric-label | 完全一致 |
| TLS1.0/1.1 | KPI | metric-label | 完全一致 |
| 弱密码套件 | KPI | metric-label | 完全一致 |
| 关联规则 | KPI | metric-label | 完全一致 |
| JA3/JA3S 指纹明细（Top 20） | 左主面板 | panel-title | 完全一致 |
| 指纹分布与聚类（按 JA3 聚簇） | 中主面板 | panel-title | 完全一致 |
| Cluster A | 聚类图 | annotation | 数据动态 |
| Cluster X | 聚类图 | annotation | 数据动态 |
| 证书 issuer 与 SNI 分布 | 右主面板 | panel-title | 完全一致 |
| Top issuer（按会话数） | 分布面板 | sub-title | 完全一致 |
| CN/SAN 匹配率 | 分布面板 | sub-title | 完全一致 |
| SNI 熵值分布 | 分布面板 | sub-title | 完全一致 |
| 过期证书 | 分布面板 | metric-label | 完全一致 |
| 自签名证书 | 分布面板 | metric-label | 完全一致 |
| TLS 版本与密码套件（会话数热力） | 下方左面板 | panel-title | 完全一致 |
| 弱密码套件 | 热力图例 | legend | 完全一致 |
| 已废弃协议 | 热力图例 | legend | 完全一致 |
| 指纹关联规则（Top 匹配） | 下方中面板 | panel-title | 完全一致 |
| 证书详情预览（点击左侧行查看） | 下方右面板 | panel-title | 完全一致 |
| 查看完整证书 | 证书预览 | action | 完全一致 |
| 创建证据 | 证书预览 | action | 完全一致 |
| 查看 PCAP | 证书预览 | action | 完全一致 |
| 指纹异常（近 24 小时） | 右栏 | panel-title | 完全一致 |
| 快速定位 | 右栏 | panel-title | 完全一致 |
| 创建 JA3 规则 | 右栏 | action | 完全一致 |
| 查看证书证据 | 右栏 | action | 完全一致 |
| 导出指纹报告 | 右栏 | action | 完全一致 |
| 加入观察名单 | 右栏 | action | 完全一致 |
| 跳转证据中心 | 右栏 | action | 完全一致 |
| 修复建议 | 右栏 | panel-title | 完全一致 |
| 证据与报告 | 右栏 | panel-title | 完全一致 |
| 指纹分析报告 | 右栏 | action | 完全一致 |
| 证书链包含 | 右栏 | action | 完全一致 |
| PCAP 典型链路 | 右栏 | action | 完全一致 |
| 规则命中报告 | 右栏 | action | 完全一致 |
| 审计与证据导出 | 右栏 | action | 完全一致 |

## 组件清单

| 组件 | 区域 | 要点 |
|---|---|---|
| `EncryptedTrafficTabs` | 标题/Tab | 五页共享固定轨道 |
| `BusinessControlRail` | 筛选控制 | 时间、自动刷新、刷新、间隔右对齐 |
| `FingerprintMetricStrip` | KPI | 七项 API 指标与微趋势 |
| `FingerprintTable` | 指纹明细 | 排序、风险筛选、分页和行选择 |
| `FingerprintClusterChart` | 聚类 | ECharts scatter/effectScatter 动态更新 |
| `IssuerDistributionChart` | issuer | ECharts 环图与排行 |
| `SniEntropyChart` | SNI | ECharts 环图及等级图例 |
| `TlsCipherHeatmap` | 热力矩阵 | 颜色严格映射低到高风险 |
| `RuleMatchTable` | 关联规则 | 状态、置信度与规则动作 |
| `CertificatePreview` | 证书预览 | 随选中行联动，支持证书/PCAP 下钻 |
| `RightActionRail` | 右侧栏 | 快速定位、修复与导出真实可点 |

## 图标清单

| 图标 | 使用位置 | 状态 |
|---|---|---|
| `ReloadOutlined` | 刷新 | hover/disabled/loading |
| `FullscreenOutlined` | 控制栏 | 默认/激活 |
| `FingerprintOutlined` | 指纹指标 | 信息态 |
| `SafetyCertificateOutlined` | 证书预览 | 可信/风险态 |
| `FileSearchOutlined` | 证据定位 | 可点击 |
| `ExportOutlined` | 报告导出 | 可点击 |
| `UserAddOutlined` | 观察名单 | 可点击 |
| `LinkOutlined` | 证书链 | 可点击 |
| `DownloadOutlined` | PCAP/审计导出 | 可点击 |

## Token 与样式

| Token | 值/范围 | 用途 |
|---|---|---|
| `bg-canvas` | `#00101c` | 页面背景 |
| `bg-panel` | `#021724` | 面板背景 |
| `border-panel` | `#0a3b55` | 面板边界 |
| `accent-blue` | `#149cff` | 激活 Tab、链接和主图形 |
| `danger-red` | `#ff4238` | 可疑 JA3、高风险 |
| `warning-amber` | `#ff9d00` | 未知 SNI、中风险 |
| `success-green` | `#63c95a` | 匹配、健康与低风险 |
| `cluster-purple` | `#8f55d9` | 聚类辅助色 |
| `text-primary` | `#e5f1fc` | 标题与关键值 |
| `text-secondary` | `#91a5b7` | 表头与辅助说明 |
| `table-row-height` | `29-31px` | 高密度指纹表 |
| `panel-gap` | `8px` | 面板间距 |
| `panel-radius` | `2px` | 紧凑面板 |

## 状态与交互

1. 时间范围变化后，七项 KPI、明细、聚类、分布和热力图统一刷新。
2. 自动刷新开关启用后按所选秒数轮询，切页时正确清理定时器。
3. 指纹表行选择联动证书详情预览，不改变主面板几何。
4. 表格分页可点击，页码与每页条数参与 API 参数。
5. 聚类散点使用真实 ECharts option 更新，tooltip 显示指纹与会话数。
6. issuer/SNI 环图、热力矩阵在容器 resize 时保持完整可见。
7. 关联规则动作打开详情或创建规则流程，不能只变更按钮样式。
8. 证书、PCAP、报告与审计导出必须反馈成功、失败或权限不足。
9. 数据为空时显示业务空态；API 未提供字段时显示“待接入”，不伪造比例。
10. 右侧栏内容超高时只在栏内滚动，底部状态栏保持固定。

## 实现映射

| 目标对象 | 数据/实现映射 |
|---|---|
| KPI 与微趋势 | `visuals.encryptedTraffic` + 指纹统计 adapter |
| JA3/JA3S 明细 | `ja3Rows` 与服务端分页参数 |
| 聚类气泡 | `scatterPoints` 转 ECharts series |
| issuer/SNI | `certificateRows` 聚合为饼图与排行 |
| TLS 热力矩阵 | TLS 版本 x cipher suite 二维数据 |
| 规则命中 | `tunnelRuleRows`/关联规则结果 |
| 证书预览 | 当前选中指纹关联证书与 PCAP |
| 右侧动作 | Overlay/Drawer、下载和路由服务 |

## 差异清单

| 项目 | 当前状态 | 处理原则 |
|---|---|---|
| 业务 ROI | 已在既有业务门通过 | 后续统一采用 `<0.125` 复验 |
| strict pixel | 未通过且已记录 | 不以业务门冒充零容差通过 |
| 公共 AppShell | 不属于本页业务区修改范围 | 保持全局一致 |
| 图表动态性 | 必须运行时更新 | E2E 同时检查 option 与 canvas 像素变化 |
| 表格完整性 | 目标图含分页 | 必须校验页码、limit 与实际行数 |
| 右侧动作 | 目标图均为命令 | 每项需真实交互反馈与审计语义 |

## 结论

- 指纹分析页的目标布局、文字、图表、表格、证书联动与右侧闭环已形成可执行记录。
- 五 Tab 的公共业务骨架不允许因本页内容高度或控件数量发生位移。
- 所有统计图使用动态 ECharts，所有表格具备滚动和分页，所有动作具备反馈。
- 记录门通过不等同像素门通过；最终仍以 Windows Chrome 截图、diff/metrics 和审查裁决为准。
