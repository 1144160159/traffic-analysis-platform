# Data Quality DLQ Dry-run Web UI 证据

日期：2026-06-29

## 范围

本轮把 `/data-quality?tab=replay-reconcile` 的 DLQ fallback replay 从 action contract 展示推进为可操作 dry-run modal。页面默认使用 `dry_run=true`，要求审批人、审批单号、幂等键、重放原因和修复摘要，提交时调用 `POST /v1/dlq/replay/fallback`。

## 代码与部署

- Web UI 页面：`web/ui/src/pages/DataQualityPage.tsx`
- Web UI 样式：`web/ui/src/styles/pages.css`
- Replay API client：`web/ui/src/services/dlqReplayApi.ts`
- 生产覆盖镜像 Dockerfile：`web/ui/deployments/Dockerfile.overlay`
- Live 部署镜像：`traffic/web-ui:dq-dlq-dryrun-20260629-r2`

`Dockerfile.overlay` 同步修正为写入 `/etc/nginx/nginx.conf.template`，避免 entrypoint 启动时找不到模板。

## 已通过验证

| 检查 | 结果 | 证据 |
|---|---|---|
| Vitest 定向测试 | 3 个文件、13 项通过 | `web-ui-vitest-dlq-dryrun.txt` |
| Web build | 通过，保留既有 Vite 大 chunk warning | `web-ui-build-dlq-dryrun.txt` |
| Web UI 镜像 r2 nginx 配置 | 通过 | `web-ui-nginx-test-r2.txt` |
| 双节点 image import | 完成 | `web-ui-import-local-r2.txt`、`web-ui-import-10.0.5.9-r2.txt` |
| K8s rollout | `deployment "web-ui" successfully rolled out` | `web-ui-rollout-recheck-r2.txt` |
| Live SPA route | APISIX `/data-quality?tab=replay-reconcile` 返回 200 | `live-data-quality-page-head.txt` |
| Live API | 临时 JWT 访问 `/api/v1/data-quality` 返回 200 | `live-data-quality-api.http.txt`、`live-data-quality-api.json` |
| Live bundle markers | 部署 bundle 包含 modal、按钮、class 和 replay endpoint | `live-bundle-dlq-markers.txt` |
| Desktop Chrome bridge | Chrome extension backend 可打开 live 登录页 | `browser-smoke-desktop.json` |

本轮没有提交真实 DLQ replay 请求。

## 浏览器限制

Codex Desktop Chrome extension backend 可以打开 live 登录页，也能读取 DOM；但当前桥接的 evaluate 执行世界里 `window.localStorage` 不可用，无法注入临时 JWT 进入受保护业务页。`javascript:` URL 注入也未执行。高端口 Vite/static 预览在 Desktop Chrome 导航中超时，因此本轮没有把 live 业务页 modal 点击视为已闭环。

## 未关闭项

- live Kafka/Flink 坏消息注入、影响范围说明和合法登录态下的 Desktop Chrome `/data-quality` 业务页巡检。真实 fallback 文件重放、跨 Pod 幂等和失败样本回归已转入 `../20260629-dlq-replay-recovery/`、`../20260629-dlq-replay-failure-regression/`。
