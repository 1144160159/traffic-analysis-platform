# 告警状态机一致性与处置快捷接口门禁

Run: `20260629-alert-state-contract`
Time: `2026-06-29T00:36:48+08:00` (`2026-06-28T16:36:48Z`)
Commit baseline: `e3316aec4ac1d6592e28aefc86853128ecde7408`

## 背景

`doc/05_status/代码实证状态核对-2026-06-19.md` 指出告警处置已有前后端主体能力，但前端状态值和后端状态机不一致，快捷操作缺统一权限、原因和验收 Manifest。本轮优先修正告警主状态机，不触碰真实生产告警数据。

## 修改摘要

- 后端状态机统一为 canonical 状态：`new`、`triage`、`assigned`、`closed`。
- 后端 `state.ParseStatus` 兼容历史/展示别名：`in_progress`、`resolved`、`false_positive`、`ALERT_STATUS_*` 和中文展示值等，但写入路径只使用 canonical 值。
- 状态机允许 `new -> assigned`，使告警列表/详情的直接指派动作和后端一致。
- `UpdateStatus`、`BatchUpdateStatus`、`AssignAlert`、`CloseAlert`、`ReopenAlert` 统一走 `alert:write` 写权限。
- `CloseAlert` 在进入服务层前要求 `reason`，避免无解释审计。
- `AssignAlert` 进入服务层前读取当前状态并执行状态机校验，禁止绕过状态机写入 `assigned`。
- Web UI `alertStatus.ts` 同步后端状态机；`alertDetailApi.ts` 补状态机快捷动作请求构造；route manifest 记录告警状态机 API hints。
- live 已滚动：
  - `traffic/alert-service:alert-state-20260629-r1`
  - `traffic/web-ui:alert-state-20260629-r1`

## 验证结果

- `cd go/control-plane && go test ./internal/alert/state ./internal/alert/api ./cmd/alert-service`
  - 结果：通过。
  - 验证点：状态机流转、状态别名解析、写接口权限负例、关闭原因必填、alert-service 编译。
- `cd go/control-plane && go test ./internal/alert/service`
  - 结果：通过。
  - 验证点：服务层包编译通过。
- `cd web/ui && npm run test -- --run src/services/alertStatus.test.ts src/services/alertDetailApi.test.ts src/routes/routeManifest.test.ts`
  - 结果：通过，3 files / 12 tests。
  - 验证点：前端状态机镜像、详情动作请求构造、route manifest 状态机 API hints。
- `cd web/ui && npm run build`
  - 结果：通过。
  - 备注：仅保留既有 large chunk warning。
- K8s dry-run
  - 结果：通过。
  - 验证点：alert-service Service/Deployment 摘取清单和 web-ui 清单均可 apply dry-run。
- 镜像构建与导入
  - 结果：通过。
  - 备注：Docker daemon 代理导致 alert-service 标准 Dockerfile 无法刷新基础镜像 metadata；已改用本机 Go 编译 Linux 静态二进制，并覆盖上一版本地镜像 entrypoint `/usr/local/bin/app` 后 commit。
  - alert image: `sha256:a5c29dbd8dab2df3b7c01df42dfdff6944b70c2cecd57ff0d7153bb8edc008f8`。
  - web image: `sha256:f5c5706b4b79a409004b4fb96424246e942f8ae3468b09d3a9b0d41450fe3841`。
  - 两节点 containerd 均已导入。
- K8s live rollout
  - 结果：通过。
  - 当前镜像：`alert-service` 为 `traffic/alert-service:alert-state-20260629-r1`，`web-ui` 为 `traffic/web-ui:alert-state-20260629-r1`，均 `1/1` ready。
- live no-op API contract
  - 使用 K8s Secret 临时签发 5 分钟 access token，不打印 token。
  - `PUT /api/v1/alerts/AL-CODEX-STATE-NOOP/assign` 使用仅 `alert:read` token，返回 `403 Permission denied: alert:write required`。
  - `POST /api/v1/alerts/AL-CODEX-STATE-NOOP/close` 使用 `alert:write` token 但无 reason，返回 `400 reason is required`。
  - `PUT /api/v1/alerts/AL-CODEX-STATE-NOOP/status` 使用 `status=resolved` 和 reason，返回 `404 Alert not found`，证明别名通过 handler 解析并进入服务层，不再被当作非法状态拒绝。
  - `PUT /api/v1/alerts/AL-CODEX-STATE-NOOP/status` 使用 `status=not_a_state`，返回 `400 invalid status: not_a_state`。
- Codex Desktop Chrome bridge
  - `desktop_chrome_open_url(http://10.0.5.8:30180/alerts/AL-CODEX-STATE-NOOP)`：通过。
  - 最终 URL 为 `http://10.0.5.8:30180/login`，标题为“园区网络全流量采集与分析系统”，说明生产登录门禁仍生效。

## 未覆盖

- live 验证没有修改真实告警；真实正向状态写入、回滚和审计查询回放仍需受控 fixture 或维护窗口。
- `state_version` / ETag 并发冲突还没有通过 HTTP 契约暴露和验证。
- FP 反馈到白名单草案跳转仍未完成。
- 完整 Playwright/Desktop 权限矩阵仍未跑完。
