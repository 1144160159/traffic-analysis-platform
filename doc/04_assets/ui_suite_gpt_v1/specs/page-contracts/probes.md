# 探针管理 前端实现契约

## 基本信息

- ID：`probes`
- 路由：`/probes`
- 领域：`collection-monitoring`
- React 页面：`ProbesManagementPage`
- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/pages/probes.png`
- API：`/api/v1/probes`（列表、状态矩阵、趋势）+ `/api/v1/probes/topology`（独立、渲染中立的 2D/3D 图契约）

## 必须实现的业务层

- 探针总览
- 部署拓扑
- 吞吐与丢包
- 探针配置
- 心跳与日志
- 批量运维

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
- API 动态 SVG（2D 平面坐标 / 3D 等距投影）
- ECharts
- StatusTag

## 关联浮层

- `drawer-probe-detail`：探针详情，Drawer
- `modal-probe-config`：探针配置下发，Modal
- `modal-probe-batch-upgrade`：探针批量升级确认，Modal
- `modal-probe-cert-rotate`：证书轮换确认，Modal
- `drawer-probe-log`：探针日志抽屉，Drawer

## 验收清单

- [x] 最终 PNG 为 1920x1080
- [x] 中文为主，只保留必要英文技术词和单位
- [x] 状态色遵守 success/info/warning/danger/critical token
- [x] 危险动作具备影响范围、权限提示、queued 状态和审计留痕
- [x] 公共 AppShell 与目标公共布局保持一致
- [x] 页面主工作区使用探针管理专用组合
- [x] 所有 API 调用经 services/api.ts 或现有服务封装
- [x] React Query 覆盖 loading/error 状态；空拓扑显示明确空态
- [x] `/v1/probes/topology` 返回双布局坐标、分区多边形、节点语义、链路状态与带宽，来源标记为 `postgres.probes.hardware_info`
- [x] 拓扑读取权限限定为 `probe:read` / `probe:write` / `admin:*`；`probe:metrics` 返回 403，跨租户返回空图，非法 mode 返回 400
- [x] API 在归一化后保证节点最小间距 7，双向链路按最大带宽与最严重状态合并
- [x] 2D/3D 切换直接使用 API 的 `position_2d` / `position_3d` 重绘 SVG；无静态园区图片、无客户端拓扑夹具
- [x] SVG 支持节点选择详情、键盘激活、滚轮/按钮缩放、拖拽平移和视图重置
- [x] 状态矩阵 10 条/页，分页槽固定且末页不改变分页栏位置
- [x] Windows Chrome 1920x1080 验收 39/39；2D/3D 证据均为 1920x1080，且 2D 内容完整性门禁通过；视觉 ROI mismatch ratio `0.10098092746850873 <= 0.12`

## 当前运行语义

- 六类运维动作会在同一 PostgreSQL 事务中持久化 `probe_operations.status=queued` 并写入 `PROBE_*_QUEUED` 审计事件。
- 页面不会把控制面受理伪装成探针执行完成；完成状态必须等待后续探针控制通道 ACK 消费者回写。
- fixture 只更新带 `hardware_info.fixture=probes-ui-v1` 的记录；非 fixture 同 ID 冲突会触发守卫失败，不覆盖运营数据。
- `probes-ui-v1` 验收 fixture 使用显式状态保持目标构成（24 在线、3 告警、1 离线）；生产探针仍按 5 分钟心跳失联规则判定离线。

## 2026-07-16 业务区滚动补充

- `.taf-probes` 是探针管理页唯一的纵向滚动容器；部署拓扑、状态矩阵、吞吐与丢包趋势、批量运维和心跳日志不得再被父级 `overflow: hidden` 裁切。
- 滚动到底后，批量发送带宽阈值图、状态矩阵分页栏与心跳日志末行必须同时可达；页面不得产生横向溢出。
- Windows Chrome 1920x1080 最终验收报告：`doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-probes-latest.json`，`39/39 pass`；顶部与滚动到底截图均保留。

## 2026-07-17 多分辨率补充

- `1600x900` 保持主栏与右栏并排；`1366x768` 按主栏、详情/批量运维/心跳日志顺序纵向排列，唯一滚动主体仍为 `.taf-probes`。
- 窄屏外层网格必须以实际模块高度显式包住主栏、底部双模块和右栏，禁止子模块在父网格轨道外继续绘制；主栏与右栏之间至少保留 6px 间距。
- 响应式阻断门检查拓扑高度不少于 250px、状态行高度不少于 20px、同组模块零交叠、详情/批量运维内容不越出面板、每组父容器完整包住直接子模块、业务区无横向溢出且底部可达。
- Windows Chrome 双分辨率报告：`evidence/ui-responsive/probes-data-quality/windows-chrome-responsive-latest.json`，结果 `pass`；顶部和滚动到底截图位于同目录。桌面回归仍为 `39/39 pass`。
