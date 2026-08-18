# T1-M09-N010 版本化取证 worker 回滚

适用对象：`F-FORENSICS-001` 的兼容 idle worker、M02 PCAP v2 索引读取、M03 文件还原编排、MinIO 结果对象及 PostgreSQL checkpoint/final manifest。该候选只增加能力，部署默认值仍为：

- `FORENSICS_PIPELINE_V1_ENABLED=false`
- `FORENSICS_WORKER_ENABLED=false`
- `FORENSICS_WORKER_COMPATIBLE_READY=false`

## 停止条件

出现跨租户索引或对象命中、latest 对象回退、SHA/ETag/VersionID 不一致仍被接受、同一任务产生第二个结果对象或 manifest、checkpoint fencing 丢失、恢复文件被执行或主动打开、源过期仍继续处理时，立即停止扩大流量。

## 回滚顺序

1. 先设置 `FORENSICS_PIPELINE_V1_ENABLED=false`，停止新的命令写入；不得让 writer 在 worker 不可用时继续接单。
2. 再设置 `FORENSICS_WORKER_ENABLED=false`，停止领取新租约。可保留 `FORENSICS_WORKER_COMPATIBLE_READY=true` 的 idle 实例用于健康和兼容观察，但它不得轮询 durable queue。
3. 对已领取任务记录 tenant、task、request SHA、lease token、checkpoint revision、phase、结果 object version 和 manifest SHA。让仍在有效租约内且未触发停止条件的任务完成；其余任务等待租约到期后按同一冻结请求恢复，不手工改写 checkpoint。
4. 保留 legacy PCAP cut/status 路由，不删除已接受任务。版本化任务没有 final manifest 时不得展示为 completed；M03 receipt 为 partial/corrupt 时必须保留 partial 终态。
5. 核对 `forensics_task_checkpoints`、`forensics_job_manifests`、task history/outbox/request/audit 及 MinIO object version。对象存在但 manifest 未提交时只能按相同 tenant/task/size/SHA 恢复，不能 PUT 第二版本。

## 数据与对象处理

迁移 `202608151930` 是 expand-only。回滚不得删除或降级 checkpoint、final manifest、M02 source index、source/result object version、M03 restoration manifest/receipt、legal hold 或 retention 信息。临时 staging 文件由进程本地清理；持久对象的孤儿判定和删除必须交给后续引用、保留和法务保全感知的治理流程。

任何恢复结果都按惰性字节处理：服务端不得执行、导入、解压为可执行路径或主动渲染内容。下载仍需后续受控授权和持久审计；本任务的 worker 完成证据不替代下载授权证据。

## 重新启用

先以相同候选镜像在 K8s 部署 `FORENSICS_WORKER_COMPATIBLE_READY=true`、`FORENSICS_WORKER_ENABLED=false` 的 idle worker，确认它不读取 durable queue。再启用 worker consumer，验证过期租约恢复、重复处理只复用精确对象/manifest、bad hash/missing/expired/cross-tenant 均 fail closed。最后才可启用 `FORENSICS_PIPELINE_V1_ENABLED` 的 writer canary；任何阶段失败都按上述逆序关闭。
