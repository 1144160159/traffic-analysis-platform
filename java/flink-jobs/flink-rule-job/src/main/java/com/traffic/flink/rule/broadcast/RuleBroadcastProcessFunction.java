package com.traffic.flink.rule.broadcast;

import com.traffic.flink.common.DeterministicId;
import com.traffic.flink.rule.matcher.*;
import com.traffic.flink.rule.model.*;
import com.traffic.flink.rule.util.CommunityIdParser;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;
import com.traffic.proto.traffic.v1.FiveTuple;

import org.apache.flink.api.common.state.BroadcastState;
import org.apache.flink.api.common.state.MapStateDescriptor;
import org.apache.flink.api.common.state.ReadOnlyBroadcastState;
import org.apache.flink.api.common.typeinfo.TypeHint;
import org.apache.flink.api.common.typeinfo.TypeInformation;
import org.apache.flink.configuration.Configuration;
import org.apache.flink.metrics.Counter;
import org.apache.flink.metrics.Gauge;
import org.apache.flink.metrics.Histogram;
import org.apache.flink.metrics.Meter;
import org.apache.flink.streaming.api.functions.co.BroadcastProcessFunction;
import org.apache.flink.util.Collector;
import org.apache.flink.util.OutputTag;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;

/**
 * 规则广播处理函数（增强版）
 * 
 * 新增功能：
 * 1. IP 字段提取（从 objectId 解析）
 * 2. 规则优先级排序
 * 3. 规则命中统计（按规则维度）
 * 4. 规则更新审计日志
 * 5. 规则解析失败容错
 */
