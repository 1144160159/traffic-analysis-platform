# alert-detail-evidence-pcap.png 逐图精拆记录

## 基本信息

- page-id：`alert-detail-evidence-pcap`
- route：`/alerts`，验收状态 URL 使用 `/alerts/AL-20260620-000123?evidenceView=pcap`
- type：`menu-state`
- parent：`alerts`
- target UI：`doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail-evidence-pcap.png`
- 证据目录：`evidence/ui-image-breakdowns/pages/alert-detail-evidence-pcap/`
- 当前状态：`business-pixel-accepted`
- 生产镜像：`traffic/web-ui:ui-alert-detail-evidence-pcap-visual-20260709-r152`
- 最终验收 URL：`http://10.0.5.8:30180/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&__codex_page_id=alert-detail-evidence-pcap&evidenceView=pcap&__capture=r152-final&windowsCdpEvidenceTs=1783582819722`
- 视口：`1920 x 1080`

## 目标图观察

该图是告警详情证据链的 PCAP focus 状态，不显示 AppShell 顶栏、侧栏和底栏。顶部只有证据链标题与 6 个分类 Tab，`PCAP 1` 为选中态。主体为 8 列证据表格，一条 PCAP 证据行，下面展开对象路径和 SHA256 明细，底部提供“查看全部 PCAP 1 项”入口。

本页没有业务动态图示；表格、状态、对象路径和 SHA256 均由 React DOM/CSS 实现，业务数据来自 `fetchAlertDetailSnapshot(alertId)` 或 typed fallback，不允许使用目标截图替代表格/整卡。

## 区域与坐标

| 区域 | bbox | 说明 |
|---|---:|---|
| `canvas` | `0,0,1920,1080` | 画布；1920x1080 单屏证据链 focus 画布 |
| `focus-root` | `0,0,1920,1080` | 证据链 Focus 根节点；隐藏 AppShell 公共区后的证据链业务区域 |
| `outer-card` | `12,28,1897,1025` | 外层发光边框；承载全部 PCAP 证据链内容的主卡片 |
| `tabs-bar` | `14,30,1893,132` | 证据分类 Tab 区；标题与 6 个证据分类 Tab |
| `title` | `44,86,190,44` | 标题 证据链（6）；证据链总数标题 |
| `tab-all` | `270,72,159,90` | Tab 全部 6；全部证据计数 |
| `tab-pcap` | `429,72,178,90` | Tab PCAP 1；PCAP 分类计数 |
| `tab-session` | `607,72,186,90` | Tab Session 2；Session 分类计数 |
| `tab-log` | `793,72,150,90` | Tab 日志 1；日志分类计数 |
| `tab-graph-path` | `943,72,202,90` | Tab 图谱路径 1；图谱路径分类计数 |
| `tab-file` | `1145,72,145,90` | Tab 文件 1；文件分类计数 |
| `evidence-table` | `31,162,1859,865` | 证据表格容器；表头、PCAP 行、对象路径/SHA 详情和底部入口 |
| `table-head` | `33,164,1855,156` | 表头；8 列表头 |
| `pcap-row` | `33,320,1855,172` | PCAP 证据行；PCAP 文件记录与校验/下载/操作 |
| `detail-panel` | `33,492,1855,330` | 对象路径与 SHA 明细；PCAP 对象路径和 SHA256 校验值 |
| `footer-link` | `33,822,1855,203` | 查看全部 PCAP 入口；查看全部 PCAP 1 项 |
| `column-1` | `33,164,160,328` | 列：证据类型；证据类型 表头与数据列 |
| `column-2` | `193,164,333,328` | 列：文件 / 记录；文件 / 记录 表头与数据列 |
| `column-3` | `526,164,386,328` | 列：内容摘要；内容摘要 表头与数据列 |
| `column-4` | `912,164,158,328` | 列：大小；大小 表头与数据列 |
| `column-5` | `1070,164,184,328` | 列：生成时间；生成时间 表头与数据列 |
| `column-6` | `1254,164,226,328` | 列：校验状态；校验状态 表头与数据列 |
| `column-7` | `1480,164,242,328` | 列：下载审计；下载审计 表头与数据列 |
| `column-8` | `1722,164,166,328` | 列：操作；操作 表头与数据列 |
| `object-label` | `64,556,170,86` | 对象路径标签；对象路径字段标签 |
| `object-path` | `256,556,840,86` | 对象路径值；minio 对象路径 |
| `sha-label` | `64,672,170,86` | SHA256 标签；SHA256 字段标签 |
| `sha-value` | `256,672,382,86` | SHA256 值；SHA256 截断值 |

## 文本清单

