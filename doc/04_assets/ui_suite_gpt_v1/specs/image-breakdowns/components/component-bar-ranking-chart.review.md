# component-bar-ranking-chart.png 主线程审查记录

## 审查范围

- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-bar-ranking-chart.png`
- 拆解记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/components/component-bar-ranking-chart.md`
- 结构化记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/components/component-bar-ranking-chart.json`
- 证据目录：`evidence/ui-image-breakdowns/components/component-bar-ranking-chart/`

## 主线程检查

| 检查项 | 结论 | 证据 |
|---|---|---|
| 逐张处理 | pass | 本记录只覆盖 `component-bar-ranking-chart` |
| 目标图直接视觉读取 | pass | 已直接查看目标 PNG，按标题、条形排行、排行表、状态矩阵和底部语义区拆解 |
| prompt/layer 读取 | pass | 已读取 prompt 和 `specs/layers/component-bar-ranking-chart.json` |
| 坐标测量 | pass | 已记录画布、标题、六条 bar、排行表三行、状态矩阵、底部六卡等 bbox |
| OCR/文本校正 | pass | 已按目标图人工校正所有可见中文、英文状态、技术词和数值 |
| 组件/元素/图标确认 | pass | 已覆盖 BarRankingChart、RankingBarRow、RankingDetailTable、StateMatrixItem、StatusDot、SemanticsTile |
| token 提取 | pass | 已覆盖背景、网格、面板、bar 色、状态色、圆角、行高和组件网格 |
| 交互状态拆解 | pass | 已覆盖 hover、selected、loading、empty、error、locked、危险动作说明 |
| reference-raster 范围说明 | pass | 已注明像素验收只证明目标 PNG 复刻，不声明生产 React/ECharts 语义实现完成 |
| 辅助智能体审查 | pass | Erdos 已完成只读视觉查漏，结论已纳入本 review |
| overlay 生成 | pass | 已回看 `regions-overlay.png`，覆盖六条 bar、排行表、状态矩阵和底部六卡 |
| Windows Chrome 截图 | pass | 已使用 `Windows Chrome CDP` 截取 `implementation.png`，视口 `1920x1080` |
| 视觉 diff | pass | `metrics.json` 显示 mismatch ratio `0.0`，`diff.png` 无异常高亮 |
| 主线程最终判定 | pass | 截图、diff、overlay、辅助审查均完成，主线程判定通过 |

## 主线程观察摘要

- 画布为 1920x1080 深色蓝图网格。
- 顶部标题为 `柱状/排行图 / component-bar-ranking-chart`。
- 组件板不展示完整 AppShell。
- 左侧主视觉面板包含六条横向排行 bar。
- 六条 bar 为资产、外联、规则、证据、模型、慢查。
- 数值分别为 76、64、51、43、29、18。
- 第一条资产为红色高风险语义。
- 第二条外联为黄色或琥珀色待确认语义。
- 第三条规则为蓝色信息语义。
- 第四条证据为青色选中或强调语义。
- 第五条模型为绿色健康语义。
- 第六条慢查为紫色低优先级或次级语义。
- 右侧排行表包含表头 `排名`、`对象`、`风险`。
- 表格行 1 为 `1 / 10.20.3.8 / 高`。
- 表格行 2 为 `2 / ja3:ab7 / 中`。
- 表格行 3 为 `3 / rule-17 / 中`。
- 右侧状态矩阵与组件规范保持一致。
- 底部语义区包含尺寸、状态、动作、数据、审计、边界六张卡。

## 辅助智能体 Erdos 查漏摘要

- Erdos 确认页面结构为顶部标题区、中部主视觉和状态矩阵、底部语义区。
- Erdos 确认条形排行为 `资产/76`、`外联/64`、`规则/51`、`证据/43`、`模型/29`、`慢查/18`。
- Erdos 确认排行表为 `1 10.20.3.8 高`、`2 ja3:ab7 中`、`3 rule-17 中`。
- Erdos 确认右侧状态矩阵包含 Hover、Selected、Loading、Empty、Warning、Error、Locked；第一项主线程按目标图校正为中文 `正常`。
- Erdos 提醒生产实现不能只靠颜色表达含义，应补充 tooltip、数值文本、aria label 和键盘 focus。
- Erdos 提醒图表和排行表必须同源，避免排序、风险等级和数值不一致。
- Erdos 提醒窄屏下左侧图表、表格和右侧矩阵容易挤压，生产实现需响应式堆叠。
- Erdos 提醒目标图未展示时间范围、单位、刷新频率、drilldown 入口、真实 loading/empty/error 占位和审计写入字段。

## 主线程判断口径

- 目标 PNG 像素复刻必须保留当前六条 bar 的顺序、宽度比例、颜色和数值。
- 目标 PNG 像素复刻必须保留排行表三列三行。
- 目标 PNG 像素复刻必须保留右侧状态矩阵和底部六卡。
- 生产实现可增加 ECharts tooltip、选择联动、风险 badge、点击跳转和审计字段，但不作为目标 PNG 复刻差异。
- 如果 Windows Chrome implementation 与 target 的 mismatch ratio 为 `0.0`，并且 overlay 与坐标记录一致，则主线程可判定本图 pixel-accepted。
- 若 evidence 中出现缺图、截图不是 Windows Chrome、视口不是 1920x1080、存在滚动条、diff 有异常高亮或 console/page error，则主线程不能判定通过。

## 当前结论

- 当前状态：`pixel-accepted`
- URL：`http://10.0.5.8:40083/evidence/ui-image-breakdowns/components/component-bar-ranking-chart/implementation.html`
- 浏览器：`Chrome/150.0.7871.47`，`Windows Chrome CDP`
- 视口：`1920x1080`，devicePixelRatio `1`
- diff：mismatch ratio `0.0`
- 主线程判定：target、implementation、diff、regions-overlay、capture-meta 和辅助审查均满足验收门。
- 本 review 只服务当前单张图片，不合并其它组件板结论。
