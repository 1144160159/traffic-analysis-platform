# 告警 FP 反馈生成白名单草案验收

日期：2026-06-29

## 范围

- 后端：`POST /api/v1/alerts/{id}/feedback` 在 `label=FP` 且 `add_to_whitelist=true` 时生成 `whitelist` 草案，并在响应中返回 `whitelist_draft`。
- 数据契约：白名单条目新增 `status`、`source_alert_id`、`feedback_id`，草案状态为 `draft`；`IsWhitelisted`/子网匹配只认 `active`，草案不会直接影响后续检测过滤。
- 前端：告警详情页“反馈与学习”接入真实提交 API；顶部“加入白名单”会切换为 FP + 生成审批草案；成功响应可跳转 `/whitelist?source_alert=...&draft_id=...`。
- 部署：`traffic/alert-service:alert-feedback-whitelist-20260629-r1` 与 `traffic/web-ui:alert-feedback-whitelist-20260629-r2` 已滚动到 `traffic-analysis` 命名空间。

## 本地验证

- `go test ./internal/alert/api ./internal/alert/whitelist ./cmd/alert-service`
- `go test ./internal/alert/... ./cmd/alert-service`
- `npm run test -- --run src/services/alertDetailApi.test.ts src/services/pageSnapshotAdapters.test.ts`
- `npm run build`

## K8s 验证

- 构建镜像：
  - `traffic/alert-service:alert-feedback-whitelist-20260629-r1`
  - `traffic/web-ui:alert-feedback-whitelist-20260629-r2`
- 镜像导入调度节点 `zeus-server` containerd 后完成 rollout：
  - `kubectl -n traffic-analysis rollout status deployment/alert-service --timeout=30s`
  - `kubectl -n traffic-analysis rollout status deployment/web-ui --timeout=30s`
- 镜像核对：
  - `alert-service image=traffic/alert-service:alert-feedback-whitelist-20260629-r1 ready=1 updated=1 available=1`
  - `web-ui image=traffic/web-ui:alert-feedback-whitelist-20260629-r2 ready=1 updated=1 available=1`

## Live API 闭环

入口：`http://10.0.5.8:30180`

临时 JWT 只在脚本内生成，未打印明文 token。测试对真实告警产生一条短暂白名单草案，并在同一脚本中按草案 ID 删除清理。

```text
feedback_status=201
alert_id=alert-default-1782667923740-e8281305
draft_id=5121af60-9289-45a7-94cc-130d8e4999ad
delete_status=200
feedback_draft.status=draft
feedback_draft.type=ip
feedback_draft.value=cid:1:PrlJgF
feedback_draft.reason=FALSE_ALARM
feedback_draft.source_alert_id=alert-default-1782667923740-e8281305
feedback_draft.feedback_id=7bc68634-12a3-4f5a-80e7-449e95fc9960
feedback_draft.url=/whitelist?source_alert=alert-default-1782667923740-e8281305&draft_id=5121af60-9289-45a7-94cc-130d8e4999ad
```

验证点：

- `POST /api/v1/alerts/{id}/feedback` 返回 `201`，响应中含 `whitelist_draft`。
- `/api/v1/whitelist?limit=20` 可读到同一 `draft_id`，并带 `status=draft`、`source_alert_id`、`feedback_id`。
- 清理 `DELETE /api/v1/whitelist/{draft_id}` 返回 `200`；清理后 `/api/v1/whitelist?limit=20` 返回 `total=0`。
- alert-service 日志显示 `Feedback repository initialized` 与 `Whitelist management initialized`。

## UI 验证

Codex Desktop Chrome Bridge：

- 未登录打开 `http://10.0.5.8:30180/alerts/alert-default-1782667923740-e8281305` 正确重定向 `/login`。
- 当前 Desktop bridge 的 `evaluate/addInitScript/event` 能力不足，未能在 Chrome 扩展后端可靠注入临时 `localStorage` 登录态，因此完整受保护页交互用本地 Playwright 验证。

本地 Playwright 同一 K8s 页面：

```json
{
  "ok": true,
  "currentUrl": "http://10.0.5.8:30180/alerts/alert-default-1782667923740-e8281305",
  "hasFeedbackPanel": true,
  "hasSubmitFeedback": true,
  "hasWhitelistDraftControl": true,
  "hasStatusGate": true,
  "hasAlertId": true,
  "fpChecked": true,
  "draftChecked": true,
  "errors": [],
  "warnings": []
}
```

截图：`/tmp/codex-alert-detail-feedback-whitelist-playwright.png`

## 仍未关闭

- 完整 Playwright/live 权限矩阵仍未补。
