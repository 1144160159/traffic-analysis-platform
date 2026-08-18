package com.traffic.flink.behavior.detector;

import com.traffic.flink.behavior.model.ModelInferenceResult;
import com.traffic.flink.common.DeterministicId;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FiveTuple;

import java.util.List;

/** Builds the canonical behavior-detection envelope from a validated feature. */
final class BehaviorDetectionEventFactory {

    static final String EVENT_TYPE = "traffic.detection.behavior.v1";
    static final String SCHEMA_VERSION = "1";
    static final String PRODUCER = "flink-behavior-job";

    private BehaviorDetectionEventFactory() {
    }

    static DetectionBehavior build(
            FeatureStat input,
            ModelInferenceResult result,
            String modelVersion,
            long producedAt) {
        validate(input, result, modelVersion, producedAt);

        EventHeader inputHeader = input.getHeader();
        long eventTime = input.getTs();
        long ingestTime = inputHeader.getIngestTs() > 0
                ? inputHeader.getIngestTs()
                : eventTime;
        String eventId = eventId(input, result, modelVersion);

        EventHeader header = EventHeader.newBuilder()
                .setEventId(eventId)
                .setTenantId(inputHeader.getTenantId())
                .setRunId(inputHeader.getRunId())
                .setProbeId(inputHeader.getProbeId())
                .setFeatureSetId(inputHeader.getFeatureSetId())
                .setTraceId(inputHeader.getTraceId().isEmpty()
                        ? inputHeader.getEventId() : inputHeader.getTraceId())
                .setCausationId(inputHeader.getEventId())
                .setCorrelationId(inputHeader.getCorrelationId().isEmpty()
                        ? input.getCommunityId() : inputHeader.getCorrelationId())
                .setEventTs(eventTime)
                .setIngestTs(ingestTime)
                .setEventType(EVENT_TYPE)
                .setSchemaVersion(SCHEMA_VERSION)
                .setAggregateType("detection")
                .setAggregateId(input.getObjectId())
                .setAggregateVersion(1)
                .setOccurredAt(eventTime)
                .setProducedAt(producedAt)
                .setIdempotencyKey(eventId)
                .setProducer(PRODUCER)
                .build();

        return DetectionBehavior.newBuilder()
                .setHeader(header)
                .setModelVersion(modelVersion)
                .setCommunityId(input.getCommunityId())
                .setObjectType(input.getObjectType())
                .setObjectId(input.getObjectId())
                .setTs(eventTime)
                .setTopLabel(result.getTopLabel())
                .setTopScore(result.getTopScore())
                .setTuple(input.getTuple())
                .addAllEvidenceIds(input.getEvidenceIdsList())
                .addAllLabels(result.getLabels())
                .addAllScores(result.getScores())
                .build();
    }

    private static void validate(
            FeatureStat input,
            ModelInferenceResult result,
            String modelVersion,
            long producedAt) {
        if (input == null || result == null) {
            throw new IllegalArgumentException("feature and inference result are required");
        }
        if (!input.hasHeader()
                || input.getHeader().getTenantId().isBlank()
                || input.getHeader().getEventId().isBlank()) {
            throw new IllegalArgumentException("validated feature identity is required");
        }
        if (input.getObjectId().isBlank()
                || input.getCommunityId().isBlank()
                || input.getTs() <= 0) {
            throw new IllegalArgumentException("validated feature business identity is required");
        }
        if (!input.hasTuple()) {
            throw new IllegalArgumentException("validated feature tuple is required");
        }
        FiveTuple tuple = input.getTuple();
        if (tuple.getSrcIp().isBlank()
                || tuple.getDstIp().isBlank()
                || tuple.getProtocol() == 0
                || tuple.getProtocol() != input.getProtocol()) {
            throw new IllegalArgumentException("validated feature tuple is inconsistent");
        }
        if (modelVersion == null || modelVersion.isBlank()
                || result.getModelName() == null || result.getModelName().isBlank()
                || result.getTopLabel() == null || result.getTopLabel().isBlank()) {
            throw new IllegalArgumentException("model identity and top label are required");
        }
        if (!Float.isFinite(result.getTopScore())
                || result.getTopScore() < 0.0f
                || result.getTopScore() > 1.0f) {
            throw new IllegalArgumentException("top score must be finite and within [0,1]");
        }
        List<String> labels = result.getLabels();
        List<Float> scores = result.getScores();
        if (labels == null || scores == null || labels.size() != scores.size()) {
            throw new IllegalArgumentException("labels and scores must have equal cardinality");
        }
        for (int index = 0; index < labels.size(); index++) {
            Float score = scores.get(index);
            if (labels.get(index) == null || labels.get(index).isBlank()
                    || score == null || !Float.isFinite(score)
                    || score < 0.0f || score > 1.0f) {
                throw new IllegalArgumentException("labels and scores must be canonical");
            }
        }
        if (producedAt <= 0) {
            throw new IllegalArgumentException("produced_at must be positive");
        }
    }

    private static String eventId(
            FeatureStat input, ModelInferenceResult result, String modelVersion) {
        EventHeader header = input.getHeader();
        return DeterministicId.uuid(
                "flink-behavior-detection/v1",
                header.getTenantId(),
                header.getEventId(),
                header.getRunId(),
                input.getObjectId(),
                input.getTs(),
                result.getModelName(),
                modelVersion,
                result.getTopLabel());
    }
}
