# Codex Loop Guidance

- generated_from: `/home/wangwt/phase_2/code/traffic-analysis-platform/doc/02_acceptance/runs/mvp-49-gpt55-ranking-scout/context`
- ranking_model: `codex-cli/gpt5.5`
- ranking_model_execution: `planned_not_executed_by_guide`
- blockers: `1`
- warnings: `10`
- recommendations: `12`

## Correction Findings
- `warning` `HIGH_RISK_LOCAL` `CLE-P0-AUTH-001`: High-risk task is allowed to enter local mode. Suggestion: Keep security and reviewer gates mandatory; consider planning before implementation.
- `warning` `HIGH_RISK_LOCAL` `CLE-P0-SCREEN-001`: High-risk task is allowed to enter local mode. Suggestion: Keep security and reviewer gates mandatory; consider planning before implementation.
- `warning` `RUN_EVIDENCE_INCOMPLETE` `mvp-10-worker-adapter-repair`: Run is missing core files: patch-runner/codex-output-schema.json. Suggestion: Do not use this run to close a task until core evidence exists.
- `warning` `RUN_EVIDENCE_INCOMPLETE` `mvp-10-worker-cle-p0-screen-001`: Run is missing core files: patch-runner/codex-output-schema.json. Suggestion: Do not use this run to close a task until core evidence exists.
- `warning` `RUN_EVIDENCE_INCOMPLETE` `mvp-11-daemon-lease-i1-worker-cle-p0-screen-001`: Run is missing core files: patch-runner/codex-output-schema.json. Suggestion: Do not use this run to close a task until core evidence exists.
- `warning` `RUN_EVIDENCE_INCOMPLETE` `mvp-12-persistent-queue-metrics-i1-worker-cle-p0-screen-001`: Run is missing core files: patch-runner/codex-output-schema.json. Suggestion: Do not use this run to close a task until core evidence exists.
- `warning` `RUN_EVIDENCE_INCOMPLETE` `mvp-15-sqlite-service-once-daemon-i1-worker-cle-p0-screen-001`: Run is missing core files: patch-runner/codex-output-schema.json. Suggestion: Do not use this run to close a task until core evidence exists.
- `warning` `RUN_EVIDENCE_INCOMPLETE` `mvp-16-workflow-runner-prepare`: Run is missing core files: patch-runner/codex-output-schema.json. Suggestion: Do not use this run to close a task until core evidence exists.
- `warning` `RUN_EVIDENCE_INCOMPLETE` `mvp-46-production-maturity-audit`: Run is missing core files: task.yaml, plan.md, review-report.md. Suggestion: Do not use this run to close a task until core evidence exists.
- `warning` `RUN_EVIDENCE_INCOMPLETE` `mvp-9-patch-review-scheduler`: Run is missing core files: patch-runner/codex-output-schema.json. Suggestion: Do not use this run to close a task until core evidence exists.
- `info` `CONTRACT_IMPACT` `proto`: `proto` changes affect CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001. Suggestion: Expand verification to all listed dependent tasks before closing a contract-impacting change.
- `info` `CONTRACT_IMPACT` `kafka_topics`: `kafka_topics` changes affect CLE-P0-DLQ-001, CLE-P0-SEC-001. Suggestion: Expand verification to all listed dependent tasks before closing a contract-impacting change.
- `info` `CONTRACT_IMPACT` `database_schema`: `database_schema` changes affect CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001. Suggestion: Expand verification to all listed dependent tasks before closing a contract-impacting change.
- `blocker` `PREREQUISITE_OPEN` `CLE-P0-SCREEN-001`: Task cannot be executed or closed before prerequisites close: CLE-P0-ROUTE-001. Suggestion: Close the prerequisite tasks first, then rerun guidance and evidence check.

