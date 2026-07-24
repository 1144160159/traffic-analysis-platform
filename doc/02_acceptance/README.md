# 课题一验收证据包索引

更新时间：2026-07-02
对象：园区网络流量智能检测与分析 / 全流量采集分析系统

当前未开发、未生产化、未验收闭环和旧文档误报项，统一见 `../05_status/未开发项梳理-2026-06-19.md`；代码真实状态和证据校正见 `../05_status/代码实证状态核对-2026-06-19.md`。

最新页面校正（2026-07-23，告警中心）：`/alerts` 已按 r653/r652 完成真实列表、最新态投影、规范攻击阶段、精确同源 IP、完整筛选/CSV、持久化反馈/视图/响应动作、Kafka outbox 后台补偿和 Windows Chrome 双视口闭环。live contract 为 36/36，历史 4 条 pending outbox 已恢复，最终观测 17/17 published、pending=0；全图差异率 `0.0998302469 <= 0.13`，三路独立终审均 ACCEPT。稳定入口为 `02-regression/alert-center-development-progress-latest.json`、`alert-center-review-adjudication-latest.json` 与 `alert-center-rollout-r653.json`；整项目仍未完成，下一页面为战役列表 `/campaigns`。

最新校正（2026-07-01）：Kafka `SASL_SSL`/`SCRAM-SHA-512`/TLS/ACL live rollout 已完成，`20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live` 为 `11/11` 通过，后置 Kafka security rollout preflight `20260630-kafka-security-rollout-preflight-r9-post-sasl-ssl` 为 `9/10` 通过、0 blockers、1 warning。release manifest 已推进到 `20260701-release-manifest-r80-ha-review-r1`，`12/12` 通过并索引 656 个 acceptance runs、143 个 source file hashes、58 个 live workloads、146 个 pod image IDs 和 29 个 live Kafka topics；deployment preflight 已推进到 `20260630-deployment-preflight-r60-fusion-value-report`，`16/16` 通过、0 blockers、0 warnings，release package 覆盖 138 files；业务流 API `20260630-business-flow-api-r26-baseline-governance` 为 `46/46` 通过。production security preflight 已推进到 `20260630-production-security-preflight-r49-waiver-registry`，仍为 `blocked`：`20/21` 通过、1 blocker、0 warnings；Kafka plaintext blocker、生产 ExternalSecret blocker、live workload digest pin blocker、local/dev waiver registry 未豁免项均已关闭，剩余 blocker 是当前 Flannel-only 集群 NetworkPolicy-capable CNI 为 0。通知治理 `20260630-notification-governance-preflight-r1` 为 `26/26` 通过；专题治理 `20260630-topic-governance-preflight-r2` 为 `42/42` 通过；白名单治理 `20260630-whitelist-governance-preflight-r2` 为 `26/26` 通过；行为基线治理 `20260630-baseline-governance-preflight-r2` 为 `16/16` 通过；系统设置治理 `20260630-settings-governance-preflight-r1` 为 `45/45` 通过；探针运维治理 `20260630-probe-ops-governance-r2` 为 `27/27` 通过，当轮 alert-service 已滚动到 `docker.io/traffic/alert-service@sha256:77a5e9b4f5b8680e3c2b3451915f84ac8e2d07b3d2ea06adfcd28f4ce2086e93`；资产发现 RBAC/audit `20260630-asset-discovery-rbac-r1` 为 `11/11` 通过，资产现场清单审核包 `20260701-asset-inventory-review-r1` 为 `pass`：27 个待现场 owner 复核资产、duplicate_key_count=0，并生成 review CSV、formal-site-inventory.template.json 和 checklist；资产发现覆盖率报告门 `20260701-asset-discovery-coverage-r3-review-packet-guard` 为 `blocked`：`6/7` 通过、1 blocker、0 warnings，review template 清单 27/27 matched、raw coverage 100%，但检测到 `TBD` / `review-template` / `needs_site_owner_review` / `bootstrap` 标记，正式 `threshold_passed=false`，不能替代 site-owner approved 清单；态势大屏本地联动 `20260630-screen-live-snapshot-local` 已通过 32/32 frontend tests、Web build 和 UI suite contract 0 error/0 warning，把 `/screen` 接到真实 dashboard API snapshot；Fusion 价值量化本地联动 `20260630-fusion-value-report-local` 已新增 `/v1/fusion/value-report`、单源/多源消融响应、检出提前量、误报下降和 MTTR 下降页面映射，并通过 Go alert/api、32/32 frontend tests、code-gap 46/46 和 UI suite contract；同日新增 `tests/e2e/live_fusion_value_report_preflight.sh` 作为 Fusion 价值量化 live APISIX/JWT/K8s 结构证据门，alert-service 已滚动到 `docker.io/traffic/alert-service@sha256:89d2e52463f7bcadf103b8c6274dd5559570fc7a2ff0da7103c1faf9d83e0569`，`20260630-fusion-value-report-preflight-r2-live-rollout` 为 `pass`：19/19 passed、0 blockers、0 warnings；仍需补冻结样本窗口、真实消融结果和试点签认；auth-service 当前已滚动到 `docker.io/traffic/auth-service@sha256:b5a247f01baf26c4939711bf25c22cb3668718d2ff4f303b8c40cac572c5dc11`，asset-service 当前已滚动到 `docker.io/traffic/asset-service@sha256:0ff5bc4b1084b610e8508e47195dddbc692d8a10ff28daa653508a00af5b95bd`。

新增 NetworkPolicy enforcement 专用预检 `20260630-network-policy-enforcement-preflight-r1-flannel-blocked` 为 `blocked`：`2/4` 通过、2 blockers、0 warnings；repo NetworkPolicy dry-run 通过，20 个 live NetworkPolicy 对象存在，但 policy-capable CNI pods 为 0，默认拒绝和白名单负例探针被正确跳过，避免在 Flannel-only 网络上产生假通过。新增 NetworkPolicy enforcement readiness 工具 `tests/e2e/live_network_policy_enforcement_readiness.sh`，`20260630-network-policy-enforcement-readiness-r1` 已生成 `05-security/network-policy-readiness/latest/` 下的 CNI migration runbook、CNI selection、rollback checklist、enforcement probe review-template、post-CNI preflight command 和 evidence manifest；结果为 `pass` 但带 2 个 warning，正式门仍需 policy-capable CNI 和负例探针通过。HA readiness 最新 r10 `20260701-ha-readiness-preflight-r10-review-packet` 仍 `blocked`：13/14 checks passed、1 blocker、0 warnings；HA 演练证据准备包 `20260630-ha-drill-evidence-bootstrap-r1` 已在 `06-resilience/bootstrap/latest/` 生成 operator approval、timeline、snapshot、RTO/RPO、data consistency 和 failover report review-template，新增 HA drill review packet `20260701-ha-drill-review-r1` 已在 `06-resilience/review/latest/` 生成 5 个组件、7 个复核文件、`formal_artifact_count=0` 的维护窗口审查工作板；二者都不会替代正式根目录 failover/RTO-RPO 报告。r10 进一步要求 6 个正式根目录报告齐全并拒绝含 review-template/TBD 标记的改名草案。最新 UI 契约 r17 `20260701-ui-contract-preflight-r17-desktop-login-pass-business-redirect-current` 的 repo/UI/API 契约仍为 19/19 无 blocker，Desktop Chrome wrapper 已能打开 `/login`，但当时受保护 `/alerts` 仍重定向到 `/login`；这不是 repo/UI 页面结构失败，而是已认证 Desktop 业务页 smoke 当轮未闭环。新增 UI 视觉/交互双门禁 `tests/e2e/live_ui_visual_interaction_preflight.sh`，当前 r39 `20260702-ui-visual-interaction-preflight-r39-viewport-probe-normalized` 为 `blocked`：28/28 React page component 存在、30/30 视觉目标 source image 存在且为 1920x1080、前端源码直接嵌入设计图 blocker 为 0、repo/live Desktop smoke token 配置均通过、capture session 覆盖当前缺口，并新增 `/viewport-probe` 前置视口校准，latest probe 当前为 `blocked`、`window_metrics=2560x1271`、期望 `1920x1080`，gap report 归组 `viewport_probe_blocked=1`；但逐目标视觉 diff 仍为 0/30，逐路由业务交互证据为 4/28；r39 已把 Desktop bridge r5 证据路径写入 summary 与 project completion blocker detail。新增 `tests/e2e/ui_desktop_capture_plan.mjs` 和稳定计划 `02-regression/ui-visual-interaction/capture-plan-latest.json` / `.md`，已把 30 个视觉目标、28 个交互路由、动态详情页 URL、receiver 上传端点、capture-meta 要求和 metrics 命令整理成 Desktop Chrome 采集工作队列；新增 `tests/e2e/ui_desktop_capture_session.mjs` 与 `capture-session-latest.json` / `.md`，把当前 30 个 visual pending 和 24 个 interaction pending 绑定到 receiver、redirect helper、safe wrapper call、metrics 命令和正式复跑命令，且 r39 preflight 已校验该 session 覆盖当前缺口；这些计划只驱动真实采集，不替代通过证据。Desktop bridge r5 复核 `20260702-desktop-chrome-bridge-transport-closed-r5` 显示 `desktop_chrome_list_tabs` 与 `js_reset` 均失败于 `Transport closed`，因此当前无法新增 Desktop Chrome 截图或交互证据。项目级完成度审计已刷新到 `20260702-project-completion-audit-r69-ui-r39-viewport-probe-normalized`，稳定入口为 `09-completion/project-completion-audit-latest.json` / `.md`：当前整体仍为 `blocked`，8 个门通过、9 个门阻断，新增独立 `oidc_sso` 门并读取 r4 live OIDC/SSO 证据为 pass，`desktop_browser_smoke` 与 `ui_visual_interaction` blocker 明细均已读取当前 r39/r5 证据并写入 Desktop bridge r5 artifact、capture session status/run_id、viewport probe 校准步骤、evidence-finalization 结果和覆盖情况，不再只依赖旧 UI contract 浏览器记录；其余 blocker 继续包括生产安全/NetworkPolicy enforcement、HA RTO/RPO、10 x 100Gbps/512Mpps 性能、检测质量第三方包、资产发现现场清单覆盖和用户/第三方签认。详见 `../05_status/live-digest-secret-docsync-2026-06-29.md`。

