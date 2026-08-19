# DLQ Replay 受保护 API 契约最小闭环

Run: `20260628-dlq-replay-control`
Time: `2026-06-28T23:23:05+08:00`
Updated: `2026-06-29T00:10:22+08:00` (`2026-06-28T16:10:22Z`)
Commit baseline: `e3316aec4ac1d6592e28aefc86853128ecde7408`

## 背景

`doc/05_status/未开发项梳理-2026-06-19.md` 将 DLQ/replay/幂等恢复列为 P0 已开发但未闭环项，任务卡 `scripts/codex_loop/tasks/CLE-P0-DLQ-001.yaml` 要求补齐“人工修复、审批、重放、审计、幂等验证”。任务卡同时标记 `execution.mode: plan` 与 `allow_live_write: false`，因此本轮不向 live Kafka/APISIX/DB 注入坏消息，也不执行真实集群重放。

## 修改摘要

- `ingest/dlq.Producer` 新增导出的 `ReplayFallbackFiles(ctx)`，复用原有 fallback 文件重放逻辑，并返回可审计的 `FallbackReplayReport`。
- 原定时 fallback replay 仍保留，通过 `ReplayFallbackFiles(ctx)` 统一执行，避免自动重放和人工重放走两套逻辑。
- 新增 `ReplayManager` 控制层，要求重放请求包含：
  - `tenant_id`
  - `requested_by`
  - `approved_by`
  - `approval_id`
  - `reason`
  - `repair_summary`
  - `idempotency_key`
- `ReplayManager` 禁止自审批：`approved_by` 必须不同于 `requested_by`。
- `ReplayManager` 支持 dry-run：只读取 fallback 统计和记录审批轨迹，不触发重放，并把 remaining 统计保持为执行前 fallback 统计。
- `ReplayManager` 用 `idempotency_key` 去重，重复请求返回相同 `replay_id` 且不再次调用 replay executor；同进程内通过 manager 锁避免并发重复执行。
- `ReplayResult` 带 `AuditTrail`，表达审批、执行、重复请求去重三类审计事件。
- 新增 Redis-backed `ReplayIdempotencyStore`，使用哈希后的 idempotency key 和 24h TTL 保存 `ReplayResult`，避免跨进程/多副本重复执行。
- `ReplayManager` 的 idempotency store 读取或写入失败时 fail closed，不进入 replay executor，避免 Redis 不可用时产生重复重放风险。
- 新增 `ReplayHTTPHandler`，暴露 `POST /api/v1/dlq/replay/fallback`，要求 Bearer token 具备 `dlq:replay`、`dlq:*`、`admin:write`、`admin:*` 或 `*`。
- `ingest-gateway` health/management mux 挂载 DLQ replay handler，management 端口显式配置为 `:8080`，metrics 继续使用 `:9091`；运行时优先使用 Redis replay 幂等存储，Redis 不可用才降级为进程内存并输出告警日志。
- `deployments/kubernetes/applications/go-services.yaml` 为 ingest-gateway 增加 `management` service port/container port，并把健康探针切到 8080，避免和 metrics 端口混用。
- `deployments/kubernetes/configmaps/apisix-routes.yaml` 增加 `/api/v1/dlq* -> ingest-gateway.traffic-analysis.svc:8080` 路由。
- Auth scope 清单新增 `dlq:replay`，使 token scope 发现与校验能表达 DLQ 运维重放权限。
- Web UI 的 `data-quality` 页面新增 DLQ replay action contract：`POST /v1/dlq/replay/fallback`、`dlq:replay`/admin scope、`dry_run=true` 默认预检、`idempotency_key`、Redis 24h TTL 和审计事件在“重放对账”Tab 与数据重放 Modal 中可见。
- 前端 API plan 支持 `actions`，`data-quality` 的 DLQ replay action endpoint 纳入 APISIX route 覆盖测试；新增 `dlqReplayApi` 类型层，默认把 fallback replay 请求构造为 dry-run。
- Web UI 已构建并发布到 K8s：镜像 `traffic/web-ui:dlq-contract-20260628-r1` 已导入 10.0.5.8 和 10.0.5.9 的 containerd，并滚动到 live `deployment/web-ui`。
- `ingest-gateway` 已补齐 Postgres token validator 配置：Go 配置层支持从 `POSTGRES_HOST/PORT/DATABASE/USERNAME/PASSWORD` 组装 DSN，K8s 清单通过 `traffic-credentials/PG_PASSWORD` 注入密码，不在清单落明文。
- `ingest-gateway` 已构建并发布到 K8s：镜像 `traffic/ingest-gateway:dlq-replay-20260628-r2` 已导入 10.0.5.8 和 10.0.5.9 的 containerd，并滚动到 live `deployment/ingest-gateway`。
- `ingest-gateway` 增加 `emptyDir` fallback 目录与 initContainer 权限初始化，live 日志已确认 `DLQ fallback directory ready` 和 `fallback_enabled:true`，消除了 `/var/log/ingest-gateway` 权限导致 fallback disabled 的问题。
- `doc/05_status/未开发项梳理-2026-06-19.md` 的 DLQ P0 行已同步为“live 受保护 dry-run API 与幂等已通过，真实坏消息注入/重放仍未闭环”，避免状态总表继续误写“未见 API”或“后端未滚动”。

