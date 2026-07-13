# 试点演示脚本模板

## 1. 演示边界

本脚本用于试点、售前和验收走查。演示必须基于真实 APISIX/API/DB/Kafka/Flink 或已归档的 evidence package，不用 mock 截图替代后端证据。

当前推荐入口：

| 页面 | 目标 | 证据来源 |
|---|---|---|
| `/login` | 登录门禁和产品入口 | UI contract / Desktop Chrome smoke |
| `/dashboard` | 脱敏运营工作台 | business flow API / realtime appshell |
| `/alerts` | 告警研判和反馈闭环 | alert state machine / feedback whitelist |
| `/forensics` | PCAP 取证和任务状态 | pcap integrity / forensics task state |
| `/graph` | 资产图谱和路径探索 | business flow API |
| `/data-quality` | DLQ、迟到、重放和链路质量 | DLQ recovery / latency chain |
| `/models` | 模型版本注册、激活和弃用 | model version state machine |
| `/deployments` | 灰度、激活、暂停、恢复、回滚 | deployment state machine |
| `/audit-log` | 高敏操作审计可查 | audit rows / audit API |

## 2. 演示主线

### Step 1：看见整体态势

| 动作 | 讲述要点 | 后端证据 |
|---|---|---|
| 打开 `/dashboard` | 今日待处理、高危、采集健康、数据质量、实时通道 | business flow API latest、realtime appshell |
| 说明数据来源 | Probe -> Kafka -> Flink -> CH/PG/OS/MinIO -> API -> UI | `agent.md` 主链路、release manifest |

### Step 2：告警研判

| 动作 | 讲述要点 | 后端证据 |
|---|---|---|
| 进入 `/alerts` | 告警队列、严重级别、实时通道 | alert state machine |
| 选择一条告警 | 关联资产、证据、时间线、处置建议 | business flow API / PCAP evidence |
| 状态迁移 | `new -> triage/assigned -> closed`，含乐观锁和审计 | alert state evidence |

### Step 3：PCAP 取证

| 动作 | 讲述要点 | 后端证据 |
|---|---|---|
| 从告警进入 `/forensics` | 取证任务不是静态下载，而是任务化和可审计 | forensics state machine |
| 展示 hash/verify | 文件完整性和跨租户拒绝 | PCAP integrity |
| 展示 presign 有效期 | 降低证据泄露风险 | PCAP integrity |

### Step 4：图谱关联

| 动作 | 讲述要点 | 后端证据 |
|---|---|---|
| 进入 `/graph` | 从告警扩展到资产、会话、路径 | business flow API |
| 展示层级和路径限制 | 防止重查询和信息过载 | graph API / route manifest |

### Step 5：反馈学习

| 动作 | 讲述要点 | 后端证据 |
|---|---|---|
| 提交误报反馈 | FP 原因码可生成白名单草案 | alert feedback whitelist |
| 展示规则/模型治理 | 规则启停、模型激活/弃用都有状态机和审计 | rule/model state evidence |

### Step 6：数据质量和恢复

| 动作 | 讲述要点 | 后端证据 |
|---|---|---|
| 进入 `/data-quality` | 迟到、DLQ、链路延迟和恢复建议 | latency chain / DLQ recovery |
| 展示 DLQ 重放 | dry-run、幂等、partial failure 保留 | DLQ recovery evidence |

### Step 7：验收和审计

| 动作 | 讲述要点 | 后端证据 |
|---|---|---|
| 打开 `/audit-log` | 高敏操作可追溯 | audit API / PG rows |
| 展示 release/deployment preflight | 可复现部署和证据包 | release/deployment latest |
| 明确未验收项 | 不把预检或模板写成第三方通过 | status docs |

## 3. 演示检查单

| 检查项 | 要求 | 结果 |
|---|---|---|
| APISIX/API 无 4xx/5xx | 真实业务请求通过 | pass：business flow API r26 为 46/46 |
| 浏览器无 `requestfailed` | Desktop Chrome 或 Playwright 证据 | blocked：Desktop Chrome bridge r12 为 `Transport closed` |
| 无非 warning console/pageerror | 浏览器巡检证据 | blocked：Desktop Chrome bridge 未能打开页面，需恢复后重跑 |
| 所有演示步骤有证据链接 | 每步至少一个证据文件 | partial：内部证据已列出，浏览器和外部签认仍待补 |
| 所有 Gap 明确披露 | 不夸大 P0 专项状态 | pass：completion audit r24 仍保留 8 个 blocker |

## 4. 现场话术红线

- 可以说：主体链路和多个高风险状态机已有真实链路验证。
- 可以说：试点模板包已建立，可随现场数据填充。
- 不能说：已通过第三方测试，除非 `cnas-test-report.pdf` 或等效报告已归档。
- 不能说：100G/512Mpps、95%/5%、生产安全、HA 已完成，除非对应专项从 `blocked` 变为 `pass` 且有签认。
