package com.traffic.flink.feature.calculator;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;

/**
 * FeatureSelectionPlan —— 冻结的特征选择计划(FS01-FS09 resolve 的产物)。
 * selected calculator exact-set:未选 calculator 零调用;依赖闭包展开后无环、无额外项。
 */
public final class FeatureSelectionPlan {
    private final String featureSetId;
    private final List<String> selectedFeatureIds;
    private final String selectionHash;
    private final Map<String, List<String>> dependencyClosure;

    private FeatureSelectionPlan(String featureSetId, List<String> selectedFeatureIds,
                                 String selectionHash, Map<String, List<String>> dependencyClosure) {
        this.featureSetId = featureSetId;
        this.selectedFeatureIds = Collections.unmodifiableList(new ArrayList<>(selectedFeatureIds));
        this.selectionHash = selectionHash;
        this.dependencyClosure = dependencyClosure;
    }

    /**
     * resolve 展开依赖闭包(FS01-FS09):拒绝循环、额外项与未知 feature id。
     * 依赖表由 FeatureCatalog 提供(feature id → 依赖 feature ids)。
     */
    public static FeatureSelectionPlan resolve(String featureSetId,
                                               List<String> requestedIds,
                                               Map<String, List<String>> catalogFeatures,
                                               Map<String, List<String>> dependencies) {
        Objects.requireNonNull(requestedIds, "requestedIds");
        LinkedHashMap<String, Boolean> closed = new LinkedHashMap<>();
        Map<String, List<String>> closure = new HashMap<>();
        for (String id : requestedIds) {
            if (!catalogFeatures.containsKey(id)) {
                throw new IllegalArgumentException("unknown feature id: " + id);
            }
            expand(id, dependencies, closed, closure, new ArrayList<>());
        }
        List<String> ordered = new ArrayList<>(closed.keySet());
        String hash = sha256Hex(featureSetId + "\u001f" + String.join(",", ordered));
        return new FeatureSelectionPlan(featureSetId, ordered, hash, closure);
    }

    private static void expand(String id, Map<String, List<String>> deps,
                               Map<String, Boolean> closed, Map<String, List<String>> closure,
                               List<String> stack) {
        if (closed.containsKey(id)) {
            return;
        }
        if (stack.contains(id)) {
            throw new IllegalArgumentException("dependency cycle at feature: " + id);
        }
        stack.add(id);
        List<String> d = deps.getOrDefault(id, Collections.emptyList());
        for (String dep : d) {
            expand(dep, deps, closed, closure, stack);
        }
        stack.remove(stack.size() - 1);
        closure.put(id, d);
        closed.put(id, Boolean.TRUE);
    }

    public String featureSetId() { return featureSetId; }
    public List<String> selectedFeatureIds() { return selectedFeatureIds; }
    public String selectionHash() { return selectionHash; }
    public Map<String, List<String>> dependencyClosure() { return dependencyClosure; }

    static String sha256Hex(String s) {
        try {
            MessageDigest md = MessageDigest.getInstance("SHA-256");
            byte[] d = md.digest(s.getBytes(StandardCharsets.UTF_8));
            StringBuilder sb = new StringBuilder();
            for (byte b : d) {
                sb.append(String.format("%02x", b));
            }
            return sb.toString();
        } catch (Exception e) {
            throw new IllegalStateException("sha-256 unavailable", e);
        }
    }
}
