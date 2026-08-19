# Data Contract Sketch: CLE-P0-SCREEN-001

- run_id: `mvp-3-design-screen`
- task: /screen 只读 token 或脱敏公开边界
- priority: `P0`
- status: `DISCOVERED`
- primary_lane: `UI Rebuild`
- dependent_lanes: Deploy / SRE / Security, Product Design
- acceptance_type: `regression`

## Data Plan
- mode: `live_existing`
- tenant: `default`
- cleanup: `none`

## Data Rules
- Prefer real API/DB/Kafka paths for verification; mock data cannot prove live integration.
- Generated live data requires run_id, tenant scoping and cleanup before execution.
- Sensitive data policy must be explicit for screenshots, browser reports and acceptance artifacts.
- `/screen` may aggregate live operational metrics, but public/demo views must be desensitized.
- Display-wall tokens must not expose raw PCAP, user identity, secrets, high-cardinality logs or tenant-crossing data.
- Screenshot evidence must avoid leaking secrets or credentials.
