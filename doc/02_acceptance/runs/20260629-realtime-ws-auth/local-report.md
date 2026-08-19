# Realtime WebSocket 后端鉴权契约

Run: `20260629-realtime-ws-auth`
Time: `2026-06-29T00:23:14+08:00` (`2026-06-28T16:23:14Z`)
Commit baseline: `e3316aec4ac1d6592e28aefc86853128ecde7408`

## 背景

`doc/05_status/未开发项梳理-2026-06-19.md` 将“路由权限 Manifest、`/screen` 边界、启动态鉴权”列为 P0 已开发但未闭环项，其中 WebSocket 前端已做到默认关闭、授权后连接，但缺后端/网关鉴权契约和 live 负例证据。本轮补齐最小 WebSocket 后端鉴权链路，不打开生产自动实时连接。

## 修改摘要

- `alert-service` 新增 `/ws` 与 `/ws/events` WebSocket endpoint。
- WebSocket upgrade 前必须提供 access token，支持 `Authorization: Bearer ...` 或浏览器 WebSocket query `token=...`。
- 缺 token、坏 token、非 access token、`tenant_id` mismatch 均在 upgrade 前拒绝。
- 有效 token upgrade 后返回 `ready` 消息，包含 tenant/user/server_time，用于前端确认连接上下文。
- APISIX 新增 `/ws* -> alert-service.traffic-analysis.svc:8082`，并启用 `enable_websocket:true`，放在 SPA catch-all 前。
- Web UI runtime 默认 `WS_URL` 调整为 `/ws/events`，route manifest 的 dashboard API hints 记录 `/ws/events`。
- `ENABLE_REALTIME` 生产仍为 `false`，避免未完成合法登录态可视巡检前自动连接。
- `alert-service` auth PostgreSQL 连接改为支持 Secret-backed `AUTH_POSTGRES_*` 分离环境变量，K8s 从 `traffic-credentials/PG_PASSWORD` 注入密码，不在清单落明文 DSN。
- live 已滚动：
  - `traffic/alert-service:realtime-ws-20260629-r1`
  - `traffic/web-ui:realtime-ws-20260629-r1`

## 验证结果

- `cd go/control-plane && go test ./internal/alert/config ./internal/alert/realtime ./cmd/alert-service`
  - 结果：通过。
  - 验证点：Auth DSN 组装和 URL 转义；WebSocket 缺 token/坏 token/tenant mismatch 拒绝；有效 token upgrade 并收到 `ready`。
- `cd web/ui && npm run test -- --run src/services/realtime.test.ts src/routes/routeManifest.test.ts`
  - 结果：通过，2 files / 9 tests。
  - 验证点：前端 realtime URL 构造和 route manifest `/ws/events` 契约。
- `cd web/ui && npm run build`
  - 结果：通过。
  - 验证点：TypeScript 和生产构建通过；仅保留既有 large chunk warning。
- APISIX 内层 YAML 解析
  - 结果：通过。
  - 验证点：`/ws*` route 存在，`enable_websocket:true`，upstream 为 `alert-service.traffic-analysis.svc:8082`。
- K8s dry-run
  - 结果：通过。
  - 验证点：`alert-service` Service/Deployment、`web-ui`、`apisix-routes` 均通过 `kubectl apply --dry-run=client`。
- 镜像构建与导入
  - 结果：通过。
  - 验证点：alert image `sha256:15ff74557ecc8d83a8edaed1b31cac973e6f2e3e408bb0d1963d10867af9f33e`；web image `sha256:d7460b17e0cde715b64329cb52a0f20baa87e0927e4a23662d8fea9413e28c85`；两节点 containerd 均已导入。
- K8s live rollout
  - 结果：通过。
  - 验证点：`deployment/alert-service` 使用 `traffic/alert-service:realtime-ws-20260629-r1`，`deployment/web-ui` 使用 `traffic/web-ui:realtime-ws-20260629-r1`，APISIX statefulset 滚动完成。
- live 启动和 runtime 配置
  - 结果：通过。
  - 验证点：alert-service 日志包含 `Connected to PostgreSQL for auth`、`Auth middleware initialized`、`Realtime WebSocket endpoint registered`；`config.js` 输出 `WS_URL: "/ws/events"`、`ENABLE_REALTIME: "false"`。
- live WebSocket 负例
  - `GET http://10.0.5.8:30180/ws/events` 返回 `401 token required`。
  - `GET http://10.0.5.8:30180/ws/events?token=bad-token` 返回 `401 invalid or expired token`。
- live WebSocket 正例/tenant 负例
  - 使用 `JWT_SECRET` 临时签发 5 分钟 access token，不打印明文 token。
  - `tenant_id=tenant-b` 返回 WebSocket `unexpected-response 403`。
  - `tenant_id=campus-a` 成功 upgrade，并收到：
    - `type: ready`
    - `tenant_id: campus-a`
    - `username: codex-realtime-smoke`
- Codex Desktop Chrome bridge
  - `desktop_chrome_open_url(http://10.0.5.8:30180/login)`：通过，标题为“园区网络全流量采集与分析系统”。
  - `desktop_chrome_open_url(http://10.0.5.8:30180/dashboard)`：通过打开但最终 URL 为 `/login`，说明生产登录门禁仍生效。

## 未覆盖

- 生产 `ENABLE_REALTIME` 仍为 `false`，本轮没有让浏览器 AppShell 自动连接 WebSocket。
- live 正例使用短期签名 access token，没有在 Desktop Chrome 中提交登录表单。
- Desktop Chrome 仅验证前端入口和登录门禁；完整业务页可视巡检仍需要合法登录态。
- 本轮未运行 `tests/run_tests.sh full`、Java/Flink 全量测试、proto lint 或 100 轮 live smoke。
