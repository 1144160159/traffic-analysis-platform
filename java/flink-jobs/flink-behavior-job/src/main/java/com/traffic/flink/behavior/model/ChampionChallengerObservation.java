package com.traffic.flink.behavior.model;

import java.io.Serializable;

/** Durable, non-serving comparison emitted by the N012 shadow path. */
public final class ChampionChallengerObservation implements Serializable {
    private static final long serialVersionUID = 1L;

    private final int schemaVersion;
    private final String observationId;
    private final String tenantId;
    private final String sourceEventId;
    private final String objectId;
    private final String communityId;
    private final long eventTimeMs;
    private final long observedAtMs;
    private final int sampleBucket;
    private final String servingResultSource;
    private final String championModelId;
    private final String championVersion;
    private final String championLabel;
    private final Float championScore;
    private final Boolean championDetected;
    private final long championLatencyNanos;
    private final String challengerModelId;
    private final String challengerVersion;
    private final String challengerPackageId;
    private final String challengerPackageSha256;
    private final long challengerAggregateRevision;
    private final String challengerLabel;
    private final Float challengerScore;
    private final Boolean challengerDetected;
    private final long challengerLatencyNanos;
    private final long challengerCpuNanos;
    private final long challengerHeapDeltaBytes;
    private final Float absoluteScoreDelta;
    private final Boolean decisionChanged;
    private final Boolean labelChanged;
    private final String status;
    private final String errorCode;
    private final String errorMessage;

    private ChampionChallengerObservation(Builder value) {
        this.schemaVersion = 1;
        this.observationId = value.observationId;
        this.tenantId = value.tenantId;
        this.sourceEventId = value.sourceEventId;
        this.objectId = value.objectId;
        this.communityId = value.communityId;
        this.eventTimeMs = value.eventTimeMs;
        this.observedAtMs = value.observedAtMs;
        this.sampleBucket = value.sampleBucket;
        this.servingResultSource = "champion";
        this.championModelId = value.championModelId;
        this.championVersion = value.championVersion;
        this.championLabel = value.championLabel;
        this.championScore = value.championScore;
        this.championDetected = value.championDetected;
        this.championLatencyNanos = value.championLatencyNanos;
        this.challengerModelId = value.challengerModelId;
        this.challengerVersion = value.challengerVersion;
        this.challengerPackageId = value.challengerPackageId;
        this.challengerPackageSha256 = value.challengerPackageSha256;
        this.challengerAggregateRevision = value.challengerAggregateRevision;
        this.challengerLabel = value.challengerLabel;
        this.challengerScore = value.challengerScore;
        this.challengerDetected = value.challengerDetected;
        this.challengerLatencyNanos = value.challengerLatencyNanos;
        this.challengerCpuNanos = value.challengerCpuNanos;
        this.challengerHeapDeltaBytes = value.challengerHeapDeltaBytes;
        this.absoluteScoreDelta = value.absoluteScoreDelta;
        this.decisionChanged = value.decisionChanged;
        this.labelChanged = value.labelChanged;
        this.status = value.status;
        this.errorCode = value.errorCode;
        this.errorMessage = truncate(value.errorMessage);
    }

    private static String truncate(String value) {
        if (value == null) return "";
        String singleLine = value.replace('\n', ' ').replace('\r', ' ');
        return singleLine.length() <= 512 ? singleLine : singleLine.substring(0, 512);
    }

    public int getSchemaVersion() { return schemaVersion; }
    public String getObservationId() { return observationId; }
    public String getTenantId() { return tenantId; }
    public String getSourceEventId() { return sourceEventId; }
    public String getObjectId() { return objectId; }
    public String getCommunityId() { return communityId; }
    public long getEventTimeMs() { return eventTimeMs; }
    public long getObservedAtMs() { return observedAtMs; }
    public int getSampleBucket() { return sampleBucket; }
    public String getServingResultSource() { return servingResultSource; }
    public String getChampionModelId() { return championModelId; }
    public String getChampionVersion() { return championVersion; }
    public String getChampionLabel() { return championLabel; }
    public Float getChampionScore() { return championScore; }
    public Boolean getChampionDetected() { return championDetected; }
    public long getChampionLatencyNanos() { return championLatencyNanos; }
    public String getChallengerModelId() { return challengerModelId; }
    public String getChallengerVersion() { return challengerVersion; }
    public String getChallengerPackageId() { return challengerPackageId; }
    public String getChallengerPackageSha256() { return challengerPackageSha256; }
    public long getChallengerAggregateRevision() { return challengerAggregateRevision; }
    public String getChallengerLabel() { return challengerLabel; }
    public Float getChallengerScore() { return challengerScore; }
    public Boolean getChallengerDetected() { return challengerDetected; }
    public long getChallengerLatencyNanos() { return challengerLatencyNanos; }
    public long getChallengerCpuNanos() { return challengerCpuNanos; }
    public long getChallengerHeapDeltaBytes() { return challengerHeapDeltaBytes; }
    public Float getAbsoluteScoreDelta() { return absoluteScoreDelta; }
    public Boolean getDecisionChanged() { return decisionChanged; }
    public Boolean getLabelChanged() { return labelChanged; }
    public String getStatus() { return status; }
    public String getErrorCode() { return errorCode; }
    public String getErrorMessage() { return errorMessage; }

