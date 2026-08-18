package com.traffic.flink.log.source;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.google.protobuf.InvalidProtocolBufferException;
import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.RawKafkaRecord;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.flink.log.LogJobConfig;
import com.traffic.flink.log.parser.SyslogParser;
import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.streaming.api.functions.ProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import java.net.Inet6Address;
import java.net.InetAddress;
import java.util.Locale;
import java.util.Set;

/** Strictly decodes DeviceLog and validates its topic, key, headers, identity and time. */
public final class DeviceLogParseFunction
        extends ProcessFunction<RawKafkaRecord, ValidatedDeviceLog> {
    private static final long serialVersionUID = 1L;
    private static final int MAX_PAYLOAD_BYTES = 1_048_576;
    private static final int MAX_MESSAGE_CHARS = 262_144;
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final Set<String> SOURCES = Set.of("syslog", "snmp_trap", "netflow");

    private final EventTimePolicy eventTimePolicy;
    private final OutputTag<CanonicalDlqMessage> dlqTag;
    private final String consumerGroup;
    private final OutputTag<SourceQualityReceipt> qualityTag;

    public DeviceLogParseFunction(
            long maxFutureSkewMs, OutputTag<CanonicalDlqMessage> dlqTag) {
        this(new EventTimePolicy(0L, 1L, 0L, maxFutureSkewMs, 0L), dlqTag);
    }

    public DeviceLogParseFunction(
            EventTimePolicy eventTimePolicy, OutputTag<CanonicalDlqMessage> dlqTag) {
        this(eventTimePolicy, dlqTag, "", null);
    }

    public DeviceLogParseFunction(
            EventTimePolicy eventTimePolicy,
            OutputTag<CanonicalDlqMessage> dlqTag,
            String consumerGroup,
            OutputTag<SourceQualityReceipt> qualityTag) {
        if (eventTimePolicy == null) throw new IllegalArgumentException("event-time policy is required");
        this.eventTimePolicy = eventTimePolicy;
        this.dlqTag = dlqTag;
        this.consumerGroup = consumerGroup == null ? "" : consumerGroup;
        this.qualityTag = qualityTag;
    }

    @Override
    public void processElement(
            RawKafkaRecord source, Context context, Collector<ValidatedDeviceLog> out) {
        ParseResult result = parse(source, eventTimePolicy);
        if (result.input != null) {
            out.collect(result.input);
        } else {
            context.output(dlqTag, result.failure);
            if (qualityTag != null) {
                context.output(qualityTag, failureReceipt(
                        source, result.failure, consumerGroup, context.timerService().currentProcessingTime()));
            }
        }
    }

    static ParseResult parse(RawKafkaRecord source, long maxFutureSkewMs) {
        return parse(source, new EventTimePolicy(0L, 1L, 0L, maxFutureSkewMs, 0L));
    }

    static ParseResult parse(RawKafkaRecord source, EventTimePolicy eventTimePolicy) {
        if (source == null) {
            throw new IllegalArgumentException("RawKafkaRecord must not be null");
        }
        if (!LogJobConfig.INPUT_TOPIC.equals(source.getTopic())) {
            return failure(source, null, "WRONG_SOURCE_TOPIC", "contract_violation",
                    "expected source topic " + LogJobConfig.INPUT_TOPIC);
        }
        byte[] payload = source.getValue();
        if (payload == null || payload.length == 0) {
            return failure(source, null, "BAD_SCHEMA", "parse_error",
                    "empty DeviceLog payload");
        }
        if (payload.length > MAX_PAYLOAD_BYTES) {
            return failure(source, null, "PAYLOAD_TOO_LARGE", "validation_error",
                    "DeviceLog payload exceeds " + MAX_PAYLOAD_BYTES + " bytes");
        }

        final DeviceLog log;
        try {
            log = DeviceLog.parseFrom(payload);
        } catch (InvalidProtocolBufferException error) {
            return failure(source, null, "BAD_SCHEMA", "parse_error",
                    "invalid DeviceLog protobuf: " + error.getMessage());
        }
        if (!log.getUnknownFields().asMap().isEmpty()) {
            return failure(source, log, "BAD_SCHEMA", "schema_version_mismatch",
                    "DeviceLog contains unknown protobuf fields");
        }

        String validationError = validateValue(log);
        if (validationError != null) {
            String code = validationError.startsWith("timestamp:")
                    ? "BAD_TIMESTAMP" : "VALIDATION_ERROR";
            return failure(source, log, code, "validation_error", validationError);
        }

        String envelopeError = validateEnvelope(
                source, log, eventTimePolicy.getMaxFutureSkewMs());
        if (envelopeError != null) {
            String code = envelopeError.startsWith("timestamp:")
                    ? "BAD_TIMESTAMP" : "ENVELOPE_MISMATCH";
            return failure(source, log, code, "contract_violation", envelopeError);
        }
        return new ParseResult(new ValidatedDeviceLog(source, log), null);
    }

    static String validateValue(DeviceLog log) {
        if (isBlank(log.getLogId()) || log.getLogId().length() > 128) {
            return "missing or oversized DeviceLog log_id";
        }
        if (isBlank(log.getTenantId()) || log.getTenantId().length() > 128) {
            return "missing or oversized DeviceLog tenant_id";
        }
        if (!isIpLiteral(log.getDeviceIp())) return "invalid DeviceLog device_ip";
        if (log.getTimestamp() <= 0L) return "timestamp: DeviceLog timestamp must be positive";
        if (isBlank(log.getMessage())) return "missing DeviceLog message";
        if (log.getMessage().length() > MAX_MESSAGE_CHARS) return "DeviceLog message is too large";
        if (log.getFacility() > 23) return "DeviceLog facility is outside syslog range 0..23";
        if (log.getSeverity() > 7) return "DeviceLog severity is outside syslog range 0..7";

        String source = log.getSource().toLowerCase(Locale.ROOT);
        if (!SOURCES.contains(source) || !source.equals(log.getSource())) {
            return "DeviceLog source is unsupported or not canonical lowercase";
        }
        if ("syslog".equals(source) && !SyslogParser.isSupportedMessage(log.getMessage())) {
            return "DeviceLog syslog message is not RFC5424, RFC3164 or valid PRI-prefixed data";
        }
        if (!isBlank(log.getParsed())) {
            try {
                JSON.readTree(log.getParsed());
            } catch (Exception error) {
                return "DeviceLog parsed field is not valid JSON";
            }
        }
        return null;
    }

    static String validateEnvelope(
            RawKafkaRecord source, DeviceLog log, long maxFutureSkewMs) {
        if (!source.getDuplicateHeaderNames().isEmpty()) {
            return "Kafka envelope contains duplicate headers: "
                    + String.join(",", source.getDuplicateHeaderNames());
        }
        String expectedKey = log.getTenantId() + ":" + log.getDeviceIp();
        if (!expectedKey.equals(source.keyAsString())) {
            return "Kafka key must equal tenant_id:device_ip";
        }
        String[][] requiredHeaders = {
                {"tenant_id", log.getTenantId()},
                {"device_ip", log.getDeviceIp()},
                {"event_id", log.getLogId()},
                {"source", log.getSource()},
                {"schema_version", "device-log/v1"},
                {"content_type", "application/x-protobuf"},
                {"message_type", "traffic.v1.DeviceLog"}
        };
        for (String[] header : requiredHeaders) {
            if (!header[1].equals(source.header(header[0]))) {
                return "Kafka header " + header[0] + " is missing or inconsistent";
            }
        }
        if (source.getTimestamp() <= 0L) {
            return "timestamp: Kafka record timestamp must be positive ingest time";
        }
        if (EventTimePolicy.isFuture(
                log.getTimestamp(), source.getTimestamp(), maxFutureSkewMs)) {
            return "timestamp: DeviceLog event time exceeds ingest time plus future skew";
        }
        return null;
    }

    private static ParseResult failure(
            RawKafkaRecord source,
            DeviceLog log,
            String code,
            String type,
            String message) {
        String tenantId = log == null || isBlank(log.getTenantId())
                ? source.header("tenant_id") : log.getTenantId();
        String logId = log == null ? source.header("event_id") : log.getLogId();
        CanonicalDlqMessage failure = CanonicalDlqMessage.failure(
                source,
                code,
                type,
                message,
                tenantId,
                logId,
                source.header("trace_id"),
                source.header("run_id"),
                source.header("probe_id"),
                "flink-log-job",
                "traffic.v1.DeviceLog",
                "v1");
        return new ParseResult(null, failure);
    }

    static SourceQualityReceipt failureReceipt(
            RawKafkaRecord source,
            CanonicalDlqMessage failure,
            String consumerGroup,
            long processingTimeMs) {
        String category = "invalid";
        if ("WRONG_SOURCE_TOPIC".equals(failure.errorCode())
                || "ENVELOPE_MISMATCH".equals(failure.errorCode())) {
            category = "rejected";
        }
        Object event = failure.fields().get("event_id");
        long observedAt = source.getTimestamp() > 0L
                ? source.getTimestamp() : Math.max(1L, processingTimeMs);
        return new SourceQualityReceipt(
                failure.tenantId(),
                "device_log",
                consumerGroup,
                source.getTopic(),
                source.getPartition(),
                source.getOffset(),
                category,
                event == null ? "" : String.valueOf(event),
                SourceQualityReceipt.hashSource(source.getValue()),
                Long.MIN_VALUE,
                observedAt,
                failure.errorCode());
    }

    private static boolean isIpLiteral(String value) {
        if (isBlank(value)) return false;
        if (value.indexOf(':') >= 0) {
            if (!value.matches("[0-9A-Fa-f:.]+")) return false;
            try {
                return InetAddress.getByName(value) instanceof Inet6Address;
            } catch (Exception error) {
                return false;
            }
        }
        String[] pieces = value.split("\\.", -1);
        if (pieces.length != 4) return false;
        for (String piece : pieces) {
            if (piece.isEmpty() || piece.length() > 3 || !piece.matches("[0-9]+")) return false;
            if (piece.length() > 1 && piece.charAt(0) == '0') return false;
            if (Integer.parseInt(piece) > 255) return false;
        }
        return true;
    }

    private static boolean isBlank(String value) {
        return value == null || value.trim().isEmpty();
    }

    static final class ParseResult {
        final ValidatedDeviceLog input;
        final CanonicalDlqMessage failure;

        ParseResult(ValidatedDeviceLog input, CanonicalDlqMessage failure) {
            this.input = input;
            this.failure = failure;
        }
    }
}
