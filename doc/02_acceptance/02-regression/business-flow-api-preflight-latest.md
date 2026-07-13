# 业务流 API 契约预检报告

- Run ID：`20260630-business-flow-api-r26-baseline-governance`
- 结果：`pass`
- APISIX：`http://10.0.5.8:30180`
- 检查数：46/46 passed，API=43/43 passed，blockers=0，warnings=0

## Blockers

- 无

## Warnings

- 无

## 证据

- NDJSON：`doc/02_acceptance/runs/20260630-business-flow-api-r26-baseline-governance/live-business-flow-api-preflight-20260630-business-flow-api-r26-baseline-governance.ndjson`
- Summary：`doc/02_acceptance/runs/20260630-business-flow-api-r26-baseline-governance/live-business-flow-api-preflight-20260630-business-flow-api-r26-baseline-governance-summary.json`
- Endpoint matrix：`doc/02_acceptance/runs/20260630-business-flow-api-r26-baseline-governance/business-flow-api-matrix.json`

## 口径

本报告从 `doc/04_assets/ui_suite_gpt_v1/specs/business-flow-acceptance.json` 抽取所有唯一 API，经 APISIX 使用短期 admin JWT 做只读 GET 验证。动态详情接口使用 live `/alerts` 和 `/campaigns` 解析真实 ID；未解析到 ID 时按 blocker 记录。
