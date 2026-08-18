# T1-M09-N022 告警详情路由样式拆分

## 当前结论

实现状态为 `PARTIAL`。本次只处理 `/alerts/:alertId` 当前路由中的响应建议和反馈组件：等值 token 与组件规则已迁移到 `web/ui/src/styles/alert-detail.css`，入口在 `pages.css` 之后加载；Kubernetes 观察通过后，原规则才从 `pages.css` 删除。没有顺带搬迁其他页面，也没有声称告警详情的全部历史样式已经拆完。

最新 Kubernetes run 为 `a1daf6e7-88e2-4f1f-826b-7e954fee2a66`。基线镜像 `traffic/web-ui:m09-n022-baseline-pre-refactor-20260816-r1` 与候选镜像 `traffic/web-ui:m09-n022-alert-detail-css-20260816-r2` 同时运行在节点 `8-2tb`，浏览器 Job 使用 `traffic/css-visual-diff:m09-n022-20260816-r3`。1366×900 和 1600×900 下，声明组件的 computed style 逐字段一致，截图字节与 SHA256 也分别完全相同。证据位于 `doc/02_acceptance/topic1/tasks/t1-m09-n022/k8s-css-refactor-latest.json`，所有临时 Pod、Service、ConfigMap 和 Job 均已按 run-id 清理。

## 运行

先构建不可变 Web 候选和 Chromium 比较镜像，将镜像导入当前 Kubernetes 节点的 containerd，再执行：

```bash
python3 scripts/alignment/run_m09_css_refactor_k8s.py \
  --baseline-image traffic/web-ui:m09-n022-baseline-pre-refactor-20260816-r1 \
  --candidate-image traffic/web-ui:m09-n022-alert-detail-css-20260816-r2 \
  --visual-image traffic/css-visual-diff:m09-n022-20260816-r3 \
  --timeout 900
```

静态合同与定向负例：

```bash
python3 scripts/alignment/verify_m09_css_refactor.py
python3 -m unittest tests.alignment.test_m09_css_refactor -v
```

## 失败与回滚

任一视口出现 computed style 差异、截图 SHA 不同、候选样式未加载、旧选择器仍留在 `pages.css`、镜像非精确 tag 或资源未清理，均不得接受本切片。回滚只恢复响应/反馈规则到 `pages.css` 并移除新样式入口，不涉及数据库、事件或生产流量。

## 边界

该 run 没有触碰共享数据库、Kafka、MinIO 或生产 Deployment；不证明 Windows Chrome、全告警详情页面、其他路由或全站 `pages.css` 已完成迁移，也不构成 M09 晋级授权。
