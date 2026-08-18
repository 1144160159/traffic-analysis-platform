# M02代码直达Locator覆盖清单

状态：`BLOCKED_COVERAGE_INCOMPLETE / NO-GO`

本清单只分类resolver覆盖，不声称任何locator已解析。

## 汇总

- PLANNED叶：213；locator occurrence：270；唯一locator：269。
- 已有受信resolver但缺clean candidate：125。
- 文件存在但缺trusted resolver：0。
- planned文件尚不存在：145。
- ownership冲突：1。

## Resolver profile

| Profile | Locator occurrence |
|---|---:|
| `GO_AST_V1` | 45 |
| `PLANNED_ARTIFACT_REQUIRED` | 145 |
| `PROTO_DESCRIPTOR_V1` | 5 |
| `PYTHON_AST_V1` | 5 |
| `RUST_SYN_V1` | 60 |
| `SHELL_AST_V1` | 1 |
| `STRUCTURED_CONFIG_V1` | 9 |

## 阻断冲突

- `rust/probe-agent/probe-agent/src/capture/pcap_offline.rs#ManifestPcapReplayer::poll_manifest`：M02-N004-L21, M02-N004-L48；处置=`REGISTER_THE_SAME_SUPERSESSION_HASH_IN_FOUR_CANDIDATE_CATALOGS_THEN_SIGN_BOTH_FUNCTION_REVIEWS`。

## 下一批次

1. 先冻结clean candidate，复用现有Go/Python/Rust/Java/shell AST、Protobuf descriptor、SQL DDL parse tree和structured-config resolver处理125个现存文件occurrence；planned Java/SQL文件创建后必须走对应candidate-bound receipt。
2. planned文件只在兼容seam评审后创建，再以对应语言/结构化resolver解析after-state locator。
3. 以append-only supersession消除P408对poll_manifest的可写companion范围，再完成default-off与P165/P408函数评审；不得重写冻结P308-P485历史。

## 证明上限

`LOCATOR_COVERAGE_CLASSIFICATION_ONLY_NOT_LOCATOR_RESOLVED_TARGET_BINDING_FUNCTION_REVIEW_IMPLEMENTATION_OR_EXECUTION_AUTHORIZATION`
