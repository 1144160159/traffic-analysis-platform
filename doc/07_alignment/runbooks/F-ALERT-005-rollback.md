# F-ALERT-005 告警证据链回滚

## 适用范围

本手册只回滚默认关闭的 `ALERT_EVIDENCE_CHAIN_V1_ENABLED` 严格读取路径。PostgreSQL `alert_evidence_manifests`、不可变历史、告警证据、MinIO 对象、访问审计和事件身份均属于保留事实，不得因回滚删除、覆盖或重新编号。

## 切换前提

1. expand migration `202608091700` 已完成双重回放并核对结构摘要。
2. shadow 清单已按 tenant、alert、evidence、event、revision、object version、size 和 SHA-256 与 ClickHouse/MinIO 对账。
3. 严格路径只对已批准租户 canary 开启；旧 GET、页面路由和 `alert:read` 权限保持兼容。
4. 发布记录绑定源码哈希、镜像摘要、effective config hash、清单水位和回滚责任人。

## 回滚触发条件

- 清单缺失、跨租户引用、对象版本或 SHA-256 不一致超过批准预算；
- 证据列表 partial 比例、授权 P95/P99 或 MinIO 读取放大超过预算；
- Windows Chrome 出现错误态不可恢复、下载授权跨会话失效或旧客户端回归；
- 审计写入、权限或签名验证出现任何 fail-open 行为。

## 执行动作

1. 将 canary workload 的 `ALERT_EVIDENCE_CHAIN_V1_ENABLED` 置为 `false`，记录 effective config hash，并逐批恢复到兼容读取路径。
2. 停止新增严格清单投影，但保留已经提交的 manifest/history；继续记录 ClickHouse 与 MinIO 漂移，禁止以 fixture 填补缺失证据。
3. 撤销尚未过期的严格下载授权应通过轮换专用签名密钥完成；不得复用旧签名生成新的对象身份。
4. 保持对象版本、Object Lock、保留期和法律保全策略；任何对象删除必须进入 MinIO 独立删除协议。
5. 复核跨租户访问、过期链接、manifest revision 变化、size/hash mismatch 均继续 fail closed，并保存失败审计。

## 回滚验证

- 原有 `/alerts`、`/alerts/:alertId`、证据读取与下载控件仍可用；无演示数据或伪成功回退。
- PostgreSQL current/history 行数不减少，事件 ID、object key/version/SHA-256 不变。
- MinIO 对象数与版本数不因回滚变化；审计可按同一 trace 回放授权、拒绝和下载结果。
- 账本保持 `IMPLEMENTING`，直至 G2—G6 和独立裁决完成；回滚成功不等于 canonical 关闭。
