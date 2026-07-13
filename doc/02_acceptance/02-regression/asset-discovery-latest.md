# SNMP/LLDP 主动资产发现回归报告

- Run ID：`20260630-asset-discovery-rbac-r1`
- 结果：`pass`
- APISIX：`http://10.0.5.8:30180`
- 检查数：11/11 passed，blockers=0，warnings=0
- Discovery Run：`3eabf2b8-4c39-4b2f-8ff0-8bb364350eea`
- Worker Run：`259373a6-5599-4abd-bd37-bb70eb285064`
- Credential：`68074985-57ef-41d1-87e1-9c8113890743`

## 证据

- Summary：`doc/02_acceptance/runs/20260630-asset-discovery-rbac-r1/live-asset-discovery-20260630-asset-discovery-rbac-r1-summary.json`
- NDJSON：`doc/02_acceptance/runs/20260630-asset-discovery-rbac-r1/live-asset-discovery-20260630-asset-discovery-rbac-r1.ndjson`
- Credential response：`doc/02_acceptance/runs/20260630-asset-discovery-rbac-r1/credential-response.json`
- Run response：`doc/02_acceptance/runs/20260630-asset-discovery-rbac-r1/run-response.json`
- Worker run response：`doc/02_acceptance/runs/20260630-asset-discovery-rbac-r1/worker-run-response.json`
- Neighbor response：`doc/02_acceptance/runs/20260630-asset-discovery-rbac-r1/neighbors-response.json`

## 口径

本报告通过真实 APISIX、auth-service、asset-service 和 PostgreSQL 验证 SNMP/LLDP 主动发现控制面：auth scope catalog 包含 asset:discover；viewer 仅 asset:read 时写接口返回 403；凭据只登记 Secret 引用、不接收明文；发现任务写入 asset_discovery_runs；成功写操作同步进入 audit_logs；观测资产写入 assets；LLDP 邻居关系写入 asset_topology_links；无 observations 的 scanner worker 路径会创建 failed run 并记录错误，不会静默停留 queued。
