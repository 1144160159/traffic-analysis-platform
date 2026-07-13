# campaign-detail-impact-department 拆解记录

## 基本信息

- page-id：`campaign-detail-impact-department`
- 分类：`pages`
- 队列顺序：28
- 分组：威胁分析
- 类型：`menu-state`
- 父页面：`campaigns`
- 宿主路由：`/campaigns/:campaignId`
- 状态路由：`/campaigns/:campaignId?impact=department`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail-impact-department.png`
- 证据目录：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/`
- 当前实现镜像：`traffic/web-ui:ui-campaign-impact-department-20260710-r177`
- 当前阶段：`business-pixel-accepted`（业务容差）；strict pixel 为 fail
- 目标图用途：拆解、测量、视觉 diff 和验收
- 禁止用途：不得作为业务页面截图资源加载

本图不是完整战役详情页面截图，而是战役详情中“影响范围 / 部门”业务模块的焦点视觉状态。

普通生产路由仍然显示顶部状态栏、左侧威胁分析菜单、战役详情主体和底部状态栏。

焦点状态仅用于逐图视觉验收，不代表生产环境出现全屏 Modal、Drawer 或覆盖层。

## 目标图观察

目标图由一个带青蓝描边的深色焦点框组成。

左上方标题为“影响范围”。

标题下方是一行六个等宽业务对象 Tab：

1. 资产
2. 账号
3. 服务
4. 部门
5. 校区
6. 业务系统

当前激活项是“部门”。

中部上半区是部门影响风险汇总。

左侧为大尺寸椭圆风险环。

风险环中心显示：

- 总数：7
- 单位：受影响部门

右侧为三行风险分布：

- 高风险：2，28.6%
- 中风险：3，42.8% 或 42.9%
- 低风险：2，28.6%

下半区标题为“关键部门（Top 5）”。

表格包含四列：

- 部门名称
- 责任人
- 风险
- 处置进度

表格包含五行：

1. 科研处，王主任，高危，40%
2. 信息中心，sec_manager，高危，55%
3. 财务处，李主任，中危，60%
4. 教务处，张主任，中危，72%
5. 图书馆，运维组，中危，80%

底部居中显示“查看全部部门 >”。

高危文本和进度条使用红色。

中危文本和进度条使用琥珀色。

## 区域与坐标

所有坐标均以目标 PNG 左上角为原点，单位为像素。

| 区域 | bbox | 说明 |
|---|---:|---|
| 画布 | `0,0,1920,1080` | Windows Chrome 目标截图画布 |
| 焦点框 | `15,8,1892,1062` | 业务模块外框 |
| 标题 | `60,31,350,42` | 影响范围 |
| Tab 条 | `58,88,1807,58` | 六个等宽业务对象 Tab |
| 部门激活 Tab | `957,88,280,58` | 蓝色激活态 |
| 风险汇总 | `58,181,1800,303` | 风险环和三行风险分布 |
| 风险环 | `157,184,647,276` | 动态 conic-gradient 椭圆环 |
| 风险列表 | `884,198,971,233` | 高中低风险分布 |
| 分隔线 | `60,483,1798,2` | 汇总与表格分隔 |
| 表格标题 | `95,507,520,43` | 关键部门（Top 5） |
| 部门表格 | `60,558,1798,418` | 表头和五行数据 |
| 下钻链接 | `728,1003,468,39` | 查看全部部门 |

## 动态数据契约

## 文本清单

- 标题必须为 `影响范围`，当前对象 Tab 必须为 `部门`。
- 六个 Tab 按 `资产 / 账号 / 服务 / 部门 / 校区 / 业务系统` 固定顺序展示。
- 风险摘要必须显示 `7 受影响部门`、`高风险 2 28.6%`、`中风险 3 42.8%`、`低风险 2 28.6%`。
- 表头必须为 `部门名称 / 责任人 / 风险 / 处置进度`。
- 五行部门依次为 `科研处 / 信息中心 / 财务处 / 教务处 / 图书馆`。
- 责任人依次为 `王主任 / sec_manager / 李主任 / 张主任 / 运维组`。
- 风险依次为 `高危 / 高危 / 中危 / 中危 / 中危`，不可用颜色替代文字。
- 进度依次为 `40% / 55% / 60% / 72% / 80%`，数值与进度条长度同时表达。
- 底部下钻动作必须为 `查看全部部门 >`。

## 组件清单

- `CampaignImpactDepartmentPanel`：影响范围部门焦点面板。
- `ImpactTabs`：六等分对象切换条，部门态使用蓝色激活底。
- `CampaignImpactRiskSummary`：风险环与三行风险分布组合。
- `CSSConicGradientDonut`：按 2/3/2 计算红黄绿弧段。
- `RiskBreakdownRow`：风险色点、标签、数量和百分比四列对齐。
- `DepartmentImpactTable`：五行关键部门表及处置进度条。
- `ProgressBar`：风险色前景与深蓝轨道，不改变行高。
- `RouterLink`：进入全部部门列表的可点击下钻入口。

## 图标清单

- 风险图例使用三个实心圆点，分别绑定高风险红、中风险琥珀、低风险黄绿。
- Tab 激活态由蓝色描边和内发光表达，不额外放置装饰图标。
- 进度条使用水平胶囊轨道，但文字百分比独立于轨道右侧。
- 下钻入口使用右尖括号语义，点击区域应覆盖完整文案。
- 本图没有可复用位图图标，禁止截取目标图作为组件背景。

## Token 与样式

