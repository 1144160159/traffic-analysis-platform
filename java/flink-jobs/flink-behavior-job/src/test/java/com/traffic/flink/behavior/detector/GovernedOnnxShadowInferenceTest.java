package com.traffic.flink.behavior.detector;

import ai.onnxruntime.OrtEnvironment;
import ai.onnxruntime.OrtSession;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Base64;

import static org.assertj.core.api.Assertions.assertThat;

class GovernedOnnxShadowInferenceTest {
    // Generated in the pinned M08 Kubernetes trainer image with XGBoost 2.0.3,
    // onnxmltools 1.16.0 and opset 15. It exposes classifier labels plus a
    // sequence/map probability output, which exercises the production parser.
    private static final String BINARY_CLASSIFIER_ONNX =
            "CAgSC09ubnhNTFRvb2xzGgYxLjE2LjAiBm9ubnhtbCgAMgA68QQK/QMKC2Zsb2F0X2lucHV0EgVsYWJlbBINcHJvYmFiaWxpdGllcxoWVHJlZUVuc2VtYmxlQ2xhc3NpZmllciIWVHJlZUVuc2VtYmxlQ2xhc3NpZmllcioVCgtiYXNlX3ZhbHVlcz0AAAA/oAEGKhIKCWNsYXNzX2lkc0AAQACgAQcqFgoNY2xhc3Nfbm9kZWlkc0AAQACgAQcqFgoNY2xhc3NfdHJlZWlkc0AAQAGgAQcqHAoNY2xhc3Nfd2VpZ2h0cz0AAAAAPQAAAACgAQYqGwoSY2xhc3NsYWJlbHNfaW50NjRzQABAAaABByobChJub2Rlc19mYWxzZW5vZGVpZHNAAEAAoAEHKhkKEG5vZGVzX2ZlYXR1cmVpZHNAAEAAoAEHKigKH25vZGVzX21pc3NpbmdfdmFsdWVfdHJhY2tzX3RydWVAAEAAoAEHKhwKC25vZGVzX21vZGVzSgRMRUFGSgRMRUFGoAEIKhYKDW5vZGVzX25vZGVpZHNAAEAAoAEHKhYKDW5vZGVzX3RyZWVpZHNAAEABoAEHKhoKEW5vZGVzX3RydWVub2RlaWRzQABAAKABByobCgxub2Rlc192YWx1ZXM9AAAAAD0AAAAAoAEGKh0KDnBvc3RfdHJhbnNmb3JtIghMT0dJU1RJQ6ABAzoKYWkub25ueC5tbBIgMDIwNTE0Y2Q4ODllNGZiNjg4MmRiMWNmYjQ2MmEzZWNaGwoLZmxvYXRfaW5wdXQSDAoKCAESBgoACgIIAmIRCgVsYWJlbBIICgYIBxICCgBiHQoNcHJvYmFiaWxpdGllcxIMCgoIARIGCgAKAggCQg4KCmFpLm9ubngubWwQAQ==";

    @TempDir
    Path temp;

    @Test
    void extractsPositiveProbabilityFromRealOnnxClassifierOutput() throws Exception {
        Path model = temp.resolve("baseline-model.onnx");
        Files.write(model, Base64.getDecoder().decode(BINARY_CLASSIFIER_ONNX));
        OrtEnvironment environment = OrtEnvironment.getEnvironment();
        try (OrtSession session = environment.createSession(
                model.toString(), new OrtSession.SessionOptions())) {
            float score = GovernedModelPackageLoader.runBaseline(
                    environment, session, "float_input", new float[]{1.0f, 1.0f});
            assertThat(score).isFinite().isBetween(0.0f, 1.0f);
        }
    }
}
