# T-PKI-001 证书、传输与轮换运行手册

## 1. 控制目标

本控制项覆盖探针—ingest、Kafka、Keycloak、OpenSearch 以及后续 PostgreSQL、MinIO 和浏览器入口的证书身份、信任链、SAN/EKU、最小 TLS 版本、私钥托管、轮换、撤销和回滚。

权威目录为 `contracts/security/pki-catalog.v1.json`。目录只保存证书公开元数据、Secret/key 引用、authority hash 和缺口；禁止保存私钥、keystore/truststore 口令、token 或 Secret data。live 采集只允许解析公开证书并保存派生的 fingerprint、subject、issuer、serial、有效期、SAN/EKU 与验证结果。

## 2. 当前候选事实

- probe—ingest 已要求 TLS 1.3 双向认证；远程明文、`https` 缺少证书三件套、部分 TLS 配置、生产关闭 mTLS 或允许无 token 均启动失败。
- probe 叶证书默认不超过 90 天，生成时验证 CA、SAN、EKU 和权限；部署前对既有证书验证同一 CA、server/client purpose、服务 DNS 及 30 天续期窗口。临近到期时停止部署并要求受控双信任轮换，不自动覆盖现网 Secret。
- 当前 probe DaemonSet 仍共享一个客户端证书，未达到每探针唯一身份；签发仍是仓库脚本自签 CA，缺少获批在线 issuer、撤销和两次连续轮换证据。
- Kafka 叶证书当前 825 天且三个 broker 共用，业务客户端仅有 CA/truststore 并用 SCRAM 身份；Keycloak 使用 825 天自签 leaf；两者都缺少到期护栏与 serial canary。
- OpenSearch mTLS 与分服务客户端证书仅为 ExternalSecret target，issuer 路径仍是 placeholder 且未发布。
- MinIO 已新增第五个证书域和 expand-only TLS 材料合同：服务端 Secret 固定 `public.crt`、`private.key`、`ca.crt`，覆盖 Service 与四个 Pod DNS SAN，并向 `traffic-analysis`、`flink`、`argo` 分发 CA 引用。14 组件默认关闭切换包及五个本地 `linux/amd64` 客户端候选镜像已形成；镜像仍未签名、repo-digest 固定或分发到双节点，`cutover_ready=false`，服务端和客户端均未在生产激活。
- PostgreSQL、MinIO 与当前 OpenSearch 客户端仍有生产明文依赖，因此目录完整性 PASS 不等于 PKI 合规。

## 3. 仓库验证

```bash
python3 scripts/alignment/build_pki_catalog.py --check
python3 scripts/alignment/verify_pki_catalog.py
python3 scripts/alignment/verify_minio_tls_material.py
python3 scripts/alignment/verify_minio_tls_cutover.py
python3 -m unittest tests.alignment.test_pki_catalog -v
python3 -m unittest tests.alignment.test_minio_tls_material -v
python3 -m unittest tests.alignment.test_minio_tls_cutover tests.alignment.test_minio_tls_candidate_images -v
go -C go/control-plane test ./internal/ingest/config -count=1
cargo test --manifest-path rust/probe-agent/Cargo.toml -p probe-agent transport_tests --lib
bash -n deployments/kubernetes/deploy.sh
bash -n rust/probe-agent/scripts/generate-mtls-certs.sh
kubectl apply --dry-run=client -f deployments/kubernetes/applications/go-services.yaml
kubectl apply --dry-run=client -f deployments/kubernetes/applications/probe-agent.yaml
```

负向测试必须证明：远程明文被拒绝、部分 mTLS 被拒绝、超长 leaf 和共享 probe 身份缺口不能隐藏、MinIO SAN/CA namespace 不能缩减、MinIO 材料阶段不能伪称 cutover-ready、生产明文依赖不能从目录删除、无 live 轮换证据不能声明 PASS、私钥内容不能进入目录。

## 4. 只读 live 证据

使用与当前 candidate hash 相同的完整 G0 PASS：

```bash
make alignment-capture-pki-catalog \
  RUN_ID=<immutable-run-id> \
  G0_MANIFEST=<matching-g0-manifest>

make alignment-capture-minio-tls-candidate-images \
  RUN_ID=<immutable-image-run-id> \
  G0_MANIFEST=<matching-g0-manifest>
```

采集器读取公开证书字段和 Secret metadata，不读取或落盘私钥与密码。缺失 Secret、证书临近到期、CA 不一致、SAN/EKU/hostname 验证失败、候选未部署或只能看到 truststore 而不能确认 leaf 时，一律记录 UNKNOWN/FAIL/PARTIAL，不得按 HTTP 2xx 或 Pod Ready 推断证书正确。

## 5. 签发与轮换流程

1. 批准离线 root custody、在线 intermediate issuer、命名规范、SAN/EKU、90 天以内 leaf、30 天 renewBefore、撤销机制和审计 owner。
2. 为每个服务、broker、Flink job 和 probe 建立唯一 identity；私钥只由批准的 Secret/CSI/issuer 路径交付，禁止进入 Git、ConfigMap、日志或证据包。
3. expand 新 CA/intermediate 和新 leaf，客户端先 dual trust；旧证书继续服务但禁止新增使用者。
4. 先内部 tenant 和单 probe/单 broker/单服务副本 canary，验证双向握手、hostname、EKU、认证主体、审计、业务读写和性能。
5. serial 扩大后证明旧 identity 使用量归零，再撤销旧 leaf/intermediate；保留旧 trust 到回滚窗口结束。
6. 至少执行两次连续轮换，并保存到期、未生效、错误 SAN、错误 EKU、未知 CA、撤销和时钟偏移负向证据。

## 6. 停止扩大与回滚

出现明文降级、hostname/SAN/EKU 不匹配、匿名或共享主体获得额外权限、证书/私钥泄漏、握手错误持续扩大、Kafka/Flink 不可恢复、数据丢失、跨租户、审计缺失或 P99/资源越界时立即停止扩大。

回滚恢复上一版 digest-bound 工作负载、旧 leaf 和 dual-trust bundle；先验证身份与只读，再恢复写入。在途 Kafka/Flink/异步任务按稳定 ID 对账。疑似私钥泄漏时必须吊销并重签，不得恢复泄漏密钥。

## 7. 关闭标准

所有生产传输均有批准的加密/身份边界；每个 workload/probe 身份唯一；live 证书与发布 manifest 的 issuer、fingerprint、SAN、EKU、有效期完全对账；PostgreSQL、MinIO、OpenSearch、Kafka、服务间和浏览器入口不再依赖未批准明文或跳过校验；两次轮换、撤销、故障、回滚、T+0/T+1/T+3/T+7 与完整业务周期通过；独立安全、QA、SRE 签认后才可关闭 T-PKI-001。G8 外部门禁仍独立判定。
