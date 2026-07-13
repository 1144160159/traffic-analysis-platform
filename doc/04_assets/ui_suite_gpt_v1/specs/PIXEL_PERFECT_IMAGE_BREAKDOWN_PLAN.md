# UI 图逐张精拆与 100% 复刻验收方案

## 目标

把 `doc/04_assets/ui_suite_gpt_v1/screens/` 下每一张 canonical PNG 都拆成可直接指导前端实现的精确记录，并通过逐图实现截图与目标图对比，做到“无未解释差异、无视觉重叠、无交互状态遗漏”。

本方案不把程序抽取结果视为真源。程序只能用于建档、查漏和格式校验；每张图的布局、组件、元素、图标、文本、状态和复刻结论必须经过逐张视觉校正。

## 适用范围

- 页面图：`screens/pages/*.png`
- 浮层图：`screens/overlays/*.png`
- 组件板：`screens/components/*.png`
- foundation 规范板：`screens/foundations/*.png`
- 状态图：`screens/states/*.png`
- 响应式图：`screens/responsive/*.png`

排除：

- `*.raw-*`
- `*.before-*`
- 临时截图、实验图、未进入 canonical 目录的素材

## 核心原则

1. 一次只处理一张图片。
2. 每张图片必须有独立 Markdown、JSON、证据截图和验收结论。
3. 不允许用批量 inventory、程序推断或公共模板替代单图精拆。
4. 所有结论必须能回指到目标 PNG、视口尺寸、截图证据和复现步骤。
5. 不能只看 DOM；实现验收必须看截图，并做视觉 diff。
6. 若存在无法确认的元素，记录为 `unresolved`，该图不得标记为复刻完成。
7. “100% 复刻完成”只表示该图通过全部门禁，不表示程序天然 100% 正确。

## 单图产物结构

每张图固定生成和维护以下产物：

```text
specs/image-breakdowns/<category>/<image-id>.md
specs/image-breakdowns/<category>/<image-id>.json
specs/image-breakdowns/<category>/<image-id>.review.md
evidence/ui-image-breakdowns/<category>/<image-id>/
```

证据目录至少包含：

```text
target.png
implementation.png
diff.png
regions-overlay.png
text-ocr.txt
measurement.json
verification.json
```

## 工具边界

全量索引只允许用于排队：

```bash
node doc/04_assets/ui_suite_gpt_v1/build_pixel_breakdown_queue.mjs
```

单图精拆启动只允许一次处理一张图：

```bash
node doc/04_assets/ui_suite_gpt_v1/start_pixel_image_breakdown.mjs --image doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-color-status.png
```

上述工具不产生最终拆解结论。最终结论必须来自人工视觉校正、Windows Chrome 截图和视觉 diff。

## 单图精拆流程

### 1. 锁定目标图

记录：

- 图片路径
- 图片尺寸
- 分类
- 对应 prompt
- 对应 manifest 项
- 对应路由或宿主路由
- 是否属于 AppShell 页面、独立登录页、浮层、组件板或状态图

完成条件：

- 图片源文件存在
- 不是 raw/before 临时图
- 与当前拆解记录一一对应

### 2. 视觉读取

必须直接查看目标 PNG，不接受只读 prompt 或文件名推断。

记录：

- 整体布局结构
- 主内容比例
- 左/中/右区域边界
- 顶部、左侧、底部公共区是否出现
- 是否存在遮罩、弹窗、抽屉、下拉、确认框
- 是否存在图表、表格、状态机、图谱、表单、证据卡、操作栏

完成条件：

- 每个可见区域都有 bbox
- 每个区域都有“用途”和“复刻要点”
- AppShell 和业务区边界不能混淆

### 3. 空间测量

对目标图做坐标级拆解。

记录：

- 画布尺寸
- 全局 AppShell 坐标
- 主内容区坐标
- 每个面板 bbox
- 面板内标题、工具栏、图表、表格、按钮、状态标签 bbox
- 弹窗/抽屉/下拉的锚点、遮罩和容器 bbox

完成条件：

- 页面/浮层/组件的一级区域 bbox 覆盖完整画面
- 关键元素 bbox 精确到可实现布局
- 重叠关系、层级和 z-index 有记录

### 4. 文本 OCR 与人工校正

先 OCR，再人工对照目标图修正。

记录：

- 所有可见中文标题
- 表格列名
- 按钮文案
- 状态标签
- 指标名称
- 图例
- 错误提示
- trace id、时间窗、对象名等业务上下文

完成条件：

- 关键文本 100% 逐项列出
- 不可读文本必须标注原因和处理策略
- 前端实现必须使用相同文案或明确记录允许差异

### 5. 组件/元素/图标识别

逐个区域拆出前端实现单位。

记录：

- React 页面组件
- Ant Design 组件
- ECharts 图表类型
- 本地业务组件
- HTML/CSS 基础元素
- 图标库名称和图标语义
- 自定义图形或 SVG 需求

完成条件：

