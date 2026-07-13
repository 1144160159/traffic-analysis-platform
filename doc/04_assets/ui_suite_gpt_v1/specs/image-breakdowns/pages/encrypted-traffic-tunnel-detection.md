# encrypted-traffic-tunnel-detection 拆解记录

## 基本信息

- page-id：`encrypted-traffic-tunnel-detection`
- 分类：`pages`
- 队列顺序：33
- 分组：威胁分析 / 加密流量
- 类型：`menu-state`
- 生产路由：`/encrypted-traffic?tab=tunnel-detection`
- 宿主路由：`/encrypted-traffic`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-tunnel-detection.png`
- 证据目录：`evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/`
- 当前实现镜像：`traffic/web-ui:ui-encrypted-traffic-tunnel-detection-20260710-r182`
- 当前阶段：`business-pixel-accepted`；strict pixel 为 `fail-documented`

本图是“加密流量”页面的“隧道检测”子状态。生产实现必须通过 `tab=tunnel-detection` 激活隧道检测内容，不能加载目标 PNG、`implementation.html` 或 breakdown evidence 作为页面内容。

## 目标图观察

- 顶部页签：总览、指纹分析、隧道检测、外联画像、证据中心，其中“隧道检测”为激活态。
- KPI：隧道告警、DoH 会话、异常长连接、高熵流量、低熵心跳、疑似 VPN、已创建告警。
- 主工作区：隧道告警与异常特征、隧道异常列表、熵值与会话时长散点、心跳通信时间序列。
- 下方工作区：DoH 与隧道特征、检测规则命中、会话证据预览。
- 右侧闭环栏：隧道异常分布、快速定位、修复建议、证据与报告。

## 动态数据契约

主数据入口：

```text
fetchPageSnapshot(route.id) -> adaptEncryptedTraffic -> visuals.encryptedTraffic
```

路由状态：

```text
resolveEncryptedTrafficTab(searchParams.get('tab')) -> tunnel-detection
```

API 计划：

- `GET /v1/encrypted-traffic/stats`
- `GET /v1/encrypted-traffic/sessions`
- `GET /v1/encrypted-traffic/ja3`
- `GET /v1/encrypted-traffic/tunnels`
- `GET /v1/encrypted-traffic/exfiltration`

动态绑定：

- `visuals.encryptedTraffic.tunnelCards` -> `TunnelFeatureCards`
- `visuals.encryptedTraffic.tunnelRows` -> `TunnelTable`
- `visuals.encryptedTraffic.scatterPoints` -> `Ja3Scatter`（熵值与会话时长）
- `visuals.encryptedTraffic.heartbeatBars` -> `HeartbeatSeries`
- `visuals.encryptedTraffic.tunnelRuleRows` -> `EncryptedDenseRows`（检测规则命中）
- `visuals.encryptedTraffic.evidenceRows` -> `EncryptedDenseRows`（会话证据预览）

稀疏 API payload 会由 adapter 使用 typed fallback 补足业务视觉密度：心跳序列扩展为 48 点，检测规则补足为 6 行，并保留 DoH、低熵心跳、VPN/Proxy 等隧道语义。

## 回修记录

首次 normal route runtime 发现 `heartbeat_bars`、`dense_rows_total` 和关键业务文案未达标。已回修：

- `web/ui/src/services/pageSnapshotAdapters.ts`：心跳序列补足 48 点，隧道规则补足 6 行并规范 DoH/低熵/VPN 语义。
- `web/ui/src/pages/EncryptedTrafficPage.tsx`：稀疏 bars 触发 48 点 fallback，规则 fallback 增加 `可疑 VPN / Proxy`。
- `web/ui/src/routes/encryptedTrafficVisuals.test.ts` 与 `web/ui/src/services/pageSnapshotAdapters.test.ts`：新增隧道检测子状态和数据密度守卫。

回修后重新 build、重建 r182、双节点导入、rollout restart、Windows Chrome 真实路由截图和 runtime 验收均已通过。

## 验收证据

| 证据 | 路径 |
|---|---|
| target | `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/target.png` |
| final implementation | `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/implementation-r182-final.png` |
| business diff | `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/diff-business-r182-final.png` |
| strict diff | `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/diff-strict-r182-final.png` |
| business metrics | `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/metrics-business-r182-final.json` |
| strict metrics | `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/metrics-strict-r182-final.json` |
| normal route screenshot | `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/normal-route-r182.png` |
| normal route runtime | `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/normal-route-runtime-r182.json` |
| verification | `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/verification.json` |
| acceptance report | `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-encrypted-traffic-tunnel-detection-r182.json` |

## 验收结论

- Focused tests：`encryptedTrafficVisuals`、`noBitmapUi` 和 encrypted-traffic adapter 用例 passed。
- Production build：`npm --prefix web/ui run build` passed，仅保留 Vite 大 chunk warning。
- Deployment：`traffic/web-ui:ui-encrypted-traffic-tunnel-detection-20260710-r182`，`web-ui` Deployment `1/1`，APISIX `200`。
- Business visual diff：`0.11427420910493827 <= 0.35`，channel tolerance `64`。
- Strict visual diff：`0.9999836033950618 > 0.015`，channel tolerance `0`，记录为非阻断 strict pixel failure。
- Normal route：`/encrypted-traffic?tab=tunnel-detection` pass，激活页签为“隧道检测”，5 个页签、7 个 KPI、6 个隧道卡、6 条隧道行、34 个散点、48 个心跳柱、9 条 dense rows 均可见。
- Runtime：无 4xx/5xx、requestfailed、console/pageerror、horizontal overflow，`smoke_hash_consumed=true`。
- Auxiliary review：PASS（agent `019f4b28-6185-7642-a8cc-b10622b1708b`，read-only）。
- Main-thread judgment：`business-pixel-accepted`。

## 区域与坐标

本目标图为 1920 x 1080；与另两张图不同，它没有底部全局状态栏，内容区一直延伸到画布底部。

| 区域 | bbox | 内容 |
|---|---:|---|
| 画布 | `0,0,1920,1080` | 深色流量分析平台单屏 |
| 左侧导航 | `0,0,213,1054` | 品牌、一级菜单、威胁分析二级菜单和收起按钮 |
| 顶部工具栏 | `213,0,1707,58` | 汉堡菜单、区域、搜索、通知、消息、帮助、用户 |
| 页面标题控制 | `225,58,1672,52` | 加密流量标题、日期范围、近 24 小时和刷新 |
| 五 Tab | `225,110,1672,45` | 隧道检测为激活态 |
| KPI 带 | `225,164,1446,74` | 七项隧道异常指标 |
| 异常列表 | `225,247,629,377` | 八条隧道会话与近场动作 |
| 熵值散点 | `862,247,405,377` | 持续时间与熵值气泡图 |
| 心跳时序 | `1275,247,395,377` | P95、抖动、包数及双轴时序 |
| DoH/隧道特征 | `225,633,414,379` | 六张特征卡与微趋势 |
| 检测规则命中 | `647,633,549,379` | 五条规则、置信度和处置 |
| 会话证据预览 | `1204,633,466,379` | 会话字段与 Payload 熵值时序 |
| 右侧异常摘要 | `1682,164,216,166` | 风险等级环图与数量 |
| 右侧快捷定位 | `1682,340,216,181` | 告警、会话证据和观察名单 |
| 右侧修复建议 | `1682,530,216,215` | 阻断、限制、TLS 策略与审计 |
| 右侧证据报告 | `1682,756,216,170` | 报告导出和证据中心 |

### 页面几何约束

- 左侧导航宽度固定 `213px`，不可套用另一个 AppShell 的 `166px` 宽度。
- 顶栏高度约 `58px`，搜索框和全局动作不属于业务区回修范围。
- 五个 Tab 位于标题行下方，激活下划线不改变轨道高度。
- KPI 带仅占主区宽度，右侧异常摘要从 `x=1682` 独立起栏。
- 三个中部面板共用 `y=247` 顶边和 `y=624` 底边。
- 三个下部面板共用 `y=633` 顶边，页面无底栏遮挡。

## 文本清单

| 文本 | 区域 | 类型 | 要求 |
|---|---|---|---|
| 流量分析平台 | 左上品牌 | brand | 完全一致 |
| 加密流量 | 页面标题 | title | 完全一致 |
| 总览 | Tab | tab | 完全一致 |
| 指纹分析 | Tab | tab | 完全一致 |
| 隧道检测 | Tab | active-tab | 完全一致 |
| 外联画像 | Tab | tab | 完全一致 |
| 证据中心 | Tab | tab | 完全一致 |
| 近24小时 | 标题控制 | select | 完全一致 |
| 隧道告警 | KPI | metric-label | 完全一致 |
| DoH 会话 | KPI | metric-label | 完全一致 |
| 异常长连接 | KPI | metric-label | 完全一致 |
| 高熵流量 | KPI | metric-label | 完全一致 |
| 低频心跳 | KPI | metric-label | 完全一致 |
| 疑似 VPN | KPI | metric-label | 完全一致 |
| 已创建告警 | KPI | metric-label | 完全一致 |
| 隧道异常列表 | 左主面板 | panel-title | 完全一致 |
| DoH over TLS | 列表 | type | 数据动态 |
| DoH over QUIC | 列表 | type | 数据动态 |
| TLS over 443 | 列表 | type | 数据动态 |
| 异常长连接 | 列表 | type | 数据动态 |
| 低频心跳 | 列表 | type | 数据动态 |
| 可疑 VPN | 列表 | type | 数据动态 |
| 创建告警 | 列表/右栏 | action | 完全一致 |
| 查看证据 | 列表 | action | 完全一致 |
| 加入观察 | 列表 | action | 完全一致 |
| 熵值与会话时长散点图 | 中主面板 | panel-title | 完全一致 |
| 气泡大小：流量 | 散点图 | hint | 完全一致 |
| DoH 聚类 | 散点图 | annotation | 完全一致 |
| 异常长连接聚类 | 散点图 | annotation | 完全一致 |
| 心跳通信时间序列 | 右主面板 | panel-title | 完全一致 |
| P95 间隔 | 心跳面板 | metric-label | 完全一致 |
| 抖动 (P95) | 心跳面板 | metric-label | 完全一致 |
| 包数 | 心跳面板 | metric-label | 完全一致 |
| DoH 与隧道特征 | 下方左面板 | panel-title | 完全一致 |
| DNS over HTTPS | 特征卡 | feature | 完全一致 |
| Unknown SNI | 特征卡 | feature | 完全一致 |
| ALPN h2 / h3 | 特征卡 | feature | 完全一致 |
| Constant Packet Size | 特征卡 | feature | 完全一致 |
| Low Entropy Beacon | 特征卡 | feature | 完全一致 |
| High Entropy Payload | 特征卡 | feature | 完全一致 |
| 检测规则命中 | 下方中面板 | panel-title | 完全一致 |
| 调整规则 | 规则表 | action | 完全一致 |
| 会话证据预览 | 下方右面板 | panel-title | 完全一致 |
| PCAP 索引 | 证据预览 | field | 完全一致 |
| 证书 Hash (SHA256) | 证据预览 | field | 完全一致 |
| Payload 熵值 | 证据预览 | field | 完全一致 |
| 隧道异常 | 右栏 | panel-title | 完全一致 |
| 快速定位 | 右栏 | panel-title | 完全一致 |
| 创建隧道告警 | 右栏 | danger-action | 完全一致 |
| 查看会话证据 | 右栏 | action | 完全一致 |
| 加入观察名单 | 右栏 | action | 完全一致 |
| 修复建议 | 右栏 | panel-title | 完全一致 |
| 阻断可疑域名/IP | 右栏 | action | 完全一致 |
| 限制异常端口外联 | 右栏 | action | 完全一致 |
| 启用 TLS 检测策略 | 右栏 | action | 完全一致 |
| 加强 DNS 解析审计 | 右栏 | action | 完全一致 |
| 证据与报告 | 右栏 | panel-title | 完全一致 |
| 导出隧道检测报告 | 右栏 | action | 完全一致 |
| 跳转证据中心 | 右栏 | action | 完全一致 |

## 组件清单

| 组件 | 区域 | 实现约束 |
|---|---|---|
| `EncryptedTrafficTabs` | 五 Tab | 固定位置、固定尺寸、无切换位移 |
| `TunnelMetricStrip` | KPI 带 | 七项数据从 API 映射 |
| `TunnelSessionTable` | 异常列表 | 筛选、分页、滚动和三项近场动作 |
| `EntropyDurationScatter` | 熵值散点 | ECharts scatter + markLine/graphic 注释 |
| `HeartbeatTimeseries` | 心跳时序 | ECharts 双轴 line/bar 动态刷新 |
| `TunnelFeatureGrid` | 特征卡 | 六卡 2 x 3 排列及 sparkline |
| `RuleHitTable` | 规则命中 | 置信度、命中数、调整规则、创建告警 |
| `SessionEvidencePreview` | 会话证据 | 选中行联动字段和熵值曲线 |
| `RiskDonut` | 右栏摘要 | ECharts 环图与四级风险语义 |
| `RightActionRail` | 右栏 | 快速定位、修复、导出真实可点击 |

## 图标清单

| 图标 | 位置 | 语义 |
|---|---|---|
| `MenuOutlined` | 顶栏 | 展开/收起导航 |
| `SearchOutlined` | 顶栏搜索 | 全局检索 |
| `BellOutlined` | 顶栏/告警 | 通知及创建告警 |
| `ReloadOutlined` | 标题控制 | 刷新业务数据 |
| `WarningOutlined` | 隧道告警 | 高风险异常 |
| `GlobalOutlined` | DoH 会话 | DNS over HTTPS |
| `LinkOutlined` | 异常长连接 | 连接持续异常 |
| `SecurityScanOutlined` | 已创建告警 | 检测闭环 |
| `FilterOutlined` | 异常列表 | 筛选 |
| `DownloadOutlined` | 列表/报告 | 导出证据 |
| `FullscreenOutlined` | 异常列表 | 扩展查看 |

## Token 与样式

| Token | 值/范围 | 用途 |
|---|---|---|
| `canvas-bg` | `#071726` | 页面背景 |
| `sidebar-bg` | `#081929` | 左侧导航 |
| `panel-bg` | `#0a1d2d` | 面板背景 |
| `panel-border` | `#284357` | 面板描边 |
| `accent-blue` | `#2f8cff` | 激活、主按钮、图表信息 |
| `danger-red` | `#ee4b45` | 高风险与告警 |
| `warning-orange` | `#f5a623` | 中风险与异常长连接 |
| `success-green` | `#58c978` | 低风险与心跳基线 |
| `feature-purple` | `#a86bc7` | 高熵类指标 |
| `text-primary` | `#e4edf5` | 标题与关键值 |
| `text-secondary` | `#9bacba` | 辅助字段 |
| `control-height` | `32px` | Tab、Select、Button |
| `row-height` | `38px` 左右 | 异常列表 |
| `panel-gap` | `8px` | 主面板间距 |

