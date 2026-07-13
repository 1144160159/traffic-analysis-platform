# 功能回归与前端契约证据

本目录承载功能回归层证据，重点证明 UI 设计契约、菜单路由、权限门禁和真实入口没有漂移。这里的证据不能替代 10 x 100Gbps/512Mpps 性能验收、检测质量第三方盲测、生产安全或 HA 破坏性演练。

## 当前稳定证据

- `ui-contract-preflight-latest.json` / `ui-contract-preflight-latest.md`：UI 契约回归预检结果。
- 当前 UI 契约最新 run 为 `20260701-ui-contract-preflight-r17-desktop-login-pass-business-redirect-current`：20/21 checks passed，repo/UI/API 非浏览器契约 19/19 无 blocker，Codex Desktop Chrome extension backend 已能打开生产 `/login`；当时唯一 browser blocker 是受保护业务页 `/alerts` 直开后回落 `/login`。2026-07-02 已通过受控 smoke-token acceptance window 补到 `/alerts` interaction 证据，但还未重写 `ui-contract-preflight` 的历史 r17 结论。
- `ui-contract-matrix-latest.json`：设计菜单、routeManifest、UI suite route-page-map、业务流和代码差距的结构化对账。
- `ui-visual-interaction-preflight-latest.json` / `ui-visual-interaction-preflight-latest.md`：真实 React 页面 1:1 复刻双门禁，要求逐页面 1920x1080 截图 diff、receiver `capture-meta.json` 原始尺寸证明和业务交互证据；这不是图片验收，也不能由 `ui-contract-preflight` 替代。
- 当前 UI 视觉/交互双门禁 run 为 `20260702-ui-visual-interaction-preflight-r33-capture-session-bridge-r4`，结果 `blocked`：目标路由 28 个，视觉目标 30 个（`/topics` 按加密隧道、数据外传、APT 战役三张页内状态图计算），React page component 28/28 存在，目标 source image 30/30 存在且均为 1920x1080，前端源码直接嵌入 `doc/04_assets/.../screens/pages` 设计图的 blocker 为 0；Desktop Chrome extension backend 已真实捕获 `/login` 页面截图、登录表单交互证据以及 `/screen`、`/dashboard`、`/alerts` 已认证业务交互证据。当前逐目标视觉 diff 仍为 0/30，其中已有实际截图仍无法满足浏览器原生 1920x1080 capture-meta，正式视觉证据必须包含 receiver 生成的 `capture-meta.json` 来证明上传原图就是 1920x1080 且未后处理缩放；逐路由业务交互证据推进到 4/28，剩余 24/28 仍缺真实业务交互。r33 已显式校验 `capture-session-latest.json` 覆盖当前 30 个 visual gap 和 24 个 interaction gap。因此当前不能宣称前端已按 UI 1:1 完整复刻，只能宣称结构契约、页面实现入口、部分 Desktop 采集样本和完整采集执行队列已建立。
- `ui-visual-interaction/capture-plan-latest.json` / `.md`：从 `visual-acceptance.json` 生成的 Desktop Chrome 采集工作队列，解析 30 个视觉目标、28 个交互路由、动态详情页 URL、receiver 上传地址、缺失 evidence、`capture-meta.json` 要求和每个目标的 metrics 命令。该计划只用于驱动真实采集，不作为通过证据。
- `ui-visual-interaction/capture-session-latest.json` / `.md`：从 capture plan 和 gap report 生成的 Desktop Chrome 执行会话包，绑定 receiver、nonce-only redirect helper、待采集 visual batch、待采集 interaction batch、safe wrapper call、metrics 命令和正式复跑命令；当前状态为 `blocked_desktop_transport_closed`，只表示本会话 Desktop bridge 不可用，不替代真实证据。
- `ui-visual-interaction/desktop-chrome-viewport-probe-latest.json` / `.md`：Desktop Chrome extension bridge 能 claim `/login`、evaluate、DOM snapshot 和真实截图，但当前截图为 `2559x1271`，`window.resizeTo` 不存在，桥接对象未暴露 viewport/window/bounds/emulation 控制方法。因此正式视觉 diff 仍必须等待真实 `1920x1080` Desktop 截图和 receiver `capture-meta.json`，不能接受裁剪、缩放或改名证据。
- `ui-visual-interaction/desktop-chrome-auth-storage-probe-latest.json` / `.md`：2026-07-02 复测证明短期 JWT 可被 `/api/v1/auth/me` 接受，但 direct localStorage 注入不可用。随后通过短时打开 `DESKTOP_SMOKE_TOKEN_ENABLED=true` 的受控 hash smoke acceptance window 采集到 `/alerts` interaction pass；为持续采集双门禁证据，当前 repo 与 live runtime config 均保持该开关为 `true`。
- `desktop-chrome-business-smoke-latest.json` / `desktop-chrome-business-smoke-latest.md`：历史 Codex Desktop Chrome extension backend 合法登录态业务页点击证据，最终页为 `/alerts`；当前 1:1 视觉/交互双门禁以 `ui-visual-interaction-preflight-latest.json` 和 auth-storage probe 为准，不以历史 smoke 替代。
- `business-flow-api-preflight-latest.json` / `business-flow-api-preflight-latest.md`：业务流 API 契约经 APISIX 的真实接口回归结果。
- `threat-intel-service-latest.json` / `threat-intel-service-latest.md`：Threat Intel 独立服务、`threat.intel.v1` Topic、K8s/APISIX 路由、JWT/RBAC read-write gates、PostgreSQL-backed lookup/upsert/import/list/enrich、scheduled feed import、同步 `audit_logs` 和 `threat.intel.v1` publish/consume 的 live preflight 结果。
- `fusion-threat-intel-latest.json` / `fusion-threat-intel-latest.md`：Fusion 页面契约消费 Threat Intel 服务，并完成冲突处理/规则编辑写入 PostgreSQL 与 `audit_logs` 的 UI/API/Web rollout 联动回归结果。
- `../runs/20260630-fusion-value-report-local/local-report.md`：Fusion 价值量化本地联动证据，证明 `/v1/fusion/value-report` 已进入后端只读接口、`/fusion` API plan、snapshot adapter、UI suite 契约和本地 Go/Vitest 回归；该证据不等同于 live APISIX/K8s 或第三方试点评估通过。
- `fusion-value-report-preflight-latest.json` / `fusion-value-report-preflight-latest.md`：Fusion 价值量化 live preflight 稳定副本，验证 repo 契约、本地 Go/Vitest/UI suite 门禁、K8s JWT Secret、真实 APISIX `/api/v1/fusion/stats|entities|value-report` 和 value-report 响应结构；当前 `20260630-fusion-value-report-preflight-r2-live-rollout` 为 `pass`：19/19 passed、0 blockers，已证明 live APISIX/JWT/K8s 链路暴露 value-report 结构。冻结样本窗口、真实单源/多源消融结论和试点签认仍需独立补齐。
- `asset-discovery-latest.json` / `asset-discovery-latest.md`：SNMP/LLDP 主动资产发现控制面、`asset:discover` scope catalog、viewer 写入 403、SecretRef 凭据登记、发现任务、scanner worker failed-run 安全落库、同步 `audit_logs`、资产 upsert 和 LLDP 拓扑边写入的真实 APISIX 回归结果。
- `asset-discovery-coverage-latest.json` / `asset-discovery-coverage-latest.md`：SNMP/LLDP 资产发现覆盖率报告门，真实读取 APISIX 与 PostgreSQL 的 `assets`、`asset_discovery_runs`、`asset_topology_links`，并在提供 `SITE_ASSET_INVENTORY_JSON` 后按 MAC/IP/hostname 计算现场期望资产发现率；当前 r3 为 `blocked`，现场审核模板 27/27 matched、raw coverage 100%，但因检测到 `TBD` / `review-template` / `needs_site_owner_review` / `bootstrap` 标记，正式 `threshold_passed=false`，不能替代 site-owner approved 清单。
- `asset-inventory-review/asset-inventory-review-latest.json` / `asset-inventory-review/asset-inventory-review-latest.md`：资产现场清单审核包稳定副本；当前 `20260701-asset-inventory-review-r1` 为 `pass`，生成 27 行待现场 owner 复核资产、`formal-site-inventory.template.json`、review CSV 和 checklist，但该包只用于人工复核准备，不关闭正式覆盖率门。
- `asset-discovery-site-inventory.template.json`：现场期望资产清单 JSON 模板，用于覆盖率报告门计算真实发现率。
- `../runs/20260630-asset-frontend-detail-local/local-report.md`：资产台账前端本地联动证据，证明 `/assets` snapshot adapter 已消费 `/v1/assets/discovery/runs` 与 `/v1/assets/discovery/neighbors` 并在详情侧栏展示最近发现任务和 LLDP/SNMP 邻居上下文；该证据不等同于 Desktop Chrome 浏览器通过。
- `../runs/20260630-screen-live-snapshot-local/local-report.md`：态势大屏前端本地联动证据，证明 `/screen` snapshot adapter 已消费 `/v1/dashboard/stats`、`/v1/dashboard/encrypted/trend` 与 `/v1/dashboard/attack-phases`，并把真实 API 指标映射到楼宇覆盖、探针在线、全流量处理链路、PCAP 证据、攻击阶段和响应动作展示；该证据不等同于 Desktop Chrome 浏览器通过。
- `token-lifecycle-matrix-latest.json` / `token-lifecycle-matrix-latest.md`：auth-service API token 创建、读取、SHA-256 hash/prefix 持久化、跨租户拒绝、人工重签发、撤销、过期和同步 `audit_logs` 的 live matrix 结果。
- `playbook-state-machine-latest.json` / `playbook-state-machine-latest.md`：SOAR Playbook 启停、max_runs/cooldown、执行写入和状态恢复的 live 状态机回归结果。
- `forensics-task-state-machine-latest.json` / `forensics-task-state-machine-latest.md`：PCAP 取证任务取消状态机回归结果，覆盖 processing 可取消、completed 409、跨租户 403 和 `PCAP_CANCEL` 审计。
- `rule-state-machine-latest.json` / `rule-state-machine-latest.md`：规则启用/停用状态机回归结果，覆盖 `rule:enable` 权限、跨租户/只读拒绝、版本递增、`rule_versions`、`rule_outbox` 和 `RULE_ENABLE` / `RULE_DISABLE` 审计。
- `deployment-state-machine-latest.json` / `deployment-state-machine-latest.md`：部署管理状态机回归结果，覆盖灰度、激活、暂停、恢复、回滚、非法迁移 409、跨租户/只读拒绝、`deployment_history` 和审计落库。
- `model-version-state-machine-latest.json` / `model-version-state-machine-latest.md`：模型版本状态机回归结果，覆盖注册、激活、旧 active 弃用、主动弃用、非法弃用 409、跨租户/只读拒绝和 `MODEL_VERSION_*` 审计落库。
- `compliance-audit-preflight-latest.json` / `compliance-audit-preflight-latest.md`：合规审计 live 闭环结果，覆盖 admin 生成合规报告、报告查询、合规 audit-trail、审计日志 API、PostgreSQL 持久化、跨租户隔离和 viewer 生成报告 403。
- `notification-governance-preflight-latest.json` / `notification-governance-preflight-latest.md`：通知治理 live 闭环结果，覆盖通知设置更新、明文密钥拒绝、测试发送、静默规则创建/停用、viewer 写入拒绝、跨租户隔离、PostgreSQL 持久化和审计日志可查。
- `topic-governance-preflight-latest.json` / `topic-governance-preflight-latest.md`：专题治理 live 闭环结果，覆盖专题保存视图、范围更新、订阅、报告导出、证据包导出、viewer 写入/导出拒绝、跨租户隔离、PostgreSQL 持久化和审计日志可查。
- `whitelist-governance-preflight-latest.json` / `whitelist-governance-preflight-latest.md`：白名单治理 live 闭环结果，覆盖草案创建、提交审批、审批激活、延期、停用、命中检查、viewer 写入拒绝、跨租户隔离、PostgreSQL 持久化和审计日志可查。
- `baseline-governance-preflight-latest.json` / `baseline-governance-preflight-latest.md`：行为基线治理 live 闭环结果，覆盖基线列表/详情读取、基线 reset、viewer 写入拒绝、跨租户审计隔离、PostgreSQL 持久化和审计日志可查。
- `settings-governance-preflight-latest.json` / `settings-governance-preflight-latest.md`：系统设置治理 live 闭环结果，覆盖 display 用户设置保存/读取、API token 创建、Scope 更新、轮换、吊销、validate、viewer 写入拒绝、跨租户隔离、PostgreSQL 持久化和 token 审计日志可查；token 响应证据仅保留脱敏版本。
- `probe-ops-governance-preflight-latest.json` / `probe-ops-governance-preflight-latest.md`：探针运维治理 live 闭环结果，覆盖配置下发、连通性测试、mTLS 证书 SecretRef 轮换、批量升级、viewer 写入拒绝、跨租户隔离、PostgreSQL 持久化、`PROBE_*` 审计日志和明文敏感材料拒绝。

