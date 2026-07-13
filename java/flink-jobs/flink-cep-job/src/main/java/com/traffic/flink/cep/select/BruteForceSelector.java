////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/select/BruteForceSelector.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.cep.select;

import com.traffic.flink.cep.model.CampaignType;
import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.Campaign;
import com.traffic.proto.traffic.v1.EventHeader;

import org.apache.flink.cep.functions.PatternProcessFunction;
import org.apache.flink.util.Collector;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.*;
import java.util.stream.Collectors;

/**
 * 暴力破解选择器
 * 
 * 从 CEP 模式匹配结果构建 Campaign
 * 攻击链：凭据访问 → 初始访问
 */
public class BruteForceSelector extends PatternProcessFunction<Alert, Campaign> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(BruteForceSelector.class);

    @Override
    public void processMatch(
            Map<String, List<Alert>> pattern,
            Context ctx,
            Collector<Campaign> out
    ) throws Exception {
        List<Alert> failedAlerts = pattern.get("failed");
        List<Alert> successAlerts = pattern.get("success");

        if (failedAlerts == null || failedAlerts.isEmpty() || 
            successAlerts == null || successAlerts.isEmpty()) {
            LOG.warn("Invalid brute force pattern: failed={}, success={}", 
                    failedAlerts != null ? failedAlerts.size() : 0,
                    successAlerts != null ? successAlerts.size() : 0);
            return;
        }

        try {
            Campaign campaign = buildCampaign(failedAlerts, successAlerts);
            if (campaign != null) {
                out.collect(campaign);
                LOG.info("Generated Brute Force Campaign: id={}, failed={}, success={}, score={}", 
                        campaign.getCampaignId(), failedAlerts.size(), successAlerts.size(), campaign.getScore());
            }
        } catch (Exception e) {
            LOG.error("Error creating brute force campaign: {}", e.getMessage(), e);
        }
    }

    private Campaign buildCampaign(List<Alert> failedAlerts, List<Alert> successAlerts) {
        // 提取所有告警
        List<Alert> allAlerts = new ArrayList<>();
        allAlerts.addAll(failedAlerts);
        allAlerts.addAll(successAlerts);

        // 按时间排序
        allAlerts.sort(Comparator.comparingLong(Alert::getFirstSeen));

        if (allAlerts.isEmpty()) {
            return null;
        }

        // 提取时间范围
        long tsStart = allAlerts.stream()
                .mapToLong(Alert::getFirstSeen)
                .min()
                .orElse(System.currentTimeMillis());
        
        long tsEnd = allAlerts.stream()
                .mapToLong(Alert::getLastSeen)
                .max()
                .orElse(System.currentTimeMillis());

        // 提取实体（统一使用 ip: 前缀）
        Set<String> entities = extractEntities(allAlerts);

        // 提取告警 ID
        List<String> alertIds = allAlerts.stream()
                .map(Alert::getAlertId)
                .distinct()
                .collect(Collectors.toList());

        // 计算评分（失败次数越多，分数越高）
        float score = calculateScore(failedAlerts, successAlerts);

        // 生成摘要
        String summary = buildSummary(failedAlerts, successAlerts);

        // 提取租户信息
        String tenantId = allAlerts.get(0).getTenantId();
        if (tenantId == null || tenantId.isEmpty()) {
            tenantId = "unknown";
        }

        // 生成 Campaign ID
        String campaignId = generateCampaignId(tenantId, tsStart);

        // 生成 Event ID
        String eventId = UUID.randomUUID().toString();
        long now = System.currentTimeMillis();

        // 构建 EventHeader（Alert 没有 header 字段，使用默认值）
        EventHeader header = EventHeader.newBuilder()
                .setEventId(eventId)
                .setTenantId(tenantId)
                .setRunId("realtime")
                .setEventTs(tsEnd)
                .setIngestTs(now)
                .setProbeId("cep-engine")
                .setFeatureSetId("campaign")
                .build();

        // 攻击阶段
        List<String> attackPhases = Arrays.asList("credential_access", "initial_access");

        // 提取规则/模型 ID
        List<String> ruleIds = extractRuleIds(allAlerts);
        List<String> modelIds = extractModelIds(allAlerts);

        // 构建 Campaign
        return Campaign.newBuilder()
                .setHeader(header)
                .setTenantId(tenantId)
                .setCampaignId(campaignId)
                .setTsStart(tsStart)
                .setTsEnd(tsEnd)
                .addAllEntities(new ArrayList<>(entities))
                .addAllAlerts(alertIds)
                .setScore(score)
                .setSummary(summary)
                .setEventId(eventId)
                .setIngestTs(now)
                .setCampaignType(CampaignType.BRUTE_FORCE.getCode())
                .addAllAttackPhases(attackPhases)
                .addAllRuleIds(ruleIds)
                .addAllModelIds(modelIds)
                .build();
    }

    /**
     * 提取实体（统一使用 ip: 前缀）
     */
    private Set<String> extractEntities(List<Alert> alerts) {
        Set<String> entities = new LinkedHashSet<>();
        for (Alert alert : alerts) {
            if (alert.getSrcIp() != null && !alert.getSrcIp().isEmpty()) {
                entities.add("ip:" + alert.getSrcIp());
            }
            if (alert.getDstIp() != null && !alert.getDstIp().isEmpty()) {
                entities.add("ip:" + alert.getDstIp());
            }
        }
        return entities;
    }

    /**
     * 计算评分
     */
    private float calculateScore(List<Alert> failedAlerts, List<Alert> successAlerts) {
        // 基础分：失败次数越多，分数越高
        float baseScore = Math.min((float) failedAlerts.size() / 10.0f, 0.6f);
        
        // 成功登录加分
        float successBonus = 0.3f;
        
        // 告警本身的平均分
        float avgAlertScore = 0.0f;
        List<Alert> allAlerts = new ArrayList<>(failedAlerts);
        allAlerts.addAll(successAlerts);
        for (Alert alert : allAlerts) {
            avgAlertScore += alert.getScore();
        }
        avgAlertScore = avgAlertScore / allAlerts.size() * 0.1f;
        
        return Math.min(baseScore + successBonus + avgAlertScore, 1.0f);
    }

    /**
     * 构建摘要
     */
    private String buildSummary(List<Alert> failedAlerts, List<Alert> successAlerts) {
        Alert firstFailed = failedAlerts.get(0);
        Alert success = successAlerts.get(0);
        
        long durationMs = success.getLastSeen() - firstFailed.getFirstSeen();
        String duration = formatDuration(durationMs);
        
        return String.format(
                "检测到暴力破解攻击：攻击者 %s 在 %s 内对目标 %s:%d 进行了 %d 次失败尝试后成功登录",
                firstFailed.getSrcIp(),
                duration,
                success.getDstIp(),
                success.getDstPort(),
                failedAlerts.size()
        );
    }

    /**
     * 格式化时间跨度
     */
    private String formatDuration(long durationMs) {
        if (durationMs < 60000) {
            return String.format("%d秒", durationMs / 1000);
        } else if (durationMs < 3600000) {
            return String.format("%d分钟", durationMs / 60000);
        } else {
            return String.format("%.1f小时", durationMs / 3600000.0);
        }
    }

    /**
     * 生成 Campaign ID
     */
    private String generateCampaignId(String tenantId, long tsStart) {
        return String.format("campaign-bf-%s-%d-%s", 
                tenantId, 
                tsStart, 
                UUID.randomUUID().toString().substring(0, 8));
    }

    /**
     * 提取规则 ID
     */
    private List<String> extractRuleIds(List<Alert> alerts) {
        return alerts.stream()
                .map(Alert::getRuleVersion)
                .filter(id -> id != null && !id.isEmpty())
                .distinct()
                .collect(Collectors.toList());
    }

    /**
     * 提取模型 ID
     */
    private List<String> extractModelIds(List<Alert> alerts) {
        return alerts.stream()
                .map(Alert::getModelVersion)
                .filter(id -> id != null && !id.isEmpty())
                .distinct()
                .collect(Collectors.toList());
    }
}