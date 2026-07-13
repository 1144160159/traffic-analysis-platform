package com.traffic.flink.rule.broadcast;

import com.traffic.flink.rule.matcher.*;
import com.traffic.flink.rule.model.*;
import com.traffic.flink.rule.util.CommunityIdParser;
import com.traffic.proto.traffic.v1.DetectionBehavior;
import com.traffic.proto.traffic.v1.EventHeader;
import com.traffic.proto.traffic.v1.FeatureStat;

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
    private transient Counter ipExtractionFailed;
    
    // Metrics - 规则维度命中统计
    private transient Map<String, Counter> ruleHitCounters;
    
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
        RuleAction action = rule.getAction();

        switch (action) {
            case UPDATE:
            case ENABLE:
                if (rule.isEnabled()) {
                    // 检查版本，只更新更新版本
                    Rule existingRule = ruleState.get(ruleKey);
                    long oldVersion = existingRule != null ? existingRule.getVersion() : 0;
                    
                    if (existingRule == null || existingRule.getVersion() < rule.getVersion()) {
                        ruleState.put(ruleKey, rule);
                        
                        // 更新黑名单缓存
                        if (rule.getType() == RuleType.BLACKLIST) {
                            BlacklistMatcher.updateBlacklist(rule, matchContext);
                        }
                        
                        rulesUpdated.inc();
                        
                        // 审计日志
                        LOG.info("[RULE_AUDIT] Rule updated: ruleId={}, tenantId={}, version={}→{}, type={}, enabled={}, updatedBy={}", 
                                rule.getRuleId(), 
                                rule.getTenantId(),
                                oldVersion,
                                rule.getVersion(),
                                rule.getType(),
                                rule.isEnabled(),
                                rule.getUpdatedBy() != null ? rule.getUpdatedBy() : "system");
                    } else {
                        LOG.debug("Ignoring outdated rule version: {} (current: {}, incoming: {})",
                                rule.getRuleId(), existingRule.getVersion(), rule.getVersion());
                    }
                }
                break;

            case DELETE:
            case DISABLE:
                Rule removed = ruleState.get(ruleKey);
                if (removed != null) {
                    ruleState.remove(ruleKey);
                    
                    // 从黑名单缓存移除
                    if (rule.getType() == RuleType.BLACKLIST) {
                        BlacklistMatcher.removeFromBlacklist(rule, matchContext);
                    }
                    
                    rulesDeleted.inc();
                    
                    // 审计日志
                    LOG.info("[RULE_AUDIT] Rule removed: ruleId={}, tenantId={}, action={}, removedBy={}", 
                            rule.getRuleId(), 
                            rule.getTenantId(),
                            action,
                            rule.getUpdatedBy() != null ? rule.getUpdatedBy() : "system");
                }
                break;
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

        // 存储所有匹配结果
        List<DetectionResult> detections = new ArrayList<>();

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
                    detections.add(result.get());
                    
                    // 规则命中统计
                    incrementRuleHitCounter(rule.getRuleId());
                }
            } catch (Exception e) {
                LOG.error("Error matching rule {}: {}", rule.getRuleId(), e.getMessage(), e);
            }
        }

        // 输出检测结果
        for (DetectionResult detection : detections) {
            DetectionBehavior detectionEvent = buildDetectionEvent(feature, detection);
            out.collect(detectionEvent);
            rulesMatched.inc();
        }

        // 更新延迟指标
        long endTime = System.nanoTime();
        lastMatchTime = (endTime - startTime) / 1_000_000; // 转换为毫秒
    }

    /**
     * 获取排序后的规则列表（按优先级降序）
     */
    private List<Rule> getSortedRules(ReadOnlyBroadcastState<String, Rule> ruleState, String tenantId) throws Exception {
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

        // 尝试从 objectId 解析五元组
        // objectId 格式示例：192.168.1.1:443-10.0.0.1:52345
        CommunityIdParser.FiveTuple tuple = CommunityIdParser.parseObjectId(feature.getObjectId());
        
        if (tuple != null) {
            matchContext.setSrcIp(tuple.srcIp);
            matchContext.setDstIp(tuple.dstIp);
            matchContext.setSrcPort(tuple.srcPort);
            matchContext.setDstPort(tuple.dstPort);
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
    private DetectionBehavior buildDetectionEvent(FeatureStat feature, DetectionResult detection) {
        EventHeader header = EventHeader.newBuilder()
                .setEventId(UUID.randomUUID().toString())
                .setTenantId(feature.getHeader().getTenantId())
                .setRunId(feature.getHeader().getRunId())
                .setEventTs(System.currentTimeMillis())
                .setIngestTs(System.currentTimeMillis())
                .setProbeId(feature.getHeader().getProbeId())
                .setFeatureSetId(feature.getHeader().getFeatureSetId())
                .build();

        // 构建标签列表
        List<String> labels = new ArrayList<>();
        labels.add(detection.getRuleType().getValue());
        if (detection.getLabels() != null) {
            labels.addAll(detection.getLabels());
        }

        // 构建分数列表
        List<Float> scores = new ArrayList<>();
        scores.add(detection.getScore());

        return DetectionBehavior.newBuilder()
                .setHeader(header)
                .setModelVersion("rule-engine-v1")
                .setCommunityId(feature.getCommunityId())
                .setObjectType(feature.getObjectType())
                .setObjectId(feature.getObjectId())
                .setTs(System.currentTimeMillis())
                .addAllLabels(labels)
                .addAllScores(scores)
                .setTopLabel(detection.getRuleType().getValue())
                .setTopScore(detection.getScore())
                .build();
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