## 验证结果

- `gofmt -w go/control-plane/internal/ingest/dlq/producer.go go/control-plane/internal/ingest/dlq/replay_manager.go go/control-plane/internal/ingest/dlq/replay_manager_test.go && cd go/control-plane && go test ./internal/ingest/dlq`
  - 结果：通过。
  - 验证点：必须提供修复说明和审批；禁止自审批；dry-run 不执行重放；幂等 key 防止重复重放；部分失败返回 `partial` 并保留错误。
- `cd go/control-plane && go test ./internal/ingest/...`
  - 结果：通过。
  - 验证点：新增导出 replay 方法未破坏 ingest auth/config/dedup/dlq/queue/server 现有包。
- `gofmt -w ... && cd go/control-plane && go test ./internal/ingest/dlq ./internal/ingest/... ./internal/auth/model ./cmd/ingest-gateway`
  - 结果：通过。
  - 验证点：HTTP handler 权限负例、dry-run token tenant/actor fallback、scope wildcard、Redis idempotency store round-trip/miss/error、idempotency store fail-closed、ingest-gateway main 编译面通过。
- `python3` 解析 `deployments/kubernetes/applications/go-services.yaml` 和 `deployments/kubernetes/configmaps/apisix-routes.yaml`
  - 结果：通过。
  - 验证点：APISIX 内层 `apisix.yaml` 可解析，且存在 `/api/v1/dlq*` 路由。
- `kubectl apply --dry-run=client -f deployments/kubernetes/applications/go-services.yaml`
  - 结果：通过。
- `kubectl apply --dry-run=client -f deployments/kubernetes/configmaps/apisix-routes.yaml`
  - 结果：通过。
- `cd web/ui && npm run test -- --run src/services/pageApiPlans.test.ts src/services/dlqReplayApi.test.ts src/routes/routeManifest.test.ts`
  - 结果：通过，11 tests passed。
  - 验证点：`data-quality` action endpoint 被 `/api/v1/dlq*` APISIX route 覆盖；DLQ replay action 要求 `dlq:replay`，默认 `dry_run=true`；routeManifest 的“重放 DLQ”动作带 `/api/v1/dlq/replay/fallback` hint。
- `cd web/ui && npm run build`
  - 结果：通过。
  - 验证点：新增 TypeScript action plan、DLQ replay service 类型和 DataQualityPage 展示契约均通过 `tsc` 与 Vite production build；仅保留既有 large chunk warning。
- `cd go/control-plane && gofmt -w internal/ingest/config/config.go internal/ingest/config/config_test.go cmd/ingest-gateway/main.go && go test ./internal/ingest/config ./internal/ingest/dlq`
  - 结果：通过。
  - 验证点：Postgres DSN 支持从分离环境变量组装，密码做 URL 转义；DLQ 包现有测试仍通过。
- `docker build -t traffic/web-ui:dlq-contract-20260628-r1 ... -f deployments/Dockerfile .`
  - 结果：通过。
  - 验证点：生产构建参数为 `VITE_AUTH_ENABLED=true`、`VITE_USE_MOCK=false`、`VITE_ENABLE_REALTIME=false`、`VITE_SCREEN_ACCESS_MODE=protected`。
- `docker run --rm traffic/web-ui:dlq-contract-20260628-r1 grep ... /usr/share/nginx/html/assets`
  - 结果：通过。
  - 验证点：镜像内 `DataQualityPage-JM9L6hlJ.js` 包含 `DLQ Replay API 契约`、`POST /v1/dlq/replay/fallback`、`dry_run=true`、`Redis 24h TTL`。
- `docker save` + `ctr -n k8s.io images import` on 10.0.5.8 and 10.0.5.9
  - 结果：通过。
  - 验证点：新 Web UI 镜像已进入两个 K8s 节点的 containerd，避免依赖外部 registry。
- `kubectl apply -f deployments/kubernetes/applications/web-ui.yaml && kubectl -n traffic-analysis rollout status deployment/web-ui --timeout=180s`
  - 结果：通过。
  - 验证点：live `deployment/web-ui` 已使用 `traffic/web-ui:dlq-contract-20260628-r1`，ready `1/1`，新 Pod `web-ui-5d5d697588-z6zjj` 在 `zeus-server` Running。
- `curl --noproxy '*' http://10.0.5.8:30180/assets/DataQualityPage-JM9L6hlJ.js | rg ...`
  - 结果：通过。
  - 验证点：APISIX 30180 已对外提供包含 DLQ replay contract 的新 DataQuality chunk。
