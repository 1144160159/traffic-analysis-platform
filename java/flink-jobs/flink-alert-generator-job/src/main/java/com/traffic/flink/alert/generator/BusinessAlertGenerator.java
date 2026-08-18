package com.traffic.flink.alert.generator;

import com.traffic.flink.alert.dedup.DedupState;
import com.traffic.flink.alert.evidence.EvidenceBuilder;
import com.traffic.flink.common.DeterministicId;
import com.traffic.proto.traffic.v1.*;

import org.apache.flink.api.common.state.MapState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.api.common.typeinfo.TypeHint;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.api.java.tuple.Tuple2;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.List;

/**
 * Business Alert 生成器
 * 
 * 处理来自规则引擎（Rule Job）的 DetectionBusiness 事件
 * 生成对应的 Alert 和 Evidence
 * 
 * 与 AlertGenerator 的区别：
 * - 输入是 DetectionBusiness（规则检测结果）
 * - 包含 rule_version 信息
 * - 告警类型来自规则定义
 */
public class BusinessAlertGenerator extends KeyedProcessFunction<String, DetectionBusiness, Tuple2<Alert, Evidence>> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(BusinessAlertGenerator.class);

    // 配置参数
    private final long dedupWindowMinutes;
    private final String arkimeUrl;
    private final int arkimeTimeBufferSeconds;

    // Severity 阈值配置（与 AlertGenerator 同源，避免口径分裂）
    private final float severityCriticalThreshold;
    private final float severityHighThreshold;
    private final float severityMediumThreshold;
    private final float severityLowThreshold;

    // 去重状态（带 TTL）：key = fingerprint
    private transient MapState<String, DedupState> dedupState;

    // Metrics
    private transient Counter alertsGenerated;
    private transient Counter alertsDeduplicated;
    private transient Counter alertsUpdated;
    private transient Counter evidencesGenerated;
    private transient Counter alertsRejected;

    /**
     * 构造函数
     */
    public BusinessAlertGenerator(
            long dedupWindowMinutes,
            String arkimeUrl,
            int arkimeTimeBufferSeconds,
            float severityCriticalThreshold,
            float severityHighThreshold,
            float severityMediumThreshold,
            float severityLowThreshold
    ) {
        this.dedupWindowMinutes = dedupWindowMinutes;
        this.arkimeUrl = arkimeUrl;
        this.arkimeTimeBufferSeconds = arkimeTimeBufferSeconds;
        this.severityCriticalThreshold = severityCriticalThreshold;
        this.severityHighThreshold = severityHighThreshold;
        this.severityMediumThreshold = severityMediumThreshold;
        this.severityLowThreshold = severityLowThreshold;
    }

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        // 配置 State TTL
        StateTtlConfig ttlConfig = StateTtlConfig
                .newBuilder(Time.minutes(dedupWindowMinutes * 2))
                .setUpdateType(StateTtlConfig.UpdateType.OnReadAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .cleanupFullSnapshot()
                .build();

        // 初始化去重状态
        MapStateDescriptor<String, DedupState> dedupDescriptor = new MapStateDescriptor<>(
                "business-dedup-map-state",
                TypeInformation.of(String.class),
                TypeInformation.of(new TypeHint<DedupState>() {})
        );
        dedupDescriptor.enableTimeToLive(ttlConfig);
        dedupState = getRuntimeContext().getMapState(dedupDescriptor);

        // 注册 Metrics
        alertsGenerated = getRuntimeContext()
                .getMetricGroup()
                .counter("business_alerts_generated_total");

        alertsDeduplicated = getRuntimeContext()
                .getMetricGroup()
                .counter("business_alerts_deduplicated_total");

        alertsUpdated = getRuntimeContext()
                .getMetricGroup()
                .counter("business_alerts_updated_total");

        evidencesGenerated = getRuntimeContext()
                .getMetricGroup()
                .counter("business_evidences_generated_total");

        alertsRejected = getRuntimeContext()
                .getMetricGroup()
                .counter("business_alerts_rejected_total");

        LOG.info("BusinessAlertGenerator initialized: dedupWindow={}min, arkimeUrl={}, " +
                        "thresholds=[critical={}, high={}, medium={}, low={}]",
                dedupWindowMinutes, arkimeUrl,
                severityCriticalThreshold, severityHighThreshold,
                severityMediumThreshold, severityLowThreshold);
    }

    @Override
    public void processElement(
            DetectionBusiness detection,
            Context ctx,
            Collector<Tuple2<Alert, Evidence>> out
    ) throws Exception {

        if (detection == null || !detection.hasHeader()) {
            LOG.warn("Received null/broken business detection, skipping");
            if (alertsRejected != null) alertsRejected.inc();
            return;
        }
        EventHeader header = detection.getHeader();
        String tenantId = header.getTenantId();
        if (tenantId == null || tenantId.trim().isEmpty()) {
            LOG.warn("Business detection has empty tenant_id, rejecting eventId={}", header.getEventId());
            if (alertsRejected != null) alertsRejected.inc();
            return;
        }
        String communityId = detection.getCommunityId();
        if (communityId == null || communityId.isEmpty()) {
            LOG.warn("Business detection has empty community_id, rejecting eventId={}", header.getEventId());
            if (alertsRejected != null) alertsRejected.inc();
            return;
        }
        long currentTime = detection.getTs();

        // 生成去重指纹
        String fingerprint = generateFingerprint(
                tenantId,
                detection.getDetectionType(),
                detection.getLabel(),
                communityId
        );

        // 获取去重状态（按指纹）
        DedupState state = dedupState.get(fingerprint);

        // 判断是否需要去重
        if (state != null && state.getFingerprint().equals(fingerprint)) {
            long windowStartTime = currentTime - (dedupWindowMinutes * 60 * 1000);

            if (state.getLastSeen() >= windowStartTime) {
                // 更新状态
                state.setLastSeen(currentTime);
                state.setCount(state.getCount() + 1);
                state.setStateVersion(state.getStateVersion() + 1);
                dedupState.put(fingerprint, state);

                // 输出更新事件
                Alert updateAlert = buildUpdateAlert(
                        detection,
                        state.getAlertId(),
                        fingerprint,
                        state.getFirstSeen(),
                        currentTime,
                        state.getCount(),
                        state.getStateVersion()
                );

                out.collect(Tuple2.of(updateAlert, null));

                alertsDeduplicated.inc();
                alertsUpdated.inc();
                return;
            }
        }

        // 生成新告警
        String alertId = generateAlertId(detection, currentTime);
        String evidenceId = generateEvidenceId(alertId);
        String arkimeLink = generateArkimeLink(communityId, currentTime);

        // 构建 Alert
        Alert alert = buildNewAlert(
                detection,
                alertId,
                fingerprint,
                currentTime,
                arkimeLink,
                evidenceId
        );

        // 构建 Evidence
        Evidence evidence = buildEvidence(
                detection,
                alertId,
                evidenceId,
                currentTime,
                arkimeLink
        );

        // 更新去重状态
        DedupState newState = new DedupState();
        newState.setFingerprint(fingerprint);
        newState.setAlertId(alertId);
        newState.setFirstSeen(currentTime);
        newState.setLastSeen(currentTime);
        newState.setCount(1);
        newState.setStateVersion(1);
        dedupState.put(fingerprint, newState);

        // 输出
        out.collect(Tuple2.of(alert, evidence));

        alertsGenerated.inc();
        evidencesGenerated.inc();

        LOG.debug("New business alert generated: id={}, type={}, label={}",
                alertId, detection.getDetectionType(), detection.getLabel());
    }

    /**
     * 生成去重指纹
     */
    private String generateFingerprint(
            String tenantId,
            String detectionType,
            String label,
            String communityId
    ) {
        try {
            String raw = String.format("%s:%s:%s:%s", tenantId, detectionType, label, communityId);
            MessageDigest md = MessageDigest.getInstance("MD5");
            byte[] hash = md.digest(raw.getBytes(StandardCharsets.UTF_8));

            StringBuilder sb = new StringBuilder();
            for (byte b : hash) {
                sb.append(String.format("%02x", b));
            }
            return sb.toString();
        } catch (Exception e) {
            LOG.error("Failed to generate fingerprint", e);
            return DeterministicId.shortId(
                    "flink-business-alert-fingerprint-fallback/v1", 32,
                    tenantId, detectionType, label, communityId);
        }
    }

    /**
     * 生成 Alert ID
     */
    private String generateAlertId(DetectionBusiness detection, long timestamp) {
        String tenantId = detection.getHeader().getTenantId();
        return String.format("alert-biz-%s-%d-%s",
                tenantId,
                timestamp,
                DeterministicId.shortId(
                        "flink-business-alert/v1", 8,
                        tenantId,
                        detection.getHeader().getEventId(),
                        detection.getDetectionType(),
                        detection.getRuleVersion(),
                        timestamp));
    }

    /**
     * 生成 Evidence ID
     */
    private String generateEvidenceId(String alertId) {
        return String.format("evidence-%s-%s",
                alertId.substring(10),
                DeterministicId.shortId(
                        "flink-business-evidence/v1", 8, alertId));
    }

    /**
     * 生成 Arkime 链接
     */
    private String generateArkimeLink(String communityId, long timestamp) {
        if (arkimeUrl == null || arkimeUrl.isEmpty()) {
            return "";
        }

        long startTime = timestamp - (arkimeTimeBufferSeconds * 1000L);
        long endTime = timestamp + (arkimeTimeBufferSeconds * 1000L);

        // communityId 含 '=' 等字符（base64 填充），必须 URL 编码后再拼入 query
        String encodedCommunityId;
        try {
            encodedCommunityId = URLEncoder.encode(communityId, StandardCharsets.UTF_8.name());
        } catch (Exception e) {
            LOG.warn("Failed to URL-encode communityId={}, using raw value", communityId);
            encodedCommunityId = communityId;
        }

        return String.format("%s?date=-1&expression=community.id==%s&startTime=%d&stopTime=%d",
                arkimeUrl,
                encodedCommunityId,
                startTime / 1000,
                endTime / 1000);
    }

    /**
     * 映射严重程度（使用与 AlertGenerator 同源的可配置阈值）
     */
    private Severity mapSeverity(float score) {
        if (score >= severityCriticalThreshold) {
            return Severity.SEVERITY_CRITICAL;
        } else if (score >= severityHighThreshold) {
            return Severity.SEVERITY_HIGH;
        } else if (score >= severityMediumThreshold) {
            return Severity.SEVERITY_MEDIUM;
        } else if (score >= severityLowThreshold) {
            return Severity.SEVERITY_LOW;
        } else {
            return Severity.SEVERITY_INFO;
        }
    }

    /**
     * 构建新告警
     */
    private Alert buildNewAlert(
            DetectionBusiness detection,
            String alertId,
            String fingerprint,
            long currentTime,
            String arkimeLink,
            String evidenceId
    ) {
        EventHeader header = detection.getHeader();
        Severity severity = mapSeverity(detection.getScore());

        List<String> labels = new ArrayList<>();
        labels.add(detection.getDetectionType());
        labels.add(detection.getLabel());

        List<String> evidenceIds = new ArrayList<>();
        evidenceIds.add(evidenceId);

        // DetectionBusiness 不携带五元组（proto 无 tuple 字段，禁止伪造）。
        // 以显式 UNKNOWN 占位：srcIp/dstIp 为空、协议 0（UNKNOWN），
        // 而不是伪造成 0.0.0.0/TCP。真实五元组需要 proto 契约扩展后接入。
        return Alert.newBuilder()
                .setTenantId(header.getTenantId())
                .setAlertId(alertId)
                .setFirstSeen(currentTime)
                .setLastSeen(currentTime)
                .setSeverity(severity)
                .setAlertType(detection.getDetectionType())
                .setScore(detection.getScore())
                .addAllLabels(labels)
                .setSrcIp("")
                .setDstIp("")
                .setSrcPort(0)
                .setDstPort(0)
                .setProtocol(0)
                .setProtocolName("UNKNOWN")
                .setCommunityId(detection.getCommunityId())
                .setSessionId(detection.getSessionId())
                .setCampaignId(detection.getCampaignId())
                .setModelVersion(detection.getModelVersion())
                .setRuleVersion(detection.getRuleVersion())
                .setFeatureSetId(header.getFeatureSetId())
                .setStatus(AlertStatus.ALERT_STATUS_NEW)
                .setAssignee("")
                .addAllEvidenceIds(evidenceIds)
                .setDedupFingerprint(fingerprint)
                .setUpdatedTs(currentTime)
                .setEventId(DeterministicId.uuid(
                        "flink-business-alert-event/v1", alertId, fingerprint, currentTime, 1))
                .setTraceId(header.getTraceId())
                .setIngestTs(System.currentTimeMillis())
                .setCount(1)
                .setArkimeSessionLink(arkimeLink)
                .setFeedbackLabel("")
                .setFeedbackCount(0)
                .setStateVersion(1)
                .build();
    }

    /**
     * 构建更新告警
     */
    private Alert buildUpdateAlert(
            DetectionBusiness detection,
            String alertId,
            String fingerprint,
            long firstSeen,
            long lastSeen,
            int count,
            long stateVersion
    ) {
        EventHeader header = detection.getHeader();
        Severity severity = mapSeverity(detection.getScore());

        List<String> labels = new ArrayList<>();
        labels.add(detection.getDetectionType());
        labels.add(detection.getLabel());

        return Alert.newBuilder()
                .setTenantId(header.getTenantId())
                .setAlertId(alertId)
                .setFirstSeen(firstSeen)
                .setLastSeen(lastSeen)
                .setSeverity(severity)
                .setAlertType(detection.getDetectionType())
                .setScore(detection.getScore())
                .addAllLabels(labels)
                .setSrcIp("")
                .setDstIp("")
                .setSrcPort(0)
                .setDstPort(0)
                .setProtocol(0)
                .setProtocolName("UNKNOWN")
                .setCommunityId(detection.getCommunityId())
                .setSessionId(detection.getSessionId())
                .setCampaignId(detection.getCampaignId())
                .setModelVersion(detection.getModelVersion())
                .setRuleVersion(detection.getRuleVersion())
                .setFeatureSetId(header.getFeatureSetId())
                .setStatus(AlertStatus.ALERT_STATUS_NEW)
                .setAssignee("")
                .setDedupFingerprint(fingerprint)
                .setUpdatedTs(lastSeen)
                .setEventId(DeterministicId.uuid(
                        "flink-business-alert-event/v1", alertId, fingerprint, lastSeen, stateVersion))
                .setTraceId(header.getTraceId())
                .setIngestTs(System.currentTimeMillis())
                .setCount(count)
                .setArkimeSessionLink(generateArkimeLink(detection.getCommunityId(), lastSeen))
                .setFeedbackLabel("")
                .setFeedbackCount(0)
                .setStateVersion(stateVersion)
                .build();
    }

    /**
     * 构建 Evidence
     */
    private Evidence buildEvidence(
            DetectionBusiness detection,
            String alertId,
            String evidenceId,
            long currentTime,
            String arkimeLink
    ) {
        EventHeader header = detection.getHeader();

        EvidenceBuilder builder = new EvidenceBuilder(
                header.getTenantId(),
                evidenceId,
                alertId
        );

        String summary = String.format(
                "业务规则检测 [%s]: %s，置信度 %.2f%%",
                detection.getDetectionType(),
                detection.getLabel(),
                detection.getScore() * 100
        );

        builder.setType("business_detection")
                .setSummary(summary)
                .setConfidence(detection.getScore())
                .setArkimeLink(arkimeLink)
                .addMetric("detection_type", detection.getDetectionType())
                .addMetric("label", detection.getLabel())
                .addMetric("model_version", detection.getModelVersion())
                .addMetric("rule_version", detection.getRuleVersion())
                .addMetric("community_id", detection.getCommunityId())
                .addMetric("session_id", detection.getSessionId())
                .addMetric("campaign_id", detection.getCampaignId())
                .addMetric("feature_set_id", header.getFeatureSetId())
                .addMetric("probe_id", header.getProbeId())
                .addMetric("run_id", header.getRunId());

        builder.addSnippet("detection_source", "rule_engine")
                .addSnippet("detection_timestamp", String.valueOf(detection.getTs()));

        return builder.build(currentTime);
    }
}
