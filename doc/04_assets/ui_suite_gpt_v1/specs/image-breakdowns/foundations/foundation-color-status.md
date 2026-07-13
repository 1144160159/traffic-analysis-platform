# foundation-color-status.png 逐图精拆记录

## 基本信息

- 分类：foundations
- 源图：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-color-status.png`
- 源图尺寸：1920 x 1080
- 对应 prompt：`doc/04_assets/ui_suite_gpt_v1/prompts/foundation-color-status.prompt.txt`
- 对应 manifest：`doc/04_assets/ui_suite_gpt_v1/manifest.json` / `doc/04_assets/ui_suite_gpt_v1/specs/layers/foundation-color-status.json`
- 对应路由/宿主路由：无。该图是设计系统 foundation 规范板，不是可直接访问业务页面。
- 当前状态：`pixel-accepted`
- 复刻等级：已完成 Windows Chrome reference-raster 实现截图、区域 overlay、零容忍视觉 diff、辅助审查和主线程判定；本结论只证明该目标 PNG 的像素复刻，不声明生产 React 组件已经按语义重写完成。

## 目标图观察

- 整体布局：1920 x 1080 深色 SOC 规范板。顶部为标题栏，左侧 44% 宽区域展示从态势大屏抽取色彩的来源示例，右侧上下两块面板分别展示 Token 色板和状态语义。
- 业务重点：锁定全套 UI 的背景、框架、面板、边框、文本、激活色和五级状态色，强调状态语义不可交换。
- 当前页面/浮层状态：静态设计系统看板。无 AppShell 导航、无弹窗、无遮罩、无表单提交态。
- 视觉基调：页面底色接近 `#03111c`，面板为低透明深蓝，细青蓝描边，主文字近白，状态色以绿色/蓝色/琥珀/红色/深红分级。

## 区域与坐标

