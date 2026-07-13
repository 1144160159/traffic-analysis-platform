# alert-detail-evidence-graph-path.png 逐图精拆记录

## 基本信息

- page-id: `alert-detail-evidence-graph-path`
- route: `/alerts`
- type: `menu-state`
- parent: `alerts`
- target UI: `doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail-evidence-graph-path.png`
- 证据目录: `evidence/ui-image-breakdowns/pages/alert-detail-evidence-graph-path/`
- 当前状态: `business-pixel-accepted`
- 生产 URL: `http://10.0.5.8:30180/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&__codex_page_id=alert-detail-evidence-graph-path&evidenceView=graph-path`
- 生产镜像: `traffic/web-ui:ui-alert-detail-evidence-graph-path-visual-20260709-r142`
- 视口: `1920 x 1080`

## 目标图观察

- 目标图是告警详情证据链的 `图谱路径` 分类面板，不显示公共 AppShell。
- 顶部 tab 为 `全部 6`、`PCAP 1`、`Session 2`、`日志 1`、`图谱路径 1`、`文件 1`，其中 `图谱路径 1` 高亮。
- 主体表格展示 1 条图谱路径证据：`path-20260620-000123.json`，摘要为 `172.16.5.10 -> 185.22.14.9 / 路径关系`，边权重 `0.86 / 横向访问`。
- 关联实体为 `资产 DB-SRV-01`、`账号 svc_backup`、`域名 downloads.campus.local`，生成时间 `06-20 03:43:10`，状态 `已生成`。
- 中部业务动态图示为路径关系图：可疑外部 IP -> 边界网关 -> 核心业务服务器 -> 账号，边标签为 `通信`、`登录`、`访问`。
- 右侧路径统计：节点数 4、边数 3、平均边权重 0.86、风险评分 85（高风险）。
- 下方关联资源：`PCAP 1`、`Session 2`、`日志 1`；底部入口 `查看全部 图谱路径 1 项 >`。

## 区域与坐标

坐标以 1920 x 1080 目标图左上角为原点。

| 区域 | bbox | 说明 |
|---|---:|---|
| canvas | `0,0,1920,1080` | 单屏业务面板画布 |
| outer-panel | `7,23,1905,1030` | 外层蓝色描边容器 |
| tab-title | `39,58,190,76` | `证据链（6）` 标题 |
| evidence-tabs | `260,56,1012,80` | 6 个证据分类 tab |
| table-frame | `22,134,1875,895` | 表格、路径图、资源区、底部入口容器 |
| table-head | `36,171,1843,101` | 表头行 |
| graph-row | `36,272,1843,171` | 单条图谱路径证据记录 |
| graph-detail | `36,443,1843,347` | 路径关系图与统计 |
| graph-map | `51,476,1331,292` | 4 节点 3 边动态路径图 |
| graph-stats | `1406,476,447,292` | 路径统计卡 |
| related-resources | `36,790,1843,100` | 关联资源入口 |
| footer-link | `59,925,310,48` | 查看全部入口 |

## 文本清单

| 文本 | 必须一致 | 备注 |
|---|---|---|
| 证据链（6） | 是 | 顶部标题 |
| 全部 6 | 是 | tab |
| PCAP 1 | 是 | tab |
| Session 2 | 是 | tab |
| 日志 1 | 是 | tab |
| 图谱路径 1 | 是 | active tab |
| 文件 1 | 是 | tab |
| 证据类型 | 是 | 表头 |
| 路径文件 | 是 | 表头 |
| 路径摘要 | 是 | 表头 |
| 边权重 | 是 | 表头 |
| 关联实体 | 是 | 表头 |
| 生成时间 | 是 | 表头 |
| 状态 | 是 | 表头 |
| 操作 | 是 | 表头 |
| 图谱路径 | 是 | 证据类型 |
| path-20260620-000123.json | 是 | 路径文件 |
| 172.16.5.10 -> 185.22.14.9 | 是 | 路径摘要 |
| 路径关系 | 是 | 路径摘要 |
| 0.86 / 横向访问 | 是 | 边权重 |
| 资产 DB-SRV-01 | 是 | 关联实体 |
| 账号 svc_backup | 是 | 关联实体 |
| 域名 downloads.campus.local | 是 | 关联实体 |
| 06-20 03:43:10 | 是 | 生成时间 |
| 已生成 | 是 | 状态 |
| 路径关系图 | 是 | 图示标题 |
| 通信 | 是 | 边标签 |
| 登录 | 是 | 边标签 |
| 访问 | 是 | 边标签 |
| 可疑外部IP | 是 | 节点 |
| 185.22.14.9 | 是 | 节点 |
| 边界网关 | 是 | 节点 |
| 10.20.0.1 | 是 | 节点 |
| 核心业务服务器 | 是 | 节点 |
| 10.20.4.18 | 是 | 节点 |
| 账号 | 是 | 节点 |
| svc_backup | 是 | 节点 |
| 路径统计 | 是 | 统计标题 |
| 节点数：4 | 是 | 统计 |
| 边数：3 | 是 | 统计 |
| 平均边权重：0.86 | 是 | 统计 |
| 风险评分：85（高风险） | 是 | 统计 |
| 关联资源 | 是 | 资源区 |
| PCAP 1 | 是 | 资源 |
| Session 2 | 是 | 资源 |
| 日志 1 | 是 | 资源 |
| 查看全部 图谱路径 1 项 | 是 | 底部入口 |

