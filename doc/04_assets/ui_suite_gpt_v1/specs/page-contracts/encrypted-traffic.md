# 加密流量 前端实现契约

## 基本信息

- ID：`encrypted-traffic`
- 路由：`/encrypted-traffic`
- 领域：`threat-analysis`
- React 页面：`EncryptedTrafficPage`
- Tab 目标图：
  - 总览：`doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic.png`
  - 指纹分析：`doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-fingerprint.png`
  - 隧道检测：`doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-tunnel-detection.png`
  - 外联画像：`doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-egress-profile.png`
  - 证据中心：`doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic-evidence-center.png`
- 查询 API：`/api/v1/encrypted-traffic/stats`、`/api/v1/encrypted-traffic/sessions`、`/api/v1/encrypted-traffic/ja3`、`/api/v1/encrypted-traffic/tunnels`、`/api/v1/encrypted-traffic/exfiltration`、`/api/v1/encrypted-traffic/evidence`
- 动作 API：`/api/v1/encrypted-traffic/egress-actions`、`/api/v1/encrypted-traffic/evidence-actions`

## 必须实现的业务层

- 加密流量总览
- 指纹分析
- 隧道检测
- 外联画像
- 证据提取

## Tab 同步开发约束

- 菜单页与 `overview`、`fingerprint`、`tunnel-detection`、`egress-profile`、`evidence-center` 五个 Tab 必须属于同一个开发、测试与验收批次。
- Tab 状态通过 `/encrypted-traffic?tab=<slug>` 表达；刷新和直接访问必须保持目标 Tab。
- 每个 Tab 必须有独立业务组件、独立目标图和独立截图证据，不能只替换标题或复用相同内容。
- API 为空时展示明确空态和数据源状态；禁止注入伪造会话、IP、JA3、PCAP、趋势或 KPI。
- 所有真实表格必须启用分页；查询结果行数变化时分页控件位置保持稳定。
- 总览业务区域由 `.taf-encrypted-grid` 统一承担纵向滚动；在 1920x1080 验收视口必须存在正向滚动范围并显示滚动条，左右业务列随同滚动且任何模块不得被覆盖。
- 总览 JA3 分布必须使用 ECharts `scatter` 系列渲染，禁止以 CSS 点阵或静态图片替代。
- 外联画像的目的地地图是首要视觉区域；在 1920x1080 验收视口地图面板必须为“纵向更高、横向更窄”的 860–940 px 宽、至少 510 px 高，且不得挤压、裁切右侧闭环操作区。
- 外联画像目的地地图的 `WorkPanel -> panel body -> EgressProfile -> .taf-encrypted-map -> ECharts canvas` 高度链必须连续；地图画布需铺满可用内容区，园区出口位于东亚区域并向真实目的地区域绘制流向，禁止退化为顶部固定高度条带。
- 总览右栏“处置与分析建议”和“生成与导出”必须具有独立非折叠轨道，正文 `scrollHeight` 不得大于 `clientHeight`；超出首屏的右栏内容由 `.taf-encrypted-grid` 统一滚动到达，不能只显示面板标题。
- 总览“证据与握手元数据”、总览右栏“外联画像”目的地表和隧道检测“隧道异常列表”的分页栏必须位于最后一行下方及表格边界内；证据中心“加密会话证据表”不得生成纵向滚动体，翻页时分页栏坐标变化不得超过 2 px。
- PCAP 回放使用 `/home/wangwt/task/datasets` 的只读样本；先通过清单和离线解析门禁，再进入 Kafka/Flink/ClickHouse/API/UI 链路。

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

- `drawer-encrypted-fingerprint`：加密指纹详情，Drawer
- `drawer-certificate-detail`：证书详情，Drawer

## 验收清单

- [x] 最终 PNG 必须为 1920x1080
- [x] 中文为主，只保留必要英文技术词和单位
- [x] 状态色必须遵守 success/info/warning/danger/critical token
- [x] 危险动作必须具备影响范围、权限提示和审计留痕
- [x] 公共 AppShell 使用项目统一生产壳层，五张 UI 原图直接约束业务区域
- [x] 页面主工作区不得复用相邻页面的业务组件组合
- [x] 所有 API 调用必须经 services/api.ts 或现有服务封装
- [x] React Query 必须覆盖 loading/error/empty 状态
- [x] 五个 Tab 均有独立 1920x1080 截图与交互证据
- [x] JA3 数据来自 `traffic.feature_fp`，表不存在时明确返回 `unavailable`
- [x] 空接口不会触发任何仿真数据回退
- [x] PCAP 数据集清单与回放结果已写入 `doc/02_acceptance/02-regression/`
- [x] 所有表格均有分页；翻页前后分页栏内容坐标变化不超过 2px
- [x] 总览业务区滚动范围、可见滚动条与滚动后末模块可达性均通过 Windows Chrome 验收
- [x] JA3 散点图为 ECharts `scatter`；外联画像目的地地图面板实测 913.2x520.0 px，地图内容区 899.7x478.5 px、Canvas 898.0x477.0 px，完整铺满面板正文
- [x] 总览“处置与分析建议”正文 134.5 px（scroll/client 均 135 px）、“生成与导出”正文 84.5 px（scroll/client 均 85 px），均无内容覆盖
- [x] 总览证据表、总览外联目的地表、隧道异常列表和加密会话证据表均通过“分页不覆盖数据行”几何门；会话证据表无纵向滚动体
- [x] Windows Chrome 五页签验收 61/61，五张 UI 原图视觉门 5/5
