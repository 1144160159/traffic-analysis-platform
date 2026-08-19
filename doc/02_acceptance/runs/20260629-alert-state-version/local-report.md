# 告警状态 state_version 乐观锁验收

日期：2026-06-29

## 范围

- 后端：`/api/v1/alerts/{id}/status` 接收 `state_version` / `expected_state_version`，兼容 `If-Match` 数字版本，冲突返回 `409 BIZ_3005`。
- 服务层：单条告警状态更新在客户端版本与当前 `updated_at` 毫秒版本不一致时拒绝写入；ClickHouse 仓库层按 Unix 毫秒比较版本。
- 前端：`AlertDetailPage` 从详情快照读取 `stateVersion`，状态流转提交时携带 `state_version`。
- 部署：`traffic/alert-service:alert-state-version-20260629-r1`、`traffic/web-ui:alert-state-version-20260629-r1` 已滚动到 `traffic-analysis` 命名空间。

## 本地验证

- `go test ./internal/alert/api ./internal/alert/service ./internal/alert/state ./cmd/alert-service`
- `go test ./internal/alert/... ./cmd/alert-service`
- `npm run test -- --run src/services/alertStatus.test.ts src/services/alertDetailApi.test.ts src/routes/routeManifest.test.ts`
- `npm run build`

## K8s 验证

- `kubectl apply -f deployments/kubernetes/applications/go-services.yaml --dry-run=client`
- `kubectl apply -f deployments/kubernetes/applications/web-ui.yaml --dry-run=client`
- `kubectl rollout status deployment/alert-service -n traffic-analysis --timeout=180s`
- `kubectl rollout status deployment/web-ui -n traffic-analysis --timeout=180s`
- 镜像核对：
  - `alert-service traffic/alert-service:alert-state-version-20260629-r1 1/1`
  - `web-ui traffic/web-ui:alert-state-version-20260629-r1 1/1`

备注：整份 `go-services.yaml` apply 触发了非目标 `asset-service`、`graph-service`、`forensics-service` 的短暂 rollout，因 `:latest` 本地镜像拉取失败未完成。已执行 `kubectl rollout undo` 恢复到原稳定镜像，并清理残留失败 Pod；最终三者均为 `1/1 Running`。

## Desktop Chrome 门禁

通过 Codex Desktop Chrome Bridge 打开：

- `http://10.0.5.8:30180/alerts/alert-default-1782665411936-d4c54906`

读回结果：

- URL：`http://10.0.5.8:30180/login`
- Title：`园区网络全流量采集与分析系统`
- Body 首屏：`统一身份认证入口`、`账号密码登录`、`验证码`、`登 录`

结论：未登录访问告警详情深链仍被生产登录门禁拦截。

## Live no-op 证据

入口：`http://127.0.0.1:30180`

临时 JWT 只在脚本内生成，未打印明文 token。

```text
list_status=200
alert_id=alert-default-1782665411936-d4c54906
current_status=ALERT_STATUS_NEW
state_version=1782665411936
stale_version=1782665411935
target_status=assigned
stale_update_status=409
stale_error_code=BIZ_3005
stale_error_message=[BIZ_3005] alert state_version conflict: expected 1782665411935, actual 1782665411936
detail_after_status_code=200
detail_after_status=ALERT_STATUS_NEW
detail_after_state_version=1782665411936
```

结论：真实告警的 stale `state_version` 更新被 409 拦截，随后详情读取确认 `status` 与 `state_version` 均未变化，本轮只产生读请求和被拒绝写请求，无业务状态写入。

## 后续追踪

- 真实告警正向写入/回滚证据后续已由 `doc/02_acceptance/runs/20260629-alert-positive-audit/` 补齐。
- 批量状态更新的逐项 `state_version` 冲突契约后续已由 `doc/02_acceptance/runs/20260629-alert-batch-state-version/` 补齐。
- 审计查询回放后续已由 `doc/02_acceptance/runs/20260629-alert-positive-audit/` 补齐。
- FP 到白名单草案跳转后续已由 `doc/02_acceptance/runs/20260629-alert-feedback-whitelist/` 补齐。
- 完整 Playwright 权限矩阵后续已由 `doc/02_acceptance/runs/20260629-live-permission-matrix/` 补齐。