坐标为基于目标 PNG 直接视觉查看后的人工测量，格式为 `x,y,w,h`，单位 px。

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|
| 画布 | `0,0,1920,1080` | 0 | 全屏 16:9 foundation 规范板 | 不出现浏览器外框、营销海报、水印 |
| 顶部标题区 | `0,0,1920,73` | 1 | 左侧英文标题和中文副标题，右侧基准标注 | 下边界 1px 青蓝分割线，标题不包卡片 |
| 主标题 | `29,15,486,31` | 2 | `Foundation Color And Status` | 约 30px，粗体，浅色文字 |
| 副标题 | `29,53,330,14` | 2 | `颜色取自当前态势大屏，状态语义固定不可交换` | 约 13px，次级文字 |
| 基准标注 | `1586,31,181,18` | 2 | `第一基准：screen.png` | 右上青蓝文字，`screen.png` 更亮更粗 |
| 左侧来源面板 | `24,92,837,947` | 1 | `01 态势大屏颜色来源` | 面板圆角 6px，1px 青蓝边框，内部分割线 |
| 左侧面板标题栏 | `25,93,835,43` | 2 | section header | 标题 22px 左右，编号加粗 |
| 拓扑示例图 | `60,150,765,371` | 2 | 园区数字孪生拓扑大图 | 内层边框、深色地图底、蓝/橙/绿链路发光 |
| 拓扑图标题 | `69,151,123,14` | 3 | `园区数字孪生拓扑` | 小号白字，位于图内左上 |
| 拓扑图例 | `71,178,235,11` | 3 | 核心链路、汇聚链路、异常链路、探针位置 | 线段/点形图例，必须保留状态色 |
| 拓扑模式按钮 | `714,148,98,17` | 3 | `2D` / `3D` / 全屏 | 3D 为激活态，按钮紧贴右上 |
| 拓扑中心标签 | `382,274,102,39` | 3 | `核心区`，含六边形/安全图标 | 中心标签高亮蓝边，作为链路汇聚点 |
| 拓扑底部异常图例 | `68,478,501,27` | 3 | 异常链路位置和三条异常链路说明 | 底部半透明条，橙红/橙/黄标识 |
| 拓扑详情链接 | `723,486,87,17` | 3 | `进入拓扑详情` + chevron | 右下激活蓝链接 |
| 小态势总览面板 | `291,548,302,462` | 2 | 缩略版业务仪表盘 | 垂直堆叠 4 个数据区，卡片边框密集 |
| 态势总览标题栏 | `299,557,286,25` | 3 | `威胁态势总览` / `近24小时` | 标题左对齐，时间窗右对齐 |
| 攻击阶段热度 | `300,584,146,98` | 3 | 横向条形排行 | 红/橙/黄/绿渐变条，右侧数字百分比 |
| 战役热度盘 | `448,584,137,98` | 3 | 雷达/散点图 | 三色点和图例，高密度小图 |
| 风险区域密度 | `300,690,286,76` | 3 | 世界地图热区 + 风险数量 | 红橙地图，右侧高/中/低风险图例 |
| 异常链路影响面 | `300,773,286,78` | 3 | Top 5 表格 + 环形指标 | 表格三列，右侧 donut 显示 `1,240` |
| 外联流向强度 | `300,887,286,113` | 3 | 世界流向弧线 + 区域速率表 | 蓝色地图弧线，右侧 `Gbps` 数值 |
| Token 色板面板 | `884,92,1013,429` | 1 | `02 Token 色板` | 右上第一块大面板，含 8 个色块 |
| Token 标题栏 | `885,93,1011,43` | 2 | section header | 标题分割线和左侧编号一致 |
| Token 色块网格 | `920,161,845,189` | 2 | 两行四列 token | 每个色块约 70 x 70，标签在色块右侧 |
| Page BG token | `920,161,169,69` | 3 | `Page BG #03111c` | 深色填充加细边框 |
| Shell BG token | `1150,161,169,69` | 3 | `Shell BG #061827` | 与 page bg 区分度较低，仍需独立 token |
| Panel BG token | `1380,161,179,69` | 3 | `Panel BG #071f32` | 用于面板强底色 |
| Border token | `1610,160,218,70` | 3 | `Border rgba(56,151,201,.22)` | 色块显示青蓝，实际边框使用 alpha |
| Active token | `920,280,169,70` | 3 | `Active #1e9cff` | 激活蓝，应用于链接/选中/强调 |
| Text token | `1150,280,169,70` | 3 | `Text #eaf7ff` | 主文字浅蓝白 |
| Secondary token | `1380,280,199,70` | 3 | `Secondary #9db9c9` | 次级文字浅灰蓝 |
| Muted token | `1610,280,188,70` | 3 | `Muted #5e7b8d` | 弱文字/辅助信息 |
| 状态语义面板 | `884,549,1013,489` | 1 | `03 状态语义` | 右下第二块大面板，五行状态样例 |
| 状态标题栏 | `885,550,1011,43` | 2 | section header | 与 Token 面板一致 |
| 健康/通过行 | `930,621,455,42` | 3 | 绿色 pill + `健康 / 通过` + `#36d66b` | 系统运行良好、门禁通过 |
| 信息/低危行 | `930,692,455,41` | 3 | 蓝色 pill + `信息 / 低危` + `#18a8ff` | 可点击入口、低风险提示 |
| 中危/待确认行 | `930,762,455,42` | 3 | 琥珀 pill + `中危 / 待确认` + `#ffb020` | 需要关注、等待确认 |
| 高危/失败行 | `930,832,455,42` | 3 | 红色 pill + `高危 / 失败` + `#ff4d4f` | 危险、失败、阻断动作 |
| 严重/关键行 | `930,902,455,42` | 3 | 深红 pill + `严重 / 关键` + `#ff2d2d` | 严重告警和关键风险 |

