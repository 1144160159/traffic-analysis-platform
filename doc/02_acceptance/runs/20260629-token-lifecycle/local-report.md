# Token Lifecycle Matrix Report

日期：2026-06-29
对象：auth-service API token 生命周期、租户边界、撤销/过期和审计闭环。

## 变更

- 将 `api_tokens.token_hash` 的 service 侧生成/验证统一为 SHA-256 表契约，匹配 ingest PG token validator 和现有 live e2e 临时 token 写入方式。
- 创建 API token 时写入安全展示用 `token_prefix`，响应不暴露 hash。
- `create_token`、`regenerate_token`、`revoke_token` 成功/失败路径同步写入 `audit_logs`，保留既有 Kafka audit logger。
- 新增 `tests/e2e/live_token_lifecycle_matrix.sh`，不把明文 token 写入证据目录。
- auth-service live 镜像滚动到 `traffic/auth-service:token-lifecycle-20260629-r2`。

## 验证

```text
go test ./internal/auth/...
RUN_ID=20260629-token-lifecycle-r2 LOG_DIR=doc/02_acceptance/runs/20260629-token-lifecycle tests/e2e/live_token_lifecycle_matrix.sh
RUN_ID=20260629-token-lifecycle-permission-r1 LOG_DIR=doc/02_acceptance/runs/20260629-token-lifecycle tests/e2e/live_permission_matrix.sh
RUN_ID=20260629-release-manifest-r9 tests/e2e/live_release_manifest.sh
```

## 结果

| 验证项 | 结果 |
|---|---:|
| token lifecycle matrix | 23/23 passed |
| permission matrix regression | 11/11 passed |
| release manifest | 12/12 passed |

关键断言：

- 创建后 raw token 可通过 `/api/v1/tokens/validate` 返回 200。
- PostgreSQL `api_tokens` 中 hash/prefix/scopes/status 与响应匹配。
- viewer 创建 token 返回 403。
- tenant-b admin 读取 tenant default token 返回 404。
- regenerate 后旧 raw token 返回 401，新 raw token 返回 200。
- revoke 后新 raw token 返回 401。
- 1 秒短期 token 初始 200，过期后 401。
- `audit_logs` 存在 `create_token`、`regenerate_token`、`revoke_token` 行。

