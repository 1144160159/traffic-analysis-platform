package com.traffic.flink.alert.generator;

import com.traffic.flink.alert.dedup.DedupState;
import com.traffic.flink.alert.evidence.EvidenceBuilder;
import com.traffic.proto.traffic.v1.*;

import org.apache.flink.api.common.state.StateTtlConfig;
import org.apache.flink.api.common.state.ValueState;
import org.apache.flink.api.common.state.ValueStateDescriptor;
import org.apache.flink.api.common.time.Time;
import org.apache.flink.api.common.typeinfo.TypeHint;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.api.java.tuple.Tuple2;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.Gauge;
import org.apache.flink.streaming.api.functions.KeyedProcessFunction;
import org.apache.flink.util.Collector;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;

/**
 * Alert 生成器 (重构版)
 * 
 * 核心功能：
 * 1. 从 DetectionBehavior 生成 Alert
 * 2. 基于指纹的去重聚合（支持更新事件输出）
 * 3. State TTL 自动清理
 * 4. 生成 Evidence 并关联 Arkime 链接
 * 5. 支持五元组信息提取
 * 
 * 修复内容：
 * - 添加 State TTL 防止内存泄漏
 * - 去重命中时输出更新事件（支持 ReplacingMergeTree）
 * - 修复五元组提取逻辑
 * - 指纹计算与设计文档一致
 * - Arkime 时间窗口可配置
 * - Severity 阈值可配置
 */