- 每个可见按钮、输入、选择器、标签、图标都有组件归属
- 图标不能只写“图标”，必须写候选库名、图标名或“需自绘”
- 图表必须写清数据维度、坐标轴、图例、颜色和空态
- 业务图示必须区分可截图静态资产与基于 API 的动态图表；业务动态图表优先使用 ECharts 等数据驱动组件
- ECharts 图表必须与所在模块自适应：canvas、tooltip 触发区、标题、图例和数值标签不得脱离卡片/面板边界，不得覆盖模块内文字
- 业务区域所有模块内部都必须自适应：标题、数值、说明、按钮、图标、图表、表格和列表项都必须落在模块边界内，并随模块尺寸变化保持可读
- 所有因空间受限而省略的文字，必须提供悬浮查看完整内容的机制，例如 `title` 或 Tooltip；验收时必须覆盖菜单、按钮、表格单元格、图表标签和卡片说明

### 6. 视觉 token 提取

记录该图实际使用的视觉参数。

至少包括：

- 背景色
- 面板色
- 边框色
- 主文字、次级文字、弱文字颜色
- 状态色
- 圆角
- 阴影/发光
- 字号、字重、行高
- 间距
- 表格行高
- 图表网格线和轴线样式

完成条件：

- 能映射到 `tokens.json` 或记录新增 token
- 与 foundation 冲突时必须标出冲突和处理结论

### 7. 状态与交互拆解

记录目标图所表达的页面状态和交互状态。

包括：

- 默认态
- hover/focus/selected/disabled/loading/error/empty
- 弹窗 opening/open/submitting/success/failed
- 表格排序/筛选/分页
- 图谱节点选中
- Drawer/Modal 关闭与提交
- 权限不足、危险操作、审计留痕

完成条件：

- 每张图至少有当前画面状态
- 有交互控件的图必须列出可操作状态
- 危险动作必须包含权限、影响范围、二次确认和审计 trace

### 8. 实现映射

把拆解结果落到前端文件和组件。

记录：

- 页面文件
- 组件文件
- service/API
- fixture 或真实 API 数据来源
- CSS token 文件
- ECharts option builder
- 需要新增或复用的组件

完成条件：

- 前端开发者能按记录直接实现
- 没有“后续再看”“按图实现”等空泛描述
- 所有 mock/fixture 只用于视觉比对，不替代真实链路验收
- ECharts option builder 必须使用容器宽高自适应策略，避免固定像素高度导致响应式或数据刷新后溢出

### 9. 逐图实现与截图

实现后必须使用 Windows Chrome 做截图验收。

默认要求：

- 优先使用用户指定的 Windows Chrome CDP `http://127.0.0.1:9224`
- 9224 不可用时先恢复 Windows Chrome CDP 和 SSH 隧道
- 禁止回退到 Linux 本机浏览器完成正式视觉证据

记录：

- URL
- 视口尺寸
- 页面状态
- 登录态/数据态
- 截图路径
- 控制台错误
- 网络错误

完成条件：

- `implementation.png` 来自 Windows Chrome
- 截图覆盖目标图同等视口
- 无 4xx/5xx、requestfailed、非 warning console/pageerror

### 10. 视觉 diff 与人工判定

每张图必须同时做机器 diff 和人工审查。

机器 diff：

- 目标图 vs 实现截图
- 生成 `diff.png`
- 输出 mismatch ratio、颜色差、区域差异
- 对 AppShell、主内容、浮层、文本、图标分别统计

人工审查：

- 布局是否一致
- 文本是否一致
- 菜单是否一致
- 图标是否一致
- 组件密度是否一致
- 是否有视觉重叠
- 是否有错色、错字号、错圆角、错间距
- 交互状态是否可用

完成条件：

- 所有差异都有结论：已修复、可接受、设计源图差异、业务数据允许差异
- 没有未解释差异
- 没有文本溢出、遮挡、错位、重叠
- 没有错误菜单、错误图标、错误状态色

## 单图验收等级

### `draft`

已有程序初稿或人工初稿，但未完成视觉校正。

### `review-ready`

已完成人工精拆，但尚未实现或尚未截图 diff。

`review-ready` 不是完成状态。自动流水线必须继续进入实现、Windows Chrome 截图和视觉 diff；如果做不到，必须标记 `blocked` 并写清阻塞。

### `implemented`

前端已实现，已有 Windows Chrome 截图。

### `diff-pending`

已有目标图、实现图和 diff，但仍有待修复差异。

### `pixel-accepted`

该图通过全部验收门禁：

- 拆解记录完整
- JSON 与 Markdown 一致
- 实现截图来自 Windows Chrome
- 机器 diff 达标
- 人工审查无未解释差异
- 交互状态无阻塞
- 复现步骤完整

## 差异处理规则

任何差异都必须进入单图 review 记录。

差异类型：

