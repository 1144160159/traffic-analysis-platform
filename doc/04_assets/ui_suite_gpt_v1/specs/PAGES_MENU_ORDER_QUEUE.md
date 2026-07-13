# Pages 菜单顺序闭环队列

本队列用于 pages 分类重新闭环：先登录页，再按应用菜单顺序处理主页面和挂靠状态页。`pixel-perfect-breakdown-index.json` 只作为全集来源，不作为执行顺序。

- pages 总数：85
- 队列总数：85
- 未映射队列项：0

| 顺序 | 分组 | 图片 ID | 父页面 | 类型 | 路由 |
|---:|---|---|---|---|---|
| 1 | 登录入口 | `login` | - | `menu-route` | `/login` |
| 2 | 综合态势 | `dashboard` | - | `menu-route` | `/dashboard` |
| 3 | 综合态势 | `screen` | - | `menu-route` | `/screen` |
| 4 | 综合态势 | `topics-encrypted-tunnel` | `topics` | `menu-state` | `/topics` |
| 5 | 综合态势 | `topics-data-exfiltration` | `topics` | `menu-state` | `/topics` |
| 6 | 综合态势 | `topics-apt-campaign` | `topics` | `menu-state` | `/topics` |
| 7 | 采集监测 | `probes` | - | `menu-route` | `/probes` |
| 8 | 采集监测 | `data-quality` | - | `menu-route` | `/data-quality` |
| 9 | 采集监测 | `data-quality-topic-health` | `data-quality` | `menu-state` | `/data-quality` |
| 10 | 采集监测 | `data-quality-flink-quality` | `data-quality` | `menu-state` | `/data-quality` |
| 11 | 采集监测 | `data-quality-field-quality` | `data-quality` | `menu-state` | `/data-quality` |
| 12 | 采集监测 | `data-quality-storage-quality` | `data-quality` | `menu-state` | `/data-quality` |
| 13 | 采集监测 | `data-quality-replay-reconcile` | `data-quality` | `menu-state` | `/data-quality` |
| 14 | 采集监测 | `data-quality-report` | `data-quality` | `menu-state` | `/data-quality` |
| 15 | 采集监测 | `data-quality-settings` | `data-quality` | `menu-state` | `/data-quality` |
| 16 | 威胁分析 | `alerts` | - | `menu-route` | `/alerts` |
| 17 | 威胁分析 | `alert-detail` | `alerts` | `menu-state` | `/alerts` |
| 18 | 威胁分析 | `alert-detail-evidence-files` | `alerts` | `menu-state` | `/alerts` |
| 19 | 威胁分析 | `alert-detail-evidence-graph-path` | `alerts` | `menu-state` | `/alerts` |
| 20 | 威胁分析 | `alert-detail-evidence-logs` | `alerts` | `menu-state` | `/alerts` |
| 21 | 威胁分析 | `alert-detail-evidence-pcap` | `alerts` | `menu-state` | `/alerts` |
| 22 | 威胁分析 | `alert-detail-evidence-session` | `alerts` | `menu-state` | `/alerts` |
| 23 | 威胁分析 | `campaigns` | - | `menu-route` | `/campaigns` |
| 24 | 威胁分析 | `campaign-detail` | `campaigns` | `menu-state` | `/campaigns` |
| 25 | 威胁分析 | `campaign-detail-impact-account` | `campaigns` | `menu-state` | `/campaigns` |
| 26 | 威胁分析 | `campaign-detail-impact-business-system` | `campaigns` | `menu-state` | `/campaigns` |
| 27 | 威胁分析 | `campaign-detail-impact-campus` | `campaigns` | `menu-state` | `/campaigns` |
| 28 | 威胁分析 | `campaign-detail-impact-department` | `campaigns` | `menu-state` | `/campaigns` |
| 29 | 威胁分析 | `campaign-detail-impact-service` | `campaigns` | `menu-state` | `/campaigns` |
| 30 | 威胁分析 | `attack-chains` | - | `menu-route` | `/attack-chains` |
| 31 | 威胁分析 | `encrypted-traffic` | - | `menu-route` | `/encrypted-traffic` |
| 32 | 威胁分析 | `encrypted-traffic-fingerprint` | `encrypted-traffic` | `menu-state` | `/encrypted-traffic` |
| 33 | 威胁分析 | `encrypted-traffic-tunnel-detection` | `encrypted-traffic` | `menu-state` | `/encrypted-traffic` |
| 34 | 威胁分析 | `encrypted-traffic-egress-profile` | `encrypted-traffic` | `menu-state` | `/encrypted-traffic` |
| 35 | 威胁分析 | `encrypted-traffic-evidence-center` | `encrypted-traffic` | `menu-state` | `/encrypted-traffic` |
| 36 | 威胁分析 | `forensics` | - | `menu-route` | `/forensics` |
| 37 | 资产图谱 | `assets` | - | `menu-route` | `/assets` |
| 38 | 资产图谱 | `assets-network-device` | `assets` | `menu-state` | `/assets` |
| 39 | 资产图谱 | `assets-server` | `assets` | `menu-state` | `/assets` |
| 40 | 资产图谱 | `assets-unknown` | `assets` | `menu-state` | `/assets` |
| 41 | 资产图谱 | `assets-business-system` | `assets` | `menu-state` | `/assets` |
| 42 | 资产图谱 | `assets-detail-basic` | `assets` | `menu-state` | `/assets` |
| 43 | 资产图谱 | `assets-detail-network-interface` | `assets` | `menu-state` | `/assets` |
| 44 | 资产图谱 | `assets-detail-open-services` | `assets` | `menu-state` | `/assets` |
| 45 | 资产图谱 | `assets-detail-ownership` | `assets` | `menu-state` | `/assets` |
| 46 | 资产图谱 | `assets-detail-history` | `assets` | `menu-state` | `/assets` |
| 47 | 资产图谱 | `graph` | - | `menu-route` | `/graph` |
| 48 | 资产图谱 | `graph-account-access-path` | `graph` | `menu-state` | `/graph` |
| 49 | 资产图谱 | `graph-attack-path` | `graph` | `menu-state` | `/graph` |
| 50 | 资产图谱 | `graph-communication-path` | `graph` | `menu-state` | `/graph` |
| 51 | 资产图谱 | `fusion` | - | `menu-route` | `/fusion` |
| 52 | 资产图谱 | `baselines` | - | `menu-route` | `/baselines` |
| 53 | 资产图谱 | `baselines-account` | `baselines` | `menu-state` | `/baselines` |
| 54 | 资产图谱 | `baselines-port` | `baselines` | `menu-state` | `/baselines` |
| 55 | 资产图谱 | `baselines-protocol` | `baselines` | `menu-state` | `/baselines` |
| 56 | 资产图谱 | `baselines-time-window` | `baselines` | `menu-state` | `/baselines` |
| 57 | 检测运营 | `rules` | - | `menu-route` | `/rules` |
| 58 | 检测运营 | `rules-editor-dependencies` | `rules` | `menu-state` | `/rules` |
| 59 | 检测运营 | `rules-editor-test-validation` | `rules` | `menu-state` | `/rules` |
| 60 | 检测运营 | `rules-sample-logs` | `rules` | `menu-state` | `/rules` |
| 61 | 检测运营 | `rules-sample-session` | `rules` | `menu-state` | `/rules` |
| 62 | 检测运营 | `deployments` | - | `menu-route` | `/deployments` |
| 63 | 检测运营 | `models` | - | `menu-route` | `/models` |
| 64 | 检测运营 | `models-activation-audit-gate` | `models` | `menu-state` | `/models` |
| 65 | 检测运营 | `models-feature-anomaly-explanation` | `models` | `menu-state` | `/models` |
| 66 | 检测运营 | `models-feature-rule-contribution` | `models` | `menu-state` | `/models` |
| 67 | 检测运营 | `models-feature-sample-examples` | `models` | `menu-state` | `/models` |
| 68 | 检测运营 | `mlops` | - | `menu-route` | `/mlops` |
| 69 | 检测运营 | `playbooks` | - | `menu-route` | `/playbooks` |
| 70 | 检测运营 | `whitelist` | - | `menu-route` | `/whitelist` |
| 71 | 检测运营 | `whitelist-condition-account` | `whitelist` | `menu-state` | `/whitelist` |
| 72 | 检测运营 | `whitelist-condition-asset` | `whitelist` | `menu-state` | `/whitelist` |
| 73 | 检测运营 | `whitelist-condition-ip` | `whitelist` | `menu-state` | `/whitelist` |
| 74 | 检测运营 | `whitelist-condition-model` | `whitelist` | `menu-state` | `/whitelist` |
| 75 | 检测运营 | `whitelist-condition-rule` | `whitelist` | `menu-state` | `/whitelist` |
| 76 | 检测运营 | `whitelist-expiry-expired-unhandled` | `whitelist` | `menu-state` | `/whitelist` |
| 77 | 检测运营 | `whitelist-expiry-long-lived` | `whitelist` | `menu-state` | `/whitelist` |
| 78 | 检测运营 | `whitelist-expiry-unassigned-owner` | `whitelist` | `menu-state` | `/whitelist` |
| 79 | 审计配置 | `compliance` | - | `menu-route` | `/compliance` |
| 80 | 审计配置 | `audit-log` | - | `menu-route` | `/audit-log` |
| 81 | 审计配置 | `audit-log-operation-context` | `audit-log` | `menu-state` | `/audit-log` |
| 82 | 审计配置 | `audit-log-related-chain` | `audit-log` | `menu-state` | `/audit-log` |
| 83 | 审计配置 | `notifications` | - | `menu-route` | `/notifications` |
| 84 | 审计配置 | `settings` | - | `menu-route` | `/settings` |
| 85 | 兜底页面 | `not-found` | - | `menu-route` | `/__codex_visual_not_found__` |