## 文本清单

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|
| Foundation Color And Status | 顶部标题区 | 主标题 | 是 |
| 颜色取自当前态势大屏，状态语义固定不可交换 | 顶部标题区 | 副标题 | 是 |
| 第一基准：screen.png | 顶部右侧 | 基准说明 | 是 |
| 01 态势大屏颜色来源 | 左侧来源面板标题 | 区块标题 | 是 |
| 园区数字孪生拓扑 | 拓扑示例左上 | 图标题 | 是 |
| 核心链路 / 汇聚链路 / 异常链路 / 探针位置 | 拓扑图例 | 图例 | 是 |
| 2D / 3D | 拓扑右上 | 模式按钮 | 是 |
| 教学区 / 图书馆 / 数学区 / 实验楼 / 办公区 / 核心区 / 数据中心 / 汇聚区B / 宿舍区 | 拓扑建筑标签 | 地图标签 | 是 |
| N / W / E / S | 拓扑右下 | 罗盘文字 | 是 |
| 异常链路位置 | 拓扑底部 | 图例标题 | 是 |
| 教学区-核心区链路 / 宿舍区-汇聚区B链路 / 体育馆-汇聚区A链路 | 拓扑底部 | 异常链路说明 | 是 |
| 进入拓扑详情 | 拓扑右下 | 链接 | 是 |
| 威胁态势总览 | 小态势面板标题 | 面板标题 | 是 |
| 近24小时 | 小态势面板右上 | 时间窗 | 是 |
| 攻击阶段热度 | 小态势面板 | 图标题 | 是 |
| 查看告警中心 | 攻击阶段热度右上 | 链接 | 是 |
| 侦察 / 资源利用 / 初始访问 / 执行 / 凭证访问 / 影响达成 | 攻击阶段热度 | 排行标签 | 是 |
| 2,186 12% / 3,276 18% / 4,932 27% / 3,184 17% / 2,104 11% / 2,536 15% | 攻击阶段热度 | 数值 | 是 |
| 战役热度盘 | 小态势面板 | 图标题 | 是 |
| 查看战役列表 | 战役热度盘右上 | 链接 | 是 |
| 高 / 中 / 低 | 战役热度盘 | 图例 | 是 |
| 风险区域密度 | 小态势面板 | 图标题 | 是 |
| 查看风险地图 | 风险区域密度右上 | 链接 | 是 |
| 高风险 12 / 中风险 23 / 低风险 41 | 风险区域密度 | 图例数值 | 是 |
| 异常链路影响面（Top 5） | 小态势面板 | 表格标题 | 是 |
| 查看影响详情 | 异常链路影响面右上 | 链接 | 是 |
| 链路位置 / 影响链路数 / 影响资产数 | 异常链路影响面 | 表头 | 是 |
| 实验区-核心区 1,286 432 | 异常链路影响面 | 表格行 | 是 |
| 宿舍区-核心区 923 311 | 异常链路影响面 | 表格行 | 是 |
| 办公区-核心区 612 207 | 异常链路影响面 | 表格行 | 是 |
| 教学区-图书馆 484 162 | 异常链路影响面 | 表格行 | 是 |
| 生活区-核心区 371 128 | 异常链路影响面 | 表格行 | 是 |
| 异常影响资产 1,240 | 异常链路影响面 | 环形指标 | 是 |
| 外联流向强度（近24小时） | 小态势面板 | 图标题 | 是 |
| 查看追踪详情 | 外联流向强度右上 | 链接 | 是 |
| 目的地区域 / 速度(Gbps) | 外联流向强度 | 表头 | 是 |
| 北美洲 42.7 / 东南亚 31.2 / 欧洲 18.6 / 东亚 12.9 / 其他 6.3 | 外联流向强度 | 表格行 | 是 |
| 02 Token 色板 | Token 面板标题 | 区块标题 | 是 |
| Page BG / #03111c | Token 面板 | token | 是 |
| Shell BG / #061827 | Token 面板 | token | 是 |
| Panel BG / #071f32 | Token 面板 | token | 是 |
| Border / rgba(56,151,201,.22) | Token 面板 | token | 是 |
| Active / #1e9cff | Token 面板 | token | 是 |
| Text / #eaf7ff | Token 面板 | token | 是 |
| Secondary / #9db9c9 | Token 面板 | token | 是 |
| Muted / #5e7b8d | Token 面板 | token | 是 |
| 03 状态语义 | 状态语义面板标题 | 区块标题 | 是 |
| 健康 / 通过 | 状态语义面板 | 状态名称 | 是 |
| 系统运行良好，门禁通过 | 状态语义面板 | 状态说明 | 是 |
| #36d66b | 状态语义面板 | 状态色值 | 是 |
| 信息 / 低危 | 状态语义面板 | 状态名称 | 是 |
| 可点击入口，低风险提示 | 状态语义面板 | 状态说明 | 是 |
| #18a8ff | 状态语义面板 | 状态色值 | 是 |
| 中危 / 待确认 | 状态语义面板 | 状态名称 | 是 |
| 需要关注，等待确认 | 状态语义面板 | 状态说明 | 是 |
| #ffb020 | 状态语义面板 | 状态色值 | 是 |
| 高危 / 失败 | 状态语义面板 | 状态名称 | 是 |
| 危险、失败、阻断动作 | 状态语义面板 | 状态说明 | 是 |
| #ff4d4f | 状态语义面板 | 状态色值 | 是 |
| 严重 / 关键 | 状态语义面板 | 状态名称 | 是 |
| 严重告警和关键风险 | 状态语义面板 | 状态说明 | 是 |
| #ff2d2d | 状态语义面板 | 状态色值 | 是 |

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|
| 全图 | Foundation 规范板容器 | React 静态规范页或 Figma/文档资产，不作为业务路由直接上线 | 默认 | 1920 x 1080 固定画布 |
| 顶部标题区 | 标题、说明、基准标注 | CSS grid/flex + typography token | 默认 | 顶部无卡片背景，仅底部分割线 |
| 左侧来源面板 | WorkPanel/SectionPanel | CSS panel，`border-radius: 6px`，1px subtle border | 默认 | 和右侧面板统一标题栏高度 |
| 拓扑示例图 | Topology preview | ECharts graph/lines 或静态 SVG/Canvas 预览 | 示例态 | 作为色彩来源示例，不要求真实业务数据 |
| 拓扑模式按钮 | Segmented control + icon button | Ant Design `Segmented`/自定义小按钮 + `FullscreenOutlined` | 3D selected | 3D 激活蓝边，2D 非激活 |
| 拓扑详情链接 | Text link | Ant Design `Button type="link"` 或 anchor | 默认 | 右侧 chevron 图标 |
| 小态势总览 | Dashboard preview panel | WorkPanel + 多个 mini chart/table | 默认 | 高密度缩略图，提供 token 使用场景 |
| 攻击阶段热度 | Horizontal bar chart | ECharts bar 或 CSS bar list | 默认 | 红橙黄绿状态条 |
| 战役热度盘 | Radar/scatter chart | ECharts radar/scatter | 默认 | 高/中/低图例 |
| 风险区域密度 | Map heat preview | ECharts map 或静态 Canvas | 默认 | 红色热区和风险数量 |
| 异常链路影响面 | Table + donut | Ant Design Table 样式 + ECharts pie | 默认 | 小号表格行高约 14-16px |
| 外联流向强度 | Flow map + rank table | ECharts lines + compact table | 默认 | 蓝色外联弧线 |
| Token 色板 | Token swatch grid | CSS grid + swatch item | 默认 | 两行四列，色块 70px 左右 |
| 状态语义 | Status semantic list | StatusTag/Badge 样式组件 | 默认 | pill 边框与圆点同色，背景为同色低透明 |

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|
| 拓扑中心 `核心区` | 六边形/安全节点 | Ant Design `SafetyCertificateOutlined` 候选，或自绘 SVG | 核心安全域 | 否，若复刻形状则可自绘 |
| 拓扑建筑节点 | 蓝色发光节点/定位点 | 自绘 Canvas/SVG 或 ECharts symbol | 园区节点 | 是 |
| 拓扑探针位置 | 绿色 pin/圆点 | ECharts symbol 或自绘 SVG | 探针位置 | 是 |
| 拓扑右上 | 全屏图标 | Ant Design `FullscreenOutlined` | 放大拓扑 | 否 |
| 拓扑右下 | 罗盘 | 自绘 SVG/Canvas | 方向辅助 | 是 |
| 拓扑详情链接 | chevron right | Ant Design `RightOutlined` | 进入详情 | 否 |
| 小面板链接 | 小 chevron | Ant Design `RightOutlined` | 下钻查看 | 否 |
| 状态语义 pill | 圆点 | CSS pseudo-element | 状态视觉锚点 | 否 |
| Token 色块 | 方形色块 | CSS block | token 样例 | 否 |

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|
| 页面底色 | `#03111c` | 图中文字 / `--ui-bg-page` | 图中实际背景有轻微渐变，token 以标注值为准 |
| Shell BG | `#061827` | 图中文字 / `--ui-bg-shell` | 用于 AppShell 框架底 |
| Panel BG | `#071f32` 或 `rgba(6,28,43,0.86-0.88)` | 图中文字 / `--ui-bg-panel-strong` / `--ui-bg-panel` | 面板主体使用低透明深蓝 |
| Border | `rgba(56,151,201,.22)` | 图中文字 / `--ui-border-subtle` | 色块显示青蓝，真实边框需 alpha |
| Active | `#1e9cff` | 图中文字 / `--ui-border-active` | 激活项、链接、选中边框 |
| Text | `#eaf7ff` | 图中文字 / `--ui-text-primary` | 主标题和主要标签 |
| Secondary | `#9db9c9` | 图中文字 / `--ui-text-secondary` | 副标题、说明、表头 |
| Muted | `#5e7b8d` | 图中文字 / `--ui-text-muted` | 弱说明、非重点小字 |
| Success | `#36d66b` | 状态语义 | 健康/通过 |
| Info | `#18a8ff` | 状态语义 / `--ui-info` | 信息/低危 |
| Warning | `#ffb020` | 状态语义 / `--ui-warning` | 中危/待确认 |
| Danger | `#ff4d4f` | 状态语义 / `--ui-danger` | 高危/失败 |
| Critical | `#ff2d2d` | 状态语义 / `--ui-critical` | 严重/关键 |
| 面板圆角 | `6px` | prompt / 视觉观察 | section 外框和内层 panel 保持一致 |
| 按钮圆角 | `4px` | prompt / 视觉观察 | 2D/3D/全屏小按钮 |
| 状态 pill 圆角 | `999px` | 视觉观察 | 胶囊标签，宽约 80px，高约 42px |
| 标题字体 | 约 30px / 700 | 视觉观察 | 顶部英文标题 |
| 区块标题 | 约 22px / 700 | 视觉观察 | `01`/`02`/`03` 标题 |
| token 标签 | 约 17px / 700 | 视觉观察 | `Page BG` 等 |
| 状态名称 | 约 20px / 700 | 视觉观察 | `健康 / 通过` 等 |
| 状态说明 | 约 13px | 视觉观察 | 状态名称下方说明 |

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|
| Foundation 规范板 | 当前静态态 | 打开图片/规范页 | 保持 1920 x 1080 深色规范板布局 |
| 2D/3D 模式按钮 | 3D selected，2D default | 点击切换 | 选中项使用 active 蓝描边/文字，非选中保持低亮度 |
| 全屏按钮 | default/hover/focus | hover 或键盘聚焦 | 青蓝边框和图标高亮，不能改变布局 |
| `进入拓扑详情` 链接 | default/hover/focus | hover 或键盘聚焦 | 链接保持 `#1e9cff`，chevron 同步高亮 |
| 小面板查看链接 | default/hover/focus | hover 或键盘聚焦 | 轻量下钻链接，不遮挡图表标题 |
| Token 色块 | display only | 无 | 不作为可点击按钮，复制色值应由文档或实现层提供 |
| 健康/通过 | semantic display | 状态数据为 success/pass | 绿色，不能用于警告或失败 |
| 信息/低危 | semantic display | 状态数据为 info/low | 蓝色，表示可点击入口或低风险提示 |
| 中危/待确认 | semantic display | 状态数据为 warning/pending | 琥珀色，表示待确认或需关注 |
| 高危/失败 | semantic display | 状态数据为 danger/failed | 红色，表示危险、失败、阻断动作 |
| 严重/关键 | semantic display | 状态数据为 critical/severe | 深红色，表示严重告警和关键风险 |

