# Graph Service 业务流闭环修复证据

- Run ID: `20260629-business-flow-api-preflight-r3`
- 镜像: `traffic/graph-service:business-flow-20260629-r1`
- 目标: 修复业务流契约中 `/api/v1/graph/explore` 经 APISIX 超时，完成 43 个唯一 API 端点只读 live 预检闭环。

## 修复内容

- `go/control-plane/internal/common/storage/clickhouse.go`: ClickHouse client 在正常启动后也常驻自动重连；等待连接时尊重请求 context，不再固定阻塞 30 秒。
- `go/control-plane/internal/graph/query/graph_query.go`: Graph Explore 增加 `QUERY_TIMEOUT` 查询上下文，后端依赖慢/断开时快速截断，不拖过网关超时。
- `go/control-plane/internal/graph/api/handler.go`: 查询日志在 graph 为 nil 时不再 panic。
- `deployments/kubernetes/applications/go-services.yaml`: graph-service 切唯一本地镜像 tag，`imagePullPolicy: Never`，ClickHouse hosts 覆盖两台服务，查询/读写超时为 12 秒。

## 发布与验证

- `go test ./internal/common/storage ./internal/graph/query ./internal/graph/api`: 通过。
- `go build ./cmd/graph-service`: 通过。
- 镜像已导入 `10.0.5.8` 与 `10.0.5.9` 的 containerd `k8s.io` namespace。
- `kubectl -n traffic-analysis rollout status deployment/graph-service --timeout=240s`: 通过。
- 新 Pod: `graph-service-986dc8b86-6d5w8`, `1/1 Running`, `0` restarts。
- 启动日志: ClickHouse hosts 为 `clickhouse-1.middleware.svc:9000,clickhouse-2.middleware.svc:9000`，状态从 `connecting` 到 `connected`；cache warmup `success=5 failed=0`。
- `/api/v1/graph/explore`: r3 预检中返回 `200`，graph-service access log latency `38ms`。

## 最终证据

- Summary: `live-business-flow-api-preflight-20260629-business-flow-api-preflight-r3-summary.json`
- Stable summary: `doc/02_acceptance/02-regression/business-flow-api-preflight-latest.json`
- Stable report: `doc/02_acceptance/02-regression/business-flow-api-preflight-latest.md`
- Result: `pass`, `46/46` checks passed, `43/43` API checks passed, `0` blockers, `0` warnings。