## 运行方式

```bash
ALLOW_BLOCKERS=true \
DESKTOP_CHROME_STATUS=pass \
DESKTOP_CHROME_URL=http://10.0.5.8:30180/login \
DESKTOP_CHROME_TITLE='园区网络全流量采集与分析系统' \
DESKTOP_CHROME_ARTIFACT='desktop-chrome-login-smoke-20260629-r1.json' \
DESKTOP_CHROME_BUSINESS_STATUS=pass \
DESKTOP_CHROME_BUSINESS_URL=http://10.0.5.8:30180/alerts \
DESKTOP_CHROME_BUSINESS_ARTIFACT='desktop-chrome-business-smoke-r4.json' \
RUN_ID=20260629-ui-contract-preflight-r5 \
tests/e2e/live_ui_contract_preflight.sh
```

`DESKTOP_CHROME_STATUS` 与 `DESKTOP_CHROME_BUSINESS_STATUS` 必须来自 Codex Desktop Chrome wrapper 的实测结果。若 wrapper 不可用、返回 transport closed 或 title/url 不匹配，脚本会将 Desktop Chrome 登录门禁或受保护业务页点击记为 blocker。

真实页面 1:1 视觉/交互双门禁：

```bash
ALLOW_BLOCKERS=true \
DESKTOP_CHROME_STATUS=blocked \
DESKTOP_CHROME_DETAIL='codex-desktop-node-repl desktop_chrome_list_tabs and js_reset returned Transport closed; Chrome extension backend unavailable, iab fallback forbidden' \
DESKTOP_CHROME_ARTIFACT=doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-latest.json \
RUN_ID=20260702-ui-visual-interaction-preflight-r33-capture-session-bridge-r4 \
LOG_DIR=doc/02_acceptance/runs/20260702-ui-visual-interaction-preflight-r33-capture-session-bridge-r4 \
tests/e2e/live_ui_visual_interaction_preflight.sh
```

