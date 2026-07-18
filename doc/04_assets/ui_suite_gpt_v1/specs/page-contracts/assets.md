# 资产台账前端实现契约

## 基本信息

- ID：`assets`
- 路由：`/assets`
- React 页面：`AssetInventoryPage`
- 列表 API：`GET /api/v1/assets`
- 详情 API：`GET /api/v1/assets/{assetId}`
- 历史 API：`GET /api/v1/assets/{assetId}/history`
- 详情上下文 API：`GET /api/v1/assets/{assetId}/details`
- UI 图策略：UI 图是视觉层级、布局结构、信息密度和风格的主基准；如果图中的业务逻辑与真实服务契约、数据库字段或系统状态模型冲突，开发中允许纠正，但必须记录差异原因并通过逻辑、布局双审查。

## 主线程裁决后的状态模型

```text
/assets?tab=<category>&assetId=<canonical-uuid>
/assets?tab=server&assetId=<canonical-uuid>&detail=<available-detail>
```

| 状态轴 | 合法值 | 约束 |
|---|---|---|
| `tab` | `endpoint/server/network-device/business-system/unknown` | 唯一页面级 Tab，对应后端 `asset_type` |
| `assetId` | 当前分类记录的 `asset_id` UUID | 路由和 API 使用规范主键；`display_code` 仅用于展示 |
| `detail` | `basic/network-interface/open-services/ownership/history` | 仅 `server` 分类可打开；五态必须可达并由真实数据驱动 |

- URL 没有 `assetId` 时，页面在真实 API 返回后选择本页第一条 UUID，并规范化 URL。
- URL 中的 UUID 不在当前分类或当前页时，页面回退到本页第一条真实记录，不生成 `PC-*`、`SRV-*` 等伪主键。
- 切换分类会重置页码并重新查询对应 `asset_type`，不会沿用上一分类的资产。
- `network-interface/open-services/ownership` 必须读取资产详情上下文 API；空数据时展示真实空态，不得以禁用页签替代实现。
- “高风险、待确认、最近变更”属于筛选或治理状态，不新增页面级 Tab。

## 五类资产页面逻辑

五个 Tab 使用同一套真实数据骨架，不再维护五份本地常量：

1. 服务端分页资产表：展示 `display_code`，行键和下钻参数使用 `asset_id`。
2. 真实筛选：关键词、状态、园区、部门传入 `/v1/assets`；租户只能来自认证上下文。
3. 右侧上下文：随选中行同步身份、风险、责任边界、最近活跃和邻居证据。
4. 本页质量摘要：只根据本次 API 返回记录计算，不把本页统计冒充全局总量。
5. 未实现能力明确标为“待接入”，不渲染本地服务、漏洞、证据或工单数据。

## 详情与跨域下钻

- `basic` 读取 `/v1/assets/{assetId}`，展示 UUID、展示编号、类型、状态、IP/MAC、主机、OS、厂商、来源、网络标识、园区、部门、责任人、首次发现、最近活跃和真实标签。
- `history` 读取 `/v1/assets/{assetId}/history`，展示事件 ID、时间、类型和变更前后值。
- `network-interface` 读取 `/v1/assets/{assetId}/details` 的 `network_interfaces`，展示接口身份、IP/MAC、VLAN、镜像/状态、速率和流量/错误观测。
- `open-services` 读取同一接口的 `open_services`，展示端口、协议、服务/版本、暴露范围、访问来源、风险和关联告警。
- `ownership` 读取同一接口的 `ownership`，展示园区/部门/负责人、责任角色、业务系统、资产组、数据域和待确认字段。
- 关系下钻：`/graph?assetId=<canonical-uuid>`。
- 证据下钻：`/forensics?assetId=<canonical-uuid>`；`GET /v1/pcap/jobs` 必须以任务 `params.asset_id` 真正过滤，不能只把参数留在 URL 或请求层。
- 基线页能够接收 `/baselines?assetId=<canonical-uuid>`，但资产页在没有明确业务动作前不额外暴露按钮。
- 目标页面必须读取并显示来源 `assetId`；图谱必须解析真实 IP，取证响应只能包含该资产任务。无匹配任务时显示真实空态，不复制或生成模拟任务。

