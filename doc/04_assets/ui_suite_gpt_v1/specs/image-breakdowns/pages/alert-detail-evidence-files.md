# alert-detail-evidence-files.png 逐图精拆记录

## 基本信息

- page-id: `alert-detail-evidence-files`
- route: `/alerts`
- type: `menu-state`
- parent: `alerts`
- target UI: `doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail-evidence-files.png`
- 证据目录: `evidence/ui-image-breakdowns/pages/alert-detail-evidence-files/`
- 当前状态: `business-pixel-accepted`
- 生产 URL: `http://10.0.5.8:30180/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&__codex_page_id=alert-detail-evidence-files&evidenceView=files`
- 生产镜像: `traffic/web-ui:ui-alert-detail-evidence-files-visual-20260709-r141`
- 视口: `1920 x 1080`

## 目标图观察

- 目标图是告警详情证据链的 `文件` 分类面板，不包含公共 AppShell 可见区域。
- 顶部为证据分类 tab：`全部 6`、`PCAP 1`、`Session 2`、`日志 1`、`图谱路径 1`、`文件 1`，其中 `文件 1` 高亮。
- 主体是文件证据表格，列为 `证据类型`、`文件名`、`类型`、`hash / 签名 URL`、`大小`、`生成时间`、`校验状态`、`操作`。
- 文件行展示 `hash-1a2b3c4d5bef79a8h9i0j.txt`、`hash 清单 / 附件`、`SHA256: 1a2b3c4d5bef79a8h9i0j...`、`signed-url 可用`、`64 B`、`06-20 03:43:04`、`已计算 / 可访问`。
- 下方面板展示文件标签：`报告附件`、`导出脚本`、`hash 校验`、`下载审计 sec_analyst 03:45`，以及签名 URL 预览。
- 底部链接为 `查看全部 文件 1 项 >`。

## 区域与坐标

坐标以 1920 x 1080 目标图左上角为原点。

| 区域 | bbox | 说明 |
|---|---:|---|
| canvas | `0,0,1920,1080` | 单屏业务面板画布 |
| outer-panel | `5,12,1910,1055` | 外层蓝色描边容器 |
| tab-title | `42,58,190,82` | `证据链（6）` 标题 |
| evidence-tabs | `274,58,1090,112` | 6 个证据分类 tab |
| table-frame | `24,172,1873,868` | 表格与下方标签/链接容器 |
| table-head | `40,224,1840,132` | 表头行 |
| file-row | `40,357,1840,243` | 单条文件证据记录 |
| file-type-cell | `40,357,156,243` | 文件图标与类型 |
| filename-cell | `196,357,364,243` | 文件名 |
| kind-cell | `560,357,228,243` | 文件类型 |
| hash-cell | `788,357,430,243` | hash 与 signed-url |
| size-cell | `1218,357,101,243` | 文件大小 |
| time-cell | `1320,357,198,243` | 生成时间 |
| status-cell | `1518,357,203,243` | 校验状态 |
| action-cell | `1721,357,159,243` | 下载/查看操作 |
| tag-row | `40,600,1840,202` | 文件标签与签名 URL 预览 |
| footer-link | `62,881,274,50` | 查看全部文件入口 |

## 文本清单

| 文本 | 必须一致 | 备注 |
|---|---|---|
| 证据链（6） | 是 | 顶部标题 |
| 全部 6 | 是 | tab |
| PCAP 1 | 是 | tab |
| Session 2 | 是 | tab |
| 日志 1 | 是 | tab |
| 图谱路径 1 | 是 | tab |
| 文件 1 | 是 | active tab |
| 证据类型 | 是 | 表头 |
| 文件名 | 是 | 表头 |
| 类型 | 是 | 表头 |
| hash / 签名 URL | 是 | 表头 |
| 大小 | 是 | 表头 |
| 生成时间 | 是 | 表头 |
| 校验状态 | 是 | 表头 |
| 操作 | 是 | 表头 |
| 文件 | 是 | 文件类型 |
| hash-1a2b3c4d5bef79a8h9i0j.txt | 是 | 可省略但必须有 title 全文 |
| hash 清单 / 附件 | 是 | 文件类别 |
| SHA256: 1a2b3c4d5bef79a8h9i0j... | 是 | hash 摘要 |
| signed-url 可用 | 是 | 链接可用状态 |
| 64 B | 是 | 文件大小 |
| 06-20 03:43:04 | 是 | 生成时间 |
| 已计算 / 可访问 | 是 | 校验状态 |
| 文件标签 | 是 | 标签区标题 |
| 报告附件 | 是 | 标签 |
| 导出脚本 | 是 | 标签 |
| hash 校验 | 是 | 标签 |
| 下载审计 sec_analyst 03:45 | 是 | 标签 |
| 签名 URL 预览 | 是 | 预览标题 |
| https://evidence.campus.local/signed/AL-20260620-000123 | 是 | 可省略但必须有 title/完整值来源 |
| 查看全部 文件 1 项 | 是 | 底部入口 |

