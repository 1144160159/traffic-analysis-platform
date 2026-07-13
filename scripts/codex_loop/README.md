# Codex Loop Engineering MVP

本目录是 `doc/01_design/自动开发Loop引擎设计.md` 的第一阶段落地骨架。它默认不启动常驻进程，但提供可选服务控制面，并包含只读发现、运行前 preflight、per-lane resource quota、dynamic resource monitor、workspace isolation planner、executor pool stress、remote pool stress、remote pool K8s stress、remote pool K8s readiness、objective stop、bounded soak、隔离执行计划、隔离执行池桥接器、bounded executor pool、HTTP queue service、上帝视角快照、纠偏引导、设计包生成、任务级上下文包、工作流编排、租约锁、bounded daemon、持久队列、运行指标、实现守门、Codex patch work order、模型画像选择、安全 Codex runner、外部 Codex adapter、diff-aware/semantic/LLM Reviewer、证据判定、失败修复计划、自动修复下一步计划、任务队列/锁/重试计划、轻量 worker、任务状态闭环、任务建模、计划生成、证据收集和受控本地执行。

## 安全边界

- 默认只生成计划和证据骨架，不自动修改业务代码。
- `run_task.py` 只有显式传入 `--execute-local` 才会执行任务里的本地验证命令。
- `service.py once/run/start` 默认先执行 runtime preflight；只有 blocker 会阻断，warning 会进入健康/发布证据。
- `scheduler.py` 默认执行 per-lane resource quota；可用 `--skip-quota` 仅作调试，不建议生产入口关闭。
- `resource_monitor.py` 采集 CPU/load/memory/disk/process/thread/queue 压力；BLOCKED 会阻断 preflight/executor pool，DEGRADED 会让 executor pool 降到单 worker。
- `workspace_isolation.py` 默认只规划每任务 workspace，不创建、不删除工作区；`--create-worktrees` 必须显式设置 `CODEX_LOOP_ALLOW_WORKTREE_CREATE=1`，workspace backend 可选 `git-worktree` 或 `local-clone`，生成内容基于 HEAD，不包含未提交改动。`executor_pool.py --activate-workspaces` 只会使用已创建且策略允许的 workspace，否则直接阻断。
- `sandbox.py` 默认只渲染隔离执行计划，不 apply K8s 资源，不启动本地容器；`sandbox_executor.py` 默认只审计计划，真实执行必须显式 `--execute` 并设置环境闸门。
- `sandbox_worker.py` 默认只把 scheduler queue 转成 sandbox plans；只有同时传入 `--execute-sandbox --claim-queue` 才会调用 executor 并回写队列状态。
- `executor_pool.py` 默认使用 `sandbox-plan` runner，只并发生成隔离计划；非 plan 型并发必须显式 `--allow-parallel-execution` 且使用 SQLite 队列，`sandbox-execute` 仍受 `CODEX_LOOP_ALLOW_SANDBOX_EXECUTION` 闸门约束。
- `soak.py` 只重复编排 bounded service/daemon cycle、resource monitor、health 和 metrics；它不绕过 preflight、worker、sandbox、queue、reviewer、evidence 或外部 Codex 闸门。
- `queue_service.py` 默认绑定 loopback 和 SQLite 队列；非 loopback 暴露必须提供 token 环境变量，HTTP 服务只代理 queue 操作，不绕过 scheduler/worker/reviewer/evidence gate。
- `remote_pool_stress.py` 只压测 loopback HTTP queue service 的 enqueue/claim/complete/fail 仲裁和 lease owner 回写校验；不执行真实任务、不连接生产服务、不绕过 worker/reviewer/evidence gate。
- `remote_pool_k8s_stress.py` 默认只渲染 K8s Indexed Job 和 dry-run 证据；真实 apply 必须显式 `--execute` 且设置 `CODEX_LOOP_ALLOW_K8S_REMOTE_POOL_STRESS=1`，worker Pod 只处理 synthetic queue task。
- `remote_pool_k8s_readiness.py` 只读审计 namespace、Service、PVC、Secret、worker Job dry-run 和执行闸门；不会创建/修改/删除 K8s 资源，也不会写出 Secret 值。
- `k8s_bootstrap.py` 默认只验证 queue service 的 PVC/Deployment/Service 和 Secret 创建命令；真实 apply 必须显式 `--execute`、`CODEX_LOOP_ALLOW_K8S_BOOTSTRAP=1` 和 `CODEX_LOOP_QUEUE_TOKEN`。
- `objective_stop.py` 只评估项目级停止条件，不修改任务、不执行部署；`OBJECTIVE_STOP_READY` 才是成功停止，`BLOCKED/HUMAN_GATE` 是修复或人工决策停止。
- `model_profile.py` 只选择外部 Codex 的模型、sandbox、schema 和命令模板；它不调用模型、不应用 patch，并且命令模板仍要通过 `codex_runner.py` 的策略白名单。
- `llm_reviewer.py` 默认只生成 LLM 审阅请求、JSON Schema、模型画像和命令模板；只有外部模型输出被显式提供时才做 intake，非 pass 结论会进入 evidence blocker。
- `codex_runner.py` 默认只生成外部 Codex 执行计划；真实执行必须同时传入 `--execute`，并设置策略文件指定的环境闸门。
- live 写入、生产 Secret、证书、PVC、Topic、Bucket、数据库破坏性操作不在 MVP 自动执行范围内。
- smoke、regression、acceptance、third-party 证据必须分层记录，不能互相替代。

## 常用命令

校验并评估一轮约束式学习 episode。该命令先执行 JSON Schema 和冻结策略校验，再检查全部硬门禁；只有硬门禁全部通过才计算 reward。它只写评估证据，不修改业务代码、任务状态或生产环境：

```bash
python scripts/codex_loop/learning_episode.py \
  --episode doc/02_acceptance/runs/<run_id>/learning/episode.json
```

默认输出到 episode 同级的 `evaluation/`：

```text
evaluation/
  episode.evaluated.json
  episode-report.md
```

只做契约检查可使用 `--validate-only`；需要集中输出时可使用 `--output-dir <path>`。校验器同时拒绝重复/未知 gate、仓库外证据路径、不存在的证据文件和 SHA-256 不匹配。策略和 Schema 分别固定在 `doc/04_assets/ui_suite_gpt_v1/specs/reinforcement-learning-policy.json` 与 `learning-episode.schema.json`，禁止根据单次结果临时改权重或绕过硬门禁。

生成 P0/P1 任务池：

```bash
python scripts/codex_loop/discover.py --priority all --out scripts/codex_loop/tasks
```

生成架构地图：

```bash
python scripts/codex_loop/discover.py \
  --architecture-map doc/02_acceptance/runs/manual/context/architecture-map.json
```

生成 Context Scout 上帝视角账本：

```bash
python scripts/codex_loop/scout.py --run-id 20260623-scout
```

评估项目级目标停止条件：

```bash
python scripts/codex_loop/objective_stop.py \
  --run-id 20260623-objective-stop \
  --objective "完成 traffic-analysis-platform 项目开发" \
  --context-run-id 20260623-scout \
  --release-run-id 20260623-loop-release
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/objective-stop/
  stop-summary.json
  stop-report.md
  stop-policy.json
```

停止状态含义：`OBJECTIVE_STOP_READY` 表示目标完成且可成功停止；`OBJECTIVE_STOP_CONTINUE` 表示没有硬阻断但任务或证据仍需继续；`OBJECTIVE_STOP_BLOCKED` 表示应停止自动循环进入修复；`OBJECTIVE_STOP_HUMAN_GATE` 表示需要人工裁决。`stop-summary.json` 同时写入 `stop_conditions`，用机器可读结构固定成功停止、继续循环、修复停止和人工门禁停止的判断条件；`--no-write` 可用于只读检查，不落盘。guidance 中指向未闭合任务的 blocker 会转成 `GUIDANCE_BLOCKS_OPEN_TASK` pending，防止把“当前任务需要纠偏”误判成“整个 loop 需要停止”；全局 blocker 或已闭合任务被反证才会让目标进入 BLOCKED。

生成生产级能力成熟度审计。该命令只读扫描脚本、策略、任务池和历史 run summary，不执行任务、不修改状态、不 apply K8s、不调用外部 Codex：

```bash
python scripts/codex_loop/maturity_audit.py \
  --run-id 20260623-production-maturity-audit
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/maturity-audit/
  maturity-audit.json
  maturity-audit.md
```

