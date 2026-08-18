# Web UI 栅格、尺寸与组件状态规范速查

更新：2026-08-11

本文件是项目中性规范，不复制商业图书正文。`必须`来自 WCAG/WAI-ARIA 的可验收要求；`建议`是从多个官方设计系统归纳出的工程基线。设计系统中的品牌颜色、字体和专属组件外观不纳入项目规范。

## 1. 规范层级

1. **必须**：WCAG 2.2 AA、原生 HTML 语义、WAI-ARIA 1.2、ARIA APG 键盘行为。
2. **项目统一值**：栅格、间距、圆角、字号、层级和状态令牌；同一产品只能有一套权威令牌。
3. **参考值**：USWDS、Carbon、Material、Ant Design 等官方系统的布局或组件做法，只用于比较和验证，不直接混搭视觉值。

## 2. 栅格与尺寸

### 2.1 官方系统常见数值

| 参考 | 列数/基准 | 断点或容器 | 间距/边距 |
|---|---|---|---|
| USWDS Layout Grid | 12 列、mobile-first、Flexbox | 默认容器最大宽度 1024 px | 窄屏水平内边距 2 units；desktop 及以上 4 units，可配置 |
| Carbon 2x Grid | 4 / 8 / 16 / 16 / 16 列 | 320 / 672 / 1056 / 1312 / 1584 px | 基础单元 8 px；标准 padding 16 px；宽 gutter 32 px |
| Ant Design Grid | 24 等分 | 1440 示例的内容区为 1168 px | 由 Row/Col 和 gutter 配置控制 |

这些数值是设计系统选择，不是 Web 的全球强制标准。项目必须先选定一种列模型，再固定 token；不得把 12、16、24 列同时用于同一页面层级。

### 2.2 本项目建议基线

- 空间原子：4 px；主要节奏：8 px。
- 常用间距令牌：4、8、12、16、24、32、48、64 px。
- 常规 Web 内容区：12 列；密集数据页面也保持 12 列，通过子网格或局部布局解决，不另建整页列制。
- 页面最小左右安全边距：窄屏 16 px，桌面 24–32 px。
- 常规列 gutter：24 px；信息密度很高时可用 16 px，但必须整页一致。
- 不把设备型号当断点；断点由内容开始拥挤、截断或失去层级时确定，并至少验证 320 CSS px 宽度下的回流。
- 设计稿必须同时给出窄屏、中屏、桌面和宽屏的列数、margin、gutter、最大内容宽度和组件重排规则。

## 3. WCAG 2.2 可验收尺寸与视觉门槛

- 正文文字对比度至少 4.5:1；大号文字至少 3:1。
- 非文本控件、图形和可交互状态的必要视觉信息，对相邻颜色至少 3:1。
- 键盘焦点始终可见；焦点外观不得只依赖轻微颜色变化。
- AA 级目标尺寸至少 24 × 24 CSS px，或满足 WCAG 2.2 的间距等价例外；项目建议鼠标/触控主要操作目标采用 44 × 44 CSS px 作为更稳妥基线。
- 页面在 320 CSS px 宽度等价视口下应能回流，除确实需要二维布局的内容外，不产生双向滚动。
- 文本放大到 200% 时，内容和功能不得丢失；文本间距被用户覆盖后仍需可用。

## 4. 组件状态最小集合

每个交互组件在设计稿、组件文档和测试中至少检查下列状态。不存在的状态要明确写“不适用”，不得默默缺失。

| 状态 | 必须表达的内容 | 可访问性/行为检查 |
|---|---|---|
| Enabled / Default | 可操作、默认层级 | 有可访问名称；角色正确 |
| Hover | 指针悬停反馈 | 不能作为唯一信息入口；触屏无 hover 时仍可用 |
| Focus-visible | 当前键盘焦点 | 焦点清晰可见；顺序与 DOM/阅读顺序一致 |
| Pressed / Active | 正在按下或激活 | 激活时机一致；按钮支持 Enter/Space |
| Selected / Checked / On | 持久选择或开关状态 | 与 focus 明确区分；使用 checked、selected、pressed 等正确语义 |
| Loading / Busy | 操作处理中 | 防重复提交；提供可感知进度或忙碌状态；完成后反馈 |
| Disabled / Unavailable | 当前不可操作 | 优先解释原因；需要被发现时可用 `aria-disabled` 保持可聚焦，否则使用原生 `disabled` |
| Read-only | 可读但不可编辑 | 不伪装为 disabled；仍允许选择、复制和必要导航 |
| Error / Invalid | 输入或操作失败 | 错误与字段关联；说明原因和修复方法；不只用红色 |
| Success / Confirmation | 操作已完成 | 信息可被辅助技术感知；避免只靠短暂 toast |
| Empty / No result | 无数据或无匹配 | 区分“初始为空”“过滤无结果”“加载失败”；给下一步操作 |
| Dragged / Drop target | 正在拖动与可放置位置 | 同时提供非拖拽键盘替代；明确允许/禁止落点 |

## 5. 常见组件专项清单

- **按钮**：默认、hover、focus-visible、pressed、loading、disabled；危险操作还要有确认或撤销策略。
- **输入框**：空、已填、hover、focus、readonly、disabled、校验中、error、success；标签不可用 placeholder 替代。
- **下拉/组合框**：关闭、展开、选项 hover/focus/selected、无结果、加载、disabled；键盘行为按 APG 对应模式。
- **表格/数据网格**：默认、行 hover、focus、selected、排序、筛选、加载、空、错误；普通表格不要错误使用 `grid` 角色。
- **模态对话框**：打开后焦点进入、焦点被约束、Escape/关闭规则明确、关闭后焦点返回触发点；破坏性操作不能只给含糊的“确定”。
- **导航/标签页**：当前项、hover、focus、disabled；focus 与 selected 分离；延迟加载面板不建议让选择自动跟随焦点。
- **通知/Toast**：info、success、warning、error；持续时间与可读性匹配，重要错误不能自动消失。

## 6. 验收交付物

- 栅格标注：列数、margin、gutter、最大宽度、断点和重排规则。
- 组件状态矩阵：每个组件 × 状态 × 视觉 token × ARIA/HTML 语义 × 键盘行为。
- 焦点路线图：Tab 顺序、复合组件内部方向键行为、模态焦点返回点。
- 可访问性测试：键盘、200% 缩放、320 CSS px 回流、对比度、辅助技术名称/角色/值。
- 状态不得只存在于设计稿；实现层必须能由自动测试或人工步骤复现。

## 官方来源

- WCAG 2.2：https://www.w3.org/TR/WCAG22/
- WAI-ARIA APG 键盘接口：https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/
- Material 3 States：https://m3.material.io/foundations/interaction/states/overview
- USWDS Layout Grid：https://designsystem.digital.gov/utilities/layout-grid/
- Carbon 2x Grid：https://v10.carbondesignsystem.com/guidelines/2x-grid/overview/
- Ant Design Grid：https://ant.design/components/grid-cn/

