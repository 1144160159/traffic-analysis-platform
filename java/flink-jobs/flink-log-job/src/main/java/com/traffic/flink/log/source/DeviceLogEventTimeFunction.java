package com.traffic.flink.log.source;

import com.traffic.flink.common.CanonicalDlqMessage;
import com.traffic.flink.common.eventtime.EventTimePolicy;
import com.traffic.flink.common.sourcequality.SourceQualityReceipt;
import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.api.common.state.ValueState;
import org.apache.flink.api.common.state.ValueStateDescriptor;
import org.apache.flink.api.common.state.MapState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

/** Enforces per-device clock monotonicity and the global watermark lateness budget. */
public final class DeviceLogEventTimeFunction
        extends KeyedProcessFunction<String, ValidatedDeviceLog, DeviceLog> {
    private static final long serialVersionUID = 1L;
    private static final String STATE_NAME = "device-log-max-event-time-v1";

    private final EventTimePolicy eventTimePolicy;
    private final OutputTag<CanonicalDlqMessage> dlqTag;
    private final String consumerGroup;
    private final OutputTag<SourceQualityReceipt> qualityTag;
    private final OutputTag<ValidatedDeviceLog> acceptedFactTag;
    private transient ValueState<Long> maxEventTimeState;
    private transient MapState<String, String> logHashes;

    public DeviceLogEventTimeFunction(
            long allowedLatenessMs,
            long maxClockRollbackMs,
            OutputTag<CanonicalDlqMessage> dlqTag) {
        this(new EventTimePolicy(
                0L, 1L, allowedLatenessMs, Long.MAX_VALUE, maxClockRollbackMs), dlqTag);
    }

    public DeviceLogEventTimeFunction(
            EventTimePolicy eventTimePolicy,
            OutputTag<CanonicalDlqMessage> dlqTag) {
        this(eventTimePolicy, dlqTag, "", null);
    }

    public DeviceLogEventTimeFunction(
            EventTimePolicy eventTimePolicy,
            OutputTag<CanonicalDlqMessage> dlqTag,
            String consumerGroup,
            OutputTag<SourceQualityReceipt> qualityTag) {
        this(eventTimePolicy, dlqTag, consumerGroup, qualityTag, null);
    }

    public DeviceLogEventTimeFunction(
            EventTimePolicy eventTimePolicy,
            OutputTag<CanonicalDlqMessage> dlqTag,
            String consumerGroup,
            OutputTag<SourceQualityReceipt> qualityTag,
            OutputTag<ValidatedDeviceLog> acceptedFactTag) {
        if (eventTimePolicy == null) throw new IllegalArgumentException("event-time policy is required");
        this.eventTimePolicy = eventTimePolicy;
        this.dlqTag = dlqTag;
        this.consumerGroup = consumerGroup == null ? "" : consumerGroup;
        this.qualityTag = qualityTag;
        this.acceptedFactTag = acceptedFactTag;
    }

    @Override
    public void open(Configuration parameters) {
        // maxEventTimeState 也配置 TTL（与 logHashes 同为 7 天）：
        // key(tenant:deviceIp) 的单调时钟状态过期后重新建立基线，
        // 避免无界状态增长。过期仅放宽一次时钟回退容差，不丢已落库数据。
        ValueStateDescriptor<Long> maxEventTimeDescriptor =
                new ValueStateDescriptor<>(STATE_NAME, Long.class);
        maxEventTimeDescriptor.enableTimeToLive(StateTtlConfig
                .newBuilder(Time.days(7))
                .setUpdateType(StateTtlConfig.UpdateType.OnCreateAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .build());
        maxEventTimeState = getRuntimeContext().getState(maxEventTimeDescriptor);
        MapStateDescriptor<String, String> hashes = new MapStateDescriptor<>(
                "device-log-source-hash-v1", String.class, String.class);
        hashes.enableTimeToLive(StateTtlConfig.newBuilder(Time.days(7))
                .setUpdateType(StateTtlConfig.UpdateType.OnCreateAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .build());
        logHashes = getRuntimeContext().getMapState(hashes);
    }

    @Override
    public void processElement(
            ValidatedDeviceLog input,
            Context context,
            Collector<DeviceLog> out) throws Exception {
        DeviceLog log = input.getLog();
        long eventTime = log.getTimestamp();
        String sourceHash = SourceQualityReceipt.hashSource(input.getSource().getValue());
        String previousHash = logHashes.get(log.getLogId());
        long watermark = context.timerService().currentWatermark();
        if (previousHash != null) {
            if (previousHash.equals(sourceHash)) {
                emitReceipt(context, input, "duplicate", "DUPLICATE_EVENT", watermark);
                return;
            }
            context.output(dlqTag, failure(
                    input,
                    "EVENT_ID_CONFLICT",
                    "device_log_quality",
                    "same DeviceLog log_id has another payload"));
            emitReceipt(context, input, "conflict", "EVENT_ID_CONFLICT", watermark);
            return;
        }
        Long previousMaximum = maxEventTimeState.value();
        long processingTime = context.timerService().currentProcessingTime();
        EventTimePolicy.Decision decision = eventTimePolicy.classify(
                eventTime,
                input.getSource().getTimestamp(),
                processingTime,
                watermark,
                previousMaximum);

        if (decision.getStatus() == EventTimePolicy.Status.CLOCK_ROLLBACK) {
            context.output(dlqTag, failure(
                    input,
                    "CLOCK_ROLLBACK",
                    "event_time_clock_rollback",
                    "DeviceLog event_time=" + eventTime
                            + " is behind device maximum=" + previousMaximum
                            + " beyond max_clock_rollback_ms="
                            + eventTimePolicy.getMaxClockRollbackMs()));
            emitReceipt(context, input, "conflict", "CLOCK_ROLLBACK", watermark);
            return;
        }

        if (decision.getStatus() == EventTimePolicy.Status.LATE_EVENT) {
            context.output(dlqTag, failure(
                    input,
                    "LATE_EVENT",
                    "event_time_lateness",
                    "DeviceLog event_time=" + eventTime
                            + " is older than watermark=" + watermark
                            + " with allowed_lateness_ms="
                            + eventTimePolicy.getAllowedLatenessMs()));
            emitReceipt(context, input, "late", "LATE_EVENT", watermark);
            return;
        }

        if (!decision.isAccepted()) {
            context.output(dlqTag, failure(
                    input,
                    decision.getStatus().name(),
                    "event_time_contract",
                    "DeviceLog rejected by shared event-time policy: "
                            + decision.getStatus().name()));
            emitReceipt(context, input, "invalid", decision.getStatus().name(), watermark);
            return;
        }

        if (previousMaximum == null || eventTime > previousMaximum) {
            maxEventTimeState.update(eventTime);
        }
        logHashes.put(log.getLogId(), sourceHash);
        emitReceipt(context, input, "accepted", "", watermark);
        if (acceptedFactTag != null) context.output(acceptedFactTag, input);
        out.collect(log);
    }

    static boolean isClockRollback(long eventTime, long previousMaximum, long toleranceMs) {
        return EventTimePolicy.isClockRollback(eventTime, previousMaximum, toleranceMs);
    }

    static boolean isLate(long eventTime, long watermark, long allowedLatenessMs) {
        return EventTimePolicy.isLate(eventTime, watermark, allowedLatenessMs);
    }

    static CanonicalDlqMessage failure(
            ValidatedDeviceLog input, String code, String type, String message) {
        DeviceLog log = input.getLog();
        return CanonicalDlqMessage.failure(
                input.getSource(),
                code,
                type,
                message,
                log.getTenantId(),
                log.getLogId(),
                input.getSource().header("trace_id"),
                input.getSource().header("run_id"),
                input.getSource().header("probe_id"),
                "flink-log-job",
                "traffic.v1.DeviceLog",
                "v1");
    }

    private void emitReceipt(
            Context context,
            ValidatedDeviceLog input,
            String category,
            String reasonCode,
            long watermark) {
        if (qualityTag == null) return;
        DeviceLog log = input.getLog();
        context.output(qualityTag, new SourceQualityReceipt(
                log.getTenantId(),
                "device_log",
                consumerGroup,
                input.getSource().getTopic(),
                input.getSource().getPartition(),
                input.getSource().getOffset(),
                category,
                log.getLogId(),
                SourceQualityReceipt.hashSource(input.getSource().getValue()),
                watermark,
                input.getSource().getTimestamp(),
                reasonCode));
    }

}