新增 completion blocker closure readiness 工具 `tests/e2e/live_completion_blocker_closure_readiness.sh`。`20260702-completion-blocker-closure-readiness-r59-ui-r39-viewport-probe-normalized` 读取最新项目完成度审计，把 9 个 completion blockers 统一整理为 `09-completion/blocker-closure/latest/` 下的 closure ledger、review board、owner matrix、evidence readiness map、exception register 和 formal rerun commands，并在 summary 顶层记录 source audit run `20260702-project-completion-audit-r69-ui-r39-viewport-probe-normalized`；结果为 `pass`：17/17 checks、0 blockers、0 warnings，记录 `blocker_count=9`、`ready_input_count=30`、`external_action_count=9`、`formal_rerun_command_count=20`。该包已把 `ui-visual-interaction-gap-report-latest.json`、`ui-visual-interaction/capture-session-latest.json` 与 `ui-visual-interaction/evidence-finalization-latest.json` 纳入 Desktop browser smoke 与 UI visual/interaction 的 ready inputs；它只是执行准备板，不会把项目完成度从 `blocked` 改为 `pass`。

继续校正（2026-07-02）：Codex Desktop Chrome wrapper 通过工具发现暴露后再次复检，Chrome extension backend 已能打开生产 `/login`；最新 UI 契约 `20260701-ui-contract-preflight-r17-desktop-login-pass-business-redirect-current` 仍为 `blocked`，20/21 通过，repo/UI/API 非浏览器契约为 19/19、0 个非浏览器 blocker，当轮唯一 blocker 是受保护 `/alerts` 业务页回落 `/login`。本轮已新增 nonce-only redirect helper `tests/e2e/ui_desktop_smoke_redirect.py`，并通过短时 hash smoke acceptance window 采集 `/alerts` interaction pass；生产与仓库 `DESKTOP_SMOKE_TOKEN_ENABLED` 当前均保持为 `true` 以持续采集。新增 `doc/02_acceptance/05-security/production-security-waivers.yaml` 并扩展 `tests/e2e/live_production_security_preflight.sh` 消费结构化 waiver registry；`20260630-production-security-preflight-r49-waiver-registry` 为 `blocked`：20/21 checks passed、1 blocker、0 warnings，local/dev Kafka plaintext、placeholder raw Secret template、privileged containers 和 host namespace workloads 均已被显式 waiver 覆盖且 unwaived count 为 0，唯一 blocker 仍是 Flannel-only 集群 NetworkPolicy-capable CNI 为 0。release manifest 已刷新到 `20260701-release-manifest-r80-ha-review-r1`：12/12 passed，索引 656 个 acceptance runs、143 个 source file hashes、58 个 workloads、146 个 pod image IDs 和 29 个 live Kafka topics。第三方签认 readiness 已刷新到 `20260702-third-party-signoff-readiness-r25-ui-r39-viewport-probe-normalized`，其 baseline evidence input 与当前 release r80 一致，并已加入 UI visual/interaction、UI evidence finalizer、OIDC/SSO、资产覆盖和 completion snapshot 输入。项目级完成度审计已刷新到 `20260702-project-completion-audit-r69-ui-r39-viewport-probe-normalized`，仍为 `blocked`：8 个门通过、9 个门阻断，新增 `oidc_sso` gate 通过，Desktop browser smoke 与 UI blocker 明细已包含 Desktop bridge r5 artifact 与 capture session status/run_id、viewport probe 校准步骤、evidence-finalization 结果和覆盖情况；closure board 已刷新到 `20260702-completion-blocker-closure-readiness-r59-ui-r39-viewport-probe-normalized`，并把 UI gap report、capture session 与 evidence finalization 纳入执行准备输入。

追加校正（2026-07-02 r39）：`20260702-ui-visual-interaction-preflight-r39-viewport-probe-normalized` 为当前 UI visual/interaction 双门禁稳定入口，结果仍为 `blocked`，但已把 repo/live `DESKTOP_SMOKE_TOKEN_ENABLED=true` 纳入正式 blocker 检查且均通过；当前通过 `9/13` checks，另有 1 个 warning，视觉目标源图 `30/30`、React page component `28/28`、设计图实现引用 blocker `0`，capture session 覆盖当前 `30` 个 visual gap 和 `24` 个 interaction gap，evidence finalizer 为当前 4 张已有 actual 生成 metrics/diff 且结果 blocked，剩余 blocker 是视觉 diff `0/30`、业务交互 `4/28` 和 Codex Desktop Chrome extension backend 当前 MCP transport closed；另有 viewport probe warning：`2560x1271` != `1920x1080`。r39 已在 summary 和 gap report 中写入 `desktop_chrome_artifact=doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-latest.json`，对应 r5 bridge 复核。配合 `tests/e2e/ui_desktop_smoke_redirect.py` 的 nonce-only helper 打开受保护路由；门禁现在要求 protected route hash 被消费、final path 匹配目标路由、不能回落 `/login` 且 final URL 不残留 smoke token。OIDC/SSO live 预检 `20260702-oidc-sso-preflight-r4-completion-gate` 已通过，并作为 project completion r69 的独立 `oidc_sso` pass gate，证明生产 `/login` 分发 SSO tab 与 `/oidc/callback` chunk，`/api/v1/auth/oidc/login` 302 到公开 Keycloak，且 `traffic-ui` 客户端登录页可用。

资产发现覆盖率新增只读清单草案工具 `tests/e2e/live_asset_inventory_bootstrap.sh`。`20260630-asset-inventory-bootstrap-r1` 已从 live `/api/v1/assets` 导出 27 个 observed assets 到 `02-regression/asset-discovery-site-inventory.bootstrap-latest.json`，并用 run-local `STABLE_DIR` 验证该草案可被 `live_asset_discovery_coverage_report.sh` 消费，草案自检 `20260630-asset-discovery-coverage-bootstrap-r1` 为 27/27 matched、coverage 100%。新增现场审核包工具 `tests/e2e/live_asset_inventory_review_packet.sh`，`20260701-asset-inventory-review-r1` 已生成 27 行待现场 owner 复核资产、review CSV、formal-site-inventory.template.json 和 checklist。正式覆盖率报告门已重跑为 `20260701-asset-discovery-coverage-r3-review-packet-guard`：审核模板同样匹配 27/27、raw coverage 100%，但因仍含 `TBD` / `review-template` / `needs_site_owner_review` / `bootstrap` 标记，脚本把正式 `threshold_passed=false` 并保持 `blocked`，避免把 live observed inventory 或 review template 当作真实 site-owner approved inventory。

检测质量新增只读盲测包草案工具 `tests/e2e/live_detection_quality_package_bootstrap.sh`。`20260630-detection-quality-bootstrap-r1` 已从 live `/api/v1/alerts` 导出 45 个候选样本，并生成 `04-detection-quality/bootstrap/latest/` 下的 `sample-index.bootstrap.csv`、label/prediction review-template 和第三方签认模板。新增盲测复核包工具 `tests/e2e/live_detection_quality_review_packet.sh`，`20260701-detection-quality-review-r1` 为 `pass`：45 个候选样本、0 个重复 `sample_id`、6/6 checks passed，并生成 sample-review、labeling/prediction worklist、formal-package manifest template、threshold lock template、attestation template 和 checklist。随后正式 `live_detection_quality_preflight.sh` 复跑为 `20260701-detection-quality-preflight-r5-review-packet`，结果仍为 `blocked`，因为 review packet 不生成正式 `dataset-manifest.yaml`、`threshold-lock.json`、`labels.csv`、`predictions.csv` 或 `third-party-attestation.yaml`；`evaluate_blind_package.py` 还会扫描正式 artifact 中的 bootstrap/review-template 标记和空签认字段，合成改名包也不能通过。该设计用于准备第三方冻结包，不能替代第三方盲测结论。

