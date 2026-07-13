# 安全开发规范

基于 OWASP Top 10、CWE Top 25。

## 1. 凭证管理

```
禁止:
  ✗ 代码中硬编码密码/Token/密钥
  ✗ 提交 .env / credentials.json
  ✗ 日志中打印密码/Token
  ✗ 生产环境使用默认密码

必须:
  ✓ K8s Secret 注入敏感配置
  ✓ Keycloak + Cert-Manager 管理认证
  ✓ 日志脱敏 (password → ***)
  ✓ API Token 设置过期时间
```

## 2. 传输安全

```
Ingest Gateway: gRPC + mTLS 双向证书
APISIX → 后端: TLS + JWT 验证
Kafka: SASL/SCRAM (生产)
DB/Redis: K8s 内部网络, 不对外暴露
MinIO: TLS + IAM Policy
```

## 3. 输入校验

```go
// 所有外部输入必须校验
func (s *AlertService) ListAlerts(ctx context.Context, req *pb.ListAlertsRequest) (*pb.ListAlertsResponse, error) {
    if req.TenantId == "" {
        return nil, errors.NewInvalidArgument("tenant_id is required")
    }
    if req.PageSize > 100 {
        req.PageSize = 100  // 限制
    }
    // ...
}
```

## 4. SQL 注入防护

```go
// 参数化查询
db.Query("SELECT * FROM alerts WHERE tenant_id = $1 AND id = $2", tenantID, alertID)  // ✓
db.Query(fmt.Sprintf("SELECT * FROM alerts WHERE tenant_id = '%s'", tenantID))         // ✗
```

## 5. 多租户隔离

```
全链路 tenant_id:
  Kafka: key = tenant_id + community_id
  ClickHouse: ORDER BY (tenant_id, timestamp)
  OpenSearch: 索引按 tenant 分片
  MinIO: /{tenant}/{date}/{hour}/{probe_id}/
  API: JWT 提取 tenant_id → 注入 context → 所有查询过滤
```

## 6. 依赖安全

```bash
go list -m -u all | grep '\[.*\]'     # Go 过期依赖
cargo audit                             # Rust 漏洞检查
mvn dependency-check:check              # Java OWASP 检查
npm audit                               # JS 漏洞检查
```
