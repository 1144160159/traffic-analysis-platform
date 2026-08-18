package com.traffic.flink.log.parser;

import com.traffic.proto.traffic.v1.DeviceLog;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.apache.flink.api.common.functions.MapFunction;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Syslog 解析器 — 解析 RFC 5424 和 RFC 3164 格式
 *
 * 业务价值：将原始 syslog 消息解析为结构化字段 (facility, severity, hostname, message)
 *
 * RFC 5424: <PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG
 * RFC 3164: <PRI>TIMESTAMP HOSTNAME MSG
 */
public class SyslogParser implements MapFunction<DeviceLog, DeviceLog> {
    private static final Logger LOG = LoggerFactory.getLogger(SyslogParser.class);
    private static final ObjectMapper JSON = new ObjectMapper();

    // RFC 5424: <134>1 2024-01-01T00:00:00Z hostname app procid msgid [key="val"] message
    private static final Pattern RFC5424 = Pattern.compile(
            "<(\\d+)>(\\d+)\\s+(\\S+)\\s+(\\S+)\\s+(\\S+)\\s+(\\S+)\\s+(\\S+)\\s+(\\S+\\s+)?(.+)?");
    // RFC 3164: <134>Jan  1 00:00:00 hostname message
    private static final Pattern RFC3164 = Pattern.compile(
            "<(\\d+)>(\\w{3}\\s+\\d+\\s+\\d{2}:\\d{2}:\\d{2})\\s+(\\S+)\\s+(.+)");
    private static final Pattern PRI_PREFIX = Pattern.compile("<(\\d{1,3})>.+", Pattern.DOTALL);

    /** Consumer contract syntax check used before offsets are checkpointed. */
    public static boolean isSupportedMessage(String message) {
        if (message == null || message.trim().isEmpty()) return false;
        if (RFC5424.matcher(message).matches() || RFC3164.matcher(message).matches()) return true;
        Matcher matcher = PRI_PREFIX.matcher(message);
        if (!matcher.matches()) return false;
        try {
            int priority = Integer.parseInt(matcher.group(1));
            return priority >= 0 && priority <= 191;
        } catch (NumberFormatException error) {
            return false;
        }
    }

    @Override
    public DeviceLog map(DeviceLog log) {
        try {
            String msg = log.getMessage();
            if (msg == null || msg.isEmpty()) return log;

            DeviceLog.Builder builder = log.toBuilder();

            // Try RFC 5424 first
            Matcher m5424 = RFC5424.matcher(msg);
            if (m5424.matches() && "1".equals(m5424.group(2))) {
                int pri = Integer.parseInt(m5424.group(1));
                builder.setFacility(pri / 8)
                       .setSeverity(pri % 8)
                       .setDeviceType(inferDeviceType(m5424.group(4))) // hostname→device_type
                       .setMessage(m5424.group(9) != null ? m5424.group(9).trim() : msg)
                       // 用 Jackson 构造 parsed，避免手拼 JSON 转义不全
                       .setParsed(parsedJson("app", m5424.group(5), "host", m5424.group(4)));
                return builder.build();
            }

            // Try RFC 3164
            Matcher m3164 = RFC3164.matcher(msg);
            if (m3164.matches()) {
                int pri = Integer.parseInt(m3164.group(1));
                builder.setFacility(pri / 8)
                       .setSeverity(pri % 8)
                       .setDeviceType(inferDeviceType(m3164.group(3))) // hostname
                       .setMessage(m3164.group(4));
                return builder.build();
            }

            // Unrecognized format — keep original, extract PRI if present
            Matcher priOnly = Pattern.compile("<(\\d+)>(.+)").matcher(msg);
            if (priOnly.matches()) {
                int pri = Integer.parseInt(priOnly.group(1));
                builder.setFacility(pri / 8).setSeverity(pri % 8);
            }
            return builder.build();
        } catch (Exception e) {
            LOG.warn("Syslog parse error for log_id={}: {}", log.getLogId(), e.getMessage());
            return log;
        }
    }

    // 根据 hostname 推断设备类型
    private String inferDeviceType(String hostname) {
        if (hostname == null) return "unknown";
        String h = hostname.toLowerCase();
        if (h.contains("sw") || h.contains("switch")) return "switch";
        if (h.contains("rt") || h.contains("router") || h.contains("gw")) return "router";
        if (h.contains("fw") || h.contains("firewall") || h.contains("ngfw")) return "firewall";
        if (h.contains("srv") || h.contains("server") || h.contains("vm")) return "server";
        if (h.contains("ap") || h.contains("wlc")) return "wireless";
        return "network_device";
    }

    /**
     * 用 Jackson 构造 parsed JSON 对象，键值自动转义
     * （手拼 String.format 对含引号/控制字符的 hostname/app 会产出非法 JSON）。
     */
    private static String parsedJson(String key1, String value1, String key2, String value2) {
        try {
            com.fasterxml.jackson.databind.node.ObjectNode node =
                    JSON.createObjectNode();
            node.put(key1, value1 == null ? "" : value1);
            node.put(key2, value2 == null ? "" : value2);
            node.put("version", 1);
            return JSON.writeValueAsString(node);
        } catch (Exception e) {
            throw new IllegalStateException("failed to serialize parsed JSON", e);
        }
    }
}
