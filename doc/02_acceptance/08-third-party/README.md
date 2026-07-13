# 试点与第三方材料包

更新时间：2026-07-02

本目录承载课题一面向试点、售前演示、第三方测试和成果转化的交付模板。它把 `doc/01_design/课题一产品与技术总体设计.md` 第 7/8/11 章、`doc/03_review/专家深评整改清单.md` 中 PM/PRD/QA/实施/销售相关整改项，落成可以随 release evidence package 一起交付的材料。

## 状态口径

| 状态 | 当前结论 |
|---|---|
| 模板包 | 已建立 |
| 签认 readiness | `20260702-third-party-signoff-readiness-r25-ui-r39-viewport-probe-normalized` 已建立，仍需人工复核和签字 |
| 真实试点签认 | 未闭环 |
| 第三方测试报告 | 未闭环 |
| 经济效益证明 | 已有测算模板，缺真实试点数据 |

最新 readiness r25 为 `pass` 包生成状态：10/12 checks passed、0 blockers、2 warnings。它记录 162 个仍需复核/填写/签认的占位项，并按 owner 分类为：`project_review=84`、`user_signoff=48`、`site_operations=15`、`third_party_lab=8`、`project_team=3`、`performance_lab=2`、`maintenance_window=2`。该分类只用于分派复核责任，不改变正式签认 gate。r25 同时记录 `evidence_input_run_ids`，其中 baseline 绑定 `20260701-release-manifest-r80-ha-review-r1`，UI visual/interaction 绑定 `20260702-ui-visual-interaction-preflight-r39-viewport-probe-normalized`，UI evidence finalizer 绑定 `20260702-ui-visual-evidence-finalize-r1-current-capture`，OIDC/SSO 绑定 `20260702-oidc-sso-preflight-r4-completion-gate`，completion snapshot 绑定 `20260702-project-completion-audit-r68-ui-r38-viewport-probe`，detection_quality 绑定 `20260701-detection-quality-preflight-r5-review-packet`，HA 绑定 `20260701-ha-readiness-preflight-r10-review-packet`，performance 绑定 `20260701-capture-performance-preflight-r4-review-packet`；当前共有 9 个上游非 pass 或 blocked 输入需例外决策，项目完成度审计会拒绝与当前 release manifest 不一致的陈旧签认包。

本目录不能单独证明 10 x 100Gbps、512Mpps、95%/5%、生产安全或 HA 通过。上述专项仍以 `03-performance/`、`04-detection-quality/`、`05-security/`、`06-resilience/` 的正式证据为准。

## 文件清单

| 文件 | 用途 | 主要输入 |
|---|---|---|
| `pilot-package-manifest.json` | 试点材料包清单和证据映射 | release manifest、deployment preflight、UI contract、UI visual/interaction、OIDC/SSO、业务流 API、completion snapshot |
| `pilot-deployment-proof.md` | 入场部署、拓扑、资源、版本和连续运行证明模板 | site values、K8s workload、Probe、APISIX、release package |
| `demo-script.md` | 售前/验收演示脚本 | UI 路由、状态机证据、业务流 API、Desktop Chrome smoke |
| `pilot-weekly-report-template.md` | 第 2-4 周试点周报模板 | 流量、资产、告警、取证、反馈、DLQ、延迟链 |
| `economic-benefit.md` | 经济效益测算模板 | MTTR、误报率、资产覆盖、取证耗时、人力成本 |
| `user-acceptance-signoff.md` | 用户确认和遗留事项签认模板 | 验收项、证据链接、例外项、双方签字 |
| `ipr-index.md` | 论文/专利/软著/成果转化索引模板 | 模块、证据、贡献点、归属关系 |

## Readiness 预检

`tests/e2e/live_third_party_signoff_readiness.sh` 会读取本目录模板和 `pilot-package-manifest.json` 的 evidence inputs，生成 `readiness/latest/` 下的复核草案：

- `readiness-manifest.bootstrap.json`
- `evidence-ledger.bootstrap.json`
- `placeholder-inventory.bootstrap.csv`
- `placeholder-owner-summary.bootstrap.json`
- `signoff-checklist.review-template.md`
- `exception-register.review-template.csv`
- `claim-boundary.review-template.md`

其中 `placeholder-inventory.bootstrap.csv` 包含 `owner` 和 `owner_reason` 列，`placeholder-owner-summary.bootstrap.json` 汇总每类缺口数量，summary 同步写入 `evidence_inputs` 与 `evidence_input_run_ids`，用于 release 绑定校验。该 readiness 只能证明“材料可以进入人工复核”，不能替代用户签字、第三方报告、试点盖章或经济效益确认。只要 `user-acceptance-signoff.md` 或 readiness package 仍含占位标记、签名缺口、上游 blocked 输入、陈旧 release 绑定或外部确认缺口，项目完成度审计必须保持 `trial_third_party_signoff` blocked。

## 使用规则

1. 每次试点前复制本目录模板到对应 release evidence package，并填写 site、版本、时间窗和证据链接。
2. 不在模板中写入明文 token、Secret、客户敏感 IP 明细或未脱敏 PCAP。
3. 每个演示步骤必须能回指到 API、DB/Kafka/Flink、UI 或报告证据，禁止只用截图代表真实链路。
4. 对外措辞必须区分 Template Ready、Evidence Ready、Acceptance Ready 和 Third-party Passed。
5. 客户签认、CNAS/第三方报告和经济效益证明进入本目录后，需同步更新 `doc/02_acceptance/README.md` 和 `doc/05_status/*`。
