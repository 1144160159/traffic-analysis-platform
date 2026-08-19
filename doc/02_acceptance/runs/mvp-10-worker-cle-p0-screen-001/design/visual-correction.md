# Frontend Visual Correction: CLE-P0-SCREEN-001

- run_id: `mvp-10-worker-cle-p0-screen-001`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

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
