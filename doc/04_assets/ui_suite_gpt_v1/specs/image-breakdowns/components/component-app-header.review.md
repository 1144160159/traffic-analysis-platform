# component-app-header.png 主线程审查记录

## 审查范围

- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-app-header.png`
- 拆解记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/components/component-app-header.md`
- 结构化记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/components/component-app-header.json`
- 证据目录：`evidence/ui-image-breakdowns/components/component-app-header/`

## 主线程检查

| 检查项 | 结论 | 证据 |
|---|---|---|
| 逐张处理 | pass | 本记录只覆盖 `component-app-header` |
| 目标图直接视觉读取 | pass | 已直接查看目标 PNG，按 01-05 五个规范区和底部验收口径拆解 |
| prompt/layer 读取 | pass | 已读取 prompt 和 `specs/layers/component-app-header.json` |
| 坐标测量 | pass | 已记录标题区、主样例、结构拆分、职责边界、状态变体、归属关系、底部验收口径等 bbox |
| OCR/文本校正 | pass | 已按目标图人工校正 `规则检测`、topbar=80px、no user/no bell、验收口径等文字 |
| 组件/元素/图标确认 | pass | 已覆盖 AppHeader、RuntimeMetricCard、QuickEntryGroup、BoundaryPanel、StateVariantPanel、OwnershipPanel |
| token 提取 | pass | 已覆盖背景、网格、面板、topbar 80px、font、border、radius、状态色 |
| 交互状态拆解 | pass | 已覆盖站点下拉、指标状态、快捷入口、正常/高危/加载/离线降级、顶部禁止项 |
| reference-raster 范围说明 | pass | 已注明像素验收只证明目标 PNG 复刻，不声明生产 React 语义实现完成 |
| 辅助智能体审查 | pass | Ampere 已完成只读查漏，结论已纳入本 review |
| overlay 生成 | pass | 已回看 `regions-overlay.png` |
| Windows Chrome 截图 | pass | 已通过 Windows Chrome CDP 生成并回看 `implementation.png` |
| 视觉 diff | pass | 已检查 `diff.png` 和 `metrics.json`，mismatch ratio `0.0` |
| 主线程最终判定 | pass | 截图、diff、辅助审查均完成后主线程判定通过 |

## 辅助智能体 Ampere 查漏摘要

- Ampere 确认本图是组件规范说明页，不是常规组件样例板。
- Ampere 确认布局为顶部标题区、01 主样例区、02 结构拆分区、03 职责边界区、04 状态变体区、05 归属关系区和底部验收口径区。
- Ampere 确认主样例模块顺序为品牌区、站点选择、时间、风险态势、告警总数、关键告警、采集健康度、数据质量、六个快捷入口。
- Ampere 校正快捷入口文本为 `PCAP检索`、`资产检索`、`规则检测`、`脚本中心`、`帮助中心`、`更多应用`。
- Ampere 确认结构拆分区的固定顺序为品牌、站点、时间、风险、告警、关键、健康、质量、PCAP、资产、规则、脚本、帮助、更多。
- Ampere 确认顶部保留项为运行指标、站点时间、快捷入口。
- Ampere 确认顶部禁止项为通知铃铛、用户头像/菜单、设置/电源。
- Ampere 确认状态变体为正常、高危、加载、离线降级，且右端仍止于快捷入口。
- Ampere 确认用户身份、通知、设置、电源归属于左下或底部区域，不属于顶部栏。
- Ampere 提醒 `告警总数` 使用铃铛图标，容易和通知入口混淆，生产交互必须限制为态势指标。
- Ampere 提醒固定 80px 加六个快捷入口在窄屏可能溢出，生产实现需要响应式收纳策略。
- Ampere 提醒 `更多应用` 图标可能和窗口操作语义混淆，生产实现需 tooltip 或图标语义校正。

## 主线程判断口径

- 目标 PNG 像素复刻必须保留现有 01-05 编号区布局、文本、颜色和裁切关系。
- Ampere 提出的响应式收纳、tooltip、交互限制属于生产组件增强建议，不作为目标 PNG 复刻差异。
- 本图最重要的验收红线是：顶部出现用户头像、用户名、通知铃铛、设置或电源即不合格。
- Windows Chrome implementation 与 target 的 mismatch ratio 为 `0.0`，overlay 与坐标记录一致，主线程判定本图 pixel-accepted。
- evidence 中 target、implementation、diff、overlay、metrics、capture meta、CDP 记录均已生成。

## 当前结论

- 当前状态：`pixel-accepted`
- 是否可标记 pixel-accepted：是。
- 证据闭环：overlay、Windows Chrome 截图、diff、metrics、辅助审查和 verification 均已完成。
- 本 review 只服务当前单张图片，不合并其它组件板结论。