## 状态与交互

1. 五 Tab 切换保持标题、控制、Tab 轨道和业务区起点固定。
2. 日期范围和近 24 小时选择共同进入查询参数，刷新后保留选择。
3. KPI、散点、心跳、特征、规则和证据来自同一数据快照或显式时间戳。
4. 异常列表分页真实改变 offset/limit，并保留选中会话上下文。
5. 点击列表行联动心跳对象和会话证据预览。
6. 散点图 tooltip 展示持续时间、熵值、流量、风险和会话标识。
7. 心跳图支持动态数据、tooltip、legend 和容器 resize。
8. 创建告警、查看证据、加入观察均必须有 API 结果和用户反馈。
9. 规则调整与告警创建需权限检查，写入审计记录。
10. 右栏超高时内部滚动；页面不可出现不可达动作或水平溢出。

## 实现映射

| 目标模块 | 实现/数据 |
|---|---|
| KPI 带 | `visuals.encryptedTraffic.tunnelCards` |
| 异常列表 | `tunnelRows` + 服务端分页 |
| 熵值散点 | tunnel session 的 duration/entropy/bytes |
| 心跳时序 | 选中会话的 interval/jitter/packet series |
| 特征卡 | DoH、SNI、ALPN、包长和熵特征聚合 |
| 规则命中 | `tunnelRuleRows` |
| 会话证据 | session、certificate、PCAP 与 payload entropy |
| 右栏动作 | Alert/Watchlist/Evidence/Report API |

## 差异清单

| 类型 | 现状 | 要求 |
|---|---|---|
| AppShell | 此目标图壳层与另两页不同 | 业务复刻不擅自修改全局公共实现 |
| 底部状态栏 | 目标图不存在 | 不在业务区人为补出底栏 |
| 业务 ROI | 既有业务门已接受 | 后续统一以 `<0.125` 复验 |
| strict pixel | 已记录未通过 | 保留差异，不虚报 |
| ECharts | 散点、时序、环图均需动态 | 检查 option 和 canvas 像素变化 |
| 表格 | 两张表都含分页 | 校验 API 参数、页码和行数 |
| 动作 | 近场与右栏动作密集 | 每个按钮均需真实反馈 |

## 结论

- 隧道检测页已按目标图独立记录其 213px 导航、无底栏布局和三列主工作区。
- 动态散点、心跳时序、风险环图、表格分页及会话联动均属于必验功能。
- 开发只处理业务区域，不以本页目标图为理由修改全局顶部、左侧或底部公共组件。
- 最终接受仍需 Windows Chrome 实际路由截图、ROI/diff、交互证据和智能体复核。