性能验收新增只读硬件窗口草案工具 `tests/perf/100g_capture/live_capture_performance_package_bootstrap.sh`。`20260630-capture-performance-bootstrap-r1` 已把当前 live 节点、probe capture profile、500k stress context 和正式 preflight 结果整理到 `03-performance/bootstrap/latest/`，生成 review-required 的硬件/流量草案、10x100G/512Mpps 结果 review-template 和 operator runbook。新增复核包工具 `tests/perf/100g_capture/live_capture_performance_review_packet.sh`，`20260701-capture-performance-review-r1` 为 `pass`：2 个目标、7 个复核文件、0 个正式 artifact、6/6 checks passed；稳定包在 `03-performance/review/latest/`。随后正式 `live_capture_performance_preflight.sh` 复跑为 `20260701-capture-performance-preflight-r4-review-packet`，结果仍为 `blocked`：11/18 checks passed、4 blockers、3 warnings，缺真实 `hardware-inventory.yaml`、`traffic-profile.yaml`、`results/10x100g-summary.json` 和 `results/512mpps-summary.json`；脚本已增加正式 artifact 的 bootstrap/review-template 标记 guard，合成改名包也不能通过。该设计用于准备硬件窗口，不能替代 GATE-P0-03/04。

用户/第三方签认 readiness 已推进到 `20260702-third-party-signoff-readiness-r25-ui-r39-viewport-probe-normalized`。本轮把 `user-acceptance-signoff.md`、`pilot-deployment-proof.md` 和 `demo-script.md` 中可由 release/deployment/business-flow/UI/security/HA/performance/detection-quality 证据证明的字段预填为“内部证据状态 + 待外部签认”，并扩展 `tests/e2e/live_third_party_signoff_readiness.sh`，使 readiness 输出 `placeholder-owner-summary.bootstrap.json`、带 owner/owner_reason 的占位清单、`evidence_inputs` 和 `evidence_input_run_ids`；同时扩展 `tests/e2e/live_project_completion_audit.sh`，使 formal signoff gate 拒绝与当前 release manifest 不一致的陈旧签认包。r25 仍为 `pass` readiness 但带 2 个 warning：162 个占位/待签认标记仍需人工填写，其中 `project_review=84`、`user_signoff=48`、`site_operations=15`、`third_party_lab=8`、`project_team=3`、`performance_lab=2`、`maintenance_window=2`；9 个上游非 pass 或 blocked 证据需例外决策；baseline evidence input 绑定当前 release r80，UI visual/interaction 绑定 r39，UI evidence finalizer 绑定 r1，OIDC/SSO 绑定 r4，completion snapshot 绑定 r68，HA evidence input 绑定当前 HA r10，performance evidence input 绑定当前性能 r4。因此正式 `trial_third_party_signoff` 仍必须保持 `blocked`。

HA 破坏性演练新增证据准备包工具 `tests/chaos/live_ha_drill_evidence_bootstrap.sh` 和审查包工具 `tests/chaos/live_ha_drill_review_packet.sh`。`20260630-ha-drill-evidence-bootstrap-r1` 已把 `tests/chaos/ha_drill_plan.yaml`、最新 HA readiness、operator approval、timeline、snapshot index、RTO/RPO table、data consistency report 和 Kafka/Flink/ClickHouse/PostgreSQL/MinIO failover report review-template 整理到 `06-resilience/bootstrap/latest/`；`20260701-ha-drill-review-r1` 已把 bootstrap 模板整理成 `06-resilience/review/latest/` 下的 component drill review、RTO/RPO evidence worklist、maintenance-window approval template、formal artifact manifest template、data-consistency checklist 和 operator checklist。两者结果均只表示草案/复核包可用。正式 `live_ha_readiness_preflight.sh` 复跑为 `20260701-ha-readiness-preflight-r10-review-packet`，仍为 `blocked`，因为 6 个根目录正式演练报告均未产出；脚本已增加 bootstrap/review-template/TBD 标记 guard，合成改名包也不能通过。

合规审计闭环已推进到 2026-07-19 r367/r368：alert-service `docker.io/traffic/alert-service@sha256:8b364a7b6171b239395cb4c6fb4bc10850f9d6880c4621be2edbc4d92a41fba9` 与 Web UI `docker.io/traffic/web-ui@sha256:48e92b7da33a305f9c769b7a83bb86797dcbd2668acc4259e93b9aa96ccb3e89` 已完成两节点导入和 Ready rollout。最新真实 APISIX/JWT/PostgreSQL 预检 `compliance-r367-final-20260719110622` 为 52/52 passed、0 blockers、0 warnings，覆盖 9 个 fail-closed section、`compliance:read/write/export/remediate/finalize` 与 `audit:read`、viewer 403、跨租户 404、invalidated 历史报告隔离、真实 ZIP/PDF/DOCX、canonical hash、整改幂等和数据库不可变固化。Xshell `127.0.0.1:9224` 下 Windows Chrome 150 已完成 r368 业务交互和 4/4 视觉门禁，差异率 `0.0749951775–0.1018557099 < 0.125`，逻辑/布局复审均 P0=0/P1=0/P2=0；页面验收证据入口为 `02-regression/compliance-development-progress-latest.json`。该页面闭环不替代项目级第三方正式评测材料与生产安全 blocker。

通知治理闭环已补齐：`20260630-notification-governance-preflight-r1` 为 `pass`，26/26 通过，证明通知设置更新、明文密钥拒绝、通知测试发送、静默规则创建/停用、viewer 403、跨租户隔离、`notification_silence_rules` 持久化和 `NOTIFICATION_*` 审计落库均已通过真实 APISIX/JWT/PostgreSQL 验证。该轮同时将 alert-service 滚动到 `docker.io/traffic/alert-service@sha256:d46cffeb90f649eacd07da2b8f34713706d4819c72adbff4dd8c74c55972061c`。

专题治理闭环已补齐：`20260630-topic-governance-preflight-r2` 为 `pass`，42/42 通过，证明专题面板三类专题读取、保存视图创建/共享/收藏、专题范围更新、订阅创建/停用、报告导出、证据包导出、viewer 403、跨租户隔离、`topic_saved_views` / `topic_scope_overrides` / `topic_subscriptions` / `topic_exports` 持久化和 `TOPIC_*` 审计落库均已通过真实 APISIX/JWT/PostgreSQL 验证。该轮同时将 alert-service 滚动到 `docker.io/traffic/alert-service@sha256:ea3a74019e53f18c2cadd9c460234eb0fd634fd3babed9279205aff0c09ae919`。

白名单治理闭环已补齐：`20260630-whitelist-governance-preflight-r2` 为 `pass`，26/26 通过，证明白名单草案创建、提交审批、审批激活、延期、停用、命中检查、viewer 403、跨租户 404、`whitelist` 持久化和 `WHITELIST_*` 审计落库均已通过真实 APISIX/JWT/PostgreSQL 验证。该轮同时将 alert-service 滚动到 `docker.io/traffic/alert-service@sha256:c0bf72ca45d3c22acf2c3586e3ef88c49c79edd507a870b704131d6678d27f22`。

行为基线治理闭环已补齐：`20260630-baseline-governance-preflight-r2` 为 `pass`，17/17 通过，证明行为基线列表/详情读取、`POST /v1/baselines/{id}/reset` 重置、viewer 403、跨租户审计隔离、`behavior_baseline_resets` 持久化和 `BEHAVIOR_BASELINE_RESET` 审计落库均已通过真实 APISIX/JWT/PostgreSQL 验证。该轮同时将 alert-service 滚动到 `docker.io/traffic/alert-service@sha256:b72cabec23b3cce37332501fe4a90fc05a54020b132b69c9121d2db6fcc6ba4f`。

系统设置治理闭环已推进到 2026-07-20 r509/r510：Auth Service `docker.io/traffic/auth-service@sha256:6e3cb398fb1c68c4dccbcc727af17bfb5f08b4f8218fa2baf128c290a8109735` 与 Web UI `docker.io/traffic/web-ui@sha256:ad61548ca0f8434d7c93a9529960a065d21e09863f241bc41533178afb952cb7` 已双节点导入并 Ready。最新 `20260720-settings-governance-r510` 为 `pass`，88/88 通过，覆盖系统配置乐观锁、真实连接探测、审计状态/发现/精确 tested_count、路由与 API RBAC、严格域通配符、目标令牌权限上限、令牌创建/范围/轮换/吊销事务审计及跨租户隔离。Xshell `127.0.0.1:9224` 的 Windows Chrome 150 完成 20/20 交互检查与 18 个原生点击；五类 560px Drawer 均不全屏且位于业务区，视觉差异率 `0.0711535494 < 0.125`。逻辑与布局复审均 ACCEPT，P0=0/P1=0/P2=0；完整证据见 `02-regression/settings-development-progress-latest.json`。

探针运维治理闭环已补齐：`20260630-probe-ops-governance-r2` 为 `pass`，27/27 通过，证明 `/probes` 前端动作契约、`/v1/probes` 列表、配置下发、连通性测试、mTLS 证书 SecretRef 轮换、批量升级、viewer 403、跨租户 404、`probe_operations` 持久化、`probes.software_version` 更新、`PROBE_*` 审计落库和敏感明文拒绝均已通过真实 APISIX/JWT/PostgreSQL 验证。该轮同时新增 APISIX route `44` 覆盖 `/api/v1/probes/*`，并将 alert-service 滚动到 `docker.io/traffic/alert-service@sha256:77a5e9b4f5b8680e3c2b3451915f84ac8e2d07b3d2ea06adfcd28f4ce2086e93`。

## 1. 证据分层

验收材料必须分层管理，不能把功能冒烟、回归测试、性能验收、算法验收和第三方测试混写成一个“已通过”。

