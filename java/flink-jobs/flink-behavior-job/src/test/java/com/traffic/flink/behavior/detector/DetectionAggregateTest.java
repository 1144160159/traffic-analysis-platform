package com.traffic.flink.behavior.detector;

import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

/** DetectionAggregate 守恒测试(oracle:ATC-AGG-*)。 */
class DetectionAggregateTest {

    private SelectedThreatDetector.DetectionOutcome out(SelectedThreatDetector.Disposition d) {
        return new SelectedThreatDetector.DetectionOutcome("in-1", "det", d, 0.9,
                List.of(), List.of(), "x");
    }

    @Test
    void aggregateCountsAreConserved() {
        List<SelectedThreatDetector.DetectionOutcome> outcomes = List.of(
                out(SelectedThreatDetector.Disposition.POSITIVE),
                out(SelectedThreatDetector.Disposition.NEGATIVE),
                out(SelectedThreatDetector.Disposition.ERROR));
        DetectionAggregate.AggregatedDetection a = DetectionAggregate.aggregate("in-1", 3, outcomes);
        assertEquals(3, a.detectorCount());
        assertEquals(1, a.positive());
        assertEquals(1, a.negative());
        assertEquals(1, a.error());
        assertTrue(a.hasTrustedPositive());
        assertFalse(a.allExplicitNegative());
    }

    @Test
    void aggregateRejectsOutcomeGap() {
        assertThrows(IllegalStateException.class, () ->
                DetectionAggregate.aggregate("in-1", 3, List.of(out(SelectedThreatDetector.Disposition.NEGATIVE))));
    }

    @Test
    void aggregateDeterministicHash() {
        List<SelectedThreatDetector.DetectionOutcome> o = List.of(out(SelectedThreatDetector.Disposition.NEGATIVE));
        assertEquals(DetectionAggregate.aggregate("in-1", 1, o).aggregateHash(),
                DetectionAggregate.aggregate("in-1", 1, o).aggregateHash());
    }

    @Test
    void allExplicitNegativeFlag() {
        List<SelectedThreatDetector.DetectionOutcome> o = List.of(out(SelectedThreatDetector.Disposition.NEGATIVE));
        DetectionAggregate.AggregatedDetection a = DetectionAggregate.aggregate("in-1", 1, o);
        assertTrue(a.allExplicitNegative());
        assertFalse(a.hasTrustedPositive());
    }

    @Test
    void runCountsConservation() {
        List<DetectionAggregate.AggregatedDetection> per = List.of(
                DetectionAggregate.aggregate("in-1", 1, List.of(out(SelectedThreatDetector.Disposition.POSITIVE))),
                DetectionAggregate.aggregate("in-2", 1, List.of(out(SelectedThreatDetector.Disposition.NEGATIVE))),
                DetectionAggregate.aggregate("in-3", 1, List.of(out(SelectedThreatDetector.Disposition.ERROR))));
        DetectionAggregate.RunCounts rc = DetectionAggregate.runCounts(per);
        assertEquals(3, rc.inputs);
        assertEquals(1, rc.positiveInputs);
        assertEquals(1, rc.allNegativeInputs);
        assertEquals(1, rc.errorInputs);
    }
}