- `layout`：坐标、尺寸、间距、层级
- `text`：文案、字号、截断、换行
- `component`：组件类型或状态错误
- `icon`：图标缺失、图标错误、语义错误
- `color`：状态色、背景、边框、透明度
- `chart`：图表类型、坐标轴、图例、数据形态
- `table`：列名、行高、密度、排序/筛选
- `interaction`：点击、hover、loading、错误态不可用
- `responsive`：移动端/窄屏溢出、菜单折叠错误
- `business`：业务指标、链路、权限、审计语义错误

每条差异必须有：

- 截图证据
- 目标区域
- 当前表现
- 期望表现
- 修复建议
- 修复状态

## 逐图执行顺序

优先级从低层到页面：

1. foundations：先固化 token、布局、图标、表格、图表规范。
2. components：拆出可复用组件，避免页面重复实现。
3. screen/login：锁定 AppShell 和认证页基准。
4. pages：逐页面拆解主工作区和业务闭环。
5. overlays：按宿主页面逐个拆解浮层。
6. states：统一 loading/empty/error/forbidden/offline/success。
7. responsive：最后校验断点、抽屉、移动端操作。

同一阶段仍然必须逐张处理，不允许一次性批量关闭。

## 单图记录模板

```markdown
# <image-id>.png 逐图精拆记录

## 基本信息

- 分类：
- 源图：
- 源图尺寸：
- 对应 prompt：
- 对应路由/宿主路由：
- 当前状态：
- 复刻等级：

## 目标图观察

- 整体布局：
- 业务重点：
- 当前页面/浮层状态：

## 区域与坐标

| 区域 | bbox | 层级 | 说明 | 复刻要点 |
|---|---:|---:|---|---|

## 文本清单

| 文本 | 位置 | 类型 | 是否必须完全一致 |
|---|---|---|---|

## 组件清单

| 区域 | 组件/元素 | 实现方式 | 状态 | 备注 |
|---|---|---|---|---|

## 图标清单

| 位置 | 图标 | 图标库/实现 | 语义 | 是否需自绘 |
|---|---|---|---|---|

## Token 与样式

| 项 | 值 | 来源 | 备注 |
|---|---|---|---|

## 状态与交互

| 控件/区域 | 状态 | 触发方式 | 期望表现 |
|---|---|---|---|

## 实现映射

- 页面：
- 组件：
- API/数据：
- 样式：

## 验收证据

- URL：
- 视口：
- 目标图：
- 实现截图：
- diff 图：
- 控制台/网络：

## 差异清单

| 类型 | 位置 | 当前 | 期望 | 状态 |
|---|---|---|---|---|

## 结论

- 是否 pixel-accepted：
- 未解决问题：
- 下一步：
```

## JSON 必填字段

```json
{
  "id": "",
  "category": "",
  "source_image": "",
  "canvas": { "width": 1920, "height": 1080 },
  "status": "draft",
  "route": null,
  "host_route": null,
  "regions": [],
  "texts": [],
  "components": [],
  "icons": [],
  "tokens": [],
  "interactions": [],
  "implementation": {
    "pages": [],
    "components": [],
    "services": [],
    "styles": []
  },
  "evidence": {
    "target": "",
    "implementation": "",
    "diff": "",
    "regions_overlay": "",
    "viewport": "",
    "url": ""
  },
  "differences": [],
  "unresolved": [],
  "accepted": false
}
```

## 完成定义

整个 UI 图拆解工作只有在以下条件全部满足时才算完成：

- `screens/` 下每张 canonical PNG 都有精拆记录。
- 每张图都有独立 review 文件。
- 每张图都有目标图、实现截图、diff 图和验证 JSON。
- 每张图状态为 `pixel-accepted`。
- 没有 `unresolved`。
- 没有未解释视觉差异。
- 没有无法复现的验收结论。
- 生成一份最终索引，只汇总状态和证据路径，不替代逐图记录。

## 自动流水线门禁

每轮智能体或脚本执行后先运行拆解记录门禁：

```bash
node doc/04_assets/ui_suite_gpt_v1/validate_image_breakdown_records.mjs
```

该门禁只判断逐图视觉拆解是否完整。若某图未通过，继续拆解该图，直到 `breakdown-accepted`。

随后运行完整像素复刻门禁：

```bash
node doc/04_assets/ui_suite_gpt_v1/validate_pixel_breakdown_pipeline.mjs
```

完整像素复刻门禁按以下状态判断每张图是否完整走完：

- `breakdown-missing`
- `breakdown-incomplete`
- `implementation-missing`
- `windows-cdp-screenshot-missing`
- `visual-diff-missing`
- `diff-metrics-missing`
- `not-accepted`
- `pixel-accepted`

除 `pixel-accepted` 外全部视为未完成。

## 风险声明

程序无法从 PNG 中天然识别 100% 真实组件语义。100% 复刻只能通过“逐图人工精拆 + 逐图实现 + Windows Chrome 截图 + 视觉 diff + 差异清零”来达成工程验收。任何未经过截图和 diff 的记录，都只能算拆解草稿，不能作为最终复刻依据。
