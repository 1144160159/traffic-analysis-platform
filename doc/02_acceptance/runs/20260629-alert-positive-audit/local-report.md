# 告警真实写入、回滚与审计查询验收

日期：2026-06-29

## 范围

- 后端：告警写操作新增同步 `audit_logs` 落库路径，使 `/api/v1/audit/logs` 可以在写操作完成后按告警 ID 查询审计记录。
- 查询：`/api/v1/audit/logs` 支持 `object_type/resource_type` 与 `object_id/resource_id` 过滤，并优先返回 `event_id` 作为 `log_id`。
- 状态读取：`GetByID` 改为 `ORDER BY updated_at DESC LIMIT 1`，避免 ClickHouse ReplacingMergeTree/Distributed 多版本数据下读回旧状态。
- 部署：`traffic/alert-service:alert-audit-positive-20260629-r2` 已滚动到 `traffic-analysis` 命名空间。

## 本地验证

- `go test ./internal/alert/... ./cmd/alert-service`
- `go test ./internal/alert/api ./internal/alert/repository ./internal/alert/service ./cmd/alert-service`
- `go test ./internal/common/audit`
- `npm run test -- --run src/services/pageSnapshotAdapters.test.ts src/services/alertBatchApi.test.ts src/services/alertStatus.test.ts`

## K8s 验证

- `kubectl apply -f deployments/kubernetes/applications/go-services.yaml --dry-run=client`
- 镜像已导入两台 containerd 节点：
  - `8-2tb`
  - `zeus-server`
- 实际滚动使用目标化命令，避免整份 `go-services.yaml` 触发非目标服务重启：
  - `kubectl set image deployment/alert-service -n traffic-analysis alert-service=traffic/alert-service:alert-audit-positive-20260629-r2`
- `kubectl rollout status deployment/alert-service -n traffic-analysis --timeout=180s`
- 镜像核对：
  - `alert-service traffic/alert-service:alert-audit-positive-20260629-r2 1/1`

## r1 暴露的问题

首轮 r1 live 验证中，`PUT /api/v1/alerts/{id}/status` 返回 `200` 和新 `state_version`，同步审计记录也已写入，但随后 `GET /api/v1/alerts/{id}` 仍读回旧状态：

```text
alert_id=alert-default-1782666971003-bf23f23a
baseline_status=ALERT_STATUS_NEW
baseline_version=1782666971003
forward_status=200
forward_response={"success":true,"data":{"alert_id":"alert-default-1782666971003-bf23f23a","new_status":"closed","old_status":"ALERT_STATUS_NEW","reason":"codex live positive write then rollback","state_version":1782667108052},"error":null}
forward_detail_status=ALERT_STATUS_NEW
forward_detail_version=1782666971003
forward_audit_count=1
rollback_status=400
```

根因：`GetByID` 对 `traffic.alerts` 使用 `LIMIT 1`，没有按 `updated_at` 选择最新版本。r2 已修复为 `ORDER BY updated_at DESC LIMIT 1`。遗留的 `closed` 新版本已通过 `POST /api/v1/alerts/{id}/reopen` 恢复到 `new`。

## Live 正向写入、回滚与审计查询

入口：`http://127.0.0.1:30180`

临时 JWT 只在脚本内生成，未打印明文 token。测试对真实告警产生短暂状态写入，并在同一脚本中回滚到原状态。

```text
run_dir=/tmp/alert-positive-audit-live-20260629012134
alert_id=alert-default-1782667180412-05700975
baseline_status=ALERT_STATUS_NEW
baseline_version=1782667180412
forward_status=200
forward_response={"success":true,"data":{"alert_id":"alert-default-1782667180412-05700975","new_status":"closed","old_status":"ALERT_STATUS_NEW","reason":"codex live positive write then rollback","state_version":1782667294682},"error":null}
forward_detail_status=closed
forward_detail_version=1782667294682
forward_audit_count=1
rollback_status=200
rollback_response={"success":true,"data":{"alert_id":"alert-default-1782667180412-05700975","status":"new"},"error":null}
rollback_detail_status=new
rollback_detail_version=1782667294892
rollback_audit_count=1
restored=1
latest_audit_actions=ALERT_REOPENED,ALERT_STATUS_UPDATED
```

验证点：

- `state_version` 正向写入成功，详情读回 `closed` 和新版本 `1782667294682`。
- `/api/v1/audit/logs?object_type=alert&object_id=alert-default-1782667180412-05700975&limit=10` 可查到 `ALERT_STATUS_UPDATED`。
- 回滚 `POST /api/v1/alerts/{id}/reopen` 成功，详情读回 `new` 和新版本 `1782667294892`。
- 同一审计查询可查到 `ALERT_REOPENED`。

## Desktop Chrome 门禁

通过 Codex Desktop Chrome Bridge 打开：

- `http://10.0.5.8:30180/alerts`

读回结果：

- URL：`http://10.0.5.8:30180/login`
- Title：`园区网络全流量采集与分析系统`
- Body 首屏包含：`统一身份认证入口`、`账号密码登录`、`验证码`、`登 录`

结论：未登录访问告警队列仍被生产登录门禁拦截。

## 仍未关闭

- FP 到白名单草案跳转仍未补。
- 完整 Playwright/live 权限矩阵仍未补。
