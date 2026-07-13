# alert-detail-evidence-session.png 逐图精拆记录

## 基本信息

- page-id: `alert-detail-evidence-session`
- route: `/alerts`，生产验收状态 URL：`/alerts/AL-20260620-000123?evidenceView=session`
- type: `menu-state`
- parent: `alerts`
- target UI: `doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail-evidence-session.png`
- evidence dir: `evidence/ui-image-breakdowns/pages/alert-detail-evidence-session/`
- 当前状态：`business-pixel-accepted`
- 生产镜像：`traffic/web-ui:ui-alert-detail-evidence-session-popup-close-20260709-r160`
- 视口：1920 x 1080，Windows Chrome CDP `http://127.0.0.1:9224`

## 目标图观察

目标图内容是告警详情证据链的 `Session 2` 证据视图。按最新用户铁律，本页状态必须作为小弹窗处理：弹窗不占满 Windows 或浏览器窗口，右侧顶部必须有关闭叉号，点击后弹窗消失，父级告警详情页仍显示，左侧菜单继续选中 `威胁分析 / 告警中心`。

因此 r160 验收采用 `business-popup-crop-and-interaction`：目标 PNG 作为内容参照，最终通过以生产截图中的小弹窗区域、关闭按钮和关闭后截图为准。r155 中“近全屏焦点面板”的结论已废止。

## 区域与坐标

| 区域 | bbox | 说明 |
|---|---:|---|
| `canvas` | `0,0,1920,1080` | Windows Chrome CDP 实现截图 |
| `topbar` | `0,0,1920,72` | AppShell 顶栏保持可见 |
| `sidebar` | `0,72,171,933` | 左侧菜单保持可见，告警中心选中 |
| `bottombar` | `0,1005,1920,75` | 底部状态栏保持可见 |
| `alert-detail-underlay` | `178,72,1733,933` | 父级告警详情页，弹窗关闭后仍显示 |
| `popup-focus-host` | `171,72,1749,933` | 弹窗承载层位于业务区内 |
| `popup-card` | `469,214,1152,648` | Session 证据小弹窗，约占视口 60% x 60% |
| `popup-close` | `1575,229,31,31` | 右上角关闭叉号，`aria-label/title=关闭弹窗` |
| `tabs` | `471,216,1149,88` | 证据类型 Tab，`Session 2` active |
| `session-table` | `485,304,1121,539` | Session 证据表格，两条记录 |
| `session-flow` | `486,584,1118,128` | Session 事件链，React/CSS 数据驱动 |

## 文本清单

| 文本 | 类型 | 必须一致 |
|---|---|---|
| `告警详情` | parent-heading | 是 |
| `证据链（6）` | popup-heading | 是 |
| `关闭弹窗` | close-button-title | 是 |
| `全部 6` | tab | 是 |
| `PCAP 1` | tab | 是 |
| `Session 2` | tab-active | 是 |
| `日志 1` | tab | 是 |
| `图谱路径 1` | tab | 是 |
| `文件 1` | tab | 是 |
| `证据类型` / `Session ID` / `五元组` / `请求/响应摘要` / `字节数` / `持续时间` / `状态` / `操作` | table-header | 是 |
| `session-20260620-000123.json` | row-link | 是 |
| `172.16.5.10:443 -> 185.22.14.9:8443 / TCP` | tuple | 是 |
| `异常长连接，双向持续传输，SNI 缺失` | summary | 是 |
| `1.2 MB` / `12m 38s` / `已生成` | row | 是 |
| `session-20260620-000124.json` | row-link | 是 |
| `10.20.4.18:51514 -> 185.22.14.9:443 / TCP` | tuple | 是 |
| `周期心跳，每 30s 上行小包` | summary | 是 |
| `768 KB` / `08m 16s` / `已生成` | row | 是 |
| `03:31 建连` / `03:34 心跳` / `03:43 切片关联` | flow | 是 |
| `关联 PCAP: AL-20260620-000123.pcap` | flow-link | 是 |
| `查看全部 Session 2 项` | footer-link | 是 |

## 图标清单

| 类型 | 实现 | 数据来源 | 说明 |
|---|---|---|---|
| 业务动态图示 | React/CSS 事件链 | `AlertDetailSessionEvidence.timeline` typed API/fallback | `03:31 建连 -> 03:34 心跳 -> 03:43 切片关联 -> 关联 PCAP`，不是截图资源 |
| 表格 | CSS grid table | `sessionEvidence` rows | Session ID、五元组、摘要、字节、持续时间、状态、操作均来自 API adapter/fallback |
| 独立图标 | AntD icons | `actionKind` / UI 状态 | 关闭叉号、shield、reload/file、eye、link 是独立图标 |
| 背景 | CSS gradient/border/glow | 设计 token | 不包含业务动态图或目标 UI 截图 |

## 组件清单

