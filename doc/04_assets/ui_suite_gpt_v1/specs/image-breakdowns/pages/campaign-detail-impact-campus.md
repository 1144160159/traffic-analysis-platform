# campaign-detail-impact-campus 拆解记录

## 基本信息

- page-id：`campaign-detail-impact-campus`
- 分类：`pages`
- 队列顺序：27
- 分组：威胁分析
- 类型：`menu-state`
- 父页面：`campaigns`
- 宿主路由：`/campaigns/:campaignId`
- 状态路由：`/campaigns/:campaignId?impact=campus`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail-impact-campus.png`
- 证据目录：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/`
- 目标尺寸：`1920 x 1080`
- 当前实现镜像：`traffic/web-ui:ui-campaign-impact-campus-20260710-r176`
- 当前阶段：`business-pixel-accepted`（业务容差）；strict pixel 为 fail
- 目标图用途：拆解、测量、视觉 diff 和验收
- 禁止用途：不得作为业务页面截图资源加载

本图不是完整战役详情页面截图，而是战役详情中“影响范围 / 校区”业务模块的焦点视觉状态。

普通生产路由仍然显示顶部状态栏、左侧威胁分析菜单、战役详情主体和底部状态栏。

焦点状态仅用于逐图视觉验收，不代表生产环境出现全屏 Modal、Drawer 或覆盖层。

## 目标图观察

目标图由一个带青蓝描边的深色焦点框组成。

最上方左侧显示“影响范围”。

标题下方是一行六个等宽业务对象 Tab：

1. 资产
2. 账号
3. 服务
4. 部门
5. 校区
6. 业务系统

当前激活项是“校区”。

激活项使用蓝色背景、蓝色边框和轻微发光。

未激活项使用深色背景和低对比度青蓝边框。

中部上半区是校区影响风险汇总。

左侧为大尺寸椭圆风险环。

风险环按高风险、中风险、低风险三段着色。

风险环中心显示：

- 总数：4
- 单位：受影响校区

右侧为三行风险分布：

- 高风险：1，25.0%
- 中风险：2，50.0%
- 低风险：1，25.0%

风险分布使用固定四列对齐：

- 状态色标
- 风险名称
- 数量
- 百分比

中部与下部之间使用一条低对比度青蓝分隔线。

下半区标题为“关键校区（Top 5）”。

表格包含四列：

- 校区/楼宇
- 覆盖资产
- 风险
- 链路

表格包含五行：

1. 主校区-数据中心，26，高危，核心链路
2. 主校区-科研楼，18，中危，东西向
3. 东校区-办公楼，9，中危，VPN 链路
4. 南校区-教学楼，7，中危，无线网
5. 西校区-图书馆，5，低危，出口链路

底部居中显示“查看全部校区 >”。

高危文本和核心链路使用红色。

中危链路使用琥珀色。

低危和出口链路使用绿色。

## 区域与坐标

所有坐标均以目标 PNG 左上角为原点，单位为像素。

| 区域 | bbox | 说明 |
|---|---:|---|
| 画布 | `0,0,1920,1080` | Windows Chrome 目标截图画布 |
| 焦点框 | `15,8,1892,1062` | 业务模块外框 |
| 标题 | `60,31,350,42` | 影响范围 |
| Tab 条 | `58,88,1807,58` | 六个等宽业务对象 Tab |
| 校区激活 Tab | `1241,88,280,58` | 蓝色激活态 |
| 风险汇总 | `58,181,1800,303` | 风险环和三行风险分布 |
| 风险环 | `157,184,647,276` | 动态 conic-gradient 椭圆环 |
| 风险列表 | `884,198,971,233` | 高中低风险分布 |
| 分隔线 | `60,483,1798,2` | 汇总与表格分隔 |
| 表格标题 | `95,507,520,43` | 关键校区（Top 5） |
| 校区表格 | `60,558,1798,418` | 表头和五行数据 |
| 下钻链接 | `728,1003,468,39` | 查看全部校区 |

焦点框四边均不得超出画布。

Tab 条宽度应保持稳定。

六个 Tab 切换时不允许改变整体长度、位置和行高。

风险环与风险列表必须位于同一水平业务区。

表格不得被焦点框底部或浏览器底部裁切。

下钻链接必须完整显示在焦点框内部。

## 文本清单

以下文本来自目标图人工读取，OCR 仅作为辅助。

### 标题与 Tab

- `影响范围`
- `资产`
- `账号`
- `服务`
- `部门`
- `校区`
- `业务系统`

### 风险汇总

- `4`
- `受影响校区`
- `高风险`
- `1`
- `25.0%`
- `中风险`
- `2`
- `50.0%`
- `低风险`
- `1`
- `25.0%`

### 表格标题与表头

- `关键校区（Top 5）`
- `校区/楼宇`
- `覆盖资产`
- `风险`
- `链路`

### 第一行

- `主校区-数据中心`
- `26`
- `高危`
- `核心链路`

