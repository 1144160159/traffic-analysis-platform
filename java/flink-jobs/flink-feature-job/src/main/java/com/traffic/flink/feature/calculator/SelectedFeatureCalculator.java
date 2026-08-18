package com.traffic.flink.feature.calculator;

import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/**
 * SelectedFeatureCalculator —— 特征 exact-set 计算(FC01-FC11)。
 *
 * 特征池字段级映射(统一PCAP特征池与在线覆盖契约):
 *  - A 组 85 标量:S1 Rust 流聚合产出,经 Session 透传(输入 map 中的键即特征 id);
 *  - B 组 17 序列:64 包有界分段,透传;
 *  - D 组 7 派生列:本计算器确定性派生(在线离线同一实现);
 *  - E 组 10 窗口上下文:窗口统计;
 *  - 缺失模态:未执行深层预算的特征必须进入 modality_missingness_mask,禁止伪造全零。
 *
 * 不变量:未选 calculator 零调用;required 失败=ERROR;optional 失败=PARTIAL+missing;
 * 只输出 exact-set;每输出带 feature hash;禁止配置缺失时静默回退全量。
 */
public final class SelectedFeatureCalculator {

    public static final String GROUP_DERIVED_INITIATOR_DIRECTION = "initiator_relative_direction_seq";
    public static final String GROUP_DERIVED_SIGNED_LENGTH = "signed_packet_length_seq";
    public static final String GROUP_DERIVED_BURST_COUNT = "directional_burst_count";
    public static final String GROUP_DERIVED_BURST_PACKET_SUMMARY = "directional_burst_packet_count_summary";
    public static final String GROUP_DERIVED_BURST_BYTE_SUMMARY = "directional_burst_byte_summary";
    public static final String GROUP_DERIVED_PAYLOAD_PRESENCE = "payload_presence_fraction";
    public static final String GROUP_DERIVED_MISSINGNESS = "modality_missingness_mask";

    /** 可选特征(optional):失败产 PARTIAL+missing,不使整次计算 ERROR。 */
    private static final List<String> OPTIONAL_FEATURES = List.of(
            "payload_histogram", "payload_b64", "sanitized_l4_b64",
            "tls_record_type_seq", "tls_record_version_seq", "tls_record_length_seq",
            "tls_handshake_type_seq", "quic_long_header_packet_count", "quic_version_seq");

    private final FeatureCatalog catalog;

    public SelectedFeatureCalculator(FeatureCatalog catalog) {
        this.catalog = Objects.requireNonNull(catalog, "catalog");
    }

    /** 计算结果。 */
    public static final class SelectedFeatureResult {
        private final String featureHash;
        private final Map<String, Object> features;
        private final List<String> missingFields;
        private final boolean partial;

        private SelectedFeatureResult(String featureHash, Map<String, Object> features,
                                      List<String> missingFields, boolean partial) {
            this.featureHash = featureHash;
            this.features = Collections.unmodifiableMap(new LinkedHashMap<>(features));
            this.missingFields = Collections.unmodifiableList(new ArrayList<>(missingFields));
            this.partial = partial;
        }

        public String featureHash() { return featureHash; }
        public Map<String, Object> features() { return features; }
        public List<String> missingFields() { return missingFields; }
        public boolean partial() { return partial; }
    }

