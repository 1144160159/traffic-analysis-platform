# Realtime AppShell Live Report

日期：2026-06-29
对象：真实 K8s `deployment/web-ui`、APISIX `http://10.0.5.8:30180`、alert-service `/ws/events`。

## 范围

本次关闭“合法登录态业务页 WebSocket 自动连接可视巡检”缺口。前序 `20260629-realtime-ws-auth` 已证明后端/网关鉴权正负例，本轮把 live Web UI runtime 打开为 `ENABLE_REALTIME=true`，并证明授权用户进入业务页后 AppShell 自动连接 `/ws/events`，页面可见状态为“实时通道 / 已连接”。

新增脚本：

```text
tests/e2e/live_realtime_appshell.sh
```

## Runtime

已同步源码清单和 live deployment：

```text
deployments/kubernetes/applications/web-ui.yaml
web/ui/deployments/kubernetes/deployment.yaml
```

live `deployment/web-ui` 当前关键环境变量：

```text
WS_URL=/ws/events
VITE_ENABLE_WEBSOCKET=true
ENABLE_REALTIME=true
AUTH_ENABLED=true
USE_MOCK=false
SCREEN_ACCESS_MODE=protected
```

APISIX 读取到的 `/config.js`：

```text
ENABLE_REALTIME: "true"
WS_URL: "/ws/events"
AUTH_ENABLED: "true"
USE_MOCK: "false"
```

## 命令

```text
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl apply --dry-run=client -f deployments/kubernetes/applications/web-ui.yaml
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl apply --dry-run=client -f web/ui/deployments/kubernetes/deployment.yaml
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl -n traffic-analysis set env deployment/web-ui VITE_ENABLE_WEBSOCKET=true ENABLE_REALTIME=true
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl -n traffic-analysis rollout status deployment/web-ui --timeout=180s
LOG_DIR=/tmp/realtime-appshell-20260629020212 tests/e2e/live_realtime_appshell.sh
LOG_DIR=/tmp/perm-matrix-after-realtime-20260629020236 tests/e2e/live_permission_matrix.sh
npm run test -- --run src/services/realtime.test.ts src/routes/access.test.ts src/routes/routeManifest.test.ts
```

Desktop Chrome wrapper 烟测：

```text
desktop_chrome_open_url(url="http://10.0.5.8:30180/dashboard", keep=true, wait_ms=2500)
```

## 结果

| 层级 | 通过 | 失败 | 结论 |
|---|---:|---:|---|
| runtime config | 1 | 0 | live `/config.js` 已启用实时通道 |
| authorized Playwright page | 1 | 0 | `/dashboard` 显示 `实时通道` / `已连接`，捕获 `/ws/events` ready 帧 |
| permission matrix regression | 11 | 0 | 实时通道打开后原权限矩阵仍通过 |
| Web contract tests | 13 | 0 | realtime/access/route manifest 单测通过 |
| Desktop Chrome wrapper smoke | 1 | 0 | 匿名 `/dashboard` 仍被登录门禁重定向到 `/login` |

Playwright 关键断言：

```text
run_id=20260629020212-4103941 total=2 passed=2 failed=0
url=http://10.0.5.8:30180/dashboard
hasDashboard=true
hasRealtimeConnected=true
websocket.path=/ws/events
websocket.hasToken=true
websocket.tenantId=default
ready.type=ready
ready.tenant_id=default
ready.username=codex-realtime-visual
consoleErrors=[]
pageErrors=[]
requestFailures=[]
serverErrors=[]
```

原始摘要：

```text
/tmp/realtime-appshell-20260629020212/live-realtime-appshell-20260629020212-4103941-summary.json
/tmp/perm-matrix-after-realtime-20260629020236/live-permission-matrix-20260629020236-4105136-summary.json
```

## 边界

- 本轮证明合法登录态业务页自动连接和可视状态，不替代 token 轮换、撤销、过期和跨租户矩阵。
- Desktop Chrome wrapper 当前只用于匿名登录门禁烟测；合法登录态注入 token 的自动连接由 Playwright 完成，且打到真实 APISIX 与后端 `/ws/events`。
- 当前 Codex 沙箱内常驻 `npm run preview` 仍受 `listen EPERM` 限制，不作为本轮关闭项。
