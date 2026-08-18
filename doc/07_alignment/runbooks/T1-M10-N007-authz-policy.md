# T1-M10-N007 最小权限与失败关闭运行手册

## 目标与边界

本任务把 OpenAPI `x-required-scope`、四个运行时角色及 tenant/object/field 授权规则固化为一个可复现候选。候选默认关闭；当前 K8s Job 只运行镜像内 Go 负例，不修改 APISIX、Keycloak、Deployment、Secret 或业务数据。

## 构建与本地验证

```bash
python3 scripts/alignment/build_m10_authz_policy.py --write
python3 scripts/alignment/build_m10_authz_policy.py --check
python3 scripts/alignment/verify_m10_authz_policy.py
PYTHONPATH=. python3 -m unittest tests/alignment/test_m10_authz_policy.py -v
(cd go/control-plane && go test ./...)
docker build -f scripts/alignment/Dockerfile.m10-authz-tests -t traffic-analysis/m10-authz-tests:<run-id> .
```

若容器构建器无法访问 Go module proxy，可在确认 `go version go1.25.12 linux/amd64` 后，用 `CGO_ENABLED=0 go test -c` 在临时目录生成五个测试二进制，并以 `scripts/alignment/Dockerfile.m10-authz-prebuilt` 和固定 Debian digest 组装同一运行镜像。临时目录不得进入候选清单，最终 image ID 必须写入 K8s 收据。

镜像标签必须是非 `latest` 的 run-scoped 标签。导入两个节点的 containerd 后运行：

```bash
python3 scripts/alignment/run_m10_authz_k8s.py \
  --image traffic-analysis/m10-authz-tests:<run-id> \
  --nodes 8-2tb,zeus-server \
  --run-id <canonical-uuid>
python3 scripts/alignment/verify_m10_authz_k8s.py
```

## 后置启用门

只有 N006 route/workload diff 为零、候选 ID 非空、审批人已授权，才可进入真实 Keycloak/APISIX/service 的短时启用。后置测试必须使用受控 token 分别验证：无 token、过期 token、错误 scope、异租户断言、异租户对象 ID、禁止字段；任一请求成功即停止并回滚。

## 回滚 RB-T1-M10-P015-OPS-n007-s1

1. 将认证/授权候选保持或恢复为 `default_runtime_state=off`。
2. 恢复启用前 Deployment template、APISIX route ConfigMap 和 Keycloak role mapping 的精确版本。
3. 等待工作负载 rollout 完成并验证健康检查。
4. 重跑原版本的授权正例与拒绝负例，确认没有双版本混用。
5. 保存回滚窗口、资源 UID/resourceVersion、镜像 ID 和响应状态证据；未形成该证据时 G6 仍为 BLOCKED。

## 证明上限

双节点 Job PASS 只证明绑定候选在 K8s 容器中通过声明的 Go 测试，不证明现网路由已应用、真实 OIDC token 已验证、服务端后置测试完成、回滚成功或生产晋级获批。