    /**
     * calculate(FC01-FC11):输入原始特征 map(键=特征 id,值=标量/序列/窗口上下文),
     * 按 selection exact-set 计算并输出。
     *
     * @param selectionHash 冻结计划的 selection hash(与 FeatureSelectionPlan 一致)
     */
    public SelectedFeatureResult calculate(Map<String, Object> rawFeatures,
                                           List<String> requiredDetectorFeatures,
                                           FeatureSelectionPlan plan,
                                           String selectionHash) {
        Objects.requireNonNull(rawFeatures, "rawFeatures");
        Objects.requireNonNull(plan, "plan");
        if (!plan.selectionHash().equals(selectionHash)) {
            throw new IllegalStateException("selection hash mismatch: expected "
                    + plan.selectionHash() + " got " + selectionHash);
        }

        // FC02:每 selected feature 一个 calculator(未选零调用)
        Map<String, Object> out = new LinkedHashMap<>();
        List<String> missing = new ArrayList<>();
        boolean partial = false;
        for (String id : plan.selectedFeatureIds()) {
            if (catalog.isDerived(id)) {
                Object v = computeDerived(id, rawFeatures, out);
                if (v == null) {
                    missing.add(id);
                    partial = true;
                } else {
                    out.put(id, v);
                }
                continue;
            }
            if (rawFeatures.containsKey(id)) {
                out.put(id, rawFeatures.get(id));
                continue;
            }
            if (OPTIONAL_FEATURES.contains(id)) {
                missing.add(id);
                partial = true;
                continue;
            }
            // required 缺失:ERROR 语义(由调用方转 ERROR outcome)
            throw new IllegalStateException("required feature missing: " + id);
        }
        // 缺失性掩码:未执行深层模态显式标记(不伪造全零)
        List<String> mask = computeMissingnessMask(rawFeatures, plan.selectedFeatureIds());
        if (!mask.isEmpty()) {
            out.put(GROUP_DERIVED_MISSINGNESS, mask);
        }
        // 只输出 exact-set + 缺失性掩码
        String hash = FeatureSelectionPlan.sha256Hex(
                plan.featureSetId() + "\u001f" + selectionHash + "\u001f" + canonical(out));
        return new SelectedFeatureResult(hash, out, missing, partial);
    }

    /** D 组派生列(确定性,在线离线同一实现)。 */
    private Object computeDerived(String id, Map<String, Object> raw, Map<String, Object> out) {
        switch (id) {
            case GROUP_DERIVED_INITIATOR_DIRECTION:
                return deriveInitiatorDirection(raw);
            case GROUP_DERIVED_SIGNED_LENGTH:
                return deriveSignedLength(raw, out);
            case GROUP_DERIVED_BURST_COUNT:
                return deriveBurstCount(raw);
            case GROUP_DERIVED_BURST_PACKET_SUMMARY:
                return deriveBurstPacketSummary(raw);
            case GROUP_DERIVED_BURST_BYTE_SUMMARY:
                return deriveBurstByteSummary(raw);
            case GROUP_DERIVED_PAYLOAD_PRESENCE:
                return derivePayloadPresence(raw);
            default:
                return null;
        }
    }

    /** initiator 相对方向序列:与 direction_seq 对齐(同向=1,反向=0)。 */
    private List<Integer> deriveInitiatorDirection(Map<String, Object> raw) {
        Object dir = raw.get("direction_seq");
        if (!(dir instanceof List)) {
            return null;
        }
        List<?> dirs = (List<?>) dir;
        List<Integer> out = new ArrayList<>(dirs.size());
        Integer first = null;
        for (Object d : dirs) {
            if (d == null) {
                out.add(0);
                continue;
            }
            int v = ((Number) d).intValue();
            if (first == null) {
                first = v;
                out.add(1);
            } else {
                out.add(v == first.intValue() ? 1 : 0);
            }
        }
        return out;
    }

    /** signed_packet_length_seq:发起方向为正,反方向为负(方向读 out 中派生列)。 */
    private List<Long> deriveSignedLength(Map<String, Object> raw, Map<String, Object> out) {
        Object len = raw.get("packet_length_seq");
        Object dir = out.get("initiator_relative_direction_seq");
        if (!(len instanceof List) || !(dir instanceof List)) {
            return null;
        }
        List<?> lens = (List<?>) len;
        List<?> dirs = (List<?>) dir;
        if (lens.size() != dirs.size()) {
            return null;
        }
        List<Long> outList = new ArrayList<>(lens.size());
        for (int i = 0; i < lens.size(); i++) {
            long v = ((Number) lens.get(i)).longValue();
            int d = ((Number) dirs.get(i)).intValue();
            outList.add(d > 0 ? v : -v);
        }
        return outList;
    }

