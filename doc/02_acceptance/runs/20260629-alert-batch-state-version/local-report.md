# 告警批量状态 state_version 乐观锁验收

日期：2026-06-29

## 范围

- 后端：`PUT /api/v1/alerts/batch/status` 兼容原 `alert_ids`，新增 `items: [{alert_id, state_version}]`，逐项执行 `state_version` 乐观锁。
- 服务层：批量状态更新逐项调用带版本的状态机更新；冲突不覆盖、不中断其他项，在结果中返回 `failed_ids`、`errors`、`error_codes`。
- 前端：`AlertTriagePage` 的批量状态变更按钮接入真实 `/v1/alerts/batch/status`，从告警列表隐藏字段携带 `__stateVersion`；`alertStatus` 兼容 `ALERT_STATUS_*` 后端枚举。
- 部署：`traffic/alert-service:alert-batch-state-version-20260629-r1`、`traffic/web-ui:alert-batch-state-version-20260629-r1` 已滚动到 `traffic-analysis` 命名空间。

## 本地验证

- `go test ./internal/alert/api ./internal/alert/service ./internal/alert/state ./cmd/alert-service`
- `go test ./internal/alert/... ./cmd/alert-service`
- `npm run test -- --run src/services/alertStatus.test.ts src/services/alertBatchApi.test.ts src/services/pageSnapshotAdapters.test.ts src/routes/routeManifest.test.ts`
- `npm run build`

## K8s 验证

- `kubectl apply -f deployments/kubernetes/applications/go-services.yaml --dry-run=client`
- `kubectl apply -f deployments/kubernetes/applications/web-ui.yaml --dry-run=client`
- 实际滚动使用目标化命令，避免整份 `go-services.yaml` 触发非目标服务重启：
  - `kubectl set image deployment/alert-service -n traffic-analysis alert-service=traffic/alert-service:alert-batch-state-version-20260629-r1`
  - `kubectl set image deployment/web-ui -n traffic-analysis web-ui=traffic/web-ui:alert-batch-state-version-20260629-r1`
- `kubectl rollout status deployment/alert-service -n traffic-analysis --timeout=180s`
- `kubectl rollout status deployment/web-ui -n traffic-analysis --timeout=180s`
- 镜像核对：
  - `alert-service traffic/alert-service:alert-batch-state-version-20260629-r1 1/1`
  - `web-ui traffic/web-ui:alert-batch-state-version-20260629-r1 1/1`
- 非目标服务核对：`asset-service`、`graph-service`、`forensics-service` 均保持原稳定镜像并 `Running`。

## Live no-op 证据

入口：`http://127.0.0.1:30180`

临时 JWT 只在脚本内生成，未打印明文 token。

```text
list_status=200
batch_status=200
success_count=0
failed_count=2
error_codes=BIZ_3005,BIZ_3005
unchanged=1

baseline:
alert-default-1782666416062-966461f8 ALERT_STATUS_NEW 1782666416062
alert-default-1782665883233-a408e184 ALERT_STATUS_NEW 1782666400959

after:
alert-default-1782666416062-966461f8 200 ALERT_STATUS_NEW ALERT_STATUS_NEW 1782666416062 1782666416062
alert-default-1782665883233-a408e184 200 ALERT_STATUS_NEW ALERT_STATUS_NEW 1782666400959 1782666400959
```

结论：两个真实告警使用 stale `state_version` 批量更新时均被逐项判定为 `BIZ_3005`，返回 `failed_count=2`、`success_count=0`；随后详情读取确认两条告警的 `status` 和 `state_version` 均未变化，本轮无业务状态写入。

## Desktop Chrome 门禁

通过 Codex Desktop Chrome Bridge 打开：

- `http://10.0.5.8:30180/alerts`

读回结果：

- URL：`http://10.0.5.8:30180/login`
- Title：`园区网络全流量采集与分析系统`
- Body 首屏：`统一身份认证入口`、`账号密码登录`、`验证码`、`登 录`

结论：未登录访问告警队列仍被生产登录门禁拦截。

## 后续追踪

- 真实告警正向写入/回滚证据后续已由 `doc/02_acceptance/runs/20260629-alert-positive-audit/` 补齐。
- 审计查询回放后续已由 `doc/02_acceptance/runs/20260629-alert-positive-audit/` 补齐。
- FP 到白名单草案跳转后续已由 `doc/02_acceptance/runs/20260629-alert-feedback-whitelist/` 补齐。
- 完整 Playwright 权限矩阵后续已由 `doc/02_acceptance/runs/20260629-live-permission-matrix/` 补齐。
