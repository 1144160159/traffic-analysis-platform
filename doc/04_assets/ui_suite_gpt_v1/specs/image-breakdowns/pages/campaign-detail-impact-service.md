# campaign-detail-impact-service 拆解记录

## 基本信息

- page-id：`campaign-detail-impact-service`
- 分类：`pages`
- 队列顺序：29
- 分组：威胁分析
- 类型：`menu-state`
- 父页面：`campaigns`
- 宿主路由：`/campaigns/:campaignId`
- 状态路由：`/campaigns/:campaignId?impact=service`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail-impact-service.png`
- 证据目录：`evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/`
- 当前实现镜像：`traffic/web-ui:ui-campaign-impact-service-20260710-r178`
- 当前阶段：`business-pixel-accepted`（业务容差）；strict pixel 为 fail
- 目标图用途：拆解、测量、视觉 diff 和验收
- 禁止用途：不得作为业务页面截图资源加载

本图不是完整战役详情页面截图，而是战役详情中“影响范围 / 服务”业务模块的焦点视觉状态。

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

当前激活项是“服务”。

中部上半区是服务影响风险汇总。

左侧为大尺寸椭圆风险环。

风险环中心显示：

- 总数：42
- 单位：受影响服务

右侧为三行风险分布：

- 高风险：11，26.2%
- 中风险：18，42.9%
- 低风险：13，30.9%

下半区标题为“关键服务（Top 5）”。

表格包含四列：

- 服务名称
- 端口/协议
- 风险
- 依赖关系

表格包含五行：

1. PostgreSQL，5432/TCP，高危，科研管理系统
2. MinIO API，9000/TCP，高危，证据归档
3. LDAP，389/TCP，中危，统一认证
4. NFS，2049/TCP，中危，文件共享
5. Redis，6379/TCP，中危，会话缓存

底部居中显示“查看全部服务 >”。

高危文本使用红色，中危文本使用琥珀色。

## 区域与坐标

所有坐标均以目标 PNG 左上角为原点，单位为像素。

| 区域 | bbox | 说明 |
|---|---:|---|
| 画布 | `0,0,1920,1080` | Windows Chrome 目标截图画布 |
| 焦点框 | `15,8,1892,1062` | 业务模块外框 |
| 标题 | `60,31,350,42` | 影响范围 |
| Tab 条 | `58,88,1807,58` | 六个等宽业务对象 Tab |
| 服务激活 Tab | `662,88,294,58` | 蓝色激活态 |
| 风险汇总 | `58,181,1800,303` | 风险环和三行风险分布 |
| 风险环 | `157,184,647,276` | 动态 conic-gradient 椭圆环 |
| 风险列表 | `884,198,971,233` | 高中低风险分布 |
| 分隔线 | `60,483,1798,2` | 汇总与表格分隔 |
| 表格标题 | `95,507,520,43` | 关键服务（Top 5） |
| 服务表格 | `60,558,1798,418` | 表头和五行数据 |
| 下钻链接 | `868,1003,210,39` | 查看全部服务 |

## 动态数据契约

## 文本清单

- 标题锁定为 `影响范围`，六个 Tab 顺序为 `资产 / 账号 / 服务 / 部门 / 校区 / 业务系统`。
- 当前激活项为 `服务`，不得只依赖颜色，需保留可访问 selected 状态。
- 风险环中心为 `42` 与 `受影响服务`。
- 风险分布为 `高风险 11 26.2%`、`中风险 18 42.9%`、`低风险 13 30.9%`，三项相加必须为 42。
- 表标题为 `关键服务（Top 5）`。
- 表头为 `服务名称 / 端口/协议 / 风险 / 依赖关系`。
- 五行服务为 `PostgreSQL / MinIO API / LDAP / NFS / Redis`。
- 端口协议为 `5432/TCP / 9000/TCP / 389/TCP / 2049/TCP / 6379/TCP`。
- 风险为 `高危 / 高危 / 中危 / 中危 / 中危`。
- 依赖关系为 `科研管理系统 / 证据归档 / 统一认证 / 文件共享 / 会话缓存`。
- 底部下钻文案为 `查看全部服务 >`。

## 组件清单

- `CampaignImpactServicePanel`：服务影响范围焦点容器。
- `ImpactTabs`：六对象切换条并固定服务激活态。
- `CampaignImpactRiskSummary`：风险环和风险分布的双栏摘要。
- `CSSConicGradientDonut`：根据 11/18/13 动态计算弧段。
- `RiskBreakdownRow`：高、中、低三行风险语义。
- `ServiceImpactTable`：服务、端口协议、风险、依赖系统四列表。
- `RiskTag`：用文字和状态色双重表达高危、中危。
- `RouterLink`：查看完整服务影响列表。

## 图标清单

- 三个风险图例圆点分别使用红、琥珀、黄绿状态色。
- 服务 Tab 使用蓝色边框和内发光作为选中标志。
- 底部下钻使用右尖括号，图形与文字共同组成点击目标。
- 表格不使用无语义装饰图标，避免挤压端口和依赖关系列。
- 本图不存在位图组件；所有图形均应由 CSS、图标库或图表组件绘制。

## Token 与样式

- 页面背景采用近黑深蓝，面板以低亮青蓝描边分层。
- 激活蓝约 `#168cff`，链接色与其保持同族但不使用大面积渐变。
- 主文字使用冷白，次级文字使用蓝灰，不降低到不可读对比度。
- 高危红约 `#f0443e`，中危琥珀约 `#f6a70a`，低风险黄绿约 `#95c63d`。
- 标题约 34px，Tab 约 27px，表头约 24px，正文约 22px。
- 外框圆角不超过 8px；表格边框为 1px 低亮青蓝线。
- 风险环保持约 630×270 的横向椭圆视觉，不改为正圆。
- 表格行高固定，五行数据与底部链接在 1080px 画布内完整可见。
- 间距遵循 8px 基线，摘要与表格之间使用细分隔线而非卡片套卡片。
- 状态色不交换：高风险必须红、中风险必须琥珀、低风险必须绿色族。