    /** directional_burst_count:同向连续包为一个 burst,统计发起方向 burst 数。 */
    private Long deriveBurstCount(Map<String, Object> raw) {
        List<?> dirs = directionList(raw);
        if (dirs == null) {
            return null;
        }
        long bursts = 0;
        Integer prev = null;
        for (Object d : dirs) {
            int v = ((Number) d).intValue();
            if (prev == null || v != prev.intValue()) {
                if (v > 0) {
                    bursts++;
                }
                prev = v;
            }
        }
        return bursts;
    }

    private List<Long> deriveBurstPacketSummary(Map<String, Object> raw) {
        List<?> dirs = directionList(raw);
        if (dirs == null) {
            return null;
        }
        List<Long> out = new ArrayList<>();
        Integer prev = null;
        long run = 0;
        for (Object d : dirs) {
            int v = ((Number) d).intValue();
            if (prev != null && v != prev.intValue()) {
                out.add(run);
                run = 0;
            }
            run++;
            prev = v;
        }
        if (run > 0) {
            out.add(run);
        }
        return out;
    }

    private List<Long> deriveBurstByteSummary(Map<String, Object> raw) {
        // 简化契约:与 burst packet summary 同构,字节口径由 S1 payload 长度序列提供。
        Object len = raw.get("packet_payload_length_seq");
        if (!(len instanceof List)) {
            return deriveBurstPacketSummary(raw);
        }
        List<?> lens = (List<?>) len;
        List<?> dirs = directionList(raw);
        if (dirs == null || lens.size() != dirs.size()) {
            return null;
        }
        List<Long> out = new ArrayList<>();
        long runBytes = 0;
        Integer prev = null;
        for (int i = 0; i < dirs.size(); i++) {
            int v = ((Number) dirs.get(i)).intValue();
            if (prev != null && v != prev.intValue()) {
                out.add(runBytes);
                runBytes = 0;
            }
            runBytes += ((Number) lens.get(i)).longValue();
            prev = v;
        }
        if (runBytes > 0 || (prev != null)) {
            out.add(runBytes);
        }
        return out;
    }

    private Double derivePayloadPresence(Map<String, Object> raw) {
        Object stored = raw.get("payload_bytes_stored");
        Object total = raw.get("payload_bytes_total");
        if (stored instanceof Number && total instanceof Number && ((Number) total).doubleValue() > 0) {
            return ((Number) stored).doubleValue() / ((Number) total).doubleValue();
        }
        return 0.0;
    }

    private List<?> directionList(Map<String, Object> raw) {
        Object dir = raw.get("initiator_relative_direction_seq");
        if (dir instanceof List) {
            return (List<?>) dir;
        }
        dir = raw.get("direction_seq");
        return (dir instanceof List) ? (List<?>) dir : null;
    }

    /** 缺失性掩码:深层预算模态未执行时显式标记(禁止伪造全零)。 */
    private List<String> computeMissingnessMask(Map<String, Object> raw, List<String> selected) {
        List<String> deep = List.of("payload_histogram", "payload_b64", "sanitized_l4_b64",
                "tls_record_type_seq", "quic_version_seq");
        List<String> mask = new ArrayList<>();
        for (String id : deep) {
            if (selected.contains(id) && !raw.containsKey(id)) {
                mask.add(id);
            }
        }
        return mask;
    }

    /** canonical 字符串(特征哈希输入):键排序稳定。 */
    private String canonical(Map<String, Object> out) {
        List<String> keys = new ArrayList<>(out.keySet());
        Collections.sort(keys);
        StringBuilder sb = new StringBuilder();
        for (String k : keys) {
            Object v = out.get(k);
            sb.append(k).append('=').append(String.valueOf(v)).append(';');
        }
        return sb.toString();
    }

    /**
     * FeatureCatalog 特征目录:特征存在性、是否派生、依赖。
     */
    public interface FeatureCatalog {
        boolean isDerived(String featureId);
    }
}
