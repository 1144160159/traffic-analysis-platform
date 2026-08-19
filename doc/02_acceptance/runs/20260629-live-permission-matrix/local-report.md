# Live Permission Matrix Report

日期：2026-06-29
对象：真实 APISIX 入口 `http://10.0.5.8:30180`、K8s Secret 签发的短期 JWT、生产态 Web UI。

## 范围

本次补齐 routeManifest / `/auth/me` / `/screen` 边界的真实权限矩阵证据。脚本从 `traffic-analysis/traffic-credentials` 读取 `JWT_SECRET`，只在内存中生成 admin/viewer 短期 token，不打印、不落盘 token 或 Secret。

新增脚本：

```text
tests/e2e/live_permission_matrix.sh
```

## 命令

```text
LOG_DIR=/tmp/perm-matrix-20260629015619 tests/e2e/live_permission_matrix.sh
npm run test -- --run src/routes/access.test.ts src/routes/routeManifest.test.ts
```

Desktop Chrome wrapper 烟测：

```text
desktop_chrome_open_url(url="http://10.0.5.8:30180/dashboard", keep=true, wait_ms=2500)
```

## 结果

| 层级 | 通过 | 失败 | 结论 |
|---|---:|---:|---|
| API RBAC | 6 | 0 | 匿名、admin、viewer 权限边界符合预期 |
| Playwright route matrix | 5 | 0 | 匿名登录门禁、admin 仪表盘、viewer 告警、viewer 设置 403、viewer 大屏均符合预期 |
| Web 权限契约单测 | 10 | 0 | `access.test.ts` 与 `routeManifest.test.ts` 通过 |
| Desktop Chrome wrapper smoke | 1 | 0 | 外部 Chrome 扩展桥打开 `/dashboard` 后落到 `/login`，标题和登录文案可读 |

脚本汇总：

```text
run_id=20260629015619-4081801 total=11 passed=11 failed=0
raw_report=/tmp/perm-matrix-20260629015619/live-permission-matrix-20260629015619-4081801.ndjson
raw_summary=/tmp/perm-matrix-20260629015619/live-permission-matrix-20260629015619-4081801-summary.json
```

## 关键断言

| 用例 | 断言 |
|---|---|
| anonymous `/api/v1/auth/me` | `401` |
| admin `/api/v1/auth/me` | `200`，permissions 包含 `*` |
| viewer `/api/v1/auth/me` | `200`，permissions 包含 `alert:read` |
| viewer `/api/v1/alerts?limit=1` | `200` |
| viewer `/api/v1/models?limit=1` | `401`，错误信息包含 `model:read` |
| viewer `PUT /api/v1/alerts/{id}/status` | `403`，错误信息包含 `alert:write` |
| anonymous `/dashboard` | 重定向 `/login`，显示登录页 |
| admin `/dashboard` | 显示 `仪表盘`、`优先级待办队列`，不显示 `权限不足` |
| viewer `/alerts` | 显示 `告警队列`、`处置与反馈`，不显示 `权限不足` |
| viewer `/settings` | 显示 `权限不足`、`系统设置`、`admin:*`、`token:read` |
| viewer `/screen` | 显示 `园区数字孪生拓扑`，不显示 `权限不足` |

## 边界

本报告证明真实 APISIX + Web UI 的权限矩阵首轮闭环，不替代以下专项：

- `/screen` 只读 token 的后端/网关校验策略后续已由 `doc/02_acceptance/runs/20260629-screen-readonly-token/` 补齐首轮证据。
- 合法登录态业务页 WebSocket 自动连接的可视化巡检。
- 当前 Codex 沙箱内常驻 `npm run preview` 浏览器证据，仍受 `listen EPERM` 限制。
- 跨租户、token 轮换、过期 token、撤销 token 等更细 RBAC 负例矩阵。