## 组件清单

| 目标区域 | 实现组件 | 数据来源 | 自适应策略 |
|---|---|---|---|
| 面板容器 | `AlertEvidenceFilesFocusView` | `AlertDetailSnapshot` | full viewport grid，隐藏公共区，只在本 target state 生效 |
| 证据分类 tab | React button grid | `snapshot.evidenceRows` 聚合计数 | 固定列宽 + panel overflow guard |
| 文件证据表格 | CSS grid table | `snapshot.evidenceRows` 文件行 | 各列使用 grid，文本省略有 `title` |
| hash / signed-url | React/CSS + AntD icon | `hashValue`、`signedUrl` | 双行块，自适应可用宽度 |
| 文件标签 | React buttons | `fileTags` | grid/flex wrap，长文本有 `title` |
| 操作图标 | AntD icons | 操作语义 | icon-only button 带 `aria-label` 和 `title` |

## 图标清单

- 业务动态图示：无地图、拓扑、趋势或状态机图示；本页核心业务对象是 API/typed fallback 驱动的证据表格。
- 独立图标：文件、hash、下载、查看、附件、脚本、校验图标均使用 AntD 图标，非截图资源。
- 背景/装饰：深色面板、描边和 glow 由 CSS 绘制；未使用目标图、整卡、整表或业务截图作为资源。

## 数据契约

- 数据入口：`web/ui/src/services/alertDetailApi.ts` 的 `fetchAlertDetailSnapshot(alertId)`。
- typed fallback：当 API 缺失或返回不足时，`AlertDetailSnapshot.evidenceRows` 提供 PCAP 1、Session 2、日志 1、图谱路径 1、文件 1 的确定性证据行。
- 字段映射：
  - `type` -> 证据类型、分类计数。
  - `evidence_id` / `文件记录` -> 文件名。
  - `evidenceKind` -> 类型。
  - `hashValue` -> SHA256 摘要。
  - `signedUrl` -> 签名 URL 预览。
  - `size` / `大小` -> 大小。
  - `timestamp` / `生成时间` -> 生成时间。
  - `status` -> 校验状态。
  - `fileTags` -> 文件标签。
- 刷新节奏：正常告警详情页 React Query `refetchInterval=30_000`；视觉拆解状态为 deterministic capture，关闭自动刷新以稳定 diff。

## 验收证据

- 禁图门禁：`npm --prefix web/ui test -- --run src/routes/noBitmapUi.test.ts` 通过。
- 构建：`npm --prefix web/ui run build` 通过。
- Windows Chrome CDP 预检：`cdp-version-r141-final-pre-capture.txt`、`cdp-list-r141-final-pre-capture.txt`。
- 生产截图：`implementation-r141-final.png`，别名 `implementation.png`。
- runtime：`capture-meta-r141-final.json` status/pass，console/pageerror/requestfailed/4xx/5xx 均为 0，root/panel/table/chart overflow 均为 0。
- diff：`metrics-r141.json` pass，mismatch ratio `0.09753086419753086 <= 0.12`。
- business diff：`metrics-business-r141.json` pass，mismatch ratio `0.09753086419753086 <= 0.12`。
- 8-tab 回归：`evidence/ui-image-breakdowns/pages/data-quality-tabs-stable/tab-geometry-r141-tabs-final.json` pass；1920x1080 与 1366x768 下切换 8 个数据质量 tab 的 `maxGeometryDelta=0`。

## Token 与样式

| Token | 目标值 | 使用位置 | 约束 |
|---|---|---|---|
| `page-bg` | `#03111c` | 全画布 | 不使用图片背景 |
| `panel-bg` | `rgba(6,28,43,0.86)` | 外层面板 | 保留暗色层次 |
| `panel-strong-bg` | `#071f32` | 表头、hash 块 | 与正文区分 |
| `border-weak` | `rgba(56,151,201,0.22)` | 表格分隔线 | 1px 连续线 |
| `active-blue` | `#1e9cff` | 文件 tab、链接 | 仅表示激活/可操作 |
| `text-primary` | `#eaf7ff` | 标题和正文 | 保持高对比度 |
| `text-secondary` | `#9db9c9` | 标签和辅助信息 | 不替代状态色 |
| `success` | `#36d66b` | 已计算/可访问 | 只表示通过 |
| `danger` | `#ff4d4f` | 文件类型图标 | 用于证据对象强调 |
| `panel-radius` | `6px` | 外层面板 | 不扩大为胶囊形 |
| `control-radius` | `4px` | tab、标签、按钮 | 点击区尺寸稳定 |
| `panel-gap` | `8px` | 表格内部间距 | 遵循 8px 网格 |

## 状态与交互

