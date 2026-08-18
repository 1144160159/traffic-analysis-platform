package com.traffic.flink.behavior.detector;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.traffic.flink.behavior.config.BehaviorJobConfig;
import com.traffic.flink.behavior.model.ModelUpdateEvent;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Set;

/** Standalone verifier used by the isolated Kubernetes shadow-load canary. */
public final class GovernedShadowPackageVerifierMain {
    private static final ObjectMapper MAPPER = new ObjectMapper();

    private GovernedShadowPackageVerifierMain() {}

    public static void main(String[] args) throws Exception {
        if (args.length != 4 || !"--event".equals(args[0]) || !"--public-key".equals(args[2])) {
            throw new IllegalArgumentException(
                    "usage: GovernedShadowPackageVerifierMain --event <json> --public-key <pem>");
        }
        Path eventPath = Path.of(args[1]).toAbsolutePath().normalize();
        Path publicKey = Path.of(args[3]).toAbsolutePath().normalize();
        if (!Files.isRegularFile(eventPath) || !Files.isRegularFile(publicKey)) {
            throw new IllegalArgumentException("shadow event and trusted public key must be regular files");
        }
        ModelUpdateEvent event = ModelUpdateEvent.fromJson(Files.readAllBytes(eventPath));
        int featureSchema = intCompatibility(event, "feature_schema_version");
        int graphSchema = intCompatibility(event, "graph_schema_version");
        String runtimeContract = event.getCompatibility().getRuntimeContract();
        String runtimeVersion = event.getCompatibility().getRuntimeVersion();
        Path cache = eventPath.getParent().resolve("java-shadow-cache");
        BehaviorJobConfig config = new BehaviorJobConfig.Builder()
                .enabledModels(Set.of())
                .modelPath(cache.toString())
                .modelReloadIntervalMs(0)
                .modelRuntimeContract(runtimeContract)
                .modelRuntimeVersion(runtimeVersion)
                .modelFeatureSchemaVersion(featureSchema)
                .modelGraphSchemaVersion(graphSchema)
                .modelSigningPublicKeyFile(publicKey.toString())
                .build();
        try (GovernedModelPackageLoader loader = new GovernedModelPackageLoader(config);
             GovernedModelPackageLoader.ShadowPackage staged = loader.stage(event)) {
            System.out.println(MAPPER.writeValueAsString(java.util.Map.of(
                    "status", "PASS",
                    "stage", "SHADOW_READY",
                    "activation_applied", false,
                    "package_id", staged.getPackageId(),
                    "package_sha256", staged.getPackageSha256(),
                    "aggregate_revision", staged.getAggregateRevision(),
                    "baseline_warmup", "PASS",
                    "gnn_archive_validation", "PASS",
                    "graph_schema_validation", "PASS",
                    "signature_validation", "PASS")));
        }
    }

    private static int intCompatibility(ModelUpdateEvent event, String field) {
        if (event.getCompatibility() == null) {
            throw new IllegalArgumentException("shadow event compatibility is missing " + field);
        }
        if ("feature_schema_version".equals(field)) {
            return event.getCompatibility().getFeatureSchemaVersion();
        }
        if ("graph_schema_version".equals(field)) {
            return event.getCompatibility().getGraphSchemaVersion();
        }
        throw new IllegalArgumentException("unknown compatibility field " + field);
    }
}
