# SNMP/LLDP 资产发现覆盖率报告

- Run ID：`20260701-asset-discovery-coverage-r3-review-packet-guard`
- 结果：`blocked`
- APISIX：`http://10.0.5.8:30180`
- 检查数：6/7 passed，blockers=1，warnings=0
- 期望资产清单：`doc/02_acceptance/02-regression/asset-inventory-review/latest/formal-site-inventory.template.json`
- 覆盖率阈值：`95%`

## 证据

- Summary：`doc/02_acceptance/runs/20260701-asset-discovery-coverage-r3-review-packet-guard/live-asset-discovery-coverage-20260701-asset-discovery-coverage-r3-review-packet-guard-summary.json`
- NDJSON：`doc/02_acceptance/runs/20260701-asset-discovery-coverage-r3-review-packet-guard/live-asset-discovery-coverage-20260701-asset-discovery-coverage-r3-review-packet-guard.ndjson`
- PostgreSQL coverage：`doc/02_acceptance/runs/20260701-asset-discovery-coverage-r3-review-packet-guard/postgres-coverage-summary.json`
- Coverage match report：`doc/02_acceptance/runs/20260701-asset-discovery-coverage-r3-review-packet-guard/coverage-match-report.json`
- Site inventory template：`doc/02_acceptance/02-regression/asset-discovery-site-inventory.template.json`

## 口径

本报告只读真实 APISIX 和 PostgreSQL，统计 assets、asset_discovery_runs 和 asset_topology_links，并在提供 SITE_ASSET_INVENTORY_JSON 后按 MAC/IP/hostname 计算现场期望资产发现覆盖率。未提供现场期望清单时结果必须保持 blocked，不能声明真实园区设备发现率达标。

带有 `review_required=true` 的 bootstrap 草案只能用于现场清单起草和人工复核，即使匹配率达到阈值也必须保持 blocked，不能作为正式 SITE_ASSET_INVENTORY_JSON 关闭验收。
