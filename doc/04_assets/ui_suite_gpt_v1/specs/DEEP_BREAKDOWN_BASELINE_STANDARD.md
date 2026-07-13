# 逐图深拆基准标准

本标准以 `foundation-color-status` 为第一张保留基准。后续所有图片必须达到同等拆解深度，不能只满足字段非空。

## 基准文件

- 基准图片：`doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-color-status.png`
- 基准记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/foundations/foundation-color-status.md`
- 结构化记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/foundations/foundation-color-status.json`
- review：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/foundations/foundation-color-status.review.md`
- 目标证据：`evidence/ui-image-breakdowns/foundations/foundation-color-status/target.png`

## 最低门槛

`breakdown-accepted` 的最低门槛如下。它只代表建档深度达标，不代表像素复刻完成：

| 项 | 最低要求 | 说明 |
|---|---:|---|
| 主拆解记录行数 | `>= 220` | 不能用短说明替代深拆 |
| review 行数 | `>= 35` | 必须有人审视角的核查结论 |
| regions | `>= 12` | 必须拆到主要容器、子区和关键元素 |
| texts | `>= 30` | 必须覆盖标题、字段、按钮、状态、说明等可见文本 |
| components | `>= 6` | 必须映射为可实现的 React/Ant/ECharts 元件 |
| icons | `>= 5` | 必须说明图标/符号/状态点/无图标原因 |
| tokens | `>= 10` | 必须给出颜色、尺寸、圆角、字体、状态等 token |
| interactions | `>= 5` | 必须覆盖默认、hover/focus/selected/loading/error/danger 等相关状态 |
| evidence.target | 必须存在 | 必须保存目标图证据 |

## Pixel Accepted 门禁

每张图只有同时满足下列条件，才能标记为 `pixel-accepted`：

| 项 | 要求 |
|---|---|
| 独立记录 | 每张图必须有独立 `.md`、`.json`、`.review.md` |
| evidence 目录 | 每张图必须有独立 `evidence/ui-image-breakdowns/<category>/<id>/` |
| 目标图 | 必须有 `target.png` |
| 实现截图 | 必须有 Windows Chrome 采集的 `implementation.png` |
| 视觉 diff | 必须有 `diff.png` 和 diff 指标 |
| 区域覆盖 | 必须有 `regions-overlay.png`，用于证明坐标拆解覆盖目标图 |
| 验证记录 | 必须有 `verification.json`，记录截图环境、视口、diff 结果、结论 |
| unresolved | `.json`、`.md`、`.review.md`、`verification.json` 不得存在未关闭的 `unresolved` |
| 差异解释 | 所有视觉差异必须清零；若存在差异，必须被明确解释且不影响复刻目标，否则不得通过 |
| 主线程判定 | 最终 `pixel-accepted` 只能由主线程基于全部证据判断 |

## 逐图执行流程

每张图必须按以下顺序执行，禁止批量跳过：

1. **锁定单图**：从队列取一张图，确认 `category`、`id`、源图路径、目标 evidence 目录。
2. **直接视觉读取**：打开目标图，人工读取整体布局、业务语义、弹窗/菜单/状态、异常和特殊点。
3. **坐标测量**：测量画布、面板、表格、按钮、图表、图标、状态条和关键文本 bbox。
4. **OCR 辅助与人工校正**：OCR 只做初筛，最终文本以人工校正为准；所有可见文本进入 `texts`。
5. **组件与图标确认**：拆出 React/Ant Design/ECharts 可实现组件、图标来源、状态点、无图标原因。
6. **Token 提取**：记录颜色、字号、行高、间距、圆角、边框、状态色和响应式常量。
7. **交互状态拆解**：记录 normal、hover、selected、loading、empty、warning、error、locked、danger confirm、tooltip、权限、审计等状态。
8. **生成拆解记录**：写入独立 `.md`、`.json`、`.review.md`，并保存 `target.png`。
9. **区域覆盖图**：生成 `regions-overlay.png`，核对 bbox 是否覆盖所有关键区域。
10. **确定性实现**：生成或更新当前图的确定性 HTML/页面/组件实现。
11. **Windows Chrome 截图**：只使用 Windows Chrome CDP 截取 `implementation.png`，不得回退到 Linux 浏览器。
12. **视觉 diff**：生成 `diff.png`、metrics，并把差异写回记录。
13. **修复循环**：若存在 mismatch、缺证据、未解释差异或 unresolved，继续修复和复测。
14. **智能体辅助审查**：调用智能体对证据、截图、diff、文本、组件、图标、token、交互状态和 unresolved 进行查漏。
15. **主线程最终判断**：主线程复核智能体意见和全部证据，只有主线程可以写最终通过/不通过结论。
16. **未过返回拆解**：只要主线程判定不通过，必须回到拆解层重新补齐原因对应的坐标、文本、组件、图标、token、交互、实现映射或差异说明，再重新进入截图和 diff，不得只在实现层反复试错。

## 主线程未过回拆机制

主线程最终判断不通过时，必须按失败类型回到对应拆解阶段：

| 主线程失败原因 | 返回阶段 | 必须补齐 |
|---|---|---|
| 目标图理解不完整 | 直接视觉读取 | 重新观察整体布局、业务语义、特殊状态、弹窗/菜单/遮挡关系 |
| 坐标/区域不准 | 坐标测量 | 修正 `regions`、生成新的 `regions-overlay.png`、确认关键区域无遗漏 |
| OCR 或文字错误 | OCR 辅助与人工校正 | 修正 `texts`，补齐所有标题、字段、按钮、状态、数值、说明 |
| 组件拆错或漏拆 | 组件与图标确认 | 修正 `components`、`icons`，明确 React/Ant Design/ECharts 实现方式 |
| token 或样式偏差 | Token 提取 | 修正颜色、字号、行高、间距、圆角、边框、状态色 |
| 交互状态缺失 | 交互状态拆解 | 补齐 hover、selected、loading、empty、error、locked、danger confirm、tooltip、权限、审计 |
| evidence 缺失 | 生成拆解记录/区域覆盖图/Windows Chrome 截图 | 补齐 `target.png`、`implementation.png`、`diff.png`、`regions-overlay.png`、`verification.json` |
| diff 未达标 | 坐标测量或确定性实现 | 先判断是拆解漏项还是实现偏差；若拆解漏项，先改拆解记录，再改实现 |
| unresolved 未关闭 | 差异清单与 review | 每个 unresolved 必须修复、复测、关闭；不能靠文字绕过 |
| 智能体发现新问题 | 对应拆解阶段 | 主线程逐项判定后，问题成立则回拆并重新生成证据 |

回拆后的记录必须覆盖旧判断：

- `.md` 更新失败原因、补拆内容、差异关闭方式。
- `.json` 更新结构化 `regions/texts/components/icons/tokens/interactions/differences/unresolved`。
- `.review.md` 记录智能体辅助意见和主线程复判。
- `verification.json` 记录最新主线程结论。
- 重新生成 Windows Chrome `implementation.png`、`diff.png` 和 `regions-overlay.png`。

## 智能体辅助审查边界

- 智能体负责辅助检查，不负责最终验收。
- 智能体必须基于截图、diff、overlay、verification 和记录文件提出查漏项。
- 智能体输出只能作为 `.review.md` 或 `verification.json` 的辅助意见来源。
- 若智能体认为通过，但主线程发现证据缺失、diff 未达标、unresolved 未关闭或差异未解释，仍判定不通过。
- 若智能体发现问题，主线程必须逐项判断：修复、解释为可接受差异，或保持不通过。
- 最终 `pixel-accepted` 必须由主线程写入，并明确引用证据路径。
- 主线程判定未过时，智能体意见不能作为结束理由；必须进入“主线程未过回拆机制”。

## 必备章节

主拆解记录必须包含：

- `## 基本信息`
- `## 目标图观察`
- `## 区域与坐标`
- `## 文本清单`
- `## 组件清单`
- `## 图标清单`
- `## Token 与样式`
- `## 状态与交互`
- `## 实现映射`
- `## 验收证据`
- `## 差异清单`
- `## 结论`

## 处理规则

- 只保留第一张基准记录；其它旧拆解记录全部删除后重新生成。
- 后续逐张拆解必须先直接查看目标图片，再写独立 md/json/review。
- 校验脚本必须拒绝浅拆记录；`breakdown-accepted` 只表示达到深拆记录门槛，不表示像素 diff 通过。
- 像素级复刻必须由 Windows Chrome 截图、区域 overlay、视觉 diff、智能体辅助审查和主线程最终判断共同闭环。
- 主线程最终判断未过时，流程必须回到拆解层，重新补拆并重跑证据链，直到主线程基于完整证据判定通过。
