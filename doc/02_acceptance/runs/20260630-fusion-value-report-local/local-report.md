# Fusion 价值量化本地联动报告

日期：2026-06-30
范围：多源融合价值量化 MVP，本地代码与前端契约验证

## 本次补齐

- 后端新增只读接口 `GET /api/v1/fusion/value-report`，基于同租户同时间窗口的 Fusion 数据源、实体对齐、告警反馈和 MTTR 摘要输出：
  - `single_source_baseline`
  - `multi_source`
  - `delta.lead_time_minutes`
  - `delta.false_positive_reduction_pct`
  - `delta.mttr_reduction_pct`
  - `quality_gates`
  - `evidence`
- 前端 `/fusion` API plan 新增 secondary endpoint `/v1/fusion/value-report`。
- `pageSnapshotAdapters.ts` 将 value-report 映射到 Fusion 工作台质量看板，展示“检出提前量 / 误报下降 / MTTR 下降”。
- UI suite Fusion 页面、浮层、业务流与 code-gap 契约同步纳入 `/api/v1/fusion/value-report`。
- 新增 `tests/e2e/live_fusion_value_report_preflight.sh`，把 repo 契约、本地 Go/Vitest/UI suite、K8s JWT Secret、真实 APISIX `/api/v1/fusion/stats|entities|value-report` 和 value-report 响应结构收敛为 live 证据门。

## 验证结果

| 检查 | 命令 | 结果 |
|---|---|---|
| Go 定向接口测试 | `go test ./internal/alert/api -run 'Test(FusionValueReportNoDependenciesReturnsGatedReport\|TopicGovernanceRoutesAreRegisteredUnderAPIV1)$'` | passed |
| Go alert/api 包测试 | `go test ./internal/alert/api` | passed |
| Fusion snapshot adapter | `npm --prefix web/ui run test -- --run src/services/pageSnapshotAdapters.test.ts` | 24/24 passed |
| 路由与 adapter 回归 | `npm --prefix web/ui run test -- --run src/services/pageSnapshotAdapters.test.ts src/routes/routeManifest.test.ts` | 32/32 passed |
| UI code-gap 生成 | `node doc/04_assets/ui_suite_gpt_v1/build_frontend_code_gap.mjs` | pages 28/28, api endpoints 46/46, fix queue 0 |
| UI 契约校验 | `node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs` | 181 manifest items, 28 route contracts, 70 overlay contracts, 0 errors, 0 warnings |
| Live preflight 脚本语法 | `bash -n tests/e2e/live_fusion_value_report_preflight.sh` | passed |
| Alert-service live rollout | `kubectl -n traffic-analysis set image deployment/alert-service alert-service=docker.io/traffic/alert-service@sha256:89d2e52463f7bcadf103b8c6274dd5559570fc7a2ff0da7103c1faf9d83e0569` | rollout succeeded, 1/1 ready |
| Live preflight 正式运行 | `RUN_ID=20260630-fusion-value-report-preflight-r2-live-rollout LOG_DIR=doc/02_acceptance/runs/20260630-fusion-value-report-preflight-r2-live-rollout tests/e2e/live_fusion_value_report_preflight.sh` | pass: 19 passed, 0 blockers, 0 warnings |
| Codex Desktop Chrome smoke | `desktop_chrome_open_url(url="http://10.0.5.8:30180/login", keep=true, wait_ms=2500)` | blocked: Desktop bridge returned `Transport closed` |

## 边界说明

本报告证明 Fusion 价值量化的接口、页面映射、契约、本地测试和 live APISIX/JWT/K8s 结构门已闭环；它仍不等同于第三方检测或试点评估结论。`20260630-fusion-value-report-preflight-r2-live-rollout` 已证明 live `/api/v1/fusion/value-report` 返回 `fusion-value-ablation-v1`、单源/多源对象、delta 指标、quality gates 和 evidence。当前 live 样本窗口仍显示告警样本量、反馈与 MTTR quality gate 为 `blocked`，因此后续仍需冻结真实样本窗口，复核单源/多源对比、检出提前量、误报下降、MTTR 降低和试点材料签认。Codex Desktop Chrome bridge 当前仍为 `Transport closed`，不能声明最新浏览器视觉验收通过。