成熟度状态含义：`MATURITY_AUDIT_READY` 表示所有能力域都有实现和证据；`MATURITY_AUDIT_PARTIAL` 表示控制面已具备但缺少真实任务闭合、长稳或模型/执行证据；`MATURITY_AUDIT_INCOMPLETE` 表示存在生产级必需能力缺失。

运行生产入口 preflight：

```bash
python scripts/codex_loop/preflight.py \
  --run-id 20260623-preflight \
  --profile sqlite_conservative \
  --queue-backend sqlite
```

该命令会检查 Python 版本、关键工具、关键路径、磁盘/内存、队列路径、资源 profile 安全项和 workspace lock，写入：

```text
doc/02_acceptance/runs/<run_id>/preflight/
  preflight.json
  preflight.md
```

生成隔离执行计划。默认策略只允许 `prepare` / `dry-run`，禁止 live 写入、外部 Codex、service account token、特权提升和网络 egress：

```bash
python scripts/codex_loop/sandbox.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-sandbox-screen \
  --stage prepare \
  --driver kubernetes-job \
  --validate
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/sandbox/
  sandbox-plan.json
  sandbox-report.md
  codex-loop-sandbox-job.yaml
  codex-loop-sandbox-networkpolicy.yaml
  local-container-command.txt
  validation.json
  kubectl-dry-run.txt
```

也可以只生成本地容器命令计划：

```bash
python scripts/codex_loop/sandbox.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-sandbox-local-screen \
  --stage prepare \
  --driver local-container
```

审计或执行隔离计划：

```bash
python scripts/codex_loop/sandbox_executor.py \
  --sandbox-plan doc/02_acceptance/runs/20260623-sandbox-screen/sandbox/sandbox-plan.json \
  --run-id 20260623-sandbox-exec-screen
```

默认只写 `SANDBOX_EXECUTION_PLANNED`。真实执行必须显式开启：

```bash
CODEX_LOOP_ALLOW_SANDBOX_EXECUTION=1 python scripts/codex_loop/sandbox_executor.py \
  --sandbox-plan doc/02_acceptance/runs/20260623-sandbox-screen/sandbox/sandbox-plan.json \
  --run-id 20260623-sandbox-exec-screen \
  --execute \
  --cleanup
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/sandbox-executor/
  execution.json
  execution-report.md
  *.stdout.txt
  *.stderr.txt
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/context/
  context.snapshot.json
  gap-index.json
  dependency-map.json
  evidence-ledger.json
  god-view.md
```

生成纠偏与下一步引导：

```bash
python scripts/codex_loop/guide.py \
  --context-dir doc/02_acceptance/runs/20260623-scout/context \
  --run-id 20260623-guidance
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/guidance/
  guidance.json
  guidance-report.md
```

生成上帝视角设计包：

```bash
python scripts/codex_loop/design.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --context-dir doc/02_acceptance/runs/20260623-scout/context \
  --guidance doc/02_acceptance/runs/20260623-guidance/guidance/guidance.json \
  --run-id 20260623-design-screen
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/design/
  design-summary.json
  product-iteration.md
  feature-spec.md
  user-flow.md
  state-machine.md
  api-contract.md
  data-contract.md
  visual-correction.md
  architecture-evolution.md
  acceptance-cases.md
  implementation-plan.md
```

生成任务级上下文包，用于处理很长的自动化上下文：

```bash
python scripts/codex_loop/context_pack.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --context-dir doc/02_acceptance/runs/20260623-scout/context \
  --guidance doc/02_acceptance/runs/20260623-guidance/guidance/guidance.json \
  --design-dir doc/02_acceptance/runs/20260623-design-screen/design \
  --run-id 20260623-context-screen \
  --max-chars 12000
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/context-pack/
  task-context-pack.md
  task-context-pack.json
  context-budget.json
  decision-log.jsonl
  handoff.md
```

从上帝视角到具体任务执行的工作流总控：

```bash
python scripts/codex_loop/workflow.py \
  --context-dir doc/02_acceptance/runs/20260623-scout/context \
  --guidance doc/02_acceptance/runs/20260623-guidance/guidance/guidance.json \
  --run-id 20260623-workflow \
  --stage prepare
```

如果不传 `--task`，`workflow.py` 会从 `guidance.recommended_next` 选择第一个存在 YAML 的任务。`--stage prepare` 只生成设计、上下文包、计划、实现简报、审阅模板和证据；`--stage dry-run` 会调用 `run_task.py` 但不执行本地命令；`--stage execute-local` 才会传入 `--execute-local`。若任务存在 blocker，默认写入 `workflow/gate-decision.md` 并阻止执行。

工作流末尾会自动调用 `evidence_check.py`。如果证据不满足任务关闭条件，会继续生成 `repair.py` 的修复计划；这使 Codex 的一次失败能够回写成下一轮可执行输入，而不是只停留在聊天上下文里。

工作流在实现简报之后还会调用 `patch_runner.py` 生成 Codex patch work order、结构化输出契约和 JSON Schema，再由 `model_profile.py` 选择模型画像和默认命令模板，随后 `codex_adapter.py` 生成兼容调用计划，`codex_runner.py` 生成安全执行审计，并让 `review.py` / `semantic_reviewer.py` / `llm_reviewer.py` 读取本轮 patch scope、guidance、diff、local report、设计上下文和证据口径做第三视角审阅。

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/workflow/
  workflow-summary.json
  workflow-report.md
  gate-decision.md
doc/02_acceptance/runs/<run_id>/patch-runner/
  patch-request.md
  patch-request.json
  codex-output-contract.json
  codex-output-schema.json
  patch-intake.json
  patch-runner-summary.json
doc/02_acceptance/runs/<run_id>/model-profile/
  model-profile.json
  model-profile.md
  command-template.txt
doc/02_acceptance/runs/<run_id>/codex-adapter/
  invocation-plan.md
  invocation.json
  stdout.txt
  stderr.txt
doc/02_acceptance/runs/<run_id>/codex-runner/
  invocation.json
  codex-runner-report.md
  stdout.txt
  stderr.txt
doc/02_acceptance/runs/<run_id>/review/
  review-summary.json
doc/02_acceptance/runs/<run_id>/semantic-review/
  semantic-review.json
  semantic-review-report.md
doc/02_acceptance/runs/<run_id>/llm-review/
  llm-review-request.md
  llm-review-schema.json
  llm-review-profile.json
  command-template.txt
  llm-review-summary.json
  llm-review-report.md
doc/02_acceptance/runs/<run_id>/evidence-check/
  evidence-check.json
  evidence-check-report.md
doc/02_acceptance/runs/<run_id>/repair/
  repair-plan.json
  repair-report.md
  codex-repair-prompt.md
doc/02_acceptance/runs/<run_id>/auto-repair/
  auto-repair-summary.json
  auto-repair-report.md
```

生成任务状态看板和迁移计划：

```bash
python scripts/codex_loop/task_state.py \
  --context-dir doc/02_acceptance/runs/20260623-scout/context \
  --guidance doc/02_acceptance/runs/20260623-guidance/guidance/guidance.json \
  --run-id 20260623-task-state
