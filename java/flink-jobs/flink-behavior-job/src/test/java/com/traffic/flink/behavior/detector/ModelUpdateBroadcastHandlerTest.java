package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.MinioModelLoader;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.streaming.api.operators.co.CoBroadcastWithNonKeyedOperator;
import org.apache.flink.streaming.runtime.streamrecord.StreamRecord;
import org.apache.flink.streaming.util.BroadcastOperatorTestHarness;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.List;
import java.util.Map;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ModelUpdateBroadcastHandlerTest {

    @TempDir
    Path modelCache;

    @Test
    void rollbackActivationUsesTheHotReloadPath() {
        assertTrue(ModelUpdateBroadcastHandler.isActivationAction("rollback-activated"));
        assertTrue(ModelUpdateBroadcastHandler.isActivationAction("activated"));
        assertFalse(ModelUpdateBroadcastHandler.isActivationAction("registered"));
    }

    @Test
    void producerEventContractPreservesStableEventId() {
        ModelUpdateEvent event = ModelUpdateEvent.fromJson(("{"
                + "\"event_id\":\"evt-rollback-1\","
                + "\"schema_version\":1,"
                + "\"model_id\":\"model-1\","
                + "\"model_name\":\"scan\","
                + "\"model_type\":\"scan\","
                + "\"version\":\"v2\","
                + "\"artifact_uri\":\"s3://models/scan/v2.onnx\","
                + "\"action\":\"rollback-activated\"}").getBytes());

        assertEquals("evt-rollback-1", event.getEventId());
        assertEquals(1, event.getSchemaVersion());
        assertEquals("rollback-activated", event.getAction());
    }

    @Test
    void replayWithTheSameEventIdIsIgnored() {
        ModelUpdateBroadcastHandler.ModelUpdateState current =
                new ModelUpdateBroadcastHandler.ModelUpdateState(
                        "scan", "v2", "s3://models/scan/v2.onnx",
                        "rollback-activated", "evt-rollback-1", 1L);
        ModelUpdateEvent replay = new ModelUpdateEvent();
        replay.setEventId("evt-rollback-1");

        assertTrue(ModelUpdateBroadcastHandler.isDuplicateEvent(current, replay));

        replay.setEventId("evt-rollback-2");
        assertFalse(ModelUpdateBroadcastHandler.isDuplicateEvent(current, replay));
    }

    @Test
    void nonContiguousReplayIsIgnoredByRealBroadcastState() throws Exception {
        ModelUpdateBroadcastHandler function = new ModelUpdateBroadcastHandler();
        CoBroadcastWithNonKeyedOperator<FeatureStat, ModelUpdateEvent, FeatureStat> operator =
                new CoBroadcastWithNonKeyedOperator<>(function, List.of(
                        ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
                        ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE));
        try (BroadcastOperatorTestHarness<FeatureStat, ModelUpdateEvent, FeatureStat> harness =
                     new BroadcastOperatorTestHarness<>(operator, 4, 1, 0)) {
            harness.open();
            ModelUpdateEvent first = registration("event-a");
            ModelUpdateEvent second = registration("event-b");
            harness.processBroadcastElement(new StreamRecord<>(first));
            String firstKey = "tenant-a\u001fmodel-1\u001fevent-a";
            Long firstTimestamp = harness.getBroadcastState(
                    ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE).get(firstKey);
            assertTrue(firstTimestamp != null && firstTimestamp > 0);

            harness.processBroadcastElement(new StreamRecord<>(second));
            harness.processBroadcastElement(new StreamRecord<>(first));

            assertEquals(firstTimestamp, harness.getBroadcastState(
                    ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE).get(firstKey));
            int count = 0;
            for (java.util.Map.Entry<String, Long> ignored : harness.getBroadcastState(
                    ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE).entries()) {
                count++;
            }
            assertEquals(2, count);
        }
    }

    @Test
    void runtimeOpenCreatesRegistryAndAppliesRealArtifact() throws Exception {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .enabledModels(Set.of())
                .modelReloadIntervalMs(0)
                .modelPath(modelCache.toString())
                .build();
        ModelUpdateBroadcastHandler function = new ModelUpdateBroadcastHandler(config);
        CoBroadcastWithNonKeyedOperator<FeatureStat, ModelUpdateEvent, FeatureStat> operator =
                new CoBroadcastWithNonKeyedOperator<>(function, List.of(
                        ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
                        ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE));

        Path fixture = Paths.get("../../../tests/fixtures/model-management/model.json")
                .toAbsolutePath().normalize();
        ModelUpdateEvent event = new ModelUpdateEvent();
        event.setEventId("runtime-apply-1");
        event.setTenantId("tenant-runtime");
        event.setModelId("model-runtime");
        event.setModelName("runtime-model");
        event.setModelType("xgboost");
        event.setVersion("v1");
        event.setArtifactUri(fixture.toUri().toString());
        event.setAction("activated");
        event.setMetrics(Map.of(
                "artifact_sha256", MinioModelLoader.sha256(fixture),
                "threshold", 0.5));

        try (BroadcastOperatorTestHarness<FeatureStat, ModelUpdateEvent, FeatureStat> harness =
                     new BroadcastOperatorTestHarness<>(operator, 4, 1, 0)) {
            harness.open();
            harness.processBroadcastElement(new StreamRecord<>(event));

            ModelUpdateBroadcastHandler.ModelUpdateState active = harness.getBroadcastState(
                    ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE)
                    .get("tenant-runtime\u001fmodel-runtime");
            assertTrue(active != null && !active.isPending());
            assertEquals("v1", active.getVersion());
        }
    }

    private static ModelUpdateEvent registration(String eventId) {
        ModelUpdateEvent event = new ModelUpdateEvent();
        event.setEventId(eventId);
        event.setTenantId("tenant-a");
        event.setModelId("model-1");
        event.setModelName("fraud");
        event.setVersion("v1");
        event.setAction("registered");
        return event;
    }
}
