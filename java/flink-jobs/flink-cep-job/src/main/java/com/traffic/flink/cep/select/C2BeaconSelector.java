////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/select/C2BeaconSelector.java
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
 * C2 信标选择器
 * 
 * 从 CEP 模式匹配结果构建 Campaign
 * 攻击阶段：命令与控制
 */
public class C2BeaconSelector extends PatternProcessFunction<Alert, Campaign> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(C2BeaconSelector.class);

    @Override
    public void processMatch(
            Map<String, List<Alert>> pattern,
            Context ctx,
            Collector<Campaign> out
    ) throws Exception {
        List<Alert> beaconAlerts = pattern.get("beacon");

        if (beaconAlerts == null || beaconAlerts.size() < 5) {
            LOG.warn("Invalid C2 beacon pattern: beacons={}", 
                    beaconAlerts != null ? beaconAlerts.size() : 0);
            return;
        }

        try {
            Campaign campaign = buildCampaign(beaconAlerts);
            if (campaign != null) {
                out.collect(campaign);
                LOG.info("Generated C2 Beacon Campaign: id={}, beacons={}, score={}", 
                        campaign.getCampaignId(), beaconAlerts.size(), campaign.getScore());
            }
        } catch (Exception e) {
            LOG.error("Error creating C2 beacon campaign: {}", e.getMessage(), e);
        }
    }

    private Campaign buildCampaign(List<Alert> beaconAlerts) {
        // 按时间排序
        beaconAlerts.sort(Comparator.comparingLong(Alert::getFirstSeen));

        if (beaconAlerts.isEmpty()) {
            return null;
        }

        // 提取时间范围
        long tsStart = beaconAlerts.stream()
                .mapToLong(Alert::getFirstSeen)
                .min()
                .orElse(System.currentTimeMillis());
        
        long tsEnd = beaconAlerts.stream()
                .mapToLong(Alert::getLastSeen)
                .max()
                .orElse(System.currentTimeMillis());

        // 提取实体（统一使用 ip: 前缀）
        Set<String> entities = extractEntities(beaconAlerts);

        // 提取告警 ID
        List<String> alertIds = beaconAlerts.stream()
                .map(Alert::getAlertId)
                .distinct()
                .collect(Collectors.toList());

        // 计算平均间隔
        long avgInterval = calculateAverageInterval(beaconAlerts);

        // 计算评分（信标数量越多，分数越高）
        float score = calculateScore(beaconAlerts, avgInterval);

        // 生成摘要
        String summary = buildSummary(beaconAlerts, avgInterval);

        // 提取租户信息
        String tenantId = beaconAlerts.get(0).getTenantId();
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
        List<String> attackPhases = Collections.singletonList("command_and_control");

        // 提取规则/模型 ID
        List<String> ruleIds = extractRuleIds(beaconAlerts);
        List<String> modelIds = extractModelIds(beaconAlerts);

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
                .setCampaignType(CampaignType.C2_COMMUNICATION.getCode())
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
     * 计算平均间隔
     */
    private long calculateAverageInterval(List<Alert> beaconAlerts) {
        if (beaconAlerts.size() < 2) {
            return 0;
        }
        
        long totalInterval = 0;
        for (int i = 1; i < beaconAlerts.size(); i++) {
            totalInterval += beaconAlerts.get(i).getFirstSeen() - 
                             beaconAlerts.get(i - 1).getFirstSeen();
        }
        
        return totalInterval / (beaconAlerts.size() - 1);
    }

    /**
     * 计算评分
     */
    private float calculateScore(List<Alert> beaconAlerts, long avgInterval) {
        // 基础分：信标数量
        float baseScore = Math.min((float) beaconAlerts.size() / 20.0f, 0.5f);
        
        // 规律性加分：间隔越规律，分数越高
        float regularityScore = calculateRegularityScore(beaconAlerts, avgInterval);
        
        // 告警本身的平均分
        float avgAlertScore = 0.0f;
        for (Alert alert : beaconAlerts) {
            avgAlertScore += alert.getScore();
        }
        avgAlertScore = avgAlertScore / beaconAlerts.size() * 0.2f;
        
        return Math.min(baseScore + regularityScore + avgAlertScore, 1.0f);
    }

    /**
     * 计算规律性分数
     */
    private float calculateRegularityScore(List<Alert> beaconAlerts, long avgInterval) {
        if (beaconAlerts.size() < 3 || avgInterval == 0) {
            return 0.1f;
        }
        
        // 计算间隔标准差
        double sumSquaredDiff = 0;
        for (int i = 1; i < beaconAlerts.size(); i++) {
            long interval = beaconAlerts.get(i).getFirstSeen() - beaconAlerts.get(i - 1).getFirstSeen();
            double diff = interval - avgInterval;
            sumSquaredDiff += diff * diff;
        }
        double stdDev = Math.sqrt(sumSquaredDiff / (beaconAlerts.size() - 1));
        
        // 变异系数越小，规律性越高
        double cv = stdDev / avgInterval;
        if (cv < 0.1) {
            return 0.3f;  // 非常规律
        } else if (cv < 0.2) {
            return 0.2f;  // 比较规律
        } else if (cv < 0.3) {
            return 0.1f;  // 一般
        } else {
            return 0.05f; // 不太规律
        }
    }

    /**
     * 构建摘要
     */
    private String buildSummary(List<Alert> beaconAlerts, long avgInterval) {
        Alert first = beaconAlerts.get(0);
        Alert last = beaconAlerts.get(beaconAlerts.size() - 1);
        
        long durationMs = last.getLastSeen() - first.getFirstSeen();
        String duration = formatDuration(durationMs);
        
        return String.format(
                "检测到C2信标通信：受控主机 %s 在 %s 内与C2服务器 %s 进行了 %d 次周期性通信，平均间隔 %d 秒",
                first.getSrcIp(),
                duration,
                first.getDstIp(),
                beaconAlerts.size(),
                avgInterval / 1000
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
        return String.format("campaign-c2-%s-%d-%s", 
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