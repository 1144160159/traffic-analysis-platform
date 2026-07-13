# encrypted-traffic 拆解记录

## 基本信息

- page-id：`encrypted-traffic`
- 分类：`pages`
- 队列顺序：31
- 分组：威胁分析
- 类型：`menu-route`
- 路由：`/encrypted-traffic`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic.png`
- 证据目录：`evidence/ui-image-breakdowns/pages/encrypted-traffic/`
- 当前实现镜像：`traffic/web-ui:ui-encrypted-traffic-20260710-r180`
- 当前阶段：`business-pixel-accepted`（业务容差）；strict pixel 为 fail-documented
- 禁止用途：不得作为生产业务页面截图资源加载

本图是完整 AppShell 页面，业务区为“加密流量”总览。验收要求顶部状态栏、左侧威胁分析菜单、底部状态栏和加密流量主工作区同时可见。

## 目标图观察

页面业务区包含：

- 顶部页签：总览、指纹分析、隧道检测、外联画像、证据中心。
- KPI：加密流量总量、TLS/QUIC/未知加密占比、异常证书数、可疑 JA3 数、未知 SNI 比例。
- 主画布：协议分布与趋势、Top JA3、JA3 散点、隧道检测卡片、异常隧道列表、证据与握手元数据表。
- 右侧闭环栏：外联画像、处置与分析建议、关联与下钻、生成与导出。

## 动态数据契约

主数据入口：

```text
fetchPageSnapshot(route.id) -> adaptEncryptedTraffic -> visuals.encryptedTraffic
```

API 计划：

- `GET /v1/encrypted-traffic/stats`
- `GET /v1/encrypted-traffic/sessions`
- `GET /v1/encrypted-traffic/ja3`
- `GET /v1/encrypted-traffic/tunnels`
- `GET /v1/encrypted-traffic/exfiltration`

动态绑定：

- `metrics`：7 个页面 KPI。
- `rows`：证据与握手元数据表。
- `visuals.encryptedTraffic.protocolRows/protocolTrend`：协议分布与趋势。
- `visuals.encryptedTraffic.ja3Rows/scatterPoints`：JA3 表与散点。
- `visuals.encryptedTraffic.tunnelCards/tunnelRows/tunnelRuleRows`：隧道检测与规则命中。
- `visuals.encryptedTraffic.destinationRows/egressKpis`：外联画像。
- `visuals.encryptedTraffic.certificateRows/evidenceRows/adviceRows`：证书、证据和处置建议。

真实 API 返回空或稀疏 payload 时，adapter 使用 typed fallback 补足业务视觉密度；生产路由仍保留真实 API 入口，不回退到目标图贴图。

## 实现映射

| 目标元素 | 前端实现 |
|---|---|
| 页面容器 | `EncryptedTrafficPage` |
| 协议分布与趋势 | `ProtocolDistribution` |
| JA3 表 | `Ja3Table` |
| JA3 散点 | `Ja3Scatter` |
| 隧道卡片/列表 | `TunnelFeatureCards` / `TunnelTable` |
| 证据表 | `EvidenceTable` |
| 外联画像 | `EgressProfile` |
| 处置建议 | `AdviceList` |
| 数据适配 | `adaptEncryptedTraffic` |
| 详情抽屉契约 | `OverlayContractHost` |

## 验收证据

| 证据 | 路径 |
|---|---|
| target | `evidence/ui-image-breakdowns/pages/encrypted-traffic/target.png` |
| final implementation | `evidence/ui-image-breakdowns/pages/encrypted-traffic/implementation-r180-final.png` |
| business diff | `evidence/ui-image-breakdowns/pages/encrypted-traffic/diff-business-r180-final.png` |
| strict diff | `evidence/ui-image-breakdowns/pages/encrypted-traffic/diff-strict-r180-final.png` |
| business metrics | `evidence/ui-image-breakdowns/pages/encrypted-traffic/metrics-business-r180-final.json` |
| strict metrics | `evidence/ui-image-breakdowns/pages/encrypted-traffic/metrics-strict-r180-final.json` |
| normal route screenshot | `evidence/ui-image-breakdowns/pages/encrypted-traffic/normal-route-r180.png` |
| normal route runtime | `evidence/ui-image-breakdowns/pages/encrypted-traffic/normal-route-runtime-r180.json` |
| verification | `evidence/ui-image-breakdowns/pages/encrypted-traffic/verification.json` |
| acceptance report | `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-encrypted-traffic-r180.json` |

## 验收结论

- Focused tests：`encryptedTrafficVisuals`、`noBitmapUi` 和 encrypted-traffic adapter 用例 passed。
- Production build：`npm --prefix web/ui run build` passed，仅保留 Vite 大 chunk warning。
- Deployment：`traffic/web-ui:ui-encrypted-traffic-20260710-r180`，`web-ui` Deployment `1/1`，APISIX `/` 返回 `200`。
- Focus runtime：Windows Chrome CDP pass，无 4xx/5xx、requestfailed、console/pageerror、forbidden target resource。
- Normal route：`/encrypted-traffic` pass，AppShell、5 个页签、7 个 KPI、协议区、6 条 JA3、34 个散点、6 个隧道卡片、6 条隧道行、6 条证据行和右侧动作均可见。
- Business visual diff：`0.11367332175925926` <= `0.35`，channel tolerance `64`。
- Strict visual diff：`0.9999262152777778` > `0.015`，channel tolerance `0`，记录为非阻断 strict pixel failure。
- Auxiliary review：PASS（agent `019f4b06-5981-7542-8112-19d447ba1144`，read-only）。
- Main-thread judgment：`business-pixel-accepted`。

## 区域与坐标

坐标以目标图左上角为原点，单位为 px；页面为 1920 x 1080。

| 区域 | bbox | 内容与对齐要求 |
|---|---:|---|
| 全画布 | `0,0,1920,1080` | 单屏 SOC 页面，不含浏览器外框 |
| 顶部状态栏 | `10,10,1900,68` | 品牌、站点、时间、风险、告警、采集健康与快捷入口 |
| 左侧导航 | `10,78,158,898` | 六个一级业务域；威胁分析展开且加密流量高亮 |
| 页面标题及控制 | `178,78,1728,74` | 标题、五 Tab 与右侧时间/刷新/分析按钮 |
| KPI 行 | `190,160,1274,99` | 七项指标连续同排，卡片间距约 8px |
| 外联摘要 | `1474,160,412,105` | 四个外联 KPI 与“查看详情” |
| 协议趋势 | `190,268,348,263` | 环图、图例与三条时间序列 |
| Top JA3 表 | `548,268,516,263` | 七行指纹排行与风险标签 |
| JA3 散点 | `1072,268,392,263` | 流量/会话数二维散点与风险图例 |
| 外联地图 | `1474,274,412,183` | 世界地图、外联弧线与流量图例 |
| 外联目的表 | `1474,457,412,170` | Top 目的地五行明细 |
| 隧道特征 | `190,544,396,178` | 六项隧道检测指标 |
| 异常隧道表 | `594,544,870,178` | 六类异常会话与持续时间/流量/风险 |
| 证据握手表 | `190,730,1274,224` | 最近证据，支持查看与下载 |
| 右侧闭环栏 | `1474,628,412,326` | 建议、下钻、生成与导出动作 |
| 底部状态栏 | `0,992,1920,88` | 数据延迟、SLA、质量、容量、带宽、吞吐与系统动作 |

### 几何复核要点

- 公共顶部、左侧和底部只记录位置，不在本业务页开发中重排。
- 五个 Tab 共用固定高度与起点，激活态只改变底色和描边。
- 时间范围、刷新、一键分析及刷新图标固定在标题栏最右侧。
- 中间业务区左右边界为 `190` 与 `1464`，右栏从 `1474` 开始。
- 三个第一行主面板顶边一致，均从 `y=268` 开始。
- 证据表底边不得压住 `y=992` 的全局状态栏。

## 文本清单

以下为目标图直接可见且必须保持业务语义的关键文案。

| 文本 | 区域 | 类型 | 要求 |
|---|---|---|---|
| 加密流量 | 页面标题 | title | 完全一致 |
| 总览 | 页签 | active-tab | 完全一致 |
| 指纹分析 | 页签 | tab | 完全一致 |
| 隧道检测 | 页签 | tab | 完全一致 |
| 外联画像 | 页签 | tab | 完全一致 |
| 证据中心 | 页签 | tab | 完全一致 |
| 时间范围 | 控制区 | label | 完全一致 |
| 近 24 小时 | 控制区 | select | 完全一致 |
| 一键分析 | 控制区 | primary-button | 完全一致 |
| 加密流量总量 | KPI | metric-label | 完全一致 |
| 78.3 Gbps | KPI | metric-value | 数据动态 |
| TLS 流量占比 | KPI | metric-label | 完全一致 |
| 63.7% | KPI | metric-value | 数据动态 |
| QUIC 流量占比 | KPI | metric-label | 完全一致 |
| 未知加密占比 | KPI | metric-label | 完全一致 |
| 异常证书数 | KPI | metric-label | 完全一致 |
| 可疑 JA3 数 | KPI | metric-label | 完全一致 |
| 未知 SNI 比例 | KPI | metric-label | 完全一致 |
| 协议分布与趋势 | 主面板 | panel-title | 完全一致 |
| 指纹分析（Top JA3） | 主面板 | panel-title | 完全一致 |
| JA3 分布（流量 vs 会话数） | 主面板 | panel-title | 完全一致 |
| 外联画像 | 右栏 | panel-title | 完全一致 |
| 境外 IP | 右栏 | metric-label | 完全一致 |
| CDN / 云服务 | 右栏 | metric-label | 完全一致 |
| 异常域名 | 右栏 | metric-label | 完全一致 |
| 首次出现目的地 | 右栏 | metric-label | 完全一致 |
| 隧道检测与异常特征 | 下方主区 | panel-title | 完全一致 |
| DNS over HTTPS 会话 | 下方主区 | metric-label | 完全一致 |
| 异常长连接（> 1h） | 下方主区 | metric-label | 完全一致 |
| 高熵流量（> 7.5） | 下方主区 | metric-label | 完全一致 |
| 低频流量（< 3.0） | 下方主区 | metric-label | 完全一致 |
| 低流量心跳（疑似） | 下方主区 | metric-label | 完全一致 |
| 疑似 VPN 会话 | 下方主区 | metric-label | 完全一致 |
| 异常隧道列表 | 下方主区 | panel-title | 完全一致 |
| 证据与握手元数据（最新 200 条） | 下方表格 | panel-title | 完全一致 |
| 处置与分析建议 | 右栏 | panel-title | 完全一致 |
| 关联与下钻 | 右栏 | panel-title | 完全一致 |
| 生成与导出 | 右栏 | panel-title | 完全一致 |
| 生成规则 | 右栏 | action | 完全一致 |
| 隔离主机 | 右栏 | action | 完全一致 |
| 评估白名单 | 右栏 | action | 完全一致 |
| 检查目的地 | 右栏 | action | 完全一致 |
| 关联告警（18） | 右栏 | action | 数字动态 |
| 关联战役（2） | 右栏 | action | 数字动态 |
| 攻击链分析 | 右栏 | action | 完全一致 |
| 实体图谱 | 右栏 | action | 完全一致 |
| 取证分析 | 右栏 | action | 完全一致 |
| PCAP 检索 | 右栏 | action | 完全一致 |
| 创建告警 | 右栏 | action | 完全一致 |
| 创建战役 | 右栏 | action | 完全一致 |
| 生成报告 | 右栏 | action | 完全一致 |
| 导出 PCAP 索引 | 右栏 | action | 完全一致 |
| 导出证书 | 右栏 | action | 完全一致 |
| 写入审计日志 | 右栏 | action | 完全一致 |

## 组件清单

| 组件 | 目标区域 | 实现约束 |
|---|---|---|
| `EncryptedTrafficPage` | 业务内容区 | 保持五 Tab 的共享几何轨道 |
| `EncryptedTrafficTabs` | 标题下方 | 五项等高，切换只替换业务内容 |
| `BusinessControlRail` | 标题栏右侧 | 时间、刷新、分析固定右对齐 |
| `MetricTile` | KPI 行 | 数值、单位、同比和微趋势稳定对齐 |
| `ProtocolDistribution` | 协议趋势 | ECharts 环图和折线动态更新 |
| `Ja3Table` | Top JA3 | 固定表头、风险标签与截断指纹 |
| `Ja3Scatter` | JA3 散点 | ECharts tooltip、legend、resize 可用 |
| `EgressMap` | 外联地图 | ECharts geo/lines/effectScatter 动态渲染 |
| `TunnelFeatureCards` | 隧道特征 | 六项指标按 3 x 2 排列 |
| `TunnelTable` | 异常隧道 | 风险颜色和分页语义一致 |
| `EvidenceTable` | 证据握手 | 表格滚动、查看、下载与分页可用 |
| `ActionRail` | 右侧闭环栏 | 动作真实可点击并有反馈/权限状态 |

## 图标清单

| 图标 | 位置 | 语义 |
|---|---|---|
| `ReloadOutlined` | 标题控制区 | 刷新当前时间窗数据 |
| `SafetyCertificateOutlined` | 异常证书 KPI | 证书风险 |
| `FingerprintOutlined` | 可疑 JA3 KPI | 指纹识别 |
| `QuestionCircleOutlined` | 未知 SNI KPI | 未知分类 |
| `GlobalOutlined` | 外联画像 | 境外目的地 |
| `EyeOutlined` | 证据表操作 | 查看详情 |
| `DownloadOutlined` | 证据表操作 | 下载证据 |
| `BellOutlined` | 创建告警 | 生成告警动作 |
| `FileSearchOutlined` | PCAP 检索 | 进入取证检索 |
| `AuditOutlined` | 写入审计日志 | 留痕动作 |

## Token 与样式

| Token | 目标值/范围 | 用途 |
|---|---|---|
| `canvas-bg` | `#00111d` 附近 | 页面底色 |
| `panel-bg` | `#021826` 附近 | 面板填充 |
| `panel-border` | `#0a3b55` | 1px 面板描边 |
| `primary-cyan` | `#00a9ff` | 激活、链接、主折线 |
| `success-green` | `#75d44a` | 健康、低危、上升正向 |
| `warning-amber` | `#f5a623` | 中危、待确认 |
| `danger-red` | `#ff3b30` | 高危、异常 |
| `unknown-purple` | `#9b63d5` | QUIC/未知分类辅助色 |
| `text-primary` | `#e8f3ff` | 标题与关键数值 |
| `text-secondary` | `#91a6b8` | 表头、单位与说明 |
| `panel-radius` | `2px` | 紧凑业务面板 |
| `control-height` | `30-32px` | Tab、Select、Button |
| `table-row-height` | `27-30px` | 高密度证据表 |
| `gap-unit` | `8px` | 业务区基础间距 |

