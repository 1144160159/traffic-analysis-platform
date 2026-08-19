# SNMP/LLDP 资产发现覆盖率报告

- Run ID：`20260630-asset-discovery-coverage-r1`
- 结果：`blocked`
- APISIX：`http://10.0.5.8:30180`
- 检查数：6/7 passed，blockers=1，warnings=0
- 期望资产清单：`未提供`
- 覆盖率阈值：`95%`

## 证据

- Summary：`doc/02_acceptance/runs/20260630-asset-discovery-coverage-r1/live-asset-discovery-coverage-20260630-asset-discovery-coverage-r1-summary.json`
- NDJSON：`doc/02_acceptance/runs/20260630-asset-discovery-coverage-r1/live-asset-discovery-coverage-20260630-asset-discovery-coverage-r1.ndjson`
- PostgreSQL coverage：`doc/02_acceptance/runs/20260630-asset-discovery-coverage-r1/postgres-coverage-summary.json`
- Coverage match report：`doc/02_acceptance/runs/20260630-asset-discovery-coverage-r1/coverage-match-report.json`
- Site inventory template：`doc/02_acceptance/02-regression/asset-discovery-site-inventory.template.json`

## 口径

本报告只读真实 APISIX 和 PostgreSQL，统计 assets、asset_discovery_runs 和 asset_topology_links，并在提供 SITE_ASSET_INVENTORY_JSON 后按 MAC/IP/hostname 计算现场期望资产发现覆盖率。未提供现场期望清单时结果必须保持 blocked，不能声明真实园区设备发现率达标。
