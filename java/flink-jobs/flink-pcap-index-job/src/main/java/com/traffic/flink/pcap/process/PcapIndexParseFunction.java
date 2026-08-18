package com.traffic.flink.pcap.process;

import com.google.protobuf.InvalidProtocolBufferException;
import com.traffic.flink.pcap.source.PcapDeadLetter;
import com.traffic.flink.pcap.source.PcapIndexedRecord;
import com.traffic.flink.pcap.source.PcapRawKafkaRecord;
import com.traffic.proto.traffic.v1.PcapIndexMeta;

import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import java.util.Locale;
import java.util.regex.Pattern;

/** Classifies every raw PCAP Kafka coordinate into exactly one main or DLQ output. */
public final class PcapIndexParseFunction
        extends ProcessFunction<PcapRawKafkaRecord, PcapIndexedRecord> {
    private static final long serialVersionUID = 1L;
    private static final Pattern SHA256 = Pattern.compile("^[0-9a-fA-F]{64}$");
    private static final long MAX_FILE_BYTES = 10L * 1024 * 1024 * 1024;

    public static final OutputTag<PcapDeadLetter> DLQ_TAG =
            new OutputTag<PcapDeadLetter>("pcap-raw-canonical-dlq") { };

    @Override
    public void processElement(PcapRawKafkaRecord record, Context ctx, Collector<PcapIndexedRecord> out)
            throws Exception {
        if (record == null || ctx == null || out == null) {
            throw new IllegalArgumentException("PCAP parse record context and collector are required");
        }
        ParseResult result = parseRecord(record);
        if (result.indexedRecord != null) {
            out.collect(result.indexedRecord);
        } else {
            ctx.output(DLQ_TAG, result.deadLetter);
        }
    }

    static ParseResult parseRecord(PcapRawKafkaRecord record) {
        if (record == null) throw new IllegalArgumentException("raw PCAP record is required");
        byte[] value = record.getValue();
        if (value.length == 0) return rejected(record, "EMPTY_PROTOBUF", "empty PcapIndexMeta protobuf", null);

        final PcapIndexMeta meta;
        try {
            meta = PcapIndexMeta.parseFrom(value);
        } catch (InvalidProtocolBufferException error) {
            return rejected(record, "INVALID_PROTOBUF", "invalid PcapIndexMeta protobuf", null);
        }

        String validationCode = validateMeta(meta);
        if (validationCode != null) return rejected(record, validationCode, validationCode, meta);
        try {
            return ParseResult.indexed(new PcapIndexedRecord(meta, record));
        } catch (IllegalArgumentException error) {
            return rejected(record, "IDENTITY_CONFLICT", error.getMessage(), meta);
        }
    }

    private static String validateMeta(PcapIndexMeta meta) {
        if (blank(meta.getTenantId())) return "MISSING_TENANT_ID";
        if (blank(meta.getProbeId())) return "MISSING_PROBE_ID";
        if (blank(meta.getFileKey())) return "MISSING_FILE_KEY";
        if (meta.getTsStart() <= 0 || meta.getTsEnd() < meta.getTsStart()) return "INVALID_TIME_RANGE";
        if (meta.getByteSize() == 0 || Long.compareUnsigned(meta.getByteSize(), MAX_FILE_BYTES) > 0) return "INVALID_BYTE_SIZE";
        if (!SHA256.matcher(meta.getSha256()).matches()) return "INVALID_OBJECT_SHA256";
        if (Long.compareUnsigned(meta.getOffsetEnd(), meta.getOffsetStart()) < 0 ||
                Long.compareUnsigned(meta.getOffsetEnd(), meta.getByteSize()) > 0) return "INVALID_OBJECT_OFFSETS";
        if (meta.getCreatedTs() <= 0) return "INVALID_CREATED_TS";
        return null;
    }

    private static ParseResult rejected(PcapRawKafkaRecord record, String code, String message, PcapIndexMeta meta) {
        String tenant = meta == null ? record.firstHeader("tenant_id") : meta.getTenantId();
        String probe = meta == null ? record.firstHeader("probe_id") : meta.getProbeId();
        String safeMessage = message == null ? code : message;
        if (safeMessage.length() > 512) safeMessage = safeMessage.substring(0, 512);
        return ParseResult.deadLetter(new PcapDeadLetter(
                record, code.toUpperCase(Locale.ROOT), safeMessage, tenant, probe));
    }

    private static boolean blank(String value) { return value == null || value.trim().isEmpty(); }

    static final class ParseResult {
        final PcapIndexedRecord indexedRecord;
        final PcapDeadLetter deadLetter;
        private ParseResult(PcapIndexedRecord indexedRecord, PcapDeadLetter deadLetter) {
            this.indexedRecord = indexedRecord; this.deadLetter = deadLetter;
        }
        static ParseResult indexed(PcapIndexedRecord record) { return new ParseResult(record, null); }
        static ParseResult deadLetter(PcapDeadLetter letter) { return new ParseResult(null, letter); }
    }
}