### 第二行

- `主校区-科研楼`
- `18`
- `中危`
- `东西向`

### 第三行

- `东校区-办公楼`
- `9`
- `中危`
- `VPN 链路`

### 第四行

- `南校区-教学楼`
- `7`
- `中危`
- `无线网`

### 第五行

- `西校区-图书馆`
- `5`
- `低危`
- `出口链路`

### 下钻入口

- `查看全部校区 >`

表格中的校区名称必须提供完整文字。

链路名称不得因图标挤压而截断。

正常生产路由中的长战役 ID 可以省略显示，但必须通过 `title` 或 Tooltip 查看全文。

## 组件清单

| 组件 | 文件 | 责任 |
|---|---|---|
| `CampaignDetailPage` | `web/ui/src/pages/CampaignDetailPage.tsx` | 路由和状态选择 |
| `CampaignImpactCampusPanel` | 同上 | 校区影响焦点容器 |
| `ImpactTabs` | 同上 | 六个影响对象切换 |
| `CampaignImpactCampusContent` | 同上 | 校区业务内容组合 |
| `CampaignImpactRiskSummary` | 同上 | 风险环和风险列表 |
| `RiskBreakdownRow` | 同上 | 单行风险分布 |
| `CampusImpactTable` | 同上 | Top 5 校区表格 |
| `fetchCampaignDetailSnapshot` | `web/ui/src/services/campaignDetailApi.ts` | 真实 API 数据入口 |
| `normalizeCampaignDetailSnapshot` | 同上 | payload 归一化 |
| `buildImpactCampus` | 同上 | typed 校区影响模型 |
| `windowFrameState` | `web/ui/src/utils/windowFrameState.ts` | Windows 和浏览器窗口状态 |

业务图示不得引用目标 PNG。

风险环必须由数据计算角度。

表格必须由 `snapshot.impactCampus.rows` 渲染。

Tab 必须更新 query 状态。

## 图标清单

| 图标 | 用途 | 来源 |
|---|---|---|
| `LinkOutlined` | 核心链路 | Ant Design Icons |
| `SwapOutlined` | 东西向链路 | Ant Design Icons |
| `SafetyOutlined` | VPN 链路 | Ant Design Icons |
| `WifiOutlined` | 无线网 | Ant Design Icons |
| `UploadOutlined` | 出口链路 | Ant Design Icons |

图标是独立语义图标，可以使用组件库实现。

图标不得与完整业务卡片合并为图片资源。

链路状态颜色由风险级别决定。

图标尺寸必须稳定，不能改变表格行高。

## Token 与样式

| Token | 值 | 用途 |
|---|---|---|
| `canvas-bg` | `#020c14` | 焦点态背景 |
| `panel-bg` | `rgba(4,22,36,.72)` | 风险和表格面板 |
| `border-primary` | `rgba(28,118,205,.82)` | 外框 |
| `border-subtle` | `rgba(56,151,201,.24)` | 分隔线和表格线 |
| `text-primary` | `rgba(245,248,252,.92)` | 标题和关键值 |
| `text-secondary` | `rgba(236,244,250,.84)` | Tab 和正文 |
| `text-muted` | `#7da9c8` | 表头和辅助文字 |
| `status-high` | `#ff4149` | 高风险 |
| `status-medium` | `#ffb020` | 中风险 |
| `status-low` | `#75c743` | 低风险 |
| `active-tab` | `rgba(11,62,126,.9)` | 校区激活态 |
| `focus-canvas` | `2133x1200` | 语义设计画布 |

目标截图像素尺寸固定为 `1920x1080`。

设计画布按真实 CSS viewport 等比缩放。

缩放取宽度比例和高度比例中的较小值。

缩放原点位于焦点画布中心。

正常生产页面不使用焦点态全屏布局。

## 状态与交互

### Tab 状态

- 默认宿主状态可为资产。
- `impact=account` 显示账号状态。
- `impact=business-system` 显示业务系统状态。
- `impact=campus` 显示校区状态。
- 校区 Tab 必须具有 `is-active` 状态。
- 切换 Tab 时父级战役详情对象保持不变。

### 数据状态

- 正常生产路由每 30 秒刷新一次战役快照。
- 视觉验收模式关闭定时刷新，确保 diff 可复现。
- API 成功时优先使用真实字段。
- API 字段缺失时使用 typed fallback。
- fallback 字段保持与真实 API 可迁移的结构。

### 下钻状态

- 点击“查看全部校区”进入 `/assets?tab=campus`。
- 进入校区影响状态时父级菜单仍选中“战役列表”。
- 正常生产路由不得自动打开全屏 Modal 或 Drawer。

### 响应式状态

- 焦点态设计画布为 `2133x1200`。
- 画布使用 `100dvw / 2133px` 和 `100dvh / 1200px` 计算缩放。
- Windows 浏览器 DPI 变化不能成为布局正确的前提。
- 浏览器窗口缩小时，焦点内容整体等比收缩。
- 正常生产详情继续由 AppShell 窗口类控制布局。

