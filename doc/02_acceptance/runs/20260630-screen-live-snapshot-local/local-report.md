# /screen 真实快照联动本地证据

- Run ID: `20260630-screen-live-snapshot-local`
- 范围: Web UI `/screen` 态势大屏
- 结果: local frontend pass；Desktop Chrome bridge 仍为 `Transport closed`

## 改动

- `web/ui/src/services/pageSnapshotAdapters.ts` 新增 `screen` 专用 adapter，把 `/v1/dashboard/stats`、`/v1/dashboard/encrypted/trend`、`/v1/dashboard/attack-phases` 转换为大屏可消费的楼宇覆盖、探针在线、采集吞吐、协议解析率、Kafka 积压、Flink P95、证据完整度、高危告警、攻击阶段和闭环动作指标。
- `web/ui/src/pages/SituationalScreen.tsx` 不再只用真实 API 返回行数判断状态，覆盖率卡片、采集与流处理管道、PCAP 证据环、响应动作和风险计数会消费 `screen` snapshot metrics/evidence。
- `web/ui/src/services/pageSnapshotAdapters.test.ts` 新增 screen adapter 回归用例，锁定全流量处理链路、攻击阶段、响应动作和证据字段映射。

## 验证

```bash
npm --prefix web/ui run test -- --run src/services/pageSnapshotAdapters.test.ts src/routes/routeManifest.test.ts
```

结果: 2 个测试文件通过，32/32 tests passed。

```bash
npm --prefix web/ui run build
```

结果: TypeScript 与 Vite production build 通过；仅保留既有 chunk size warning。

```bash
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
```

结果: validated UI frontend contracts: 181 manifest items, 28 route contracts, 70 overlay contracts; errors: 0, warnings: 0。

```text
mcp__codex_desktop_node_repl.desktop_chrome_open_url(
  url="http://10.0.5.8:30180/login",
  keep=true,
  wait_ms=2500
)
```

结果: `Transport closed`。该项为 Codex Desktop Chrome bridge 运行时阻塞，不代表 `/screen` 页面代码或 UI 契约失败。

## 边界

本证据证明 `/screen` 已从静态展示推进为消费真实 dashboard API snapshot 的一屏闭环视图；它不替代 4K/2K/1080p 浏览器视觉巡检，也不关闭 Desktop Chrome bridge blocker。
