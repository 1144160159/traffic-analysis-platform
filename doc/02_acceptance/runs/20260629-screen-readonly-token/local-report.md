# Screen Readonly Token Report

日期：2026-06-29
对象：`/screen` 生产保护模式、`screen:view` 只读权限、真实 APISIX `http://10.0.5.8:30180`。

## 范围

本次关闭 `/screen` 只读 token 首轮后端/网关校验缺口。此前前端 routeManifest 已将 `/screen` 标为 `protected + readonly + screen:view`，但后端 scope catalog 未把 `screen:view` 纳入一等权限。本轮补齐 auth-service scope 模型并滚动 live 镜像，随后用只含 `screen:view` 的短期 JWT 做真实 APISIX API + Playwright 矩阵。

新增/修改：

```text
go/control-plane/internal/auth/model/scopes.go
go/control-plane/internal/auth/model/user.go
go/control-plane/internal/auth/model/scopes_test.go
tests/e2e/live_screen_readonly_matrix.sh
deployments/kubernetes/applications/go-services.yaml
```

live 镜像：

```text
traffic/auth-service:screen-scope-20260629-r1
```

## 命令

```text
go test ./internal/auth/model ./internal/auth/service ./cmd/auth-service
npm run test -- --run src/routes/routeManifest.test.ts src/services/pageApiPlans.test.ts src/routes/access.test.ts
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl apply --dry-run=client -f deployments/kubernetes/applications/go-services.yaml
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl -n traffic-analysis rollout status deployment/auth-service --timeout=180s
LOG_DIR=/tmp/screen-readonly-20260629020953 tests/e2e/live_screen_readonly_matrix.sh
LOG_DIR=/tmp/perm-matrix-after-screen-20260629021014 tests/e2e/live_permission_matrix.sh
LOG_DIR=/tmp/realtime-after-screen-20260629021014 tests/e2e/live_realtime_appshell.sh
```

Desktop Chrome wrapper 烟测：

```text
desktop_chrome_open_url(url="http://10.0.5.8:30180/screen", keep=true, wait_ms=2500)
```

## 结果

| 层级 | 通过 | 失败 | 结论 |
|---|---:|---:|---|
| auth model / compile | 3 packages | 0 | `screen:view` 是合法 scope，auth-service 可编译 |
| Web route contracts | 14 tests | 0 | `/screen` routeManifest/access/page plan 契约未漂移 |
| screen readonly live matrix | 8 | 0 | 只读大屏 token 可看 `/screen`，不能访问敏感设置或写接口 |
| permission matrix regression | 11 | 0 | 原 anonymous/admin/viewer 权限矩阵不回退 |
| realtime appshell regression | 2 | 0 | 实时通道仍自动连接成功 |
| Desktop Chrome wrapper smoke | 1 | 0 | 匿名 `/screen` 被生产登录门禁拦截 |

只读矩阵关键断言：

```text
run_id=20260629020953-4136661 total=8 passed=8 failed=0
scope catalog includes screen:view -> 200
screen token /api/v1/auth/me -> 200, permissions only include screen:view
screen token /api/v1/dashboard/stats -> 200
screen token /api/v1/tokens -> 403
screen token PUT /api/v1/alerts/{id}/status -> 403, message contains alert:write
screen token /screen -> renders 园区数字孪生拓扑 and 真实 API
screen token /settings -> 权限不足, required admin:* / token:read
screen token /screen navigation excludes 系统设置 / 规则管理 / 模型管理
```

原始摘要：

```text
/tmp/screen-readonly-20260629020953/live-screen-readonly-matrix-20260629020953-4136661-summary.json
/tmp/perm-matrix-after-screen-20260629021014/live-permission-matrix-20260629021014-4138582-summary.json
/tmp/realtime-after-screen-20260629021014/live-realtime-appshell-20260629021014-4138586-summary.json
```

## 边界

- 本轮证明 `screen:view` 首轮只读访问边界，不替代 token 轮换、撤销、过期和跨租户细矩阵。
- 业务 API 的 `Authorization: Bearer` middleware 仍接受 JWT access token；API Token 目录中的 `screen:view` 用于 scope catalog、令牌治理和后续正式令牌流程，不改变现有 JWT 认证路径。
- `/screen` 产品化大屏的一屏闭环、4K/2K/1080p 专项巡检和沙箱常驻 preview 仍属于后续 UI/验收工作。
