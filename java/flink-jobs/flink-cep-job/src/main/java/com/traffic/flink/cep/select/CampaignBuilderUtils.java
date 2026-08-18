////////////////////////////////////////////////////////////////////////////////
// FILE PATH: flink-jobs/flink-cep-job/src/main/java/com/traffic/flink/cep/select/CampaignBuilderUtils.java
////////////////////////////////////////////////////////////////////////////////

package com.traffic.flink.cep.select;

import com.traffic.flink.common.DeterministicId;
import com.traffic.proto.traffic.v1.Alert;
import com.traffic.proto.traffic.v1.EventHeader;

import java.util.*;
import java.util.stream.Collectors;

/**
 * Campaign 构建工具类
 * 
 * 提供各 Selector 共用的工具方法
 */
public final class CampaignBuilderUtils {

    private CampaignBuilderUtils() {
        // Utility class
    }

    /**
     * 提取实体（统一使用 ip: 前缀）
     */
    public static Set<String> extractEntities(List<Alert> alerts) {
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
     * 提取告警 ID 列表
     */
    public static List<String> extractAlertIds(List<Alert> alerts) {
        return alerts.stream()
                .map(Alert::getAlertId)
                .filter(id -> id != null && !id.isEmpty())
                .distinct()
                .collect(Collectors.toList());
    }

    /**
     * 提取规则 ID 列表
     */
    public static List<String> extractRuleIds(List<Alert> alerts) {
        return alerts.stream()
                .map(Alert::getRuleVersion)
                .filter(id -> id != null && !id.isEmpty())
                .distinct()
                .collect(Collectors.toList());
    }

    /**
     * 提取模型 ID 列表
     */
    public static List<String> extractModelIds(List<Alert> alerts) {
        return alerts.stream()
                .map(Alert::getModelVersion)
                .filter(id -> id != null && !id.isEmpty())
                .distinct()
                .collect(Collectors.toList());
    }

    /**
     * 构建 EventHeader
     */
    public static EventHeader buildEventHeader(String tenantId, long eventTs) {
		if (tenantId == null || tenantId.trim().isEmpty() || "unknown".equalsIgnoreCase(tenantId.trim())) {
			throw new IllegalArgumentException("campaign tenant_id is required");
		}
		tenantId = tenantId.trim();
        String eventId = DeterministicId.uuid(
                "flink-cep-header/v1", tenantId, eventTs, "campaign");
        long now = System.currentTimeMillis();
        
        return EventHeader.newBuilder()
                .setEventId(eventId)
				.setTenantId(tenantId)
                .setRunId("realtime")
                .setEventTs(eventTs)
                .setIngestTs(now)
                .setProbeId("cep-engine")
                .setFeatureSetId("campaign")
                .build();
    }

    /**
     * 获取时间范围起始时间
     */
    public static long getStartTime(List<Alert> alerts) {
        return alerts.stream()
                .mapToLong(Alert::getFirstSeen)
                .min()
                .orElse(System.currentTimeMillis());
    }

    /**
     * 获取时间范围结束时间
     */
    public static long getEndTime(List<Alert> alerts) {
        return alerts.stream()
                .mapToLong(Alert::getLastSeen)
                .max()
                .orElse(System.currentTimeMillis());
    }

    /**
     * 获取租户 ID
     */
    public static String getTenantId(List<Alert> alerts) {
        if (alerts == null || alerts.isEmpty()) {
			throw new IllegalArgumentException("campaign requires at least one alert");
        }
		String tenantId = alerts.get(0).getTenantId().trim();
		if (tenantId.isEmpty() || "unknown".equalsIgnoreCase(tenantId)) {
			throw new IllegalArgumentException("campaign tenant_id is required");
		}
		for (Alert alert : alerts) {
			if (alert == null || !tenantId.equals(alert.getTenantId().trim())) {
				throw new IllegalArgumentException("campaign pattern contains multiple tenants");
			}
		}
		return tenantId;
    }

	/**
	 * Length-delimited tenant/source key used by every CEP pattern. Empty
	 * tenant or source identities fail before keyed state admission.
	 */
	public static String tenantSourceKey(Alert alert) {
		if (alert == null) {
			throw new IllegalArgumentException("campaign alert is required");
		}
		String tenantId = alert.getTenantId().trim();
		String sourceIp = alert.getSrcIp().trim();
		if (tenantId.isEmpty() || "unknown".equalsIgnoreCase(tenantId) || sourceIp.isEmpty()) {
			throw new IllegalArgumentException("campaign tenant_id and src_ip are required");
		}
		return tenantId.length() + ":" + tenantId + sourceIp.length() + ":" + sourceIp;
	}

    /**
     * 格式化时间跨度
     */
    public static String formatDuration(long durationMs) {
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
    public static String generateCampaignId(String prefix, String tenantId, long tsStart) {
		if (tenantId == null || tenantId.trim().isEmpty() || "unknown".equalsIgnoreCase(tenantId.trim())) {
			throw new IllegalArgumentException("campaign tenant_id is required");
		}
        return String.format("campaign-%s-%s-%d-%s", 
                prefix,
                tenantId, 
                tsStart, 
                DeterministicId.shortId(
                        "flink-cep-campaign-legacy/v1", 8, prefix, tenantId, tsStart));
    }
}
