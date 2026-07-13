# attack-chains 拆解记录

## 基本信息

- page-id：`attack-chains`
- 分类：`pages`
- 队列顺序：30
- 分组：威胁分析
- 类型：`menu-route`
- 路由：`/attack-chains`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/attack-chains.png`
- 证据目录：`evidence/ui-image-breakdowns/pages/attack-chains/`
- 当前实现镜像：`traffic/web-ui:ui-attack-chains-20260710-r179`
- 当前阶段：`business-pixel-accepted`（业务容差）；strict pixel 为 fail-documented
- 目标图用途：拆解、测量、视觉 diff 和验收
- 禁止用途：不得作为业务页面截图资源加载

本图是完整 AppShell 页面，业务区为“攻击链分析”。验收要求顶部状态栏、左侧威胁分析菜单、底部状态栏和业务主工作区同时可见。

## 目标图观察

页面顶部业务标题为“攻击链分析”，包含选择战役、时间范围、资产范围、视图模式和导出/下钻/响应动作。

主工作区由三块组成：

- 左侧主画布：攻击阶段泳道，包含攻击阶段、实体/资产、告警事件、证据锚点、处置动作。
- 下方明细：ATT&CK 阶段矩阵和路径明细（关键跳转）。
- 右侧闭环栏：证据锚点列表和处置建议。

目标业务数据：

1. 6 个攻击阶段：侦察、初始访问、执行、横向移动、C2 通信、数据外传。
2. 6 个证据锚点：PCAP、日志、Session 等。
3. 6 条处置建议：封禁域名、阻断 IP、隔离主机、加强访问控制、收紧防火墙策略、限制管理网段。
4. 路径明细展示关键跳转的阶段、实体、告警、证据、处置建议和状态。

## 动态数据契约

主数据入口：

```text
GET /v1/attack-chains
```

前端适配路径：

```text
fetchPageSnapshot(route.id) -> adaptAttackChains
```

适配输出：

- `metrics`：阶段节点、实体节点、证据锚点、阻断点、置信度。
- `rows`：路径明细表。
- `timeline`：阶段事件流。
- `evidence`：API 与证据摘要。

画布、证据锚点、处置建议当前使用组件内稳定业务种子数据，路径明细和指标通过 `PageSnapshot` 接入真实 API 或 typed fallback。

## 实现映射

| 目标元素 | 前端实现 |
|---|---|
| 页面容器 | `AttackChainAnalysisPage` |
| 攻击链画布 | `AttackCanvas` |
| ATT&CK 阶段矩阵 | `PhaseMatrix` |
| 路径明细 | `PathDetail` + `snapshot.rows` |
| 证据锚点 | `EvidenceAnchorList` |
| 处置建议 | `ResponseRecommendations` |
| 数据适配 | `adaptAttackChains` |
| 详情抽屉契约 | `OverlayContractHost` |

## 验收证据

| 证据 | 路径 |
|---|---|
| target | `evidence/ui-image-breakdowns/pages/attack-chains/target.png` |
| final implementation | `evidence/ui-image-breakdowns/pages/attack-chains/implementation-r179-final.png` |
| business diff | `evidence/ui-image-breakdowns/pages/attack-chains/diff-business-r179-final.png` |
| strict diff | `evidence/ui-image-breakdowns/pages/attack-chains/diff-strict-r179-final.png` |
| business metrics | `evidence/ui-image-breakdowns/pages/attack-chains/metrics-business-r179-final.json` |
| strict metrics | `evidence/ui-image-breakdowns/pages/attack-chains/metrics-strict-r179-final.json` |
| normal route screenshot | `evidence/ui-image-breakdowns/pages/attack-chains/normal-route-r179.png` |
| normal route runtime | `evidence/ui-image-breakdowns/pages/attack-chains/normal-route-runtime-r179.json` |
| verification | `evidence/ui-image-breakdowns/pages/attack-chains/verification.json` |
| acceptance report | `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-attack-chains-r179.json` |

## 验收结论

- Focused tests：`attackChainCanvas`、`noBitmapUi` 和 attack-chain adapter 用例 passed；完整 `pageSnapshotAdapters.test.ts` 仍有两个非本页既有失败，已记录为非本页阻断。
- Production build：`npm --prefix web/ui run build` passed，仅保留 Vite 大 chunk warning。
- Deployment：`traffic/web-ui:ui-attack-chains-20260710-r179`，`web-ui` Deployment `1/1`，APISIX `/` 返回 `200`。
- Focus runtime：Windows Chrome CDP pass，无 4xx/5xx、requestfailed、console/pageerror、forbidden target resource。
- Normal route：`/attack-chains` pass，AppShell、6 个阶段、6 个证据锚点、6 条处置建议和工具栏动作均可见。
- Business visual diff：`0.09817563657407408` <= `0.35`，channel tolerance `64`。
- Strict visual diff：`0.9999156057098766` > `0.015`，channel tolerance `0`，记录为非阻断 strict pixel failure。
- Auxiliary review：PASS（agent `019f4aed-f7da-7cb2-a19d-adeba76c9b0f`，read-only）。
- Main-thread judgment：`business-pixel-accepted`。

## 区域与坐标

| 区域 | bbox | 目标图事实 |
|---|---:|---|
| canvas | `0,0,1920,1080` | 完整 AppShell 与业务区 |
| topbar | `0,0,1920,80` | 站点、时间、风险、告警、采集和数据质量 |
| sidebar | `0,80,178,917` | 威胁分析菜单，攻击链分析高亮 |
| content-root | `184,80,1192,917` | 筛选、主路径和下方明细 |
| filter-toolbar | `198,128,1158,66` | 链路、时间、资产、视图和缩放 |
| attack-stage-row | `198,200,1158,82` | 六个 ATT&CK 阶段 |
| attack-path | `296,282,1060,442` | 实体、告警、证据和处置四层路径 |
| matrix-panel | `198,730,526,234` | 阶段矩阵 |
| path-detail | `734,730,642,234` | 关键跳转明细 |
| right-rail | `1384,126,512,858` | 证据锚点与处置建议 |
| evidence-table | `1398,252,484,286` | 证据锚点分页表 |
| response-table | `1398,632,484,320` | 阻断点建议表 |

## 文本清单

| 类别 | 可见文本 |
|---|---|
| 主标题 | `攻击链分析` |
| 链路 | `疑似 C2 隧道通信` |
| 时间 | `2026-06-19 00:00:00 ~ 2026-06-20 03:45:00` |
| 视图 | `攻击链视图` |
| 阶段 | `侦察 / 初始访问 / 执行 / 横向移动 / C2 通信 / 数据外传` |
| ATT&CK | `TA0043 / TA0001 / TA0002 / TA0008 / TA0011 / TA0010` |
| 实体 | `203.0.113.45 / FW-01 / 10.12.5.23 / 10.12.1.10 / 10.12.8.45 / c2.example.com` |
| 告警 | `端口扫描探测 / Web 漏洞利用 / 恶意命令执行 / 凭证窃取 / C2 隧道通信 / 数据外传尝试` |
| 证据 | `DNS 解析记录 / HTTP 请求包 / 进程创建日志 / LSASS 访问 / TLS 流量会话 / 外传流量样本` |
| 处置 | `封禁源 IP / WAF 规则加固 / 终止恶意进程 / 重置域控凭证 / 阻断 C2 域名 / 阻断外传通道` |
| 右栏 | `证据锚点 / 处置建议` |
| 底部 | `ATT&CK 阶段矩阵 / 路径明细（关键跳转）` |

## 组件清单

| 组件 | 职责 |
|---|---|
| `AppShell` | 固定顶部、左侧和底部公共区 |
| `AttackChainFilters` | 链路、时间、资产和视图筛选 |
| `AttackStageStrip` | 六阶段状态条 |
| `AttackChainGraph` | API 驱动 SVG 动态路径图 |
| `EvidenceAnchorTable` | 证据锚点分页表 |
| `ResponseSuggestionTable` | 阻断点建议分页表 |
| `AttackMatrix` | 阶段发生状态矩阵 |
| `PathDetailTable` | 关键跳转明细 |

## 图标清单

| 图标 | 语义 |
|---|---|
| 地球 | 外部 IP 实体 |
| 盾牌 | 边界防火墙 |
| 服务器 | Web 与内网主机 |
| 网络节点 | 域控服务器 |
| 靶标 | 外部域名 |
| 警告三角 | 告警事件 |
| 文档 | 证据锚点 |
| 勾选方框 | 处置动作 |
| 眼睛与下载 | 查看、下载证据 |
| 全屏、缩放、时间 | 图谱控制 |

## Token 与样式

| Token | 值 | 用途 |
|---|---|---|
| `page-bg` | `#03111c` | 页面底色 |
| `panel-bg` | `rgba(6,28,43,0.86)` | 业务面板 |
| `border-weak` | `rgba(56,151,201,0.22)` | 分隔线 |
| `active-blue` | `#1e9cff` | 选中菜单和操作 |
| `success` | `#36d66b` | 已发生、健康、完成 |
| `warning` | `#ffb020` | 横向移动与 C2 阶段 |
| `danger` | `#ff4d4f` | 告警、高风险、外传 |
| `text-primary` | `#eaf7ff` | 主文字 |
| `text-secondary` | `#9db9c9` | 辅助文字 |
| `panel-radius` | `6px` | 业务面板圆角 |

