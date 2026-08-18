package com.traffic.flink.feature.calculator;

import com.traffic.flink.common.DeterministicId;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.SessionEvent;

import java.util.ArrayList;
import java.util.List;
import java.util.TreeSet;

final class FeatureLineage {
    private FeatureLineage() {}

    static EventHeader header(SessionEvent session, String kind, String schemaVersion) {
        EventHeader source = session.getHeader();
        String eventId = DeterministicId.uuid(
                "flink-feature-" + kind + "/v1",
                source.getTenantId(), source.getRunId(), session.getSessionId(),
                session.getCommunityId(), session.getTsStart(), session.getTsEnd());
        long producedAt = System.currentTimeMillis();
        return EventHeader.newBuilder()
                .setEventId(eventId)
                .setTenantId(source.getTenantId())
                .setRunId(source.getRunId())
                .setEventTs(session.getTsEnd())
                .setIngestTs(producedAt)
                .setProbeId(source.getProbeId())
                .setFeatureSetId(source.getFeatureSetId())
                .setEventType("traffic.feature." + kind + ".v1")
                .setSchemaVersion(schemaVersion)
                .setAggregateType("session")
                .setAggregateId(session.getSessionId())
                .setAggregateVersion(1)
                .setOccurredAt(session.getTsEnd())
                .setProducedAt(producedAt)
                .setTraceId(source.getTraceId().isEmpty() ? source.getEventId() : source.getTraceId())
                .setCausationId(source.getEventId())
                .setCorrelationId(source.getCorrelationId().isEmpty()
                        ? session.getCommunityId() : source.getCorrelationId())
                .setIdempotencyKey(eventId)
                .setProducer("flink-feature-job")
                .build();
    }

    static List<String> sourceEventIds(SessionEvent session) {
        List<String> values = new ArrayList<>(session.getSourceEventIdsList());
        values.add(session.getHeader().getEventId());
        return sorted(values);
    }

    static List<String> evidenceIds(SessionEvent session) {
        return sorted(session.getEvidenceIdsList().isEmpty()
                ? session.getFlowIdsList() : session.getEvidenceIdsList());
    }

    static List<String> sorted(Iterable<String> values) {
        TreeSet<String> result = new TreeSet<>();
        for (String value : values) {
            if (value != null && !value.isEmpty()) result.add(value);
        }
        return new ArrayList<>(result);
    }
}