正式通过必须把 `ALLOW_BLOCKERS=false`，并为每个 visual target id 提供 `ui-visual-interaction/latest/<visual-target-id>/actual-1920.png`、`diff-1920.png`、`metrics.json` 和 `capture-meta.json`，同时为每个 route id 提供 `ui-visual-interaction/latest/<route-id>/interaction.json`。`metrics.json` 必须证明真实 React 页面截图相对 `visual-acceptance.json` 的 source image 通过阈值；`capture-meta.json` 必须由 receiver 生成，并证明 Desktop Chrome 上传原图和落盘 PNG 都是 1920x1080，且 `post_capture_resize=false`；`interaction.json` 必须证明 Codex Desktop Chrome backend、无 4xx/5xx、无 `requestfailed`、无 `pageerror`、无非预期 console error，并覆盖该页面的业务动作。

截图由 Codex Desktop Chrome wrapper 产出后，用仓库工具生成 diff 与 metrics：

```bash
tests/e2e/ui_visual_diff_metrics.py \
  --target-id dashboard \
  --route /dashboard \
  --source doc/04_assets/ui_suite_gpt_v1/screens/pages/dashboard.png \
  --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/dashboard/actual-1920.png \
  --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/dashboard/diff-1920.png \
  --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/dashboard/metrics.json
```