public class RuleBroadcastProcessFunction 
        extends BroadcastProcessFunction<FeatureStat, Rule, DetectionBehavior> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(RuleBroadcastProcessFunction.class);

    public static final OutputTag<RuleUpdateAppliedAck> RULE_UPDATE_ACK_TAG =
            new OutputTag<RuleUpdateAppliedAck>("rule-update-applied-ack") {};

    // 规则状态描述符
    private static final MapStateDescriptor<String, Rule> RULE_STATE_DESC =
            new MapStateDescriptor<>(
                    "rule-state",
                    TypeInformation.of(String.class),
                    TypeInformation.of(new TypeHint<Rule>() {})
            );

    // 匹配器工厂
    private transient MatcherFactory matcherFactory;

    // 匹配上下文（包含 IP 黑名单等）
    private transient MatchContext matchContext;

    // Metrics - 基础
    private transient Counter featuresProcessed;
    private transient Counter rulesMatched;
    private transient Counter rulesUpdated;
    private transient Counter rulesDeleted;
    private transient Counter ruleUpdatesDuplicate;
    private transient Counter ruleUpdatesStale;
    private transient Counter ruleUpdatesConflict;
    private transient Counter ipExtractionFailed;
    
    // Metrics - 规则维度命中统计
    private transient Map<String, Counter> ruleHitCounters;

    // 按租户缓存的已排序启用规则（广播变更时失效；恢复后首次调用重建）
    private transient Map<String, List<Rule>> tenantRulesCache;

    // Metrics - 状态
    private transient volatile int activeRuleCount = 0;
    private transient volatile long lastMatchTime = 0;

    @Override
    public void open(Configuration parameters) throws Exception {
        super.open(parameters);

        // 初始化匹配器工厂
        matcherFactory = new MatcherFactory();
        matcherFactory.initialize();

        // 初始化匹配上下文
        matchContext = new MatchContext();

        // 初始化规则命中计数器
        ruleHitCounters = new ConcurrentHashMap<>();
        tenantRulesCache = new HashMap<>();

        // 注册基础 Metrics
        featuresProcessed = getRuntimeContext()
                .getMetricGroup()
                .counter("features_processed_total");

        rulesMatched = getRuntimeContext()
                .getMetricGroup()
                .counter("rules_matched_total");

        rulesUpdated = getRuntimeContext()
                .getMetricGroup()
                .counter("rules_updated_total");

        rulesDeleted = getRuntimeContext()
                .getMetricGroup()
                .counter("rules_deleted_total");

        ruleUpdatesDuplicate = getRuntimeContext()
                .getMetricGroup()
                .counter("rule_updates_duplicate_total");

        ruleUpdatesStale = getRuntimeContext()
                .getMetricGroup()
                .counter("rule_updates_stale_total");

        ruleUpdatesConflict = getRuntimeContext()
                .getMetricGroup()
                .counter("rule_updates_conflict_total");

        ipExtractionFailed = getRuntimeContext()
                .getMetricGroup()
                .counter("ip_extraction_failed_total");

        getRuntimeContext()
                .getMetricGroup()
                .gauge("active_rule_count", (Gauge<Integer>) () -> activeRuleCount);

        getRuntimeContext()
                .getMetricGroup()
                .gauge("last_match_time_ms", (Gauge<Long>) () -> lastMatchTime);

        LOG.info("RuleBroadcastProcessFunction initialized");
    }

    @Override
    public void close() throws Exception {
        if (matcherFactory != null) {
            matcherFactory.close();
        }
        super.close();
    }

    /**
     * 处理规则更新（广播流）
     */
    @Override
    public void processBroadcastElement(
            Rule rule,
            Context ctx,
            Collector<DetectionBehavior> out
    ) throws Exception {
        BroadcastState<String, Rule> ruleState = ctx.getBroadcastState(RULE_STATE_DESC);

        String ruleKey = buildRuleKey(rule.getTenantId(), rule.getRuleId());
        Rule existingRule = ruleState.get(ruleKey);
        long oldVersion = existingRule != null ? existingRule.getVersion() : 0;
        RuleUpdateStateMachine.Decision decision =
                RuleUpdateStateMachine.decideForRuntime(
                        existingRule,
                        rule,
                        matcherFactory.getMatcher(rule.getType()) != null);

        switch (decision.getStatus()) {
            case APPLIED:
                // 黑名单 IP 现在直接从广播规则读取（BlacklistMatcher.match 按
                // 规则 ip_list 判定），不再维护 MatchContext 内的内存缓存，因此
                // checkpoint 恢复后无需重建缓存即可正确匹配。
                // Disabled/deleted rules remain as tombstones so that a stale
                // replay cannot resurrect a superseded active version.
                ruleState.put(ruleKey, decision.getState());
                // 规则集变更后失效排序缓存
                if (tenantRulesCache != null) {
                    tenantRulesCache.clear();
                }
                if (rule.isEnabled()) rulesUpdated.inc();
                else {
                    rulesDeleted.inc();
                    // 规则停用/删除时清理该规则的命中计数器，避免 metric 无界增长
                    if (ruleHitCounters != null) {
                        ruleHitCounters.remove(rule.getRuleId());
                    }
                }
                LOG.info("[RULE_AUDIT] Rule transition applied: ruleId={}, tenantId={}, "
                                + "version={}→{}, action={}, enabled={}, eventId={}, updatedBy={}",
                        rule.getRuleId(), rule.getTenantId(), oldVersion, rule.getVersion(),
                        rule.getAction(), rule.isEnabled(), rule.getCommandEventId(),
                        rule.getUpdatedBy() != null ? rule.getUpdatedBy() : "system");
                break;
            case DUPLICATE:
                ruleUpdatesDuplicate.inc();
                LOG.info("[RULE_AUDIT] Duplicate rule command ignored: ruleId={}, version={}, eventId={}",
                        rule.getRuleId(), rule.getVersion(), rule.getCommandEventId());
                break;
            case STALE:
                ruleUpdatesStale.inc();
                LOG.warn("[RULE_AUDIT] Stale rule command rejected: ruleId={}, currentVersion={}, "
                                + "incomingVersion={}, eventId={}",
                        rule.getRuleId(), oldVersion, rule.getVersion(), rule.getCommandEventId());
                break;
            case CONFLICT:
                ruleUpdatesConflict.inc();
                LOG.error("[RULE_AUDIT] Conflicting rule command rejected: ruleId={}, version={}, "
                                + "eventId={}, reason={}",
                        rule.getRuleId(), rule.getVersion(), rule.getCommandEventId(),
                        decision.getReason());
                break;
            default:
                throw new IllegalStateException("unknown rule transition decision");
        }

        long currentVersion = decision.getState() == null
                ? oldVersion : decision.getState().getVersion();
        String ackStatus = decision.getStatus().name().toLowerCase(Locale.ROOT);
        String ackError = decision.getStatus() == RuleUpdateStateMachine.Status.CONFLICT
                || decision.getStatus() == RuleUpdateStateMachine.Status.STALE
                ? decision.getReason() : "";
        if (rule.isCanonicalCommandEnvelope()) {
            ctx.output(RULE_UPDATE_ACK_TAG, RuleUpdateAppliedAck.from(
                    rule,
                    currentVersion,
                    ackStatus,
                    ackError,
                    getRuntimeContext().getIndexOfThisSubtask(),
                    getRuntimeContext().getNumberOfParallelSubtasks()));
        } else {
            // Historical flattened records remain readable, but they have no
            // authoritative outbox event to aggregate against in rule-manager.
            LOG.warn("[RULE_AUDIT] Legacy flattened command applied without runtime receipt: "
                            + "ruleId={}, version={}",
                    rule.getRuleId(), rule.getVersion());
        }

        // 更新活跃规则计数
        updateActiveRuleCount(ruleState);
    }

    /**
     * 处理特征流（数据流）
     */
    @Override
    public void processElement(
            FeatureStat feature,
            ReadOnlyContext ctx,
            Collector<DetectionBehavior> out
    ) throws Exception {
        featuresProcessed.inc();
        long startTime = System.nanoTime();

        ReadOnlyBroadcastState<String, Rule> ruleState = ctx.getBroadcastState(RULE_STATE_DESC);
        String tenantId = feature.getHeader().getTenantId();

        // 更新匹配上下文
        updateMatchContext(feature);

        // 收集所有启用的规则并按优先级排序
        List<Rule> sortedRules = getSortedRules(ruleState, tenantId);

        // 按优先级顺序匹配规则
        for (Rule rule : sortedRules) {
            // 获取匹配器
            RuleMatcher matcher = matcherFactory.getMatcher(rule.getType());
            if (matcher == null) {
                LOG.warn("No matcher found for rule type: {}", rule.getType());
                continue;
            }

            // 执行匹配
            try {
                Optional<DetectionResult> result = matcher.match(feature, rule, matchContext);
                if (result.isPresent()) {
                    out.collect(buildDetectionEvent(feature, result.get(), rule));
                    rulesMatched.inc();

                    // 规则命中统计
                    incrementRuleHitCounter(rule.getRuleId());
                }
            } catch (Exception e) {
                // matcher 异常不允许静默吞掉：检测丢失且 offset 照常推进等于
                // 数据丢失。统一上抛，由 Flink 重启策略（fixedDelayRestart）恢复。
                LOG.error("Matcher failure for rule {}, feature {}; failing job for recovery: {}",
                        rule.getRuleId(), feature.getObjectId(), e.getMessage(), e);
                throw new RuntimeException(
                        "rule matcher failure: rule=" + rule.getRuleId()
                                + " feature=" + feature.getObjectId(), e);
            }
        }

        // 更新延迟指标
        long endTime = System.nanoTime();
        lastMatchTime = (endTime - startTime) / 1_000_000; // 转换为毫秒
    }

    /**
     * 获取排序后的规则列表（按优先级降序）
     *
     * 按租户缓存已排序的启用规则；缓存在本实例内有效，并在每次收到广播规则
     * 变更时失效重建。checkpoint 恢复会创建新实例（transient 缓存为空），
     * 首次调用从已恢复的广播状态重建，因此不存在恢复后缓存陈旧的问题。
     */
    private List<Rule> getSortedRules(ReadOnlyBroadcastState<String, Rule> ruleState, String tenantId) throws Exception {
        if (tenantRulesCache == null) {
            tenantRulesCache = new HashMap<>();
        }
        List<Rule> cached = tenantRulesCache.get(tenantId);
        if (cached != null) {
            return cached;
        }

        List<Rule> rules = new ArrayList<>();
        
        for (Map.Entry<String, Rule> entry : ruleState.immutableEntries()) {
            Rule rule = entry.getValue();

            // 租户隔离
            if (!tenantId.equals(rule.getTenantId()) && !"*".equals(rule.getTenantId())) {
                continue;
            }

            // 跳过禁用的规则
            if (!rule.isEnabled()) {
                continue;
            }

            rules.add(rule);
        }

        // 按优先级降序排序（priority 越大越优先）
        rules.sort((r1, r2) -> Integer.compare(r2.getPriority(), r1.getPriority()));

        tenantRulesCache.put(tenantId, rules);
        return rules;
    }

    /**
     * 更新匹配上下文（修复：提取 IP 字段）
     */
    private void updateMatchContext(FeatureStat feature) {
        // 设置基础信息
        matchContext.setTenantId(feature.getHeader().getTenantId());
        matchContext.setProtocol(feature.getProtocol());
        matchContext.setTimestamp(feature.getTs());

        FiveTuple tuple = resolveSourceTuple(feature);

        if (tuple != null) {
            matchContext.setSrcIp(tuple.getSrcIp());
            matchContext.setDstIp(tuple.getDstIp());
            matchContext.setSrcPort(tuple.getSrcPort());
            matchContext.setDstPort(tuple.getDstPort());
        } else {
            // objectId 解析失败，设置为 null
            matchContext.setSrcIp(null);
            matchContext.setDstIp(null);
            matchContext.setSrcPort(0);
            matchContext.setDstPort(0);
            
            ipExtractionFailed.inc();
            
            if (LOG.isDebugEnabled()) {
                LOG.debug("Cannot extract IP from objectId: {}, community_id: {}", 
                        feature.getObjectId(), feature.getCommunityId());
            }
        }
    }

    /**
     * 构建检测事件
     */
    DetectionBehavior buildDetectionEvent(
            FeatureStat feature,
            DetectionResult detection,
            Rule rule) {
        FiveTuple tuple = resolveSourceTuple(feature);
        if (tuple == null) {
            throw new IllegalArgumentException(
                    "rule detection requires an observed source tuple; object_id fallback was not parseable");
        }

        long eventTime = feature.getTs();
        EventHeader inputHeader = feature.getHeader();
        long producedAt = System.currentTimeMillis();
        String eventId = DeterministicId.uuid(
                "flink-rule-detection/v1",
                inputHeader.getTenantId(),
                inputHeader.getEventId(),
                detection.getRuleId(),
                rule.getVersion(),
                detection.getRuleType().getValue(),
                eventTime,
                "rule-engine-v1");
        EventHeader header = EventHeader.newBuilder()
                .setEventId(eventId)
                .setTenantId(inputHeader.getTenantId())
                .setRunId(inputHeader.getRunId())
                .setEventTs(eventTime)
                .setIngestTs(inputHeader.getIngestTs() > 0 ? inputHeader.getIngestTs() : eventTime)
                .setProbeId(inputHeader.getProbeId())
                .setFeatureSetId(inputHeader.getFeatureSetId())
                .setEventType("traffic.detection.behavior.v1")
                .setSchemaVersion("1")
                .setAggregateType("detection")
                .setAggregateId(feature.getObjectId())
                .setAggregateVersion(rule.getVersion())
                .setOccurredAt(eventTime)
                .setProducedAt(producedAt)
                .setTraceId(nonBlank(inputHeader.getTraceId(), inputHeader.getEventId()))
                .setCausationId(inputHeader.getEventId())
                .setCorrelationId(nonBlank(inputHeader.getCorrelationId(), feature.getCommunityId()))
                .setIdempotencyKey(eventId)
                .setProducer("flink-rule-job")
                .build();

        // 构建标签列表
        List<String> labels = new ArrayList<>();
        labels.add(detection.getRuleType().getValue());
        if (detection.getLabels() != null) {
            labels.addAll(detection.getLabels());
        }
        labels.add("rule_id:" + rule.getRuleId());
        labels.add("rule_version:" + rule.getVersion());
        labels.add("detection_source:rule");

        // 构建分数列表
        List<Float> scores = new ArrayList<>();
        scores.add(detection.getScore());

        return DetectionBehavior.newBuilder()
                .setHeader(header)
                .setModelVersion("rule-engine-v1")
                .setCommunityId(feature.getCommunityId())
                .setObjectType(feature.getObjectType())
                .setObjectId(feature.getObjectId())
                .setTs(eventTime)
                .addAllLabels(labels)
                .addAllScores(scores)
                .setTopLabel(detection.getRuleType().getValue())
                .setTopScore(detection.getScore())
                .setTuple(tuple)
                .addAllEvidenceIds(feature.getEvidenceIdsList())
                .build();
    }

    private FiveTuple resolveSourceTuple(FeatureStat feature) {
        if (feature.hasTuple()
                && !feature.getTuple().getSrcIp().trim().isEmpty()
                && !feature.getTuple().getDstIp().trim().isEmpty()
                && feature.getTuple().getProtocol() > 0) {
            return feature.getTuple();
        }

        // Legacy compatibility is accepted only when object_id reversibly
        // carries the tuple. Community ID is one-way and is never decoded or
        // turned into placeholder addresses.
        CommunityIdParser.FiveTuple legacy = CommunityIdParser.parseObjectId(feature.getObjectId());
        if (legacy == null || feature.getProtocol() == 0) {
            return null;
        }
        return FiveTuple.newBuilder()
                .setSrcIp(legacy.srcIp)
                .setDstIp(legacy.dstIp)
                .setSrcPort(legacy.srcPort)
                .setDstPort(legacy.dstPort)
                .setProtocol(feature.getProtocol())
                .build();
    }

    private static String nonBlank(String value, String fallback) {
        return value == null || value.trim().isEmpty() ? fallback : value;
    }

    /**
     * 增加规则命中计数
     */
    private void incrementRuleHitCounter(String ruleId) {
        Counter counter = ruleHitCounters.computeIfAbsent(ruleId, 
                id -> getRuntimeContext()
                        .getMetricGroup()
                        .addGroup("rule", id)
                        .counter("hit_count"));
        counter.inc();
    }

    /**
     * 更新活跃规则计数
     */
    private void updateActiveRuleCount(BroadcastState<String, Rule> ruleState) throws Exception {
        int count = 0;
        for (Map.Entry<String, Rule> entry : ruleState.entries()) {
            if (entry.getValue().isEnabled()) {
                count++;
            }
        }
        activeRuleCount = count;
    }

    /**
     * 构建规则 Key
     */
    private String buildRuleKey(String tenantId, String ruleId) {
        return tenantId + ":" + ruleId;
    }

    /**
     * 获取规则状态描述符
     */
    public static MapStateDescriptor<String, Rule> getRuleStateDescriptor() {
        return RULE_STATE_DESC;
    }
}