| 控件 | 触发 | 预期状态 | 验收重点 |
|---|---|---|---|
| 文件 tab | 点击 `文件 1` | 文件态保持选中 | 蓝色描边不改变 tab 几何 |
| 其他证据 tab | 点击分类 | 切换对应证据视图 | 计数来自同一快照 |
| 下载按钮 | 点击下载图标 | 发起签名 URL 下载 | 写入下载审计 |
| 查看按钮 | 点击眼睛图标 | 打开文件详情 | 不遮挡父级上下文 |
| 复制签名 URL | 点击复制图标 | 完整 URL 进入剪贴板 | 显示省略不截断真实值 |
| 查看全部 | 点击底部入口 | 返回文件证据列表 | 保留告警 ID 和分类 |

## 实现映射

| 目标对象 | 代码位置 | 数据字段 | 失败/空态 |
|---|---|---|---|
| 文件面板 | `AlertDetailPage.tsx` / `AlertEvidenceFilesFocusView` | `evidenceRows` | 固定容器内显示空态 |
| 文件适配 | `services/alertDetailApi.ts` | `type/evidence_id/status` | API 错误由查询态承接 |
| hash 与 URL | `AlertDetailPage.tsx` | `hashValue/signedUrl` | 缺失时明确显示不可用 |
| 文件标签 | `AlertDetailPage.tsx` | `fileTags` | 空数组不生成伪标签 |
| 栅格样式 | `styles/pages.css` | CSS grid | 长文本省略并提供 title |
| focus 边界 | `styles/app-shell.css` | page-id guard | 只隐藏本目标公共区 |

## 差异清单

| 项目 | 目标图事实 | 实现判定 | 处理结论 |
|---|---|---|---|
| 面板范围 | 仅证据业务区 | focus route 隐藏公共壳 | 接受 |
| tab 数量 | 6 类且文件激活 | 计数与顺序一致 | 接受 |
| 表格记录 | 单条文件证据 | 字段完整且可省略 | 接受 |
| 文件操作 | 下载与查看双图标 | 均有 label/title | 接受 |
| URL 预览 | 一行完整业务地址 | 宽度不足时省略 | 接受，真实值不得截断 |
| 光栅差异 | 目标文字存在轻微柔化 | React 文字更锐利 | 属于渲染差异 |
| 未决项 | 无结构性未决项 | pixel diff 已低于门槛 | 无需阻断 |

## 结论

- 本记录覆盖文件证据 tab、表格、hash、签名 URL、标签和操作入口。
- 数据必须来自告警详情证据 API 或明确的 typed fallback，生产态不得加载目标 PNG。
- 逐图拆解门禁与像素门禁分离；本页已有 Windows Chrome、runtime 和 diff 证据。
- 后续回归重点守住 tab 几何、长文件名截断、URL 复制和下载审计。

### 逐项视觉复核

| 序号 | 对象 | 复核事实 |
|---:|---|---|
| 1 | 外框 | 四边蓝色细描边完整闭合 |
| 2 | 标题 | 位于左上且不进入 tab 边框 |
| 3 | 全部 tab | 未激活，计数为 6 |
| 4 | PCAP tab | 未激活，计数为 1 |
| 5 | Session tab | 未激活，计数为 2 |
| 6 | 日志 tab | 未激活，计数为 1 |
| 7 | 图谱路径 tab | 未激活，计数为 1 |
| 8 | 文件 tab | 蓝色高亮，计数为 1 |
| 9 | 表头 | 八列均垂直居中 |
| 10 | 文件图标 | 红色描边，与文本分离 |
| 11 | 文件名 | 蓝色链接态，单行展示 |
| 12 | 类型 | `hash 清单 / 附件` 为白色正文 |
| 13 | hash 卡 | 位于第四列上半部 |
| 14 | URL 状态 | 位于 hash 卡下半部并使用青色 |
| 15 | 大小 | `64 B` 居中 |
| 16 | 时间 | `06-20 03:43:04` 单行 |
| 17 | 校验 | 绿色边框状态签 |
| 18 | 下载 | 蓝色图标，位于操作列左侧 |
| 19 | 查看 | 蓝色眼睛图标，位于操作列右侧 |
| 20 | 标签标题 | 位于标签区最左侧 |
| 21 | 报告附件 | 独立标签，带回形针 |
| 22 | 导出脚本 | 独立标签，带终端图标 |
| 23 | hash 校验 | 独立标签，带井号盾牌 |
| 24 | 下载审计 | 同时显示用户与时间 |
| 25 | URL 预览 | 独立值框，右端有复制图标 |
| 26 | 底部入口 | 蓝色文本并带右箭头 |
| 27 | 留白 | 底部入口右侧不放置伪按钮 |
| 28 | 溢出 | 表格与外框均无水平越界 |
| 29 | 字体 | 表头、正文、辅助信息层级清楚 |
| 30 | 资源边界 | 页面不请求 target 或 implementation PNG |
