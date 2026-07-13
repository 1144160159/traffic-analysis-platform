# Pixel Perfect 全流程状态

本文件是机器门禁状态，不替代逐图精拆记录。只有 `pixel-accepted` 才表示该图完整走完拆解、实现、Windows Chrome 截图、overlay、diff、智能体辅助审查和主线程验收。

## 汇总

- 总图数：241
- 已 pixel accepted：105
- 未完成：136
- pixel-accepted：105
- auxiliary-agent-review-missing：78
- unresolved-open：17
- production-route-evidence-missing：30
- main-thread-judgment-missing：9
- visual-diff-failed：1
- not-accepted：1

## 未完成队列

| 分类 | 图片 ID | 当前阶段 | 状态 | mismatch | 主线程判定 | 未解决项 |
|---|---|---|---|---:|---|---|
| `pages` | `alert-detail` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.073474 | `business-pixel-accepted` | - |
| `pages` | `alert-detail-evidence-files` | `unresolved-open` | `evidence-ready` | 0.071480 | `not-pixel-accepted` | `record:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification-difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4` |
| `pages` | `alert-detail-evidence-graph-path` | `unresolved-open` | `evidence-ready` | 0.073885 | `not-pixel-accepted` | `record:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification-difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4` |
| `pages` | `alert-detail-evidence-logs` | `unresolved-open` | `evidence-ready` | 0.068280 | `not-pixel-accepted` | `record:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification-difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4` |
| `pages` | `alert-detail-evidence-pcap` | `unresolved-open` | `evidence-ready` | 0.069007 | `not-pixel-accepted` | `record:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification-difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4` |
| `pages` | `alert-detail-evidence-session` | `unresolved-open` | `evidence-ready` | 0.069512 | `not-pixel-accepted` | `record:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4`<br>`verification-difference:production-state-mapping:/alerts/alert-default-1782752318016-1dd589c4` |
| `pages` | `alerts` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.073127 | `not-pixel-accepted` | - |
| `pages` | `assets` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.091182 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `assets-business-system` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.090109 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `assets-detail-basic` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.072521 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `assets-detail-history` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.066508 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `assets-detail-network-interface` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.066470 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `assets-detail-open-services` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.080345 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `assets-detail-ownership` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.066352 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `assets-network-device` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.092418 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `assets-server` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.094118 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `assets-unknown` | `production-route-evidence-missing` | `frontend-preview-evidence-ready` | 0.101463 | `not-pixel-accepted` | `difference:production-evidence-scope:/assets` |
| `pages` | `attack-chains` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.076585 | `not-pixel-accepted` | - |
| `pages` | `audit-log` | `unresolved-open` | `evidence-ready` | 0.075393 | `not-pixel-accepted` | `verification:production-visual-diff:full image`<br>`verification-difference:production-visual-diff:full image` |
| `pages` | `audit-log-operation-context` | `main-thread-judgment-missing` | `evidence-ready` | 0.075413 | `changes-required-production-state-mapping` | - |
| `pages` | `audit-log-related-chain` | `main-thread-judgment-missing` | `evidence-ready` | 0.086058 | `changes-required-production-state-mapping` | - |
| `pages` | `baselines` | `main-thread-judgment-missing` | `evidence-ready` | 0.079266 | `awaiting-main-thread-decision` | - |
| `pages` | `baselines-account` | `main-thread-judgment-missing` | `evidence-ready` | 0.086353 | `changes-required-production-state-mapping` | - |
| `pages` | `baselines-port` | `main-thread-judgment-missing` | `evidence-ready` | 0.097304 | `changes-required-production-state-mapping` | - |
| `pages` | `baselines-protocol` | `main-thread-judgment-missing` | `evidence-ready` | 0.095761 | `changes-required-production-state-mapping` | - |
| `pages` | `baselines-time-window` | `main-thread-judgment-missing` | `evidence-ready` | 0.119128 | `changes-required-production-state-mapping` | - |
| `pages` | `campaign-detail` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.080500 | `not-pixel-accepted` | - |
| `pages` | `campaign-detail-impact-account` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.104235 | `not-pixel-accepted` | - |
| `pages` | `campaign-detail-impact-business-system` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.108049 | `not-pixel-accepted` | - |
| `pages` | `campaign-detail-impact-campus` | `unresolved-open` | `evidence-ready` | 0.113117 | `not-pixel-accepted` | `review:contains unresolved/open diff wording` |
| `pages` | `campaign-detail-impact-department` | `unresolved-open` | `evidence-ready` | 0.105759 | `not-pixel-accepted` | `verification:strict-pixel-diff:full image`<br>`difference:strict-pixel-diff:full image`<br>`verification-difference:strict-pixel-diff:full image`<br>`review:contains unresolved/open diff wording` |
| `pages` | `campaign-detail-impact-service` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.101381 | `not-pixel-accepted` | - |
| `pages` | `campaigns` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.081509 | `not-pixel-accepted` | - |
| `pages` | `compliance` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.088522 | `not-pixel-accepted` | - |
| `pages` | `dashboard` | `visual-diff-failed` | `production-route-diff-failed` | 0.128044 | `not-pixel-accepted` | `record:production-visual-diff:full image`<br>`verification:production-visual-diff:full image`<br>`difference:production-visual-diff:full image`<br>`verification-difference:production-visual-diff:full image` |
| `pages` | `data-quality` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.115868 | `not-pixel-accepted` | - |
| `pages` | `data-quality-field-quality` | `main-thread-judgment-missing` | `evidence-ready` | 0.120016 | `business-pixel-accepted` | - |
| `pages` | `data-quality-flink-quality` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.112391 | `not-pixel-accepted` | - |
| `pages` | `data-quality-replay-reconcile` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.079072 | `not-pixel-accepted` | - |
| `pages` | `data-quality-report` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.105131 | `not-pixel-accepted` | - |
| `pages` | `data-quality-settings` | `unresolved-open` | `evidence-ready` | 0.083732 | `not-pixel-accepted` | `review:contains unresolved/open diff wording` |
| `pages` | `data-quality-storage-quality` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.088092 | `not-pixel-accepted` | - |
| `pages` | `data-quality-topic-health` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.113237 | `not-pixel-accepted` | - |
| `pages` | `deployments` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `encrypted-traffic` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.088279 | `not-pixel-accepted` | - |
| `pages` | `encrypted-traffic-egress-profile` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.100691 | `not-pixel-accepted` | - |
| `pages` | `encrypted-traffic-evidence-center` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.076500 | `not-pixel-accepted` | - |
| `pages` | `encrypted-traffic-fingerprint` | `unresolved-open` | `evidence-ready` | 0.099467 | `not-pixel-accepted` | `verification:strict-pixel-gate:full screenshot`<br>`difference:strict-pixel-gate:full screenshot`<br>`verification-difference:strict-pixel-gate:full screenshot` |
| `pages` | `encrypted-traffic-tunnel-detection` | `unresolved-open` | `evidence-ready` | 0.086759 | `not-pixel-accepted` | `verification:strict-pixel-gate:full screenshot`<br>`difference:strict-pixel-gate:full screenshot`<br>`verification-difference:strict-pixel-gate:full screenshot` |
| `pages` | `forensics` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.085466 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `fusion` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.079236 | `not-pixel-accepted` | - |
| `pages` | `graph` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.077957 | `not-pixel-accepted` | - |
| `pages` | `graph-account-access-path` | `unresolved-open` | `evidence-ready` | 0.084890 | `not-pixel-accepted` | `record:production-state-mapping:/graph`<br>`verification:production-state-mapping:/graph`<br>`difference:production-state-mapping:/graph`<br>`verification-difference:production-state-mapping:/graph` |
| `pages` | `graph-attack-path` | `unresolved-open` | `evidence-ready` | 0.091501 | `not-pixel-accepted` | `record:production-state-mapping:/graph`<br>`verification:production-state-mapping:/graph`<br>`difference:production-state-mapping:/graph`<br>`verification-difference:production-state-mapping:/graph` |
| `pages` | `graph-communication-path` | `unresolved-open` | `evidence-ready` | 0.076133 | `not-pixel-accepted` | `record:production-state-mapping:/graph`<br>`verification:production-state-mapping:/graph`<br>`difference:production-state-mapping:/graph`<br>`verification-difference:production-state-mapping:/graph` |
| `pages` | `login` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.077224 | `not-pixel-accepted` | - |
| `pages` | `mlops` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `business-pixel-accepted-r289` | - |
| `pages` | `models` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `business-pixel-accepted-r283` | - |
| `pages` | `models-activation-audit-gate` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `models-feature-anomaly-explanation` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `models-feature-rule-contribution` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `models-feature-sample-examples` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `not-found` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `notifications` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `playbooks` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `probes` | `main-thread-judgment-missing` | `evidence-ready` | 0.067818 | `business-pixel-accepted` | - |
| `pages` | `rules` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.078346 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `rules-editor-dependencies` | `unresolved-open` | `evidence-ready` | 0.096174 | `not-pixel-accepted` | `record:production-state-mapping:/rules`<br>`verification:production-state-mapping:/rules`<br>`difference:production-state-mapping:/rules`<br>`verification-difference:production-state-mapping:/rules` |
| `pages` | `rules-editor-test-validation` | `unresolved-open` | `evidence-ready` | 0.103706 | `not-pixel-accepted` | `record:production-state-mapping:/rules`<br>`verification:production-state-mapping:/rules`<br>`difference:production-state-mapping:/rules`<br>`verification-difference:production-state-mapping:/rules` |
| `pages` | `rules-sample-logs` | `unresolved-open` | `evidence-ready` | 0.086531 | `not-pixel-accepted` | `record:production-state-mapping:/rules`<br>`verification:production-state-mapping:/rules`<br>`difference:production-state-mapping:/rules`<br>`verification-difference:production-state-mapping:/rules` |
| `pages` | `rules-sample-session` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `screen` | `not-accepted` | `evidence-ready` | 0.091307 | `pixel-accepted` | - |
| `pages` | `settings` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `topics-apt-campaign` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.095487 | `business-pixel-accepted` | - |
| `pages` | `topics-data-exfiltration` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.109829 | `not-pixel-accepted` | - |
| `pages` | `topics-encrypted-tunnel` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.097819 | `not-pixel-accepted` | - |
| `pages` | `whitelist` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.081608 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `whitelist-condition-account` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `whitelist-condition-asset` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `whitelist-condition-ip` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `whitelist-condition-model` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `whitelist-condition-rule` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `whitelist-expiry-expired-unhandled` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `whitelist-expiry-long-lived` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `pages` | `whitelist-expiry-unassigned-owner` | `production-route-evidence-missing` | `evidence-ready` | 0.000000 | `awaiting-visible-chrome-rerun-and-real-auxiliary-review` | - |
| `overlays` | `drawer-probe-detail` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `drawer-probe-log` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `drawer-rule-detail` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `drawer-session-replay` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `drawer-settings-rbac-edit` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `drawer-topic-scope-edit` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `drawer-topic-subscription` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `drawer-whitelist-approval` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `dropdown-alert-batch-actions` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `dropdown-alert-row-actions` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `dropdown-quick-entry` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `dropdown-topic-share-favorite` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `dropdown-user-menu` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-alert-batch` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-alert-feedback` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-alert-status` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-asset-edit` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-audit-export` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-baseline-threshold` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-campaign-report-export` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-compliance-evidence-package-export` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-compliance-report-export` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-data-replay-task` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-deployment-create` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-deployment-rollback` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-evidence-detail` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-forensics-evidence-export` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-forensics-task` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-fusion-rule-edit` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-global-search` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-login-error-captcha` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-notification-channel-edit` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-notification-template-preview-test` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-playbook-edit` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-playbook-trigger` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-probe-batch-upgrade` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-probe-cert-rotate` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-probe-config` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-rule-edit` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-rule-publish` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-screen-readonly-token` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-settings-token` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-topic-evidence-package-export` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-topic-report-export` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-topic-save-view` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-whitelist-add` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `modal-whitelist-draft-from-alert` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `popconfirm-delete` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `popconfirm-pcap-download` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `overlays` | `popconfirm-settings-token-revoke` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
| `states` | `state-api-error` | `auxiliary-agent-review-missing` | `evidence-ready` | 0.000000 | `awaiting-real-auxiliary-review` | - |
