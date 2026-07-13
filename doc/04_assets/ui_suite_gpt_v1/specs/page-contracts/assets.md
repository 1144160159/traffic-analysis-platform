# 资产台账 前端实现契约

## 基本信息

- ID：`assets`
- 路由：`/assets`
- 领域：`asset-graph`
- React 页面：`AssetInventoryPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/assets.png`
- API：`/api/v1/assets`

## 必须实现的业务层

- 资产分类工作台：终端、服务器、网络设备、业务系统、未知资产
- 选中资产上下文：列表选中项、右侧摘要、风险和证据必须同步
- 单资产右侧详情 Drawer：基础信息、网络接口、开放服务、归属信息、历史变更；宿主服务器工作台必须保持可见
- 流量画像与动态图表
- 证据关联、处置任务和审计反馈

## V2 状态模型

资产分类和资产详情是两层状态，不得互相代替：

- `tab`：资产分类，取值 `endpoint|server|network-device|business-system|unknown`
- `assetId`：当前选中对象；切换列表行时更新，右侧摘要随之更新
- `detail`：同一资产的详情子页，取值 `basic|network-interface|open-services|ownership|history`
- 详情 URL：`/assets?tab=server&assetId=SRV-0007&detail=<detail>`
- 关闭详情只移除 `detail`，保留原 `tab` 和 `assetId`，返回原列表上下文

| 图片 ID | 分类状态 | 选中对象 | 详情状态 | 页面语义 |
|---|---|---|---|---|
| `assets` | `endpoint` | `PC-0082` | - | 终端分类工作台 |
| `assets-server` | `server` | `SRV-0007` | - | 服务器分类工作台 |
| `assets-network-device` | `network-device` | `NET-0001` | - | 网络设备分类工作台 |
| `assets-business-system` | `business-system` | `BIZ-0001` | - | 业务系统分类工作台 |
| `assets-unknown` | `unknown` | `UNK-10.12.88.45` | - | 未知资产确认工作台 |
| `assets-detail-basic` | `server` | `SRV-0007` | `basic` | 单资产基础信息 |
| `assets-detail-network-interface` | `server` | `SRV-0007` | `network-interface` | 同一资产网络接口 |
| `assets-detail-open-services` | `server` | `SRV-0007` | `open-services` | 同一资产开放服务 |
| `assets-detail-ownership` | `server` | `SRV-0007` | `ownership` | 同一资产归属关系 |
| `assets-detail-history` | `server` | `SRV-0007` | `history` | 同一资产变更与回滚 |

## 交互约束

- 分类 Tab 切换后，Tab 几何位置保持固定，不复用详情子 Tab 状态。
- 主列表必须支持行选择、服务端或真实 API 分页、筛选、刷新和导出。
- 所有可见按钮必须有点击结果：路由变化、Drawer/Modal、下载、任务反馈或错误反馈之一。
- 图表使用 API 数据驱动的 ECharts；拓扑继续使用既有 API 驱动 SVG 动图。
- 详情五个子页共享标题、资产身份、风险状态和子 Tab 几何，仅业务内容变化。
- 基础信息、归属信息使用较窄 Drawer；网络接口、开放服务、历史变更使用较宽 Drawer。Drawer 不得覆盖左侧菜单和全部宿主业务上下文。
- 详情页不得用 `server/network-device/business-system` 分类 Tab 冒充 `open-services/network-interface/ownership`。
- 生产截图证据必须记录完整 `tab + assetId + detail`，并在同一次运行生成 capture、diff、metrics 和 verification。

## 分层参数

- `topbar`：global-app-shell，bbox=`{"x":0,"y":0,"w":1920,"h":80}`
- `sidebar`：global-app-shell，bbox=`{"x":0,"y":80,"w":166,"h":917}`
- `content`：page-workspace，bbox=`{"x":198,"y":80,"w":1722,"h":917}`
- `bottombar`：global-app-shell，bbox=`{"x":0,"y":997,"w":1920,"h":83}`
- `right-rail`：closed-loop-rail，bbox=`{"x":1460,"y":104,"w":420,"h":860}`

## 组件映射

- AppShell
- WorkPanel
- MetricTile
- Table
- Tabs
- ECharts
- StatusTag

## 关联浮层

- `drawer-asset-detail`：资产详情，Drawer
- `modal-asset-edit`：编辑资产，Modal
- `drawer-asset-history`：资产历史，Drawer

## 验收清单

- [ ] 最终 PNG 必须为 1920x1080
- [ ] 中文为主，只保留必要英文技术词和单位
- [ ] 状态色必须遵守 success/info/warning/danger/critical token
- [ ] 危险动作必须具备影响范围、权限提示和审计留痕
- [ ] 公共 AppShell 必须与 screen.png 目标参数一致
- [ ] 页面主工作区不得复用相邻页面的业务组件组合
- [ ] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [ ] React Query 必须覆盖 loading/error/empty 状态