| 层级 | 目的 | 可证明内容 | 不能证明内容 |
|---|---|---|---|
| Smoke | 快速证明系统主链路可用 | 页面、API、基础数据链路未断 | 线速性能、检测质量、第三方验收 |
| Regression | 证明版本变更未破坏既有功能 | 路由、接口、状态机、取证、反馈、权限负例 | 10 x 100Gbps、95%/5% |
| Acceptance | 证明任务书核心指标可复测 | 吞吐、P95、准确率、误报率、安全、HA、证据包 | 第三方认证结论 |
| Third-party | 证明外部机构认可 | CNAS/第三方测试、试点证明、经济效益材料 | 代码质量和后续持续运行 |

## 2. 证据目录建议

```text
doc/02_acceptance/
  README.md
  runs/
    <run_id>/
      run-summary.json
      task.yaml
      plan.md
      context/
        context.snapshot.json
        gap-index.json
        dependency-map.json
        evidence-ledger.json
        god-view.md
      guidance/
        guidance.json
        guidance-report.md
      design/
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
      context-pack/
        task-context-pack.md
        task-context-pack.json
        context-budget.json
        decision-log.jsonl
        handoff.md
      workflow/
        workflow-summary.json
        workflow-report.md
        gate-decision.md
      implementation/
        implementation-brief.md
        codex-implementation-prompt.md
        patch-scope.json
        patch-validation.json
        apply-report.md
      task-state/
        task-state.json
        task-board.md
        transition-plan.json
        apply-report.md
        transition-log.jsonl
      git-status.txt
      changed-files.txt
      local-report.md
      review-report.md
      design-delta.md
      live-report.md
      evidence-report.md
  00-baseline/
    commit-sha.txt
    images.txt
    manifests.txt
    database-schema.txt
    kafka-topics.txt
    release-manifest-latest.json
    release-manifest-<run-id>.json
  01-smoke/
    live-100-round-summary.json
    route-matrix.md
    browser-console-report.md
  02-regression/
    README.md
    ui-contract-preflight-latest.json
    ui-contract-preflight-latest.md
    ui-contract-matrix-latest.json
    ui-visual-interaction-preflight-latest.json
    ui-visual-interaction-preflight-latest.md
    ui-visual-interaction-matrix-latest.json
    ui-visual-interaction-gap-report-latest.json
    ui-visual-interaction-gap-report-latest.md
    ui-visual-interaction/
      capture-session-latest.json
      capture-session-latest.md
      latest/
    api-contract-report.md
    ui-playwright-report.md
    rbac-negative-report.md
    pcap-forensics-report.md
    feedback-mlops-report.md
    compliance-audit-preflight-latest.json
    compliance-audit-preflight-latest.md
    compliance-reports-latest.json
    compliance-audit-trail-latest.json
    compliance-development-progress-latest.json
    compliance-review-adjudication-latest.json
    compliance-rollout-r368.json
    audit-log-generated-latest.json
    audit-governance-preflight-latest.json
    audit-log-development-progress-latest.json
    audit-log-review-adjudication-latest.json
    audit-log-rollout-r425.json
    audit-log-rollout-r477.json
    audit-log-rollout-r483.json
    notification-governance-preflight-latest.json
    notification-governance-preflight-latest.md
    notification-settings-latest.json
    notification-silence-rules-latest.json
    notification-audit-latest.json
    topic-governance-preflight-latest.json
    topic-governance-preflight-latest.md
    topic-views-latest.json
    topic-subscriptions-latest.json
    topic-report-export-latest.json
    topic-evidence-package-export-latest.json
    topic-audit-latest.json
    whitelist-governance-preflight-latest.json
    whitelist-governance-preflight-latest.md
    whitelist-latest.json
    whitelist-disable-latest.json
    whitelist-audit-latest.json
    whitelist-development-progress-latest.json
    whitelist-review-adjudication-latest.json
    whitelist-rollout-r352.json
    settings-governance-preflight-latest.json
    settings-governance-preflight-latest.md
    settings-display-latest.json
    settings-token-create-redacted-latest.json
    settings-token-regenerate-redacted-latest.json
    asset-discovery-site-inventory.bootstrap-latest.json
    asset-discovery-site-inventory.bootstrap-latest.md
  03-performance/
    README.md
    capture-performance-plan.yaml
    capture-performance-result-schema.json
    capture-performance-preflight-latest.json
    capture-performance-preflight-latest.md
    hardware-inventory.template.yaml
    traffic-profile.template.yaml
    repo-stress-500k-summary-latest.json
    live-probe-capture-profile-latest.json
    live-node-summary-latest.json
    bootstrap/
      capture-performance-bootstrap-latest.json
      capture-performance-bootstrap-latest.md
      latest/
    review/
      capture-performance-review-latest.json
      capture-performance-review-latest.md
      latest/
  04-detection-quality/
    README.md
    dataset-manifest.template.yaml
    label-schema.yaml
    metric-definition.md
    bootstrap/
      detection-quality-bootstrap-latest.json
      detection-quality-bootstrap-latest.md
      latest/
    detection-quality-preflight-latest.json
    detection-quality-preflight-latest.md
    package-file-inventory-latest.json
    confusion-matrix-latest.csv
    stratum-metrics-latest.csv
  05-security/
    kafka-tls-sasl-acl-report.md
    external-secret-report.md
    mtls-rotation-report.md
    pcap-download-audit-report.md
    network-policy-report.md
    README.md
    production-security-preflight-latest.json
    production-security-preflight-latest.md
    network-policy-enforcement-preflight-latest.json
    network-policy-enforcement-preflight-latest.md
    network-policy-enforcement-probe-latest.json
    external-secret-operator-canary-latest.json
    external-secret-operator-canary-latest.md
    expected-production-externalsecrets-latest.json
    live-production-externalsecret-readiness-latest.json
    kafka-security-rollout-preflight-latest.json
    kafka-security-rollout-preflight-latest.md
    kafka-security-secret-readiness-latest.json
    kafka-tls-material-validation-latest.json
    kafka-scram-readiness-latest.json
    kafka-acl-live-summary-latest.json
    live-kafka-listener-summary-latest.json
    repo-default-credential-pattern-files-latest.txt
    repo-kafka-plaintext-files-latest.txt
    repo-unpinned-or-latest-image-files-latest.txt
    repo-image-lock-summary-latest.json
    repo-latest-or-mutable-image-lines-latest.txt
    repo-service-exposure-summary-latest.json
    repo-service-exposure-blockers-latest.json
    live-external-secret-reconciliation-summary-latest.json
    live-secret-operator-crds-latest.json
    live-secret-operator-pods-latest.json
    live-externalsecrets-inventory-latest.json
    live-sealedsecrets-inventory-latest.json
    live-cni-policy-capability-latest.json
    live-cni-policy-capability-summary-latest.json
    network-policy-live-latest.json
    network-policy-readiness/
      network-policy-enforcement-readiness-latest.json
      network-policy-enforcement-readiness-latest.md
      latest/
  06-resilience/
    ha-readiness-preflight-latest.json
    ha-readiness-preflight-latest.md
    kafka-topic-health-latest.json
    flink-running-job-health-latest.json
    clickhouse-replication-latest.json
    bootstrap/
      ha-drill-evidence-bootstrap-latest.json
      ha-drill-evidence-bootstrap-latest.md
      latest/
    review/
      ha-drill-review-latest.json
      ha-drill-review-latest.md
      latest/
  07-deployment/
    preflight-report.md
    site-values.md
    namespace-drift-report.md
    release-package-manifest.md
  09-completion/
    project-completion-audit-latest.json
    project-completion-audit-latest.md
    blocker-closure/
      completion-blocker-closure-readiness-latest.json
      completion-blocker-closure-readiness-latest.md
      latest/
    deployment-preflight-latest.json
    release-package-manifest-latest.json
    site-values-observed-latest.json
    secret-reference-readiness-latest.json
    non-business-external-ports-latest.json
    unpinned-or-latest-images-latest.json
    repo-image-lock-summary-latest.json
    repo-service-exposure-summary-latest.json
    repo-service-exposure-blockers-latest.json
  08-third-party/
    README.md
    pilot-package-manifest.json
    pilot-deployment-proof.md
    demo-script.md
    pilot-weekly-report-template.md
    economic-benefit.md
    user-acceptance-signoff.md
    ipr-index.md
    readiness/
      third-party-signoff-readiness-latest.json
      third-party-signoff-readiness-latest.md
      latest/
    cnas-test-report.pdf
```

## 3. P0 验收门禁

