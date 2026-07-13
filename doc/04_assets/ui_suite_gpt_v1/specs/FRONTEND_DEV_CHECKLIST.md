# 前端开发 Checklist

## 开发前

- 读取 `FRONTEND_TASK_MATRIX.md`，确认所在批次和前置依赖。
- 读取 `page-contracts/<id>.md` 和 `layers/<id>.json`。
- 打开对应目标 PNG，对照 `app-shell.json` 和 `tokens.json`。
- 检查 `route-page-map.json` 中的 React 页面、API endpoint 和契约路径。
- 检查关联 `overlay-contracts/`，不要漏掉行操作、批量操作和详情抽屉。
- 按 `FULL_STACK_PAGE_DELIVERY_WORKFLOW.md` 确认 API 契约、后端 owner、数据库表、seed scenario 和业务/视觉双轨验收用例。

## 开发中

- 先补 service/hook，再写页面视图。
- 表格、图表、KPI、时间线、证据卡片优先复用 `component-map.json` 对应组件。
- 页面状态至少覆盖 loading、error、empty；鉴权页覆盖 401/403。
- 危险操作先做不可误触状态，再接入真实动作。
- AppShell 公共区不要在单页内重写。
- 正常生产路由只使用 live/seeded 数据；视觉仿真模式不得用于证明 API、分页、权限和审计。
- 不得复制 API 行伪造分页，不得以 `setTimeout` 或 sessionStorage 成功提示替代服务端动作。

## 提交前

```bash
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
cd web/ui && npm run build
```

浏览器验收页面时需确认：

- 无 4xx/5xx API response。
- 无 requestfailed。
- 无 pageerror 或非 warning console error。
- 1920x1080 截图公共区与目标 AppShell 参数一致。
- 相关浮层能从页面真实入口触发。
- 正常生产路由的分页发出真实 offset/page 参数，动作响应可从服务端审计或任务记录复核。
- 数据库 seed 可重复执行，并能覆盖第二页、空筛选、异常状态和图表时间序列。
