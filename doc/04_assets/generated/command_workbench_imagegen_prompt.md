# 园区网络全流量采集与分析系统 ImageGen Prompt

更新时间：2026-06-19

用途：基于用户确认的“1+2 混合视觉”，复用 GPT 画图功能生成前端主视觉概念图。公开产品标题统一为“园区网络全流量采集与分析系统”；Command Workbench 仅作为内部视觉方向，不作为画面主标题。

```text
Use case: ui-mockup
Asset type: archived product UI visual direction mockup for a React + Ant Design + ECharts campus network security analytics platform
Primary request: Create one realistic, production-quality widescreen UI mockup, 16:9 composition, for a campus network full-traffic collection and analysis system.

Product title requirement: the main screen title must be 园区网络全流量采集与分析系统. Do not use 指挥研判平台 as the product title. Do not display Command Workbench as the product title; Command Workbench is only an internal design direction.

Critical navigation requirement: the left sidebar MUST have two visible levels. Level 1: a narrow vertical grouped navigation rail with exactly these five business-domain labels, readable Chinese text: 综合态势, 威胁分析, 资产图谱, 检测运营, 审计配置. Level 2: an expanded submenu panel next to the rail for the active group. Set 威胁分析 as active, and visibly list these second-level items: 告警中心, 战役列表, 攻击链分析, 加密流量, 取证分析. Highlight 告警中心 as the current page. Do not omit the second-level menu. Do not collapse it into icons only.

Reference context: Use the dark campus network blueprint image as moodboard inspiration for the enterprise cyber command atmosphere, campus digital twin, flow arrows, pipeline, evidence, and response-loop language. Do not copy it literally; redesign it as a cleaner usable product screen.

Workflow to communicate visually: the user should understand the closed loop from full traffic collection to alert investigation, PCAP evidence, response action, feedback learning, and acceptance evidence. Do not render the workflow verbs as the left first-level navigation labels.

Layout: Full-screen enterprise app shell. Left: two-level navigation as specified. Top: title, site/time/risk/collection health bar. Main center: campus topology/digital twin plus collection and streaming pipeline health, including probes, Kafka, Flink, ClickHouse, OpenSearch, NebulaGraph, MinIO. Investigation zone: alert queue, selected alert timeline, correlated alert cluster, traffic flow chart, and PCAP/session/log evidence table. Right fixed action/evidence rail: selected alert summary, asset context, risk score, response actions, feedback labeling, model retraining status, and acceptance evidence status.

Must show these product modules: campus topology, probe health, collection pipeline, Kafka/Flink stream processing status, data lake/evidence storage, alert queue, selected alert timeline, PCAP evidence, asset context, topology graph, response actions, feedback learning, model status, data quality/acceptance gate.

Style/medium: polished dark enterprise cybersecurity SaaS UI, Ant Design and ECharts compatible, readable Chinese UI labels with small English subtitles only where helpful. Dense but orderly, suitable for SOC operator and project acceptance demo.

Visual system: deep charcoal background, subtle blue-gray surfaces, semantic cyan for data flow, green for healthy/closed-loop, amber for medium risk, red for high risk. Use restrained glow only for active flows. 8px or smaller border radius. No browser chrome, no device frame, no marketing hero, no stock photos, no decorative orbs, no card-inside-card nesting, no unreadably tiny text, no fake lorem ipsum. Every panel should have a clear operational purpose.

Typography: product-readable sizes, strong section hierarchy, no oversized hero type. Text must fit inside controls. Use crisp icons where appropriate. Make the screen feel like a production control surface, not a poster.
```