public class AlertGenerator extends KeyedProcessFunction<String, DetectionBehavior, Tuple2<Alert, Evidence>> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(AlertGenerator.class);

    // 配置参数
    private final long dedupWindowMinutes;
    private final String arkimeUrl;
    private final int arkimeTimeBufferSeconds;
    
    // Severity 阈值配置
    private final float severityCriticalThreshold;
    private final float severityHighThreshold;
    private final float severityMediumThreshold;
    private final float severityLowThreshold;

    // 去重状态（带 TTL）
    private transient ValueState<DedupState> dedupState;

    // Metrics
    private transient Counter alertsGenerated;
    private transient Counter alertsDeduplicated;
    private transient Counter alertsUpdated;
    private transient Counter evidencesGenerated;
    private transient long currentStateCount;

    /**
     * 构造函数（简化版，使用默认阈值）
     */
    public AlertGenerator(long dedupWindowMinutes, String arkimeUrl) {
        this(dedupWindowMinutes, arkimeUrl, 120, 0.9f, 0.7f, 0.5f, 0.3f);
    }

    /**
     * 构造函数（完整版）
     * 
     * @param dedupWindowMinutes 去重窗口（分钟）
     * @param arkimeUrl Arkime 基础 URL
     * @param arkimeTimeBufferSeconds Arkime 查询时间缓冲（秒）
     * @param severityCriticalThreshold Critical 严重度阈值
     * @param severityHighThreshold High 严重度阈值
     * @param severityMediumThreshold Medium 严重度阈值
     * @param severityLowThreshold Low 严重度阈值
     */
    public AlertGenerator(
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

        // 配置 State TTL（去重窗口的 2 倍，确保有足够缓冲）
        StateTtlConfig ttlConfig = StateTtlConfig
                .newBuilder(Time.minutes(dedupWindowMinutes * 2))
                .setUpdateType(StateTtlConfig.UpdateType.OnReadAndWrite)
                .setStateVisibility(StateTtlConfig.StateVisibility.NeverReturnExpired)
                .cleanupFullSnapshot()
                .build();

        // 初始化去重状态（带 TTL）
        ValueStateDescriptor<DedupState> dedupDescriptor = new ValueStateDescriptor<>(
                "dedup-state",
                TypeInformation.of(new TypeHint<DedupState>() {})
        );
        dedupDescriptor.enableTimeToLive(ttlConfig);
        dedupState = getRuntimeContext().getState(dedupDescriptor);

        // 注册 Metrics
        alertsGenerated = getRuntimeContext()
                .getMetricGroup()
                .counter("alerts_generated_total");

        alertsDeduplicated = getRuntimeContext()
                .getMetricGroup()
                .counter("alerts_deduplicated_total");

        alertsUpdated = getRuntimeContext()
                .getMetricGroup()
                .counter("alerts_updated_total");

        evidencesGenerated = getRuntimeContext()
                .getMetricGroup()
                .counter("evidences_generated_total");

        // 状态数量 Gauge
        getRuntimeContext()
                .getMetricGroup()
                .gauge("dedup_state_count", (Gauge<Long>) () -> currentStateCount);

        LOG.info("AlertGenerator initialized: dedupWindow={}min, arkimeUrl={}, timeBuffer={}s, " +
                        "thresholds=[critical={}, high={}, medium={}, low={}]",
                dedupWindowMinutes, arkimeUrl, arkimeTimeBufferSeconds,
                severityCriticalThreshold, severityHighThreshold,
                severityMediumThreshold, severityLowThreshold);
    }

    @Override
    public void processElement(
            DetectionBehavior detection,
            Context ctx,
            Collector<Tuple2<Alert, Evidence>> out
    ) throws Exception {

        // 空值保护：过滤 null detection
        if (detection == null) {
            LOG.warn("Received null detection, skipping");
            return;
        }

        // 提取基本信息
        EventHeader header = detection.getHeader();
        if (header == null) {
            LOG.warn("Detection has null header, skipping: communityId={}",
                    detection.getCommunityId());
            return;
        }
        String tenantId = header.getTenantId();
        if (tenantId == null || tenantId.isEmpty()) {
            LOG.warn("Detection has empty tenant_id, using default");
            tenantId = "default";
        }
        String communityId = detection.getCommunityId();
        if (communityId == null || communityId.isEmpty()) {
            LOG.debug("Detection has empty community_id, skipping");
            return;
        }
        String topLabel = detection.getTopLabel();
        if (topLabel == null || topLabel.isEmpty()) {
            LOG.debug("Detection has empty top_label, skipping");
            return;
        }
        long currentTime = detection.getTs();

        // 提取五元组信息（从 community_id 或 extra 字段）
        FiveTupleInfo tupleInfo = extractFiveTuple(detection);

        // 生成去重指纹（按设计文档：alert_type + src_ip + dst_ip + dst_port）
        String fingerprint = generateFingerprint(
                tenantId,
                detection.getTopLabel(),
                tupleInfo.srcIp,
                tupleInfo.dstIp,
                tupleInfo.dstPort
        );

        // 获取去重状态
        DedupState state = dedupState.value();

        // 判断是否需要去重
        if (state != null && state.getFingerprint().equals(fingerprint)) {
            // 检查是否在去重窗口内
            long windowStartTime = currentTime - (dedupWindowMinutes * 60 * 1000);

            if (state.getLastSeen() >= windowStartTime) {
                // 在去重窗口内，更新状态并输出更新事件
                state.setLastSeen(currentTime);
                state.setCount(state.getCount() + 1);
                state.setStateVersion(state.getStateVersion() + 1);
                dedupState.update(state);

                // 构建更新告警（用于 ReplacingMergeTree 合并）
                Alert updateAlert = buildUpdateAlert(
                        detection,
                        state.getAlertId(),
                        fingerprint,
                        state.getFirstSeen(),
                        currentTime,
                        state.getCount(),
                        state.getStateVersion(),
                        tupleInfo
                );

                // 输出更新事件（Evidence 为 null）
                out.collect(Tuple2.of(updateAlert, null));

                alertsDeduplicated.inc();
                alertsUpdated.inc();

                LOG.debug("Alert updated (dedup): id={}, fingerprint={}, count={}",
                        state.getAlertId(), fingerprint, state.getCount());
                return;
            }
        }

        // 生成新告警
        String alertId = generateAlertId(tenantId, currentTime);
        String evidenceId = generateEvidenceId(alertId);

        // 生成 Arkime 链接
        String arkimeLink = generateArkimeLink(communityId, currentTime);

        // 构建 Alert
        Alert alert = buildNewAlert(
                detection,
                alertId,
                fingerprint,
                currentTime,
                tupleInfo,
                arkimeLink,
                evidenceId
        );

        // 构建 Evidence
        Evidence evidence = buildEvidence(
                detection,
                alertId,
                evidenceId,
                currentTime,
                arkimeLink,
                tupleInfo
        );

        // 更新去重状态
        DedupState newState = new DedupState();
        newState.setFingerprint(fingerprint);
        newState.setAlertId(alertId);
        newState.setFirstSeen(currentTime);
        newState.setLastSeen(currentTime);
        newState.setCount(1);
        newState.setStateVersion(1);
        dedupState.update(newState);

        currentStateCount++;

        // 输出 Alert 和 Evidence
        out.collect(Tuple2.of(alert, evidence));

        alertsGenerated.inc();
        evidencesGenerated.inc();

        LOG.debug("New alert generated: id={}, type={}, score={}, severity={}",
                alertId, alert.getAlertType(), alert.getScore(), alert.getSeverity());
    }

    /**
     * 提取五元组信息
     * 
     * 优先从 detection 的 extra 字段提取，如果没有则尝试解析 community_id
     */
    private FiveTupleInfo extractFiveTuple(DetectionBehavior detection) {
        FiveTupleInfo info = new FiveTupleInfo();

        // 方法1：从 extra 字段提取（如果上游 Flink Job 已添加）
        // 假设 extra 字段格式：[srcIp_hash, dstIp_hash, srcPort, dstPort, protocol]
        // 这里需要根据实际的上游实现调整

        // 方法2：从 object_id 关联查询（需要额外实现）
        // 当前简化处理：使用 community_id 的 hash 作为标识

        // 方法3：解析 community_id（标准格式包含五元组信息的 hash）
        // community_id 格式: "1:<base64_of_sha1>"
        // 由于 community_id 是单向 hash，无法直接解析出五元组
        // 因此需要从上游携带

        // 当前实现：尝试从 detection 的上下文提取
        // 如果无法获取，使用 community_id 作为唯一标识
        String communityId = detection.getCommunityId();

        // 检查是否有 extra 字段携带五元组信息
        if (detection.getLabelsCount() > 0) {
            // 尝试从 labels 中提取 IP 信息（某些模型会输出）
            for (String label : detection.getLabelsList()) {
                if (label.startsWith("src_ip:")) {
                    info.srcIp = label.substring(7);
                } else if (label.startsWith("dst_ip:")) {
                    info.dstIp = label.substring(7);
                } else if (label.startsWith("dst_port:")) {
                    try {
                        info.dstPort = Integer.parseInt(label.substring(9));
                    } catch (NumberFormatException e) {
                        // ignore
                    }
                } else if (label.startsWith("src_port:")) {
                    try {
                        info.srcPort = Integer.parseInt(label.substring(9));
                    } catch (NumberFormatException e) {
                        // ignore
                    }
                } else if (label.startsWith("protocol:")) {
                    try {
                        info.protocol = Integer.parseInt(label.substring(9));
                    } catch (NumberFormatException e) {
                        // ignore
                    }
                }
            }
        }

        // 如果仍无法获取五元组，使用 community_id 作为唯一标识
        // 并记录警告日志（生产环境应确保上游正确携带信息）
        if (info.srcIp.equals("0.0.0.0") && info.dstIp.equals("0.0.0.0")) {
            LOG.warn("Five-tuple info not available for detection, " +
                            "using community_id as identifier: communityId={}, objectId={}",
                    communityId, detection.getObjectId());

            // 使用 community_id 的部分作为伪 IP（仅用于指纹计算）
            // 这不是理想方案，生产环境应确保上游正确携带五元组
            info.srcIp = "cid:" + communityId.substring(0, Math.min(8, communityId.length()));
            info.dstIp = "cid:" + communityId.substring(Math.min(8, communityId.length()));
        }

        return info;
    }

    /**
     * 生成去重指纹
     * 
     * 按设计文档：MD5(alert_type + src_ip + dst_ip + dst_port)
     */
    private String generateFingerprint(
            String tenantId,
            String alertType,
            String srcIp,
            String dstIp,
            int dstPort
    ) {
        try {
            // 按设计文档的指纹格式
            String raw = String.format("%s:%s:%s:%s:%d",
                    tenantId, alertType, srcIp, dstIp, dstPort);

            MessageDigest md = MessageDigest.getInstance("MD5");
            byte[] hash = md.digest(raw.getBytes());

            StringBuilder sb = new StringBuilder();
            for (byte b : hash) {
                sb.append(String.format("%02x", b));
            }

            return sb.toString();
        } catch (Exception e) {
            LOG.error("Failed to generate fingerprint", e);
            return UUID.randomUUID().toString();
        }
    }

    /**
     * 生成确定性 Alert ID (确保幂等)
     */
    private String generateAlertId(String tenantId, long timestamp) {
        // 使用确定性hash避免随机UUID，确保相同输入产生相同alertId
        String raw = tenantId + ":" + timestamp;
        try {
            MessageDigest md = MessageDigest.getInstance("MD5");
            byte[] hash = md.digest(raw.getBytes());
            StringBuilder sb = new StringBuilder();
            for (byte b : hash) sb.append(String.format("%02x", b));
            return String.format("alert-%s-%d-%s", tenantId, timestamp, sb.substring(0, 8));
        } catch (Exception e) {
            return String.format("alert-%s-%d-%s",
                    tenantId, timestamp, UUID.randomUUID().toString().substring(0, 8));
        }
    }

    /**
     * 生成确定性 Evidence ID
     */
    private String generateEvidenceId(String alertId) {
        String raw = alertId + ":evidence";
        try {
            MessageDigest md = MessageDigest.getInstance("MD5");
            byte[] hash = md.digest(raw.getBytes());
            StringBuilder sb = new StringBuilder();
            for (byte b : hash) sb.append(String.format("%02x", b));
            return String.format("evidence-%s-%s",
                    alertId.substring(Math.min(6, alertId.length())),
                    sb.substring(0, 8));
        } catch (Exception e) {
            return String.format("evidence-%s-%s",
                    alertId.substring(Math.min(6, alertId.length())),
                    UUID.randomUUID().toString().substring(0, 8));
        }
    }

    /**
     * 生成 Arkime 链接
     */
    private String generateArkimeLink(String communityId, long timestamp) {
        if (arkimeUrl == null || arkimeUrl.isEmpty()) {
            return "";
        }

        // 使用可配置的时间缓冲
        long startTime = timestamp - (arkimeTimeBufferSeconds * 1000L);
        long endTime = timestamp + (arkimeTimeBufferSeconds * 1000L);

        // 转换为秒（Arkime 使用秒级时间戳）
        return String.format("%s?date=-1&expression=community.id==%s&startTime=%d&stopTime=%d",
                arkimeUrl,
                communityId,
                startTime / 1000,
                endTime / 1000);
    }

    /**
     * 构建新告警
     */
    private Alert buildNewAlert(
            DetectionBehavior detection,
            String alertId,
            String fingerprint,
            long currentTime,
            FiveTupleInfo tupleInfo,
            String arkimeLink,
            String evidenceId
    ) {
        EventHeader header = detection.getHeader();
        String tenantId = (header != null && header.getTenantId() != null && !header.getTenantId().isEmpty())
                ? header.getTenantId() : "default";

        // 提取标签
        List<String> labels = new ArrayList<>(detection.getLabelsList());

        // 映射严重程度
        Severity severity = mapSeverity(detection.getTopScore());

        // 获取协议名称
        String protocolName = getProtocolName(tupleInfo.protocol);

        // 构建 evidence_ids 列表
        List<String> evidenceIds = new ArrayList<>();
        evidenceIds.add(evidenceId);

        return Alert.newBuilder()
                .setTenantId(tenantId)
                .setAlertId(alertId)
                .setFirstSeen(currentTime)
                .setLastSeen(currentTime)
                .setSeverity(severity)
                .setAlertType(detection.getTopLabel())
                .setScore(detection.getTopScore())
                .addAllLabels(labels)
                .setSrcIp(tupleInfo.srcIp)
                .setDstIp(tupleInfo.dstIp)
                .setSrcPort(tupleInfo.srcPort)
                .setDstPort(tupleInfo.dstPort)
                .setProtocol(tupleInfo.protocol)
                .setProtocolName(protocolName)
                .setCommunityId(detection.getCommunityId())
                .setSessionId(detection.getObjectId())
                .setCampaignId("") // 由 CEP Job 后续填充
                .setModelVersion(detection.getModelVersion())
                .setRuleVersion("") // 行为检测无规则版本
                .setFeatureSetId(header.getFeatureSetId())
                .setStatus(AlertStatus.ALERT_STATUS_NEW)
                .setAssignee("")
                .addAllEvidenceIds(evidenceIds)
                .setDedupFingerprint(fingerprint)
                .setUpdatedTs(currentTime)
                .setEventId(generateDeterministicEventId(alertId, fingerprint, currentTime))
                .setIngestTs(System.currentTimeMillis())
                .setCount(1)
                .setArkimeSessionLink(arkimeLink)
                .setFeedbackLabel("")
                .setFeedbackCount(0)
                .setStateVersion(1)
                .build();
    }

    /**
     * 构建更新告警（用于去重合并）
     */
    private Alert buildUpdateAlert(
            DetectionBehavior detection,
            String alertId,
            String fingerprint,
            long firstSeen,
            long lastSeen,
            int count,
            long stateVersion,
            FiveTupleInfo tupleInfo
    ) {
        EventHeader header = detection.getHeader();
        String tenantId = (header != null && header.getTenantId() != null && !header.getTenantId().isEmpty())
                ? header.getTenantId() : "default";

        // 映射严重程度（可能随着 score 变化）
        Severity severity = mapSeverity(detection.getTopScore());

        // 获取协议名称
        String protocolName = getProtocolName(tupleInfo.protocol);

        // 更新告警只需要关键字段
        // ReplacingMergeTree 会根据 updated_ts 保留最新记录
        return Alert.newBuilder()
                .setTenantId(tenantId)
                .setAlertId(alertId)
                .setFirstSeen(firstSeen)
                .setLastSeen(lastSeen)
                .setSeverity(severity)
                .setAlertType(detection.getTopLabel())
                .setScore(detection.getTopScore())
                .addAllLabels(detection.getLabelsList())
                .setSrcIp(tupleInfo.srcIp)
                .setDstIp(tupleInfo.dstIp)
                .setSrcPort(tupleInfo.srcPort)
                .setDstPort(tupleInfo.dstPort)
                .setProtocol(tupleInfo.protocol)
                .setProtocolName(protocolName)
                .setCommunityId(detection.getCommunityId())
                .setSessionId(detection.getObjectId())
                .setCampaignId("")
                .setModelVersion(detection.getModelVersion())
                .setRuleVersion("")
                .setFeatureSetId(header.getFeatureSetId())
                .setStatus(AlertStatus.ALERT_STATUS_NEW)
                .setAssignee("")
                .setDedupFingerprint(fingerprint)
                .setUpdatedTs(lastSeen) // 使用最新时间
                .setEventId(generateDeterministicEventId(alertId, fingerprint, firstSeen))
                .setIngestTs(System.currentTimeMillis())
                .setCount(count)
                .setArkimeSessionLink(generateArkimeLink(detection.getCommunityId(), lastSeen))
                .setFeedbackLabel("")
                .setFeedbackCount(0)
                .setStateVersion(stateVersion)
                .build();
    }

    /**
     * 生成确定性 event_id（确保幂等）
     * 相同 input 总是生成相同的 event_id
     */
    private String generateDeterministicEventId(String alertId, String fingerprint, long timestamp) {
        try {
            String raw = String.format("%s:%s:%d", alertId, fingerprint, timestamp);
            MessageDigest md = MessageDigest.getInstance("MD5");
            byte[] hash = md.digest(raw.getBytes());
            long msb = 0;
            long lsb = 0;
            for (int i = 0; i < 8; i++) {
                msb = (msb << 8) | (hash[i] & 0xff);
            }
            for (int i = 8; i < 16; i++) {
                lsb = (lsb << 8) | (hash[i] & 0xff);
            }
            // UUID v3 style
            msb = (msb & 0xffffffffffff0fffL) | 0x0000000000003000L;
            lsb = (lsb & 0x3fffffffffffffffL) | 0x8000000000000000L;
            return new UUID(msb, lsb).toString();
        } catch (Exception e) {
            LOG.warn("Failed to generate deterministic event_id, falling back to random", e);
            return UUID.randomUUID().toString();
        }
    }

    /**
     * 构建 Evidence
     */
    private Evidence buildEvidence(
            DetectionBehavior detection,
            String alertId,
            String evidenceId,
            long currentTime,
            String arkimeLink,
            FiveTupleInfo tupleInfo
    ) {
        EventHeader header = detection.getHeader();
        String tenantId = (header != null && header.getTenantId() != null && !header.getTenantId().isEmpty())
                ? header.getTenantId() : "default";

        // 使用 EvidenceBuilder 构建证据
        EvidenceBuilder builder = new EvidenceBuilder(
                tenantId,
                evidenceId,
                alertId
        );

        // 构建摘要
        String summary = String.format(
                "检测到 [%s] 行为异常，置信度 %.2f%%。来源: %s:%d -> %s:%d (%s)",
                detection.getTopLabel(),
                detection.getTopScore() * 100,
                tupleInfo.srcIp,
                tupleInfo.srcPort,
                tupleInfo.dstIp,
                tupleInfo.dstPort,
                getProtocolName(tupleInfo.protocol)
        );

        builder.setType("behavior_detection")
                .setSummary(summary)
                .setConfidence(detection.getTopScore())
                .setArkimeLink(arkimeLink)
                .addMetric("model_version", detection.getModelVersion())
                .addMetric("object_type", detection.getObjectType())
                .addMetric("object_id", detection.getObjectId())
                .addMetric("community_id", detection.getCommunityId())
                .addMetric("top_label", detection.getTopLabel())
                .addMetric("top_score", String.format("%.4f", detection.getTopScore()))
                .addMetric("feature_set_id", header.getFeatureSetId())
                .addMetric("probe_id", header.getProbeId())
                .addMetric("run_id", header.getRunId());

        // 添加五元组信息
        builder.addMetric("src_ip", tupleInfo.srcIp)
                .addMetric("dst_ip", tupleInfo.dstIp)
                .addMetric("src_port", String.valueOf(tupleInfo.srcPort))
                .addMetric("dst_port", String.valueOf(tupleInfo.dstPort))
                .addMetric("protocol", String.valueOf(tupleInfo.protocol));

        // 添加所有标签和分数
        for (int i = 0; i < detection.getLabelsCount(); i++) {
            builder.addMetric("label_" + i, detection.getLabels(i));
            if (i < detection.getScoresCount()) {
                builder.addMetric("score_" + i, String.format("%.4f", detection.getScores(i)));
            }
        }

        // 添加代码片段引用（如果有）
        builder.addSnippet("detection_source", "behavior_model")
                .addSnippet("detection_timestamp", String.valueOf(detection.getTs()));

        return builder.build(currentTime);
    }

    /**
     * 映射严重程度（使用可配置阈值）
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
     * 获取协议名称
     */
    private String getProtocolName(int protocol) {
        switch (protocol) {
            case 1:
                return "ICMP";
            case 6:
                return "TCP";
            case 17:
                return "UDP";
            case 47:
                return "GRE";
            case 50:
                return "ESP";
            case 51:
                return "AH";
            case 58:
                return "ICMPv6";
            case 89:
                return "OSPF";
            case 132:
                return "SCTP";
            default:
                return "PROTO_" + protocol;
        }
    }

    /**
     * 五元组信息内部类
     */
    private static class FiveTupleInfo {
        String srcIp = "0.0.0.0";
        String dstIp = "0.0.0.0";
        int srcPort = 0;
        int dstPort = 0;
        int protocol = 6; // 默认 TCP

        @Override
        public String toString() {
            return String.format("%s:%d -> %s:%d [%d]",
                    srcIp, srcPort, dstIp, dstPort, protocol);
        }
    }
}