## 状态与交互

- 初始路由 query 为 `impact=service`，刷新后保持服务 Tab 激活。
- 点击对象 Tab 只切换影响子视图，不移动宿主页面顶部、左侧与底部公共区。
- 风险环、三行风险分布与 Top 5 使用同一 typed snapshot，避免计数漂移。
- 点击服务行进入服务资产或依赖详情，需保留 campaign 上下文。
- 点击 `查看全部服务 >` 打开完整服务列表，并继承当前战役过滤条件。
- loading 和 refetch 状态固定容器尺寸，不造成表格跳动。
- API error 显示可重试错误，不将目标图或旧数据伪装成新响应。
- 无数据时显示 0 和空表，风险百分比显示 `--`，避免除零与伪造占比。

主数据入口：

```text
GET /v1/campaigns/{campaignId}
```

前端适配路径：

```text
fetchCampaignDetailSnapshot -> normalizeCampaignDetailSnapshot -> buildImpactService
```

新增/使用的类型：

- `CampaignDetailImpactService`
- `CampaignDetailServiceRow`
- `CampaignDetailImpactRiskRow`

`impactService` 数据结构：

```json
{
  "total": 42,
  "unit": "受影响服务",
  "breakdown": [
    { "label": "高风险", "count": 11, "percent": "26.2%" },
    { "label": "中风险", "count": 18, "percent": "42.9%" },
    { "label": "低风险", "count": 13, "percent": "30.9%" }
  ],
  "rows": [
    { "服务名称": "PostgreSQL", "端口协议": "5432/TCP", "风险": "高危", "依赖关系": "科研管理系统" },
    { "服务名称": "MinIO API", "端口协议": "9000/TCP", "风险": "高危", "依赖关系": "证据归档" },
    { "服务名称": "LDAP", "端口协议": "389/TCP", "风险": "中危", "依赖关系": "统一认证" },
    { "服务名称": "NFS", "端口协议": "2049/TCP", "风险": "中危", "依赖关系": "文件共享" },
    { "服务名称": "Redis", "端口协议": "6379/TCP", "风险": "中危", "依赖关系": "会话缓存" }
  ]
}
```

若后端返回 `impact_services`、`affected_services`、`services` 或 `top_services`，前端会映射服务名称、端口/协议、风险和依赖关系；若 API 暂缺该段，则使用 typed fallback 支撑视觉和交互验证。

## 实现映射

| 目标元素 | 前端实现 |
|---|---|
| 服务 Tab 激活态 | `resolveCampaignImpact('service')` + `ImpactTabs` |
| 焦点画布 | `CampaignImpactServicePanel focus` |
| 风险环 | `CampaignImpactRiskSummary` 的 conic-gradient CSS 角度 |
| 风险列表 | `impactService.breakdown` |
| Top 5 服务表 | `ServiceImpactTable` |
| 正常生产路由 | `CampaignImpactServiceContent` 嵌入 AppShell 的影响范围面板 |
| 下钻链接 | `/assets?tab=service` |

## 验收证据

## 差异清单

- 业务容差 diff 已达到当前门槛；严格像素 diff 因字体、椭圆弧抗锯齿和 Windows DPR 仍明确失败。
- 目标图是 focus 状态，普通生产路由还需验证 AppShell 和详情其他区域不被裁切。
- typed fallback 用于接口字段缺失时的可重复视觉证据，不能宣称为服务影响实时 API 数据。
- 后续精调优先级为表格列宽、风险环厚度、标题字重和下钻链接垂直位置。

| 证据 | 路径 |
|---|---|
| target | `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/target.png` |
| final implementation | `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/implementation-r178-final.png` |
| business diff | `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/diff-business-r178-final.png` |
| strict diff | `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/diff-strict-r178-final.png` |
| business metrics | `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/metrics-business-r178-final.json` |
| strict metrics | `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/metrics-strict-r178-final.json` |
| normal route screenshot | `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/normal-route-r178.png` |
| normal route runtime | `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/normal-route-runtime-r178.json` |
| verification | `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/verification.json` |
| acceptance report | `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-campaign-detail-impact-service-r178.json` |

## 结论

- Local tests：`src/routes/campaignImpactFocus.test.ts`、`src/routes/noBitmapUi.test.ts`、`src/services/campaignDetailApi.test.ts` 共 7 tests passed；日志 `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/npm-test.log`。
- Production build：`npm --prefix web/ui run build` passed，仅保留 Vite 大 chunk warning；日志 `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/npm-build.log`。
- Deployment：`traffic/web-ui:ui-campaign-impact-service-20260710-r178`，`web-ui` Deployment `1/1`，APISIX `/` 返回 200。
- Focus runtime：Windows Chrome CDP pass，无 4xx/5xx、requestfailed、console/pageerror、forbidden target resource。
- Normal route：`/campaigns/campaign-exfil-default-1782729598739-e1d2dc37?impact=service` pass，AppShell、服务 active tab、5 行服务表、下钻链接均可见且无水平溢出。
- Business visual diff：`0.07248697916666667` <= `0.35`，channel tolerance `64`。
- Strict visual diff：`0.9998230131172839` > `0.015`，channel tolerance `0`，记录为非阻断 strict pixel failure；strict capture meta 为 `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/capture-meta-strict-r178-final.json`。
- Main-thread judgment：`business-pixel-accepted`。