## 组件清单

| 目标区域 | 实现组件 | 数据来源 | 自适应策略 |
|---|---|---|---|
| 面板容器 | `AlertEvidenceGraphPathFocusView` | `AlertDetailSnapshot` | full viewport grid，只在本 target state 隐藏 AppShell |
| 证据分类 tab | React button grid | `snapshot.evidenceRows` 聚合计数 | 固定列宽 + overflow guard |
| 图谱路径表格 | CSS grid table | `AlertDetailEvidenceRow.graphPath` | 列宽稳定，长文本带 `title` |
| 路径关系图 | `GraphPathEdges` + React 节点 | `graphPath.nodes` / `graphPath.edges` | SVG 线与 React 节点按容器定位 |
| 路径统计 | React/CSS | `graphPath.nodes/edges/riskScore` | 右侧固定统计卡 |
| 关联资源 | React buttons | `graphPath.resources` | 固定按钮区，文本带 title |

## 图标清单

- 业务动态图示：`路径关系图` 是业务动态图示，使用 typed fallback/API 数据字段 `nodes`、`edges`、`riskScore` 进入 React/SVG 组件；禁止截图替代。
- 独立图标：图谱路径、查看、资源链接、节点图标使用 AntD 图标和 SVG/React 元素。
- 背景/装饰：深色面板、描边、glow 由 CSS 绘制；未使用目标图、整卡、整表或业务截图作为资源。

## 数据契约

- 数据入口：`web/ui/src/services/alertDetailApi.ts` 的 `fetchAlertDetailSnapshot(alertId)`。
- typed fallback：`AlertDetailEvidenceRow.graphPath` 提供 `pathFile`、`pathSummary`、`edgeWeight`、`relationType`、`relatedEntities`、`nodes`、`edges`、`resources`、`riskScore`。
- 字段映射：
  - `pathFile` -> 路径文件。
  - `pathSummary` -> 路径摘要。
  - `edgeWeight` + `relationType` -> 边权重。
  - `relatedEntities` -> 关联实体。
  - `nodes` -> 路径关系图节点。
  - `edges` -> 路径关系图边和边标签。
  - `resources` -> 关联资源按钮。
  - `riskScore` -> 风险评分。
- 刷新节奏：正常告警详情页 React Query `refetchInterval=30_000`；视觉拆解状态为 deterministic capture，关闭自动刷新以稳定 diff。

## 验收证据

- 禁图门禁：`npm --prefix web/ui test -- --run src/routes/noBitmapUi.test.ts` 通过。
- 构建：`npm --prefix web/ui run build` 通过。
- Windows Chrome CDP 预检：`cdp-version-r142-final-pre-capture.txt`、`cdp-list-r142-final-pre-capture.txt`。
- 生产截图：`implementation-r142-final.png`，别名 `implementation.png`。
- runtime：`capture-meta-r142-final.json` status/pass，console/pageerror/requestfailed/4xx/5xx 均为 0，root/panel/table/chart overflow 均为 0。
- diff：`metrics-r142.json` pass，mismatch ratio `0.10408564814814815 <= 0.12`。
- business diff：`metrics-business-r142.json` pass，mismatch ratio `0.10408564814814815 <= 0.12`。
- 8-tab 回归：`evidence/ui-image-breakdowns/pages/data-quality-tabs-stable/tab-geometry-r142-tabs-final.json` pass；1920x1080 与 1366x768 下切换 8 个数据质量 tab 的 `maxGeometryDelta=0`。

## Token 与样式

| Token | 目标值 | 使用位置 | 约束 |
|---|---|---|---|
| `page-bg` | `#03111c` | 画布 | 暗色业务背景 |
| `panel-bg` | `rgba(6,28,43,0.86)` | 路径面板 | 不使用位图卡片 |
| `panel-strong-bg` | `#071f32` | 表头、统计框 | 建立层级 |
| `border-weak` | `rgba(56,151,201,0.22)` | 表格和节点框 | 1px 线条 |
| `active-blue` | `#1e9cff` | 激活 tab、链接 | 信息与操作语义 |
| `text-primary` | `#eaf7ff` | 标题和实体名 | 高对比 |
| `text-secondary` | `#9db9c9` | 地址、统计说明 | 次级层级 |
| `success` | `#36d66b` | 已生成、账号节点 | 成功语义 |
| `danger` | `#ff4d4f` | 外部 IP、高风险 | 风险语义 |
| `warning` | `#ffb020` | 需确认路径 | 不与成功色互换 |
| `panel-radius` | `6px` | 面板边角 | 紧凑工具界面 |
| `panel-gap` | `8px` | 关系图与统计区 | 稳定栅格 |

