# T1-M10-N003 Kubernetes site values 合同与回滚

严格合同位于 `deployments/kubernetes/site-values.v1.template.yaml`，旧 `site-values.template.yaml` 作为历史 preflight 兼容输入保留。v1 明确分离 `global`、`site`、`tenants` 和 `secretRefs`，并固化端口、DNS、CA、保留期、配额和外部依赖。

验证：

```bash
python3 scripts/alignment/validate_m10_site_values.py
python3 -m unittest tests.alignment.test_m10_site_values -v
```

任一未知字段、明文 password/secret/token/key、内联 PEM、默认 tenant/site、越界端口、TLS 缺 CA、非 TLS 携带 CA、重复 tenant/依赖/secret ref、或 tenant 引用未登记 Secret 时必须失败。现场实例必须从 v1 模板复制后替换 siteId、tenantId、容量和 Secret 引用，不能将真实密钥写入 values。

回滚仅停止使用 v1 输入并撤销 N003 验证器；不得删除旧模板、现场 Secret、Kubernetes 对象或历史 preflight 证据。