如需把 Codex Desktop Chrome 侧截图和交互结果写回 Linux 工作区，可启动短生命周期接收器。该接收器要求 `CODEX_CAPTURE_KEY` 请求头，不会把 smoke token 写入磁盘；Chrome bridge 当前返回 JPEG 截图，receiver 会转换为门禁固定读取的 `actual-1920.png`，交互结果通过 `POST /interaction/<route-id>` 写入 `interaction.json`：

```bash
DESKTOP_SMOKE_TOKEN=<redacted> \
CODEX_CAPTURE_KEY=<redacted> \
tests/e2e/ui_desktop_capture_receiver.py \
  --host 0.0.0.0 \
  --port <port> \
  --evidence-dir doc/02_acceptance/02-regression/ui-visual-interaction/latest \
  --max-uploads 30 \
  --expected-width 1920 \
  --expected-height 1080
```

采集前先生成目标驱动的 Desktop Chrome 工作队列，避免手工拼接动态 URL 或遗漏 `/topics` 的三种视觉状态：

```bash
tests/e2e/ui_desktop_capture_plan.mjs \
  --base-url http://10.0.5.8:30180 \
  --receiver-url http://10.0.5.8:<port> \
  --output-json doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.json \
  --output-md doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.md
```

`capture-plan-latest.json` 中的 `visual_targets[].wrapper_call` 和 `interactions[].wrapper_call` 必须由 Codex Desktop Chrome extension backend 实际执行；截图上传后 receiver 必须写入 `capture-meta.json`，再按同一条 `metrics_command` 生成 `diff-1920.png` 与 `metrics.json`。该计划不会把缺失截图、缺失 capture meta 或失败 diff 记为通过。

