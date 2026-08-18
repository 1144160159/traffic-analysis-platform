package com.traffic.flink.feature.calculator;

import org.junit.jupiter.api.Test;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

/**
 * SelectedFeatureCalculator 单元测试(oracle:ATC-FEAT-T-001..010)。
 */
class SelectedFeatureCalculatorTest {

    private static final List<String> CATALOG = Arrays.asList(
            "packet_length_seq", "direction_seq", "packet_payload_length_seq",
            "payload_bytes_stored", "payload_bytes_total", "payload_histogram",
            "pktlen_mean", "iat_mean_ms", "tcp_flag_syn_cnt",
            "initiator_relative_direction_seq", "signed_packet_length_seq",
            "directional_burst_count", "directional_burst_packet_count_summary",
            "directional_burst_byte_summary", "payload_presence_fraction",
            "modality_missingness_mask");

    private static SelectedFeatureCalculator.FeatureCatalog catalog = featureId ->
            featureId.startsWith("initiator_relative_direction_seq")
                    || featureId.startsWith("signed_packet_length_seq")
                    || featureId.startsWith("directional_burst_")
                    || featureId.equals("payload_presence_fraction");

    private static FeatureSelectionPlan planOf(String... ids) {
        Map<String, List<String>> cat = new HashMap<>();
        for (String id : CATALOG) {
            cat.put(id, List.of());
        }
        Map<String, List<String>> deps = Map.of(
                "signed_packet_length_seq", List.of("packet_length_seq", "initiator_relative_direction_seq"),
                "initiator_relative_direction_seq", List.of("direction_seq"),
                "directional_burst_count", List.of("initiator_relative_direction_seq"),
                "directional_burst_packet_count_summary", List.of("initiator_relative_direction_seq"),
                "directional_burst_byte_summary", List.of("initiator_relative_direction_seq", "packet_payload_length_seq"));
        return FeatureSelectionPlan.resolve("fs-v1", Arrays.asList(ids), cat, deps);
    }

    private static Map<String, Object> raw() {
        Map<String, Object> m = new HashMap<>();
        m.put("pktlen_mean", 64.0f);
        m.put("iat_mean_ms", 1.5f);
        m.put("tcp_flag_syn_cnt", 2);
        m.put("direction_seq", List.of(0, 1, 0, 1));
        m.put("packet_length_seq", List.of(100, 200, 100, 300));
        m.put("packet_payload_length_seq", List.of(80, 180, 80, 280));
        m.put("payload_bytes_stored", 620);
        m.put("payload_bytes_total", 1240);
        return m;
    }

    @Test
    void resolveExpandsDependencyClosureWithoutCycles() {
        FeatureSelectionPlan plan = planOf("signed_packet_length_seq");
        assertTrue(plan.selectedFeatureIds().contains("packet_length_seq"));
        assertTrue(plan.selectedFeatureIds().contains("direction_seq"));
        assertTrue(plan.selectedFeatureIds().contains("initiator_relative_direction_seq"));
    }

    @Test
    void resolveRejectsUnknownFeature() {
        Map<String, List<String>> cat = new HashMap<>();
        for (String id : CATALOG) {
            cat.put(id, List.of());
        }
        assertThrows(IllegalArgumentException.class, () ->
                FeatureSelectionPlan.resolve("fs-v1", List.of("not-a-feature"), cat, Map.of()));
    }

    @Test
    void resolveRejectsCycle() {
        Map<String, List<String>> cat = Map.of("a", List.of(), "b", List.of());
        assertThrows(IllegalArgumentException.class, () ->
                FeatureSelectionPlan.resolve("fs-v1", List.of("a"), cat, Map.of("a", List.of("b"), "b", List.of("a"))));
    }

    @Test
    void calculateOutputsOnlyExactSetPlusDerived() {
        FeatureSelectionPlan plan = planOf("pktlen_mean", "initiator_relative_direction_seq");
        SelectedFeatureCalculator calc = new SelectedFeatureCalculator(catalog);
        SelectedFeatureCalculator.SelectedFeatureResult r = calc.calculate(raw(), List.of(), plan, plan.selectionHash());
        assertTrue(r.features().containsKey("pktlen_mean"));
        assertTrue(r.features().containsKey("initiator_relative_direction_seq"));
        assertFalse(r.features().containsKey("iat_mean_ms"), "unselected calculator must not be invoked");
    }

