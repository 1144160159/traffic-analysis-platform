# Codex Loop Guidance

- generated_from: `/home/wangwt/phase_2/code/traffic-analysis-platform/doc/02_acceptance/runs/mvp-47-post-close-scout/context`
- blockers: `0`
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

## Recommended Next
- score `1450` `CLE-P0-UIBACKUP-001` [P0/backup/regression]: 备份现有 web/ui 并生成旧前端清单
- score `1170` `CLE-P0-DLQ-001` [P0/plan/regression]: DLQ replay API、审批、审计、幂等验证
- score `1145` `CLE-P0-P95-001` [P0/plan/acceptance-prep]: 完整 P95 时间戳链设计与埋点计划
- score `1145` `CLE-P0-PCAP-001` [P0/plan/regression]: PCAP hash、签名 URL、跨租户拒绝、下载审计
- score `1145` `CLE-P0-SEC-001` [P0/plan/security]: Kafka TLS/SASL/ACL、ExternalSecret、NetworkPolicy profile
- score `1115` `CLE-P0-AUTH-001` [P0/local/regression]: 启动 /auth/me 鉴权和 WebSocket 延迟连接
- score `1080` `CLE-P0-ROUTE-001` [P0/local/regression]: routeManifest 统一菜单、路由、权限、验收点
- score `1045` `CLE-P0-REVIEWER-001` [P0/review/regression]: 开启第三视角 Reviewer Gate

## Status Suggestions
- `CLE-P0-UIBACKUP-001`: `DISCOVERED` -> `RECOMMENDED_NEXT` because Highest current priority after guidance scoring.
- `CLE-P0-DLQ-001`: `DISCOVERED` -> `RECOMMENDED_NEXT` because Highest current priority after guidance scoring.
- `CLE-P0-P95-001`: `DISCOVERED` -> `RECOMMENDED_NEXT` because Highest current priority after guidance scoring.
- `CLE-P0-SCREEN-001`: `CLOSED` -> `EVIDENCE_INCOMPLETE` because Closed task lacks complete core evidence.
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