```

默认只生成状态建议，不修改任务 YAML。确认后可加 `--apply` 写回 `scripts/codex_loop/tasks/*.yaml` 的 `status` 字段。

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/task-state/
  task-state.json
  task-board.md
  transition-plan.json
  apply-report.md
  transition-log.jsonl
```

生成实现简报和 patch 范围校验：

```bash
python scripts/codex_loop/implement.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --context-pack doc/02_acceptance/runs/20260623-workflow/context-pack/task-context-pack.md \
  --design-dir doc/02_acceptance/runs/20260623-workflow/design \
  --plan doc/02_acceptance/runs/20260623-workflow/plan.md \
  --guidance doc/02_acceptance/runs/20260623-workflow/guidance/guidance.json \
  --run-id 20260623-implement-screen
```

默认不修改业务代码。若提供 unified diff，可用 `--patch <file>` 做范围和契约校验；只有同时提供 `--apply-patch` 才会在校验通过后调用 `git apply`。

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/implementation/
  implementation-brief.md
  codex-implementation-prompt.md
  patch-scope.json
  patch-validation.json
  apply-report.md
```

生成 Codex patch work order，或校验 Codex 返回的 patch：

```bash
python scripts/codex_loop/patch_runner.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --context-pack doc/02_acceptance/runs/20260623-workflow/context-pack/task-context-pack.md \
  --design-dir doc/02_acceptance/runs/20260623-workflow/design \
  --guidance doc/02_acceptance/runs/20260623-workflow/guidance/guidance.json \
  --run-id 20260623-workflow
```

默认只写 patch request 和 `codex-output-contract.json`。如果有 unified diff，可加 `--patch <file>` 校验范围、契约和 blocker；只有同时提供 `--apply-patch` 才会应用 patch。

生成外部 Codex 模型画像和默认命令模板：

```bash
python scripts/codex_loop/model_profile.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-workflow \
  --patch-request doc/02_acceptance/runs/20260623-workflow/patch-runner/patch-request.md \
  --output-schema doc/02_acceptance/runs/20260623-workflow/patch-runner/codex-output-schema.json
```

`model_profile.py` 会按 `policies/model-profiles.yaml` 选择 profile、模型、sandbox、timeout 和 `codex exec` 命令模板，并用 `policies/codex-execution.yaml` 预校验命令片段。输出只是一份执行画像，不调用模型、不信任 patch。

生成外部 Codex 调用计划，或显式执行外部 Codex 命令：

```bash
python scripts/codex_loop/codex_adapter.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-workflow \
  --patch-request doc/02_acceptance/runs/20260623-workflow/patch-runner/patch-request.md
```

默认不执行外部命令。只有同时提供 `--execute --command '...'` 才会运行；命令可用 `{prompt}` 占位 patch request 路径。

通过安全 runner 生成外部 Codex 执行计划：

```bash
python scripts/codex_loop/codex_runner.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-workflow \
  --patch-request doc/02_acceptance/runs/20260623-workflow/patch-runner/patch-request.md \
  --model-profile doc/02_acceptance/runs/20260623-workflow/model-profile/model-profile.json
```

`codex_runner.py` 是生产推荐入口：默认只写 `codex-runner/invocation.json`、`codex-runner-report.md`、`stdout.txt` 和 `stderr.txt`。它不用 shell 执行命令，只允许 `policies/codex-execution.yaml` 白名单中的二进制和命令前缀，只转发白名单环境变量，并在落盘前脱敏 stdout/stderr。真实执行还必须加 `--execute`，并显式设置：

```bash
CODEX_LOOP_ALLOW_EXTERNAL_CODEX=1 python scripts/codex_loop/codex_runner.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-workflow \
  --patch-request doc/02_acceptance/runs/20260623-workflow/patch-runner/patch-request.md \
  --model-profile doc/02_acceptance/runs/20260623-workflow/model-profile/model-profile.json \
  --execute
```

外部 Codex 产出的 patch 仍需回到 `patch_runner.py --patch ...` 校验和可选应用，不能绕过 Reviewer 和 `evidence_check.py`。

检查任务证据是否足够关闭：

```bash
python scripts/codex_loop/evidence_check.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-workflow \
  --guidance doc/02_acceptance/runs/20260623-workflow/guidance/guidance.json
```

该命令会保守检查 evidence.required、证据层级、run 状态、guidance blocker、patch runner/patch 校验、本地验证、live/browser 报告、SQL artifacts、Reviewer/LLM Reviewer 决策和 `close_when` 映射。准备型证据、未执行本地验证、pending review、LLM reviewer 非通过结论、缺少 `evidence-report.md` 或未证明关闭条件，都不能关闭任务。

生成失败后的修复计划：

```bash
python scripts/codex_loop/repair.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-workflow
```

`repair.py` 读取 `evidence-check/evidence-check.json`，把失败原因归类为 design、implement、verify、review、evidence 或 triage，并生成下一轮 Codex 修复提示。它不自动修改代码或任务状态。

为单个任务生成计划：

```bash
python scripts/codex_loop/plan.py \
  --task scripts/codex_loop/tasks/CLE-P0-ROUTE-001.yaml \
  --run-id 20260623-cle-p0-route-001 \
  --write
```

生成 diff-aware 第三视角审阅：

```bash
python scripts/codex_loop/review.py \
  --task scripts/codex_loop/tasks/CLE-P0-ROUTE-001.yaml \
  --run-id 20260623-cle-p0-route-001 \
  --guidance doc/02_acceptance/runs/20260623-cle-p0-route-001/guidance/guidance.json
```

`review.py` 会读取本轮 `implementation/patch-scope.json`、`patch-runner/patch-intake.json`、可选 `--patch`、`local-report.md` 和 guidance blocker，输出 `review-report.md`、`design-delta.md` 和 `review/review-summary.json`。

生成语义审阅：

```bash
python scripts/codex_loop/semantic_reviewer.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-workflow
```

`semantic_reviewer.py` 会读取任务、设计包、上下文包、review 和 evidence 文本，补充产品/技术语义层面的启发式检查。

生成 LLM reviewer 请求和 intake 证据。默认只生成请求、JSON Schema、模型画像和命令模板，不调用模型：

```bash
python scripts/codex_loop/llm_reviewer.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --run-id 20260623-workflow \
  --review-summary doc/02_acceptance/runs/20260623-workflow/review/review-summary.json \
  --semantic-review doc/02_acceptance/runs/20260623-workflow/semantic-review/semantic-review.json \
  --context-pack doc/02_acceptance/runs/20260623-workflow/context-pack/task-context-pack.md \
  --design-dir doc/02_acceptance/runs/20260623-workflow/design \
  --patch-request doc/02_acceptance/runs/20260623-workflow/patch-runner/patch-request.md \
  --patch-intake doc/02_acceptance/runs/20260623-workflow/patch-runner/patch-intake.json
```

如果已有外部模型输出，可加 `--llm-output <json>` 做 intake。`LLM_REVIEW_PASSED` 只是额外审阅证据，仍需 `review.py`、本地/真实链路验证和 `evidence_check.py` 共同通过；`LLM_REVIEW_REPAIR_REQUIRED`、`LLM_REVIEW_DESIGN_REQUIRED`、`LLM_REVIEW_HUMAN_GATE_REQUIRED` 和 `LLM_REVIEW_BLOCKED` 会被 release/evidence gate 识别为阻断。

生成任务队列、锁和重试计划：

```bash
python scripts/codex_loop/scheduler.py \
  --context-dir doc/02_acceptance/runs/20260623-scout/context \
  --guidance doc/02_acceptance/runs/20260623-guidance/guidance/guidance.json \
  --run-id 20260623-scheduler \
  --max-items 3 \
  --persist-queue \
  --queue-backend sqlite
```

需要保护工作区时可加 `--acquire-lock`。调度器只写 queue intent，不直接运行 `workflow.py`；`--persist-queue` 会把可执行项写入所选队列后端。

单独评估 per-lane resource quota：

```bash
python scripts/codex_loop/resource_quota.py \
  --guidance doc/02_acceptance/runs/20260623-guidance/guidance/guidance.json \
  --run-id 20260623-resource-quota \
  --max-items 3
```

该命令会按 `policies/resource-quotas.yaml` 计算 lane、mode、subsystem、live-generated 和总资源权重，写入 `resource-quota/resource-quota.json` 与 `resource-quota/resource-quota.md`。它只影响调度证据，不执行任务。

采集动态资源观测：

```bash
python scripts/codex_loop/resource_monitor.py \
  --run-id 20260623-resource-monitor \
  --queue-backend sqlite \
  --queue-path doc/02_acceptance/runs/.loop/queue.sqlite3
```

该命令会按 `policies/resource-observability.yaml` 采集 CPU busy、load per CPU、可用内存、磁盘、进程/线程和队列压力，写入 `resource-monitor/resource-monitor.json`、`resource-monitor/resource-monitor.md` 和 `.loop/resource-monitor-latest.json`。`RESOURCE_MONITOR_DEGRADED` 建议串行执行；`RESOURCE_MONITOR_BLOCKED` 会阻断 preflight、executor pool 和 release。

生成每任务 workspace isolation 计划：

```bash
python scripts/codex_loop/workspace_isolation.py \
  --scheduler-plan doc/02_acceptance/runs/20260623-scheduler/scheduler/scheduler-plan.json \
  --run-id 20260623-workspace-isolation \
  --max-items 2
```

该命令会按 `policies/workspace-isolation.yaml` 为 scheduler 选中项生成 workspace 隔离计划，写入 `workspace-isolation/isolation-plan.json` 和 `workspace-isolation/isolation-report.md`。默认 backend 是 `git-worktree`；当控制面 `.git` 只读或容器化运行不适合写源仓库 worktree metadata 时，可传入 `--workspace-backend local-clone`，只在 workspace root 下写入本地 clone。默认状态是计划级隔离；创建真实 workspace 需要额外传入 `--create-worktrees` 并设置 `CODEX_LOOP_ALLOW_WORKTREE_CREATE=1`。如果源工作区有未提交改动，报告会进入 `WORKSPACE_ISOLATION_DEGRADED`，提醒 workspace 基于 HEAD，不会自动携带这些改动。

按 scheduler 队列执行 worker：

```bash
python scripts/codex_loop/worker.py \
  --scheduler-plan doc/02_acceptance/runs/20260623-scheduler/scheduler/scheduler-plan.json \
  --run-id 20260623-worker \
  --stage prepare \
  --max-tasks 1 \
  --claim-queue \
  --queue-backend sqlite
```

worker 默认只执行 `workflow.py --stage prepare`，不会自动执行 live 写入或外部 Codex 命令。带 `--claim-queue` 时，worker 会先认领持久队列项，执行后写回 done、failed 或 quarantine。

把 scheduler 队列转为隔离执行计划：

```bash
python scripts/codex_loop/sandbox_worker.py \
  --scheduler-plan doc/02_acceptance/runs/20260623-scheduler/scheduler/scheduler-plan.json \
  --run-id 20260623-sandbox-worker \
  --stage prepare \
  --max-tasks 1 \
  --driver kubernetes-job \
  --validate
```

`sandbox_worker.py` 的默认模式只生成每个任务的 `sandbox-plan.json`，不认领队列、不执行容器、不写回 done/failed。真实隔离执行必须显式：

```bash
CODEX_LOOP_ALLOW_SANDBOX_EXECUTION=1 python scripts/codex_loop/sandbox_worker.py \
  --scheduler-plan doc/02_acceptance/runs/20260623-scheduler/scheduler/scheduler-plan.json \
  --run-id 20260623-sandbox-worker-exec \
  --stage prepare \
  --max-tasks 1 \
  --driver kubernetes-job \
  --execute-sandbox \
  --claim-queue \
  --queue-backend sqlite \
  --cleanup
```

用 bounded executor pool 并发生成隔离计划：

```bash
python scripts/codex_loop/executor_pool.py \
  --scheduler-plan doc/02_acceptance/runs/20260623-scheduler/scheduler/scheduler-plan.json \
  --run-id 20260623-executor-pool \
  --runner sandbox-plan \
  --max-workers 2 \
  --max-tasks 2 \
  --stage prepare
```

也可以直接从 SQLite 持久队列取任务：

```bash
python scripts/codex_loop/executor_pool.py \
  --run-id 20260623-executor-pool-queue \
  --runner sandbox-plan \
  --max-workers 2 \
  --max-tasks 2 \
  --queue-backend sqlite \
  --queue-path doc/02_acceptance/runs/.loop/queue.sqlite3
```

`executor_pool.py` 会先应用 per-lane resource quota 和 workspace isolation plan，再为每个任务生成带 `workspace_isolation` 信息的单任务 synthetic scheduler plan，并通过有界线程池调用 `sandbox_worker.py` 或 `worker.py`。默认 `sandbox-plan` 不认领队列、不执行容器；非 plan 型并发必须使用 SQLite 队列并显式开启 `--allow-parallel-execution`。默认模式只生成隔离计划；显式 `--create-worktrees --activate-workspaces` 时，child subprocess 会在对应 per-task workspace 中运行，证据仍写回主 runs 根。

显式创建并激活 per-task workspace：

```bash
CODEX_LOOP_ALLOW_WORKTREE_CREATE=1 python scripts/codex_loop/executor_pool.py \
  --scheduler-plan doc/02_acceptance/runs/20260623-scheduler/scheduler/scheduler-plan.json \
  --run-id 20260623-executor-pool-workspace \
  --runner sandbox-plan \
  --max-workers 2 \
  --max-tasks 1 \
  --stage prepare \
  --queue-backend sqlite \
  --queue-path doc/02_acceptance/runs/.loop/queue.sqlite3 \
  --create-worktrees \
  --activate-workspaces
```

`--activate-workspaces` 会把 child subprocess 的 `CODEX_LOOP_REPO_ROOT` 指向对应 workspace，并把 `CODEX_LOOP_RUNS_ROOT` 保持为主工作区 `doc/02_acceptance/runs`，因此代码/计划在隔离 workspace 上执行，证据仍进入主账本。若 workspace 未创建、路径越界或 spec 缺失，执行池会进入 `EXECUTOR_POOL_BLOCKED`。

清理 per-task workspace。默认只生成清理计划，不删除任何目录：

```bash
python scripts/codex_loop/workspace_cleanup.py \
  --workspace-isolation doc/02_acceptance/runs/20260623-executor-pool-workspace/executor-pool/workspace-isolation.json \
  --run-id 20260623-workspace-cleanup-plan
```

真实清理必须显式开启环境闸门：

```bash
CODEX_LOOP_ALLOW_WORKTREE_CLEANUP=1 python scripts/codex_loop/workspace_cleanup.py \
  --workspace-isolation doc/02_acceptance/runs/20260623-executor-pool-workspace/executor-pool/workspace-isolation.json \
  --run-id 20260623-workspace-cleanup \
  --execute
```

`workspace_cleanup.py` 只允许清理 `policies/workspace-isolation.yaml` 的 allowed roots 下的已知 workspace backend。`git-worktree` 走 `git worktree remove`，`local-clone` 只删除已确认是 git workspace 的隔离目录，并尝试清理空父目录；脏 workspace 默认阻断，只有人工确认后才能加 `--force`。

运行轻量 executor pool stress。该命令会重复调用 executor pool，并在每轮后执行 workspace cleanup，最后检查是否有新增 git worktree 或 workspace 目录泄漏：

```bash
CODEX_LOOP_ALLOW_WORKTREE_CREATE=1 CODEX_LOOP_ALLOW_WORKTREE_CLEANUP=1 \
  python scripts/codex_loop/executor_pool_stress.py \
    --scheduler-plan doc/02_acceptance/runs/20260623-scheduler/scheduler/scheduler-plan.json \
    --run-id 20260623-executor-pool-stress \
    --iterations 3 \
    --max-workers 2 \
    --max-tasks 1 \
    --queue-backend sqlite \
    --queue-path doc/02_acceptance/runs/.loop/queue.sqlite3 \
    --skip-resource-monitor \
    --workspace-backend local-clone \
    --create-worktrees \
    --activate-workspaces \
    --cleanup-worktrees
```

`executor_pool_stress.py` 只编排已有的 executor/cleanup 闸门，不自行绕过环境授权；如果 cleanup-enabled stress 后仍留下新 git worktree 或 workspace 目录，会进入 `EXECUTOR_POOL_STRESS_BLOCKED`。生产节点若可写源仓库 `.git`，可以使用默认 `git-worktree`；受限控制面或容器环境建议显式使用 `local-clone`。

队列后端有三种：

- `repo-json`：默认后端，写入 `doc/02_acceptance/runs/.loop/queue.json` 和 `queue-events.jsonl`，便于人工审计。
- `sqlite`：事务后端，写入 `doc/02_acceptance/runs/.loop/queue.sqlite3`，使用 SQLite/WAL 做原子 claim、完成、失败和过期恢复，适合 supervisor/CronJob 场景。
- `http`：远程后端，`--queue-path` 是 queue service base URL，例如 `http://codex-loop-queue-service.traffic-analysis.svc:8765`；worker、sandbox worker、scheduler 和 executor pool 通过该服务仲裁 enqueue、claim、complete、fail 和 recover，token 读取 `CODEX_LOOP_QUEUE_TOKEN`。

运行 HTTP 队列服务冒烟。该命令会启动 loopback 服务，用临时 SQLite 队列走 enqueue、status、claim、complete、recover 和 stop 全链路：

```bash
python scripts/codex_loop/queue_service.py smoke \
  --run-id 20260623-queue-service-smoke
```

前台运行 HTTP 队列服务：

```bash
CODEX_LOOP_QUEUE_TOKEN=replace-with-secret python scripts/codex_loop/queue_service.py serve \
  --host 127.0.0.1 \
  --port 8765 \
  --queue-backend sqlite \
  --queue-path doc/02_acceptance/runs/.loop/queue.sqlite3 \
  --auth-token-env CODEX_LOOP_QUEUE_TOKEN
```

服务端点包括 `/health`、`/v1/queue/status`、`/v1/queue/enqueue-plan`、`/v1/queue/claim`、`/v1/queue/complete`、`/v1/queue/fail`、`/v1/queue/recover` 和 `/v1/service/stop`。它只提供进程边界和远程 claim API，不改变 queue/retry/quarantine 语义。

运行远程队列执行池压测。默认命令会启动 embedded loopback HTTP queue service，用多个 remote-style worker 并发争抢同一批 synthetic queue task，验证无重复成功 claim、最终队列 drain、以及非 lease owner 不能 complete/fail：

```bash
python scripts/codex_loop/remote_pool_stress.py \
  --run-id 20260623-remote-pool-stress \
  --workers 4 \
  --tasks 8 \
  --rounds 3
```

也可以指向一个已经部署的 queue service。loopback URL 可直接压测；非 loopback URL 必须显式传入 `--allow-external-service` 或设置 `CODEX_LOOP_ALLOW_REMOTE_POOL_STRESS=1`，且必须提供 `CODEX_LOOP_QUEUE_TOKEN` 或 `--auth-token`。外部服务默认不会被停止，只有显式传入 `--stop-external-service` 才会调用 `/v1/service/stop`：

```bash
CODEX_LOOP_QUEUE_TOKEN=replace-with-secret python scripts/codex_loop/remote_pool_stress.py \
  --run-id 20260623-remote-pool-stress-service \
  --service-url http://codex-loop-queue-service.traffic-analysis.svc:8765 \
  --allow-external-service \
  --workers 4 \
  --tasks 8 \
  --rounds 3
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/remote-pool-stress/
  stress-summary.json
  stress-report.md
  worker-results.json
  http-responses.json
```

生成 K8s 多 Pod 远程队列压测计划。该命令渲染一个 `completionMode: Indexed` 的 Job，每个 Pod 作为独立 worker 通过 HTTP queue service 认领/完成 synthetic task；默认只 dry-run，不 apply：

```bash
python scripts/codex_loop/remote_pool_k8s_stress.py \
  --run-id 20260623-remote-pool-k8s-stress \
  --service-url http://codex-loop-queue-service.traffic-analysis.svc:8765 \
  --allow-external-service \
  --workers 3 \
  --tasks 6 \
  --rounds 2 \
  --validate
```

真实执行必须额外设置环境闸门，并确保 image、PVC 和 `codex-loop-queue-token` Secret 已存在：

```bash
CODEX_LOOP_ALLOW_K8S_REMOTE_POOL_STRESS=1 CODEX_LOOP_QUEUE_TOKEN=replace-with-secret \
  python scripts/codex_loop/remote_pool_k8s_stress.py \
    --run-id 20260623-remote-pool-k8s-execute \
    --service-url http://codex-loop-queue-service.traffic-analysis.svc:8765 \
    --allow-external-service \
    --workers 3 \
    --tasks 6 \
    --rounds 2 \
    --validate \
    --execute \
    --cleanup
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/remote-pool-k8s-stress/
  stress-summary.json
  stress-report.md
  seed-plan.json
  remote-pool-worker-job.yaml
  command-template.txt
  kubectl-dry-run.txt
```

审计 K8s 多 Pod 压测的真实集群 readiness。该命令不会 apply 资源；它会检查 namespace、queue service、workspace PVC、token Secret、worker Job dry-run 和执行闸门，并把 Secret 数据脱敏：

```bash
python scripts/codex_loop/remote_pool_k8s_readiness.py \
  --run-id 20260623-remote-pool-k8s-readiness \
  --stress-summary doc/02_acceptance/runs/20260623-remote-pool-k8s-stress/remote-pool-k8s-stress/stress-summary.json
```

该命令会写入：

```text
doc/02_acceptance/runs/<run_id>/remote-pool-k8s-readiness/
  readiness-summary.json
  readiness-report.md
  kubectl-checks.json
  kubectl-dry-run.txt
```

通过 HTTP queue backend 运行远程 worker / pool：

```bash
CODEX_LOOP_QUEUE_TOKEN=replace-with-secret python scripts/codex_loop/executor_pool.py \
  --run-id 20260623-executor-pool-http \
  --runner sandbox-plan \
  --max-workers 2 \
  --max-tasks 1 \
  --queue-backend http \
  --queue-path http://127.0.0.1:8765
```

K8s 远程 worker profile：

```bash
python scripts/codex_loop/deploy.py \
  --run-id 20260623-http-queue-worker-deploy \
  --profile http_queue_worker_k8s \
  --target kubernetes \
  --validate
```

`http_queue_worker_k8s` 会渲染 `service.py once --queue-backend http`，并从 `codex-loop-queue-token` Secret 注入 `CODEX_LOOP_QUEUE_TOKEN`。

运行 bounded daemon cycle：

```bash
python scripts/codex_loop/daemon.py \
  --run-id 20260623-daemon \
  --iterations 1 \
  --max-items 1 \
  --worker-stage prepare \
  --worker-runner workflow
```

daemon 每轮执行 `scout -> guide -> scheduler --acquire-lock -> worker -> metrics`，默认 release workspace lock。`--worker-runner workflow` 保持本机 workflow worker；`--worker-runner sandbox-plan` 只生成隔离计划；`--worker-runner sandbox-execute` 会调用 `sandbox_worker.py --execute-sandbox --claim-queue`，但真实执行仍受 `CODEX_LOOP_ALLOW_SANDBOX_EXECUTION` 闸门约束。daemon 是有界循环，不是默认常驻服务。

需要让 daemon 每轮后检查项目级目标停止条件时，增加：

```bash
python scripts/codex_loop/daemon.py \
  --run-id 20260623-daemon-stop-aware \
  --iterations 3 \
  --max-items 1 \
  --objective "完成 traffic-analysis-platform 项目开发" \
  --objective-stop-release-run-id 20260623-loop-release \
  --check-objective-stop \
  --stop-on-objective
```

`--stop-on-objective` 会在 `OBJECTIVE_STOP_READY`、`OBJECTIVE_STOP_BLOCKED` 或 `OBJECTIVE_STOP_HUMAN_GATE` 时提前结束 bounded daemon；其中只有 READY 表示成功完成。daemon 会把 `--objective`、guidance、context 和可选 release 证据传给 `objective_stop.py`；显式传入的 context 或 release 证据路径缺失、解析失败时，objective stop 会进入 BLOCKED，避免把证据丢失误判为可继续。

前台执行一次服务化 cycle，并生成 service 证据：

```bash
python scripts/codex_loop/service.py once \
  --run-id 20260623-service-once \
  --max-items 1 \
  --worker-stage prepare \
  --worker-runner workflow
```

服务化入口也可传入项目级目标停止条件：

```bash
python scripts/codex_loop/service.py once \
  --run-id 20260623-service-objective \
  --max-items 1 \
  --objective "完成 traffic-analysis-platform 项目开发" \
  --objective-stop-release-run-id 20260623-loop-release \
  --check-objective-stop \
  --stop-on-objective
```

`service.py once/run/start` 会把目标、release 证据和停止策略传给 daemon，并在 service report 中记录 child daemon 的 objective stop 状态。`OBJECTIVE_STOP_CONTINUE` 不会停止 service 循环；`OBJECTIVE_STOP_READY` 会形成 `SERVICE_OBJECTIVE_READY`，`OBJECTIVE_STOP_BLOCKED/HUMAN_GATE` 会形成 `SERVICE_OBJECTIVE_STOPPED`。

查看服务状态和健康：

```bash
python scripts/codex_loop/service.py status
python scripts/codex_loop/service.py health \
  --run-id 20260623-service-health \
  --profile sqlite_conservative \
  --queue-backend sqlite
```

恢复过期 claim 或过期 workspace lock：

```bash
python scripts/codex_loop/service.py recover --run-id 20260623-service-recover
```

后台启动与停止可用：

```bash
python scripts/codex_loop/service.py start --run-id 20260623-service --interval-seconds 300
python scripts/codex_loop/service.py stop
```

`once/run/start` 会默认先执行 runtime preflight，并将 `preflight/preflight.json` 与 `preflight/preflight.md` 写入同一 run 目录；只有 `RUNTIME_PREFLIGHT_BLOCKED` 会阻断服务执行。`start` 只是 repo-native supervisor，内部仍然重复 bounded daemon cycle；默认 worker runner 是 `workflow`、stage 是 `prepare`，不自动 live 写入、隔离执行或调用外部 Codex。

运行 bounded soak。该命令会重复执行服务化 cycle，并在每轮采集 resource monitor、service health 和 metrics：

```bash
python scripts/codex_loop/soak.py \
  --run-id 20260623-soak \
  --cycles 3 \
  --interval-seconds 10 \
  --max-items 1 \
  --worker-stage prepare \
  --worker-runner sandbox-plan \
  --queue-backend sqlite \
  --queue-path doc/02_acceptance/runs/.loop/queue.sqlite3
```

短验证可把 `--cycles` 调小。`soak.py` 会写入 `soak/soak-summary.json` 和 `soak/soak-report.md`；任一 runner、health、metrics 或 resource monitor blocker 超过 `--max-failures` 会进入 `SOAK_BLOCKED`。`SOAK_DEGRADED` 表示存在资源或健康 warning，仍需要在 release 中可见。

生成 systemd/K8s 部署计划。默认只渲染清单，不安装也不 apply：

```bash
python scripts/codex_loop/deploy.py \
  --run-id 20260623-deploy \
  --target all \
  --profile sqlite_conservative \
  --validate
```

该命令会根据 `policies/resource-profiles.yaml` 生成 `deploy/codex-loop.service`、`deploy/codex-loop-pvc.yaml`、K8s workload、`deploy/kustomization.yaml` 和 `deploy/deploy-report.md`。推荐生产 profile 为 `sqlite_conservative`：单 worker、SQLite 事务队列、`prepare` stage、禁止 live write、禁止外部 Codex。需要受限并发计划池时使用 `sqlite_pool_plan`：`executor_pool.py`、`sandbox-plan`、2 workers、SQLite 队列、禁止 live write、禁止外部 Codex。需要外部队列进程边界时使用 `queue_service_sqlite`：`queue_service.py serve`、SQLite/WAL、K8s Deployment/Service、token env。需要跨 Pod/跨机器经服务仲裁队列时使用 `http_queue_worker_k8s`：HTTP queue backend、token Secret、单 worker `prepare`。带 `--validate` 时会额外写入 `deploy/validation.json`、`deploy/kubectl-dry-run.txt` 和 `deploy/systemd-verify.txt`。

K8s 清单默认把 loop-control 镜像代码放在 `/app`，把队列、运行证据和状态放在 PVC 挂载的 `/workspace`。因此 `queue_service_sqlite` 在 K8s 中会把 SQLite 队列写到 `/workspace/doc/02_acceptance/runs/.loop/queue.sqlite3`，不会用空 PVC 覆盖 `/app/scripts/codex_loop`。如需调整可传入 `--app-root` 或 `--state-root`。部署证据会记录 `image_layout`：`queue_service_sqlite` 默认为 `control-only`，代码执行型 K8s profile 默认为 `full-repo`；如果显式把 `control-only` 用在 service/executor 工作流，部署计划会阻断。

构建 loop-control 镜像时使用 `scripts/codex_loop` 作为 Docker context，避免把整个 27G 仓库发送进镜像构建：

```bash
python scripts/codex_loop/image_build.py \
  --run-id 20260623-image-build \
  --execute
```

本环境使用 containerd 节点本地镜像时，需要把该镜像导入每个可能调度的节点后再 apply queue service Deployment 或 synthetic remote-pool Job。真正执行项目代码修改的 K8s profile 需要完整项目镜像或显式挂载受控 workspace；不要把生产 Secret 或 token 写入镜像、文档或日志。

验证或显式应用 K8s queue service 前置资源：

```bash
python scripts/codex_loop/k8s_bootstrap.py \
  --run-id 20260623-k8s-bootstrap \
  --deploy-dir doc/02_acceptance/runs/20260623-queue-service-deploy/deploy
```

默认只执行 dry-run，并写入：

```text
doc/02_acceptance/runs/<run_id>/k8s-bootstrap/
  bootstrap-summary.json
  bootstrap-report.md
  command-template.txt
  kubectl-dry-run.txt
  secret-dry-run.txt
```

真实 apply 必须显式设置：

```bash
CODEX_LOOP_ALLOW_K8S_BOOTSTRAP=1 CODEX_LOOP_QUEUE_TOKEN=replace-with-secret \
  python scripts/codex_loop/k8s_bootstrap.py \
    --run-id 20260623-k8s-bootstrap-apply \
    --deploy-dir doc/02_acceptance/runs/20260623-queue-service-deploy/deploy \
    --execute
```

冻结发布证据和回滚计划：

```bash
python scripts/codex_loop/release.py \
  --run-id 20260623-loop-release \
  --deploy-plan doc/02_acceptance/runs/20260623-deploy/deploy/deploy-plan.json \
  --k8s-bootstrap doc/02_acceptance/runs/20260623-k8s-bootstrap/k8s-bootstrap/bootstrap-summary.json \
  --sandbox-plan doc/02_acceptance/runs/20260623-sandbox-screen/sandbox/sandbox-plan.json \
  --sandbox-execution doc/02_acceptance/runs/20260623-sandbox-exec-screen/sandbox-executor/execution.json \
  --sandbox-worker doc/02_acceptance/runs/20260623-sandbox-worker/sandbox-worker/sandbox-worker-summary.json \
  --resource-quota doc/02_acceptance/runs/20260623-resource-quota/resource-quota/resource-quota.json \
  --resource-monitor doc/02_acceptance/runs/20260623-resource-monitor/resource-monitor/resource-monitor.json \
  --workspace-isolation doc/02_acceptance/runs/20260623-workspace-isolation/workspace-isolation/isolation-plan.json \
  --workspace-cleanup doc/02_acceptance/runs/20260623-workspace-cleanup/workspace-cleanup/cleanup-plan.json \
  --executor-pool doc/02_acceptance/runs/20260623-executor-pool/executor-pool/executor-pool-summary.json \
  --executor-pool-stress doc/02_acceptance/runs/20260623-executor-pool-stress/executor-pool-stress/stress-summary.json \
  --remote-pool-stress doc/02_acceptance/runs/20260623-remote-pool-stress/remote-pool-stress/stress-summary.json \
  --remote-pool-k8s-stress doc/02_acceptance/runs/20260623-remote-pool-k8s-stress/remote-pool-k8s-stress/stress-summary.json \
  --remote-pool-k8s-readiness doc/02_acceptance/runs/20260623-remote-pool-k8s-readiness/remote-pool-k8s-readiness/readiness-summary.json \
  --soak doc/02_acceptance/runs/20260623-soak/soak/soak-summary.json \
  --model-profile doc/02_acceptance/runs/20260623-workflow/model-profile/model-profile.json \
  --llm-review doc/02_acceptance/runs/20260623-workflow/llm-review/llm-review-summary.json \
  --queue-service doc/02_acceptance/runs/20260623-queue-service-smoke/queue-service/queue-service-summary.json \
  --objective-stop doc/02_acceptance/runs/20260623-objective-stop/objective-stop/stop-summary.json \
  --queue-backend sqlite
```

该命令会写入 `release/release-manifest.json`、`release/release-manifest.md`、`release/rollback-plan.md`、`release/git-status.txt` 和 `release/loop-diff.patch`。如果传入 `--deploy-plan`，release 只接受 `DEPLOY_PLAN_READY`，并记录 `image_layout`、`app_root`、`state_root` 和 `k8s_queue_path`。如果传入 `--k8s-bootstrap`，release 接受 `K8S_BOOTSTRAP_VALIDATED` 或 `K8S_BOOTSTRAP_APPLIED`，拒绝 bootstrap blocker。如果传入 `--workspace-isolation`，release 会接受 `WORKSPACE_ISOLATION_PLANNED`、`WORKSPACE_ISOLATION_READY` 或 `WORKSPACE_ISOLATION_DEGRADED`，拒绝 `WORKSPACE_ISOLATION_BLOCKED`。如果传入 `--workspace-cleanup`，release 会接受 `WORKSPACE_CLEANUP_PLANNED`、`WORKSPACE_CLEANUP_COMPLETED` 或 `WORKSPACE_CLEANUP_EMPTY`，拒绝 `WORKSPACE_CLEANUP_BLOCKED`。如果传入 `--executor-pool-stress`，release 只接受 `EXECUTOR_POOL_STRESS_COMPLETED` 或 `EXECUTOR_POOL_STRESS_EMPTY`，拒绝 stress blocker。如果传入 `--remote-pool-stress`，release 只接受 `REMOTE_POOL_STRESS_COMPLETED` 或 `REMOTE_POOL_STRESS_EMPTY`，拒绝 remote stress blocker。如果传入 `--remote-pool-k8s-stress`，release 接受 `REMOTE_POOL_K8S_STRESS_PLANNED`、`REMOTE_POOL_K8S_STRESS_VALIDATED` 或 `REMOTE_POOL_K8S_STRESS_COMPLETED`，拒绝 K8s stress blocker。如果传入 `--remote-pool-k8s-readiness`，release 接受 `REMOTE_POOL_K8S_READINESS_READY` 或 `REMOTE_POOL_K8S_READINESS_DEGRADED`，拒绝 readiness blocker。如果传入 `--objective-stop`，release 只接受 `OBJECTIVE_STOP_READY`，拒绝 continue、blocked 或 human gate 状态。如果传入 `--soak`，release 接受 `SOAK_COMPLETED`、`SOAK_DEGRADED` 或 `SOAK_EMPTY`，拒绝 `SOAK_BLOCKED`。如果传入 `--model-profile`，release 只接受 `MODEL_PROFILE_SELECTED` 且 findings 中没有 blocker。如果传入 `--llm-review`，release 接受 `LLM_REVIEW_PLANNED` 或 `LLM_REVIEW_PASSED`，拒绝 blocked 或实际非通过结论。它不会执行回滚。

查看持久队列：

```bash
python scripts/codex_loop/queue_store.py status
```

生成 loop 运行指标：

```bash
python scripts/codex_loop/metrics.py --run-id 20260623-metrics
```

该命令会汇总 `doc/02_acceptance/runs/*/run-summary.json`、持久队列和 workspace lock，写入 `metrics/loop-metrics.json`、`metrics/loop-metrics.md`，并更新 `doc/02_acceptance/runs/.loop/metrics-latest.json`。

查看或管理 workspace 租约锁：

```bash
python scripts/codex_loop/lock_manager.py status
```

从 repair plan 生成下一轮自动修复计划：

```bash
python scripts/codex_loop/auto_repair_loop.py \
  --task scripts/codex_loop/tasks/CLE-P0-SCREEN-001.yaml \
  --repair-plan doc/02_acceptance/runs/20260623-workflow/repair/repair-plan.json \
  --context-dir doc/02_acceptance/runs/20260623-workflow/context \
  --guidance doc/02_acceptance/runs/20260623-workflow/guidance/guidance.json \
  --run-id 20260623-auto-repair