可在采集前进一步生成本轮 Desktop Chrome 执行会话包，把 receiver、nonce-only redirect helper、待采集 visual batch、待采集 interaction batch、metrics 命令和正式复跑命令固化到 `capture-session-latest.json` / `.md`。该包仍只是执行队列，不替代真实截图或交互证据：

```bash
tests/e2e/ui_desktop_capture_session.mjs \
  --session-id <session-id> \
  --receiver-url 'http://10.0.5.8:<receiver-port>' \
  --smoke-redirect-base-url 'http://10.0.5.8:<redirect-port>'
```

Threat Intel 服务化回归：

```bash
RUN_ID=20260629-threat-intel-service-r6 \
tests/e2e/live_threat_intel_service.sh
```

该脚本会通过真实 APISIX/K8s/Kafka/PostgreSQL 验证情报 lookup、entry upsert、feed import、scheduled feed import、source list、告警富化、匿名 401、过期 token 401、只读 token 写入 403、租户隔离负例、upsert/import/scheduled feed 同步审计落库和 `threat.intel.v1` 事件发布消费；会写入/更新 `codex-live-smoke`、`codex-live-import`、`codex-live-scheduled`、`codex-live-cross-tenant` 测试情报条目。

Fusion × Threat Intel 联动回归：

```bash
RUN_ID=20260629-fusion-write-r2 \
tests/e2e/live_fusion_threat_intel_contract.sh
```

该脚本会验证 Fusion 页面 API plan、route manifest、snapshot adapter、Workbench 数据源卡联动、定向 Vitest、真实 APISIX `/fusion` 与 `/threat-intel` 读接口、viewer 缺 `rule:write` 写入 403、admin `POST /v1/fusion/conflicts/{id}/resolve` 和 `PATCH /v1/fusion/rules/{id}` 写入、PostgreSQL `fusion_conflict_resolutions`/`fusion_rule_overrides`/`audit_logs` 行级证据、live Web UI 镜像和 live bundle marker。