- API 入口：`fetchAlertDetailSnapshot(alertId)`。
- 弹窗打开：`evidenceView=session` 进入 Session 证据状态时设置 `sessionEvidencePopupOpen=true`。
- 弹窗关闭：右上角 `CloseOutlined` 按钮调用 `onClose`，设置 `sessionEvidencePopupOpen=false`，弹窗卸载。
- 行过滤：`isSessionEvidence(row)` 选择 Session 证据。
- Session ID：`sessionEvidence.sessionId`。
- 五元组：`sessionEvidence.tupleLines[]`。
- 请求/响应摘要：`sessionEvidence.summaryLines[]`。
- 字节数/持续时间/状态：`bytes` / `duration` / `status`。
- 操作图标：`actionKind` 决定 reload 或 file，查看动作用 eye。
- 事件链：`timeline[].time` + `timeline[].label`，关联证据 `linkedPcap`。
- 刷新节奏：非视觉拆解模式下 React Query 30s refetch；视觉验收模式冻结刷新避免截图漂移。
- 自适应策略：弹窗卡片设置业务区内居中和最大 `1280 x 720`，同时受 `var(--taf-window-inner-width/height)` 约束，不随 Windows 或浏览器窗口撑满。

## 验收证据

- 测试：`npm --prefix web/ui test -- --run src/routes/noBitmapUi.test.ts src/routes/dataQualityTabs.test.ts src/services/alertDetailApi.test.ts` 通过，7 tests passed。
- 构建：`npm --prefix web/ui run build` 通过。
- 生产镜像：`traffic/web-ui:ui-alert-detail-evidence-session-popup-close-20260709-r160`。
- 生产 URL：`http://10.0.5.8:30180/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&__codex_page_id=alert-detail-evidence-session&evidenceView=session&__capture=r160-close-final&windowsCdpEvidenceTs=1783596176357`。
- runtime：无 console/pageerror/requestfailed/HTTP 4xx/5xx，禁用资源标记为空，无根溢出。
- 小弹窗门禁：`card_not_full_width=true`，`card_not_full_height=true`，`close_button_assertion.visible=true`，`top_right=true`。
- 关闭交互：`interaction.png` 证明点击叉号后弹窗消失，告警详情页仍可见。
- diff：`metrics.status=pass`，业务弹窗 crop mismatch `0.08514044281550069` <= `0.35`。
- 主线程判断：`business-pixel-accepted`。

## Token 与样式

| Token | 值 | 使用位置 | 约束 |
|---|---|---|---|
| `page-bg` | `#03111c` | 画布 | 深色背景 |
| `panel-bg` | `rgba(6,28,43,0.86)` | Session 面板 | 不使用截图填充 |
| `panel-strong-bg` | `#071f32` | 表头、时间轴节点 | 内容层级 |
| `border-weak` | `rgba(56,151,201,0.22)` | 表格线 | 1px 稳定分隔 |
| `active-blue` | `#1e9cff` | Session tab 和链接 | 激活语义 |
| `text-primary` | `#eaf7ff` | 标题、会话字段 | 高对比 |
| `text-secondary` | `#9db9c9` | 摘要与时间 | 次级层级 |
| `success` | `#36d66b` | 已生成、Session 图标 | 成功语义 |
| `info` | `#18a8ff` | 时间轴圆点、操作 | 信息语义 |
| `panel-radius` | `6px` | 外框 | 紧凑圆角 |
| `control-radius` | `4px` | tab 与时间节点 | 固定形态 |
| `panel-gap` | `8px` | 行列间距 | 8px 网格 |

## 状态与交互

| 控件 | 触发 | 预期状态 | 验收重点 |
|---|---|---|---|
| Session tab | 点击 | Session 2 激活 | 计数和位置稳定 |
| 第一行回放 | 点击时钟 | 打开会话回放 | 使用第一条 Session ID |
| 第二行记录 | 点击文档 | 打开会话记录 | 使用第二条 Session ID |
| 查看图标 | 点击眼睛 | 打开会话详情 | 不丢失告警上下文 |
| 关联 PCAP | 点击链接 | 跳转 PCAP 证据 | 文件 ID 精确 |
| 查看全部 | 点击底部入口 | 展开 Session 列表 | 保留当前告警 |

## 实现映射

| 目标对象 | 代码位置 | 数据字段 | 空态/错误态 |
|---|---|---|---|
| Session 面板 | `AlertDetailPage.tsx` / Session focus view | `evidenceRows` | 固定容器展示空态 |
| 会话字段 | `services/alertDetailApi.ts` | `sessionId/fiveTuple/summary` | 查询错误可见 |
| 指标字段 | `AlertDetailPage.tsx` | `bytes/duration/status` | 不使用固定伪值 |
| 时间轴 | `AlertDetailPage.tsx` | `timeline` | 无节点时显示无活动 |
| PCAP 关联 | `AlertDetailPage.tsx` | `relatedPcap` | 缺失时禁用链接 |
| 页面样式 | `styles/pages.css` | grid/flex | 长文本可查看全值 |

