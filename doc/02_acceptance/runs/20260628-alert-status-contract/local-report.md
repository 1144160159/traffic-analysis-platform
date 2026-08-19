# 告警状态机前端契约收敛

Run: `20260628-alert-status-contract`
Time: `2026-06-28T22:53:53+08:00`
Commit baseline: `e3316aec4ac1d6592e28aefc86853128ecde7408`

## 背景

`agent.md` 和 `doc/05_status/项目开发内容全景梳理-2026-06-25.md` 将告警闭环列为 P0 业务链路之一。后端告警状态机已经收敛到 `new -> triage -> assigned -> closed`，但前端仍存在旧状态语义，例如 `processing`、`resolved`、`false_positive` 被当作生命周期状态展示。

本轮目标是收敛 Web UI 的告警状态契约：页面显示、筛选选项、快照适配与详情 mock 都以 `new / triage / assigned / closed` 为准；`false_positive` 只作为反馈结果，不再作为告警生命周期状态。同时在告警详情页补齐状态流转门禁，让 UI 按后端状态机禁用非法迁移，并在提交前要求填写状态变更原因。

## 修改摘要

- 新增 `web/ui/src/services/alertStatus.ts`，集中维护告警四态、中文标签、下拉选项和 legacy alias 归一化。
- `alertStatus` 增加与 Go 后端一致的状态迁移表：`new -> triage/closed`、`triage -> assigned/closed`、`assigned -> triage/closed`、`closed -> new`。
- `pageSnapshotAdapters` 在告警列表快照中先归一化状态，再生成 KPI 和表格状态标签。
- `AlertTriagePage` 状态筛选改为 `未处理 / 研判中 / 已指派 / 已关闭`。
- `AlertDetailPage` 新增“状态流转门禁”面板：当前态和非法迁移禁用，可迁移目标可选，状态变更原因少于 4 个字符时提交按钮禁用。
- `alertDetailApi` 新增 `updateAlertStatus`，生产态调用 `PUT /v1/alerts/{id}/status`，成功后刷新详情；前端不做乐观更新，失败时保留原快照。
- `AlertDetailPage` 与 `alertDetailApi` 去掉旧的 `processing` 默认状态，并把误报说明限定为反馈结果。
- `alert-service` 的单条和批量状态更新请求新增必填 `reason`，service 层状态审计写入 reason，并移除 handler 层对单条状态更新的重复审计写入。
- `alert-service` 的单条和批量状态更新在进入请求体验证和 service 调用前校验 `alert:write` 写权限；只读 `alert:read` 用户返回 `403`，`*`、`alert:*`、`admin:*` 与 `alert:write` 可通过。
- `AlertService.UpdateStatus` 严格使用 `go/control-plane/internal/alert/state` 的迁移表校验，避免 `closed -> closed` 这类状态机外迁移被放行。
- `common/audit` 新增带 detail 的 alert action 审计方法，并增加单元测试确认 reason 写入审计事件；`alert/api` 增加单元测试确认单条和批量状态更新缺少 reason 时在进入 service 前返回 4xx。
- 补充 Vitest 与 Playwright E2E，覆盖 legacy alias 到闭环状态的映射，以及 `/alerts` 筛选下拉的四态显示。

## 验证结果

- `npm run test -- --run src/services/alertStatus.test.ts src/services/alertDetailApi.test.ts src/services/pageSnapshotAdapters.test.ts src/routes/routeManifest.test.ts`
  - 结果：通过，`4 files passed, 31 tests passed`。
- `npm run lint:check`
  - 结果：通过。
- `npm run build`
  - 结果：通过。
  - 非阻断警告：Vite 提示 `vendor-antd-BJFyC555.js` 超过 500 kB。
- `gofmt -w internal/common/audit/logger.go internal/common/audit/logger_alert_action_test.go internal/alert/audit/logger.go internal/alert/api/handler_status_test.go && go test ./internal/common/audit ./internal/alert/...`
  - 结果：通过。
- `gofmt -w go/control-plane/internal/alert/api/handler_permissions.go go/control-plane/internal/alert/api/handler.go go/control-plane/internal/alert/api/handler_status_test.go && cd go/control-plane && go test ./internal/alert/api ./internal/alert/...`
  - 结果：通过。
  - 验证点：状态更新缺少 `alert:write` 时返回 `403`，且不进入 service；`* / alert:* / admin:* / alert:write` 均允许进入后续校验。
- `VITE_USE_MOCK=true VITE_AUTH_ENABLED=false VITE_API_BASE_URL=http://10.0.5.8:4198 npm run dev -- --host 0.0.0.0 --port 4198`
  - 结果：预览服务可用，`curl --noproxy '*' -I http://127.0.0.1:4198/alerts` 返回 `HTTP/1.1 200 OK`。
- `PLAYWRIGHT_BASE_URL=http://127.0.0.1:4198 npx playwright test e2e/product-navigation.spec.ts -g "alerts expose backend canonical status options" --project=chromium --reporter=line`
  - 结果：通过，`1 passed (1.4s)`。
  - 验证点：`/alerts` 状态筛选下拉出现 `未处理 / 研判中 / 已指派 / 已关闭`，且不出现旧的 `处理中 / 已确认 / 已忽略`。
- `PLAYWRIGHT_BASE_URL=http://127.0.0.1:4198 npx playwright test e2e/product-navigation.spec.ts -g "alert detail renders evidence" --project=chromium --reporter=line`
  - 结果：通过，`1 passed (1.9s)`。
  - 验证点：`/alerts/AL-20260620-000123` 在 `研判中` 当前态下禁用 `未处理` 和当前态，允许 `已指派 / 已关闭`，未填写原因时不能提交状态变更，填写原因并提交后出现成功提示。

## Desktop Chrome 证据边界

已使用 Codex Desktop Chrome extension bridge 验证真实部署入口：

- `desktop_chrome_open_url` 打开 `http://10.0.5.8:30180/login` 成功，页面标题为 `园区网络全流量采集与分析系统`。
- 按 `agent.md` 要求，没有提交登录表单。

本轮 Desktop Chrome 无法稳定访问临时 Vite 预览端口：

- `http://10.0.5.8:4198/alerts` 通过 Desktop Chrome bridge 超时。
- `http://192.168.100.2:4198/alerts` 通过 Desktop Chrome bridge 超时。
- 使用 Desktop Node REPL 直连 Chrome extension 时，先发现技能记录的旧 client path 已漂移；定位到 Desktop 侧 `26.623.42026` 后，底层探针仍在桥调用层超时。
- 超时尝试留下的 `about:blank` 标签页已清理，已有用户标签页保持不动。

因此本轮浏览器证据由本地 Playwright 对临时 Vite 预览完成，Desktop Chrome 证据只证明桥可用与正式 APISIX 登录页可达，不声称已通过 Desktop Chrome 完成 Vite 页面断言。

## 未覆盖

- 未运行完整 live APISIX/API/DB 写路径验证。
- 未运行 `tests/run_tests.sh full`、`make python-test` 或 `ROUNDS=100 ... tests/run_tests.sh live`。
- 未验证登录后所有业务菜单的 live 数据闭环；本 run 仅关闭告警状态契约这个 P0 子项。
