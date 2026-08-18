package com.traffic.flink.log.enricher;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.traffic.proto.traffic.v1.DeviceLog;
import org.apache.flink.api.common.functions.MapFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.net.InetAddress;
import java.util.Map;

/**
 * 资产富化器 — 关联 Asset Service 为设备日志添加 tenant_id 和资产标签
 *
 * 业务价值：
 *   - 通过设备 IP 关联到租户（多租户隔离）
 *   - 添加设备角色标签 (core/access/distribution)
 *   - 计算日志严重级别映射
 */
public class AssetEnricher implements MapFunction<DeviceLog, DeviceLog> {
    private static final Logger LOG = LoggerFactory.getLogger(AssetEnricher.class);

    private static final ObjectMapper JSON = new ObjectMapper();

    // 严重级别名称映射 (Syslog severity → 人类可读)
    private static final String[] SEVERITY_NAMES = {
        "emergency", "alert", "critical", "error", "warning", "notice", "info", "debug"
    };

    // 默认租户（可通过 Asset Service 查询）
    private static final String DEFAULT_TENANT = "default";

    // 数据类型优先级: 交换机 > 路由器 > 防火墙 > 服务器 > 未知
    private static final Map<String, Integer> DEVICE_PRIORITY = Map.of(
        "switch", 3, "router", 3, "firewall", 4, "server", 2, "wireless", 1
    );

    @Override
    public DeviceLog map(DeviceLog log) {
        DeviceLog.Builder builder = log.toBuilder();

        // 1. Tenant ID: 从设备 IP 推断（生产环境通过 Asset Service gRPC 查询）
        if (log.getTenantId() == null || log.getTenantId().isEmpty()) {
            builder.setTenantId(inferTenant(log.getDeviceIp()));
        }

        // 2. Source normalization
        if (log.getSource() == null || log.getSource().isEmpty()) {
            builder.setSource("syslog");
        }

        // 3. Severity name + 富化字段：以 merge 方式扩充 parsed JSON，
        //    不得用 setParsed 整体覆盖 SyslogParser 已写入的 app/host/version。
        int sev = (int) log.getSeverity();
        if (sev >= 0 && sev < SEVERITY_NAMES.length) {
            builder.setParsed(mergeParsed(log.getParsed(),
                    SEVERITY_NAMES[sev],
                    log.getTenantId() != null ? log.getTenantId() : "",
                    DEVICE_PRIORITY.getOrDefault(log.getDeviceType(), 0)));
        }

        return builder.build();
    }

    private static String mergeParsed(String existing, String severityName, String tenant, int priority) {
        try {
            JsonNode root = (existing == null || existing.isBlank())
                    ? JSON.createObjectNode()
                    : JSON.readTree(existing);
            ObjectNode node = root.isObject() ? (ObjectNode) root : JSON.createObjectNode();
            node.put("severity_name", severityName);
            node.put("tenant", tenant);
            node.put("priority", priority);
            return JSON.writeValueAsString(node);
        } catch (Exception e) {
            // 既有 parsed 不是合法 JSON：退化为只含富化字段的新对象，
            // 不把解析失败当作成功覆盖。
            LOG.warn("Existing parsed is not valid JSON, replacing with enrichment-only object: {}", e.getMessage());
            ObjectNode node = JSON.createObjectNode();
            node.put("severity_name", severityName);
            node.put("tenant", tenant);
            node.put("priority", priority);
            try {
                return JSON.writeValueAsString(node);
            } catch (Exception fatal) {
                throw new IllegalStateException("failed to serialize parsed JSON", fatal);
            }
        }
    }

    // 根据 IP 推断租户（简化实现：根据 IP 段对应到租户）
    private String inferTenant(String ip) {
        if (ip == null) return DEFAULT_TENANT;
        try {
            byte[] addr = InetAddress.getByName(ip).getAddress();
            if (addr[0] == 10 && addr[1] == 0) return "tenant-campus-a";
            if (addr[0] == 10 && addr[1] == 1) return "tenant-campus-b";
        } catch (Exception e) {
            LOG.debug("IP resolve failed for {}: {}", ip, e.getMessage());
        }
        return DEFAULT_TENANT;
    }
}
