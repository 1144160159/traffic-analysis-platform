# SNMP/LLDP 主动资产发现回归报告

- Run ID：`20260630-asset-discovery-worker-r1`
- 结果：`pass`
- APISIX：`http://10.0.5.8:30180`
- 检查数：6/6 passed，blockers=0，warnings=0
- Discovery Run：`6978b339-9b34-4785-b13e-a3ecdd05946d`
- Credential：`6bb3f888-f519-4c16-b817-dae5455712c9`

## 证据

- Summary：`doc/02_acceptance/runs/20260630-asset-discovery-worker-r1/live-asset-discovery-20260630-asset-discovery-worker-r1-summary.json`
- NDJSON：`doc/02_acceptance/runs/20260630-asset-discovery-worker-r1/live-asset-discovery-20260630-asset-discovery-worker-r1.ndjson`
- Credential response：`doc/02_acceptance/runs/20260630-asset-discovery-worker-r1/credential-response.json`
- Run response：`doc/02_acceptance/runs/20260630-asset-discovery-worker-r1/run-response.json`
- Neighbor response：`doc/02_acceptance/runs/20260630-asset-discovery-worker-r1/neighbors-response.json`

## 口径

本报告通过真实 APISIX 和 asset-service 验证 SNMP/LLDP 主动发现控制面：凭据只登记 Secret 引用、不接收明文；发现任务写入 asset_discovery_runs；观测资产写入 assets；LLDP 邻居关系写入 asset_topology_links。
