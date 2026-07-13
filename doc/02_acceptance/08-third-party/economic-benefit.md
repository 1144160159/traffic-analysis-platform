# 经济效益测算模板

## 1. 测算口径

本模板用于将试点运行数据转化为可审核的经济效益证明。所有数值必须来自试点周报、验收证据或用户确认，不用演示样例替代真实收益。

## 2. 输入参数

| 参数 | 符号 | 单位 | 填写 | 数据来源 |
|---|---|---|---:|---|
| 安全分析人员平均小时成本 | C_h | 元/小时 | TBD | 用户财务或项目约定 |
| 试点前平均研判耗时 | T_before | 小时/告警 | TBD | 历史工单或访谈 |
| 试点后平均研判耗时 | T_after | 小时/告警 | TBD | 试点周报 |
| 月均有效告警数 | A_valid | 条/月 | TBD | 告警状态机证据 |
| 试点前误报率 | FPR_before | % | TBD | 用户历史/第三方基线 |
| 试点后误报率 | FPR_after | % | TBD | 反馈闭环/检测质量报告 |
| 单次取证平均耗时下降 | T_pcap_saved | 小时/次 | TBD | PCAP 取证任务证据 |
| 月均取证次数 | N_pcap | 次/月 | TBD | Forensics evidence |
| 资产盘点人工节省 | H_asset_saved | 小时/月 | TBD | 资产覆盖报告 |

## 3. 计算公式

| 收益项 | 公式 |
|---|---|
| 研判效率收益 | `(T_before - T_after) * A_valid * C_h` |
| 误报减少收益 | `(FPR_before - FPR_after) * A_valid * T_after * C_h` |
| PCAP 取证效率收益 | `T_pcap_saved * N_pcap * C_h` |
| 资产盘点效率收益 | `H_asset_saved * C_h` |
| 月度直接人力收益 | 上述四项之和 |
| 年化直接人力收益 | `月度直接人力收益 * 12` |

## 4. 试点收益填报

| 收益项 | 月度收益 | 年化收益 | 证据 |
|---|---:|---:|---|
| 研判效率收益 | TBD | TBD | 告警状态机、审计、周报 |
| 误报减少收益 | TBD | TBD | 反馈闭环、检测质量报告 |
| PCAP 取证效率收益 | TBD | TBD | PCAP integrity、Forensics state |
| 资产盘点效率收益 | TBD | TBD | asset/graph evidence |
| 合计 | TBD | TBD | 用户确认 |

## 5. 非直接收益

| 类别 | 说明 | 证据 |
|---|---|---|
| 风险发现提前量 | 多源融合或实时链路提前发现异常 | Fusion/alert/latency evidence |
| 法证可信度提升 | hash、presign、审计、跨租户拒绝 | PCAP integrity |
| 合规支撑 | 可追溯审计和证据包 | audit/release evidence |
| 交付可复现 | release manifest 和 deployment preflight | deployment evidence |

## 6. 结论模板

> 在 YYYY-MM-DD 至 YYYY-MM-DD 的试点周期内，系统基于真实流量和真实业务闭环，将平均研判耗时从 `T_before` 小时降低至 `T_after` 小时，PCAP 取证平均节省 `T_pcap_saved` 小时/次，按用户确认的人力成本 `C_h` 元/小时估算，年化直接人力收益为 `TBD` 元。该结论仅覆盖本试点时间窗和已签认证据，不外推为第三方检测质量或生产安全结论。
