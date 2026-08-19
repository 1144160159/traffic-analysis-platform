# Frontend Visual Correction: CLE-P0-P95-001

- run_id: `20260624-loop-prepare-002-workflow`
- task: 完整 P95 时间戳链设计与埋点计划
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `Go Control-plane`
- dependent_lanes: Proto / Kafka / Flink, UI Rebuild, Mission / Acceptance
- acceptance_type: `acceptance-prep`

## Visual Source Of Truth
- `doc/01_design/面向园区网络的全流量采集分析系统-UI前端规范.md`
- `doc/01_design/面向园区网络的全流量采集分析系统-UI设计套装.md`
- `doc/01_design/面向园区网络的全流量采集分析系统-左侧菜单信息架构.md`
- `doc/01_design/面向园区网络的全流量采集分析系统-Tab页功能点与表现形式矩阵.md`
- `doc/04_assets/ui_suite_gpt_v1/README.md`

## Correction Rules
- Use the dark security-operations visual token system from the UI frontend specification.
- Keep the product title and six primary business domains aligned with the documented UI suite.
- Do not add a third-level left menu; do not turn second-level navigation into large cards or topic blocks.
- Keep `/screen`, `/dashboard` and topic/workbench pages differentiated by business purpose, not by random styling.
- For `/screen`, prioritize one-screen closure: campus topology, collection pipeline, threat posture, evidence integrity, response feedback and runtime base.
- Use real API states for loading/error/empty/degraded; visual success must not be backed by hidden mock data in production mode.
- Before broad UI rebuild, run the backup task `CLE-P0-UIBACKUP-001` or explicitly record why it is not needed.

## Visual QA Cases
- 1920x1080 screen baseline does not overlap text or navigation.
- 2K/4K display-wall scaling preserves information hierarchy.
- Unauthorized and read-only states are visibly different from normal live operation.
- Console/pageerror/requestfailed criteria remain clean during browser smoke when implementation occurs.
