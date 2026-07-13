////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/select/DataExfilSelector.java
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
 * 数据外泄选择器
 * 
 * 从 CEP 模式匹配结果构建 Campaign
 * 攻击阶段：收集 → 外泄 → 命令与控制
 */
public class DataExfilSelector extends PatternProcessFunction<Alert, Campaign> {

    private static final long serialVersionUID = 1L;
    private static final Logger LOG = LoggerFactory.getLogger(DataExfilSelector.class);

    @Override
    public void processMatch(
            Map<String, List<Alert>> pattern,
            Context ctx,
            Collector<Campaign> out
    ) throws Exception {
        List<Alert> collectionAlerts = pattern.get("collection");
        List<Alert> transferAlerts = pattern.get("transfer");
        List<Alert> exfilAlerts = pattern.get("exfil");

        if (transferAlerts == null || transferAlerts.isEmpty() || 
            exfilAlerts == null || exfilAlerts.isEmpty()) {
            LOG.warn("Invalid data exfiltration pattern: transfer={}, exfil={}",
                    transferAlerts != null ? transferAlerts.size() : 0,
                    exfilAlerts != null ? exfilAlerts.size() : 0);
            return;
        }

        try {
            Campaign campaign = buildCampaign(collectionAlerts, transferAlerts, exfilAlerts);
            if (campaign != null) {
                out.collect(campaign);
                LOG.info("Generated Data Exfiltration Campaign: id={}, transfers={}, score={}", 
                        campaign.getCampaignId(), transferAlerts.size(), campaign.getScore());
            }
        } catch (Exception e) {
            LOG.error("Error creating data exfiltration campaign: {}", e.getMessage(), e);
        }
    }

    private Campaign buildCampaign(
            List<Alert> collectionAlerts,
            List<Alert> transferAlerts,
            List<Alert> exfilAlerts
    ) {
        // 提取所有告警
        List<Alert> allAlerts = new ArrayList<>();
        if (collectionAlerts != null) {
            allAlerts.addAll(collectionAlerts);
        }
        allAlerts.addAll(transferAlerts);
        allAlerts.addAll(exfilAlerts);

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

        // 计算评分
        float score = calculateScore(collectionAlerts, transferAlerts, exfilAlerts);

        // 生成摘要
        String summary = buildSummary(collectionAlerts, transferAlerts, exfilAlerts);

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
        List<String> attackPhases = Arrays.asList(
                "collection", "exfiltration", "command_and_control"
        );

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
                .setCampaignType(CampaignType.DATA_EXFILTRATION.getCode())
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
    private float calculateScore(
            List<Alert> collectionAlerts,
            List<Alert> transferAlerts,
            List<Alert> exfilAlerts
    ) {
        // 基础分
        float baseScore = 0.4f;
        
        // 收集阶段加分
        float collectionScore = 0.0f;
        if (collectionAlerts != null && !collectionAlerts.isEmpty()) {
            collectionScore = Math.min(collectionAlerts.size() * 0.05f, 0.15f);
        }
        
        // 传输阶段加分
        float transferScore = Math.min(transferAlerts.size() * 0.1f, 0.25f);
        
        // 外泄阶段加分
        float exfilScore = Math.min(exfilAlerts.size() * 0.1f, 0.2f);
        
        return Math.min(baseScore + collectionScore + transferScore + exfilScore, 1.0f);
    }

    /**
     * 构建摘要
     */
    private String buildSummary(
            List<Alert> collectionAlerts, 
            List<Alert> transferAlerts, 
            List<Alert> exfilAlerts
    ) {
        int collectionCount = collectionAlerts != null ? collectionAlerts.size() : 0;
        
        // 获取源主机和外泄目标
        String srcIp = "unknown";
        String exfilTarget = "unknown";
        
        if (!transferAlerts.isEmpty()) {
            srcIp = transferAlerts.get(0).getSrcIp();
        }
        if (!exfilAlerts.isEmpty()) {
            exfilTarget = exfilAlerts.get(0).getDstIp();
        }
        
        StringBuilder sb = new StringBuilder();
        sb.append(String.format("检测到数据外泄攻击链：主机 %s ", srcIp));
        
        if (collectionCount > 0) {
            sb.append(String.format("在 %d 次数据收集活动后，", collectionCount));
        }
        
        sb.append(String.format("进行了 %d 次大量数据传输，", transferAlerts.size()));
        sb.append(String.format("最终通过外部通道传输至 %s", exfilTarget));
        
        return sb.toString();
    }

    /**
     * 生成 Campaign ID
     */
    private String generateCampaignId(String tenantId, long tsStart) {
        return String.format("campaign-exfil-%s-%d-%s", 
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