## 状态与交互

1. 点击五个 Tab 时，标题栏、控制组、KPI 顶边与业务内容起点保持不动。
2. 切换时间范围后，KPI、协议趋势、JA3、地图和表格使用同一查询窗口刷新。
3. 点击刷新显示 loading，成功后保留筛选；失败时呈现可重试错误态。
4. 一键分析必须发起真实分析请求或明确模拟状态，不允许无反馈空按钮。
5. 协议折线与散点图支持 tooltip，窗口尺寸变化后调用 ECharts resize。
6. 地图目的地与弧线从 API 数据生成，空数据时显示空态，不保留伪造线路。
7. 证据表支持内部纵向滚动和分页，页面本身不横向溢出。
8. 查看证据打开右侧 Drawer；下载动作必须返回文件或可解释错误。
9. 右侧关联按钮跳转到带当前会话/对象上下文的业务路由。
10. 创建告警、创建战役、生成规则与隔离主机均需权限检查和审计反馈。

## 差异清单

| 类型 | 当前结论 | 后续要求 |
|---|---|---|
| 公共壳层 | 不属于本业务区重排范围 | 保持与全局 AppShell 一致 |
| 业务像素 | r180 business ROI 已通过 | 后续按全局 `<0.125` 继续复验 |
| strict pixel | 零容差仍未通过 | 保留证据，不虚报精确复刻 |
| 图表 | 必须为动态 ECharts | 禁止静态图片替代 |
| 地图 | 必须为 API 驱动 ECharts 地图 | 空数据使用明确空态 |
| 表格 | 内容较多 | 保留滚动、分页和真实操作 |
| 按钮 | 目标图动作密集 | 全部需可点击、有状态和审计语义 |

## 结论

- 本记录已覆盖目标图的业务几何、关键文本、组件、图标、Token 和交互。
- 页面开发边界只计算加密流量业务区域，不因本页修改全局顶部、左侧和底部。
- 五 Tab 使用统一布局基线，顶部业务按钮固定在最右侧。
- 动态图表、地图、分页、滚动和按钮行为属于系统级业务页验收约束。
- 当前记录可进入实现/对比回路；像素接受仍以对应 metrics 与 verification 为准。