```
证据链（6）
全部 6
PCAP 1
Session 2
日志 1
图谱路径 1
文件 1

证据类型 | 文件 / 记录 | 内容摘要 | 大小 | 生成时间 | 校验状态 | 下载审计 | 操作
PCAP | AL-20260620-000123.pcap | PCAP 切片，TLS over HTTP 隧道，疑似隧道通信 | 24.8 MB | 06-20 03:43:05 | 已生成 / SHA256通过 | sec_analyst 03:44 下载 | 下载 / 查看

对象路径  minio://traffic-evidence/alerts/2026/06/20/AL-20260620-000123.pcap
# SHA256  1a2b3c4d5bef79a8h9i0j...

查看全部 PCAP 1 项 >
```

## 组件清单

| 模块 | 实现 | 数据来源 | 刷新/自适应 |
|---|---|---|---|
| PCAP focus 根组件 | `web/ui/src/pages/AlertDetailPage.tsx::AlertEvidencePcapFocusView` | `fetchAlertDetailSnapshot(alertId)` / typed fallback | 正常模式 30s 刷新；visual mode 冻结以保证截图确定性 |
| PCAP 数据映射 | `web/ui/src/services/alertDetailApi.ts::pcapEvidenceFrom` | `pcap_evidence` / `pcapEvidence` / fallback | 字段映射到文件名、摘要、大小、生成时间、校验状态、下载审计、对象路径、SHA256 |
| 视觉布局 | `web/ui/src/styles/pages.css::.taf-alert-evidence-pcap-*` | DOM 数据驱动 | 固定列宽 grid；长文本 `title` 保留全文；无页面级溢出 |
| Focus 外壳 | `web/ui/src/styles/app-shell.css` | route state | 仅 focus evidence 状态隐藏公共区，不影响普通告警详情 |

## 图标清单

| 类型 | 本页结论 | 实现方式 |
|---|---|---|
| 业务动态图示 | 无 | 无 ECharts/canvas 业务图；`capture-meta` 记录 `canvas=0, echarts=0` |
| 独立图标 | 有 | AntD 图标：链路、校验、下载、查看、文件、复制、箭头 |
| 背景/装饰 | 有 | CSS 深色背景、描边和发光；不含业务页面截图资源 |

## 验收证据

- Windows Chrome CDP 预检：`cdp-version-r152-final-pre-capture.txt`、`cdp-list-r152-final-pre-capture.txt`
- 生产截图：`implementation-r152-final.png`，别名 `implementation.png`
- 运行时：`capture-meta-r152-final.json`，别名 `capture-meta.json`，status=`pass`
- diff：`diff-r152-final.png`，别名 `diff.png`
- metrics：status=`pass`，mismatch ratio=`0.072551`，阈值=`0.12`
- business crop：本页 focus 面板占满 1920x1080，`target-business-r152.png` / `implementation-business-r152.png` 与全图一致。

## 实现映射

通过。人工复看 `implementation.png`、`diff.png`、`metrics.json`、`capture-meta.json` 和生产 URL 后，确认本页无遮挡、无重叠、无越界；PCAP 文件名、摘要、校验状态、下载审计、对象路径、SHA256 和底部入口完整可见或具备全文 title；没有截图替代业务表格；证据文件齐全。状态建议：`business-pixel-accepted`。

## Token 与样式

| Token | 值 | 使用位置 | 说明 |
|---|---|---|---|
| `page-bg` | `#03111c` | 全画布 | 深色底色 |
| `panel-bg` | `rgba(6,28,43,0.86)` | 表格容器 | 内容可扫描 |
| `panel-strong-bg` | `#071f32` | 表头和明细框 | 层级强调 |
| `border-weak` | `rgba(56,151,201,0.22)` | 表格线 | 1px 分隔 |
| `active-blue` | `#1e9cff` | PCAP tab、链接 | 激活/操作语义 |
| `text-primary` | `#eaf7ff` | 标题、主字段 | 高对比 |
| `text-secondary` | `#9db9c9` | 标签、摘要 | 次级文字 |
| `success` | `#36d66b` | SHA256 通过 | 通过语义 |
| `info` | `#18a8ff` | 下载和查看 | 信息动作 |
| `panel-radius` | `6px` | 外框 | 控制圆角 |
| `control-radius` | `4px` | tab、值框 | 非胶囊形 |
| `panel-gap` | `8px` | 各控件间距 | 8px 网格 |

## 状态与交互

| 控件 | 触发 | 预期 | 验收重点 |
|---|---|---|---|
| PCAP tab | 点击 | PCAP 1 激活 | tab 位置不漂移 |
| 下载图标 | 点击 | 下载 PCAP | 写入下载审计 |
| 查看图标 | 点击 | 打开 PCAP 详情 | 保留告警上下文 |
| 对象路径复制 | 点击复制 | 复制 MinIO URI | 复制完整值 |
| SHA256 复制 | 点击复制 | 复制完整 hash | 状态仍显示通过 |
| 查看全部 | 点击底部入口 | 展开 PCAP 列表 | 保留告警 ID |

## 差异清单

