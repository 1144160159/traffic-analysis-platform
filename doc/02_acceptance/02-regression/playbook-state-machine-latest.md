# Playbook 状态机 live 预检

- Run ID：`20260629-playbook-state-machine-r1`
- 结果：`pass`
- APISIX：`http://10.0.5.8:30180`
- 目标剧本：`log-lateral-movement`
- 检查数：21/21 passed，blockers=0，warnings=0

## 证据

- NDJSON：`doc/02_acceptance/runs/20260629-playbook-state-machine/live-playbook-state-machine-20260629-playbook-state-machine-r1.ndjson`
- Summary：`doc/02_acceptance/runs/20260629-playbook-state-machine/live-playbook-state-machine-20260629-playbook-state-machine-r1-summary.json`
- 截图：`doc/02_acceptance/runs/20260629-playbook-state-machine/live-playbook-state-machine-20260629-playbook-state-machine-r1.png`
- API 执行：`playbook-first-execute-response.json`、`playbook-second-execute-response.json`
- PostgreSQL：`playbook-pg-row-count.txt`

## 口径

本报告验证 SOAR 剧本目录、禁用门禁、手动执行 max_runs 状态机、执行记录落库、执行历史 API 和 /playbooks 前端消费链路。脚本会临时 PATCH `log-lateral-movement` 并在退出时恢复原 enabled/max_runs/cooldown_seconds 配置。
