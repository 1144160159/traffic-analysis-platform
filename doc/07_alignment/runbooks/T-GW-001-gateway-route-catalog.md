# T-GW-001 网关路由目录与安全门禁

## 目标和证据边界

本候选把 APISIX standalone 路由、REST OpenAPI 操作和 Kubernetes Service 收敛为一个版本化目录，固定每条路由的 trust zone、method/path、upstream、认证、scope、tenant 来源、请求限制、超时/重试、限流、CORS、缓存、WebSocket、审计、owner 和回滚语义。

目录完整性通过不代表网关安全完成。当前候选明确记录现有 APISIX 路由缺少网关认证、限流、请求校验、body limit 和显式 timeout/retry；`production_applied=false`，不得把服务层返回 401 或管理 UI 能打开写成 T-GW-001 已关闭。

## 权威源

- APISIX route 真源：`deployments/kubernetes/configmaps/apisix-routes.yaml`
- REST 真源：`contracts/openapi/alignment-v1.openapi.json`
- Service 真源：`deployments/kubernetes/**` 与兼容 Go manifests
- 生成目录：`contracts/gateway/route-catalog.v1.json`
- 构建器：`scripts/alignment/build_gateway_route_catalog.py`
- 校验器：`scripts/alignment/verify_gateway_route_catalog.py`
- scope 补全/漂移检查：`scripts/alignment/backfill_openapi_scopes.py`
- 负向测试：`tests/alignment/test_gateway_route_catalog.py`

## 本轮已关闭的声明态问题

1. 57 条 APISIX 路由全部登记，route ID 唯一；91 个 OpenAPI 操作均有路由覆盖。
2. 原缺失的 37 个 `x-required-scope` 已使用 IAM scope 真源补齐；校验器禁止再次缺失。
3. `/minio/*` 的 `minio-proxy.minio.svc:9002` 原只有 Deployment、没有仓库 Service；现已在 `06-minio.yaml` 补齐 ClusterIP Service。
4. APISIX admin API 必须保持 ClusterIP，公共 APISIX Service 不得暴露 9180；负向测试会阻断 NodePort/admin port 漂移。
5. 未认证、未限流等现状不能被隐藏：受保护路由缺插件时必须保留 blocking gap。

## 仓库门禁

```bash
python3 scripts/alignment/backfill_openapi_scopes.py --check
python3 scripts/alignment/build_gateway_route_catalog.py --check
python3 scripts/alignment/verify_gateway_route_catalog.py
python3 -m unittest tests.alignment.test_gateway_route_catalog -v
kubectl apply --dry-run=client -f deployments/kubernetes/infrastructure/06-minio.yaml
```

任何新增、删除或修改路由，OpenAPI 操作变化，上游 Service 缺失，protected route 隐藏认证缺口，或 admin API 外部暴露都会使 G1 失败。六类兼容差异中的 removed route/API/operation 必须继续为空。

## live 只读采集

先生成与当前候选 hash 一致的完整 G0 manifest，再执行：

```bash
python3 scripts/alignment/capture_gateway_route_catalog.py \
  --run-id <immutable-run-id> \
  --g0-manifest <g0-manifest>
```

采集只读取 APISIX ConfigMap/StatefulSet/Service、全命名空间 Service/Endpoints，并执行六个 GET 探针；不读取 Secret、不保存响应正文、不执行生产写入。证据包括 route content hash、APISIX副本、admin Service 类型、所有 upstream ready address，以及未认证入口的 HTTP 状态。

## 后续发布顺序

1. 从 site 配置取得明确的 VPN/bastion allowlist 和 Keycloak client，不在仓库猜测 CIDR 或凭据。
2. 先为内部管理入口生成 OIDC/IP allowlist shadow 候选，验证登录、回调、静态资源和回滚。
3. 再对业务 API 和 WebSocket 灰度网关认证；服务层认证继续保留，双层校验必须使用同一 issuer/audience/tenant 语义。
4. 按路由类型设置 body/field limit、timeout、retry、rate limit、CORS、cache 和 access trace；变更命令仅在幂等证明后允许 retry。
5. 依次验证匿名、过期 token、错误 audience、under-scope、跨租户、伪造 tenant header、oversize、限流和 WebSocket 重连。
6. 只对内部 tenant canary；任一登录回路、API、trace/audit、P99 或资源预算异常立即恢复 previous route hash。

## 关闭条件

T-GW-001 只有在 blocking gap 清零、生产候选 bundle 与 route/plugin/upstream hash 对账、浏览器和 API 负例通过、跨组件 trace/audit 可证明、回滚演练及 T+0/T+1/T+3/T+7 观察完成后才能关闭。当前只可登记为 `IMPLEMENTING`。