## 实现映射

真实数据入口：

```text
GET /v1/campaigns/{campaignId}
  -> fetchCampaignDetailSnapshot
  -> normalizeCampaignDetailSnapshot
  -> buildImpactCampus
  -> snapshot.impactCampus
  -> CampaignImpactCampusContent
```

typed contract：

- `CampaignDetailImpactCampus`
- `CampaignDetailCampusRow`
- `CampaignDetailImpactRiskRow`

动态字段：

- `impactCampus.total`
- `impactCampus.unit`
- `impactCampus.breakdown[].label`
- `impactCampus.breakdown[].count`
- `impactCampus.breakdown[].percent`
- `impactCampus.rows[].校区楼宇`
- `impactCampus.rows[].覆盖资产`
- `impactCampus.rows[].风险`
- `impactCampus.rows[].链路`

风险环实现：

```text
count / total * 360deg
  -> --taf-impact-high-deg
  -> --taf-impact-medium-deg
  -> CSS conic-gradient
```

该实现是动态 DOM/CSS 图示。

该实现不是 Canvas 截图。

该实现不读取 `target.png`。

正常路由证据证明相同组件嵌入真实战役详情和 AppShell 中。

## 验收证据

- 目标图：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/target.png`
- 区域图：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/regions-overlay.png`
- 实现图：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/implementation-r176-final.png`
- 实现别名：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/implementation.png`
- 业务容差 diff：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/diff-business-r176-final.png`
- 严格像素 diff：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/diff-strict-r176-final.png`
- diff 别名：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/diff.png`
- 业务容差 metrics：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/metrics-business-r176-final.json`
- 严格像素 metrics：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/metrics-strict-r176-final.json`
- capture meta：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/capture-meta-r176-final.json`
- 生产路由报告：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/production-route-report-r176-final.json`
- 正常路由截图：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/normal-route-r176.png`
- 正常路由 runtime：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/normal-route-runtime-r176.json`
- CDP version：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/cdp-version-r176-final.json`
- CDP list：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/cdp-list-r176-final.json`
- 生产镜像：`traffic/web-ui:ui-campaign-impact-campus-20260710-r176`

焦点态 runtime：

- console error：0
- page error：0
- requestfailed：0
- HTTP 4xx/5xx：0
- 禁止资源请求：0
- 横向溢出：false
- 纵向滚动：false

业务容差视觉 diff：

- target：`1920x1080`
- implementation：`1920x1080`
- mismatch pixels：`148200 / 2073600`
- mismatch ratio：`0.07146990740740741`
- threshold：`0.35`
- channel tolerance：`64`
- status：pass

严格像素 diff：

- mismatch ratio：`0.9999136766975308`
- threshold：`0.015`
- channel tolerance：`0`
- status：fail

这里的 `business-pixel-accepted` 只能表示业务容差门通过，不能表述为 strict pixel pass 或 pixel-perfect。

正常生产路由：

- AppShell 顶栏：存在
- 左侧菜单：存在
- 底部状态栏：存在
- 焦点全屏宿主：不存在
- 校区内容：存在
- 校区数据行：5
- 六个表格行（表头加 5 条数据）全部位于面板 body：true
- “查看全部校区”位于面板 body：true
- 校区内容容器位于面板 body：true
- 打开的 Modal：0
- 打开的 Drawer：0
- 横向溢出：false
- runtime：pass

## 差异清单

### 字体差异

目标图使用生成图中文字字形。

Windows Chrome 使用系统中文字体和浏览器抗锯齿。

标题、Tab、风险标签和表格文字存在字形与 glow 差异。

业务文字和数值没有缺失。

### 图标差异

目标图链路图标具有生成图轮廓。

实现使用 Ant Design 语义图标。

核心、东西向、VPN、无线、出口五种语义均完整。

图标差异不改变业务含义。

### 风险环差异

目标和实现均为三段椭圆环。

实现环角度由 typed risk count 动态计算。

剩余热点主要来自边缘抗锯齿和环厚度。

### 上下文差异

目标图是业务模块焦点态。

正常生产路由必须保留 AppShell 和战役详情上下文。

本轮分别保留焦点视觉证据和正常路由业务证据。

## 结论

r176 已完成 UI 拆解、动态代码实现、测试、构建、双节点发布、Windows Chrome 真实生产路由截图、runtime 检查、普通路由几何回归和双视觉指标。

独立辅助智能体复核动态实现、真实路由、普通态几何、runtime 和双视觉指标后给出 PASS；主线程复核代码、三张截图、指标和脱敏结果后同意业务容差验收。

本页状态为 `business-pixel-accepted`，仅表示 `0.35 / tolerance 64` 业务容差门通过；严格 `0.015 / tolerance 0` 像素门仍明确为 fail，不能称为 pixel-perfect。