## 状态与交互

| 触发 | 预期结果 |
|---|---|
| 切换链路 | 主路径、矩阵、证据和处置建议同步刷新 |
| 变更时间 | 重新查询 API，并保留 loading/error/empty 状态 |
| 缩放或自动布局 | 只改变图谱视图，不改变外围尺寸 |
| 选择路径节点 | 右栏证据按阶段和实体过滤 |
| 查看或下载证据 | 执行真实动作并记录审计 |
| 点击触发响应 | 显示权限、影响范围和确认状态 |
| 表格翻页 | 独立分页且页面高度不变化 |

## 差异清单

- 目标图为完整 AppShell；业务区 ROI 必须排除公共顶部、左侧和底部。
- 主图是 API 驱动 SVG 动态拓扑，不得替换为 ECharts 或静态图片。
- 右栏两张表必须独立滚动或分页，避免底部内容不可见。
- 状态色严格使用健康绿、信息蓝、警告黄、高危红。
- 文本锐度与目标图抗锯齿差异不视为业务结构缺失。
- 节点、告警、证据或处置字段缺失属于业务差异，不能用固定文字填充。

## 结论

- 拆解覆盖公共壳、六阶段攻击链、四层路径、阶段矩阵、关键跳转、证据和处置建议。
- 页面核心是动态 SVG 关系图与 API 数据联动，按钮、分页和筛选必须可交互。
- JSON 已达到深拆数量门槛，本次 Markdown 补齐逐图事实与验收边界。

### 关键跳转复核

| 序号 | 源 | 目标 | 协议/端口 | 说明 |
|---:|---|---|---|---|
| 1 | `203.0.113.45` | `10.12.5.23` | `TCP/443` | 漏洞利用请求 |
| 2 | `10.12.5.23` | `10.12.1.10` | `SMB/445` | 凭证窃取 |
| 3 | `10.12.1.10` | `10.12.8.45` | `RDP/3389` | 横向移动 |
| 4 | `10.12.8.45` | `c2.example.com` | `TLS/443` | C2 隧道通信 |
| 5 | `10.12.8.45` | `198.51.100.27` | `HTTPS/443` | 数据外传尝试 |

### 证据与处置联动

- 六个阶段各自具有证据锚点，不允许用同一证据重复填充全部阶段。
- 右栏完整度来自证据 API，分页切换后保持阶段编号和类型一致。
- 阻断建议按高、中、低优先级排序，颜色与优先级语义一致。
- 每条建议包含阻断点、建议动作、影响评估和操作入口。
- 触发响应前必须重新校验权限，并在确认框显示受影响对象。
- 处置提交后刷新路径节点状态和审计记录，不静默假成功。
