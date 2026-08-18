# T1-M10-N008 原子 PKI 轮换运行手册

## 目标与边界

本任务为 Probe Agent 到 Ingest Gateway 的既有 mTLS 增加默认关闭的原子轮换路径。一个 generation 必须同时绑定服务端证书、私钥、双 CA 信任包和 issuer 签名 CRL；任何文件摘要、SAN、EKU、有效期、签名或 CRL 不合法时，服务保留上一代快照。当前 Kubernetes Job 只运行内存证书链负例，不读取或修改 live Secret、Deployment 或业务数据。

## 构建与本地验证

```bash
(cd go/control-plane && go test ./internal/common/pki ./internal/ingest/config ./cmd/ingest-gateway -count=1)
python3 scripts/alignment/build_pki_catalog.py --check
python3 scripts/alignment/verify_pki_catalog.py
python3 -m unittest tests.alignment.test_pki_catalog -v
```

使用仓库指定的 Go 1.25.12 预编译测试二进制，并以固定 Debian digest 组装镜像：

```bash
tmp_dir="$(mktemp -d)"
(cd go/control-plane && CGO_ENABLED=0 go test -c -o "$tmp_dir/pki.test" ./internal/common/pki)
cp scripts/alignment/run_m10_pki_test_binary.sh "$tmp_dir/"
docker build -f scripts/alignment/Dockerfile.m10-pki-prebuilt \
  -t traffic-analysis/m10-pki-tests:<run-id> "$tmp_dir"
```

镜像以非 `latest` 标签分别导入 8-2tb 和 zeus-server 的 containerd 后执行：

```bash
python3 scripts/alignment/run_m10_pki_rotation_k8s.py \
  --image traffic-analysis/m10-pki-tests:<run-id> \
  --nodes 8-2tb,zeus-server \
  --run-id <canonical-uuid>
python3 scripts/alignment/verify_m10_pki_rotation_k8s.py
```

临时构建目录不得进入候选，删除时须先限定到 `mktemp -d` 返回的精确路径。Job 必须保持非 root、只读根文件系统、无 ServiceAccount token、无 Secret volume，并在收据写入两个节点的同一 image ID。

## 启用顺序

只有 N007 与 deployable candidate 已通过、issuer/根密钥保管获得批准、每个 probe 有唯一身份后，才能进入 live 启用：

1. issuer 发布旧 CA 与新 CA 的双信任包、两份当前 CRL、新服务端证书和完整 `generation.json`。
2. 仅对精确 canary Deployment 打开 `TLS_ROTATION_V1_ENABLED=true`，确认旧、新客户端均可握手，错 CA、错 SAN、过期及吊销客户端均失败。
3. 更新 canary probe Secret；现有连接保留，下一次重连重新读取 Secret。
4. 所有目标 probe 切换后发布仅含新 CA/CRL 的下一代 generation，验证旧 CA 立即失败。
5. 完成两次连续 live 轮换、故障注入、观测窗口与 G6 回滚，才可申请晋级。

## 回滚 RB-T1-M10-P018-OPS-n008-s1

1. 停止新 generation 发布，不删除当前可用 Secret。
2. 将同一旧 generation 的原始四份材料和 manifest 原子恢复；generation ID 复用但任一摘要变化会被运行时拒绝。
3. 若新运行路径尚未稳定，恢复 `TLS_ROTATION_V1_ENABLED=false` 和启用前 Deployment template，再等待 rollout。
4. 验证旧客户端正例以及错 CA、错 SAN、过期、吊销负例；保存资源 UID/resourceVersion、镜像 ID、证书序列号和时间窗口。
5. 任一步无证据或出现双版本混用，G6 保持 BLOCKED。

## 证明上限

双节点 Job PASS 只证明绑定源码在 Kubernetes 容器内通过声明的 TLS 负例和轮换状态测试；不证明 approved issuer、离线根保管、每 probe 唯一证书、live Secret 切换、连续两次轮换、live 回滚或生产晋级已经完成。
