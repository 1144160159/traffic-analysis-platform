# Codex Loop Ranking Reasoning Request

You are the Codex Loop ranking reasoner. Rerank tasks only when the development and test order requires it.

## Hard Rules
- Do not rank a task above an open prerequisite that blocks it.
- Prefer route/menu/auth foundations before page-level UI tasks.
- Prefer rollback, reviewer and baseline evidence before broad implementation when their tasks are open.
- Prefer design/planning for contract-impacting cross-subsystem work before local implementation.
- Prefer tasks with concrete local tests and negative/security close_when over vague acceptance claims.
- Return JSON only and match the schema.

## Input
```json
{
  "ranking_model": {
    "version": "codex-cli-gpt5.5-dev-test-reasoning-v1",
    "provider": "codex-cli",
    "model": "gpt5.5",
    "model_source": "Codex CLI `codex exec --model`",
    "execution": "planned_not_executed_by_guide",
    "rules": [
      "explicit prerequisites block scheduling and closure until CLOSED",
      "tasks that unlock blocked dependents receive priority",
      "rollback, review and route foundations are ranked before page-level local UI work",
      "contract-impacting tasks in plan mode are preferred before multi-subsystem implementation",
      "negative/security close_when and concrete local verification improve test-readiness",
      "acceptance and third-party evidence types are delayed until regression foundations are ready"
    ],
    "request": "doc/02_acceptance/runs/mvp-49-gpt55-ranking-guidance/guidance/ranking-reasoning-request.md",
    "schema": "doc/02_acceptance/runs/mvp-49-gpt55-ranking-guidance/guidance/ranking-reasoning-schema.json",
    "command_template": "doc/02_acceptance/runs/mvp-49-gpt55-ranking-guidance/guidance/ranking-reasoning-command.txt",
    "command": "codex exec --model gpt5.5 --sandbox read-only --output-schema doc/02_acceptance/runs/mvp-49-gpt55-ranking-guidance/guidance/ranking-reasoning-schema.json 'Read the Codex Loop ranking reasoning request at {prompt}. Return JSON only matching the provided schema.'"
  },
  "rules": [
    "explicit prerequisites block scheduling and closure until CLOSED",
    "tasks that unlock blocked dependents receive priority",
    "rollback, review and route foundations are ranked before page-level local UI work",
    "contract-impacting tasks in plan mode are preferred before multi-subsystem implementation",
    "negative/security close_when and concrete local verification improve test-readiness",
    "acceptance and third-party evidence types are delayed until regression foundations are ready"
  ],
  "findings": [
    {
      "level": "warning",
      "code": "HIGH_RISK_LOCAL",
      "target": "CLE-P0-AUTH-001",
      "message": "High-risk task is allowed to enter local mode.",
      "suggestion": "Keep security and reviewer gates mandatory; consider planning before implementation."
    },
    {
      "level": "warning",
      "code": "HIGH_RISK_LOCAL",
      "target": "CLE-P0-SCREEN-001",
      "message": "High-risk task is allowed to enter local mode.",
      "suggestion": "Keep security and reviewer gates mandatory; consider planning before implementation."
    },
    {
      "level": "warning",
      "code": "RUN_EVIDENCE_INCOMPLETE",
      "target": "mvp-10-worker-adapter-repair",
      "message": "Run is missing core files: patch-runner/codex-output-schema.json.",
      "suggestion": "Do not use this run to close a task until core evidence exists."
    },
    {
      "level": "warning",
      "code": "RUN_EVIDENCE_INCOMPLETE",
      "target": "mvp-10-worker-cle-p0-screen-001",
      "message": "Run is missing core files: patch-runner/codex-output-schema.json.",
      "suggestion": "Do not use this run to close a task until core evidence exists."
    },
    {
      "level": "warning",
      "code": "RUN_EVIDENCE_INCOMPLETE",
      "target": "mvp-11-daemon-lease-i1-worker-cle-p0-screen-001",
      "message": "Run is missing core files: patch-runner/codex-output-schema.json.",
      "suggestion": "Do not use this run to close a task until core evidence exists."
    },
    {
      "level": "warning",
      "code": "RUN_EVIDENCE_INCOMPLETE",
      "target": "mvp-12-persistent-queue-metrics-i1-worker-cle-p0-screen-001",
      "message": "Run is missing core files: patch-runner/codex-output-schema.json.",
      "suggestion": "Do not use this run to close a task until core evidence exists."
    },
    {
      "level": "warning",
      "code": "RUN_EVIDENCE_INCOMPLETE",
      "target": "mvp-15-sqlite-service-once-daemon-i1-worker-cle-p0-screen-001",
      "message": "Run is missing core files: patch-runner/codex-output-schema.json.",
      "suggestion": "Do not use this run to close a task until core evidence exists."
    },
    {
      "level": "warning",
      "code": "RUN_EVIDENCE_INCOMPLETE",
      "target": "mvp-16-workflow-runner-prepare",
      "message": "Run is missing core files: patch-runner/codex-output-schema.json.",
      "suggestion": "Do not use this run to close a task until core evidence exists."
    },
    {
      "level": "warning",
      "code": "RUN_EVIDENCE_INCOMPLETE",
      "target": "mvp-46-production-maturity-audit",
      "message": "Run is missing core files: task.yaml, plan.md, review-report.md.",
      "suggestion": "Do not use this run to close a task until core evidence exists."
    },
    {
      "level": "warning",
      "code": "RUN_EVIDENCE_INCOMPLETE",
      "target": "mvp-9-patch-review-scheduler",
      "message": "Run is missing core files: patch-runner/codex-output-schema.json.",
      "suggestion": "Do not use this run to close a task until core evidence exists."
    },
    {
      "level": "info",
      "code": "CONTRACT_IMPACT",
      "target": "proto",
      "message": "`proto` changes affect CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001.",
      "suggestion": "Expand verification to all listed dependent tasks before closing a contract-impacting change."
    },
    {
      "level": "info",
      "code": "CONTRACT_IMPACT",
      "target": "kafka_topics",
      "message": "`kafka_topics` changes affect CLE-P0-DLQ-001, CLE-P0-SEC-001.",
      "suggestion": "Expand verification to all listed dependent tasks before closing a contract-impacting change."
    },
    {
      "level": "info",
      "code": "CONTRACT_IMPACT",
      "target": "database_schema",
      "message": "`database_schema` changes affect CLE-P0-DLQ-001, CLE-P0-P95-001, CLE-P0-PCAP-001.",
      "suggestion": "Expand verification to all listed dependent tasks before closing a contract-impacting change."
    },
    {
      "level": "blocker",
      "code": "PREREQUISITE_OPEN",
      "target": "CLE-P0-SCREEN-001",
      "message": "Task cannot be executed or closed before prerequisites close: CLE-P0-ROUTE-001.",
      "suggestion": "Close the prerequisite tasks first, then rerun guidance and evidence check."
    }
  ],
  "candidates": [
    {
      "id": "CLE-P0-ROUTE-001",
      "title": "routeManifest 统一菜单、路由、权限、验收点",
      "priority": "P0",
      "status": "DISCOVERED",
      "stage": "ui-route-foundation",
      "mode": "local",
      "acceptance_type": "regression",
      "score": 1970,
      "score_components": {
        "priority": 1000,
        "mode": 35,
        "risk": 20,
        "contract": 25,
        "correction": 0,
        "unblocks": 450,
        "development_stage": 330,
        "test_logic": 75,
        "status": 0,
        "evidence_type": 35,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Route/menu/auth visibility foundation should precede page-level UI work.",
        "has local verification command(s)",
        "declares live readonly smoke",
        "close_when includes negative/security behavior",
        "unblocks CLE-P0-SCREEN-001"
      ],
      "blocked_by": [],
      "unblocks": [
        "CLE-P0-SCREEN-001"
      ]
    },
    {
      "id": "CLE-P0-PCAP-001",
      "title": "PCAP hash、签名 URL、跨租户拒绝、下载审计",
      "priority": "P0",
      "status": "DISCOVERED",
      "stage": "contract-design",
      "mode": "plan",
      "acceptance_type": "regression",
      "score": 1520,
      "score_components": {
        "priority": 1000,
        "mode": 15,
        "risk": 80,
        "contract": 50,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 230,
        "test_logic": 110,
        "status": 0,
        "evidence_type": 35,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Multi-contract work should be designed before local implementation.",
        "has local verification command(s)",
        "declares live readonly smoke",
        "close_when includes negative/security behavior",
        "contract-impacting task is still in plan mode"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P0-DLQ-001",
      "title": "DLQ replay API、审批、审计、幂等验证",
      "priority": "P0",
      "status": "DISCOVERED",
      "stage": "contract-design",
      "mode": "plan",
      "acceptance_type": "regression",
      "score": 1515,
      "score_components": {
        "priority": 1000,
        "mode": 15,
        "risk": 80,
        "contract": 75,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 230,
        "test_logic": 80,
        "status": 0,
        "evidence_type": 35,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Multi-contract work should be designed before local implementation.",
        "has local verification command(s)",
        "declares live readonly smoke",
        "contract-impacting task is still in plan mode"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P0-SEC-001",
      "title": "Kafka TLS/SASL/ACL、ExternalSecret、NetworkPolicy profile",
      "priority": "P0",
      "status": "DISCOVERED",
      "stage": "contract-design",
      "mode": "plan",
      "acceptance_type": "security",
      "score": 1470,
      "score_components": {
        "priority": 1000,
        "mode": 15,
        "risk": 80,
        "contract": 50,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 230,
        "test_logic": 110,
        "status": 0,
        "evidence_type": -15,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Multi-contract work should be designed before local implementation.",
        "has local verification command(s)",
        "declares live readonly smoke",
        "close_when includes negative/security behavior",
        "contract-impacting task is still in plan mode"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P0-REVIEWER-001",
      "title": "开启第三视角 Reviewer Gate",
      "priority": "P0",
      "status": "DISCOVERED",
      "stage": "closure-gate",
      "mode": "review",
      "acceptance_type": "regression",
      "score": 1455,
      "score_components": {
        "priority": 1000,
        "mode": 25,
        "risk": 20,
        "contract": 0,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 330,
        "test_logic": 45,
        "status": 0,
        "evidence_type": 35,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Reviewer and closure gates should exist before trusting task completion.",
        "has local verification command(s)",
        "declares live readonly smoke"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P0-UIBACKUP-001",
      "title": "备份现有 web/ui 并生成旧前端清单",
      "priority": "P0",
      "status": "DISCOVERED",
      "stage": "rollback-safety",
      "mode": "backup",
      "acceptance_type": "regression",
      "score": 1450,
      "score_components": {
        "priority": 1000,
        "mode": 30,
        "risk": 20,
        "contract": 0,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 320,
        "test_logic": 45,
        "status": 0,
        "evidence_type": 35,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Rollback evidence should precede broad UI or code changes.",
        "has local verification command(s)",
        "declares live readonly smoke"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P0-BASELINE-001",
      "title": "生成 baseline/release manifest 草案",
      "priority": "P0",
      "status": "DISCOVERED",
      "stage": "baseline-evidence",
      "mode": "plan",
      "acceptance_type": "regression",
      "score": 1415,
      "score_components": {
        "priority": 1000,
        "mode": 15,
        "risk": 20,
        "contract": 0,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 300,
        "test_logic": 45,
        "status": 0,
        "evidence_type": 35,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Baseline evidence gives later implementation and release checks a reference point.",
        "has local verification command(s)",
        "declares live readonly smoke"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P0-P95-001",
      "title": "完整 P95 时间戳链设计与埋点计划",
      "priority": "P0",
      "status": "DISCOVERED",
      "stage": "contract-design",
      "mode": "plan",
      "acceptance_type": "acceptance-prep",
      "score": 1410,
      "score_components": {
        "priority": 1000,
        "mode": 15,
        "risk": 80,
        "contract": 50,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 230,
        "test_logic": 80,
        "status": 0,
        "evidence_type": -45,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Multi-contract work should be designed before local implementation.",
        "has local verification command(s)",
        "declares live readonly smoke",
        "contract-impacting task is still in plan mode"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P0-AUTH-001",
      "title": "启动 /auth/me 鉴权和 WebSocket 延迟连接",
      "priority": "P0",
      "status": "DISCOVERED",
      "stage": "ui-feature-local",
      "mode": "local",
      "acceptance_type": "regression",
      "score": 1355,
      "score_components": {
        "priority": 1000,
        "mode": 35,
        "risk": 80,
        "contract": 0,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 130,
        "test_logic": 75,
        "status": 0,
        "evidence_type": 35,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Page-level UI work should follow route/auth foundations and carry local regression tests.",
        "has local verification command(s)",
        "declares live readonly smoke",
        "close_when includes negative/security behavior"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P1-FUSION-001",
      "title": "多源融合消融实验框架",
      "priority": "P1",
      "status": "DISCOVERED",
      "stage": "acceptance-prep",
      "mode": "plan",
      "acceptance_type": "acceptance-prep",
      "score": 625,
      "score_components": {
        "priority": 500,
        "mode": 15,
        "risk": 20,
        "contract": 0,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 90,
        "test_logic": 45,
        "status": 0,
        "evidence_type": -45,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Acceptance-prep work is important but should usually follow P0 regression foundations.",
        "has local verification command(s)",
        "declares live readonly smoke"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P1-PILOT-001",
      "title": "试点交付包和 25 分钟演示证据脚本",
      "priority": "P1",
      "status": "DISCOVERED",
      "stage": "acceptance-prep",
      "mode": "plan",
      "acceptance_type": "third-party-prep",
      "score": 580,
      "score_components": {
        "priority": 500,
        "mode": 15,
        "risk": 20,
        "contract": 0,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 90,
        "test_logic": 45,
        "status": 0,
        "evidence_type": -90,
        "prerequisite_penalty": 0,
        "blocker_penalty": 0,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Acceptance-prep work is important but should usually follow P0 regression foundations.",
        "has local verification command(s)",
        "declares live readonly smoke"
      ],
      "blocked_by": [],
      "unblocks": []
    },
    {
      "id": "CLE-P0-SCREEN-001",
      "title": "/screen 只读 token 或脱敏公开边界",
      "priority": "P0",
      "status": "DESIGN_ITERATING",
      "stage": "ui-feature-local",
      "mode": "local",
      "acceptance_type": "regression",
      "score": -1225,
      "score_components": {
        "priority": 1000,
        "mode": 35,
        "risk": 80,
        "contract": 0,
        "correction": 0,
        "unblocks": 0,
        "development_stage": 130,
        "test_logic": 75,
        "status": -80,
        "evidence_type": 35,
        "prerequisite_penalty": -2000,
        "blocker_penalty": -500,
        "terminal_penalty": 0
      },
      "reasoning": [
        "Page-level UI work should follow route/auth foundations and carry local regression tests.",
        "has local verification command(s)",
        "declares live readonly smoke",
        "close_when includes negative/security behavior",
        "blocked by open prerequisite(s): CLE-P0-ROUTE-001"
      ],
      "blocked_by": [
        "CLE-P0-ROUTE-001"
      ],
      "unblocks": []
    }
  ]
}
```