Fusion 价值量化 live preflight：

```bash
RUN_ID=20260630-fusion-value-report-preflight-r2-live-rollout \
LOG_DIR=doc/02_acceptance/runs/20260630-fusion-value-report-preflight-r2-live-rollout \
tests/e2e/live_fusion_value_report_preflight.sh
```

该脚本会先验证 `/api/v1/fusion/value-report` 后端路由、公式版本、前端 API plan、snapshot adapter 和 UI suite 契约，再使用 K8s `traffic-credentials` 中的 JWT Secret 通过真实 APISIX 请求 `/api/v1/fusion/stats`、`/api/v1/fusion/entities` 与 `/api/v1/fusion/value-report?window_hours=168`。只有 value-report 返回 `fusion-value-ablation-v1`、单源/多源对象、三个 delta 指标、`formula_reproducibility` quality gate 和 `Fusion Stats API` / `Alert MTTR` evidence 时才可作为 live 结构验收证据；冻结样本窗口、真实消融结果和试点签认仍需独立补齐。

SNMP/LLDP 主动资产发现回归：

```bash
RUN_ID=20260629-asset-discovery-r2-report \
tests/e2e/live_asset_discovery_preflight.sh
```

该脚本会通过真实 APISIX、auth-service、asset-service 和 PostgreSQL 验证 SNMP/LLDP 发现：scope catalog 包含 `asset:discover`，viewer 仅 `asset:read` 时写接口返回 403；凭据只登记 `k8s://...` SecretRef、不接收明文；发现任务写入 `asset_discovery_runs`；无 observations 的 scanner worker 路径会创建 failed run 并记录错误；成功写操作同步进入 `audit_logs`；观测资产写入 `assets`；LLDP 邻居关系写入 `asset_topology_links`；稳定证据为 `asset-discovery-latest.*`。

SNMP/LLDP 资产发现覆盖率报告门：

```bash
ALLOW_BLOCKERS=true \
SITE_ASSET_INVENTORY_JSON=doc/02_acceptance/02-regression/asset-inventory-review/latest/formal-site-inventory.template.json \
RUN_ID=20260701-asset-discovery-coverage-r3-review-packet-guard \
LOG_DIR=doc/02_acceptance/runs/20260701-asset-discovery-coverage-r3-review-packet-guard \
tests/e2e/live_asset_discovery_coverage_report.sh
```

生成现场审核包：

```bash
RUN_ID=20260701-asset-inventory-review-r1 \
LOG_DIR=doc/02_acceptance/runs/20260701-asset-inventory-review-r1 \
tests/e2e/live_asset_inventory_review_packet.sh
```

提供现场期望资产清单后复跑：

```bash
SITE_ASSET_INVENTORY_JSON=/path/to/site-assets.json \
MIN_DISCOVERY_COVERAGE_PCT=95 \
RUN_ID=<site-run-id> \
tests/e2e/live_asset_discovery_coverage_report.sh
```

该脚本只读真实 APISIX 和 PostgreSQL；没有 `SITE_ASSET_INVENTORY_JSON` 时必须保持 `blocked`。如果输入清单带 `review_required=true`，或仍含 `TBD`、`review-template`、`needs_site_owner_review`、`bootstrap` 等审核/草案标记，脚本会计算匹配率但仍把正式覆盖率门保持 `blocked`，避免把 live observed bootstrap 或审核模板误报为真实现场设备发现率达标。

Token 生命周期回归：

```bash
RUN_ID=20260629-token-lifecycle-r2 \
LOG_DIR=doc/02_acceptance/runs/20260629-token-lifecycle \
tests/e2e/live_token_lifecycle_matrix.sh
```

该脚本会验证 auth-service API token scope catalog、创建/读取、`api_tokens` SHA-256 hash 与 `token_prefix` 持久化、viewer 403、跨租户 404、raw token validate、regenerate 后旧 token 401/新 token 200、revoke 后 401、短期过期 401，以及 `create_token`/`regenerate_token`/`revoke_token` 同步 `audit_logs` 行级证据；不会把明文 token 写入证据目录。