- 画布背景：`#020e18` 附近的深海军蓝。
- 面板背景：`#03131f`，与画布保持轻微层级差。
- 主边框：青蓝低亮线，约 `1px`；焦点外框允许更亮一档。
- 激活色：`#168cff`；链接和部门 Tab 使用同一语义色族。
- 主文字：冷白 `#d8dde5`；辅助文字降低亮度但保持可读。
- 高风险：红 `#f0443e`；中风险：琥珀 `#f6a70a`；低风险：黄绿 `#95c63d`。
- 标题约 34px，Tab 约 27px，表头约 24px，表格正文约 22px。
- 焦点框圆角不超过 8px；表格和风险框使用直角感更强的小圆角。
- 主要间距按 8px 基线组织；标题到 Tab、摘要到表格均保持稳定垂直节奏。
- 禁止渐变球、插画背景和营销式大留白。

## 状态与交互

- 默认状态为 `部门` Tab 激活，刷新或直达 URL 后不得回到资产 Tab。
- 点击其他对象 Tab 更新 query `impact`，面板宽高和 Tab 几何保持不变。
- 风险环、风险分布和 Top 5 必须来自同一 snapshot，合计值必须等于 7。
- 点击部门行进入对应部门影响详情，键盘焦点可见。
- 点击 `查看全部部门 >` 进入部门资产台账或过滤后的完整部门列表。
- loading 使用固定尺寸骨架，不允许摘要与表格上下跳动。
- error 状态显示可重试信息，不得回放目标 PNG。
- 空数据状态显示 0 和空表说明，不得继续展示 7 的 fallback 冒充实时结果。

主数据入口：

```text
GET /v1/campaigns/{campaignId}
```

前端适配链路：

```text
fetchCampaignDetailSnapshot
  -> normalizeCampaignDetailSnapshot
  -> buildImpactDepartment
  -> snapshot.impactDepartment
  -> CampaignImpactDepartmentContent
```

关键字段：

- `impactDepartment.total`
- `impactDepartment.unit`
- `impactDepartment.breakdown[].label`
- `impactDepartment.breakdown[].count`
- `impactDepartment.breakdown[].percent`
- `impactDepartment.rows[].部门名称`
- `impactDepartment.rows[].责任人`
- `impactDepartment.rows[].风险`
- `impactDepartment.rows[].处置进度`

生产实现允许后端字段为空时使用 typed fallback，但 UI 仍由 `snapshot.impactDepartment` 驱动。

不得将 `target.png`、`implementation.html`、`screens/pages/*.png` 或 evidence 位图作为页面实现资源加载。

## 实现映射

- 页面：`web/ui/src/pages/CampaignDetailPage.tsx`
- 数据：`web/ui/src/services/campaignDetailApi.ts`
- 样式：`web/ui/src/styles/pages.css`
- 单测：`web/ui/src/routes/campaignImpactFocus.test.ts`
- 单测：`web/ui/src/services/campaignDetailApi.test.ts`
- 生产镜像：`traffic/web-ui:ui-campaign-impact-department-20260710-r177`

焦点状态组件：

- `CampaignImpactDepartmentPanel`
- `CampaignImpactDepartmentContent`
- `CampaignImpactRiskSummary`
- `DepartmentImpactTable`

普通生产路由组件：

- `CampaignDetailPage`
- `ImpactTabs`
- `WorkPanel title="影响范围"`
- `CampaignImpactDepartmentContent`

## Windows Chrome 证据

## 验收证据

- 浏览器：Windows Chrome 150.0.7871.49
- CDP：`http://127.0.0.1:9224`
- 生产入口：`http://10.0.5.8:30180`
- 截图像素：1920 x 1080
- CSS viewport：2133 x 1200
- DPR：0.9

焦点生产路由：

```text
/campaigns/campaign-exfil-default-1782729598739-e1d2dc37?impact=department&__codex_page_id=campaign-detail-impact-department&__codex_ui_breakdown_production=1
```

普通业务路由：

```text
/campaigns/campaign-exfil-default-1782729598739-e1d2dc37?impact=department
```

## 验收门

## 差异清单

- 业务容差截图已通过；严格逐像素比较仍受字体栅格化、曲线抗锯齿和 Windows 缩放影响而失败。
- 目标图风险环偏横向椭圆，生产实现必须保持该比例，不能自动收缩为正圆。
- 全局 AppShell 不属于本焦点图，普通生产路由仍需保留宿主页面上下文。
- API 暂无部门影响字段时只允许明确标注的 typed fallback；该事实不能被描述为实时接口成功。

业务容差门：

- 状态：pass
- mismatch pixels：154881 / 2073600
- ratio：`0.07469184027777778`
- 阈值：`0.35`
- channel tolerance：`64`

严格像素门：

- 状态：fail
- mismatch pixels：2073024 / 2073600
- ratio：`0.9997222222222222`
- 阈值：`0.015`
- channel tolerance：`0`

普通业务路由：

- AppShell topbar/sidebar/bottombar：pass
- 激活 Tab：`部门7 个`
- Top5 行数：5
- 处置进度值：`40% / 55% / 60% / 72% / 80%`
- 行和下钻链接均在影响范围面板内：pass
- horizontal overflow：false
- runtime errors：0

## 证据清单

- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/target.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/regions-overlay.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/measurement.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/implementation-r177-final.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/diff-business-r177-final.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/diff-strict-r177-final.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/metrics-business-r177-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/metrics-strict-r177-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/capture-meta-r177-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/production-route-report-r177-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/normal-route-r177.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/normal-route-runtime-r177.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/cdp-version-r177-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/cdp-list-r177-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/verification.json`

## 结论

本页状态为 `business-pixel-accepted`，仅表示 `0.35 / tolerance 64` 业务容差门通过；严格 `0.015 / tolerance 0` 像素门仍明确为 fail，不能称为 pixel-perfect。
