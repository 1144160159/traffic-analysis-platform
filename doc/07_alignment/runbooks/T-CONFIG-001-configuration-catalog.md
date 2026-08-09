# T-CONFIG-001 配置目录、优先级与漂移门禁

## 当前边界

本候选建立消费者级机器目录，覆盖 Go 环境变量消费点、Flink properties 和 Kubernetes 环境绑定；同名环境变量按消费者和声明位置分别登记，禁止把不同服务的同名键误判为同一配置。目录还固定 Kafka Topic/ACL、Flink Job、ClickHouse Schema、MinIO lifecycle、IAM scope 和 APISIX route 的权威来源及内容 hash。

当前状态只能是 `IMPLEMENTING/PARTIAL`。目录治理元数据已成为候选真源，但运行值仍处于 `legacy_exact_diff_until_catalog_renderers_land` 过渡期；生产进程尚未全部输出脱敏 effective config 摘要、`config_version` 和 `effective_config_sha256` 指标，因此不能证明声明、渲染、集群对象和进程实际值四者一致，也不能关闭 T-CONFIG-001。

## 不变量

- 每个消费点必须登记 `type/default/required/secret/legal_range/owner/consumer/hot_reload/restart_required/environment_override/deprecation/sources`。
- Secret不得在目录、普通ConfigMap、日志或证据中保存明文；目录只记录Secret引用，live证据只记录引用名称、key和资源版本。
- 非空Secret代码默认值必须使CI失败；认证启用但JWT Secret缺失或不足32字符时，告警服务启动失败。
- 配置优先级固定为 `CLI → environment → Secret reference → ConfigMap/file → code/properties default`，不得由服务自行产生未登记的另一套顺序。
- 同一Kubernetes运行时绑定允许存在过渡期重复清单，但声明身份必须完全一致；任何值、Secret引用或optional语义差异均使门禁失败。
- effective hash采用规范JSON SHA-256；非Secret包含解析后的值，Secret仅包含引用和resourceVersion，禁止包含Secret值。
- 热加载必须有实例ACK；未ACK实例保持旧版本并退出流量。重启型配置逐实例滚动并经过readiness后再扩大。

## 生成与静态门禁

```bash
python3 scripts/alignment/build_configuration_catalog.py
python3 scripts/alignment/build_configuration_catalog.py --check
python3 scripts/alignment/verify_configuration_catalog.py
python3 -m unittest tests.alignment.test_configuration_catalog -v
```

生成结果为 `contracts/configuration/configuration-catalog.v1.json`。`--check`不会写文件，只比较全部受治理源码与已提交目录；源码或目录任一侧变化但未同步时失败。负向测试覆盖Secret默认值和重复运行时绑定冲突。

## 只读live核对

候选通过完整G0后执行：

```bash
python3 scripts/alignment/capture_configuration_catalog.py \
  --run-id <immutable-run-id> \
  --g0-manifest doc/02_acceptance/runs/<g0-run>/manifest.json
```

采集器只读查询Deployment、StatefulSet、DaemonSet、Job和CronJob；普通值只保存SHA-256，Secret只查询metadata.resourceVersion，不读取或保存Secret data。输出逐workload rendered hash、已登记匹配数、漂移ID、未登记ID和进程effective hash缺失原因。任何live查询错误不得改写为PASS。

## 灰度顺序

1. 固定目录hash、渲染器版本、候选镜像digest和Secret版本引用。
2. 先将Kafka、Flink、ClickHouse、MinIO、IAM和APISIX已有专项真源接入目录渲染或精确diff。
3. 为Go、Rust、Flink进程增加统一脱敏启动摘要和effective hash指标，确认同一Deployment所有实例一致。
4. 内部tenant先验证非法值、缺少必填、环境覆盖冲突、部分实例未更新和Secret轮换。
5. 热加载配置要求全部ACK；重启配置逐Pod滚动，readiness和业务对账通过后才扩大。
6. 保存T+0、T+1、T+3、T+7的声明、渲染、集群和进程hash矩阵。

## 停止与回滚

发现Secret明文、未登记配置、同一实例hash混合、必填值缺失仍启动、热加载无ACK、滚动后readiness失败、数据兼容或资源预算越界时立即停止扩大。

回滚必须恢复完整不可变目录、渲染产物、Secret版本引用和候选镜像，不允许在线手改单个键。回滚后重新采集四层hash并核对在途任务；目录和历史证据保留，不删除以伪造一致。

## 关闭条件

T-CONFIG-001关闭需要全部生产配置由目录生成或受精确diff约束，所有关键进程输出脱敏effective hash，声明/渲染/集群/进程四层对账一致，并完成非法值、缺少必填、覆盖冲突、部分实例、热加载ACK、滚动重启、Secret轮换、回滚、性能和观察期证据。仓库目录PASS或Kubernetes对象一致不能单独证明关闭。