| 项目 | 目标图 | 实现判定 | 结论 |
|---|---|---|---|
| tab | PCAP 1 激活 | 顺序、计数、状态一致 | 接受 |
| 行字段 | 八列一条记录 | 字段拆分完整 | 接受 |
| 校验 | 已生成/SHA256通过 | 成功状态明确 | 接受 |
| 审计 | sec_analyst 03:44 下载 | 下载动作可追溯 | 接受 |
| 对象路径 | MinIO URI | 可显示并复制全值 | 接受 |
| hash | 摘要显示 | 完整值保留在数据层 | 接受 |
| 未决项 | 无结构缺口 | diff 已低于门槛 | 无需阻断 |

## 结论

- 拆解覆盖 tab、八列表格、对象路径、SHA256、审计和操作入口。
- 文件、路径和 hash 来自告警证据 API；目标截图不允许作为生产资源。
- 下载必须可点击并产生审计记录，不能仅绘制图标。
- 本记录通过后仍需由像素与交互双门禁独立判断生产实现。

### 表格列逐项测量

| 列 | x 范围 | 宽度 | 可见内容 |
|---|---:|---:|---|
| 证据类型 | `33-193` | 160 | PCAP 图标与文字 |
| 文件 / 记录 | `193-526` | 333 | `AL-20260620-000123.pcap` |
| 内容摘要 | `526-912` | 386 | 三段业务摘要 |
| 大小 | `912-1070` | 158 | `24.8 MB` |
| 生成时间 | `1070-1254` | 184 | `06-20 03:43:05` |
| 校验状态 | `1254-1480` | 226 | 两行绿色状态 |
| 下载审计 | `1480-1722` | 242 | 用户、时间和动作 |
| 操作 | `1722-1888` | 166 | 下载与查看 |

### 可见字段逐项复核

| 序号 | 字段 | 判定 |
|---:|---|---|
| 1 | `证据链（6）` | 标题完整，括号为全角 |
| 2 | `全部 6` | 未激活 |
| 3 | `PCAP 1` | 激活蓝色背景 |
| 4 | `Session 2` | 未激活 |
| 5 | `日志 1` | 未激活 |
| 6 | `图谱路径 1` | 未激活 |
| 7 | `文件 1` | 未激活 |
| 8 | `PCAP` | 左侧绿色波形图标 |
| 9 | 文件名 | 蓝色链接态且不换行 |
| 10 | `PCAP 切片` | 摘要第一段 |
| 11 | `TLS over HTTP 隧道` | 摘要第二段 |
| 12 | `疑似隧道通信` | 摘要第三段 |
| 13 | `24.8 MB` | 单位与数值间有空格 |
| 14 | 生成时间 | 月日与时分秒完整 |
| 15 | `已生成` | 校验签第一行 |
| 16 | `SHA256通过` | 校验签第二行 |
| 17 | `sec_analyst` | 审计操作者 |
| 18 | `03:44` | 审计时间 |
| 19 | `下载` | 审计动作 |
| 20 | 对象路径 | 标签使用灰白色 |
| 21 | MinIO URI | 值使用蓝色并保留全文 |
| 22 | 路径复制 | 图标在值框右侧 |
| 23 | SHA256 标签 | 井号与文字同行 |
| 24 | hash 摘要 | 值框宽度小于路径值框 |
| 25 | hash 复制 | 图标在值框右侧 |
| 26 | 查看全部 | 蓝色入口位于底部左侧 |
| 27 | 右箭头 | 与入口文字保持间距 |
| 28 | 空白区 | 不填充无依据统计卡 |
| 29 | 表格线 | 表头与记录行边界清楚 |
| 30 | 外框 | 全屏业务面板无越界 |

### 数据真实性边界

- `AL-20260620-000123.pcap` 必须对应当前告警，不从 URL 文本临时拼造。
- `minio://traffic-evidence/...` 由 API 返回对象键转换，前端不得硬编码生产桶地址。
- SHA256 完整值保留在数据对象中，界面只允许视觉省略。
- 下载前校验签名 URL；不可用时按钮禁用并显示原因。
- 下载成功与失败都写审计，记录操作者、告警、证据和时间。
- 查看动作只打开证据详情，不隐式触发下载。
- API 空数组显示 PCAP 空态，不回填目标图中的示例记录。
- API 错误显示重试与 trace 信息，不把错误态伪装为成功状态。

### 可访问性与操作审计

- PCAP tab 使用可聚焦的 tab 语义，并暴露选中状态。
- 下载、查看和复制按钮都有中文 `aria-label` 与悬停提示。
- 键盘焦点环使用信息蓝，不能被外层 `overflow` 裁切。
- 状态签不仅依靠绿色，还同时展示 `已生成 / SHA256通过` 文本。
- 长文件名和对象路径提供完整 `title`，读屏顺序与视觉顺序一致。
- 下载失败时保留原记录并显示可重试反馈，不移除审计上下文。
- 复制成功采用短暂反馈，不弹出遮挡业务区的大型对话框。
- 查看详情使用窄 Drawer 或既有详情容器，父级证据列表保持可见。
- 所有操作事件携带 alert ID、evidence ID 和 trace ID。
- 未授权用户看到禁用或拒绝反馈，不允许前端假成功。
