package com.traffic.flink.common.eventtime;

import org.apache.flink.api.common.eventtime.WatermarkStrategy;

import java.io.Serializable;
import java.time.Duration;

/**
 * Shared, deterministic event-time policy for all traffic Flink jobs.
 *
 * <p>All inputs use Unix epoch milliseconds. Processing time is accepted as an
 * explicit argument instead of being read from the wall clock so classification
 * is replayable. The late boundary is strict: an event is late only when
 * {@code eventTimeMs < watermarkMs - allowedLatenessMs}.</p>
 */
public final class EventTimePolicy implements Serializable {
    private static final long serialVersionUID = 1L;

    public enum Status {
        ACCEPT,
        INVALID_EVENT_TIME,
        INVALID_INGEST_TIME,
        INVALID_PROCESSING_TIME,
        FUTURE_EVENT,
        CLOCK_ROLLBACK,
        LATE_EVENT
    }

    @FunctionalInterface
    public interface TimestampExtractor<T> extends Serializable {
        long extractTimestamp(T value);
    }

    public static final class Decision implements Serializable {
        private static final long serialVersionUID = 1L;

        private final Status status;
        private final long eventTimeMs;
        private final long ingestTimeMs;
        private final long processingTimeMs;
        private final long watermarkMs;
        private final long asOfMs;

        private Decision(
                Status status,
                long eventTimeMs,
                long ingestTimeMs,
                long processingTimeMs,
                long watermarkMs,
                long asOfMs) {
            this.status = status;
            this.eventTimeMs = eventTimeMs;
            this.ingestTimeMs = ingestTimeMs;
            this.processingTimeMs = processingTimeMs;
            this.watermarkMs = watermarkMs;
            this.asOfMs = asOfMs;
        }

        public Status getStatus() { return status; }
        public boolean isAccepted() { return status == Status.ACCEPT; }
        public long getEventTimeMs() { return eventTimeMs; }
        public long getIngestTimeMs() { return ingestTimeMs; }
        public long getProcessingTimeMs() { return processingTimeMs; }
        public long getWatermarkMs() { return watermarkMs; }
        public long getAsOfMs() { return asOfMs; }
    }

    private final long watermarkDelayMs;
    private final long watermarkIdlenessMs;
    private final long allowedLatenessMs;
    private final long maxFutureSkewMs;
    private final long maxClockRollbackMs;

    public EventTimePolicy(
            long watermarkDelayMs,
            long watermarkIdlenessMs,
            long allowedLatenessMs,
            long maxFutureSkewMs,
            long maxClockRollbackMs) {
        requireNonNegative(watermarkDelayMs, "watermarkDelayMs");
        if (watermarkIdlenessMs <= 0L) {
            throw new IllegalArgumentException("watermarkIdlenessMs must be positive");
        }
        requireNonNegative(allowedLatenessMs, "allowedLatenessMs");
        requireNonNegative(maxFutureSkewMs, "maxFutureSkewMs");
        requireNonNegative(maxClockRollbackMs, "maxClockRollbackMs");
        this.watermarkDelayMs = watermarkDelayMs;
        this.watermarkIdlenessMs = watermarkIdlenessMs;
        this.allowedLatenessMs = allowedLatenessMs;
        this.maxFutureSkewMs = maxFutureSkewMs;
        this.maxClockRollbackMs = maxClockRollbackMs;
    }

    public Decision classify(
            long eventTimeMs,
            long ingestTimeMs,
            long processingTimeMs,
            long watermarkMs,
            Long previousMaximumEventTimeMs) {
        Status status;
        if (eventTimeMs <= 0L) {
            status = Status.INVALID_EVENT_TIME;
        } else if (ingestTimeMs <= 0L) {
            status = Status.INVALID_INGEST_TIME;
        } else if (processingTimeMs <= 0L) {
            status = Status.INVALID_PROCESSING_TIME;
        } else if (isFuture(eventTimeMs, ingestTimeMs, maxFutureSkewMs)) {
            status = Status.FUTURE_EVENT;
        } else if (previousMaximumEventTimeMs != null
                && isClockRollback(
                        eventTimeMs, previousMaximumEventTimeMs, maxClockRollbackMs)) {
            status = Status.CLOCK_ROLLBACK;
        } else if (isLate(eventTimeMs, watermarkMs, allowedLatenessMs)) {
            status = Status.LATE_EVENT;
        } else {
            status = Status.ACCEPT;
        }
        return new Decision(
                status,
                eventTimeMs,
                ingestTimeMs,
                processingTimeMs,
                watermarkMs,
                effectiveAsOf(watermarkMs, processingTimeMs));
    }

    public <T> WatermarkStrategy<T> watermarkStrategy(TimestampExtractor<T> extractor) {
        if (extractor == null) throw new IllegalArgumentException("timestamp extractor is required");
        return WatermarkStrategy.<T>forBoundedOutOfOrderness(Duration.ofMillis(watermarkDelayMs))
                .withTimestampAssigner((value, ignored) -> extractor.extractTimestamp(value))
                .withIdleness(Duration.ofMillis(watermarkIdlenessMs));
    }

    public static boolean isFuture(long eventTimeMs, long ingestTimeMs, long maxFutureSkewMs) {
        requireNonNegative(maxFutureSkewMs, "maxFutureSkewMs");
        return eventTimeMs > saturatingAdd(ingestTimeMs, maxFutureSkewMs);
    }

    public static boolean isClockRollback(
            long eventTimeMs, long previousMaximumEventTimeMs, long toleranceMs) {
        requireNonNegative(toleranceMs, "toleranceMs");
        return eventTimeMs < saturatingSubtract(previousMaximumEventTimeMs, toleranceMs);
    }

    public static boolean isLate(
            long eventTimeMs, long watermarkMs, long allowedLatenessMs) {
        requireNonNegative(allowedLatenessMs, "allowedLatenessMs");
        return watermarkMs != Long.MIN_VALUE
                && eventTimeMs < saturatingSubtract(watermarkMs, allowedLatenessMs);
    }

    /** Returns a replay-explicit read boundary that can never exceed processing time. */
    public static long effectiveAsOf(long watermarkMs, long processingTimeMs) {
        if (processingTimeMs <= 0L) {
            throw new IllegalArgumentException("processingTimeMs must be positive");
        }
        if (watermarkMs == Long.MIN_VALUE) return processingTimeMs;
        return Math.min(watermarkMs, processingTimeMs);
    }

    public long getAllowedLatenessMs() { return allowedLatenessMs; }
    public long getMaxFutureSkewMs() { return maxFutureSkewMs; }
    public long getMaxClockRollbackMs() { return maxClockRollbackMs; }

    private static long saturatingAdd(long left, long right) {
        if (right > 0L && left > Long.MAX_VALUE - right) return Long.MAX_VALUE;
        return left + right;
    }

    private static long saturatingSubtract(long left, long right) {
        if (right > 0L && left < Long.MIN_VALUE + right) return Long.MIN_VALUE;
        return left - right;
    }

    private static void requireNonNegative(long value, String field) {
        if (value < 0L) throw new IllegalArgumentException(field + " must not be negative");
    }
}
