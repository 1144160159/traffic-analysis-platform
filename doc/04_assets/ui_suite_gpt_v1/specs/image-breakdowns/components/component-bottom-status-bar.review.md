# component-bottom-status-bar.png 主线程审查记录

## 审查范围

- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-bottom-status-bar.png`
- 拆解记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/components/component-bottom-status-bar.md`
- 结构化记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/components/component-bottom-status-bar.json`
- 证据目录：`evidence/ui-image-breakdowns/components/component-bottom-status-bar/`

## 主线程检查

| 检查项 | 结论 | 证据 |
|---|---|---|
| 逐张处理 | pass | 本记录只覆盖 `component-bottom-status-bar` |
| 目标图直接视觉读取 | pass | 已直接查看目标 PNG，并按 01-05 五个区块拆解 |
| prompt/layer 读取 | pass | 已读取 prompt 和 `specs/layers/component-bottom-status-bar.json` |
| 坐标测量 | pass | 已记录主样例、七个状态项、全局动作区、职责边界、状态变体、React code 和 footer bbox |
| OCR/文本校正 | pass | 已通过全图和局部裁图人工校正状态项、职责边界、变体、token 和 React 代码 |
| 组件/元素/图标确认 | pass | 已覆盖 BottomStatusBar、StatusItem、GlobalActions、OwnershipBoundaryPanel、StatusVariantRows、ReactMappingPanel |
| token 提取 | pass | 已覆盖 y=997、height=83、radius 4-6、divider 低透明青色、状态色和 skeleton |
| 交互状态拆解 | pass | 已覆盖 fixed、normal、warning、danger、loading、notification、settings、config、power |
| reference-raster 范围说明 | pass | 已注明像素验收只证明目标 PNG 复刻，不声明生产 React 语义实现完成 |
| 辅助智能体审查 | pass | Nash 已完成只读视觉查漏，结论已纳入本 review |
| overlay 生成 | pass | 已回看 `regions-overlay.png`，覆盖主样例、七个状态项、职责边界、状态变体、React code 和 footer |
| Windows Chrome 截图 | pass | 已使用 `Windows Chrome CDP` 截取 `implementation.png`，视口 `1920x1080` |
| 视觉 diff | pass | `metrics.json` 显示 mismatch ratio `0.0`，`diff.png` 无异常高亮 |
| 主线程最终判定 | pass | 截图、diff、overlay、辅助审查均完成，主线程判定通过 |

## 主线程观察摘要

- 该图是底部全局状态栏组件规范板，不是完整业务页面。
- 01 区块展示 screen.png 底部 y=997/h=83 裁切和固定顺序。
- 02 区块拆分七个 StatusItem 和一个 GlobalActions。
- 03 区块用绿色允许项和红色禁止项定义职责边界。
- 04 区块展示正常运行、延迟升高、链路降级、加载骨架四个状态变体。
- 05 区块展示七个 StatusItem kind 和 GlobalActions React 拆分。
- 底部验收口径强调不得改成页面专属指标栏，不得把通知、设置、电源移到顶部。

## 辅助智能体 Nash 查漏摘要

- Nash 确认整体结构为顶部标题、01 主样例、02 状态项拆分、03 职责边界、04 状态变体、05 React 可实现拆分和底部验收口径。
- Nash 确认主样例状态栏顺序为数据延迟、系统运行、告警处理SLA、数据质量合格率、存储使用、带宽使用、日志吞吐、全局动作区。
- Nash 确认右侧全局动作区包含通知铃铛红色角标 `9`、设置齿轮、全局配置齿轮、电源按钮。
- Nash 确认职责边界允许通知角标、设置/全局配置、电源在底部，禁止顶部通知铃铛、顶部用户头像、页面专属底栏。
- Nash 确认状态变体强调指标可变化但动作区位置不变，loading 保留骨架布局。
- Nash 提醒生产实现需统一 `23 天 14 小时` 与 `23天14小时`、`68.7 / 120 TB` 与 `68.7/120TB` 等 formatter。
- Nash 提醒两个齿轮图标语义接近，生产实现必须用 tooltip/aria label 区分设置和全局配置。
- Nash 提醒状态变体只展示部分指标，存储、带宽、运行时长的异常和加载表现需在生产实现明确。
- Nash 提醒 `y=997/h=83` 是截图验收坐标，生产 CSS 应使用固定底部高度和 safe-area 约束。
- Nash 提醒通知红点、电源动作、用户菜单 overlay 需权限、确认和不携带完整 AppShell 的约束校验。

## 主线程判断口径

- 目标 PNG 像素复刻必须保留 01-05 五区块布局。
- 目标 PNG 像素复刻必须保留 screen.png 底部 y=997/h=83 约束。
- 目标 PNG 像素复刻必须保留七个状态项固定顺序和右侧全局动作区。
- 目标 PNG 像素复刻必须保留职责边界的绿色允许项和红色禁止项。
- 目标 PNG 像素复刻必须保留四行状态变体和底部 token 行。
- 目标 PNG 像素复刻必须保留 React 组件建议的八个 code chip。
- 全局动作弹层属于生产实现和其它 UI 状态，不作为当前目标 PNG 复刻差异。
- 如果 Windows Chrome implementation 与 target 的 mismatch ratio 为 `0.0`，并且 overlay 与坐标记录一致，则主线程可判定本图 pixel-accepted。
- 若 evidence 中出现缺图、截图不是 Windows Chrome、视口不是 1920x1080、存在滚动条、diff 有异常高亮或 console/page error，则主线程不能判定通过。

## 当前结论

- 当前状态：`pixel-accepted`
- URL：`http://10.0.5.8:43457/evidence/ui-image-breakdowns/components/component-bottom-status-bar/implementation.html`
- 浏览器：`Chrome/150.0.7871.47`，`Windows Chrome CDP`
- 视口：`1920x1080`，devicePixelRatio `1`
- diff：mismatch ratio `0.0`
- 主线程判定：target、implementation、diff、regions-overlay、capture-meta 和辅助审查均满足验收门。
- 本 review 只服务当前单张图片，不合并其它组件板结论。