- Codex Desktop Chrome bridge 对照验证：
  - `desktop_chrome_open_url(url="http://10.0.5.8:30180/login")` 结果：通过，backend 为 Chrome extension，页面标题为“园区网络全流量采集与分析系统”。
  - `desktop_chrome_open_url(url="http://10.0.5.8:5173/...")` 与 `http://10.0.5.8:32173/...` 结果：超时；裸 Vite 端口无法从 Desktop Chrome 侧访问。本轮未把 Desktop 页面巡检计为通过。
  - `desktop_chrome_open_url(url="http://10.0.5.8:30180/data-quality?tab=replay-reconcile")` 结果：通过打开但最终 URL 为 `/login`，说明生产登录门禁生效；本轮未提交登录表单、未伪造浏览器会话，因此未把业务页可视巡检计为通过。
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/ingest-gateway` + overlay image build
  - 结果：通过。
  - 验证点：镜像 `traffic/ingest-gateway:dlq-replay-20260628-r2` 构建成功，image id `sha256:a93ff8690b27001dcf772d7e81b21c4b60b862a377dd801e4311ae42abdebf40`，运行用户仍为 `appuser`，入口为 `/usr/local/bin/app`。
- `docker save` + `ctr -n k8s.io images import` on 10.0.5.8 and 10.0.5.9
  - 结果：通过。
  - 验证点：新 ingest-gateway 镜像已进入两个 K8s 节点的 containerd；仅出现 containerd mirror deprecation warning。
- `sed -n '1,68p' deployments/kubernetes/applications/go-services.yaml | kubectl apply -f - && kubectl -n traffic-analysis rollout status deployment/ingest-gateway --timeout=240s`
  - 结果：通过。
  - 验证点：live `deployment/ingest-gateway` 已使用 `traffic/ingest-gateway:dlq-replay-20260628-r2`，ready `2/2`。
- `kubectl -n traffic-analysis logs deploy/ingest-gateway --tail=180 | rg ...`
  - 结果：通过。
  - 验证点：启动日志包含 `Connected to PostgreSQL (fallback)`、`PG Token Validator initialized`、`pg_fallback:true`、`DLQ fallback directory ready`、`fallback_enabled:true`，且没有新的 `permission denied`。
- `curl --noproxy '*' -i -X POST http://10.0.5.8:30180/api/v1/dlq/replay/fallback -H 'Content-Type: application/json' -d '{}'`
  - 结果：通过。
  - 验证点：APISIX live 路由返回 `401 Unauthorized` 与 `bearer token required`，说明不再是 502，且受保护入口门禁生效。
- 临时 `api_tokens` 行 + `dlq:replay` scope + `POST dry_run=true` 到 `http://10.0.5.8:30180/api/v1/dlq/replay/fallback`
  - 结果：通过，HTTP `200`。
  - 验证点：返回 `status: dry_run`、`duplicate:false`、`tenant_id: campus-a`、`pre_fallback_files:0`、`remaining_fallback_files:0`、审计动作 `dlq_replay_approved`；没有执行 fallback 文件重放。
- 同一个临时 token、同一个 `idempotency_key` 连续 dry-run 两次
  - 结果：通过，两次 HTTP `200`。
  - 验证点：第一次 `duplicate:false`，第二次 `duplicate:true`，两次 `replay_id` 相同，第二次审计动作包含 `dlq_replay_duplicate`，证明 live Redis-backed 幂等路径实际生效。
- `select count(*) from api_tokens where name like 'codex-dlq-replay-%'`
  - 结果：通过，返回 `0`。
  - 验证点：本轮用于 live dry-run/幂等验证的短期 token 行已清理。
- `redis-cli DEL ... && redis-cli EXISTS ...` on Sentinel-reported Redis master `10.244.1.80:6379`
  - 结果：通过，`DEL` 返回 `2`，`EXISTS` 返回 `0`。
  - 验证点：本轮两个 live dry-run/idempotency 测试写入的 Redis 幂等记录已清理。

## 未覆盖

- 未向 live Kafka 注入坏消息。
- 未执行真实 fallback 文件重放；live 验证限定为 `dry_run=true`。
- Redis-backed 幂等已通过 live APISIX 重复请求验证，但本轮没有强制把两次请求分别打到两个不同 Pod。
- Web UI 新增 DLQ action contract 已通过 Vitest/build、镜像内 grep、K8s rollout 和 30180 asset 验证；Desktop Chrome 到 `/data-quality?tab=replay-reconcile` 被生产登录门禁重定向到 `/login`，仍需在合法登录态下完成业务页可视巡检。
- 本轮未执行 `tests/run_tests.sh full`、Java/Flink 全量测试、proto lint 或 100 轮 live smoke。
