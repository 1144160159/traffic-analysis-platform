# PCAP 法证完整性闭环报告

日期：2026-06-29

## 变更

- `forensics-service` 异步裁剪结果新增 `result_sha256` 持久化，任务响应暴露 `sha256`。
- `/api/v1/pcap/download/{key}` 返回 `X-Content-SHA256`，并把下载审计同步写入 `audit_logs`。
- `/api/v1/pcap/presign` 与下载统一限制为 `results/{tenant}/...`，校验对象存在、限制最长 86400 秒，并写入 presign 审计。
- 新增 `/api/v1/pcap/verify`，对对象流式计算 SHA-256，并与请求值或任务登记值比对。
- 新增 `tests/e2e/live_pcap_forensics_integrity.sh`，固化 live 回归矩阵。

## 部署

- DB：`ALTER TABLE tasks ADD COLUMN IF NOT EXISTS result_sha256 TEXT NOT NULL DEFAULT ''` 已在 live PostgreSQL 执行。
- 镜像：`traffic/forensics-service:pcap-integrity-20260629-r1` 已导入 10.0.5.8 与 10.0.5.9 的 containerd。
- K8s：`forensics-service` 已滚动到 `traffic/forensics-service:pcap-integrity-20260629-r1` 并 Ready。

## 验证

- Go：`go test ./internal/forensics/... ./cmd/forensics-service` 通过。
- Live API：`tests/e2e/live_pcap_forensics_integrity.sh` 9/9 通过。
- 覆盖项：jobs 返回 hash、presign 暴露 hash 且过期上限被截断、verify hash 匹配、路径遍历 400、跨租户 presign 403、download body SHA 与 `X-Content-SHA256` 一致、`/audit/logs` 可查 `presign` / `integrity_verify` / `download` 三类 PCAP 审计。
- Desktop Chrome：`desktop-chrome-forensics-anonymous.json` 证明匿名 `/forensics` 重定向 `/login`，页面含登录与验证码。
- Playwright：`playwright-forensics-auth.json` 证明合法 `pcap:read` 登录态打开 `/forensics`，无 console error、pageerror、requestfailed 或 5xx。

## 证据文件

- `live-pcap-forensics-integrity-20260629-pcap-forensics-integrity-summary.json`
- `live-pcap-forensics-integrity-20260629-pcap-forensics-integrity.ndjson`
- `20260629-pcap-forensics-integrity-audit.json`
- `desktop-chrome-forensics-anonymous.json`
- `playwright-forensics-auth.json`
