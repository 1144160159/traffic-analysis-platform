# GPT ImageGen Prompt: 综合态势-态势大屏

Use case: ui-mockup
Asset type: enterprise SOC dashboard visual reference
Input image: use the uploaded screenshot in this conversation as the visual style reference. It is a style and density reference, not a pixel-edit target.

Primary request:
Generate one complete UI visual effect image for the “综合态势 / 态势大屏” page of “园区网络全流量采集与分析系统”.

Required page:
- Product title: 园区网络全流量采集与分析系统
- Primary menu active: 综合态势
- Secondary menu active: 态势大屏
- Route context: /screen

Navigation requirements:
- The left sidebar must use the finalized six primary menus: 综合态势、采集监测、威胁分析、资产图谱、检测运营、审计配置.
- Under 综合态势, show secondary menu items: 仪表盘、态势大屏、专题面板, with 态势大屏 highlighted.
- Do not use 看见、研判、取证、治理、验收 as navigation labels.

Business logic requirements:
This status screen must clearly tell the full-traffic operational story:
1. 园区网络拓扑与数字孪生: show campus buildings, core links, access links, aggregation links, and key areas such as 教学区、办公区、图书馆、实验楼、宿舍区、体育馆.
2. 采集监测: show probe coverage, online probes 24/25, endpoint count 18,642, key assets 2,317, collection traffic 78.3 Gbps, packet/session EPS, drop rate 0.02%.
3. 流处理链路: show 探针采集 -> Kafka 集群 -> Flink 集群 -> ClickHouse/OpenSearch/NebulaGraph/MinIO storage, each with health status.
4. 威胁态势: show high risk 128, medium risk 382, low risk 1,256, key alerts 9, risk trend, threat map, attack stage distribution, top risky assets.
5. 资产与流量态势: show top talkers, east-west traffic, north-south traffic, protocol distribution TLS/HTTP/DNS/QUIC, asset risk heatmap.
6. 处置与验收闭环: show response SLA, processed alerts, evidence completeness, model feedback status, acceptance evidence pass rate.

Visual style:
- Match the uploaded reference image closely: deep navy SOC command center, compact enterprise dashboard, cyan-blue panel borders, low-saturation dark panels, fine grid dividers, restrained glow, blue data-flow lines, green healthy states, amber medium warnings, red high-risk alerts.
- Use dense but readable enterprise data panels, Ant Design + ECharts compatible, engineering system feel.
- Include a top KPI/status bar and a complete bottom status strip.
- Keep Chinese-only panel titles. Do not render bilingual titles like “中文 / English”.
- Technical terms may remain English: Kafka, Flink, ClickHouse, OpenSearch, NebulaGraph, MinIO, PCAP, TLS, DNS, IP, JA3, MLOps, SOAR, Gbps, K EPS.
- Use consistent title sizes: product title large, panel titles uniform, table/chart text readable.

Composition:
- Widescreen desktop dashboard, no browser chrome, no phone mockup, no marketing hero.
- Keep the left two-level navigation visible, unlike a pure kiosk display.
- Center the page around campus topology plus collection/streaming health, not around a selected alert detail panel.
- The page should feel like the first screen a duty officer or project reviewer sees to understand whether the campus network full-traffic system is healthy and whether threats are under control.

Avoid:
- Do not copy the reference page content as a threat-analysis detail page.
- Do not make the active menu 威胁分析.
- Do not omit the bottom status bar.
- Do not use decorative gradient orbs, stock illustrations, oversized hero text, or card-in-card clutter.
- Do not make text unreadably tiny or randomly change title sizes.
