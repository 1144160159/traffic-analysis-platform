# T1-M10-N002 候选制品来源守卫与回滚

`candidate_snapshot.py` 扫描生效 Kubernetes application manifest 中的第一方镜像，以及从源码内容树排除的预构建二进制。每个制品登记必须闭合 binary SHA、builder/source SHA、toolchain、SBOM、SLSA attestation、不可变 image digest 和镜像内 binary SHA。

`capture_g0.py` 在执行任何 G0 命令之前读取扫描结果。结果不是 `PASS` 时，它只写入 `status=BLOCKED`、`commands=[]` 的 manifest 并以返回码 2 退出；不得通过跳过来源守卫继续执行测试。

验证：

```bash
python3 -m unittest tests.alignment.test_m10_candidate_provenance_guard -v
python3 scripts/alignment/candidate_snapshot.py > /tmp/candidate-snapshot.json
jq '.artifact_provenance | {status,blocking_codes}' /tmp/candidate-snapshot.json
```

当前仓库没有 `deployments/releases/topic1/m10-artifact-provenance.v1.json`，且两个旧 overlay 没有被 Makefile/CI 明确选中，因此预期状态是 `BLOCKED`。这是正确的 fail-closed 结果，不是 G0 PASS。

回滚只撤销来源扫描字段和 G0 前置守卫；不得删除二进制、SBOM、attestation、历史 G0 manifest 或更改 Kubernetes 工作负载。