| 编号 | 门禁 | 当前口径 | 通过标准 | 责任角色 |
|---|---|---|---|---|
| GATE-P0-01 | 基线冻结 | 已建立 release manifest 回归证据包 | 2026-06-29 已新增 `tests/e2e/live_release_manifest.sh` 并生成 release manifest 稳定副本；最新 r80 `20260701-release-manifest-r80-ha-review-r1` 为 12/12 checks passed，固化 commit、dirty status、143 个 source file hashes、K8s core manifest dry-run 124 objects、58 个 workloads、146 个 pod image IDs、29 个 live Kafka topics、模型/规则/部署 API 目录和 656 个历史证据索引。该证据仅为回归基线，不等同于性能、第三方检测质量、生产安全或 HA 验收 | 项目经理/技术经理 |
| GATE-P0-02 | 功能主链路 | 业务流 API、token 生命周期、Playbook、Forensics、合规审计、通知治理、专题治理、白名单治理、行为基线治理、系统设置治理、探针运维治理和 OIDC/SSO 入口预检已 pass；最新 UI visual/interaction 双门禁 r39 证明 repo/live Desktop smoke token 配置通过，并强化 protected route interaction 证据、capture session 覆盖检查和 Desktop bridge r5 artifact 追踪，但 1:1 视觉/交互双门禁仍缺逐页 diff、capture-meta 与剩余业务交互证据 | 真实 API、Kafka/Flink/DB/UI 链路以无 4xx/5xx、无 `requestfailed`、无非 warning 控制台错误为准。2026-07-02 `20260702-ui-visual-interaction-preflight-r39-viewport-probe-normalized` 为当前 UI 双门禁稳定入口：`9/13` checks passed，React page component `28/28` 存在、`30/30` 视觉目标 source image 存在且为 `1920x1080`、未发现直接嵌入设计图作为页面实现，repo/live `DESKTOP_SMOKE_TOKEN_ENABLED=true` 均通过检查，capture session 覆盖当前 `30` 个 visual gap 和 `24` 个 interaction gap，evidence finalizer 为当前 4 张已有 actual 生成 metrics/diff 且结果 blocked；剩余 blocker 是视觉 diff `0/30`、业务交互 `4/28` 和 Codex Desktop Chrome extension backend 当前 MCP transport closed；另有 viewport probe warning：`2560x1271` != `1920x1080`。该门禁现在要求 protected route interaction 同时证明 hash 被消费、final path 匹配目标路由、未回落 `/login` 且 final URL 不残留 smoke token，并引用 Desktop bridge r5 evidence。OIDC/SSO live preflight r4 `20260702-oidc-sso-preflight-r4-completion-gate` 为 `passed`，并已纳入 project completion r69 的独立 `oidc_sso` pass gate，证明生产 `/login` 分发 SSO tab 与 `/oidc/callback` chunk、`/api/v1/auth/oidc/login` 302 到公开 Keycloak、`client_id=traffic-ui` 且 Keycloak 授权页 200。业务流 API preflight 最新 r26 `20260630-business-flow-api-r26-baseline-governance` 为 46/46 passed、`/api/v1/graph/explore`、`/api/v1/compliance/reports`、`/api/v1/compliance/audit-trail` 和 `/api/v1/audit/logs` 均经 APISIX 返回 200、0 blockers；通知治理 r1 为 26/26 passed；专题治理 r2 为 42/42 passed；白名单治理 r2 为 26/26 passed；行为基线治理 r2 为 16/16 passed；系统设置治理 r1 为 45/45 passed；探针运维治理 r2 为 27/27 passed，覆盖 `/api/v1/probes*` 配置下发、连通性测试、mTLS SecretRef 轮换、批量升级、viewer/cross-tenant 负例、`probe_operations` 和 `PROBE_*` 审计；Playbook 状态机 r1 为 21/21 passed；Forensics 任务状态机 r1 为 7/7 passed；token 生命周期矩阵 r2 为 23/23 passed | QA/全栈 |
| GATE-P0-03 | 10 x 100Gbps | 性能验收包、草案工具、复核包和预检已建立但验收仍 blocked | 2026-06-29 已新增 `tests/perf/100g_capture/`、`tests/perf/100g_capture/live_capture_performance_preflight.sh` 和 `03-performance/` 稳定证据；2026-06-30 新增 `tests/perf/100g_capture/live_capture_performance_package_bootstrap.sh`，`20260630-capture-performance-bootstrap-r1` 已生成 `03-performance/bootstrap/latest/` 草案包；2026-07-01 新增 `tests/perf/100g_capture/live_capture_performance_review_packet.sh`，`20260701-capture-performance-review-r1` 已生成 `03-performance/review/latest/` 复核包，2 个目标、7 个复核文件、0 个正式 artifact、6/6 checks passed。正式 r4 `20260701-capture-performance-preflight-r4-review-packet` 仍为 `blocked`，11/18 checks passed、4 blockers、3 warnings：缺 `hardware-inventory.yaml`、`traffic-profile.yaml`、`results/10x100g-summary.json`；已有 500k stress 仅约 0.94Mpps/1.3Gbps，不能替代多口线速；live 探针当前为 `af_packet`/2 cores 小 profile，不是 100G 验收 profile；本轮已加入正式性能 artifact 的 bootstrap/review-template guard，合成改名包也不能通过 | 性能/Probe |
| GATE-P0-04 | 512Mpps | 性能验收包、草案工具、复核包和预检已建立但验收仍 blocked | 同 GATE-P0-03 的 r4 预检证据；review packet 只生成 `result-summary-worklist.csv` 和 `formal-artifact-manifest.template.json`，正式门仍缺 `results/512mpps-summary.json`。通过必须包含 64B 或约定小包分布、持续时长、丢包率、解析率、Kafka lag、Flink backpressure 和资源水位的真实硬件窗口结果，且正式结果不能含 bootstrap/review-template 标记 | 性能/Probe |
| GATE-P0-05 | P95 <= 60s | 已有 3 分钟 live 闭环证据 | 2026-06-29 已补齐 `event_ts/ingest_ts/kafka_ts/flink_out_ts/api_seen_ts/ui_seen_ts` 证据链、`/api/v1/data-quality/latency-chain`、ClickHouse 落库、Flink Session r8 并行度 12 与 TaskManager slot/metaspace 调优；`runs/20260629-latency-chain/live-latency-chain-20260629-latency-chain-r15-3m-final-r8-slot32-summary.json` 结果 `pass`、`full_chain_closed=true`、0 gaps、0 command failures。3 分钟窗口各段 P95：flow event->ingest 5.96s、session ingest->Kafka 0.198s、session Kafka->Flink 16.11s、session event->Flink 22.01s、alert last_seen->created 22.28s | QA/SRE |
| GATE-P0-06 | 95%/5% | 盲测包契约/预检已建立但验收仍 blocked | 2026-06-29 已新增 `mlops/eval_packages/topic1_blind/`、`mlops/scripts/evaluate_blind_package.py` 和 `tests/e2e/live_detection_quality_preflight.sh`；当前稳定证据为 r5 `20260701-detection-quality-preflight-r5-review-packet`，结果 `blocked`：5/10 checks passed、5 blockers、0 warnings，缺冻结 `dataset-manifest.yaml`、`threshold-lock.json`、真实 `labels.csv`、真实 `predictions.csv` 和签名第三方 `third-party-attestation.yaml`。本轮新增 `tests/e2e/live_detection_quality_review_packet.sh`，`20260701-detection-quality-review-r1` 已把 45 个 live alert 候选样本整理成第三方标注、无标签预测、阈值锁定和签认复核工作台，但不会写入正式 artifact。本轮已在 `evaluate_blind_package.py` 加入正式包完整性 guard：正式 artifact 会扫描 `review_required` / `review-template` / bootstrap 标记，并拒绝 `signed_by` / `signed_at` 为空的第三方签认；临时把 bootstrap/template 改名成正式路径的合成包验证也会被阻断。因此不能声明 95% 检出率、5% 误报率或 CNAS 通过 | 算法/测试 |
| GATE-P0-07 | 生产安全 | Kafka SASL_SSL/TLS/ACL rollout 已完成；repo Kafka 安全 profile、live Secret/TLS readiness、生产 ExternalSecret reconciliation、repo 镜像锁和 live workload digest pin 已完成；live preflight 仍 blocked 于 CNI enforcement | 2026-06-29 已新增 `deployments/kubernetes/security/`：NetworkPolicy starter profile client dry-run 通过，ExternalSecret 模板已升级到 `external-secrets.io/v1` 并在 CRD 安装后 server-side dry-run 通过；`tests/e2e/live_external_secret_operator_canary.sh` r2 `20260629-external-secret-operator-canary-r2-local-chart` 为 11/11 passed，证明 ESO 2.7.0 三个 controller Ready、3 个镜像 digest-pinned，canary SecretStore/ExternalSecret 1/1 Ready 且源/目标 Secret reconciliation 一致。新增 `tests/e2e/live_external_secret_production_reconciliation.sh`，`20260629-external-secret-production-reconciliation-r1` 为 10/10 passed，`ClusterSecretStore/traffic-platform-secret-store` Ready，`13/13` 生产 ExternalSecrets Ready，58 个期望 key 源/目标一致，证据只记录名称、类型、key 集合和布尔结果，不落盘 secret value。Kafka rollout r6 `20260630-kafka-sasl-ssl-rollout-r6-controller-mtls-live` 为 `pass`：11/11 checks passed、0 blockers，证明 Kafka StatefulSet 已滚到 `SASL_SSL`/`SCRAM-SHA-512`、ACL authorizer active、init topic/ACL job 使用 secure client 完成、broker API/topic/ACL 检查通过；r4 的空 truststore 与 r5 的 KRaft controller `ANONYMOUS` 授权失败均已被 source truststore 修复、PKCS12 类型、controller mTLS 和 broker cert DN super user 关闭。Kafka security preflight r9 `20260630-kafka-security-rollout-preflight-r9-post-sasl-ssl` 为 `pass`：9/10 passed、0 blockers、1 warning，live listener 已无 plaintext marker。`tests/e2e/live_production_security_preflight.sh` 最新 r49 `20260630-production-security-preflight-r49-waiver-registry` 结果仍为 `blocked`：21 checks 中 20 passed、1 blocker、0 warnings；生产 ExternalSecret 13/13 Ready，live ExternalSecret/SealedSecret operator reconciliation 为 14/14，Secret reconciliation blocker 为 0，live workload digest pin 缺口为 0，live Kafka TLS/SASL listener profile、Keycloak TLS/SecretRef profile、local/dev waiver registry、placeholder raw Secret waiver、privileged container waiver 和 host namespace waiver 均已闭环。新增 `tests/e2e/live_network_policy_enforcement_preflight.sh`，最新 r1 `20260630-network-policy-enforcement-preflight-r1-flannel-blocked` 为 `blocked`：repo dry-run 通过、20 个 live NetworkPolicy 对象存在，但 policy-capable CNI pods 为 0，默认拒绝/白名单负例探针被跳过。新增 `tests/e2e/live_network_policy_enforcement_readiness.sh`，r1 `20260630-network-policy-enforcement-readiness-r1` 为 `pass`，已生成 CNI migration runbook、CNI selection、rollback checklist、probe review-template 和 post-CNI preflight command；该包只用于维护窗口准备，不证明 enforcement。剩余 production blocker 是 NetworkPolicy enforcement-capable CNI 缺失；安全负例和镜像签名/准入也未闭环 | 安全/实施 |
| GATE-P0-08 | 故障恢复 | readiness preflight、演练 bootstrap 和 review packet 已建立但验收仍 blocked | 2026-06-29 已新增 `tests/chaos/live_ha_readiness_preflight.sh` 和 `tests/chaos/ha_drill_plan.yaml`；2026-06-30 新增 `tests/chaos/live_ha_drill_evidence_bootstrap.sh`，`20260630-ha-drill-evidence-bootstrap-r1` 已生成 `06-resilience/bootstrap/latest/` 草案包，包含 operator approval、timeline、snapshot、RTO/RPO、data consistency 和 5 类组件 failover report review-template；2026-07-01 新增 `tests/chaos/live_ha_drill_review_packet.sh`，`20260701-ha-drill-review-r1` 已生成 `06-resilience/review/latest/` 复核包，5 个 HA 组件、7 个 review 文件、`formal_artifact_count=0`。最新 r10 `20260701-ha-readiness-preflight-r10-review-packet` 证据见 `06-resilience/ha-readiness-preflight-latest.json`。当前 r10 结果 `blocked`：13/14 通过、0 warnings，Kafka 29 topics leader/ISR 正常，Flink 9 个 RUNNING job checkpoint/异常检查通过，ClickHouse 13 张复制表健康，PostgreSQL 2 个 streaming replicas，Redis Sentinel、MinIO cluster-local health、APISIX 可达；HA readiness Kafka 检查使用 secure admin config，避免 plaintext/heap 假阻塞。唯一 blocker 是还没有维护窗口内的 Kafka/Flink/ClickHouse/PostgreSQL/MinIO 破坏性 RTO/RPO 演练报告；r10 要求根目录 6 个正式报告齐全，并扫描阻断 `review-template`、`review_required`、`TBD` 等草案标记，bootstrap/review 的 `formal_artifact_count=0` 不能替代正式根目录 failover/RTO-RPO 报告 | SRE/QA |
| GATE-P0-09 | 可复现部署 | site values/package/preflight 已建立且 deployment preflight 已 pass | 2026-06-29 已新增 `deployments/kubernetes/site-values.template.yaml`、`tests/e2e/live_deployment_preflight.sh` 和 `07-deployment/` 稳定证据；最新 r60 `20260630-deployment-preflight-r60-fusion-value-report` 见 `runs/20260630-deployment-preflight-r60-fusion-value-report/` 与 `07-deployment/deployment-preflight-latest.json`，结果 `pass`：17/17 checks passed、0 blockers、0 warnings，core K8s manifests client dry-run 124 objects，release package manifest 覆盖 138 files，repo 镜像 evidence lock 缺口 0，repo Service 非业务外部端口 0，2 节点/local-hdd/runtime workloads/APISIX 业务入口正常，live workload digest pin 缺口 0，Pending PVC 为 0 | 实施/SRE |
| GATE-P0-10 | 现场攻击面 | Service 暴露面已闭环，NetworkPolicy enforcement 未闭环 | `deployments/kubernetes/security/00-network-policies.yaml` 已定义默认拒绝和 APISIX 业务入口白名单，dry-run 通过；repo Service profile 已收敛为仅 APISIX 业务入口 `gateway/apisix:http:30180`，并由 `tests/e2e/k8s_service_exposure.py` 证明 0 个 repo 非业务外部端口；live Service 已收敛为仅 APISIX 业务入口；`20260630-network-policy-enforcement-preflight-r1-flannel-blocked` 证明 20 个 live NetworkPolicy 对象已创建，但当前 CNI 为 Flannel-only 且 policy-capable CNI pods 为 0，默认拒绝与白名单负例探针不能执行。`20260630-network-policy-enforcement-readiness-r1` 已生成维护窗口 CNI 切换和负例探针准备包。仍需接入或切换支持 NetworkPolicy enforcement 的 CNI，并重跑 `ALLOW_BLOCKERS=false RUN_ENFORCEMENT_PROBE=auto tests/e2e/live_network_policy_enforcement_preflight.sh` 完成负例证据 | 安全/网络 |

