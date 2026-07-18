# 资产台账 V3 页面工作包

更新日期：2026-07-13

## 当前裁决

- 不再生成新的资产台账 UI 图。
- 现有 UI 图是视觉层级、布局、密度和风格主基准；逻辑不合理时允许依据真实契约纠正，并记录偏离原因。
- 页面不得用静态常量填满空间，也不得保留大面积无业务意义空白。
- 页面逻辑合理性和页面布局合理性分别由子代理审核，主线程记录裁决并负责回修。
- 2026-07-13 复核撤销“资产台账阶段通过”结论：既有 `40/40` 只覆盖单路由交互断言，且把三个禁用详情页签当作通过；既有 ROI 只覆盖 `assets` 主态，不能证明 10 个目标态全部通过。

## 状态与身份

```text
/assets?tab=<endpoint|server|network-device|business-system|unknown>&assetId=<uuid>
/assets?tab=server&assetId=<uuid>&detail=<basic|history>
```

- UUID 是 API、历史、图谱和取证的规范身份。
- `END/SRV/NET/BIZ/UNK-*` 是 `display_code`，只用于展示。
- 详情预留页签 `network-interface/open-services/ownership` 在真实后端契约完成前禁用。

## 10 页严格验收矩阵

| 序号 | 页面/状态 | 当前真实实现 | 独立生产截图 | 独立 ROI `<0.12` | 当前裁决 |
|---|---|---|---|---|---|
| 1 | `assets`（终端） | 有 | 有 | 旧证据使用视觉快照参数，已撤销 | 未通过 |
| 2 | `assets-network-device` | 有 | 有 | 无 | 未通过 |
| 3 | `assets-server` | 有 | 有 | 无 | 未通过 |
| 4 | `assets-unknown` | 有 | 有 | 无 | 未通过 |
| 5 | `assets-business-system` | 有 | 有 | 无 | 未通过 |
| 6 | `assets-detail-basic` | 有 | 有 | 无 | 未通过 |
| 7 | `assets-detail-network-interface` | 页签被禁用 | 无 | 无 | 未通过 |
| 8 | `assets-detail-open-services` | 页签被禁用 | 无 | 无 | 未通过 |
| 9 | `assets-detail-ownership` | 页签被禁用 | 无 | 无 | 未通过 |
| 10 | `assets-detail-history` | 有 | 有 | 无 | 未通过 |

严格通过条件：每一行必须同时具备可达 URL 状态、真实 API/数据库数据、目标态关键交互、Windows Chrome 1920x1080 生产截图、对应同名 UI 图的业务区域 diff/metrics 且 ROI `<0.12`、逻辑与布局子代理复审、主线程裁决。禁用状态、同路由的其他截图或主态 ROI 均不得替代。

## 已完成范围

- 五类资产真实分类查询、服务端分页、关键词/状态/园区/部门筛选。
- 列表、详情、历史、发现相关 GET 接口统一 `asset:read` 与可信租户边界。
- 服务器基础详情和历史事件真实读取；静态接口、服务、归属与变更样例已移除。
- 图谱先按资产 UUID 读取真实资产 IP，再请求图谱；取证列表在后端按任务参数中的规范 `asset_id` 过滤，生产页面不复制模拟任务。
- PostgreSQL 新增展示编号、类型、状态和治理字段；五类各 8 条稳定验收数据、共 80 条历史事件。
- 生产镜像 `traffic/web-ui:asset-inventory-v3-20260713-r11`、`traffic/asset-service:asset-inventory-v3-20260713-r3`、`traffic/forensics-service:asset-scope-20260713-r1` 已发布并滚动完成。
- 右栏按 UI 图的“摘要、风险、关联数据、操作”层级铺满；参考态复刻四画像和证据带，生产态在聚合 API 未接入前明确显示“待接入”，不使用流量、漏洞或证据伪数据。

## 当前证据（仅代表已覆盖子集）

- Go：`go test ./internal/asset/...` 通过。
- Web：资产适配器、状态和路由权限 35 个定向测试通过，`npm run build` 通过。
- 临时 PostgreSQL 16：五类各 8 条、历史事件 80 条。
- 在线 PostgreSQL：五类各 8 条验收数据、历史事件 80 条。
- 在线 API：未鉴权 `401`；五类查询、服务器详情、历史均通过。
- Windows Chrome：经 Xshell 隧道 `127.0.0.1:9224` 连接 Windows 10 / Chrome 150，旧专用交互验收 `40/40` 通过；该结果不再作为 10/10 页面通过证据。
- 运行态布局：五类正常态业务表面覆盖率均为 `0.9431`，详情态 `0.9737`；无页面横向溢出、坏几何、错误提示或业务控制台错误。
- 证据：`doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-asset-interactions-latest.json` 与 `evidence/learning/asset-inventory/20260713-windows-xshell-acceptance-01/`。
- 业务 ROI：`assets-business-roi-v1 (198,78,1712,920)`，通道容差 `64`，最终差异比 `0.11824715562779357 < 0.12`，通过用户固定门槛；更细像素优化延期。
- 同屏证据：`evidence/learning/asset-inventory/20260713-ui-clone-roi-01/reference-vs-windows-r11.png`。
- 安全回归：视觉参数不再绕过登录；伪造 `X-Scopes/X-Tenant-ID` 的线上请求返回 `401`。

## 主线程裁决与后续优化

1. 布局子代理最终结论为 PASS；ROI 阈值严格按用户给定 `<0.12`，不在本轮自行收紧。
2. 逻辑子代理首轮发现取证后端未使用 `asset_id` 的 P1；主线程接受并完成后端过滤、前端去模拟和响应范围断言，最终证据为 `40/40`。
3. 旧逻辑/布局 PASS 仅适用于当时审查的已实现子集；主线程已撤销“资产台账 10 页阶段通过”结论。
4. 非阻断 P2 延期：CSV 增加规范 UUID/“当前页”提示、KPI 统一全局或本页口径、资产失活状态与历史事件事务化、取证正反样本与分页 total、旧任务适配器去样例、右栏标题编号分行与更细像素优化。
5. 先补齐三个详情数据契约与页面，再逐页生成 10 组独立视觉证据；达到 10/10 后才按页面队列推广到全系统强化学习型开发。
