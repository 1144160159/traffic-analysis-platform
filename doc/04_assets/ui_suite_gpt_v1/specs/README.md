# UI Suite 前端实现契约包

本目录由 `build_frontend_contracts.mjs` 从 `manifest.json`、现有 UI 图和前端代码约束生成，用于指导 React + Ant Design + ECharts 前端准确实现 manifest 中的 181 个 UI 契约项，并配合 `image-breakdowns/` 覆盖 `screens/` 下全部 canonical PNG。

逐图拆解要求：`screens/` 下每一张页面、浮层、组件、状态或响应式图片都必须有独立拆解记录。拆解记录只能逐张新增或更新，禁止一次性批量生成全量清单。

## 入口文件

- `IMPLEMENTATION_PLAYBOOK.md`：前端执行顺序和完成定义。
- `FULL_STACK_PAGE_DELIVERY_WORKFLOW.md`：逐页面前端、后端、数据库仿真、生产浏览器和视觉验收的统一流程与完成定义。
- `REINFORCEMENT_LEARNING_DELIVERY_LOOP.md`：产品业务、功能完备、测试、视觉和生产性能的受约束强化学习闭环、奖励协议与经验回放规则。
- `reinforcement-learning-policy.json`、`learning-episode.schema.json`：强化学习策略和单次学习 episode 的机器可读契约。
- `tokens.json`：颜色、字号、密度、AppShell 坐标参数。
- `app-shell.json`：公共顶部栏、左侧菜单和底部栏契约。
- `route-page-map.json`：路由、页面组件、API、目标图和契约文件映射。
- `component-map.json`：48 张组件板到前端组件/Ant Design/ECharts 的映射。
- `visual-acceptance.json`：Playwright 与视觉回归验收规则。
- `frontend-delta.md`：当前前端 token 与 UI 图目标参数差异。
- `FRONTEND_TASK_MATRIX.md`：前端批次、页面、API、浮层派工矩阵。
- `BUSINESS_FLOW_ACCEPTANCE.md`：关键业务闭环验收链路。
- `FRONTEND_DEV_CHECKLIST.md`：逐页开发与提交前检查清单。
- `FRONTEND_IMPLEMENTATION_METHODS.md`：准确实现 UI 图的 5 种方法。
- `FRONTEND_CODE_GAP.md`：当前前端代码与 UI 契约的静态差距。
- `FRONTEND_FIX_QUEUE.md`：按优先级排序的前端修复队列。
- `PIXEL_PERFECT_IMAGE_BREAKDOWN_PLAN.md`：逐张精拆、Windows Chrome 截图、视觉 diff 和差异清零的 100% 复刻验收方案。
- `image-breakdowns/`：逐图拆解记录；每个 PNG 对应一个 Markdown 和可选 JSON，不允许用批量汇总替代。
- `page-contracts/`：逐页面开发契约。
- `overlay-contracts/`：逐浮层开发契约。
- `layers/`：181 个 manifest 契约项的机器可读分层 JSON。

## 重新生成

```bash
node doc/04_assets/ui_suite_gpt_v1/build_frontend_contracts.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_handoff.mjs
node doc/04_assets/ui_suite_gpt_v1/build_frontend_code_gap.mjs
```

## 逐图拆解

先生成待办索引；该索引只用于排队和统计，不是拆解记录：

```bash
node doc/04_assets/ui_suite_gpt_v1/build_pixel_breakdown_queue.mjs
```

每次只允许启动一张图片的精拆记录：

```bash
node doc/04_assets/ui_suite_gpt_v1/start_pixel_image_breakdown.mjs --image doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-color-status.png
```

产物固定为：

- `doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/<分类>/<图片ID>.md`
- `doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/<分类>/<图片ID>.json`
- `doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/<分类>/<图片ID>.review.md`
- `evidence/ui-image-breakdowns/<分类>/<图片ID>/`

## 全流程门禁

业务页面开发先遵循 `FULL_STACK_PAGE_DELIVERY_WORKFLOW.md`。逐图 pipeline 只负责视觉链路，不能替代真实 API、数据库、权限、分页、动作和审计验收。

拆解记录门禁只判断每张图是否有完整的视觉拆解记录，不判断实现截图和 pixel diff：

```bash
node doc/04_assets/ui_suite_gpt_v1/validate_image_breakdown_records.mjs
```

状态文件：

- `doc/04_assets/ui_suite_gpt_v1/specs/image-breakdown-record-status.json`
- `doc/04_assets/ui_suite_gpt_v1/specs/IMAGE_BREAKDOWN_RECORD_STATUS.md`

逐图记录不能停在 `review-ready`。每张图都必须继续推进到实现、Windows Chrome 截图、视觉 diff 和最终状态：

```bash
node doc/04_assets/ui_suite_gpt_v1/validate_pixel_breakdown_pipeline.mjs
```

状态文件：

- `doc/04_assets/ui_suite_gpt_v1/specs/pixel-perfect-pipeline-status.json`
- `doc/04_assets/ui_suite_gpt_v1/specs/PIXEL_PERFECT_PIPELINE_STATUS.md`

只有 `pixel-accepted` 才表示该图完整走完全部流程；`review-ready`、`diff-pending`、`blocked` 都不是完成。

## 契约自检

```bash
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
```
