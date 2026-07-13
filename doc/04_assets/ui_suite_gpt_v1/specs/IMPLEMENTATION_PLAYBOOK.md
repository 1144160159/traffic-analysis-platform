# UI Suite 实现手册

本目录把 manifest 中的 181 个 UI 契约项转换为实现契约。页面开发必须以这里的 JSON/Markdown 为入口，而不是只凭 PNG 目测实现；业务页面的完整交付同时遵循 `FULL_STACK_PAGE_DELIVERY_WORKFLOW.md`。

## 执行顺序

1. 先实现 `tokens.json` 和 `app-shell.json`：统一顶部栏、左侧单栏、底部栏、颜色、字号、密度。
2. 再实现 `component-map.json`：把 48 张 component 图映射到 Ant Design、ECharts 和现有 React 组件。
3. 按 `route-page-map.json` 逐页实现 27 张页面主图和 `/topics` 合并页。
4. 按 `overlay-contracts/` 实现 70 张 Modal/Drawer/Dropdown/Popconfirm。
5. 按 `visual-acceptance.json` 做 Playwright 截图回归和业务状态验收。
6. 每轮开发前后运行 `validate_frontend_contracts.mjs`，确保契约、图、prompt、trace、路由映射未断链。
7. 运行 `build_frontend_handoff.mjs` 生成任务矩阵、业务链路验收和开发 checklist，用于派工和 PR 验收。
8. 运行 `build_frontend_code_gap.mjs` 对比当前 `web/ui` 与契约，生成代码缺口报告和修复队列。
9. 按 `FULL_STACK_PAGE_DELIVERY_WORKFLOW.md` 补齐真实后端、数据库 seed、正常生产路由验收和独立审查；视觉 pipeline 不能单独代表页面完成。

## 前端代码落点

- 路由：`web/ui/src/routes/routeManifest.tsx`
- AppShell：`web/ui/src/layouts/AppShell.tsx`
- 全局样式：`web/ui/src/styles/tokens.css`、`web/ui/src/styles/app-shell.css`
- 页面：`web/ui/src/pages/*.tsx`
- 组件：`web/ui/src/components/*.tsx`
- API：`web/ui/src/services/*.ts`

## 不可偏离项

- 不允许恢复双栏左侧导航。
- 不允许把通知、用户、设置、电源放到顶部栏。
- 不允许页面组件直接 `fetch`；必须走 `services/api.ts` 或既有 service 封装。
- 不允许忽略 loading/error/empty/401/403。
- 不允许危险动作缺少影响范围、权限提示和审计留痕。

## 每页前端阶段完成定义

- 路由可访问，权限与 `requiredScopes` 一致。
- 页面 AppShell 与 `app-shell.json` 对齐。
- 主工作区实现 `page-contracts/<id>.md` 的业务层。
- API endpoint 与 `route-page-map.json` 对齐。
- 关联浮层全部可触发。
- Windows Chrome 截图与目标图做视觉 diff，业务 ROI 严格 `<0.125`。

前端阶段完成不等于业务页面完成。页面最终状态必须满足 `FULL_STACK_PAGE_DELIVERY_WORKFLOW.md` 的 `full-stack-accepted` 定义。

## 契约自检

```bash
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
```

## 继续梳理交付物

- `FRONTEND_TASK_MATRIX.md`：批次、页面、路由、API、浮层依赖。
- `BUSINESS_FLOW_ACCEPTANCE.md`：从登录到业务闭环的验收链路。
- `FRONTEND_DEV_CHECKLIST.md`：前端开发前、中、提交前 checklist。
- `FRONTEND_IMPLEMENTATION_METHODS.md`：分层、组件、路由/API、截图回归、人工标注 5 种方法。
- `FRONTEND_CODE_GAP.md`：当前前端代码与 UI 契约的静态差距。
- `FRONTEND_FIX_QUEUE.md`：按优先级排序的前端修复队列。
- `FULL_STACK_PAGE_DELIVERY_WORKFLOW.md`：全栈阶段、双数据模式、证据矩阵和最终完成定义。