## 状态与交互

| 控件 | 触发 | 预期状态 | 验收重点 |
|---|---|---|---|
| 图谱路径 tab | 点击 | 当前分类激活 | 顺序和宽度固定 |
| 图谱操作图标 | 点击节点图标 | 聚焦路径关系 | 关系数据来自 API |
| 查看图标 | 点击眼睛 | 打开路径详情 | 保留告警上下文 |
| 资源标签 | 点击 PCAP/Session/日志 | 跳转关联证据 | 告警和证据 ID 同步 |
| 查看全部 | 点击底部入口 | 展开完整路径列表 | 保留筛选状态 |
| 节点悬停 | 指针进入节点 | 显示实体摘要 | 不改变节点布局 |

## 实现映射

| 目标对象 | 代码位置 | 数据字段 | 实现边界 |
|---|---|---|---|
| 路径面板 | `AlertDetailPage.tsx` / `AlertEvidenceGraphPathFocusView` | `evidenceRows[].graphPath` | 仅业务区 focus |
| 路径边 | `GraphPathEdges` | `graphPath.edges` | API 驱动 SVG 动态关系线 |
| 节点 | `AlertDetailPage.tsx` | `nodes/type/label/address` | 图标和状态色按类型映射 |
| 路径统计 | `AlertDetailPage.tsx` | `nodeCount/edgeCount/weight/risk` | 缺失值明确为空 |
| 数据适配 | `services/alertDetailApi.ts` | 路径证据字段 | typed contract |
| 视觉样式 | `styles/pages.css` | grid/flex/SVG | 禁止加载目标图 |

## 差异清单

| 项目 | 目标图事实 | 实现判定 | 结论 |
|---|---|---|---|
| 分类 | 图谱路径 1 激活 | tab 计数与状态一致 | 接受 |
| 路径行 | 一条 json 证据 | 文件、摘要、权重齐全 | 接受 |
| 关系图 | 四节点三边 | 动态 SVG 节点和边 | 接受 |
| 统计 | 4 节点、3 边、0.86、85 | 字段逐项映射 | 接受 |
| 资源 | PCAP 1、Session 2、日志 1 | 三个可操作标签 | 接受 |
| 图形锐度 | 目标图有抗锯齿柔化 | 浏览器 SVG 更锐利 | 非结构差异 |
| 未决项 | 无业务结构缺失 | 既有 diff 已通过 | 无需阻断 |

## 结论

- 本页拆解覆盖路径证据行、四节点关系图、路径统计、关联资源和底部入口。
- 路径图保持 API 驱动 SVG 动态实现，不替换为静态截图或 ECharts 拓扑。
- 风险、边权重和实体关联必须来自证据契约；空态不能展示固定伪数据。
- 已有生产截图与业务区 diff 证据可用于后续像素回归。

### 节点与路径复核

| 序号 | 路径对象 | 目标图事实 |
|---:|---|---|
| 1 | 外部 IP | `185.22.14.9`，红色地球节点 |
| 2 | 边界网关 | `10.20.0.1`，蓝色盾牌节点 |
| 3 | 核心服务器 | `10.20.4.18`，紫色数据库节点 |
| 4 | 账号 | `svc_backup`，绿色用户节点 |
| 5 | 第一条边 | 外部 IP 到网关，标签 `通信` |
| 6 | 第二条边 | 网关到服务器，标签 `登录` |
| 7 | 第三条边 | 服务器到账号，标签 `访问` |
| 8 | 路径摘要 | `172.16.5.10 -> 185.22.14.9` |
| 9 | 权重 | `0.86 / 横向访问` 使用青色签 |
| 10 | 实体摘要 | 资产、账号、域名分三行 |
| 11 | 生成时间 | `06-20 03:43:10` |
| 12 | 状态 | `已生成` 使用绿色 |
| 13 | 节点统计 | 节点数为 4 |
| 14 | 边统计 | 边数为 3 |
| 15 | 平均权重 | 数值为 0.86 |
| 16 | 风险评分 | `85（高风险）` 使用红色 |
| 17 | 关联 PCAP | 计数为 1，带链接图标 |
| 18 | 关联 Session | 计数为 2，带链接图标 |
| 19 | 关联日志 | 计数为 1，带链接图标 |
| 20 | 底部入口 | `查看全部 图谱路径 1 项` |
| 21 | 图形边界 | 关系图与统计框互不覆盖 |
