# T1-M10-N001 可部署候选闭合与回滚

本任务只建立 Kubernetes 部署候选的八维闭合合同，不创建、部署或晋级候选。

生成与校验：

```bash
python3 scripts/alignment/build_m10_deployable_candidate_closure.py
python3 scripts/alignment/build_m10_deployable_candidate_closure.py --check
python3 scripts/alignment/verify_m10_deployable_candidate_closure.py
python3 -m unittest tests.alignment.test_m10_deployable_candidate_closure -v
```

八个维度固定为 source tree、image digest、config、schema、topic、model、threshold、runbook。每个引用必须存在且带精确 SHA-256；任一维度不是 `BOUND`、任一上游候选不是同一 GO 候选、或 Git 源码树不干净时，输出必须为 `BLOCKED_INCOMPLETE`，`candidate_id` 必须为空。

本任务默认无运行时变化，因此回滚仅删除本任务新增的 schema、生成器、验证器、测试和 `deployments/releases/topic1/m10-deployable-candidate-closure.v1.json`。不得借回滚修改 Kubernetes 工作负载、共享存储或 M09 发布指针。