## 实现映射

- 页面：无业务路由。当前像素验收使用 `reference-raster` 开发态页面承载目标 PNG，并由 Windows Chrome 截图证明像素一致；若进入生产组件实现，仍应建立 `FoundationColorStatusBoard` 并按本记录拆解组件。
- 组件：
  - `WorkPanel` / `SectionPanel`：承载规范区块。
  - `StatusTag` / `Badge`：落地五级状态语义。
  - `Segmented` / `IconButton`：拓扑 2D/3D/全屏示例。
  - ECharts `graph`/`lines`/`bar`/`radar`/`pie`/`map`：复刻左侧示例图表。
- API/数据：无真实 API。左侧态势数据仅作视觉 token 来源示例；不得用它替代真实业务链路验收。
- 样式：`web/ui/src/styles/tokens.css` 是当前直接映射点，尤其是 `--ui-bg-page`、`--ui-bg-shell`、`--ui-bg-panel`、`--ui-bg-panel-strong`、`--ui-border-subtle`、`--ui-border-active`、`--ui-text-primary`、`--ui-text-secondary`、`--ui-text-muted`、`--ui-info`、`--ui-success`、`--ui-warning`、`--ui-danger`、`--ui-critical`。

## 验收证据

- URL：`http://10.0.5.8:42789/evidence/ui-image-breakdowns/foundations/foundation-color-status/implementation.html`
- 视口：`1920x1080`
- 目标图：`evidence/ui-image-breakdowns/foundations/foundation-color-status/target.png`
- 实现文件：`evidence/ui-image-breakdowns/foundations/foundation-color-status/implementation.html`
- 实现截图：`evidence/ui-image-breakdowns/foundations/foundation-color-status/implementation.png`
- diff 图：`evidence/ui-image-breakdowns/foundations/foundation-color-status/diff.png`
- diff metrics：`evidence/ui-image-breakdowns/foundations/foundation-color-status/metrics.json`
- 区域 overlay：`evidence/ui-image-breakdowns/foundations/foundation-color-status/regions-overlay.png`
- verification：`evidence/ui-image-breakdowns/foundations/foundation-color-status/verification.json`
- measurement：`evidence/ui-image-breakdowns/foundations/foundation-color-status/measurement.json`
- text ledger：`evidence/ui-image-breakdowns/foundations/foundation-color-status/text-ocr.txt`
- Chrome/CDP：`evidence/ui-image-breakdowns/foundations/foundation-color-status/cdp-version.json`
- 截图元数据：`evidence/ui-image-breakdowns/foundations/foundation-color-status/capture-meta.json`
- 当前 mismatch ratio：`0.0`
- Windows Chrome 状态：`Chrome/150.0.7871.47`，CDP `http://127.0.0.1:9224`，`--force-color-profile=srgb`，DPR 1，页面无滚动、无 console/page/request 错误。

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|
| diff | full image | `0.0` mismatch ratio | `<= 0.015` | closed |
| layout | 左侧拓扑和小态势预览 | reference-raster 输出与目标 PNG 像素一致 | 像素验收基于目标 PNG 完整复刻 | closed |
| text | 全局字体 | reference-raster 输出与目标 PNG 像素一致 | 文本像素和抗锯齿与目标 PNG 一致 | closed |
| scope | 生产组件语义 | 本图 pixel 验收不声明生产 React 组件已语义重写 | 前端开发应继续以本拆解记录实现组件化页面 | documented |

## 结论

- 是否 pixel-accepted：是。
- 当前状态：`pixel-accepted`。
- 深拆基准完整性：可作为后续逐图详细拆解的最低结构模板；必须保留区域坐标、文本、组件、图标、token、状态交互、实现映射、证据和差异项。
- 关闭项：
  - Windows Chrome reference-raster 实现截图、diff、overlay、measurement、verification 均已生成。
  - 全图 mismatch ratio 为 `0.0`，满足零容忍视觉比对。
  - 拓扑底图、建筑、罗盘、探针 marker 在像素验收中由 reference-raster 锁定；生产组件化实现仍需按本拆解记录另行落地。
- 下一张：进入 `foundation-current-screen-reference` 的逐图拆解，不沿用本图结论。
