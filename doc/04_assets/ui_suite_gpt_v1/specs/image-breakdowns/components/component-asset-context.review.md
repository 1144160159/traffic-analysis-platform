# component-asset-context.png 主线程审查记录

## 审查范围

- 目标图：`doc/04_assets/ui_suite_gpt_v1/screens/components/component-asset-context.png`
- 拆解记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/components/component-asset-context.md`
- 结构化记录：`doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/components/component-asset-context.json`
- 证据目录：`evidence/ui-image-breakdowns/components/component-asset-context/`

## 主线程检查

| 检查项 | 结论 | 证据 |
|---|---|---|
| 逐张处理 | pass | 本记录只覆盖 `component-asset-context` |
| 目标图直接视觉读取 | pass | 已直接查看目标 PNG，按顶部、资产关系图、资产字段表、状态矩阵、底部语义区拆解 |
| prompt/layer 读取 | pass | 已读取 prompt 和 `specs/layers/component-asset-context.json` |
| 坐标测量 | pass | 已记录画布、标题、五个关系图节点、七条连线、字段表、状态矩阵、语义卡片等 bbox |
| OCR/文本校正 | pass | 已按目标图人工校正节点、字段表、状态、规则和底部卡片文字 |
| 组件/元素/图标确认 | pass | 已覆盖 AssetContextGraph、AssetGraphNode、AssetGraphEdge、AssetFieldTable、StateMatrixItem、StatusDot、SemanticsTile |
| token 提取 | pass | 已覆盖背景、网格、面板、节点色、连线色、状态色、圆角、字段表行高和组件网格 |
| 交互状态拆解 | pass | 已覆盖节点点击、连线 hover、字段表 loading/empty/error、状态矩阵和危险确认语义 |
| reference-raster 范围说明 | pass | 已注明像素验收只证明目标 PNG 复刻，不声明生产 React/ECharts 语义实现完成 |
| 辅助智能体审查 | pass | Wegener 已完成只读查漏，结论已纳入本 review |
| overlay 生成 | pass | 已回看 `regions-overlay.png`，覆盖标题、图谱、字段表、状态矩阵和底部语义卡 |
| Windows Chrome 截图 | pass | 已使用 `Windows Chrome CDP` 截取 `implementation.png`，视口 `1920x1080` |
| 视觉 diff | pass | `metrics.json` 显示 mismatch ratio `0.0`，`diff.png` 无异常高亮 |
| 主线程最终判定 | pass | 截图、diff、overlay、辅助审查均完成，主线程判定通过 |

## 辅助智能体 Wegener 查漏摘要

- Wegener 确认布局为顶部标题区、左侧主面板、右侧状态矩阵和底部语义卡。
- Wegener 确认可见节点为 `Probe`、`Kafka`、`Flink`、`CH`、`Graph`。
- Wegener 确认节点语义：Probe/Kafka 蓝色信息态，Flink 绿色健康态，CH 黄色待确认态，Graph 紫色强调态。
- Wegener 确认可见连线为 Probe-Kafka、Probe-CH、Kafka-CH、Kafka-Flink、Kafka-Graph、Flink-CH、Flink-Graph。
- Wegener 确认资产字段表为 `资产 server-12`、`业务 统一认证`、`风险 高`、`开放端口 443/8080`。
- Wegener 提醒 `禁用` 在说明中出现，但状态矩阵用 `Locked` 表达，生产组件需明确 Disabled 与 Locked 的关系。
- Wegener 提醒关系图连线没有方向箭头，资产流向需要依赖上下文推断。
- Wegener 提醒节点缺少图例，颜色语义依赖右侧状态矩阵间接推断。
- Wegener 提醒 `Graph` 紫色未在状态矩阵中定义，生产设计需补充 graph emphasis token。
- Wegener 提醒字段表缺少 owner、region、ip、env、trace_id/request_id 等定位和审计字段。
- Wegener 提醒风险为 `高`，但节点没有同步红色高危强调，生产组件需明确风险映射策略。

## 主线程判断口径

- 目标 PNG 像素复刻必须保留当前五节点关系图、七条连线、字段表四行和右侧状态矩阵。
- Wegener 提出的方向箭头、图例、Graph token、审计字段和风险映射属于生产组件增强建议，不作为目标 PNG 复刻差异。
- 如果 Windows Chrome implementation 与 target 的 mismatch ratio 为 `0.0`，并且 overlay 与坐标记录一致，则主线程可判定本图 pixel-accepted。
- 若 evidence 中出现缺图、截图不是 Windows Chrome、视口不是 1920x1080、存在滚动条、diff 有异常高亮或 console/page error，则主线程不能判定通过。

## 当前结论

- 当前状态：`pixel-accepted`
- URL：`http://10.0.5.8:40377/evidence/ui-image-breakdowns/components/component-asset-context/implementation.html`
- 浏览器：`Chrome/150.0.7871.47`，`Windows Chrome CDP`
- 视口：`1920x1080`，devicePixelRatio `1`
- diff：mismatch ratio `0.0`
- 主线程判定：target、implementation、diff、regions-overlay、capture-meta 和辅助审查均满足验收门。
- 本 review 只服务当前单张图片，不合并其它组件板结论。