    public static Builder builder() { return new Builder(); }

    public static final class Builder {
        private String observationId = "";
        private String tenantId = "";
        private String sourceEventId = "";
        private String objectId = "";
        private String communityId = "";
        private long eventTimeMs;
        private long observedAtMs;
        private int sampleBucket;
        private String championModelId = "";
        private String championVersion = "";
        private String championLabel = "";
        private Float championScore;
        private Boolean championDetected;
        private long championLatencyNanos;
        private String challengerModelId = "";
        private String challengerVersion = "";
        private String challengerPackageId = "";
        private String challengerPackageSha256 = "";
        private long challengerAggregateRevision;
        private String challengerLabel = "";
        private Float challengerScore;
        private Boolean challengerDetected;
        private long challengerLatencyNanos;
        private long challengerCpuNanos;
        private long challengerHeapDeltaBytes;
        private Float absoluteScoreDelta;
        private Boolean decisionChanged;
        private Boolean labelChanged;
        private String status = "error";
        private String errorCode = "";
        private String errorMessage = "";

        public Builder observationId(String value) { observationId = value; return this; }
        public Builder tenantId(String value) { tenantId = value; return this; }
        public Builder sourceEventId(String value) { sourceEventId = value; return this; }
        public Builder objectId(String value) { objectId = value; return this; }
        public Builder communityId(String value) { communityId = value; return this; }
        public Builder eventTimeMs(long value) { eventTimeMs = value; return this; }
        public Builder observedAtMs(long value) { observedAtMs = value; return this; }
        public Builder sampleBucket(int value) { sampleBucket = value; return this; }
        public Builder championModelId(String value) { championModelId = value; return this; }
        public Builder championVersion(String value) { championVersion = value; return this; }
        public Builder championLabel(String value) { championLabel = value; return this; }
        public Builder championScore(Float value) { championScore = value; return this; }
        public Builder championDetected(Boolean value) { championDetected = value; return this; }
        public Builder championLatencyNanos(long value) { championLatencyNanos = value; return this; }
        public Builder challengerModelId(String value) { challengerModelId = value; return this; }
        public Builder challengerVersion(String value) { challengerVersion = value; return this; }
        public Builder challengerPackageId(String value) { challengerPackageId = value; return this; }
        public Builder challengerPackageSha256(String value) { challengerPackageSha256 = value; return this; }
        public Builder challengerAggregateRevision(long value) { challengerAggregateRevision = value; return this; }
        public Builder challengerLabel(String value) { challengerLabel = value; return this; }
        public Builder challengerScore(Float value) { challengerScore = value; return this; }
        public Builder challengerDetected(Boolean value) { challengerDetected = value; return this; }
        public Builder challengerLatencyNanos(long value) { challengerLatencyNanos = value; return this; }
        public Builder challengerCpuNanos(long value) { challengerCpuNanos = value; return this; }
        public Builder challengerHeapDeltaBytes(long value) { challengerHeapDeltaBytes = value; return this; }
        public Builder absoluteScoreDelta(Float value) { absoluteScoreDelta = value; return this; }
        public Builder decisionChanged(Boolean value) { decisionChanged = value; return this; }
        public Builder labelChanged(Boolean value) { labelChanged = value; return this; }
        public Builder status(String value) { status = value; return this; }
        public Builder errorCode(String value) { errorCode = value; return this; }
        public Builder errorMessage(String value) { errorMessage = value; return this; }
        public ChampionChallengerObservation build() { return new ChampionChallengerObservation(this); }
    }
}
