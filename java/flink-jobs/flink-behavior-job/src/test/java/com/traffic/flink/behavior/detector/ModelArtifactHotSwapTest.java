package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.BehaviorModel;
import com.traffic.flink.behavior.model.MinioModelLoader;
import com.traffic.flink.behavior.model.ModelInferenceResult;
import com.traffic.proto.traffic.v1.FeatureStat;
import ml.dmlc.xgboost4j.java.Booster;
import ml.dmlc.xgboost4j.java.DMatrix;
import ml.dmlc.xgboost4j.java.XGBoost;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.io.TempDir;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.HashMap;
import java.util.Map;
import java.util.Set;

import static org.junit.jupiter.api.Assertions.*;

class ModelArtifactHotSwapTest {
    @TempDir Path tempDir;

    @Test
    void realArtifactIsVerifiedWarmedAndIsolatedByTenant() throws Exception {
        Path artifactDir = tempDir.resolve("artifact");
        Files.createDirectories(artifactDir);
        Path modelFile = artifactDir.resolve("model.json");
        Files.writeString(artifactDir.resolve("feature_columns.json"), "[\"pps\",\"bps\"]");

        DMatrix training = new DMatrix(new float[]{0, 0, 0, 1, 1, 0, 1, 1}, 4, 2, Float.NaN);
        training.setLabel(new float[]{0, 0, 1, 1});
        Map<String, Object> params = new HashMap<>();
        params.put("objective", "binary:logistic");
        params.put("max_depth", 2);
        params.put("eta", 1.0);
        Booster booster = XGBoost.train(training, params, 4, Map.of("train", training), null, null);
        booster.saveModel(modelFile.toString());
        booster.dispose();
        training.dispose();

        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .enabledModels(Set.of())
                .modelPath(tempDir.resolve("cache").toString())
                .modelReloadIntervalMs(0)
                .build();
        ModelRegistry registry = new ModelRegistry(config);
        String sha256 = MinioModelLoader.sha256(modelFile);
        ModelRegistry.ApplyReceipt receipt = registry.applyModelUpdate(
                "tenant-a", "model-1", "fraud", "xgboost", "v2",
                modelFile.toUri().toString(), sha256, 0.5f);

        assertTrue(receipt.isSwitched());
        assertEquals(sha256, receipt.getArtifactSha256());
        assertTrue(Float.isFinite(receipt.getWarmupScore()));
        assertTrue(registry.getModelsForTenant("tenant-a").containsKey("model-1"));
        assertFalse(registry.getModelsForTenant("tenant-b").containsKey("model-1"));
        assertEquals("v2", registry.getModelVersion("tenant-a", "model-1"));

        BehaviorModel model = registry.getModelsForTenant("tenant-a").get("model-1");
        ModelInferenceResult result = model.infer(FeatureStat.newBuilder().setPps(1).setBps(1).build());
        assertFalse(result.hasError());
        assertTrue(Float.isFinite(result.getTopScore()));

        assertThrows(IllegalStateException.class, () -> registry.applyModelUpdate(
                "tenant-a", "model-1", "fraud", "xgboost", "v3",
                modelFile.toUri().toString(), "00", 0.5f));
        assertEquals("v2", registry.getModelVersion("tenant-a", "model-1"));

        registry.removeTenantModel("tenant-a", "model-1");
        registry.close();
    }

    @Test
    void acceptanceFixtureLoadsWithProductionRuntime() throws Exception {
        Path fixture = Path.of("../../../tests/fixtures/model-management/model.json")
                .toAbsolutePath().normalize();
        Booster booster = XGBoost.loadModel(fixture.toString());
        assertEquals(2, booster.getNumFeature());
        DMatrix input = new DMatrix(new float[]{1, 1}, 1, 2, Float.NaN);
        float[][] prediction = booster.predict(input);
        assertTrue(Float.isFinite(prediction[0][0]));
        input.dispose();
        booster.dispose();
    }
}