## 差异清单

| 项目 | 目标图事实 | 实现判定 | 结论 |
|---|---|---|---|
| tab | Session 2 激活 | 六类 tab 顺序一致 | 接受 |
| 数据行 | 两条 Session | 五元组和摘要完整 | 接受 |
| 状态 | 两行均已生成 | 成功色语义正确 | 接受 |
| 时间轴 | 03:31、03:34、03:43 | 三节点及关联 PCAP | 接受 |
| 操作 | 每行两种动作 | 图标语义按行区分 | 接受 |
| 宿主形态 | 目标为业务面板 | production focus route 对齐 | 接受 |
| 未决项 | 无结构性缺失 | 已有 runtime/diff 证据 | 无需阻断 |

## 结论

- 本记录覆盖两条 Session、五元组、摘要、字节、持续时间、状态、操作和时间轴。
- Session 与 PCAP 的关联必须基于 API ID，不能通过静态文本拼接。
- 可见按钮均需具备真实点击行为，并保留错误、空态和权限反馈。
- 拆解记录通过不替代生产视觉 diff 与交互回归。

### 两条 Session 对照

| 字段 | 第一条 | 第二条 |
|---|---|---|
| Session ID | `session-20260620-000123.json` | `session-20260620-000124.json` |
| 源端 | `172.16.5.10:443` | `10.20.4.18:51514` |
| 目的端 | `185.22.14.9:8443` | `185.22.14.9:443` |
| 协议 | `TCP` | `TCP` |
| 摘要首行 | `异常长连接，双向持续传输` | `周期心跳，每 30s 上行小包` |
| 摘要次行 | `SNI 缺失` | 无第二行 |
| 字节数 | `1.2 MB` | `768 KB` |
| 持续时间 | `12m 38s` | `08m 16s` |
| 状态 | `已生成` | `已生成` |
| 首个动作 | 会话回放 | 会话记录 |
| 第二动作 | 查看详情 | 查看详情 |

### 时间轴逐项复核

| 序号 | 时间 | 事件 | 视觉关系 |
|---:|---|---|---|
| 1 | `03:31` | 建连 | 蓝色圆点与实线节点框 |
| 2 | `03:34` | 心跳 | 与前节点用虚线相连 |
| 3 | `03:43` | 切片关联 | 与后续 PCAP 关联相连 |
| 4 | 关联 | `AL-20260620-000123.pcap` | 链接文字使用蓝色 |
| 5 | 连接线 | 三段水平虚线 | 不穿过节点文字 |

### 视觉对象逐项复核

| 序号 | 对象 | 判定 |
|---:|---|---|
| 1 | 标题 | `证据链（6）` 位于左上 |
| 2 | 全部 tab | 计数 6，未激活 |
| 3 | PCAP tab | 计数 1，未激活 |
| 4 | Session tab | 计数 2，蓝色激活 |
| 5 | 日志 tab | 计数 1，未激活 |
| 6 | 图谱路径 tab | 计数 1，未激活 |
| 7 | 文件 tab | 计数 1，未激活 |
| 8 | 表头 | 八列且垂直居中 |
| 9 | 第一行图标 | 绿色盾牌波形 |
| 10 | 第二行图标 | 与第一行同类同尺寸 |
| 11 | Session ID | 两行均为蓝色链接 |
| 12 | 五元组 | 地址、端口和协议不截断 |
| 13 | 摘要 | 第一行两段，第二行一段 |
| 14 | 字节 | 两种单位按原值展示 |
| 15 | 时长 | 第一条长于第二条 |
| 16 | 状态 | 两行均为绿色 |
| 17 | 操作分隔 | 两图标之间有竖线 |
| 18 | 时间轴 | 位于表格行下方独立区域 |
| 19 | 底部入口 | `查看全部 Session 2 项` |
| 20 | 外框 | 面板四边闭合且无溢出 |

### 数据与错误边界

- 两条记录按 API 返回顺序展示，不能按示例 ID 固定排序。
- 五元组由结构化源/目的地址、端口和协议格式化。
- 字节数使用统一格式化函数，保留 MB/KB 量级。
- 持续时间由毫秒或秒值换算，禁止把显示文字作为计算源。
- 时间轴事件与 Session ID 绑定，切换行后不得串线。
- 关联 PCAP 必须存在对应 evidence ID 后才可点击。
- 空数据展示 Session 空态；错误数据展示重试入口和 trace。
- 关闭操作只关闭局部面板，父级告警详情状态保持不变。
- 行级操作的焦点顺序按第一行到第二行排列，不能跳入隐藏控件。
- `已生成` 状态同时有文字与颜色，满足非颜色单一提示要求。
