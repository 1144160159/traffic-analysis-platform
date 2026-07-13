# 性能验收证据

本目录承载 GATE-P0-03（10 x 100Gbps）和 GATE-P0-04（512Mpps）的稳定证据副本。功能 smoke、离线 PCAP 小压测、P95 延迟报告和架构能力说明都不能替代这里的专项性能验收。

## 当前入口

- 验收包：`tests/perf/100g_capture/`
- 预检脚本：`tests/perf/100g_capture/live_capture_performance_preflight.sh`
- 草案工具：`tests/perf/100g_capture/live_capture_performance_package_bootstrap.sh`
- 复核包工具：`tests/perf/100g_capture/live_capture_performance_review_packet.sh`
- 执行计划：`tests/perf/100g_capture/capture-performance-plan.yaml`

## 稳定证据

运行预检后会刷新：

- `capture-performance-preflight-latest.json`
- `capture-performance-preflight-latest.md`
- `capture-performance-plan.yaml`
- `capture-performance-result-schema.json`
- `hardware-inventory.template.yaml`
- `traffic-profile.template.yaml`
- `repo-stress-500k-summary-latest.json`
- `live-probe-capture-profile-latest.json`
- `live-node-summary-latest.json`
- `bootstrap/capture-performance-bootstrap-latest.json`
- `bootstrap/capture-performance-bootstrap-latest.md`
- `bootstrap/latest/`
- `review/capture-performance-review-latest.json`
- `review/capture-performance-review-latest.md`
- `review/latest/`

如果缺少真实 `hardware-inventory.yaml`、`traffic-profile.yaml`、`results/10x100g-summary.json` 或 `results/512mpps-summary.json`，结果必须保持 `blocked`。

`bootstrap/latest/` 只用于准备人工复核和硬件窗口材料，文件名保持 `*.bootstrap.*` / `*.review-template.*`。它不能替代正式 `tests/perf/100g_capture/` 下的硬件清单、流量 profile 或结果 summary。

`review/latest/` 是从 bootstrap 草案派生的硬件窗口复核工作包，只包含 operator review worklist、manifest template、approval template 和 checklist。它不写入正式 `tests/perf/100g_capture/` artifact，不能替代 GATE-P0-03/04。

## Review Packet

`20260701-capture-performance-review-r1` 为 `pass`：2 个目标、7 个复核文件、0 个正式 artifact、6/6 checks passed、0 blockers、0 warnings。稳定入口为 `review/capture-performance-review-latest.json` / `.md`，包目录为 `review/latest/`。

生成文件包括 `hardware-review.csv`、`traffic-profile-review.csv`、`result-summary-worklist.csv`、`formal-artifact-manifest.template.json`、`operator-approval.template.md`、`review-checklist.md` 和 `review-summary.json`。这些文件用于硬件窗口前的人工分派和签核，不会让正式预检通过。

## 最新正式预检

`20260701-capture-performance-preflight-r4-review-packet` 仍为 `blocked`：11/18 checks passed、4 blockers、3 warnings。当前阻断项仍是缺正式 `hardware-inventory.yaml`、`traffic-profile.yaml`、`results/10x100g-summary.json` 和 `results/512mpps-summary.json`；live probe 仍是 `af_packet`/2 cores 小 profile，repo 500k stress 仅作上下文，不能替代 GATE-P0-03/04。

`tests/perf/100g_capture/live_capture_performance_preflight.sh` 已加入正式包完整性 guard：所有正式性能 artifact 会扫描 bootstrap/review-template 标记。本轮已用临时合成包把 bootstrap/template 文件改名成正式路径验证 guard 会阻断，避免草案材料被误当作 10 x 100Gbps 或 512Mpps 正式结果。