## 4. 功能点闭环证据映射

| 功能点 | 最小证据 | 验收证据 |
|---|---|---|
| 探针采集 | Probe 心跳、吞吐、丢包、gRPC mTLS；2026-06-30 已新增 `tests/e2e/live_probe_ops_governance_preflight.sh`，alert-service 滚动到 `docker.io/traffic/alert-service@sha256:77a5e9b4f5b8680e3c2b3451915f84ac8e2d07b3d2ea06adfcd28f4ce2086e93`；r2 `20260630-probe-ops-governance-r2` 为 27/27 passed，覆盖 `/v1/probes` 列表、配置下发、连通性测试、mTLS SecretRef 轮换、批量升级、viewer 403、跨租户 404、`probe_operations` 持久化、软件版本更新和 `PROBE_*` 审计落库；前端 `probes` API plan 已约束 batch-upgrade/config/connectivity/cert-rotate actions | 多端口线速压测和采集丢包报告仍需真实硬件窗口结果 |
| Ingest/Kafka | mTLS、token、限流、去重、topic offset | Kafka TLS/SASL/ACL、DLQ、重放和 topic catalog 对账 |
| Flink 分析 | RUNNING、checkpoint、无异常 | checkpoint age、lag、反压、故障恢复和输出一致性 |
| 告警运营 | 告警列表、详情、状态机、反馈 API | 状态迁移、并发、批量、审计、RBAC 负例 |
| 通知配置 | 2026-07-20 r467 已接受：Alert Service `notifications-r466` 以 `docker.io/traffic/alert-service@sha256:f4587a890b2dd5edbcaaf4d9aaaa1d0d61cdadbe5323cfd73b34bf43f9b5c9b0` 固定部署，live APISIX/JWT/PostgreSQL 预检 63/63 passed；升级 worker 在 deadline 使用完整 ClickHouse AlertInfo，绑定 policy ID/version、阶段延迟与 SHA-256 指纹，支持 stale lease recovery、lock-token heartbeat、最多 5 次退避；角色接收人会改变真实非邮件 endpoint，Slack/企微/钉钉/飞书业务成功码 fail-closed。Xshell `127.0.0.1:9224` 的 Windows Chrome 150 完成 18 类操作、78/78 可点击按钮、36/36 右侧 560px 非全屏 Drawer，业务 ROI `0.078162898 < 0.125`；逻辑与布局复审均 P0=0/P1=0。完整证据见 `02-regression/notification-development-progress-latest.json` | 已接受 r467；保留 provider 幂等/逐 endpoint 历史、长 I/O heartbeat 集成、状态别名规范化、键盘焦点与 200% 缩放等非阻断 P2，下一项进入系统设置 `/settings` |
| 系统设置 | 2026-07-20 r510 已接受：Auth Service r509 与 Web UI r502 均以 digest 固定并双节点 Ready；live APISIX/JWT/PostgreSQL 预检 88/88，覆盖真实连接探测、revision 乐观锁、严格 RBAC、目标令牌 scope 上限和事务审计。Xshell `127.0.0.1:9224` 的 Windows Chrome 150 完成 20/20 检查与 18 个原生点击，五类 560px Drawer 均位于业务区；视觉差异率 `0.0711535494 < 0.125`；逻辑与布局复审均 P0=0/P1=0/P2=0。完整证据见 `02-regression/settings-development-progress-latest.json` | 已接受 r510；下一项进入 404 `not-found` 页面 |
| 专题面板 | 2026-06-30 已新增 `tests/e2e/live_topic_governance_preflight.sh`，alert-service 滚动到 `docker.io/traffic/alert-service@sha256:ea3a74019e53f18c2cadd9c460234eb0fd634fd3babed9279205aff0c09ae919`；r2 `20260630-topic-governance-preflight-r2` 为 42/42 passed，覆盖三类专题读取、保存视图创建/共享/收藏、专题范围更新、订阅创建/停用、报告导出、证据包导出、viewer 写入/导出 403、跨租户 404、PostgreSQL `topic_saved_views` / `topic_scope_overrides` / `topic_subscriptions` / `topic_exports` 持久化和 `TOPIC_*` 审计落库；前端 `topics` API plan 与 snapshot adapter 已消费 `/v1/topics/views` 和 `/v1/topics/subscriptions` | 后续扩展到真实导出文件存储、专题模板审批、订阅渠道投递回执和更多 Desktop 浏览器视觉巡检 |
| 白名单治理 | 2026-07-19 r351/r352 已完成 draft-first、合法状态对、重复创建保护、两人审批、乐观锁、延期、停用、带版本删除、租户/RBAC 负例和 6 类 append-only 审计；FP 反馈 `add_to_whitelist` 在查询告警前要求 `alert:write`，草案与 `WHITELIST_CREATED` 在同一 PostgreSQL 事务提交，源/Docker/K8s schema 已同步。Alert Service 已推进到 `docker.io/traffic/alert-service@sha256:d623d9e54d8fdd754e46d6d46949f69c8117b46af7f24579429deef640b427bb`，live APISIX/JWT/PostgreSQL 预检 46/46 passed、0 blocker；Web UI 已推进到 `docker.io/traffic/web-ui@sha256:082270accbb437552d3cda8868e5f9fe6cd73601f8081534bbc80d26eacb5d24`。Xshell `127.0.0.1:9224` 下 Windows Chrome 150 完成 1920×1080 的真实创建→提交→独立审批→延期→停用→删除及 11/11 页面/浮层视觉门禁，删除后业务行消失且 6 条审计保留，像素差异率 `0.0398871528–0.0840128279 < 0.125`；逻辑与布局独立复审均 P0=0/P1=0。完整证据见 `02-regression/whitelist-development-progress-latest.json` | 已接受 r352；仅保留标题换行/次级 ID、主表固定操作列、Modal 留白三项非阻断 P2，继续合规队列 |
| 行为基准 | 2026-07-23 r641/r642 已按 UI 图重构并正式接受：五类基线、五维筛选、状态机、真实 ClickHouse 分布/时间桶/P50/P95/P99 ECharts、偏离解释、版本与治理操作均已接入；summary 按窗口、reset、阈值、冻结、漂移和待重建状态互斥重算，outbox `published/failed` 与错误直接来自 PostgreSQL。live APISIX/JWT/PostgreSQL/ClickHouse 为 65/65，Windows Chrome 150 经 Xshell `127.0.0.1:9224` 在 1920×1080 与 1600×900 为 16/16，完整前端测试 205/205，视觉全图及七个 ROI 全部 PASS；逻辑、布局、综合终审均 ACCEPT（P0=P1=0）。完整证据见 `02-regression/baseline-development-progress-latest.json` | 当前单页已闭环；保留后续生产数据规模与下游消费者长周期观测，不把本页接受写成全项目完成；下一页为告警中心 `/alerts` |
| 数据融合 | 2026-07-22 r616 修复浏览器缩窄时的整体自适应：外层业务网格改为流式详情列，编排主区采用 container query，输入/规则/输出列宽与间距使用 `clamp + cqw`，六个规则阶段始终保持单行顺序。原有 `ResizeObserver` 实测节点边界、ECharts custom 曲线/箭头、数据库 `recent_hits` ECharts bar、六列事件审计和 `object_type + object_id` 过滤保持不变。Web UI `sha256:8d5c84ac27f75f70eafefc85249b534cca957d8b84c80ffac89262f731dbf7ac` 已双节点导入并滚动至 generation 813 | Xshell `127.0.0.1:9224` 的 Windows Chrome 150 在 1920×1080、1536×864、1366×768 三档非全屏通过：每档节点 contained、`overlapPairs=0`、17/17 连线命中边界；1536/1366 无 document/stage 横向溢出，标题、主区与详情区均在视口内。业务请求/网络/console/page error 为 0，live contract 77/77。全局差异率 `0.117242 < 0.125`；pipeline/bottom 两项 raw ROI 仍作为 P3 原样保留，详见 `02-regression/fusion-development-progress-latest.json` 与 `../../design-qa.md` |
| Threat Intel 情报富化 | 2026-06-29 已新增独立 `threat-intel-service`、`threat.intel.v1` Topic、K8s Deployment、APISIX `/api/v1/threat-intel*` 路由和 `tests/e2e/live_threat_intel_service.sh`；r6 为 47/47 passed，线上镜像 `traffic/threat-intel-service:threat-intel-20260629-r6` 覆盖 JWT/RBAC 匿名 401、过期 token 401、只读 lookup 200、只读写入 403、内置 C2 lookup、tenant-scoped PostgreSQL upsert/lookup/list/import、跨租户 lookup/list 负例、feed import、scheduled feed import、告警 enrich、upsert/import/scheduled feed 同步 `audit_logs` 落库和 `threat.intel.v1` publish/consume；同日 `tests/e2e/live_fusion_threat_intel_contract.sh` 已推进到 `20260629-fusion-write-r2`，29/29 passed，证明 Fusion 页面契约消费 Threat Intel 服务并可通过 APISIX 写入 `POST /v1/fusion/conflicts/{id}/resolve`、`PATCH /v1/fusion/rules/{id}`，同步落库 `fusion_conflict_resolutions`、`fusion_rule_overrides` 和 `audit_logs`，且 viewer 写入 403、live Web UI 已滚动 `traffic/web-ui:fusion-write-20260629-r1` 并通过 bundle marker 验证 | Fusion 读写链路已闭环；后续进入多源融合价值量化等竞争力增强 |
| Web 路由权限 | routeManifest、`/auth/me`、403 证据页、UI 契约回归预检、OIDC/SSO 入口和 UI visual/interaction 双门禁；2026-06-29 `runs/20260629-ui-contract-preflight/` 历史 r5 为 21/21 passed，Desktop Chrome wrapper 曾通过 Chrome extension backend 打开生产 `/login`，并用受控 smoke token 完成 `/dashboard -> /alerts` 合法登录态业务页点击；2026-07-02 OIDC/SSO r4 为 passed，证明生产 `/login` SSO tab、callback chunk、OIDC discovery、302 到 Keycloak 和 `traffic-ui` 客户端页可用，且已纳入 project completion r69；最新 UI visual/interaction r39 为 blocked：React page component 28/28、30/30 source image 为 1920x1080、无直接嵌入设计图 blocker、repo/live smoke token 配置 OK、业务交互 4/28 且 protected route interaction 严格校验已生效，但还缺 30 个通过的逐页 screenshot diff、receiver capture-meta 原始尺寸证明和剩余 24 个 business interaction evidence，且当前 Codex Desktop Chrome MCP transport closed，不能声明 1:1 复刻；`runs/20260629-token-lifecycle/` r2 已补 API token 跨租户、重签发、撤销、过期和审计负例 | 真实 APISIX API + Playwright 权限矩阵已闭环；补可认证 Desktop Chrome 截图/交互流程后生成逐页真实 React 截图、diff、metrics、capture-meta 和 interaction 证据 |
| 态势大屏只读边界 | `screen:view`、生产 `/screen` 登录门禁、只读大屏 token 矩阵；token 生命周期 r2 已验证跨租户、重签发、撤销、过期和审计；`20260630-screen-live-snapshot-local` 已让 `/screen` 专用 snapshot adapter 消费 `/v1/dashboard/stats`、`/v1/dashboard/encrypted/trend`、`/v1/dashboard/attack-phases`，并把楼宇覆盖、探针在线、采集吞吐、协议解析率、Kafka 积压、Flink P95、证据完整度、攻击阶段和响应动作映射到大屏视觉模块，本地 32/32 frontend tests、Web build 和 UI suite contract 0 error/0 warning 通过 | 4K/2K/1080p 大屏巡检；当前只能证明 Desktop Chrome 登录页可达，不能声明 `/screen` 最新浏览器视觉验收通过 |
| WebSocket 实时通道 | `/ws/events` 鉴权、合法 token ready 帧、AppShell `实时通道/已连接` | JWT session 轮换/撤销与高并发实时推送 |
| PCAP 取证 | completed 下载、裁剪任务和取消状态机 | 2026-06-29 已补 hash、签名 URL 过期上限、路径/跨租户拒绝、下载校验和审计可查，证据见 `runs/20260629-pcap-forensics-integrity/`；同日新增 `tests/e2e/live_forensics_task_state_machine.sh` 与 `02-regression/forensics-task-state-machine-latest.*`，r1 为 7/7 passed，覆盖 processing 取消、completed 取消 409、跨租户 403、任务状态持久化和 `PCAP_CANCEL` 审计可查 |
| 图谱分析 | 节点、边、路径查询；2026-06-30 `tests/e2e/live_business_flow_api_preflight.sh` 最新 r26 `20260630-business-flow-api-r26-baseline-governance` 证明业务流所需 `/api/v1/graph/explore` 经 APISIX 返回 200，46/46 checks passed | 层级限制、慢查询、OpenSearch/Nebula 故障降级 |
| 资产融合与主动发现 | 资产列表、详情、SNMP/LLDP SecretRef 凭据、发现任务、scanner worker、RBAC/audit、LLDP 拓扑边和发现覆盖率报告门；2026-06-30 auth-service 已滚动到 `docker.io/traffic/auth-service@sha256:b5a247f01baf26c4939711bf25c22cb3668718d2ff4f303b8c40cac572c5dc11`，asset-service 已滚动到 `docker.io/traffic/asset-service@sha256:0ff5bc4b1084b610e8508e47195dddbc692d8a10ff28daa653508a00af5b95bd`，`tests/e2e/live_asset_discovery_preflight.sh` `20260630-asset-discovery-rbac-r1` 为 11/11 passed，证明 auth scope catalog 含 `asset:discover`、viewer 仅 `asset:read` 写入 403、凭据只登记 SecretRef、不接收明文，发现任务写入 `asset_discovery_runs`，成功写操作同步进入 `audit_logs`，观测资产 upsert 到 `assets`，LLDP 邻居写入 `asset_topology_links`，无 observations 的 scanner worker 路径会创建 `failed` run 并记录错误而非静默 queued；`20260630-asset-frontend-detail-local` 已把 `/v1/assets/discovery/runs` 与 `/v1/assets/discovery/neighbors` 合入 `/assets` 前端详情侧栏并通过 49 个 Vitest、Web build 和 UI suite contract 0 error/0 warning；`tests/e2e/live_asset_inventory_review_packet.sh` `20260701-asset-inventory-review-r1` 已生成 27 行待现场 owner 复核资产、review CSV、formal-site-inventory.template.json 和 checklist；`tests/e2e/live_asset_discovery_coverage_report.sh` `20260701-asset-discovery-coverage-r3-review-packet-guard` 已生成覆盖率报告门，真实统计 live 27 个资产、10 个 active discovery 资产、6 个 completed run、5 条拓扑边，并证明 review template 清单 27/27 matched、raw coverage 100%，但因存在审核标记不能关闭正式覆盖率 gate | 后续补真实交换机/园区网扫描窗口、周期任务结果覆盖率、Desktop Chrome 资产页浏览器验证，并提供不含 `TBD`/review-template/bootstrap 标记的 site-owner approved `SITE_ASSET_INVENTORY_JSON` 复跑覆盖率门；当前 r3 blocker 是 `inventory_review_marker_detected=true`，因此不能声明全网设备发现率达标 |
| MLOps | 训练、注册、激活、热更新；2026-06-29 已有 GATE-P0-06 盲测包契约和预检证据；同日新增 `tests/e2e/live_model_version_state_machine.sh` 与 `02-regression/model-version-state-machine-latest.*`，r1 为 20/20 passed，覆盖模型版本注册、registered -> active、旧 active -> deprecated、active -> deprecated、registered 弃用 409、跨租户/只读 403 和 `MODEL_VERSION_*` 审计落库 | 冻结样本量、盲测标签、预测输出、95% CI、第三方签认、champion/challenger 质量对比和维护窗口回滚演练 |
| SOAR 剧本 | 2026-07-19 r346 已完成 PostgreSQL 租户剧本、草稿版本、提交审批、两人审批、启停、仅模拟演练、回滚记录、完整证据导出和专用 `playbook:*` RBAC；未知 severity/asset risk fail-closed，未配置真实 provider 时 `/api/v1/playbooks/{name}/execute` 固定返回 501。Windows Chrome 经 Xshell CDP `127.0.0.1:9224` 对生产 `/playbooks` 完成交互与视觉双门禁，205 条导出边界探针为 205/205，回滚证据计数为 1，业务区差异率 `0.07687100161233736 < 0.125`；详细证据见 `02-regression/playbook-development-progress-latest.json` 和 `../../evidence/ui-image-breakdowns/pages/playbooks/verification.json` | 真实封禁、隔离、通知或工单 provider 接入前继续保持 live execute fail-closed；后续把完整导出升级为同一只读快照与流式输出，并在 provider 接入后补外部回执、失败补偿和 provider 级回滚演练 |
| 数据质量 | DLQ、迟到、解析率看板；2026-06-29 已有 DLQ dry-run modal、用户 JWT 到 replay API 契约、真实 fallback 文件非 dry-run 重放、Kafka 消费核对、跨 Pod 幂等、partial 失败样本回归和 live Kafka/Flink 坏消息注入进入 `dlq.v1` 证据：`runs/20260629-data-quality-dlq-dryrun/`、`runs/20260629-data-quality-dlq-business/`、`runs/20260629-dlq-replay-recovery/`、`runs/20260629-dlq-replay-failure-regression/`、`runs/20260629-kafka-flink-bad-message-dlq/` | 合法 JWT Playwright 业务页 dry-run 4/4 通过；Desktop Chrome wrapper 直开受保护页按门禁回落 `/login` |
| 合规审计 | 2026-07-19 r367/r368 已完成 9 section fail-closed 报告、dedicated compliance scopes、invalidated 历史隔离、真实 ZIP/PDF/DOCX、canonical hash、整改幂等和数据库不可变固化；live 预检 52/52 passed、0 blocker，Xshell/Windows Chrome 150 完成生成→导出→整改→固化交互及主页面/Drawer/双 Modal 4/4 视觉门禁，应用错误 0，差异率 `0.0749951775–0.1018557099 < 0.125`。详细证据见 `02-regression/compliance-development-progress-latest.json` | 页面与业务闭环已接受；项目级仍需断 Kafka/可靠队列降级专项、正式第三方评测材料和外部签认，不能由当前“证据不足”报告替代 |
| 审计日志 | 2026-07-20 r478/r483 对“详情/关联/复核全部不可用”完成二次专项回修：去除列表后台 `isFetching` 对缓存行按钮的错误禁用，操作列扩至 172px，三按钮扩至 `42×30`，固定列与按钮强制接受 pointer events 并增加当前动作选中反馈；详情、关联、复核仍统一留在 `604×514` 右侧“操作详情 / Diff 视图”。Windows Chrome 150 经 Xshell `127.0.0.1:9224` 使用真实鼠标坐标实测 37/37：3/3 中心点顶层元素均为对应 `BUTTON`，受控延迟 `/api/v1/audit/logs` 时详情仍未禁用并成功切换；同时覆盖 A→B 复核绑定、viewer 门禁、HTTP 201/PostgreSQL、导出/保存/完整性及 0 应用错误。raw 全图 diff `0.263769 > 0.125` 原样保留并沿用用户区域契约裁决。详细证据见 `02-regression/audit-log-development-progress-latest.json` | 逻辑与布局终审均 ACCEPT，P0/P1=0；r478/r483 已接受，仅保留 review POST pending 延迟证据与 44×44/键盘/缩放/读屏两项非阻断 P2 |
| 现场部署 | K8s 资源可 apply，Pod Running | preflight 全 PASS、无 default 漂移、site values 渲染、release 包可复现 |
| 中间件安全 | Kafka TLS/SASL/ACL 已 live rollout；2026-06-30 已有生产安全 preflight 证据 | Kafka TLS/SASL/ACL、CH/OS/Nebula/MinIO 非默认凭证、NetworkPolicy 负例；当前 Kafka security rollout/preflight 已通过，但 `runs/20260630-production-security-preflight-r48-topic-governance/` 仍因 NetworkPolicy-capable CNI 为 0 明确 blocked，不能作为生产安全验收通过 |
| 状态恢复 | 组件重启恢复 | 2026-06-29 已有只读 HA readiness preflight 和演练计划；最终验收仍需 Flink checkpoint/savepoint 跨节点恢复、PG RTO/RPO、MinIO lifecycle、CH schema diff 和 Kafka/CH/MinIO/PG 数据一致性报告 |

