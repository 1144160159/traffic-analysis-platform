# Forensics task state-machine live 预检

- Run ID：`20260629-forensics-task-state-machine-r1`
- 结果：`pass`
- APISIX：`http://10.0.5.8:30180`
- 取消任务：`fa60e6c6-ce27-41cb-884f-c1e06290f066`
- 检查数：7/7 passed，blockers=0，warnings=0

## 证据

- NDJSON：`doc/02_acceptance/runs/20260629-forensics-task-state-machine/live-forensics-task-state-machine-20260629-forensics-task-state-machine-r1.ndjson`
- Summary：`doc/02_acceptance/runs/20260629-forensics-task-state-machine/live-forensics-task-state-machine-20260629-forensics-task-state-machine-r1-summary.json`
- API/DB/Audit 响应：`doc/02_acceptance/runs/20260629-forensics-task-state-machine/20260629-forensics-task-state-machine-r1-*.json`、`doc/02_acceptance/runs/20260629-forensics-task-state-machine/20260629-forensics-task-state-machine-r1-*.txt`

## 口径

本报告验证取证任务取消状态机：processing 任务可取消并持久化为 cancelled，completed 任务取消返回 409，跨租户取消返回 403 且原任务状态不变，成功取消同步写入 `audit_logs` 的 `PCAP_CANCEL` 事件。
