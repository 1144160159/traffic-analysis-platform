# SNMP/LLDP 主动资产发现回归报告

- Run ID：`20260629-asset-discovery-r2-report`
- 结果：`pass`
- APISIX：`http://10.0.5.8:30180`
- 检查数：6/6 passed，blockers=0，warnings=0
- Discovery Run：`bd1d3e32-d72e-4901-98bc-5af6c8b124fd`
- Credential：`d017d0c7-96d5-4340-aa10-a85aca08da7b`

## 证据

- Summary：`doc/02_acceptance/runs/20260629-asset-discovery/live-asset-discovery-20260629-asset-discovery-r2-report-summary.json`
- NDJSON：`doc/02_acceptance/runs/20260629-asset-discovery/live-asset-discovery-20260629-asset-discovery-r2-report.ndjson`
- Credential response：`doc/02_acceptance/runs/20260629-asset-discovery/credential-response.json`
- Run response：`doc/02_acceptance/runs/20260629-asset-discovery/run-response.json`
- Neighbor response：`doc/02_acceptance/runs/20260629-asset-discovery/neighbors-response.json`

## 口径

本报告通过真实 APISIX 和 asset-service 验证 SNMP/LLDP 主动发现控制面：凭据只登记 Secret 引用、不接收明文；发现任务写入 asset_discovery_runs；观测资产写入 assets；LLDP 邻居关系写入 asset_topology_links。