```

默认只规划下一轮 stage；只有显式 `--execute` 才会调用 `workflow.py`。

收集轻量证据：

```bash
python scripts/codex_loop/collect_evidence.py \
  --task scripts/codex_loop/tasks/CLE-P0-ROUTE-001.yaml \
  --run-id 20260623-cle-p0-route-001
```

受控执行任务。默认只写计划和 `local-report.md` 占位：

```bash
python scripts/codex_loop/run_task.py \
  --task scripts/codex_loop/tasks/CLE-P0-ROUTE-001.yaml \
  --mode local
```

显式执行本地验证：

```bash
python scripts/codex_loop/run_task.py \
  --task scripts/codex_loop/tasks/CLE-P0-ROUTE-001.yaml \
  --mode local \
  --execute-local
```

## 目录说明

```text
scripts/codex_loop/
  discover.py              # 从内置首批任务目录生成 YAML 和架构地图
  scout.py                 # 生成上下文快照、缺口索引、依赖影响图和证据账本
  guide.py                 # 基于上帝视角账本生成纠偏、排序和状态建议
  daemon.py                # 运行有界 scout/guide/scheduler/worker 循环
  objective_stop.py        # 评估项目级目标停止条件，给出 READY/CONTINUE/BLOCKED/HUMAN_GATE
  service.py               # 服务化 supervisor，提供 once/start/stop/status/health/recover
  soak.py                  # 重复运行 bounded cycle、resource monitor、health 和 metrics，生成长稳证据
  deploy.py                # 渲染 systemd/K8s 部署计划，不自动安装或 apply
  Dockerfile               # loop-control 镜像，只包含引擎脚本，不打包完整项目仓库
  image_build.py           # 计划或执行 loop-control 镜像构建，并写入可供 release/objective-stop 消费的证据
  k8s_bootstrap.py         # 验证或显式应用 queue service 的 PVC/Secret/Deployment/Service 前置资源
  release.py               # 冻结发布证据、健康、队列、diff 和回滚计划
  preflight.py             # 检查生产入口 runtime readiness，并生成可审计 preflight 证据
  resource_quota.py        # 评估 lane/mode/subsystem/live-generated/total-weight 调度配额
  resource_monitor.py      # 采集动态资源压力并给出 admission/worker 建议
  workspace_isolation.py   # 为 bounded executor pool 生成每任务 workspace 隔离计划
  workspace_cleanup.py     # 为 per-task workspace 生成受控清理计划，可在环境闸门下执行清理
  sandbox.py               # 渲染 K8s Job/NetworkPolicy 和本地容器命令，不执行也不 apply
  sandbox_executor.py      # 在显式环境闸门下执行 sandbox plan，并收集日志/状态/cleanup 证据
  sandbox_worker.py        # 将 scheduler queue 桥接到 sandbox plan/execution，并按门禁回写队列
  executor_pool.py         # 从 scheduler plan 或持久队列启动有界执行池，默认并发生成 sandbox plan
  executor_pool_stress.py  # 重复运行 executor pool + cleanup，检查并发执行和 workspace 泄漏
  remote_pool_stress.py    # 通过 HTTP queue service 压测远程 worker 仲裁和 lease owner 回写
  remote_pool_k8s_stress.py # 渲染或执行 K8s Indexed Job 多 Pod 远程队列压测
  remote_pool_k8s_readiness.py # 只读审计 K8s 多 Pod 压测的 Service/PVC/Secret/Job readiness
  lock_manager.py          # 管理 workspace 租约锁、heartbeat 和 release
  queue_store.py           # 管理持久队列、claim、retry、done 和 quarantine
  queue_sqlite.py          # 管理 SQLite/WAL 事务队列
  queue_http.py            # 通过 HTTP queue service 访问远程队列仲裁入口
  queue_backend.py         # 在 repo-json、sqlite 与 http 队列后端之间分发
  queue_service.py         # 提供 HTTP 队列服务边界和 smoke 验证
  metrics.py               # 汇总 runs、持久队列和 workspace lock 运行指标
  design.py                # 基于任务、上帝视角和纠偏结果生成产品/功能/视觉/架构设计包
  context_pack.py          # 将长上下文压成任务级工作包、预算报告、决策日志和接力 handoff
  workflow.py              # 将上帝视角推荐任务编排到设计、上下文包、计划、审阅和受控执行
  worker.py                # 按 scheduler 队列执行受控 workflow，默认 prepare
  implement.py             # 生成实现简报，校验 patch 是否符合任务范围、契约声明和 blocker 门禁
  patch_runner.py          # 生成 Codex patch work order，校验结构化 Codex 输出和 unified diff
  model_profile.py         # 根据任务风险选择外部 Codex 模型画像、schema 和命令模板
  codex_adapter.py         # 生成或显式执行外部 Codex 调用计划，保留兼容用途
  codex_runner.py          # 生产推荐的外部 Codex 执行闸门，默认 plan-only，带策略、脱敏和审计
  evidence_check.py        # 判定 run 证据是否足够关闭任务，保守拒绝准备型或缺失证据
  repair.py                # 将证据失败转成下一轮 Codex 修复计划和修复提示
  auto_repair_loop.py      # 从 repair plan 选择下一轮 workflow stage，默认只规划
  scheduler.py             # 基于 guidance/evidence 生成任务队列、工作区锁和重试计划
  task_state.py            # 根据 guidance、workflow run 和证据账本生成任务状态看板与迁移计划
  plan.py                  # 为单任务生成 Gate 化执行计划
  run_task.py              # 受控任务执行入口，默认不执行命令
  review.py                # 基于 diff、patch scope、guidance 和 local report 生成第三视角审阅
  semantic_reviewer.py     # 基于任务/设计/证据文本做语义层启发式审阅
  llm_reviewer.py          # 生成外部 LLM reviewer 请求/schema/profile，并 intake 模型审阅输出
  collect_evidence.py      # 收集 git 状态和 run-summary
  update_status.py         # 更新 run-summary 状态
  policies/                # 默认策略、模型/LLM 审阅策略、资源策略、workspace isolation 和 live 写入拒绝清单
  templates/               # 任务、报告和证据模板
  tasks/                   # discover.py 生成的任务 YAML
  runs/                    # 临时运行态占位，长期证据进入 doc/02_acceptance/runs