## 权限与租户

- 前端路由要求 `asset:read`，`graph:read` 不再隐式获得资产台账权限。
- 列表、详情、历史、发现任务、凭据列表和邻居列表均要求资产读取权限。
- 详情和历史查询必须同时使用认证身份中的租户与资产 UUID。
- Query 参数不能覆盖认证租户；缺少可信租户时返回拒绝结果。
- 未鉴权生产请求必须返回 `401`，无权限请求返回 `403`。

## 数据库与展示编号

- `asset_id`：规范主键 UUID，用于关系、历史、审计和跨页上下文。
- `display_code`：租户内唯一的人类可读编号，如 `SRV-0001`，不可用作 API 主键。
- `asset_type/status/department/campus/owner`：资产台账真实治理字段。
- 索引必须覆盖租户 + 类型 + 状态；展示编号在租户内唯一。
- 旧库允许多个资产观察到相同 `ip_address`；迁移不得强制把重复地址回填到历史唯一 `ip` 字段。

## 交互与布局约束

- 页面必须填满可用工作区；主体采用“主表 + 全高右侧上下文 + 下方四块业务摘要”的密集布局，不允许无业务意义的大面积空白。
- 终端业务摘要中的“流量画像”和“协议分布”必须使用真实 ECharts：流量画像展示 `Mbps` 坐标轴、时间轴以及入站/出站/东西向三组序列；协议分布展示七类协议扇区、逐项百分比图例和中心总流量。不得退化为 CSS 柱条或 Ant Design 圆形进度。
- 图表数据分别读取资产持久化 metadata 中的 `traffic_profile/traffic_outbound/traffic_east_west/traffic_time_labels` 与 `protocols/protocol_total_throughput`；百分比总和必须闭合为 `100%`。
- 右栏遵循 UI 图的摘要、风险、关联数据与操作层级；关联数据只能显示真实资产字段、真实 API 能力和明确待接入边界。
- 主表在窄窗口使用横向滚动；右侧栏在 1200px 以下下沉，不把 11–12 列表格压入半宽面板。
- Drawer 最大约占视口 68%，保留宿主列表、顶部导航和关闭路径。
- 可见按钮必须对应真实刷新、筛选、分页、路由或 Drawer 状态变化。
- 未实现写动作不显示伪成功 toast；可以不显示，或以禁用态明确说明缺失契约。
- loading、error、empty、401/403 必须具有明确状态，不使用常量填充。
- 键盘焦点必须可见，表格行选择和详情关闭必须可操作。

## 双子代理审核门禁

每次资产台账实现必须执行两轮审核，主线程负责裁决：

1. 实现前：页面逻辑合理性子代理 + 页面布局合理性子代理。
2. 主线程记录接受、拒绝或延期的 finding，并决定自主开发范围。
3. 实现后：基于同批生产截图、API 结果和交互证据，由两名子代理复审。
4. 主线程只在 P0/P1 闭合、生产截图有效、交互可用后作通过裁决。

UI 图是视觉验收主基准，子代理意见是审查输入；发生业务逻辑冲突时，主线程必须结合项目逻辑和可验证证据裁决是否偏离 UI 图，并记录原因。

## 验收清单

- [x] 五类 Tab 使用真实 `asset_type` 查询
- [x] UUID 与 `display_code` 分离
- [x] 服务端筛选、分页和 total
- [x] `asset:read` 路由与后端读权限
- [x] 详情与历史真实 API
- [x] 数据库五类验收数据与历史事件
- [x] 本地测试、构建和在线 API 回归
- [ ] 网络接口、开放服务、归属信息真实详情契约与数据库数据
- [ ] 10 个页面/状态各自的 Windows Chrome 1920×1080 生产截图（Xshell CDP）
- [ ] 10 个页面/状态各自的 capture / diff / metrics / verification
- [ ] 10 个页面/状态业务 ROI 均 `<0.12`（旧 `assets=0.11824715562779357` 使用视觉快照参数，已撤销）
- [ ] 页面逻辑子代理对 10 页运行态复审
- [ ] 页面布局子代理对 10 页运行态复审
- [ ] 主线程 10/10 最终裁决与回修闭环
