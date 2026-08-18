package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.MinioModelLoader;
import com.traffic.flink.behavior.model.ModelUpdateAppliedAck;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import com.traffic.flink.behavior.model.ShadowEvaluationRequest;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import org.apache.flink.runtime.checkpoint.OperatorSubtaskState;
import org.apache.flink.streaming.api.operators.co.CoBroadcastWithNonKeyedOperator;
import org.apache.flink.streaming.runtime.streamrecord.StreamRecord;
import org.apache.flink.streaming.util.BroadcastOperatorTestHarness;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Path;
import java.nio.file.Paths;
import java.util.List;
import java.util.Queue;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
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
	void savepointRestoreReemitsExactRollbackAcknowledgement() throws Exception {
		BehaviorJobConfig config = new BehaviorJobConfig.Builder()
				.enabledModels(Set.of())
				.modelReloadIntervalMs(0)
				.modelPath(modelCache.toString())
				.modelHotUpdateEnabled(true)
				.modelConsumerDeploymentId("behavior-job-r1")
				.modelConsumerProfileSha256("a".repeat(64))
				.build();
		Path fixture = Paths.get("../../../tests/fixtures/model-management/model.json")
				.toAbsolutePath().normalize();
		String artifactSha256 = MinioModelLoader.sha256(fixture);
		ModelUpdateEvent event = new ModelUpdateEvent();
		event.setEventId("rollback-savepoint-event-1");
		event.setTenantId("tenant-savepoint");
		event.setModelId("model-savepoint");
		event.setModelName("rollback-model");
		event.setModelType("xgboost");
		event.setVersion("v1");
		event.setArtifactUri(fixture.toUri().toString());
		event.setAction("rollback-activated");
		event.setRollbackId("11111111-1111-4111-8111-111111111111");
		event.setRollbackPhase("attempt");
		event.setConsumerDeploymentId("behavior-job-r1");
		event.setConsumerProfileSha256("a".repeat(64));
		ModelUpdateEvent.Metrics metrics = new ModelUpdateEvent.Metrics();
		metrics.setArtifactSha256(artifactSha256);
		metrics.setThreshold(0.5f);
		event.setMetrics(metrics);

		OperatorSubtaskState savepoint;
		ModelUpdateBroadcastHandler firstFunction = new ModelUpdateBroadcastHandler(config);
		CoBroadcastWithNonKeyedOperator<FeatureStat, ModelUpdateEvent, FeatureStat> firstOperator =
				new CoBroadcastWithNonKeyedOperator<>(firstFunction, List.of(
						ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
						ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE,
						ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE));
		try (BroadcastOperatorTestHarness<FeatureStat, ModelUpdateEvent, FeatureStat> first =
					 new BroadcastOperatorTestHarness<>(firstOperator, 1, 1, 0)) {
			first.open();
			first.processBroadcastElement(new StreamRecord<>(event));
			savepoint = first.snapshot(7L, 7L);
		}

		ModelUpdateBroadcastHandler restoredFunction = new ModelUpdateBroadcastHandler(config);
		CoBroadcastWithNonKeyedOperator<FeatureStat, ModelUpdateEvent, FeatureStat> restoredOperator =
				new CoBroadcastWithNonKeyedOperator<>(restoredFunction, List.of(
						ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
						ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE,
						ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE));
		try (BroadcastOperatorTestHarness<FeatureStat, ModelUpdateEvent, FeatureStat> restored =
					 new BroadcastOperatorTestHarness<>(restoredOperator, 1, 1, 0)) {
			restored.initializeState(savepoint);
			restored.open();
			restored.processBroadcastElement(new StreamRecord<>(event));
			Queue<StreamRecord<ModelUpdateAppliedAck>> acknowledgements = restored.getSideOutput(
					ModelUpdateBroadcastHandler.MODEL_UPDATE_ACK_TAG);
			assertEquals(1, acknowledgements.size());
			ModelUpdateAppliedAck replay = acknowledgements.remove().getValue();
			assertEquals("applied", replay.status);
			assertEquals(artifactSha256, replay.artifactSha256);
			assertEquals(event.getRollbackId(), replay.rollbackId);
			assertEquals("attempt", replay.rollbackPhase);
			assertEquals("behavior-job-r1", replay.consumerDeploymentId);
		}
	}

	@Test
	void rollbackForAnotherConsumerIdentityFailsClosedBeforeRuntimeSwap() throws Exception {
		BehaviorJobConfig config = new BehaviorJobConfig.Builder()
				.enabledModels(Set.of())
				.modelReloadIntervalMs(0)
				.modelPath(modelCache.toString())
				.modelHotUpdateEnabled(true)
				.modelConsumerDeploymentId("behavior-job-r1")
				.modelConsumerProfileSha256("a".repeat(64))
				.build();
		ModelUpdateEvent event = new ModelUpdateEvent();
		event.setSchemaVersion(2);
		event.setEventId("rollback-wrong-consumer-1");
		event.setTenantId("tenant-runtime");
		event.setModelId("model-runtime");
		event.setModelName("runtime-model");
		event.setModelType("xgboost");
		event.setVersion("v1");
		event.setArtifactUri(modelCache.resolve("must-not-be-opened.onnx").toUri().toString());
		event.setAction("rollback-activated");
		event.setRollbackId("22222222-2222-4222-8222-222222222222");
		event.setRollbackPhase("attempt");
		event.setConsumerDeploymentId("behavior-job-r2");
		event.setConsumerProfileSha256("b".repeat(64));

		ModelUpdateBroadcastHandler function = new ModelUpdateBroadcastHandler(config);
		CoBroadcastWithNonKeyedOperator<FeatureStat, ModelUpdateEvent, FeatureStat> operator =
				new CoBroadcastWithNonKeyedOperator<>(function, List.of(
						ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
						ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE,
						ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE));
		try (BroadcastOperatorTestHarness<FeatureStat, ModelUpdateEvent, FeatureStat> harness =
				 new BroadcastOperatorTestHarness<>(operator, 1, 1, 0)) {
			harness.open();
			harness.processBroadcastElement(new StreamRecord<>(event));

			Queue<StreamRecord<ModelUpdateAppliedAck>> acknowledgements = harness.getSideOutput(
					ModelUpdateBroadcastHandler.MODEL_UPDATE_ACK_TAG);
			assertEquals(1, acknowledgements.size());
			ModelUpdateAppliedAck rejected = acknowledgements.remove().getValue();
			assertEquals("failed", rejected.status);
			assertTrue(rejected.error.contains("consumer deployment or profile differs"));
			assertNull(harness.getBroadcastState(ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE)
					.get("tenant-runtime\u001fmodel-runtime"));
		}
	}

    @Test
    void shadowRevisionClassificationIsMonotonicAndDigestBound() {
        ModelUpdateBroadcastHandler.ModelUpdateState current =
                new ModelUpdateBroadcastHandler.ModelUpdateState(
                        "governed", "v2", "s3://models/package/manifest.json",
                        "shadow-load", "event-2", 1L);
        current.setAggregateRevision(12L);
        current.setPackageSha256("a".repeat(64));
        current.setStage("SHADOW_READY");

        ModelUpdateEvent candidate = new ModelUpdateEvent();
        candidate.setAggregateRevision(11L);
        candidate.setPackageSha256("a".repeat(64));
        assertEquals("stale", ModelUpdateBroadcastHandler.shadowDisposition(current, candidate));

        candidate.setAggregateRevision(12L);
        assertEquals("duplicate", ModelUpdateBroadcastHandler.shadowDisposition(current, candidate));

        candidate.setPackageSha256("b".repeat(64));
        assertEquals("stale", ModelUpdateBroadcastHandler.shadowDisposition(current, candidate));

        candidate.setAggregateRevision(13L);
        assertEquals("new", ModelUpdateBroadcastHandler.shadowDisposition(current, candidate));
    }

    @Test
    void nonContiguousReplayIsIgnoredByRealBroadcastState() throws Exception {
        ModelUpdateBroadcastHandler function = new ModelUpdateBroadcastHandler();
        CoBroadcastWithNonKeyedOperator<FeatureStat, ModelUpdateEvent, FeatureStat> operator =
                new CoBroadcastWithNonKeyedOperator<>(function, List.of(
                        ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
                        ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE,
                        ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE));
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
                .modelHotUpdateEnabled(true)
                .build();
        ModelUpdateBroadcastHandler function = new ModelUpdateBroadcastHandler(config);
        CoBroadcastWithNonKeyedOperator<FeatureStat, ModelUpdateEvent, FeatureStat> operator =
                new CoBroadcastWithNonKeyedOperator<>(function, List.of(
                        ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
                        ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE,
                        ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE));

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
        ModelUpdateEvent.Metrics metrics = new ModelUpdateEvent.Metrics();
        metrics.setArtifactSha256(MinioModelLoader.sha256(fixture));
        metrics.setThreshold(0.5f);
        event.setMetrics(metrics);

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

    @Test
    void checkpointedShadowEventIsBoundToFeatureForAnyDownstreamSlot() throws Exception {
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .enabledModels(Set.of())
                .modelPath(modelCache.toString())
                .modelShadowEvaluationEnabled(true)
                .build();
        ModelUpdateBroadcastHandler function = new ModelUpdateBroadcastHandler(config);
        CoBroadcastWithNonKeyedOperator<FeatureStat, ModelUpdateEvent, FeatureStat> operator =
                new CoBroadcastWithNonKeyedOperator<>(function, List.of(
                        ModelUpdateBroadcastHandler.MODEL_UPDATE_STATE,
                        ModelUpdateBroadcastHandler.PROCESSED_EVENT_STATE,
                        ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE));
        try (BroadcastOperatorTestHarness<FeatureStat, ModelUpdateEvent, FeatureStat> harness =
                     new BroadcastOperatorTestHarness<>(operator, 4, 1, 0)) {
            harness.open();
            ModelUpdateEvent candidate = registration("shadow-event-1");
            candidate.setAction("shadow-load");
            candidate.setPackageId("package-1");
            candidate.setPackageSha256("a".repeat(64));
            candidate.setAggregateRevision(3L);
            harness.getBroadcastState(ModelUpdateBroadcastHandler.SHADOW_PACKAGE_EVENT_STATE)
                    .put("tenant-a\u001fmodel-1", candidate);
            FeatureStat feature = FeatureStat.newBuilder()
                    .setHeader(EventHeader.newBuilder()
                            .setTenantId("tenant-a").setEventId("feature-1"))
                    .setObjectId("flow-1")
                    .build();

            harness.processElement(new StreamRecord<>(feature));

            Queue<StreamRecord<ShadowEvaluationRequest>> requests = harness.getSideOutput(
                    ModelUpdateBroadcastHandler.SHADOW_EVALUATION_REQUEST_TAG);
            assertEquals(1, requests.size());
            ShadowEvaluationRequest request = requests.remove().getValue();
            assertEquals(feature, request.getFeature());
            assertEquals(1, request.getCandidates().size());
            assertEquals("package-1", request.getCandidates().get(0).getPackageId());
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