```

## MVP 关闭标准

- 至少能生成 5 个 P0 任务。
- 每个任务包含 lane、source、subsystems、acceptance_type、verification 和 close_when。
- 能生成 `plan.md`、`review-report.md`、`design-delta.md`、`run-summary.json`。
- 能生成 `context.snapshot.json`、`gap-index.json`、`dependency-map.json`、`evidence-ledger.json`。
- 能生成 `guidance.json`、`guidance-report.md`，列出 blocker、warning、推荐下一任务和状态建议。
- 能生成 `design-summary.json` 和产品迭代、功能设计、视觉纠偏、架构演进、验收用例、实施计划文档。
- 能生成 `task-context-pack.md/json`、`context-budget.json`、`decision-log.jsonl` 和 `handoff.md`，用于长上下文任务恢复。
- 能通过 `workflow.py --stage prepare` 自动选择推荐任务并生成 workflow-summary/report。
- 能通过 `implement.py` 生成实现简报并校验 patch 范围和契约门禁。
- 能通过 `patch_runner.py` 生成 Codex patch work order、结构化输出契约和 JSON Schema，并校验 Codex 结构化输出与 unified diff。
- 能通过 `model_profile.py` 选择外部 Codex 模型画像、sandbox、timeout、schema 和命令模板，并让 `codex_runner.py` 复用该画像。
- 能通过 `codex_adapter.py` 生成外部 Codex 调用计划，并在显式授权时执行外部命令。
- 能通过 `codex_runner.py` 生成安全外部 Codex 执行计划；真实执行必须经过策略文件、命令白名单、环境变量白名单、脱敏审计和环境闸门。
- 能通过 `review.py` 生成 diff-aware review-summary、review-report 和 design-delta。
- 能通过 `semantic_reviewer.py` 生成语义审阅报告。
- 能通过 `llm_reviewer.py` 生成 LLM reviewer 请求、JSON Schema、模型画像和命令模板，并能 intake 外部模型审阅输出。
- 能通过 `evidence_check.py` 判定任务证据是否满足 evidence.required、Reviewer、local report、live/browser report、SQL artifacts、显式前置依赖、close_when 和证据层级。
- 能通过 `repair.py` 将 evidence blocker 转成下一轮修复计划。
- 能通过 `resource_quota.py` 生成 per-lane resource quota 证据。
- 能通过 `resource_monitor.py` 生成动态资源观测证据，并让 preflight/release/executor pool 使用该 admission 结果。
- 能通过 `workspace_isolation.py` 生成每任务 workspace 隔离计划，并让 executor pool/release/scout 记录 backend 和证据。
- 能通过 `workspace_cleanup.py` 生成 workspace 清理证据，并在显式环境闸门下移除已注册 worktree 或本地 clone。
- 能通过 `executor_pool_stress.py` 重复运行隔离执行池并检查 cleanup 后无新增 workspace 泄漏。
- 能通过 `remote_pool_stress.py` 验证多个 remote-style worker 经 HTTP queue service 并发 claim 时无重复成功认领、错误 worker 不能 complete/fail、最终队列 drain。
- 能通过 `remote_pool_k8s_stress.py` 渲染 K8s Indexed Job 多 Pod 压测计划，并用 dry-run 或显式执行证据冻结跨 Pod worker 仲裁路径。
- 能通过 `remote_pool_k8s_readiness.py` 审计 K8s namespace、Service、PVC、Secret、worker Job dry-run 和执行闸门，并让 release/scout 识别该 readiness 证据。
- 能通过 `objective_stop.py` 评估项目级停止条件，区分成功停止、继续执行、修复停止和人工裁决停止，并让 release/scout/daemon 识别该证据。
- 能通过 `deploy.py` 记录 `image_layout`、`app_root`、`state_root` 和 `k8s_queue_path`，并阻断把 `control-only` 镜像用于 K8s service/executor 工作流。
- 能通过 `scripts/codex_loop/Dockerfile` 构建 queue service / synthetic remote-pool worker 所需的 loop-control 镜像，避免默认打包完整项目仓库。
- 能通过 `image_build.py` 把 Docker build 成功或失败记录成 `IMAGE_BUILD_COMPLETED/BLOCKED`，并让 release/objective-stop 识别镜像前置状态。
- 能通过 `scheduler.py` 生成队列、锁、retry plan 和 quota usage/deferred 证据。
- 能通过 `queue_store.py` 持久化队列、认领任务、记录 retry 和 quarantine。
- 能通过 `queue_store.py` / `queue_sqlite.py` 持久化 lane、mode、subsystem 和 resource weight 元数据。
- 能通过 `queue_sqlite.py` 使用 SQLite/WAL 事务队列完成原子 claim、done、failed 和过期 claim 恢复。
- 能通过 `queue_service.py smoke` 验证 HTTP 队列服务以及 `queue_backend=http` 的 status、enqueue、claim、complete、recover 和 stop 链路，并在 release 中冻结服务证据。
- 能通过 `http_queue_worker_k8s` profile 渲染远程 worker，经 HTTP queue service 仲裁任务认领。
- 能通过 `worker.py` 按队列执行受控 workflow。
- 能通过 `daemon.py` 运行有界自动循环，默认 prepare，并可用 `--check-objective-stop --stop-on-objective` 在目标完成、阻断或人工门禁时停止。
- 能通过 `service.py` 前台执行一次服务 cycle、后台启动/停止、状态查看、健康检查和恢复过期运行态。
- 能通过 `soak.py` 重复运行 bounded service/daemon cycle，并冻结 resource monitor、health 和 metrics 长稳证据。
- 能通过 `preflight.py` 检查 Python/工具链/关键路径/磁盘/内存/队列路径/profile 安全项/workspace lock，并让 service/release 记录该状态。
- 能通过 `sandbox.py` 生成隔离执行计划、K8s Job、deny-all NetworkPolicy、本地容器命令和 dry-run 证据。
- 能通过 `sandbox_executor.py` 审计或显式执行 sandbox plan，并让 release 对执行完成态做冻结检查。
- 能通过 `sandbox_worker.py` 把 scheduler queue 转成隔离计划，并在显式执行门禁下完成 executor 调用和队列回写。
- 能通过 `executor_pool.py` 从 scheduler plan 或 SQLite 队列运行 bounded executor pool，默认并发生成 sandbox plan，并在 release 中冻结 pool 证据。
- 能通过 `deploy.py` 生成 systemd/K8s 部署计划和资源隔离检查。
- 能通过 `release.py` 校验 `DEPLOY_PLAN_READY`，并冻结部署镜像类型、K8s 代码目录、状态目录和队列路径。
- 能通过 `release.py` 校验 `IMAGE_BUILD_COMPLETED`，避免未构建镜像时误冻结 K8s 发布证据。
- 能通过 `k8s_bootstrap.py` 验证 queue service K8s PVC/Deployment/Service 和 token Secret 创建命令，并在显式闸门下应用前置资源。
- 能通过 `sqlite_pool_plan` profile 渲染 executor pool 型 systemd/K8s 计划，限制为 `sandbox-plan`、SQLite 队列和安全默认值。
- 能通过 `queue_service_sqlite` profile 渲染 HTTP queue service 的 systemd 和 K8s Deployment/Service 计划。
- 能通过 `release.py` 生成发布冻结证据和回滚计划。
- 能通过 `lock_manager.py` 管理 workspace lock 租约、heartbeat 和 release。
- 能通过 `metrics.py` 汇总 run、队列和锁的运行指标。
- 能通过 `auto_repair_loop.py` 从 repair plan 生成下一轮修复 stage。
- 能通过 `task_state.py` 生成任务状态看板、迁移计划和可选 `--apply` 状态写回。
- 不执行任何 live 写入。
- 策略失败时产出 `gate-request.md`，而不是继续执行。
