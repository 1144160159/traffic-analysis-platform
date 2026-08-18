package com.traffic.flink.behavior.detector;

import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

/**
 * EncryptedTrafficRecognizer 与 SelectedThreatDetector typed outcome 测试
 * (oracle: ATC-ENC 与 ATC-DET 系列)。
 */
class TypedOutcomeTest {

    @Test
    void recognizerPlaintextReturnsNotEncrypted() {
        EncryptedTrafficRecognizer r = new EncryptedTrafficRecognizer();
        EncryptedTrafficRecognizer.RecognitionOutcome o = r.recognize("in-1",
                new EncryptedTrafficRecognizer.RecognitionSelection("enc@v2", "sha"),
                Map.of("is_encrypted", 0));
        assertEquals(EncryptedTrafficRecognizer.RecognitionState.NOT_ENCRYPTED, o.state());
    }

    @Test
    void recognizerInsufficientFeaturesReturnsUnknown() {
        EncryptedTrafficRecognizer r = new EncryptedTrafficRecognizer();
        EncryptedTrafficRecognizer.RecognitionOutcome o = r.recognize("in-1",
                new EncryptedTrafficRecognizer.RecognitionSelection("enc@v2", "sha"),
                Map.of("is_encrypted", 1));
        assertEquals(EncryptedTrafficRecognizer.RecognitionState.UNKNOWN, o.state());
    }

    @Test
    void recognizerTlsReturnsRecognized() {
        EncryptedTrafficRecognizer r = new EncryptedTrafficRecognizer();
        EncryptedTrafficRecognizer.RecognitionOutcome o = r.recognize("in-1",
                new EncryptedTrafficRecognizer.RecognitionSelection("enc@v2", "sha"),
                Map.of("is_encrypted", 1, "tls_version", "1.3", "ja3", "abc123"));
        assertEquals(EncryptedTrafficRecognizer.RecognitionState.RECOGNIZED, o.state());
        assertEquals("enc@v2", o.recognitionModelId());
    }

    @Test
    void detectorRequiresExplicitOutcomePerDetector() {
        SelectedThreatDetector.ThreatDetectionSelection sel =
                new SelectedThreatDetector.ThreatDetectionSelection(
                        List.of("rule-scan@v3", "behavior-known@v7"), Map.of());
        List<SelectedThreatDetector.DetectionOutcome> outcomes = SelectedThreatDetector.detect(
                "in-1", sel, Map.of("pktlen_mean", 64.0), "RECOGNIZED",
                (detectorId, inputId, env) -> Map.of("positive", detectorId.contains("behavior"),
                        "score", 0.88, "labels", List.of("scan"), "evidence", List.of("rule:hit")));
        assertEquals(2, outcomes.size(), "每 input×required detector 一个 outcome");
        assertEquals(SelectedThreatDetector.Disposition.NEGATIVE, outcomes.get(0).disposition());
        assertEquals(SelectedThreatDetector.Disposition.POSITIVE, outcomes.get(1).disposition());
    }

    @Test
    void detectorIncompatibleRecognitionYieldsExplicitOutcome() {
        SelectedThreatDetector.ThreatDetectionSelection sel =
                new SelectedThreatDetector.ThreatDetectionSelection(List.of("rule-scan@v3"), Map.of());
        List<SelectedThreatDetector.DetectionOutcome> outcomes = SelectedThreatDetector.detect(
                "in-1", sel, Map.of(), "INCOMPATIBLE", (d, i, e) -> { throw new AssertionError("must not run"); });
        assertEquals(1, outcomes.size());
        assertEquals(SelectedThreatDetector.Disposition.INCOMPATIBLE, outcomes.get(0).disposition());
    }

    @Test
    void detectorExecutorExceptionYieldsErrorNotEmpty() {
        SelectedThreatDetector.ThreatDetectionSelection sel =
                new SelectedThreatDetector.ThreatDetectionSelection(List.of("rule-scan@v3"), Map.of());
        List<SelectedThreatDetector.DetectionOutcome> outcomes = SelectedThreatDetector.detect(
                "in-1", sel, Map.of(), "RECOGNIZED", (d, i, e) -> { throw new RuntimeException("boom"); });
        assertEquals(1, outcomes.size());
        assertEquals(SelectedThreatDetector.Disposition.ERROR, outcomes.get(0).disposition());
        assertEquals("EXECUTOR_EXCEPTION", outcomes.get(0).reasonCode());
    }
}