    @Test
    void calculateDeterministicFeatureHash() {
        FeatureSelectionPlan plan = planOf("pktlen_mean", "iat_mean_ms");
        SelectedFeatureCalculator calc = new SelectedFeatureCalculator(catalog);
        SelectedFeatureCalculator.SelectedFeatureResult a = calc.calculate(raw(), List.of(), plan, plan.selectionHash());
        SelectedFeatureCalculator.SelectedFeatureResult b = calc.calculate(raw(), List.of(), plan, plan.selectionHash());
        assertEquals(a.featureHash(), b.featureHash());
    }

    @Test
    void calculateSelectionHashMismatchFailsClosed() {
        FeatureSelectionPlan plan = planOf("pktlen_mean");
        SelectedFeatureCalculator calc = new SelectedFeatureCalculator(catalog);
        assertThrows(IllegalStateException.class, () ->
                calc.calculate(raw(), List.of(), plan, "wrong-hash"));
    }

    @Test
    void calculateRequiredMissingThrows() {
        // tcp_flag_ack_cnt 在 catalog 内但输入缺失 → required 缺失 ERROR 语义
        Map<String, List<String>> cat = new HashMap<>();
        for (String id : CATALOG) {
            cat.put(id, List.of());
        }
        cat.put("tcp_flag_ack_cnt", List.of());
        FeatureSelectionPlan p2 = FeatureSelectionPlan.resolve("fs-v1", List.of("tcp_flag_ack_cnt"), cat, Map.of());
        SelectedFeatureCalculator calc = new SelectedFeatureCalculator(catalog);
        assertThrows(IllegalStateException.class, () -> calc.calculate(raw(), List.of(), p2, p2.selectionHash()));
    }

    @Test
    void calculateOptionalMissingMarksPartial() {
        FeatureSelectionPlan plan = planOf("payload_histogram");
        SelectedFeatureCalculator calc = new SelectedFeatureCalculator(catalog);
        SelectedFeatureCalculator.SelectedFeatureResult r = calc.calculate(raw(), List.of(), plan, plan.selectionHash());
        assertTrue(r.partial());
        assertTrue(r.missingFields().contains("payload_histogram"));
        // 缺失性掩码显式标记,不伪造全零
        assertTrue(r.features().containsKey("modality_missingness_mask"));
    }

    @Test
    void deriveInitiatorDirectionAndSignedLength() {
        FeatureSelectionPlan plan = planOf("signed_packet_length_seq");
        SelectedFeatureCalculator calc = new SelectedFeatureCalculator(catalog);
        SelectedFeatureCalculator.SelectedFeatureResult r = calc.calculate(raw(), List.of(), plan, plan.selectionHash());
        List<?> dir = (List<?>) r.features().get("initiator_relative_direction_seq");
        assertEquals(List.of(1, 0, 1, 0), dir);
        List<?> signed = (List<?>) r.features().get("signed_packet_length_seq");
        assertEquals(Arrays.asList(100L, -200L, 100L, -300L), signed);
    }

    @Test
    void deriveBurstAndPayloadPresence() {
        FeatureSelectionPlan plan = planOf("directional_burst_count", "payload_presence_fraction");
        SelectedFeatureCalculator calc = new SelectedFeatureCalculator(catalog);
        SelectedFeatureCalculator.SelectedFeatureResult r = calc.calculate(raw(), List.of(), plan, plan.selectionHash());
        assertEquals(2L, r.features().get("directional_burst_count"));
        assertEquals(0.5, (Double) r.features().get("payload_presence_fraction"), 1e-9);
    }

    @Test
    void zeroExtraCallsForUnselected() {
        // 未选 burst 派生:输出不得包含任何 burst 键
        FeatureSelectionPlan plan = planOf("pktlen_mean");
        SelectedFeatureCalculator calc = new SelectedFeatureCalculator(catalog);
        SelectedFeatureCalculator.SelectedFeatureResult r = calc.calculate(raw(), List.of(), plan, plan.selectionHash());
        assertFalse(r.features().containsKey("directional_burst_count"));
        assertFalse(r.features().containsKey("signed_packet_length_seq"));
    }

    static {
        // 保持 CATALOG 列表可读(避免未用告警)
        assertEquals(16, CATALOG.size());
        assertEquals(0, new ArrayList<>().size());
    }
}