## 5. 试点交付节奏

试点采用“只读镜像/TAP 接入优先、先观测后联动”的策略。

| 阶段 | 时间 | 目标 | 输出 |
|---|---|---|---|
| 入场前 | T-2 周 | 数据授权、点位设计、site values、资源和保留期确认 | 授权单、部署拓扑、preflight 报告 |
| 第 1 周 | T+1 周 | 部署探针、网关、基础看板和采集健康 | 安装验收单、探针心跳、基础流量截图 |
| 第 2-4 周 | T+1 月 | 积累流量、资产、告警、PCAP 和误报样本 | 周报、告警样例、PCAP 证据、反馈记录 |
| 第 2 月 | T+2 月 | 形成误报率、MTTR、资产覆盖率、取证耗时对比 | 试点中期报告 |
| 第 3 月 | T+3 月 | 输出用户确认、经济效益证明和可复测演示数据集 | 盖章证明、收益材料、演示数据包 |

`08-third-party/` 已建立试点和第三方材料模板包，包含部署证明、演示脚本、周报、经济效益、用户确认和成果转化索引。该目录当前状态为 Template Ready；真实用户盖章、第三方报告和经济效益数据仍需在试点执行后补齐，不能据此宣称 Third-party Passed。

## 6. 状态标记规则

| 状态 | 含义 | 可用于汇报的措辞 |
|---|---|---|
| Done | 已经实现且有真实链路证据 | “已通过真实链路验证” |
| Evidence Ready | 有报告、日志、截图、数据对账 | “具备验收证据” |
| Acceptance Ready | 满足任务书专项指标 | “满足专项验收口径” |
| Third-party Passed | 第三方测试通过 | “通过第三方测试” |
| Gap | 没有证据或证据不够 | “待专项验证/待第三方验证” |

禁止把 Gap 写成 Done，禁止把 Done 写成 Acceptance Ready，禁止把内部 smoke 写成 Third-party Passed。