## Recommended Next
- score `1970` `CLE-P0-ROUTE-001` stage `ui-route-foundation` [P0/local/regression]: routeManifest 统一菜单、路由、权限、验收点. Reasoning: Route/menu/auth visibility foundation should precede page-level UI work.; has local verification command(s); declares live readonly smoke
- score `1520` `CLE-P0-PCAP-001` stage `contract-design` [P0/plan/regression]: PCAP hash、签名 URL、跨租户拒绝、下载审计. Reasoning: Multi-contract work should be designed before local implementation.; has local verification command(s); declares live readonly smoke
- score `1515` `CLE-P0-DLQ-001` stage `contract-design` [P0/plan/regression]: DLQ replay API、审批、审计、幂等验证. Reasoning: Multi-contract work should be designed before local implementation.; has local verification command(s); declares live readonly smoke
- score `1470` `CLE-P0-SEC-001` stage `contract-design` [P0/plan/security]: Kafka TLS/SASL/ACL、ExternalSecret、NetworkPolicy profile. Reasoning: Multi-contract work should be designed before local implementation.; has local verification command(s); declares live readonly smoke
- score `1455` `CLE-P0-REVIEWER-001` stage `closure-gate` [P0/review/regression]: 开启第三视角 Reviewer Gate. Reasoning: Reviewer and closure gates should exist before trusting task completion.; has local verification command(s); declares live readonly smoke
- score `1450` `CLE-P0-UIBACKUP-001` stage `rollback-safety` [P0/backup/regression]: 备份现有 web/ui 并生成旧前端清单. Reasoning: Rollback evidence should precede broad UI or code changes.; has local verification command(s); declares live readonly smoke
- score `1415` `CLE-P0-BASELINE-001` stage `baseline-evidence` [P0/plan/regression]: 生成 baseline/release manifest 草案. Reasoning: Baseline evidence gives later implementation and release checks a reference point.; has local verification command(s); declares live readonly smoke
- score `1410` `CLE-P0-P95-001` stage `contract-design` [P0/plan/acceptance-prep]: 完整 P95 时间戳链设计与埋点计划. Reasoning: Multi-contract work should be designed before local implementation.; has local verification command(s); declares live readonly smoke

## Ranking Reasoning
- `CLE-P0-ROUTE-001`: priority=1000, mode=35, risk=20, contract=25, unblocks=450, development_stage=330, test_logic=75, evidence_type=35
- `CLE-P0-PCAP-001`: priority=1000, mode=15, risk=80, contract=50, development_stage=230, test_logic=110, evidence_type=35
- `CLE-P0-DLQ-001`: priority=1000, mode=15, risk=80, contract=75, development_stage=230, test_logic=80, evidence_type=35
- `CLE-P0-SEC-001`: priority=1000, mode=15, risk=80, contract=50, development_stage=230, test_logic=110, evidence_type=-15
- `CLE-P0-REVIEWER-001`: priority=1000, mode=25, risk=20, development_stage=330, test_logic=45, evidence_type=35

## Status Suggestions
- `CLE-P0-ROUTE-001`: `DISCOVERED` -> `RECOMMENDED_NEXT` because Highest current priority after guidance scoring.
- `CLE-P0-PCAP-001`: `DISCOVERED` -> `RECOMMENDED_NEXT` because Highest current priority after guidance scoring.
- `CLE-P0-DLQ-001`: `DISCOVERED` -> `RECOMMENDED_NEXT` because Highest current priority after guidance scoring.
- `mvp-10-worker-adapter-repair`: `DESIGN_ITERATING` -> `EVIDENCE_INCOMPLETE` because Run has missing core files.
- `mvp-10-worker-cle-p0-screen-001`: `DESIGN_ITERATING` -> `EVIDENCE_INCOMPLETE` because Run has missing core files.
- `mvp-11-daemon-lease-i1-worker-cle-p0-screen-001`: `DESIGN_ITERATING` -> `EVIDENCE_INCOMPLETE` because Run has missing core files.
- `mvp-12-persistent-queue-metrics-i1-worker-cle-p0-screen-001`: `DESIGN_ITERATING` -> `EVIDENCE_INCOMPLETE` because Run has missing core files.
- `mvp-15-sqlite-service-once-daemon-i1-worker-cle-p0-screen-001`: `DESIGN_ITERATING` -> `EVIDENCE_INCOMPLETE` because Run has missing core files.
- `mvp-16-workflow-runner-prepare`: `DESIGN_ITERATING` -> `EVIDENCE_INCOMPLETE` because Run has missing core files.
- `mvp-46-production-maturity-audit`: `MATURITY_AUDIT_PARTIAL` -> `EVIDENCE_INCOMPLETE` because Run has missing core files.
- `mvp-9-patch-review-scheduler`: `DESIGN_ITERATING` -> `EVIDENCE_INCOMPLETE` because Run has missing core files.

## Guardrail
- This guidance does not modify task status by itself.
- A blocker means do not close the affected task or evidence run until the issue is resolved.
- A recommendation is a scheduling hint, not permission to bypass task gates.
