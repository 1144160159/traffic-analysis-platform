# M02写范围Supersession账本

状态：`DESIGNED_NOT_REGISTERED / BLOCKED / NO-GO`

本账本追加有效执行范围的窄化规则，不修改P408冻结记录，也不使任何active catalog或执行包生效。

## 唯一规则

- predecessor：`T1-M02-P408-WRT-n004-l48`。
- 有效可写：`rust/probe-agent/probe-agent/src/capture/pcap_offline.rs#replay_delay`。
- 只读context：`rust/probe-agent/probe-agent/src/capture/pcap_offline.rs#ManifestPcapReplayer::poll_manifest`。
- context函数体owner：`T1-M02-P165-WRT-n004-l21`。
- 集成方向：P408 helper先于P165 body；P165在自己拥有的body内调用helper。

## 激活门

- `ONE_CLEAN_CANDIDATE_MANIFEST`
- `P408_REPLAY_DELAY_EXACT_RUST_AST_RECEIPT`
- `P165_POLL_MANIFEST_EXACT_RUST_AST_RECEIPT`
- `P408_UNIFIED_FUNCTION_REVIEW_RECEIPT`
- `P165_UNIFIED_FUNCTION_REVIEW_RECEIPT`
- `FOUR_CANDIDATE_CATALOGS_APPLY_THE_SAME_SUPERSESSION_HASH`

失败规则：`ANY_MISSING_OR_HASH_MISMATCH_KEEPS_P408_UNCLAIMABLE_AND_ACTIVE_REGISTRIES_UNCHANGED`。

## 证明上限

`WRITE_SCOPE_SUPERSESSION_DESIGN_ONLY_NOT_REGISTERED_LOCATOR_RESOLVED_FUNCTION_REVIEWED_IMPLEMENTED_OR_AUTHORIZED`
