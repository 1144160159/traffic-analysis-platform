# 知识产权与成果转化索引模板

## 1. 成果映射

| 成果类别 | 名称 | 关联模块 | 证据 | 状态 |
|---|---|---|---|---|
| 软件著作权 | TBD | Web UI / Go control-plane / Probe Agent / Flink jobs | release manifest、代码目录 | 待填写 |
| 专利 | TBD | 全流量采集、PCAP 证据链、反馈学习、模型热更新 | 技术设计、验证报告 | 待填写 |
| 论文 | TBD | 多源融合、未知攻击检测、流式检测 | 数据集、实验、混淆矩阵 | 待填写 |
| 试点证明 | TBD | 全系统 | 用户签认、周报、经济效益 | 待填写 |
| 经济效益 | TBD | 研判、取证、资产、误报治理 | `economic-benefit.md` | 待填写 |

## 2. 模块贡献点

| 模块 | 技术贡献 | 可引用证据 |
|---|---|---|
| Probe Agent | AF_XDP/AF_PACKET/PCAP 采集、解析、归档 | performance preflight、probe code |
| Ingest/Kafka/Flink | 实时链路、DLQ、重放、checkpoint | DLQ evidence、latency chain、Flink health |
| Alert/Forensics | 告警状态机、PCAP hash、presign、审计 | state machine、pcap integrity |
| Rule/MLOps | 规则版本、模型版本、热更新治理 | rule/model state evidence |
| Threat Intel/Fusion | 情报服务、融合冲突处理、租户隔离 | threat intel/fusion evidence |
| Web UI | 设计契约、路由权限、业务闭环页面 | UI contract、route manifest |

## 3. 归档要求

1. 每个成果条目必须绑定版本基线、证据文件和负责人。
2. 论文/专利不得引用客户未脱敏数据。
3. 软件著作权材料必须和 release package 的源代码范围一致。
4. 经济效益和试点证明必须有用户签认或可复核数据来源。
