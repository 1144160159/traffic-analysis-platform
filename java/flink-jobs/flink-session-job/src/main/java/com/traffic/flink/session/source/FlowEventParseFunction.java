package com.traffic.flink.session.source;

import com.google.protobuf.InvalidProtocolBufferException;
import com.traffic.proto.traffic.v1.DeadLetter;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FlowEvent;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import java.nio.charset.StandardCharsets;
import java.util.Base64;
import java.util.Locale;

/**
 * Parses raw Kafka bytes into FlowEvent and emits bad input records to DLQ.
 */
public class FlowEventParseFunction extends ProcessFunction<RawKafkaRecord, FlowEvent> {

    private static final long serialVersionUID = 1L;
    private static final int MAX_ERROR_LENGTH = 512;

    private final OutputTag<DeadLetter> dlqTag;

    public FlowEventParseFunction(OutputTag<DeadLetter> dlqTag) {
        this.dlqTag = dlqTag;
    }

    @Override
    public void processElement(RawKafkaRecord record, Context ctx, Collector<FlowEvent> out) {
        ParseResult result = parseRecord(record);
        if (result.flowEvent != null) {
            out.collect(result.flowEvent);
            return;
        }
        ctx.output(dlqTag, result.deadLetter);
    }

    static ParseResult parseRecord(RawKafkaRecord record) {
        byte[] payload = record.getValue();
        if (payload == null || payload.length == 0) {
            return ParseResult.deadLetter(buildDeadLetter(record, "empty FlowEvent payload", null));
        }

        FlowEvent flow;
        try {
            flow = FlowEvent.parseFrom(payload);
        } catch (InvalidProtocolBufferException e) {
            return ParseResult.deadLetter(buildDeadLetter(record, "invalid FlowEvent protobuf: " + e.getMessage(), null));
        }

        if (!flow.hasHeader()) {
            return ParseResult.deadLetter(buildDeadLetter(record, "missing FlowEvent header", flow));
        }
        EventHeader header = flow.getHeader();
        if (isBlank(header.getTenantId())) {
            return ParseResult.deadLetter(buildDeadLetter(record, "missing FlowEvent tenant_id", flow));
        }
        if (isBlank(flow.getCommunityId())) {
            return ParseResult.deadLetter(buildDeadLetter(record, "missing FlowEvent community_id", flow));
        }
        return ParseResult.flow(flow);
    }

    private static DeadLetter buildDeadLetter(RawKafkaRecord record, String errorMsg, FlowEvent parsedFlow) {
        String sourceKey = record.keyAsString();
        String tenantId = tenantFrom(record, parsedFlow, sourceKey);
        long createdAt = record.getTimestamp() > 0 ? record.getTimestamp() : System.currentTimeMillis();

        return DeadLetter.newBuilder()
                .setEventId(String.format(Locale.ROOT, "flink-session-parse:%s:%d:%d",
                        record.getTopic(), record.getPartition(), record.getOffset()))
                .setTenantId(tenantId)
                .setSourceTopic(record.getTopic())
                .setSourceKey(sourceKey)
                .setErrorMsg(truncate(errorMsg, MAX_ERROR_LENGTH))
                .setRawPayload(Base64.getEncoder().encodeToString(nullToEmpty(record.getValue())))
                .setRetryCount(0)
                .setCreatedAt(createdAt)
                .build();
    }

    private static String tenantFrom(RawKafkaRecord record, FlowEvent parsedFlow, String sourceKey) {
        if (parsedFlow != null && parsedFlow.hasHeader() && !isBlank(parsedFlow.getHeader().getTenantId())) {
            return parsedFlow.getHeader().getTenantId();
        }
        String headerTenant = record.header("tenant_id");
        if (!isBlank(headerTenant)) {
            return headerTenant;
        }
        int separator = sourceKey.indexOf(':');
        if (separator > 0) {
            return sourceKey.substring(0, separator);
        }
        return "unknown";
    }

    private static byte[] nullToEmpty(byte[] value) {
        return value == null ? new byte[0] : value;
    }

    private static boolean isBlank(String value) {
        return value == null || value.trim().isEmpty();
    }

    private static String truncate(String value, int maxLength) {
        if (value == null) {
            return "";
        }
        if (value.length() <= maxLength) {
            return value;
        }
        return value.substring(0, maxLength);
    }

    static class ParseResult {
        final FlowEvent flowEvent;
        final DeadLetter deadLetter;

        private ParseResult(FlowEvent flowEvent, DeadLetter deadLetter) {
            this.flowEvent = flowEvent;
            this.deadLetter = deadLetter;
        }

        static ParseResult flow(FlowEvent flowEvent) {
            return new ParseResult(flowEvent, null);
        }

        static ParseResult deadLetter(DeadLetter deadLetter) {
            return new ParseResult(null, deadLetter);
        }
    }
}
