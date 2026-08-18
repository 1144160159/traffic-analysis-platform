# T1-M10-N004 Kubernetes 站点 preflight 与回滚

编排器为每个当前 Kubernetes 节点创建一个 run-scoped Job。Pod 以非 root、无 ServiceAccount token、只读根文件系统运行，只读挂载宿主机 `/proc`、`/sys` 和 `/home/data`，检查 CPU、内存、NUMA、NIC、磁盘、节点时钟、DNS、TCP 和 TLS 证书。控制面额外执行只读 `kubectl auth can-i` 和 Secret key 名称存在性检查，不输出 Secret 值。

```bash
python3 -m unittest tests.alignment.test_m10_site_preflight -v
python3 scripts/alignment/run_m10_site_preflight_k8s.py \
  --image <immutable-local-image> --run-id <uuid>
```

结果 `status=PASS` 只表示探针本身完整执行；站点结论在 `acceptance_status` 和 `evaluation.blocking_codes`。任何容量、DNS、端点、证书、RBAC 或 Secret 缺口都必须阻断，且 readiness 不能替代真实部署、性能、HA 或回滚验收。

回滚只删除本次带 `traffic.analysis/canary-run=<run-id>` 标签的 Job/Pod/ConfigMap；不得修改宿主机、Kubernetes 工作负载或 Secret。
