package com.traffic.flink.behavior;

import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.ModelConsumerProfile;
import com.traffic.flink.behavior.model.ModelUpdateEvent;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.api.java.typeutils.PojoTypeInfo;
import org.apache.flink.streaming.api.environment.StreamExecutionEnvironment;
import org.junit.jupiter.api.Test;

import java.security.KeyPairGenerator;
import java.util.Base64;
import java.util.Set;
import java.util.stream.Collectors;

import static org.junit.jupiter.api.Assertions.assertDoesNotThrow;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.junit.jupiter.api.Assertions.assertThrows;

class ModelUpdateConsumerJobTest {
    private static BehaviorJobConfig.Builder validBuilder() {
        return new BehaviorJobConfig.Builder()
                .detectionMode("off")
                .modelUpdateConsumerEnabled(true)
                .modelHotUpdateEnabled(false)
                .modelConsumerDeploymentId("behavior-shadow-r1")
                .modelConsumerProfileSha256("a".repeat(64))
                .modelRuntimeContract("traffic.behavior.inference.v1")
                .modelRuntimeVersion("1.0.0")
                .modelFeatureSchemaVersion(1)
                .modelGraphSchemaVersion(1)
                .modelSigningPublicKeyFile("/trust/model-signing.pem")
                .parallelism(4);
    }

    @Test
    void acceptsConsumerFirstWithoutDetectionOrActivation() {
        assertDoesNotThrow(() -> ModelUpdateConsumerJob.validateConsumerOnlyConfig(validBuilder().build()));
    }

    @Test
    void rejectsActivationAndDetectionServingModes() {
        assertThrows(IllegalArgumentException.class, () ->
                ModelUpdateConsumerJob.validateConsumerOnlyConfig(
                        validBuilder().modelHotUpdateEnabled(true).build()));
        assertThrows(IllegalArgumentException.class, () ->
                ModelUpdateConsumerJob.validateConsumerOnlyConfig(
                        validBuilder().detectionMode("dynamic").build()));
    }

    @Test
    void graphContainsOnlyShadowConsumerAndAckOperators() throws Exception {
        String publicKeyBase64 = Base64.getEncoder().encodeToString(pemPublicKey());
        BehaviorJobConfig provisional = validBuilder()
                .modelSigningPublicKeyFile("")
                .modelSigningPublicKeyPemBase64(publicKeyBase64)
                .build();
        String profile = ModelConsumerProfile.calculateSha256(provisional);
        BehaviorJobConfig config = validBuilder()
                .modelConsumerProfileSha256(profile)
                .modelSigningPublicKeyFile("")
                .modelSigningPublicKeyPemBase64(publicKeyBase64)
                .build();
        StreamExecutionEnvironment env = StreamExecutionEnvironment.getExecutionEnvironment();
        env.setParallelism(config.getParallelism());

        ModelUpdateConsumerJob.buildPipeline(env, config);

        Set<String> names = env.getStreamGraph().getStreamNodes().stream()
                .map(node -> node.getOperatorName().toLowerCase())
                .collect(Collectors.toSet());
        assertTrue(names.stream().anyMatch(name -> name.contains("model-updates shadow-only")));
        assertTrue(names.stream().anyMatch(name -> name.contains("shadow loader")));
        assertTrue(names.stream().anyMatch(name -> name.contains("model shadow ack")));
        assertFalse(names.stream().anyMatch(name -> name.contains("clickhouse")));
        assertFalse(names.stream().anyMatch(name -> name.contains("behavior detector")));
    }

    @Test
    void modelUpdateEventUsesPojoStateInsteadOfGenericKryoMaps() {
        TypeInformation<ModelUpdateEvent> type = TypeInformation.of(ModelUpdateEvent.class);
        assertTrue(type instanceof PojoTypeInfo,
                () -> "ModelUpdateEvent must be a stable Flink POJO type, got " + type);
        PojoTypeInfo<?> pojo = (PojoTypeInfo<?>) type;
        assertTrue(pojo.getTypeAt("compatibility") instanceof PojoTypeInfo);
        assertTrue(pojo.getTypeAt("metrics") instanceof PojoTypeInfo);
    }

    private static byte[] pemPublicKey() throws Exception {
        byte[] der = KeyPairGenerator.getInstance("Ed25519").generateKeyPair().getPublic().getEncoded();
        String pem = "-----BEGIN PUBLIC KEY-----\n"
                + Base64.getMimeEncoder(64, new byte[]{'\n'}).encodeToString(der)
                + "\n-----END PUBLIC KEY-----\n";
        return pem.getBytes(java.nio.charset.StandardCharsets.US_ASCII);
    }
}
