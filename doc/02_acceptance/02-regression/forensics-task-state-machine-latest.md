# Forensics task state-machine live 预检

- Run ID：`20260716082451-1013412`
- 结果：`pass`
- APISIX：`http://10.0.5.8:30180`
- 取消任务：`b997fa93-9166-4eda-bf2f-a5bfb9733509`
- 检查数：7/7 passed，blockers=0，warnings=0

## 证据

- NDJSON：`doc/02_acceptance/runs/20260629-forensics-task-state-machine/live-forensics-task-state-machine-20260716082451-1013412.ndjson`
- Summary：`doc/02_acceptance/runs/20260629-forensics-task-state-machine/live-forensics-task-state-machine-20260716082451-1013412-summary.json`
- API/DB/Audit 响应：`doc/02_acceptance/runs/20260629-forensics-task-state-machine/20260716082451-1013412-*.json`、`doc/02_acceptance/runs/20260629-forensics-task-state-machine/20260716082451-1013412-*.txt`

## 口径

本报告验证取证任务取消状态机：processing 任务可取消并持久化为 cancelled，completed 任务取消返回 409，跨租户取消返回 403 且原任务状态不变，成功取消同步写入 `audit_logs` 的 `PCAP_CANCEL` 事件。