Playbook 状态机回归：

```bash
RUN_ID=20260629-playbook-state-machine-r3 \
tests/e2e/live_playbook_state_machine.sh
```

该脚本会通过 APISIX 验证 Playbook catalog、禁用后执行拒绝、max_runs/cooldown 更新、执行记录写入和最终状态恢复；会改写并恢复指定 Playbook 的 override 状态。

PCAP 取证任务状态机回归：

```bash
RUN_ID=20260629-forensics-task-state-machine-r1 \
tests/e2e/live_forensics_task_state_machine.sh
```

该脚本会写入临时 `tasks` 夹具，验证 processing 任务取消持久化、completed 任务取消返回 409、跨租户取消返回 403 且原状态不变，并查询 `audit_logs` 中的 `PCAP_CANCEL`。

规则启用/停用状态机回归：

```bash
RUN_ID=20260629-rule-state-machine-r2 \
tests/e2e/live_rule_state_machine.sh
```

该脚本会写入临时 `rules` 夹具，验证 `rule:enable` 用户可启用/停用规则、只读和跨租户请求返回 403、`rules.version` 递增、`rule_versions` 与 `rule_outbox` 写入，以及审计页可查询 `RULE_ENABLE` / `RULE_DISABLE`。

部署管理状态机回归：

```bash
RUN_ID=20260629-deployment-state-machine-r2 \
tests/e2e/live_deployment_state_machine.sh
```

该脚本会写入隔离 tenant 的用户、规则版本、模型版本和部署夹具，验证 planned -> gray -> active -> paused -> active -> rolled_back，激活时 supersede 前一 active deployment，planned rollback 返回 409，跨租户和 viewer 动作返回 403，并查询 `deployment_history` 与 `audit_logs` 中的部署动作证据。

模型版本状态机回归：

```bash
RUN_ID=20260629-model-version-state-machine-r1 \
tests/e2e/live_model_version_state_machine.sh
```

该脚本会写入隔离 tenant 的用户、feature set、模型和模型版本夹具，通过 APISIX 验证版本注册、registered -> active、旧 active -> deprecated、active -> deprecated、registered 弃用 409、跨租户和 viewer 动作 403，并查询 `audit_logs` 中的 `MODEL_VERSION_CREATE` / `MODEL_VERSION_ACTIVATE` / `MODEL_VERSION_DEPRECATE` 与失败审计证据。

合规审计闭环回归：

```bash
RUN_ID=20260630-compliance-audit-preflight-r1 \
tests/e2e/live_compliance_audit_preflight.sh
```

该脚本会通过真实 APISIX/JWT/PostgreSQL 验证合规报告生成、`/v1/compliance/reports` 查询、`/v1/compliance/audit-trail` 查询、`/v1/audit/logs` 查询、`COMPLIANCE_REPORT_GENERATED` 审计落库、跨租户审计查询隔离，以及 viewer 生成报告返回 403。

通知治理闭环回归：

```bash
ALLOW_BLOCKERS=true \
RUN_ID=20260630-notification-governance-preflight-r1 \
LOG_DIR=doc/02_acceptance/runs/20260630-notification-governance-preflight-r1 \
tests/e2e/live_notification_governance_preflight.sh
```

该脚本会通过真实 APISIX/JWT/PostgreSQL 验证 `/v1/notifications/settings` 读写、敏感通知值必须走 `secret_ref`、`/v1/notifications/test` 测试发送、`/v1/notifications/silence-rules` 创建/查询/停用、viewer 写入 403、跨租户更新 404、`notification_silence_rules` 持久化，以及 `NOTIFICATION_SETTINGS_UPDATED` / `NOTIFICATION_TEST_SENT` / `NOTIFICATION_SILENCE_RULE_CREATED` / `NOTIFICATION_SILENCE_RULE_UPDATED` 审计日志可查。

专题治理闭环回归：

```bash
RUN_ID=20260630-topic-governance-preflight-r2 \
LOG_DIR=doc/02_acceptance/runs/20260630-topic-governance-preflight-r2 \
tests/e2e/live_topic_governance_preflight.sh
```

该脚本会通过真实 APISIX/JWT/PostgreSQL 验证 `/v1/topics/tunnel|exfil|apt` 读取、`/v1/topics/views` 保存/共享/收藏、`/v1/topics/scopes/{topic}` 范围更新、`/v1/topics/subscriptions` 创建/停用、`/v1/topics/reports/export` 报告导出、`/v1/topics/evidence-packages/export` 证据包导出、viewer 写入/导出 403、跨租户更新 404、`topic_saved_views` / `topic_scope_overrides` / `topic_subscriptions` / `topic_exports` 持久化，以及 `TOPIC_*` 审计日志可查。

白名单治理闭环回归：

```bash
RUN_ID=20260630-whitelist-governance-preflight-r2 \
LOG_DIR=doc/02_acceptance/runs/20260630-whitelist-governance-preflight-r2 \
tests/e2e/live_whitelist_governance_preflight.sh
```

该脚本会通过真实 APISIX/JWT/PostgreSQL 验证 `/v1/whitelist` 草案创建、提交审批、审批激活、延期、停用、`/v1/whitelist/check` 命中与停用负例、viewer 写入 403、跨租户更新 404、`whitelist` 持久化，以及 `WHITELIST_CREATED` / `WHITELIST_APPROVAL_SUBMITTED` / `WHITELIST_APPROVED` / `WHITELIST_EXTENDED` / `WHITELIST_DISABLED` 审计日志可查。

行为基线治理闭环回归：

```bash
RUN_ID=20260630-baseline-governance-preflight-r2 \
LOG_DIR=doc/02_acceptance/runs/20260630-baseline-governance-preflight-r2 \
tests/e2e/live_baseline_governance_preflight.sh
```

该脚本会通过真实 APISIX/JWT/PostgreSQL 验证 `/v1/baselines` 列表、`/v1/baselines/{id}` 详情、`POST /v1/baselines/{id}/reset` 重置、viewer 写入 403、跨租户审计查询隔离、`behavior_baseline_resets` 持久化，以及 `BEHAVIOR_BASELINE_RESET` 审计日志可查。

系统设置治理闭环回归：

```bash
RUN_ID=20260630-settings-governance-preflight-r1 \
LOG_DIR=doc/02_acceptance/runs/20260630-settings-governance-preflight-r1 \
tests/e2e/live_settings_governance_preflight.sh
```

该脚本会通过真实 APISIX/JWT/PostgreSQL 验证 `/v1/auth/settings/display` 保存/读取、非法 settings category 拒绝、API token scope catalog、token 创建、Scope 更新、invalid scope 拒绝、raw token validate、regenerate 后旧 token 401 / 新 token 200、revoke 后 401、viewer 写入 403、跨租户 404、`user_settings` / `api_tokens` 持久化，以及 `create_token` / `regenerate_token` / `revoke_token` 审计日志可查；带明文 token 的响应只进入临时文件，latest artifact 只保存 redacted JSON。

探针运维治理闭环回归：

```bash
RUN_ID=20260630-probe-ops-governance-r2 \
LOG_DIR=doc/02_acceptance/runs/20260630-probe-ops-governance-r2 \
tests/e2e/live_probe_ops_governance_preflight.sh
```

该脚本会通过真实 APISIX/JWT/PostgreSQL 验证 `/v1/probes` 列表、`POST /v1/probes/{id}/config` 配置下发、`POST /v1/probes/{id}/connectivity-test` 连通性测试、`POST /v1/probes/{id}/certificates/rotate` mTLS SecretRef 轮换、`POST /v1/probes/batch-upgrade` 批量升级、viewer 写入 403、跨租户 404、`probe_operations` 持久化、`probes.software_version` 更新、`PROBE_CONFIG_PUSH` / `PROBE_CONNECTIVITY_TEST` / `PROBE_CERT_ROTATE` / `PROBE_BATCH_UPGRADE` 审计日志可查，并确认 latest artifact 不含明文证书、私钥、token 或 Bearer 凭据。
